// Package ipstream scans byte streams and emits IP address and non-IP segments.
//
// The public API is stable: backward-incompatible changes will only be made
// in a new major version (v2+).
//
// # Examples
//
// See [ExampleNewStreamer] for a basic usage example.
package ipstream

import (
	"io"
	"net/netip"
	"unsafe"
)

const (
	minIPv4Len = 7  // 0.0.0.0
	maxIPv4Len = 15 // 255.255.255.255
	minIPv6Len = 2  // ::
	maxIPv6Len = 45 // ffff:ffff:ffff:ffff:ffff:ffff:255.255.255.255

	// linux allows interface names up to 15 bytes (IFNAMSIZ=16 including NUL).
	maxIPv6ZoneLen     = 15
	minIPv6WithZoneLen = minIPv6Len + 1 + 1 // "::%1"
	maxIPv6WithZoneLen = maxIPv6Len + 1 + maxIPv6ZoneLen

	// maxTokenLen is the longest scanner-supported IP spelling.
	// Any candidate token longer than this is immediately rejected as overflow.
	maxTokenLen = maxIPv6WithZoneLen
)

// Handler receives each emitted segment from the Streamer.
//
// When addr.IsValid() is true, raw was successfully parsed as an IP address and
// addr holds the result. When addr.IsValid() is false (zero netip.Addr), raw is
// a non-IP segment (delimiters, malformed candidates, or oversized text).
type Handler interface {
	Handle(raw []byte, addr netip.Addr)
}

// HandleFunc adapts a plain function to the Handler interface.
type HandleFunc func(raw []byte, addr netip.Addr)

// Handle calls the underlying function f with the segment bytes and parsed address.
func (f HandleFunc) Handle(raw []byte, addr netip.Addr) {
	f(raw, addr)
}

// Streamer scans byte streams for IPv4 and IPv6 addresses.
//
// After the final Write, call Flush to emit any trailing partial token.
// The Streamer can continue receiving Write calls after Flush.
//
// Streamer is not safe for concurrent use. Each goroutine must use its own
// Streamer instance or provide external synchronization.
type Streamer struct {
	h           Handler
	carrier     []byte
	dotCount    uint8
	colonCount  uint8
	pctCount    uint8
	colonRun    uint8
	maxColonRun uint8
	overflowing bool
}

func (s *Streamer) resetTokenState() {
	s.dotCount = 0
	s.colonCount = 0
	s.pctCount = 0
	s.colonRun = 0
	s.maxColonRun = 0
	s.overflowing = false
}

// NewStreamer creates a new Streamer with the provided segment handler.
func NewStreamer(h Handler) *Streamer {
	return &Streamer{
		h:       h,
		carrier: make([]byte, 0, maxTokenLen),
	}
}

// Write scans the byte slice p and emits complete segments (IP addresses and
// non-IP runs) to the Handler as they are identified.
func (s *Streamer) Write(p []byte) {
	ct := &charType

	for pl := len(p); pl > 0; pl = len(p) {

		// --- Overflow path: the current run exceeded maxTokenLen. ---
		// We skip over all IP-characters until we find a non-IP delimiter,
		// then reset state and continue processing the rest of p normally.
		if s.overflowing {
			i := 0
			_ = p[pl-1] // bounds check hint
			for i < pl && ct[p[i]] != 0 {
				i++
			}

			// If we found a delimiter before exhausting p, the overflow run ended.
			if i < pl {
				s.resetTokenState()
			}

			// Emit the consumed overflow bytes as a non-IP segment.
			if i > 0 {
				_ = p[i-1] // bounds check hint
				s.h.Handle(p[:i], netip.Addr{})
				p = p[i:]
				continue
			}
		}

		// --- Non-IP batching path: the first byte of p is a delimiter. ---
		// The pctCount == 0 guard prevents zone chars (NonHexAlpha, OtherIPv6ZoneChar)
		// from being treated as delimiters when inside a zone token.
		// Those chars pass through to the main scanning loop instead.

		// In non-zone context, delimiters are any chars that are not valid IP chars.
		ctDelimiter := ctIPChar
		if s.pctCount > 0 {
			// In zone context, delimiters are any chars that are not valid IPv6 zone chars.
			ctDelimiter = ctIPv6ZoneChar
		}

		if ct[p[0]]&ctDelimiter == 0 {
			if len(s.carrier) > 0 {
				s.tryParse(s.carrier)
				s.carrier = s.carrier[:0]
			}

			i := 1
			_ = p[pl-1] // bounds check hint
			for i < pl && ct[p[i]]&ctDelimiter == 0 {
				i++
			}

			_ = p[i-1] // bounds check hint
			s.h.Handle(p[:i], netip.Addr{})
			p = p[i:]
			continue
		}

		dotCount := s.dotCount
		colonCount := s.colonCount
		pctCount := s.pctCount
		colonRun := s.colonRun
		maxColonRun := s.maxColonRun

		// --- Main IP-character scanning loop ---
		// At this point, p[0] is either an IP character (digits, hex, dot, colon,
		// percent) or we are in zone context and p[0] is a zone character.
		i := 0
		_ = p[pl-1] // bounds check hint
		for ; i < pl; i++ {
			switch cType := ct[p[i]]; {
			case cType&(ctDigit|ctHexAlpha) != 0:
				// Hex character (0-9, a-f, A-F): valid in both address and zone.
				colonRun = 0
			case pctCount == 1 && cType&(ctNonHexAlpha|ctDot|ctOtherIPv6ZoneChar) != 0:
				// Zone character when inside a zone (pctCount == 1):
				// non-hex letters (g-z, G-Z), dots, and other zone chars (_, -, ~).
				// dots in a zone are NOT counted toward the address dotCount.
				colonRun = 0
			case cType == ctDot:
				// Dot character: counted as address structure.
				// In zone context (pctCount == 1), dots are handled by the zone-char
				// branch above instead of here, so zone dots don't inflate dotCount.
				dotCount++
				colonRun = 0
			case cType == ctPct:
				// Percent character: the IPv6 zone delimiter.
				// Only one % is valid; tokens with pctCount >= 2 will be rejected
				// in tryParse.
				pctCount++
				colonRun = 0
			case pctCount == 0 && cType == ctColon:
				// Colon character: only counted when outside a zone (pctCount == 0).
				// Colons inside a zone portion fall through to the else/break below.
				// This means zone text like "eth0:1" is NOT supported; the colon in
				// zone portion acts as a delimiter and terminates the token.
				colonCount++
				colonRun++
				if colonRun > maxColonRun {
					maxColonRun = colonRun
				}
			default:
				goto scanDone
			}
		}
	scanDone:

		// Write accumulated counters back to the Streamer struct.
		s.dotCount = dotCount
		s.colonCount = colonCount
		s.pctCount = pctCount
		s.colonRun = colonRun
		s.maxColonRun = maxColonRun

		switch {

		// Total candidate bytes (carrier + new) exceed maxTokenLen.
		case len(s.carrier)+i > maxTokenLen:
			// Emit carrier bytes first to preserve handler order.
			if len(s.carrier) > 0 {
				s.h.Handle(s.carrier, netip.Addr{})
				s.carrier = s.carrier[:0]
			}
			// Emit the oversized scanned bytes as non-IP.
			s.h.Handle(p[:i], netip.Addr{})
			s.resetTokenState()
			if i == pl {
				// The entire p chunk was consumed. If no delimiter was found,
				// subsequent bytes in the next Write belong to the same
				// oversized run → enter overflow mode.
				s.overflowing = true
			}
			// If i < pl, a delimiter was found at position i.
			// State was reset, so the next loop iteration will process p[i:]
			// normally (the delimiter will be handled by the non-IP batching path).

		// A delimiter broke the scan loop (i < pl means p[i] is a delimiter).
		// The bytes in p[0:i] form a complete token.
		case i < pl:
			if len(s.carrier) == 0 {
				if i > 0 {
					s.tryParse(p[:i])
				}
			} else {
				s.carrier = append(s.carrier, p[:i]...)
				s.tryParse(s.carrier)
				s.carrier = s.carrier[:0]
			}
			// p[i] is a delimiter; it will be handled in the next loop iteration
			// by the non-IP batching path.

		// The entire p was consumed by the scan loop (i == pl).
		// No delimiter was found → the token crosses a Write boundary.
		// Append scanned bytes to the carrier for the next Write call.
		default:
			s.carrier = append(s.carrier, p[:i]...)
		}

		p = p[i:]
	}
}

// Flush emits any pending partial token and leaves the Streamer ready for
// more Write calls.
func (s *Streamer) Flush() {
	if s.overflowing {
		// In overflow mode, the oversized bytes were already emitted.
		// Just reset so the Streamer can process new input cleanly.
		s.resetTokenState()
		return
	}
	if len(s.carrier) > 0 {
		// The carrier holds an incomplete token from the last Write.
		// Try to parse it (it will likely fail as incomplete) and emit.
		s.tryParse(s.carrier)
		s.carrier = s.carrier[:0]
	}
}

// Writer returns an io.Writer that delegates to the Streamer's Write method.
// The returned writer always returns len(p), nil on success.
func (s *Streamer) Writer() io.Writer {
	return writer{s: s}
}

type writer struct {
	s *Streamer
}

func (w writer) Write(p []byte) (int, error) {
	w.s.Write(p)
	return len(p), nil
}

func (s *Streamer) tryParse(raw []byte) {
	var ok bool
	var addr netip.Addr
	rawLen := len(raw)
	ct := &charType

	switch {
	case s.colonCount == 0:
		// IPv4 requires:
		//   - exactly 3 dots (dotCount)
		//   - no '%' character (pctCount == 0)
		//   - length between minIPv4Len (7) and maxIPv4Len (15)
		//   - first and last bytes must be digits
		if s.pctCount == 0 && s.dotCount == 3 &&
			rawLen >= minIPv4Len && rawLen <= maxIPv4Len &&
			(ct[raw[0]]&ctDigit) != 0 && (ct[raw[rawLen-1]]&ctDigit) != 0 {
			addr, ok = parseIPv4Fast(raw)
			recordParseIPv4Fast(ok)
		}

	case s.pctCount == 0:
		// IPv6 (no zone) requires:
		//   - 2-7 colons (the full range of possible IPv6 colon counts,
		//     with a single "::" compression counting as at least 2)
		//   - maxConsecutiveColonRun <= 2 (only "::" is valid; ":::" is not)
		//   - either 0 dots (pure hex IPv6) or 3 dots (IPv4-mapped tail like ::ffff:1.2.3.4)
		//   - length between minIPv6Len (2) and maxIPv6Len (45)
		//   - first byte must be hex-digit or colon (not dot or %)
		//   - last byte must be hex-digit or colon (not dot or %)
		//   - without dots, all-single-colon tokens (maxColonRun==1) with !=7 colons
		//     are never valid IPv6 (e.g. time-format "00:00:00")
		if s.colonCount >= 2 && s.colonCount <= 7 && s.maxColonRun <= 2 &&
			(s.dotCount == 0 || s.dotCount == 3) &&
			rawLen >= minIPv6Len && rawLen <= maxIPv6Len &&
			(ct[raw[0]]&ctHexOrColon) != 0 && (ct[raw[rawLen-1]]&ctHexOrColon) != 0 &&
			(s.dotCount != 0 || s.maxColonRun != 1 || s.colonCount == 7) {
			var err error
			addr, err = netip.ParseAddr(unsafe.String(&raw[0], rawLen)) //nolint:gosec // raw is a caller-provided byte slice, length validated above
			ok = err == nil
			recordParseAddr(ok)
		}

	case s.pctCount == 1:
		// IPv6 with zone requires the same structural checks as plain IPv6:
		//   - 2-7 colons, maxColonRun <= 2
		//   - 0 or 3 dots
		//   - first byte must be hex-digit or colon
		//   - lastType must match ctIPv6ZoneChar (hex, non-hex-alpha, dot, or
		//     other zone char like _, -, ~)
		//   - without dots, all-single-colon tokens (maxColonRun==1) with !=7 colons
		//     are never valid IPv6 (e.g. time-format "00:00:00")
		if s.colonCount >= 2 && s.colonCount <= 7 && s.maxColonRun <= 2 &&
			(s.dotCount == 0 || s.dotCount == 3) &&
			rawLen >= minIPv6WithZoneLen && rawLen <= maxIPv6WithZoneLen &&
			(ct[raw[0]]&ctHexOrColon) != 0 && (ct[raw[rawLen-1]]&ctIPv6ZoneChar) != 0 &&
			(s.dotCount != 0 || s.maxColonRun != 1 || s.colonCount == 7) {

			// Locate the '%' separator. Search backward from rawLen-2 down to
			// minIPv6Len because zone length at least 1 byte.
			pctIdx := rawLen - 2
			for pctIdx >= minIPv6Len && raw[pctIdx] != '%' {
				pctIdx--
			}

			// Validate the '%' position:
			//   - It must be after the IPv6 address
			//   - Zone length must be at most maxIPv6ZoneLen
			//   - The byte before '%' must be a hex digit or colon (valid address end)
			//   - The byte after '%' must be a valid zone character
			if pctIdx >= minIPv6Len &&
				pctIdx >= rawLen-maxIPv6ZoneLen-1 &&
				(ct[raw[pctIdx-1]]&ctHexOrColon) != 0 &&
				(ct[raw[pctIdx+1]]&ctIPv6ZoneChar) != 0 {
				var err error
				addr, err = netip.ParseAddr(unsafe.String(&raw[0], rawLen)) //nolint:gosec // raw is a caller-provided byte slice, length validated above
				ok = err == nil
				recordParseAddr(ok)
			}
		}
	}

	s.h.Handle(raw, addr)
	s.resetTokenState()
}

// parseIPv4Fast parses a dotted-decimal IPv4 address from a byte slice without
// any heap allocations. It returns the parsed address and true on success,
// or a zero address and false on failure.
//
// Validation rules:
//   - Exactly 4 dot-separated octets.
//   - No leading zeroes (e.g., "01.2.3.4" is rejected).
//   - Each octet is in the range 0-255.
//   - No trailing content after the fourth octet.
func parseIPv4Fast(b []byte) (netip.Addr, bool) {
	n := len(b)

	if n < minIPv4Len || n > maxIPv4Len {
		return netip.Addr{}, false
	}

	var a [4]byte
	i := 0

	// octet 0
	{
		d := b[i] - '0'
		if d > 9 {
			return netip.Addr{}, false
		}
		v := int(d)
		i++

		if b[i] != '.' {
			d = b[i] - '0'
			if d > 9 || v == 0 {
				return netip.Addr{}, false
			}
			v = v*10 + int(d)
			i++

			if b[i] != '.' {
				d = b[i] - '0'
				if d > 9 {
					return netip.Addr{}, false
				}
				v = v*10 + int(d)
				if v > 255 {
					return netip.Addr{}, false
				}
				i++

				if b[i] != '.' {
					return netip.Addr{}, false
				}
			}
		}

		a[0] = byte(v) //nolint:gosec // v is range-checked to 0-255 above
		i++
	}

	// octet 1
	{
		d := b[i] - '0'
		if d > 9 {
			return netip.Addr{}, false
		}
		v := int(d)
		i++

		if b[i] != '.' {
			d = b[i] - '0'
			if d > 9 || v == 0 {
				return netip.Addr{}, false
			}
			v = v*10 + int(d)
			i++

			if b[i] != '.' {
				d = b[i] - '0'
				if d > 9 {
					return netip.Addr{}, false
				}
				v = v*10 + int(d)
				if v > 255 {
					return netip.Addr{}, false
				}
				i++

				if i >= n || b[i] != '.' {
					return netip.Addr{}, false
				}
			}
		}

		a[1] = byte(v) //nolint:gosec // v is range-checked to 0-255 above
		i++
	}

	// octet 2
	{
		if i >= n {
			return netip.Addr{}, false
		}
		d := b[i] - '0'
		if d > 9 {
			return netip.Addr{}, false
		}
		v := int(d)
		i++

		if i < n && b[i] != '.' {
			d = b[i] - '0'
			if d > 9 || v == 0 {
				return netip.Addr{}, false
			}
			v = v*10 + int(d)
			i++

			if i < n && b[i] != '.' {
				d = b[i] - '0'
				if d > 9 {
					return netip.Addr{}, false
				}
				v = v*10 + int(d)
				if v > 255 {
					return netip.Addr{}, false
				}
				i++

				if i >= n || b[i] != '.' {
					return netip.Addr{}, false
				}
			}
		}

		a[2] = byte(v) //nolint:gosec // v is range-checked to 0-255 above
		i++
	}

	// octet 3
	{
		if i >= n {
			return netip.Addr{}, false
		}
		d := b[i] - '0'
		if d > 9 {
			return netip.Addr{}, false
		}
		v := int(d)
		i++

		if i < n && b[i] != '.' {
			d = b[i] - '0'
			if d > 9 || v == 0 {
				return netip.Addr{}, false
			}
			v = v*10 + int(d)
			i++

			if i < n && b[i] != '.' {
				d = b[i] - '0'
				if d > 9 {
					return netip.Addr{}, false
				}
				v = v*10 + int(d)
				if v > 255 {
					return netip.Addr{}, false
				}
				i++
			}
		}

		if i != n {
			return netip.Addr{}, false
		}
		a[3] = byte(v) //nolint:gosec // v is range-checked to 0-255 above
	}

	return netip.AddrFrom4(a), true
}

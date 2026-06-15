// Package ipstream scans byte streams and emits IP address and non-IP segments.
package ipstream

import (
	"bytes"
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
	for pl := len(p); pl > 0; pl = len(p) {

		// --- Overflow path: the current run exceeded maxTokenLen. ---
		// We skip over all IP-characters until we find a non-IP delimiter,
		// then reset state and continue processing the rest of p normally.
		if s.overflowing {
			i := 0
			_ = p[pl-1] // bounds check hint
			for i < pl && charType[p[i]] != 0 {
				i++
			}

			// If we found a delimiter before exhausting p, the overflow run ended.
			if i < pl {
				s.resetTokenState()
			}

			// Emit the consumed overflow bytes as a non-IP segment.
			if i > 0 {
				s.h.Handle(p[:i], netip.Addr{})
				p = p[i:]
				continue
			}
		}

		// --- Non-IP batching path: the first byte of p is a delimiter. ---
		// The pctCount == 0 guard prevents zone chars (NonHexAlpha, OtherIPv6ZoneChar)
		// from being treated as delimiters when inside a zone token.
		// Those chars pass through to the main scanning loop instead.
		if (s.pctCount == 0 && charType[p[0]]&ctIPChar == 0) || (s.pctCount > 0 && charType[p[0]]&ctIPv6ZoneChar == 0) {
			if len(s.carrier) > 0 {
				s.tryParse(s.carrier)
				s.carrier = s.carrier[:0]
			}

			i := 1
			_ = p[pl-1] // bounds check hint
			if s.pctCount == 0 {
				// In non-zone context, delimiters are any chars that are not valid IP chars.
				for i < pl && charType[p[i]]&ctIPChar == 0 {
					i++
				}
			} else {
				// In zone context, delimiters are any chars that are not valid IPv6 zone chars.
				for i < pl && charType[p[i]]&ctIPv6ZoneChar == 0 {
					i++
				}
			}

			s.h.Handle(p[:i], netip.Addr{})
			p = p[i:]
			continue
		}

		// --- Main IP-character scanning loop ---
		// At this point, p[0] is either an IP character (digits, hex, dot, colon,
		// percent) or we are in zone context and p[0] is a zone character.
		dotCount := s.dotCount
		colonCount := s.colonCount
		pctCount := s.pctCount
		colonRun := s.colonRun
		maxColonRun := s.maxColonRun

		i := 0
		_ = p[pl-1] // bounds check hint
		for ; i < pl; i++ {
			cType := charType[p[i]]

			// Hex character (0-9, a-f, A-F): valid in both address and zone.
			if cType&(ctDigit|ctHexAlpha) != 0 {
				colonRun = 0

				// Zone character when inside a zone (pctCount == 1):
				// non-hex letters (g-z, G-Z), dots, and other zone chars (_, -, ~).
				// dots in a zone are NOT counted toward the address dotCount.
			} else if pctCount == 1 && cType&(ctNonHexAlpha|ctDot|ctOtherIPv6ZoneChar) != 0 {
				colonRun = 0

				// Dot character: counted as address structure.
				// In zone context (pctCount == 1), dots are handled by the zone-char
				// branch above instead of here, so zone dots don't inflate dotCount.
			} else if cType == ctDot {
				dotCount++
				colonRun = 0

				// Percent character: the IPv6 zone delimiter.
				// Only one % is valid; tokens with pctCount >= 2 will be rejected
				// in tryParse.
			} else if cType == ctPct {
				pctCount++
				colonRun = 0

				// Colon character: only counted when outside a zone (pctCount == 0).
				// Colons inside a zone portion fall through to the else/break below.
				// This means zone text like "eth0:1" is NOT supported; the colon in
				// zone portion acts as a delimiter and terminates the token.
			} else if pctCount == 0 && cType == ctColon {
				colonCount++
				colonRun++
				if colonRun > maxColonRun {
					maxColonRun = colonRun
				}
			} else {
				break
			}
		}

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

func (s *Streamer) tryParse(raw []byte) {
	var ok bool
	var addr netip.Addr

	rawLen := len(raw)
	firstType := charType[raw[0]]
	lastType := charType[raw[rawLen-1]]

	switch {
	// --- IPv4: no colons in the candidate token ---
	case s.colonCount == 0:
		// IPv4 requires:
		//   - exactly 3 dots (dotCount)
		//   - no '%' character (pctCount == 0)
		//   - length between minIPv4Len (7) and maxIPv4Len (15)
		//   - first and last bytes must be digits
		if s.pctCount == 0 && s.dotCount == 3 &&
			rawLen >= minIPv4Len && rawLen <= maxIPv4Len &&
			(firstType&ctDigit) != 0 && (lastType&ctDigit) != 0 {

			addr, ok = parseIPv4Fast(raw)
			if parseStatsEnabled {
				parseIPv4FastCalls++
				if ok {
					parseIPv4FastOK++
				}
			}
		}

	// --- IPv6 without zone ID ---
	case s.pctCount == 0:
		// IPv6 (no zone) requires:
		//   - 2-7 colons (the full range of possible IPv6 colon counts,
		//     with a single "::" compression counting as at least 2)
		//   - maxConsecutiveColonRun <= 2 (only "::" is valid; ":::" is not)
		//   - either 0 dots (pure hex IPv6) or 3 dots (IPv4-mapped tail like ::ffff:1.2.3.4)
		//   - length between minIPv6Len (2) and maxIPv6Len (45)
		//   - first byte must be hex-digit or colon (not dot or %)
		//   - last byte must be hex-digit or colon (not dot or %)
		if s.colonCount >= 2 && s.colonCount <= 7 && s.maxColonRun <= 2 &&
			(s.dotCount == 0 || s.dotCount == 3) &&
			rawLen >= minIPv6Len && rawLen <= maxIPv6Len &&
			(firstType&ctHexOrColon) != 0 && (lastType&ctHexOrColon) != 0 {

			var err error
			addr, err = netip.ParseAddr(unsafe.String(&raw[0], rawLen)) //nolint:gosec
			ok = err == nil
			if parseStatsEnabled {
				parseAddrCalls++
				if ok {
					parseAddrOK++
				}
			}
		}

	// --- IPv6 with zone ID ---
	case s.pctCount == 1:
		// IPv6 with zone requires the same structural checks as plain IPv6:
		//   - 2-7 colons, maxColonRun <= 2
		//   - 0 or 3 dots
		//   - first byte must be hex-digit or colon
		//   - lastType must match ctIPv6ZoneChar (hex, non-hex-alpha, dot, or
		//     other zone char like _, -, ~)
		if s.colonCount >= 2 && s.colonCount <= 7 && s.maxColonRun <= 2 &&
			(s.dotCount == 0 || s.dotCount == 3) &&
			rawLen >= minIPv6WithZoneLen && rawLen <= maxIPv6WithZoneLen &&
			(firstType&ctHexOrColon) != 0 && (lastType&ctIPv6ZoneChar) != 0 {

			// Locate the '%' separator. We search starting from position minIPv6Len
			// because the shortest valid IPv6 address before the zone is "::" (2 bytes).
			// The offset is relative to raw[minIPv6Len:].
			pctOffset := bytes.IndexByte(raw[minIPv6Len:], '%')

			// Validate the '%' position:
			//   - It must NOT be too close to the start (zone must fit within maxIPv6ZoneLen)
			//   - It must NOT be too close to the end (at least 1 byte of zone + pctOffset offset)
			//   - The byte before '%' must be a hex digit or colon (valid address end)
			//   - The byte after '%' must be a valid zone character
			if pctOffset >= rawLen-maxIPv6ZoneLen-1-minIPv6Len &&
				pctOffset <= rawLen-2-minIPv6Len &&
				(charType[raw[minIPv6Len+pctOffset-1]]&ctHexOrColon) != 0 &&
				(charType[raw[minIPv6Len+pctOffset+1]]&ctIPv6ZoneChar) != 0 {

				var err error
				addr, err = netip.ParseAddr(unsafe.String(&raw[0], rawLen)) //nolint:gosec
				ok = err == nil
				if parseStatsEnabled {
					parseAddrCalls++
					if ok {
						parseAddrOK++
					}
				}
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

	if n < 7 || n > 15 {
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

				if i < n && b[i] != '.' {
					return netip.Addr{}, false
				}
			}
		}

		a[0] = byte(v) //nolint:gosec
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

				if i < n && b[i] != '.' {
					return netip.Addr{}, false
				}
			}
		}

		if i >= n {
			return netip.Addr{}, false
		}
		a[1] = byte(v) //nolint:gosec
		i++
	}

	// octet 2
	{
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

				if i < n && b[i] != '.' {
					return netip.Addr{}, false
				}
			}
		}

		if i >= n {
			return netip.Addr{}, false
		}
		a[2] = byte(v) //nolint:gosec
		i++
	}

	// octet 3
	{
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

				if i < n && b[i] != '.' {
					return netip.Addr{}, false
				}
			}
		}

		if i != n {
			return netip.Addr{}, false
		}
		a[3] = byte(v) //nolint:gosec
	}

	return netip.AddrFrom4(a), true
}

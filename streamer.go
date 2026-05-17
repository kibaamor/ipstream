// Package ipstream scans byte streams and emits IP address and non-IP segments.
package ipstream

import (
	"bytes"
	"net/netip"
	"unsafe"
)

const (
	minIPv4Len = 7
	maxIPv4Len = 15

	minIPv6Len = 2
	maxIPv6Len = 45

	// Scanner-enforced zone ID cap; netip.ParseAddr may accept longer zones.
	maxIPv6ZoneLen     = 15
	minIPv6WithZoneLen = minIPv6Len + 1 + 1 // "::%1"
	maxIPv6WithZoneLen = maxIPv6Len + 1 + maxIPv6ZoneLen

	// maxTokenLen is the longest scanner-supported IP spelling.
	maxTokenLen = maxIPv6WithZoneLen
)

// Handler receives each emitted segment.
// When addr.IsValid() is true, addr was parsed from raw; otherwise addr is zero.
type Handler interface {
	Handle(raw []byte, addr netip.Addr)
}

// HandleFunc adapts a function to Handler.
type HandleFunc func(raw []byte, addr netip.Addr)

// Handle calls f(raw, addr).
func (f HandleFunc) Handle(raw []byte, addr netip.Addr) {
	f(raw, addr)
}

// Streamer implements io.Writer and io.Closer for IPv4/IPv6 stream scanning.
type Streamer struct {
	h Handler

	carrier []byte

	// Current token counters, including carrier bytes.
	dotCount    uint8
	colonCount  uint8
	pctCount    uint8
	colonRun    uint8
	maxColonRun uint8

	// Current IP-character run already exceeded maxTokenLen.
	overflowing     bool
	overflowingZone bool
}

func (s *Streamer) resetTokenState() {
	s.dotCount = 0
	s.colonCount = 0
	s.pctCount = 0
	s.colonRun = 0
	s.maxColonRun = 0
	s.overflowing = false
	s.overflowingZone = false
}

// NewStreamer creates a new Streamer with the provided segment handler.
func NewStreamer(h Handler) *Streamer {
	return &Streamer{
		h:       h,
		carrier: make([]byte, 0, maxTokenLen),
	}
}

// Write scans p and emits complete segments; call Close to flush a trailing token.
func (s *Streamer) Write(p []byte) (int, error) {
	originalLen := len(p)

	for pl := len(p); pl > 0; pl = len(p) {
		if s.overflowing {
			// Skip the rest of an oversized IP-character run.
			i := 0

			_ = p[pl-1] // bounds check hint for loop below
			if s.overflowingZone {
				for i < pl && charType[p[i]]&ctIPv6ZoneChar != 0 {
					i++
				}
			} else {
				for i < pl && charType[p[i]]&ctIPChar != 0 {
					i++
				}
			}

			if i < pl {
				// Keep the delimiter run in the same non-IP segment.
				for i < pl && charType[p[i]]&ctIPChar == 0 {
					i++
				}

				s.resetTokenState()
			}
			s.h.Handle(p[:i], netip.Addr{})
			p = p[i:]
			continue
		}

		if charType[p[0]]&ctIPChar == 0 &&
			(len(s.carrier) == 0 || s.pctCount == 0 || charType[p[0]]&ctZoneChar == 0) {
			if len(s.carrier) > 0 {
				s.tryParse(s.carrier)
				s.carrier = s.carrier[:0]
			}

			i := 1
			_ = p[pl-1] // bounds check hint for loop below
			for i < pl && charType[p[i]]&ctIPChar == 0 {
				i++
			}

			s.h.Handle(p[:i], netip.Addr{})
			p = p[i:]
			continue
		}

		dotCount := s.dotCount
		colonCount := s.colonCount
		pctCount := s.pctCount
		colonRun := s.colonRun
		maxColonRun := s.maxColonRun

		i := 0
		_ = p[pl-1] // bounds check hint for loop below
	forLoop:
		for ; i < pl; i++ {
			cType := charType[p[i]]

			if cType&ctHex != 0 {
				colonRun = 0
				continue
			}

			switch cType {
			case ctNone:
				break forLoop
			case ctZoneChar, ctZoneChar | ctZoneBoundary:
				if pctCount > 0 {
					colonRun = 0
					continue
				}
				break forLoop
			case ctColon:
				colonRun++
				if colonRun > maxColonRun {
					maxColonRun = colonRun
				}
				colonCount++
			case ctDot:
				colonRun = 0
				dotCount++
			case ctPct:
				colonRun = 0
				pctCount++
			}
		}

		s.dotCount = dotCount
		s.colonCount = colonCount
		s.pctCount = pctCount
		s.colonRun = colonRun
		s.maxColonRun = maxColonRun

		switch {
		case len(s.carrier)+i > maxTokenLen:
			overflowingZone := pctCount > 0
			// Emit carried bytes first to preserve handler order.
			if len(s.carrier) > 0 {
				s.h.Handle(s.carrier, netip.Addr{})
				s.carrier = s.carrier[:0]
			}
			s.h.Handle(p[:i], netip.Addr{})
			s.resetTokenState()
			if i == pl {
				// No delimiter yet; the oversized run continues in the next Write.
				s.overflowing = true
				s.overflowingZone = overflowingZone
			}
		case i < pl:
			if len(s.carrier) == 0 {
				s.tryParse(p[:i])
			} else {
				s.carrier = append(s.carrier, p[:i]...)
				s.tryParse(s.carrier)
				s.carrier = s.carrier[:0]
			}
		default:
			s.carrier = append(s.carrier, p[:i]...)
		}

		p = p[i:]
	}

	return originalLen, nil
}

// Close flushes any pending token data.
func (s *Streamer) Close() error {
	if s.overflowing {
		s.resetTokenState()
		return nil
	}
	if len(s.carrier) > 0 {
		s.tryParse(s.carrier)
		s.carrier = s.carrier[:0]
	}
	return nil
}

func (s *Streamer) tryParse(raw []byte) {
	var ok bool
	var addr netip.Addr

	rawLen := len(raw)
	firstType := charType[raw[0]]
	lastType := charType[raw[rawLen-1]]

	// Pure IPv4 stays on the allocation-free parser.
	if s.colonCount == 0 && s.pctCount == 0 && s.dotCount == 3 &&
		rawLen >= minIPv4Len && rawLen <= maxIPv4Len &&
		(firstType&ctDigit) != 0 && (lastType&ctDigit) != 0 {
		addr, ok = parseIPv4Fast(raw)
		if parseStatsEnabled {
			parseIPv4FastCalls++
			if ok {
				parseIPv4FastOK++
			}
		}
	} else if s.pctCount == 0 &&
		s.colonCount >= 2 && s.colonCount <= 7 && s.maxColonRun <= 2 &&
		(s.dotCount == 3 || s.colonCount == 7 || s.maxColonRun == 2) {

		if (s.dotCount == 0 || s.dotCount == 3) &&
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
	} else if s.pctCount == 1 &&
		rawLen >= minIPv6WithZoneLen && rawLen <= maxIPv6WithZoneLen &&
		(firstType&ctHexOrColon) != 0 && lastType&ctIPv6ZoneBoundary != 0 {
		pctOffset := bytes.IndexByte(raw[minIPv6Len:], '%')
		if pctOffset >= 0 && pctOffset >= rawLen-maxIPv6ZoneLen-1-minIPv6Len &&
			pctOffset <= rawLen-2-minIPv6Len {
			pctIndex := minIPv6Len + pctOffset
			if (charType[raw[pctIndex-1]]&ctHexOrColon) != 0 &&
				charType[raw[pctIndex+1]]&ctIPv6ZoneBoundary != 0 {
				addrColonCount, addrMaxColonRun := ipv6ColonStats(raw[:pctIndex])
				if addrColonCount >= 2 && addrColonCount <= 7 && addrMaxColonRun <= 2 {
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
	}

	s.h.Handle(raw, addr)
	s.resetTokenState()
}

func ipv6ColonStats(raw []byte) (colonCount, maxColonRun uint8) {
	var colonRun uint8
	for _, c := range raw {
		if c != ':' {
			colonRun = 0
			continue
		}
		colonRun++
		if colonRun > maxColonRun {
			maxColonRun = colonRun
		}
		colonCount++
	}
	return colonCount, maxColonRun
}

// parseIPv4Fast parses a dotted-decimal IPv4 address without leading zeroes.
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

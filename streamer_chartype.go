package ipstream

const (
	ctNone  = 0
	ctDigit = 1 << iota
	ctHexAlpha
	ctDot
	ctColon
	ctPct
	ctZoneChar
	ctZoneBoundary

	ctHex              = ctDigit | ctHexAlpha
	ctHexOrColon       = ctHex | ctColon
	ctIPChar           = ctHex | ctDot | ctColon | ctPct
	ctIPv6ZoneChar     = ctHex | ctDot | ctColon | ctZoneChar
	ctIPv6ZoneBoundary = ctHex | ctZoneBoundary
)

// charType marks bytes allowed inside IP candidate tokens and IPv6 zones.
var charType [256]byte

func init() {
	for i := range 256 {
		c := byte(i)
		switch {
		case c >= '0' && c <= '9':
			charType[i] = ctDigit
		case c >= 'a' && c <= 'f', c >= 'A' && c <= 'F':
			charType[i] = ctHexAlpha
		case c == '.':
			charType[i] = ctDot
		case c == ':':
			charType[i] = ctColon
		case c == '%':
			charType[i] = ctPct
		}

		// Zone chars: g-z/G-Z can start/end zones (ctZoneChar | ctZoneBoundary),
		// while _/- can only continue them (ctZoneChar only).
		switch {
		case c >= 'g' && c <= 'z', c >= 'G' && c <= 'Z':
			charType[i] = ctZoneChar | ctZoneBoundary
		case c == '_' || c == '-':
			charType[i] |= ctZoneChar
		}
	}
}

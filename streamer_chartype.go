package ipstream

const (
	// Character type flags for charType table.
	ctDigit byte = 1 << iota
	ctHexAlpha
	ctNonHexAlpha
	ctDot
	ctColon
	ctPct
	ctOtherIPv6ZoneChar

	// Composite character type flags.
	ctHex          = ctDigit | ctHexAlpha
	ctHexOrColon   = ctHex | ctColon
	ctIPChar       = ctHex | ctDot | ctColon | ctPct
	ctIPv6ZoneChar = ctHex | ctNonHexAlpha | ctDot | ctOtherIPv6ZoneChar
)

// charType marks bytes allowed inside IP candidate tokens and IPv6 zones.
var charType [256]byte

func init() {
	for i := 0; i < 256; i++ {
		c := byte(i)
		switch {
		case c >= '0' && c <= '9':
			charType[i] = ctDigit
		case c >= 'a' && c <= 'f', c >= 'A' && c <= 'F':
			charType[i] = ctHexAlpha
		case c >= 'g' && c <= 'z', c >= 'G' && c <= 'Z':
			charType[i] = ctNonHexAlpha
		case c == '.':
			charType[i] = ctDot
		case c == ':':
			charType[i] = ctColon
		case c == '%':
			charType[i] = ctPct
		case c == '_', c == '-', c == '~':
			charType[i] = ctOtherIPv6ZoneChar
		}
	}
}

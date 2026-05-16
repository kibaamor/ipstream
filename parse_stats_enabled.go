//go:build ipstreamstats

package ipstream

const parseStatsEnabled = true

// ParseStats reports parser call and success counters.
type ParseStats struct {
	IPv4FastCalls  uint64
	IPv4FastOK     uint64
	ParseAddrCalls uint64
	ParseAddrOK    uint64
}

var (
	parseIPv4FastCalls uint64
	parseIPv4FastOK    uint64
	parseAddrCalls     uint64
	parseAddrOK        uint64
)

// ResetParseStats clears parser counters.
func ResetParseStats() {
	parseIPv4FastCalls = 0
	parseIPv4FastOK = 0
	parseAddrCalls = 0
	parseAddrOK = 0
}

// ParseStatsSnapshot returns the current parser counters.
func ParseStatsSnapshot() ParseStats {
	return ParseStats{
		IPv4FastCalls:  parseIPv4FastCalls,
		IPv4FastOK:     parseIPv4FastOK,
		ParseAddrCalls: parseAddrCalls,
		ParseAddrOK:    parseAddrOK,
	}
}

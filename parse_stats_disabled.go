//go:build !ipstreamstats

package ipstream

const parseStatsEnabled = false

//nolint:unused // Referenced only from compile-time-disabled stats branches.
var (
	parseIPv4FastCalls uint64
	parseIPv4FastOK    uint64
	parseAddrCalls     uint64
	parseAddrOK        uint64
)

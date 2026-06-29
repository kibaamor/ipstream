//go:build ipstreamstats

package ipstream

import "sync/atomic"

// ParseStats reports parser call and success counters.
type ParseStats struct {
	IPv4FastCalls  uint64
	IPv4FastOK     uint64
	ParseAddrCalls uint64
	ParseAddrOK    uint64
}

var (
	parseIPv4FastCalls atomic.Uint64
	parseIPv4FastOK    atomic.Uint64
	parseAddrCalls     atomic.Uint64
	parseAddrOK        atomic.Uint64
)

// ResetParseStats clears parser counters.
func ResetParseStats() {
	parseIPv4FastCalls.Store(0)
	parseIPv4FastOK.Store(0)
	parseAddrCalls.Store(0)
	parseAddrOK.Store(0)
}

// ParseStatsSnapshot returns the current parser counters.
func ParseStatsSnapshot() ParseStats {
	return ParseStats{
		IPv4FastCalls:  parseIPv4FastCalls.Load(),
		IPv4FastOK:     parseIPv4FastOK.Load(),
		ParseAddrCalls: parseAddrCalls.Load(),
		ParseAddrOK:    parseAddrOK.Load(),
	}
}

func recordParseIPv4Fast(ok bool) {
	parseIPv4FastCalls.Add(1)
	if ok {
		parseIPv4FastOK.Add(1)
	}
}

func recordParseAddr(ok bool) {
	parseAddrCalls.Add(1)
	if ok {
		parseAddrOK.Add(1)
	}
}

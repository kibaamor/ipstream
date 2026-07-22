//go:build ipstreamstats

package ipstream

import (
	"fmt"
	"os"
	"testing"
)

var overallBenchParseStats parseStats

func TestMain(m *testing.M) {
	code := m.Run()
	if overallBenchParseStats.IPv4FastCalls != 0 || overallBenchParseStats.ParseAddrCalls != 0 {
		fmt.Fprintf(os.Stderr,
			"parse stats overall: ipv4fast_calls=%d ipv4fast_ok=%d ipv4fast_ok%%=%.2f parseaddr_calls=%d parseaddr_ok=%d parseaddr_ok%%=%.2f\n",
			overallBenchParseStats.IPv4FastCalls,
			overallBenchParseStats.IPv4FastOK,
			percent(overallBenchParseStats.IPv4FastOK, overallBenchParseStats.IPv4FastCalls),
			overallBenchParseStats.ParseAddrCalls,
			overallBenchParseStats.ParseAddrOK,
			percent(overallBenchParseStats.ParseAddrOK, overallBenchParseStats.ParseAddrCalls),
		)
	}
	os.Exit(code)
}

func resetBenchParseStats() {
	resetParseStats()
}

func reportBenchParseStats(b *testing.B, iterations int) {
	b.Helper()
	if iterations == 0 {
		return
	}

	stats := parseStatsSnapshot()
	overallBenchParseStats.IPv4FastCalls += stats.IPv4FastCalls
	overallBenchParseStats.IPv4FastOK += stats.IPv4FastOK
	overallBenchParseStats.ParseAddrCalls += stats.ParseAddrCalls
	overallBenchParseStats.ParseAddrOK += stats.ParseAddrOK

	b.ReportMetric(float64(stats.IPv4FastCalls)/float64(iterations), "ipv4fast_calls/op")
	b.ReportMetric(percent(stats.IPv4FastOK, stats.IPv4FastCalls), "ipv4fast_ok_%")
	b.ReportMetric(float64(stats.ParseAddrCalls)/float64(iterations), "parseaddr_calls/op")
	b.ReportMetric(percent(stats.ParseAddrOK, stats.ParseAddrCalls), "parseaddr_ok_%")
}

func percent(n, d uint64) float64 {
	if d == 0 {
		return 0
	}
	return 100 * float64(n) / float64(d)
}

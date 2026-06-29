//go:build ipstreamtests && ipstreamstats
// +build ipstreamtests,ipstreamstats

package ipstream_test

import (
	"net/netip"
	"strings"
	"sync"
	"testing"

	"github.com/kibaamor/ipstream"
)

func TestParseStats_ConcurrentStreamers(t *testing.T) {
	const (
		goroutines       = 8
		addressesPerFeed = 100
	)

	ipstream.ResetParseStats()

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			s := ipstream.NewStreamer(ipstream.HandleFunc(func([]byte, netip.Addr) {}))
			s.Write([]byte(strings.Repeat("1.2.3.4 ", addressesPerFeed)))
			s.Flush()
		}()
	}
	wg.Wait()

	stats := ipstream.ParseStatsSnapshot()
	want := uint64(goroutines * addressesPerFeed)
	if stats.IPv4FastCalls != want || stats.IPv4FastOK != want {
		t.Fatalf("IPv4 stats = calls %d ok %d, want %d/%d", stats.IPv4FastCalls, stats.IPv4FastOK, want, want)
	}
}

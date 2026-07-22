//go:build ipstreamtests && ipstreamstats

package ipstream

import (
	"net/netip"
	"strings"
	"sync"
	"testing"
)

func TestParseStats_ConcurrentStreamers(t *testing.T) {
	const (
		goroutines       = 8
		addressesPerFeed = 100
	)

	resetParseStats()

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			s := NewStreamer(HandleFunc(func([]byte, netip.Addr) {}))
			s.Write([]byte(strings.Repeat("1.2.3.4 ", addressesPerFeed)))
			s.Flush()
		}()
	}
	wg.Wait()

	stats := parseStatsSnapshot()
	want := uint64(goroutines * addressesPerFeed)
	if stats.IPv4FastCalls != want || stats.IPv4FastOK != want {
		t.Fatalf("IPv4 stats = calls %d ok %d, want %d/%d", stats.IPv4FastCalls, stats.IPv4FastOK, want, want)
	}
}

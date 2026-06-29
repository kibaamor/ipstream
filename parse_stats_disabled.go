//go:build !ipstreamstats

package ipstream

func recordParseIPv4Fast(_ bool) {}

func recordParseAddr(_ bool) {}

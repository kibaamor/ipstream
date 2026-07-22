//go:build !ipstreamstats

package ipstream

import "testing"

func resetBenchParseStats() {}

func reportBenchParseStats(_ *testing.B, _ int) {}

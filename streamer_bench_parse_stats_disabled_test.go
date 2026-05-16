//go:build ipstreamtests && !ipstreamstats
// +build ipstreamtests,!ipstreamstats

package ipstream_test

import "testing"

func resetBenchParseStats() {}

func reportBenchParseStats(_ *testing.B, _ int) {}

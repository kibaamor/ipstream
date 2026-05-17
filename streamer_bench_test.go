//go:build ipstreamtests
// +build ipstreamtests

package ipstream_test

import (
	"net/netip"
	"strings"
	"testing"

	"github.com/kibaamor/ipstream"
)

// benchSink prevents Handler calls from being optimised away.
var benchSink int

func benchHandle(raw []byte, addr netip.Addr) {
	benchSink += len(raw)
	if addr.IsValid() {
		benchSink++
	}
}

func benchmarkWrite(b *testing.B, input []byte) {
	b.SetBytes(int64(len(input)))
	b.ReportAllocs()
	resetBenchParseStats()
	b.ResetTimer()
	for b.Loop() {
		s := ipstream.NewStreamer(ipstream.HandleFunc(benchHandle))
		s.Write(input)
		s.Flush()
	}
	b.StopTimer()
	reportBenchParseStats(b, b.N)
}

func benchmarkWriteChunks(b *testing.B, data []byte, chunks [][]byte) {
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	resetBenchParseStats()
	b.ResetTimer()
	for b.Loop() {
		s := ipstream.NewStreamer(ipstream.HandleFunc(benchHandle))
		for _, chunk := range chunks {
			s.Write(chunk)
		}
		s.Flush()
	}
	b.StopTimer()
	reportBenchParseStats(b, b.N)
}

func fixedChunks(data []byte, size int) [][]byte {
	chunks := make([][]byte, 0, len(data)/size+1)
	for off := 0; off < len(data); off += size {
		end := min(off+size, len(data))
		chunks = append(chunks, data[off:end])
	}
	return chunks
}

func benchmarkWriteByteByByte(b *testing.B, data []byte) {
	buf := make([]byte, 1)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	resetBenchParseStats()
	b.ResetTimer()
	for b.Loop() {
		s := ipstream.NewStreamer(ipstream.HandleFunc(benchHandle))
		for _, c := range data {
			buf[0] = c
			s.Write(buf)
		}
		s.Flush()
	}
	b.StopTimer()
	reportBenchParseStats(b, b.N)
}

func benchmarkWriteSplitAt(b *testing.B, data []byte, split int) {
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	resetBenchParseStats()
	b.ResetTimer()
	for b.Loop() {
		s := ipstream.NewStreamer(ipstream.HandleFunc(benchHandle))
		s.Write(data[:split])
		s.Write(data[split:])
		s.Flush()
	}
	b.StopTimer()
	reportBenchParseStats(b, b.N)
}

func benchmarkWriteSplitAfter(b *testing.B, data []byte, token string, prefixLen int) {
	b.Helper()

	tokenStart := strings.Index(string(data), token)
	if tokenStart < 0 {
		b.Fatalf("token %q not found in benchmark data", token)
	}
	split := tokenStart + prefixLen
	benchmarkWriteSplitAt(b, data, split)
}

var (
	// Most datasets end with a delimiter; *_NoTrailingDelimiter measures Flush.
	//
	// ~40 KB — dense IPv4 addresses separated by spaces.
	denseIPv4Input                    = []byte(strings.Repeat("192.168.1.1 10.0.0.2 172.16.0.3 8.8.8.8 ", 1000))
	denseIPv4NoTrailingDelimiterInput = denseIPv4Input[:len(denseIPv4Input)-1]

	// ~24 KB — dense IPv6 addresses.
	denseIPv6Input                    = []byte(strings.Repeat("::1 2001:db8::1 fe80::1 ", 1000))
	denseIPv6NoTrailingDelimiterInput = denseIPv6Input[:len(denseIPv6Input)-1]

	// ~40 KB — mixed IPv4, IPv6, and plain-text fields (structured log style).
	mixedInput = []byte(strings.Repeat("src=192.168.0.1 dst=::1 status=200 msg=ok\n", 1000))

	// ~67 KB — broad mix of valid IP spellings.
	allValidIPFormsInput = []byte(strings.Repeat(
		"0.0.0.0 255.255.255.255 :: ::1 2001:db8:: 2001:DB8::ABCD "+
			"::192.0.2.1 ::ffff:192.0.2.128 64:ff9b::192.0.2.33 "+
			"fe80::1%1 fe80::1%abc.def FE80::ABCD%ABC.DEF ",
		300,
	))

	// ~61 KB — invalid scanner-contiguous IP-like tokens.
	allInvalidIPFormsInput = []byte(strings.Repeat(
		"192.168.001.001 1.2.3.4. 1..2.3 2001:db8:::1 2001:db8::1:: "+
			"12345:: 1:2:3:4:5:6:7 12:34:56 ::ffff:999.1.1.1 "+
			"fe80::1% fe80::1%abcdefabcdefabcd fe80::1%1: ",
		300,
	))

	// ~32 KB — realistic access-log lines; one IPv4 per line among other text.
	sparseInput = []byte(strings.Repeat(
		"2024-01-01T00:00:00Z GET /path HTTP/1.1 200 1024 - 192.168.1.100\n",
		500,
	))

	// ~48 KB — plain prose with no valid IP addresses.
	noIPInput = []byte(strings.Repeat("nothing to see here, just logs flowing through\n", 1000))

	// ~34 KB — hex-dotted tokens rejected by IPv4 digit checks.
	falseIPv4Input = []byte(strings.Repeat("dead.beef.cafe.1 dead.beef.cafe.2 ", 1000))

	// ~37 KB — time-like tokens rejected before ParseAddr.
	timestampInput = []byte(strings.Repeat("00:00:00 01:23:45 12:59:59 23:00:01 ", 1000))

	// ~40 KB — fully-expanded IPv6.
	fullIPv6Input = []byte(strings.Repeat("2001:0db8:85a3:0000:0000:8a2e:0370:7334 ", 1000))

	// ~35 KB — IPv4-mapped IPv6.
	mappedIPv6Input = []byte(strings.Repeat("::ffff:192.168.1.1 ::ffff:10.0.0.2 ", 1000))

	// ~30 KB — zone-scoped IPv6.
	zonedIPv6Input = []byte(strings.Repeat("fe80::1%1 fe80::2%2 fe80::3%3 ", 1000))

	// ~25 KB — uppercase IPv6 spellings.
	uppercaseIPv6Input = []byte(strings.Repeat("2001:DB8::ABCD FE80::1 ", 1000))

	// ~38 KB — compressed IPv6 addresses with embedded IPv4 dotted-quad tails.
	dottedTailIPv6Input = []byte(strings.Repeat("::192.0.2.1 64:ff9b::192.0.2.33 ", 1000))

	// ~38 KB — colon-heavy tokens rejected before ParseAddr.
	shortNoCompressionIPv6Input = []byte(strings.Repeat("1:2:3:4:5:6:7 00:00:00 12:34:56 ", 1000))

	// ~38 KB — zone IDs containing dots and uppercase hex letters.
	dottedZoneIPv6Input = []byte(strings.Repeat("fe80::1%abc.def FE80::ABCD%ABC.DEF ", 1000))

	// ~34 KB — pure hex tokens rejected immediately.
	hexOnlyInput = []byte(strings.Repeat("deadbeefcafebabe cafebabe0deadc0de ", 1000))

	// ~40 KB — 72-char hex tokens triggering overflow.
	overflowTokenInput = []byte(strings.Repeat(strings.Repeat("aabbccddeeff", 6)+" ", 550))

	// ~45 KB — overflow followed by valid IPv4.
	overflowThenValidInput = []byte(strings.Repeat(strings.Repeat("aabbccddeeff", 6)+" 1.2.3.4 ", 500))

	// ~40 KB — densely packed "::1".
	shortIPv6Input = []byte(strings.Repeat("::1 ", 10000))

	// ~42 KB — leading-zero IPv4 tokens.
	leadingZeroIPv4Input = []byte(strings.Repeat("192.168.01.1 010.020.030.040 ", 1500))

	// ~43 KB — delimiter-only input.
	allDelimitersInput = []byte(strings.Repeat("                \n", 2500))

	// ~42 KB — one IPv4 per 512-byte block.
	extremeSparseInput = []byte(strings.Repeat(strings.Repeat(" ", 511)+"192.168.1.1\n", 80))

	// Length-boundary focused datasets.
	minIPv4Input = []byte(strings.Repeat("0.0.0.0 ", 6000))
	maxIPv4Input = []byte(strings.Repeat("255.255.255.255 ", 3000))

	minIPv6Input = []byte(strings.Repeat(":: ", 12000))
	maxIPv6Input = []byte(strings.Repeat("ffff:ffff:ffff:ffff:ffff:ffff:255.255.255.255 ", 1000))

	// Scanner zone-id boundary (15 chars) and too-long rejection (16 chars).
	maxZoneIPv6Input  = []byte(strings.Repeat("fe80::1%1234567890abcde ", 2000))
	longZoneIPv6Input = []byte(strings.Repeat("fe80::1%1234567890abcdef ", 2000))

	// IPv4 length rejection.
	shortLenIPv4Input = []byte(strings.Repeat("1.1.1. ", 6000))
	longLenIPv4Input  = []byte(strings.Repeat("1111.1111.1111.1111 ", 2000))

	// IPv6 length rejection.
	longLenIPv6Input = []byte(strings.Repeat("ffff:ffff:ffff:ffff:ffff:ffff:255.255.255.255f ", 900))

	// Boundary-character rejection datasets.
	badIPv4BoundaryInput = []byte(strings.Repeat("a1.2.3.4 1.2.3.a ", 2500))
	badIPv6BoundaryInput = []byte(strings.Repeat(".::1 ::1. ", 5000))
	badZoneBoundaryInput = []byte(strings.Repeat(".fe80::1%1 fe80::1%1: ", 2000))

	// Zoned IPv6 total-length boundaries.
	maxTotalZoneIPv6Input = []byte(strings.Repeat(
		"ffff:ffff:ffff:ffff:ffff:ffff:255.255.255.255%1234567890abcde ",
		600,
	))
	tooLongTotalZoneIPv6Input = []byte(strings.Repeat(
		"ffff:ffff:ffff:ffff:ffff:ffff:255.255.255.255%1234567890abcdef ",
		600,
	))
)

// BenchmarkWrite_DenseIPv4 measures dense IPv4 throughput.
func BenchmarkWrite_DenseIPv4(b *testing.B) {
	benchmarkWrite(b, denseIPv4Input)
}

func BenchmarkWrite_DenseIPv4_NoTrailingDelimiter(b *testing.B) {
	benchmarkWrite(b, denseIPv4NoTrailingDelimiterInput)
}

// BenchmarkWrite_DenseIPv6 measures dense IPv6 throughput.
func BenchmarkWrite_DenseIPv6(b *testing.B) {
	benchmarkWrite(b, denseIPv6Input)
}

func BenchmarkWrite_DenseIPv6_NoTrailingDelimiter(b *testing.B) {
	benchmarkWrite(b, denseIPv6NoTrailingDelimiterInput)
}

// BenchmarkWrite_Mixed measures structured-log throughput.
func BenchmarkWrite_Mixed(b *testing.B) {
	benchmarkWrite(b, mixedInput)
}

func BenchmarkWrite_AllValidIPForms(b *testing.B) {
	benchmarkWrite(b, allValidIPFormsInput)
}

func BenchmarkWrite_AllInvalidIPForms(b *testing.B) {
	benchmarkWrite(b, allInvalidIPFormsInput)
}

// BenchmarkWrite_SparseIPs measures sparse IP throughput.
func BenchmarkWrite_SparseIPs(b *testing.B) {
	benchmarkWrite(b, sparseInput)
}

// BenchmarkWrite_NoIPs measures non-IP throughput.
func BenchmarkWrite_NoIPs(b *testing.B) {
	benchmarkWrite(b, noIPInput)
}

// BenchmarkWrite_SmallChunks measures 64-byte Write calls.
func BenchmarkWrite_SmallChunks(b *testing.B) {
	const chunkSize = 64
	data := denseIPv4Input
	benchmarkWriteChunks(b, data, fixedChunks(data, chunkSize))
}

// BenchmarkWrite_ByteByByte measures one-byte Write calls.
func BenchmarkWrite_ByteByByte(b *testing.B) {
	data := []byte("192.168.1.1 ::1 10.0.0.1 ")
	benchmarkWriteByteByByte(b, data)
}

func BenchmarkWrite_IPv4SplitAcrossWrites(b *testing.B) {
	data := []byte("prefix 192.168.1.1 suffix")
	benchmarkWriteSplitAfter(b, data, "192.168.1.1", len("192.168"))
}

func BenchmarkWrite_IPv6SplitAcrossWrites(b *testing.B) {
	data := []byte("prefix 2001:db8::1 suffix")
	benchmarkWriteSplitAfter(b, data, "2001:db8::1", len("2001:db8:"))
}

// BenchmarkWrite_FalsePositiveIPv4 measures hex-dotted rejection.
func BenchmarkWrite_FalsePositiveIPv4(b *testing.B) {
	benchmarkWrite(b, falseIPv4Input)
}

// BenchmarkWrite_FalsePositiveIPv6 measures time-like rejection.
func BenchmarkWrite_FalsePositiveIPv6(b *testing.B) {
	benchmarkWrite(b, timestampInput)
}

// BenchmarkWrite_FullyExpandedIPv6 measures full IPv6 throughput.
func BenchmarkWrite_FullyExpandedIPv6(b *testing.B) {
	benchmarkWrite(b, fullIPv6Input)
}

// BenchmarkWrite_IPv4MappedIPv6 measures dotted-tail IPv6 throughput.
func BenchmarkWrite_IPv4MappedIPv6(b *testing.B) {
	benchmarkWrite(b, mappedIPv6Input)
}

// BenchmarkWrite_ZonedIPv6 measures numeric-zone IPv6 throughput.
func BenchmarkWrite_ZonedIPv6(b *testing.B) {
	benchmarkWrite(b, zonedIPv6Input)
}

func BenchmarkWrite_UppercaseIPv6(b *testing.B) {
	benchmarkWrite(b, uppercaseIPv6Input)
}

func BenchmarkWrite_DottedTailIPv6(b *testing.B) {
	benchmarkWrite(b, dottedTailIPv6Input)
}

func BenchmarkWrite_IPv6ShortNoCompressionRejected(b *testing.B) {
	benchmarkWrite(b, shortNoCompressionIPv6Input)
}

func BenchmarkWrite_ZonedIPv6_DottedZone(b *testing.B) {
	benchmarkWrite(b, dottedZoneIPv6Input)
}

// BenchmarkWrite_HexOnlyTokens measures minimum-overhead rejection.
func BenchmarkWrite_HexOnlyTokens(b *testing.B) {
	benchmarkWrite(b, hexOnlyInput)
}

// BenchmarkWrite_OverflowTokens measures oversized-token handling.
func BenchmarkWrite_OverflowTokens(b *testing.B) {
	benchmarkWrite(b, overflowTokenInput)
}

func BenchmarkWrite_OverflowThenValidToken(b *testing.B) {
	benchmarkWrite(b, overflowThenValidInput)
}

// BenchmarkWrite_ShortIPv6 measures minimal IPv6 parse cost.
func BenchmarkWrite_ShortIPv6(b *testing.B) {
	benchmarkWrite(b, shortIPv6Input)
}

// BenchmarkWrite_IPv4LeadingZero measures leading-zero rejection.
func BenchmarkWrite_IPv4LeadingZero(b *testing.B) {
	benchmarkWrite(b, leadingZeroIPv4Input)
}

// BenchmarkWrite_AllDelimiters measures delimiter-only overhead.
func BenchmarkWrite_AllDelimiters(b *testing.B) {
	benchmarkWrite(b, allDelimitersInput)
}

// BenchmarkWrite_ExtremeSparse measures mostly-delimiter input.
func BenchmarkWrite_ExtremeSparse(b *testing.B) {
	benchmarkWrite(b, extremeSparseInput)
}

func BenchmarkWrite_MinIPv4Boundary(b *testing.B) {
	benchmarkWrite(b, minIPv4Input)
}

func BenchmarkWrite_MaxIPv4Boundary(b *testing.B) {
	benchmarkWrite(b, maxIPv4Input)
}

func BenchmarkWrite_MinIPv6Boundary(b *testing.B) {
	benchmarkWrite(b, minIPv6Input)
}

func BenchmarkWrite_MaxIPv6Boundary(b *testing.B) {
	benchmarkWrite(b, maxIPv6Input)
}

func BenchmarkWrite_ZonedIPv6_MaxZoneBoundary(b *testing.B) {
	benchmarkWrite(b, maxZoneIPv6Input)
}

func BenchmarkWrite_ZonedIPv6_TooLongZoneRejected(b *testing.B) {
	benchmarkWrite(b, longZoneIPv6Input)
}

func BenchmarkWrite_IPv4TooShortLengthRejected(b *testing.B) {
	benchmarkWrite(b, shortLenIPv4Input)
}

func BenchmarkWrite_IPv4TooLongLengthRejected(b *testing.B) {
	benchmarkWrite(b, longLenIPv4Input)
}

func BenchmarkWrite_IPv6TooLongLengthRejected(b *testing.B) {
	benchmarkWrite(b, longLenIPv6Input)
}

func BenchmarkWrite_IPv4BoundaryRejected(b *testing.B) {
	benchmarkWrite(b, badIPv4BoundaryInput)
}

func BenchmarkWrite_IPv6BoundaryRejected(b *testing.B) {
	benchmarkWrite(b, badIPv6BoundaryInput)
}

func BenchmarkWrite_ZonedIPv6BoundaryRejected(b *testing.B) {
	benchmarkWrite(b, badZoneBoundaryInput)
}

func BenchmarkWrite_ZonedIPv6_MaxTotalLengthBoundary(b *testing.B) {
	benchmarkWrite(b, maxTotalZoneIPv6Input)
}

func BenchmarkWrite_ZonedIPv6_TooLongTotalLengthRejected(b *testing.B) {
	benchmarkWrite(b, tooLongTotalZoneIPv6Input)
}

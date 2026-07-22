package ipstream_test

import (
	"fmt"
	"net/netip"
	"strings"
	"testing"

	"github.com/kibaamor/ipstream"
)

// call captures a single Handler invocation.
type call struct {
	raw  string
	addr netip.Addr
}

// newStreamer returns a Streamer and a snapshot accessor for recorded calls.
func newStreamer() (*ipstream.Streamer, func() []call) {
	var calls []call
	s := ipstream.NewStreamer(ipstream.HandleFunc(func(raw []byte, addr netip.Addr) {
		calls = append(calls, call{string(raw), addr})
	}))
	return s, func() []call { return calls }
}

// reconstruct joins all raw fields in order; must equal the original input.
func reconstruct(calls []call) string {
	var b strings.Builder
	for _, c := range calls {
		b.WriteString(c.raw)
	}
	return b.String()
}

// writeAll writes in one shot and flushes the streamer.
func writeAll(t *testing.T, s *ipstream.Streamer, input string) {
	t.Helper()
	s.Write([]byte(input))
	s.Flush()
}

func writeChunks(t *testing.T, s *ipstream.Streamer, chunks ...string) {
	t.Helper()
	for _, chunk := range chunks {
		s.Write([]byte(chunk))
	}
	s.Flush()
}

func tokenBoundaryInputs(token string) []struct {
	name  string
	input string
} {
	return []struct {
		name  string
		input string
	}{
		{"bare", token},
		{"leading_space", " " + token},
		{"trailing_space", token + " "},
		{"spaced", " " + token + " "},
	}
}

func assertTokenValid(t *testing.T, token string, wantValid bool) {
	t.Helper()
	for _, tt := range tokenBoundaryInputs(token) {
		t.Run(tt.name, func(t *testing.T) {
			s, calls := newStreamer()
			writeAll(t, s, tt.input)

			found := false
			for _, c := range calls() {
				if c.raw != token {
					continue
				}
				found = true
				if gotValid := c.addr.IsValid(); gotValid != wantValid {
					t.Errorf("token %q valid=%v, want %v addr=%v", token, gotValid, wantValid, c.addr)
				}
			}
			if !found {
				t.Fatalf("token %q not found in calls: %+v", token, calls())
			}
		})
	}
}

func assertTokenAddr(t *testing.T, token string, want string) {
	t.Helper()
	for _, tt := range tokenBoundaryInputs(token) {
		t.Run(tt.name, func(t *testing.T) {
			s, calls := newStreamer()
			writeAll(t, s, tt.input)
			if !findAddr(calls(), mustAddr(want)) {
				t.Fatalf("addr %s not found for token %q input %q; calls: %+v", want, token, tt.input, calls())
			}
		})
	}
}

func assertInputAddr(t *testing.T, input string, want string) {
	t.Helper()
	s, calls := newStreamer()
	writeAll(t, s, input)
	if !findAddr(calls(), mustAddr(want)) {
		t.Fatalf("addr %s not found for input %q; calls: %+v", want, input, calls())
	}
}

func findAddr(calls []call, want netip.Addr) bool {
	for _, c := range calls {
		if c.addr == want {
			return true
		}
	}
	return false
}

func validAddrs(calls []call) []netip.Addr {
	var addrs []netip.Addr
	for _, c := range calls {
		if c.addr.IsValid() {
			addrs = append(addrs, c.addr)
		}
	}
	return addrs
}

func mustAddr(s string) netip.Addr {
	a, err := netip.ParseAddr(s)
	if err != nil {
		panic(err)
	}
	return a
}

const maxCandidateTokenLen = len("ffff:ffff:ffff:ffff:ffff:ffff:255.255.255.255%1234567890abcde")

// --- Basics ---

func TestWrite_AcceptsFullInput(t *testing.T) {
	s, calls := newStreamer()
	input := []byte("hello 1.2.3.4 world")
	s.Write(input)
	s.Flush()
	if got := reconstruct(calls()); got != string(input) {
		t.Errorf("reconstruct=%q, want %q", got, input)
	}
}

func TestFlush_EmptyDoesNothing(t *testing.T) {
	s, calls := newStreamer()
	s.Flush()
	if got := calls(); len(got) != 0 {
		t.Errorf("expected no calls, got %+v", got)
	}
}

func TestEmptyInput_NoCalls(t *testing.T) {
	s, calls := newStreamer()
	writeAll(t, s, "")
	if got := calls(); len(got) != 0 {
		t.Errorf("expected no calls, got %+v", got)
	}
}

// --- IPv4 ---

func TestIPv4_Valid(t *testing.T) {
	tests := []string{
		"127.0.0.1",
		"0.0.0.0",
		"255.255.255.255",
		"10.0.0.1",
		"192.168.100.200",
	}
	for _, addr := range tests {
		t.Run(addr, func(t *testing.T) {
			assertTokenAddr(t, addr, addr)
		})
	}
}

func TestIPv4_WrongDotCount_EmittedAsFalse(t *testing.T) {
	// IPv4 candidates need exactly 3 dots.
	tests := []struct {
		name  string
		token string
	}{
		{"no_dots", "12345"},
		{"one_dot", "1.2"},
		{"two_dots", "1.2.3"},
		{"four_dots", "1.2.3.4.5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertTokenValid(t, tt.token, false)
		})
	}
}

func TestIPv4_ParseFails_EmittedAsFalse(t *testing.T) {
	// Exactly 3 dots but rejected by the IPv4 parser.
	tokens := []string{
		"999.999.999.999",
		"256.0.0.1",
		"0.0.0.256",
		"1.2.3.999",
	}
	for _, tok := range tokens {
		t.Run(tok, func(t *testing.T) {
			assertTokenValid(t, tok, false)
		})
	}
}

// --- IPv6 ---

func TestIPv6_Valid(t *testing.T) {
	tests := []string{
		"::",
		"::1",
		"fe80::1",
		"2001:db8::1",
		"2001:db8:0:0:0:0:0:1", // 7 colons — upper bound of heuristic
		"::ffff:192.168.1.1",   // IPv4-mapped; dotCount=3 triggers heuristic
	}
	for _, addr := range tests {
		t.Run(addr, func(t *testing.T) {
			assertTokenAddr(t, addr, addr)
		})
	}
}

func TestIPv6_AdditionalValidForms(t *testing.T) {
	tests := []string{
		"2001:DB8::ABCD",        // uppercase hex is accepted and normalized
		"::192.0.2.1",           // compressed IPv6 with dotted-quad tail
		"64:ff9b::192.0.2.33",   // prefix plus dotted-quad tail
		"0:0:0:0:0:ffff:0:0",    // fully expanded with exactly 7 colons
		"ffff:0:0:0:0:0:0:ffff", // fully expanded high/low boundary groups
	}
	for _, addr := range tests {
		t.Run(addr, func(t *testing.T) {
			assertTokenAddr(t, addr, addr)
		})
	}
}

func TestIPv6_WithZone(t *testing.T) {
	addr := "fe80::1%1" // zone "1"
	assertTokenAddr(t, addr, addr)
}

func TestIPv6_WithZoneAdditionalValidForms(t *testing.T) {
	tests := []string{
		"::1%eth0",
		"::1%Eth0",
		"::1%eth0.1",
		"::1%eth0-a",
		"::1%eth0_a",
		"fe80::1%abc.def",
		"fe80::1%enp0s3",
		"fe80::1%br-abcd",
		"fe80::1%veth_abcd",
		"FE80::ABCD%ABC.DEF",
		"ffff:ffff:ffff:ffff:ffff:ffff:255.255.255.255%abc.def",
	}
	for _, addr := range tests {
		t.Run(addr, func(t *testing.T) {
			assertTokenAddr(t, addr, addr)
		})
	}
}

func TestIPForms_ComprehensiveValidAndInvalid(t *testing.T) {
	valid := []struct {
		name  string
		token string
		want  string
	}{
		// IPv4 boundary and common private/public ranges.
		{"ipv4_unspecified", "0.0.0.0", "0.0.0.0"},
		{"ipv4_loopback", "127.0.0.1", "127.0.0.1"},
		{"ipv4_private_10", "10.0.0.1", "10.0.0.1"},
		{"ipv4_private_172", "172.16.0.1", "172.16.0.1"},
		{"ipv4_private_192", "192.168.1.1", "192.168.1.1"},
		{"ipv4_broadcast", "255.255.255.255", "255.255.255.255"},

		// IPv6 compression at the beginning, middle, and end.
		{"ipv6_unspecified", "::", "::"},
		{"ipv6_loopback", "::1", "::1"},
		{"ipv6_compressed_start", "::ffff:0:0", "::ffff:0:0"},
		{"ipv6_compressed_middle", "2001:db8::1:0:0:1", "2001:db8::1:0:0:1"},
		{"ipv6_compressed_end", "2001:db8::", "2001:db8::"},
		{"ipv6_fully_expanded", "2001:0db8:85a3:0000:0000:8a2e:0370:7334", "2001:0db8:85a3:0000:0000:8a2e:0370:7334"},
		{"ipv6_uppercase", "2001:DB8::ABCD", "2001:db8::abcd"},

		// IPv6 forms with embedded IPv4 dotted-quad tails.
		{"ipv6_ipv4_compatible", "::192.0.2.1", "::192.0.2.1"},
		{"ipv6_ipv4_mapped", "::ffff:192.0.2.128", "::ffff:192.0.2.128"},
		{"ipv6_well_known_prefix", "64:ff9b::192.0.2.33", "64:ff9b::192.0.2.33"},
		{"ipv6_expanded_dotted_tail", "0:0:0:0:0:0:13.1.68.3", "0:0:0:0:0:0:13.1.68.3"},

		// Scanner-supported zone text.
		{"zone_unspecified_addr", "fe80::%abc", "fe80::%abc"},
		{"zone_numeric", "fe80::1%1", "fe80::1%1"},
		{"zone_interface_name", "::1%eth0", "::1%eth0"},
		{"zone_loopback_uppercase_interface_name", "::1%Eth0", "::1%Eth0"},
		{"zone_loopback_dotted_interface_name", "::1%eth0.1", "::1%eth0.1"},
		{"zone_loopback_hyphen_interface_name", "::1%eth0-a", "::1%eth0-a"},
		{"zone_loopback_underscore_interface_name", "::1%eth0_a", "::1%eth0_a"},
		{"zone_dotted", "fe80::1%abc.def", "fe80::1%abc.def"},
		{"zone_hyphen", "fe80::1%br-abcd", "fe80::1%br-abcd"},
		{"zone_underscore", "fe80::1%veth_abcd", "fe80::1%veth_abcd"},
		{"zone_uppercase", "FE80::ABCD%ABC.DEF", "fe80::abcd%ABC.DEF"},
		{"zone_dotted_tail", "ffff:ffff:ffff:ffff:ffff:ffff:255.255.255.255%abc.def", "ffff:ffff:ffff:ffff:ffff:ffff:255.255.255.255%abc.def"},
	}

	for _, tt := range valid {
		t.Run("valid_"+tt.name, func(t *testing.T) {
			assertTokenAddr(t, tt.token, tt.want)
		})
	}

	invalid := []struct {
		name  string
		token string
	}{
		// IPv4 invalid shapes.
		{"ipv4_leading_zero", "192.168.001.001"},
		{"ipv4_trailing_dot", "1.2.3.4."},
		{"ipv4_empty_octet", "1..2.3"},
		{"ipv4_too_few_octets", "1.2.3"},
		{"ipv4_too_many_octets", "1.2.3.4.5"},
		{"ipv4_octet_overflow", "256.255.255.255"},

		// IPv6 invalid compression/group-count/group-width forms.
		{"ipv6_triple_colon", "2001:db8:::1"},
		{"ipv6_multiple_double_colon", "2001:db8::1::"},
		{"ipv6_group_too_long", "12345::"},
		{"ipv6_middle_group_too_long", "2001:12345::1"},
		{"ipv6_too_many_groups", "1:2:3:4:5:6:7:8:9"},
		{"ipv6_too_few_groups_without_compression", "1:2:3:4:5:6:7"},
		{"ipv6_time_like_false_positive", "12:34:56"},
		{"ipv6_bad_dotted_tail", "::ffff:999.1.1.1"},
		{"ipv6_bad_dot_count", "2001:db8::1.2.3"},

		// Scanner zone policy.
		{"zone_empty", "fe80::1%"},
		{"zone_too_long_for_scanner", "fe80::1%abcdefabcdefabcd"},
		{"zone_double_percent", "fe80::1%1%2"},
	}

	for _, tt := range invalid {
		t.Run("invalid_"+tt.name, func(t *testing.T) {
			assertTokenValid(t, tt.token, false)
		})
	}
}

func TestIPv6_WrongColonCount_EmittedAsFalse(t *testing.T) {
	// IPv6 candidates need 2-7 colons.
	tests := []struct {
		name  string
		token string
	}{
		{"one_colon", "1:2"},
		{"eight_colons", "1:2:3:4:5:6:7:8:9"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertTokenValid(t, tt.token, false)
		})
	}
}

func TestIPv6_ParseFails_EmittedAsFalse(t *testing.T) {
	// Malformed IPv6-like tokens must be emitted with valid=false.
	tokens := []string{":::", "::::", "%1", "%abc", "1:2:3:4:5:6:7"}
	for _, tok := range tokens {
		t.Run(tok, func(t *testing.T) {
			assertTokenValid(t, tok, false)
		})
	}
}

// --- Non-IP segments ---

func TestNonIPOnly_AllEmittedAsFalse(t *testing.T) {
	s, calls := newStreamer()
	writeAll(t, s, ", , ; \n\t")
	for _, c := range calls() {
		if c.addr.IsValid() {
			t.Errorf("expected valid=false for all calls, got valid=true for %q", c.raw)
		}
		if c.addr != (netip.Addr{}) {
			t.Errorf("expected zero addr for %q, got %v", c.raw, c.addr)
		}
	}
}

// --- Reconstruction invariant ---

func TestReconstructInvariant(t *testing.T) {
	// Raw segments reconstruct the input.
	inputs := []string{
		"",
		"hello",
		"1.2.3.4",
		" 1.2.3.4 ",
		"::1",
		" ::1 ",
		"::1%eth0",
		"prefix(::1%eth0),suffix",
		" 192.168.0.1 ::1 10.0.0.1 ",
		"no ips here\n",
		"ip=1.2.3.4;port=80",
	}
	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			s, calls := newStreamer()
			writeAll(t, s, input)
			if got := reconstruct(calls()); got != input {
				t.Errorf("reconstruct=%q, want %q", got, input)
			}
		})
	}
}

func TestIPv6_LoopbackWithEth0Zone_DelimiterBoundaries(t *testing.T) {
	input := "before[::1%eth0],after"
	s, calls := newStreamer()
	writeAll(t, s, input)

	want := mustAddr("::1%eth0")
	found := false
	for _, c := range calls() {
		if c.raw != "::1%eth0" {
			continue
		}
		found = true
		if !c.addr.IsValid() || c.addr != want {
			t.Fatalf("segment %q valid=%v addr=%v, want valid=true addr=%v", c.raw, c.addr.IsValid(), c.addr, want)
		}
	}
	if !found {
		t.Fatalf("::1%%eth0 segment not found; calls: %+v", calls())
	}
	if got := reconstruct(calls()); got != input {
		t.Fatalf("reconstruct=%q, want %q", got, input)
	}
}

// --- Flush ---

func TestFlush_EmitsPendingTokens(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  netip.Addr
	}{
		{"ipv4", "1.2.3.4", netip.MustParseAddr("1.2.3.4")},
		{"ipv6", "::1", netip.MustParseAddr("::1")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, calls := newStreamer()
			s.Write([]byte(tt.input)) // no trailing non-IP delimiter

			if got := calls(); len(got) != 0 {
				t.Fatalf("pending token emitted before Flush: %+v", got)
			}

			s.Flush()
			if !findAddr(calls(), tt.want) {
				t.Errorf("%s not found after Flush; calls: %+v", tt.want, calls())
			}
		})
	}
}

func TestFlush_Idempotent(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"empty_carrier", " 1.2.3.4 "}, // trailing space forces flush inside Write
		{"pending_token", "10.0.0.1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, calls := newStreamer()
			s.Write([]byte(tt.input))
			s.Flush()
			n := len(calls())
			s.Flush()
			if len(calls()) != n {
				t.Errorf("extra calls after second Flush: %+v", calls()[n:])
			}
		})
	}
}

// --- Streaming across Write calls ---

func TestStreaming_IPv4SplitAcrossWrites(t *testing.T) {
	s, calls := newStreamer()
	writeChunks(t, s, "192.168", ".1.1 ")
	if !findAddr(calls(), netip.MustParseAddr("192.168.1.1")) {
		t.Errorf("192.168.1.1 not found; calls: %+v", calls())
	}
}

func TestStreaming_IPv6SplitAcrossWrites(t *testing.T) {
	s, calls := newStreamer()
	writeChunks(t, s, "2001:db8", "::1 ")
	if !findAddr(calls(), netip.MustParseAddr("2001:db8::1")) {
		t.Errorf("2001:db8::1 not found; calls: %+v", calls())
	}
}

func TestStreaming_SplitAcrossWritesWithoutTrailingDelimiter(t *testing.T) {
	tests := []struct {
		name      string
		chunks    []string
		raw       string
		wantValid bool
		wantAddr  string
	}{
		{
			name:      "valid_ipv4",
			chunks:    []string{"192", ".168", ".1", ".1"},
			raw:       "192.168.1.1",
			wantValid: true,
			wantAddr:  "192.168.1.1",
		},
		{
			name:   "invalid_ipv4_wrong_dot_count",
			chunks: []string{"192", ".168", ".1"},
			raw:    "192.168.1",
		},
		{
			name:      "valid_ipv6",
			chunks:    []string{"2001", ":db8", "::", "1"},
			raw:       "2001:db8::1",
			wantValid: true,
			wantAddr:  "2001:db8::1",
		},
		{
			name:   "invalid_ipv6_no_double_colon",
			chunks: []string{"00", ":00", ":00"},
			raw:    "00:00:00",
		},
		{
			name:      "valid_ipv6_zone",
			chunks:    []string{"fe80", "::", "1%", "1"},
			raw:       "fe80::1%1",
			wantValid: true,
			wantAddr:  "fe80::1%1",
		},
		{
			name:      "valid_ipv6_interface_zone",
			chunks:    []string{"::", "1%e", "th", "0"},
			raw:       "::1%eth0",
			wantValid: true,
			wantAddr:  "::1%eth0",
		},
		{
			name:      "valid_ipv6_interface_zone_split_before_zone_only_char",
			chunks:    []string{"::1%e", "t", "h0"},
			raw:       "::1%eth0",
			wantValid: true,
			wantAddr:  "::1%eth0",
		},
		{
			name:      "valid_ipv6_interface_zone_split_at_percent",
			chunks:    []string{"::1%", "eth0"},
			raw:       "::1%eth0",
			wantValid: true,
			wantAddr:  "::1%eth0",
		},
		{
			name:   "invalid_ipv6_zone_missing_zone",
			chunks: []string{"fe80", "::", "1%"},
			raw:    "fe80::1%",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, calls := newStreamer()
			for _, chunk := range tt.chunks {
				s.Write([]byte(chunk))
			}
			if got := calls(); len(got) != 0 {
				t.Fatalf("token emitted before Flush without trailing delimiter: %+v", got)
			}

			s.Flush()
			gotCalls := calls()
			if reconstruct(gotCalls) != tt.raw {
				t.Fatalf("reconstructed input = %q, want %q; calls: %+v", reconstruct(gotCalls), tt.raw, gotCalls)
			}
			if len(gotCalls) != 1 {
				t.Fatalf("got %d calls, want 1 flushed token; calls: %+v", len(gotCalls), gotCalls)
			}

			got := gotCalls[0]
			if got.raw != tt.raw {
				t.Fatalf("raw = %q, want %q", got.raw, tt.raw)
			}
			if got.addr.IsValid() != tt.wantValid {
				t.Fatalf("valid = %v, want %v; call: %+v", got.addr.IsValid(), tt.wantValid, got)
			}
			if tt.wantValid {
				if want := mustAddr(tt.wantAddr); got.addr != want {
					t.Fatalf("addr = %v, want %v", got.addr, want)
				}
			} else if got.addr.IsValid() {
				t.Fatalf("invalid token addr = %v, want zero", got.addr)
			}
		})
	}
}

func TestStreaming_ByteByByte(t *testing.T) {
	input := " 10.20.30.40 "
	want := netip.MustParseAddr("10.20.30.40")

	s, calls := newStreamer()
	for i := 0; i < len(input); i++ {
		s.Write([]byte{input[i]})
	}
	s.Flush()

	if !findAddr(calls(), want) {
		t.Errorf("%s not found; calls: %+v", want, calls())
	}
	if got := reconstruct(calls()); got != input {
		t.Errorf("reconstruct=%q, want %q", got, input)
	}
}

func TestStreaming_NonIPBetweenChunks_TerminatesToken(t *testing.T) {
	// A non-IP byte terminates the carried token.
	s, calls := newStreamer()
	s.Write([]byte("192.168"))
	s.Write([]byte(" 1.1 ")) // space terminates "192.168"
	s.Flush()

	for _, c := range calls() {
		if c.raw == "192.168" && c.addr.IsValid() {
			t.Errorf("192.168 (2 dots) should be valid=false, got valid=true addr=%v", c.addr)
		}
	}
	for _, c := range calls() {
		if c.raw == "1.1" && c.addr.IsValid() {
			t.Errorf("1.1 (1 dot) should be valid=false, got valid=true addr=%v", c.addr)
		}
	}
}

// --- maxTokenLen ---

func TestOversizedToken_EmittedAsFalse(t *testing.T) {
	tok := strings.Repeat("a", maxCandidateTokenLen+1) // 'a'-'f' are valid IP chars (hex digits)
	s, calls := newStreamer()
	writeAll(t, s, " "+tok+" ")
	found := false
	for _, c := range calls() {
		if c.raw == tok {
			found = true
			if c.addr.IsValid() {
				t.Errorf("expected valid=false for oversized token, got valid=true addr=%v", c.addr)
			}
		}
	}
	if !found {
		t.Errorf("oversized token not found; calls: %+v", calls())
	}
}

func TestOversizedToken_CarrierFlushedFirst(t *testing.T) {
	prefix := "192" // 3 bytes in carrier after first write
	bulk := strings.Repeat("a", maxCandidateTokenLen-len(prefix))
	s, calls := newStreamer()
	s.Write([]byte(prefix))     // carrier = "192"
	s.Write([]byte("." + bulk)) // carrier + current chunk exceeds maxTokenLen
	s.Flush()

	gotCalls := calls()
	raws := make([]string, 0, len(gotCalls))
	for _, c := range gotCalls {
		raws = append(raws, c.raw)
		if c.addr.IsValid() {
			t.Errorf("expected all calls to be valid=false in this scenario, got valid=true for %q addr=%v", c.raw, c.addr)
		}
	}
	if len(raws) < 2 {
		t.Errorf("expected at least 2 calls (carrier + bulk), got %d: %v", len(raws), raws)
	}
	if raws[0] != prefix {
		t.Errorf("first call raw=%q, want %q", raws[0], prefix)
	}
	wantBulk := "." + bulk
	if raws[1] != wantBulk {
		t.Errorf("second call raw=%q (len %d), want %q (len %d)", raws[1], len(raws[1]), wantBulk, len(wantBulk))
	}
}

// --- Multiple IPs ---

func TestMultipleIPsInOneWrite(t *testing.T) {
	s, calls := newStreamer()
	writeAll(t, s, " 1.2.3.4 ::1 10.0.0.1 ")

	wantAddrs := []netip.Addr{
		netip.MustParseAddr("1.2.3.4"),
		netip.MustParseAddr("::1"),
		netip.MustParseAddr("10.0.0.1"),
	}

	gotAddrs := validAddrs(calls())
	if len(gotAddrs) != len(wantAddrs) {
		t.Fatalf("got %d valid addrs %v, want %d %v", len(gotAddrs), gotAddrs, len(wantAddrs), wantAddrs)
	}
	for i := range wantAddrs {
		if gotAddrs[i] != wantAddrs[i] {
			t.Errorf("addr[%d] = %v, want %v", i, gotAddrs[i], wantAddrs[i])
		}
	}
}

// --- Token shape edge cases ---

func TestMaxTokenLen_ExactBoundary(t *testing.T) {
	tok := strings.Repeat("a", maxCandidateTokenLen) // 'a' is a hex IP char; no dots/colons → valid=false
	s, calls := newStreamer()
	writeAll(t, s, " "+tok+" ")
	found := false
	for _, c := range calls() {
		if c.raw == tok {
			found = true
			if c.addr.IsValid() {
				t.Errorf("maxTokenLen all-hex token should be valid=false (no dots/colons), got valid=true")
			}
		}
	}
	if !found {
		t.Errorf("maxTokenLen token not found; calls: %+v", calls())
	}
}

func TestToken_OnlyDots(t *testing.T) {
	// Too short for IPv4 despite 3 dots.
	assertTokenValid(t, "...", false)
}

func TestToken_OnlyColons_IsValidIPv6(t *testing.T) {
	assertTokenAddr(t, "::", "::")
}

func TestToken_LeadingDot(t *testing.T) {
	tok := ".1.2.3.4"
	assertTokenValid(t, tok, false)
}

func TestToken_TrailingDot(t *testing.T) {
	// Trailing dot fails IPv4 digit-boundary checks.
	tok := "1.2.3."
	assertTokenValid(t, tok, false)
}

func TestIPv4_LeadingZero_ParseFails(t *testing.T) {
	// IPv4 octets with leading zeros are rejected.
	tokens := []string{"01.2.3.4", "1.02.3.4", "1.2.03.4", "1.2.3.04"}
	for _, tok := range tokens {
		t.Run(tok, func(t *testing.T) {
			assertTokenValid(t, tok, false)
		})
	}
}

func TestIPv6_TwoPctSigns_HeuristicRejects(t *testing.T) {
	tok := "::1%2%3" // colonCount=2, pctCount=2
	assertTokenValid(t, tok, false)
}

func TestToken_MixedDotsAndColons_HeuristicRejects(t *testing.T) {
	tok := "1:2.3.4"
	assertTokenValid(t, tok, false)
}

func TestIPv6_FullyExpanded_SevenColons(t *testing.T) {
	expanded := "0:0:0:0:0:0:0:1"
	assertTokenAddr(t, expanded, expanded)
}

// --- Streaming edge cases ---

func TestStreaming_EmptyWriteMidToken(t *testing.T) {
	// An empty Write in the middle of a token must not break streaming.
	s, calls := newStreamer()
	writeChunks(t, s, "192.168", "", ".1.1 ")
	if !findAddr(calls(), mustAddr("192.168.1.1")) {
		t.Errorf("192.168.1.1 not found after empty mid-write; calls: %+v", calls())
	}
}

func TestStreaming_IPv6SplitAtDoubleColon(t *testing.T) {
	s, calls := newStreamer()
	writeChunks(t, s, "::", "1 ")
	if !findAddr(calls(), mustAddr("::1")) {
		t.Errorf("::1 not found; calls: %+v", calls())
	}
}

func TestStreaming_IPv6DoubleColonAcrossWriteBoundary(t *testing.T) {
	s, calls := newStreamer()
	writeChunks(t, s, "2001:db8:", ":1 ")
	if !findAddr(calls(), mustAddr("2001:db8::1")) {
		t.Errorf("2001:db8::1 not found; calls: %+v", calls())
	}
}

func TestFlush_IncompleteTokenEmittedAsFalse(t *testing.T) {
	// Flush emits the partial token and resets state.
	s, calls := newStreamer()
	s.Write([]byte("192.168")) // 2 dots, incomplete
	s.Flush()                  // forces "192.168" out as valid=false; resets counters

	s.Write([]byte("1.2.3.4 ")) // fresh token, valid
	s.Flush()

	partialFound := false
	for _, c := range calls() {
		if c.raw == "192.168" {
			partialFound = true
			if c.addr.IsValid() {
				t.Errorf("partial token 192.168 should be valid=false, got valid=true")
			}
		}
	}
	if !partialFound {
		t.Errorf("partial token 192.168 not found; calls: %+v", calls())
	}
	if !findAddr(calls(), mustAddr("1.2.3.4")) {
		t.Errorf("1.2.3.4 not found after flush of partial token; calls: %+v", calls())
	}
}

func TestFlush_IncompleteIPv6TokenEmittedAsFalse(t *testing.T) {
	s, calls := newStreamer()
	s.Write([]byte("2001:db8"))
	s.Flush()

	found := false
	for _, c := range calls() {
		if c.raw == "2001:db8" {
			found = true
			if c.addr.IsValid() {
				t.Errorf("partial token 2001:db8 should be valid=false, got valid=true")
			}
		}
	}
	if !found {
		t.Errorf("partial token 2001:db8 not found; calls: %+v", calls())
	}
}

func TestNullByteIsDelimiter(t *testing.T) {
	s, calls := newStreamer()
	input := "\x001.2.3.4\x00"
	writeAll(t, s, input)
	if !findAddr(calls(), mustAddr("1.2.3.4")) {
		t.Errorf("1.2.3.4 not found with null-byte delimiters; calls: %+v", calls())
	}
	if got := reconstruct(calls()); got != input {
		t.Errorf("reconstruct=%q, want %q", got, input)
	}
}

func TestIPv4_AtStartOfInput(t *testing.T) {
	assertInputAddr(t, "10.0.0.1 tail", "10.0.0.1")
}

func TestIPv6_AtStartOfInput(t *testing.T) {
	assertInputAddr(t, "::1 tail", "::1")
}

// --- Property-based split tests ---

func TestIPDetection_ConsistentAcrossSplitPoints(t *testing.T) {
	// Split position must not affect detected IPs.
	input := " 1.2.3.4 ::1 "
	wantAddrs := []netip.Addr{mustAddr("1.2.3.4"), mustAddr("::1")}

	for split := 1; split < len(input); split++ {
		s, calls := newStreamer()
		s.Write([]byte(input[:split]))
		s.Write([]byte(input[split:]))
		s.Flush()

		got := validAddrs(calls())
		if len(got) != len(wantAddrs) {
			t.Errorf("split at %d: got addrs %v, want %v", split, got, wantAddrs)
			continue
		}
		for i := range wantAddrs {
			if got[i] != wantAddrs[i] {
				t.Errorf("split at %d: addr[%d]=%v, want %v", split, i, got[i], wantAddrs[i])
			}
		}
	}
}

// --- Overflow counter reset (regression for the stale-counter bug) ---

func TestOversizedToken_CountersResetAfterOverflow(t *testing.T) {
	// Overflow resets counters for the next token.
	oversize := strings.Repeat("1", maxCandidateTokenLen/2) + "." + strings.Repeat("1", maxCandidateTokenLen/2+1)
	s, calls := newStreamer()
	writeChunks(t, s, oversize, " 1.2.3.4 ")

	if !findAddr(calls(), mustAddr("1.2.3.4")) {
		t.Errorf("1.2.3.4 misclassified after oversized token (stale counter not reset); calls: %+v", calls())
	}
}

func TestOversizedToken_ContinuationUntilDelimiterStaysFalse(t *testing.T) {
	s, calls := newStreamer()
	oversize := strings.Repeat("a", maxCandidateTokenLen+1)

	writeChunks(t, s, oversize, "1.2.3.4 ")

	foundContinuation := false
	for _, c := range calls() {
		if c.raw == "1.2.3.4" && !c.addr.IsValid() {
			foundContinuation = true
		}
		if c.raw == "1.2.3.4" && c.addr.IsValid() {
			t.Fatalf("overflow continuation parsed as IP: %+v", calls())
		}
	}
	if !foundContinuation {
		t.Fatalf("overflow continuation not emitted as non-IP: %+v", calls())
	}
	if reconstruct(calls()) != oversize+"1.2.3.4 " {
		t.Fatalf("reconstructed input mismatch: got %q", reconstruct(calls()))
	}
}

func TestOversizedZonedIPv6_Eth0ContinuationUntilDelimiterStaysFalse(t *testing.T) {
	s, calls := newStreamer()
	oversize := "::1%" + strings.Repeat("eth0", maxCandidateTokenLen/4+1)

	writeChunks(t, s, oversize, "eth0 ::1%eth0 ")

	foundContinuation := false
	for _, c := range calls() {
		if c.raw == "eth0" && !c.addr.IsValid() {
			foundContinuation = true
		}
	}
	if !foundContinuation {
		t.Fatalf("overflow zone continuation and delimiter were not emitted together as non-IP: %+v", calls())
	}
	if got := reconstruct(calls()); got != oversize+"eth0 ::1%eth0 " {
		t.Fatalf("reconstructed input mismatch: got %q", got)
	}
	if !findAddr(calls(), mustAddr("::1%eth0")) {
		t.Fatalf("valid ::1%%eth0 after overflow delimiter not parsed; calls: %+v", calls())
	}
}

func TestOversizedToken_ResumesParsingAfterDelimiterInSameWrite(t *testing.T) {
	s, calls := newStreamer()
	oversize := strings.Repeat("a", maxCandidateTokenLen+1)

	writeChunks(t, s, oversize, "abc 1.2.3.4")

	if !findAddr(calls(), mustAddr("1.2.3.4")) {
		t.Fatalf("valid token after overflow delimiter not parsed; calls: %+v", calls())
	}
	if got := reconstruct(calls()); got != oversize+"abc 1.2.3.4" {
		t.Fatalf("reconstructed input mismatch: got %q", got)
	}
}

// --- valid=false always carries zero netip.Addr ---

func TestInvalid_AlwaysZeroAddr(t *testing.T) {
	// valid=false always carries a zero Addr.
	inputs := []string{
		" , ; \t\n ",        // pure non-IP bytes
		" 999.999.999.999 ", // octet overflow
		" 1:2 ",             // 1 colon, heuristic fails
		" ::: ",             // maxColonRun > 2
		" 1.2.3. ",          // trailing dot
		" aaaaaa ",          // hex chars, no dots/colons
	}
	for _, input := range inputs {
		t.Run(strings.TrimSpace(input), func(t *testing.T) {
			s, calls := newStreamer()
			writeAll(t, s, input)
			for _, c := range calls() {
				if !c.addr.IsValid() && c.addr != (netip.Addr{}) {
					t.Errorf("valid=false call has non-zero addr %v for raw=%q", c.addr, c.raw)
				}
			}
		})
	}
}

// --- Non-IP character batching ---

func TestNonIPChars_BatchedIntoSingleCall(t *testing.T) {
	// Consecutive delimiters are batched.
	s, calls := newStreamer()
	writeAll(t, s, " , \t; ")
	var nonIPCalls []call
	for _, c := range calls() {
		if !c.addr.IsValid() {
			nonIPCalls = append(nonIPCalls, c)
		}
	}
	if len(nonIPCalls) != 1 {
		t.Errorf("expected 1 non-IP call for a single run of delimiters, got %d: %+v", len(nonIPCalls), nonIPCalls)
	}
	if nonIPCalls[0].raw != " , \t; " {
		t.Errorf("non-IP call raw=%q, want %q", nonIPCalls[0].raw, " , \t; ")
	}
}

func TestNonIPChars_TwoRunsSeparatedByToken(t *testing.T) {
	s, calls := newStreamer()
	writeAll(t, s, "!! 1.2.3.4 !!")
	var bangCalls []call
	for _, c := range calls() {
		if strings.Contains(c.raw, "!") {
			bangCalls = append(bangCalls, c)
		}
	}
	if len(bangCalls) != 2 {
		t.Errorf("expected 2 calls containing '!', got %d: %+v", len(bangCalls), bangCalls)
	}
}

// --- Single-character IP-char tokens ---

func TestToken_OnlyPercent(t *testing.T) {
	assertTokenValid(t, "%", false)
}

func TestToken_OnlyColon(t *testing.T) {
	assertTokenValid(t, ":", false)
}

// --- Both heuristics fire simultaneously ---

func TestToken_ThreeDotsAndTwoColons_ParseFails(t *testing.T) {
	tok := "1.2.3.4::1"
	assertTokenValid(t, tok, false)
}

func TestDottedIPv6Tail_HeuristicRejectsInvalidColonShape(t *testing.T) {
	tests := []struct {
		name  string
		token string
	}{
		{
			name:  "too_few_colons",
			token: "1:2.3.4.5",
		},
		{
			name:  "colon_run_too_long",
			token: ":::1.2.3.4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertTokenValid(t, tt.token, false)
		})
	}
}

func TestIPv4_ThreeDots_TwoPcts_ParseFails(t *testing.T) {
	tok := "1.2.3.4%5%6"
	assertTokenValid(t, tok, false)
}

// --- Carrier exactly at maxTokenLen boundary ---

func TestCarrier_AtExactBoundary_ThenMoreIPChars(t *testing.T) {
	// Overflow after a full carrier emits carrier and overflow byte separately.
	carrierMax := strings.Repeat("a", maxCandidateTokenLen) // valid IP chars, no dots/colons
	s, calls := newStreamer()
	writeChunks(t, s, carrierMax, "b ")

	var raws []string
	for _, c := range calls() {
		if c.raw == carrierMax || c.raw == "b" {
			raws = append(raws, c.raw)
			if c.addr.IsValid() {
				t.Errorf("expected valid=false for raw=%q, got valid=true", c.raw)
			}
		}
	}
	if len(raws) < 2 {
		t.Errorf("expected at least 2 calls (carrier + overflow byte), got %d in calls: %+v", len(raws), calls())
	}
}

// --- IPv6 in-range colonCount but invalid address ---

func TestIPv6_SixColons_InvalidAddress(t *testing.T) {
	tok := "::::::"
	assertTokenValid(t, tok, false)
}

func TestIPv6_TwoColons_InvalidAddress(t *testing.T) {
	tok := ":a:"
	assertTokenValid(t, tok, false)
}

// --- Write after Flush ---

func TestFlush_AllowsMoreWrites(t *testing.T) {
	s, calls := newStreamer()
	s.Write([]byte("1.2.3.4 "))
	s.Flush()
	n1 := len(calls())

	s.Write([]byte("::1 "))
	s.Flush()

	if !findAddr(calls(), mustAddr("1.2.3.4")) {
		t.Errorf("1.2.3.4 not found; calls: %+v", calls())
	}
	if !findAddr(calls(), mustAddr("::1")) {
		t.Errorf("::1 not found after write-after-flush; calls: %+v", calls())
	}
	if len(calls()) <= n1 {
		t.Errorf("no new calls after write-after-flush")
	}
}

// --- Reconstruction with streaming splits ---

func TestStreaming_SplitReconstructionAlwaysValid(t *testing.T) {
	// Every two-chunk split must preserve reconstruction.
	input := "host=10.0.0.1, addr=fe80::1%1, bad=999.999.999.999 end"
	for split := 1; split < len(input); split++ {
		s, calls := newStreamer()
		writeChunks(t, s, input[:split], input[split:])
		if got := reconstruct(calls()); got != input {
			t.Errorf("split at %d: reconstruct=%q, want %q", split, got, input)
		}
	}
}

// --- charType delimiter boundaries ---

func TestNonIPChar_UpperNonHexNotIPChar(t *testing.T) {
	// 'G'-'Z' are not IP chars and must act as delimiters.
	s, calls := newStreamer()
	writeAll(t, s, "G1.2.3.4H")
	if !findAddr(calls(), mustAddr("1.2.3.4")) {
		t.Errorf("1.2.3.4 not found when surrounded by non-hex uppercase letters; calls: %+v", calls())
	}
	if got := reconstruct(calls()); got != "G1.2.3.4H" {
		t.Errorf("reconstruct=%q, want %q", got, "G1.2.3.4H")
	}
}

func TestNonIPChar_LowerNonHexNotIPChar(t *testing.T) {
	// 'g'-'z' are not IP chars and act as delimiters.
	s, calls := newStreamer()
	writeAll(t, s, "g::1z")
	if !findAddr(calls(), mustAddr("::1")) {
		t.Errorf("::1 not found when surrounded by non-hex lowercase letters; calls: %+v", calls())
	}
	if got := reconstruct(calls()); got != "g::1z" {
		t.Errorf("reconstruct=%q, want %q", got, "g::1z")
	}
}

// --- Length boundaries ---

func TestIPv4_LengthBoundaries(t *testing.T) {
	tests := []struct {
		name  string
		token string
		valid bool
	}{
		{"min_valid_len_7", "0.0.0.0", true},
		{"max_valid_len_15", "255.255.255.255", true},
		{"too_short_len_6", "1.1.1.", false},              // 3 dots but below min length
		{"too_long_len_19", "1111.1111.1111.1111", false}, // 3 dots but above max length
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertTokenValid(t, tt.token, tt.valid)
		})
	}
}

func TestIPv6_LengthBoundaries(t *testing.T) {
	tests := []struct {
		name  string
		token string
		valid bool
	}{
		{"min_valid_len_2", "::", true},
		{"max_valid_len_45", "ffff:ffff:ffff:ffff:ffff:ffff:255.255.255.255", true},
		{"too_long_len_46", "ffff:ffff:ffff:ffff:ffff:ffff:255.255.255.255f", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertTokenValid(t, tt.token, tt.valid)
		})
	}
}

func TestIPv6_WithZone_LengthBoundaries(t *testing.T) {
	tests := []struct {
		name  string
		token string
		valid bool
	}{
		{"min_valid_with_zone_len_4", "::%1", true},
		{"max_zone_len_15", "fe80::1%1234567890abcde", true},
		{"zone_too_long_len_16", "fe80::1%1234567890abcdef", false},
		{"empty_zone_rejected", "fe80::1%", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertTokenValid(t, tt.token, tt.valid)
		})
	}
}

func TestIPv6_WithZone_TotalLengthBoundaries(t *testing.T) {
	tests := []struct {
		name  string
		token string
		valid bool
	}{
		{
			"max_total_len_61",
			"ffff:ffff:ffff:ffff:ffff:ffff:255.255.255.255%1234567890abcde",
			true,
		},
		{
			"too_long_total_len_62",
			"ffff:ffff:ffff:ffff:ffff:ffff:255.255.255.255%1234567890abcdef",
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertTokenValid(t, tt.token, tt.valid)
		})
	}
}

// --- Boundary character filters ---

func TestIPv4_BoundaryChars(t *testing.T) {
	tests := []struct {
		name  string
		token string
		valid bool
	}{
		{"start_digit_end_digit_valid", "1.2.3.4", true},
		{"start_non_digit_invalid", "a1.2.3.4", false},
		{"end_non_digit_invalid", "1.2.3.a", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertTokenValid(t, tt.token, tt.valid)
		})
	}
}

func TestIPv6_BoundaryChars_WithoutZone(t *testing.T) {
	tests := []struct {
		name  string
		token string
		valid bool
	}{
		{"start_colon_end_hex_valid", "::1", true},
		{"start_hex_end_colon_valid", "2001:db8::", true},
		{"start_dot_invalid", ".::1", false},
		{"end_dot_invalid", "::1.", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertTokenValid(t, tt.token, tt.valid)
		})
	}
}

func TestIPv6_BoundaryChars_WithZone(t *testing.T) {
	tests := []struct {
		name  string
		token string
		valid bool
	}{
		{"normal_zone_valid", "fe80::1%1", true},
		{"interface_zone_valid", "::1%eth0", true},
		{"interface_zone_with_dot_valid", "::1%eth0.1", true},
		{"interface_zone_with_hyphen_valid", "::1%eth0-a", true},
		{"interface_zone_with_underscore_valid", "::1%eth0_a", true},
		{"zone_with_dot_valid", "fe80::1%1.2", true},
		{"zone_with_hyphen_valid", "fe80::1%br-abcd", true},
		{"zone_with_underscore_valid", "fe80::1%veth_abcd", true},
		{"start_dot_invalid", ".fe80::1%1", false},
		{"empty_zone_invalid", "fe80::1%", false},
		{"zone_starts_dot_valid", "fe80::1%.eth0", true},
		{"zone_starts_hyphen_valid", "fe80::1%-eth0", true},
		{"zone_starts_underscore_valid", "fe80::1%_eth0", true},
		{"loopback_zone_starts_dot_valid", "::1%.eth0", true},
		{"loopback_zone_starts_hyphen_valid", "::1%-eth0", true},
		{"loopback_zone_starts_underscore_valid", "::1%_eth0", true},
		{"zone_ends_dot_valid", "fe80::1%eth0.", true},
		{"zone_ends_hyphen_valid", "fe80::1%eth0-", true},
		{"zone_ends_underscore_valid", "fe80::1%eth0_", true},
		{"loopback_zone_ends_dot_valid", "::1%eth0.", true},
		{"loopback_zone_ends_hyphen_valid", "::1%eth0-", true},
		{"loopback_zone_ends_underscore_valid", "::1%eth0_", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertTokenValid(t, tt.token, tt.valid)
		})
	}
}

func ExampleNewStreamer() {
	s := ipstream.NewStreamer(ipstream.HandleFunc(func(_ []byte, addr netip.Addr) {
		if addr.IsValid() {
			fmt.Println(addr)
		}
	}))
	s.Write([]byte("client=192.168.1.1 gateway=2001:db8::1"))
	s.Flush()
	// Output:
	// 192.168.1.1
	// 2001:db8::1
}

func ExampleStreamer_Writer() {
	s := ipstream.NewStreamer(ipstream.HandleFunc(func(_ []byte, addr netip.Addr) {
		if addr.IsValid() {
			fmt.Println(addr)
		}
	}))
	w := s.Writer()
	_, _ = w.Write([]byte("client=10.0.0.1 peer=::1"))
	s.Flush()
	// Output:
	// 10.0.0.1
	// ::1
}

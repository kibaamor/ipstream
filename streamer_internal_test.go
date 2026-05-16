//go:build ipstreamtests
// +build ipstreamtests

package ipstream

import (
	"net/netip"
	"testing"
)

func TestParseIPv4Fast_Valid(t *testing.T) {
	tests := []struct {
		name string
		ip   string
	}{
		{name: "min", ip: "0.0.0.0"},
		{name: "max", ip: "255.255.255.255"},
		{name: "typical", ip: "192.168.1.1"},
		{name: "single_digits", ip: "1.2.3.4"},
		{name: "mixed_digits", ip: "10.20.30.40"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := netip.MustParseAddr(tt.ip)
			got, ok := parseIPv4Fast([]byte(tt.ip))
			if !ok {
				t.Fatalf("parseIPv4Fast(%q) = invalid, want %v", tt.ip, want)
			}
			if got != want {
				t.Fatalf("parseIPv4Fast(%q) = %v, want %v", tt.ip, got, want)
			}
		})
	}
}

func TestParseIPv4Fast_Invalid(t *testing.T) {
	tests := []struct {
		name string
		ip   string
	}{
		{name: "leading_zero_first_octet", ip: "01.2.3.4"},
		{name: "leading_zero_middle_octet", ip: "1.02.3.4"},
		{name: "leading_zero_last_octet", ip: "1.2.3.04"},
		{name: "octet_overflow", ip: "256.1.1.1"},
		{name: "octet_overflow_large", ip: "999.1.1.1"},
		{name: "too_few_octets", ip: "1.2.3"},
		{name: "too_few_octets_len_7", ip: "1.2.333"},
		{name: "too_few_octets_second_octet_len_7", ip: "111.222"},
		{name: "too_few_octets_third_octet_len_7", ip: "1.2.255"},
		{name: "too_many_octets", ip: "1.2.3.4.5"},
		{name: "empty_first_octet", ip: ".1.2.3"},
		{name: "empty_middle_octet", ip: "1..2.3"},
		{name: "empty_last_octet", ip: "1.2.3."},
		{name: "alpha_first_octet", ip: "a.2.3.4"},
		{name: "alpha_middle_octet", ip: "1.2.b.4"},
		{name: "alpha_last_octet", ip: "1.2.3.c"},
		{name: "alpha_third_char_first_octet", ip: "12a.1.1.1"},
		{name: "alpha_first_char_second_octet", ip: "1.a.1.1"},
		{name: "alpha_third_char_second_octet", ip: "1.12a.1.1"},
		{name: "overflow_second_octet", ip: "1.256.1.1"},
		{name: "too_many_digits_second_octet", ip: "1.1234.1.1"},
		{name: "alpha_third_char_third_octet", ip: "1.1.12a.1"},
		{name: "overflow_third_octet", ip: "1.1.256.1"},
		{name: "too_many_digits_third_octet", ip: "1.1.1234.1"},
		{name: "alpha_third_char_last_octet", ip: "1.1.1.12a"},
		{name: "too_many_digits_last_octet", ip: "1.1.1.1234"},
		{name: "negative_like", ip: "1.2.3.-1"},
		{name: "too_many_digits_in_octet", ip: "1111.1.1.1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, ok := parseIPv4Fast([]byte(tt.ip)); ok {
				t.Fatalf("parseIPv4Fast(%q) = (%v, true), want invalid", tt.ip, got)
			}
		})
	}
}

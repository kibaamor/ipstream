//go:build ipstreamtests
// +build ipstreamtests

package ipstream

import (
	"net/netip"
	"testing"
)

func BenchmarkParseIPv4Fast_Valid(b *testing.B) {
	tests := []struct {
		name string
		ip   []byte
		want netip.Addr
	}{
		{name: "min", ip: []byte("0.0.0.0"), want: netip.MustParseAddr("0.0.0.0")},
		{name: "max", ip: []byte("255.255.255.255"), want: netip.MustParseAddr("255.255.255.255")},
		{name: "typical", ip: []byte("192.168.1.1"), want: netip.MustParseAddr("192.168.1.1")},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			b.SetBytes(int64(len(tt.ip)))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				got, ok := parseIPv4Fast(tt.ip)
				if !ok || got != tt.want {
					b.Fatalf("parseIPv4Fast(%q) = (%v, %v), want (%v, true)", tt.ip, got, ok, tt.want)
				}
			}
		})
	}
}

func BenchmarkParseIPv4Fast_Invalid(b *testing.B) {
	tests := []struct {
		name string
		ip   []byte
	}{
		{name: "trailing_dot", ip: []byte("1.2.3.")},
		{name: "too_few_octets", ip: []byte("1.2.3")},
		{name: "too_many_octets", ip: []byte("1.2.3.4.5")},
		{name: "leading_zero", ip: []byte("192.168.01.1")},
		{name: "octet_overflow", ip: []byte("256.1.1.1")},
		{name: "alpha", ip: []byte("a.b.c.d")},
		{name: "empty_octet", ip: []byte("1..1.1")},
		{name: "too_many_digits", ip: []byte("1111.1.1.1")},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			b.SetBytes(int64(len(tt.ip)))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if got, ok := parseIPv4Fast(tt.ip); ok {
					b.Fatalf("parseIPv4Fast(%q) = (%v, true), want invalid", tt.ip, got)
				}
			}
		})
	}
}

func BenchmarkParseIPv4Fast_RejectStages(b *testing.B) {
	tests := []struct {
		name string
		ip   []byte
	}{
		{name: "reject_first_char", ip: []byte("a.1.1.1")},
		{name: "reject_leading_zero", ip: []byte("1.01.1.1")},
		{name: "reject_overflow_third_digit", ip: []byte("255.255.255.256")},
		{name: "reject_too_many_digits_in_octet", ip: []byte("1.2.3.4444")},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			b.SetBytes(int64(len(tt.ip)))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if got, ok := parseIPv4Fast(tt.ip); ok {
					b.Fatalf("parseIPv4Fast(%q) = (%v, true), want invalid", tt.ip, got)
				}
			}
		})
	}
}

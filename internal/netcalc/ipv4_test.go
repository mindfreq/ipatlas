package netcalc

import (
	"errors"
	"net/netip"
	"strconv"
	"strings"
	"testing"
)

func TestParseIPv4CIDR(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		address   string
		network   string
		prefix    int
		mask      string
		wildcard  string
		broadcast string
		first     string
		last      string
		gateway   string
		usable    uint64
	}{
		{
			name:      "class C",
			input:     "192.168.1.0/24",
			address:   "192.168.1.0",
			network:   "192.168.1.0",
			prefix:    24,
			mask:      "255.255.255.0",
			wildcard:  "0.0.0.255",
			broadcast: "192.168.1.255",
			first:     "192.168.1.1",
			last:      "192.168.1.254",
			gateway:   "192.168.1.1",
			usable:    254,
		},
		{
			name:      "host bits are cleared",
			input:     "192.168.1.5/24",
			address:   "192.168.1.5",
			network:   "192.168.1.0",
			prefix:    24,
			mask:      "255.255.255.0",
			wildcard:  "0.0.0.255",
			broadcast: "192.168.1.255",
			first:     "192.168.1.1",
			last:      "192.168.1.254",
			gateway:   "192.168.1.1",
			usable:    254,
		},
		{
			name:      "point to point",
			input:     "10.0.0.0/30",
			address:   "10.0.0.0",
			network:   "10.0.0.0",
			prefix:    30,
			mask:      "255.255.255.252",
			wildcard:  "0.0.0.3",
			broadcast: "10.0.0.3",
			first:     "10.0.0.1",
			last:      "10.0.0.2",
			gateway:   "10.0.0.1",
			usable:    2,
		},
		{
			name:      "rfc 3021 slash 31",
			input:     "10.0.0.10/31",
			address:   "10.0.0.10",
			network:   "10.0.0.10",
			prefix:    31,
			mask:      "255.255.255.254",
			wildcard:  "0.0.0.1",
			broadcast: "10.0.0.11",
			first:     "10.0.0.10",
			last:      "10.0.0.11",
			gateway:   "10.0.0.10",
			usable:    2,
		},
		{
			name:      "single host",
			input:     "10.0.0.7/32",
			address:   "10.0.0.7",
			network:   "10.0.0.7",
			prefix:    32,
			mask:      "255.255.255.255",
			wildcard:  "0.0.0.0",
			broadcast: "10.0.0.7",
			first:     "10.0.0.7",
			last:      "10.0.0.7",
			gateway:   "10.0.0.7",
			usable:    1,
		},
		{
			name:      "default route",
			input:     "0.0.0.0/0",
			address:   "0.0.0.0",
			network:   "0.0.0.0",
			prefix:    0,
			mask:      "0.0.0.0",
			wildcard:  "255.255.255.255",
			broadcast: "255.255.255.255",
			first:     "0.0.0.1",
			last:      "255.255.255.254",
			gateway:   "0.0.0.1",
			usable:    4294967294,
		},
		{
			name:      "class B private",
			input:     "172.16.0.0/12",
			address:   "172.16.0.0",
			network:   "172.16.0.0",
			prefix:    12,
			mask:      "255.240.0.0",
			wildcard:  "0.15.255.255",
			broadcast: "172.31.255.255",
			first:     "172.16.0.1",
			last:      "172.31.255.254",
			gateway:   "172.16.0.1",
			usable:    1048574,
		},
		{
			name:      "unaligned block",
			input:     "203.0.113.42/26",
			address:   "203.0.113.42",
			network:   "203.0.113.0",
			prefix:    26,
			mask:      "255.255.255.192",
			wildcard:  "0.0.0.63",
			broadcast: "203.0.113.63",
			first:     "203.0.113.1",
			last:      "203.0.113.62",
			gateway:   "203.0.113.1",
			usable:    62,
		},
		{
			name:      "all ones",
			input:     "255.255.255.255/32",
			address:   "255.255.255.255",
			network:   "255.255.255.255",
			prefix:    32,
			mask:      "255.255.255.255",
			wildcard:  "0.0.0.0",
			broadcast: "255.255.255.255",
			first:     "255.255.255.255",
			last:      "255.255.255.255",
			gateway:   "255.255.255.255",
			usable:    1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseIPv4CIDR(tc.input)
			if err != nil {
				t.Fatalf("ParseIPv4CIDR(%q): %v", tc.input, err)
			}

			checkAddr(t, "Address", got.Address, tc.address)
			checkAddr(t, "Network", got.Network, tc.network)
			checkAddr(t, "Mask", got.Mask, tc.mask)
			checkAddr(t, "Wildcard", got.Wildcard, tc.wildcard)
			checkAddr(t, "Broadcast", got.Broadcast, tc.broadcast)
			checkAddr(t, "First", got.First, tc.first)
			checkAddr(t, "Last", got.Last, tc.last)
			checkAddr(t, "Gateway", got.Gateway, tc.gateway)
			if got.Prefix != tc.prefix {
				t.Errorf("Prefix = %d, want %d", got.Prefix, tc.prefix)
			}
			if got.Usable != tc.usable {
				t.Errorf("Usable = %d, want %d", got.Usable, tc.usable)
			}
		})
	}
}

func TestParseIPv4CIDRInvalid(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  error
	}{
		{name: "empty", input: "", want: ErrInvalidCIDR},
		{name: "missing prefix", input: "192.168.1.1", want: ErrInvalidCIDR},
		{name: "prefix too large", input: "192.168.1.1/33", want: ErrInvalidCIDR},
		{name: "negative prefix", input: "192.168.1.1/-1", want: ErrInvalidCIDR},
		{name: "bad address", input: "not-an-ip/24", want: ErrInvalidCIDR},
		{name: "short address", input: "192.168.1/24", want: ErrInvalidCIDR},
		{name: "two prefixes", input: "192.168.1.1/24/8", want: ErrInvalidCIDR},
		{name: "surrounding space", input: " 192.168.1.1/24", want: ErrInvalidCIDR},
		{name: "ipv6", input: "2001:db8::/32", want: ErrNotIPv4},
		{name: "ipv4 mapped ipv6", input: "::ffff:192.168.1.0/120", want: ErrNotIPv4},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseIPv4CIDR(tc.input)
			if !errors.Is(err, tc.want) {
				t.Fatalf("ParseIPv4CIDR(%q) error = %v, want %v", tc.input, err, tc.want)
			}
			if err != nil && !strings.Contains(err.Error(), tc.input) {
				t.Errorf("error %q does not mention input %q", err, tc.input)
			}
		})
	}
}

func TestIPv4SubnetInvariants(t *testing.T) {
	const input = "203.0.113.200"

	for bits := 0; bits <= 32; bits++ {
		t.Run("slash "+strconv.Itoa(bits), func(t *testing.T) {
			got, err := ParseIPv4CIDR(input + "/" + strconv.Itoa(bits))
			if err != nil {
				t.Fatalf("ParseIPv4CIDR: %v", err)
			}

			network := addrToUint32(got.Network)
			broadcast := addrToUint32(got.Broadcast)
			mask := addrToUint32(got.Mask)
			wildcard := addrToUint32(got.Wildcard)

			if mask&wildcard != 0 || mask|wildcard != ^uint32(0) {
				t.Errorf("mask %s and wildcard %s are not complements", got.Mask, got.Wildcard)
			}
			if network&wildcard != 0 {
				t.Errorf("network %s has host bits set", got.Network)
			}
			if broadcast != network|wildcard {
				t.Errorf("broadcast %s is not network %s plus wildcard %s", got.Broadcast, got.Network, got.Wildcard)
			}
			if got.First.Compare(got.Network) < 0 || got.Last.Compare(got.Broadcast) > 0 {
				t.Errorf("usable range %s-%s is outside %s-%s", got.First, got.Last, got.Network, got.Broadcast)
			}
			if got.First.Compare(got.Last) > 0 {
				t.Errorf("first %s is after last %s", got.First, got.Last)
			}
			if got.Gateway != got.First {
				t.Errorf("gateway %s is not first usable %s", got.Gateway, got.First)
			}

			total := uint64(1) << (32 - bits)
			wantUsable := total - 2
			switch bits {
			case 31:
				wantUsable = 2
			case 32:
				wantUsable = 1
			}
			if got.Usable != wantUsable {
				t.Errorf("Usable = %d, want %d", got.Usable, wantUsable)
			}
		})
	}
}

func checkAddr(t *testing.T, field string, got netip.Addr, want string) {
	t.Helper()
	wantAddr := netip.MustParseAddr(want)
	if got != wantAddr {
		t.Errorf("%s = %s, want %s", field, got, wantAddr)
	}
}

package netcalc

import (
	"errors"
	"net/netip"
	"testing"
)

func TestParseIPv6Address(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		compressed string
		expanded   string
		kind       IPv6Kind
	}{
		{
			name:       "compressed global",
			input:      "2001:db8::1",
			compressed: "2001:db8::1",
			expanded:   "2001:0db8:0000:0000:0000:0000:0000:0001",
			kind:       IPv6GlobalUnicast,
		},
		{
			name:       "expanded input",
			input:      "2001:0db8:0000:0000:0000:0000:0000:0001",
			compressed: "2001:db8::1",
			expanded:   "2001:0db8:0000:0000:0000:0000:0000:0001",
			kind:       IPv6GlobalUnicast,
		},
		{
			name:       "loopback",
			input:      "::1",
			compressed: "::1",
			expanded:   "0000:0000:0000:0000:0000:0000:0000:0001",
			kind:       IPv6Loopback,
		},
		{
			name:       "link local",
			input:      "fe80::1",
			compressed: "fe80::1",
			expanded:   "fe80:0000:0000:0000:0000:0000:0000:0001",
			kind:       IPv6LinkLocal,
		},
		{
			name:       "unique local",
			input:      "fd00::1",
			compressed: "fd00::1",
			expanded:   "fd00:0000:0000:0000:0000:0000:0000:0001",
			kind:       IPv6UniqueLocal,
		},
		{
			name:       "multicast",
			input:      "ff02::1",
			compressed: "ff02::1",
			expanded:   "ff02:0000:0000:0000:0000:0000:0000:0001",
			kind:       IPv6Multicast,
		},
		{
			name:       "unspecified",
			input:      "::",
			compressed: "::",
			expanded:   "0000:0000:0000:0000:0000:0000:0000:0000",
			kind:       IPv6Unspecified,
		},
		{
			name:       "ipv4 mapped",
			input:      "::ffff:192.0.2.1",
			compressed: "::ffff:192.0.2.1",
			expanded:   "0000:0000:0000:0000:0000:ffff:c000:0201",
			kind:       IPv6IPv4Mapped,
		},
		{
			name:       "zoned link local",
			input:      "fe80::1%eth0",
			compressed: "fe80::1%eth0",
			expanded:   "fe80:0000:0000:0000:0000:0000:0000:0001",
			kind:       IPv6LinkLocal,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseIPv6Address(tc.input)
			if err != nil {
				t.Fatalf("ParseIPv6Address(%q): %v", tc.input, err)
			}
			if got.Compressed != tc.compressed {
				t.Errorf("compressed = %q, want %q", got.Compressed, tc.compressed)
			}
			if got.Expanded != tc.expanded {
				t.Errorf("expanded = %q, want %q", got.Expanded, tc.expanded)
			}
			if got.Kind != tc.kind {
				t.Errorf("kind = %s, want %s", got.Kind, tc.kind)
			}
			if got.Address.String() != tc.compressed {
				t.Errorf("address = %s, want %s", got.Address, tc.compressed)
			}
		})
	}
}

func TestParseIPv6AddressInvalid(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  error
	}{
		{name: "empty", input: "", want: ErrInvalidIPv6},
		{name: "garbage", input: "not-an-ip", want: ErrInvalidIPv6},
		{name: "prefix instead of address", input: "2001:db8::/64", want: ErrInvalidIPv6},
		{name: "ipv4", input: "192.168.1.1", want: ErrNotIPv6},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseIPv6Address(tc.input)
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestIPv6Helpers(t *testing.T) {
	if got := ClassifyIPv6(netip.MustParseAddr("192.0.2.1")); got != IPv6Other {
		t.Errorf("IPv4 kind = %s, want other", got)
	}
	if got := ExpandIPv6(netip.MustParseAddr("192.0.2.1")); got != "" {
		t.Errorf("ExpandIPv6(IPv4) = %q, want empty", got)
	}
	if got := CompressIPv6(netip.MustParseAddr("192.0.2.1")); got != "" {
		t.Errorf("CompressIPv6(IPv4) = %q, want empty", got)
	}

	addr := netip.MustParseAddr("2001:db8::1")
	if got := ExpandIPv6(addr); got != "2001:0db8:0000:0000:0000:0000:0000:0001" {
		t.Errorf("expanded = %q", got)
	}
	if got := CompressIPv6(addr); got != "2001:db8::1" {
		t.Errorf("compressed = %q", got)
	}
	for kind := IPv6Other; kind <= IPv6IPv4Mapped; kind++ {
		if kind.String() == "" {
			t.Errorf("IPv6Kind(%d).String() is empty", kind)
		}
	}
}

func TestNewIPv6Split(t *testing.T) {
	tests := []struct {
		name      string
		parent    string
		childBits int
		wantTotal uint64
		wantErr   error
	}{
		{name: "48 to 64", parent: "2001:db8::/48", childBits: 64, wantTotal: 65536},
		{name: "host bits normalized", parent: "2001:db8::1/48", childBits: 64, wantTotal: 65536},
		{name: "same prefix", parent: "2001:db8::/64", childBits: 64, wantTotal: 1},
		{name: "maximum difference", parent: "::/32", childBits: 64, wantTotal: 1 << 32},
		{name: "invalid parent", parent: "not-a-prefix", childBits: 64, wantErr: ErrInvalidCIDR},
		{name: "ipv4 parent", parent: "192.168.0.0/24", childBits: 64, wantErr: ErrNotIPv6},
		{name: "child shorter than parent", parent: "2001:db8::/48", childBits: 40, wantErr: ErrInvalidIPv6Split},
		{name: "child past 128", parent: "2001:db8::/48", childBits: 129, wantErr: ErrInvalidIPv6Split},
		{name: "difference too large", parent: "::/0", childBits: 64, wantErr: ErrIPv6SplitTooLarge},
		{name: "difference just too large", parent: "::/0", childBits: 33, wantErr: ErrIPv6SplitTooLarge},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			split, err := NewIPv6Split(tc.parent, tc.childBits)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("error = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewIPv6Split: %v", err)
			}
			if got := split.Total(); got != tc.wantTotal {
				t.Errorf("total = %d, want %d", got, tc.wantTotal)
			}
			if split.ChildBits() != tc.childBits {
				t.Errorf("child bits = %d, want %d", split.ChildBits(), tc.childBits)
			}
			if !split.Parent().Addr().Is6() {
				t.Errorf("parent = %s, want IPv6", split.Parent())
			}
		})
	}
}

func TestIPv6SplitPaging(t *testing.T) {
	split, err := NewIPv6Split("2001:db8::/48", 64)
	if err != nil {
		t.Fatalf("NewIPv6Split: %v", err)
	}

	page, err := split.Page(0, 3)
	if err != nil {
		t.Fatalf("Page: %v", err)
	}
	want := []string{
		"2001:db8::/64",
		"2001:db8:0:1::/64",
		"2001:db8:0:2::/64",
	}
	if len(page.Prefixes) != len(want) {
		t.Fatalf("prefixes = %d, want %d", len(page.Prefixes), len(want))
	}
	for i, w := range want {
		if got := page.Prefixes[i].String(); got != w {
			t.Errorf("prefix %d = %s, want %s", i, got, w)
		}
		if page.Prefixes[i].Bits() != 64 {
			t.Errorf("prefix %d bits = %d, want 64", i, page.Prefixes[i].Bits())
		}
		if !split.Parent().Contains(page.Prefixes[i].Addr()) {
			t.Errorf("prefix %d %s is outside %s", i, page.Prefixes[i], split.Parent())
		}
	}
	if page.Offset != 0 || page.End() != 3 || page.Total != 65536 {
		t.Errorf("page offset/end/total = %d/%d/%d, want 0/3/65536", page.Offset, page.End(), page.Total)
	}
	if page.HasPrev() || !page.HasNext() {
		t.Errorf("page prev/next = %v/%v, want false/true", page.HasPrev(), page.HasNext())
	}

	middle, err := split.Page(100, 2)
	if err != nil {
		t.Fatalf("Page(100): %v", err)
	}
	if got := middle.Prefixes[0].String(); got != "2001:db8:0:64::/64" {
		t.Errorf("middle first = %s, want 2001:db8:0:64::/64", got)
	}
	if !middle.HasPrev() || !middle.HasNext() {
		t.Errorf("middle prev/next = %v/%v, want true/true", middle.HasPrev(), middle.HasNext())
	}

	last, err := split.Page(65535, 256)
	if err != nil {
		t.Fatalf("Page(65535): %v", err)
	}
	if len(last.Prefixes) != 1 {
		t.Fatalf("last page has %d prefixes, want 1", len(last.Prefixes))
	}
	if got := last.Prefixes[0].String(); got != "2001:db8:0:ffff::/64" {
		t.Errorf("last prefix = %s, want 2001:db8:0:ffff::/64", got)
	}
	if !last.HasPrev() || last.HasNext() {
		t.Errorf("last prev/next = %v/%v, want true/false", last.HasPrev(), last.HasNext())
	}
}

func TestIPv6SplitPageBounds(t *testing.T) {
	split, err := NewIPv6Split("2001:db8::/48", 64)
	if err != nil {
		t.Fatalf("NewIPv6Split: %v", err)
	}

	capped, err := split.Page(0, MaxIPv6PageSize+1)
	if err != nil {
		t.Fatalf("Page with large limit: %v", err)
	}
	if len(capped.Prefixes) != MaxIPv6PageSize {
		t.Errorf("capped page = %d, want %d", len(capped.Prefixes), MaxIPv6PageSize)
	}

	if _, err := split.Page(65536, 1); !errors.Is(err, ErrIPv6PageOutOfRange) {
		t.Errorf("offset past end error = %v, want ErrIPv6PageOutOfRange", err)
	}
	for _, limit := range []int{0, -1} {
		if _, err := split.Page(0, limit); !errors.Is(err, ErrIPv6PageSize) {
			t.Errorf("limit %d error = %v, want ErrIPv6PageSize", limit, err)
		}
	}
}

func TestIPv6SplitSinglePrefix(t *testing.T) {
	split, err := NewIPv6Split("2001:db8::/64", 64)
	if err != nil {
		t.Fatalf("NewIPv6Split: %v", err)
	}
	if split.Total() != 1 {
		t.Fatalf("total = %d, want 1", split.Total())
	}
	page, err := split.Page(0, 256)
	if err != nil {
		t.Fatalf("Page: %v", err)
	}
	if len(page.Prefixes) != 1 || page.Prefixes[0].String() != "2001:db8::/64" {
		t.Errorf("page = %v", page.Prefixes)
	}
	if page.HasPrev() || page.HasNext() {
		t.Errorf("single page prev/next = %v/%v, want false/false", page.HasPrev(), page.HasNext())
	}
}

func TestIPv6SplitLastPrefix(t *testing.T) {
	tests := []struct {
		parent   string
		child    int
		wantLast string
	}{
		{parent: "2001:db8::/32", child: 64, wantLast: "2001:db8:ffff:ffff::/64"},
		{parent: "::/0", child: 32, wantLast: "ffff:ffff::/32"},
	}

	for _, tc := range tests {
		t.Run(tc.parent, func(t *testing.T) {
			split, err := NewIPv6Split(tc.parent, tc.child)
			if err != nil {
				t.Fatalf("NewIPv6Split: %v", err)
			}
			page, err := split.Page(split.Total()-1, 1)
			if err != nil {
				t.Fatalf("Page: %v", err)
			}
			if got := page.Prefixes[0].String(); got != tc.wantLast {
				t.Errorf("last prefix = %s, want %s", got, tc.wantLast)
			}
			if page.HasNext() {
				t.Errorf("last page has a next page")
			}
		})
	}
}

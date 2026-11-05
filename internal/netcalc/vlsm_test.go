package netcalc

import (
	"errors"
	"testing"
)

func TestAllocateVLSMExample(t *testing.T) {
	plan, err := AllocateVLSM("192.168.1.0/24", []VLSMRequest{
		{Name: "Programming", Hosts: 100},
		{Name: "Management", Hosts: 50},
		{Name: "Servers", Hosts: 10},
	})
	if err != nil {
		t.Fatalf("AllocateVLSM: %v", err)
	}
	if got := plan.Parent.Network.String(); got != "192.168.1.0" {
		t.Errorf("parent network = %s, want 192.168.1.0", got)
	}

	want := []struct {
		name      string
		network   string
		prefix    int
		mask      string
		first     string
		last      string
		gateway   string
		broadcast string
	}{
		{
			name: "Programming", network: "192.168.1.0", prefix: 25,
			mask: "255.255.255.128", first: "192.168.1.1", last: "192.168.1.126",
			gateway: "192.168.1.1", broadcast: "192.168.1.127",
		},
		{
			name: "Management", network: "192.168.1.128", prefix: 26,
			mask: "255.255.255.192", first: "192.168.1.129", last: "192.168.1.190",
			gateway: "192.168.1.129", broadcast: "192.168.1.191",
		},
		{
			name: "Servers", network: "192.168.1.192", prefix: 28,
			mask: "255.255.255.240", first: "192.168.1.193", last: "192.168.1.206",
			gateway: "192.168.1.193", broadcast: "192.168.1.207",
		},
	}

	if len(plan.Allocations) != len(want) {
		t.Fatalf("allocations = %d, want %d", len(plan.Allocations), len(want))
	}
	for i, w := range want {
		got := plan.Allocations[i]
		if got.Name != w.name {
			t.Errorf("allocation %d name = %q, want %q", i, got.Name, w.name)
		}
		checkAddr(t, "Network", got.Subnet.Network, w.network)
		if got.Subnet.Prefix != w.prefix {
			t.Errorf("%s prefix = %d, want %d", w.name, got.Subnet.Prefix, w.prefix)
		}
		checkAddr(t, "Mask", got.Subnet.Mask, w.mask)
		checkAddr(t, "First", got.Subnet.First, w.first)
		checkAddr(t, "Last", got.Subnet.Last, w.last)
		checkAddr(t, "Gateway", got.Subnet.Gateway, w.gateway)
		checkAddr(t, "Broadcast", got.Subnet.Broadcast, w.broadcast)
	}
}

func TestAllocateVLSMLargestFirst(t *testing.T) {
	plan, err := AllocateVLSM("10.0.0.0/24", []VLSMRequest{
		{Name: "Small", Hosts: 10},
		{Name: "Large", Hosts: 100},
		{Name: "Medium", Hosts: 50},
	})
	if err != nil {
		t.Fatalf("AllocateVLSM: %v", err)
	}

	want := []struct {
		name    string
		network string
		prefix  int
	}{
		{name: "Large", network: "10.0.0.0", prefix: 25},
		{name: "Medium", network: "10.0.0.128", prefix: 26},
		{name: "Small", network: "10.0.0.192", prefix: 28},
	}
	for i, w := range want {
		got := plan.Allocations[i]
		if got.Name != w.name {
			t.Errorf("allocation %d = %q, want %q", i, got.Name, w.name)
		}
		checkAddr(t, "Network", got.Subnet.Network, w.network)
		if got.Subnet.Prefix != w.prefix {
			t.Errorf("%s prefix = %d, want %d", w.name, got.Subnet.Prefix, w.prefix)
		}
	}
}

func TestAllocateVLSMAlignment(t *testing.T) {
	plan, err := AllocateVLSM("10.0.0.0/24", []VLSMRequest{
		{Name: "Three", Hosts: 3},
		{Name: "Two", Hosts: 2},
		{Name: "Another two", Hosts: 2},
	})
	if err != nil {
		t.Fatalf("AllocateVLSM: %v", err)
	}

	want := []struct {
		name    string
		network string
		prefix  int
	}{
		{name: "Three", network: "10.0.0.0", prefix: 29},
		{name: "Two", network: "10.0.0.8", prefix: 30},
		{name: "Another two", network: "10.0.0.12", prefix: 30},
	}
	for i, w := range want {
		got := plan.Allocations[i]
		if got.Name != w.name {
			t.Errorf("allocation %d = %q, want %q", i, got.Name, w.name)
		}
		checkAddr(t, "Network", got.Subnet.Network, w.network)
		if got.Subnet.Prefix != w.prefix {
			t.Errorf("%s prefix = %d, want %d", w.name, got.Subnet.Prefix, w.prefix)
		}
	}
}

func TestAllocateVLSMEqualSizesKeepInputOrder(t *testing.T) {
	plan, err := AllocateVLSM("10.0.0.0/24", []VLSMRequest{
		{Name: "First", Hosts: 10},
		{Name: "Second", Hosts: 10},
	})
	if err != nil {
		t.Fatalf("AllocateVLSM: %v", err)
	}
	if plan.Allocations[0].Name != "First" || plan.Allocations[1].Name != "Second" {
		t.Errorf("order = %q, %q; want First, Second", plan.Allocations[0].Name, plan.Allocations[1].Name)
	}
	checkAddr(t, "First network", plan.Allocations[0].Subnet.Network, "10.0.0.0")
	checkAddr(t, "Second network", plan.Allocations[1].Subnet.Network, "10.0.0.16")
}

func TestAllocateVLSMMinimumBlock(t *testing.T) {
	plan, err := AllocateVLSM("10.0.0.0/24", []VLSMRequest{
		{Name: "One", Hosts: 1},
		{Name: "Two", Hosts: 2},
	})
	if err != nil {
		t.Fatalf("AllocateVLSM: %v", err)
	}
	for i, network := range []string{"10.0.0.0", "10.0.0.4"} {
		got := plan.Allocations[i]
		checkAddr(t, "Network", got.Subnet.Network, network)
		if got.Subnet.Prefix != 30 {
			t.Errorf("%s prefix = %d, want 30", got.Name, got.Subnet.Prefix)
		}
	}
}

func TestAllocateVLSMFullParent(t *testing.T) {
	plan, err := AllocateVLSM("10.0.0.0/24", []VLSMRequest{
		{Name: "A", Hosts: 126},
		{Name: "B", Hosts: 126},
	})
	if err != nil {
		t.Fatalf("AllocateVLSM: %v", err)
	}
	checkAddr(t, "A network", plan.Allocations[0].Subnet.Network, "10.0.0.0")
	checkAddr(t, "B network", plan.Allocations[1].Subnet.Network, "10.0.0.128")

	_, err = AllocateVLSM("10.0.0.0/24", []VLSMRequest{
		{Name: "A", Hosts: 126},
		{Name: "B", Hosts: 126},
		{Name: "C", Hosts: 1},
	})
	if !errors.Is(err, ErrVLSMNoSpace) {
		t.Fatalf("error = %v, want ErrVLSMNoSpace", err)
	}
}

func TestAllocateVLSMDefaultRoute(t *testing.T) {
	plan, err := AllocateVLSM("0.0.0.0/0", []VLSMRequest{
		{Name: "Everything", Hosts: maxIPv4Usable},
	})
	if err != nil {
		t.Fatalf("AllocateVLSM: %v", err)
	}
	got := plan.Allocations[0].Subnet
	checkAddr(t, "Network", got.Network, "0.0.0.0")
	if got.Prefix != 0 {
		t.Errorf("prefix = %d, want 0", got.Prefix)
	}
}

func TestAllocateVLSMErrors(t *testing.T) {
	tests := []struct {
		name     string
		parent   string
		requests []VLSMRequest
		want     error
	}{
		{
			name:   "no requests",
			parent: "10.0.0.0/24",
			want:   ErrNoVLSMRequests,
		},
		{
			name:     "zero hosts",
			parent:   "10.0.0.0/24",
			requests: []VLSMRequest{{Name: "Empty", Hosts: 0}},
			want:     ErrInvalidHosts,
		},
		{
			name:     "too many hosts",
			parent:   "10.0.0.0/24",
			requests: []VLSMRequest{{Name: "Huge", Hosts: maxIPv4Usable + 1}},
			want:     ErrVLSMNoSpace,
		},
		{
			name:     "invalid parent",
			parent:   "not-a-network/24",
			requests: []VLSMRequest{{Name: "A", Hosts: 10}},
			want:     ErrInvalidCIDR,
		},
		{
			name:     "ipv6 parent",
			parent:   "2001:db8::/32",
			requests: []VLSMRequest{{Name: "A", Hosts: 10}},
			want:     ErrNotIPv4,
		},
		{
			name:     "parent too small",
			parent:   "10.0.0.0/28",
			requests: []VLSMRequest{{Name: "A", Hosts: 10}, {Name: "B", Hosts: 1}},
			want:     ErrVLSMNoSpace,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := AllocateVLSM(tc.parent, tc.requests)
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestAllocateVLSMNonOverlapping(t *testing.T) {
	plan, err := AllocateVLSM("192.168.10.0/24", []VLSMRequest{
		{Name: "A", Hosts: 60},
		{Name: "B", Hosts: 30},
		{Name: "C", Hosts: 10},
		{Name: "D", Hosts: 5},
		{Name: "E", Hosts: 1},
	})
	if err != nil {
		t.Fatalf("AllocateVLSM: %v", err)
	}

	parentStart := addrToUint32(plan.Parent.Network)
	parentEnd := addrToUint32(plan.Parent.Broadcast)
	previousEnd := parentStart - 1
	for _, allocation := range plan.Allocations {
		start := addrToUint32(allocation.Subnet.Network)
		end := addrToUint32(allocation.Subnet.Broadcast)
		size := uint64(1) << (32 - allocation.Subnet.Prefix)

		if start < parentStart || end > parentEnd {
			t.Errorf("%s (%s-%s) is outside the parent", allocation.Name, allocation.Subnet.Network, allocation.Subnet.Broadcast)
		}
		if start <= previousEnd {
			t.Errorf("%s starts at %s, overlapping the previous allocation ending at %d", allocation.Name, allocation.Subnet.Network, previousEnd)
		}
		if uint64(start)%size != 0 {
			t.Errorf("%s network %s is not aligned to /%d", allocation.Name, allocation.Subnet.Network, allocation.Subnet.Prefix)
		}
		previousEnd = end
	}
}

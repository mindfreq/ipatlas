package ui

import (
	"strconv"
	"strings"
	"testing"

	myui "github.com/egoist/mygo/ui"
)

func TestVLSMAllocate(t *testing.T) {
	s := vlsmState{
		parent: "192.168.1.0/24",
		rows: []*vlsmRow{
			{id: 1, name: "Programming", hosts: "100"},
			{id: 2, name: "Management", hosts: "50"},
			{id: 3, name: "Servers", hosts: "10"},
		},
	}
	s.allocate()

	if s.err != "" {
		t.Fatalf("error = %q, want none", s.err)
	}
	if s.plan == nil {
		t.Fatal("plan = nil, want allocations")
	}
	want := []struct {
		name    string
		network string
		prefix  int
	}{
		{name: "Programming", network: "192.168.1.0", prefix: 25},
		{name: "Management", network: "192.168.1.128", prefix: 26},
		{name: "Servers", network: "192.168.1.192", prefix: 28},
	}
	if len(s.plan.Allocations) != len(want) {
		t.Fatalf("allocations = %d, want %d", len(s.plan.Allocations), len(want))
	}
	for i, w := range want {
		got := s.plan.Allocations[i]
		if got.Name != w.name {
			t.Errorf("allocation %d = %q, want %q", i, got.Name, w.name)
		}
		if got.Subnet.Network.String() != w.network || got.Subnet.Prefix != w.prefix {
			t.Errorf("%s = %s/%d, want %s/%d", w.name, got.Subnet.Network, got.Subnet.Prefix, w.network, w.prefix)
		}
	}
}

func TestVLSMAllocateBlankName(t *testing.T) {
	s := vlsmState{
		parent: "10.0.0.0/24",
		rows:   []*vlsmRow{{id: 1, name: "  ", hosts: "10"}},
	}
	s.allocate()

	if s.err != "" {
		t.Fatalf("error = %q, want none", s.err)
	}
	if s.plan == nil || s.plan.Allocations[0].Name != "Department 1" {
		t.Errorf("allocation name = %+v, want Department 1", s.plan)
	}
}

func TestVLSMAllocateErrors(t *testing.T) {
	tests := []struct {
		name string
		s    *vlsmState
		want string
	}{
		{
			name: "empty parent",
			s:    &vlsmState{rows: []*vlsmRow{{id: 1, name: "A", hosts: "10"}}},
			want: "Enter a parent network",
		},
		{
			name: "no departments",
			s:    &vlsmState{parent: "10.0.0.0/24"},
			want: "Add at least one department",
		},
		{
			name: "missing hosts",
			s:    &vlsmState{parent: "10.0.0.0/24", rows: []*vlsmRow{{id: 1, name: "A"}}},
			want: "Enter a host count",
		},
		{
			name: "non numeric hosts",
			s:    &vlsmState{parent: "10.0.0.0/24", rows: []*vlsmRow{{id: 1, name: "A", hosts: "many"}}},
			want: "whole number",
		},
		{
			name: "zero hosts",
			s:    &vlsmState{parent: "10.0.0.0/24", rows: []*vlsmRow{{id: 1, name: "A", hosts: "0"}}},
			want: "at least 1",
		},
		{
			name: "too small",
			s:    &vlsmState{parent: "10.0.0.0/28", rows: []*vlsmRow{{id: 1, name: "A", hosts: "10"}, {id: 2, name: "B", hosts: "10"}}},
			want: "Not enough space",
		},
		{
			name: "ipv6 parent",
			s:    &vlsmState{parent: "2001:db8::/32", rows: []*vlsmRow{{id: 1, name: "A", hosts: "10"}}},
			want: "must be an IPv4 CIDR",
		},
		{
			name: "invalid parent",
			s:    &vlsmState{parent: "nope", rows: []*vlsmRow{{id: 1, name: "A", hosts: "10"}}},
			want: "not a valid CIDR",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.s.allocate()
			if !strings.Contains(tc.s.err, tc.want) {
				t.Errorf("error = %q, want it to contain %q", tc.s.err, tc.want)
			}
			if tc.s.plan != nil {
				t.Errorf("plan = %+v, want nil", tc.s.plan)
			}
		})
	}
}

func TestVLSMPlanner(t *testing.T) {
	a := New()
	tt := myui.NewTester(a.View, 1100, 720)

	if err := tt.Click("VLSM"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("VLSM planner") {
		t.Fatalf("texts %q do not show the VLSM planner", tt.Texts())
	}

	fill := func(label, value string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatalf("click %q: %v", label, err)
		}
		tt.Type(value)
	}

	fill("Parent network", "192.168.1.0/24")
	fill("Department 1", "Programming")
	fill("Hosts 1", "100")

	for i, row := range []struct{ name, hosts string }{
		{name: "Management", hosts: "50"},
		{name: "Servers", hosts: "10"},
	} {
		if err := tt.Click("Add department"); err != nil {
			t.Fatal(err)
		}
		id := i + 2
		fill("Department "+strconv.Itoa(id), row.name)
		fill("Hosts "+strconv.Itoa(id), row.hosts)
	}

	if err := tt.Click("Allocate"); err != nil {
		t.Fatal(err)
	}
	if a.vlsm.err != "" {
		t.Fatalf("error = %q, want none", a.vlsm.err)
	}
	if a.vlsm.plan == nil {
		t.Fatalf("plan = nil, texts %q", tt.Texts())
	}
	if len(a.vlsm.plan.Allocations) != 3 {
		t.Fatalf("allocations = %d, want 3", len(a.vlsm.plan.Allocations))
	}

	for _, want := range []string{
		"Programming",
		"192.168.1.0/25",
		"255.255.255.128",
		"192.168.1.1 - 192.168.1.126",
		"192.168.1.127",
		"Management",
		"192.168.1.128/26",
		"Servers",
		"192.168.1.192/28",
	} {
		if !hasExactText(tt, want) {
			t.Errorf("texts %q do not contain exact text %q", tt.Texts(), want)
		}
	}
	if !tt.HasText("208 of 256 addresses allocated") {
		t.Errorf("texts %q do not show the allocation summary", tt.Texts())
	}
}

// The allocation card sits below the fold at the minimum window size, so the
// page must scroll and still build the result.
func TestVLSMResultAtMinimumSize(t *testing.T) {
	a := New()
	tt := myui.NewTester(a.View, 820, 560)

	if err := tt.Click("VLSM"); err != nil {
		t.Fatal(err)
	}
	fill := func(label, value string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatalf("click %q: %v", label, err)
		}
		tt.Type(value)
	}
	fill("Parent network", "192.168.1.0/24")
	fill("Department 1", "Programming")
	fill("Hosts 1", "100")
	if err := tt.Click("Allocate"); err != nil {
		t.Fatal(err)
	}
	if a.vlsm.plan == nil {
		t.Fatalf("plan = nil, error = %q", a.vlsm.err)
	}
	if !tt.HasText("128 of 256 addresses allocated") {
		t.Errorf("texts %q do not show the allocation summary", tt.Texts())
	}
	if !tt.HasText("192.168.1.0/25") {
		t.Errorf("texts %q do not show the allocated subnet", tt.Texts())
	}
}

func TestVLSMCSV(t *testing.T) {
	s := vlsmState{
		parent: "192.168.1.0/24",
		rows: []*vlsmRow{
			{id: 1, name: "Programming", hosts: "100"},
			{id: 2, name: "Management", hosts: "50"},
		},
	}
	s.allocate()
	if s.plan == nil {
		t.Fatalf("plan = nil, error = %q", s.err)
	}

	got := string(vlsmCSV(s.plan))
	if !strings.HasPrefix(got, "\xef\xbb\xbf") {
		t.Error("CSV is missing the UTF-8 BOM Excel expects")
	}
	want := []string{
		"Department,CIDR,Mask,Usable range,Gateway,Broadcast,Usable hosts",
		"Programming,192.168.1.0/25,255.255.255.128,192.168.1.1 - 192.168.1.126,192.168.1.1,192.168.1.127,126",
		"Management,192.168.1.128/26,255.255.255.192,192.168.1.129 - 192.168.1.190,192.168.1.129,192.168.1.191,62",
	}
	for _, row := range want {
		if !strings.Contains(got, row) {
			t.Errorf("CSV = %q, want it to contain %q", got, row)
		}
	}
}

func TestVLSMAddRemoveRows(t *testing.T) {
	a := New()
	tt := myui.NewTester(a.View, 1100, 720)

	if err := tt.Click("VLSM"); err != nil {
		t.Fatal(err)
	}
	if len(a.vlsm.rows) != 1 || a.vlsm.rows[0].id != 1 {
		t.Fatalf("initial rows = %d, want one row with id 1", len(a.vlsm.rows))
	}

	if err := tt.Click("Add department"); err != nil {
		t.Fatal(err)
	}
	if len(a.vlsm.rows) != 2 || a.vlsm.rows[1].id != 2 {
		t.Fatalf("rows after add = %d, want two rows ending in id 2", len(a.vlsm.rows))
	}

	if err := tt.Click("Remove row 1"); err != nil {
		t.Fatal(err)
	}
	if len(a.vlsm.rows) != 1 || a.vlsm.rows[0].id != 2 {
		t.Fatalf("rows after remove = %d, want one row with id 2", len(a.vlsm.rows))
	}

	if err := tt.Click("Add department"); err != nil {
		t.Fatal(err)
	}
	if len(a.vlsm.rows) != 2 || a.vlsm.rows[1].id != 3 {
		t.Fatalf("rows after second add = %d, want two rows ending in id 3", len(a.vlsm.rows))
	}
}

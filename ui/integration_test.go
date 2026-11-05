package ui

import (
	"strings"
	"testing"

	myui "github.com/egoist/mygo/ui"
)

// TestEndToEndWorkflow walks every tab in one session: calculate and save a
// network, plan it with VLSM, inspect and split IPv6, label an IP and scan a
// live port.
func TestEndToEndWorkflow(t *testing.T) {
	store, _ := openIPAM(t)
	a := NewWithStore(store)
	tt := myui.NewTester(a.View, 1100, 720)

	// IPv4: calculate and save.
	if err := tt.Click("CIDR"); err != nil {
		t.Fatal(err)
	}
	tt.Type("192.168.50.9/24")
	if err := tt.Click("Calculate"); err != nil {
		t.Fatal(err)
	}
	if a.ipv4.result == nil {
		t.Fatalf("IPv4 result = nil, texts %q", tt.Texts())
	}
	if err := tt.Click("Network name"); err != nil {
		t.Fatal(err)
	}
	tt.Type("HQ")
	if err := tt.Click("Save to IPAM"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(a.ipv4.saveMsg, "Saved 192.168.50.0/24") {
		t.Fatalf("save message = %q", a.ipv4.saveMsg)
	}

	// VLSM: plan departments in the saved network.
	if err := tt.Click("VLSM"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Parent network"); err != nil {
		t.Fatal(err)
	}
	tt.Type("192.168.50.0/24")
	if err := tt.Click("Department 1"); err != nil {
		t.Fatal(err)
	}
	tt.Type("Engineering")
	if err := tt.Click("Hosts 1"); err != nil {
		t.Fatal(err)
	}
	tt.Type("100")
	if err := tt.Click("Allocate"); err != nil {
		t.Fatal(err)
	}
	if a.vlsm.plan == nil || len(a.vlsm.plan.Allocations) != 1 {
		t.Fatalf("VLSM plan = %+v, error = %q", a.vlsm.plan, a.vlsm.err)
	}
	if got := a.vlsm.plan.Allocations[0].Subnet.Network.String(); got != "192.168.50.0" {
		t.Errorf("VLSM network = %s, want 192.168.50.0", got)
	}
	if !tt.HasText("192.168.50.0/25") {
		t.Errorf("texts %q do not show the VLSM result", tt.Texts())
	}

	// IPv6: inspect and split.
	if err := tt.Click("IPv6"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("IPv6 address"); err != nil {
		t.Fatal(err)
	}
	tt.Type("2001:db8::1")
	if err := tt.Click("Inspect"); err != nil {
		t.Fatal(err)
	}
	if a.ipv6.info == nil {
		t.Fatalf("IPv6 info = nil, texts %q", tt.Texts())
	}
	if err := tt.Click("IPv6 prefix"); err != nil {
		t.Fatal(err)
	}
	tt.Type("2001:db8::/48")
	if err := tt.Click("New prefix length"); err != nil {
		t.Fatal(err)
	}
	tt.Type("64")
	if err := tt.Click("Split"); err != nil {
		t.Fatal(err)
	}
	if a.ipv6.split == nil || a.ipv6.page.Total != 65536 {
		t.Fatalf("IPv6 split = %+v, error = %q", a.ipv6.split, a.ipv6.splitErr)
	}

	// IPAM: the saved network is there; label an address.
	if err := tt.Click("IPAM"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("HQ") || !tt.HasText("192.168.50.0/24") {
		t.Fatalf("texts %q do not show the saved network", tt.Texts())
	}
	if err := tt.Click("192.168.50.10"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Label"); err != nil {
		t.Fatal(err)
	}
	tt.Type("Web server")
	if err := tt.Click("Owner"); err != nil {
		t.Fatal(err)
	}
	tt.Type("Operations")
	if err := tt.Click("Save assignment"); err != nil {
		t.Fatal(err)
	}
	if a.ipam.editErr != "" {
		t.Fatalf("IPAM edit error = %q", a.ipam.editErr)
	}
	if !tt.HasText("Web server") {
		t.Errorf("texts %q do not show the assignment", tt.Texts())
	}

	// Scan: a live local port shows as alive.
	port := openPort(t)
	a.scan.ports = []int{port}
	if err := tt.Click("Scan"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Scan target"); err != nil {
		t.Fatal(err)
	}
	tt.Type("127.0.0.1/32")
	if err := tt.Click("Start scan"); err != nil {
		t.Fatal(err)
	}
	snap := waitScan(t, &a.scan)
	if snap.alive != 1 || len(snap.hosts) != 1 {
		t.Fatalf("scan snapshot = %+v, want one alive host", snap)
	}
	tt.Frame()
	if !tt.HasText("1 alive, 0 documented but silent, 0 no answer") {
		t.Errorf("texts %q do not show the scan summary", tt.Texts())
	}
}

func TestEmptyAndErrorStates(t *testing.T) {
	a := New()
	tt := myui.NewTester(a.View, 1100, 720)

	// IPv4 with no input.
	if err := tt.Click("Calculate"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Enter a CIDR block") {
		t.Errorf("texts %q do not show the IPv4 empty error", tt.Texts())
	}

	// VLSM with no parent.
	if err := tt.Click("VLSM"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Allocate"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Enter a parent network") {
		t.Errorf("texts %q do not show the VLSM empty error", tt.Texts())
	}

	// IPv6 without input.
	if err := tt.Click("IPv6"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Inspect"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Enter an IPv6 address") {
		t.Errorf("texts %q do not show the IPv6 address error", tt.Texts())
	}
	if err := tt.Click("Split"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Enter a prefix") {
		t.Errorf("texts %q do not show the IPv6 split error", tt.Texts())
	}

	// IPAM without a database.
	if err := tt.Click("IPAM"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("The IPAM database is unavailable.") {
		t.Errorf("texts %q do not show the missing database", tt.Texts())
	}

	// Scan without a target.
	if err := tt.Click("Scan"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Start scan"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Enter a target network") {
		t.Errorf("texts %q do not show the scan empty error", tt.Texts())
	}
}

func TestIPAMEmptyState(t *testing.T) {
	store, _ := openIPAM(t)
	a := NewWithStore(store)
	tt := myui.NewTester(a.View, 1100, 720)

	if err := tt.Click("IPAM"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("No networks saved yet") {
		t.Errorf("texts %q do not show the empty IPAM state", tt.Texts())
	}
}

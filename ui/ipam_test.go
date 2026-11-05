package ui

import (
	"net/netip"
	"path/filepath"
	"strings"
	"testing"

	myui "github.com/egoist/mygo/ui"

	"ipatlas/internal/ipam"
)

func openIPAM(t *testing.T) (*ipam.Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ipam.db")
	store, err := ipam.Open(path)
	if err != nil {
		t.Fatalf("ipam.Open: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store, path
}

func TestIPAMSaveFromCalculator(t *testing.T) {
	store, _ := openIPAM(t)
	a := NewWithStore(store)
	tt := myui.NewTester(a.View, 1100, 720)

	if err := tt.Click("CIDR"); err != nil {
		t.Fatal(err)
	}
	tt.Type("192.168.1.5/24")
	if err := tt.Click("Calculate"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Network name"); err != nil {
		t.Fatal(err)
	}
	tt.Type("Office")
	if err := tt.Click("Save to IPAM"); err != nil {
		t.Fatal(err)
	}

	if a.ipv4.saveErr {
		t.Fatalf("save error = %q", a.ipv4.saveMsg)
	}
	if !strings.Contains(a.ipv4.saveMsg, "Saved") {
		t.Errorf("save message = %q, want a confirmation", a.ipv4.saveMsg)
	}
	networks, err := store.Networks()
	if err != nil {
		t.Fatalf("Networks: %v", err)
	}
	if len(networks) != 1 || networks[0].Name != "Office" || networks[0].CIDR != "192.168.1.0/24" {
		t.Fatalf("networks = %+v, want Office 192.168.1.0/24", networks)
	}

	if err := tt.Click("IPAM"); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Office", "192.168.1.0/24", "0 used, 254 free of 254"} {
		if !tt.HasText(want) {
			t.Errorf("texts %q do not show %q", tt.Texts(), want)
		}
	}
}

func TestIPAMSaveWithoutStore(t *testing.T) {
	a := New()
	tt := myui.NewTester(a.View, 1100, 720)

	if err := tt.Click("CIDR"); err != nil {
		t.Fatal(err)
	}
	tt.Type("10.0.0.0/24")
	if err := tt.Click("Calculate"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Save to IPAM"); err != nil {
		t.Fatal(err)
	}
	if !a.ipv4.saveErr || !strings.Contains(a.ipv4.saveMsg, "unavailable") {
		t.Errorf("save message = %q, error = %v; want an unavailable error", a.ipv4.saveMsg, a.ipv4.saveErr)
	}

	if err := tt.Click("IPAM"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("The IPAM database is unavailable.") {
		t.Errorf("texts %q do not show the missing database", tt.Texts())
	}
}

func TestIPAMEditAndReload(t *testing.T) {
	store, path := openIPAM(t)
	if _, err := store.SaveNetwork("10.0.0.0/24", "Lab"); err != nil {
		t.Fatalf("SaveNetwork: %v", err)
	}

	a := NewWithStore(store)
	tt := myui.NewTester(a.View, 1100, 720)
	if err := tt.Click("IPAM"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Lab") {
		t.Fatalf("texts %q do not show the saved network", tt.Texts())
	}

	if err := tt.Click("10.0.0.10"); err != nil {
		t.Fatal(err)
	}
	if !a.ipam.editAddr.IsValid() || a.ipam.editAddr.String() != "10.0.0.10" {
		t.Fatalf("editing = %v, want 10.0.0.10", a.ipam.editAddr)
	}
	if err := tt.Click("Label"); err != nil {
		t.Fatal(err)
	}
	tt.Type("Printer")
	if err := tt.Click("Owner"); err != nil {
		t.Fatal(err)
	}
	tt.Type("Office")
	if err := tt.Click("Save assignment"); err != nil {
		t.Fatal(err)
	}
	if a.ipam.editErr != "" {
		t.Fatalf("edit error = %q", a.ipam.editErr)
	}

	assignment, ok, err := store.Assignment(a.ipam.network, mustAddr("10.0.0.10"))
	if err != nil {
		t.Fatalf("Assignment: %v", err)
	}
	if !ok || assignment.Label != "Printer" || assignment.Owner != "Office" {
		t.Fatalf("assignment = %+v, ok = %v", assignment, ok)
	}
	if !tt.HasText("1 used, 253 free of 254") {
		t.Errorf("texts %q do not show the used count", tt.Texts())
	}

	// Reopen the database and build a fresh app: the assignment must survive.
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	reopened, err := ipam.Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()

	b := NewWithStore(reopened)
	bt := myui.NewTester(b.View, 1100, 720)
	if err := bt.Click("IPAM"); err != nil {
		t.Fatal(err)
	}
	if !bt.HasText("Printer") {
		t.Errorf("texts %q do not show the persisted assignment", bt.Texts())
	}
	persisted, ok, err := reopened.Assignment(b.ipam.network, mustAddr("10.0.0.10"))
	if err != nil {
		t.Fatalf("Assignment after reopen: %v", err)
	}
	if !ok || persisted.Label != "Printer" || persisted.Owner != "Office" {
		t.Fatalf("persisted assignment = %+v, ok = %v", persisted, ok)
	}

	if err := bt.Click("10.0.0.10"); err != nil {
		t.Fatal(err)
	}
	if err := bt.Click("Remove assignment"); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := reopened.Assignment(b.ipam.network, mustAddr("10.0.0.10")); err != nil || ok {
		t.Errorf("assignment after remove: ok = %v, err = %v", ok, err)
	}
}

func TestIPAMSelectAndDelete(t *testing.T) {
	store, _ := openIPAM(t)
	if _, err := store.SaveNetwork("10.0.0.0/24", "First"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveNetwork("192.168.0.0/24", "Second"); err != nil {
		t.Fatal(err)
	}

	a := NewWithStore(store)
	tt := myui.NewTester(a.View, 1100, 720)
	if err := tt.Click("IPAM"); err != nil {
		t.Fatal(err)
	}
	first, err := store.Network("10.0.0.0/24")
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Network("192.168.0.0/24")
	if err != nil {
		t.Fatal(err)
	}
	if a.ipam.network != second.ID {
		t.Errorf("initial selection = %d, want newest %d", a.ipam.network, second.ID)
	}

	if err := tt.Click("First"); err != nil {
		t.Fatal(err)
	}
	if a.ipam.network != first.ID {
		t.Errorf("selection after click = %d, want %d", a.ipam.network, first.ID)
	}
	if !tt.HasText("10.0.0.1") {
		t.Errorf("texts %q do not show the first network's grid", tt.Texts())
	}

	if err := tt.Click("Delete network"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Network(first.ID); err == nil {
		t.Error("deleted network is still stored")
	}
	if a.ipam.network != second.ID {
		t.Errorf("selection after delete = %d, want %d", a.ipam.network, second.ID)
	}
}

func TestIPAMPaging(t *testing.T) {
	store, _ := openIPAM(t)
	if _, err := store.SaveNetwork("10.0.0.0/23", "Wide"); err != nil {
		t.Fatal(err)
	}

	a := NewWithStore(store)
	tt := myui.NewTester(a.View, 1100, 720)
	if err := tt.Click("IPAM"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Page 1 of 2 - addresses 1-256 of 510") {
		t.Fatalf("texts %q do not show page 1", tt.Texts())
	}

	if err := tt.Click("Next"); err != nil {
		t.Fatal(err)
	}
	if a.ipam.offset != ipamPageSize {
		t.Errorf("offset after Next = %d, want %d", a.ipam.offset, ipamPageSize)
	}
	if !tt.HasText("Page 2 of 2 - addresses 257-510 of 510") {
		t.Errorf("texts %q do not show page 2", tt.Texts())
	}

	if err := tt.Click("Previous"); err != nil {
		t.Fatal(err)
	}
	if a.ipam.offset != 0 {
		t.Errorf("offset after Previous = %d, want 0", a.ipam.offset)
	}
}

func mustAddr(s string) netip.Addr {
	return netip.MustParseAddr(s)
}

func TestIPAMStateErrors(t *testing.T) {
	s := ipamState{store: nil}
	s.loadPage()
	if s.storeErr == "" {
		t.Error("loadPage without a store left no error")
	}
	if _, err := s.saveNetwork("10.0.0.0/24", ""); err == nil {
		t.Error("saveNetwork without a store succeeded")
	}
	s.assign()
	if !strings.Contains(s.editErr, "unavailable") {
		t.Errorf("assign error = %q, want an unavailable message", s.editErr)
	}
}

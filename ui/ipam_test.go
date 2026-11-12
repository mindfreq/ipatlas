package ui

import (
	"bytes"
	"net/netip"
	"path/filepath"
	"strings"
	"testing"
	"time"

	myui "github.com/egoist/mygo/ui"

	"ipatlas/internal/ipam"
)

// The IPAM page does its database work on a goroutine and reports back on
// the UI thread. Tests run the whole round-trip synchronously so the state
// is deterministic right after a click. The live search's debounce runs
// the same way, so typing brings the matches up within the frame.
func init() {
	runOffUI = func(job, fn func()) {
		job()
		fn()
	}
	runSearchDebounce = func(_ time.Duration, fn func()) *time.Timer {
		fn()
		return nil
	}
}

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

func TestIPAMRenameNetwork(t *testing.T) {
	store, _ := openIPAM(t)
	if _, err := store.SaveNetwork("192.168.1.0/24", "Office"); err != nil {
		t.Fatalf("SaveNetwork: %v", err)
	}
	a := NewWithStore(store)
	tt := myui.NewTester(a.View, 1100, 720)

	if err := tt.Click("IPAM"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Office") {
		t.Fatalf("texts %q do not show the saved network", tt.Texts())
	}

	if err := tt.Click("Rename network"); err != nil {
		t.Fatal(err)
	}
	if !a.ipam.renaming {
		t.Fatal("rename editor did not open")
	}
	if a.ipam.renameName != "Office" {
		t.Fatalf("rename name = %q, want the prefilled Office", a.ipam.renameName)
	}

	// The field is prefilled; start the edit from an empty value.
	a.ipam.renameName = ""
	if err := tt.Click("Network name"); err != nil {
		t.Fatal(err)
	}
	tt.Type("Branch")
	if err := tt.Click("Save name"); err != nil {
		t.Fatal(err)
	}

	if a.ipam.renaming {
		t.Error("rename editor still open after saving")
	}
	if a.ipam.renameErr != "" {
		t.Fatalf("rename error = %q", a.ipam.renameErr)
	}
	networks, err := store.Networks()
	if err != nil {
		t.Fatalf("Networks: %v", err)
	}
	if len(networks) != 1 || networks[0].Name != "Branch" {
		t.Fatalf("networks = %+v, want one named Branch", networks)
	}
	if !hasExactText(tt, "Branch") {
		t.Errorf("texts %q do not show the new name", tt.Texts())
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
	// A /24 fits one page, so the page arrows stay hidden for it.
	if _, ok := tt.Find("Prev"); ok {
		t.Error("the page arrows show for a one-page network")
	}

	// Deleting asks for confirmation first and does not remove anything yet.
	if err := tt.Click("Delete network"); err != nil {
		t.Fatal(err)
	}
	if !a.ipam.confirmDelete {
		t.Fatal("delete confirmation dialog did not open")
	}
	if !tt.HasText("This cannot be undone") {
		t.Errorf("texts %q do not show the confirmation warning", tt.Texts())
	}
	if _, err := store.Network(first.ID); err != nil {
		t.Error("network was deleted before confirmation")
	}

	// Cancel keeps the network.
	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	if a.ipam.confirmDelete {
		t.Error("dialog still open after cancel")
	}
	if _, err := store.Network(first.ID); err != nil {
		t.Error("cancel deleted the network")
	}

	// Confirming the dialog deletes it and selects the remaining network.
	if err := tt.Click("Delete network"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Delete"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Network(first.ID); err == nil {
		t.Error("deleted network is still stored")
	}
	if a.ipam.network != second.ID {
		t.Errorf("selection after delete = %d, want %d", a.ipam.network, second.ID)
	}
	if a.ipam.confirmDelete {
		t.Error("dialog still open after deleting")
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
	if _, ok := tt.Find("Prev"); !ok {
		t.Fatalf("texts %q do not show the page arrows", tt.Texts())
	}

	if err := tt.Click("Next"); err != nil {
		t.Fatal(err)
	}
	if a.ipam.offset != ipamPageSize {
		t.Errorf("offset after Next = %d, want %d", a.ipam.offset, ipamPageSize)
	}

	if err := tt.Click("Prev"); err != nil {
		t.Fatal(err)
	}
	if a.ipam.offset != 0 {
		t.Errorf("offset after Previous = %d, want 0", a.ipam.offset)
	}

	// Typing a page number in the pager's box jumps straight to it.
	if err := tt.Click("Page"); err != nil {
		t.Fatal(err)
	}
	tt.Type("2")
	tt.Key(0, myui.KeyEnter)
	tt.Frame()
	if a.ipam.offset != ipamPageSize {
		t.Errorf("offset after a page jump = %d, want %d", a.ipam.offset, ipamPageSize)
	}
	tt.Type("9")
	tt.Key(0, myui.KeyEnter)
	tt.Frame()
	if a.ipam.offset != ipamPageSize {
		t.Errorf("offset after a past-the-end page jump = %d, want the last page %d", a.ipam.offset, ipamPageSize)
	}
}

func mustAddr(s string) netip.Addr {
	return netip.MustParseAddr(s)
}

func TestIPAMLiveSearch(t *testing.T) {
	store, _ := openIPAM(t)
	n, err := store.SaveNetwork("10.0.0.0/24", "Lab")
	if err != nil {
		t.Fatalf("SaveNetwork: %v", err)
	}
	if err := store.Assign(n.ID, mustAddr("10.0.0.10"), "Printer", "Office"); err != nil {
		t.Fatalf("Assign: %v", err)
	}

	a := NewWithStore(store)
	tt := myui.NewTester(a.View, 1100, 720)
	if err := tt.Click("IPAM"); err != nil {
		t.Fatal(err)
	}

	// Typing in the search box runs the search by itself: no search
	// button to press.
	if err := tt.Click("Search"); err != nil {
		t.Fatal(err)
	}
	tt.Type("printer")
	tt.Frame()
	if !a.ipam.searching || a.ipam.searchQuery != "printer" {
		t.Fatalf("after typing: searching = %v, query = %q; want a live search for printer", a.ipam.searching, a.ipam.searchQuery)
	}
	if !tt.HasText("10.0.0.10") {
		t.Errorf("texts %q do not show the match", tt.Texts())
	}

	// The field's own clear button empties it and returns to the range.
	if err := tt.Click("Clear"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.ipam.searching || a.ipam.searchQuery != "" {
		t.Errorf("after clearing: searching = %v, query = %q; want the address range back", a.ipam.searching, a.ipam.searchQuery)
	}
	if !tt.HasText("10.0.0.1") {
		t.Errorf("texts %q do not show the address range again", tt.Texts())
	}

	// Enter with a full address still jumps to its page instead of
	// searching the assignments for it.
	if err := tt.Click("Search"); err != nil {
		t.Fatal(err)
	}
	tt.Type("10.0.0.10")
	tt.Key(0, myui.KeyEnter)
	tt.Frame()
	if a.ipam.searching {
		t.Error("Enter with an address left search mode")
	}
	if !a.ipam.editAddr.IsValid() || a.ipam.editAddr.String() != "10.0.0.10" {
		t.Errorf("editing after Enter = %v, want 10.0.0.10", a.ipam.editAddr)
	}
}

func TestIPAMCSV(t *testing.T) {
	store, _ := openIPAM(t)
	n, err := store.SaveNetwork("10.0.0.0/24", "Lab")
	if err != nil {
		t.Fatalf("SaveNetwork: %v", err)
	}
	if err := store.Assign(n.ID, mustAddr("10.0.0.10"), "Printer", "Office"); err != nil {
		t.Fatalf("Assign: %v", err)
	}
	if err := store.Assign(n.ID, mustAddr("10.0.0.1"), "Gateway", ""); err != nil {
		t.Fatalf("Assign: %v", err)
	}

	var b bytes.Buffer
	if err := ipamCSV(store, n, &b, true); err != nil {
		t.Fatalf("ipamCSV(all hosts): %v", err)
	}
	csv := b.String()
	if !strings.HasPrefix(csv, "\xef\xbb\xbf") {
		t.Error("CSV is missing the UTF-8 BOM Excel expects")
	}
	want := []string{
		"Address,State,Label,Owner,Updated",
		"10.0.0.1,used,Gateway,,",
		"10.0.0.2,,,,",
		"10.0.0.10,used,Printer,Office,",
		"10.0.0.254,,,,",
	}
	for _, row := range want {
		if !strings.Contains(csv, row) {
			t.Errorf("CSV = %q, want it to contain %q", csv, row)
		}
	}

	// Every usable address of the range is in the sheet, free ones
	// included, and nothing outside it.
	if got := strings.Count(csv, "\n") - 1; got != 254 { // minus the header
		t.Errorf("CSV has %d address rows, want all 254 usable hosts", got)
	}

	// The assignments sheet keeps only the assigned ones, without the
	// State column.
	b.Reset()
	if err := ipamCSV(store, n, &b, false); err != nil {
		t.Fatalf("ipamCSV(assignments): %v", err)
	}
	csv = b.String()
	want = []string{
		"Address,Label,Owner,Updated",
		"10.0.0.1,Gateway,,",
		"10.0.0.10,Printer,Office,",
	}
	for _, row := range want {
		if !strings.Contains(csv, row) {
			t.Errorf("CSV = %q, want it to contain %q", csv, row)
		}
	}
	if got := strings.Count(csv, "\n") - 1; got != 2 { // minus the header
		t.Errorf("CSV has %d rows, want only the 2 assignments", got)
	}
}

func TestIPAMExportMenu(t *testing.T) {
	store, _ := openIPAM(t)
	if _, err := store.SaveNetwork("10.0.0.0/24", "Lab"); err != nil {
		t.Fatalf("SaveNetwork: %v", err)
	}

	a := NewWithStore(store)
	tt := myui.NewTester(a.View, 1100, 720)
	if err := tt.Click("IPAM"); err != nil {
		t.Fatal(err)
	}

	// The export button opens a menu in the window with the two sheets.
	if err := tt.Click("Export CSV"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	for _, item := range []string{"Export all hosts", "Export assignments"} {
		if _, ok := tt.Find(item); !ok {
			t.Errorf("texts %q do not show the menu item %q", tt.Texts(), item)
		}
	}
}

func TestIPAMStateErrors(t *testing.T) {
	s := ipamState{store: nil}
	s.loadPage()
	if s.storeErr == "" {
		t.Error("loadPage without a store left no error")
	}
	saved := false
	s.saveNetwork("10.0.0.0/24", "", func(_ ipam.Network, err error) {
		saved = err == nil
	})
	if saved {
		t.Error("saveNetwork without a store succeeded")
	}
	s.assign()
	if !strings.Contains(s.editErr, "unavailable") {
		t.Errorf("assign error = %q, want an unavailable message", s.editErr)
	}
}

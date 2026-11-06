package ui

import (
	"testing"

	myui "github.com/egoist/mygo/ui"
)

// TestCollapsedRailTab clicks through the icon-only rail: collapsing must
// not disable the tab buttons, and every icon has to select its page.
func TestCollapsedRailTab(t *testing.T) {
	a := New()
	tt := myui.NewTester(a.View, 1100, 720)

	if err := tt.Click("Collapse sidebar"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()

	for _, tc := range []struct {
		label string
		want  string // first words of the page blurb
	}{
		{"VLSM", "Allocate non-overlapping subnets"},
		{"IPAM", "Save calculated networks locally"},
		{"Scan", "Probe a network with a bounded worker pool"},
		{"About", "about"},
		{"IPv4", "Enter a CIDR block"},
		{"IPv6", "Compress, expand and classify"},
	} {
		if err := tt.Click(tc.label); err != nil {
			t.Fatalf("%s: %v; texts %q", tc.label, err, tt.Texts())
		}
		tt.Frame()
		if a.nav != tc.label {
			t.Errorf("click on %s left nav = %q", tc.label, a.nav)
		}
		if tc.want == "about" {
			if sections[a.tab].title != "About IPAtlas" {
				t.Errorf("About click shows %q", sections[a.tab].title)
			}
			continue
		}
		if !tt.HasText(tc.want) {
			t.Errorf("click on %s shows texts %q, want %q", tc.label, tt.Texts(), tc.want)
		}
	}

	// The theme switch rides along in the collapsed rail: still at the
	// foot, still clickable.
	if err := tt.Click("Switch to dark mode"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.darkMode(false) {
		t.Error("theme toggle in the collapsed rail does not switch to dark")
	}
}

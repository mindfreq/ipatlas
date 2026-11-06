package ui

import (
	"testing"

	myui "github.com/egoist/mygo/ui"
)

// TestThemeToggle checks the rail's appearance switch: it starts following
// the system (light in the tester) and flips dark and light on every click,
// in both the open and the collapsed rail.
func TestThemeToggle(t *testing.T) {
	a := New()
	tt := myui.NewTester(a.View, 1100, 720)

	if a.darkMode(false) {
		t.Error("theme follows the system before the first toggle")
	}
	if err := tt.Click("Switch to dark mode"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.darkMode(false) {
		t.Error("theme is still light after choosing dark")
	}
	if !tt.HasText("Switch to light mode") {
		t.Errorf("texts %q do not offer light mode after switching", tt.Texts())
	}

	if err := tt.Click("Switch to light mode"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.darkMode(true) {
		t.Error("theme ignores the explicit light choice")
	}

	// The collapsed rail carries the switch too.
	if err := tt.Click("Collapse sidebar"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Switch to dark mode"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.darkMode(false) {
		t.Error("dark mode does not survive a rail collapse")
	}
}

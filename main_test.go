package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// The view runs without a window in tests, which click and type as a user
// would.
func TestShellTabs(t *testing.T) {
	a := &app{}
	tt := ui.NewTester(a.view, 1100, 720)

	if a.tab != 0 {
		t.Fatalf("initial tab = %d, want 0", a.tab)
	}
	if !tt.HasText(sections[0].title) {
		t.Fatalf("initial texts %q, want %q", tt.Texts(), sections[0].title)
	}

	for i, label := range tabLabels {
		if err := tt.Click(label); err != nil {
			t.Fatalf("click %q: %v", label, err)
		}
		if a.tab != i {
			t.Errorf("tab after click %q = %d, want %d", label, a.tab, i)
		}
		if !tt.HasText(sections[i].title) {
			t.Errorf("texts after click %q = %q, want %q", label, tt.Texts(), sections[i].title)
		}
	}
}

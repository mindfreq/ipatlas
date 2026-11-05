package ui

import (
	"strings"
	"testing"

	myui "github.com/egoist/mygo/ui"
)

// The view runs without a window in tests, which click and type as a user
// would.
func TestShellTabs(t *testing.T) {
	a := New()
	tt := myui.NewTester(a.View, 1100, 720)

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

// The navigation rail stays clickable at the window's minimum size.
func TestShellMinimumSize(t *testing.T) {
	a := New()
	tt := myui.NewTester(a.View, 820, 560)

	for i, label := range tabLabels {
		if err := tt.Click(label); err != nil {
			t.Fatalf("click %q at 820x560: %v", label, err)
		}
		if a.tab != i {
			t.Errorf("tab after click %q = %d, want %d", label, a.tab, i)
		}
		if !tt.HasText(sections[i].title) {
			t.Errorf("texts after click %q = %q, want %q", label, tt.Texts(), sections[i].title)
		}
	}
}

// The shell and every section build in the dark theme.
func TestShellDarkTheme(t *testing.T) {
	a := New()
	tt := myui.NewTester(a.View, 1100, 720)
	tt.SetDark(true)

	for i, label := range tabLabels {
		if err := tt.Click(label); err != nil {
			t.Fatalf("click %q in dark theme: %v", label, err)
		}
		if a.tab != i {
			t.Errorf("tab after click %q = %d, want %d", label, a.tab, i)
		}
	}
}

func TestIPv4Calculator(t *testing.T) {
	a := New()
	tt := myui.NewTester(a.View, 1100, 720)

	if err := tt.Click("CIDR"); err != nil {
		t.Fatal(err)
	}
	tt.Type("192.168.1.5/24")
	if err := tt.Click("Calculate"); err != nil {
		t.Fatal(err)
	}

	if a.ipv4.result == nil {
		t.Fatalf("no result after calculating, texts %q", tt.Texts())
	}
	if got := a.ipv4.result.Network.String(); got != "192.168.1.0" {
		t.Errorf("network = %s, want 192.168.1.0", got)
	}

	for _, want := range []string{
		"192.168.1.0/24",
		"192.168.1.5",
		"192.168.1.255",
		"255.255.255.0",
		"0.0.0.255",
		"192.168.1.1 - 192.168.1.254",
		"254",
		"192.168.1.1",
	} {
		if !hasExactText(tt, want) {
			t.Errorf("texts %q do not contain exact text %q", tt.Texts(), want)
		}
	}
}

func TestIPv4CalculatorInvalid(t *testing.T) {
	a := New()
	tt := myui.NewTester(a.View, 1100, 720)

	if err := tt.Click("CIDR"); err != nil {
		t.Fatal(err)
	}
	tt.Type("not-a-cidr")
	if err := tt.Click("Calculate"); err != nil {
		t.Fatal(err)
	}

	if a.ipv4.result != nil {
		t.Errorf("result = %+v after invalid input, want nil", a.ipv4.result)
	}
	if !strings.Contains(a.ipv4.err, "not valid") {
		t.Errorf("error = %q, want a validation message", a.ipv4.err)
	}
	if !tt.HasText("not valid") {
		t.Errorf("texts %q do not show the validation message", tt.Texts())
	}
}

func TestIPv4Calculate(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantNetwork string
		wantPrefix  int
		wantErr     string
	}{
		{
			name:        "trims and normalizes host bits",
			input:       " 192.168.1.5/24 ",
			wantNetwork: "192.168.1.0",
			wantPrefix:  24,
		},
		{
			name:    "empty",
			input:   "",
			wantErr: "Enter a CIDR",
		},
		{
			name:    "invalid",
			input:   "nope",
			wantErr: "not valid",
		},
		{
			name:    "ipv6",
			input:   "2001:db8::/32",
			wantErr: "not an IPv4",
		},
		{
			name:    "prefix too large",
			input:   "192.168.1.1/33",
			wantErr: "not valid",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := ipv4State{input: tc.input}
			s.calculate()

			if tc.wantErr != "" {
				if s.result != nil {
					t.Errorf("result = %+v, want nil", s.result)
				}
				if !strings.Contains(s.err, tc.wantErr) {
					t.Errorf("error = %q, want it to contain %q", s.err, tc.wantErr)
				}
				return
			}

			if s.err != "" {
				t.Fatalf("error = %q, want none", s.err)
			}
			if s.result == nil {
				t.Fatal("result = nil, want a subnet")
			}
			if got := s.result.Network.String(); got != tc.wantNetwork {
				t.Errorf("network = %s, want %s", got, tc.wantNetwork)
			}
			if s.result.Prefix != tc.wantPrefix {
				t.Errorf("prefix = %d, want %d", s.result.Prefix, tc.wantPrefix)
			}
		})
	}
}

func hasExactText(tt *myui.Tester, want string) bool {
	for _, text := range tt.Texts() {
		if text == want {
			return true
		}
	}
	return false
}

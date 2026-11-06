package ipam

import (
	"errors"
	"net/netip"
	"testing"
)

func openSearchTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// searchFixture saves 10.0.0.0/29 (usable .1 to .6) with three assignments.
func searchFixture(t *testing.T) (*Store, Network) {
	t.Helper()
	s := openSearchTestStore(t)
	n, err := s.SaveNetwork("10.0.0.0/29", "Lab")
	if err != nil {
		t.Fatalf("SaveNetwork: %v", err)
	}
	for _, a := range []struct{ addr, label, owner string }{
		{"10.0.0.1", "gateway", "net-team"},
		{"10.0.0.2", "100% printer", ""},
		{"10.0.0.3", "Printer_2", "Ana"},
	} {
		if err := s.Assign(n.ID, netip.MustParseAddr(a.addr), a.label, a.owner); err != nil {
			t.Fatalf("Assign %s: %v", a.addr, err)
		}
	}
	return s, n
}

func TestSearchAssignmentsMatches(t *testing.T) {
	s, n := searchFixture(t)
	tests := []struct {
		name  string
		query string
		want  []string
	}{
		{"label ignoring case", "printer", []string{"10.0.0.2", "10.0.0.3"}},
		{"owner", "ana", []string{"10.0.0.3"}},
		{"owner with dash", "net-team", []string{"10.0.0.1"}},
		{"address part", ".1", []string{"10.0.0.1"}},
		{"percent is literal", "%", []string{"10.0.0.2"}},
		{"underscore is literal", "_", []string{"10.0.0.3"}},
		{"surrounding spaces ignored", "  gateway  ", []string{"10.0.0.1"}},
		{"no match", "nothing", nil},
		{"empty query", "   ", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, total, err := s.SearchAssignments(n.ID, tt.query, 0, 10)
			if err != nil {
				t.Fatalf("SearchAssignments: %v", err)
			}
			if int(total) != len(tt.want) || len(got) != len(tt.want) {
				t.Fatalf("got %d entries, total %d; want %d", len(got), total, len(tt.want))
			}
			for i, e := range got {
				if e.Addr.String() != tt.want[i] || e.State != Used {
					t.Errorf("entry %d = %v (%v), want %s used", i, e.Addr, e.State, tt.want[i])
				}
			}
		})
	}
}

func TestSearchAssignmentsPaging(t *testing.T) {
	s, n := searchFixture(t)

	got, total, err := s.SearchAssignments(n.ID, "printer", 1, 1)
	if err != nil {
		t.Fatalf("SearchAssignments: %v", err)
	}
	if total != 2 || len(got) != 1 || got[0].Addr.String() != "10.0.0.3" {
		t.Errorf("second match = %v, total %d; want 10.0.0.3, 2", got, total)
	}

	got, total, err = s.SearchAssignments(n.ID, "printer", 5, 1)
	if err != nil || total != 2 || len(got) != 0 {
		t.Errorf("offset past the end = %v, %d, %v; want none, 2, nil", got, total, err)
	}

	if _, _, err := s.SearchAssignments(n.ID, "printer", 0, 0); !errors.Is(err, ErrInvalidLimit) {
		t.Errorf("limit 0 error = %v, want ErrInvalidLimit", err)
	}
	if _, _, err := s.SearchAssignments(999, "x", 0, 1); !errors.Is(err, ErrNetworkNotFound) {
		t.Errorf("unknown network error = %v, want ErrNetworkNotFound", err)
	}
}

func TestSearchAssignmentsStaysInNetwork(t *testing.T) {
	s, _ := searchFixture(t)
	other, err := s.SaveNetwork("192.168.1.0/29", "Other")
	if err != nil {
		t.Fatalf("SaveNetwork: %v", err)
	}
	got, total, err := s.SearchAssignments(other.ID, "printer", 0, 10)
	if err != nil || total != 0 || len(got) != 0 {
		t.Errorf("other network = %v, %d, %v; want none", got, total, err)
	}
}

func TestNextFree(t *testing.T) {
	s, n := searchFixture(t) // offsets 0, 1, 2 are used

	tests := []struct {
		name string
		from uint64
		want string
	}{
		{"from the start", 0, "10.0.0.4"},
		{"from inside the used block", 1, "10.0.0.4"},
		{"from a free address", 4, "10.0.0.5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, ok, err := s.NextFree(n.ID, tt.from)
			if err != nil || !ok {
				t.Fatalf("NextFree = %v, %v, %v", e, ok, err)
			}
			if e.Addr.String() != tt.want || e.State != Free {
				t.Errorf("got %v (%v), want %s free", e.Addr, e.State, tt.want)
			}
		})
	}

	// A gap in the middle is found.
	if err := s.Unassign(n.ID, netip.MustParseAddr("10.0.0.2")); err != nil {
		t.Fatalf("Unassign: %v", err)
	}
	if e, ok, _ := s.NextFree(n.ID, 0); !ok || e.Addr.String() != "10.0.0.2" || e.Offset != 1 {
		t.Errorf("gap = %v, %v; want 10.0.0.2 at offset 1", e, ok)
	}

	// Past the last usable address there is nothing, and no error.
	if _, ok, err := s.NextFree(n.ID, 99); ok || err != nil {
		t.Errorf("from past the end = %v, %v; want false, nil", ok, err)
	}
	if _, _, err := s.NextFree(999, 0); !errors.Is(err, ErrNetworkNotFound) {
		t.Errorf("unknown network error = %v, want ErrNetworkNotFound", err)
	}
}

func TestNextFreeFullAndTinyNetworks(t *testing.T) {
	s := openSearchTestStore(t)

	full, err := s.SaveNetwork("10.1.0.0/30", "Full") // usable .1 and .2
	if err != nil {
		t.Fatalf("SaveNetwork: %v", err)
	}
	for _, a := range []string{"10.1.0.1", "10.1.0.2"} {
		if err := s.Assign(full.ID, netip.MustParseAddr(a), "x", ""); err != nil {
			t.Fatalf("Assign: %v", err)
		}
	}
	if _, ok, err := s.NextFree(full.ID, 0); ok || err != nil {
		t.Errorf("full network = %v, %v; want false, nil", ok, err)
	}

	single, err := s.SaveNetwork("10.2.0.5/32", "Host")
	if err != nil {
		t.Fatalf("SaveNetwork: %v", err)
	}
	e, ok, err := s.NextFree(single.ID, 0)
	if err != nil || !ok || e.Addr.String() != "10.2.0.5" {
		t.Errorf("/32 = %v, %v, %v; want 10.2.0.5", e, ok, err)
	}
}

func TestOffset(t *testing.T) {
	s, n := searchFixture(t)

	if off, err := s.Offset(n.ID, netip.MustParseAddr("10.0.0.4")); err != nil || off != 3 {
		t.Errorf("Offset(.4) = %d, %v; want 3", off, err)
	}
	for _, outside := range []string{"10.0.0.0", "10.0.0.7", "10.9.9.9"} {
		if _, err := s.Offset(n.ID, netip.MustParseAddr(outside)); !errors.Is(err, ErrAddressNotInNetwork) {
			t.Errorf("Offset(%s) error = %v, want ErrAddressNotInNetwork", outside, err)
		}
	}
	if _, err := s.Offset(n.ID, netip.MustParseAddr("2001:db8::1")); !errors.Is(err, ErrAddressNotInNetwork) {
		t.Errorf("Offset(IPv6) error = %v, want ErrAddressNotInNetwork", err)
	}
}

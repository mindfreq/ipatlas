package ipam

import (
	"errors"
	"net/netip"
	"path/filepath"
	"testing"
)

func openTest(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ipam.db")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s, path
}

func mustSave(t *testing.T, s *Store, cidr, name string) Network {
	t.Helper()
	n, err := s.SaveNetwork(cidr, name)
	if err != nil {
		t.Fatalf("SaveNetwork(%q): %v", cidr, err)
	}
	return n
}

func TestMigrateAndReopen(t *testing.T) {
	s, path := openTest(t)

	var version int
	if err := s.db.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&version); err != nil {
		t.Fatalf("read schema version: %v", err)
	}
	if version != len(migrations) {
		t.Errorf("schema version = %d, want %d", version, len(migrations))
	}

	mustSave(t, s, "192.168.1.0/24", "Office")
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()

	networks, err := reopened.Networks()
	if err != nil {
		t.Fatalf("Networks: %v", err)
	}
	if len(networks) != 1 || networks[0].Name != "Office" {
		t.Fatalf("networks = %+v, want the saved Office network", networks)
	}
}

func TestSaveNetwork(t *testing.T) {
	s, _ := openTest(t)

	n := mustSave(t, s, "192.168.1.9/24", " Office ")
	if n.CIDR != "192.168.1.0/24" {
		t.Errorf("cidr = %q, want 192.168.1.0/24", n.CIDR)
	}
	if n.Name != "Office" {
		t.Errorf("name = %q, want Office", n.Name)
	}
	if n.Subnet.Prefix != 24 || n.Subnet.Usable != 254 {
		t.Errorf("subnet = /%d with %d usable, want /24 with 254", n.Subnet.Prefix, n.Subnet.Usable)
	}
	if n.Subnet.First.String() != "192.168.1.1" || n.Subnet.Last.String() != "192.168.1.254" {
		t.Errorf("range = %s-%s", n.Subnet.First, n.Subnet.Last)
	}
	if n.CreatedAt.IsZero() {
		t.Error("created at is zero")
	}

	again := mustSave(t, s, "192.168.1.0/24", "Main office")
	if again.ID != n.ID {
		t.Errorf("duplicate ID = %d, want %d", again.ID, n.ID)
	}
	if again.Name != "Main office" {
		t.Errorf("duplicate name = %q, want Main office", again.Name)
	}
	networks, err := s.Networks()
	if err != nil {
		t.Fatalf("Networks: %v", err)
	}
	if len(networks) != 1 {
		t.Errorf("networks = %d, want 1 after saving the same CIDR twice", len(networks))
	}
}

func TestSaveNetworkDefaultsNameAndOrders(t *testing.T) {
	s, _ := openTest(t)

	first := mustSave(t, s, "10.0.0.0/8", "")
	if first.Name != "10.0.0.0/8" {
		t.Errorf("default name = %q, want the CIDR", first.Name)
	}
	second := mustSave(t, s, "192.168.0.0/24", "Branch")

	networks, err := s.Networks()
	if err != nil {
		t.Fatalf("Networks: %v", err)
	}
	if len(networks) != 2 {
		t.Fatalf("networks = %d, want 2", len(networks))
	}
	if networks[0].ID != second.ID || networks[1].ID != first.ID {
		t.Errorf("order = %d, %d; want newest %d first", networks[0].ID, networks[1].ID, second.ID)
	}
}

func TestSaveNetworkErrors(t *testing.T) {
	s, _ := openTest(t)

	for _, tc := range []struct {
		name string
		cidr string
	}{
		{name: "empty", cidr: ""},
		{name: "garbage", cidr: "nope"},
		{name: "ipv6", cidr: "2001:db8::/32"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := s.SaveNetwork(tc.cidr, ""); err == nil {
				t.Fatal("SaveNetwork succeeded, want an error")
			}
		})
	}
}

func TestNetworkLookup(t *testing.T) {
	s, _ := openTest(t)
	n := mustSave(t, s, "10.1.0.0/16", "Lab")

	byID, err := s.Network(n.ID)
	if err != nil {
		t.Fatalf("Network(id): %v", err)
	}
	if byID.CIDR != "10.1.0.0/16" {
		t.Errorf("cidr = %q", byID.CIDR)
	}
	byCIDR, err := s.Network("10.1.0.0/16")
	if err != nil {
		t.Fatalf("Network(cidr): %v", err)
	}
	if byCIDR.ID != n.ID {
		t.Errorf("id = %d, want %d", byCIDR.ID, n.ID)
	}

	if _, err := s.Network(int64(9999)); !errors.Is(err, ErrNetworkNotFound) {
		t.Errorf("missing network error = %v, want ErrNetworkNotFound", err)
	}
}

func TestAssignAndStats(t *testing.T) {
	s, _ := openTest(t)
	n := mustSave(t, s, "192.168.1.0/29", "Small")

	tests := []struct {
		addr  string
		label string
		owner string
	}{
		{addr: "192.168.1.1", label: "Gateway", owner: "netops"},
		{addr: "192.168.1.6", label: "Printer", owner: "office"},
		{addr: "192.168.1.2", label: "", owner: ""},
	}
	for _, tc := range tests {
		if err := s.Assign(n.ID, netip.MustParseAddr(tc.addr), tc.label, tc.owner); err != nil {
			t.Fatalf("Assign(%s): %v", tc.addr, err)
		}
	}

	got, ok, err := s.Assignment(n.ID, netip.MustParseAddr("192.168.1.1"))
	if err != nil {
		t.Fatalf("Assignment: %v", err)
	}
	if !ok || got.Label != "Gateway" || got.Owner != "netops" {
		t.Errorf("assignment = %+v, ok = %v", got, ok)
	}
	if got.Offset != 0 {
		t.Errorf("offset = %d, want 0", got.Offset)
	}

	if _, ok, err := s.Assignment(n.ID, netip.MustParseAddr("192.168.1.3")); err != nil || ok {
		t.Errorf("unassigned address: ok = %v, err = %v; want false, nil", ok, err)
	}

	stats, err := s.Stats(n.ID)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.Total != 6 || stats.Used != 3 || stats.Free != 3 {
		t.Errorf("stats = %+v, want total 6, used 3, free 3", stats)
	}

	if err := s.Assign(n.ID, netip.MustParseAddr("192.168.1.1"), "Router", "netops"); err != nil {
		t.Fatalf("reassign: %v", err)
	}
	stats, err = s.Stats(n.ID)
	if err != nil {
		t.Fatalf("Stats after reassign: %v", err)
	}
	if stats.Used != 3 {
		t.Errorf("used after reassign = %d, want 3", stats.Used)
	}
}

func TestAssignValidation(t *testing.T) {
	s, _ := openTest(t)
	n := mustSave(t, s, "10.0.0.0/30", "Point to point")

	for _, tc := range []struct {
		name string
		addr string
	}{
		{name: "network address", addr: "10.0.0.0"},
		{name: "broadcast address", addr: "10.0.0.3"},
		{name: "outside", addr: "10.0.0.4"},
		{name: "other network", addr: "10.0.1.1"},
		{name: "ipv6", addr: "2001:db8::1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := s.Assign(n.ID, netip.MustParseAddr(tc.addr), "", "")
			if !errors.Is(err, ErrAddressNotInNetwork) {
				t.Errorf("error = %v, want ErrAddressNotInNetwork", err)
			}
		})
	}

	if err := s.Assign(int64(9999), netip.MustParseAddr("10.0.0.1"), "", ""); !errors.Is(err, ErrNetworkNotFound) {
		t.Errorf("missing network error = %v, want ErrNetworkNotFound", err)
	}
}

func TestAssignSlash31And32(t *testing.T) {
	s, _ := openTest(t)

	p2p := mustSave(t, s, "10.0.0.0/31", "Link")
	for _, addr := range []string{"10.0.0.0", "10.0.0.1"} {
		if err := s.Assign(p2p.ID, netip.MustParseAddr(addr), "", ""); err != nil {
			t.Errorf("Assign(%s) on /31: %v", addr, err)
		}
	}
	stats, err := s.Stats(p2p.ID)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.Total != 2 || stats.Used != 2 || stats.Free != 0 {
		t.Errorf("stats = %+v, want 2 used of 2", stats)
	}

	host := mustSave(t, s, "10.0.0.7/32", "Host")
	if err := s.Assign(host.ID, netip.MustParseAddr("10.0.0.7"), "loopback", ""); err != nil {
		t.Fatalf("Assign on /32: %v", err)
	}
	if err := s.Assign(host.ID, netip.MustParseAddr("10.0.0.8"), "", ""); !errors.Is(err, ErrAddressNotInNetwork) {
		t.Errorf("neighbour on /32 error = %v, want ErrAddressNotInNetwork", err)
	}
}

func TestUnassign(t *testing.T) {
	s, _ := openTest(t)
	n := mustSave(t, s, "192.168.0.0/24", "Office")
	addr := netip.MustParseAddr("192.168.0.10")

	if err := s.Unassign(n.ID, addr); err != nil {
		t.Fatalf("Unassign without an assignment: %v", err)
	}
	if err := s.Assign(n.ID, addr, "Printer", "office"); err != nil {
		t.Fatalf("Assign: %v", err)
	}
	if err := s.Unassign(n.ID, addr); err != nil {
		t.Fatalf("Unassign: %v", err)
	}
	if _, ok, err := s.Assignment(n.ID, addr); err != nil || ok {
		t.Errorf("assignment after unassign: ok = %v, err = %v", ok, err)
	}
	if err := s.Unassign(n.ID, netip.MustParseAddr("192.168.0.255")); !errors.Is(err, ErrAddressNotInNetwork) {
		t.Errorf("broadcast unassign error = %v, want ErrAddressNotInNetwork", err)
	}
}

func TestAddresses(t *testing.T) {
	s, _ := openTest(t)
	n := mustSave(t, s, "192.168.1.0/24", "Office")

	if err := s.Assign(n.ID, netip.MustParseAddr("192.168.1.10"), "Printer", "office"); err != nil {
		t.Fatalf("Assign: %v", err)
	}
	if err := s.Assign(n.ID, netip.MustParseAddr("192.168.1.254"), "Gateway", "netops"); err != nil {
		t.Fatalf("Assign: %v", err)
	}

	page, err := s.Addresses(n.ID, 0, 3)
	if err != nil {
		t.Fatalf("Addresses: %v", err)
	}
	if len(page) != 3 {
		t.Fatalf("page length = %d, want 3", len(page))
	}
	if page[0].Addr.String() != "192.168.1.1" || page[0].State != Free {
		t.Errorf("first entry = %+v, want free 192.168.1.1", page[0])
	}
	if page[0].Offset != 0 || page[2].Offset != 2 {
		t.Errorf("offsets = %d, %d; want 0, 2", page[0].Offset, page[2].Offset)
	}

	page, err = s.Addresses(n.ID, 9, 2)
	if err != nil {
		t.Fatalf("Addresses middle page: %v", err)
	}
	if len(page) != 2 {
		t.Fatalf("middle page length = %d, want 2", len(page))
	}
	if page[0].Addr.String() != "192.168.1.10" || page[0].State != Used || page[0].Label != "Printer" {
		t.Errorf("entry = %+v, want used printer 192.168.1.10", page[0])
	}
	if page[1].Addr.String() != "192.168.1.11" || page[1].State != Free {
		t.Errorf("entry = %+v, want free 192.168.1.11", page[1])
	}

	page, err = s.Addresses(n.ID, 253, 10)
	if err != nil {
		t.Fatalf("Addresses last page: %v", err)
	}
	if len(page) != 1 || page[0].Addr.String() != "192.168.1.254" || page[0].Owner != "netops" {
		t.Errorf("last page = %+v, want the gateway", page)
	}
}

func TestAddressesBounds(t *testing.T) {
	s, _ := openTest(t)
	n := mustSave(t, s, "192.168.1.0/24", "Office")

	if _, err := s.Addresses(n.ID, 254, 1); !errors.Is(err, ErrOffsetOutOfRange) {
		t.Errorf("offset past end error = %v, want ErrOffsetOutOfRange", err)
	}
	for _, limit := range []int{0, -1} {
		if _, err := s.Addresses(n.ID, 0, limit); !errors.Is(err, ErrInvalidLimit) {
			t.Errorf("limit %d error = %v, want ErrInvalidLimit", limit, err)
		}
	}
	large := mustSave(t, s, "10.0.0.0/16", "Large")
	capped, err := s.Addresses(large.ID, 0, MaxAddressPage+100)
	if err != nil {
		t.Fatalf("Addresses capped: %v", err)
	}
	if len(capped) != MaxAddressPage {
		t.Errorf("capped page = %d, want %d", len(capped), MaxAddressPage)
	}
	if last := capped[len(capped)-1].Addr.String(); last != "10.0.4.0" {
		t.Errorf("last capped address = %s, want 10.0.4.0", last)
	}
}

func TestDeleteNetworkCascades(t *testing.T) {
	s, _ := openTest(t)
	n := mustSave(t, s, "10.10.0.0/24", "Lab")
	if err := s.Assign(n.ID, netip.MustParseAddr("10.10.0.1"), "Router", ""); err != nil {
		t.Fatalf("Assign: %v", err)
	}

	if err := s.DeleteNetwork(n.ID); err != nil {
		t.Fatalf("DeleteNetwork: %v", err)
	}
	if _, err := s.Network(n.ID); !errors.Is(err, ErrNetworkNotFound) {
		t.Errorf("lookup after delete error = %v, want ErrNetworkNotFound", err)
	}

	var assignments int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM assignments`).Scan(&assignments); err != nil {
		t.Fatalf("count assignments: %v", err)
	}
	if assignments != 0 {
		t.Errorf("assignments = %d, want 0 after cascade", assignments)
	}

	if err := s.DeleteNetwork(n.ID); !errors.Is(err, ErrNetworkNotFound) {
		t.Errorf("second delete error = %v, want ErrNetworkNotFound", err)
	}
}

func TestClosedStore(t *testing.T) {
	s, _ := openTest(t)
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
	if _, err := s.Networks(); !errors.Is(err, ErrClosed) {
		t.Errorf("Networks after close error = %v, want ErrClosed", err)
	}
	if _, err := s.SaveNetwork("10.0.0.0/24", ""); !errors.Is(err, ErrClosed) {
		t.Errorf("SaveNetwork after close error = %v, want ErrClosed", err)
	}
}

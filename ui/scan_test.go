package ui

import (
	"net"
	"net/netip"
	"strings"
	"testing"
	"time"

	myui "github.com/egoist/mygo/ui"

	"ipatlas/internal/netcalc"
)

func openPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	return ln.Addr().(*net.TCPAddr).Port
}

func closedPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	if err := ln.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}
	return port
}

func waitScan(t *testing.T, s *scanState) scanSnapshot {
	t.Helper()
	s.mu.Lock()
	done := s.doneCh
	err := s.err
	s.mu.Unlock()
	if done == nil {
		t.Fatalf("scan did not start: %s", err)
	}
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("scan did not finish")
	}
	return s.snapshot()
}

func TestScanTargets(t *testing.T) {
	tests := []struct {
		name    string
		cidr    string
		want    int
		wantErr string
	}{
		{name: "slash 32", cidr: "10.0.0.7/32", want: 1},
		{name: "slash 31", cidr: "10.0.0.0/31", want: 2},
		{name: "slash 30", cidr: "10.0.0.0/30", want: 2},
		{name: "slash 24", cidr: "10.0.0.0/24", want: 254},
		{name: "too large", cidr: "10.0.0.0/8", wantErr: "at most 4096"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			subnet, err := netcalc.ParseIPv4CIDR(tc.cidr)
			if err != nil {
				t.Fatal(err)
			}
			targets, err := scanTargets(subnet)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %v, want it to contain %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("scanTargets: %v", err)
			}
			if len(targets) != tc.want {
				t.Errorf("targets = %d, want %d", len(targets), tc.want)
			}
			if targets[0] != subnet.First {
				t.Errorf("first target = %s, want %s", targets[0], subnet.First)
			}
		})
	}
}

func TestScanStateAliveAndSilent(t *testing.T) {
	port := openPort(t)
	s := scanState{cidr: "127.0.0.1/32", ports: []int{port}, timeout: time.Second}
	s.start(nil, nil)
	snap := waitScan(t, &s)

	if snap.running {
		t.Error("scan is still running after its channel closed")
	}
	if snap.done != 1 || snap.total != 1 || snap.alive != 1 {
		t.Fatalf("snapshot = %+v, want one alive address", snap)
	}
	if snap.hosts[0].State != hostAlive || len(snap.hosts[0].Ports) != 1 {
		t.Errorf("host = %+v, want alive with one port", snap.hosts[0])
	}

	closed := closedPort(t)
	// A closed loopback port now counts as refused and therefore alive, so
	// the silent and no-answer paths use an address that never answers.
	s = scanState{cidr: "10.255.255.1/32", ports: []int{closed}, timeout: 300 * time.Millisecond}
	s.start(func(netip.Addr) (string, bool) { return "Printer", true }, nil)
	snap = waitScan(t, &s)
	if snap.documented != 1 || snap.hosts[0].State != hostDocumentedSilent {
		t.Errorf("snapshot = %+v, want one documented silent host", snap)
	}
	if snap.hosts[0].Label != "Printer" {
		t.Errorf("label = %q, want Printer", snap.hosts[0].Label)
	}

	s = scanState{cidr: "10.255.255.1/32", ports: []int{closed}, timeout: 300 * time.Millisecond}
	s.start(nil, nil)
	snap = waitScan(t, &s)
	if snap.noAnswer != 1 || snap.hosts[0].State != hostNoAnswer {
		t.Errorf("snapshot = %+v, want one unanswered host", snap)
	}
}

func TestScanStateStop(t *testing.T) {
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	docs := func(netip.Addr) (string, bool) {
		select {
		case entered <- struct{}{}:
		default:
		}
		<-release
		return "", false
	}

	s := scanState{cidr: "10.255.255.0/24", ports: []int{closedPort(t)}, timeout: 3 * time.Second}
	s.start(docs, nil)

	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("no probe reached the documentation hook")
	}
	s.stop()
	close(release)

	snap := waitScan(t, &s)
	if !snap.stopped || snap.running {
		t.Errorf("snapshot = %+v, want a stopped scan", snap)
	}
	if snap.done == snap.total {
		t.Errorf("done = %d, want fewer than %d after stopping", snap.done, snap.total)
	}
}

func TestScanInvalidTargets(t *testing.T) {
	tests := []struct {
		cidr string
		want string
	}{
		{cidr: "", want: "Enter a target network"},
		{cidr: "nope", want: "not a valid CIDR"},
		{cidr: "2001:db8::/32", want: "must be an IPv4 network"},
		{cidr: "10.0.0.0/8", want: "at most 4096"},
	}
	for _, tc := range tests {
		s := scanState{cidr: tc.cidr}
		s.start(nil, nil)
		if !strings.Contains(s.err, tc.want) {
			t.Errorf("cidr %q error = %q, want it to contain %q", tc.cidr, s.err, tc.want)
		}
		if s.running {
			t.Errorf("cidr %q started a scan", tc.cidr)
		}
	}
}

func TestScanViewSuccess(t *testing.T) {
	port := openPort(t)
	a := New()
	a.scan.ports = []int{port}
	tt := myui.NewTester(a.View, 1100, 720)

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
	if snap.alive != 1 {
		t.Fatalf("snapshot = %+v, want one alive host", snap)
	}
	tt.Frame()

	for _, want := range []string{
		"127.0.0.1",
		"1 alive, 0 documented but silent, 0 no answer",
		"Scanned 1 of 1 addresses",
		"alive",
	} {
		if !tt.HasText(want) {
			t.Errorf("texts %q do not contain %q", tt.Texts(), want)
		}
	}
}

func TestScanViewSlash24StaysResponsive(t *testing.T) {
	port := closedPort(t)
	a := New()
	a.scan.ports = []int{port}
	a.scan.timeout = 50 * time.Millisecond
	tt := myui.NewTester(a.View, 1100, 720)

	if err := tt.Click("Scan"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Scan target"); err != nil {
		t.Fatal(err)
	}
	// Otherwise every loopback refusal counts as alive, so the type-in
	// target is an unroutable private address that never answers.
	tt.Type("10.255.255.0/24")
	if err := tt.Click("Start scan"); err != nil {
		t.Fatal(err)
	}
	// Building a frame must not wait for the scan.
	tt.Frame()

	snap := waitScan(t, &a.scan)
	if snap.done != 254 || snap.total != 254 {
		t.Fatalf("snapshot = %+v, want all 254 addresses scanned", snap)
	}
	if snap.noAnswer != 254 {
		t.Errorf("no answer = %d, want 254", snap.noAnswer)
	}
	tt.Frame()
	if !tt.HasText("Scanned 254 of 254 addresses") {
		t.Errorf("texts %q do not show the finished scan", tt.Texts())
	}
}

func TestScanDocumentsIPAMAddresses(t *testing.T) {
	store, _ := openIPAM(t)
	// A closed loopback port now counts as refused and therefore alive, so
	// the documented-silent path uses an address that never answers.
	network, err := store.SaveNetwork("10.255.255.0/24", "Unrouted")
	if err != nil {
		t.Fatalf("SaveNetwork: %v", err)
	}
	if err := store.Assign(network.ID, netip.MustParseAddr("10.255.255.1"), "Printer", "Office"); err != nil {
		t.Fatalf("Assign: %v", err)
	}

	a := NewWithStore(store)
	a.scan.cidr = "10.255.255.1/32"
	a.scan.ports = []int{closedPort(t)}
	a.scan.timeout = 300 * time.Millisecond
	a.StartScan(nil)
	snap := waitScan(t, &a.scan)

	if snap.documented != 1 || snap.hosts[0].State != hostDocumentedSilent {
		t.Fatalf("snapshot = %+v, want a documented silent host", snap)
	}
	if snap.hosts[0].Label != "Printer" {
		t.Errorf("label = %q, want Printer", snap.hosts[0].Label)
	}
}

func TestScanViewInitialButtons(t *testing.T) {
	a := New()
	tt := myui.NewTester(a.View, 1100, 720)
	if err := tt.Click("Scan"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Start scan") || !tt.HasText("Stop scan") {
		t.Errorf("texts %q do not show both scan buttons", tt.Texts())
	}
}

// TestScanViewFilter scopes the results grid with the segmented filter and
// the search box after a scan finds one alive host.
func TestScanViewFilter(t *testing.T) {
	port := openPort(t)
	a := New()
	a.scan.ports = []int{port}
	tt := myui.NewTester(a.View, 1100, 720)

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
	if snap := waitScan(t, &a.scan); snap.alive != 1 {
		t.Fatalf("snapshot = %+v, want one alive host", snap)
	}
	tt.Frame()
	if !tt.HasText("127.0.0.1") {
		t.Fatalf("texts %q do not show the result tile", tt.Texts())
	}

	// The no-answer filter hides the alive host.
	if err := tt.Click("No answer"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tt.HasText("127.0.0.1") {
		t.Error("no-answer filter still shows the alive tile")
	}
	if !tt.HasText("No addresses match this filter") {
		t.Errorf("texts %q do not show the empty filter message", tt.Texts())
	}

	// The alive filter brings it back.
	if err := tt.Click("Alive"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("127.0.0.1") {
		t.Error("alive filter hides the alive tile")
	}

	// A query that matches nothing clears the grid again.
	if err := tt.Click("Search results"); err != nil {
		t.Fatal(err)
	}
	tt.Type("no-such-host")
	tt.Frame()
	if tt.HasText("127.0.0.1") {
		t.Error("search that matches nothing still shows the tile")
	}

	// Resetting the filter and search restores the grid.
	a.scan.search = ""
	a.scan.filter = scanAll
	tt2 := myui.NewTester(a.View, 1100, 720)
	tt2.Frame()
	if !tt2.HasText("127.0.0.1") {
		t.Error("all filter with empty search hides the tile")
	}
}

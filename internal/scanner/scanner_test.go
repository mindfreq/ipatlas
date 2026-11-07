package scanner

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

func listen(t *testing.T) (net.Listener, int) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	return ln, ln.Addr().(*net.TCPAddr).Port
}

func collect(t *testing.T, results <-chan Result, timeout time.Duration) []Result {
	t.Helper()
	var got []Result
	deadline := time.After(timeout)
	for {
		select {
		case r, ok := <-results:
			if !ok {
				return got
			}
			got = append(got, r)
		case <-deadline:
			t.Fatalf("scan did not close its channel within %v", timeout)
		}
	}
}

func TestOptionsDefaults(t *testing.T) {
	cfg := Options{}.config()
	if cfg.workers != DefaultWorkers {
		t.Errorf("workers = %d, want %d", cfg.workers, DefaultWorkers)
	}
	if cfg.timeout != DefaultTimeout {
		t.Errorf("timeout = %v, want %v", cfg.timeout, DefaultTimeout)
	}
	if len(cfg.ports) != len(DefaultPorts) {
		t.Fatalf("ports = %v, want %v", cfg.ports, DefaultPorts)
	}
	for i, port := range DefaultPorts {
		if cfg.ports[i] != port {
			t.Errorf("port %d = %d, want %d", i, cfg.ports[i], port)
		}
	}
	if cfg.icmp {
		t.Error("ICMP is enabled by default")
	}

	cfg = Options{
		Ports:   []int{0, 22, 70000, 443},
		Workers: maxWorkers + 10,
		Timeout: -time.Second,
		ICMP:    true,
	}.config()
	if len(cfg.ports) != 2 || cfg.ports[0] != 22 || cfg.ports[1] != 443 {
		t.Errorf("filtered ports = %v, want [22 443]", cfg.ports)
	}
	if cfg.workers != maxWorkers {
		t.Errorf("workers = %d, want %d", cfg.workers, maxWorkers)
	}
	if cfg.timeout != DefaultTimeout {
		t.Errorf("timeout = %v, want %v", cfg.timeout, DefaultTimeout)
	}
	if !cfg.icmp {
		t.Error("ICMP option was lost")
	}
}

func TestScanOpenPorts(t *testing.T) {
	_, portA := listen(t)
	_, portB := listen(t)

	results := Scan(context.Background(), []netip.Addr{netip.MustParseAddr("127.0.0.1")}, Options{
		Ports:   []int{portA, portB, 1},
		Workers: 2,
		Timeout: time.Second,
	})
	got := collect(t, results, 5*time.Second)

	if len(got) != 1 {
		t.Fatalf("results = %d, want 1", len(got))
	}
	r := got[0]
	if !r.Alive {
		t.Errorf("alive = false, want true")
	}
	wantPorts := []int{portA, portB}
	sort.Ints(wantPorts)
	if len(r.OpenPorts) != 2 || r.OpenPorts[0] != wantPorts[0] || r.OpenPorts[1] != wantPorts[1] {
		t.Errorf("open ports = %v, want %v", r.OpenPorts, wantPorts)
	}
	if r.Address.String() != "127.0.0.1" {
		t.Errorf("address = %s", r.Address)
	}
}

func TestScanClosedPort(t *testing.T) {
	ln, port := listen(t)
	ln.Close()

	results := Scan(context.Background(), []netip.Addr{netip.MustParseAddr("127.0.0.1")}, Options{
		Ports:   []int{port},
		Workers: 1,
		Timeout: 300 * time.Millisecond,
	})
	got := collect(t, results, 5*time.Second)

	if len(got) != 1 {
		t.Fatalf("results = %d, want 1", len(got))
	}
	// The refused connect proves the host answered: alive with no open ports.
	if !got[0].Alive {
		t.Errorf("closed port result = %+v, want alive", got[0])
	}
	if len(got[0].OpenPorts) != 0 {
		t.Errorf("closed port result = %+v, want no open ports", got[0])
	}
}

// TestProbePortClassification checks each dial outcome: an open connect asks
// for OpenPorts, a refused one (RST) keeps the host alive without the port,
// and a no-response dial has no effect.
func TestProbePortClassification(t *testing.T) {
	_, openPort := listen(t)
	ln, port := listen(t)
	ln.Close()

	tests := []struct {
		name      string
		port      int
		wantAlive bool
		wantOpen  []int
	}{
		{name: "open port", port: openPort, wantAlive: true, wantOpen: []int{openPort}},
		{name: "refused port", port: port, wantAlive: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Probe(context.Background(), netip.MustParseAddr("127.0.0.1"), Options{
				Ports:   []int{tc.port, 9},
				Timeout: 300 * time.Millisecond,
			})
			if got.Alive != tc.wantAlive {
				t.Errorf("alive = %v, want %v", got.Alive, tc.wantAlive)
			}
			if len(got.OpenPorts) != len(tc.wantOpen) || (len(tc.wantOpen) > 0 && got.OpenPorts[0] != tc.wantOpen[0]) {
				t.Errorf("open ports = %v, want %v", got.OpenPorts, tc.wantOpen)
			}
		})
	}
}

// TestProbeARPSignals covers ARP entries read after the probes: a valid entry
// alone keeps a silent host alive, while incomplete (0x0) and zero MAC
// entries do not. The hosts dial a TEST-NET address no machine answers, so
// the ARP entry is the only signal.
func TestProbeARPSignals(t *testing.T) {
	fixture := `IP address       HW type     Flags       HW address            Mask     Device
198.51.100.9     0x1         0x2         aa:bb:cc:dd:ee:ff     *        eth0
198.51.100.7     0x1         0x0         00:00:00:00:00:00     *        eth0
198.51.100.8     0x1         0x2         00:00:00:00:00:00     *        eth0
`
	path := filepath.Join(t.TempDir(), "arp")
	if err := os.WriteFile(path, []byte(fixture), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	tests := []struct {
		name      string
		addr      string
		wantAlive bool
		wantMAC   string
	}{
		{name: "valid entry keeps a silent host alive", addr: "198.51.100.9", wantAlive: true, wantMAC: "aa:bb:cc:dd:ee:ff"},
		{name: "incomplete entry", addr: "198.51.100.7"},
		{name: "zero MAC entry", addr: "198.51.100.8"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Probe(context.Background(), netip.MustParseAddr(tc.addr), Options{
				Ports:        []int{9},
				Timeout:      250 * time.Millisecond,
				ARPTablePath: path,
			})
			if got.Alive != tc.wantAlive {
				t.Errorf("alive = %v, want %v", got.Alive, tc.wantAlive)
			}
			if got.MAC != tc.wantMAC {
				t.Errorf("mac = %q, want %q", got.MAC, tc.wantMAC)
			}
		})
	}
}

// TestProbeReadsARPAfterProbing proves the ARP read happens after the port
// loop: the table can only return once the listener saw the dial, so a read
// before the loop would hang the probe until the test fails on the deadline.
func TestProbeReadsARPAfterProbing(t *testing.T) {
	ln, port := listen(t)
	accepted := make(chan struct{})
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			close(accepted)
			return
		}
		conn.Close()
		close(accepted)
	}()

	// The hook blocks until the dial is observed, then hands back the table.
	table := make(chan map[netip.Addr]string, 1)
	hook := func(string) map[netip.Addr]string { return <-table }

	done := make(chan Result, 1)
	go func() {
		done <- Probe(context.Background(), netip.MustParseAddr("127.0.0.1"), Options{
			Ports:   []int{port},
			Timeout: 2 * time.Second,
			arpRead: hook,
		})
	}()

	select {
	case <-accepted:
	case <-time.After(2 * time.Second):
		t.Fatal("probe did not dial before reading the ARP table")
	}

	table <- map[netip.Addr]string{
		netip.MustParseAddr("127.0.0.1"): "ab:cd:ef:01:02:03",
	}

	select {
	case got := <-done:
		if !got.Alive {
			t.Error("alive = false, want the open port and the ARP entry")
		}
		if len(got.OpenPorts) != 1 || got.OpenPorts[0] != port {
			t.Errorf("open ports = %v, want [%d]", got.OpenPorts, port)
		}
		if got.MAC != "ab:cd:ef:01:02:03" {
			t.Errorf("mac = %q, want ab:cd:ef:01:02:03", got.MAC)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("probe did not finish")
	}
}

func TestScanManyTargets(t *testing.T) {
	ln, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	openPort := ln.Addr().(*net.TCPAddr).Port

	targets := []netip.Addr{
		netip.MustParseAddr("127.0.0.1"),
		netip.MustParseAddr("127.0.0.2"),
		netip.MustParseAddr("127.0.0.1"), // duplicate
	}
	results := Scan(context.Background(), targets, Options{
		Ports:   []int{openPort},
		Workers: 1,
		Timeout: time.Second,
	})
	got := collect(t, results, 10*time.Second)

	if len(got) != 2 {
		t.Fatalf("results = %d, want 2 after dropping the duplicate", len(got))
	}
	for _, r := range got {
		if !r.Alive {
			t.Errorf("%s is not alive, want an open port", r.Address)
		}
	}
}

func TestScanCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	results := Scan(ctx, []netip.Addr{netip.MustParseAddr("192.0.2.1")}, Options{
		Ports:   []int{80, 443},
		Workers: 4,
		Timeout: 5 * time.Second,
	})
	got := collect(t, results, 2*time.Second)
	for _, r := range got {
		if r.Alive {
			t.Errorf("%s reported alive after cancellation", r.Address)
		}
	}
	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Errorf("context error = %v, want context.Canceled", ctx.Err())
	}
}

func TestProbeCancellationDuringDial(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	// Reading until the channel closes proves the dial honored the context;
	// the address is normally unreachable, so without cancellation this
	// would wait for the 10 second dial timeout.
	for range Scan(ctx, []netip.Addr{netip.MustParseAddr("10.255.255.1")}, Options{
		Ports:   []int{9},
		Workers: 1,
		Timeout: 10 * time.Second,
	}) {
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("scan took %v after a 50ms deadline", elapsed)
	}
}

func TestProbeWithARPTable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "arp")
	fixture := `IP address       HW type     Flags       HW address            Mask     Device
127.0.0.1        0x1         0x2         AA:BB:CC:DD:EE:FF     *        lo
`
	if err := os.WriteFile(path, []byte(fixture), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	ln, port := listen(t)
	_ = ln
	result := Probe(context.Background(), netip.MustParseAddr("127.0.0.1"), Options{
		Ports:        []int{port},
		Timeout:      time.Second,
		ARPTablePath: path,
	})
	if !result.Alive {
		t.Error("alive = false, want the local listener")
	}
	if result.MAC != "aa:bb:cc:dd:ee:ff" {
		t.Errorf("mac = %q, want aa:bb:cc:dd:ee:ff", result.MAC)
	}
}

func TestParseARPTable(t *testing.T) {
	fixture := `IP address       HW type     Flags       HW address            Mask     Device
192.168.1.1      0x1         0x2         aa:bb:cc:dd:ee:ff     *        eth0
192.168.1.2      0x1         0x0         00:00:00:00:00:00     *        eth0
192.168.1.3      0x1         0x2         11:22:33:44:55:66     *        wlan0
192.168.1.4      0x1         0x2         not-a-mac             *        eth0
not-an-ip        0x1         0x2         22:33:44:55:66:77     *        eth0
192.168.1.5      0x1         0x2
fd00::1          0x1         0x2         aa:bb:cc:dd:ee:ff     *        eth0

`
	table, err := ParseARPTable(strings.NewReader(fixture))
	if err != nil {
		t.Fatalf("ParseARPTable: %v", err)
	}
	if len(table) != 2 {
		t.Fatalf("table = %v, want 2 valid entries", table)
	}
	if got := table[netip.MustParseAddr("192.168.1.1")]; got != "aa:bb:cc:dd:ee:ff" {
		t.Errorf("MAC for 192.168.1.1 = %q", got)
	}
	if got := table[netip.MustParseAddr("192.168.1.3")]; got != "11:22:33:44:55:66" {
		t.Errorf("MAC for 192.168.1.3 = %q", got)
	}
	if _, ok := table[netip.MustParseAddr("192.168.1.2")]; ok {
		t.Error("zero MAC entry was kept")
	}
	if _, ok := table[netip.MustParseAddr("fd00::1")]; ok {
		t.Error("IPv6 entry was kept")
	}
}

func TestLookupMACFromMissingTable(t *testing.T) {
	if got := LookupMACFrom(filepath.Join(t.TempDir(), "missing"), netip.MustParseAddr("192.168.1.1")); got != "" {
		t.Errorf("MAC = %q, want empty for a missing table", got)
	}
}

func TestICMPFallsBack(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if icmpAlive(context.Background(), netip.MustParseAddr("127.0.0.1")) {
		t.Error("icmpAlive succeeded without a ping binary")
	}

	ln, port := listen(t)
	_ = ln
	result := Probe(context.Background(), netip.MustParseAddr("127.0.0.1"), Options{
		Ports:   []int{port},
		Timeout: time.Second,
		ICMP:    true,
	})
	if !result.Alive {
		t.Error("TCP probe failed with ICMP enabled and ping missing")
	}
}

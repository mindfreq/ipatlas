package ui

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	myui "github.com/egoist/mygo/ui"

	"ipatlas/internal/netcalc"
	"ipatlas/internal/scanner"
)

// maxScanTargets bounds one run so a typo cannot queue millions of probes.
const maxScanTargets = 4096

type hostState uint8

const (
	hostPending hostState = iota
	hostAlive
	hostDocumentedSilent
	hostNoAnswer
)

type scanHost struct {
	Addr   netip.Addr
	State  hostState
	Ports  []int
	MAC    string
	Label  string
	Silent bool
}

type scanSnapshot struct {
	running    bool
	stopped    bool
	done       int
	total      int
	alive      int
	documented int
	noAnswer   int
	hosts      []scanHost
	err        string
}

type scanState struct {
	mu sync.Mutex

	cidr    string
	icmp    bool
	ports   []int
	timeout time.Duration

	running  bool
	stopped  bool
	done     int
	total    int
	alive    int
	document int
	noAnswer int
	hosts    []scanHost
	index    map[netip.Addr]int
	err      string

	cancel context.CancelFunc
	doneCh chan struct{}
}

// start validates the target and launches the probes. documented, when not
// nil, names an address that the IPAM database already knows; such a host
// that does not answer is shown as documented but silent.
func (s *scanState) start(documented func(netip.Addr) (string, bool), invalidate func()) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.err = ""
	s.stopped = false

	input := strings.TrimSpace(s.cidr)
	if input == "" {
		s.err = "Enter a target network, such as 192.168.1.0/24."
		s.mu.Unlock()
		return
	}
	subnet, err := netcalc.ParseIPv4CIDR(input)
	if err != nil {
		switch {
		case errors.Is(err, netcalc.ErrNotIPv4):
			s.err = "The scan target must be an IPv4 network."
		case errors.Is(err, netcalc.ErrInvalidCIDR):
			s.err = "The scan target is not a valid CIDR block."
		default:
			s.err = err.Error()
		}
		s.mu.Unlock()
		return
	}
	targets, err := scanTargets(subnet)
	if err != nil {
		s.err = err.Error()
		s.mu.Unlock()
		return
	}

	hosts := make([]scanHost, len(targets))
	index := make(map[netip.Addr]int, len(targets))
	for i, addr := range targets {
		hosts[i] = scanHost{Addr: addr}
		index[addr] = i
	}

	ports := append([]int(nil), s.ports...)
	timeout := s.timeout
	if timeout <= 0 {
		timeout = scanner.DefaultTimeout
	}
	icmp := s.icmp

	ctx, cancel := context.WithCancel(context.Background())
	doneCh := make(chan struct{})

	s.hosts = hosts
	s.index = index
	s.done = 0
	s.total = len(targets)
	s.alive = 0
	s.document = 0
	s.noAnswer = 0
	s.running = true
	s.cancel = cancel
	s.doneCh = doneCh
	s.mu.Unlock()

	go s.consume(ctx, cancel, targets, ports, timeout, icmp, documented, invalidate, doneCh)
}

func (s *scanState) consume(
	ctx context.Context,
	cancel context.CancelFunc,
	targets []netip.Addr,
	ports []int,
	timeout time.Duration,
	icmp bool,
	documented func(netip.Addr) (string, bool),
	invalidate func(),
	doneCh chan struct{},
) {
	defer cancel()

	results := scanner.Scan(ctx, targets, scanner.Options{
		Ports:   ports,
		Timeout: timeout,
		ICMP:    icmp,
	})

	processed := 0
	for result := range results {
		label := ""
		silent := false
		if !result.Alive && documented != nil {
			if name, ok := documented(result.Address); ok {
				label = name
				silent = true
			}
		}

		s.mu.Lock()
		if i, ok := s.index[result.Address]; ok {
			host := &s.hosts[i]
			host.Ports = result.OpenPorts
			host.MAC = result.MAC
			host.Label = label
			host.Silent = silent
			switch {
			case result.Alive:
				host.State = hostAlive
				s.alive++
			case silent:
				host.State = hostDocumentedSilent
				s.document++
			default:
				host.State = hostNoAnswer
				s.noAnswer++
			}
			s.done++
		}
		s.mu.Unlock()

		processed++
		if invalidate != nil && (processed%8 == 0 || processed == len(targets)) {
			invalidate()
		}
	}

	s.mu.Lock()
	s.running = false
	s.stopped = ctx.Err() != nil
	s.cancel = nil
	close(doneCh)
	s.mu.Unlock()

	if invalidate != nil {
		invalidate()
	}
}

func (s *scanState) stop() {
	s.mu.Lock()
	if s.running && s.cancel != nil {
		s.cancel()
	}
	s.mu.Unlock()
}

func (s *scanState) snapshot() scanSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	hosts := make([]scanHost, len(s.hosts))
	copy(hosts, s.hosts)
	for i := range hosts {
		hosts[i].Ports = append([]int(nil), hosts[i].Ports...)
	}
	return scanSnapshot{
		running:    s.running,
		stopped:    s.stopped,
		done:       s.done,
		total:      s.total,
		alive:      s.alive,
		documented: s.document,
		noAnswer:   s.noAnswer,
		hosts:      hosts,
		err:        s.err,
	}
}

func scanTargets(subnet netcalc.IPv4Subnet) ([]netip.Addr, error) {
	if !subnet.First.IsValid() || !subnet.Last.IsValid() {
		return nil, fmt.Errorf("the network has no usable addresses")
	}
	first := addrUint32(subnet.First)
	last := addrUint32(subnet.Last)
	count := uint64(last-first) + 1
	if count > maxScanTargets {
		return nil, fmt.Errorf(
			"that network has %d usable addresses; scan at most %d at once",
			count, maxScanTargets,
		)
	}
	targets := make([]netip.Addr, 0, int(count))
	for ip := first; ; ip++ {
		targets = append(targets, uint32Addr(ip))
		if ip == last {
			break
		}
	}
	return targets, nil
}

func addrUint32(addr netip.Addr) uint32 {
	b := addr.As4()
	return binary.BigEndian.Uint32(b[:])
}

func uint32Addr(v uint32) netip.Addr {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], v)
	return netip.AddrFrom4(b)
}

// scanDocumentation captures the current IPAM network so background probes
// can look assignments up without touching UI state.
func (a *App) scanDocumentation() func(netip.Addr) (string, bool) {
	store := a.ipam.store
	network := a.ipam.network
	if store == nil || network == 0 {
		return nil
	}
	return func(addr netip.Addr) (string, bool) {
		assignment, ok, err := store.Assignment(network, addr)
		if err != nil || !ok {
			return "", false
		}
		switch {
		case assignment.Label != "":
			return assignment.Label, true
		case assignment.Owner != "":
			return assignment.Owner, true
		default:
			return "documented", true
		}
	}
}

// StartScan begins scanning the target network, if the view has one.
func (a *App) StartScan(invalidate func()) {
	a.scan.start(a.scanDocumentation(), invalidate)
}

// StopScan cancels a running scan.
func (a *App) StopScan() {
	a.scan.stop()
}

func (a *App) scanView(c *myui.Context) {
	s := &a.scan
	t := c.Theme()
	services := c.Services()
	snap := s.snapshot()

	myui.Column(c).Fill().Gap(12).Children(func() {
		myui.Text(c, "Live scan").FontSize(20).Bold()
		myui.Text(c, "Probe a network with TCP connects; addresses update as results arrive.").TextColor(t.TextMuted)

		myui.Row(c).Gap(8).AlignItems(myui.Center).Children(func() {
			myui.TextInput(c, &s.cidr).Label("Scan target").Placeholder("192.168.1.0/24").Width(280)
			myui.Checkbox(c, &s.icmp, "Try ICMP when TCP finds nothing")
			scanButton := myui.PrimaryButton(c, "Start scan").OnClick(func() {
				a.StartScan(services.Invalidate)
			})
			scanButton.Disabled(snap.running)
			stopButton := myui.Button(c, "Stop scan").OnClick(a.StopScan)
			stopButton.Disabled(!snap.running)
		})

		if snap.err != "" {
			myui.Text(c, snap.err).TextColor(t.Danger).Selectable()
		}
		if snap.total > 0 {
			a.scanResults(c, snap)
		}
	})
}

func (a *App) scanResults(c *myui.Context, snap scanSnapshot) {
	t := c.Theme()

	status := fmt.Sprintf("Scanned %d of %d addresses", snap.done, snap.total)
	if snap.running {
		status = fmt.Sprintf("Scanning %d of %d addresses", snap.done, snap.total)
	} else if snap.stopped {
		status = fmt.Sprintf("Stopped after %d of %d addresses", snap.done, snap.total)
	}

	myui.Column(c).Grow(1).Gap(8).Children(func() {
		myui.Row(c).Gap(12).AlignItems(myui.Center).Children(func() {
			myui.Text(c, status).Bold()
			myui.Text(c, fmt.Sprintf(
				"%d alive, %d documented but silent, %d no answer",
				snap.alive, snap.documented, snap.noAnswer,
			)).TextColor(t.TextMuted)
			if left := snap.total - snap.done; left > 0 && !snap.running {
				myui.Text(c, fmt.Sprintf("%d not scanned", left)).TextColor(t.Warning)
			}
		})

		a.scanProgress(c, snap)
		a.scanLegend(c)
		a.scanGrid(c, snap)
	})
}

func (a *App) scanProgress(c *myui.Context, snap scanSnapshot) {
	t := c.Theme()
	if snap.total == 0 {
		return
	}
	done := snap.done
	if done > snap.total {
		done = snap.total
	}
	myui.Row(c).Height(10).Children(func() {
		if done > 0 {
			myui.Box(c).Height(10).Grow(float32(done)).Background(t.Accent)
		}
		if remaining := snap.total - done; remaining > 0 {
			myui.Box(c).Height(10).Grow(float32(remaining)).Background(t.SurfacePressed)
		}
	})
}

func (a *App) scanLegend(c *myui.Context) {
	t := c.Theme()
	legend := func(color myui.Color, label string) {
		myui.Row(c).Gap(4).AlignItems(myui.Center).Children(func() {
			myui.Box(c).Width(8).Height(8).Radius(2).Background(color)
			myui.Text(c, label).FontSize(11).TextColor(t.TextMuted)
		})
	}
	myui.Row(c).Gap(14).Children(func() {
		legend(t.Success, "alive")
		legend(t.Warning, "documented but silent")
		legend(t.TextMuted, "no answer")
	})
}

func (a *App) scanGrid(c *myui.Context, snap scanSnapshot) {
	t := c.Theme()
	myui.Scroll(c).Grow(1).Children(func() {
		myui.Grid(c).Columns(6).Gap(4).Children(func() {
			for _, host := range snap.hosts {
				host := host
				myui.Column(c).Key(host.Addr.String()).Gap(2).Children(func() {
					myui.Row(c).Gap(4).AlignItems(myui.Center).Children(func() {
						myui.Box(c).Width(8).Height(8).Radius(2).Background(scanColor(t, host.State))
						myui.Text(c, host.Addr.String()).FontSize(12).TextColor(scanColor(t, host.State))
					})
					myui.Text(c, scanDetail(host)).FontSize(11).SingleLine().TextColor(t.TextMuted)
				})
			}
		})
	})
}

func scanColor(t *myui.Theme, state hostState) myui.Color {
	switch state {
	case hostAlive:
		return t.Success
	case hostDocumentedSilent:
		return t.Warning
	case hostNoAnswer:
		return t.TextMuted
	default:
		return t.Border
	}
}

func scanDetail(host scanHost) string {
	switch host.State {
	case hostAlive:
		if len(host.Ports) > 0 {
			parts := make([]string, len(host.Ports))
			for i, port := range host.Ports {
				parts[i] = strconv.Itoa(port)
			}
			return strings.Join(parts, ", ")
		}
		return "alive"
	case hostDocumentedSilent:
		if host.Label != "" {
			return host.Label + " (silent)"
		}
		return "documented but silent"
	case hostNoAnswer:
		return "no answer"
	default:
		return "waiting"
	}
}

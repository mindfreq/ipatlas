// Package scanner probes IPv4 hosts with bounded TCP connect scans and reads
// MAC addresses from the operating system's ARP table. It has no UI
// dependencies.
package scanner

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DefaultPorts are the services a host scan probes.
var DefaultPorts = []int{80, 443, 22, 445, 3389}

const (
	// DefaultWorkers bounds how many hosts are probed at once.
	DefaultWorkers = 64
	// DefaultTimeout bounds each TCP connect and ICMP attempt.
	DefaultTimeout = 750 * time.Millisecond
	// maxWorkers keeps a mistaken setting from spawning unbounded goroutines.
	maxWorkers = 1024
	// arpTablePath is where Linux lists the neighbours it has learned.
	arpTablePath = "/proc/net/arp"
)

// Options configures a scan. The zero value uses DefaultPorts, DefaultWorkers
// and DefaultTimeout without ICMP.
type Options struct {
	// Ports to probe. Empty means DefaultPorts.
	Ports []int
	// Workers is the size of the probe pool. Zero means DefaultWorkers.
	Workers int
	// Timeout bounds each port attempt. Zero means DefaultTimeout.
	Timeout time.Duration
	// ICMP also tries one ping after the TCP probes find nothing. A missing
	// ping binary or a permission error is ignored.
	ICMP bool
	// ARPTablePath overrides the OS ARP table file, for tests. Empty means
	// the platform's table.
	ARPTablePath string
}

type config struct {
	ports   []int
	workers int
	timeout time.Duration
	icmp    bool
	arpPath string
}

func (o Options) config() config {
	cfg := config{
		workers: o.Workers,
		timeout: o.Timeout,
		icmp:    o.ICMP,
		arpPath: o.ARPTablePath,
	}
	if len(o.Ports) == 0 {
		cfg.ports = append([]int(nil), DefaultPorts...)
	} else {
		for _, port := range o.Ports {
			if port > 0 && port <= 65535 {
				cfg.ports = append(cfg.ports, port)
			}
		}
		if len(cfg.ports) == 0 {
			cfg.ports = append([]int(nil), DefaultPorts...)
		}
	}
	if cfg.workers <= 0 {
		cfg.workers = DefaultWorkers
	}
	if cfg.workers > maxWorkers {
		cfg.workers = maxWorkers
	}
	if cfg.timeout <= 0 {
		cfg.timeout = DefaultTimeout
	}
	return cfg
}

// Result is one probed host.
type Result struct {
	Address   netip.Addr
	Alive     bool
	OpenPorts []int
	MAC       string
}

// Scan probes targets concurrently and sends each result as it completes. It
// closes the returned channel when every worker is done or ctx is canceled.
// The caller must keep receiving until the channel closes; cancel ctx to stop
// early.
func Scan(ctx context.Context, targets []netip.Addr, opts Options) <-chan Result {
	cfg := opts.config()
	arp := readARPTable(cfg.arpPath)

	out := make(chan Result)
	jobs := make(chan netip.Addr)

	var wg sync.WaitGroup
	wg.Add(cfg.workers)
	for i := 0; i < cfg.workers; i++ {
		go func() {
			defer wg.Done()
			for addr := range jobs {
				result := probe(ctx, addr, cfg, arp)
				select {
				case out <- result:
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	go func() {
		defer close(jobs)
		seen := make(map[netip.Addr]struct{}, len(targets))
		for _, addr := range targets {
			if !addr.Is4() || addr.IsUnspecified() || addr.IsMulticast() {
				continue
			}
			if _, ok := seen[addr]; ok {
				continue
			}
			seen[addr] = struct{}{}
			select {
			case jobs <- addr:
			case <-ctx.Done():
				return
			}
		}
	}()

	go func() {
		wg.Wait()
		close(out)
	}()

	return out
}

// Probe scans one address synchronously.
func Probe(ctx context.Context, addr netip.Addr, opts Options) Result {
	cfg := opts.config()
	return probe(ctx, addr, cfg, readARPTable(cfg.arpPath))
}

func probe(ctx context.Context, addr netip.Addr, cfg config, arp map[netip.Addr]string) Result {
	result := Result{Address: addr, MAC: arp[addr]}

	for _, port := range cfg.ports {
		if ctx.Err() != nil {
			return result
		}
		if tcpOpen(ctx, addr, port, cfg.timeout) {
			result.Alive = true
			result.OpenPorts = append(result.OpenPorts, port)
		}
	}
	sort.Ints(result.OpenPorts)

	if !result.Alive && cfg.icmp && ctx.Err() == nil {
		if icmpAlive(ctx, addr) {
			result.Alive = true
		}
	}
	return result
}

func tcpOpen(ctx context.Context, addr netip.Addr, port int, timeout time.Duration) bool {
	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(addr.String(), strconv.Itoa(port)))
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// icmpAlive runs the system ping command. It returns false on any error,
// including a missing binary or one the user may not run, so a scan without
// TCP answers still finishes.
func icmpAlive(ctx context.Context, addr netip.Addr) bool {
	path, err := exec.LookPath("ping")
	if err != nil {
		return false
	}

	args := []string{"-c", "1", "-W", "1"}
	if runtime.GOOS == "windows" {
		args = []string{"-n", "1", "-w", "1000"}
	}
	args = append(args, addr.String())

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run() == nil
}

// ParseARPTable reads a Linux /proc/net/arp table. It ignores the header,
// malformed rows, incomplete entries and zero MAC addresses.
func ParseARPTable(r io.Reader) (map[netip.Addr]string, error) {
	table := make(map[netip.Addr]string)
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "IP address") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		addr, err := netip.ParseAddr(fields[0])
		if err != nil || !addr.Is4() {
			continue
		}
		if fields[2] == "0x0" {
			continue
		}
		mac, err := net.ParseMAC(fields[3])
		if err != nil || isZeroMAC(mac) {
			continue
		}
		table[addr] = strings.ToLower(mac.String())
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return table, nil
}

// readARPTable loads the ARP table and silently ignores platforms or
// permission errors that leave MAC addresses unknown.
func readARPTable(path string) map[netip.Addr]string {
	if path == "" {
		path = arpTablePath
	}
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()

	table, err := ParseARPTable(file)
	if err != nil {
		return nil
	}
	return table
}

func isZeroMAC(mac net.HardwareAddr) bool {
	for _, b := range mac {
		if b != 0 {
			return false
		}
	}
	return true
}

// LookupMAC returns the MAC address the OS has learned for addr, or "" when
// the table has no entry. It reads the platform ARP table and never sends raw
// ARP requests.
func LookupMAC(addr netip.Addr) string {
	return LookupMACFrom(arpTablePath, addr)
}

// LookupMACFrom reads a specific ARP table file, for tests.
func LookupMACFrom(path string, addr netip.Addr) string {
	return readARPTable(path)[addr]
}

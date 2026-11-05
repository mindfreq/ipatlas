// Package ipam stores saved IPv4 networks and their IP assignments in a local
// SQLite database. It has no UI dependencies.
package ipam

import (
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"ipatlas/internal/netcalc"
)

var (
	// ErrNetworkNotFound reports an operation on a network ID that does not
	// exist.
	ErrNetworkNotFound = errors.New("network not found")
	// ErrAddressNotInNetwork reports an address outside a network's usable
	// range.
	ErrAddressNotInNetwork = errors.New("address is not usable in this network")
	// ErrInvalidLimit reports an address-page size of zero or less.
	ErrInvalidLimit = errors.New("invalid page size")
	// ErrOffsetOutOfRange reports an address-page offset past the last usable
	// address.
	ErrOffsetOutOfRange = errors.New("page offset out of range")
	// ErrClosed reports use of a store that was closed.
	ErrClosed = errors.New("store is closed")
)

// MaxAddressPage is the largest page Addresses returns at once.
const MaxAddressPage = 1024

// Network is a saved IPv4 network.
type Network struct {
	ID        int64
	Name      string
	CIDR      string
	Subnet    netcalc.IPv4Subnet
	CreatedAt time.Time
}

// AddressState says whether an address is assigned.
type AddressState uint8

const (
	// Free is an address with no assignment.
	Free AddressState = iota
	// Used is an address with an assignment.
	Used
)

// String returns the label shown to users.
func (s AddressState) String() string {
	if s == Used {
		return "used"
	}
	return "free"
}

// AddressEntry is one usable address in a saved network.
type AddressEntry struct {
	Addr   netip.Addr
	Offset uint64
	State  AddressState
	Label  string
	Owner  string
}

// Assignment is one labelled address.
type Assignment struct {
	NetworkID int64
	Addr      netip.Addr
	Offset    uint64
	Label     string
	Owner     string
	UpdatedAt time.Time
}

// Stats counts a network's usable addresses.
type Stats struct {
	Total uint64
	Used  uint64
	Free  uint64
}

// Store is an open IPAM database.
type Store struct {
	db *sql.DB
}

// migrations are applied in order; the schema version is the count applied.
var migrations = []string{
	`CREATE TABLE networks (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		cidr TEXT NOT NULL UNIQUE,
		name TEXT NOT NULL,
		network TEXT NOT NULL,
		prefix INTEGER NOT NULL,
		broadcast TEXT NOT NULL,
		first TEXT NOT NULL,
		last TEXT NOT NULL,
		usable INTEGER NOT NULL,
		created_at INTEGER NOT NULL
	)`,
	`CREATE TABLE assignments (
		network_id INTEGER NOT NULL REFERENCES networks(id) ON DELETE CASCADE,
		ip INTEGER NOT NULL,
		address TEXT NOT NULL,
		label TEXT NOT NULL DEFAULT '',
		owner TEXT NOT NULL DEFAULT '',
		updated_at INTEGER NOT NULL,
		PRIMARY KEY (network_id, ip)
	)`,
}

// Open opens or creates the database at path, creating its parent directory,
// and applies pending migrations.
func Open(path string) (*Store, error) {
	if path == "" {
		return nil, fmt.Errorf("ipam: empty database path")
	}
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, fmt.Errorf("ipam: create data directory: %w", err)
		}
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("ipam: open: %w", err)
	}
	// SQLite serializes writers anyway; one connection avoids SQLITE_BUSY
	// between pooled connections and keeps in-memory databases coherent.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	s := &Store{db: db}
	if _, err := db.Exec(`PRAGMA foreign_keys = ON; PRAGMA busy_timeout = 5000;`); err != nil {
		db.Close()
		return nil, fmt.Errorf("ipam: configure: %w", err)
	}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// migrate applies every migration the database has not seen and records the
// resulting schema version.
func (s *Store) migrate() error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("ipam: begin migration: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at INTEGER NOT NULL
	)`); err != nil {
		return fmt.Errorf("ipam: create migration table: %w", err)
	}

	var version int
	row := tx.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`)
	if err := row.Scan(&version); err != nil {
		return fmt.Errorf("ipam: read schema version: %w", err)
	}
	if version > len(migrations) {
		return fmt.Errorf("ipam: database schema %d is newer than this app", version)
	}

	for i := version; i < len(migrations); i++ {
		if _, err := tx.Exec(migrations[i]); err != nil {
			return fmt.Errorf("ipam: apply migration %d: %w", i+1, err)
		}
		if _, err := tx.Exec(
			`INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`,
			i+1, time.Now().Unix(),
		); err != nil {
			return fmt.Errorf("ipam: record migration %d: %w", i+1, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("ipam: commit migration: %w", err)
	}
	return nil
}

// Close closes the database.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	err := s.db.Close()
	s.db = nil
	return err
}

func (s *Store) check() error {
	if s == nil || s.db == nil {
		return ErrClosed
	}
	return nil
}

// SaveNetwork parses cidr, normalizes it and saves it under name. Saving an
// existing CIDR updates its name and returns the existing network.
func (s *Store) SaveNetwork(cidr, name string) (Network, error) {
	if err := s.check(); err != nil {
		return Network{}, err
	}
	subnet, err := netcalc.ParseIPv4CIDR(cidr)
	if err != nil {
		return Network{}, fmt.Errorf("ipam: save network: %w", err)
	}
	normalized := subnet.Network.String() + "/" + strconv.Itoa(subnet.Prefix)
	name = strings.TrimSpace(name)
	if name == "" {
		name = normalized
	}

	now := time.Now()
	_, err = s.db.Exec(`
		INSERT INTO networks
			(cidr, name, network, prefix, broadcast, first, last, usable, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(cidr) DO UPDATE SET name = excluded.name`,
		normalized, name, subnet.Network.String(), subnet.Prefix,
		subnet.Broadcast.String(), subnet.First.String(), subnet.Last.String(),
		subnet.Usable, now.Unix(),
	)
	if err != nil {
		return Network{}, fmt.Errorf("ipam: save network: %w", err)
	}

	return s.Network(normalized)
}

// RenameNetwork updates the name of the saved network with the given ID. An
// empty name falls back to the network's CIDR, as SaveNetwork does.
func (s *Store) RenameNetwork(id int64, name string) (Network, error) {
	if err := s.check(); err != nil {
		return Network{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		n, err := s.Network(id)
		if err != nil {
			return Network{}, err
		}
		name = n.CIDR
	}
	result, err := s.db.Exec(`UPDATE networks SET name = ? WHERE id = ?`, name, id)
	if err != nil {
		return Network{}, fmt.Errorf("ipam: rename network: %w", err)
	}
	if rows, err := result.RowsAffected(); err == nil && rows == 0 {
		return Network{}, fmt.Errorf("ipam: rename network: %w", ErrNetworkNotFound)
	}
	return s.Network(id)
}

// Network returns the saved network with the given ID or normalized CIDR.
func (s *Store) Network(idOrCIDR any) (Network, error) {
	if err := s.check(); err != nil {
		return Network{}, err
	}
	query := `SELECT id, cidr, name, network, prefix, broadcast, first, last, usable, created_at
		FROM networks WHERE id = ?`
	var arg any
	if cidr, ok := idOrCIDR.(string); ok {
		query = `SELECT id, cidr, name, network, prefix, broadcast, first, last, usable, created_at
			FROM networks WHERE cidr = ?`
		arg = cidr
	} else {
		arg = idOrCIDR
	}

	var (
		n         Network
		cidr      string
		network   string
		broadcast string
		first     string
		last      string
		usable    uint64
		created   int64
	)
	err := s.db.QueryRow(query, arg).Scan(
		&n.ID, &cidr, &n.Name, &network, &n.Subnet.Prefix, &broadcast,
		&first, &last, &usable, &created,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Network{}, fmt.Errorf("ipam: %w", ErrNetworkNotFound)
	}
	if err != nil {
		return Network{}, fmt.Errorf("ipam: read network: %w", err)
	}

	n.CIDR = cidr
	n.Subnet.Network = netip.MustParseAddr(network)
	n.Subnet.Broadcast = netip.MustParseAddr(broadcast)
	n.Subnet.First = netip.MustParseAddr(first)
	n.Subnet.Last = netip.MustParseAddr(last)
	n.Subnet.Usable = usable
	n.CreatedAt = time.Unix(created, 0)
	return n, nil
}

// Networks returns every saved network, newest first.
func (s *Store) Networks() ([]Network, error) {
	if err := s.check(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`
		SELECT id, cidr, name, network, prefix, broadcast, first, last, usable, created_at
		FROM networks ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return nil, fmt.Errorf("ipam: list networks: %w", err)
	}
	defer rows.Close()

	var networks []Network
	for rows.Next() {
		var (
			n         Network
			cidr      string
			network   string
			broadcast string
			first     string
			last      string
			usable    uint64
			created   int64
		)
		if err := rows.Scan(
			&n.ID, &cidr, &n.Name, &network, &n.Subnet.Prefix, &broadcast,
			&first, &last, &usable, &created,
		); err != nil {
			return nil, fmt.Errorf("ipam: read network: %w", err)
		}
		n.CIDR = cidr
		n.Subnet.Network = netip.MustParseAddr(network)
		n.Subnet.Broadcast = netip.MustParseAddr(broadcast)
		n.Subnet.First = netip.MustParseAddr(first)
		n.Subnet.Last = netip.MustParseAddr(last)
		n.Subnet.Usable = usable
		n.CreatedAt = time.Unix(created, 0)
		networks = append(networks, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ipam: list networks: %w", err)
	}
	return networks, nil
}

// DeleteNetwork removes a network and its assignments.
func (s *Store) DeleteNetwork(id int64) error {
	if err := s.check(); err != nil {
		return err
	}
	result, err := s.db.Exec(`DELETE FROM networks WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("ipam: delete network: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("ipam: delete network: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("ipam: %w", ErrNetworkNotFound)
	}
	return nil
}

// Assign labels and owns addr in a network. It creates or replaces the
// assignment.
func (s *Store) Assign(networkID int64, addr netip.Addr, label, owner string) error {
	if err := s.check(); err != nil {
		return err
	}
	n, err := s.Network(networkID)
	if err != nil {
		return err
	}
	offset, ok := usableOffset(n.Subnet, addr)
	if !ok {
		return fmt.Errorf("ipam: %s: %w", addr, ErrAddressNotInNetwork)
	}

	_, err = s.db.Exec(`
		INSERT INTO assignments (network_id, ip, address, label, owner, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(network_id, ip) DO UPDATE SET
			address = excluded.address,
			label = excluded.label,
			owner = excluded.owner,
			updated_at = excluded.updated_at`,
		networkID, offset, addr.String(), strings.TrimSpace(label), strings.TrimSpace(owner), time.Now().Unix(),
	)
	if err != nil {
		return fmt.Errorf("ipam: assign: %w", err)
	}
	return nil
}

// Unassign removes addr's assignment. It is not an error when addr has none.
func (s *Store) Unassign(networkID int64, addr netip.Addr) error {
	if err := s.check(); err != nil {
		return err
	}
	n, err := s.Network(networkID)
	if err != nil {
		return err
	}
	offset, ok := usableOffset(n.Subnet, addr)
	if !ok {
		return fmt.Errorf("ipam: %s: %w", addr, ErrAddressNotInNetwork)
	}
	if _, err := s.db.Exec(
		`DELETE FROM assignments WHERE network_id = ? AND ip = ?`,
		networkID, offset,
	); err != nil {
		return fmt.Errorf("ipam: unassign: %w", err)
	}
	return nil
}

// Assignment returns addr's assignment when it has one.
func (s *Store) Assignment(networkID int64, addr netip.Addr) (Assignment, bool, error) {
	if err := s.check(); err != nil {
		return Assignment{}, false, err
	}
	n, err := s.Network(networkID)
	if err != nil {
		return Assignment{}, false, err
	}
	offset, ok := usableOffset(n.Subnet, addr)
	if !ok {
		return Assignment{}, false, fmt.Errorf("ipam: %s: %w", addr, ErrAddressNotInNetwork)
	}

	var (
		a       Assignment
		address string
		updated int64
	)
	err = s.db.QueryRow(`
		SELECT network_id, ip, address, label, owner, updated_at
		FROM assignments WHERE network_id = ? AND ip = ?`,
		networkID, offset,
	).Scan(&a.NetworkID, &a.Offset, &address, &a.Label, &a.Owner, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return Assignment{}, false, nil
	}
	if err != nil {
		return Assignment{}, false, fmt.Errorf("ipam: read assignment: %w", err)
	}
	a.Addr = netip.MustParseAddr(address)
	a.UpdatedAt = time.Unix(updated, 0)
	return a, true, nil
}

// Addresses returns up to limit usable addresses starting at offset, with
// their assignment state. limit is capped at MaxAddressPage.
func (s *Store) Addresses(networkID int64, offset uint64, limit int) ([]AddressEntry, error) {
	if err := s.check(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		return nil, fmt.Errorf("ipam: %w: %d", ErrInvalidLimit, limit)
	}
	if limit > MaxAddressPage {
		limit = MaxAddressPage
	}
	n, err := s.Network(networkID)
	if err != nil {
		return nil, err
	}
	if offset >= n.Subnet.Usable {
		return nil, fmt.Errorf("ipam: %w: offset %d", ErrOffsetOutOfRange, offset)
	}
	count := uint64(limit)
	if remaining := n.Subnet.Usable - offset; count > remaining {
		count = remaining
	}

	used := make(map[uint64]struct {
		label string
		owner string
	}, count)
	rows, err := s.db.Query(`
		SELECT ip, label, owner FROM assignments
		WHERE network_id = ? AND ip BETWEEN ? AND ?`,
		networkID, offset, offset+count-1,
	)
	if err != nil {
		return nil, fmt.Errorf("ipam: read assignments: %w", err)
	}
	for rows.Next() {
		var (
			ip    uint64
			label string
			owner string
		)
		if err := rows.Scan(&ip, &label, &owner); err != nil {
			rows.Close()
			return nil, fmt.Errorf("ipam: read assignment: %w", err)
		}
		used[ip] = struct {
			label string
			owner string
		}{label, owner}
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("ipam: read assignments: %w", err)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ipam: read assignments: %w", err)
	}

	first := addrToUint32(n.Subnet.First)
	entries := make([]AddressEntry, 0, int(count))
	for i := uint64(0); i < count; i++ {
		entry := AddressEntry{
			Addr:   addrFromUint32(first + uint32(offset+i)),
			Offset: offset + i,
			State:  Free,
		}
		if a, ok := used[offset+i]; ok {
			entry.State = Used
			entry.Label = a.label
			entry.Owner = a.owner
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// Stats returns the used and free counts of a network.
func (s *Store) Stats(networkID int64) (Stats, error) {
	if err := s.check(); err != nil {
		return Stats{}, err
	}
	n, err := s.Network(networkID)
	if err != nil {
		return Stats{}, err
	}
	var used uint64
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM assignments WHERE network_id = ?`,
		networkID,
	).Scan(&used); err != nil {
		return Stats{}, fmt.Errorf("ipam: count assignments: %w", err)
	}
	if used > n.Subnet.Usable {
		used = n.Subnet.Usable
	}
	return Stats{Total: n.Subnet.Usable, Used: used, Free: n.Subnet.Usable - used}, nil
}

// usableOffset maps addr to its index among a subnet's usable addresses.
func usableOffset(subnet netcalc.IPv4Subnet, addr netip.Addr) (uint64, bool) {
	if !addr.Is4() || !subnet.First.IsValid() || !subnet.Last.IsValid() {
		return 0, false
	}
	ip := addrToUint32(addr)
	first := addrToUint32(subnet.First)
	last := addrToUint32(subnet.Last)
	if ip < first || ip > last {
		return 0, false
	}
	return uint64(ip - first), true
}

func addrToUint32(addr netip.Addr) uint32 {
	b := addr.As4()
	return binary.BigEndian.Uint32(b[:])
}

func addrFromUint32(v uint32) netip.Addr {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], v)
	return netip.AddrFrom4(b)
}

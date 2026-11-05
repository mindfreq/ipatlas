package netcalc

import (
	"errors"
	"fmt"
	"math/big"
	"net/netip"
	"strings"
)

var (
	// ErrInvalidIPv6 reports input that is not a valid IPv6 address.
	ErrInvalidIPv6 = errors.New("invalid IPv6 address")
	// ErrNotIPv6 reports a valid address that is not IPv6.
	ErrNotIPv6 = errors.New("not an IPv6 address")
	// ErrInvalidIPv6Split reports child bits outside the parent prefix.
	ErrInvalidIPv6Split = errors.New("invalid IPv6 split")
	// ErrIPv6SplitTooLarge reports a split with more than MaxIPv6SplitBits
	// extra bits.
	ErrIPv6SplitTooLarge = errors.New("IPv6 split has too many subnets")
	// ErrIPv6PageSize reports a page size of zero or less.
	ErrIPv6PageSize = errors.New("invalid IPv6 page size")
	// ErrIPv6PageOutOfRange reports an offset past the last subnet.
	ErrIPv6PageOutOfRange = errors.New("IPv6 page out of range")
)

// MaxIPv6SplitBits bounds the number of extra bits a split may add, keeping
// the total subnet count in a uint64 (at most 2^32) and paging practical.
const MaxIPv6SplitBits = 32

// MaxIPv6PageSize is the largest page Page returns at once.
const MaxIPv6PageSize = 1024

// IPv6Kind classifies an IPv6 address by its role.
type IPv6Kind int

const (
	IPv6Other IPv6Kind = iota
	IPv6Unspecified
	IPv6Loopback
	IPv6LinkLocal
	IPv6UniqueLocal
	IPv6Multicast
	IPv6GlobalUnicast
	IPv6IPv4Mapped
)

// String returns the label shown to users.
func (k IPv6Kind) String() string {
	switch k {
	case IPv6Unspecified:
		return "unspecified"
	case IPv6Loopback:
		return "loopback"
	case IPv6LinkLocal:
		return "link-local"
	case IPv6UniqueLocal:
		return "unique-local"
	case IPv6Multicast:
		return "multicast"
	case IPv6GlobalUnicast:
		return "global unicast"
	case IPv6IPv4Mapped:
		return "IPv4-mapped"
	default:
		return "other"
	}
}

// IPv6Info is an IPv6 address in compressed and expanded form, with its kind.
type IPv6Info struct {
	Address    netip.Addr
	Compressed string
	Expanded   string
	Kind       IPv6Kind
}

// ParseIPv6Address parses an IPv6 address, including IPv4-mapped and zoned
// addresses. The compressed form keeps a zone; the expanded form drops it.
func ParseIPv6Address(s string) (IPv6Info, error) {
	addr, err := netip.ParseAddr(s)
	if err != nil {
		return IPv6Info{}, fmt.Errorf("netcalc: %w: %q", ErrInvalidIPv6, s)
	}
	if !addr.Is6() {
		return IPv6Info{}, fmt.Errorf("netcalc: %w: %q", ErrNotIPv6, s)
	}

	return IPv6Info{
		Address:    addr,
		Compressed: CompressIPv6(addr),
		Expanded:   ExpandIPv6(addr),
		Kind:       ClassifyIPv6(addr),
	}, nil
}

// CompressIPv6 returns the canonical compressed form of an IPv6 address, or
// "" for a non-IPv6 address.
func CompressIPv6(addr netip.Addr) string {
	if !addr.Is6() {
		return ""
	}
	return addr.String()
}

// ExpandIPv6 returns all eight groups of an IPv6 address in lowercase hex,
// without a zone, or "" for a non-IPv6 address.
func ExpandIPv6(addr netip.Addr) string {
	if !addr.Is6() {
		return ""
	}

	const hex = "0123456789abcdef"
	bytes := addr.As16()
	var b strings.Builder
	b.Grow(39)
	for i := 0; i < 8; i++ {
		if i > 0 {
			b.WriteByte(':')
		}
		hi, lo := bytes[2*i], bytes[2*i+1]
		b.WriteByte(hex[hi>>4])
		b.WriteByte(hex[hi&0x0f])
		b.WriteByte(hex[lo>>4])
		b.WriteByte(hex[lo&0x0f])
	}
	return b.String()
}

// ClassifyIPv6 reports what kind of IPv6 address addr is. An IPv4 address is
// IPv6Other.
func ClassifyIPv6(addr netip.Addr) IPv6Kind {
	if !addr.Is6() {
		return IPv6Other
	}

	switch {
	case addr.Is4In6():
		return IPv6IPv4Mapped
	case addr.IsUnspecified():
		return IPv6Unspecified
	case addr.IsLoopback():
		return IPv6Loopback
	case addr.IsLinkLocalUnicast():
		return IPv6LinkLocal
	case addr.IsMulticast():
		return IPv6Multicast
	case addr.IsPrivate():
		return IPv6UniqueLocal
	case addr.IsGlobalUnicast():
		return IPv6GlobalUnicast
	default:
		return IPv6Other
	}
}

// IPv6Split describes a parent prefix split into equally sized child prefixes.
type IPv6Split struct {
	parent    netip.Prefix
	childBits int
}

// NewIPv6Split validates a parent prefix and child prefix length. The split
// may add at most MaxIPv6SplitBits bits.
func NewIPv6Split(parentCIDR string, childBits int) (IPv6Split, error) {
	parent, err := netip.ParsePrefix(parentCIDR)
	if err != nil {
		return IPv6Split{}, fmt.Errorf("netcalc: %w: %q", ErrInvalidCIDR, parentCIDR)
	}
	if !parent.Addr().Is6() {
		return IPv6Split{}, fmt.Errorf("netcalc: %w: %q", ErrNotIPv6, parentCIDR)
	}
	if childBits < parent.Bits() || childBits > 128 {
		return IPv6Split{}, fmt.Errorf(
			"netcalc: %w: /%d from /%d", ErrInvalidIPv6Split, childBits, parent.Bits(),
		)
	}
	if childBits-parent.Bits() > MaxIPv6SplitBits {
		return IPv6Split{}, fmt.Errorf(
			"netcalc: %w: /%d from /%d", ErrIPv6SplitTooLarge, childBits, parent.Bits(),
		)
	}

	return IPv6Split{parent: parent.Masked(), childBits: childBits}, nil
}

// Parent returns the normalized parent prefix.
func (s IPv6Split) Parent() netip.Prefix {
	return s.parent
}

// ChildBits returns the length of each child prefix.
func (s IPv6Split) ChildBits() int {
	return s.childBits
}

// Total returns the number of child prefixes.
func (s IPv6Split) Total() uint64 {
	if !s.parent.IsValid() || s.childBits < s.parent.Bits() {
		return 0
	}
	return uint64(1) << uint(s.childBits-s.parent.Bits())
}

// IPv6Page is one page of child prefixes.
type IPv6Page struct {
	Prefixes []netip.Prefix
	Offset   uint64
	Total    uint64
}

// End returns the offset just past the last prefix in the page.
func (p IPv6Page) End() uint64 {
	return p.Offset + uint64(len(p.Prefixes))
}

// HasPrev reports whether an earlier page exists.
func (p IPv6Page) HasPrev() bool {
	return p.Offset > 0
}

// HasNext reports whether a later page exists.
func (p IPv6Page) HasNext() bool {
	return p.End() < p.Total
}

// Page returns at most limit child prefixes starting at offset. The result is
// bounded by MaxIPv6PageSize and never materializes the whole split.
func (s IPv6Split) Page(offset uint64, limit int) (IPv6Page, error) {
	if !s.parent.IsValid() || s.childBits < s.parent.Bits() || s.childBits > 128 {
		return IPv6Page{}, fmt.Errorf("netcalc: %w", ErrInvalidIPv6Split)
	}
	if limit <= 0 {
		return IPv6Page{}, fmt.Errorf("netcalc: %w: %d", ErrIPv6PageSize, limit)
	}

	total := s.Total()
	if offset >= total {
		return IPv6Page{}, fmt.Errorf("netcalc: %w: offset %d", ErrIPv6PageOutOfRange, offset)
	}
	if limit > MaxIPv6PageSize {
		limit = MaxIPv6PageSize
	}
	count := uint64(limit)
	if remaining := total - offset; count > remaining {
		count = remaining
	}

	base := new(big.Int).SetBytes(s.parent.Addr().AsSlice())
	shift := uint(128 - s.childBits)
	prefixes := make([]netip.Prefix, 0, int(count))
	for i := uint64(0); i < count; i++ {
		index := new(big.Int).SetUint64(offset + i)
		index.Lsh(index, shift)
		addr := new(big.Int).Add(base, index)
		var b [16]byte
		addr.FillBytes(b[:])
		prefixes = append(prefixes, netip.PrefixFrom(netip.AddrFrom16(b), s.childBits))
	}

	return IPv6Page{Prefixes: prefixes, Offset: offset, Total: total}, nil
}

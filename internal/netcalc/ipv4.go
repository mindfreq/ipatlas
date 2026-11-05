// Package netcalc provides the IP math behind IPatlas. It has no UI or MyGo
// dependencies and uses net/netip for all address handling.
package netcalc

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net/netip"
)

var (
	// ErrInvalidCIDR reports input that is not valid CIDR notation.
	ErrInvalidCIDR = errors.New("invalid CIDR notation")
	// ErrNotIPv4 reports valid CIDR notation that does not describe an IPv4
	// network.
	ErrNotIPv4 = errors.New("not an IPv4 CIDR")
)

// IPv4Subnet is the address plan for one IPv4 prefix.
//
// Address is the address as entered, before host bits are cleared. For a
// /31 the two addresses are both usable and First and Last are the network
// and broadcast. A /32 has one usable address. Gateway is the first usable
// address; on /31 and /32 it is that same address because there is no
// separate router address.
type IPv4Subnet struct {
	Address   netip.Addr
	Network   netip.Addr
	Prefix    int
	Mask      netip.Addr
	Wildcard  netip.Addr
	Broadcast netip.Addr
	First     netip.Addr
	Last      netip.Addr
	Gateway   netip.Addr
	Usable    uint64
}

// ParseIPv4CIDR parses an IPv4 CIDR such as "192.168.1.0/24" and returns the
// subnet it describes. Host bits in the input are cleared in Network, so
// "192.168.1.5/24" produces the 192.168.1.0/24 plan while Address keeps
// 192.168.1.5.
func ParseIPv4CIDR(s string) (IPv4Subnet, error) {
	p, err := netip.ParsePrefix(s)
	if err != nil {
		return IPv4Subnet{}, fmt.Errorf("netcalc: %w: %q", ErrInvalidCIDR, s)
	}
	if !p.Addr().Is4() {
		return IPv4Subnet{}, fmt.Errorf("netcalc: %w: %q", ErrNotIPv4, s)
	}

	bits := p.Bits()
	network := p.Masked().Addr()
	ip := addrToUint32(network)
	mask := prefixMask(bits)
	wildcard := ^mask
	broadcast := ip | wildcard

	total := uint64(1) << (32 - bits)
	var first, last netip.Addr
	var usable uint64
	switch bits {
	case 32:
		first, last = network, network
		usable = 1
	case 31:
		first, last = network, addrFromUint32(broadcast)
		usable = 2
	default:
		first = addrFromUint32(ip + 1)
		last = addrFromUint32(broadcast - 1)
		usable = total - 2
	}

	return IPv4Subnet{
		Address:   p.Addr(),
		Network:   network,
		Prefix:    bits,
		Mask:      addrFromUint32(mask),
		Wildcard:  addrFromUint32(wildcard),
		Broadcast: addrFromUint32(broadcast),
		First:     first,
		Last:      last,
		Gateway:   first,
		Usable:    usable,
	}, nil
}

func prefixMask(bits int) uint32 {
	if bits == 0 {
		return 0
	}
	return ^uint32(0) << (32 - bits)
}

func addrToUint32(a netip.Addr) uint32 {
	b := a.As4()
	return binary.BigEndian.Uint32(b[:])
}

func addrFromUint32(v uint32) netip.Addr {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], v)
	return netip.AddrFrom4(b)
}

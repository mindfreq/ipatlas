package netcalc

import (
	"errors"
	"fmt"
	"math/bits"
	"sort"
)

var (
	// ErrNoVLSMRequests reports an allocation with no departments.
	ErrNoVLSMRequests = errors.New("no VLSM requests")
	// ErrInvalidHosts reports a department with no usable host request.
	ErrInvalidHosts = errors.New("invalid host count")
	// ErrVLSMNoSpace reports departments that do not fit in the parent.
	ErrVLSMNoSpace = errors.New("not enough space in the parent network")
)

// maxIPv4Usable is the largest usable-host count in an IPv4 network, from a
// /0 allocation.
const maxIPv4Usable = uint64(1<<32) - 2

// VLSMRequest is one department to place in a parent network.
type VLSMRequest struct {
	Name  string
	Hosts uint64
}

// VLSMAllocation is one department's subnet in a VLSM plan.
type VLSMAllocation struct {
	Name      string
	Requested uint64
	Subnet    IPv4Subnet
}

// VLSMPlan is a parent network and its non-overlapping allocations, largest
// first.
type VLSMPlan struct {
	Parent      IPv4Subnet
	Allocations []VLSMAllocation
}

// AllocateVLSM splits parentCIDR into one subnet per request, largest first.
// Each request is rounded up to a power-of-two block after reserving the
// network and broadcast addresses, so every department gets at least a /30.
// Blocks are aligned to their size and never overlap. The allocation order is
// the order of the returned allocations; equal-sized requests keep their input
// order.
func AllocateVLSM(parentCIDR string, requests []VLSMRequest) (VLSMPlan, error) {
	parent, err := ParseIPv4CIDR(parentCIDR)
	if err != nil {
		return VLSMPlan{}, fmt.Errorf("netcalc: vlsm parent: %w", err)
	}
	if len(requests) == 0 {
		return VLSMPlan{}, fmt.Errorf("netcalc: %w", ErrNoVLSMRequests)
	}

	type planned struct {
		request   VLSMRequest
		blockSize uint64
		prefix    int
	}
	plans := make([]planned, 0, len(requests))
	for _, req := range requests {
		if req.Hosts == 0 {
			return VLSMPlan{}, fmt.Errorf("netcalc: %w: %q has 0 hosts", ErrInvalidHosts, req.Name)
		}
		if req.Hosts > maxIPv4Usable {
			return VLSMPlan{}, fmt.Errorf("netcalc: %w: %q needs %d hosts", ErrVLSMNoSpace, req.Name, req.Hosts)
		}

		blockSize := nextPowerOfTwo(req.Hosts + 2)
		plans = append(plans, planned{
			request:   req,
			blockSize: blockSize,
			prefix:    32 - (bits.Len64(blockSize) - 1),
		})
	}

	sort.SliceStable(plans, func(i, j int) bool {
		return plans[i].blockSize > plans[j].blockSize
	})

	cursor := uint64(addrToUint32(parent.Network))
	end := uint64(addrToUint32(parent.Broadcast))
	allocations := make([]VLSMAllocation, 0, len(plans))
	for _, plan := range plans {
		aligned := (cursor + plan.blockSize - 1) &^ (plan.blockSize - 1)
		if aligned+plan.blockSize-1 > end {
			return VLSMPlan{}, fmt.Errorf(
				"netcalc: %w: %q needs a /%d block",
				ErrVLSMNoSpace, plan.request.Name, plan.prefix,
			)
		}

		allocations = append(allocations, VLSMAllocation{
			Name:      plan.request.Name,
			Requested: plan.request.Hosts,
			Subnet:    subnetFrom(addrFromUint32(uint32(aligned)), plan.prefix),
		})
		cursor = aligned + plan.blockSize
	}

	return VLSMPlan{Parent: parent, Allocations: allocations}, nil
}

func nextPowerOfTwo(n uint64) uint64 {
	if n <= 1 {
		return 1
	}
	return uint64(1) << bits.Len64(n-1)
}

package confine

import (
	"context"
	"net/netip"
	"sync"

	"golang.org/x/xerrors"
)

const (
	// DefaultNetNSPool is divided into point-to-point /30 networks, one per
	// confined network namespace.
	DefaultNetNSPool = "100.115.92.0/24"
	netNSPrefixBits  = 30
)

type subnetPrefixLister func(context.Context) ([]netip.Prefix, error)

type subnetLease struct {
	Prefix netip.Prefix
	Host   netip.Addr
	Peer   netip.Addr
}

type subnetAllocator struct {
	mu       sync.Mutex
	leased   map[netip.Prefix]struct{}
	listUsed subnetPrefixLister
}

func newSubnetAllocator(listUsed subnetPrefixLister) *subnetAllocator {
	return &subnetAllocator{
		leased:   make(map[netip.Prefix]struct{}),
		listUsed: listUsed,
	}
}

func (a *subnetAllocator) Allocate(ctx context.Context, pool netip.Prefix) (subnetLease, error) {
	pool = pool.Masked()
	if !pool.Addr().Is4() {
		return subnetLease{}, xerrors.Errorf("netns subnet pool %q is not IPv4", pool)
	}
	if pool.Bits() > netNSPrefixBits {
		return subnetLease{}, xerrors.Errorf("netns subnet pool %q is smaller than /%d", pool, netNSPrefixBits)
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	used, err := a.listUsed(ctx)
	if err != nil {
		return subnetLease{}, xerrors.Errorf("list host network prefixes: %w", err)
	}

	subnetCount := uint64(1) << (netNSPrefixBits - pool.Bits())
	for index := range subnetCount {
		subnetAddr := addIPv4(pool.Addr(), index*4)
		prefix := netip.PrefixFrom(subnetAddr, netNSPrefixBits)
		if a.isLeased(prefix) || overlapsAny(prefix, used) {
			continue
		}

		lease := subnetLease{
			Prefix: prefix,
			Host:   subnetAddr.Next(),
			Peer:   subnetAddr.Next().Next(),
		}
		a.leased[prefix] = struct{}{}
		return lease, nil
	}

	return subnetLease{}, xerrors.Errorf("netns subnet pool %q is exhausted", pool)
}

func (a *subnetAllocator) Release(prefix netip.Prefix) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.leased, prefix.Masked())
}

func (a *subnetAllocator) isLeased(candidate netip.Prefix) bool {
	for leased := range a.leased {
		if prefixesOverlap(candidate, leased) {
			return true
		}
	}
	return false
}

func overlapsAny(candidate netip.Prefix, prefixes []netip.Prefix) bool {
	for _, prefix := range prefixes {
		if prefixesOverlap(candidate, prefix) {
			return true
		}
	}
	return false
}

func prefixesOverlap(left, right netip.Prefix) bool {
	if left.Addr().BitLen() != right.Addr().BitLen() {
		return false
	}
	left = left.Masked()
	right = right.Masked()
	return left.Contains(right.Addr()) || right.Contains(left.Addr())
}

func addIPv4(addr netip.Addr, increment uint64) netip.Addr {
	value := addr.As4()
	current := uint64(value[0])<<24 |
		uint64(value[1])<<16 |
		uint64(value[2])<<8 |
		uint64(value[3])
	current += increment
	return netip.AddrFrom4([4]byte{
		byte(current >> 24),
		byte(current >> 16),
		byte(current >> 8),
		byte(current),
	})
}

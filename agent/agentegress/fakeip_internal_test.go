package agentegress

import (
	"fmt"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFakeIPPool_Deterministic(t *testing.T) {
	t.Parallel()

	a := newFakeIPPool(fakeIPPrefix, fakeIPMaxEntries, fakeIPMaxEntries)
	b := newFakeIPPool(fakeIPPrefix, fakeIPMaxEntries, fakeIPMaxEntries)

	addr := a.Lookup("Example.COM.")
	require.True(t, fakeIPPrefix.Contains(addr), addr)
	require.NotEqual(t, byte(0), addr.As4()[3])
	require.NotEqual(t, byte(255), addr.As4()[3])
	// Names are normalized, so spelling variants share an address.
	require.Equal(t, addr, a.Lookup("example.com"))
	require.Equal(t, 1, a.Len())
	// A fresh pool derives the same address for the same name.
	require.Equal(t, addr, b.Lookup("example.com"))

	name, ok := a.Reverse(addr)
	require.True(t, ok)
	require.Equal(t, "example.com", name)
	_, ok = a.Reverse(netip.MustParseAddr("198.18.0.1"))
	require.False(t, ok, "unassigned address must not reverse")
	require.True(t, a.Contains(netip.MustParseAddr("198.19.255.254")))
	require.False(t, a.Contains(netip.MustParseAddr("198.20.0.1")))
}

func TestFakeIPPool_CollisionProbing(t *testing.T) {
	t.Parallel()

	// A /30 leaves three usable host addresses (.1 through .3; .0 is
	// skipped), so the pool must probe past taken slots and wrap without
	// looping.
	p := newFakeIPPool(netip.MustParsePrefix("198.18.0.0/30"), 3, 3)
	first := p.Lookup("a.test")
	second := p.Lookup("b.test")
	third := p.Lookup("c.test")
	require.NotEqual(t, first, second)
	require.NotEqual(t, second, third)
	require.NotEqual(t, first, third)
	require.Equal(t, 3, p.Len())
	for _, addr := range []netip.Addr{first, second, third} {
		require.NotEqual(t, byte(0), addr.As4()[3], addr)
	}

	// At capacity, the least recently used name is evicted.
	require.Equal(t, first, p.Lookup("a.test"), "touch a.test so b.test is oldest")
	require.Equal(t, third, p.Lookup("c.test"))
	fourth := p.Lookup("d.test")
	require.Equal(t, second, fourth, "d.test reuses the evicted address")
	name, ok := p.Reverse(second)
	require.True(t, ok)
	require.Equal(t, "d.test", name)
	require.Equal(t, 3, p.Len())
	// Re-adding b.test evicts the next oldest (a.test) and reuses its
	// address rather than growing.
	require.Equal(t, first, p.Lookup("b.test"))
	require.Equal(t, 3, p.Len())
	name, ok = p.Reverse(first)
	require.True(t, ok)
	require.Equal(t, "b.test", name)
}

func TestFakeIPPool_ReverseTouchesLRU(t *testing.T) {
	t.Parallel()

	p := newFakeIPPool(fakeIPPrefix, 2, 2)
	first := p.Lookup("first.test")
	second := p.Lookup("second.test")

	name, ok := p.Reverse(first)
	require.True(t, ok)
	require.Equal(t, "first.test", name)
	_ = p.Lookup("third.test")

	_, ok = p.Reverse(first)
	require.True(t, ok, "a connection lookup should keep the mapping recent")
	_, ok = p.Reverse(second)
	require.False(t, ok, "the untouched mapping should be evicted")
}

func TestFakeIPPool_AcquirePinsMapping(t *testing.T) {
	t.Parallel()

	p := newFakeIPPool(fakeIPPrefix, 2, 2)
	first := p.Lookup("first.test")
	second := p.Lookup("second.test")
	name, release, ok := p.Acquire(first)
	require.True(t, ok)
	require.Equal(t, "first.test", name)

	_ = p.Lookup("third.test")
	_, ok = p.Reverse(first)
	require.True(t, ok, "a pinned mapping must not be evicted")
	_, ok = p.Reverse(second)
	require.False(t, ok, "the unpinned mapping should be evicted first")

	fourth := p.Lookup("fourth.test")
	name, ok = p.Reverse(fourth)
	require.True(t, ok)
	require.Equal(t, "fourth.test", name)
	release()
}

func TestFakeIPPool_ExhaustionAndPinnedCap(t *testing.T) {
	t.Parallel()

	p := newFakeIPPool(netip.MustParsePrefix("198.18.0.0/30"), 3, 3)
	for _, name := range []string{"a.test", "b.test", "c.test"} {
		addr, err := p.Allocate(name)
		require.NoError(t, err)
		_, _, ok := p.Acquire(addr)
		require.True(t, ok)
	}
	_, err := p.Allocate("d.test")
	require.ErrorContains(t, err, "pinned entry limit")
	require.Equal(t, 3, p.Len())

	exhausted := newFakeIPPool(netip.MustParsePrefix("198.18.0.0/30"), 4, 4)
	for _, name := range []string{"a.test", "b.test", "c.test"} {
		addr, err := exhausted.Allocate(name)
		require.NoError(t, err)
		_, _, ok := exhausted.Acquire(addr)
		require.True(t, ok)
	}
	exhausted.mu.Lock()
	_, err = exhausted.allocateLocked("d.test")
	exhausted.mu.Unlock()
	require.ErrorContains(t, err, "address space exhausted")
}

func TestFakeIPPool_EvictionBound(t *testing.T) {
	t.Parallel()

	const capacity = 100
	p := newFakeIPPool(fakeIPPrefix, capacity, capacity)
	seen := make(map[netip.Addr]struct{})
	for i := range capacity * 3 {
		addr := p.Lookup(fmt.Sprintf("host-%d.test", i))
		seen[addr] = struct{}{}
		require.LessOrEqual(t, p.Len(), capacity)
	}
	require.Equal(t, capacity, p.Len())
	// The most recent names are still resolvable, the oldest are gone.
	_, ok := p.Reverse(p.Lookup(fmt.Sprintf("host-%d.test", capacity*3-1)))
	require.True(t, ok)
	require.Greater(t, len(seen), capacity, "evicted addresses were reused or fresh ones assigned")
}

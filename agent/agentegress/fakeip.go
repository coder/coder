package agentegress

import (
	"container/list"
	"encoding/binary"
	"hash/fnv"
	"net/netip"
	"strings"
	"sync"
)

const (
	// fakeIPMaxEntries bounds the pool. Beyond it the least recently used
	// name is evicted; a client still holding that address gets a
	// connection to whatever name the address is next assigned to, which
	// the exit node then judges by that name. The 1s TTL keeps this rare.
	fakeIPMaxEntries = 65536
	// fakeIPMaxProbes bounds collision probing so a pathological hash
	// cluster cannot spin.
	fakeIPMaxProbes = 4096
)

// fakeIPPrefix is the benchmarking range (RFC 2544), which is never routed
// on the public internet and is therefore safe to hand out as placeholder
// addresses that the proxy translates back into names.
var fakeIPPrefix = netip.MustParsePrefix("198.18.0.0/15")

// fakeIPPool assigns placeholder IPv4 addresses to hostnames so that every
// flow the workspace opens carries the name the client asked for, not an
// address the proxy would have to reverse-resolve.
type fakeIPPool struct {
	mu     sync.Mutex
	prefix netip.Prefix
	max    int
	byName map[string]*list.Element
	byAddr map[netip.Addr]*list.Element
	// lru holds fakeIPEntry values, most recently used first.
	lru *list.List
}

type fakeIPEntry struct {
	name string
	addr netip.Addr
}

func newFakeIPPool(prefix netip.Prefix, maxEntries int) *fakeIPPool {
	return &fakeIPPool{
		prefix: prefix.Masked(),
		max:    maxEntries,
		byName: make(map[string]*list.Element),
		byAddr: make(map[netip.Addr]*list.Element),
		lru:    list.New(),
	}
}

// normalizeName lowercases a DNS name and strips the trailing dot so that
// the pool, the exempt list and CONNECT targets all agree on spelling.
func normalizeName(name string) string {
	return strings.TrimSuffix(strings.ToLower(name), ".")
}

// Contains reports whether addr falls in the fake range, regardless of
// whether it is currently assigned.
func (p *fakeIPPool) Contains(addr netip.Addr) bool {
	return p.prefix.Contains(addr.Unmap())
}

// Lookup returns the address assigned to name, allocating one on first
// use. The address is derived from a hash of the name so the same name
// maps to the same address across restarts as long as no collision
// intervenes.
func (p *fakeIPPool) Lookup(name string) netip.Addr {
	name = normalizeName(name)
	p.mu.Lock()
	defer p.mu.Unlock()
	if e, ok := p.byName[name]; ok {
		p.lru.MoveToFront(e)
		return entryOf(e).addr
	}
	for p.lru.Len() >= p.max {
		p.removeLocked(p.lru.Back())
	}
	addr := p.allocateLocked(name)
	e := p.lru.PushFront(fakeIPEntry{name: name, addr: addr})
	p.byName[name], p.byAddr[addr] = e, e
	return addr
}

// Reverse returns the name assigned to addr.
func (p *fakeIPPool) Reverse(addr netip.Addr) (string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	e, ok := p.byAddr[addr.Unmap()]
	if !ok {
		return "", false
	}
	p.lru.MoveToFront(e)
	return entryOf(e).name, true
}

// Len returns the number of assigned addresses.
func (p *fakeIPPool) Len() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lru.Len()
}

func entryOf(e *list.Element) fakeIPEntry {
	entry, _ := e.Value.(fakeIPEntry)
	return entry
}

func (p *fakeIPPool) removeLocked(e *list.Element) {
	entry := entryOf(e)
	p.lru.Remove(e)
	delete(p.byName, entry.name)
	delete(p.byAddr, entry.addr)
}

// allocateLocked hashes name to an offset and probes linearly until it
// finds a free host address. Addresses whose last octet is 0 or 255 are
// skipped because some clients treat them as network and broadcast
// addresses and refuse to connect.
func (p *fakeIPPool) allocateLocked(name string) netip.Addr {
	h := fnv.New32a()
	_, _ = h.Write([]byte(name))
	first := p.prefix.Addr().As4()
	base := binary.BigEndian.Uint32(first[:])
	size := uint32(1) << (32 - p.prefix.Bits())
	offset := h.Sum32() % size
	var fallback netip.Addr
	for range fakeIPMaxProbes {
		var raw [4]byte
		binary.BigEndian.PutUint32(raw[:], base+offset)
		offset = (offset + 1) % size
		if raw[3] == 0 || raw[3] == 255 {
			continue
		}
		addr := netip.AddrFrom4(raw)
		if _, taken := p.byAddr[addr]; !taken {
			return addr
		}
		if !fallback.IsValid() {
			fallback = addr
		}
	}
	// Every probed address is taken, which can only happen when the pool is
	// nearly full relative to the probe window. Evict the holder of the
	// first candidate and reuse it rather than fail the query.
	if e, ok := p.byAddr[fallback]; ok {
		p.removeLocked(e)
	}
	return fallback
}

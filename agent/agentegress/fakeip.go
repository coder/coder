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
	// size is the number of host addresses in prefix.
	size   uint32
	max    int
	byName map[string]*fakeIPEntry
	byAddr map[netip.Addr]*fakeIPEntry
	// lru orders entries most recently used first.
	lru *list.List
}

type fakeIPEntry struct {
	name string
	addr netip.Addr
	elem *list.Element
}

func newFakeIPPool(prefix netip.Prefix, maxEntries int) *fakeIPPool {
	prefix = prefix.Masked()
	return &fakeIPPool{
		prefix: prefix,
		size:   uint32(1) << (32 - prefix.Bits()),
		max:    maxEntries,
		byName: make(map[string]*fakeIPEntry),
		byAddr: make(map[netip.Addr]*fakeIPEntry),
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
		p.lru.MoveToFront(e.elem)
		return e.addr
	}
	for p.lru.Len() >= p.max {
		p.evictLocked()
	}
	addr := p.allocateLocked(name)
	e := &fakeIPEntry{name: name, addr: addr}
	e.elem = p.lru.PushFront(e)
	p.byName[name] = e
	p.byAddr[addr] = e
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
	p.lru.MoveToFront(e.elem)
	return e.name, true
}

// Len returns the number of assigned addresses.
func (p *fakeIPPool) Len() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lru.Len()
}

func (p *fakeIPPool) evictLocked() {
	back := p.lru.Back()
	if back == nil {
		return
	}
	e, _ := back.Value.(*fakeIPEntry)
	p.lru.Remove(back)
	delete(p.byName, e.name)
	delete(p.byAddr, e.addr)
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
	offset := h.Sum32() % p.size
	var fallback netip.Addr
	for range fakeIPMaxProbes {
		var raw [4]byte
		binary.BigEndian.PutUint32(raw[:], base+offset)
		addr := netip.AddrFrom4(raw)
		offset = (offset + 1) % p.size
		if raw[3] == 0 || raw[3] == 255 {
			continue
		}
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
		p.lru.Remove(e.elem)
		delete(p.byName, e.name)
		delete(p.byAddr, e.addr)
	}
	return fallback
}

package agentegress

import "net/netip"

// SetUDPOriginalDstForTest replaces the decoder that recovers a redirected
// datagram's original destination. Only netfilter can produce a real one,
// so tests supply destinations directly. It must be called before Start.
func (p *Proxy) SetUDPOriginalDstForTest(fn func(oob []byte) netip.AddrPort) {
	p.udpOrigDst = fn
}

// FakeIPForTest returns the fake address the proxy would hand out for name,
// allocating it if needed.
func (p *Proxy) FakeIPForTest(name string) netip.Addr {
	return p.fake.Lookup(name)
}

// UDPDropCountsForTest returns the oversized and unknown-destination drop
// counters of the UDP proxy.
func (p *Proxy) UDPDropCountsForTest() (oversized, unknownDst int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.udp == nil {
		return 0, 0
	}
	return p.udp.oversized.Load(), p.udp.unknownDst.Load()
}

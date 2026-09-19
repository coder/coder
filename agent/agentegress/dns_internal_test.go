package agentegress

import (
	"context"
	"net"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/net/dns/dnsmessage"

	"github.com/coder/quartz"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// fakeUpstreamDNS is a DNS-over-TCP server standing in for the system
// resolvers. It answers every query with a fixed A record and records the
// names it was asked.
type fakeUpstreamDNS struct {
	addr  netip.AddrPort
	mu    sync.Mutex
	names []string
}

func newFakeUpstreamDNS(t testing.TB, answer netip.Addr) *fakeUpstreamDNS {
	t.Helper()
	f := &fakeUpstreamDNS{}
	ln := serveTCP(t, func(conn net.Conn) {
		for {
			query, err := readFrame(conn)
			if err != nil {
				return
			}
			var m dnsmessage.Message
			if err := m.Unpack(query); err != nil || len(m.Questions) != 1 {
				return
			}
			f.mu.Lock()
			f.names = append(f.names, m.Questions[0].Name.String())
			f.mu.Unlock()
			if writeFrame(conn, dnsAnswer(query, &dnsmessage.AResource{A: answer.As4()})) != nil {
				return
			}
		}
	})
	f.addr = netip.MustParseAddrPort(ln.Addr().String())
	return f
}

func (f *fakeUpstreamDNS) seen() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.names)
}

// dnsAnswer builds a response to query carrying body as its single answer,
// or nil when the query is malformed.
func dnsAnswer(query []byte, body dnsmessage.ResourceBody) []byte {
	var m dnsmessage.Message
	if err := m.Unpack(query); err != nil || len(m.Questions) != 1 {
		return nil
	}
	m.Response, m.RecursionAvailable = true, true
	m.Answers = []dnsmessage.Resource{{
		Header: dnsmessage.ResourceHeader{Name: m.Questions[0].Name, Class: dnsmessage.ClassINET, TTL: 60},
		Body:   body,
	}}
	out, _ := m.Pack()
	return out
}

// dnsQuery builds a query for name of type typ with the given ID.
func dnsQuery(t testing.TB, id uint16, name string, typ dnsmessage.Type) []byte {
	t.Helper()
	m := dnsmessage.Message{
		Header:    dnsmessage.Header{ID: id, RecursionDesired: true},
		Questions: []dnsmessage.Question{{Name: dnsmessage.MustNewName(name), Type: typ, Class: dnsmessage.ClassINET}},
	}
	msg, err := m.Pack()
	require.NoError(t, err)
	return msg
}

// dnsExchange sends msg to addr over network ("udp" or "tcp") and returns
// the parsed reply.
func dnsExchange(ctx context.Context, t testing.TB, network string, addr netip.AddrPort, msg []byte) (dnsmessage.Header, []dnsmessage.Resource) {
	t.Helper()
	var d net.Dialer
	conn, err := d.DialContext(ctx, network, addr.String())
	require.NoError(t, err)
	defer conn.Close()
	deadline, _ := ctx.Deadline()
	require.NoError(t, conn.SetDeadline(deadline))

	resp := make([]byte, 4096)
	if network == "tcp" {
		require.NoError(t, writeFrame(conn, msg))
		resp, err = readFrame(conn)
	} else {
		_, err = conn.Write(msg)
		require.NoError(t, err)
		var n int
		n, err = conn.Read(resp)
		resp = resp[:n]
	}
	require.NoError(t, err)

	var m dnsmessage.Message
	require.NoError(t, m.Unpack(resp))
	require.True(t, m.Response)
	return m.Header, m.Answers
}

func TestDNS_FakeIPForA(t *testing.T) {
	t.Parallel()

	exit := newFakeExitNode(t, nil)
	proxy := startProxy(t, exit, proxyOptions{})
	ctx := testutil.Context(t, testutil.WaitShort)

	hdr, answers := dnsExchange(ctx, t, "udp", proxy.DNSAddr(), dnsQuery(t, 0x1234, "Example.COM.", dnsmessage.TypeA))
	require.Equal(t, uint16(0x1234), hdr.ID)
	require.Equal(t, dnsmessage.RCodeSuccess, hdr.RCode)
	require.Len(t, answers, 1)
	a, ok := answers[0].Body.(*dnsmessage.AResource)
	require.True(t, ok, "expected A record, got %T", answers[0].Body)
	addr := netip.AddrFrom4(a.A)
	require.True(t, fakeIPPrefix.Contains(addr), addr)
	require.Equal(t, uint32(30), answers[0].Header.TTL)
	require.Equal(t, proxy.fake.Lookup("example.com"), addr)

	// The same name over TCP yields the same address.
	_, tcpAnswers := dnsExchange(ctx, t, "tcp", proxy.DNSAddr(), dnsQuery(t, 7, "example.com.", dnsmessage.TypeA))
	require.Len(t, tcpAnswers, 1)
	require.Equal(t, answers[0].Body, tcpAnswers[0].Body)

	// The decision reached the exit node once and was cached for the TCP query.
	require.Len(t, exit.connectsTo(dnsRelayTarget), 1)
}

func TestDNS_AAAAIsEmpty(t *testing.T) {
	t.Parallel()

	exit := newFakeExitNode(t, nil)
	proxy := startProxy(t, exit, proxyOptions{})
	ctx := testutil.Context(t, testutil.WaitShort)

	hdr, answers := dnsExchange(ctx, t, "udp", proxy.DNSAddr(), dnsQuery(t, 1, "example.com.", dnsmessage.TypeAAAA))
	require.Equal(t, dnsmessage.RCodeSuccess, hdr.RCode)
	require.Empty(t, answers)
}

func TestDNS_ExemptAAAAForwardedUpstream(t *testing.T) {
	t.Parallel()

	upstream := newFakeUpstreamDNS(t, netip.MustParseAddr("203.0.113.6"))
	exit := newFakeExitNode(t, nil)
	proxy := startProxy(t, exit, proxyOptions{
		exemptHosts: []string{"tcp/control.example.com:443"},
		upstream:    []netip.AddrPort{upstream.addr},
	})
	ctx := testutil.Context(t, testutil.WaitShort)

	hdr, answers := dnsExchange(ctx, t, "udp", proxy.DNSAddr(), dnsQuery(t, 8, "control.example.com.", dnsmessage.TypeAAAA))
	require.Equal(t, dnsmessage.RCodeSuccess, hdr.RCode)
	require.Len(t, answers, 1)
	require.Equal(t, []string{"control.example.com."}, upstream.seen())
	require.Empty(t, exit.connectsTo(dnsRelayTarget))
}

func TestDNS_PTRForFakeIP(t *testing.T) {
	t.Parallel()

	exit := newFakeExitNode(t, nil)
	proxy := startProxy(t, exit, proxyOptions{})
	ctx := testutil.Context(t, testutil.WaitShort)

	octets := strings.Split(proxy.fake.Lookup("reverse.example.net").String(), ".")
	slices.Reverse(octets)
	ptrName := strings.Join(octets, ".") + ".in-addr.arpa."
	hdr, answers := dnsExchange(ctx, t, "udp", proxy.DNSAddr(), dnsQuery(t, 2, ptrName, dnsmessage.TypePTR))
	require.Equal(t, dnsmessage.RCodeSuccess, hdr.RCode)
	require.Len(t, answers, 1)
	ptr, ok := answers[0].Body.(*dnsmessage.PTRResource)
	require.True(t, ok)
	require.Equal(t, "reverse.example.net.", ptr.PTR.String())

	// An unassigned address in the fake range is NXDOMAIN, not relayed.
	hdr, _ = dnsExchange(ctx, t, "udp", proxy.DNSAddr(), dnsQuery(t, 3, "254.255.19.198.in-addr.arpa.", dnsmessage.TypePTR))
	require.Equal(t, dnsmessage.RCodeNameError, hdr.RCode)
	require.Empty(t, exit.connectsTo(dnsRelayTarget))
}

func TestDNS_ExemptNameForwardedUpstream(t *testing.T) {
	t.Parallel()

	upstream := newFakeUpstreamDNS(t, netip.MustParseAddr("203.0.113.5"))
	exit := newFakeExitNode(t, nil)
	proxy := startProxy(t, exit, proxyOptions{
		exemptHosts: []string{"tcp/Coder.Example.com:443", "tcp/198.51.100.7:443"},
		upstream:    []netip.AddrPort{upstream.addr},
	})
	ctx := testutil.Context(t, testutil.WaitShort)

	hdr, answers := dnsExchange(ctx, t, "udp", proxy.DNSAddr(), dnsQuery(t, 9, "coder.example.com.", dnsmessage.TypeA))
	require.Equal(t, uint16(9), hdr.ID)
	require.Equal(t, dnsmessage.RCodeSuccess, hdr.RCode)
	require.Len(t, answers, 1)
	require.Equal(t, &dnsmessage.AResource{A: [4]byte{203, 0, 113, 5}}, answers[0].Body)
	require.Equal(t, []string{"coder.example.com."}, upstream.seen())

	// Built-in exemptions take the same path, for every record type.
	_, answers = dnsExchange(ctx, t, "udp", proxy.DNSAddr(), dnsQuery(t, 10, "host.docker.internal.", dnsmessage.TypeTXT))
	require.Len(t, answers, 1)
	require.Contains(t, upstream.seen(), "host.docker.internal.")
	require.Empty(t, exit.connectsTo(dnsRelayTarget))
}

func TestDNS_UpstreamFailureIsServfail(t *testing.T) {
	t.Parallel()

	// A listener that is closed immediately gives a refused connection.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	dead := netip.MustParseAddrPort(ln.Addr().String())
	require.NoError(t, ln.Close())

	exit := newFakeExitNode(t, nil)
	proxy := startProxy(t, exit, proxyOptions{
		exemptHosts: []string{"tcp/coder.example.com:443"},
		upstream:    []netip.AddrPort{dead},
	})
	ctx := testutil.Context(t, testutil.WaitShort)

	hdr, answers := dnsExchange(ctx, t, "udp", proxy.DNSAddr(), dnsQuery(t, 11, "coder.example.com.", dnsmessage.TypeA))
	require.Equal(t, dnsmessage.RCodeServerFailure, hdr.RCode)
	require.Empty(t, answers)
}

func TestDNS_OtherTypesRelayedToExitNode(t *testing.T) {
	t.Parallel()

	exit := newFakeExitNode(t, nil)
	// Answer queries two at a time in reverse so correlation by message ID
	// is exercised.
	exit.dnsReverseBatch = 2
	proxy := startProxy(t, exit, proxyOptions{})
	ctx := testutil.Context(t, testutil.WaitShort)

	type result struct {
		hdr     dnsmessage.Header
		answers []dnsmessage.Resource
	}
	results := make(chan result, 2)
	for _, id := range []uint16{100, 200} {
		go func() {
			hdr, answers := dnsExchange(ctx, t, "udp", proxy.DNSAddr(), dnsQuery(t, id, "mail.example.org.", dnsmessage.TypeTXT))
			results <- result{hdr: hdr, answers: answers}
		}()
	}
	ids := map[uint16]bool{}
	for range 2 {
		r := testutil.RequireReceive(ctx, t, results)
		require.Equal(t, dnsmessage.RCodeSuccess, r.hdr.RCode)
		require.Len(t, r.answers, 1)
		require.Equal(t, &dnsmessage.TXTResource{TXT: []string{"via-exit-node"}}, r.answers[0].Body)
		ids[r.hdr.ID] = true
	}
	require.Equal(t, map[uint16]bool{100: true, 200: true}, ids, "each client got its own ID back")

	// One long-lived stream serves both queries.
	connects := exit.connectsTo(dnsRelayTarget)
	require.Len(t, connects, 1)
	require.Equal(t, codersdk.ExitNodeProtocolDNS, connects[0].proto)
}

func TestDNS_ExitNodeFailureIsServfail(t *testing.T) {
	t.Parallel()

	exit := newFakeExitNode(t, func(target string) (string, bool) {
		return "dns relay disabled", target == dnsRelayTarget
	})
	proxy := startProxy(t, exit, proxyOptions{})
	ctx := testutil.Context(t, testutil.WaitShort)

	hdr, _ := dnsExchange(ctx, t, "udp", proxy.DNSAddr(), dnsQuery(t, 12, "example.org.", dnsmessage.TypeMX))
	require.Equal(t, dnsmessage.RCodeServerFailure, hdr.RCode)
	hdr, answers := dnsExchange(ctx, t, "udp", proxy.DNSAddr(), dnsQuery(t, 13, "example.org.", dnsmessage.TypeA))
	require.Equal(t, dnsmessage.RCodeServerFailure, hdr.RCode)
	require.Empty(t, answers)
}

func dnsResponse(query []byte, rcode dnsmessage.RCode, ttl uint32) []byte {
	var m dnsmessage.Message
	if err := m.Unpack(query); err != nil || len(m.Questions) != 1 {
		return nil
	}
	m.Response, m.RecursionAvailable, m.RCode = true, true, rcode
	if rcode == dnsmessage.RCodeSuccess {
		m.Answers = []dnsmessage.Resource{{
			Header: dnsmessage.ResourceHeader{Name: m.Questions[0].Name, Class: dnsmessage.ClassINET, TTL: ttl},
			Body:   &dnsmessage.AResource{A: [4]byte{203, 0, 113, 1}},
		}}
	}
	out, _ := m.Pack()
	return out
}

func TestDNS_AddressPolicyDecision(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name      string
		rcode     dnsmessage.RCode
		ttl       uint32
		wantRCode dnsmessage.RCode
		wantTTL   uint32
		wantA     bool
	}{
		{name: "refused", rcode: dnsmessage.RCodeRefused, ttl: 30, wantRCode: dnsmessage.RCodeRefused},
		{name: "nxdomain", rcode: dnsmessage.RCodeNameError, ttl: 30, wantRCode: dnsmessage.RCodeNameError},
		{name: "ttl minimum", rcode: dnsmessage.RCodeSuccess, ttl: 1, wantRCode: dnsmessage.RCodeSuccess, wantTTL: 5, wantA: true},
		{name: "ttl maximum", rcode: dnsmessage.RCodeSuccess, ttl: 600, wantRCode: dnsmessage.RCodeSuccess, wantTTL: 30, wantA: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			exit := newFakeExitNode(t, nil)
			exit.dnsResponse = func(query []byte) []byte { return dnsResponse(query, tt.rcode, tt.ttl) }
			proxy := startProxy(t, exit, proxyOptions{})
			ctx := testutil.Context(t, testutil.WaitShort)
			hdr, answers := dnsExchange(ctx, t, "udp", proxy.DNSAddr(), dnsQuery(t, 20, "policy.example.", dnsmessage.TypeA))
			require.Equal(t, tt.wantRCode, hdr.RCode)
			if tt.wantA {
				require.Len(t, answers, 1)
				require.Equal(t, tt.wantTTL, answers[0].Header.TTL)
				addr := netip.AddrFrom4(answers[0].Body.(*dnsmessage.AResource).A)
				require.True(t, fakeIPPrefix.Contains(addr))
			} else {
				require.Empty(t, answers)
			}
		})
	}
}

func TestDNS_AddressDecisionCacheKeyIncludesType(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var exchanges int
	exit := newFakeExitNode(t, nil)
	exit.dnsResponse = func(query []byte) []byte {
		mu.Lock()
		exchanges++
		mu.Unlock()
		return dnsResponse(query, dnsmessage.RCodeSuccess, 60)
	}
	proxy := startProxy(t, exit, proxyOptions{})
	ctx := testutil.Context(t, testutil.WaitShort)
	ids := []uint16{40, 41, 42}
	for i, typ := range []dnsmessage.Type{dnsmessage.TypeA, dnsmessage.TypeAAAA, dnsmessage.TypeA} {
		hdr, _ := dnsExchange(ctx, t, "udp", proxy.DNSAddr(), dnsQuery(t, ids[i], "typed.example.", typ))
		require.Equal(t, dnsmessage.RCodeSuccess, hdr.RCode)
	}
	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, 2, exchanges)
}

func TestDNS_AddressDecisionRemainingTTL(t *testing.T) {
	t.Parallel()

	clock := quartz.NewMock(t)
	exit := newFakeExitNode(t, nil)
	exit.dnsResponse = func(query []byte) []byte { return dnsResponse(query, dnsmessage.RCodeSuccess, 60) }
	proxy := startProxy(t, exit, proxyOptions{clock: clock})
	ctx := testutil.Context(t, testutil.WaitShort)

	_, answers := dnsExchange(ctx, t, "udp", proxy.DNSAddr(), dnsQuery(t, 50, "remaining.example.", dnsmessage.TypeA))
	require.Equal(t, uint32(30), answers[0].Header.TTL)
	clock.Advance(29 * time.Second).MustWait(t.Context())
	_, answers = dnsExchange(ctx, t, "udp", proxy.DNSAddr(), dnsQuery(t, 51, "remaining.example.", dnsmessage.TypeA))
	require.Equal(t, uint32(1), answers[0].Header.TTL)
	require.Len(t, exit.connectsTo(dnsRelayTarget), 1)
}

func TestDNS_AddressDecisionCache(t *testing.T) {
	t.Parallel()

	var exchanges int
	exit := newFakeExitNode(t, nil)
	exit.dnsResponse = func(query []byte) []byte {
		exchanges++
		return dnsResponse(query, dnsmessage.RCodeSuccess, 60)
	}
	proxy := startProxy(t, exit, proxyOptions{})
	ctx := testutil.Context(t, testutil.WaitShort)
	for _, id := range []uint16{30, 31} {
		hdr, _ := dnsExchange(ctx, t, "udp", proxy.DNSAddr(), dnsQuery(t, id, "cached.example.", dnsmessage.TypeA))
		require.Equal(t, dnsmessage.RCodeSuccess, hdr.RCode)
	}
	require.Equal(t, 1, exchanges)
}

func TestParseReverseName(t *testing.T) {
	t.Parallel()

	addr, ok := parseReverseName("4.3.2.1.in-addr.arpa")
	require.True(t, ok)
	require.Equal(t, netip.MustParseAddr("1.2.3.4"), addr)
	for _, bad := range []string{"3.2.1.in-addr.arpa", "5.4.3.2.1.in-addr.arpa", "a.b.c.d.in-addr.arpa", "256.3.2.1.in-addr.arpa", "4.3.2.1.ip6.arpa"} {
		_, ok := parseReverseName(bad)
		require.False(t, ok, bad)
	}
}

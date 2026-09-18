package agentegress_test

import (
	"context"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/net/dns/dnsmessage"

	"github.com/coder/coder/v2/testutil"
)

// fakeUpstreamDNS is a DNS-over-TCP server standing in for the system
// resolvers. It answers A queries with a fixed address and records what
// it was asked.
type fakeUpstreamDNS struct {
	t        testing.TB
	listener net.Listener
	answer   netip.Addr

	mu    sync.Mutex
	names []string
	wg    sync.WaitGroup
}

func newFakeUpstreamDNS(t testing.TB, answer netip.Addr) *fakeUpstreamDNS {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	f := &fakeUpstreamDNS{t: t, listener: ln, answer: answer}
	f.wg.Add(1)
	go f.serve()
	t.Cleanup(func() {
		_ = ln.Close()
		f.wg.Wait()
	})
	return f
}

func (f *fakeUpstreamDNS) addr() netip.AddrPort {
	return f.listener.Addr().(*net.TCPAddr).AddrPort()
}

func (f *fakeUpstreamDNS) serve() {
	defer f.wg.Done()
	for {
		conn, err := f.listener.Accept()
		if err != nil {
			return
		}
		f.wg.Add(1)
		go func() {
			defer f.wg.Done()
			defer conn.Close()
			for {
				query, err := readTestFrame(conn)
				if err != nil {
					return
				}
				var p dnsmessage.Parser
				hdr, err := p.Start(query)
				require.NoError(f.t, err)
				q, err := p.Question()
				require.NoError(f.t, err)
				f.mu.Lock()
				f.names = append(f.names, q.Name.String())
				f.mu.Unlock()
				b := dnsmessage.NewBuilder(nil, dnsmessage.Header{ID: hdr.ID, Response: true, RecursionAvailable: true})
				require.NoError(f.t, b.StartQuestions())
				require.NoError(f.t, b.Question(q))
				require.NoError(f.t, b.StartAnswers())
				require.NoError(f.t, b.AResource(dnsmessage.ResourceHeader{
					Name: q.Name, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET, TTL: 300,
				}, dnsmessage.AResource{A: f.answer.As4()}))
				out, err := b.Finish()
				require.NoError(f.t, err)
				if err := writeTestFrame(conn, out); err != nil {
					return
				}
			}
		}()
	}
}

func (f *fakeUpstreamDNS) seen() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.names...)
}

// dnsQuery builds a query for name of type typ with the given ID.
func dnsQuery(t testing.TB, id uint16, name string, typ dnsmessage.Type) []byte {
	t.Helper()
	n, err := dnsmessage.NewName(name)
	require.NoError(t, err)
	b := dnsmessage.NewBuilder(nil, dnsmessage.Header{ID: id, RecursionDesired: true})
	require.NoError(t, b.StartQuestions())
	require.NoError(t, b.Question(dnsmessage.Question{Name: n, Type: typ, Class: dnsmessage.ClassINET}))
	msg, err := b.Finish()
	require.NoError(t, err)
	return msg
}

// dnsExchangeUDP sends msg to addr over UDP and returns the parsed reply.
func dnsExchangeUDP(ctx context.Context, t testing.TB, addr netip.AddrPort, msg []byte) (dnsmessage.Header, []dnsmessage.Resource) {
	t.Helper()
	var d net.Dialer
	conn, err := d.DialContext(ctx, "udp", addr.String())
	require.NoError(t, err)
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		require.NoError(t, conn.SetDeadline(deadline))
	}
	_, err = conn.Write(msg)
	require.NoError(t, err)
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	require.NoError(t, err)
	return parseDNSResponse(t, buf[:n])
}

// dnsExchangeTCP sends msg to addr over DNS-over-TCP and returns the reply.
func dnsExchangeTCP(ctx context.Context, t testing.TB, addr netip.AddrPort, msg []byte) (dnsmessage.Header, []dnsmessage.Resource) {
	t.Helper()
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr.String())
	require.NoError(t, err)
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		require.NoError(t, conn.SetDeadline(deadline))
	}
	require.NoError(t, writeTestFrame(conn, msg))
	resp, err := readTestFrame(conn)
	require.NoError(t, err)
	return parseDNSResponse(t, resp)
}

func parseDNSResponse(t testing.TB, resp []byte) (dnsmessage.Header, []dnsmessage.Resource) {
	t.Helper()
	var p dnsmessage.Parser
	hdr, err := p.Start(resp)
	require.NoError(t, err)
	require.True(t, hdr.Response)
	require.NoError(t, p.SkipAllQuestions())
	answers, err := p.AllAnswers()
	require.NoError(t, err)
	return hdr, answers
}

func TestDNS_FakeIPForA(t *testing.T) {
	t.Parallel()

	exit := newFakeExitNode(t, nil)
	proxy := startProxy(t, exit, proxyOptions{})
	ctx := testutil.Context(t, testutil.WaitShort)

	hdr, answers := dnsExchangeUDP(ctx, t, proxy.DNSAddr(), dnsQuery(t, 0x1234, "Example.COM.", dnsmessage.TypeA))
	require.Equal(t, uint16(0x1234), hdr.ID)
	require.Equal(t, dnsmessage.RCodeSuccess, hdr.RCode)
	require.Len(t, answers, 1)
	a, ok := answers[0].Body.(*dnsmessage.AResource)
	require.True(t, ok, "expected A record, got %T", answers[0].Body)
	addr := netip.AddrFrom4(a.A)
	require.True(t, netip.MustParsePrefix("198.18.0.0/15").Contains(addr), addr)
	require.Equal(t, uint32(1), answers[0].Header.TTL)
	require.Equal(t, proxy.FakeIPForTest("example.com"), addr)

	// The same name over TCP yields the same address.
	_, tcpAnswers := dnsExchangeTCP(ctx, t, proxy.DNSAddr(), dnsQuery(t, 7, "example.com.", dnsmessage.TypeA))
	require.Len(t, tcpAnswers, 1)
	require.Equal(t, a.A, tcpAnswers[0].Body.(*dnsmessage.AResource).A)

	// Nothing reached the exit node for these.
	require.Empty(t, exit.connectsTo("dns:53"))
}

func TestDNS_AAAAIsEmpty(t *testing.T) {
	t.Parallel()

	exit := newFakeExitNode(t, nil)
	proxy := startProxy(t, exit, proxyOptions{})
	ctx := testutil.Context(t, testutil.WaitShort)

	hdr, answers := dnsExchangeUDP(ctx, t, proxy.DNSAddr(), dnsQuery(t, 1, "example.com.", dnsmessage.TypeAAAA))
	require.Equal(t, dnsmessage.RCodeSuccess, hdr.RCode)
	require.Empty(t, answers)
}

func TestDNS_PTRForFakeIP(t *testing.T) {
	t.Parallel()

	exit := newFakeExitNode(t, nil)
	proxy := startProxy(t, exit, proxyOptions{})
	ctx := testutil.Context(t, testutil.WaitShort)

	fake := proxy.FakeIPForTest("reverse.example.net")
	octets := fake.As4()
	ptrName := strings.Join([]string{
		itoa(octets[3]), itoa(octets[2]), itoa(octets[1]), itoa(octets[0]), "in-addr.arpa.",
	}, ".")
	hdr, answers := dnsExchangeUDP(ctx, t, proxy.DNSAddr(), dnsQuery(t, 2, ptrName, dnsmessage.TypePTR))
	require.Equal(t, dnsmessage.RCodeSuccess, hdr.RCode)
	require.Len(t, answers, 1)
	ptr, ok := answers[0].Body.(*dnsmessage.PTRResource)
	require.True(t, ok)
	require.Equal(t, "reverse.example.net.", ptr.PTR.String())

	// An unassigned address in the fake range is NXDOMAIN, not relayed.
	hdr, _ = dnsExchangeUDP(ctx, t, proxy.DNSAddr(), dnsQuery(t, 3, "254.255.19.198.in-addr.arpa.", dnsmessage.TypePTR))
	require.Equal(t, dnsmessage.RCodeNameError, hdr.RCode)
	require.Empty(t, exit.connectsTo("dns:53"))
}

func TestDNS_ExemptNameForwardedUpstream(t *testing.T) {
	t.Parallel()

	upstream := newFakeUpstreamDNS(t, netip.MustParseAddr("203.0.113.5"))
	exit := newFakeExitNode(t, nil)
	proxy := startProxy(t, exit, proxyOptions{
		exemptHosts: []string{"Coder.Example.com:443", "198.51.100.7"},
		upstream:    []netip.AddrPort{upstream.addr()},
	})
	ctx := testutil.Context(t, testutil.WaitShort)

	hdr, answers := dnsExchangeUDP(ctx, t, proxy.DNSAddr(), dnsQuery(t, 9, "coder.example.com.", dnsmessage.TypeA))
	require.Equal(t, uint16(9), hdr.ID)
	require.Equal(t, dnsmessage.RCodeSuccess, hdr.RCode)
	require.Len(t, answers, 1)
	require.Equal(t, [4]byte{203, 0, 113, 5}, answers[0].Body.(*dnsmessage.AResource).A)
	require.Equal(t, []string{"coder.example.com."}, upstream.seen())

	// Built-in exemptions take the same path, for every record type.
	_, answers = dnsExchangeUDP(ctx, t, proxy.DNSAddr(), dnsQuery(t, 10, "host.docker.internal.", dnsmessage.TypeTXT))
	require.Len(t, answers, 1)
	require.Contains(t, upstream.seen(), "host.docker.internal.")
	require.Empty(t, exit.connectsTo("dns:53"))
}

func TestDNS_UpstreamFailureIsServfail(t *testing.T) {
	t.Parallel()

	// A listener that is closed immediately gives a refused connection.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	dead := ln.Addr().(*net.TCPAddr).AddrPort()
	require.NoError(t, ln.Close())

	exit := newFakeExitNode(t, nil)
	proxy := startProxy(t, exit, proxyOptions{
		exemptHosts: []string{"coder.example.com"},
		upstream:    []netip.AddrPort{dead},
	})
	ctx := testutil.Context(t, testutil.WaitShort)

	hdr, answers := dnsExchangeUDP(ctx, t, proxy.DNSAddr(), dnsQuery(t, 11, "coder.example.com.", dnsmessage.TypeA))
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
			hdr, answers := dnsExchangeUDP(ctx, t, proxy.DNSAddr(), dnsQuery(t, id, "mail.example.org.", dnsmessage.TypeTXT))
			results <- result{hdr: hdr, answers: answers}
		}()
	}
	ids := map[uint16]bool{}
	for range 2 {
		r := testutil.RequireReceive(ctx, t, results)
		require.Equal(t, dnsmessage.RCodeSuccess, r.hdr.RCode)
		require.Len(t, r.answers, 1)
		txt, ok := r.answers[0].Body.(*dnsmessage.TXTResource)
		require.True(t, ok)
		require.Equal(t, []string{"via-exit-node"}, txt.TXT)
		ids[r.hdr.ID] = true
	}
	require.Equal(t, map[uint16]bool{100: true, 200: true}, ids, "each client got its own ID back")

	// One long-lived stream serves both queries.
	connects := exit.connectsTo("dns:53")
	require.Len(t, connects, 1)
	require.Equal(t, "dns", connects[0].proto)
}

func TestDNS_ExitNodeFailureIsServfail(t *testing.T) {
	t.Parallel()

	exit := newFakeExitNode(t, func(target string) (string, bool) {
		return "dns relay disabled", target == "dns:53"
	})
	proxy := startProxy(t, exit, proxyOptions{})
	ctx := testutil.Context(t, testutil.WaitShort)

	hdr, _ := dnsExchangeUDP(ctx, t, proxy.DNSAddr(), dnsQuery(t, 12, "example.org.", dnsmessage.TypeMX))
	require.Equal(t, dnsmessage.RCodeServerFailure, hdr.RCode)
	// A queries are unaffected by the relay being down.
	hdr, answers := dnsExchangeUDP(ctx, t, proxy.DNSAddr(), dnsQuery(t, 13, "example.org.", dnsmessage.TypeA))
	require.Equal(t, dnsmessage.RCodeSuccess, hdr.RCode)
	require.Len(t, answers, 1)
}

func itoa(b byte) string {
	return strconv.Itoa(int(b))
}

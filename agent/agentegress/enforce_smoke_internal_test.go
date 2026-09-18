//go:build linux

package agentegress

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/dns/dnsmessage"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/testutil"
)

// TestEnforcer_Iptables exercises the real iptables path. It mutates the
// host's netfilter state and briefly redirects all new TCP, UDP and DNS
// traffic into a test proxy, so it only runs when CODER_TEST_IPTABLES=1 is
// set and root or passwordless sudo is available. The phases share the
// host's single rule set and therefore run sequentially inside one test.
func TestEnforcer_Iptables(t *testing.T) {
	t.Parallel()
	if os.Getenv("CODER_TEST_IPTABLES") != "1" {
		t.Skip("set CODER_TEST_IPTABLES=1 to run against real iptables")
	}

	ctx := testutil.Context(t, testutil.WaitLong)
	testEnforcerInstallRemove(ctx, t)
	testEnforcerTransparentRedirect(ctx, t)
}

// testEnforcerInstallRemove checks rule contents, idempotency and cleanup.
func testEnforcerInstallRemove(ctx context.Context, t *testing.T) {
	t.Helper()
	e, err := NewEnforcer(testutil.Logger(t), EnforcerOptions{
		ProxyPort:         40123,
		DNSPort:           40124,
		UDPPort:           40125,
		ControlPlaneHosts: []string{"198.51.100.7:443", "203.0.113.10"},
		Resolvers:         []netip.Addr{netip.MustParseAddr("192.0.2.53")},
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = e.Remove(ctx)
	})

	err = e.Install(ctx)
	require.NoError(t, err)
	// Idempotent.
	require.NoError(t, e.Install(ctx))

	list := func(table, chain string) string {
		t.Helper()
		out, err := e.run(ctx, "iptables", "-t", table, "-S", chain)
		require.NoError(t, err)
		return out
	}
	nat := list("nat", natChain)
	require.Contains(t, nat, "-A CODER_EGRESS -o lo -j RETURN")
	require.Contains(t, nat, "-A CODER_EGRESS -d 198.51.100.7/32 -p tcp -m tcp --dport 443 -j RETURN")
	require.Contains(t, nat, "-A CODER_EGRESS -d 198.51.100.7/32 -p udp -m udp --dport 443 -j RETURN")
	require.Contains(t, nat, "-A CODER_EGRESS -d 203.0.113.10/32 -p tcp -j RETURN")
	require.Contains(t, nat, "-A CODER_EGRESS -d 203.0.113.10/32 -p udp -j RETURN")
	require.Contains(t, nat, "-A CODER_EGRESS -d 192.0.2.53/32 -p tcp -m tcp --dport 53 -j RETURN")
	require.Contains(t, nat, "-A CODER_EGRESS -p udp -m udp --dport 53 -j REDIRECT --to-ports 40124")
	require.Contains(t, nat, "-A CODER_EGRESS -p tcp -m tcp --dport 53 -j REDIRECT --to-ports 40124")
	require.Contains(t, nat, "-A CODER_EGRESS -p udp -j REDIRECT --to-ports 40125")
	require.Contains(t, nat, "-A CODER_EGRESS -p tcp -j REDIRECT --to-ports 40123")
	// The flush on reinstall keeps exactly one copy of each rule.
	require.Equal(t, 4, strings.Count(nat, "-j REDIRECT"))
	filter := list("filter", filterChain)
	require.Contains(t, filter, "-A CODER_EGRESS_FILTER -p icmp -j DROP")
	require.NotContains(t, filter, "-p udp -j DROP")
	output := list("nat", "OUTPUT")
	require.Equal(t, 1, strings.Count(output, "-j CODER_EGRESS"))

	require.NoError(t, e.Remove(ctx))
	// Idempotent.
	require.NoError(t, e.Remove(ctx))

	for _, table := range []string{"nat", "filter"} {
		out, err := e.run(ctx, "iptables", "-t", table, "-S")
		require.NoError(t, err)
		require.NotContains(t, out, "CODER_EGRESS", table)
	}
}

// testEnforcerTransparentRedirect verifies the redirect paths end to end
// with rules installed:
//
//   - a plain HTTP request to a non-local address is redirected into the
//     proxy, which recovers the original destination via SO_ORIGINAL_DST
//     and tunnels it through a fake exit node;
//   - a UDP DNS query to a non-local resolver is redirected into the DNS
//     listener and answered with a fake address;
//   - a UDP datagram to a non-local address is redirected into the UDP
//     listener, where the kernel cannot report its original destination,
//     so it is counted as dropped rather than relayed.
//
// The fake exit node answers with a canned response and never dials out,
// so no traffic loops back through the redirect.
func testEnforcerTransparentRedirect(ctx context.Context, t *testing.T) {
	t.Helper()
	// TEST-NET-1 is never routable, but the default route still carries the
	// packets into nat OUTPUT where REDIRECT catches them.
	const target = "192.0.2.1:80"
	seen := make(chan string, 1)
	exit := newCannedExitNode(t, target, seen)

	proxy, err := New(testutil.Logger(t), Options{
		Dialer:            exit,
		Config:            agentsdk.EgressConfig{ExitNodeID: uuid.New(), ExitNodePort: 3128},
		UpstreamResolvers: []netip.AddrPort{},
	})
	require.NoError(t, err)
	require.NoError(t, proxy.Start(ctx))
	t.Cleanup(func() { _ = proxy.Close() })

	e, err := NewEnforcer(testutil.Logger(t), EnforcerOptions{
		ProxyPort: proxy.Addr().Port(),
		DNSPort:   proxy.DNSAddr().Port(),
		UDPPort:   proxy.UDPAddr().Port(),
		Resolvers: []netip.Addr{},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = e.Remove(ctx) })
	require.NoError(t, e.Install(ctx))

	client := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+target+"/redirected", nil)
	require.NoError(t, err)
	resp, err := client.Do(req)
	require.NoError(t, err)
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "canned", string(body))
	require.Equal(t, "GET /redirected", testutil.RequireReceive(ctx, t, seen))

	// DNS to a non-local resolver lands in the DNS listener.
	dnsConn, err := net.Dial("udp4", "192.0.2.53:53")
	require.NoError(t, err)
	defer dnsConn.Close()
	deadline, _ := ctx.Deadline()
	require.NoError(t, dnsConn.SetDeadline(deadline))
	name, err := dnsmessage.NewName("redirected.example.com.")
	require.NoError(t, err)
	b := dnsmessage.NewBuilder(nil, dnsmessage.Header{ID: 42, RecursionDesired: true})
	require.NoError(t, b.StartQuestions())
	require.NoError(t, b.Question(dnsmessage.Question{Name: name, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET}))
	query, err := b.Finish()
	require.NoError(t, err)
	_, err = dnsConn.Write(query)
	require.NoError(t, err)
	buf := make([]byte, 512)
	n, err := dnsConn.Read(buf)
	require.NoError(t, err)
	var p dnsmessage.Parser
	hdr, err := p.Start(buf[:n])
	require.NoError(t, err)
	require.Equal(t, uint16(42), hdr.ID)
	require.NoError(t, p.SkipAllQuestions())
	answers, err := p.AllAnswers()
	require.NoError(t, err)
	require.Len(t, answers, 1)
	a, ok := answers[0].Body.(*dnsmessage.AResource)
	require.True(t, ok)
	require.True(t, fakeIPPrefix.Contains(netip.AddrFrom4(a.A)))

	// Other UDP lands in the UDP listener, which cannot recover the
	// destination from a REDIRECTed datagram and drops it.
	udpConn, err := net.Dial("udp4", "192.0.2.1:9999")
	require.NoError(t, err)
	defer udpConn.Close()
	_, err = udpConn.Write([]byte("hello"))
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		_, unknown := proxy.udp.oversized.Load(), proxy.udp.unknownDst.Load()
		return unknown >= 1
	}, testutil.WaitShort, testutil.IntervalFast)

	require.NoError(t, e.Remove(ctx))
}

// cannedExitNode accepts CONNECT for exactly one destination, then answers
// any HTTP request on the tunnel with a fixed 200 body.
type cannedExitNode struct {
	listener net.Listener
	target   string
	seen     chan<- string
}

func newCannedExitNode(t testing.TB, target string, seen chan<- string) *cannedExitNode {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	c := &cannedExitNode{listener: ln, target: target, seen: seen}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go c.handle(conn)
		}
	}()
	return c
}

func (c *cannedExitNode) handle(conn net.Conn) {
	defer conn.Close()
	br := bufio.NewReader(conn)
	connect, err := http.ReadRequest(br)
	if err != nil || connect.Method != http.MethodConnect || connect.Host != c.target {
		_, _ = io.WriteString(conn, "HTTP/1.1 403 Forbidden\r\n"+codersdk.ExitNodeDenyReasonHeader+": unexpected destination\r\nContent-Length: 0\r\n\r\n")
		return
	}
	if _, err := io.WriteString(conn, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		return
	}
	inner, err := http.ReadRequest(br)
	if err != nil {
		return
	}
	select {
	case c.seen <- inner.Method + " " + inner.URL.Path:
	default:
	}
	_, _ = io.WriteString(conn, "HTTP/1.1 200 OK\r\nContent-Length: 6\r\nConnection: close\r\n\r\ncanned")
}

func (c *cannedExitNode) DialContextTCP(ctx context.Context, _ netip.AddrPort) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, "tcp", c.listener.Addr().String())
}

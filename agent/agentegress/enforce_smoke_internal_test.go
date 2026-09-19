//go:build linux

package agentegress

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/dns/dnsmessage"

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
		ProxyPort:               40123,
		DNSPort:                 40124,
		UDPPort:                 40125,
		ControlPlaneHosts:       []string{"198.51.100.7:443", "203.0.113.10"},
		Resolvers:               []netip.Addr{netip.MustParseAddr("192.0.2.53")},
		LockdownCapabilities:    false,
		LockdownCapabilitiesSet: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = e.Remove(ctx) })

	require.NoError(t, e.Install(ctx))
	// Idempotent.
	require.NoError(t, e.Install(ctx))

	list := func(table, chain string) string {
		t.Helper()
		out, err := e.run(ctx, "iptables", "-t", table, "-S", chain)
		require.NoError(t, err)
		return out
	}
	nat := list("nat", natChain)
	for _, want := range []string{
		"-A CODER_EGRESS -o lo -j RETURN",
		"-A CODER_EGRESS -d 198.51.100.7/32 -p tcp -m tcp --dport 443 -j RETURN",
		"-A CODER_EGRESS -d 198.51.100.7/32 -p udp -m udp --dport 443 -j RETURN",
		"-A CODER_EGRESS -d 203.0.113.10/32 -p tcp -j RETURN",
		"-A CODER_EGRESS -d 203.0.113.10/32 -p udp -j RETURN",
		"-A CODER_EGRESS -d 192.0.2.53/32 -p tcp -m tcp --dport 53 -j RETURN",
		"-A CODER_EGRESS -p udp -m udp --dport 53 -j REDIRECT --to-ports 40124",
		"-A CODER_EGRESS -p tcp -m tcp --dport 53 -j REDIRECT --to-ports 40124",
		"-A CODER_EGRESS -p tcp -j REDIRECT --to-ports 40123",
	} {
		require.Contains(t, nat, want)
	}
	// This test has no transparent listener, so Install uses the loud
	// REDIRECT fallback and keeps one copy of every rule.
	require.Equal(t, 4, strings.Count(nat, "-j REDIRECT"))
	require.Contains(t, nat, "-A CODER_EGRESS -p udp -j REDIRECT --to-ports 40125")
	filter := list("filter", filterChain)
	require.Contains(t, filter, "-A CODER_EGRESS_FILTER -p icmp -j DROP")
	require.NotContains(t, filter, "-p udp -j DROP")
	require.Equal(t, 1, strings.Count(list("nat", "OUTPUT"), "-j CODER_EGRESS"))

	require.NoError(t, e.Remove(ctx))
	// Idempotent.
	require.NoError(t, e.Remove(ctx))

	for _, table := range []string{"nat", "filter", "mangle"} {
		out, err := e.run(ctx, "iptables", "-t", table, "-S")
		require.NoError(t, err)
		require.NotContains(t, out, "CODER_EGRESS", table)
	}
	ipRules, err := e.run(ctx, "ip", "-4", "rule", "show")
	require.NoError(t, err)
	require.NotContains(t, ipRules, "fwmark 0x434f lookup 54321")
	routes, err := e.run(ctx, "ip", "-4", "route", "show", "table", tproxyTable)
	if err == nil {
		require.Empty(t, strings.TrimSpace(routes))
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
// The fake exit node only dials the loopback origin, so no traffic loops
// back through the redirect.
func testEnforcerTransparentRedirect(ctx context.Context, t *testing.T) {
	t.Helper()
	// TEST-NET-1 is never routable, but the default route still carries the
	// packets into nat OUTPUT where REDIRECT catches them.
	const target = "192.0.2.1:80"
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/redirected", r.URL.Path)
		_, _ = io.WriteString(w, "canned")
	}))
	t.Cleanup(origin.Close)
	exit := newFakeExitNode(t, func(got string) (string, bool) {
		switch got {
		case target, "192.0.2.1:9999", "192.0.2.1:9998":
			return "", false
		default:
			return "unexpected destination", true
		}
	})
	exit.resolve[target] = strings.TrimPrefix(origin.URL, "http://")
	proxy := startProxy(t, exit, proxyOptions{})

	e, err := NewEnforcer(testutil.Logger(t), EnforcerOptions{
		ProxyPort:               proxy.Addr().Port(),
		DNSPort:                 proxy.DNSAddr().Port(),
		UDPPort:                 proxy.UDPAddr().Port(),
		Resolvers:               []netip.Addr{},
		LockdownCapabilities:    false,
		LockdownCapabilitiesSet: true,
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
	require.Len(t, exit.connectsTo(target), 1)

	// DNS to a non-local resolver lands in the DNS listener.
	hdr, answers := dnsExchange(ctx, t, "udp", netip.MustParseAddrPort("192.0.2.53:53"), dnsQuery(t, 42, "redirected.example.com.", dnsmessage.TypeA))
	require.Equal(t, uint16(42), hdr.ID)
	require.Len(t, answers, 1)
	a, ok := answers[0].Body.(*dnsmessage.AResource)
	require.True(t, ok)
	require.True(t, fakeIPPrefix.Contains(netip.AddrFrom4(a.A)))

	// Other UDP is relayed when transparent sockets and TPROXY are available.
	// Otherwise REDIRECT captures and loudly drops it.
	if e.TransparentUDP() {
		connected, err := net.DialUDP("udp4", nil, net.UDPAddrFromAddrPort(netip.MustParseAddrPort("192.0.2.1:9999")))
		require.NoError(t, err)
		defer connected.Close()
		deadline, _ := ctx.Deadline()
		require.NoError(t, connected.SetDeadline(deadline))
		_, err = connected.Write([]byte("connected"))
		require.NoError(t, err)
		buf := make([]byte, 64)
		n, err := connected.Read(buf)
		require.NoError(t, err)
		require.Equal(t, "echo:connected", string(buf[:n]))

		unconnected, err := net.ListenUDP("udp4", nil)
		require.NoError(t, err)
		defer unconnected.Close()
		require.NoError(t, unconnected.SetDeadline(deadline))
		dst := net.UDPAddrFromAddrPort(netip.MustParseAddrPort("192.0.2.1:9998"))
		_, err = unconnected.WriteToUDP([]byte("unconnected"), dst)
		require.NoError(t, err)
		n, src, err := unconnected.ReadFromUDP(buf)
		require.NoError(t, err)
		require.Equal(t, dst.String(), src.String())
		require.Equal(t, "echo:unconnected", string(buf[:n]))

		require.Eventually(t, func() bool {
			return len(exit.connectsTo("192.0.2.1:9999")) >= 1 &&
				len(exit.connectsTo("192.0.2.1:9998")) >= 1
		}, testutil.WaitShort, testutil.IntervalFast)
	} else {
		udpConn, err := net.Dial("udp4", "192.0.2.1:9999")
		require.NoError(t, err)
		defer udpConn.Close()
		_, err = udpConn.Write([]byte("hello"))
		require.NoError(t, err)
		require.Eventually(t, func() bool {
			return proxy.udp.unknownDst.Load() >= 1
		}, testutil.WaitShort, testutil.IntervalFast)
	}

	require.NoError(t, e.Remove(ctx))
}

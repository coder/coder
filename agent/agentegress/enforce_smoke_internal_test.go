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
	"os/exec"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/dns/dnsmessage"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/testutil"
)

// TestEnforcer_Iptables mutates host netfilter state and only runs when
// CODER_TEST_IPTABLES=1 is set.
func TestEnforcer_Iptables(t *testing.T) {
	t.Parallel()
	if os.Getenv("CODER_TEST_IPTABLES") != "1" {
		t.Skip("set CODER_TEST_IPTABLES=1 to run against real iptables")
	}

	ctx := testutil.Context(t, testutil.WaitLong)
	testEnforcerInstallRemove(ctx, t)
	testEnforcerTransparentRedirect(ctx, t)
	testCapabilityLockdownChildren(ctx, t)
}

func testCapabilityLockdownChildren(ctx context.Context, t *testing.T) {
	t.Helper()
	// #nosec G204 -- os.Args[0] is the currently running test binary.
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run", "^TestCapabilityLockdownHelper$")
	cmd.Env = append(os.Environ(), "CODER_CAPABILITY_LOCKDOWN_HELPER=1")
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, string(output))
}

func TestCapabilityLockdownHelper(t *testing.T) {
	t.Parallel()
	if os.Getenv("CODER_CAPABILITY_LOCKDOWN_HELPER") != "1" {
		t.Skip("helper process")
	}
	require.NoError(t, lockdownNetworkCapabilities())

	const children = 8
	results := make(chan error, children)
	var wg sync.WaitGroup
	for range children {
		wg.Add(1)
		go func() {
			defer wg.Done()
			output, err := exec.Command("sh", "-c", "grep '^Cap\\(Bnd\\|Prm\\|Eff\\):' /proc/self/status").CombinedOutput()
			if err != nil {
				results <- xerrors.Errorf("read child capabilities: %w: %s", err, output)
				return
			}
			caps, err := statusCapabilities(output)
			if err != nil {
				results <- xerrors.Errorf("parse child capabilities: %w", err)
				return
			}
			for _, name := range []string{"CapBnd", "CapPrm", "CapEff"} {
				for _, capability := range lockedCapabilities {
					if caps[name]&capability.mask != 0 {
						results <- xerrors.Errorf("child retained %s in %s", capability.name, name)
						return
					}
				}
			}
			results <- nil
		}()
	}
	wg.Wait()
	close(results)
	for err := range results {
		require.NoError(t, err)
	}
}

func testEnforcerInstallRemove(ctx context.Context, t *testing.T) {
	t.Helper()
	e, err := NewEnforcer(testutil.Logger(t), EnforcerOptions{
		ProxyPort:               40123,
		DNSPort:                 40124,
		UDPPort:                 40125,
		ControlPlaneHosts:       []string{"tcp/198.51.100.7:443", "udp/203.0.113.10:41641"},
		LockdownCapabilities:    false,
		LockdownCapabilitiesSet: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = e.Remove(ctx) })

	require.NoError(t, e.Install(ctx))
	require.NoError(t, e.Install(ctx))

	list := func(table, chain string) string {
		t.Helper()
		out, err := e.run(ctx, "iptables", "-t", table, "-S", chain)
		require.NoError(t, err)
		return out
	}
	nat := list("nat", natChain)
	for _, want := range []string{
		"-A CODER_EGRESS -m mark --mark 0x4350 -j RETURN",
		"-A CODER_EGRESS -o lo -j RETURN",
		"-A CODER_EGRESS -d 198.51.100.7/32 -p tcp -m tcp --dport 443 -j RETURN",
		"-A CODER_EGRESS -d 203.0.113.10/32 -p udp -m udp --dport 41641 -j RETURN",
		"-A CODER_EGRESS -p udp -m udp --dport 53 -j REDIRECT --to-ports 40124",
		"-A CODER_EGRESS -p tcp -m tcp --dport 53 -j REDIRECT --to-ports 40124",
		"-A CODER_EGRESS -p tcp -j REDIRECT --to-ports 40123",
	} {
		require.Contains(t, nat, want)
	}
	require.Equal(t, 4, strings.Count(nat, "-j REDIRECT"))
	require.Contains(t, nat, "-A CODER_EGRESS -p udp -j REDIRECT --to-ports 40125")
	filter := list("filter", filterChain)
	require.Contains(t, filter, "-A CODER_EGRESS_FILTER -p icmp -j DROP")
	require.NotContains(t, filter, "-p udp -j DROP")
	require.Equal(t, 1, strings.Count(list("nat", "OUTPUT"), "-j CODER_EGRESS"))

	require.NoError(t, e.Remove(ctx))
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
		case target, "192.0.2.1:9999", "192.0.2.1:9998", dnsRelayTarget:
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
		LockdownCapabilities:    false,
		LockdownCapabilitiesSet: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = e.Remove(ctx) })
	require.NoError(t, e.Install(ctx))
	testTCPDNSMarkBypass(ctx, t)

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

func testTCPDNSMarkBypass(ctx context.Context, t *testing.T) {
	t.Helper()
	const resolver = "192.0.2.53:53"

	unmarked, err := (&net.Dialer{}).DialContext(ctx, "tcp4", resolver)
	require.NoError(t, err, "unmarked TCP DNS must be redirected to the DNS proxy")
	require.NoError(t, unmarked.Close())

	markedDialer := net.Dialer{Control: bypassControl}
	markedCtx, cancel := context.WithTimeout(ctx, testutil.IntervalMedium)
	defer cancel()
	marked, err := markedDialer.DialContext(markedCtx, "tcp4", resolver)
	if err == nil {
		_ = marked.Close()
	}
	require.Error(t, err, "marked TCP DNS must bypass the local DNS proxy")
}

package exitnode_test

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/exitnode"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/testutil"
)

// recordingFlows collects flow reports and signals each one on a channel.
type recordingFlows struct {
	mu      sync.Mutex
	reports []codersdk.ExitNodeFlowReport
	ch      chan codersdk.ExitNodeFlowReport
}

func newRecordingFlows() *recordingFlows {
	return &recordingFlows{ch: make(chan codersdk.ExitNodeFlowReport, 16)}
}

func (r *recordingFlows) Record(report codersdk.ExitNodeFlowReport) {
	r.mu.Lock()
	r.reports = append(r.reports, report)
	r.mu.Unlock()
	r.ch <- report
}

// fakeAddrConn gives a net.Pipe end a TCP-looking remote address so the
// proxy can attribute it to an agent.
type fakeAddrConn struct {
	net.Conn
	remote net.Addr
}

func (c *fakeAddrConn) RemoteAddr() net.Addr { return c.remote }

type proxyHarness struct {
	agentID uuid.UUID
	flows   *recordingFlows
	metrics *exitnode.Metrics
	proxy   *exitnode.ConnectProxy
}

func newProxyHarness(t *testing.T, policyYAML string, configure ...func(*exitnode.ConnectProxyOptions)) *proxyHarness {
	t.Helper()
	policy, err := exitnode.ParsePolicy([]byte(policyYAML))
	require.NoError(t, err)

	agentID := uuid.New()
	agents := exitnode.NewAgentTable()
	agents.Set([]uuid.UUID{agentID})

	flows := newRecordingFlows()
	metrics := exitnode.NewMetrics(nil)
	opts := exitnode.ConnectProxyOptions{
		Logger:       testutil.Logger(t),
		Policy:       policy,
		Agents:       agents,
		Flows:        flows,
		Metrics:      metrics,
		SniffTimeout: testutil.WaitShort,
		// Tests must never depend on the host's DNS configuration.
		Resolver:     fakeResolver{},
		DNSExchanger: &exitnode.StaticDNSExchanger{},
	}
	for _, fn := range configure {
		fn(&opts)
	}
	proxy := exitnode.NewConnectProxy(opts)
	return &proxyHarness{agentID: agentID, flows: flows, metrics: metrics, proxy: proxy}
}

// fakeResolver answers hostname lookups from a fixed table.
type fakeResolver map[string][]netip.Addr

func (r fakeResolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	addrs, ok := r[host]
	if !ok {
		return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
	}
	return addrs, nil
}

// connectResult is the CONNECT response as seen by the client, or ok=false
// when the proxy closed without answering.
type connectResult struct {
	ok     bool
	status int
	header http.Header
}

// connect opens a pipe to the proxy as the given source address, sends a
// CONNECT for target, and returns the client end plus the parsed response.
func (h *proxyHarness) connect(ctx context.Context, t *testing.T, src netip.Addr, target string) (net.Conn, *bufio.Reader, connectResult, <-chan struct{}) {
	t.Helper()
	return h.connectWithHeaders(ctx, t, src, target, nil)
}

// connectWithHeaders is connect with extra request headers, for selecting a
// protocol or supplying the original host.
func (h *proxyHarness) connectWithHeaders(ctx context.Context, t *testing.T, src netip.Addr, target string, headers http.Header) (net.Conn, *bufio.Reader, connectResult, <-chan struct{}) {
	t.Helper()
	clientSide, serverSide := net.Pipe()
	t.Cleanup(func() { _ = clientSide.Close() })

	serverConn := &fakeAddrConn{
		Conn:   serverSide,
		remote: &net.TCPAddr{IP: src.AsSlice(), Port: 40000},
	}
	done := make(chan struct{}, 1)
	go func() {
		h.proxy.HandleConn(ctx, serverConn)
		done <- struct{}{}
	}()

	writeErr := make(chan error, 1)
	go func() {
		var b strings.Builder
		_, _ = fmt.Fprintf(&b, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n", target, target)
		for k, vs := range headers {
			for _, v := range vs {
				_, _ = fmt.Fprintf(&b, "%s: %s\r\n", k, v)
			}
		}
		_, _ = b.WriteString("\r\n")
		_, err := io.WriteString(clientSide, b.String())
		writeErr <- err
	}()

	br := bufio.NewReader(clientSide)
	resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodConnect})
	if err != nil {
		// The proxy closed without answering (unknown source).
		return clientSide, br, connectResult{}, done
	}
	_ = resp.Body.Close()
	require.NoError(t, testutil.RequireReceive(ctx, t, writeErr))
	return clientSide, br, connectResult{ok: true, status: resp.StatusCode, header: resp.Header}, done
}

// readTunneled parses an HTTP response relayed through the tunnel and returns
// its body.
func readTunneled(t *testing.T, br *bufio.Reader) string {
	t.Helper()
	resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodGet})
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return string(body)
}

func TestConnectProxy_AllowProxiesAndReports(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "hello from "+r.Host)
	}))
	defer upstream.Close()
	target := strings.TrimPrefix(upstream.URL, "http://")
	targetAddr := netip.MustParseAddrPort(target)

	h := newProxyHarness(t, `
default: deny
rules:
  - id: allow-example
    allow:
      hosts: ["*.example.com"]
`)
	src := tailnet.TailscaleServicePrefix.AddrFromUUID(h.agentID)
	client, br, resp, done := h.connect(ctx, t, src, target)
	require.True(t, resp.ok)
	require.Equal(t, http.StatusOK, resp.status)

	// Speak HTTP through the tunnel; the Host header is what the policy
	// sees in phase two. Connection: close makes the upstream hang up so
	// both copy directions finish deterministically.
	request := "GET /x HTTP/1.1\r\nHost: api.example.com\r\nConnection: close\r\n\r\n"
	go func() { _, _ = io.WriteString(client, request) }()

	require.Equal(t, "hello from api.example.com", readTunneled(t, br))
	_ = client.Close()
	testutil.RequireReceive(ctx, t, done)

	connectReport := testutil.RequireReceive(ctx, t, h.flows.ch)
	require.Equal(t, codersdk.ExitNodeFlowAllow, connectReport.Decision)
	require.Equal(t, "allow-example", connectReport.RuleID)
	require.Equal(t, h.agentID, connectReport.AgentID)
	require.Equal(t, targetAddr.Addr().String(), connectReport.DestinationIP)
	require.Equal(t, int(targetAddr.Port()), connectReport.DestinationPort)
	require.Equal(t, "api.example.com", connectReport.Host)
	require.Nil(t, connectReport.DisconnectTime)

	disconnectReport := testutil.RequireReceive(ctx, t, h.flows.ch)
	require.Equal(t, connectReport.FlowID, disconnectReport.FlowID)
	require.NotNil(t, disconnectReport.DisconnectTime)
	require.Equal(t, int64(len(request)), disconnectReport.BytesOut)
	require.Positive(t, disconnectReport.BytesIn)

	require.Equal(t, float64(1), promtestutil.ToFloat64(h.metrics.FlowsTotal.WithLabelValues("allow")))
	require.Equal(t, float64(0), promtestutil.ToFloat64(h.metrics.ActiveFlows))
	require.Equal(t, float64(len(request)), promtestutil.ToFloat64(h.metrics.BytesTotal.WithLabelValues("out")))
}

func TestConnectProxy_DenyBeforeConnect(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)

	h := newProxyHarness(t, `
default: allow
rules:
  - id: no-metadata
    deny:
      cidrs: ["169.254.0.0/16"]
`)
	src := tailnet.TailscaleServicePrefix.AddrFromUUID(h.agentID)
	client, _, resp, done := h.connect(ctx, t, src, "169.254.169.254:80")
	require.True(t, resp.ok)
	require.Equal(t, http.StatusForbidden, resp.status)
	require.Equal(t, "matched rule no-metadata", resp.header.Get(codersdk.ExitNodeDenyReasonHeader))
	require.Equal(t, "no-metadata", resp.header.Get(codersdk.ExitNodeDenyRuleHeader))

	// The proxy closes after the 403.
	_, err := client.Read(make([]byte, 1))
	require.ErrorIs(t, err, io.EOF)
	testutil.RequireReceive(ctx, t, done)

	report := testutil.RequireReceive(ctx, t, h.flows.ch)
	require.Equal(t, codersdk.ExitNodeFlowDeny, report.Decision)
	require.Equal(t, "no-metadata", report.RuleID)
	require.Equal(t, "169.254.169.254", report.DestinationIP)
	require.Equal(t, 80, report.DestinationPort)
	require.NotNil(t, report.DisconnectTime)
	require.Equal(t, float64(1), promtestutil.ToFloat64(h.metrics.FlowsTotal.WithLabelValues("deny")))
}

func TestConnectProxy_DenyByHostAfterConnect(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)

	// The upstream must never be reached; a listener that fails the test on
	// accept proves it.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			_ = conn.Close()
			t.Error("upstream was dialed for a denied host")
		}
	}()

	h := newProxyHarness(t, `
default: allow
rules:
  - id: no-evil
    deny:
      hosts: ["evil.example"]
`)
	src := tailnet.TailscaleServicePrefix.AddrFromUUID(h.agentID)
	client, _, resp, done := h.connect(ctx, t, src, ln.Addr().String())
	require.True(t, resp.ok)
	// Phase one cannot know the host, so the CONNECT is provisionally
	// accepted.
	require.Equal(t, http.StatusOK, resp.status)

	go func() {
		_, _ = io.WriteString(client, "GET / HTTP/1.1\r\nHost: evil.example\r\n\r\n")
	}()
	// Phase two denies and the only signal left is a closed connection.
	_, err = client.Read(make([]byte, 1))
	require.ErrorIs(t, err, io.EOF)
	testutil.RequireReceive(ctx, t, done)

	report := testutil.RequireReceive(ctx, t, h.flows.ch)
	require.Equal(t, codersdk.ExitNodeFlowDeny, report.Decision)
	require.Equal(t, "no-evil", report.RuleID)
	require.Equal(t, "evil.example", report.Host)
}

func TestConnectProxy_UnknownSourceRejected(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)

	h := newProxyHarness(t, "default: allow\n")
	stranger := tailnet.TailscaleServicePrefix.AddrFromUUID(uuid.New())
	client, _, resp, done := h.connect(ctx, t, stranger, "127.0.0.1:80")
	require.False(t, resp.ok, "unknown sources get no HTTP response at all")
	_, err := client.Read(make([]byte, 1))
	require.ErrorIs(t, err, io.EOF)
	testutil.RequireReceive(ctx, t, done)

	require.Equal(t, float64(1), promtestutil.ToFloat64(h.metrics.UnknownSourceTotal))
	require.Empty(t, h.flows.reports)
}

func TestConnectProxy_RejectsBadTarget(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		target  string
		headers http.Header
		want    string
	}{
		{name: "no port", target: "example.com", want: "host:port"},
		{name: "zero port", target: "example.com:0", want: "invalid port"},
		{name: "garbage host", target: "bad!host:443", want: "not an ip or hostname"},
		{name: "unknown protocol", target: "127.0.0.1:443", headers: http.Header{codersdk.ExitNodeProtocolHeader: []string{"sctp"}}, want: "must be tcp, udp, or dns"},
		{name: "dns wrong target", target: "127.0.0.1:53", headers: http.Header{codersdk.ExitNodeProtocolHeader: []string{"dns"}}, want: "dns:53"},
		{name: "bad original host", target: "127.0.0.1:443", headers: http.Header{codersdk.ExitNodeOriginalHostHeader: []string{"bad host"}}, want: "not a valid hostname"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)

			h := newProxyHarness(t, "default: allow\n")
			src := tailnet.TailscaleServicePrefix.AddrFromUUID(h.agentID)
			_, _, resp, done := h.connectWithHeaders(ctx, t, src, tt.target, tt.headers)
			require.True(t, resp.ok)
			require.Equal(t, http.StatusBadRequest, resp.status)
			require.Contains(t, resp.header.Get(codersdk.ExitNodeDenyReasonHeader), tt.want)
			testutil.RequireReceive(ctx, t, done)
			require.Empty(t, h.flows.reports)
		})
	}
}

func TestConnectProxy_Hostname(t *testing.T) {
	t.Parallel()

	t.Run("Allow", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, "hello from "+r.Host)
		}))
		defer upstream.Close()
		upstreamAddr := netip.MustParseAddrPort(strings.TrimPrefix(upstream.URL, "http://"))

		// The resolver hands back IPv6 first to prove IPv4 is preferred.
		resolver := fakeResolver{"api.example.com": {netip.MustParseAddr("::1"), upstreamAddr.Addr()}}
		h := newProxyHarness(t, `
default: deny
rules:
  - id: allow-example
    allow:
      hosts: ["*.example.com"]
`, func(o *exitnode.ConnectProxyOptions) { o.Resolver = resolver })
		src := tailnet.TailscaleServicePrefix.AddrFromUUID(h.agentID)
		client, br, resp, done := h.connect(ctx, t, src, fmt.Sprintf("API.example.com:%d", upstreamAddr.Port()))
		require.True(t, resp.ok)
		require.Equal(t, http.StatusOK, resp.status)

		// The Host header names a host the policy would deny, proving the
		// CONNECT hostname is authoritative and nothing is sniffed.
		request := "GET /x HTTP/1.1\r\nHost: other.test\r\nConnection: close\r\n\r\n"
		go func() { _, _ = io.WriteString(client, request) }()
		require.Equal(t, "hello from other.test", readTunneled(t, br))
		_ = client.Close()
		testutil.RequireReceive(ctx, t, done)

		connectReport := testutil.RequireReceive(ctx, t, h.flows.ch)
		require.Equal(t, codersdk.ExitNodeFlowAllow, connectReport.Decision)
		require.Equal(t, codersdk.ExitNodeProtocolTCP, connectReport.Protocol)
		require.Equal(t, "allow-example", connectReport.RuleID)
		require.Equal(t, "api.example.com", connectReport.Host)
		require.Equal(t, upstreamAddr.Addr().String(), connectReport.DestinationIP)
		require.Equal(t, int(upstreamAddr.Port()), connectReport.DestinationPort)

		disconnectReport := testutil.RequireReceive(ctx, t, h.flows.ch)
		require.Equal(t, connectReport.FlowID, disconnectReport.FlowID)
		require.Equal(t, int64(len(request)), disconnectReport.BytesOut)
	})

	t.Run("HostDenyIsImmediate", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		resolver := fakeResolver{"evil.example": {netip.MustParseAddr("192.0.2.1")}}
		h := newProxyHarness(t, `
default: allow
rules:
  - id: no-evil
    deny:
      hosts: ["evil.example"]
`, func(o *exitnode.ConnectProxyOptions) { o.Resolver = resolver })
		src := tailnet.TailscaleServicePrefix.AddrFromUUID(h.agentID)
		client, _, resp, done := h.connect(ctx, t, src, "evil.example:443")
		require.True(t, resp.ok)
		require.Equal(t, http.StatusForbidden, resp.status)
		require.Equal(t, "matched rule no-evil", resp.header.Get(codersdk.ExitNodeDenyReasonHeader))
		require.Equal(t, "no-evil", resp.header.Get(codersdk.ExitNodeDenyRuleHeader))
		_, err := client.Read(make([]byte, 1))
		require.ErrorIs(t, err, io.EOF)
		testutil.RequireReceive(ctx, t, done)

		report := testutil.RequireReceive(ctx, t, h.flows.ch)
		require.Equal(t, codersdk.ExitNodeFlowDeny, report.Decision)
		require.Equal(t, "no-evil", report.RuleID)
		require.Equal(t, "evil.example", report.Host)
		require.Equal(t, "192.0.2.1", report.DestinationIP)
	})

	t.Run("OriginalHostHeader", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		h := newProxyHarness(t, `
default: allow
rules:
  - id: no-evil
    deny:
      hosts: ["evil.example"]
`)
		src := tailnet.TailscaleServicePrefix.AddrFromUUID(h.agentID)
		// An IP literal with the original host supplied is decided up front
		// instead of waiting for a sniff.
		_, _, resp, done := h.connectWithHeaders(ctx, t, src, "192.0.2.1:443", http.Header{
			codersdk.ExitNodeOriginalHostHeader: []string{"evil.example"},
		})
		require.True(t, resp.ok)
		require.Equal(t, http.StatusForbidden, resp.status)
		require.Equal(t, "no-evil", resp.header.Get(codersdk.ExitNodeDenyRuleHeader))
		testutil.RequireReceive(ctx, t, done)
		report := testutil.RequireReceive(ctx, t, h.flows.ch)
		require.Equal(t, "evil.example", report.Host)
	})

	t.Run("ResolveFailure", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		h := newProxyHarness(t, "default: allow\n")
		src := tailnet.TailscaleServicePrefix.AddrFromUUID(h.agentID)
		client, _, resp, done := h.connect(ctx, t, src, "nxdomain.example:443")
		require.True(t, resp.ok)
		require.Equal(t, http.StatusBadGateway, resp.status)
		require.Equal(t, "resolve failed", resp.header.Get(codersdk.ExitNodeDenyReasonHeader))
		_, err := client.Read(make([]byte, 1))
		require.ErrorIs(t, err, io.EOF)
		testutil.RequireReceive(ctx, t, done)
		require.Empty(t, h.flows.reports)
	})
}

func TestConnectProxy_Serve(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(testutil.Context(t, testutil.WaitLong))
	defer cancel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	defer upstream.Close()
	target := strings.TrimPrefix(upstream.URL, "http://")

	h := newProxyHarness(t, "default: allow\n")
	// Serve over a loopback listener; the source is loopback, so register
	// it as if it were the agent's tailnet address via a resolver that
	// accepts anything from 127.0.0.1.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	loopbackAgents := exitnode.NewAgentTable()
	proxy := exitnode.NewConnectProxy(exitnode.ConnectProxyOptions{
		Logger:  testutil.Logger(t),
		Policy:  mustPolicy(t, "default: allow\n"),
		Agents:  loopbackResolver{agentID: h.agentID, table: loopbackAgents},
		Flows:   h.flows,
		Metrics: h.metrics,
	})
	serveErr := make(chan error, 1)
	go func() { serveErr <- proxy.Serve(ctx, ln) }()

	conn, err := net.Dial("tcp", ln.Addr().String())
	require.NoError(t, err)
	defer conn.Close()
	_, err = fmt.Fprintf(conn, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", target, target)
	require.NoError(t, err)
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodConnect})
	require.NoError(t, err)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	_, err = io.WriteString(conn, "GET / HTTP/1.1\r\nHost: anything\r\nConnection: close\r\n\r\n")
	require.NoError(t, err)
	require.Equal(t, "ok", readTunneled(t, br))

	cancel()
	require.NoError(t, testutil.RequireReceive(testutil.Context(t, testutil.WaitShort), t, serveErr))
	proxy.Wait()
}

// loopbackResolver attributes any loopback source to a fixed agent, so the
// accept loop can be exercised over real TCP without a tailnet.
type loopbackResolver struct {
	agentID uuid.UUID
	table   *exitnode.AgentTable
}

func (r loopbackResolver) AgentForAddr(addr netip.Addr) (uuid.UUID, bool) {
	if addr.IsLoopback() {
		return r.agentID, true
	}
	return r.table.AgentForAddr(addr)
}

func mustPolicy(t *testing.T, yaml string) exitnode.PolicyEvaluator {
	t.Helper()
	p, err := exitnode.ParsePolicy([]byte(yaml))
	require.NoError(t, err)
	return p
}

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
	"github.com/coder/coder/v2/enterprise/exitnode/yamlpolicy"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/testutil"
)

type recordingFlows struct {
	mu      sync.Mutex
	reports []codersdk.ExitNodeFlowReport
	ch      chan codersdk.ExitNodeFlowReport
}

func (r *recordingFlows) Record(report codersdk.ExitNodeFlowReport) {
	r.mu.Lock()
	r.reports = append(r.reports, report)
	r.mu.Unlock()
	r.ch <- report
}

type fakeAddrConn struct {
	net.Conn
	remote net.Addr
}

func (c *fakeAddrConn) RemoteAddr() net.Addr { return c.remote }

type proxyHarness struct {
	agentID uuid.UUID
	src     netip.Addr
	flows   *recordingFlows
	metrics *exitnode.Metrics
	proxy   *exitnode.ConnectProxy
}

func newProxyHarness(t *testing.T, policyYAML string, configure ...func(*exitnode.ConnectProxyOptions)) *proxyHarness {
	t.Helper()
	agentID := uuid.New()
	agents := exitnode.NewAgentTable()
	agents.Set([]uuid.UUID{agentID})
	h := &proxyHarness{
		agentID: agentID,
		src:     tailnet.TailscaleServicePrefix.AddrFromUUID(agentID),
		flows:   &recordingFlows{ch: make(chan codersdk.ExitNodeFlowReport, 16)},
		metrics: exitnode.NewMetrics(nil),
	}
	opts := exitnode.ConnectProxyOptions{
		Logger:       testutil.Logger(t),
		Policy:       mustPolicy(t, policyYAML),
		Agents:       agents,
		Flows:        h.flows,
		Metrics:      h.metrics,
		SniffTimeout: testutil.WaitShort,
		Resolver:     fakeResolver{},
		DNSExchanger: &exitnode.StaticDNSExchanger{},
	}
	for _, fn := range configure {
		fn(&opts)
	}
	h.proxy = exitnode.NewConnectProxy(opts)
	return h
}
func mustPolicy(t *testing.T, yaml string) *yamlpolicy.Policy {
	t.Helper()
	p, err := yamlpolicy.Parse([]byte(yaml))
	require.NoError(t, err)
	return p
}

type fakeResolver map[string][]netip.Addr

func (r fakeResolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	addrs, ok := r[host]
	if !ok {
		return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
	}
	return addrs, nil
}

type connectResult struct {
	ok     bool
	status int
	header http.Header
}

func (h *proxyHarness) connect(ctx context.Context, t *testing.T, src netip.Addr, target string, headers ...http.Header) (net.Conn, *bufio.Reader, connectResult, <-chan struct{}) {
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
		for _, hdr := range headers {
			_ = hdr.Write(&b)
		}
		_, _ = b.WriteString("\r\n")
		_, err := io.WriteString(clientSide, b.String())
		writeErr <- err
	}()
	br := bufio.NewReader(clientSide)
	resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodConnect})
	if err != nil {
		return clientSide, br, connectResult{}, done
	}
	_ = resp.Body.Close()
	require.NoError(t, testutil.RequireReceive(ctx, t, writeErr))
	return clientSide, br, connectResult{ok: true, status: resp.StatusCode, header: resp.Header}, done
}
func (h *proxyHarness) connectStatus(ctx context.Context, t *testing.T, target string, status int, headers ...http.Header) (net.Conn, *bufio.Reader, connectResult, <-chan struct{}) {
	t.Helper()
	client, br, resp, done := h.connect(ctx, t, h.src, target, headers...)
	require.True(t, resp.ok)
	require.Equal(t, status, resp.status)
	return client, br, resp, done
}
func requireEOF(t *testing.T, r io.Reader) {
	t.Helper()
	_, err := r.Read(make([]byte, 1))
	require.ErrorIs(t, err, io.EOF)
}
func readTunneled(t *testing.T, br *bufio.Reader) string {
	t.Helper()
	resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodGet})
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return string(body)
}
func startEchoHostServer(t *testing.T) netip.AddrPort {
	t.Helper()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "hello from "+r.Host)
	}))
	t.Cleanup(upstream.Close)
	return netip.MustParseAddrPort(strings.TrimPrefix(upstream.URL, "http://"))
}

const allowExamplePolicy = `
default: deny
rules:
  - id: allow-example
    allow:
      hosts: ["*.example.com"]
`
const noEvilPolicy = `
default: allow
rules:
  - id: no-evil
    deny:
      hosts: ["evil.example"]
`

func TestConnectProxy_AllowProxiesAndReports(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name            string
		target          func(netip.AddrPort) string
		resolver        func(netip.AddrPort) fakeResolver
		hostHeader      string
		provisionalHost bool
	}{
		{
			name:            "IPLiteralSniffsHost",
			target:          func(ap netip.AddrPort) string { return ap.String() },
			resolver:        func(netip.AddrPort) fakeResolver { return nil },
			hostHeader:      "api.example.com",
			provisionalHost: true,
		},
		{
			name:   "HostnameIsAuthoritative",
			target: func(ap netip.AddrPort) string { return fmt.Sprintf("API.example.com:%d", ap.Port()) },
			resolver: func(ap netip.AddrPort) fakeResolver {
				return fakeResolver{"api.example.com": {netip.MustParseAddr("::1"), ap.Addr()}}
			},
			hostHeader: "other.test",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			upstream := startEchoHostServer(t)
			h := newProxyHarness(t, allowExamplePolicy, func(o *exitnode.ConnectProxyOptions) {
				o.Resolver = tt.resolver(upstream)
				o.ProvisionalHostAllow = tt.provisionalHost
			})
			client, br, _, done := h.connectStatus(ctx, t, tt.target(upstream), http.StatusOK)
			request := "GET /x HTTP/1.1\r\nHost: " + tt.hostHeader + "\r\nConnection: close\r\n\r\n"
			go func() { _, _ = io.WriteString(client, request) }()
			require.Equal(t, "hello from "+tt.hostHeader, readTunneled(t, br))
			_ = client.Close()
			testutil.RequireReceive(ctx, t, done)
			connectReport := testutil.RequireReceive(ctx, t, h.flows.ch)
			require.Equal(t, codersdk.ExitNodeFlowAllow, connectReport.Decision)
			require.Equal(t, codersdk.ExitNodeProtocolTCP, connectReport.Protocol)
			require.Equal(t, "allow-example", connectReport.RuleID)
			require.Equal(t, h.agentID, connectReport.AgentID)
			require.Equal(t, upstream.Addr().String(), connectReport.DestinationIP)
			require.Equal(t, int(upstream.Port()), connectReport.DestinationPort)
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
		})
	}
}
func TestConnectProxy_StrictHostAllowDefault(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	upstream := startEchoHostServer(t)
	h := newProxyHarness(t, allowExamplePolicy)
	client, _, resp, done := h.connectStatus(ctx, t, upstream.String(), http.StatusForbidden)
	require.Equal(t, "default deny", resp.header.Get(codersdk.ExitNodeDenyReasonHeader))
	requireEOF(t, client)
	testutil.RequireReceive(ctx, t, done)
	report := testutil.RequireReceive(ctx, t, h.flows.ch)
	require.Equal(t, codersdk.ExitNodeFlowDeny, report.Decision)
	require.Empty(t, report.Host)
}
func TestConnectProxy_OriginalHostMustResolveToTarget(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	upstream := startEchoHostServer(t)
	h := newProxyHarness(t, allowExamplePolicy, func(o *exitnode.ConnectProxyOptions) {
		o.ProvisionalHostAllow = true
		o.Resolver = fakeResolver{"api.example.com": {netip.MustParseAddr("192.0.2.1")}}
	})
	client, _, _, done := h.connectStatus(ctx, t, upstream.String(), http.StatusOK,
		http.Header{codersdk.ExitNodeOriginalHostHeader: []string{"api.example.com"}})
	go func() {
		_, _ = io.WriteString(client, "GET / HTTP/1.1\r\nHost: blocked.test\r\n\r\n")
	}()
	requireEOF(t, client)
	testutil.RequireReceive(ctx, t, done)
	report := testutil.RequireReceive(ctx, t, h.flows.ch)
	require.Equal(t, codersdk.ExitNodeFlowDeny, report.Decision)
	require.Equal(t, "blocked.test", report.Host)
}
func TestConnectProxy_DenyBeforeConnect(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		policy   string
		target   string
		headers  http.Header
		resolver fakeResolver
		wantRule string
		wantHost string
		wantIP   string
		wantPort int
	}{
		{
			name: "CIDR",
			policy: `
default: allow
rules:
  - id: no-metadata
    deny:
      cidrs: ["169.254.0.0/16"]
`,
			target:   "169.254.169.254:80",
			wantRule: "no-metadata",
			wantIP:   "169.254.169.254",
			wantPort: 80,
		},
		{
			name:     "Hostname",
			policy:   noEvilPolicy,
			target:   "evil.example:443",
			resolver: fakeResolver{"evil.example": {netip.MustParseAddr("192.0.2.1")}},
			wantRule: "no-evil",
			wantHost: "evil.example",
			wantIP:   "192.0.2.1",
			wantPort: 443,
		},
		{
			name:     "OriginalHostHeader",
			policy:   noEvilPolicy,
			target:   "192.0.2.1:443",
			headers:  http.Header{codersdk.ExitNodeOriginalHostHeader: []string{"evil.example"}},
			resolver: fakeResolver{"evil.example": {netip.MustParseAddr("192.0.2.1")}},
			wantRule: "no-evil",
			wantHost: "evil.example",
			wantIP:   "192.0.2.1",
			wantPort: 443,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			h := newProxyHarness(t, tt.policy, func(o *exitnode.ConnectProxyOptions) { o.Resolver = tt.resolver })
			client, _, resp, done := h.connectStatus(ctx, t, tt.target, http.StatusForbidden, tt.headers)
			require.Equal(t, "matched rule "+tt.wantRule, resp.header.Get(codersdk.ExitNodeDenyReasonHeader))
			require.Equal(t, tt.wantRule, resp.header.Get(codersdk.ExitNodeDenyRuleHeader))
			requireEOF(t, client)
			testutil.RequireReceive(ctx, t, done)
			report := testutil.RequireReceive(ctx, t, h.flows.ch)
			require.Equal(t, codersdk.ExitNodeFlowDeny, report.Decision)
			require.Equal(t, tt.wantRule, report.RuleID)
			require.Equal(t, tt.wantHost, report.Host)
			require.Equal(t, tt.wantIP, report.DestinationIP)
			require.Equal(t, tt.wantPort, report.DestinationPort)
			require.NotNil(t, report.DisconnectTime)
			require.Equal(t, float64(1), promtestutil.ToFloat64(h.metrics.FlowsTotal.WithLabelValues("deny")))
		})
	}
}
func TestConnectProxy_DenyByHostAfterConnect(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
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
	h := newProxyHarness(t, noEvilPolicy)
	client, _, _, done := h.connectStatus(ctx, t, ln.Addr().String(), http.StatusOK)
	go func() {
		_, _ = io.WriteString(client, "GET / HTTP/1.1\r\nHost: evil.example\r\n\r\n")
	}()
	requireEOF(t, client)
	testutil.RequireReceive(ctx, t, done)
	report := testutil.RequireReceive(ctx, t, h.flows.ch)
	require.Equal(t, codersdk.ExitNodeFlowDeny, report.Decision)
	require.Equal(t, "no-evil", report.RuleID)
	require.Equal(t, "evil.example", report.Host)
}
func TestConnectProxy_Rejections(t *testing.T) {
	t.Parallel()
	t.Run("unknown source", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		h := newProxyHarness(t, "default: allow\n")
		client, _, resp, done := h.connect(ctx, t, tailnet.TailscaleServicePrefix.AddrFromUUID(uuid.New()), "127.0.0.1:80")
		require.False(t, resp.ok)
		requireEOF(t, client)
		testutil.RequireReceive(ctx, t, done)
		require.Equal(t, float64(1), promtestutil.ToFloat64(h.metrics.UnknownSourceTotal))
		require.Empty(t, h.flows.reports)
	})
	t.Run("resolve failure", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		h := newProxyHarness(t, "default: allow\n")
		client, _, resp, done := h.connectStatus(ctx, t, "missing.example:443", http.StatusBadGateway)
		require.Equal(t, "resolve failed", resp.header.Get(codersdk.ExitNodeDenyReasonHeader))
		requireEOF(t, client)
		testutil.RequireReceive(ctx, t, done)
		require.Empty(t, h.flows.reports)
	})
}

func TestConnectProxy_PolicyFunc(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	upstream := startEchoHostServer(t)
	var mu sync.Mutex
	var seen []exitnode.FlowInfo
	policy := exitnode.PolicyFunc(func(flow exitnode.FlowInfo) exitnode.Decision {
		mu.Lock()
		seen = append(seen, flow)
		mu.Unlock()
		if flow.Port == int(upstream.Port()) {
			return exitnode.Decision{Allow: true, RuleID: "custom-allow", Reason: "allowed port"}
		}
		return exitnode.Decision{RuleID: "custom-deny", Reason: "denied port"}
	})
	h := newProxyHarness(t, "default: deny\n", func(o *exitnode.ConnectProxyOptions) {
		o.Policy = policy
		o.Resolver = fakeResolver{"allowed.test": {upstream.Addr()}, "denied.test": {upstream.Addr()}}
	})
	client, _, _, done := h.connectStatus(ctx, t, fmt.Sprintf("allowed.test:%d", upstream.Port()), http.StatusOK)
	_ = client.Close()
	testutil.RequireReceive(ctx, t, done)
	require.Equal(t, "custom-allow", testutil.RequireReceive(ctx, t, h.flows.ch).RuleID)
	require.Equal(t, "custom-allow", testutil.RequireReceive(ctx, t, h.flows.ch).RuleID)
	client, _, resp, done := h.connectStatus(ctx, t, "denied.test:1", http.StatusForbidden)
	require.Equal(t, "custom-deny", resp.header.Get(codersdk.ExitNodeDenyRuleHeader))
	require.Equal(t, "denied port", resp.header.Get(codersdk.ExitNodeDenyReasonHeader))
	requireEOF(t, client)
	testutil.RequireReceive(ctx, t, done)
	require.Equal(t, "custom-deny", testutil.RequireReceive(ctx, t, h.flows.ch).RuleID)
	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, []exitnode.FlowInfo{
		{Protocol: codersdk.ExitNodeProtocolTCP, Host: "allowed.test", IP: upstream.Addr(), Port: int(upstream.Port())},
		{Protocol: codersdk.ExitNodeProtocolTCP, Host: "denied.test", IP: upstream.Addr(), Port: 1},
	}, seen)
}

type loopbackResolver struct{ agentID uuid.UUID }

func (r loopbackResolver) AgentForAddr(addr netip.Addr) (uuid.UUID, bool) {
	return r.agentID, addr.IsLoopback()
}

func TestConnectProxy_Serve(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(testutil.Context(t, testutil.WaitLong))
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	h := newProxyHarness(t, "default: allow\n")
	proxy := exitnode.NewConnectProxy(exitnode.ConnectProxyOptions{Logger: testutil.Logger(t), Policy: mustPolicy(t, "default: allow\n"), Agents: loopbackResolver{h.agentID}, Flows: h.flows, Metrics: h.metrics})
	done := make(chan error, 1)
	go func() { done <- proxy.Serve(ctx, ln) }()
	conn, err := net.Dial("tcp", ln.Addr().String())
	require.NoError(t, err)
	_, err = fmt.Fprint(conn, "CONNECT 127.0.0.1:1 HTTP/1.1\r\nHost: 127.0.0.1:1\r\n\r\n")
	require.NoError(t, err)
	resp, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: http.MethodConnect})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()
	_ = conn.Close()
	waitCtx := testutil.Context(t, testutil.WaitShort)
	cancel()
	_ = ln.Close()
	require.NoError(t, testutil.RequireReceive(waitCtx, t, done))
	proxy.Wait()
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
		{name: "dns wrong target", target: "127.0.0.1:53", headers: dnsHeader, want: "dns:53"},
		{name: "bad original host", target: "127.0.0.1:443", headers: http.Header{codersdk.ExitNodeOriginalHostHeader: []string{"bad host"}}, want: "not a valid hostname"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			h := newProxyHarness(t, "default: allow\n")
			_, _, resp, done := h.connectStatus(ctx, t, tt.target, http.StatusBadRequest, tt.headers)
			require.Contains(t, resp.header.Get(codersdk.ExitNodeDenyReasonHeader), tt.want)
			testutil.RequireReceive(ctx, t, done)
			require.Empty(t, h.flows.reports)
		})
	}
}

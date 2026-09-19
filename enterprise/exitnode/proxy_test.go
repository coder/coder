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

// recordingFlows collects flow reports and signals each one on a channel.
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

// fakeAddrConn gives a net.Pipe end a TCP-looking remote address so the
// proxy can attribute it to an agent.
type fakeAddrConn struct {
	net.Conn
	remote net.Addr
}

func (c *fakeAddrConn) RemoteAddr() net.Addr { return c.remote }

type proxyHarness struct {
	agentID uuid.UUID
	// src is the agent's tailnet address, which the proxy attributes to it.
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
		// Tests must never depend on the host's DNS configuration.
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
// CONNECT for target with any extra headers (for selecting a protocol or
// supplying the original host), and returns the client end plus the parsed
// response. done is signaled when the proxy has finished with the conn.
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
		// The proxy closed without answering (unknown source).
		return clientSide, br, connectResult{}, done
	}
	_ = resp.Body.Close()
	require.NoError(t, testutil.RequireReceive(ctx, t, writeErr))
	return clientSide, br, connectResult{ok: true, status: resp.StatusCode, header: resp.Header}, done
}

// connectStatus is connect for the agent's own address, requiring the given
// response status.
func (h *proxyHarness) connectStatus(ctx context.Context, t *testing.T, target string, status int, headers ...http.Header) (net.Conn, *bufio.Reader, connectResult, <-chan struct{}) {
	t.Helper()
	client, br, resp, done := h.connect(ctx, t, h.src, target, headers...)
	require.True(t, resp.ok)
	require.Equal(t, status, resp.status)
	return client, br, resp, done
}

// requireEOF asserts the proxy closed the client connection.
func requireEOF(t *testing.T, r io.Reader) {
	t.Helper()
	_, err := r.Read(make([]byte, 1))
	require.ErrorIs(t, err, io.EOF)
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

// startEchoHostServer serves HTTP that reports the Host header it received.
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
		name string
		// target builds the CONNECT target from the upstream address.
		target   func(netip.AddrPort) string
		resolver func(netip.AddrPort) fakeResolver
		// hostHeader is what the tunneled HTTP request carries.
		hostHeader string
	}{
		{
			// The Host header is what the policy sees in phase two.
			name:       "IPLiteralSniffsHost",
			target:     func(ap netip.AddrPort) string { return ap.String() },
			resolver:   func(netip.AddrPort) fakeResolver { return nil },
			hostHeader: "api.example.com",
		},
		{
			// The Host header names a host the policy would deny, proving the
			// CONNECT hostname is authoritative and nothing is sniffed. The
			// resolver hands back IPv6 first to prove IPv4 is preferred.
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
			})
			client, br, _, done := h.connectStatus(ctx, t, tt.target(upstream), http.StatusOK)

			// Connection: close makes the upstream hang up so both copy
			// directions finish deterministically.
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

// TestConnectProxy_CustomPolicy plugs a PolicyFunc into the proxy to prove the
// Policy interface is the extension seam: no YAML is involved, and the
// custom decision drives both the CONNECT response and the flow report.
func TestConnectProxy_CustomPolicy(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	upstream := startEchoHostServer(t)

	var seen []exitnode.FlowInfo
	var mu sync.Mutex
	onlyUpstreamPort := exitnode.PolicyFunc(func(flow exitnode.FlowInfo) exitnode.Decision {
		mu.Lock()
		seen = append(seen, flow)
		mu.Unlock()
		if flow.Port == int(upstream.Port()) {
			return exitnode.Decision{Allow: true, RuleID: "custom-allow", Reason: "upstream port"}
		}
		return exitnode.Decision{RuleID: "custom-deny", Reason: "not the upstream port"}
	})
	h := newProxyHarness(t, allowExamplePolicy, func(o *exitnode.ConnectProxyOptions) {
		o.Policy = onlyUpstreamPort
	})

	// Allowed: the YAML harness policy would deny a bare IP under default
	// deny, so a 200 proves the custom policy is the one consulted.
	client, br, _, done := h.connectStatus(ctx, t, upstream.String(), http.StatusOK)
	go func() {
		_, _ = io.WriteString(client, "GET /x HTTP/1.1\r\nHost: any.test\r\nConnection: close\r\n\r\n")
	}()
	require.Equal(t, "hello from any.test", readTunneled(t, br))
	_ = client.Close()
	testutil.RequireReceive(ctx, t, done)
	report := testutil.RequireReceive(ctx, t, h.flows.ch)
	require.Equal(t, codersdk.ExitNodeFlowAllow, report.Decision)
	require.Equal(t, "custom-allow", report.RuleID)
	require.NotNil(t, testutil.RequireReceive(ctx, t, h.flows.ch).DisconnectTime)

	// Denied: the custom rule ID and reason surface on the response.
	_, br, resp, done := h.connectStatus(ctx, t, "127.0.0.1:1", http.StatusForbidden)
	require.Equal(t, "not the upstream port", resp.header.Get("X-Coder-Deny-Reason"))
	requireEOF(t, br)
	testutil.RequireReceive(ctx, t, done)
	report = testutil.RequireReceive(ctx, t, h.flows.ch)
	require.Equal(t, codersdk.ExitNodeFlowDeny, report.Decision)
	require.Equal(t, "custom-deny", report.RuleID)

	mu.Lock()
	defer mu.Unlock()
	require.NotEmpty(t, seen)
	for _, flow := range seen {
		require.Equal(t, netip.MustParseAddr("127.0.0.1"), flow.IP.Unmap())
	}
}

// TestConnectProxy_DenyBeforeConnect covers denials that are final before
// the 200: by CIDR, by a CONNECT hostname, and by the original host header
// on an IP literal, each answered with a 403 and reported.
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
			// An IP literal with the original host supplied is decided up
			// front instead of waiting for a sniff.
			name:     "OriginalHostHeader",
			policy:   noEvilPolicy,
			target:   "192.0.2.1:443",
			headers:  http.Header{codersdk.ExitNodeOriginalHostHeader: []string{"evil.example"}},
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

			// The proxy closes after the 403.
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

	h := newProxyHarness(t, noEvilPolicy)
	// Phase one cannot know the host, so the CONNECT is provisionally
	// accepted.
	client, _, _, done := h.connectStatus(ctx, t, ln.Addr().String(), http.StatusOK)

	go func() {
		_, _ = io.WriteString(client, "GET / HTTP/1.1\r\nHost: evil.example\r\n\r\n")
	}()
	// Phase two denies and the only signal left is a closed connection.
	requireEOF(t, client)
	testutil.RequireReceive(ctx, t, done)

	report := testutil.RequireReceive(ctx, t, h.flows.ch)
	require.Equal(t, codersdk.ExitNodeFlowDeny, report.Decision)
	require.Equal(t, "no-evil", report.RuleID)
	require.Equal(t, "evil.example", report.Host)
}

func TestConnectProxy_ResolveFailure(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)

	h := newProxyHarness(t, "default: allow\n")
	client, _, resp, done := h.connectStatus(ctx, t, "nxdomain.example:443", http.StatusBadGateway)
	require.Equal(t, "resolve failed", resp.header.Get(codersdk.ExitNodeDenyReasonHeader))
	requireEOF(t, client)
	testutil.RequireReceive(ctx, t, done)
	require.Empty(t, h.flows.reports)
}

func TestConnectProxy_UnknownSourceRejected(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)

	h := newProxyHarness(t, "default: allow\n")
	stranger := tailnet.TailscaleServicePrefix.AddrFromUUID(uuid.New())
	client, _, resp, done := h.connect(ctx, t, stranger, "127.0.0.1:80")
	require.False(t, resp.ok, "unknown sources get no HTTP response at all")
	requireEOF(t, client)
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

func TestConnectProxy_Serve(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(testutil.Context(t, testutil.WaitLong))
	defer cancel()
	upstream := startEchoHostServer(t)

	// Serve over a loopback listener; the source is loopback, so attribute
	// anything from 127.0.0.1 to the agent.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	h := newProxyHarness(t, "default: allow\n")
	proxy := exitnode.NewConnectProxy(exitnode.ConnectProxyOptions{
		Logger:  testutil.Logger(t),
		Policy:  mustPolicy(t, "default: allow\n"),
		Agents:  loopbackResolver{agentID: h.agentID},
		Flows:   h.flows,
		Metrics: h.metrics,
	})
	serveErr := make(chan error, 1)
	go func() { serveErr <- proxy.Serve(ctx, ln) }()

	conn, err := net.Dial("tcp", ln.Addr().String())
	require.NoError(t, err)
	defer conn.Close()
	_, err = fmt.Fprintf(conn, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", upstream, upstream)
	require.NoError(t, err)
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodConnect})
	require.NoError(t, err)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	_, err = io.WriteString(conn, "GET / HTTP/1.1\r\nHost: anything\r\nConnection: close\r\n\r\n")
	require.NoError(t, err)
	require.Equal(t, "hello from anything", readTunneled(t, br))

	cancel()
	require.NoError(t, testutil.RequireReceive(testutil.Context(t, testutil.WaitShort), t, serveErr))
	proxy.Wait()
}

// loopbackResolver attributes any loopback source to a fixed agent, so the
// accept loop can be exercised over real TCP without a tailnet.
type loopbackResolver struct {
	agentID uuid.UUID
}

func (r loopbackResolver) AgentForAddr(addr netip.Addr) (uuid.UUID, bool) {
	return r.agentID, addr.IsLoopback()
}

package agentegress_test

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agentegress"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/testutil"
)

// fakeExitNode is an in-memory exit node that speaks the CONNECT protocol
// over a loopback listener. Destinations are allowed unless deny returns a
// reason.
type fakeExitNode struct {
	t        testing.TB
	listener net.Listener
	deny     func(dst string) (reason string, denied bool)

	mu    sync.Mutex
	hosts map[string]string // dst -> X-Coder-Original-Host
	wg    sync.WaitGroup
}

func newFakeExitNode(t testing.TB, deny func(dst string) (string, bool)) *fakeExitNode {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	if deny == nil {
		deny = func(string) (string, bool) { return "", false }
	}
	f := &fakeExitNode{t: t, listener: ln, deny: deny, hosts: map[string]string{}}
	f.wg.Add(1)
	go f.serve()
	t.Cleanup(func() {
		_ = ln.Close()
		f.wg.Wait()
	})
	return f
}

func (f *fakeExitNode) serve() {
	defer f.wg.Done()
	for {
		conn, err := f.listener.Accept()
		if err != nil {
			return
		}
		f.wg.Add(1)
		go func() {
			defer f.wg.Done()
			f.handle(conn)
		}()
	}
}

func (f *fakeExitNode) handle(conn net.Conn) {
	defer conn.Close()
	br := bufio.NewReader(conn)
	req, err := http.ReadRequest(br)
	if err != nil {
		return
	}
	if req.Method != http.MethodConnect {
		_, _ = io.WriteString(conn, "HTTP/1.1 405 Method Not Allowed\r\nContent-Length: 0\r\n\r\n")
		return
	}
	dst := req.Host
	f.mu.Lock()
	f.hosts[dst] = req.Header.Get(agentegress.HeaderOriginalHost)
	f.mu.Unlock()

	if reason, denied := f.deny(dst); denied {
		_, _ = io.WriteString(conn, "HTTP/1.1 403 Forbidden\r\n"+agentegress.HeaderDenyReason+": "+reason+"\r\nContent-Length: 0\r\n\r\n")
		return
	}
	target, err := net.Dial("tcp", dst)
	if err != nil {
		_, _ = io.WriteString(conn, "HTTP/1.1 502 Bad Gateway\r\nContent-Length: 0\r\n\r\n")
		return
	}
	defer target.Close()
	if _, err := io.WriteString(conn, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		return
	}
	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(target, br)
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(conn, target)
		done <- struct{}{}
	}()
	<-done
}

// DialContextTCP implements agentegress.Dialer by ignoring the tailnet
// address and connecting to the fake listener instead.
func (f *fakeExitNode) DialContextTCP(ctx context.Context, _ netip.AddrPort) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, "tcp", f.listener.Addr().String())
}

func (f *fakeExitNode) originalHost(dst string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.hosts[dst]
}

// staticResolver maps hostnames to fixed addresses so tests do not depend
// on the host's DNS.
type staticResolver map[string]netip.Addr

func (r staticResolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	addr, ok := r[host]
	if !ok {
		return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
	}
	return []netip.Addr{addr}, nil
}

func startProxy(t testing.TB, exit *fakeExitNode, resolver agentegress.Resolver) *agentegress.Proxy {
	t.Helper()
	proxy, err := agentegress.New(testutil.Logger(t), agentegress.Options{
		Dialer: exit,
		Config: agentsdk.EgressConfig{
			ExitNodeID:   uuid.New(),
			ExitNodePort: 3128,
		},
		Resolver: resolver,
	})
	require.NoError(t, err)
	require.NoError(t, proxy.Start(testutil.Context(t, testutil.WaitLong)))
	t.Cleanup(func() { _ = proxy.Close() })
	return proxy
}

func proxyURL(t testing.TB, proxy *agentegress.Proxy) *url.URL {
	t.Helper()
	u, err := url.Parse("http://" + proxy.Addr().String())
	require.NoError(t, err)
	return u
}

func TestProxy_ConnectAllowed(t *testing.T) {
	t.Parallel()

	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "hello over "+r.Proto)
	}))
	t.Cleanup(origin.Close)

	exit := newFakeExitNode(t, nil)
	proxy := startProxy(t, exit, nil)

	// httptest's client transport carries the server's certificate; reuse
	// it so CONNECT is followed by a real TLS handshake through the tunnel.
	transport, ok := origin.Client().Transport.(*http.Transport)
	require.True(t, ok)
	transport = transport.Clone()
	transport.Proxy = http.ProxyURL(proxyURL(t, proxy))
	client := &http.Client{Transport: transport}

	ctx := testutil.Context(t, testutil.WaitShort)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, origin.URL+"/", nil)
	require.NoError(t, err)
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.True(t, strings.HasPrefix(string(body), "hello over "), string(body))

	// The origin was addressed by IP so no hostname is forwarded.
	originAddr := strings.TrimPrefix(origin.URL, "https://")
	require.Equal(t, "", exit.originalHost(originAddr))
}

func TestProxy_ConnectDenied(t *testing.T) {
	t.Parallel()

	exit := newFakeExitNode(t, func(string) (string, bool) {
		return "blocked by policy", true
	})
	proxy := startProxy(t, exit, nil)

	ctx := testutil.Context(t, testutil.WaitShort)
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", proxy.Addr().String())
	require.NoError(t, err)
	defer conn.Close()

	connect := &http.Request{
		Method: http.MethodConnect,
		URL:    &url.URL{Opaque: "192.0.2.10:443"},
		Host:   "192.0.2.10:443",
		Header: http.Header{},
	}
	require.NoError(t, connect.Write(conn))
	resp, err := http.ReadResponse(bufio.NewReader(conn), connect)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	require.Equal(t, "blocked by policy", resp.Header.Get(agentegress.HeaderDenyReason))
}

func TestProxy_ConnectResolvesHostname(t *testing.T) {
	t.Parallel()

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(origin.Close)
	originAddr := strings.TrimPrefix(origin.URL, "http://")
	_, originPort, err := net.SplitHostPort(originAddr)
	require.NoError(t, err)

	exit := newFakeExitNode(t, nil)
	proxy := startProxy(t, exit, staticResolver{
		"origin.test": netip.MustParseAddr("127.0.0.1"),
	})

	ctx := testutil.Context(t, testutil.WaitShort)
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", proxy.Addr().String())
	require.NoError(t, err)
	defer conn.Close()

	connect := &http.Request{
		Method: http.MethodConnect,
		URL:    &url.URL{Opaque: "origin.test:" + originPort},
		Host:   "origin.test:" + originPort,
		Header: http.Header{},
	}
	require.NoError(t, connect.Write(conn))
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, connect)
	require.NoError(t, err)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Speak plain HTTP through the established tunnel.
	inner, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://origin.test/", nil)
	require.NoError(t, err)
	require.NoError(t, inner.Write(conn))
	innerResp, err := http.ReadResponse(br, inner)
	require.NoError(t, err)
	_ = innerResp.Body.Close()
	require.Equal(t, http.StatusNoContent, innerResp.StatusCode)

	// The exit node saw the resolved IP as the CONNECT target and the
	// hostname in the original host header.
	require.Equal(t, "origin.test", exit.originalHost("127.0.0.1:"+originPort))
}

func TestProxy_AbsoluteURI(t *testing.T) {
	t.Parallel()

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A well-formed origin-form request has a relative RequestURI.
		assert.Equal(t, "/path?q=1", r.RequestURI)
		assert.Empty(t, r.Header.Get("Proxy-Connection"))
		_, _ = io.WriteString(w, "plain http body")
	}))
	t.Cleanup(origin.Close)

	exit := newFakeExitNode(t, nil)
	proxy := startProxy(t, exit, nil)

	client := &http.Client{Transport: &http.Transport{
		Proxy:             http.ProxyURL(proxyURL(t, proxy)),
		DisableKeepAlives: true,
	}}
	ctx := testutil.Context(t, testutil.WaitShort)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, origin.URL+"/path?q=1", nil)
	require.NoError(t, err)
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "plain http body", string(body))
}

func TestProxy_AbsoluteURIDenied(t *testing.T) {
	t.Parallel()

	exit := newFakeExitNode(t, func(string) (string, bool) {
		return "no plain http", true
	})
	proxy := startProxy(t, exit, nil)

	client := &http.Client{Transport: &http.Transport{
		Proxy:             http.ProxyURL(proxyURL(t, proxy)),
		DisableKeepAlives: true,
	}}
	ctx := testutil.Context(t, testutil.WaitShort)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://192.0.2.10/", nil)
	require.NoError(t, err)
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	require.Equal(t, "no plain http", resp.Header.Get(agentegress.HeaderDenyReason))
}

func TestProxy_CloseStopsListening(t *testing.T) {
	t.Parallel()

	exit := newFakeExitNode(t, nil)
	proxy := startProxy(t, exit, nil)
	addr := proxy.Addr().String()
	require.NoError(t, proxy.Close())
	// Idempotent.
	require.NoError(t, proxy.Close())

	ctx := testutil.Context(t, testutil.WaitShort)
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err == nil {
		_ = conn.Close()
	}
	require.Error(t, err)
}

func TestNew_Validation(t *testing.T) {
	t.Parallel()

	exit := newFakeExitNode(t, nil)
	_, err := agentegress.New(testutil.Logger(t), agentegress.Options{
		Config: agentsdk.EgressConfig{ExitNodeID: uuid.New(), ExitNodePort: 3128},
	})
	require.ErrorContains(t, err, "dialer")

	_, err = agentegress.New(testutil.Logger(t), agentegress.Options{
		Dialer: exit,
		Config: agentsdk.EgressConfig{ExitNodePort: 3128},
	})
	require.ErrorContains(t, err, "exit node ID")

	_, err = agentegress.New(testutil.Logger(t), agentegress.Options{
		Dialer: exit,
		Config: agentsdk.EgressConfig{ExitNodeID: uuid.New()},
	})
	require.ErrorContains(t, err, "port")
}

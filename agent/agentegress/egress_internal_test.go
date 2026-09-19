package agentegress

import (
	"bufio"
	"cmp"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/dns/dnsmessage"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

const established = "HTTP/1.1 200 Connection Established\r\n\r\n"

// serveTCP accepts loopback connections and hands each to handle until the
// test ends.
func serveTCP(t testing.TB, handle func(net.Conn)) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	var wg sync.WaitGroup
	wg.Go(func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			wg.Go(func() {
				defer conn.Close()
				handle(conn)
			})
		}
	})
	t.Cleanup(func() {
		_ = ln.Close()
		wg.Wait()
	})
	return ln
}

// connectRecord is one CONNECT the fake exit node received.
type connectRecord struct {
	target string
	proto  codersdk.ExitNodeProtocol
}

// fakeExitNode is an in-memory exit node that speaks the CONNECT protocol
// over a loopback listener, including the udp (echo) and dns (TXT answer)
// stream variants. Destinations are allowed unless deny returns a reason.
type fakeExitNode struct {
	listener net.Listener
	deny     func(target string) (reason string, denied bool)
	// resolve maps CONNECT targets to the address actually dialed, standing
	// in for the exit node's own resolver.
	resolve map[string]string
	// dnsReverseBatch, when > 1, holds that many DNS queries and answers
	// them in reverse order to exercise message ID correlation.
	dnsReverseBatch int

	mu       sync.Mutex
	connects []connectRecord
	// udpFrames records relayed datagrams by target.
	udpFrames map[string][][]byte
}

func newFakeExitNode(t testing.TB, deny func(target string) (string, bool)) *fakeExitNode {
	t.Helper()
	if deny == nil {
		deny = func(string) (string, bool) { return "", false }
	}
	f := &fakeExitNode{deny: deny, resolve: map[string]string{}, udpFrames: map[string][][]byte{}}
	f.listener = serveTCP(t, f.handle)
	return f
}

func (f *fakeExitNode) handle(conn net.Conn) {
	br := bufio.NewReader(conn)
	req, err := http.ReadRequest(br)
	if err != nil || req.Method != http.MethodConnect {
		return
	}
	target := req.Host
	proto := codersdk.ExitNodeProtocol(req.Header.Get(codersdk.ExitNodeProtocolHeader))
	f.mu.Lock()
	f.connects = append(f.connects, connectRecord{target: target, proto: proto})
	f.mu.Unlock()

	if reason, denied := f.deny(target); denied {
		_, _ = fmt.Fprintf(conn, "HTTP/1.1 403 Forbidden\r\n%s: %s\r\n%s: rule-1\r\nContent-Length: 0\r\n\r\n",
			codersdk.ExitNodeDenyReasonHeader, reason, codersdk.ExitNodeDenyRuleHeader)
		return
	}
	if proto == "" {
		dst, err := net.Dial("tcp", cmp.Or(f.resolve[target], target))
		if err != nil {
			_, _ = io.WriteString(conn, "HTTP/1.1 502 Bad Gateway\r\nContent-Length: 0\r\n\r\n")
			return
		}
		defer dst.Close()
		if _, err := io.WriteString(conn, established); err != nil {
			return
		}
		go func() {
			_, _ = io.Copy(dst, br)
			closeWrite(dst)
		}()
		_, _ = io.Copy(conn, dst)
		return
	}
	if _, err := io.WriteString(conn, established); err != nil {
		return
	}
	var batch [][]byte
	for {
		payload, err := readFrame(br)
		if err != nil {
			return
		}
		if proto == codersdk.ExitNodeProtocolUDP {
			f.mu.Lock()
			f.udpFrames[target] = append(f.udpFrames[target], payload)
			f.mu.Unlock()
			if writeFrame(conn, append([]byte("echo:"), payload...)) != nil {
				return
			}
			continue
		}
		if batch = append(batch, payload); len(batch) < max(f.dnsReverseBatch, 1) {
			continue
		}
		slices.Reverse(batch)
		for _, query := range batch {
			if writeFrame(conn, dnsAnswer(query, &dnsmessage.TXTResource{TXT: []string{"via-exit-node"}})) != nil {
				return
			}
		}
		batch = nil
	}
}

// DialContextTCP implements Dialer by ignoring the tailnet address and
// connecting to the fake listener instead.
func (f *fakeExitNode) DialContextTCP(ctx context.Context, _ netip.AddrPort) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, "tcp", f.listener.Addr().String())
}

func (f *fakeExitNode) connectsTo(target string) []connectRecord {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []connectRecord
	for _, c := range f.connects {
		if c.target == target {
			out = append(out, c)
		}
	}
	return out
}

func (f *fakeExitNode) udpFramesFor(target string) [][]byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.udpFrames[target])
}

type proxyOptions struct {
	exemptHosts []string
	upstream    []netip.AddrPort
	clock       quartz.Clock
	udpOrigDst  func(oob []byte) netip.AddrPort
}

func startProxy(t testing.TB, exit *fakeExitNode, opts proxyOptions) *Proxy {
	t.Helper()
	if opts.upstream == nil {
		// Never touch the host's resolvers from tests.
		opts.upstream = []netip.AddrPort{}
	}
	proxy, err := New(testutil.Logger(t), Options{
		Dialer:            exit,
		Config:            agentsdk.EgressConfig{ExitNodeID: uuid.New(), ExitNodePort: 3128},
		ExemptHosts:       opts.exemptHosts,
		UpstreamResolvers: opts.upstream,
		Clock:             opts.clock,
	})
	require.NoError(t, err)
	if opts.udpOrigDst != nil {
		proxy.udpOrigDst = opts.udpOrigDst
	}
	require.NoError(t, proxy.Start(testutil.Context(t, testutil.WaitLong)))
	t.Cleanup(func() { _ = proxy.Close() })
	return proxy
}

func proxyURL(t testing.TB, proxy *Proxy) *url.URL {
	t.Helper()
	u, err := url.Parse("http://" + proxy.Addr().String())
	require.NoError(t, err)
	return u
}

// connectVia sends a CONNECT for target through the proxy, asserts the
// response status and returns the connection, its buffered reader and the
// response headers.
func connectVia(ctx context.Context, t testing.TB, proxy *Proxy, target string, wantStatus int) (net.Conn, *bufio.Reader, http.Header) {
	t.Helper()
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", proxy.Addr().String())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	req := &http.Request{Method: http.MethodConnect, URL: &url.URL{Opaque: target}, Host: target, Header: http.Header{}}
	require.NoError(t, req.Write(conn))
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, req)
	require.NoError(t, err)
	_ = resp.Body.Close()
	require.Equal(t, wantStatus, resp.StatusCode)
	return conn, br, resp.Header
}

// originPort returns the port of an httptest server.
func originPort(t testing.TB, origin *httptest.Server) string {
	t.Helper()
	_, port, err := net.SplitHostPort(strings.TrimPrefix(strings.TrimPrefix(origin.URL, "https://"), "http://"))
	require.NoError(t, err)
	return port
}

func TestProxy_ConnectAllowed(t *testing.T) {
	t.Parallel()

	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "hello over "+r.Proto)
	}))
	t.Cleanup(origin.Close)

	exit := newFakeExitNode(t, nil)
	proxy := startProxy(t, exit, proxyOptions{})

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

	// The origin was addressed by IP, so the CONNECT target is the IP and
	// no protocol header is sent for plain TCP.
	connects := exit.connectsTo(strings.TrimPrefix(origin.URL, "https://"))
	require.Len(t, connects, 1)
	require.Equal(t, codersdk.ExitNodeProtocol(""), connects[0].proto)
}

func TestProxy_ConnectDenied(t *testing.T) {
	t.Parallel()

	exit := newFakeExitNode(t, func(string) (string, bool) { return "blocked by policy", true })
	proxy := startProxy(t, exit, proxyOptions{})

	ctx := testutil.Context(t, testutil.WaitShort)
	_, _, hdr := connectVia(ctx, t, proxy, "192.0.2.10:443", http.StatusForbidden)
	require.Equal(t, "blocked by policy", hdr.Get(codersdk.ExitNodeDenyReasonHeader))
	require.Equal(t, "rule-1", hdr.Get(codersdk.ExitNodeDenyRuleHeader))
}

func TestProxy_ConnectTarget(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		// host is what the client puts in its CONNECT target.
		host func(proxy *Proxy) string
		// want is the hostname the exit node must see.
		want string
	}{
		{
			name: "passes hostname",
			host: func(*Proxy) string { return "Origin.TEST" },
			want: "origin.test",
		},
		{
			// A client that resolved through the proxy's DNS and then
			// CONNECTs to the fake IP literal still yields a hostname.
			name: "translates fake IP",
			host: func(p *Proxy) string { return p.fake.Lookup("fake.test").String() },
			want: "fake.test",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			}))
			t.Cleanup(origin.Close)
			port := originPort(t, origin)

			exit := newFakeExitNode(t, nil)
			// The exit node, not the proxy, resolves the name.
			exit.resolve[tt.want+":"+port] = strings.TrimPrefix(origin.URL, "http://")
			proxy := startProxy(t, exit, proxyOptions{})

			ctx := testutil.Context(t, testutil.WaitShort)
			conn, br, _ := connectVia(ctx, t, proxy, net.JoinHostPort(tt.host(proxy), port), http.StatusOK)

			// Speak plain HTTP through the established tunnel.
			inner, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://origin.test/", nil)
			require.NoError(t, err)
			require.NoError(t, inner.Write(conn))
			innerResp, err := http.ReadResponse(br, inner)
			require.NoError(t, err)
			_ = innerResp.Body.Close()
			require.Equal(t, http.StatusNoContent, innerResp.StatusCode)
			require.Len(t, exit.connectsTo(tt.want+":"+port), 1)
		})
	}
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

	for _, tt := range []struct {
		name       string
		denyReason string
		url        string
		wantStatus int
		wantBody   string
	}{
		{name: "allowed", url: origin.URL + "/path?q=1", wantStatus: http.StatusOK, wantBody: "plain http body"},
		{name: "denied", denyReason: "no plain http", url: "http://192.0.2.10/", wantStatus: http.StatusForbidden},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			exit := newFakeExitNode(t, func(string) (string, bool) { return tt.denyReason, tt.denyReason != "" })
			proxy := startProxy(t, exit, proxyOptions{})
			client := &http.Client{Transport: &http.Transport{
				Proxy:             http.ProxyURL(proxyURL(t, proxy)),
				DisableKeepAlives: true,
			}}

			ctx := testutil.Context(t, testutil.WaitShort)
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, tt.url, nil)
			require.NoError(t, err)
			resp, err := client.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.Equal(t, tt.wantStatus, resp.StatusCode)
			require.Equal(t, tt.denyReason, resp.Header.Get(codersdk.ExitNodeDenyReasonHeader))
			if tt.wantBody != "" {
				require.Equal(t, tt.wantBody, string(body))
			}
		})
	}
}

func TestProxy_CloseStopsListening(t *testing.T) {
	t.Parallel()

	exit := newFakeExitNode(t, nil)
	proxy := startProxy(t, exit, proxyOptions{})
	addr, dnsAddr := proxy.Addr(), proxy.DNSAddr()
	require.True(t, dnsAddr.IsValid())
	require.True(t, proxy.UDPAddr().IsValid())
	require.NoError(t, proxy.Close())
	// Idempotent.
	require.NoError(t, proxy.Close())

	ctx := testutil.Context(t, testutil.WaitShort)
	var d net.Dialer
	for _, closed := range []netip.AddrPort{addr, dnsAddr} {
		conn, err := d.DialContext(ctx, "tcp", closed.String())
		if err == nil {
			_ = conn.Close()
		}
		require.Error(t, err, closed)
	}
}

func TestNew_Validation(t *testing.T) {
	t.Parallel()

	exit := newFakeExitNode(t, nil)
	for _, tt := range []struct {
		name    string
		opts    Options
		wantErr string
	}{
		{name: "dialer", opts: Options{Config: agentsdk.EgressConfig{ExitNodeID: uuid.New(), ExitNodePort: 3128}}, wantErr: "dialer"},
		{name: "exit node ID", opts: Options{Dialer: exit, Config: agentsdk.EgressConfig{ExitNodePort: 3128}}, wantErr: "exit node ID"},
		{name: "port", opts: Options{Dialer: exit, Config: agentsdk.EgressConfig{ExitNodeID: uuid.New()}}, wantErr: "port"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := New(testutil.Logger(t), tt.opts)
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

package agentegress

import (
	"bufio"
	"cmp"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/dns/dnsmessage"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

const established = "HTTP/1.1 200 Connection Established\r\n\r\n"

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

type connectRecord struct {
	target       string
	proto        codersdk.ExitNodeProtocol
	originalHost string
}

type fakeExitNode struct {
	listener        net.Listener
	deny            func(target string) (reason string, denied bool)
	resolve         map[string]string
	dnsReverseBatch int
	dnsResponse     func(query []byte) []byte

	mu        sync.Mutex
	connects  []connectRecord
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
	originalHost := req.Header.Get(codersdk.ExitNodeOriginalHostHeader)
	f.mu.Lock()
	f.connects = append(f.connects, connectRecord{target: target, proto: proto, originalHost: originalHost})
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
			resp := dnsAnswer(query, &dnsmessage.TXTResource{TXT: []string{"via-exit-node"}})
			if f.dnsResponse != nil {
				resp = f.dnsResponse(query)
			}
			if writeFrame(conn, resp) != nil {
				return
			}
		}
		batch = nil
	}
}

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
	exemptHosts      []string
	upstream         []netip.AddrPort
	clock            quartz.Clock
	udpOrigDst       func(oob []byte) netip.AddrPort
	hostSniffTimeout time.Duration
	fakeIPMaxEntries int
	dnsConcurrency   int
	udpSessions      int
	udpDenyEntries   int
}

func startProxy(t testing.TB, exit *fakeExitNode, opts proxyOptions) *Proxy {
	t.Helper()
	if opts.upstream == nil {
		opts.upstream = []netip.AddrPort{}
	}
	proxy, err := New(testutil.Logger(t), Options{
		Dialer:            exit,
		Config:            agentsdk.EgressConfig{ExitNodes: []agentsdk.EgressExitNode{{ID: uuid.New(), ReplicaIDs: []uuid.UUID{uuid.New()}}}, ExitNodePort: 3128},
		ListenAddr:        "127.0.0.1:0",
		ExemptHosts:       opts.exemptHosts,
		UpstreamResolvers: opts.upstream,
		Clock:             opts.clock,
		HostSniffTimeout:  opts.hostSniffTimeout,
		FakeIPMaxEntries:  opts.fakeIPMaxEntries,
	})
	require.NoError(t, err)
	proxy.resolverDial = (&net.Dialer{}).DialContext
	if opts.udpOrigDst != nil {
		proxy.udpOrigDst = opts.udpOrigDst
	}
	if opts.dnsConcurrency > 0 {
		proxy.dnsConcurrency = opts.dnsConcurrency
	}
	if opts.udpSessions > 0 {
		proxy.udpSessions = opts.udpSessions
	}
	if opts.udpDenyEntries > 0 {
		proxy.udpDenyEntries = opts.udpDenyEntries
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

func originPort(t testing.TB, origin *httptest.Server) string {
	t.Helper()
	_, port, err := net.SplitHostPort(strings.TrimPrefix(strings.TrimPrefix(origin.URL, "https://"), "http://"))
	require.NoError(t, err)
	return port
}

func TestProxy_TransparentHostSniff(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name     string
		wantHost string
		run      func(context.Context, *testing.T, net.Conn, *bufio.Reader)
	}{
		{
			name:     "HTTP",
			wantHost: "web.example.com",
			run: func(ctx context.Context, t *testing.T, conn net.Conn, br *bufio.Reader) {
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://web.example.com/path", nil)
				require.NoError(t, err)
				require.NoError(t, req.Write(conn))
				resp, err := http.ReadResponse(br, req)
				require.NoError(t, err)
				defer resp.Body.Close()
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				require.Equal(t, "received:/path", string(body))
			},
		},
		{
			name:     "TLS",
			wantHost: "tls.example.com",
			run: func(ctx context.Context, t *testing.T, conn net.Conn, _ *bufio.Reader) {
				tlsConn := tls.Client(conn, &tls.Config{
					ServerName: "tls.example.com",
					MinVersion: tls.VersionTLS12,
					//nolint:gosec // The test server uses a generated certificate.
					InsecureSkipVerify: true,
				})
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://tls.example.com/path", nil)
				require.NoError(t, err)
				require.NoError(t, req.Write(tlsConn))
				resp, err := http.ReadResponse(bufio.NewReader(tlsConn), req)
				require.NoError(t, err)
				defer resp.Body.Close()
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				require.Equal(t, "received:/path", string(body))
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.WriteString(w, "received:"+r.URL.Path)
			})
			var origin *httptest.Server
			if tt.name == "TLS" {
				origin = httptest.NewTLSServer(handler)
			} else {
				origin = httptest.NewServer(handler)
			}
			t.Cleanup(origin.Close)
			dst := netip.MustParseAddrPort(strings.TrimPrefix(strings.TrimPrefix(origin.URL, "https://"), "http://"))

			exit := newFakeExitNode(t, nil)
			exit.resolve[dst.String()] = dst.String()
			proxy := startProxy(t, exit, proxyOptions{})
			client, server := net.Pipe()
			t.Cleanup(func() { _ = client.Close() })
			ctx := testutil.Context(t, testutil.WaitShort)
			handleCtx := testutil.Context(t, testutil.WaitLong)
			done := make(chan struct{}, 1)
			go func() {
				proxy.handleTransparent(handleCtx, server, dst)
				_ = server.Close()
				done <- struct{}{}
			}()

			tt.run(ctx, t, client, bufio.NewReader(client))
			_ = client.Close()
			testutil.RequireReceive(ctx, t, done)

			connects := exit.connectsTo(dst.String())
			require.Len(t, connects, 1)
			require.Equal(t, tt.wantHost, connects[0].originalHost)
		})
	}
}

func TestProxy_TransparentSilentClient(t *testing.T) {
	t.Parallel()

	origin := serveTCP(t, func(conn net.Conn) {
		_, _ = io.WriteString(conn, "server-first")
	})
	dst := netip.MustParseAddrPort(origin.Addr().String())
	exit := newFakeExitNode(t, nil)
	proxy := startProxy(t, exit, proxyOptions{hostSniffTimeout: testutil.IntervalFast})
	client, server := net.Pipe()
	t.Cleanup(func() { _ = client.Close() })
	handleCtx := testutil.Context(t, testutil.WaitLong)
	go func() {
		proxy.handleTransparent(handleCtx, server, dst)
		_ = server.Close()
	}()

	ctx := testutil.Context(t, testutil.WaitShort)
	deadline, _ := ctx.Deadline()
	require.NoError(t, client.SetReadDeadline(deadline))
	got := make([]byte, len("server-first"))
	_, err := io.ReadFull(client, got)
	require.NoError(t, err)
	require.Equal(t, "server-first", string(got))

	connects := exit.connectsTo(dst.String())
	require.Len(t, connects, 1)
	require.Empty(t, connects[0].originalHost)
}

func TestProxy_TransparentUnknownFakeIPDropped(t *testing.T) {
	t.Parallel()

	exit := newFakeExitNode(t, nil)
	proxy := startProxy(t, exit, proxyOptions{})
	client, server := net.Pipe()
	handleCtx := testutil.Context(t, testutil.WaitLong)
	done := make(chan struct{}, 1)
	go func() {
		proxy.handleTransparent(handleCtx, server, netip.MustParseAddrPort("198.18.0.1:443"))
		_ = server.Close()
		done <- struct{}{}
	}()

	ctx := testutil.Context(t, testutil.WaitShort)
	testutil.RequireReceive(ctx, t, done)
	require.NoError(t, client.Close())
	require.Empty(t, exit.connects)
	require.Equal(t, int64(1), proxy.unknownFake.Load())
}

func TestProxy_ConnectAllowed(t *testing.T) {
	t.Parallel()

	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "hello over "+r.Proto)
	}))
	t.Cleanup(origin.Close)

	exit := newFakeExitNode(t, nil)
	proxy := startProxy(t, exit, proxyOptions{})

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

	connects := exit.connectsTo(strings.TrimPrefix(origin.URL, "https://"))
	require.Len(t, connects, 1)
	require.Equal(t, codersdk.ExitNodeProtocol(""), connects[0].proto)
	require.Empty(t, connects[0].originalHost)
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
		host func(proxy *Proxy) string
		want string
	}{
		{
			name: "passes hostname",
			host: func(*Proxy) string { return "Origin.TEST" },
			want: "origin.test",
		},
		{
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
			exit.resolve[tt.want+":"+port] = strings.TrimPrefix(origin.URL, "http://")
			proxy := startProxy(t, exit, proxyOptions{})

			ctx := testutil.Context(t, testutil.WaitShort)
			conn, br, _ := connectVia(ctx, t, proxy, net.JoinHostPort(tt.host(proxy), port), http.StatusOK)

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

func TestProxy_NoLiveReplicasRecoversAfterUpdate(t *testing.T) {
	t.Parallel()

	exit := newFakeExitNode(t, nil)
	proxy, err := New(testutil.Logger(t), Options{
		Dialer: exit,
		Config: agentsdk.EgressConfig{
			ExitNodes:    []agentsdk.EgressExitNode{{ID: uuid.New()}},
			ExitNodePort: 3128,
		},
		ListenAddr:        "127.0.0.1:0",
		UpstreamResolvers: []netip.AddrPort{},
	})
	require.NoError(t, err)
	require.NoError(t, proxy.Start(testutil.Context(t, testutil.WaitLong)))
	t.Cleanup(func() { _ = proxy.Close() })

	ctx := testutil.Context(t, testutil.WaitShort)
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", proxy.Addr().String())
	require.NoError(t, err)
	req := &http.Request{Method: http.MethodConnect, URL: &url.URL{Opaque: "example.com:443"}, Host: "example.com:443"}
	require.NoError(t, req.Write(conn))
	resp, err := http.ReadResponse(bufio.NewReader(conn), req)
	require.NoError(t, err)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.NoError(t, conn.Close())
	require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	require.Equal(t, ErrNoLiveExitNodeReplicas.Error()+"\n", string(body))

	client, server := net.Pipe()
	done := make(chan struct{}, 1)
	go func() {
		proxy.handleTransparent(ctx, server, netip.MustParseAddrPort("192.0.2.10:443"))
		_ = server.Close()
		done <- struct{}{}
	}()
	testutil.RequireReceive(ctx, t, done)
	buf := make([]byte, 1)
	_, err = client.Read(buf)
	require.ErrorIs(t, err, io.EOF)
	require.NoError(t, client.Close())

	query := dnsQuery(t, 90, "no-replicas.example.", dnsmessage.TypeMX)
	hdr, answers := dnsExchange(ctx, t, "udp", proxy.DNSAddr(), query)
	require.Equal(t, dnsmessage.RCodeServerFailure, hdr.RCode)
	require.Empty(t, answers)

	next := proxy.Config()
	next.ExitNodes[0].ReplicaIDs = []uuid.UUID{uuid.New()}
	require.NoError(t, proxy.Update(next, nil))
	conn, _, _ = connectVia(ctx, t, proxy, "example.com:443", http.StatusOK)
	require.NoError(t, conn.Close())
	require.Len(t, exit.connectsTo("example.com:443"), 1)
}

func TestConfigEqualReplicaOrder(t *testing.T) {
	t.Parallel()

	nodeID := uuid.New()
	first := uuid.New()
	second := uuid.New()
	base := agentsdk.EgressConfig{
		ExitNodes:    []agentsdk.EgressExitNode{{ID: nodeID, ReplicaIDs: []uuid.UUID{first, second}}},
		ExitNodePort: 3128,
	}
	reordered := base
	reordered.ExitNodes = []agentsdk.EgressExitNode{{ID: nodeID, ReplicaIDs: []uuid.UUID{second, first}}}
	require.True(t, ConfigEqual(base, reordered))

	empty := agentsdk.EgressConfig{ExitNodes: []agentsdk.EgressExitNode{{ID: nodeID, ReplicaIDs: []uuid.UUID{}}}, ExitNodePort: 3128}
	nilReplicas := agentsdk.EgressConfig{ExitNodes: []agentsdk.EgressExitNode{{ID: nodeID}}, ExitNodePort: 3128}
	require.True(t, ConfigEqual(empty, nilReplicas))

	reordered.ExitNodes = []agentsdk.EgressExitNode{{ID: uuid.New(), ReplicaIDs: []uuid.UUID{second, first}}}
	require.False(t, ConfigEqual(base, reordered))
}

func TestProxy_Update(t *testing.T) {
	t.Parallel()

	firstID := uuid.New()
	secondID := uuid.New()
	seen := make(chan netip.AddrPort, 1)
	dialer := DialerFunc(func(_ context.Context, addr netip.AddrPort) (net.Conn, error) {
		seen <- addr
		client, server := net.Pipe()
		go func() {
			defer server.Close()
			req, err := http.ReadRequest(bufio.NewReader(server))
			if err == nil {
				_, _ = io.WriteString(server, established)
				_ = req.Body.Close()
			}
		}()
		return client, nil
	})
	proxy, err := New(testutil.Logger(t), Options{
		Dialer: dialer,
		Config: agentsdk.EgressConfig{ExitNodes: []agentsdk.EgressExitNode{{ID: uuid.New(), ReplicaIDs: []uuid.UUID{firstID}}}, ExitNodePort: 3128},
	})
	require.NoError(t, err)
	require.True(t, proxy.isExempt("localhost"))
	require.False(t, proxy.isExempt("new.example"))

	next := agentsdk.EgressConfig{ExitNodes: []agentsdk.EgressExitNode{{ID: uuid.New(), ReplicaIDs: []uuid.UUID{secondID}}}, ExitNodePort: 4128}
	require.NoError(t, proxy.Update(next, []string{"tcp/new.example:443"}))
	require.Equal(t, next, proxy.Config())
	require.True(t, proxy.isExempt("new.example"))

	conn, err := proxy.connectUpstream(t.Context(), "example.com:443", codersdk.ExitNodeProtocolTCP)
	require.NoError(t, err)
	require.NoError(t, conn.Close())
	want := netip.AddrPortFrom(tailnet.TailscaleServicePrefix.AddrFromUUID(secondID), 4128)
	require.Equal(t, want, testutil.RequireReceive(t.Context(), t, seen))
}

func TestProxy_UpdateResetsDNS(t *testing.T) {
	t.Parallel()

	exit := newFakeExitNode(t, nil)
	proxy := startProxy(t, exit, proxyOptions{})
	ctx := testutil.Context(t, testutil.WaitShort)
	query := dnsQuery(t, 70, "update.example.", dnsmessage.TypeA)
	_, _ = dnsExchange(ctx, t, "udp", proxy.DNSAddr(), query)
	require.Len(t, exit.connectsTo(dnsRelayTarget), 1)

	next := proxy.Config()
	next.ExitNodes = []agentsdk.EgressExitNode{{ID: uuid.New(), ReplicaIDs: []uuid.UUID{uuid.New()}}}
	require.NoError(t, proxy.Update(next, nil))
	_, _ = dnsExchange(ctx, t, "udp", proxy.DNSAddr(), query)
	require.Len(t, exit.connectsTo(dnsRelayTarget), 2)
}

func TestProxy_ExitNodeSwitchResetsDNS(t *testing.T) {
	t.Parallel()

	ids := []uuid.UUID{uuid.New(), uuid.New()}
	addrs := []netip.AddrPort{
		netip.AddrPortFrom(tailnet.TailscaleServicePrefix.AddrFromUUID(ids[0]), 3128),
		netip.AddrPortFrom(tailnet.TailscaleServicePrefix.AddrFromUUID(ids[1]), 3128),
	}
	var mu sync.Mutex
	var dnsNodes []netip.AddrPort
	dialer := DialerFunc(func(_ context.Context, addr netip.AddrPort) (net.Conn, error) {
		client, server := net.Pipe()
		go func() {
			defer server.Close()
			br := bufio.NewReader(server)
			req, err := http.ReadRequest(br)
			if err != nil {
				return
			}
			if req.Header.Get(codersdk.ExitNodeProtocolHeader) == string(codersdk.ExitNodeProtocolDNS) {
				mu.Lock()
				dnsNodes = append(dnsNodes, addr)
				mu.Unlock()
				if _, err := io.WriteString(server, established); err != nil {
					return
				}
				for {
					query, err := readFrame(br)
					if err != nil {
						return
					}
					if writeFrame(server, dnsResponse(query, dnsmessage.RCodeSuccess, 60)) != nil {
						return
					}
				}
			}
			if addr == addrs[0] {
				_, _ = io.WriteString(server, "HTTP/1.1 503 Service Unavailable\r\nContent-Length: 0\r\n\r\n")
				return
			}
			_, _ = io.WriteString(server, established)
			_, _ = io.Copy(io.Discard, server)
		}()
		return client, nil
	})
	proxy, err := New(testutil.Logger(t), Options{
		Dialer:     dialer,
		Config:     agentsdk.EgressConfig{ExitNodes: []agentsdk.EgressExitNode{{ID: uuid.New(), ReplicaIDs: ids[:1]}, {ID: uuid.New(), ReplicaIDs: ids[1:]}}, ExitNodePort: 3128},
		Rand:       rand.New(rand.NewPCG(1, 2)), //nolint:gosec // Deterministic test shuffle.
		ListenAddr: "127.0.0.1:0",
	})
	require.NoError(t, err)
	proxy.resolverDial = (&net.Dialer{}).DialContext
	require.NoError(t, proxy.Start(testutil.Context(t, testutil.WaitLong)))
	t.Cleanup(func() { _ = proxy.Close() })

	ctx := testutil.Context(t, testutil.WaitShort)
	query := dnsQuery(t, 80, "switch.example.", dnsmessage.TypeA)
	_, _ = dnsExchange(ctx, t, "udp", proxy.DNSAddr(), query)
	conn, err := proxy.connectUpstream(ctx, "switch.example:443", codersdk.ExitNodeProtocolTCP)
	require.NoError(t, err)
	require.NoError(t, conn.Close())
	_, _ = dnsExchange(ctx, t, "udp", proxy.DNSAddr(), query)

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, addrs, dnsNodes)
}

func TestProxy_ConnectNegotiationFailover(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name, response         string
		wantSecond, wantDenied bool
	}{
		{name: "hangs", response: "hang", wantSecond: true},
		{name: "eof", response: "eof", wantSecond: true},
		{name: "service unavailable", response: "503", wantSecond: true},
		{name: "forbidden", response: "403", wantDenied: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ids := []uuid.UUID{uuid.New(), uuid.New()}
			addrs := []netip.AddrPort{
				netip.AddrPortFrom(tailnet.TailscaleServicePrefix.AddrFromUUID(ids[0]), 3128),
				netip.AddrPortFrom(tailnet.TailscaleServicePrefix.AddrFromUUID(ids[1]), 3128),
			}
			var mu sync.Mutex
			var dialed []netip.AddrPort
			dialer := DialerFunc(func(_ context.Context, addr netip.AddrPort) (net.Conn, error) {
				mu.Lock()
				dialed = append(dialed, addr)
				mu.Unlock()
				client, server := net.Pipe()
				if addr == addrs[0] {
					go func() {
						defer server.Close()
						if tt.response == "eof" {
							return
						}
						_, _ = http.ReadRequest(bufio.NewReader(server))
						if tt.response == "hang" {
							_, _ = io.Copy(io.Discard, server)
							return
						}
						_, _ = fmt.Fprintf(server, "HTTP/1.1 %s X\r\nContent-Length: 0\r\n\r\n", tt.response)
					}()
				} else {
					go func() {
						defer server.Close()
						_, _ = http.ReadRequest(bufio.NewReader(server))
						_, _ = io.WriteString(server, established)
					}()
				}
				return client, nil
			})
			proxy, err := New(testutil.Logger(t), Options{
				Dialer:         dialer,
				Config:         agentsdk.EgressConfig{ExitNodes: []agentsdk.EgressExitNode{{ID: uuid.New(), ReplicaIDs: ids[:1]}, {ID: uuid.New(), ReplicaIDs: ids[1:]}}, ExitNodePort: 3128},
				Rand:           rand.New(rand.NewPCG(1, 2)), //nolint:gosec // Deterministic test shuffle.
				ConnectTimeout: time.Second,
			})
			require.NoError(t, err)
			conn, err := proxy.connectUpstream(t.Context(), "example.com:443", codersdk.ExitNodeProtocolTCP)
			if tt.wantDenied {
				require.ErrorAs(t, err, new(*DeniedError))
			} else {
				require.NoError(t, err)
				require.NoError(t, conn.Close())
			}
			mu.Lock()
			defer mu.Unlock()
			if tt.wantSecond {
				require.Equal(t, addrs, dialed)
			} else {
				require.Equal(t, addrs[:1], dialed)
			}
		})
	}
}

func TestProxy_CloseStopsListening(t *testing.T) {
	t.Parallel()

	exit := newFakeExitNode(t, nil)
	proxy := startProxy(t, exit, proxyOptions{})
	addr, dnsAddr := proxy.Addr(), proxy.DNSAddr()
	require.True(t, addr.IsValid())
	require.True(t, dnsAddr.IsValid())
	require.True(t, proxy.UDPAddr().IsValid())
	require.NoError(t, proxy.Close())
	require.NoError(t, proxy.Close())

	for _, closed := range []net.Listener{proxy.listener, proxy.dns.tcp} {
		conn, err := closed.Accept()
		if conn != nil {
			_ = conn.Close()
		}
		require.ErrorIs(t, err, net.ErrClosed)
	}
}

func validOptions(dialer Dialer) Options {
	return Options{
		Dialer: dialer,
		Config: agentsdk.EgressConfig{
			ExitNodes:    []agentsdk.EgressExitNode{{ID: uuid.New(), ReplicaIDs: []uuid.UUID{uuid.New()}}},
			ExitNodePort: 3128,
		},
	}
}

func TestNew_Validation(t *testing.T) {
	t.Parallel()

	exit := newFakeExitNode(t, nil)
	opts := validOptions(exit)
	opts.FakeIPMaxEntries = 7
	proxy, err := New(testutil.Logger(t), opts)
	require.NoError(t, err)
	require.Equal(t, 7, proxy.fake.max)

	for _, tt := range []struct {
		name    string
		mutate  func(*Options)
		wantErr string
	}{
		{name: "fake IP max", mutate: func(o *Options) { o.FakeIPMaxEntries = -1 }, wantErr: "fake IP max entries"},
		{name: "exempt host", mutate: func(o *Options) { o.ExemptHosts = []string{"coder.example.com:443"} }, wantErr: "parse exempt host"},
		{name: "dialer", mutate: func(o *Options) { o.Dialer = nil }, wantErr: "dialer"},
		{name: "exit node ID", mutate: func(o *Options) { o.Config.ExitNodes[0].ID = uuid.Nil }, wantErr: "exit node ID"},
		{name: "port", mutate: func(o *Options) { o.Config.ExitNodePort = 0 }, wantErr: "port"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			opts := validOptions(exit)
			tt.mutate(&opts)
			_, err := New(testutil.Logger(t), opts)
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

package agentegress_test

import (
	"bufio"
	"context"
	"encoding/binary"
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
	"golang.org/x/net/dns/dnsmessage"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/agent/agentegress"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// connectRecord is one CONNECT the fake exit node received.
type connectRecord struct {
	target string
	proto  string
}

// fakeExitNode is an in-memory exit node that speaks the CONNECT protocol
// over a loopback listener, including the udp and dns stream variants.
// Destinations are allowed unless deny returns a reason.
type fakeExitNode struct {
	t        testing.TB
	listener net.Listener
	deny     func(target string) (reason string, denied bool)
	// resolve maps hostnames in CONNECT targets to addresses, standing in
	// for the exit node's own resolver.
	resolve map[string]string
	// udpReply produces the reply for one relayed datagram. nil echoes.
	udpReply func(target string, payload []byte) []byte
	// dnsAnswer produces the DNS response for one relayed query. nil
	// answers every query with a TXT record.
	dnsAnswer func(query []byte) []byte
	// dnsReverseBatch, when > 0, holds that many DNS queries and answers
	// them in reverse order to exercise message ID correlation.
	dnsReverseBatch int

	mu       sync.Mutex
	connects []connectRecord
	// udpFrames records relayed datagrams by target.
	udpFrames map[string][][]byte
	wg        sync.WaitGroup
}

func newFakeExitNode(t testing.TB, deny func(target string) (string, bool)) *fakeExitNode {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	if deny == nil {
		deny = func(string) (string, bool) { return "", false }
	}
	f := &fakeExitNode{
		t:         t,
		listener:  ln,
		deny:      deny,
		resolve:   map[string]string{},
		udpFrames: map[string][][]byte{},
	}
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
	target := req.Host
	proto := req.Header.Get(codersdk.ExitNodeProtocolHeader)
	f.mu.Lock()
	f.connects = append(f.connects, connectRecord{target: target, proto: proto})
	f.mu.Unlock()

	if reason, denied := f.deny(target); denied {
		_, _ = io.WriteString(conn, "HTTP/1.1 403 Forbidden\r\n"+
			codersdk.ExitNodeDenyReasonHeader+": "+reason+"\r\n"+
			codersdk.ExitNodeDenyRuleHeader+": rule-1\r\nContent-Length: 0\r\n\r\n")
		return
	}
	switch codersdk.ExitNodeProtocol(proto) {
	case codersdk.ExitNodeProtocolUDP:
		f.handleUDP(conn, br, target)
	case codersdk.ExitNodeProtocolDNS:
		if target != "dns:53" {
			_, _ = io.WriteString(conn, "HTTP/1.1 400 Bad Request\r\nContent-Length: 0\r\n\r\n")
			return
		}
		f.handleDNS(conn, br)
	default:
		f.handleTCP(conn, br, target)
	}
}

func (f *fakeExitNode) handleTCP(conn net.Conn, br *bufio.Reader, target string) {
	host, port, err := net.SplitHostPort(target)
	if err != nil {
		_, _ = io.WriteString(conn, "HTTP/1.1 400 Bad Request\r\nContent-Length: 0\r\n\r\n")
		return
	}
	if ip, ok := f.resolve[host]; ok {
		host = ip
	}
	if net.ParseIP(host) == nil {
		_, _ = io.WriteString(conn, "HTTP/1.1 502 Bad Gateway\r\nContent-Length: 0\r\n\r\n")
		return
	}
	dst, err := net.Dial("tcp", net.JoinHostPort(host, port))
	if err != nil {
		_, _ = io.WriteString(conn, "HTTP/1.1 502 Bad Gateway\r\nContent-Length: 0\r\n\r\n")
		return
	}
	defer dst.Close()
	if _, err := io.WriteString(conn, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		return
	}
	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(dst, br)
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(conn, dst)
		done <- struct{}{}
	}()
	<-done
}

// handleUDP replies to each length-prefixed datagram on the stream.
func (f *fakeExitNode) handleUDP(conn net.Conn, br *bufio.Reader, target string) {
	if _, err := io.WriteString(conn, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		return
	}
	for {
		payload, err := readTestFrame(br)
		if err != nil {
			return
		}
		f.mu.Lock()
		f.udpFrames[target] = append(f.udpFrames[target], payload)
		f.mu.Unlock()
		reply := append([]byte("echo:"), payload...)
		if f.udpReply != nil {
			reply = f.udpReply(target, payload)
		}
		if reply == nil {
			continue
		}
		if err := writeTestFrame(conn, reply); err != nil {
			return
		}
	}
}

// handleDNS answers length-prefixed DNS queries, optionally reordering.
func (f *fakeExitNode) handleDNS(conn net.Conn, br *bufio.Reader) {
	if _, err := io.WriteString(conn, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		return
	}
	answer := f.dnsAnswer
	if answer == nil {
		answer = func(query []byte) []byte {
			return txtAnswer(f.t, query, "via-exit-node")
		}
	}
	var batch [][]byte
	for {
		query, err := readTestFrame(br)
		if err != nil {
			return
		}
		if f.dnsReverseBatch <= 1 {
			if err := writeTestFrame(conn, answer(query)); err != nil {
				return
			}
			continue
		}
		batch = append(batch, query)
		if len(batch) < f.dnsReverseBatch {
			continue
		}
		for i := len(batch) - 1; i >= 0; i-- {
			if err := writeTestFrame(conn, answer(batch[i])); err != nil {
				return
			}
		}
		batch = nil
	}
}

// DialContextTCP implements agentegress.Dialer by ignoring the tailnet
// address and connecting to the fake listener instead.
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
	return append([][]byte(nil), f.udpFrames[target]...)
}

func writeTestFrame(w io.Writer, payload []byte) error {
	if len(payload) > 65535 {
		return xerrors.Errorf("payload of %d bytes does not fit a frame", len(payload))
	}
	buf := make([]byte, 2+len(payload))
	// #nosec G115 -- bounded above.
	binary.BigEndian.PutUint16(buf, uint16(len(payload)))
	copy(buf[2:], payload)
	_, err := w.Write(buf)
	return err
}

func readTestFrame(r io.Reader) ([]byte, error) {
	var hdr [2]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	payload := make([]byte, binary.BigEndian.Uint16(hdr[:]))
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

// txtAnswer builds a TXT response to query carrying text.
func txtAnswer(t testing.TB, query []byte, text string) []byte {
	var p dnsmessage.Parser
	hdr, err := p.Start(query)
	require.NoError(t, err)
	q, err := p.Question()
	require.NoError(t, err)
	b := dnsmessage.NewBuilder(nil, dnsmessage.Header{ID: hdr.ID, Response: true, RecursionAvailable: true})
	require.NoError(t, b.StartQuestions())
	require.NoError(t, b.Question(q))
	require.NoError(t, b.StartAnswers())
	require.NoError(t, b.TXTResource(dnsmessage.ResourceHeader{
		Name: q.Name, Type: dnsmessage.TypeTXT, Class: dnsmessage.ClassINET, TTL: 60,
	}, dnsmessage.TXTResource{TXT: []string{text}}))
	out, err := b.Finish()
	require.NoError(t, err)
	return out
}

type proxyOptions struct {
	exemptHosts []string
	upstream    []netip.AddrPort
	clock       quartz.Clock
	udpOrigDst  func(oob []byte) netip.AddrPort
}

func startProxy(t testing.TB, exit *fakeExitNode, opts proxyOptions) *agentegress.Proxy {
	t.Helper()
	upstream := opts.upstream
	if upstream == nil {
		// Never touch the host's resolvers from tests.
		upstream = []netip.AddrPort{}
	}
	proxy, err := agentegress.New(testutil.Logger(t), agentegress.Options{
		Dialer: exit,
		Config: agentsdk.EgressConfig{
			ExitNodeID:   uuid.New(),
			ExitNodePort: 3128,
		},
		ExemptHosts:       opts.exemptHosts,
		UpstreamResolvers: upstream,
		Clock:             opts.clock,
	})
	require.NoError(t, err)
	if opts.udpOrigDst != nil {
		proxy.SetUDPOriginalDstForTest(opts.udpOrigDst)
	}
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
	originAddr := strings.TrimPrefix(origin.URL, "https://")
	connects := exit.connectsTo(originAddr)
	require.Len(t, connects, 1)
	require.Equal(t, "", connects[0].proto)
}

func TestProxy_ConnectDenied(t *testing.T) {
	t.Parallel()

	exit := newFakeExitNode(t, func(string) (string, bool) {
		return "blocked by policy", true
	})
	proxy := startProxy(t, exit, proxyOptions{})

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
	require.Equal(t, "blocked by policy", resp.Header.Get(codersdk.ExitNodeDenyReasonHeader))
	require.Equal(t, "rule-1", resp.Header.Get(codersdk.ExitNodeDenyRuleHeader))
}

func TestProxy_ConnectPassesHostname(t *testing.T) {
	t.Parallel()

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(origin.Close)
	originAddr := strings.TrimPrefix(origin.URL, "http://")
	_, originPort, err := net.SplitHostPort(originAddr)
	require.NoError(t, err)

	exit := newFakeExitNode(t, nil)
	// The exit node, not the proxy, resolves the name.
	exit.resolve["origin.test"] = "127.0.0.1"
	proxy := startProxy(t, exit, proxyOptions{})

	ctx := testutil.Context(t, testutil.WaitShort)
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", proxy.Addr().String())
	require.NoError(t, err)
	defer conn.Close()

	connect := &http.Request{
		Method: http.MethodConnect,
		URL:    &url.URL{Opaque: "Origin.TEST:" + originPort},
		Host:   "Origin.TEST:" + originPort,
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

	// The exit node saw the normalized hostname as the CONNECT target.
	require.Len(t, exit.connectsTo("origin.test:"+originPort), 1)
}

func TestProxy_ConnectTranslatesFakeIP(t *testing.T) {
	t.Parallel()

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(origin.Close)
	_, originPort, err := net.SplitHostPort(strings.TrimPrefix(origin.URL, "http://"))
	require.NoError(t, err)

	exit := newFakeExitNode(t, nil)
	exit.resolve["fake.test"] = "127.0.0.1"
	proxy := startProxy(t, exit, proxyOptions{})
	fakeIP := proxy.FakeIPForTest("fake.test")

	// A client that resolved through the proxy's DNS and then CONNECTs to
	// the fake IP literal still yields a hostname at the exit node.
	ctx := testutil.Context(t, testutil.WaitShort)
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", proxy.Addr().String())
	require.NoError(t, err)
	defer conn.Close()
	target := net.JoinHostPort(fakeIP.String(), originPort)
	connect := &http.Request{
		Method: http.MethodConnect,
		URL:    &url.URL{Opaque: target},
		Host:   target,
		Header: http.Header{},
	}
	require.NoError(t, connect.Write(conn))
	resp, err := http.ReadResponse(bufio.NewReader(conn), connect)
	require.NoError(t, err)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Len(t, exit.connectsTo("fake.test:"+originPort), 1)
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
	proxy := startProxy(t, exit, proxyOptions{})

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
	proxy := startProxy(t, exit, proxyOptions{})

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
	require.Equal(t, "no plain http", resp.Header.Get(codersdk.ExitNodeDenyReasonHeader))
}

func TestProxy_CloseStopsListening(t *testing.T) {
	t.Parallel()

	exit := newFakeExitNode(t, nil)
	proxy := startProxy(t, exit, proxyOptions{})
	addr := proxy.Addr().String()
	dnsAddr := proxy.DNSAddr()
	udpAddr := proxy.UDPAddr()
	require.True(t, dnsAddr.IsValid())
	require.True(t, udpAddr.IsValid())
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
	conn, err = d.DialContext(ctx, "tcp", dnsAddr.String())
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

// Package agentegress runs the workspace-side half of exit node egress. It
// exposes a local proxy that forwards every outbound TCP connection to the
// exit node over the tailnet using HTTP CONNECT, and optionally installs
// netfilter rules so workspace processes cannot bypass the proxy.
package agentegress

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/quartz"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/tailnet"
)

const (
	defaultListenHost = "127.0.0.1"

	// dnsRelayTarget is the CONNECT target the exit node recognizes as "resolve
	// these DNS messages yourself".
	dnsRelayTarget = "dns:53"
)

// builtinExemptNames are always resolved by the system resolvers rather than
// given fake addresses. They name the workspace host itself.
var builtinExemptNames = []string{"localhost", "host.docker.internal"}

// Dialer opens TCP connections on the tailnet. *tailnet.Conn returns a
// concrete *gonet.TCPConn, so callers wrap it with DialerFunc.
type Dialer interface {
	DialContextTCP(ctx context.Context, addr netip.AddrPort) (net.Conn, error)
}

// DialerFunc adapts a function to the Dialer interface.
type DialerFunc func(ctx context.Context, addr netip.AddrPort) (net.Conn, error)

// DialContextTCP implements Dialer.
func (f DialerFunc) DialContextTCP(ctx context.Context, addr netip.AddrPort) (net.Conn, error) {
	return f(ctx, addr)
}

// Resolver resolves hostnames. The Enforcer uses it to turn exempt hosts
// into addresses before rules are installed.
type Resolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

// Options configure a Proxy.
type Options struct {
	// Dialer reaches the exit node over the tailnet. Required.
	Dialer Dialer
	// Config is the egress configuration from the agent manifest.
	Config agentsdk.EgressConfig
	// ListenAddr is the local address for the TCP proxy. Defaults to
	// 127.0.0.1:0 (an ephemeral port). The proxy must stay on loopback: it
	// performs no authentication. The DNS and UDP listeners bind ephemeral
	// ports on the same host.
	ListenAddr string
	// ExemptHosts are host[:port] destinations that bypass the exit node.
	// Their names are resolved by the system resolvers instead of being
	// given fake addresses. Control plane hosts should be included.
	ExemptHosts []string
	// UpstreamResolvers answer DNS queries for exempt names over TCP.
	// Defaults to the nameservers in /etc/resolv.conf.
	UpstreamResolvers []netip.AddrPort
	// Clock drives UDP session idle timeouts. Defaults to the real clock.
	Clock quartz.Clock
}

// Proxy is the local egress proxy. One loopback TCP listener serves both
// transparently redirected connections (identified via SO_ORIGINAL_DST) and
// explicit HTTP proxy clients that were pointed at it via HTTP_PROXY. A DNS
// listener hands out fake addresses so redirected flows can be tunneled by
// name, and a UDP listener relays redirected datagrams.
type Proxy struct {
	logger   slog.Logger
	dialer   Dialer
	cfg      agentsdk.EgressConfig
	exitNode netip.AddrPort
	listen   string
	clock    quartz.Clock

	fake        *fakeIPPool
	exemptNames map[string]struct{}
	upstream    []netip.AddrPort
	// udpOrigDst decodes the original destination of a redirected datagram.
	// Tests replace it because only netfilter can produce a real one.
	udpOrigDst func(oob []byte) netip.AddrPort

	mu       sync.Mutex
	listener net.Listener
	addr     netip.AddrPort
	dns      *dnsServer
	udp      *udpProxy
	cancel   context.CancelFunc
	closed   bool
	wg       sync.WaitGroup
}

// New validates the options and returns a Proxy that is not yet listening.
func New(logger slog.Logger, opts Options) (*Proxy, error) {
	if opts.Dialer == nil {
		return nil, xerrors.New("dialer is required")
	}
	if opts.Config.ExitNodeID == uuid.Nil {
		return nil, xerrors.New("exit node ID is required")
	}
	if opts.Config.ExitNodePort <= 0 || opts.Config.ExitNodePort > 65535 {
		return nil, xerrors.Errorf("invalid exit node port %d", opts.Config.ExitNodePort)
	}
	listen := opts.ListenAddr
	if listen == "" {
		listen = net.JoinHostPort(defaultListenHost, "0")
	}
	clock := opts.Clock
	if clock == nil {
		clock = quartz.NewReal()
	}
	exempt := make(map[string]struct{})
	for _, name := range builtinExemptNames {
		exempt[name] = struct{}{}
	}
	for _, hostport := range opts.ExemptHosts {
		host := normalizeName(stripPort(hostport))
		if host == "" || isIPLiteral(host) {
			continue
		}
		exempt[host] = struct{}{}
	}
	return &Proxy{
		logger: logger,
		dialer: opts.Dialer,
		cfg:    opts.Config,
		clock:  clock,
		exitNode: netip.AddrPortFrom(
			tailnet.TailscaleServicePrefix.AddrFromUUID(opts.Config.ExitNodeID),
			// #nosec G115 -- range validated above.
			uint16(opts.Config.ExitNodePort),
		),
		listen:      listen,
		fake:        newFakeIPPool(fakeIPPrefix, fakeIPMaxEntries),
		exemptNames: exempt,
		upstream:    opts.UpstreamResolvers,
		udpOrigDst:  udpOriginalDst,
	}, nil
}

// Start opens the listeners and begins serving. Connections are handled
// until Close is called or ctx is canceled.
func (p *Proxy) Start(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return xerrors.New("proxy is closed")
	}
	if p.listener != nil {
		return xerrors.New("proxy already started")
	}
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", p.listen)
	if err != nil {
		return xerrors.Errorf("listen on %s: %w", p.listen, err)
	}
	tcpAddr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		_ = ln.Close()
		return xerrors.Errorf("unexpected listener address type %T", ln.Addr())
	}
	host := tcpAddr.AddrPort().Addr().String()

	upstream := p.upstream
	if upstream == nil {
		for _, addr := range systemResolvers() {
			upstream = append(upstream, netip.AddrPortFrom(addr, 53))
		}
	}
	dns := &dnsServer{
		logger:   p.logger.Named("dns"),
		fake:     p.fake,
		exempt:   p.isExempt,
		upstream: upstream,
		relay: newDNSRelay(p.logger.Named("dns"), func(ctx context.Context) (net.Conn, error) {
			return p.connectUpstream(ctx, dnsRelayTarget, codersdk.ExitNodeProtocolDNS)
		}),
	}
	if err := dns.listen(ctx, host); err != nil {
		_ = ln.Close()
		return err
	}
	udp := &udpProxy{
		logger: p.logger.Named("udp"),
		clock:  p.clock,
		fake:   p.fake,
		connect: func(ctx context.Context, target string) (net.Conn, error) {
			return p.connectUpstream(ctx, target, codersdk.ExitNodeProtocolUDP)
		},
		origDst: p.udpOrigDst,
	}
	if err := udp.listen(ctx, host); err != nil {
		_ = ln.Close()
		dns.close()
		return err
	}

	p.addr = tcpAddr.AddrPort()
	p.listener = ln
	p.dns = dns
	p.udp = udp
	ctx, p.cancel = context.WithCancel(ctx)
	p.wg.Add(1)
	go p.acceptLoop(ctx, ln)
	dns.serve(ctx)
	udp.serve(ctx)
	return nil
}

// Addr returns the loopback address the TCP proxy listens on. It is the
// zero value before Start.
func (p *Proxy) Addr() netip.AddrPort {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.addr
}

// DNSAddr returns the loopback address the DNS listener serves on, for
// both UDP and TCP. It is the zero value before Start.
func (p *Proxy) DNSAddr() netip.AddrPort {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.dns == nil {
		return netip.AddrPort{}
	}
	return p.dns.addr
}

// UDPAddr returns the loopback address redirected UDP is delivered to. It
// is the zero value before Start.
func (p *Proxy) UDPAddr() netip.AddrPort {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.udp == nil {
		return netip.AddrPort{}
	}
	return p.udp.addr
}

// Config returns the egress configuration the proxy was started with.
func (p *Proxy) Config() agentsdk.EgressConfig {
	return p.cfg
}

// ConfigEqual reports whether two egress configurations would produce the
// same proxy and enforcement state, so a reconnect that redelivers the same
// manifest does not restart the proxy.
func ConfigEqual(a, b agentsdk.EgressConfig) bool {
	return a.ExitNodeID == b.ExitNodeID &&
		a.ExitNodePort == b.ExitNodePort &&
		a.Enforce == b.Enforce &&
		slices.Equal(a.ControlPlaneHosts, b.ControlPlaneHosts)
}

// Close stops accepting connections and waits for in-flight connections to
// finish being torn down.
func (p *Proxy) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	ln := p.listener
	dns := p.dns
	udp := p.udp
	cancel := p.cancel
	p.mu.Unlock()

	var err error
	if cancel != nil {
		cancel()
	}
	if ln != nil {
		err = ln.Close()
	}
	if dns != nil {
		dns.close()
	}
	if udp != nil {
		udp.close()
	}
	p.wg.Wait()
	if err != nil && !errors.Is(err, net.ErrClosed) {
		return xerrors.Errorf("close listener: %w", err)
	}
	return nil
}

// isExempt reports whether name bypasses the exit node.
func (p *Proxy) isExempt(name string) bool {
	_, ok := p.exemptNames[normalizeName(name)]
	return ok
}

// target names the CONNECT destination for a redirected flow. Fake
// addresses are translated back to the hostname the client resolved so the
// exit node can apply name-based policy and resolve the name itself.
func (p *Proxy) target(dst netip.AddrPort) string {
	if name, ok := p.fake.Reverse(dst.Addr()); ok {
		return net.JoinHostPort(name, strconv.Itoa(int(dst.Port())))
	}
	return dst.String()
}

// explicitTarget names the CONNECT destination for an explicit proxy
// request. Hostnames pass through for the exit node to resolve; a fake IP
// literal (from a client that resolved before proxying) is translated.
func (p *Proxy) explicitTarget(host string, port uint16) string {
	if addr, err := netip.ParseAddr(strings.Trim(host, "[]")); err == nil {
		return p.target(netip.AddrPortFrom(addr.Unmap(), port))
	}
	return net.JoinHostPort(normalizeName(host), strconv.Itoa(int(port)))
}

func (p *Proxy) acceptLoop(ctx context.Context, ln net.Listener) {
	defer p.wg.Done()
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return
			}
			p.logger.Warn(ctx, "accept egress proxy connection", slog.Error(err))
			continue
		}
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			p.handleConn(ctx, conn)
		}()
	}
}

// handleConn decides whether the connection was transparently redirected by
// netfilter or is an explicit proxy client, then tunnels it accordingly.
func (p *Proxy) handleConn(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	// Close the client when the proxy shuts down so blocked copies unwind.
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()

	dst, err := originalDestination(conn)
	if err == nil && dst.IsValid() && dst != p.Addr() {
		p.tunnel(ctx, conn, conn, p.target(dst))
		return
	}
	p.handleExplicit(ctx, conn)
}

// handleExplicit serves an HTTP proxy client: either CONNECT or a plain
// request with an absolute URI.
func (p *Proxy) handleExplicit(ctx context.Context, conn net.Conn) {
	br := bufio.NewReader(conn)
	req, err := http.ReadRequest(br)
	if err != nil {
		if !errors.Is(err, io.EOF) {
			p.logger.Debug(ctx, "read explicit proxy request", slog.Error(err))
		}
		return
	}
	defer req.Body.Close()

	if req.Method == http.MethodConnect {
		p.handleConnect(ctx, conn, br, req)
		return
	}
	if !req.URL.IsAbs() {
		writeSimpleResponse(conn, req, http.StatusBadRequest, "proxy requests must use an absolute URI")
		return
	}
	host, port := splitHostPortDefault(req.URL.Host, schemePort(req.URL.Scheme))
	target := p.explicitTarget(host, port)
	upstream, err := p.connectUpstream(ctx, target, codersdk.ExitNodeProtocolTCP)
	if err != nil {
		p.writeUpstreamError(ctx, conn, req, target, err)
		return
	}
	defer upstream.Close()

	// Rewrite the proxy request into an origin-form request. RequestURI must
	// be empty for Request.Write, which then emits the path. Hop-by-hop proxy
	// headers are not meaningful to the origin.
	req.RequestURI = ""
	req.Header.Del("Proxy-Connection")
	req.Header.Del("Proxy-Authorization")
	if err := req.Write(upstream); err != nil {
		p.logger.Debug(ctx, "forward proxied request", slog.Error(err))
		return
	}
	pipe(ctx, conn, br, upstream)
}

func (p *Proxy) handleConnect(ctx context.Context, conn net.Conn, br *bufio.Reader, req *http.Request) {
	host, port := splitHostPortDefault(req.Host, 443)
	target := p.explicitTarget(host, port)
	upstream, err := p.connectUpstream(ctx, target, codersdk.ExitNodeProtocolTCP)
	if err != nil {
		p.writeUpstreamError(ctx, conn, req, target, err)
		return
	}
	defer upstream.Close()
	if _, err := io.WriteString(conn, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		return
	}
	pipe(ctx, conn, br, upstream)
}

// tunnel forwards a transparently redirected connection to target via the
// exit node. The client sees a reset or EOF when the exit node denies the
// flow.
func (p *Proxy) tunnel(ctx context.Context, conn net.Conn, clientReader io.Reader, target string) {
	upstream, err := p.connectUpstream(ctx, target, codersdk.ExitNodeProtocolTCP)
	if err != nil {
		p.logUpstreamError(ctx, target, err)
		return
	}
	defer upstream.Close()
	pipe(ctx, conn, clientReader, upstream)
}

func (p *Proxy) writeUpstreamError(ctx context.Context, conn net.Conn, req *http.Request, target string, err error) {
	p.logUpstreamError(ctx, target, err)
	var denied *DeniedError
	if errors.As(err, &denied) {
		hdr := http.Header{codersdk.ExitNodeDenyReasonHeader: []string{denied.Reason}}
		if denied.Rule != "" {
			hdr.Set(codersdk.ExitNodeDenyRuleHeader, denied.Rule)
		}
		resp := &http.Response{
			StatusCode: http.StatusForbidden,
			ProtoMajor: 1,
			ProtoMinor: 1,
			Header:     hdr,
			Body:       io.NopCloser(strings.NewReader(denied.Error() + "\n")),
			Request:    req,
		}
		resp.ContentLength = int64(len(denied.Error()) + 1)
		_ = resp.Write(conn)
		return
	}
	writeSimpleResponse(conn, req, http.StatusBadGateway, "exit node: "+err.Error())
}

func (p *Proxy) logUpstreamError(ctx context.Context, target string, err error) {
	fields := []slog.Field{slog.F("target", target)}
	var denied *DeniedError
	if errors.As(err, &denied) {
		p.logger.Info(ctx, "egress denied by exit node",
			append(fields, slog.F("reason", denied.Reason), slog.F("rule", denied.Rule))...)
		return
	}
	p.logger.Warn(ctx, "egress connection through exit node failed", append(fields, slog.Error(err))...)
}

// DeniedError is returned when the exit node rejects a CONNECT with 403.
type DeniedError struct {
	// Target is the host:port the CONNECT asked for.
	Target string
	Reason string
	// Rule names the matching policy rule, when the exit node reported one.
	Rule string
}

// Error implements error.
func (e *DeniedError) Error() string {
	if e.Reason == "" {
		return fmt.Sprintf("egress to %s denied by exit node", e.Target)
	}
	return fmt.Sprintf("egress to %s denied by exit node: %s", e.Target, e.Reason)
}

// connectUpstream dials the exit node and negotiates a CONNECT tunnel to
// target (host:port or ip:port) carrying proto. On success the returned
// connection carries the tunnel bytes. Hostname targets are resolved by the
// exit node, so no original-host hint is needed.
func (p *Proxy) connectUpstream(ctx context.Context, target string, proto codersdk.ExitNodeProtocol) (net.Conn, error) {
	upstream, err := p.dialer.DialContextTCP(ctx, p.exitNode)
	if err != nil {
		return nil, xerrors.Errorf("dial exit node %s: %w", p.exitNode, err)
	}
	connectReq := &http.Request{
		Method: http.MethodConnect,
		URL:    &url.URL{Opaque: target},
		Host:   target,
		Header: http.Header{},
	}
	if proto != "" && proto != codersdk.ExitNodeProtocolTCP {
		connectReq.Header.Set(codersdk.ExitNodeProtocolHeader, string(proto))
	}
	if err := connectReq.Write(upstream); err != nil {
		_ = upstream.Close()
		return nil, xerrors.Errorf("write CONNECT to exit node: %w", err)
	}
	br := bufio.NewReader(upstream)
	resp, err := http.ReadResponse(br, connectReq)
	if err != nil {
		_ = upstream.Close()
		return nil, xerrors.Errorf("read CONNECT response from exit node: %w", err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		return &bufferedConn{Conn: upstream, r: br}, nil
	case http.StatusForbidden:
		_ = upstream.Close()
		return nil, &DeniedError{
			Target: target,
			Reason: resp.Header.Get(codersdk.ExitNodeDenyReasonHeader),
			Rule:   resp.Header.Get(codersdk.ExitNodeDenyRuleHeader),
		}
	default:
		_ = upstream.Close()
		return nil, xerrors.Errorf("exit node responded %s to CONNECT %s", resp.Status, target)
	}
}

// bufferedConn ensures bytes the CONNECT response reader already buffered
// are not lost when the tunnel starts copying.
type bufferedConn struct {
	net.Conn
	r *bufio.Reader
}

func (c *bufferedConn) Read(b []byte) (int, error) {
	return c.r.Read(b)
}

// pipe copies bytes in both directions until either side closes. The client
// side reads from clientReader (which may hold buffered bytes) and writes to
// client.
func pipe(ctx context.Context, client net.Conn, clientReader io.Reader, upstream net.Conn) {
	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(upstream, clientReader)
		closeWrite(upstream)
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(client, upstream)
		closeWrite(client)
		done <- struct{}{}
	}()
	// Wait for both directions so half-closed streams (for example a
	// client that sent its request then shut down writes) still drain.
	for range 2 {
		select {
		case <-done:
		case <-ctx.Done():
			_ = client.Close()
			_ = upstream.Close()
			return
		}
	}
}

func closeWrite(conn net.Conn) {
	if cw, ok := conn.(interface{ CloseWrite() error }); ok {
		_ = cw.CloseWrite()
		return
	}
	_ = conn.Close()
}

func writeSimpleResponse(conn net.Conn, req *http.Request, status int, msg string) {
	body := msg + "\n"
	resp := &http.Response{
		StatusCode:    status,
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        http.Header{"Content-Type": []string{"text/plain; charset=utf-8"}},
		Body:          io.NopCloser(strings.NewReader(body)),
		ContentLength: int64(len(body)),
		Request:       req,
	}
	_ = resp.Write(conn)
}

// splitHostPortDefault splits host[:port], falling back to def when no port
// is present or the port is unparsable.
func splitHostPortDefault(hostport string, def uint16) (string, uint16) {
	host, portStr, err := net.SplitHostPort(hostport)
	if err != nil {
		return strings.Trim(hostport, "[]"), def
	}
	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return host, def
	}
	return host, uint16(port)
}

func schemePort(scheme string) uint16 {
	if strings.EqualFold(scheme, "https") {
		return 443
	}
	return 80
}

func isIPLiteral(host string) bool {
	_, err := netip.ParseAddr(strings.Trim(host, "[]"))
	return err == nil
}

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
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/quartz"

	"github.com/coder/coder/v2/agent/agentegress/hostsniff"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/tailnet"
)

const (
	defaultListenHost       = "127.0.0.1"
	defaultProxyPort        = 41280
	defaultDNSPort          = 41253
	defaultUDPPort          = 41254
	defaultConnectTimeout   = 10 * time.Second
	defaultHostSniffTimeout = 500 * time.Millisecond
	unknownFakeLogInterval  = 30 * time.Second
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
	// ListenAddr overrides the local address for the TCP proxy. When empty,
	// ProxyPort is bound on 127.0.0.1, falling back to an ephemeral port if the
	// fixed port is unavailable. The proxy must stay on loopback because it
	// performs no authentication.
	ListenAddr string
	// ProxyPort is the preferred TCP proxy port. It defaults to 41280.
	ProxyPort uint16
	// DNSPort is the preferred DNS TCP and UDP port. It defaults to 41253.
	DNSPort uint16
	// UDPPort is the preferred redirected UDP port. It defaults to 41254.
	UDPPort uint16
	// ExemptHosts are host[:port] destinations that bypass the exit node.
	// Their names are resolved by the system resolvers instead of being
	// given fake addresses. Control plane hosts should be included.
	ExemptHosts []string
	// UpstreamResolvers answer DNS queries for exempt names over TCP.
	// Defaults to the nameservers in /etc/resolv.conf.
	UpstreamResolvers []netip.AddrPort
	// Clock drives UDP session idle timeouts. Defaults to the real clock.
	Clock quartz.Clock
	// HostSniffTimeout bounds transparent connection host detection. It
	// defaults to 500ms. A negative value disables sniffing.
	HostSniffTimeout time.Duration
	// ConnectTimeout bounds CONNECT request and response negotiation with an
	// exit node. It defaults to 10 seconds.
	ConnectTimeout time.Duration
	// FakeIPMaxEntries bounds cached DNS name mappings. It defaults to 65536.
	// Smaller values use less memory but increase churn for short-lived DNS
	// answers. Active connection mappings remain pinned beyond this limit.
	FakeIPMaxEntries int
	// FakeIPPinnedMaxEntries caps mappings retained by active connections. It
	// defaults to FakeIPMaxEntries.
	FakeIPPinnedMaxEntries int
}

// Proxy is the local egress proxy. One loopback TCP listener serves both
// transparently redirected connections (identified via SO_ORIGINAL_DST) and
// explicit HTTP proxy clients that were pointed at it via HTTP_PROXY. A DNS
// listener hands out fake addresses so redirected flows can be tunneled by
// name, and a UDP listener relays redirected datagrams.
type Proxy struct {
	logger         slog.Logger
	dialer         Dialer
	configMu       sync.RWMutex
	cfg            agentsdk.EgressConfig
	exitNodes      *exitNodeSelector
	exemptNames    map[string]struct{}
	listen         string
	dnsPort        uint16
	udpPort        uint16
	clock          quartz.Clock
	connectTimeout time.Duration
	sniffTimeout   time.Duration

	fake         *fakeIPPool
	upstream     []netip.AddrPort
	resolverDial func(context.Context, string, string) (net.Conn, error)
	// udpOrigDst decodes the original destination of a redirected datagram.
	// Tests replace it because only netfilter can produce a real one.
	udpOrigDst func(oob []byte) netip.AddrPort

	// unknownFake counts redirected TCP connections dropped because their
	// destination was in the fake range without a current mapping.
	unknownFake atomic.Int64
	fakeWarnMu  sync.Mutex
	fakeWarnAt  time.Time

	mu       sync.Mutex
	listener net.Listener
	addr     netip.AddrPort
	dns      *dnsServer
	udp      *udpProxy
	cancel   context.CancelFunc
	closed   bool
	wg       sync.WaitGroup
}

// connectFunc opens a CONNECT tunnel to target carrying proto.
type connectFunc func(ctx context.Context, target string, proto codersdk.ExitNodeProtocol) (net.Conn, error)

func exemptNameSet(hosts []string) (map[string]struct{}, error) {
	exempt := make(map[string]struct{}, len(builtinExemptNames)+len(hosts))
	for _, name := range builtinExemptNames {
		exempt[name] = struct{}{}
	}
	for _, value := range hosts {
		parsed, err := ParseExemption(value)
		if err != nil {
			return nil, xerrors.Errorf("parse exempt host: %w", err)
		}
		host := normalizeName(parsed.Host)
		if _, err := netip.ParseAddr(host); err == nil {
			continue
		}
		exempt[host] = struct{}{}
	}
	return exempt, nil
}

func exitNodeAddrs(cfg agentsdk.EgressConfig) ([]netip.AddrPort, error) {
	if len(cfg.ExitNodeIDs) == 0 {
		return nil, xerrors.New("at least one exit node ID is required")
	}
	if cfg.ExitNodePort <= 0 || cfg.ExitNodePort > 65535 {
		return nil, xerrors.Errorf("invalid exit node port %d", cfg.ExitNodePort)
	}
	exitNodes := make([]netip.AddrPort, 0, len(cfg.ExitNodeIDs))
	for _, id := range cfg.ExitNodeIDs {
		if id == uuid.Nil {
			return nil, xerrors.New("exit node ID is required")
		}
		exitNodes = append(exitNodes, netip.AddrPortFrom(
			tailnet.TailscaleServicePrefix.AddrFromUUID(id),
			// #nosec G115 -- range validated above.
			uint16(cfg.ExitNodePort),
		))
	}
	return exitNodes, nil
}

// New validates the options and returns a Proxy that is not yet listening.
func New(logger slog.Logger, opts Options) (*Proxy, error) {
	if opts.Dialer == nil {
		return nil, xerrors.New("dialer is required")
	}
	exitNodes, err := exitNodeAddrs(opts.Config)
	if err != nil {
		return nil, err
	}
	if opts.FakeIPMaxEntries < 0 {
		return nil, xerrors.New("fake IP max entries must not be negative")
	}
	if opts.FakeIPPinnedMaxEntries < 0 {
		return nil, xerrors.New("fake IP pinned max entries must not be negative")
	}
	fakeIPMax := opts.FakeIPMaxEntries
	if fakeIPMax == 0 {
		fakeIPMax = fakeIPMaxEntries
	}
	pinnedMax := opts.FakeIPPinnedMaxEntries
	if pinnedMax == 0 {
		pinnedMax = fakeIPMax
	}
	if pinnedMax < fakeIPMax {
		return nil, xerrors.New("fake IP pinned max entries must be at least fake IP max entries")
	}
	connectTimeout := opts.ConnectTimeout
	if connectTimeout == 0 {
		connectTimeout = defaultConnectTimeout
	}
	if connectTimeout < 0 {
		return nil, xerrors.New("connect timeout must not be negative")
	}
	sniffTimeout := opts.HostSniffTimeout
	if sniffTimeout == 0 {
		sniffTimeout = defaultHostSniffTimeout
	}
	exempt, err := exemptNameSet(opts.ExemptHosts)
	if err != nil {
		return nil, err
	}
	resolver := resolverDialer()
	p := &Proxy{
		logger:         logger,
		dialer:         opts.Dialer,
		cfg:            opts.Config,
		clock:          opts.Clock,
		exitNodes:      newExitNodeSelector(logger, opts.Clock, exitNodes),
		listen:         opts.ListenAddr,
		dnsPort:        opts.DNSPort,
		udpPort:        opts.UDPPort,
		connectTimeout: connectTimeout,
		sniffTimeout:   sniffTimeout,
		fake:           newFakeIPPool(fakeIPPrefix, fakeIPMax, pinnedMax),
		exemptNames:    exempt,
		upstream:       opts.UpstreamResolvers,
		resolverDial:   resolver.DialContext,
		udpOrigDst:     udpOriginalDst,
	}
	if p.clock == nil {
		p.clock = quartz.NewReal()
	}
	if p.listen == "" {
		port := opts.ProxyPort
		if port == 0 {
			port = defaultProxyPort
		}
		p.listen = net.JoinHostPort(defaultListenHost, strconv.Itoa(int(port)))
	}
	if p.dnsPort == 0 {
		p.dnsPort = defaultDNSPort
	}
	if p.udpPort == 0 {
		p.udpPort = defaultUDPPort
	}
	return p, nil
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
	preferredErr := err
	if err != nil {
		host, _, splitErr := net.SplitHostPort(p.listen)
		if splitErr != nil {
			return xerrors.Errorf("listen on %s: %w", p.listen, err)
		}
		fallback := net.JoinHostPort(host, "0")
		p.logger.Warn(ctx, "preferred egress proxy port unavailable, using ephemeral port",
			slog.F("listen_addr", p.listen), slog.Error(err))
		ln, err = lc.Listen(ctx, "tcp", fallback)
		if err != nil {
			return errors.Join(
				xerrors.Errorf("listen on %s: %w", p.listen, preferredErr),
				xerrors.Errorf("fallback on %s: %w", fallback, err),
			)
		}
	}
	addr, err := addrPortOf(ln.Addr())
	if err != nil {
		_ = ln.Close()
		return err
	}
	host := addr.Addr().String()

	upstream := p.upstream
	if upstream == nil {
		for _, addr := range systemResolvers() {
			upstream = append(upstream, netip.AddrPortFrom(addr, 53))
		}
	}
	dns := &dnsServer{
		logger:    p.logger.Named("dns"),
		fake:      p.fake,
		exempt:    p.isExempt,
		upstream:  upstream,
		relay:     newDNSRelay(p.logger.Named("dns"), p.connectUpstream),
		clock:     p.clock,
		decisions: newDNSDecisionCache(dnsDecisionCacheSize),
		dial:      p.resolverDial,
	}
	if err := dns.listen(ctx, host, p.dnsPort); err != nil {
		_ = ln.Close()
		return err
	}
	udp := &udpProxy{
		logger:  p.logger.Named("udp"),
		clock:   p.clock,
		connect: p.connectUpstream,
		target:  p.target,
		origDst: p.udpOrigDst,
	}
	if err := udp.listen(ctx, host, p.udpPort); err != nil {
		_ = ln.Close()
		dns.close()
		return err
	}

	p.addr, p.listener, p.dns, p.udp = addr, ln, dns, udp
	ctx, p.cancel = context.WithCancel(ctx)
	p.wg.Go(func() { p.acceptLoop(ctx, ln) })
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

// Config returns the current egress configuration.
func (p *Proxy) Config() agentsdk.EgressConfig {
	p.configMu.RLock()
	defer p.configMu.RUnlock()
	return p.cfg
}

// Update atomically changes the exit node selector and exempt host set without
// disturbing listeners or in-flight tunnels.
func (p *Proxy) Update(cfg agentsdk.EgressConfig, exemptHosts []string) error {
	exitNodes, err := exitNodeAddrs(cfg)
	if err != nil {
		return err
	}
	exempt, err := exemptNameSet(exemptHosts)
	if err != nil {
		return err
	}
	selector := newExitNodeSelector(p.logger, p.clock, exitNodes)
	p.configMu.Lock()
	p.cfg = cfg
	p.exitNodes = selector
	p.exemptNames = exempt
	p.configMu.Unlock()
	p.resetDNS()
	return nil
}

func (p *Proxy) clearDNSDecisions() {
	p.mu.Lock()
	dns := p.dns
	p.mu.Unlock()
	if dns != nil {
		dns.decisions.clear()
	}
}

func (p *Proxy) resetDNS() {
	p.mu.Lock()
	dns := p.dns
	p.mu.Unlock()
	if dns == nil {
		return
	}
	dns.decisions.clear()
	dns.relay.reset()
}

// ConfigEqual reports whether two egress configurations would produce the
// same proxy and enforcement state, so a reconnect that redelivers the same
// manifest does not restart the proxy.
func ConfigEqual(a, b agentsdk.EgressConfig) bool {
	return slices.Equal(a.ExitNodeIDs, b.ExitNodeIDs) &&
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
	ln, dns, udp, cancel := p.listener, p.dns, p.udp, p.cancel
	p.mu.Unlock()
	if ln == nil {
		return nil
	}

	cancel()
	err := ln.Close()
	dns.close()
	udp.close()
	p.wg.Wait()
	if err != nil && !errors.Is(err, net.ErrClosed) {
		return xerrors.Errorf("close listener: %w", err)
	}
	return nil
}

// isExempt reports whether name bypasses the exit node.
func (p *Proxy) isExempt(name string) bool {
	p.configMu.RLock()
	defer p.configMu.RUnlock()
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

// acquireTarget names and pins the destination for one TCP connection. An
// address in the fake range without a mapping is rejected instead of being
// sent to the exit node as though it were a real destination.
func (p *Proxy) acquireTarget(dst netip.AddrPort) (target string, release func(), ok bool) {
	if !p.fake.Contains(dst.Addr()) {
		return dst.String(), func() {}, true
	}
	name, release, ok := p.fake.Acquire(dst.Addr())
	if !ok {
		return "", func() {}, false
	}
	return net.JoinHostPort(name, strconv.Itoa(int(dst.Port()))), release, true
}

func (p *Proxy) warnUnknownFake(ctx context.Context, dst netip.AddrPort) {
	count := p.unknownFake.Add(1)
	p.fakeWarnMu.Lock()
	now := p.clock.Now()
	if now.Before(p.fakeWarnAt) {
		p.fakeWarnMu.Unlock()
		return
	}
	p.fakeWarnAt = now.Add(unknownFakeLogInterval)
	p.fakeWarnMu.Unlock()
	p.logger.Warn(ctx, "dropping redirected tcp connection to unmapped fake IP",
		slog.F("destination", dst.String()), slog.F("count", count))
}

// explicitTarget names the CONNECT destination for an explicit proxy
// request. Hostnames pass through for the exit node to resolve; a fake IP
// literal (from a client that resolved before proxying) is translated.
func (p *Proxy) explicitTarget(host string, port uint16) string {
	if addr, err := netip.ParseAddr(host); err == nil {
		return p.target(netip.AddrPortFrom(addr.Unmap(), port))
	}
	return net.JoinHostPort(normalizeName(host), strconv.Itoa(int(port)))
}

func (p *Proxy) acceptLoop(ctx context.Context, ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return
			}
			p.logger.Warn(ctx, "accept egress proxy connection", slog.Error(err))
			continue
		}
		p.wg.Go(func() { p.handleConn(ctx, conn) })
	}
}

// handleConn decides whether the connection was transparently redirected by
// netfilter or is an explicit proxy client, then tunnels it accordingly.
// A redirected client sees a reset or EOF when the exit node denies the flow.
func (p *Proxy) handleConn(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	// Close the client when the proxy shuts down so blocked copies unwind.
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()

	dst, err := originalDestination(conn)
	if err != nil || !dst.IsValid() || dst == p.Addr() {
		p.handleExplicit(ctx, conn)
		return
	}
	p.handleTransparent(ctx, conn, dst)
}

func (p *Proxy) handleTransparent(ctx context.Context, conn net.Conn, dst netip.AddrPort) {
	target, release, ok := p.acquireTarget(dst)
	if !ok {
		p.warnUnknownFake(ctx, dst)
		return
	}
	defer release()

	clientReader := conn
	originalHost := ""
	if !p.fake.Contains(dst.Addr()) && p.sniffTimeout > 0 {
		var err error
		originalHost, clientReader, err = hostsniff.Host(conn, p.sniffTimeout)
		if err != nil {
			p.logger.Debug(ctx, "sniff transparent connection host", slog.Error(err))
		}
	}
	upstream, err := p.connectUpstreamHost(ctx, target, codersdk.ExitNodeProtocolTCP, originalHost)
	if err != nil {
		p.logUpstreamError(ctx, target, err)
		return
	}
	defer upstream.Close()
	pipe(ctx, conn, clientReader, upstream)
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

	var host string
	var port uint16
	switch {
	case req.Method == http.MethodConnect:
		host, port = splitHostPortDefault(req.Host, 443)
	case req.URL.IsAbs():
		host, port = splitHostPortDefault(req.URL.Host, schemePort(req.URL.Scheme))
	default:
		writeResponse(conn, req, http.StatusBadRequest, nil, "proxy requests must use an absolute URI")
		return
	}
	target := p.explicitTarget(host, port)
	upstream, err := p.connectUpstream(ctx, target, codersdk.ExitNodeProtocolTCP)
	if err != nil {
		p.writeUpstreamError(ctx, conn, req, target, err)
		return
	}
	defer upstream.Close()

	if req.Method == http.MethodConnect {
		if _, err := io.WriteString(conn, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
			return
		}
	} else {
		// Rewrite the proxy request into an origin-form request. RequestURI
		// must be empty for Request.Write, which then emits the path.
		// Hop-by-hop proxy headers are not meaningful to the origin.
		req.RequestURI = ""
		req.Header.Del("Proxy-Connection")
		req.Header.Del("Proxy-Authorization")
		if err := req.Write(upstream); err != nil {
			p.logger.Debug(ctx, "forward proxied request", slog.Error(err))
			return
		}
	}
	pipe(ctx, conn, br, upstream)
}

func (p *Proxy) writeUpstreamError(ctx context.Context, conn net.Conn, req *http.Request, target string, err error) {
	p.logUpstreamError(ctx, target, err)
	if denied, ok := errors.AsType[*DeniedError](err); ok {
		hdr := http.Header{codersdk.ExitNodeDenyReasonHeader: []string{denied.Reason}}
		if denied.Rule != "" {
			hdr.Set(codersdk.ExitNodeDenyRuleHeader, denied.Rule)
		}
		writeResponse(conn, req, http.StatusForbidden, hdr, denied.Error())
		return
	}
	writeResponse(conn, req, http.StatusBadGateway, nil, "exit node: "+err.Error())
}

func (p *Proxy) logUpstreamError(ctx context.Context, target string, err error) {
	if denied, ok := errors.AsType[*DeniedError](err); ok {
		p.logger.Info(ctx, "egress denied by exit node",
			slog.F("target", target), slog.F("reason", denied.Reason), slog.F("rule", denied.Rule))
		return
	}
	p.logger.Warn(ctx, "egress connection through exit node failed", slog.F("target", target), slog.Error(err))
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
	return p.connectUpstreamHost(ctx, target, proto, "")
}

type connectOutcome int

const (
	connectOutcomeFailure connectOutcome = iota
	connectOutcomeSuccess
	connectOutcomeTerminal
)

func (p *Proxy) connectUpstreamHost(ctx context.Context, target string, proto codersdk.ExitNodeProtocol, originalHost string) (net.Conn, error) {
	p.configMu.RLock()
	selector := p.exitNodes
	p.configMu.RUnlock()
	var errs []error
	for range selector.len() {
		upstream, addr, err := selector.dial(ctx, p.dialer)
		if err != nil {
			errs = append(errs, err)
			break
		}
		conn, outcome, err := p.negotiateConnect(ctx, upstream, target, proto, originalHost)
		switch outcome {
		case connectOutcomeSuccess, connectOutcomeTerminal:
			if selector.markHealthy(ctx, addr) {
				if proto == codersdk.ExitNodeProtocolDNS {
					p.clearDNSDecisions()
				} else {
					p.resetDNS()
				}
			}
			return conn, err
		case connectOutcomeFailure:
			selector.markFailure(addr)
		}
		errs = append(errs, xerrors.Errorf("negotiate CONNECT with %s: %w", addr, err))
		if ctx.Err() != nil {
			break
		}
	}
	return nil, xerrors.Errorf("connect through exit nodes: %w", errors.Join(errs...))
}

func (p *Proxy) negotiateConnect(ctx context.Context, upstream net.Conn, target string, proto codersdk.ExitNodeProtocol, originalHost string) (net.Conn, connectOutcome, error) {
	defer func() {
		if upstream != nil {
			_ = upstream.Close()
		}
	}()
	deadline := time.Now().Add(p.connectTimeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	if err := upstream.SetDeadline(deadline); err != nil {
		return nil, connectOutcomeFailure, xerrors.Errorf("set CONNECT deadline: %w", err)
	}
	stop := context.AfterFunc(ctx, func() { _ = upstream.Close() })
	defer stop()
	connectReq := &http.Request{
		Method: http.MethodConnect,
		URL:    &url.URL{Opaque: target},
		Host:   target,
		Header: http.Header{},
	}
	if originalHost != "" {
		connectReq.Header.Set(codersdk.ExitNodeOriginalHostHeader, originalHost)
	}
	if proto != codersdk.ExitNodeProtocolTCP {
		connectReq.Header.Set(codersdk.ExitNodeProtocolHeader, string(proto))
	}
	if err := connectReq.Write(upstream); err != nil {
		return nil, connectOutcomeFailure, xerrors.Errorf("write CONNECT to exit node: %w", err)
	}
	br := bufio.NewReader(upstream)
	resp, err := http.ReadResponse(br, connectReq)
	if err != nil {
		return nil, connectOutcomeFailure, xerrors.Errorf("read CONNECT response from exit node: %w", err)
	}
	defer resp.Body.Close()
	_ = upstream.SetDeadline(time.Time{})
	stop()
	switch resp.StatusCode {
	case http.StatusOK:
		conn := &bufferedConn{Conn: upstream, r: br}
		upstream = nil
		return conn, connectOutcomeSuccess, nil
	case http.StatusForbidden:
		return nil, connectOutcomeTerminal, &DeniedError{
			Target: target,
			Reason: resp.Header.Get(codersdk.ExitNodeDenyReasonHeader),
			Rule:   resp.Header.Get(codersdk.ExitNodeDenyRuleHeader),
		}
	case http.StatusBadRequest, http.StatusBadGateway:
		return nil, connectOutcomeTerminal, xerrors.Errorf("exit node responded %s to CONNECT %s", resp.Status, target)
	default:
		if resp.StatusCode >= 500 {
			return nil, connectOutcomeFailure, xerrors.Errorf("exit node responded %s to CONNECT %s", resp.Status, target)
		}
		return nil, connectOutcomeTerminal, xerrors.Errorf("exit node responded %s to CONNECT %s", resp.Status, target)
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

// writeResponse sends a plain text HTTP response with msg as its body. hdr,
// when non-nil, supplies additional headers.
func writeResponse(conn net.Conn, req *http.Request, status int, hdr http.Header, msg string) {
	if hdr == nil {
		hdr = http.Header{}
	}
	hdr.Set("Content-Type", "text/plain; charset=utf-8")
	body := msg + "\n"
	resp := &http.Response{
		StatusCode:    status,
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        hdr,
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

// addrPortOf returns the address a socket is bound to.
func addrPortOf(addr net.Addr) (netip.AddrPort, error) {
	if addr == nil {
		return netip.AddrPort{}, xerrors.New("socket has no local address")
	}
	ap, err := netip.ParseAddrPort(addr.String())
	if err != nil {
		return netip.AddrPort{}, xerrors.Errorf("parse local address %q: %w", addr, err)
	}
	return ap, nil
}

func schemePort(scheme string) uint16 {
	if strings.EqualFold(scheme, "https") {
		return 443
	}
	return 80
}

package exitnode

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/quartz"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/tailnet"
)

const (
	// DefaultDialTimeout bounds the upstream TCP connect.
	DefaultDialTimeout = 10 * time.Second
	// DefaultResolveTimeout bounds resolving a CONNECT hostname.
	DefaultResolveTimeout = 5 * time.Second
	// connectReadTimeout bounds how long a client may take to send the
	// CONNECT request line and headers.
	connectReadTimeout = 10 * time.Second
	// maxConnectHeaderBytes bounds the CONNECT request to defend the parser.
	maxConnectHeaderBytes = 8 * 1024
	// maxHostnameLen is the DNS limit on a presentation-format name.
	maxHostnameLen = 253

	// resolveFailedReason is the deny reason header value on a 502 when a
	// CONNECT hostname could not be resolved.
	resolveFailedReason = "resolve failed"
)

// AgentResolver maps a tailnet source address to the agent that owns it.
type AgentResolver interface {
	AgentForAddr(netip.Addr) (uuid.UUID, bool)
}

// HostResolver resolves CONNECT hostnames. net.DefaultResolver satisfies it.
type HostResolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

// AgentTable is an AgentResolver keyed by the deterministic tailnet address
// of each agent. The coordinator forbids agents from advertising any other
// address, so the source IP of an accepted connection identifies the agent.
type AgentTable struct {
	mu     sync.RWMutex
	byAddr map[netip.Addr]uuid.UUID
}

var _ AgentResolver = (*AgentTable)(nil)

// NewAgentTable creates an empty table.
func NewAgentTable() *AgentTable {
	return &AgentTable{byAddr: make(map[netip.Addr]uuid.UUID)}
}

// Set replaces the table contents with the given agents.
func (t *AgentTable) Set(agentIDs []uuid.UUID) {
	byAddr := make(map[netip.Addr]uuid.UUID, len(agentIDs))
	for _, id := range agentIDs {
		byAddr[tailnet.TailscaleServicePrefix.AddrFromUUID(id)] = id
	}
	t.mu.Lock()
	t.byAddr = byAddr
	t.mu.Unlock()
}

// AgentForAddr implements AgentResolver.
func (t *AgentTable) AgentForAddr(addr netip.Addr) (uuid.UUID, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	id, ok := t.byAddr[addr.Unmap()]
	return id, ok
}

// DialFunc dials an upstream destination.
type DialFunc func(ctx context.Context, network, addr string) (net.Conn, error)

// ConnectProxyOptions configures a ConnectProxy.
type ConnectProxyOptions struct {
	Logger  slog.Logger
	Policy  PolicyEvaluator
	Agents  AgentResolver
	Flows   FlowRecorder
	Metrics *Metrics
	// Dialer dials upstream destinations, tcp and udp. Defaults to a
	// net.Dialer bounded by DialTimeout.
	Dialer DialFunc
	// DialTimeout bounds the upstream connect. Defaults to
	// DefaultDialTimeout.
	DialTimeout time.Duration
	// SniffTimeout bounds how long to wait for the client's first bytes.
	// Defaults to DefaultSniffTimeout.
	SniffTimeout time.Duration
	// Resolver resolves CONNECT hostnames. Defaults to net.DefaultResolver.
	Resolver HostResolver
	// ResolveTimeout bounds one hostname lookup. Defaults to
	// DefaultResolveTimeout.
	ResolveTimeout time.Duration
	// UDPIdleTimeout closes a udp stream with no traffic. Defaults to
	// DefaultUDPIdleTimeout.
	UDPIdleTimeout time.Duration
	// DNSExchanger answers dns streams. Defaults to the servers listed in
	// /etc/resolv.conf; when that file is unusable every query fails with
	// SERVFAIL and the error is logged once at startup.
	DNSExchanger DNSExchanger
	// DNSReportSampleRate reports one in N allowed dns queries. Denied
	// queries are always reported. Defaults to DefaultDNSReportSampleRate.
	DNSReportSampleRate int
	// Clock is for testing only.
	Clock quartz.Clock
}

// ConnectProxy terminates HTTP CONNECT requests from workspace agents,
// enforces policy, and relays the resulting flow while reporting it.
//
// Wire protocol, as seen from the agent:
//
//	CONNECT <host-or-ip>:<port> HTTP/1.1
//	Host: <host-or-ip>:<port>
//	X-Coder-Protocol: tcp | udp | dns   (optional, default tcp)
//	X-Coder-Original-Host: <name>       (optional, ip targets only)
//
// The proxy answers one of
//
//	HTTP/1.1 400 Bad Request        malformed target or protocol
//	HTTP/1.1 502 Bad Gateway        hostname did not resolve
//	  X-Coder-Deny-Reason: resolve failed
//	HTTP/1.1 403 Forbidden          denied by policy
//	  X-Coder-Deny-Reason: <reason>
//	  X-Coder-Deny-Rule: <rule id, if any>
//
// and closes, or
//
//	HTTP/1.1 200 Connection Established
//
// after which the stream carries the selected protocol.
//
// tcp: a raw byte pipe to the destination. When the target is a hostname the
// exit node resolves it (preferring IPv4), the name is known up front, and
// the policy decision is final before the 200. When the target is an IP
// literal the name is only learnable from the application bytes (TLS SNI or
// HTTP Host), which the application does not send until the agent has
// relayed the 200, so policy runs in two phases:
//
//  1. Before replying, the policy is evaluated with HostUnknown set. IP, CIDR,
//     and port based denials are final here and produce the 403 with the
//     reason header, which the agent can surface.
//  2. If phase one allowed (possibly provisionally), the proxy replies 200,
//     sniffs the first client bytes with a short deadline, and evaluates again
//     with the sniffed host. A denial here can no longer be signaled in HTTP,
//     so the connection is simply closed, which the application observes as a
//     reset or EOF. Both phases record the flow.
//
// An X-Coder-Original-Host header on an IP literal target supplies the name
// directly and skips sniffing.
//
// udp: the stream carries datagrams framed as a 2-byte big-endian length
// followed by the payload, in both directions. One UDP socket is opened to
// the target per stream. Names are known only from the target or the
// Original-Host header; there is no sniffing, so an IP literal udp flow is
// evaluated with an empty host. The stream closes after UDPIdleTimeout
// without traffic.
//
// dns: the target must be dns:53. The stream carries DNS messages with the
// same 2-byte length framing, and every message is evaluated and answered
// individually; see handleDNS.
type ConnectProxy struct {
	logger         slog.Logger
	policy         PolicyEvaluator
	agents         AgentResolver
	flows          FlowRecorder
	metrics        *Metrics
	dial           DialFunc
	dialTimeout    time.Duration
	sniffTimeout   time.Duration
	resolver       HostResolver
	resolveTimeout time.Duration
	udpIdleTimeout time.Duration
	dnsExchanger   DNSExchanger
	dnsSampleRate  int
	clock          quartz.Clock

	dnsSampleCounter atomic.Uint64
	wg               sync.WaitGroup
}

// NewConnectProxy creates a proxy. Serve or HandleConn must be called to do
// any work.
func NewConnectProxy(opts ConnectProxyOptions) *ConnectProxy {
	if opts.DialTimeout <= 0 {
		opts.DialTimeout = DefaultDialTimeout
	}
	if opts.SniffTimeout <= 0 {
		opts.SniffTimeout = DefaultSniffTimeout
	}
	if opts.ResolveTimeout <= 0 {
		opts.ResolveTimeout = DefaultResolveTimeout
	}
	if opts.UDPIdleTimeout <= 0 {
		opts.UDPIdleTimeout = DefaultUDPIdleTimeout
	}
	if opts.DNSReportSampleRate <= 0 {
		opts.DNSReportSampleRate = DefaultDNSReportSampleRate
	}
	if opts.Clock == nil {
		opts.Clock = quartz.NewReal()
	}
	if opts.Dialer == nil {
		d := &net.Dialer{Timeout: opts.DialTimeout}
		opts.Dialer = d.DialContext
	}
	if opts.Resolver == nil {
		opts.Resolver = net.DefaultResolver
	}
	if opts.DNSExchanger == nil {
		servers, err := ResolvConfServers(resolvConfPath)
		if err != nil {
			opts.Logger.Warn(context.Background(), "no upstream dns servers; dns streams will fail", slog.Error(err))
		}
		opts.DNSExchanger = &StaticDNSExchanger{Servers: servers, Dial: opts.Dialer}
	}
	if opts.Metrics == nil {
		opts.Metrics = NewMetrics(nil)
	}
	if opts.Flows == nil {
		opts.Flows = discardFlows{}
	}
	return &ConnectProxy{
		logger:         opts.Logger,
		policy:         opts.Policy,
		agents:         opts.Agents,
		flows:          opts.Flows,
		metrics:        opts.Metrics,
		dial:           opts.Dialer,
		dialTimeout:    opts.DialTimeout,
		sniffTimeout:   opts.SniffTimeout,
		resolver:       opts.Resolver,
		resolveTimeout: opts.ResolveTimeout,
		udpIdleTimeout: opts.UDPIdleTimeout,
		dnsExchanger:   opts.DNSExchanger,
		dnsSampleRate:  opts.DNSReportSampleRate,
		clock:          opts.Clock,
	}
}

type discardFlows struct{}

func (discardFlows) Record(codersdk.ExitNodeFlowReport) {}

// Serve accepts connections from ln until it is closed or ctx is canceled,
// handling each in its own goroutine. It returns nil when the listener is
// closed after ctx is done, and the accept error otherwise.
func (p *ConnectProxy) Serve(ctx context.Context, ln net.Listener) error {
	stop := context.AfterFunc(ctx, func() { _ = ln.Close() })
	defer stop()
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return xerrors.Errorf("accept: %w", err)
		}
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			p.HandleConn(ctx, conn)
		}()
	}
}

// Wait blocks until every connection handed to Serve has finished.
func (p *ConnectProxy) Wait() {
	p.wg.Wait()
}

// HandleConn serves one accepted connection and always closes it. Canceling
// ctx closes the connection so shutdown does not wait on idle peers.
func (p *ConnectProxy) HandleConn(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()

	srcAddr, ok := remoteAddr(conn)
	if !ok {
		p.metrics.UnknownSourceTotal.Inc()
		p.logger.Warn(ctx, "rejecting connection with unparseable source address",
			slog.F("remote_addr", conn.RemoteAddr()))
		return
	}
	agentID, ok := p.agents.AgentForAddr(srcAddr)
	if !ok {
		p.metrics.UnknownSourceTotal.Inc()
		p.logger.Warn(ctx, "rejecting connection from unknown source",
			slog.F("src", srcAddr))
		return
	}
	logger := p.logger.With(slog.F("agent_id", agentID), slog.F("src", srcAddr))

	if err := conn.SetReadDeadline(time.Now().Add(connectReadTimeout)); err != nil {
		logger.Debug(ctx, "set connect read deadline", slog.Error(err))
		return
	}
	br := bufio.NewReaderSize(io.LimitReader(conn, maxConnectHeaderBytes), 4096)
	req, err := http.ReadRequest(br)
	if err != nil {
		logger.Debug(ctx, "read connect request", slog.Error(err))
		return
	}
	if err := conn.SetReadDeadline(time.Time{}); err != nil {
		logger.Debug(ctx, "clear connect read deadline", slog.Error(err))
		return
	}

	target, err := parseConnectTarget(req)
	if err != nil {
		logger.Debug(ctx, "bad connect request", slog.Error(err))
		writeConnectResponse(conn, http.StatusBadRequest, http.Header{
			codersdk.ExitNodeDenyReasonHeader: []string{err.Error()},
		})
		return
	}
	logger = logger.With(slog.F("protocol", target.protocol), slog.F("dest", target.String()))

	// Any bytes bufio read past the CONNECT headers belong to the
	// application, so they are replayed ahead of the conn.
	client := conn
	if n := br.Buffered(); n > 0 {
		early, _ := br.Peek(n)
		client = &replayingConn{Conn: conn, r: io.MultiReader(bytes.NewReader(early), conn)}
	}

	switch target.protocol {
	case codersdk.ExitNodeProtocolDNS:
		p.handleDNS(ctx, logger, client, agentID)
	case codersdk.ExitNodeProtocolUDP:
		p.handleUDP(ctx, logger, client, agentID, target)
	default:
		p.handleTCP(ctx, logger, client, agentID, target)
	}
}

// resolveTarget fills in the destination address of a hostname target. It
// answers the CONNECT with 502 and returns false when resolution fails.
func (p *ConnectProxy) resolveTarget(ctx context.Context, logger slog.Logger, conn net.Conn, target *connectTarget) bool {
	if target.ip.IsValid() {
		return true
	}
	ip, err := p.resolve(ctx, target.host)
	if err != nil {
		logger.Debug(ctx, "resolve connect hostname", slog.Error(err))
		writeConnectResponse(conn, http.StatusBadGateway, http.Header{
			codersdk.ExitNodeDenyReasonHeader: []string{resolveFailedReason},
		})
		return false
	}
	target.ip = ip
	return true
}

// resolve looks up host and prefers an IPv4 address so the upstream path
// matches what most workspaces would have dialed themselves.
func (p *ConnectProxy) resolve(ctx context.Context, host string) (netip.Addr, error) {
	ctx, cancel := context.WithTimeout(ctx, p.resolveTimeout)
	defer cancel()
	addrs, err := p.resolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return netip.Addr{}, xerrors.Errorf("lookup %q: %w", host, err)
	}
	if len(addrs) == 0 {
		return netip.Addr{}, xerrors.Errorf("lookup %q: no addresses", host)
	}
	for _, addr := range addrs {
		if addr.Unmap().Is4() {
			return addr.Unmap(), nil
		}
	}
	return addrs[0].Unmap(), nil
}

func writeDeny(conn net.Conn, decision Decision) {
	hdr := http.Header{codersdk.ExitNodeDenyReasonHeader: []string{decision.Reason}}
	if decision.RuleID != "" {
		hdr.Set(codersdk.ExitNodeDenyRuleHeader, decision.RuleID)
	}
	writeConnectResponse(conn, http.StatusForbidden, hdr)
}

// handleTCP serves a tcp stream; see ConnectProxy for the phases.
func (p *ConnectProxy) handleTCP(ctx context.Context, logger slog.Logger, client net.Conn, agentID uuid.UUID, target connectTarget) {
	if !p.resolveTarget(ctx, logger, client, &target) {
		return
	}
	flow := codersdk.ExitNodeFlowReport{
		FlowID:          uuid.New(),
		AgentID:         agentID,
		Protocol:        codersdk.ExitNodeProtocolTCP,
		DestinationIP:   target.ip.String(),
		DestinationPort: int(target.port),
		Host:            target.host,
		ConnectTime:     p.clock.Now(),
	}
	hostKnown := target.host != ""

	// Phase one: decide on what is known before replying, so IP/CIDR/port
	// denials (and host denials when the name is known) reach the agent as
	// an HTTP 403 with a reason.
	decision := p.policy.Evaluate(FlowInfo{
		Protocol:    codersdk.ExitNodeProtocolTCP,
		Host:        target.host,
		IP:          target.ip,
		Port:        int(target.port),
		HostUnknown: !hostKnown,
	})
	if !decision.Allow {
		p.recordDeny(ctx, logger, flow, decision)
		writeDeny(client, decision)
		return
	}

	if _, err := io.WriteString(client, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		logger.Debug(ctx, "write connect response", slog.Error(err))
		return
	}

	if !hostKnown {
		// Phase two: the application now speaks and may reveal the name.
		host, sniffed, err := SniffHost(client, p.sniffTimeout)
		if err != nil && !errors.Is(err, io.EOF) {
			logger.Debug(ctx, "client connection failed during sniff", slog.Error(err))
			return
		}
		// An EOF here is a half-close after a partial first message; the
		// flow still proceeds with an unknown host so the peer's reply can
		// be relayed.
		client = sniffed
		flow.Host = host
		logger = logger.With(slog.F("host", host))

		decision = p.policy.Evaluate(FlowInfo{
			Protocol: codersdk.ExitNodeProtocolTCP,
			Host:     host,
			IP:       target.ip,
			Port:     int(target.port),
		})
		if !decision.Allow {
			// Too late for an HTTP status; closing is the only signal left.
			p.recordDeny(ctx, logger, flow, decision)
			return
		}
	}
	flow.Decision = codersdk.ExitNodeFlowAllow
	flow.RuleID = decision.RuleID
	flow.Reason = decision.Reason

	upstream, ok := p.dialUpstream(ctx, logger, "tcp", target, flow)
	if !ok {
		return
	}
	defer upstream.Close()

	p.metrics.FlowsTotal.WithLabelValues(string(codersdk.ExitNodeFlowAllow)).Inc()
	p.metrics.ActiveFlows.Inc()
	defer p.metrics.ActiveFlows.Dec()
	p.flows.Record(flow)
	logger.Debug(ctx, "flow allowed", slog.F("rule_id", decision.RuleID))

	bytesIn, bytesOut := pipe(ctx, client, upstream)
	p.recordDisconnect(ctx, logger, flow, bytesIn, bytesOut)
}

// handleUDP serves a udp stream: one policy decision, then framed datagrams
// relayed to a single UDP peer.
func (p *ConnectProxy) handleUDP(ctx context.Context, logger slog.Logger, client net.Conn, agentID uuid.UUID, target connectTarget) {
	if !p.resolveTarget(ctx, logger, client, &target) {
		return
	}
	flow := codersdk.ExitNodeFlowReport{
		FlowID:          uuid.New(),
		AgentID:         agentID,
		Protocol:        codersdk.ExitNodeProtocolUDP,
		DestinationIP:   target.ip.String(),
		DestinationPort: int(target.port),
		Host:            target.host,
		ConnectTime:     p.clock.Now(),
	}

	// There is no second phase for udp: nothing in the datagrams reveals a
	// name, so the decision is final now.
	decision := p.policy.Evaluate(FlowInfo{
		Protocol: codersdk.ExitNodeProtocolUDP,
		Host:     target.host,
		IP:       target.ip,
		Port:     int(target.port),
	})
	if !decision.Allow {
		p.recordDeny(ctx, logger, flow, decision)
		writeDeny(client, decision)
		return
	}
	flow.Decision = codersdk.ExitNodeFlowAllow
	flow.RuleID = decision.RuleID
	flow.Reason = decision.Reason

	if _, err := io.WriteString(client, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		logger.Debug(ctx, "write connect response", slog.Error(err))
		return
	}

	upstream, ok := p.dialUpstream(ctx, logger, "udp", target, flow)
	if !ok {
		return
	}
	defer upstream.Close()

	p.metrics.FlowsTotal.WithLabelValues(string(codersdk.ExitNodeFlowAllow)).Inc()
	p.metrics.ActiveFlows.Inc()
	defer p.metrics.ActiveFlows.Dec()
	p.flows.Record(flow)
	logger.Debug(ctx, "flow allowed", slog.F("rule_id", decision.RuleID))

	bytesIn, bytesOut := relayDatagrams(ctx, p.clock, client, upstream, p.udpIdleTimeout)
	p.recordDisconnect(ctx, logger, flow, bytesIn, bytesOut)
}

// dialUpstream connects to the resolved target. On failure the allowed flow
// is reported with an immediate disconnect so the audit trail shows the
// attempt, and ok is false.
func (p *ConnectProxy) dialUpstream(ctx context.Context, logger slog.Logger, network string, target connectTarget, flow codersdk.ExitNodeFlowReport) (net.Conn, bool) {
	dialCtx, cancel := context.WithTimeout(ctx, p.dialTimeout)
	defer cancel()
	upstream, err := p.dial(dialCtx, network, netip.AddrPortFrom(target.ip, target.port).String())
	if err != nil {
		logger.Debug(ctx, "dial upstream", slog.Error(err))
		now := p.clock.Now()
		flow.DisconnectTime = &now
		p.metrics.FlowsTotal.WithLabelValues(string(codersdk.ExitNodeFlowAllow)).Inc()
		p.flows.Record(flow)
		return nil, false
	}
	return upstream, true
}

func (p *ConnectProxy) recordDisconnect(ctx context.Context, logger slog.Logger, flow codersdk.ExitNodeFlowReport, bytesIn, bytesOut int64) {
	p.metrics.BytesTotal.WithLabelValues("in").Add(float64(bytesIn))
	p.metrics.BytesTotal.WithLabelValues("out").Add(float64(bytesOut))

	now := p.clock.Now()
	flow.BytesIn = bytesIn
	flow.BytesOut = bytesOut
	flow.DisconnectTime = &now
	p.flows.Record(flow)
	logger.Debug(ctx, "flow closed", slog.F("bytes_in", bytesIn), slog.F("bytes_out", bytesOut))
}

func (p *ConnectProxy) recordDeny(ctx context.Context, logger slog.Logger, flow codersdk.ExitNodeFlowReport, decision Decision) {
	flow.Decision = codersdk.ExitNodeFlowDeny
	flow.RuleID = decision.RuleID
	flow.Reason = decision.Reason
	now := p.clock.Now()
	flow.DisconnectTime = &now
	p.metrics.FlowsTotal.WithLabelValues(string(codersdk.ExitNodeFlowDeny)).Inc()
	p.flows.Record(flow)
	logger.Info(ctx, "flow denied by policy", slog.F("rule_id", decision.RuleID), slog.F("reason", decision.Reason))
}

// pipe copies bytes in both directions until both sides have finished or ctx
// is canceled. Each direction half-closes its destination when its source
// hits EOF so protocols that rely on shutdown(SHUT_WR) keep working. It
// returns bytes copied from upstream to client (in) and client to upstream
// (out).
func pipe(ctx context.Context, client, upstream net.Conn) (bytesIn, bytesOut int64) {
	stop := context.AfterFunc(ctx, func() {
		_ = client.Close()
		_ = upstream.Close()
	})
	defer stop()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		bytesOut, _ = io.Copy(upstream, client)
		_ = closeWrite(upstream)
	}()
	go func() {
		defer wg.Done()
		bytesIn, _ = io.Copy(client, upstream)
		_ = closeWrite(client)
	}()
	wg.Wait()
	return bytesIn, bytesOut
}

// connectTarget is a parsed CONNECT request.
type connectTarget struct {
	protocol codersdk.ExitNodeProtocol
	// host is the destination name: the CONNECT hostname, or the
	// X-Coder-Original-Host header for an IP literal target. Empty when the
	// agent supplied only an address.
	host string
	// ip is set when the target was an IP literal, and filled in by
	// resolution otherwise. It is unset for dns.
	ip   netip.Addr
	port uint16
}

func (t connectTarget) String() string {
	if t.host != "" {
		return net.JoinHostPort(t.host, strconv.Itoa(int(t.port)))
	}
	return netip.AddrPortFrom(t.ip, t.port).String()
}

// parseConnectTarget validates a CONNECT request and returns its target.
// Targets are "host:port" or "ip:port"; dns streams must target dns:53.
func parseConnectTarget(req *http.Request) (connectTarget, error) {
	if req.Method != http.MethodConnect {
		return connectTarget{}, xerrors.Errorf("method %s not allowed; use CONNECT", req.Method)
	}
	var target connectTarget
	switch proto := codersdk.ExitNodeProtocol(strings.ToLower(req.Header.Get(codersdk.ExitNodeProtocolHeader))); proto {
	case "", codersdk.ExitNodeProtocolTCP:
		target.protocol = codersdk.ExitNodeProtocolTCP
	case codersdk.ExitNodeProtocolUDP, codersdk.ExitNodeProtocolDNS:
		target.protocol = proto
	default:
		return connectTarget{}, xerrors.Errorf("%s %q must be tcp, udp, or dns", codersdk.ExitNodeProtocolHeader, proto)
	}

	raw := req.RequestURI
	if raw == "" {
		raw = req.Host
	}
	host, portStr, err := net.SplitHostPort(raw)
	if err != nil {
		return connectTarget{}, xerrors.Errorf("CONNECT target %q must be host:port", raw)
	}
	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil || port == 0 {
		return connectTarget{}, xerrors.Errorf("CONNECT target %q has an invalid port", raw)
	}
	target.port = uint16(port)

	if target.protocol == codersdk.ExitNodeProtocolDNS {
		if host != dnsTargetHost || target.port != dnsPort {
			return connectTarget{}, xerrors.Errorf("dns streams must target %s:%d, got %q", dnsTargetHost, dnsPort, raw)
		}
		return target, nil
	}

	if ip, err := netip.ParseAddr(host); err == nil {
		target.ip = ip.Unmap()
		if original := NormalizeHost(req.Header.Get(codersdk.ExitNodeOriginalHostHeader)); original != "" {
			if !validHostname(original) {
				return connectTarget{}, xerrors.Errorf("%s %q is not a valid hostname", codersdk.ExitNodeOriginalHostHeader, original)
			}
			target.host = original
		}
		return target, nil
	}
	host = NormalizeHost(host)
	if !validHostname(host) {
		return connectTarget{}, xerrors.Errorf("CONNECT target %q is not an ip or hostname", raw)
	}
	target.host = host
	return target, nil
}

// validHostname accepts names the resolver could plausibly look up. It is
// deliberately loose (underscores are allowed, for example) because the goal
// is to reject garbage and header injection, not to enforce RFC 1123.
func validHostname(host string) bool {
	if host == "" || len(host) > maxHostnameLen {
		return false
	}
	for _, r := range host {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '.', r == '_':
		default:
			return false
		}
	}
	return !strings.HasPrefix(host, ".") && !strings.Contains(host, "..")
}

// writeConnectResponse writes a bodiless HTTP/1.1 response and asks the
// client to close. Header values are sanitized so a policy reason can never
// inject headers.
func writeConnectResponse(w io.Writer, status int, hdr http.Header) {
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "HTTP/1.1 %d %s\r\n", status, http.StatusText(status))
	for k, vs := range hdr {
		for _, v := range vs {
			_, _ = fmt.Fprintf(&b, "%s: %s\r\n", k, sanitizeHeaderValue(v))
		}
	}
	_, _ = b.WriteString("Content-Length: 0\r\nConnection: close\r\n\r\n")
	_, _ = io.WriteString(w, b.String())
}

func sanitizeHeaderValue(v string) string {
	return strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' || r < 0x20 && r != '\t' {
			return ' '
		}
		return r
	}, v)
}

// remoteAddr extracts the source IP of a connection, unmapping IPv4-in-IPv6.
func remoteAddr(conn net.Conn) (netip.Addr, bool) {
	switch a := conn.RemoteAddr().(type) {
	case *net.TCPAddr:
		addr, ok := netip.AddrFromSlice(a.IP)
		return addr.Unmap(), ok
	default:
		if a == nil {
			return netip.Addr{}, false
		}
		ap, err := netip.ParseAddrPort(a.String())
		if err != nil {
			addr, err := netip.ParseAddr(a.String())
			return addr.Unmap(), err == nil
		}
		return ap.Addr().Unmap(), true
	}
}

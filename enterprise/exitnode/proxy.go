package exitnode

import (
	"bufio"
	"cmp"
	"context"
	"errors"
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

	"github.com/coder/coder/v2/agent/agentegress/hostsniff"
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

	connectEstablished = "HTTP/1.1 200 Connection Established\r\n\r\n"
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
	Policy  Policy
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
	// Defaults to hostsniff.DefaultTimeout.
	SniffTimeout time.Duration
	// Resolver resolves CONNECT hostnames. Defaults to net.DefaultResolver.
	Resolver HostResolver
	// ProvisionalHostAllow lets host-based allow rules provisionally allow an
	// IP-literal TCP target before its host is sniffed. This weakens strict
	// host enforcement and defaults to false.
	ProvisionalHostAllow bool
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

// ConnectProxy terminates agent CONNECT requests, enforces policy, proxies TCP,
// framed UDP and DNS traffic, and reports flows. Hostname TCP targets are
// resolved before policy evaluation. IP-literal TCP targets may be evaluated
// again after TLS SNI or HTTP Host sniffing. X-Coder-Original-Host is trusted
// only when it resolves to the target IP.
type ConnectProxy struct {
	ConnectProxyOptions
	dnsSampleCounter atomic.Int64
	wg               sync.WaitGroup
}

// NewConnectProxy creates a proxy. Serve or HandleConn must be called to do
// any work.
func NewConnectProxy(opts ConnectProxyOptions) *ConnectProxy {
	opts.DialTimeout = cmp.Or(max(opts.DialTimeout, 0), DefaultDialTimeout)
	opts.SniffTimeout = cmp.Or(max(opts.SniffTimeout, 0), hostsniff.DefaultTimeout)
	opts.ResolveTimeout = cmp.Or(max(opts.ResolveTimeout, 0), DefaultResolveTimeout)
	opts.UDPIdleTimeout = cmp.Or(max(opts.UDPIdleTimeout, 0), DefaultUDPIdleTimeout)
	opts.DNSReportSampleRate = cmp.Or(max(opts.DNSReportSampleRate, 0), DefaultDNSReportSampleRate)
	if opts.Clock == nil {
		opts.Clock = quartz.NewReal()
	}
	if opts.Dialer == nil {
		opts.Dialer = (&net.Dialer{Timeout: opts.DialTimeout}).DialContext
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
	return &ConnectProxy{ConnectProxyOptions: opts}
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
		p.wg.Go(func() { p.HandleConn(ctx, conn) })
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
		p.Metrics.UnknownSourceTotal.Inc()
		p.Logger.Warn(ctx, "rejecting connection with unparseable source address",
			slog.F("remote_addr", conn.RemoteAddr()))
		return
	}
	agentID, ok := p.Agents.AgentForAddr(srcAddr)
	if !ok {
		p.Metrics.UnknownSourceTotal.Inc()
		p.Logger.Warn(ctx, "rejecting connection from unknown source",
			slog.F("src", srcAddr))
		return
	}
	logger := p.Logger.With(slog.F("agent_id", agentID), slog.F("src", srcAddr))

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
		respondConnect(ctx, logger, conn, http.StatusBadRequest, Decision{Reason: err.Error()})
		return
	}
	logger = logger.With(slog.F("protocol", target.protocol), slog.F("dest", target.String()))

	// Any bytes bufio read past the CONNECT headers belong to the
	// application, so they are replayed ahead of the conn.
	client := conn
	if n := br.Buffered(); n > 0 {
		early, _ := br.Peek(n)
		client = hostsniff.Replay(conn, early)
	}

	if target.protocol == codersdk.ExitNodeProtocolDNS {
		p.handleDNS(ctx, logger, client, agentID)
		return
	}
	p.handleStream(ctx, logger, client, agentID, target)
}

// handleStream serves a tcp or udp stream; see ConnectProxy for the phases.
func (p *ConnectProxy) handleStream(ctx context.Context, logger slog.Logger, client net.Conn, agentID uuid.UUID, target connectTarget) {
	if target.ip.IsValid() && target.host != "" {
		if err := p.verifyOriginalHost(ctx, target.host, target.ip); err != nil {
			logger.Info(ctx, "ignoring unverified original host", slog.F("original_host", target.host), slog.F("destination_ip", target.ip), slog.Error(err))
			target.host = ""
		}
	}
	if !target.ip.IsValid() {
		ip, err := p.resolve(ctx, target.host)
		if err != nil {
			logger.Debug(ctx, "resolve connect hostname", slog.Error(err))
			respondConnect(ctx, logger, client, http.StatusBadGateway, Decision{Reason: resolveFailedReason})
			return
		}
		target.ip = ip
	}
	info := FlowInfo{Protocol: target.protocol, Host: target.host, IP: target.ip, Port: int(target.port)}
	sniff := target.protocol == codersdk.ExitNodeProtocolTCP && target.host == ""
	info.HostUnknown = sniff && p.ProvisionalHostAllow
	flow := newFlow(p.Clock.Now(), agentID, info)
	if !p.evaluateFlow(ctx, logger, &flow, info, "") {
		respondConnect(ctx, logger, client, http.StatusForbidden, Decision{RuleID: flow.RuleID, Reason: flow.Reason})
		return
	}
	if !respondConnect(ctx, logger, client, http.StatusOK, Decision{}) {
		return
	}

	if sniff {
		host, sniffed, err := hostsniff.Host(client, p.SniffTimeout)
		if err != nil && !errors.Is(err, io.EOF) {
			logger.Debug(ctx, "client connection failed during sniff", slog.Error(err))
			return
		}
		// Preserve a partial message after a client half-close.
		client = sniffed
		flow.Host, info.Host, info.HostUnknown = host, host, false
		logger = logger.With(slog.F("host", host))
		if !p.evaluateFlow(ctx, logger, &flow, info, "") {
			return
		}
	}
	p.allowFlow(&flow)

	dialCtx, cancel := context.WithTimeout(ctx, p.DialTimeout)
	upstream, err := p.Dialer(dialCtx, string(target.protocol), netip.AddrPortFrom(target.ip, target.port).String())
	cancel()
	if err != nil {
		logger.Debug(ctx, "dial upstream", slog.Error(err))
		p.recordEnded(flow)
		return
	}
	defer upstream.Close()

	p.Metrics.ActiveFlows.Inc()
	defer p.Metrics.ActiveFlows.Dec()
	p.Flows.Record(flow)
	logger.Debug(ctx, "flow allowed", slog.F("rule_id", flow.RuleID))

	if target.protocol == codersdk.ExitNodeProtocolUDP {
		flow.BytesIn, flow.BytesOut = relayDatagrams(ctx, p.Clock, client, upstream, p.UDPIdleTimeout)
	} else {
		flow.BytesIn, flow.BytesOut = pipe(ctx, client, upstream)
	}
	p.Metrics.BytesTotal.WithLabelValues("in").Add(float64(flow.BytesIn))
	p.Metrics.BytesTotal.WithLabelValues("out").Add(float64(flow.BytesOut))
	p.recordEnded(flow)
	logger.Debug(ctx, "flow closed", slog.F("bytes_in", flow.BytesIn), slog.F("bytes_out", flow.BytesOut))
}

func (p *ConnectProxy) lookup(ctx context.Context, host string) ([]netip.Addr, error) {
	ctx, cancel := context.WithTimeout(ctx, p.ResolveTimeout)
	defer cancel()
	addrs, err := p.Resolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, xerrors.Errorf("lookup %q: %w", host, err)
	}
	return addrs, nil
}

func (p *ConnectProxy) verifyOriginalHost(ctx context.Context, host string, ip netip.Addr) error {
	addrs, err := p.lookup(ctx, host)
	if err != nil {
		return err
	}
	ip = ip.Unmap()
	for _, addr := range addrs {
		if addr.Unmap() == ip {
			return nil
		}
	}
	return xerrors.Errorf("host %q does not resolve to %s", host, ip)
}

func (p *ConnectProxy) resolve(ctx context.Context, host string) (netip.Addr, error) {
	addrs, err := p.lookup(ctx, host)
	if err != nil {
		return netip.Addr{}, err
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

// recordEnded reports flow with the disconnect time stamped now.
func (p *ConnectProxy) recordEnded(flow codersdk.ExitNodeFlowReport) {
	now := p.Clock.Now()
	flow.DisconnectTime = &now
	p.Flows.Record(flow)
}

func newFlow(now time.Time, agentID uuid.UUID, info FlowInfo) codersdk.ExitNodeFlowReport {
	ip := ""
	if info.IP.IsValid() {
		ip = info.IP.String()
	}
	return codersdk.ExitNodeFlowReport{FlowID: uuid.New(), AgentID: agentID, Protocol: info.Protocol,
		DestinationIP: ip, DestinationPort: info.Port, Host: info.Host, ConnectTime: now}
}

func (p *ConnectProxy) evaluateFlow(ctx context.Context, logger slog.Logger, flow *codersdk.ExitNodeFlowReport, info FlowInfo, suffix string) bool {
	decision := p.Policy.Evaluate(info)
	decision.Reason += suffix
	setDecision(flow, decision, codersdk.ExitNodeFlowAllow)
	if decision.Allow {
		return true
	}
	flow.Decision = codersdk.ExitNodeFlowDeny
	p.Metrics.FlowsTotal.WithLabelValues(string(flow.Decision)).Inc()
	p.recordEnded(*flow)
	logger.Info(ctx, "flow denied by policy", slog.F("rule_id", decision.RuleID), slog.F("reason", decision.Reason))
	return false
}

func (p *ConnectProxy) allowFlow(flow *codersdk.ExitNodeFlowReport) {
	p.Metrics.FlowsTotal.WithLabelValues(string(flow.Decision)).Inc()
}

func setDecision(flow *codersdk.ExitNodeFlowReport, decision Decision, result codersdk.ExitNodeFlowDecision) {
	flow.Decision, flow.RuleID, flow.Reason = result, decision.RuleID, decision.Reason
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
	wg.Go(func() {
		bytesOut, _ = io.Copy(upstream, client)
		_ = hostsniff.CloseWrite(upstream)
	})
	wg.Go(func() {
		bytesIn, _ = io.Copy(client, upstream)
		_ = hostsniff.CloseWrite(client)
	})
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
		host = NormalizeHost(req.Header.Get(codersdk.ExitNodeOriginalHostHeader))
		if host != "" && !validHostname(host) {
			return connectTarget{}, xerrors.Errorf("%s %q is not a valid hostname", codersdk.ExitNodeOriginalHostHeader, host)
		}
	} else if host = NormalizeHost(host); !validHostname(host) {
		return connectTarget{}, xerrors.Errorf("CONNECT target %q is not an ip or hostname", raw)
	}
	target.host = host
	return target, nil
}

// validHostname accepts names the resolver could plausibly look up. It is
// deliberately loose (underscores are allowed, for example) because the goal
// is to reject garbage and header injection, not to enforce RFC 1123.
func validHostname(host string) bool {
	if host == "" || len(host) > maxHostnameLen || strings.HasPrefix(host, ".") || strings.Contains(host, "..") {
		return false
	}
	return !strings.ContainsFunc(host, func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' && r != '.' && r != '_'
	})
}

func respondConnect(ctx context.Context, logger slog.Logger, w io.Writer, status int, decision Decision) bool {
	hdr := make(http.Header)
	if decision.Reason != "" {
		hdr.Set(codersdk.ExitNodeDenyReasonHeader, decision.Reason)
	}
	if decision.RuleID != "" {
		hdr.Set(codersdk.ExitNodeDenyRuleHeader, decision.RuleID)
	}
	if status == http.StatusOK {
		if _, err := io.WriteString(w, connectEstablished); err != nil {
			logger.Debug(ctx, "write connect response", slog.Error(err))
			return false
		}
		return true
	}
	writeConnectResponse(w, status, hdr)
	return true
}

// writeConnectResponse writes a bodiless HTTP/1.1 response and asks the
// client to close. Header values are sanitized so a policy reason can never
// inject headers or control characters.
func writeConnectResponse(w io.Writer, status int, hdr http.Header) {
	for _, vs := range hdr {
		for i, v := range vs {
			vs[i] = strings.Map(func(r rune) rune {
				if r < 0x20 && r != '\t' {
					return ' '
				}
				return r
			}, v)
		}
	}
	_ = (&http.Response{StatusCode: status, ProtoMajor: 1, ProtoMinor: 1, Header: hdr, Close: true}).Write(w)
}

// remoteAddr extracts the source IP of a connection, unmapping IPv4-in-IPv6.
func remoteAddr(conn net.Conn) (netip.Addr, bool) {
	ra := conn.RemoteAddr()
	if a, ok := ra.(*net.TCPAddr); ok {
		addr, ok := netip.AddrFromSlice(a.IP)
		return addr.Unmap(), ok
	}
	if ra == nil {
		return netip.Addr{}, false
	}
	if ap, err := netip.ParseAddrPort(ra.String()); err == nil {
		return ap.Addr().Unmap(), true
	}
	addr, err := netip.ParseAddr(ra.String())
	return addr.Unmap(), err == nil
}

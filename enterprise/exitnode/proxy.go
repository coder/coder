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
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/quartz"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/tailnet"
)

const (
	// DenyReasonHeader carries the policy reason on a 403 CONNECT response so
	// the agent can log why a destination was refused.
	DenyReasonHeader = "X-Coder-Deny-Reason"
	// DenyRuleHeader carries the matching policy rule ID on a 403 CONNECT
	// response, when a rule (rather than the default) produced the denial.
	DenyRuleHeader = "X-Coder-Deny-Rule"

	// DefaultDialTimeout bounds the upstream TCP connect.
	DefaultDialTimeout = 10 * time.Second
	// connectReadTimeout bounds how long a client may take to send the
	// CONNECT request line and headers.
	connectReadTimeout = 10 * time.Second
	// maxConnectHeaderBytes bounds the CONNECT request to defend the parser.
	maxConnectHeaderBytes = 8 * 1024
)

// AgentResolver maps a tailnet source address to the agent that owns it.
type AgentResolver interface {
	AgentForAddr(netip.Addr) (uuid.UUID, bool)
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
	// Dialer dials upstream destinations. Defaults to a net.Dialer bounded by
	// DialTimeout.
	Dialer DialFunc
	// DialTimeout bounds the upstream connect. Defaults to
	// DefaultDialTimeout.
	DialTimeout time.Duration
	// SniffTimeout bounds how long to wait for the client's first bytes.
	// Defaults to DefaultSniffTimeout.
	SniffTimeout time.Duration
	// Clock is for testing only.
	Clock quartz.Clock
}

// ConnectProxy terminates HTTP CONNECT requests from workspace agents,
// enforces policy, and proxies the resulting TCP flow while reporting it.
//
// Wire protocol, as seen from the agent:
//
//	CONNECT <ip>:<port> HTTP/1.1
//	Host: <ip>:<port>
//
// The target must be an IP literal; the agent has already resolved DNS. The
// proxy answers either
//
//	HTTP/1.1 403 Forbidden
//	X-Coder-Deny-Reason: <reason>
//	X-Coder-Deny-Rule: <rule id, if any>
//
// and closes, or
//
//	HTTP/1.1 200 Connection Established
//
// after which the connection is a raw byte pipe to the destination.
//
// Policy is applied in two phases because a host name is only learnable from
// the application bytes (TLS SNI or HTTP Host), and the application does not
// send those until the agent has relayed the 200:
//
//  1. Before replying, the policy is evaluated with HostUnknown set. IP, CIDR,
//     and port based denials are final here and produce the 403 with the
//     reason header, which the agent can surface.
//  2. If phase one allowed (possibly provisionally), the proxy replies 200,
//     sniffs the first client bytes with a short deadline, and evaluates again
//     with the sniffed host. A denial here can no longer be signaled in HTTP,
//     so the connection is simply closed, which the application observes as a
//     reset or EOF. Both phases record the flow.
type ConnectProxy struct {
	logger       slog.Logger
	policy       PolicyEvaluator
	agents       AgentResolver
	flows        FlowRecorder
	metrics      *Metrics
	dial         DialFunc
	dialTimeout  time.Duration
	sniffTimeout time.Duration
	clock        quartz.Clock

	wg sync.WaitGroup
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
	if opts.Clock == nil {
		opts.Clock = quartz.NewReal()
	}
	if opts.Dialer == nil {
		d := &net.Dialer{Timeout: opts.DialTimeout}
		opts.Dialer = d.DialContext
	}
	if opts.Metrics == nil {
		opts.Metrics = NewMetrics(nil)
	}
	if opts.Flows == nil {
		opts.Flows = discardFlows{}
	}
	return &ConnectProxy{
		logger:       opts.Logger,
		policy:       opts.Policy,
		agents:       opts.Agents,
		flows:        opts.Flows,
		metrics:      opts.Metrics,
		dial:         opts.Dialer,
		dialTimeout:  opts.DialTimeout,
		sniffTimeout: opts.SniffTimeout,
		clock:        opts.Clock,
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

	dest, err := parseConnectTarget(req)
	if err != nil {
		logger.Debug(ctx, "bad connect request", slog.Error(err))
		writeConnectResponse(conn, http.StatusBadRequest, http.Header{
			DenyReasonHeader: []string{err.Error()},
		})
		return
	}
	logger = logger.With(slog.F("dest", dest))

	flow := codersdk.ExitNodeFlowReport{
		FlowID:          uuid.New(),
		AgentID:         agentID,
		DestinationIP:   dest.Addr().String(),
		DestinationPort: int(dest.Port()),
		ConnectTime:     p.clock.Now(),
	}

	// Phase one: decide on what is known before replying, so IP/CIDR/port
	// denials reach the agent as an HTTP 403 with a reason.
	decision := p.policy.Evaluate(FlowInfo{
		IP:          dest.Addr(),
		Port:        int(dest.Port()),
		HostUnknown: true,
	})
	if !decision.Allow {
		p.recordDeny(ctx, logger, flow, decision)
		hdr := http.Header{DenyReasonHeader: []string{decision.Reason}}
		if decision.RuleID != "" {
			hdr.Set(DenyRuleHeader, decision.RuleID)
		}
		writeConnectResponse(conn, http.StatusForbidden, hdr)
		return
	}

	if _, err := io.WriteString(conn, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		logger.Debug(ctx, "write connect response", slog.Error(err))
		return
	}

	// Phase two: the application now speaks. Any bytes bufio read past the
	// CONNECT headers belong to it, so they are replayed ahead of the conn.
	client := conn
	if n := br.Buffered(); n > 0 {
		early, _ := br.Peek(n)
		client = &replayingConn{Conn: conn, r: io.MultiReader(bytes.NewReader(early), conn)}
	}
	host, client, err := SniffHost(client, p.sniffTimeout)
	if err != nil && !errors.Is(err, io.EOF) {
		logger.Debug(ctx, "client connection failed during sniff", slog.Error(err))
		return
	}
	// An EOF here is a half-close after a partial first message; the flow
	// still proceeds with an unknown host so the peer's reply can be relayed.
	flow.Host = host
	logger = logger.With(slog.F("host", host))

	decision = p.policy.Evaluate(FlowInfo{
		Host: host,
		IP:   dest.Addr(),
		Port: int(dest.Port()),
	})
	if !decision.Allow {
		// Too late for an HTTP status; closing is the only signal left.
		p.recordDeny(ctx, logger, flow, decision)
		return
	}
	flow.Decision = codersdk.ExitNodeFlowAllow
	flow.RuleID = decision.RuleID
	flow.Reason = decision.Reason

	dialCtx, cancel := context.WithTimeout(ctx, p.dialTimeout)
	upstream, err := p.dial(dialCtx, "tcp", dest.String())
	cancel()
	if err != nil {
		logger.Debug(ctx, "dial upstream", slog.Error(err))
		// The flow was allowed but never established; report it with an
		// immediate disconnect so the audit trail shows the attempt.
		now := p.clock.Now()
		flow.DisconnectTime = &now
		p.metrics.FlowsTotal.WithLabelValues(string(codersdk.ExitNodeFlowAllow)).Inc()
		p.flows.Record(flow)
		return
	}
	defer upstream.Close()

	p.metrics.FlowsTotal.WithLabelValues(string(codersdk.ExitNodeFlowAllow)).Inc()
	p.metrics.ActiveFlows.Inc()
	defer p.metrics.ActiveFlows.Dec()
	p.flows.Record(flow)
	logger.Debug(ctx, "flow allowed", slog.F("rule_id", decision.RuleID))

	bytesIn, bytesOut := pipe(ctx, client, upstream)
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

// parseConnectTarget validates a CONNECT request and returns its ip:port
// target. Only IP literals are accepted: name resolution is the agent's job,
// and policy needs the concrete address the agent intends to reach.
func parseConnectTarget(req *http.Request) (netip.AddrPort, error) {
	if req.Method != http.MethodConnect {
		return netip.AddrPort{}, xerrors.Errorf("method %s not allowed; use CONNECT", req.Method)
	}
	target := req.RequestURI
	if target == "" {
		target = req.Host
	}
	addrPort, err := netip.ParseAddrPort(target)
	if err != nil {
		return netip.AddrPort{}, xerrors.Errorf("CONNECT target %q must be an ip:port literal", target)
	}
	if addrPort.Port() == 0 {
		return netip.AddrPort{}, xerrors.Errorf("CONNECT target %q has no port", target)
	}
	return netip.AddrPortFrom(addrPort.Addr().Unmap(), addrPort.Port()), nil
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

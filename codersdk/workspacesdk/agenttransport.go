package workspacesdk

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/tailnet"
)

// AgentDialer opens a TCP connection to port on the agent a transport is
// bound to.
type AgentDialer func(ctx context.Context, port uint16) (net.Conn, error)

// AgentTransport sends HTTP requests to one agent. It rejects requests for
// any other host, and dials only ports its port rule allows.
type AgentTransport struct {
	agentID   uuid.UUID
	addr      netip.Addr
	logger    slog.Logger
	transport *http.Transport
}

// NewAgentTransport returns a transport for agentID. dial must connect to
// that agent. allowPort decides which ports may be dialed, and must always
// give the same answer for a port: pooled connections are reused without
// asking again. base supplies pool, TLS and HTTP/2 settings, and may be nil
// to keep no idle connections.
func NewAgentTransport(agentID uuid.UUID, dial AgentDialer, logger slog.Logger, allowPort func(port uint16) bool, base *http.Transport) *AgentTransport {
	t := &http.Transport{DisableKeepAlives: true}
	if base != nil {
		t = base.Clone()
	}
	// Only dial may open connections: it reaches this agent over the tailnet.
	t.Proxy = nil
	t.Dial = nil //nolint:staticcheck // Cleared so it cannot be used.
	t.DialTLS = nil
	t.DialTLSContext = nil
	t.DialContext = dialAgentPort(dial, allowPort)
	return &AgentTransport{
		agentID:   agentID,
		addr:      tailnet.TailscaleServicePrefix.AddrFromUUID(agentID),
		logger:    logger,
		transport: t,
	}
}

// Addr returns the agent's address. Request URLs must use it as their host.
func (t *AgentTransport) Addr() netip.Addr {
	return t.addr
}

type dialRequestContextKey struct{}

func (t *AgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// The dialer only receives the port, so without this check a request for
	// another host would silently reach this agent.
	if addr, err := netip.ParseAddr(req.URL.Hostname()); err != nil || addr != t.addr {
		if req.Body != nil {
			_ = req.Body.Close() // Required by the http.RoundTripper contract.
		}
		t.logger.Warn(req.Context(), "blocked workspace agent request to unintended host",
			slog.F("agent_id", t.agentID),
			slog.F("request_host", req.URL.Host),
			slog.F("agent_addr", t.addr),
		)
		return nil, xerrors.Errorf("request host %q does not match agent %s address %s", req.URL.Host, t.agentID, t.addr)
	}
	ctx := context.WithValue(req.Context(), dialRequestContextKey{}, req.Context())
	return t.transport.RoundTrip(req.WithContext(ctx))
}

func (t *AgentTransport) CloseIdleConnections() {
	t.transport.CloseIdleConnections()
}

// AgentAPIPool sets the idle connections an AgentAPITransport keeps. The zero
// value keeps none.
type AgentAPIPool struct {
	MaxIdleConns    int           // Per agent.
	IdleConnTimeout time.Duration // Must be positive and below AgentHTTPAPIServerIdleTimeout if MaxIdleConns > 0.
}

// AgentAPITransport sends requests to one agent's HTTP API server.
type AgentAPITransport struct{ t *AgentTransport }

// NewAgentAPITransport returns a transport for the HTTP API server of
// agentID. dial must connect to that agent.
func NewAgentAPITransport(agentID uuid.UUID, dial AgentDialer, logger slog.Logger, pool AgentAPIPool) (*AgentAPITransport, error) {
	if pool.MaxIdleConns < 0 {
		return nil, xerrors.Errorf("max idle connections %d must not be negative", pool.MaxIdleConns)
	}
	if pool.MaxIdleConns > 0 && (pool.IdleConnTimeout <= 0 || pool.IdleConnTimeout >= AgentHTTPAPIServerIdleTimeout) {
		return nil, xerrors.Errorf("idle connection timeout %s must be positive and below the agent's %s", pool.IdleConnTimeout, AgentHTTPAPIServerIdleTimeout)
	}
	return newAPITransport(agentID, dial, logger, pool), nil
}

func newAPITransport(agentID uuid.UUID, dial AgentDialer, logger slog.Logger, pool AgentAPIPool) *AgentAPITransport {
	var base *http.Transport
	if pool.MaxIdleConns > 0 {
		base = &http.Transport{
			MaxIdleConnsPerHost: pool.MaxIdleConns,
			IdleConnTimeout:     pool.IdleConnTimeout,
		}
	}
	return &AgentAPITransport{NewAgentTransport(agentID, dial, logger, func(port uint16) bool {
		return port == AgentHTTPAPIServerPort
	}, base)}
}

// Addr returns the agent's address. Request URLs must use it as their host.
func (t *AgentAPITransport) Addr() netip.Addr {
	return t.t.Addr()
}

func (t *AgentAPITransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.t.RoundTrip(req)
}

func (t *AgentAPITransport) CloseIdleConnections() {
	t.t.CloseIdleConnections()
}

// dialAgentPort dials the port of addr if allowed. http.Transport has already
// replaced an empty port with the scheme's default. The host of addr is
// checked in RoundTrip.
func dialAgentPort(dial AgentDialer, allowed func(port uint16) bool) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		if network != "tcp" {
			return nil, xerrors.Errorf("network %q is not tcp", network)
		}
		_, rawPort, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, xerrors.Errorf("split host port %q: %w", addr, err)
		}
		port, err := strconv.ParseUint(rawPort, 10, 16)
		if err != nil {
			return nil, xerrors.Errorf("parse port %q: %w", rawPort, err)
		}
		if !allowed(uint16(port)) {
			return nil, xerrors.Errorf("port %d is not allowed", port)
		}
		// http.Transport keeps a dial running after its request ends, so a
		// later request can use the connection. End it with the request:
		// a dial to an unreachable agent otherwise waits until the agent
		// answers, which may be never.
		if reqCtx, ok := ctx.Value(dialRequestContextKey{}).(context.Context); ok {
			var cancel context.CancelFunc
			ctx, cancel = context.WithCancel(ctx)
			defer cancel()
			stop := context.AfterFunc(reqCtx, cancel)
			defer stop()
		}
		return dial(ctx, uint16(port))
	}
}

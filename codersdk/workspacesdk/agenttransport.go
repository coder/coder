package workspacesdk

import (
	"context"
	"crypto/tls"
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

// AgentAPITransport sends requests to one agent's HTTP API server. It rejects
// requests for any other host or port.
type AgentAPITransport struct{ agentTransport }

// AgentAppTransport sends requests to one agent's apps, on ports at or above
// AgentMinimumListeningPort. It rejects requests for any other host.
type AgentAppTransport struct{ agentTransport }

const (
	// An idle connection holds about 65 KB in coderd and 95 KB in the
	// agent. One serves sequential tool calls; concurrent requests beyond
	// it dial.
	agentAPIMaxIdleConns = 1
	// Below AgentHTTPAPIServerIdleTimeout, so coderd closes idle connections
	// before the agent does.
	agentAPIIdleConnTimeout = 10 * time.Minute
)

// NewAgentAPITransport returns a transport that pools connections to the HTTP
// API server of agentID. dial must connect to that agent.
func NewAgentAPITransport(agentID uuid.UUID, dial AgentDialer, logger slog.Logger) *AgentAPITransport {
	return buildAgentAPITransport(agentID, dial, logger, true)
}

func buildAgentAPITransport(agentID uuid.UUID, dial AgentDialer, logger slog.Logger, keepAlive bool) *AgentAPITransport {
	return &AgentAPITransport{newAgentTransport(agentID, logger, &http.Transport{
		DialContext: dialAgentPort(dial, func(port uint16) bool {
			return port == AgentHTTPAPIServerPort
		}),
		DisableKeepAlives:   !keepAlive,
		MaxIdleConnsPerHost: agentAPIMaxIdleConns,
		IdleConnTimeout:     agentAPIIdleConnTimeout,
	})}
}

// NewAgentAppTransport returns a transport that pools connections to the apps
// of agentID. dial must connect to that agent.
func NewAgentAppTransport(agentID uuid.UUID, dial AgentDialer, logger slog.Logger) *AgentAppTransport {
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		panic("dev error: default transport is the wrong type")
	}
	t := base.Clone()
	// We do not want to respect the proxy settings from the environment, since
	// all network traffic happens over wireguard.
	t.Proxy = nil
	t.DialContext = dialAgentPort(dial, func(port uint16) bool {
		return port >= AgentMinimumListeningPort
	})
	// These options are mostly just picked at random, and they can likely be
	// fine-tuned further. Generally, users are running applications in dev mode
	// which can generate hundreds of requests per page load, so we increased
	// MaxIdleConnsPerHost from 2 to 6 and removed the limit of total idle
	// conns.
	t.MaxIdleConnsPerHost = 6
	t.MaxIdleConns = 0
	t.IdleConnTimeout = 10 * time.Minute
	// We intentionally don't verify the certificate chain here.
	// The connection to the workspace is already established and most
	// apps are already going to be accessed over plain HTTP, this config
	// simply allows apps being run over HTTPS to be accessed without error --
	// many of which may be using self-signed certs.
	t.TLSClientConfig = &tls.Config{
		MinVersion: tls.VersionTLS12,
		//nolint:gosec
		InsecureSkipVerify: true,
	}
	return &AgentAppTransport{newAgentTransport(agentID, logger, t)}
}

type agentTransport struct {
	agentID   uuid.UUID
	addr      netip.Addr
	logger    slog.Logger
	transport *http.Transport
}

func newAgentTransport(agentID uuid.UUID, logger slog.Logger, t *http.Transport) agentTransport {
	return agentTransport{
		agentID:   agentID,
		addr:      tailnet.TailscaleServicePrefix.AddrFromUUID(agentID),
		logger:    logger,
		transport: t,
	}
}

// Addr returns the agent's address. Request URLs must use it as their host.
func (t agentTransport) Addr() netip.Addr {
	return t.addr
}

type dialRequestContextKey struct{}

func (t agentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
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

func (t agentTransport) CloseIdleConnections() {
	t.transport.CloseIdleConnections()
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

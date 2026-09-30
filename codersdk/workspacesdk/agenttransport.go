package workspacesdk

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"strconv"
	"time"

	"golang.org/x/xerrors"
)

// AgentDialer opens a TCP connection to port on one agent. It receives no
// host, so a transport built on it reaches only that agent, whatever host a
// request names.
type AgentDialer func(ctx context.Context, port uint16) (net.Conn, error)

// AgentAPITransport sends requests to one agent's HTTP API server.
type AgentAPITransport struct{ agentTransport }

// AgentAppTransport sends requests to one agent's ports at or above
// AgentMinimumListeningPort.
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

// NewAgentAPITransport returns a transport that pools connections to one
// agent's HTTP API server.
func NewAgentAPITransport(dial AgentDialer) *AgentAPITransport {
	return &AgentAPITransport{agentTransport{&http.Transport{
		DialContext: dialAgentPort(dial, func(port uint16) bool {
			return port == AgentHTTPAPIServerPort
		}),
		MaxIdleConnsPerHost: agentAPIMaxIdleConns,
		IdleConnTimeout:     agentAPIIdleConnTimeout,
	}}}
}

// NewAgentAppTransport returns a transport that pools connections to one
// agent's apps.
func NewAgentAppTransport(dial AgentDialer) *AgentAppTransport {
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
	return &AgentAppTransport{agentTransport{t}}
}

type agentTransport struct {
	transport *http.Transport
}

type dialRequestContextKey struct{}

func (t agentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := context.WithValue(req.Context(), dialRequestContextKey{}, req.Context())
	return t.transport.RoundTrip(req.WithContext(ctx))
}

func (t agentTransport) CloseIdleConnections() {
	t.transport.CloseIdleConnections()
}

// dialAgentPort dials the port of addr on the agent if allowed. The host of
// addr is ignored.
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

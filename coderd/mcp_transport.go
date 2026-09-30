package coderd

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/httpmw"
)

// newMCPTransport serves tool requests over private in-memory connections. Using
// net/http on both ends preserves streaming and WebSocket support. No listener
// is exposed to the network, and no caller-supplied context values cross the
// connection, so authentication cannot reuse the outer request's precheck.
func newMCPTransport(handler http.Handler, accessURL *url.URL, apiKeyID string) (http.RoundTripper, func()) {
	ctx, cancel := context.WithCancel(context.Background())
	listener := &mcpListener{ctx: ctx, cancel: cancel, connections: make(chan net.Conn)}
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		BaseContext: func(net.Listener) context.Context {
			return httpmw.WithMCPDelegation(ctx, apiKeyID)
		},
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = server.Serve(listener)
	}()
	transport := &http.Transport{DialContext: listener.dialContext}
	return &mcpTransport{origin: *accessURL, transport: transport}, sync.OnceFunc(func() {
		cancel()
		transport.CloseIdleConnections()
		_ = server.Close()
		<-done
	})
}

type mcpTransport struct {
	origin    url.URL
	transport *http.Transport
}

func (t *mcpTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	reject := func(message string) (*http.Response, error) {
		if req.Body != nil {
			_ = req.Body.Close()
		}
		return nil, xerrors.New(message)
	}
	// Fail closed on redirects or future tools targeting another origin. There
	// is deliberately no network fallback that could leak the bearer token.
	if req.URL.Scheme != t.origin.Scheme || req.URL.Host != t.origin.Host || req.Host != t.origin.Host || req.URL.User != nil {
		return reject("MCP tool request must target the Coder deployment")
	}
	// Reject non-canonical paths before the router can collapse slashes or
	// traverse segments into a different endpoint.
	if path.Clean(req.URL.Path) != strings.TrimSuffix(req.URL.Path, "/") {
		return reject("MCP tool request must use a canonical REST path")
	}
	if (!strings.HasPrefix(req.URL.Path, "/api/v2/") && !strings.HasPrefix(req.URL.Path, "/api/experimental/")) ||
		strings.HasPrefix(req.URL.Path, "/api/experimental/mcp/") {
		return reject("MCP tool request must target an internal REST endpoint")
	}
	clone := req.Clone(req.Context())
	// Even for HTTPS deployments, these bytes only travel over net.Pipe.
	clone.URL.Scheme = "http"
	return t.transport.RoundTrip(clone)
}

type mcpListener struct {
	ctx         context.Context
	cancel      context.CancelFunc
	connections chan net.Conn
}

func (l *mcpListener) dialContext(ctx context.Context, _, _ string) (net.Conn, error) {
	client, server := net.Pipe()
	// Closing the MCP transport also closes hijacked WebSocket connections,
	// which http.Server.Close does not own after the upgrade.
	context.AfterFunc(l.ctx, func() {
		_ = client.Close()
		_ = server.Close()
	})
	select {
	case <-ctx.Done():
		_ = client.Close()
		_ = server.Close()
		return nil, ctx.Err()
	case <-l.ctx.Done():
		_ = client.Close()
		_ = server.Close()
		return nil, net.ErrClosed
	case l.connections <- server:
		return client, nil
	}
}

func (l *mcpListener) Accept() (net.Conn, error) {
	select {
	case <-l.ctx.Done():
		return nil, net.ErrClosed
	case conn := <-l.connections:
		return conn, nil
	}
}

func (l *mcpListener) Close() error {
	l.cancel()
	return nil
}

func (*mcpListener) Addr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)}
}

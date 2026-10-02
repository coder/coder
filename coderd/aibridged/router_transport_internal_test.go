package aibridged

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/coderd/aibridged/proto"
	"github.com/coder/coder/v2/testutil"
)

// A replaced router releases its idle upstream connections, while a handler
// acquired before replacement keeps serving on its own snapshot.
func TestReplaceProvidersClosesReplacedRouterIdleConnections(t *testing.T) {
	t.Parallel()

	server, upstreamURL, closed := newConnTrackingServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, r.RemoteAddr)
	}))
	ctx := testutil.Context(t, testutil.WaitShort)
	require.NoError(t, server.ReplaceProviders(ctx, []aibridge.Provider{aibridge.NewOpenAIProvider(config.OpenAI{BaseURL: upstreamURL})}))
	retained, err := server.GetRequestHandler(ctx, Request{})
	require.NoError(t, err)
	first := serveRecorded(ctx, retained)
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	require.NoError(t, server.ReplaceProviders(ctx, []aibridge.Provider{aibridge.NewOpenAIProvider(config.OpenAI{BaseURL: upstreamURL})}))
	require.Equal(t, first.Body.String(), testutil.RequireReceive(ctx, t, closed), "replacement must close the replaced router's idle connection")
	second := serveRecorded(ctx, retained)
	require.Equal(t, http.StatusOK, second.Code, second.Body.String())
	require.NotEqual(t, first.Body.String(), second.Body.String(), "the retained router must dial a new connection")
}

// Shutdown closes the current router's connections only after admitted
// requests drain, so an active request completes on its connection.
func TestShutdownClosesRouterIdleConnectionsAfterDrain(t *testing.T) {
	t.Parallel()

	started, release := make(chan string, 1), make(chan struct{})
	server, upstreamURL, closed := newConnTrackingServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started <- r.RemoteAddr
		select {
		case <-release:
			_, _ = io.WriteString(w, r.RemoteAddr)
		case <-r.Context().Done():
		}
	}))
	ctx := testutil.Context(t, testutil.WaitShort)
	require.NoError(t, server.ReplaceProviders(ctx, []aibridge.Provider{aibridge.NewOpenAIProvider(config.OpenAI{BaseURL: upstreamURL})}))
	handler, err := server.GetRequestHandler(ctx, Request{})
	require.NoError(t, err)
	served := make(chan *httptest.ResponseRecorder, 1)
	go func() { served <- serveRecorded(ctx, handler) }()
	addr := testutil.RequireReceive(ctx, t, started)
	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- server.Shutdown(ctx) }()
	require.Eventually(t, server.shuttingDown.Load, testutil.WaitShort, testutil.IntervalFast)
	select {
	case closedAddr := <-closed:
		t.Fatalf("shutdown closed connection %s before the request finished", closedAddr)
	default:
	}
	close(release)
	rec := testutil.RequireReceive(ctx, t, served)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, addr, rec.Body.String())
	require.NoError(t, testutil.RequireReceive(ctx, t, shutdownDone))
	require.Equal(t, addr, testutil.RequireReceive(ctx, t, closed), "shutdown must close the drained connection")
}

// endRecordingClient accepts the start and end records of proxied requests.
type endRecordingClient struct{ recordingClient }

func (*endRecordingClient) RecordInterceptionEnded(context.Context, *proto.RecordInterceptionEndedRequest) (*proto.RecordInterceptionEndedResponse, error) {
	return &proto.RecordInterceptionEndedResponse{}, nil
}

// newConnTrackingServer returns a proxy server with a recorder for routers it
// builds, the URL of a real upstream, and the client address of each upstream
// connection as it closes.
func newConnTrackingServer(t *testing.T, upstreamHandler http.Handler) (*Server, string, <-chan string) {
	t.Helper()
	closed := make(chan string, 8)
	upstream := httptest.NewUnstartedServer(upstreamHandler)
	upstream.Config.ConnState = func(c net.Conn, state http.ConnState) {
		if addr := c.RemoteAddr(); state == http.StateClosed && addr != nil {
			closed <- addr.String()
		}
	}
	upstream.Start()
	t.Cleanup(upstream.Close)
	server := newProxyTestServer(t, http.NotFoundHandler())
	client := &endRecordingClient{}
	server.recorder = newRecorder(server.logger, server.tracer, false, false, func(context.Context) (DRPCClient, error) { return client, nil })
	return server, upstream.URL, closed
}

// serveRecorded sends an authenticated BYOK request through a bridged route.
func serveRecorded(ctx context.Context, handler http.Handler) *httptest.ResponseRecorder {
	ctx = aibridge.AsActor(ctx, aibridge.Actor{ID: uuid.New(), APIKeyID: uuid.NewString()})
	req := httptest.NewRequest(http.MethodPost, "/openai/v1/chat/completions", nil).WithContext(ctx)
	req.Header.Set("Authorization", "Bearer byok-key")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

// A provider that fails forwarding construction after earlier providers built
// their transports leaves the previous router serving.
func TestReplaceProvidersMalformedBaseURLRetainsRouter(t *testing.T) {
	t.Parallel()

	server := newProxyTestServer(t, http.NotFoundHandler())
	ctx := testutil.Context(t, testutil.WaitShort)
	retained := server.backend.Load().proxyRouter
	require.NotNil(t, retained)

	err := server.ReplaceProviders(ctx, []aibridge.Provider{
		aibridge.NewOpenAIProvider(config.OpenAI{Name: "valid", BaseURL: "http://upstream.test"}),
		aibridge.NewOpenAIProvider(config.OpenAI{Name: "malformed", BaseURL: "http://upstream.test/%zz"}),
	})
	require.ErrorContains(t, err, `configure provider "malformed" base URL`)
	require.Same(t, retained, server.backend.Load().proxyRouter, "a failed snapshot must not replace the router")

	handler, err := server.GetRequestHandler(ctx, Request{})
	require.NoError(t, err)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/openai/v1/models", nil))
	require.Equal(t, http.StatusNotFound, rec.Code, "the retained router must keep proxying upstream")
}

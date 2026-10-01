package aibridged

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	"storj.io/drpc"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/coderd/aibridged/proto"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
	"github.com/coder/websocket"
)

const wsTestCreate = `{"type":"response.create","model":"gpt-5","input":[{"role":"user","content":[{"type":"input_text","text":"hello"}]}]}`

// wsTestConn is a drpc.Conn that stays connected.
type wsTestConn struct{ closed chan struct{} }

func (*wsTestConn) Close() error              { return nil }
func (c *wsTestConn) Closed() <-chan struct{} { return c.closed }
func (*wsTestConn) Transport() drpc.Transport { return nil }
func (*wsTestConn) Invoke(context.Context, string, drpc.Encoding, drpc.Message, drpc.Message) error {
	return nil
}

func (*wsTestConn) NewStream(context.Context, string, drpc.Encoding) (drpc.Stream, error) {
	//nolint:nilnil // Streams are not used.
	return nil, nil
}

// wsTestClient authorizes every request for one user with Responses
// WebSocket mode on, and records the interceptions it is sent.
type wsTestClient struct {
	DRPCClient // Unused methods panic.
	conn       *wsTestConn
	ownerID    uuid.UUID

	mu      sync.Mutex
	started []string
	ended   []string
}

func (c *wsTestClient) DRPCConn() drpc.Conn { return c.conn }

func (c *wsTestClient) IsAuthorized(context.Context, *proto.IsAuthorizedRequest) (*proto.IsAuthorizedResponse, error) {
	return &proto.IsAuthorizedResponse{OwnerId: c.ownerID.String(), ApiKeyId: "key-id", Username: "u", ResponsesWebsocketEnabled: true}, nil
}

func (*wsTestClient) IsBudgetExceeded(context.Context, *proto.IsBudgetExceededRequest) (*proto.IsBudgetExceededResponse, error) {
	return &proto.IsBudgetExceededResponse{}, nil
}

func (*wsTestClient) GetMCPServerConfigs(context.Context, *proto.GetMCPServerConfigsRequest) (*proto.GetMCPServerConfigsResponse, error) {
	return &proto.GetMCPServerConfigsResponse{}, nil
}

func (c *wsTestClient) RecordInterception(_ context.Context, in *proto.RecordInterceptionRequest) (*proto.RecordInterceptionResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.started = append(c.started, in.GetId())
	return &proto.RecordInterceptionResponse{}, nil
}

func (c *wsTestClient) RecordInterceptionEnded(_ context.Context, in *proto.RecordInterceptionEndedRequest) (*proto.RecordInterceptionEndedResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ended = append(c.ended, in.GetId())
	return &proto.RecordInterceptionEndedResponse{}, nil
}

func (*wsTestClient) RecordPromptUsage(context.Context, *proto.RecordPromptUsageRequest) (*proto.RecordPromptUsageResponse, error) {
	return &proto.RecordPromptUsageResponse{}, nil
}

func (*wsTestClient) RecordTokenUsage(context.Context, *proto.RecordTokenUsageRequest) (*proto.RecordTokenUsageResponse, error) {
	return &proto.RecordTokenUsageResponse{}, nil
}

func (*wsTestClient) RecordModelThought(context.Context, *proto.RecordModelThoughtRequest) (*proto.RecordModelThoughtResponse, error) {
	return &proto.RecordModelThoughtResponse{}, nil
}

func (c *wsTestClient) interceptions() (started, ended []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.started...), append([]string(nil), c.ended...)
}

// wsTestGateway is a Server behind the AI Gateway concurrency limit, with an
// OpenAI provider whose upstream accepts Responses WebSockets.
type wsTestGateway struct {
	srv    *Server
	url    string
	client *wsTestClient
	clock  *quartz.Mock
	// upstreams receives each accepted upstream connection.
	upstreams chan *websocket.Conn
	// handshakes counts upstream WebSocket handshakes.
	handshakes chan struct{}
}

func newWSTestGateway(t *testing.T, limits socketLimits, maxConcurrency int64) *wsTestGateway {
	t.Helper()
	g := &wsTestGateway{
		client:     &wsTestClient{conn: &wsTestConn{closed: make(chan struct{})}, ownerID: uuid.New()},
		clock:      quartz.NewMock(t),
		upstreams:  make(chan *websocket.Conn, 8),
		handshakes: make(chan struct{}, 8),
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			w.WriteHeader(http.StatusTeapot)
			return
		}
		g.handshakes <- struct{}{}
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("upstream accept: %v", err)
			return
		}
		g.upstreams <- conn
	}))
	t.Cleanup(upstream.Close)

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	srv, err := New(t.Context(), func(context.Context) (DRPCClient, error) { return g.client, nil }, logger, noop.NewTracerProvider().Tracer(""), nil, nil)
	require.NoError(t, err)
	srv.sockets = newSocketRegistry(g.clock, nil, limits)
	g.srv = srv
	t.Cleanup(func() { _ = srv.Shutdown(testutil.Context(t, testutil.WaitShort)) })
	pool, err := keypool.New("openai", []string{"sk-test"}, quartz.NewReal(), nil)
	require.NoError(t, err)
	require.NoError(t, srv.ReplaceProviders(t.Context(), []aibridge.Provider{
		aibridge.NewOpenAIProvider(aibridge.OpenAIConfig{BaseURL: upstream.URL, KeyPool: pool, ResponsesWebSocket: true}),
	}))

	front := httptest.NewServer(httpmw.ConcurrencyLimit(maxConcurrency, "AI Gateway")(srv))
	t.Cleanup(front.Close)
	g.url = front.URL
	return g
}

func (g *wsTestGateway) dial(ctx context.Context) (*websocket.Conn, *http.Response, error) {
	//nolint:bodyclose // Dial owns the response body.
	return websocket.Dial(ctx, "ws"+strings.TrimPrefix(g.url, "http")+"/openai/v1/responses", &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": {"Bearer coder-token"}},
	})
}

// open opens a socket and returns its client and upstream connections.
func (g *wsTestGateway) open(ctx context.Context, t *testing.T) (client, upstream *websocket.Conn) {
	t.Helper()
	//nolint:bodyclose // Dial owns the response body.
	client, _, err := g.dial(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.CloseNow() })
	upstream = testutil.TryReceive(ctx, t, g.upstreams)
	t.Cleanup(func() { _ = upstream.CloseNow() })
	return client, upstream
}

// relayCreate sends a create through the socket and waits for upstream to
// receive it, so the socket has an open interception.
func relayCreate(ctx context.Context, t *testing.T, client, upstream *websocket.Conn) {
	t.Helper()
	require.NoError(t, client.Write(ctx, websocket.MessageText, []byte(wsTestCreate)))
	_, msg, err := upstream.Read(ctx)
	require.NoError(t, err)
	require.Equal(t, wsTestCreate, string(msg))
}

// closeStatus reads conn in the background, answering its close, and
// reports the close status it received.
func closeStatus(ctx context.Context, conn *websocket.Conn) <-chan websocket.CloseError {
	ch := make(chan websocket.CloseError, 1)
	go func() {
		for {
			_, _, err := conn.Read(ctx)
			if err != nil {
				closeErr, _ := errors.AsType[websocket.CloseError](err)
				ch <- closeErr
				return
			}
		}
	}()
	return ch
}

// TestResponsesWebSocketCapacity requires that an open socket holds a lease
// but not a concurrency slot, and that a user at the socket cap is refused
// before anything is dialed upstream with 426, on which clients such as
// Codex fall back to HTTP.
func TestResponsesWebSocketCapacity(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	g := newWSTestGateway(t, socketLimits{perActor: 1, perReplica: 8, lifetime: maxSocketLifetime}, 1)
	client, upstream := g.open(ctx, t)
	_ = testutil.TryReceive(ctx, t, g.handshakes)

	// The only concurrency slot is free while the socket is open.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.url+"/openai/v1/models", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer coder-token")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusTeapot, resp.StatusCode)

	//nolint:bodyclose // Dial owns the response body.
	_, resp, err = g.dial(ctx)
	require.Error(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusUpgradeRequired, resp.StatusCode)
	assert.Empty(t, g.handshakes, "a refused socket must not dial upstream")

	// The first socket still relays.
	relayCreate(ctx, t, client, upstream)
}

// TestResponsesWebSocketOutlivesBridge requires that a socket keeps relaying
// and recording after the per-user bridge that served its upgrade shut
// down, as on pool eviction, which cancels the bridge's request contexts.
func TestResponsesWebSocketOutlivesBridge(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	g := newWSTestGateway(t, defaultSocketLimits, 0)
	client, upstream := g.open(ctx, t)

	handler, err := g.srv.GetRequestHandler(ctx, Request{InitiatorID: g.client.ownerID, APIKeyID: "key-id"})
	require.NoError(t, err)
	bridge, ok := handler.(*aibridge.RequestBridge)
	require.True(t, ok)
	expired, cancel := context.WithCancel(ctx)
	cancel()
	_ = bridge.Shutdown(expired)

	relayCreate(ctx, t, client, upstream)
	upstreamClosed := closeStatus(ctx, upstream)
	require.NoError(t, client.Close(websocket.StatusNormalClosure, ""))
	assert.Equal(t, websocket.StatusNormalClosure, testutil.TryReceive(ctx, t, upstreamClosed).Code)
	assert.Eventually(t, func() bool {
		started, ended := g.client.interceptions()
		return len(started) == 1 && assert.ObjectsAreEqual(started, ended)
	}, testutil.WaitShort, testutil.IntervalFast)
}

// TestResponsesWebSocketMaxLifetime requires that a socket at its maximum
// lifetime closes as going away on both sides and ends its interception.
func TestResponsesWebSocketMaxLifetime(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	g := newWSTestGateway(t, defaultSocketLimits, 0)
	client, upstream := g.open(ctx, t)
	relayCreate(ctx, t, client, upstream)
	clientClosed, upstreamClosed := closeStatus(ctx, client), closeStatus(ctx, upstream)

	g.clock.Advance(maxSocketLifetime).MustWait(ctx)

	for _, closed := range []<-chan websocket.CloseError{clientClosed, upstreamClosed} {
		closeErr := testutil.TryReceive(ctx, t, closed)
		assert.Equal(t, websocket.StatusGoingAway, closeErr.Code)
		assert.Equal(t, ErrSocketLifetime.Error(), closeErr.Reason)
	}
	assert.Eventually(t, func() bool {
		started, ended := g.client.interceptions()
		return len(started) == 1 && assert.ObjectsAreEqual(started, ended)
	}, testutil.WaitShort, testutil.IntervalFast)
}

// TestResponsesWebSocketShutdown requires that shutdown with an open socket
// closes it as going away, records the end of its interception while the
// coderd connection is still up, and finishes within its deadline.
func TestResponsesWebSocketShutdown(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	g := newWSTestGateway(t, defaultSocketLimits, 0)
	client, upstream := g.open(ctx, t)
	relayCreate(ctx, t, client, upstream)
	clientClosed, upstreamClosed := closeStatus(ctx, client), closeStatus(ctx, upstream)

	shutdownCtx, cancel := context.WithTimeout(ctx, testutil.WaitShort)
	defer cancel()
	require.NoError(t, g.srv.Shutdown(shutdownCtx))

	started, ended := g.client.interceptions()
	require.Len(t, started, 1)
	assert.Equal(t, started, ended)
	for _, closed := range []<-chan websocket.CloseError{clientClosed, upstreamClosed} {
		assert.Equal(t, websocket.StatusGoingAway, testutil.TryReceive(ctx, t, closed).Code)
	}
}

// TestResponsesWebSocketShutdownUnresponsivePeers requires that sockets
// whose peers never answer the going-away close take only their share of
// the shutdown deadline, so the rest of shutdown still completes in time,
// and still record the end of their interceptions before it.
func TestResponsesWebSocketShutdownUnresponsivePeers(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	g := newWSTestGateway(t, defaultSocketLimits, 0)
	// Neither connection is read again, so neither answers a close.
	client, upstream := g.open(ctx, t)
	relayCreate(ctx, t, client, upstream)

	// The deadline is below coder/websocket's 5 second wait for a close to
	// be answered, so sockets closing without their share of the deadline
	// would make shutdown miss it.
	shutdownCtx, cancel := context.WithTimeout(ctx, testutil.IntervalSlow*4)
	defer cancel()
	require.NoError(t, g.srv.Shutdown(shutdownCtx))

	started, ended := g.client.interceptions()
	require.Len(t, started, 1)
	assert.Equal(t, started, ended)
}

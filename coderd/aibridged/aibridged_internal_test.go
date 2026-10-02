package aibridged

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	"golang.org/x/xerrors"
	"storj.io/drpc"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/aibridge/config"
	aibcontext "github.com/coder/coder/v2/aibridge/context"
	"github.com/coder/coder/v2/aibridge/recorder"
	"github.com/coder/coder/v2/aibridge/x/proxy"
	"github.com/coder/coder/v2/coderd/aibridged/proto"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestNewRecorder(t *testing.T) {
	t.Parallel()
	drpcErr := xerrors.New("client unavailable")
	for _, tc := range []struct {
		structuredLogging       bool
		disableContentRecording bool
		wantClientCalls         int
		wantLogs                int
		wantContentErr          error // nil means no content recording is expected
	}{
		{
			wantClientCalls: 6,
			wantContentErr:  drpcErr,
		},
		{
			structuredLogging: true,
			wantClientCalls:   6,
			wantLogs:          6,
			wantContentErr:    drpcErr,
		},
		{
			disableContentRecording: true,
			wantClientCalls:         3,
		},
		{
			structuredLogging:       true,
			disableContentRecording: true,
			wantClientCalls:         3,
			wantLogs:                6,
		},
	} {
		t.Run(fmt.Sprintf("Structured=%t/DisableContent=%t", tc.structuredLogging, tc.disableContentRecording), func(t *testing.T) {
			t.Parallel()
			actor := aibridge.Actor{ID: uuid.New(), APIKeyID: uuid.NewString()}
			id := uuid.NewString()
			now := time.Now().UTC()
			sink := testutil.NewFakeSink(t)
			clientCalls := 0
			rec := newRecorder(sink.Logger(), noop.NewTracerProvider().Tracer(t.Name()), tc.structuredLogging, tc.disableContentRecording, func(ctx context.Context) (DRPCClient, error) {
				clientCalls++
				require.Equal(t, &actor, aibcontext.ActorFromContext(ctx), "the client must be acquired with the record call's actor")
				return nil, drpcErr
			})
			require.Zero(t, clientCalls, "creating a recorder must not acquire a client")
			ctx := aibridge.AsActor(t.Context(), actor)
			require.ErrorIs(t, rec.RecordInterception(ctx, &recorder.InterceptionRecord{ID: id, InitiatorID: actor.ID.String(), StartedAt: now}), drpcErr)
			require.ErrorIs(t, rec.RecordInterceptionEnded(ctx, &recorder.InterceptionRecordEnded{ID: id, EndedAt: now}), drpcErr)
			require.ErrorIs(t, rec.RecordTokenUsage(ctx, &recorder.TokenUsageRecord{InterceptionID: id, CreatedAt: now, Input: 1}), drpcErr)
			require.ErrorIs(t, rec.RecordPromptUsage(ctx, &recorder.PromptUsageRecord{InterceptionID: id, CreatedAt: now, Prompt: "prompt"}), tc.wantContentErr)
			require.ErrorIs(t, rec.RecordToolUsage(ctx, &recorder.ToolUsageRecord{InterceptionID: id, CreatedAt: now, Tool: "tool"}), tc.wantContentErr)
			require.ErrorIs(t, rec.RecordModelThought(ctx, &recorder.ModelThoughtRecord{InterceptionID: id, CreatedAt: now, Content: "thought"}), tc.wantContentErr)
			require.Equal(t, tc.wantClientCalls, clientCalls, "each forwarded record acquires its own client")
			logs := sink.Entries(func(entry slog.SinkEntry) bool { return entry.Message == recorder.InterceptionLogMarker })
			require.Len(t, logs, tc.wantLogs, "logging must precede content filtering")
			require.ErrorIs(t, rec.RecordPromptUsage(ctx, &recorder.PromptUsageRecord{}), recorder.ErrInvalidRecord, "validation must precede content filtering")
			require.Equal(t, tc.wantClientCalls, clientCalls)
		})
	}

	t.Run("RecorderSharedAcrossRequestsDoesNotMixActorData", func(t *testing.T) {
		t.Parallel()
		sink := testutil.NewFakeSink(t)
		client := &recordingClient{}
		rec := newRecorder(sink.Logger(), noop.NewTracerProvider().Tracer(t.Name()), true, false, func(context.Context) (DRPCClient, error) {
			return client, nil
		})
		record := func(t *testing.T) {
			t.Helper()
			actor := aibridge.Actor{ID: uuid.New(), APIKeyID: uuid.NewString()}
			in := &recorder.InterceptionRecord{ID: uuid.NewString(), InitiatorID: actor.ID.String(), StartedAt: time.Now().UTC()}
			require.NoError(t, rec.RecordInterception(aibridge.AsActor(t.Context(), actor), in))

			value, ok := client.requests.Load(in.ID)
			require.True(t, ok, "interception must reach the DRPC client")
			rpc := value.(*proto.RecordInterceptionRequest)
			require.Equal(t, in.InitiatorID, rpc.GetInitiatorId())
			require.Equal(t, actor.APIKeyID, rpc.GetApiKeyId())
			logs := sink.Entries(func(entry slog.SinkEntry) bool {
				return entry.Message == recorder.InterceptionLogMarker && slices.ContainsFunc(entry.Fields, func(field slog.Field) bool {
					return field.Name == "interception_id" && field.Value == in.ID
				})
			})
			require.Len(t, logs, 1)
			require.Contains(t, logs[0].Fields, slog.F("initiator_id", in.InitiatorID))
			require.Contains(t, logs[0].Fields, slog.F("api_key_id", actor.APIKeyID))
		}

		t.Run("Sequential", func(t *testing.T) {
			t.Parallel()
			record(t)
			record(t)
		})
		t.Run("Concurrent", func(t *testing.T) {
			t.Parallel()
			for i := range 2 {
				t.Run(fmt.Sprintf("Request%d", i), func(t *testing.T) {
					t.Parallel()
					record(t)
				})
			}
		})
	})

	t.Run("DetachedContext", func(t *testing.T) {
		t.Parallel()
		actor := aibridge.Actor{ID: uuid.New(), APIKeyID: uuid.NewString()}
		clientCalls := 0
		rec := newRecorder(testutil.NewFakeSink(t).Logger(), noop.NewTracerProvider().Tracer(t.Name()), false, false, func(ctx context.Context) (DRPCClient, error) {
			clientCalls++
			require.NoError(t, ctx.Err(), "client acquisition must not inherit request cancellation")
			require.Equal(t, &actor, aibcontext.ActorFromContext(ctx))
			return nil, drpcErr
		})
		ctx, cancel := context.WithCancel(aibridge.AsActor(t.Context(), actor))
		cancel()
		require.ErrorIs(t, rec.RecordInterception(context.WithoutCancel(ctx), &recorder.InterceptionRecord{ID: uuid.NewString(), InitiatorID: actor.ID.String(), StartedAt: time.Now().UTC()}), drpcErr)
		require.Equal(t, 1, clientCalls)
	})
}

type noMCPConfigsClient struct{ DRPCClient }

func (noMCPConfigsClient) GetMCPServerConfigs(context.Context, *proto.GetMCPServerConfigsRequest) (*proto.GetMCPServerConfigsResponse, error) {
	return &proto.GetMCPServerConfigsResponse{}, nil
}

func TestServerProxy_RecorderReusedAcrossReloads(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	logger := slogtest.Make(t, nil)
	client := noMCPConfigsClient{}
	server := &Server{
		lifecycleCtx: ctx,
		logger:       logger,
		tracer:       noop.NewTracerProvider().Tracer(t.Name()),
		inflight:     aibridge.NewInflightGate(logger),
		poolOptions:  DefaultPoolOptions,
	}
	t.Cleanup(server.inflight.Close)
	require.NoError(t, server.initializeBackend(ctx, client))
	rec := server.recorder
	require.NotNil(t, rec, "proxy mode initializes the recorder before providers load")

	providers := []aibridge.Provider{aibridge.NewOpenAIProvider(config.OpenAI{})}
	for range 2 {
		previous := server.backend.Load().proxyRouter
		require.NoError(t, server.ReplaceProviders(ctx, providers))
		require.NotSame(t, previous, server.backend.Load().proxyRouter)
		require.Same(t, rec, server.recorder, "provider reloads must reuse the recorder")
		require.NoError(t, server.initializeBackend(ctx, client))
		require.Same(t, rec, server.recorder, "reconnects must reuse the recorder")
	}
}

// recordingClient captures interception RPCs by ID for concurrent assertions.
// Its nil embedded client makes unexpected calls fail.
type recordingClient struct {
	DRPCClient
	requests sync.Map
}

func (c *recordingClient) RecordInterception(_ context.Context, in *proto.RecordInterceptionRequest) (*proto.RecordInterceptionResponse, error) {
	if _, loaded := c.requests.LoadOrStore(in.GetId(), in); loaded {
		return nil, xerrors.New("interception recorded more than once")
	}
	return &proto.RecordInterceptionResponse{}, nil
}

// blockingHandler returns a handler that signals started, then blocks until
// release is closed or the request context is canceled.
func blockingHandler() (h http.Handler, started <-chan struct{}, release func()) {
	startedCh := make(chan struct{}, 1)
	releaseCh := make(chan struct{})
	h = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startedCh <- struct{}{}
		select {
		case <-releaseCh:
			w.WriteHeader(http.StatusNoContent)
		case <-r.Context().Done():
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	})
	return h, startedCh, func() { close(releaseCh) }
}

// serveAsync invokes an acquired handler on a passthrough route and reports
// the response status.
func serveAsync(handler http.Handler) <-chan int {
	done := make(chan int, 1)
	go func() {
		defer close(done)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/openai/v1/models", nil))
		done <- rec.Code
	}()
	return done
}

type proxyTestConn struct {
	drpc.Conn
	closed chan struct{}
	once   sync.Once
}

func (c *proxyTestConn) Closed() <-chan struct{} { return c.closed }
func (c *proxyTestConn) Close() error {
	c.once.Do(func() { close(c.closed) })
	return nil
}

// DRPC stays available, including reconnects, until requests finish or shutdown
// is canceled. An in-progress reload must not block reconnecting or closing DRPC.
func TestServerProxy_Shutdown(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		cancel     bool
		wantStatus int
		wantErr    error
	}{
		{name: "Graceful", wantStatus: http.StatusNoContent},
		// Canceling the request aborts the upstream round trip.
		{name: "Canceled", cancel: true, wantStatus: http.StatusBadGateway, wantErr: context.Canceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			upstreamHandler, started, release := blockingHandler()
			tracker := newProxyTestServer(t, upstreamHandler)
			release = sync.OnceFunc(release)
			t.Cleanup(release)
			testCtx := testutil.Context(t, testutil.WaitShort)
			first := &Client{Conn: &proxyTestConn{closed: make(chan struct{})}}
			second := &Client{Conn: &proxyTestConn{closed: make(chan struct{})}}
			reconnected := make(chan struct{})
			dials := 0
			tracker.clientCh = make(chan DRPCClient)
			tracker.clientDialer = func(context.Context) (DRPCClient, error) {
				dials++
				switch dials {
				case 1:
					return first, nil
				case 2:
					return nil, xerrors.New("transient dial failure")
				default:
					close(reconnected)
					return second, nil
				}
			}
			tracker.wg.Add(1)
			go tracker.connect()
			client, err := tracker.Client(testCtx)
			require.NoError(t, err)
			require.Same(t, first, client)

			handler, err := tracker.GetRequestHandler(testCtx, Request{})
			require.NoError(t, err)
			inflightReqDone := serveAsync(handler)
			testutil.RequireReceive(testCtx, t, started)

			shutdownCtx, cancel := context.WithCancel(testCtx)
			defer cancel()
			shutdownDone := make(chan error, 1)
			go func() { shutdownDone <- tracker.Shutdown(shutdownCtx) }()
			require.Eventually(t, tracker.shuttingDown.Load, testutil.WaitShort, testutil.IntervalFast)

			refused, err := tracker.GetRequestHandler(testCtx, Request{})
			require.NoError(t, err)
			rec := httptest.NewRecorder()
			refused.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil))
			require.Equal(t, http.StatusServiceUnavailable, rec.Code)
			require.Equal(t, "AI Gateway is shutting down\n", rec.Body.String())
			select {
			case err := <-shutdownDone:
				t.Fatalf("shutdown returned before the request finished: %v", err)
			default:
			}

			client, err = tracker.Client(testCtx)
			require.NoError(t, err)
			require.Same(t, first, client, "DRPC must remain available during drainage")
			tracker.backendMu.Lock()
			defer tracker.backendMu.Unlock()
			require.NoError(t, first.DRPCConn().Close())
			testutil.TryReceive(testCtx, t, reconnected)
			client, err = tracker.Client(testCtx)
			require.NoError(t, err)
			require.Same(t, second, client, "DRPC must reconnect during drainage")
			require.NoError(t, tracker.lifecycleCtx.Err())

			if tc.cancel {
				cancel()
			} else {
				release()
			}
			require.Equal(t, tc.wantStatus, testutil.RequireReceive(testCtx, t, inflightReqDone))
			require.ErrorIs(t, testutil.RequireReceive(testCtx, t, shutdownDone), tc.wantErr)
			require.ErrorIs(t, context.Cause(tracker.lifecycleCtx), ErrShutdown)
			testutil.TryReceive(testCtx, t, second.DRPCConn().Closed())
			require.False(t, tracker.Ready())
		})
	}
}

func TestServerProxy_CancelsWithLifecycle(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	tracker, err := New(ctx, func(ctx context.Context) (DRPCClient, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}, slogtest.Make(t, nil), nil, codersdk.Experiments{codersdk.ExperimentAIGatewayReverseProxy}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tracker.Close() })
	handler, started, release := blockingHandler()
	tracker.backend.Store(&backend{proxyRouter: newProxyTestRouter(t, handler, tracker.inflight)})
	t.Cleanup(release)
	acquired, err := tracker.GetRequestHandler(t.Context(), Request{})
	require.NoError(t, err)
	done := serveAsync(acquired)
	testCtx := testutil.Context(t, testutil.WaitShort)
	testutil.RequireReceive(testCtx, t, started)

	cancel()
	require.Equal(t, http.StatusBadGateway, testutil.RequireReceive(testCtx, t, done))
	require.ErrorIs(t, tracker.Err(), context.Canceled)
	refused, err := tracker.GetRequestHandler(testCtx, Request{})
	require.NoError(t, err)
	for _, h := range []http.Handler{acquired, refused} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil))
		require.Equal(t, http.StatusServiceUnavailable, rec.Code)
		require.Equal(t, "AI Gateway is shutting down\n", rec.Body.String())
	}
}

// blockingWriter blocks the first response write until release is closed,
// ignoring request cancellation.
type blockingWriter struct {
	*httptest.ResponseRecorder
	started chan struct{}
	release <-chan struct{}
	once    sync.Once
}

func (w *blockingWriter) Write(p []byte) (int, error) {
	w.once.Do(func() { close(w.started) })
	<-w.release
	return w.ResponseRecorder.Write(p)
}

// TestServerProxy_DeadlineDoesNotWaitForHandler ensures shutdown returns
// even if an admitted handler cannot observe cancellation, such as a blocked
// response write.
func TestServerProxy_DeadlineDoesNotWaitForHandler(t *testing.T) {
	t.Parallel()

	upstreamCanceled := make(chan struct{})
	upstreamDone := make(chan struct{})
	release := make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	testSrv := newProxyTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("chunk"))
		w.(http.Flusher).Flush()
		select {
		case <-r.Context().Done():
			close(upstreamCanceled)
		case <-upstreamDone:
		}
	}))
	handler, err := testSrv.GetRequestHandler(t.Context(), Request{})
	require.NoError(t, err)
	writer := &blockingWriter{ResponseRecorder: httptest.NewRecorder(), started: make(chan struct{}), release: release}
	inflightReqDone := make(chan int, 1)
	go func() {
		defer close(inflightReqDone)
		handler.ServeHTTP(writer, httptest.NewRequest(http.MethodPost, "/openai/v1/models", nil))
		inflightReqDone <- writer.Code
	}()
	t.Cleanup(func() {
		unblock()
		close(upstreamDone)
		select {
		case <-inflightReqDone:
		case <-time.After(testutil.WaitShort):
			t.Error("handler did not finish after release")
		}
	})
	testCtx := testutil.Context(t, testutil.WaitShort)
	testutil.TryReceive(testCtx, t, writer.started)

	ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
	defer cancel()
	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- testSrv.Shutdown(ctx) }()
	require.ErrorIs(t, testutil.RequireReceive(testCtx, t, shutdownDone), context.DeadlineExceeded)
	// Forced shutdown cancels the request, which aborts the upstream connection.
	testutil.TryReceive(testCtx, t, upstreamCanceled)

	select {
	case <-inflightReqDone:
		t.Fatal("handler returned before release")
	default:
	}

	// Release the test handler so incorrect re-admission fails rather than hangs.
	unblock()
	require.Equal(t, http.StatusOK, testutil.RequireReceive(testCtx, t, inflightReqDone))
	afterShutdownHandler, err := testSrv.GetRequestHandler(testCtx, Request{})
	require.NoError(t, err)
	for _, h := range []http.Handler{handler, afterShutdownHandler} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil))
		require.Equal(t, http.StatusServiceUnavailable, rec.Code)
		require.Equal(t, "AI Gateway is shutting down\n", rec.Body.String())
	}
}

// TestServerProxy_RefusesAfterShutdown asserts no request is admitted once
// shutdown has begun.
func TestServerProxy_RefusesAfterShutdown(t *testing.T) {
	t.Parallel()

	testSrv := newProxyTestServer(t, http.NotFoundHandler())
	acquired, err := testSrv.GetRequestHandler(t.Context(), Request{})
	require.NoError(t, err)
	require.NoError(t, testSrv.Shutdown(t.Context()))
	afterShutdownHandler, err := testSrv.GetRequestHandler(t.Context(), Request{})
	require.NoError(t, err)

	for _, tc := range []struct {
		name    string
		handler http.Handler
	}{
		{name: "Retained", handler: acquired},
		{name: "AcquiredAfterShutdown", handler: afterShutdownHandler},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := httptest.NewRecorder()
			tc.handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil))
			require.Equal(t, http.StatusServiceUnavailable, rec.Code)
			require.Equal(t, "AI Gateway is shutting down\n", rec.Body.String())
		})
	}
}

// newProxyTestRouter creates an OpenAI proxy router admitting requests through
// gate and forwarding to an httptest upstream served by upstreamHandler.
func newProxyTestRouter(t *testing.T, upstreamHandler http.Handler, gate *aibridge.InflightGate) *proxy.Router {
	t.Helper()
	upstream := httptest.NewServer(upstreamHandler)
	t.Cleanup(upstream.Close)
	router, err := proxy.NewRouter([]aibridge.Provider{aibridge.NewOpenAIProvider(config.OpenAI{BaseURL: upstream.URL})},
		slogtest.Make(t, nil), nil, noop.NewTracerProvider().Tracer(t.Name()), gate, nil)
	require.NoError(t, err)
	return router
}

// newProxyTestServer publishes a proxy router sharing the server's inflight
// gate. It registers cleanup but leaves DRPC setup and the connection loop to
// the test.
func newProxyTestServer(t *testing.T, upstreamHandler http.Handler) *Server {
	t.Helper()
	ctx, cancel := context.WithCancelCause(t.Context())
	s := &Server{
		lifecycleCtx: ctx,
		cancelFn:     cancel,
		logger:       slogtest.Make(t, nil),
		tracer:       noop.NewTracerProvider().Tracer(t.Name()),
		inflight:     aibridge.NewInflightGate(slogtest.Make(t, nil)),
	}
	s.backend.Store(&backend{proxyRouter: newProxyTestRouter(t, upstreamHandler, s.inflight)})
	t.Cleanup(func() { _ = s.Close() })
	return s
}

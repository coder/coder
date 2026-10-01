package aibridged

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
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
	"github.com/coder/coder/v2/aibridge/recorder"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestNewRecorder(t *testing.T) {
	t.Parallel()
	for _, options := range []PoolOptions{
		{},
		{StructuredLogging: true},
		{DisableContentRecording: true},
		{StructuredLogging: true, DisableContentRecording: true},
	} {
		t.Run(fmt.Sprintf("Structured=%t/DisableContent=%t", options.StructuredLogging, options.DisableContentRecording), func(t *testing.T) {
			t.Parallel()
			apiKeyID, id := uuid.NewString(), uuid.NewString()
			now := time.Now().UTC()
			clientErr := xerrors.New("client unavailable")
			contentErr, contentCalls := clientErr, 1
			if options.DisableContentRecording {
				contentErr, contentCalls = nil, 0
			}
			type contextKey struct{}
			sink := testutil.NewFakeSink(t)
			clientCalls := 0
			rec := newRecorder(sink.Logger(), noop.NewTracerProvider().Tracer(t.Name()), apiKeyID, options, func(ctx context.Context) (DRPCClient, error) {
				clientCalls++
				require.Equal(t, "record context", ctx.Value(contextKey{}))
				return nil, clientErr
			})
			require.Zero(t, clientCalls, "creating a recorder must not acquire a client")
			ctx := context.WithValue(t.Context(), contextKey{}, "record context")
			require.ErrorIs(t, rec.RecordInterception(ctx, &recorder.InterceptionRecord{ID: id, StartedAt: now}), clientErr)
			require.ErrorIs(t, rec.RecordInterceptionEnded(ctx, &recorder.InterceptionRecordEnded{ID: id, EndedAt: now}), clientErr)
			require.ErrorIs(t, rec.RecordTokenUsage(ctx, &recorder.TokenUsageRecord{InterceptionID: id, CreatedAt: now, Input: 1}), clientErr)
			require.ErrorIs(t, rec.RecordPromptUsage(ctx, &recorder.PromptUsageRecord{InterceptionID: id, CreatedAt: now, Prompt: "prompt"}), contentErr)
			require.ErrorIs(t, rec.RecordToolUsage(ctx, &recorder.ToolUsageRecord{InterceptionID: id, CreatedAt: now, Tool: "tool"}), contentErr)
			require.ErrorIs(t, rec.RecordModelThought(ctx, &recorder.ModelThoughtRecord{InterceptionID: id, CreatedAt: now, Content: "thought"}), contentErr)
			require.Equal(t, 3+3*contentCalls, clientCalls, "each forwarded record acquires its own client")
			logs := sink.Entries(func(entry slog.SinkEntry) bool { return entry.Message == recorder.InterceptionLogMarker })
			if options.StructuredLogging {
				require.Len(t, logs, 6, "logging must precede content filtering")
			} else {
				require.Empty(t, logs)
			}
			require.ErrorIs(t, rec.RecordPromptUsage(ctx, &recorder.PromptUsageRecord{}), recorder.ErrInvalidRecord, "validation must precede content filtering")
			require.Equal(t, 3+3*contentCalls, clientCalls)
		})
	}
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

// serveAsync invokes an acquired handler and reports the response status.
func serveAsync(handler http.Handler) <-chan int {
	done := make(chan int, 1)
	go func() {
		defer close(done)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil))
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
		{name: "Canceled", cancel: true, wantStatus: http.StatusServiceUnavailable, wantErr: context.Canceled},
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
	t.Cleanup(release)
	tracker.backend.Store(&backend{proxyRouter: tracker.inflight.Middleware(handler)})
	acquired, err := tracker.GetRequestHandler(t.Context(), Request{})
	require.NoError(t, err)
	done := serveAsync(acquired)
	testCtx := testutil.Context(t, testutil.WaitShort)
	testutil.RequireReceive(testCtx, t, started)

	cancel()
	require.Equal(t, http.StatusServiceUnavailable, testutil.RequireReceive(testCtx, t, done))
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

// TestServerProxy_DeadlineDoesNotWaitForHandler ensures shutdown returns
// even if an admitted handler cannot observe cancellation, such as a blocked
// response write.
func TestServerProxy_DeadlineDoesNotWaitForHandler(t *testing.T) {
	t.Parallel()

	started := make(chan context.Context, 1)
	release := make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	tracker := newProxyTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started <- r.Context()
		<-release
		w.WriteHeader(http.StatusNoContent)
	}))
	handler, err := tracker.GetRequestHandler(t.Context(), Request{})
	require.NoError(t, err)
	inflightReqDone := serveAsync(handler)
	t.Cleanup(func() {
		unblock()
		select {
		case <-inflightReqDone:
		case <-time.After(testutil.WaitLong):
			t.Error("handler did not finish after release")
		}
	})
	testCtx := testutil.Context(t, testutil.WaitLong)
	requestCtx := testutil.RequireReceive(testCtx, t, started)

	ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
	defer cancel()
	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- tracker.Shutdown(ctx) }()
	require.ErrorIs(t, testutil.RequireReceive(testCtx, t, shutdownDone), context.DeadlineExceeded)
	testutil.TryReceive(testCtx, t, requestCtx.Done())
	require.ErrorIs(t, requestCtx.Err(), context.Canceled)

	select {
	case <-inflightReqDone:
		t.Fatal("handler returned before release")
	default:
	}

	// Release the test handler so incorrect re-admission fails rather than hangs.
	unblock()
	require.Equal(t, http.StatusNoContent, testutil.RequireReceive(testCtx, t, inflightReqDone))
	refused, err := tracker.GetRequestHandler(testCtx, Request{})
	require.NoError(t, err)
	for _, h := range []http.Handler{handler, refused} {
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

	tracker := newProxyTestServer(t, http.NotFoundHandler())
	acquired, err := tracker.GetRequestHandler(t.Context(), Request{})
	require.NoError(t, err)
	require.NoError(t, tracker.Shutdown(t.Context()))
	refused, err := tracker.GetRequestHandler(t.Context(), Request{})
	require.NoError(t, err)

	for _, tc := range []struct {
		name    string
		handler http.Handler
	}{
		{name: "Retained", handler: acquired},
		{name: "AcquiredAfterShutdown", handler: refused},
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

// newProxyTestServer publishes a test handler behind the proxy admission gate.
// It registers cleanup but leaves DRPC setup and the connection loop to the test.
func newProxyTestServer(t *testing.T, handler http.Handler) *Server {
	t.Helper()
	ctx, cancel := context.WithCancelCause(t.Context())
	s := &Server{
		lifecycleCtx: ctx,
		cancelFn:     cancel,
		logger:       slogtest.Make(t, nil),
		inflight:     aibridge.NewInflightGate(slogtest.Make(t, nil)),
	}
	s.backend.Store(&backend{proxyRouter: s.inflight.Middleware(handler)})
	t.Cleanup(func() { _ = s.Close() })
	return s
}

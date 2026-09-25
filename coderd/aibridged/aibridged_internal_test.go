package aibridged

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
	"storj.io/drpc"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

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

// serveAsync serves one request through the server and reports its status code.
func serveAsync(tracker *Server, next http.Handler) <-chan int {
	done := make(chan int, 1)
	go func() {
		rec := httptest.NewRecorder()
		handler := tracker.inflight.Middleware(next)
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

// TestServerProxy_DrainsAdmitted asserts a graceful shutdown waits for the
// requests already admitted and does not cancel them. DRPC remains available,
// including reconnecting after transient failures, while new handlers are refused.
func TestServerProxy_DrainsAdmitted(t *testing.T) {
	t.Parallel()

	tracker := newProxyTestServer(t)
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
	tracker.wg.Go(tracker.connect)
	client, err := tracker.Client(testCtx)
	require.NoError(t, err)
	require.Same(t, first, client)

	handler, started, release := blockingHandler()
	release = sync.OnceFunc(release)
	t.Cleanup(release)
	done := serveAsync(tracker, handler)
	testutil.RequireReceive(testCtx, t, started)

	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- tracker.Shutdown(testCtx) }()
	require.Eventually(t, tracker.shuttingDown.Load, testutil.WaitShort, testutil.IntervalFast)

	// The admitted request is still blocked, so Shutdown cannot have passed the
	// drain and canceled the lifecycle.
	require.NoError(t, tracker.lifecycleCtx.Err(), "DRPC lifecycle must remain live while draining")
	refused, err := tracker.GetRequestHandler(testCtx, Request{})
	require.NoError(t, err)
	rec := httptest.NewRecorder()
	refused.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil))
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.Equal(t, "AI Gateway is shutting down\n", rec.Body.String())
	select {
	case err := <-shutdownDone:
		require.Fail(t, "shutdown returned before the in-flight request finished", "err: %v", err)
	default:
	}

	client, err = tracker.Client(testCtx)
	require.NoError(t, err)
	require.Same(t, first, client, "DRPC must remain available during drainage")
	require.NoError(t, first.DRPCConn().Close())
	testutil.TryReceive(testCtx, t, reconnected)
	client, err = tracker.Client(testCtx)
	require.NoError(t, err)
	require.Same(t, second, client, "DRPC must reconnect during drainage")
	require.NoError(t, tracker.lifecycleCtx.Err())

	release()
	require.Equal(t, http.StatusNoContent, testutil.RequireReceive(testCtx, t, done))
	require.NoError(t, testutil.RequireReceive(testCtx, t, shutdownDone))
	require.ErrorIs(t, context.Cause(tracker.lifecycleCtx), ErrShutdown)
	testutil.TryReceive(testCtx, t, second.DRPCConn().Closed())
	require.False(t, tracker.Ready())
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
	handler, started, _ := blockingHandler()
	tracker.backend.Store(&backend{proxyRouter: tracker.inflight.Middleware(handler)})
	acquired, err := tracker.GetRequestHandler(t.Context(), Request{})
	require.NoError(t, err)
	done := serveAsync(tracker, handler)
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

// TestServerProxy_CancelsOnDeadline asserts an expired shutdown context
// cancels admitted requests instead of waiting on them, and is reported.
func TestServerProxy_CancelsOnDeadline(t *testing.T) {
	t.Parallel()

	tracker := newProxyTestServer(t)
	handler, started, _ := blockingHandler()
	done := serveAsync(tracker, handler)
	testCtx := testutil.Context(t, testutil.WaitShort)
	testutil.RequireReceive(testCtx, t, started)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, tracker.Shutdown(ctx), context.Canceled)
	require.Equal(t, http.StatusServiceUnavailable, testutil.RequireReceive(testCtx, t, done), "the request must observe the forced cancellation")
	require.ErrorIs(t, context.Cause(tracker.lifecycleCtx), ErrShutdown)
}

// TestServerProxy_DeadlineDoesNotWaitForHandler ensures shutdown returns
// even if an admitted handler cannot observe cancellation, such as a blocked
// response write.
func TestServerProxy_DeadlineDoesNotWaitForHandler(t *testing.T) {
	t.Parallel()

	tracker := newProxyTestServer(t)
	started := make(chan context.Context, 1)
	release := make(chan struct{})
	done := serveAsync(tracker, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started <- r.Context()
		<-release
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(func() {
		close(release)
		select {
		case status := <-done:
			require.Equal(t, http.StatusNoContent, status)
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
	select {
	case <-requestCtx.Done():
	case <-testCtx.Done():
		t.Fatal("request context was not canceled")
	}
	require.ErrorIs(t, requestCtx.Err(), context.Canceled)

	select {
	case <-done:
		t.Fatal("handler returned before release")
	default:
	}

	rec := httptest.NewRecorder()
	handler := tracker.inflight.Middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("request admitted after shutdown")
	}))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil))
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

// TestServerProxy_RefusesAfterShutdown asserts no request is admitted once
// shutdown has begun.
func TestServerProxy_RefusesAfterShutdown(t *testing.T) {
	t.Parallel()

	tracker := newProxyTestServer(t)
	require.NoError(t, tracker.Shutdown(t.Context()))

	handler, _, _ := blockingHandler()
	rec := httptest.NewRecorder()
	current := tracker.inflight.Middleware(handler)
	current.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil))
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.Contains(t, rec.Body.String(), "shutting down")
}

// newProxyTestServer isolates proxy request handling from the DRPC connection.
func newProxyTestServer(t *testing.T) *Server {
	t.Helper()
	ctx, cancel := context.WithCancelCause(t.Context())
	s := &Server{
		lifecycleCtx: ctx,
		cancelFn:     cancel,
		logger:       slogtest.Make(t, nil),
		inflight:     aibridge.NewInflightGate(slogtest.Make(t, nil)),
	}
	s.backend.Store(&backend{})
	t.Cleanup(func() { _ = s.Close() })
	return s
}

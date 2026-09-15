package aibridged

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/aibridge"
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
		handler := tracker.inflight.Middleware(nil)(next)
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil))
		done <- rec.Code
	}()
	return done
}

// TestServerProxy_DrainsAdmitted asserts a graceful shutdown waits for the
// requests already admitted and does not cancel them.
func TestServerProxy_DrainsAdmitted(t *testing.T) {
	t.Parallel()

	tracker := newProxyTestServer(t)
	handler, started, release := blockingHandler()
	done := serveAsync(tracker, handler)
	<-started

	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- tracker.Shutdown(t.Context()) }()

	select {
	case err := <-shutdownDone:
		require.Fail(t, "shutdown returned before the in-flight request finished", "err: %v", err)
	default:
	}

	release()
	require.Equal(t, http.StatusNoContent, <-done)
	require.NoError(t, <-shutdownDone)
}

// TestServerProxy_CancelsOnDeadline asserts an expired shutdown context
// cancels admitted requests instead of waiting on them, and is reported.
func TestServerProxy_CancelsOnDeadline(t *testing.T) {
	t.Parallel()

	tracker := newProxyTestServer(t)
	handler, started, _ := blockingHandler()
	done := serveAsync(tracker, handler)
	<-started

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, tracker.Shutdown(ctx), context.Canceled)
	require.Equal(t, http.StatusServiceUnavailable, <-done, "the request must observe the forced cancellation")
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
	handler := tracker.inflight.Middleware(nil)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
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
	current := tracker.inflight.Middleware(nil)(handler)
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
		inflight:     aibridge.NewInflightGate(),
	}
	s.backend.Store(&backend{})
	t.Cleanup(func() { _ = s.Close() })
	return s
}

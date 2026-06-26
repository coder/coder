package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
	"github.com/coder/websocket"
)

// websocketPair sets up an httptest server with a websocket endpoint and
// returns the server-side conn. The server handler stays alive until ctx
// is done.
func websocketPair(ctx context.Context, t *testing.T) *websocket.Conn {
	t.Helper()
	serverConnCh := make(chan *websocket.Conn, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		serverConnCh <- conn
		// Keep the handler alive so the HTTP server doesn't close
		// the connection from under us.
		<-ctx.Done()
	}))
	t.Cleanup(srv.Close)

	//nolint:bodyclose
	clientConn, _, err := websocket.Dial(ctx, srv.URL, nil)
	require.NoError(t, err)
	_ = clientConn.CloseRead(ctx) // Needed to handle pings/pongs.
	t.Cleanup(func() {
		_ = clientConn.Close(websocket.StatusNormalClosure, "test cleanup")
	})

	select {
	case sc := <-serverConnCh:
		_ = sc.CloseRead(ctx) // Needed to handle pings/pongs.
		return sc
	case <-ctx.Done():
		t.Fatal("timed out waiting for server websocket accept")
		return nil
	}
}

// probeRecorder is a simple wrapper around a channel used to record probe results.
type probeRecorder struct {
	T testing.TB
	C chan ProbeResult
}

func (r *probeRecorder) record(_ context.Context, result ProbeResult) {
	select {
	case r.C <- result:
	default:
		r.T.Errorf("probeRecorder.C is full, dropping result %s", result)
	}
}

func TestWSWatcher(t *testing.T) {
	t.Parallel()

	t.Run("ServerSideClose", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		sink := testutil.NewFakeSink(t)
		logger := sink.Logger()
		mClock := quartz.NewMock(t)
		rec := &probeRecorder{T: t, C: make(chan ProbeResult, 1)}

		trap := mClock.Trap().NewTicker("WSWatcher")
		defer trap.Close()

		serverConn := websocketPair(ctx, t)

		w := &WSWatcher{rec: rec.record, clk: mClock, interval: time.Second}
		watchCtx := w.Watch(ctx, logger, serverConn)

		// Wait for the ticker to be created, then release.
		trap.MustWait(ctx).MustRelease(ctx)

		// Close the server-side connection before the tick fires.
		// The next ping will get a close/net.ErrClosed error.
		_ = serverConn.Close(websocket.StatusGoingAway, "simulated teardown")

		// Advance clock to trigger the tick.
		mClock.Advance(time.Second).MustWait(ctx)

		// The watch context should be canceled after probe failure.
		select {
		case <-watchCtx.Done():
		case <-ctx.Done():
			t.Fatal("timed out waiting for watch context to be canceled")
		}

		gotRes := testutil.RequireReceive(ctx, t, rec.C)
		assert.Equal(t, ProbePeerClosed, gotRes, "expected ProbePeerClosed result")

		// A closed connection is a normal shutdown condition. The
		// error should be logged at Debug, not Error.
		errorEntries := sink.Entries(func(e slog.SinkEntry) bool { return e.Level == slog.LevelError })
		assert.Empty(t, errorEntries,
			"closed connection should not produce error-level logs, got: %+v", errorEntries)
		debugEntries := sink.Entries(func(e slog.SinkEntry) bool { return e.Level == slog.LevelDebug })
		assert.NotEmpty(t, debugEntries,
			"expected a debug-level log entry for the closed connection")
	})

	t.Run("ContextCanceled", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		sink := testutil.NewFakeSink(t)
		logger := sink.Logger()
		mClock := quartz.NewMock(t)
		rec := &probeRecorder{T: t, C: make(chan ProbeResult, 1)}

		trap := mClock.Trap().NewTicker("WSWatcher")
		defer trap.Close()

		serverCtx, serverCancel := context.WithCancel(ctx)
		serverConn := websocketPair(ctx, t)

		w := &WSWatcher{rec: rec.record, clk: mClock, interval: time.Second}
		watchCtx := w.Watch(serverCtx, logger, serverConn)

		trap.MustWait(ctx).MustRelease(ctx)

		// Cancel the parent context. The watcher should exit via
		// the <-ctx.Done() branch without closing the conn.
		serverCancel()

		select {
		case <-watchCtx.Done():
		case <-ctx.Done():
			t.Fatal("timed out waiting for watch context to be canceled")
		}

		errorEntries := sink.Entries(func(e slog.SinkEntry) bool { return e.Level == slog.LevelError })
		assert.Empty(t, errorEntries,
			"context cancellation should not produce error-level logs, got: %+v", errorEntries)
		assert.Empty(t, rec.C, "expected no probes when context is canceled before tick")
	})

	t.Run("PingSucceeds", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(testutil.Context(t, testutil.WaitShort))
		defer cancel()

		sink := testutil.NewFakeSink(t)
		logger := sink.Logger()
		mClock := quartz.NewMock(t)
		rec := &probeRecorder{T: t, C: make(chan ProbeResult, 3)}

		trap := mClock.Trap().NewTicker("WSWatcher")
		defer trap.Close()

		serverConn := websocketPair(ctx, t)

		w := &WSWatcher{rec: rec.record, clk: mClock, interval: time.Second}
		watchCtx := w.Watch(ctx, logger, serverConn)
		t.Cleanup(func() {
			<-watchCtx.Done()
		})

		trap.MustWait(ctx).MustRelease(ctx)

		// Fire several ticks; pings should succeed each time.
		for i := range 3 {
			mClock.Advance(time.Second).MustWait(ctx)
			gotRes := testutil.RequireReceive(ctx, t, rec.C)
			assert.Equal(t, ProbeOK, gotRes, "expected probe result to be ProbeOK at tick %d", i+1)
		}

		// No logs should be emitted during normal operation.
		errorEntries := sink.Entries(func(e slog.SinkEntry) bool { return e.Level == slog.LevelError })
		assert.Empty(t, errorEntries,
			"successful pings should not produce error-level logs, got: %+v", errorEntries)
		debugEntries := sink.Entries(func(e slog.SinkEntry) bool { return e.Level == slog.LevelDebug })
		assert.Empty(t, debugEntries,
			"successful pings should not produce debug-level logs, got: %+v", debugEntries)
	})

	t.Run("RecordsPrometheusCounter", func(t *testing.T) {
		t.Parallel()

		// Use a real prometheus registry to verify end-to-end metric recording.
		registry := prometheus.NewRegistry()
		probes := prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: "api",
			Name:      "websocket_probes_total",
			Help:      "test",
		}, []string{"path", "result"})
		registry.MustRegister(probes)

		recorder := func(ctx context.Context, r ProbeResult) {
			probes.WithLabelValues("/test/path", string(r)).Inc()
		}

		sink := testutil.NewFakeSink(t)
		logger := sink.Logger()
		mClock := quartz.NewMock(t)

		trap := mClock.Trap().NewTicker("WSWatcher")
		defer trap.Close()

		ctx, cancel := context.WithCancel(testutil.Context(t, testutil.WaitShort))
		defer cancel()
		serverConn := websocketPair(ctx, t)

		w := &WSWatcher{rec: recorder, clk: mClock, interval: time.Second}
		watchCtx := w.Watch(ctx, logger, serverConn)
		t.Cleanup(func() {
			<-watchCtx.Done()
		})

		trap.MustWait(ctx).MustRelease(ctx)
		mClock.Advance(time.Second).MustWait(ctx)

		testutil.Eventually(ctx, t, func(context.Context) bool {
			select {
			case <-watchCtx.Done():
				t.Fatal("watch context should not be canceled when pings succeed")
			default:
			}
			metrics, err := registry.Gather()
			require.NoError(t, err)
			return testutil.PromCounterHasValue(t, metrics, 1,
				"coderd_api_websocket_probes_total", "/test/path", "ok")
		}, testutil.IntervalFast, "probe counter not incremented")
	})

	t.Run("ProbeTimeout", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		mClock := quartz.NewMock(t)
		trap := mClock.Trap().NewTicker("WSWatcher")
		defer trap.Close()

		sink := testutil.NewFakeSink(t)
		logger := sink.Logger()
		rec := &probeRecorder{T: t, C: make(chan ProbeResult, 1)}

		pingCh := make(chan struct{})
		closeCodeCh := make(chan websocket.StatusCode, 1)
		fConn := &fakePingCloser{
			pingFn: func(context.Context) error {
				t.Log("ping")
				close(pingCh)
				// Determinism tradeoff: by returning DeadlineExceeded directly
				// we lose coverage of the WithTimeout path in probe().
				return context.DeadlineExceeded
			},
			closeFn: func(code websocket.StatusCode, _ string) error {
				closeCodeCh <- code
				return nil
			},
		}

		w := &WSWatcher{rec: rec.record, clk: mClock, interval: time.Second}
		watchCtx := w.Watch(ctx, logger, fConn)

		trap.MustWait(ctx).MustRelease(ctx)
		mClock.Advance(time.Second).MustWait(ctx)

		_, _ = testutil.SoftTryReceive(ctx, t, pingCh)

		select {
		case <-watchCtx.Done():
		case <-ctx.Done():
			t.Fatal("timed out waiting for watch context to be canceled")
		}

		gotRes := testutil.RequireReceive(ctx, t, rec.C)
		assert.Equal(t, ProbeTimeout, gotRes, "expected ProbeTimeout result")
		gotCode := testutil.RequireReceive(ctx, t, closeCodeCh)
		assert.Equal(t, websocket.StatusGoingAway, gotCode, "expected StatusGoingAway code")

		// Timeout is an expected condition, should be Debug not Error.
		errorEntries := sink.Entries(func(e slog.SinkEntry) bool { return e.Level == slog.LevelError })
		assert.Empty(t, errorEntries,
			"probe timeout should not produce error-level logs, got: %+v", errorEntries)
	})

	t.Run("ProbeError", func(t *testing.T) {
		t.Parallel()

		sink := testutil.NewFakeSink(t)
		logger := sink.Logger()
		mClock := quartz.NewMock(t)
		trap := mClock.Trap().NewTicker("WSWatcher")
		defer trap.Close()

		rec := &probeRecorder{T: t, C: make(chan ProbeResult, 1)}
		closeCodeCh := make(chan websocket.StatusCode, 1)

		fConn := &fakePingCloser{
			pingFn: func(context.Context) error {
				return assert.AnError
			},
			closeFn: func(code websocket.StatusCode, _ string) error {
				t.Log("close error", code)
				closeCodeCh <- code
				return nil
			},
		}

		w := &WSWatcher{rec: rec.record, clk: mClock, interval: time.Second}

		ctx, cancel := context.WithCancel(testutil.Context(t, testutil.WaitShort))
		defer cancel()
		watchCtx := w.Watch(ctx, logger, fConn)
		t.Cleanup(func() {
			<-watchCtx.Done()
		})

		trap.MustWait(ctx).MustRelease(ctx)
		mClock.Advance(time.Second).MustWait(ctx)

		gotRes := testutil.RequireReceive(ctx, t, rec.C)
		assert.Equal(t, ProbeError, gotRes, "expected ProbeError result")

		gotCode := testutil.RequireReceive(ctx, t, closeCodeCh)
		assert.Equal(t, websocket.StatusGoingAway, gotCode)

		// ProbeError should log at Error level (unlike other failures).
		errorEntries := sink.Entries(func(e slog.SinkEntry) bool {
			return e.Level == slog.LevelError
		})
		assert.NotEmpty(t, errorEntries, "ProbeError should produce error-level log")
	})
}

// fakePingCloser is a test double for the pingCloser interface.
type fakePingCloser struct {
	pingFn  func(context.Context) error
	closeFn func(websocket.StatusCode, string) error
}

func (f *fakePingCloser) Ping(ctx context.Context) error {
	return f.pingFn(ctx)
}

func (f *fakePingCloser) Close(code websocket.StatusCode, reason string) error {
	return f.closeFn(code, reason)
}

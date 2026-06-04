package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

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

// probeRecords is a thread-safe collector for ProbeResult values.
type probeRecords struct {
	mu      sync.Mutex
	results []ProbeResult
}

func (r *probeRecords) record(_ context.Context, result ProbeResult) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.results = append(r.results, result)
}

func (r *probeRecords) count(want ProbeResult) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, got := range r.results {
		if got == want {
			n++
		}
	}
	return n
}

func (r *probeRecords) len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.results)
}

func TestWSWatcher(t *testing.T) {
	t.Parallel()

	t.Run("ServerSideClose", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		sink := testutil.NewFakeSink(t)
		logger := sink.Logger()
		mClock := quartz.NewMock(t)
		rec := &probeRecords{}

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

		// A closed connection is a normal shutdown condition. The
		// error should be logged at Debug, not Error.
		errorEntries := sink.Entries(func(e slog.SinkEntry) bool { return e.Level == slog.LevelError })
		assert.Empty(t, errorEntries,
			"closed connection should not produce error-level logs, got: %+v", errorEntries)
		debugEntries := sink.Entries(func(e slog.SinkEntry) bool { return e.Level == slog.LevelDebug })
		assert.NotEmpty(t, debugEntries,
			"expected a debug-level log entry for the closed connection")
		assert.Zero(t, rec.count(ProbeOK), "expected no successful probes")
		assert.Equal(t, 1, rec.len(), "expected exactly one probe recorded")
		assert.Equal(t, 1, rec.count(ProbePeerClosed), "expected one peer_closed probe")
	})

	t.Run("ContextCanceled", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		sink := testutil.NewFakeSink(t)
		logger := sink.Logger()
		mClock := quartz.NewMock(t)
		rec := &probeRecords{}

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
		assert.Zero(t, rec.len(), "expected no probes when context is canceled before tick")
	})

	t.Run("PingSucceeds", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		sink := testutil.NewFakeSink(t)
		logger := sink.Logger()
		mClock := quartz.NewMock(t)
		rec := &probeRecords{}

		trap := mClock.Trap().NewTicker("WSWatcher")
		defer trap.Close()

		serverConn := websocketPair(ctx, t)

		w := &WSWatcher{rec: rec.record, clk: mClock, interval: time.Second}
		watchCtx := w.Watch(ctx, logger, serverConn)

		trap.MustWait(ctx).MustRelease(ctx)

		// Fire several ticks; pings should succeed each time.
		for i := range 3 {
			mClock.Advance(time.Second).MustWait(ctx)

			testutil.Eventually(ctx, t, func(context.Context) bool {
				select {
				case <-watchCtx.Done():
					t.Fatal("watch context should not be canceled when pings succeed")
				default:
				}
				return rec.count(ProbeOK) == i+1
			}, testutil.IntervalFast, "probe counter not incremented at tick %d", i+1)
		}

		// No logs should be emitted during normal operation.
		errorEntries := sink.Entries(func(e slog.SinkEntry) bool { return e.Level == slog.LevelError })
		assert.Empty(t, errorEntries,
			"successful pings should not produce error-level logs, got: %+v", errorEntries)
		debugEntries := sink.Entries(func(e slog.SinkEntry) bool { return e.Level == slog.LevelDebug })
		assert.Empty(t, debugEntries,
			"successful pings should not produce debug-level logs, got: %+v", debugEntries)
		assert.Equal(t, 3, rec.count(ProbeOK), "expected 3 successful probes")
	})

	t.Run("RecordsPrometheusCounter", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

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

		serverConn := websocketPair(ctx, t)

		w := &WSWatcher{rec: recorder, clk: mClock, interval: time.Second}
		watchCtx := w.Watch(ctx, logger, serverConn)

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

		sink := testutil.NewFakeSink(t)
		logger := sink.Logger()
		mClock := quartz.NewMock(t)
		rec := &probeRecords{}

		trap := mClock.Trap().NewTicker("WSWatcher")
		defer trap.Close()

		// Set up a websocket pair manually. Do NOT call CloseRead
		// on the client so pong frames are never sent back.
		serverConnCh := make(chan *websocket.Conn, 1)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			conn, err := websocket.Accept(w, r, nil)
			if err != nil {
				return
			}
			serverConnCh <- conn
			<-ctx.Done()
		}))
		t.Cleanup(srv.Close)

		//nolint:bodyclose
		clientConn, _, err := websocket.Dial(ctx, srv.URL, nil)
		require.NoError(t, err)
		// Intentionally NOT calling clientConn.CloseRead, so pongs won't be processed.
		t.Cleanup(func() {
			_ = clientConn.Close(websocket.StatusNormalClosure, "test cleanup")
		})

		var serverConn *websocket.Conn
		select {
		case sc := <-serverConnCh:
			_ = sc.CloseRead(ctx)
			serverConn = sc
		case <-ctx.Done():
			t.Fatal("timed out waiting for server websocket accept")
		}

		// Use a very short interval so the real context.WithTimeout
		// inside probe() expires quickly when pongs aren't coming.
		w := &WSWatcher{rec: rec.record, clk: mClock, interval: time.Millisecond}
		watchCtx := w.Watch(ctx, logger, serverConn)

		trap.MustWait(ctx).MustRelease(ctx)
		mClock.Advance(time.Millisecond).MustWait(ctx)

		// Wait for the watch context to be canceled (probe failure).
		select {
		case <-watchCtx.Done():
		case <-ctx.Done():
			t.Fatal("timed out waiting for watch context to be canceled")
		}

		assert.Equal(t, 1, rec.count(ProbeTimeout), "expected one timeout probe")
		// Timeout is an expected condition, should be Debug not Error.
		errorEntries := sink.Entries(func(e slog.SinkEntry) bool { return e.Level == slog.LevelError })
		assert.Empty(t, errorEntries,
			"probe timeout should not produce error-level logs, got: %+v", errorEntries)
	})

	t.Run("ProbeError", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		sink := testutil.NewFakeSink(t)
		logger := sink.Logger()
		mClock := quartz.NewMock(t)
		rec := &probeRecords{}

		trap := mClock.Trap().NewTicker("WSWatcher")
		defer trap.Close()

		fConn := &fakePingCloser{
			pingErr: xerrors.New("unexpected internal error"),
		}

		w := &WSWatcher{rec: rec.record, clk: mClock, interval: time.Second}
		watchCtx := w.Watch(ctx, logger, fConn)

		trap.MustWait(ctx).MustRelease(ctx)
		mClock.Advance(time.Second).MustWait(ctx)

		// Wait for the watch context to be canceled (probe failure).
		select {
		case <-watchCtx.Done():
		case <-ctx.Done():
			t.Fatal("timed out waiting for watch context to be canceled")
		}

		assert.Equal(t, 1, rec.count(ProbeError), "expected one error probe")
		// ProbeError should log at Error level (unlike other failures).
		errorEntries := sink.Entries(func(e slog.SinkEntry) bool {
			return e.Level == slog.LevelError
		})
		assert.NotEmpty(t, errorEntries, "ProbeError should produce error-level log")

		// Connection should be closed with StatusGoingAway.
		fConn.mu.Lock()
		assert.True(t, fConn.closed, "connection should be closed on probe error")
		assert.Equal(t, websocket.StatusGoingAway, fConn.code)
		fConn.mu.Unlock()
	})
}

// fakePingCloser is a test double for the pingCloser interface.
type fakePingCloser struct {
	mu      sync.Mutex
	pingErr error
	closed  bool
	code    websocket.StatusCode
	reason  string
}

func (f *fakePingCloser) Ping(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.pingErr
}

func (f *fakePingCloser) Close(code websocket.StatusCode, reason string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	f.code = code
	f.reason = reason
	return nil
}

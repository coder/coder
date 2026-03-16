package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
	"github.com/coder/websocket"
)

// logSink captures log entries so tests can assert on log levels.
type logSink struct {
	mu      sync.Mutex
	entries []slog.SinkEntry
}

func (s *logSink) LogEntry(_ context.Context, e slog.SinkEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = append(s.entries, e)
}

func (*logSink) Sync() {}

func (s *logSink) entriesAtLevel(level slog.Level) []slog.SinkEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []slog.SinkEntry
	for _, e := range s.entries {
		if e.Level == level {
			result = append(result, e)
		}
	}
	return result
}

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
	t.Cleanup(func() {
		_ = clientConn.Close(websocket.StatusNormalClosure, "test cleanup")
	})

	select {
	case sc := <-serverConnCh:
		return sc
	case <-ctx.Done():
		t.Fatal("timed out waiting for server websocket accept")
		return nil
	}
}

func TestHeartbeatClose(t *testing.T) {
	t.Parallel()

	t.Run("ServerSideClose", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		sink := &logSink{}
		logger := slog.Make(sink).Leveled(slog.LevelDebug)
		mClock := quartz.NewMock(t)

		// Trap ticker creation so we can synchronize startup.
		trap := mClock.Trap().NewTicker("HeartbeatClose")
		defer trap.Close()

		serverConn := websocketPair(ctx, t)
		exitCalled := make(chan struct{})

		go heartbeatCloseWith(ctx, logger, func() {
			close(exitCalled)
		}, serverConn, mClock, time.Second)

		// Wait for the ticker to be created, then release.
		trap.MustWait(ctx).MustRelease(ctx)

		// Close the server-side connection before the tick fires.
		// The next ping will get net.ErrClosed.
		_ = serverConn.Close(websocket.StatusGoingAway, "simulated teardown")

		// Advance clock to trigger the tick.
		mClock.Advance(time.Second).MustWait(ctx)

		// Wait for heartbeatClose to call exit.
		select {
		case <-exitCalled:
		case <-ctx.Done():
			t.Fatal("timed out waiting for heartbeatClose to call exit")
		}

		// A closed connection is a normal shutdown condition. The
		// error should be logged at Debug, not Error.
		errorEntries := sink.entriesAtLevel(slog.LevelError)
		assert.Empty(t, errorEntries,
			"closed connection should not produce error-level logs, got: %+v", errorEntries)
		debugEntries := sink.entriesAtLevel(slog.LevelDebug)
		assert.NotEmpty(t, debugEntries,
			"expected a debug-level log entry for the closed connection")
	})

	t.Run("ContextCanceled", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		sink := &logSink{}
		logger := slog.Make(sink).Leveled(slog.LevelDebug)
		mClock := quartz.NewMock(t)

		trap := mClock.Trap().NewTicker("HeartbeatClose")
		defer trap.Close()

		serverCtx, serverCancel := context.WithCancel(ctx)
		serverConn := websocketPair(ctx, t)
		done := make(chan struct{})

		go func() {
			defer close(done)
			heartbeatCloseWith(serverCtx, logger, func() {
				t.Error("exit should not be called on context cancel")
			}, serverConn, mClock, time.Second)
		}()

		trap.MustWait(ctx).MustRelease(ctx)

		// Cancel the context. HeartbeatClose should return via
		// the <-ctx.Done() branch without calling exit.
		serverCancel()

		select {
		case <-done:
		case <-ctx.Done():
			t.Fatal("timed out waiting for heartbeatClose to return")
		}

		errorEntries := sink.entriesAtLevel(slog.LevelError)
		assert.Empty(t, errorEntries,
			"context cancellation should not produce error-level logs, got: %+v", errorEntries)
	})

	t.Run("PingSucceeds", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		sink := &logSink{}
		logger := slog.Make(sink).Leveled(slog.LevelDebug)
		mClock := quartz.NewMock(t)

		trap := mClock.Trap().NewTicker("HeartbeatClose")
		defer trap.Close()

		serverConn := websocketPair(ctx, t)
		exitCalled := make(chan struct{}, 1)

		go heartbeatCloseWith(ctx, logger, func() {
			exitCalled <- struct{}{}
		}, serverConn, mClock, time.Second)

		trap.MustWait(ctx).MustRelease(ctx)

		// Fire several ticks — pings should succeed each time.
		for range 3 {
			mClock.Advance(time.Second).MustWait(ctx)

			// Give the ping round-trip time to complete.
			// If exit were called, we'd catch it.
			select {
			case <-exitCalled:
				t.Fatal("exit should not be called when pings succeed")
			default:
			}
		}

		// No logs should be emitted during normal operation.
		errorEntries := sink.entriesAtLevel(slog.LevelError)
		assert.Empty(t, errorEntries,
			"successful pings should not produce error-level logs, got: %+v", errorEntries)
		debugEntries := sink.entriesAtLevel(slog.LevelDebug)
		assert.Empty(t, debugEntries,
			"successful pings should not produce debug-level logs, got: %+v", debugEntries)
	})
}

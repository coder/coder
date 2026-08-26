package cli

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// startRefresh runs refreshHeaders under a mock clock with X-Call=0 as the
// initial headers.
func startRefresh(ctx context.Context, t *testing.T, clk *quartz.Mock, interval time.Duration, fetch func(context.Context) (http.Header, error)) func() http.Header {
	t.Helper()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	return refreshHeaders(ctx, logger, clk, interval, http.Header{"X-Call": {"0"}}, clk.Now(), fetch)
}

func TestRefreshHeaders(t *testing.T) {
	t.Parallel()

	t.Run("IntervalAndBackoff", func(t *testing.T) {
		t.Parallel()
		const interval = time.Minute
		ctx := testutil.Context(t, testutil.WaitLong)
		clk := quartz.NewMock(t)

		var (
			calls   atomic.Int64
			failing atomic.Bool
		)
		get := startRefresh(ctx, t, clk, interval, func(context.Context) (http.Header, error) {
			n := calls.Add(1)
			if failing.Load() {
				return nil, xerrors.New("boom")
			}
			return http.Header{"X-Call": {strconv.FormatInt(n, 10)}}, nil
		})
		require.Equal(t, "0", get().Get("X-Call"))

		advance := func(d time.Duration) {
			for ; d > 0; d -= headerRefreshTick {
				clk.Advance(headerRefreshTick).MustWait(ctx)
			}
		}

		// Ticks before the interval elapses do not run the command.
		advance(interval - headerRefreshTick)
		require.EqualValues(t, 0, calls.Load())
		require.Equal(t, "0", get().Get("X-Call"))

		// The tick that lands on the interval does.
		advance(headerRefreshTick)
		require.EqualValues(t, 1, calls.Load())
		require.Equal(t, "1", get().Get("X-Call"))

		// Failures keep the previous headers and are retried with backoff:
		// one tick, two ticks, four ticks, ...
		failing.Store(true)
		advance(interval)
		require.EqualValues(t, 2, calls.Load())
		require.Equal(t, "1", get().Get("X-Call"))
		advance(headerRefreshTick)
		require.EqualValues(t, 3, calls.Load())
		advance(headerRefreshTick)
		require.EqualValues(t, 3, calls.Load(), "second retry waits two ticks")
		advance(headerRefreshTick)
		require.EqualValues(t, 4, calls.Load())
		advance(3 * headerRefreshTick)
		require.EqualValues(t, 4, calls.Load(), "third retry waits four ticks")
		failing.Store(false)
		advance(headerRefreshTick)
		require.EqualValues(t, 5, calls.Load())
		require.Equal(t, "5", get().Get("X-Call"))

		// After recovering the regular interval applies again.
		advance(interval - headerRefreshTick)
		require.EqualValues(t, 5, calls.Load())
		advance(headerRefreshTick)
		require.EqualValues(t, 6, calls.Load())
	})

	t.Run("TickDividesInterval", func(t *testing.T) {
		t.Parallel()
		// 25s is not a multiple of the 10s base tick; the refresh must still
		// land at 25s, not 30s.
		const interval = 25 * time.Second
		ctx := testutil.Context(t, testutil.WaitLong)
		clk := quartz.NewMock(t)
		var calls atomic.Int64
		_ = startRefresh(ctx, t, clk, interval, func(context.Context) (http.Header, error) {
			calls.Add(1)
			return http.Header{}, nil
		})
		start := clk.Now()
		for clk.Now().Sub(start) < interval {
			d, w := clk.AdvanceNext()
			w.MustWait(ctx)
			require.LessOrEqual(t, d, headerRefreshTick)
		}
		// Ticks are rounded up to whole nanoseconds.
		require.WithinDuration(t, start.Add(interval), clk.Now(), time.Microsecond)
		require.EqualValues(t, 1, calls.Load())
	})

	t.Run("RunDeadline", func(t *testing.T) {
		t.Parallel()
		// Runs are bounded by max(interval, minHeaderRefreshTimeout); the
		// test context must outlive that for the run's deadline to be the
		// one observed.
		const interval = 3 * time.Second
		ctx := testutil.Context(t, 2*minHeaderRefreshTimeout)
		clk := quartz.NewMock(t)
		var deadline atomic.Pointer[time.Duration]
		get := startRefresh(ctx, t, clk, interval, func(ctx context.Context) (http.Header, error) {
			dl, ok := ctx.Deadline()
			if !ok {
				return nil, xerrors.New("no deadline")
			}
			d := time.Until(dl)
			deadline.Store(&d)
			return http.Header{"X-Call": {"1"}}, nil
		})
		clk.Advance(interval).MustWait(ctx)
		require.Equal(t, "1", get().Get("X-Call"))
		require.NotNil(t, deadline.Load())
		require.InDelta(t, minHeaderRefreshTimeout.Seconds(), deadline.Load().Seconds(), 1)
	})
}

// headerFileCommand returns a header command that prints the contents of a
// file the test can rewrite between runs.
func headerFileCommand(t *testing.T, content string) (command string, write func(string)) {
	t.Helper()
	file := filepath.Join(t.TempDir(), "headers")
	write = func(content string) {
		require.NoError(t, os.WriteFile(file, []byte(content), 0o600))
	}
	write(content)
	command = "cat " + file
	if runtime.GOOS == "windows" {
		command = "type " + file
	}
	return command, write
}

func TestHeaderTransportInterval(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	serverURL, err := url.Parse("https://coder.example.com")
	require.NoError(t, err)

	t.Run("Once", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		command, _ := headerFileCommand(t, "X-Token=first\n")
		transport, err := headerTransport(ctx, logger, quartz.NewMock(t), serverURL, []string{"X-Static=yes"}, command, 0)
		require.NoError(t, err)
		require.Nil(t, transport.HeaderFunc)
		require.NotNil(t, transport.Header)
		require.Equal(t, "first", transport.Headers().Get("X-Token"))
		require.Equal(t, "yes", transport.Headers().Get("X-Static"))
	})

	t.Run("BelowMinimum", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		command, _ := headerFileCommand(t, "X-Token=first\n")
		_, err := headerTransport(ctx, logger, quartz.NewMock(t), serverURL, nil, command, time.Millisecond)
		require.ErrorContains(t, err, "below the minimum")
	})

	t.Run("Interval", func(t *testing.T) {
		t.Parallel()
		const interval = 5 * time.Second
		ctx := testutil.Context(t, testutil.WaitLong)
		clk := quartz.NewMock(t)
		command, write := headerFileCommand(t, "X-Token=first\n")

		transport, err := headerTransport(ctx, logger, clk, serverURL, []string{"X-Static=yes"}, command, interval)
		require.NoError(t, err)
		require.Nil(t, transport.Header, "Header is unused when refreshing")
		require.Equal(t, "first", transport.Headers().Get("X-Token"))
		require.Equal(t, "yes", transport.Headers().Get("X-Static"))

		write("X-Token=second\n")
		clk.Advance(interval).MustWait(ctx)
		require.Equal(t, "second", transport.Headers().Get("X-Token"))
		require.Equal(t, "yes", transport.Headers().Get("X-Static"))
	})

	t.Run("FirstRunCountsTowardsInterval", func(t *testing.T) {
		t.Parallel()
		// A slow first run must not delay the first refresh: the interval is
		// measured from when that run started.
		const interval = time.Minute
		ctx := testutil.Context(t, testutil.WaitLong)
		clk := quartz.NewMock(t)
		logger := slogtest.Make(t, nil)
		var calls atomic.Int64
		started := clk.Now()
		clk.Advance(40 * time.Second) // the first run took 40s
		_ = refreshHeaders(ctx, logger, clk, interval, http.Header{}, started, func(context.Context) (http.Header, error) {
			calls.Add(1)
			return http.Header{}, nil
		})
		clk.Advance(headerRefreshTick).MustWait(ctx)
		require.EqualValues(t, 0, calls.Load())
		clk.Advance(headerRefreshTick).MustWait(ctx) // 60s after the first run started
		require.EqualValues(t, 1, calls.Load())
	})
}

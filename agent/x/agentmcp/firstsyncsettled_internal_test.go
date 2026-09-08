package agentmcp

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/testutil"
)

func TestFirstSyncSettled(t *testing.T) {
	t.Parallel()

	t.Run("InitiallyFalse", func(t *testing.T) {
		t.Parallel()

		m := NewManager(context.Background(), testLogger(t), nil, nil)
		defer m.Close()

		require.False(t, m.FirstSyncSettled())
	})

	t.Run("TrueAfterMarkSettled", func(t *testing.T) {
		t.Parallel()

		m := NewManager(context.Background(), testLogger(t), nil, nil)
		defer m.Close()

		m.markFirstSyncSettled()
		require.True(t, m.FirstSyncSettled())
	})

	t.Run("WaitFirstSync_Settles", func(t *testing.T) {
		t.Parallel()

		m := NewManager(context.Background(), testLogger(t), nil, nil)
		defer m.Close()

		// Use a channel to synchronize instead of timing sleep.
		started := make(chan struct{})
		go func() {
			<-started
			m.markFirstSyncSettled()
		}()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		close(started)
		err := m.WaitFirstSync(ctx)
		require.NoError(t, err)
		require.True(t, m.FirstSyncSettled())
	})

	t.Run("WaitFirstSync_Canceled", func(t *testing.T) {
		t.Parallel()

		m := NewManager(context.Background(), testLogger(t), nil, nil)
		defer m.Close()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.IntervalFast)
		defer cancel()

		err := m.WaitFirstSync(ctx)
		require.Error(t, err)
		require.False(t, m.FirstSyncSettled())
	})

	t.Run("WaitFirstSync_ManagerClosed", func(t *testing.T) {
		t.Parallel()

		m := NewManager(context.Background(), testLogger(t), nil, nil)
		_ = m.Close()

		err := m.WaitFirstSync(context.Background())
		require.ErrorIs(t, err, ErrManagerClosed)
	})

	t.Run("SettledTriggersOnChange", func(t *testing.T) {
		t.Parallel()

		m := NewManager(context.Background(), testLogger(t), nil, nil)
		defer m.Close()

		triggered := make(chan struct{}, 1)
		m.SetOnReload(func() {
			select {
			case triggered <- struct{}{}:
			default:
			}
		})

		m.markFirstSyncSettled()

		select {
		case <-triggered:
			// onChange was called.
		case <-time.After(testutil.WaitShort):
			t.Fatal("onChange was not called after markFirstSyncSettled")
		}
	})

	t.Run("IdempotentMarkSettled", func(t *testing.T) {
		t.Parallel()

		m := NewManager(context.Background(), testLogger(t), nil, nil)
		defer m.Close()

		calls := 0
		m.SetOnReload(func() { calls++ })

		m.markFirstSyncSettled()
		m.markFirstSyncSettled() // should not panic (close of closed channel)
		m.markFirstSyncSettled()

		require.Equal(t, 1, calls, "onChange should fire only once")
		require.True(t, m.FirstSyncSettled())
	})
}

func testLogger(t *testing.T) slog.Logger {
	t.Helper()
	return slogtest.Make(t, nil).Leveled(slog.LevelDebug)
}

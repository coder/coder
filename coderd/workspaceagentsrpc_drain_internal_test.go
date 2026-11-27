package coderd

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestAgentRPCConnsDrain(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		conns      int
		window     time.Duration
		wantSpread time.Duration
	}{
		{
			// Shutdown must stay quick when there is little to spread out.
			name:       "FewConnectionsTakeLittleTime",
			conns:      50,
			window:     10 * time.Second,
			wantSpread: 50 * agentDrainInterval,
		},
		{
			name:       "ManyConnectionsUseTheWholeWindow",
			conns:      200,
			window:     time.Second,
			wantSpread: time.Second,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)
			mClock := quartz.NewMock(t)
			timerTrap := mClock.Trap().NewTimer("agentRPCConns", "drain")

			var conns agentRPCConns
			start := mClock.Now()
			closedAt := make(chan time.Duration, tc.conns)
			for range tc.conns {
				conns.add(func() { closedAt <- mClock.Since(start) })
			}

			// Move the clock along to each disconnect the drain waits for.
			clockDone := make(chan struct{})
			go func() {
				defer close(clockDone)
				for {
					call, err := timerTrap.Wait(ctx)
					if err != nil {
						return
					}
					call.MustRelease(ctx)
					mClock.Advance(call.Duration).MustWait(ctx)
				}
			}()

			count, spread, err := conns.drain(ctx, mClock, tc.window)
			timerTrap.Close()
			testutil.TryReceive(ctx, t, clockDone)
			require.NoError(t, err)
			require.Equal(t, tc.conns, count)
			require.Equal(t, tc.wantSpread, spread)

			times := make([]time.Duration, 0, tc.conns)
			for range tc.conns {
				times = append(times, testutil.RequireReceive(ctx, t, closedAt))
			}
			require.LessOrEqual(t, slices.Max(times), spread)
			// Disconnecting everyone at one moment is what this prevents. The
			// chance that 50 uniform draws all fall within half of the range
			// is below 2^-40.
			require.Greater(t, slices.Max(times)-slices.Min(times), spread/2)
		})
	}

	t.Run("StopsWhenContextEnds", func(t *testing.T) {
		t.Parallel()
		mClock := quartz.NewMock(t)
		var conns agentRPCConns
		for range 100 {
			conns.add(func() {})
		}

		// The clock never moves, so only the context can end the drain. A
		// drain that ignored it would hang the server's shutdown.
		ctx, cancel := context.WithCancel(testutil.Context(t, testutil.WaitShort))
		cancel()
		done := make(chan error, 1)
		go func() {
			_, _, err := conns.drain(ctx, mClock, 10*time.Second)
			done <- err
		}()
		err := testutil.RequireReceive(testutil.Context(t, testutil.WaitShort), t, done)
		require.ErrorIs(t, err, context.Canceled)
	})

	t.Run("ForgetsClosedConnections", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		var conns agentRPCConns
		remove := conns.add(func() { t.Error("closed a connection that was already gone") })
		remove()

		count, _, err := conns.drain(ctx, quartz.NewMock(t), 10*time.Second)
		require.NoError(t, err)
		require.Zero(t, count)
	})
}

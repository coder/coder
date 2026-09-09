package watchdog

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/quartz"
)

var errTestTimeout = xerrors.New("watchdog test timeout")

func TestTimer_FiresWithCause(t *testing.T) {
	t.Parallel()

	clock := quartz.NewMock(t)
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)

	_ = New(clock, time.Minute, cancel, errTestTimeout, "test-watchdog")
	clock.Advance(time.Minute).MustWait(context.Background())
	require.ErrorIs(t, context.Cause(ctx), errTestTimeout)
}

func TestTimer_ResetRestartsWindow(t *testing.T) {
	t.Parallel()

	clock := quartz.NewMock(t)
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)

	timer := New(clock, time.Minute, cancel, errTestTimeout, "test-watchdog")
	clock.Advance(30 * time.Second).MustWait(context.Background())
	timer.Reset()
	clock.Advance(59 * time.Second).MustWait(context.Background())
	require.NoError(t, ctx.Err())
	clock.Advance(time.Second).MustWait(context.Background())
	require.ErrorIs(t, context.Cause(ctx), errTestTimeout)
}

func TestTimer_DisarmAndFireRace(t *testing.T) {
	t.Parallel()

	for range 128 {
		var cancels atomic.Int32
		timer := New(quartz.NewReal(), time.Hour, func(err error) {
			if errors.Is(err, errTestTimeout) {
				cancels.Add(1)
			}
		}, errTestTimeout)

		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			defer wg.Done()
			<-start
			timer.onTimeout()
		}()

		go func() {
			defer wg.Done()
			<-start
			timer.Disarm()
		}()

		close(start)
		wg.Wait()

		timer.onTimeout()
		timer.Disarm()

		require.LessOrEqual(t, cancels.Load(), int32(1))
	}
}

func TestTimer_DisarmPreservesCause(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)

	timer := New(quartz.NewReal(), time.Hour, cancel, errTestTimeout)
	timer.Disarm()
	// A late fire after Disarm must not cancel the context.
	timer.onTimeout()
	require.NoError(t, ctx.Err())
	require.Nil(t, context.Cause(ctx))
}

func TestTimer_StaleFireAfterResetIsIgnored(t *testing.T) {
	t.Parallel()

	clock := quartz.NewMock(t)
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)

	wd := New(clock, time.Minute, cancel, errTestTimeout, "test-watchdog")

	// Simulate the boundary race: the timeout callback was
	// dispatched but a Reset re-armed the timer before the
	// callback acquired the lock. The stale fire must not settle
	// or cancel; the re-armed window owns the outcome.
	wd.Reset()
	wd.onTimeout()
	require.NoError(t, context.Cause(ctx))

	// The re-armed window still fires when it truly elapses.
	clock.Advance(time.Minute).MustWait(context.Background())
	require.ErrorIs(t, context.Cause(ctx), errTestTimeout)
}

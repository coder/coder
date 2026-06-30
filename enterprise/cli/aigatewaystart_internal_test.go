//go:build !slim

package cli

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/testutil"
)

// blockingReloader blocks in Reload until the context is canceled, then
// returns its error. It models the standalone gateway's initial reload
// waiting on a daemon connection to an unreachable coderd.
type blockingReloader struct {
	started chan struct{}
}

func (r *blockingReloader) Reload(ctx context.Context) error {
	select {
	case r.started <- struct{}{}:
	default:
	}
	<-ctx.Done()
	return ctx.Err()
}

// TestLoadProviders_Interruptible verifies that a stop signal,
// modeled by canceling the context, unblocks the initial provider load even
// when the reloader is stuck waiting for coderd. This guards the standalone
// "ai-gateway start" command against the regression where startup could not
// be interrupted.
func TestLoadProviders_Interruptible(t *testing.T) {
	t.Parallel()

	// testCtx bounds the test and drives the channel receives; runCtx is the
	// context handed to loadProviders and is canceled to model a
	// stop signal. They are distinct so the receives still work after the
	// signal context is canceled.
	testCtx := testutil.Context(t, testutil.WaitShort)
	runCtx, cancel := context.WithCancel(testCtx)
	defer cancel()

	reloader := &blockingReloader{started: make(chan struct{}, 1)}
	logger := slog.Make()

	done := make(chan error, 1)
	go func() {
		done <- loadProviders(runCtx, reloader, logger)
	}()

	// Wait for the reload to be in-flight, then cancel as a signal would.
	testutil.RequireReceive(testCtx, t, reloader.started)
	cancel()

	err := testutil.RequireReceive(testCtx, t, done)
	require.ErrorIs(t, err, context.Canceled)
}

// failThenSucceedReloader fails the first failUntil reloads, then succeeds,
// modeling a coderd connection or provider fetch that recovers after a few
// transient failures.
type failThenSucceedReloader struct {
	calls     atomic.Int32
	failUntil int32
}

func (r *failThenSucceedReloader) Reload(_ context.Context) error {
	if r.calls.Add(1) <= r.failUntil {
		return xerrors.New("transient failure")
	}
	return nil
}

// TestLoadProviders_RetrySucceeds verifies loadProviders keeps retrying past
// transient failures and returns nil once a reload succeeds. This guards the
// retry contract: replacing the loop's continue with a return would fail here.
func TestLoadProviders_RetrySucceeds(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	reloader := &failThenSucceedReloader{failUntil: 2}

	require.NoError(t, loadProviders(ctx, reloader, slog.Make()))
	require.GreaterOrEqual(t, reloader.calls.Load(), int32(3))
}

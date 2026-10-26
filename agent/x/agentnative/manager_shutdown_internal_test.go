//go:build linux

package agentnative

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/quartz"
)

func TestManagerShutdownCallerOnlyOwnsWait(t *testing.T) {
	t.Parallel()
	entered := make(chan context.Context, 1)
	release := make(chan struct{})
	releaseShutdown := sync.OnceFunc(func() { close(release) })
	defer releaseShutdown()
	b := &managerTestBackend{shutdown: func(ctx context.Context) error {
		entered <- ctx
		<-release
		return ctx.Err()
	}}
	m := newTestManager(t, filepath.Join(t.TempDir(), "state"), b)
	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() { result <- m.Shutdown(ctx) }()
	nativeCtx := <-entered
	cancel()
	require.ErrorIs(t, <-result, context.Canceled)
	require.ErrorIs(t, m.ctx.Err(), context.Canceled)
	require.NoError(t, nativeCtx.Err(), "neither caller nor observer cancellation owns native shutdown")
	releaseShutdown()
	require.NoError(t, m.Shutdown(t.Context()))
	require.EqualValues(t, 1, b.shutdowns.Load())
}

func TestManagerShutdownRejectsPreCancelledCall(t *testing.T) {
	t.Parallel()
	b := &managerTestBackend{}
	m := newTestManager(t, filepath.Join(t.TempDir(), "state"), b)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, m.Shutdown(ctx), context.Canceled)
	require.Zero(t, b.shutdowns.Load())
	testManagerSession(t, m)
	require.NoError(t, m.Shutdown(t.Context()))
	require.EqualValues(t, 1, b.shutdowns.Load())
}

func TestManagerShutdownHasIndependentDeadline(t *testing.T) {
	t.Parallel()
	entered := make(chan struct{})
	b := &managerTestBackend{shutdown: func(ctx context.Context) error {
		close(entered)
		<-ctx.Done()
		return context.Cause(ctx)
	}}
	m := newTestManager(t, filepath.Join(t.TempDir(), "state"), b)
	clock := m.clock.(*quartz.Mock)
	result := make(chan error, 1)
	go func() { result <- m.Shutdown(t.Context()) }()
	<-entered
	clock.Advance(30 * time.Second).Wait(t.Context())
	require.ErrorIs(t, <-result, context.DeadlineExceeded)
	require.EqualValues(t, 1, b.shutdowns.Load())
}

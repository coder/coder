package chatd

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/testutil"
)

func TestRunner_BeginRunTreatsMultipleCallsAsError(t *testing.T) {
	t.Parallel()

	sink := testutil.NewFakeSink(t)
	managerCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr := &runnerManager{
		ctx:          managerCtx,
		cleanupReqCh: make(chan runnerKey, 1),
	}
	key := runnerKey{ChatID: uuid.New(), RunnerID: uuid.New()}
	r := &runner{
		ctx: context.Background(),
		mgr: mgr,
		rec: &runnerRecord{key: key},
		opts: chatWorkerOptions{
			Logger: sink.Logger(),
		},
	}

	require.True(t, r.beginRun())
	require.False(t, r.beginRun())

	select {
	case got := <-mgr.cleanupReqCh:
		require.Equal(t, key, got)
	case <-time.After(testutil.WaitLong):
		t.Fatal("timed out waiting for cleanup request")
	}
	require.Len(t, entriesWithMessage(sink, "chatworker runner run called more than once"), 1)
}

func TestRunnerManager_RouteStateHintPreservesBufferedHintsWhenFull(t *testing.T) {
	t.Parallel()

	managerCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	chatID := uuid.New()
	runnerID := uuid.New()
	first := runnerStateUpdate{
		ChatID:          chatID,
		SnapshotVersion: 1,
		HistoryVersion:  1,
		Status:          database.ChatStatusRunning,
	}
	latest := runnerStateUpdate{
		ChatID:          chatID,
		SnapshotVersion: 2,
		HistoryVersion:  2,
		Status:          database.ChatStatusRunning,
	}
	done := make(chan struct{})
	stateCh := make(chan runnerStateUpdate, 1)
	stateCh <- first
	mgr := &runnerManager{
		ctx: managerCtx,
		runnersByChat: map[uuid.UUID]map[uuid.UUID]*runnerRecord{
			chatID: {
				runnerID: {done: done, stateCh: stateCh},
			},
		},
	}

	mgr.RouteStateHint(context.Background(), latest)

	select {
	case got := <-stateCh:
		require.Equal(t, first, got)
	default:
		t.Fatal("expected buffered state hint to remain queued")
	}
	select {
	case got := <-stateCh:
		t.Fatalf("unexpected state hint queued after full channel: %#v", got)
	default:
	}
}

func TestRunnerManager_RegisterCleanupWaiterDoesNotLockManagerOnShutdown(t *testing.T) {
	t.Parallel()

	managerCtx, cancel := context.WithCancel(context.Background())
	mgr := &runnerManager{ctx: managerCtx, cleanupDoneCh: make(chan runnerKey, 1)}
	done := make(chan struct{})
	rec := &runnerRecord{done: done}

	mgr.mu.Lock()
	defer mgr.mu.Unlock()
	cancel()
	mgr.registerCleanupWaiter(runnerKey{ChatID: uuid.New(), RunnerID: uuid.New()}, rec)
	close(done)

	waited := make(chan struct{})
	go func() {
		mgr.wait()
		close(waited)
	}()
	select {
	case <-waited:
	case <-time.After(testutil.WaitLong):
		t.Fatal("cleanup waiter blocked on manager lock during shutdown")
	}
}

func TestRunnerManager_ShouldLogLoopError(t *testing.T) {
	t.Parallel()

	errBoom := xerrors.New("boom")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	require.False(t, shouldLogRunnerManagerLoopError(context.Background(), nil))
	require.False(t, shouldLogRunnerManagerLoopError(ctx, errBoom))
	require.False(t, shouldLogRunnerManagerLoopError(context.Background(), context.Canceled))
	require.False(t, shouldLogRunnerManagerLoopError(context.Background(), xerrors.Errorf("wrapped: %w", context.Canceled)))
	require.True(t, shouldLogRunnerManagerLoopError(context.Background(), errBoom))
	require.True(t, shouldLogRunnerManagerLoopError(context.Background(), errors.Join(errBoom, context.DeadlineExceeded)))
}

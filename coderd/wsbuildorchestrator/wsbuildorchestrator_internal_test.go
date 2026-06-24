package wsbuildorchestrator

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/wspubsub"
	"github.com/coder/coder/v2/testutil"
)

func newTestOrchestrator(t *testing.T, db database.Store, ps pubsub.Pubsub) *Orchestrator {
	t.Helper()

	return New(Options{
		Logger:   testutil.Logger(t),
		Database: db,
		Pubsub:   ps,
	})
}

func TestWorkspaceBuildOrchestratorSubscribeQueuesWakeOnPubsub(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	ps := pubsub.NewInMemory()
	o := newTestOrchestrator(t, nil, ps)

	go o.subscribe(ctx)

	// subscribe sends an initial wake after registration. Drain it so
	// the publish below tests pubsub delivery without racing setup.
	testutil.RequireReceive(ctx, t, o.wakeCh)

	err := wspubsub.PublishWorkspaceBuildOrchestrationWake(ctx, ps)
	require.NoError(t, err)

	testutil.RequireReceive(ctx, t, o.wakeCh)
}

func TestWorkspaceBuildOrchestratorProcessesPromptlyAfterWake(t *testing.T) {
	t.Parallel()

	const waitBeforeBackupPoll = time.Second
	require.Greater(t, backupPollInterval, waitBeforeBackupPoll)

	ctx := testutil.Context(t, waitBeforeBackupPoll)
	store := &runStore{
		calls: make(chan struct{}),
	}
	o := newTestOrchestrator(t, store, nil)

	go o.run(ctx)

	// run() processes once before waiting for wakes. Drain that
	// initial pass.
	testutil.RequireReceive(ctx, t, store.calls)

	o.wake()

	// This pass must come from the wake because the backup poll
	// cannot fire before the context expires.
	testutil.RequireReceive(ctx, t, store.calls)
}

type runStore struct {
	database.Store
	calls chan struct{}
}

func (s *runStore) InTx(fn func(database.Store) error, _ *database.TxOptions) error {
	return fn(s)
}

func (s *runStore) GetNextPendingWorkspaceBuildOrchestrationForUpdate(
	ctx context.Context,
) (database.WorkspaceBuildOrchestration, error) {
	select {
	case s.calls <- struct{}{}:
	case <-ctx.Done():
		return database.WorkspaceBuildOrchestration{}, ctx.Err()
	}
	return database.WorkspaceBuildOrchestration{}, sql.ErrNoRows
}

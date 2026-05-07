package coderd

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

func TestWorkspaceBuildOrchestratorSubscribeQueuesWakeOnPubsub(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	ps := pubsub.NewInMemory()
	api := &API{
		Options: &Options{
			Pubsub: ps,
			Logger: testutil.Logger(t),
		},
	}
	o := newWorkspaceBuildOrchestrator(api)

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
	require.Greater(t, workspaceBuildOrchestratorBackupPollInterval, waitBeforeBackupPoll)

	ctx := testutil.Context(t, waitBeforeBackupPoll)
	store := &workspaceBuildOrchestratorRunStore{
		calls: make(chan struct{}),
	}
	api := &API{
		Options: &Options{
			Database: store,
			Logger:   testutil.Logger(t),
		},
	}
	o := newWorkspaceBuildOrchestrator(api)

	go o.run(ctx)

	// run() processes once before waiting for wakes. Drain that
	// initial pass.
	testutil.RequireReceive(ctx, t, store.calls)

	o.wake()

	// This pass must come from the wake because the backup poll
	// cannot fire before the context expires.
	testutil.RequireReceive(ctx, t, store.calls)
}

type workspaceBuildOrchestratorRunStore struct {
	database.Store
	calls chan struct{}
}

func (s *workspaceBuildOrchestratorRunStore) InTx(fn func(database.Store) error, _ *database.TxOptions) error {
	return fn(s)
}

func (s *workspaceBuildOrchestratorRunStore) GetNextPendingWorkspaceBuildOrchestrationForUpdate(
	ctx context.Context,
) (database.WorkspaceBuildOrchestration, error) {
	select {
	case s.calls <- struct{}{}:
	case <-ctx.Done():
		return database.WorkspaceBuildOrchestration{}, ctx.Err()
	}
	return database.WorkspaceBuildOrchestration{}, sql.ErrNoRows
}

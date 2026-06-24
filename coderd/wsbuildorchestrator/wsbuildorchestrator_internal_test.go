package wsbuildorchestrator

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
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

// GetWorkspaceByID returns soft-deleted rows, so the orchestrator
// must guard against them explicitly to avoid starting a build for
// them.
func TestWorkspaceBuildOrchestratorFailsForDeletedWorkspace(t *testing.T) {
	t.Parallel()

	// GIVEN: a soft-deleted workspace whose parent stop build
	// succeeded and a pending orchestration to start it.
	ctx := testutil.Context(t, testutil.WaitShort)
	db, _, rawDB := dbtestutil.NewDBWithSQLDB(t)

	org := dbgen.Organization(t, db, database.Organization{})
	user := dbgen.User(t, db, database.User{})
	versionJob := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
		OrganizationID: org.ID,
		Type:           database.ProvisionerJobTypeTemplateVersionImport,
	})
	version := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		OrganizationID: org.ID,
		JobID:          versionJob.ID,
		CreatedBy:      user.ID,
	})
	template := dbgen.Template(t, db, database.Template{
		OrganizationID:  org.ID,
		ActiveVersionID: version.ID,
		CreatedBy:       user.ID,
	})
	workspace := dbgen.Workspace(t, db, database.WorkspaceTable{
		OwnerID:        user.ID,
		OrganizationID: org.ID,
		TemplateID:     template.ID,
		Deleted:        true,
	})

	// The succeeded parent stop build makes the orchestration ready
	// to process.
	now := dbtime.Now()
	parentJob := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
		OrganizationID: org.ID,
		Type:           database.ProvisionerJobTypeWorkspaceBuild,
		StartedAt:      sql.NullTime{Time: now, Valid: true},
		CompletedAt:    sql.NullTime{Time: now, Valid: true},
	})
	require.Equal(t, database.ProvisionerJobStatusSucceeded, parentJob.JobStatus)
	parentBuild := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		WorkspaceID:       workspace.ID,
		TemplateVersionID: version.ID,
		JobID:             parentJob.ID,
		Transition:        database.WorkspaceTransitionStop,
		Reason:            database.BuildReasonInitiator,
	})

	_, err := db.InsertWorkspaceBuildOrchestration(ctx, database.InsertWorkspaceBuildOrchestrationParams{
		ID:                       uuid.New(),
		CreatedAt:                now,
		UpdatedAt:                now,
		ParentBuildID:            parentBuild.ID,
		ChildTransition:          database.WorkspaceTransitionStart,
		ChildRichParameterValues: json.RawMessage("[]"),
	})
	require.NoError(t, err)

	o := newTestOrchestrator(t, db, nil)

	// WHEN: the orchestrator processes the row.
	found, err := o.processNext(ctx)
	require.NoError(t, err)
	require.True(t, found)

	// THEN: the orchestration resolves as failed without creating a child
	// build.
	orchestration, err := dbtestutil.GetWorkspaceBuildOrchestrationByParentBuildID(ctx, rawDB, parentBuild.ID)
	require.NoError(t, err)
	require.Equal(t, "failed", orchestration.Status)
	require.False(t, orchestration.ChildBuildID.Valid)
	require.True(t, orchestration.Error.Valid)
	require.Equal(t, "workspace was deleted", orchestration.Error.String)
}

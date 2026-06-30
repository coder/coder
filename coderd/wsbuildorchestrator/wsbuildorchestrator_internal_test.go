package wsbuildorchestrator

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/wspubsub"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m, testutil.GoleakOptions...)
}

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

func TestWorkspaceBuildOrchestratorRunProcessesOnWakeAndPoll(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		// trigger causes the run loop to process another pass, either
		// via a wake signal or by advancing past the backup poll.
		trigger func(ctx context.Context, o *Orchestrator, mClock *quartz.Mock)
	}{
		{
			name: "Wake",
			trigger: func(_ context.Context, o *Orchestrator, _ *quartz.Mock) {
				o.wake()
			},
		},
		{
			name: "BackupPoll",
			trigger: func(ctx context.Context, _ *Orchestrator, mClock *quartz.Mock) {
				mClock.Advance(backupPollInterval).MustWait(ctx)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitShort)
			mClock := quartz.NewMock(t)
			store := &runStore{
				calls: make(chan struct{}),
			}
			o := New(Options{
				Logger:   testutil.Logger(t),
				Database: store,
				Clock:    mClock,
			})

			go o.run(ctx)

			// Drain the initial pass. run() creates the backup poll
			// ticker and then processes (sends) once before
			// waiting. So, receiving here also ensures the ticker
			// exists before we advance the clock.
			testutil.RequireReceive(ctx, t, store.calls)

			tc.trigger(ctx, o, mClock)
			// Now this pass can only come from the trigger.
			testutil.RequireReceive(ctx, t, store.calls)
		})
	}
}

type runStore struct {
	database.Store
	calls chan struct{}
}

func (s *runStore) InTx(fn func(database.Store) error, _ *database.TxOptions) error {
	return fn(s)
}

func (s *runStore) GetNextPendingWorkspaceBuildOrchestrationForUpdate(ctx context.Context) (
	database.WorkspaceBuildOrchestration, error,
) {
	select {
	case s.calls <- struct{}{}:
	case <-ctx.Done():
		return database.WorkspaceBuildOrchestration{}, ctx.Err()
	}
	return database.WorkspaceBuildOrchestration{}, sql.ErrNoRows
}

// Note: it overwrites parentJob's OrganizationID and Type.
func seedPendingOrchestration(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	workspaceDeleted bool,
	parentJob database.ProvisionerJob,
) (database.ProvisionerJob, database.WorkspaceBuild) {
	t.Helper()

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
		Deleted:        workspaceDeleted,
	})

	parentJob.OrganizationID = org.ID
	parentJob.Type = database.ProvisionerJobTypeWorkspaceBuild
	job := dbgen.ProvisionerJob(t, db, nil, parentJob)
	parentBuild := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		WorkspaceID:       workspace.ID,
		TemplateVersionID: version.ID,
		JobID:             job.ID,
		Transition:        database.WorkspaceTransitionStop,
		Reason:            database.BuildReasonInitiator,
	})

	now := dbtime.Now()
	_, err := db.InsertWorkspaceBuildOrchestration(ctx, database.InsertWorkspaceBuildOrchestrationParams{
		ID:                       uuid.New(),
		CreatedAt:                now,
		UpdatedAt:                now,
		ParentBuildID:            parentBuild.ID,
		ChildTransition:          database.WorkspaceTransitionStart,
		ChildRichParameterValues: json.RawMessage("[]"),
	})
	require.NoError(t, err)

	return job, parentBuild
}

// succeededJob returns a provisioner job in the succeeded state.
func succeededJob() database.ProvisionerJob {
	now := dbtime.Now()
	return database.ProvisionerJob{
		StartedAt:   sql.NullTime{Time: now, Valid: true},
		CompletedAt: sql.NullTime{Time: now, Valid: true},
	}
}

// A succeeded parent whose workspace can no longer be started
// (deleted or dormant) must fail the orchestration without creating a
// child build.
func TestWorkspaceBuildOrchestratorFailsForUnstartableWorkspace(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		// seed builds a pending orchestration whose workspace cannot
		// start, and returns its parent build.
		seed      func(ctx context.Context, t *testing.T, db database.Store) database.WorkspaceBuild
		wantError string
	}{
		{
			name: "Deleted",
			seed: func(ctx context.Context, t *testing.T, db database.Store) database.WorkspaceBuild {
				job, build := seedPendingOrchestration(ctx, t, db, true, succeededJob())
				require.Equal(t, database.ProvisionerJobStatusSucceeded, job.JobStatus)
				return build
			},
			wantError: "workspace was deleted",
		},
		{
			name: "Dormant",
			seed: func(ctx context.Context, t *testing.T, db database.Store) database.WorkspaceBuild {
				job, build := seedPendingOrchestration(ctx, t, db, false, succeededJob())
				require.Equal(t, database.ProvisionerJobStatusSucceeded, job.JobStatus)
				_, err := db.UpdateWorkspaceDormantDeletingAt(ctx, database.UpdateWorkspaceDormantDeletingAtParams{
					ID:        build.WorkspaceID,
					DormantAt: sql.NullTime{Time: dbtime.Now(), Valid: true},
				})
				require.NoError(t, err)
				return build
			},
			wantError: "workspace is dormant",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// GIVEN: a pending orchestration whose workspace cannot start.
			ctx := testutil.Context(t, testutil.WaitShort)
			db, _, rawDB := dbtestutil.NewDBWithSQLDB(t)
			parentBuild := tc.seed(ctx, t, db)

			o := newTestOrchestrator(t, db, nil)

			// WHEN: the orchestrator processes the row.
			found, err := o.processNext(ctx)
			require.NoError(t, err)
			require.True(t, found)

			// THEN: the orchestration resolves as failed without creating
			// a child build.
			orchestration, err := dbtestutil.GetWorkspaceBuildOrchestrationByParentBuildID(ctx, rawDB, parentBuild.ID)
			require.NoError(t, err)
			require.Equal(t, "failed", orchestration.Status)
			require.False(t, orchestration.ChildBuildID.Valid)
			require.True(t, orchestration.Error.Valid)
			require.Equal(t, tc.wantError, orchestration.Error.String)
		})
	}
}

// If a pending parent build is canceled before any provisioner
// acquires it, the orchestration resolves as canceled, with no error
// and without creating a child build.
func TestWorkspaceBuildOrchestratorCancelsForCanceledParent(t *testing.T) {
	t.Parallel()

	// GIVEN: a workspace whose parent stop build was canceled
	// (without a provisioner acquiring it) and a pending
	// orchestration to start it.
	ctx := testutil.Context(t, testutil.WaitShort)
	db, _, rawDB := dbtestutil.NewDBWithSQLDB(t)

	now := dbtime.Now()
	parentJob, parentBuild := seedPendingOrchestration(ctx, t, db, false, database.ProvisionerJob{
		CanceledAt:  sql.NullTime{Time: now, Valid: true},
		CompletedAt: sql.NullTime{Time: now, Valid: true},
	})
	require.Equal(t, database.ProvisionerJobStatusCanceled, parentJob.JobStatus)

	o := newTestOrchestrator(t, db, nil)

	// WHEN: the orchestrator processes the row.
	found, err := o.processNext(ctx)
	require.NoError(t, err)
	require.True(t, found)

	// THEN: the orchestration resolves as canceled, with no error and
	// without creating a child build.
	orchestration, err := dbtestutil.GetWorkspaceBuildOrchestrationByParentBuildID(ctx, rawDB, parentBuild.ID)
	require.NoError(t, err)
	require.Equal(t, "canceled", orchestration.Status)
	require.False(t, orchestration.ChildBuildID.Valid)
	require.False(t, orchestration.Error.Valid)
}

// emptyStore reports no pending orchestrations so the run loop stays
// idle.
type emptyStore struct {
	database.Store
}

func (s emptyStore) InTx(fn func(database.Store) error, _ *database.TxOptions) error {
	return fn(s)
}

func (emptyStore) GetNextPendingWorkspaceBuildOrchestrationForUpdate(context.Context) (
	database.WorkspaceBuildOrchestration, error,
) {
	return database.WorkspaceBuildOrchestration{}, sql.ErrNoRows
}

func TestWorkspaceBuildOrchestratorCloseStopsGoroutines(t *testing.T) {
	t.Parallel()

	o := New(Options{
		Logger:   testutil.Logger(t),
		Database: emptyStore{},
		Pubsub:   pubsub.NewInMemory(),
	})
	o.Start(context.Background())

	// Close blocks on wg.Wait, so it returns only once both
	// background goroutines have exited. A timeout here means a
	// goroutine never exited after cancellation, leaving Close
	// blocked.
	closed := make(chan struct{})
	go func() {
		o.Close()
		close(closed)
	}()

	ctx := testutil.Context(t, testutil.WaitShort)
	select {
	case <-ctx.Done():
		t.Fatal("Close did not stop background goroutines")
	case <-closed:
	}
}

// lookupErrorStore returns a pending orchestration, then fails the
// parent provisioner job lookup. This exercises the non-child-build
// error path in processNext and captures the resulting retry update.
type lookupErrorStore struct {
	database.Store
	orchestrationID uuid.UUID
	jobErr          error

	retryCalled bool
	retryParams database.UpdateWorkspaceBuildOrchestrationRetryByIDParams
}

func (s *lookupErrorStore) InTx(fn func(database.Store) error, _ *database.TxOptions) error {
	return fn(s)
}

func (s *lookupErrorStore) GetNextPendingWorkspaceBuildOrchestrationForUpdate(context.Context) (
	database.WorkspaceBuildOrchestration, error,
) {
	return database.WorkspaceBuildOrchestration{
		ID:            s.orchestrationID,
		ParentBuildID: uuid.New(),
	}, nil
}

func (*lookupErrorStore) GetWorkspaceBuildByID(_ context.Context, id uuid.UUID) (
	database.WorkspaceBuild, error,
) {
	return database.WorkspaceBuild{ID: id, JobID: uuid.New()}, nil
}

func (s *lookupErrorStore) GetProvisionerJobByID(context.Context, uuid.UUID) (
	database.ProvisionerJob, error,
) {
	return database.ProvisionerJob{}, s.jobErr
}

func (s *lookupErrorStore) UpdateWorkspaceBuildOrchestrationRetryByID(
	_ context.Context,
	arg database.UpdateWorkspaceBuildOrchestrationRetryByIDParams,
) (database.WorkspaceBuildOrchestration, error) {
	s.retryCalled = true
	s.retryParams = arg
	return database.WorkspaceBuildOrchestration{}, nil
}

// TestWorkspaceBuildOrchestratorRetriesUnexpectedError verifies that
// an unexpected error while processing a row makes processNext
// request a bounded retry rather than surfacing the error, which
// would leave the row pending and block newer orchestrations.
func TestWorkspaceBuildOrchestratorRetriesUnexpectedError(t *testing.T) {
	t.Parallel()

	// GIVEN: a store that returns a pending orchestration, then fails
	// the parent provisioner job lookup with an unexpected
	// (non-child-build) error.
	store := &lookupErrorStore{
		orchestrationID: uuid.New(),
		jobErr:          xerrors.New("boom"),
	}
	o := New(Options{
		// The unexpected-error path logs at error level by design, so
		// tolerate it here instead of failing via slogtest.
		Logger:   slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
		Database: store,
		Pubsub:   pubsub.NewInMemory(),
	})

	ctx := testutil.Context(t, testutil.WaitShort)

	// WHEN: the orchestrator processes the row.
	found, err := o.processNext(ctx)

	// THEN: processNext requests a bounded retry (next_retry_after
	// and maxAttempts passed) and returns without error, instead of
	// surfacing the error, which would leave the row pending.
	require.NoError(t, err)
	require.True(t, found)
	require.True(t, store.retryCalled)
	require.Equal(t, store.orchestrationID, store.retryParams.ID)
	require.Equal(t, int32(maxAttempts), store.retryParams.MaxAttemptCount)
	require.False(t, store.retryParams.NextRetryAfter.IsZero())
	require.True(t, store.retryParams.Error.Valid)
	require.Contains(t, store.retryParams.Error.String, "boom")
}

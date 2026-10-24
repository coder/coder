package githubrunners

import (
	"context"
	"testing"
	"time"

	"github.com/actions/scaleset"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

type fakeClient struct {
	Client
	removed   []int64
	removeErr error
	existing  *scaleset.RunnerReference
	generated int
}

func (c *fakeClient) GetRunnerByName(context.Context, string) (*scaleset.RunnerReference, error) {
	return c.existing, nil
}
func (c *fakeClient) RemoveRunner(_ context.Context, id int64) error {
	c.removed = append(c.removed, id)
	return c.removeErr
}
func (c *fakeClient) GenerateJitRunnerConfig(context.Context, *scaleset.RunnerScaleSetJitRunnerSetting, int) (*scaleset.RunnerScaleSetJitRunnerConfig, error) {
	c.generated++
	return &scaleset.RunnerScaleSetJitRunnerConfig{Runner: &scaleset.RunnerReference{ID: 42}, EncodedJITConfig: "runner-specific-secret"}, nil
}

type fakeSession struct {
	message  *scaleset.RunnerScaleSetMessage
	ack      func()
	acquired []int64
}

func (s *fakeSession) AcquireJobs(_ context.Context, ids []int64) ([]int64, error) {
	s.acquired = append(s.acquired, ids...)
	return ids, nil
}
func (s *fakeSession) GetMessage(context.Context, int, int) (*scaleset.RunnerScaleSetMessage, error) {
	return s.message, nil
}
func (s *fakeSession) DeleteMessage(context.Context, int) error { s.ack(); return nil }

func newWorker(t *testing.T) (*worker, context.Context, *fakeClient) {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitLong)
	db, _ := dbtestutil.NewDB(t)
	org := dbgen.Organization(t, db, database.Organization{})
	user := dbgen.User(t, db, database.User{})
	template := dbgen.Template(t, db, database.Template{OrganizationID: org.ID, CreatedBy: user.ID})
	pool, err := db.InsertGitHubRunnerPool(ctx, database.InsertGitHubRunnerPoolParams{ID: uuid.New(), OrganizationID: org.ID, OwnerID: user.ID, TemplateID: template.ID, Name: "test-pool", GithubConfigUrl: "https://github.com/example/repo", GithubToken: "secret-pat", RunnerGroupID: 1, MaxRunners: 2, StartupTimeoutSeconds: 600, JobTimeoutSeconds: 3600})
	require.NoError(t, err)
	owner := uuid.New()
	_, err = db.AcquireGitHubRunnerPoolLease(ctx, database.AcquireGitHubRunnerPoolLeaseParams{ID: pool.ID, LeaseOwner: uuid.NullUUID{UUID: owner, Valid: true}})
	require.NoError(t, err)
	clock := quartz.NewMock(t)
	clock.Set(time.Now())
	client := &fakeClient{}
	w := &worker{controller: &Controller{Options: Options{Database: db, Clock: clock, Logger: testutil.Logger(t)}}, pool: pool, owner: owner, client: client}
	w.pool.ScaleSetID = 7
	return w, ctx, client
}

func TestReconcileLifecycle(t *testing.T) {
	t.Parallel()
	w, ctx, client := newWorker(t)
	w.pool.AssignedJobs = 2
	created := 0
	w.controller.CreateWorkspace = func(_ context.Context, _ database.GithubRunnerPool, r database.GithubRunner, jit string) (uuid.UUID, error) {
		created++
		require.Equal(t, "runner-specific-secret", jit)
		require.NotEqual(t, w.pool.GithubToken, jit)
		return uuid.Nil, xerrors.New("provisioning rejected")
	}
	// Pending allocations consume capacity before workspace provisioning.
	require.NoError(t, w.reconcile(ctx))
	runners, err := w.controller.Database.GetGitHubRunnersByPoolID(ctx, w.pool.ID)
	require.NoError(t, err)
	require.Len(t, runners, 1)
	require.NoError(t, w.reconcile(ctx))
	require.Equal(t, 1, created)
	r, err := w.controller.Database.GetGitHubRunnerByID(ctx, runners[0].ID)
	require.NoError(t, err)
	require.Equal(t, "deleting", r.State)
	require.EqualValues(t, 42, r.GithubRunnerID)
	require.NoError(t, w.reconcileRunner(ctx, r))
	require.Equal(t, []int64{42}, client.removed)
	r, err = w.controller.Database.GetGitHubRunnerByID(ctx, r.ID)
	require.NoError(t, err)
	require.Equal(t, "deleted", r.State)
}

func TestMessageDurabilityAndOrdering(t *testing.T) {
	t.Parallel()
	w, ctx, _ := newWorker(t)
	r, err := w.controller.Database.InsertGitHubRunner(ctx, database.InsertGitHubRunnerParams{ID: uuid.New(), OrganizationID: w.pool.OrganizationID, PoolID: w.pool.ID, Name: uuid.NewString()})
	require.NoError(t, err)
	r.GithubRunnerID = 42
	r.State = "provisioning"
	r, err = SaveRunner(ctx, w.controller.Database, r, w.owner)
	require.NoError(t, err)
	msg := &scaleset.RunnerScaleSetMessage{MessageID: 1, JobCompletedMessages: []*scaleset.JobCompleted{{RunnerID: 42, RunnerName: r.Name, Result: "success", JobMessageBase: scaleset.JobMessageBase{RunnerRequestID: 99}}}}
	pollCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	session := &fakeSession{message: msg, ack: func() {
		observed, err := w.controller.Database.GetGitHubRunnerByID(ctx, r.ID)
		require.NoError(t, err)
		require.Equal(t, "finishing", observed.State)
		require.Equal(t, "success", observed.JobResult)
		cancel()
	}}
	require.ErrorIs(t, w.poll(pollCtx, session), context.Canceled)
	// Replayed and delayed started events cannot resurrect completed runners.
	require.NoError(t, w.processMessage(ctx, session, msg))
	require.NoError(t, w.processMessage(ctx, session, &scaleset.RunnerScaleSetMessage{JobStartedMessages: []*scaleset.JobStarted{{RunnerID: 42, RunnerName: r.Name}}}))
	r, err = w.controller.Database.GetGitHubRunnerByID(ctx, r.ID)
	require.NoError(t, err)
	require.Equal(t, "finishing", r.State)
	w.controller.Clock.(*quartz.Mock).Advance(20 * time.Second)
	require.NoError(t, w.reconcileRunner(ctx, r))
	r, err = w.controller.Database.GetGitHubRunnerByID(ctx, r.ID)
	require.NoError(t, err)
	require.Equal(t, "deleted", r.State)
}

func TestPendingPauseDrainAndLease(t *testing.T) {
	t.Parallel()
	w, ctx, client := newWorker(t)
	r, err := w.controller.Database.InsertGitHubRunner(ctx, database.InsertGitHubRunnerParams{ID: uuid.New(), OrganizationID: w.pool.OrganizationID, PoolID: w.pool.ID, Name: uuid.NewString()})
	require.NoError(t, err)
	w.pool.State = "paused"
	require.NoError(t, w.reconcileRunner(ctx, r))
	require.Zero(t, client.generated)
	w.pool.State = "draining"
	require.NoError(t, w.reconcileRunner(ctx, r))
	require.Zero(t, client.generated)
	_, err = SaveRunner(ctx, w.controller.Database, r, uuid.New())
	require.Error(t, err)
	stored, err := w.controller.Database.GetGitHubRunnerByID(ctx, r.ID)
	require.NoError(t, err)
	require.Equal(t, "deleted", stored.State)
}

func TestRegistrationRecoveryChecksOwnership(t *testing.T) {
	t.Parallel()
	w, ctx, client := newWorker(t)
	r, err := w.controller.Database.InsertGitHubRunner(ctx, database.InsertGitHubRunnerParams{ID: uuid.New(), OrganizationID: w.pool.OrganizationID, PoolID: w.pool.ID, Name: uuid.NewString()})
	require.NoError(t, err)
	client.existing = &scaleset.RunnerReference{ID: 9, RunnerScaleSetID: 8}
	require.ErrorContains(t, w.reconcileRunner(ctx, r), "another scale set")
	require.Empty(t, client.removed)
	require.Zero(t, client.generated)
}

func TestJobDeadlineDisposesBusyWorkspace(t *testing.T) {
	t.Parallel()
	w, ctx, client := newWorker(t)
	workspace := dbgen.Workspace(t, w.controller.Database, database.WorkspaceTable{OrganizationID: w.pool.OrganizationID, TemplateID: w.pool.TemplateID, OwnerID: w.pool.OwnerID})
	r, err := w.controller.Database.InsertGitHubRunner(ctx, database.InsertGitHubRunnerParams{ID: uuid.New(), OrganizationID: w.pool.OrganizationID, PoolID: w.pool.ID, Name: uuid.NewString()})
	require.NoError(t, err)
	r.State = "busy"
	r.GithubRunnerID = 42
	r.WorkspaceID = uuid.NullUUID{UUID: workspace.ID, Valid: true}
	r, err = SaveRunner(ctx, w.controller.Database, r, w.owner)
	require.NoError(t, err)
	deleted := false
	w.controller.WorkspaceStatus = func(context.Context, uuid.UUID) (string, error) { return "running", nil }
	w.controller.DeleteWorkspace = func(_ context.Context, allocation database.GithubRunner) error {
		require.Equal(t, workspace.ID, allocation.WorkspaceID.UUID)
		deleted = true
		return nil
	}
	client.removeErr = scaleset.JobStillRunningError
	w.controller.Clock.(*quartz.Mock).Advance(time.Duration(w.pool.JobTimeoutSeconds+1) * time.Second)
	require.NoError(t, w.reconcileRunner(ctx, r))
	require.True(t, deleted)
	r, err = w.controller.Database.GetGitHubRunnerByID(ctx, r.ID)
	require.NoError(t, err)
	require.Equal(t, "deleting", r.State)
	client.removeErr = scaleset.RunnerNotFoundError
	w.controller.WorkspaceStatus = func(context.Context, uuid.UUID) (string, error) { return "deleted", nil }
	require.NoError(t, w.reconcileRunner(ctx, r))
	r, err = w.controller.Database.GetGitHubRunnerByID(ctx, r.ID)
	require.NoError(t, err)
	require.Equal(t, "deleted", r.State)
}

func TestPollDoesNotAcknowledgeLostLease(t *testing.T) {
	t.Parallel()
	w, ctx, _ := newWorker(t)
	w.owner = uuid.New()
	acknowledged := false
	s := &fakeSession{message: &scaleset.RunnerScaleSetMessage{MessageID: 1}, ack: func() { acknowledged = true }}
	require.Error(t, w.poll(ctx, s))
	require.False(t, acknowledged)
}

func TestAllocationDeadlineWithoutJobStarted(t *testing.T) {
	t.Parallel()
	w, ctx, client := newWorker(t)
	workspace := dbgen.Workspace(t, w.controller.Database, database.WorkspaceTable{OrganizationID: w.pool.OrganizationID, TemplateID: w.pool.TemplateID, OwnerID: w.pool.OwnerID})
	r, err := w.controller.Database.InsertGitHubRunner(ctx, database.InsertGitHubRunnerParams{ID: uuid.New(), OrganizationID: w.pool.OrganizationID, PoolID: w.pool.ID, Name: uuid.NewString()})
	require.NoError(t, err)
	r.State = "provisioning"
	r.GithubRunnerID = 42
	r.WorkspaceID = uuid.NullUUID{UUID: workspace.ID, Valid: true}
	r, err = SaveRunner(ctx, w.controller.Database, r, w.owner)
	require.NoError(t, err)
	deleted := false
	w.controller.WorkspaceStatus = func(context.Context, uuid.UUID) (string, error) { return "running", nil }
	w.controller.DeleteWorkspace = func(context.Context, database.GithubRunner) error { deleted = true; return nil }
	client.removeErr = scaleset.JobStillRunningError
	w.controller.Clock.(*quartz.Mock).Advance(time.Duration(w.pool.StartupTimeoutSeconds+w.pool.JobTimeoutSeconds+1) * time.Second)
	require.NoError(t, w.reconcileRunner(ctx, r))
	require.True(t, deleted)
	r, err = w.controller.Database.GetGitHubRunnerByID(ctx, r.ID)
	require.NoError(t, err)
	require.Equal(t, "deleting", r.State)
}

func TestBuildCancellationIsScopedToAllocation(t *testing.T) {
	t.Parallel()
	w, ctx, _ := newWorker(t)
	workspace := dbfake.WorkspaceBuild(t, w.controller.Database, database.WorkspaceTable{OrganizationID: w.pool.OrganizationID, OwnerID: w.pool.OwnerID}).Pending().Do()
	other := dbfake.WorkspaceBuild(t, w.controller.Database, database.WorkspaceTable{OrganizationID: w.pool.OrganizationID, OwnerID: w.pool.OwnerID}).Pending().Do()
	r, err := w.controller.Database.InsertGitHubRunner(ctx, database.InsertGitHubRunnerParams{ID: uuid.New(), OrganizationID: w.pool.OrganizationID, PoolID: w.pool.ID, Name: uuid.NewString()})
	require.NoError(t, err)
	r.WorkspaceID = uuid.NullUUID{UUID: workspace.Workspace.ID, Valid: true}
	r, err = SaveRunner(ctx, w.controller.Database, r, w.owner)
	require.NoError(t, err)
	require.NoError(t, w.controller.Database.CancelGitHubRunnerWorkspaceBuild(ctx, database.CancelGitHubRunnerWorkspaceBuildParams{RunnerID: r.ID, JobID: other.Build.JobID}))
	job, err := w.controller.Database.GetProvisionerJobByID(ctx, other.Build.JobID)
	require.NoError(t, err)
	require.False(t, job.CanceledAt.Valid)
	require.NoError(t, w.controller.Database.CancelGitHubRunnerWorkspaceBuild(ctx, database.CancelGitHubRunnerWorkspaceBuildParams{RunnerID: r.ID, JobID: workspace.Build.JobID}))
	job, err = w.controller.Database.GetProvisionerJobByID(ctx, workspace.Build.JobID)
	require.NoError(t, err)
	require.True(t, job.CanceledAt.Valid)
	require.True(t, job.CompletedAt.Valid)
}

func (*fakeSession) Session() scaleset.RunnerScaleSetSession {
	return scaleset.RunnerScaleSetSession{}
}
func (*fakeSession) Close(context.Context) error { return nil }

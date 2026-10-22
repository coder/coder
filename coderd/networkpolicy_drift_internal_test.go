package coderd

import (
	"context"
	"database/sql"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestNetworkPolicyDriftScheduler(t *testing.T) {
	t.Parallel()
	t.Run("WaitsForIntervalAndStops", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		clock := quartz.NewMock(t)
		db := dbmock.NewMockStore(gomock.NewController(t))
		api := &API{ctx: ctx, Options: &Options{Clock: clock, Database: db, Logger: testutil.Logger(t)}}
		stop := api.startNetworkPolicyDrift(15 * time.Minute)
		defer stop()
		clock.Advance(14 * time.Minute).MustWait(ctx)
		db.EXPECT().GetOrganizations(gomock.Any(), database.GetOrganizationsParams{}).DoAndReturn(func(ctx context.Context, _ database.GetOrganizationsParams) ([]database.Organization, error) {
			actor, ok := dbauthz.ActorFromContext(ctx)
			require.True(t, ok)
			require.Equal(t, rbac.SubjectTypeSystemRestricted, actor.Type)
			return nil, nil
		}).Times(1)
		clock.Advance(time.Minute).MustWait(ctx)
		stop()
		clock.Advance(15 * time.Minute).MustWait(ctx)
	})

	t.Run("Disabled", func(t *testing.T) {
		t.Parallel()
		// No context, clock, or database access is needed when disabled.
		api := &API{}
		stop := api.startNetworkPolicyDrift(0)
		stop()
		stop()
	})

	t.Run("CancelJoinsActiveScan", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		apiCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		clock := quartz.NewMock(t)
		db := dbmock.NewMockStore(gomock.NewController(t))
		started := make(chan struct{})
		finished := make(chan struct{})
		db.EXPECT().GetOrganizations(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, _ database.GetOrganizationsParams) ([]database.Organization, error) {
			close(started)
			<-ctx.Done()
			close(finished)
			return nil, ctx.Err()
		})
		api := &API{ctx: apiCtx, Options: &Options{Clock: clock, Database: db, Logger: testutil.Logger(t)}}
		stop := api.startNetworkPolicyDrift(time.Minute)
		defer stop()
		advance := clock.Advance(time.Minute)
		testutil.TryReceive(ctx, t, started)
		cancel()
		stop()
		testutil.TryReceive(ctx, t, finished)
		advance.MustWait(ctx)
		clock.Advance(time.Minute).MustWait(ctx)
	})
}

func TestNetworkPolicyDriftEligibility(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name   string
		mutate func(*database.NetworkPolicy, *database.NetworkPolicyTarget)
		want   bool
	}{
		{name: "KnownSafe", want: true},
		{name: "Deleting", mutate: func(p *database.NetworkPolicy, _ *database.NetworkPolicyTarget) { p.DeletionRequestedAt.Valid = true }},
		{name: "Deleted", mutate: func(p *database.NetworkPolicy, _ *database.NetworkPolicyTarget) { p.DeletedAt.Valid = true }},
		{name: "Destroyed", mutate: func(_ *database.NetworkPolicy, target *database.NetworkPolicyTarget) { target.DestroyedAt.Valid = true }},
		{name: "Drifted", mutate: func(_ *database.NetworkPolicy, target *database.NetworkPolicyTarget) { target.Drifted = true }, want: true},
		{name: "Unpublished", mutate: func(p *database.NetworkPolicy, _ *database.NetworkPolicyTarget) { p.PublishedRevisionID.Valid = false }},
		{name: "MissingInitiator", mutate: func(p *database.NetworkPolicy, _ *database.NetworkPolicyTarget) { p.CreatedBy = uuid.Nil }},
		{name: "ActiveOrUnknown", mutate: func(_ *database.NetworkPolicy, target *database.NetworkPolicyTarget) {
			target.ActiveJobID = uuid.NullUUID{UUID: uuid.New(), Valid: true}
		}},
		{name: "NeverApplied", mutate: func(_ *database.NetworkPolicy, target *database.NetworkPolicyTarget) {
			target.AppliedRevisionID.Valid = false
		}},
		{name: "PendingDesired", mutate: func(_ *database.NetworkPolicy, target *database.NetworkPolicyTarget) {
			target.DesiredRevisionID.UUID = uuid.New()
		}},
		{name: "NoDesired", mutate: func(_ *database.NetworkPolicy, target *database.NetworkPolicyTarget) {
			target.DesiredRevisionID.Valid = false
		}},
		{name: "Superseded", mutate: func(p *database.NetworkPolicy, _ *database.NetworkPolicyTarget) {
			p.PublishedRevisionID.UUID = uuid.New()
		}},
		{name: "Failed", mutate: func(_ *database.NetworkPolicy, target *database.NetworkPolicyTarget) { target.Status = "failed" }},
		{name: "Canceled", mutate: func(_ *database.NetworkPolicy, target *database.NetworkPolicyTarget) { target.Status = "canceled" }},
		{name: "WrongPolicy", mutate: func(_ *database.NetworkPolicy, target *database.NetworkPolicyTarget) { target.PolicyID = uuid.New() }},
		{name: "WrongOrganization", mutate: func(_ *database.NetworkPolicy, target *database.NetworkPolicyTarget) {
			target.OrganizationID = uuid.New()
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			revision := uuid.NullUUID{UUID: uuid.New(), Valid: true}
			p := database.NetworkPolicy{ID: uuid.New(), OrganizationID: uuid.New(), CreatedBy: uuid.New(), PublishedRevisionID: revision}
			target := database.NetworkPolicyTarget{PolicyID: p.ID, OrganizationID: p.OrganizationID, AppliedRevisionID: revision, DesiredRevisionID: revision, Status: "succeeded", State: []byte("saved state")}
			if tt.mutate != nil {
				tt.mutate(&p, &target)
			}
			require.Equal(t, tt.want, networkPolicyDriftEligible(p, target))
		})
	}
}

func TestNetworkPolicyDriftQueuesOnlySafePlans(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	fixture := newDriftPolicyFixture(t, db)
	otherRevision, err := db.InsertNetworkPolicyRevision(ctx, database.InsertNetworkPolicyRevisionParams{
		ID: uuid.New(), PolicyID: fixture.policy.ID, OrganizationID: fixture.policy.OrganizationID,
		RevisionNumber: 2, FileID: fixture.revision.FileID, CreatedBy: fixture.policy.CreatedBy, CreatedAt: dbtime.Now(),
	})
	require.NoError(t, err)

	var eligible, skipped []database.NetworkPolicyTarget
	for _, tt := range []struct {
		name    string
		mutate  func(*database.CompleteNetworkPolicyTargetJobParams)
		active  bool
		unknown bool
		never   bool
		badTags bool
		want    bool
	}{
		{name: "healthy", want: true},
		{name: "drifted", mutate: func(p *database.CompleteNetworkPolicyTargetJobParams) { p.Drifted = true }, want: true},
		{name: "never-applied", never: true},
		{name: "active", active: true},
		{name: "unknown", active: true, unknown: true},
		{name: "failed", mutate: func(p *database.CompleteNetworkPolicyTargetJobParams) { p.Status = "failed" }},
		{name: "canceled", mutate: func(p *database.CompleteNetworkPolicyTargetJobParams) { p.Status = "canceled" }},
		{name: "missing-state", mutate: func(p *database.CompleteNetworkPolicyTargetJobParams) { p.State = []byte{} }},
		{name: "last-error", mutate: func(p *database.CompleteNetworkPolicyTargetJobParams) { p.LastError = "uncertain" }},
		{name: "superseded", mutate: func(p *database.CompleteNetworkPolicyTargetJobParams) {
			p.AppliedRevisionID = uuid.NullUUID{UUID: otherRevision.ID, Valid: true}
		}},
		{name: "bad-tags", badTags: true},
	} {
		options := driftTargetOptions{neverApplied: tt.never, active: tt.active, mutate: tt.mutate}
		if tt.badTags {
			options.tags = json.RawMessage(`[]`)
		}
		target := fixture.target(t, db, tt.name, options)
		if tt.unknown {
			now := dbtime.Now()
			require.NoError(t, db.UpdateProvisionerJobWithCompleteByID(ctx, database.UpdateProvisionerJobWithCompleteByIDParams{
				ID: target.ActiveJobID.UUID, UpdatedAt: now, CompletedAt: sql.NullTime{Time: now, Valid: true},
				Error:     sql.NullString{String: "reconciliation required", Valid: true},
				ErrorCode: sql.NullString{String: "NETWORK_POLICY_STATE_UNKNOWN", Valid: true},
			}))
		}
		if tt.want {
			eligible = append(eligible, target)
		} else {
			skipped = append(skipped, target)
		}
	}

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	var access dbauthz.AccessControlStore = dbauthz.AGPLTemplateAccessControlStore{}
	accessControl := &atomic.Pointer[dbauthz.AccessControlStore]{}
	accessControl.Store(&access)
	authorizedDB := dbauthz.New(db, rbac.NewStrictCachingAuthorizer(prometheus.NewRegistry()), logger, accessControl)
	clock := quartz.NewMock(t)
	api := &API{ctx: ctx, Options: &Options{Database: authorizedDB, Pubsub: ps, Clock: clock, Logger: logger}}
	stop := api.startNetworkPolicyDrift(15 * time.Minute)
	defer stop()
	clock.Advance(15 * time.Minute).MustWait(ctx)

	runs, err := db.GetNetworkPolicyRunsByPolicyID(ctx, fixture.policy.ID)
	require.NoError(t, err)
	require.Len(t, runs, len(eligible), "an invalid or reserved target must not block its healthy siblings")
	for _, run := range runs {
		require.Equal(t, "plan", run.Operation)
		require.Equal(t, fixture.revision.ID, run.RevisionID)
		require.Equal(t, []uuid.UUID{fixture.template.ID}, run.TemplateIDs)
		job, err := db.GetProvisionerJobByID(ctx, run.JobID)
		require.NoError(t, err)
		require.Equal(t, fixture.policy.CreatedBy, job.InitiatorID)
		require.Equal(t, database.ProvisionerJobTypeNetworkPolicy, job.Type)
	}
	for _, before := range eligible {
		after, err := db.GetNetworkPolicyTargetByID(ctx, before.ID)
		require.NoError(t, err)
		require.True(t, after.ActiveJobID.Valid)
		require.Equal(t, "pending", after.Status)
		require.Equal(t, before.State, after.State)
		require.Equal(t, before.AppliedRevisionID, after.AppliedRevisionID)
		require.Equal(t, before.DesiredRevisionID, after.DesiredRevisionID)
		require.Equal(t, before.Drifted, after.Drifted)
	}
	for _, before := range skipped {
		after, err := db.GetNetworkPolicyTargetByID(ctx, before.ID)
		require.NoError(t, err)
		require.Equal(t, before, after)
	}
	afterPolicy, err := db.GetNetworkPolicyByID(ctx, fixture.policy.ID)
	require.NoError(t, err)
	require.Equal(t, fixture.policy, afterPolicy, "report-only scans must not publish or mutate policies")
	clock.Advance(15 * time.Minute).MustWait(ctx)
	runs, err = db.GetNetworkPolicyRunsByPolicyID(ctx, fixture.policy.ID)
	require.NoError(t, err)
	require.Len(t, runs, len(eligible), "active drift checks must not overlap")
}

func TestNetworkPolicyDriftRechecksLockedPublication(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	db, _ := dbtestutil.NewDB(t)
	fixture := newDriftPolicyFixture(t, db)
	target := fixture.target(t, db, "ready", driftTargetOptions{})
	require.True(t, networkPolicyDriftEligible(fixture.policy, target))
	newRevision, err := db.InsertNetworkPolicyRevision(ctx, database.InsertNetworkPolicyRevisionParams{
		ID: uuid.New(), PolicyID: fixture.policy.ID, OrganizationID: fixture.policy.OrganizationID,
		RevisionNumber: 2, FileID: fixture.revision.FileID, CreatedBy: fixture.policy.CreatedBy, CreatedAt: dbtime.Now(),
	})
	require.NoError(t, err)
	_, err = db.PublishNetworkPolicyRevision(ctx, database.PublishNetworkPolicyRevisionParams{
		ID: fixture.policy.ID, PublishedRevisionID: uuid.NullUUID{UUID: newRevision.ID, Valid: true}, UpdatedAt: dbtime.Now(),
	})
	require.NoError(t, err)
	// Discovery observed the old publication, but the transaction must re-read it.
	jobs, err := queueNetworkPolicyDriftTarget(ctx, db, fixture.policy.ID, target.ID)
	require.NoError(t, err)
	require.Empty(t, jobs)
	after, err := db.GetNetworkPolicyTargetByID(ctx, target.ID)
	require.NoError(t, err)
	require.Equal(t, target, after)

	// A user can preview a newer desired revision without publishing it. The
	// periodic check must not restore desired to the last applied revision.
	_, err = db.PublishNetworkPolicyRevision(ctx, database.PublishNetworkPolicyRevisionParams{
		ID: fixture.policy.ID, PublishedRevisionID: fixture.policy.PublishedRevisionID, UpdatedAt: dbtime.Now(),
	})
	require.NoError(t, err)
	preview := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
		OrganizationID: fixture.policy.OrganizationID, InitiatorID: fixture.policy.CreatedBy,
		FileID: fixture.revision.FileID, Type: database.ProvisionerJobTypeNetworkPolicy,
	})
	target, err = db.StartNetworkPolicyTargetJob(ctx, database.StartNetworkPolicyTargetJobParams{
		ID: target.ID, JobID: uuid.NullUUID{UUID: preview.ID, Valid: true}, Generation: target.Generation,
		DesiredRevisionID: uuid.NullUUID{UUID: newRevision.ID, Valid: true}, Status: "pending", UpdatedAt: dbtime.Now(),
	})
	require.NoError(t, err)
	target, err = db.CompleteNetworkPolicyTargetJob(ctx, database.CompleteNetworkPolicyTargetJobParams{
		ID: target.ID, JobID: target.ActiveJobID, Generation: target.Generation,
		State: target.State, AppliedRevisionID: target.AppliedRevisionID, Status: "succeeded", UpdatedAt: dbtime.Now(),
	})
	require.NoError(t, err)
	jobs, err = queueNetworkPolicyDriftTarget(ctx, db, fixture.policy.ID, target.ID)
	require.NoError(t, err)
	require.Empty(t, jobs)
	after, err = db.GetNetworkPolicyTargetByID(ctx, target.ID)
	require.NoError(t, err)
	require.Equal(t, target, after)
}

func TestNetworkPolicyDriftConcurrentInstances(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	db, _ := dbtestutil.NewDB(t)
	fixture := newDriftPolicyFixture(t, db)
	target := fixture.target(t, db, "ready", driftTargetOptions{})
	gate := make(chan struct{})
	type result struct {
		jobs []database.ProvisionerJob
		err  error
	}
	results := make(chan result, 2)
	for range 2 {
		go func() {
			<-gate
			jobs, err := queueNetworkPolicyDriftTarget(ctx, db, fixture.policy.ID, target.ID)
			results <- result{jobs: jobs, err: err}
		}()
	}
	close(gate)
	var count int
	for range 2 {
		result := testutil.TryReceive(ctx, t, results)
		require.NoError(t, result.err)
		count += len(result.jobs)
	}
	require.Equal(t, 1, count)
	jobs, err := queueNetworkPolicyDriftTarget(ctx, db, fixture.policy.ID, target.ID)
	require.NoError(t, err)
	require.Empty(t, jobs, "a later instance must also skip the active reservation")
	runs, err := db.GetNetworkPolicyRunsByPolicyID(ctx, fixture.policy.ID)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	require.Equal(t, "plan", runs[0].Operation)
}

type driftPolicyFixture struct {
	policy   database.NetworkPolicy
	revision database.NetworkPolicyRevision
	template database.Template
}

func newDriftPolicyFixture(t *testing.T, db database.Store) driftPolicyFixture {
	t.Helper()
	ctx := t.Context()
	org := dbgen.Organization(t, db, database.Organization{})
	user := dbgen.User(t, db, database.User{})
	file := dbgen.File(t, db, database.File{CreatedBy: user.ID})
	template := dbgen.Template(t, db, database.Template{OrganizationID: org.ID, CreatedBy: user.ID})
	now := dbtime.Now()
	p, err := db.InsertNetworkPolicy(ctx, database.InsertNetworkPolicyParams{
		ID: uuid.New(), OrganizationID: org.ID, Name: uuid.NewString(), CreatedBy: user.ID, CreatedAt: now, UpdatedAt: now,
	})
	require.NoError(t, err)
	revision, err := db.InsertNetworkPolicyRevision(ctx, database.InsertNetworkPolicyRevisionParams{
		ID: uuid.New(), PolicyID: p.ID, OrganizationID: org.ID, RevisionNumber: 1, FileID: file.ID, CreatedBy: user.ID, CreatedAt: now,
	})
	require.NoError(t, err)
	p, err = db.PublishNetworkPolicyRevision(ctx, database.PublishNetworkPolicyRevisionParams{
		ID: p.ID, PublishedRevisionID: uuid.NullUUID{UUID: revision.ID, Valid: true}, UpdatedAt: now,
	})
	require.NoError(t, err)
	_, err = db.InsertNetworkPolicyAttachment(ctx, database.InsertNetworkPolicyAttachmentParams{
		PolicyID: p.ID, TemplateID: template.ID, OrganizationID: org.ID, CreatedAt: now,
	})
	require.NoError(t, err)
	return driftPolicyFixture{policy: p, revision: revision, template: template}
}

type driftTargetOptions struct {
	neverApplied bool
	active       bool
	tags         json.RawMessage
	mutate       func(*database.CompleteNetworkPolicyTargetJobParams)
}

func (f driftPolicyFixture) target(t *testing.T, db database.Store, name string, options driftTargetOptions) database.NetworkPolicyTarget {
	t.Helper()
	ctx := t.Context()
	now := dbtime.Now()
	tags := options.tags
	if tags == nil {
		tags = json.RawMessage(`{"scope":"organization"}`)
	}
	target, err := db.InsertNetworkPolicyTarget(ctx, database.InsertNetworkPolicyTargetParams{
		ID: uuid.New(), PolicyID: f.policy.ID, OrganizationID: f.policy.OrganizationID,
		Cluster: name, Context: name, Namespace: uuid.NewString(), Tags: tags, CreatedAt: now, UpdatedAt: now,
	})
	require.NoError(t, err)
	if options.neverApplied {
		return target
	}
	job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
		OrganizationID: f.policy.OrganizationID, InitiatorID: f.policy.CreatedBy, FileID: f.revision.FileID, Type: database.ProvisionerJobTypeNetworkPolicy,
	})
	target, err = db.StartNetworkPolicyTargetJob(ctx, database.StartNetworkPolicyTargetJobParams{
		ID: target.ID, JobID: uuid.NullUUID{UUID: job.ID, Valid: true}, Generation: target.Generation,
		DesiredRevisionID: uuid.NullUUID{UUID: f.revision.ID, Valid: true}, Status: "pending", UpdatedAt: now,
	})
	require.NoError(t, err)
	complete := database.CompleteNetworkPolicyTargetJobParams{
		ID: target.ID, JobID: target.ActiveJobID, Generation: target.Generation,
		State: []byte("saved policy state"), AppliedRevisionID: uuid.NullUUID{UUID: f.revision.ID, Valid: true}, Status: "succeeded", UpdatedAt: now,
	}
	if options.mutate != nil {
		options.mutate(&complete)
	}
	target, err = db.CompleteNetworkPolicyTargetJob(ctx, complete)
	require.NoError(t, err)
	if options.active {
		// An active or quarantined target can still have a previous successful
		// revision and saved state. Neither makes another plan safe to queue.
		nextJob := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			OrganizationID: f.policy.OrganizationID, InitiatorID: f.policy.CreatedBy, FileID: f.revision.FileID, Type: database.ProvisionerJobTypeNetworkPolicy,
		})
		target, err = db.StartNetworkPolicyTargetJob(ctx, database.StartNetworkPolicyTargetJobParams{
			ID: target.ID, JobID: uuid.NullUUID{UUID: nextJob.ID, Valid: true}, Generation: target.Generation,
			DesiredRevisionID: target.DesiredRevisionID, Status: "pending", UpdatedAt: now,
		})
		require.NoError(t, err)
	}
	return target
}

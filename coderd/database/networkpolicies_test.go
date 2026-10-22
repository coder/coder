package database_test

import (
	"database/sql"
	"encoding/json"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/testutil"
)

func TestNetworkPolicyPersistence(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	db, _ := dbtestutil.NewDB(t)
	org := dbgen.Organization(t, db, database.Organization{})
	otherOrg := dbgen.Organization(t, db, database.Organization{})
	user := dbgen.User(t, db, database.User{})
	template := dbgen.Template(t, db, database.Template{OrganizationID: org.ID, CreatedBy: user.ID})
	remainingTemplate := dbgen.Template(t, db, database.Template{OrganizationID: org.ID, CreatedBy: user.ID})
	otherTemplate := dbgen.Template(t, db, database.Template{OrganizationID: otherOrg.ID, CreatedBy: user.ID})
	file := dbgen.File(t, db, database.File{CreatedBy: user.ID})
	now := dbtime.Now()
	p, err := db.InsertNetworkPolicy(ctx, database.InsertNetworkPolicyParams{ID: uuid.New(), OrganizationID: org.ID, Name: uuid.NewString(), CreatedBy: user.ID, CreatedAt: now, UpdatedAt: now})
	require.NoError(t, err)
	otherPolicy, err := db.InsertNetworkPolicy(ctx, database.InsertNetworkPolicyParams{ID: uuid.New(), OrganizationID: otherOrg.ID, Name: p.Name, CreatedBy: user.ID, CreatedAt: now, UpdatedAt: now})
	require.NoError(t, err, "names are organization scoped")
	_, err = db.InsertNetworkPolicyAttachment(ctx, database.InsertNetworkPolicyAttachmentParams{PolicyID: p.ID, TemplateID: otherTemplate.ID, OrganizationID: org.ID, CreatedAt: now})
	require.Error(t, err, "template must share the policy organization")
	_, err = db.InsertNetworkPolicyAttachment(ctx, database.InsertNetworkPolicyAttachmentParams{PolicyID: p.ID, TemplateID: otherTemplate.ID, OrganizationID: otherOrg.ID, CreatedAt: now})
	require.Error(t, err, "caller cannot substitute attachment organization")
	for _, id := range []uuid.UUID{template.ID, remainingTemplate.ID} {
		_, err = db.InsertNetworkPolicyAttachment(ctx, database.InsertNetworkPolicyAttachmentParams{PolicyID: p.ID, TemplateID: id, OrganizationID: org.ID, CreatedAt: now})
		require.NoError(t, err)
	}

	revisionArgs := database.InsertNetworkPolicyRevisionParams{ID: uuid.New(), PolicyID: p.ID, OrganizationID: org.ID, RevisionNumber: 1, Hcl: "policy {}", FileID: file.ID, CreatedBy: user.ID, CreatedAt: now}
	revision, err := db.InsertNetworkPolicyRevision(ctx, revisionArgs)
	require.NoError(t, err)
	revisionArgs.ID = uuid.New()
	_, err = db.InsertNetworkPolicyRevision(ctx, revisionArgs)
	require.Error(t, err, "revision numbers cannot be reused")
	revisionArgs.PolicyID, revisionArgs.OrganizationID = otherPolicy.ID, otherOrg.ID
	otherRevision, err := db.InsertNetworkPolicyRevision(ctx, revisionArgs)
	require.NoError(t, err)
	_, err = db.PublishNetworkPolicyRevision(ctx, database.PublishNetworkPolicyRevisionParams{ID: p.ID, PublishedRevisionID: uuid.NullUUID{UUID: otherRevision.ID, Valid: true}, UpdatedAt: now})
	require.Error(t, err, "cannot publish a foreign policy revision")
	p, err = db.PublishNetworkPolicyRevision(ctx, database.PublishNetworkPolicyRevisionParams{ID: p.ID, PublishedRevisionID: uuid.NullUUID{UUID: revision.ID, Valid: true}, UpdatedAt: now})
	require.NoError(t, err)
	require.EqualValues(t, 2, p.Generation)

	targetArgs := database.InsertNetworkPolicyTargetParams{ID: uuid.New(), PolicyID: p.ID, OrganizationID: otherOrg.ID, Cluster: "cluster", Context: "context", Namespace: "namespace", Tags: json.RawMessage(`{"scope":"organization"}`), CreatedAt: now, UpdatedAt: now}
	_, err = db.InsertNetworkPolicyTarget(ctx, targetArgs)
	require.Error(t, err, "target organization must match policy")
	targetArgs.OrganizationID = org.ID
	target, err := db.InsertNetworkPolicyTarget(ctx, targetArgs)
	require.NoError(t, err)
	require.Empty(t, target.State)
	targetArgs.ID, targetArgs.Context = uuid.New(), "context-alias"
	_, err = db.InsertNetworkPolicyTarget(ctx, targetArgs)
	require.Error(t, err, "a context alias cannot create another state owner for the same cluster and namespace")
	foreignJob := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{OrganizationID: otherOrg.ID, InitiatorID: user.ID, FileID: file.ID, Type: database.ProvisionerJobTypeNetworkPolicy})
	startArgs := database.StartNetworkPolicyTargetJobParams{ID: target.ID, JobID: uuid.NullUUID{UUID: foreignJob.ID, Valid: true}, DesiredRevisionID: uuid.NullUUID{UUID: revision.ID, Valid: true}, Generation: target.Generation, Status: "planning", UpdatedAt: now}
	_, err = db.StartNetworkPolicyTargetJob(ctx, startArgs)
	require.Error(t, err, "job must share the target organization")
	startArgs.JobID = uuid.NullUUID{}
	_, err = db.StartNetworkPolicyTargetJob(ctx, startArgs)
	require.ErrorIs(t, err, sql.ErrNoRows, "a null job must not bypass fencing")

	jobs := []database.ProvisionerJob{
		dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{OrganizationID: org.ID, InitiatorID: user.ID, FileID: file.ID, Type: database.ProvisionerJobTypeNetworkPolicy}),
		dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{OrganizationID: org.ID, InitiatorID: user.ID, FileID: file.ID, Type: database.ProvisionerJobTypeNetworkPolicy}),
	}
	// Both callers observe generation 1. Exactly one may claim the target.
	gate := make(chan struct{})
	type result struct {
		target database.NetworkPolicyTarget
		err    error
	}
	results := make(chan result, len(jobs))
	var wg sync.WaitGroup
	for _, job := range jobs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-gate
			arg := startArgs
			arg.JobID = uuid.NullUUID{UUID: job.ID, Valid: true}
			row, err := db.StartNetworkPolicyTargetJob(ctx, arg)
			results <- result{target: row, err: err}
		}()
	}
	close(gate)
	wg.Wait()
	close(results)
	var wins, conflicts int
	for result := range results {
		if result.err == nil {
			wins++
			target = result.target
		} else {
			require.ErrorIs(t, result.err, sql.ErrNoRows)
			conflicts++
		}
	}
	require.Equal(t, 1, wins)
	require.Equal(t, 1, conflicts)
	require.EqualValues(t, 2, target.Generation)

	runArgs := database.InsertNetworkPolicyRunParams{ID: uuid.New(), JobID: target.ActiveJobID.UUID, PolicyID: p.ID, TargetID: target.ID, RevisionID: revision.ID, OrganizationID: org.ID, Generation: target.Generation - 1, Operation: "plan", CreatedAt: now}
	_, err = db.InsertNetworkPolicyRun(ctx, runArgs)
	require.ErrorIs(t, err, sql.ErrNoRows, "run must match active generation")
	runArgs.Generation = target.Generation
	runArgs.RevisionID = otherRevision.ID
	_, err = db.InsertNetworkPolicyRun(ctx, runArgs)
	require.ErrorIs(t, err, sql.ErrNoRows, "run must match desired revision")
	runArgs.RevisionID = revision.ID
	run, err := db.InsertNetworkPolicyRun(ctx, runArgs)
	require.NoError(t, err)
	require.ElementsMatch(t, []uuid.UUID{template.ID, remainingTemplate.ID}, run.TemplateIDs)
	deleteArgs := database.DeleteNetworkPolicyAttachmentParams{PolicyID: p.ID, TemplateID: template.ID}
	deleted, err := db.DeleteNetworkPolicyAttachment(ctx, deleteArgs)
	require.NoError(t, err)
	require.Zero(t, deleted, "active jobs prevent detach")

	stateArgs := database.UpdateNetworkPolicyTargetStateParams{ID: target.ID, JobID: target.ActiveJobID, Generation: target.Generation - 1, State: []byte("partial state"), UpdatedAt: now}
	_, err = db.UpdateNetworkPolicyTargetState(ctx, stateArgs)
	require.ErrorIs(t, err, sql.ErrNoRows)
	stateArgs.Generation = target.Generation
	target, err = db.UpdateNetworkPolicyTargetState(ctx, stateArgs)
	require.NoError(t, err)
	require.Equal(t, stateArgs.State, target.State)
	completeArgs := database.CompleteNetworkPolicyTargetJobParams{ID: target.ID, JobID: target.ActiveJobID, Generation: target.Generation, State: target.State, Status: "failed", LastError: "partial apply", UpdatedAt: now}
	target, err = db.CompleteNetworkPolicyTargetJob(ctx, completeArgs)
	require.NoError(t, err)
	require.False(t, target.ActiveJobID.Valid)
	require.Equal(t, completeArgs.State, target.State, "failed jobs preserve partial state")
	require.Equal(t, completeArgs.JobID, target.LastJobID)

	var nextJob database.ProvisionerJob
	for _, job := range jobs {
		if job.ID != target.LastJobID.UUID {
			nextJob = job
		}
	}
	err = db.InTx(func(tx database.Store) error {
		if _, err := tx.GetNetworkPolicyByIDForUpdate(ctx, p.ID); err != nil {
			return err
		}
		if _, err := tx.DeleteNetworkPolicyAttachment(ctx, deleteArgs); err != nil {
			return err
		}
		stale := startArgs
		stale.JobID = uuid.NullUUID{UUID: nextJob.ID, Valid: true}
		_, err := tx.StartNetworkPolicyTargetJob(ctx, stale)
		return err
	}, nil)
	require.ErrorIs(t, err, sql.ErrNoRows)
	attachments, err := db.GetNetworkPolicyAttachmentsByPolicyID(ctx, p.ID)
	require.NoError(t, err)
	require.Len(t, attachments, 2, "enqueue failure rolls back the selector change")

	// A selector change and its replacement run commit together, preserving
	// existing state and the immutable authorization snapshot of the old run.
	var nextRun database.NetworkPolicyRun
	err = db.InTx(func(tx database.Store) error {
		if _, err := tx.GetNetworkPolicyByIDForUpdate(ctx, p.ID); err != nil {
			return err
		}
		lockedTarget, err := tx.GetNetworkPolicyTargetByIDForUpdate(ctx, target.ID)
		if err != nil {
			return err
		}
		deleted, err = tx.DeleteNetworkPolicyAttachment(ctx, deleteArgs)
		if err != nil {
			return err
		}
		startArgs.JobID, startArgs.Generation = uuid.NullUUID{UUID: nextJob.ID, Valid: true}, lockedTarget.Generation
		target, err = tx.StartNetworkPolicyTargetJob(ctx, startArgs)
		if err != nil {
			return err
		}
		runArgs.ID, runArgs.JobID, runArgs.Generation = uuid.New(), nextJob.ID, target.Generation
		runArgs.Operation = "apply"
		nextRun, err = tx.InsertNetworkPolicyRun(ctx, runArgs)
		return err
	}, nil)
	require.NoError(t, err)
	require.EqualValues(t, 1, deleted, "stored Terraform state does not prevent selector updates")
	require.Equal(t, completeArgs.State, target.State)
	require.Equal(t, []uuid.UUID{remainingTemplate.ID}, nextRun.TemplateIDs)
	_, err = db.CompleteNetworkPolicyTargetJob(ctx, completeArgs)
	require.ErrorIs(t, err, sql.ErrNoRows, "stale job cannot clear new active job or state")
	stateArgs.Generation = target.Generation
	_, err = db.UpdateNetworkPolicyTargetState(ctx, stateArgs)
	require.ErrorIs(t, err, sql.ErrNoRows, "job ID fences writes even with current generation")
	completeArgs.JobID, completeArgs.Generation = target.ActiveJobID, target.Generation
	completeArgs.State, completeArgs.Status, completeArgs.LastError = []byte{}, "succeeded", ""
	_, err = db.CompleteNetworkPolicyTargetJob(ctx, completeArgs)
	require.NoError(t, err)
	deleteArgs.TemplateID = remainingTemplate.ID
	deleted, err = db.DeleteNetworkPolicyAttachment(ctx, deleteArgs)
	require.NoError(t, err)
	require.EqualValues(t, 1, deleted)
	run, err = db.GetNetworkPolicyRunByJobID(ctx, run.JobID)
	require.NoError(t, err)
	require.ElementsMatch(t, []uuid.UUID{template.ID, remainingTemplate.ID}, run.TemplateIDs, "detach preserves historical snapshot")
}

func TestNetworkPolicyTransactionLocking(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	org := dbgen.Organization(t, db, database.Organization{})
	user := dbgen.User(t, db, database.User{})
	now := dbtime.Now()
	p, err := db.InsertNetworkPolicy(ctx, database.InsertNetworkPolicyParams{ID: uuid.New(), OrganizationID: org.ID, Name: uuid.NewString(), CreatedBy: user.ID, CreatedAt: now, UpdatedAt: now})
	require.NoError(t, err)
	locked := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- db.InTx(func(tx database.Store) error {
			if _, err := tx.GetNetworkPolicyByIDForUpdate(ctx, p.ID); err != nil {
				return err
			}
			close(locked)
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}, nil)
	}()
	testutil.TryReceive(ctx, t, locked)
	// NOWAIT proves the first transaction holds a row lock without sleeps.
	_, err = sqlDB.ExecContext(ctx, "SELECT id FROM network_policies WHERE id = $1 FOR UPDATE NOWAIT", p.ID)
	require.Error(t, err)
	var pqErr *pq.Error
	require.ErrorAs(t, err, &pqErr)
	require.Equal(t, pq.ErrorCode("55P03"), pqErr.Code)
	close(release)
	require.NoError(t, testutil.TryReceive(ctx, t, done))
	_, err = db.GetNetworkPolicyByIDForUpdate(ctx, p.ID)
	require.NoError(t, err)
}

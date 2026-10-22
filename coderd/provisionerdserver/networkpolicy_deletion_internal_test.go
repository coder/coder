package provisionerdserver

import (
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/provisionerd/proto"
	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"
)

const emptyNetworkPolicyState = `{"version":4,"terraform_version":"1.15.0","serial":2,"lineage":"lineage","resources":[]}`

func queueDestroyFixture(t *testing.T, db database.Store, ps pubsub.Pubsub, f networkPolicyFixture) networkPolicyFixture {
	t.Helper()
	ctx := t.Context()
	now := dbtime.Now()
	target, err := db.GetNetworkPolicyTargetByID(ctx, f.target.ID)
	require.NoError(t, err)
	runID := uuid.New()
	input, err := json.Marshal(NetworkPolicyJob{NetworkPolicyRunID: runID})
	require.NoError(t, err)
	job := dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{
		OrganizationID: f.job.OrganizationID, InitiatorID: f.job.InitiatorID, FileID: f.revision.FileID,
		Type: database.ProvisionerJobTypeNetworkPolicy, Input: input,
		StartedAt: sql.NullTime{Time: now, Valid: true}, WorkerID: f.job.WorkerID,
	})
	target, err = db.StartNetworkPolicyTargetJob(ctx, database.StartNetworkPolicyTargetJobParams{
		ID: target.ID, Generation: target.Generation, JobID: uuid.NullUUID{UUID: job.ID, Valid: true}, DesiredRevisionID: uuid.NullUUID{UUID: f.revision.ID, Valid: true}, Status: "pending", UpdatedAt: now,
	})
	require.NoError(t, err)
	run, err := db.InsertNetworkPolicyRun(ctx, database.InsertNetworkPolicyRunParams{
		ID: runID, JobID: job.ID, PolicyID: f.run.PolicyID, TargetID: target.ID, RevisionID: f.revision.ID, OrganizationID: f.job.OrganizationID, Generation: target.Generation, Operation: "destroy", CreatedAt: now,
	})
	require.NoError(t, err)
	return networkPolicyFixture{server: f.server, job: job, run: run, target: target, revision: f.revision}
}

func TestNetworkPolicyDestroyLifecycle(t *testing.T) {
	t.Parallel()
	db, ps := dbtestutil.NewDB(t)
	for _, tc := range []struct {
		name                       string
		result                     networkPolicyJobResult
		cancel, destroyed, unknown bool
	}{
		{name: "Success", result: networkPolicyJobResult{success: true, disposition: proto.StateDisposition_RETURNED, state: []byte(emptyNetworkPolicyState)}, destroyed: true},
		{name: "CanceledSuccess", cancel: true, result: networkPolicyJobResult{success: true, disposition: proto.StateDisposition_RETURNED, state: []byte(emptyNetworkPolicyState)}, destroyed: true},
		{name: "BeforeApplyFailure", result: networkPolicyJobResult{error: "plan failed", disposition: proto.StateDisposition_UNCHANGED}},
		{name: "Interrupted", result: networkPolicyJobResult{error: "canceled", disposition: proto.StateDisposition_UNKNOWN, state: []byte("checkpoint")}, unknown: true},
		{name: "InvalidSuccess", result: networkPolicyJobResult{success: true, disposition: proto.StateDisposition_RETURNED, state: []byte(`{}`)}, unknown: true},
		{name: "ManagedInstancesRemain", result: networkPolicyJobResult{success: true, disposition: proto.StateDisposition_RETURNED, state: []byte(`{"version":4,"terraform_version":"1.15.0","serial":2,"lineage":"lineage","resources":[{"mode":"managed","instances":[{}]}]}`)}, unknown: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := dbauthz.AsProvisionerd(t.Context())
			f := newNetworkPolicyFixture(t, db, ps, "apply")
			// Cancellation does not suppress provenance of returned apply state.
			require.NoError(t, db.UpdateProvisionerJobWithCancelByID(ctx, database.UpdateProvisionerJobWithCancelByIDParams{ID: f.job.ID, CanceledAt: sql.NullTime{Time: dbtime.Now(), Valid: true}}))
			require.NoError(t, f.server.finalizeNetworkPolicyJob(ctx, f.job.ID, networkPolicyJobResult{success: true, disposition: proto.StateDisposition_RETURNED, state: []byte("applied state")}))
			target, err := db.GetNetworkPolicyTargetByID(ctx, f.target.ID)
			require.NoError(t, err)
			require.False(t, target.AppliedRevisionID.Valid)
			require.Equal(t, f.run.ID, target.StateRunID.UUID)
			_, err = db.RequestNetworkPolicyDeletion(ctx, database.RequestNetworkPolicyDeletionParams{ID: f.run.PolicyID, UpdatedAt: dbtime.Now()})
			require.NoError(t, err)
			destroy := queueDestroyFixture(t, db, ps, f)
			payload, err := f.server.acquireNetworkPolicyJob(ctx, destroy.job)
			require.NoError(t, err)
			require.Equal(t, proto.NetworkPolicyOperation_DESTROY, payload.Operation)
			require.Equal(t, sdkproto.WorkspaceTransition_DESTROY, payload.Metadata.WorkspaceTransition)
			require.Equal(t, []byte("applied state"), payload.State)
			require.Equal(t, f.revision.ID.String(), payload.GetRevisionId())
			if tc.cancel {
				require.NoError(t, db.UpdateProvisionerJobWithCancelByID(ctx, database.UpdateProvisionerJobWithCancelByIDParams{ID: destroy.job.ID, CanceledAt: sql.NullTime{Time: dbtime.Now(), Valid: true}}))
			}
			require.NoError(t, f.server.finalizeNetworkPolicyJob(ctx, destroy.job.ID, tc.result))
			target, err = db.GetNetworkPolicyTargetByID(ctx, target.ID)
			require.NoError(t, err)
			require.Equal(t, tc.destroyed, target.DestroyedAt.Valid)
			require.Equal(t, tc.unknown, target.ActiveJobID.Valid)
			if tc.result.disposition != proto.StateDisposition_UNCHANGED {
				require.Equal(t, tc.result.state, target.State)
			} else {
				require.Equal(t, []byte("applied state"), target.State)
			}
			p, err := db.GetNetworkPolicyByID(ctx, f.run.PolicyID)
			require.NoError(t, err)
			require.Equal(t, tc.destroyed, p.DeletedAt.Valid)
			require.True(t, p.DeletionRequestedAt.Valid)
			switch {
			case tc.unknown:
				require.ErrorContains(t, f.server.finalizeNetworkPolicyJob(ctx, destroy.job.ID, networkPolicyJobResult{success: true, disposition: proto.StateDisposition_RETURNED, state: []byte(emptyNetworkPolicyState)}), "late results are rejected")
			case !tc.destroyed:
				retry := queueDestroyFixture(t, db, ps, f)
				require.NoError(t, f.server.finalizeNetworkPolicyJob(ctx, retry.job.ID, networkPolicyJobResult{success: true, disposition: proto.StateDisposition_RETURNED, state: []byte(emptyNetworkPolicyState)}))
			default:
				require.NoError(t, f.server.finalizeNetworkPolicyJob(ctx, destroy.job.ID, tc.result))
				again, err := db.GetNetworkPolicyByID(ctx, p.ID)
				require.NoError(t, err)
				require.Equal(t, p.Generation, again.Generation)
				job, err := db.GetProvisionerJobByID(ctx, destroy.job.ID)
				require.NoError(t, err)
				require.Equal(t, tc.cancel, job.CanceledAt.Valid)
			}
		})
	}
}

func TestNetworkPolicyDestroyAtomicFinalTargets(t *testing.T) {
	t.Parallel()
	db, ps := dbtestutil.NewDB(t)
	ctx := dbauthz.AsProvisionerd(t.Context())
	first := newNetworkPolicyFixture(t, db, ps, "apply")
	second := first
	target, err := db.InsertNetworkPolicyTarget(ctx, database.InsertNetworkPolicyTargetParams{ID: uuid.New(), PolicyID: first.run.PolicyID, OrganizationID: first.job.OrganizationID, Cluster: "second", Context: "context", Namespace: "second", Tags: json.RawMessage(`{}`), CreatedAt: dbtime.Now(), UpdatedAt: dbtime.Now()})
	require.NoError(t, err)
	// A second source run has the same frozen revision and distinct target.
	runID := uuid.New()
	job := dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{OrganizationID: first.job.OrganizationID, InitiatorID: first.job.InitiatorID, FileID: first.job.FileID, Type: database.ProvisionerJobTypeNetworkPolicy, StartedAt: first.job.StartedAt, WorkerID: first.job.WorkerID})
	target, err = db.StartNetworkPolicyTargetJob(ctx, database.StartNetworkPolicyTargetJobParams{ID: target.ID, Generation: target.Generation, JobID: uuid.NullUUID{UUID: job.ID, Valid: true}, DesiredRevisionID: first.target.DesiredRevisionID, Status: "pending", UpdatedAt: dbtime.Now()})
	require.NoError(t, err)
	run, err := db.InsertNetworkPolicyRun(ctx, database.InsertNetworkPolicyRunParams{ID: runID, JobID: job.ID, PolicyID: first.run.PolicyID, TargetID: target.ID, RevisionID: first.revision.ID, OrganizationID: first.job.OrganizationID, Generation: target.Generation, Operation: "apply", CreatedAt: dbtime.Now()})
	require.NoError(t, err)
	second.job, second.target, second.run = job, target, run
	for _, f := range []networkPolicyFixture{first, second} {
		require.NoError(t, f.server.finalizeNetworkPolicyJob(ctx, f.job.ID, networkPolicyJobResult{success: true, disposition: proto.StateDisposition_RETURNED, state: []byte("state")}))
	}
	_, err = db.RequestNetworkPolicyDeletion(ctx, database.RequestNetworkPolicyDeletionParams{ID: first.run.PolicyID, UpdatedAt: dbtime.Now()})
	require.NoError(t, err)
	first = queueDestroyFixture(t, db, ps, first)
	second = queueDestroyFixture(t, db, ps, second)
	count, err := db.CompleteNetworkPolicyDeletion(ctx, database.CompleteNetworkPolicyDeletionParams{ID: first.run.PolicyID, UpdatedAt: dbtime.Now()})
	require.NoError(t, err)
	require.Zero(t, count, "pending targets must block policy completion")
	var group errgroup.Group
	for _, f := range []networkPolicyFixture{first, second} {
		group.Go(func() error {
			return f.server.finalizeNetworkPolicyJob(ctx, f.job.ID, networkPolicyJobResult{success: true, disposition: proto.StateDisposition_RETURNED, state: []byte(emptyNetworkPolicyState)})
		})
	}
	require.NoError(t, group.Wait())
	p, err := db.GetNetworkPolicyByID(ctx, first.run.PolicyID)
	require.NoError(t, err)
	require.True(t, p.DeletedAt.Valid)
	for _, f := range []networkPolicyFixture{first, second} {
		target, err := db.GetNetworkPolicyTargetByID(ctx, f.target.ID)
		require.NoError(t, err)
		require.True(t, target.DestroyedAt.Valid)
		require.False(t, target.ActiveJobID.Valid)
		require.Equal(t, []byte(emptyNetworkPolicyState), target.State)
	}
}

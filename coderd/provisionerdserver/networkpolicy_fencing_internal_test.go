package provisionerdserver

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/codersdk/drpcsdk"
	"github.com/coder/coder/v2/provisionerd/proto"
)

func TestNetworkPolicyStaleGeneration(t *testing.T) {
	t.Parallel()
	db, ps := dbtestutil.NewDB(t)
	f := newNetworkPolicyFixture(t, db, ps, "apply")
	ctx := t.Context()
	// Simulate a newer reservation with the same job ID but a different
	// generation. Both acquisition and finalization must check the run fence.
	_, err := db.CompleteNetworkPolicyTargetJob(ctx, database.CompleteNetworkPolicyTargetJobParams{
		ID: f.target.ID, JobID: f.target.ActiveJobID, Generation: f.target.Generation,
		State: f.target.State, Status: "failed", UpdatedAt: dbtime.Now(),
	})
	require.NoError(t, err)
	target, err := db.StartNetworkPolicyTargetJob(ctx, database.StartNetworkPolicyTargetJobParams{
		ID: f.target.ID, JobID: f.target.ActiveJobID, Generation: f.target.Generation,
		DesiredRevisionID: f.target.DesiredRevisionID, Status: "pending", UpdatedAt: dbtime.Now(),
	})
	require.NoError(t, err)
	_, err = f.server.acquireNetworkPolicyJob(dbauthz.AsProvisionerd(ctx), f.job)
	require.ErrorContains(t, err, "reservation is stale")
	_, err = f.server.CompleteJob(ctx, &proto.CompletedJob{JobId: f.job.ID.String(), Type: &proto.CompletedJob_NetworkPolicy_{
		NetworkPolicy: &proto.CompletedJob_NetworkPolicy{StateDisposition: proto.StateDisposition_RETURNED, State: []byte("stale result")},
	}})
	require.ErrorContains(t, err, "reservation is stale")
	_, err = f.server.FailJob(ctx, &proto.FailedJob{JobId: f.job.ID.String(), Error: "stale failure"})
	require.ErrorContains(t, err, "reservation is stale")
	current, err := db.GetNetworkPolicyTargetByID(ctx, target.ID)
	require.NoError(t, err)
	require.Equal(t, target, current)
	job, err := db.GetProvisionerJobByID(ctx, f.job.ID)
	require.NoError(t, err)
	require.False(t, job.CompletedAt.Valid)
}

// networkPolicyChangedOwnerStore simulates ownership changing after the initial
// unlocked lookup, proving that the transaction checks the locked job again.
type networkPolicyChangedOwnerStore struct{ database.Store }

func (s networkPolicyChangedOwnerStore) InTx(fn func(database.Store) error, options *database.TxOptions) error {
	return s.Store.InTx(func(tx database.Store) error { return fn(networkPolicyChangedOwnerStore{Store: tx}) }, options)
}

func (s networkPolicyChangedOwnerStore) GetProvisionerJobByIDWithLock(ctx context.Context, id uuid.UUID) (database.ProvisionerJob, error) {
	job, err := s.Store.GetProvisionerJobByIDWithLock(ctx, id)
	job.WorkerID = uuid.NullUUID{UUID: uuid.New(), Valid: true}
	return job, err
}

func TestNetworkPolicyOwnershipRevalidation(t *testing.T) {
	t.Parallel()
	db, ps := dbtestutil.NewDB(t)
	f := newNetworkPolicyFixture(t, db, ps, "apply")
	f.server.Database = networkPolicyChangedOwnerStore{Store: f.server.Database}
	_, err := f.server.CompleteJob(t.Context(), &proto.CompletedJob{JobId: f.job.ID.String(), Type: &proto.CompletedJob_NetworkPolicy_{
		NetworkPolicy: &proto.CompletedJob_NetworkPolicy{StateDisposition: proto.StateDisposition_RETURNED, State: []byte("wrong owner")},
	}})
	require.ErrorContains(t, err, "don't own")
	_, err = f.server.FailJob(t.Context(), &proto.FailedJob{JobId: f.job.ID.String(), Error: "wrong owner"})
	require.ErrorContains(t, err, "don't own")
	_, err = f.server.acquireNetworkPolicyJob(dbauthz.AsProvisionerd(t.Context()), f.job)
	require.ErrorContains(t, err, "don't own")
	target, err := db.GetNetworkPolicyTargetByID(t.Context(), f.target.ID)
	require.NoError(t, err)
	require.Equal(t, f.target.State, target.State)
	require.Equal(t, f.target.ActiveJobID, target.ActiveJobID)
	job, err := db.GetProvisionerJobByID(t.Context(), f.job.ID)
	require.NoError(t, err)
	require.False(t, job.CompletedAt.Valid)
}

func TestNetworkPolicyAcquireOversizedStatePreserved(t *testing.T) {
	t.Parallel()
	db, ps := dbtestutil.NewDB(t)
	f := newNetworkPolicyFixture(t, db, ps, "apply")
	state := make([]byte, drpcsdk.MaxMessageSize)
	state[len(state)-1] = 'x'
	_, err := db.UpdateNetworkPolicyTargetState(t.Context(), database.UpdateNetworkPolicyTargetStateParams{
		ID: f.target.ID, JobID: f.target.ActiveJobID, Generation: f.target.Generation, State: state, UpdatedAt: dbtime.Now(),
	})
	require.NoError(t, err)
	_, err = f.server.acquireProtoJob(dbauthz.AsProvisionerd(t.Context()), f.job)
	require.ErrorContains(t, err, "payload was too big")
	target, err := db.GetNetworkPolicyTargetByID(t.Context(), f.target.ID)
	require.NoError(t, err)
	require.Equal(t, state, target.State)
	require.False(t, target.ActiveJobID.Valid)
	job, err := db.GetProvisionerJobByID(t.Context(), f.job.ID)
	require.NoError(t, err)
	require.True(t, job.CompletedAt.Valid)
}

func TestNetworkPolicyReapedJobRejectsLateResults(t *testing.T) {
	t.Parallel()
	db, ps := dbtestutil.NewDB(t)
	f := newNetworkPolicyFixture(t, db, ps, "apply")
	// The generic reaper retains the reservation and completes the job without
	// ever receiving Terraform state. Neither RPC may undo that quarantine.
	require.NoError(t, db.UpdateProvisionerJobWithCompleteByID(t.Context(), database.UpdateProvisionerJobWithCompleteByIDParams{
		ID: f.job.ID, CompletedAt: sql.NullTime{Time: dbtime.Now(), Valid: true}, UpdatedAt: dbtime.Now(),
		Error: sql.NullString{String: "reaped", Valid: true}, ErrorCode: sql.NullString{String: "NETWORK_POLICY_STATE_UNKNOWN", Valid: true},
	}))
	_, err := f.server.CompleteJob(t.Context(), &proto.CompletedJob{JobId: f.job.ID.String(), Type: &proto.CompletedJob_NetworkPolicy_{
		NetworkPolicy: &proto.CompletedJob_NetworkPolicy{State: []byte("late complete state"), StateDisposition: proto.StateDisposition_RETURNED},
	}})
	require.ErrorContains(t, err, "late results are rejected")
	_, err = f.server.FailJob(t.Context(), &proto.FailedJob{JobId: f.job.ID.String(), Error: "late failure", Type: &proto.FailedJob_NetworkPolicy_{
		NetworkPolicy: &proto.FailedJob_NetworkPolicy{State: []byte("late partial state"), StateDisposition: proto.StateDisposition_RETURNED},
	}})
	require.ErrorContains(t, err, "late results are rejected")
	target, err := db.GetNetworkPolicyTargetByID(t.Context(), f.target.ID)
	require.NoError(t, err)
	require.Equal(t, f.target, target)
}

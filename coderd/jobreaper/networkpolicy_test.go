package jobreaper_test

import (
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/jobreaper"
	"github.com/coder/coder/v2/coderd/provisionerdserver"
	"github.com/coder/coder/v2/testutil"
)

func TestDetectorNetworkPolicyReservation(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		operation  string
		started    bool
		quarantine bool
	}{
		{name: "PendingApply", operation: "apply"},
		{name: "HungApply", operation: "apply", started: true, quarantine: true},
		{name: "PendingDestroy", operation: "destroy"},
		{name: "HungDestroy", operation: "destroy", started: true, quarantine: true},
		{name: "PendingPlan", operation: "plan"},
		{name: "HungPlan", operation: "plan", started: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			env := newDetectorTestEnv(ctx, t)
			defer env.close()
			now := dbtime.Now()
			old := now.Add(-jobreaper.PendingJobDuration - time.Minute)
			org := dbgen.Organization(t, env.DB, database.Organization{})
			user := dbgen.User(t, env.DB, database.User{})
			template := dbgen.Template(t, env.DB, database.Template{OrganizationID: org.ID, CreatedBy: user.ID})
			file := dbgen.File(t, env.DB, database.File{CreatedBy: user.ID})
			policy, err := env.DB.InsertNetworkPolicy(ctx, database.InsertNetworkPolicyParams{
				ID: uuid.New(), OrganizationID: org.ID, Name: uuid.NewString(), CreatedBy: user.ID, CreatedAt: old, UpdatedAt: old,
			})
			require.NoError(t, err)
			_, err = env.DB.InsertNetworkPolicyAttachment(ctx, database.InsertNetworkPolicyAttachmentParams{
				PolicyID: policy.ID, TemplateID: template.ID, OrganizationID: org.ID, CreatedAt: old,
			})
			require.NoError(t, err)
			revision, err := env.DB.InsertNetworkPolicyRevision(ctx, database.InsertNetworkPolicyRevisionParams{
				ID: uuid.New(), PolicyID: policy.ID, OrganizationID: org.ID, RevisionNumber: 1, FileID: file.ID, Hcl: "policy", CreatedBy: user.ID, CreatedAt: old,
			})
			require.NoError(t, err)
			target, err := env.DB.InsertNetworkPolicyTarget(ctx, database.InsertNetworkPolicyTargetParams{
				ID: uuid.New(), PolicyID: policy.ID, OrganizationID: org.ID, Cluster: "cluster", Context: "context", Namespace: "namespace", Tags: json.RawMessage(`{}`), CreatedAt: old, UpdatedAt: old,
			})
			require.NoError(t, err)
			previousJob := dbgen.ProvisionerJob(t, env.DB, env.Pubsub, database.ProvisionerJob{
				OrganizationID: org.ID, InitiatorID: user.ID, FileID: file.ID, Type: database.ProvisionerJobTypeNetworkPolicy,
				CreatedAt: old, UpdatedAt: old, CompletedAt: sql.NullTime{Time: old, Valid: true},
			})
			target, err = env.DB.StartNetworkPolicyTargetJob(ctx, database.StartNetworkPolicyTargetJobParams{
				ID: target.ID, JobID: uuid.NullUUID{UUID: previousJob.ID, Valid: true}, DesiredRevisionID: uuid.NullUUID{UUID: revision.ID, Valid: true},
				Generation: target.Generation, Status: "pending", UpdatedAt: old,
			})
			require.NoError(t, err)
			source, err := env.DB.InsertNetworkPolicyRun(ctx, database.InsertNetworkPolicyRunParams{ID: uuid.New(), JobID: previousJob.ID, PolicyID: policy.ID, TargetID: target.ID, RevisionID: revision.ID, OrganizationID: org.ID, Generation: target.Generation, Operation: "apply", CreatedAt: old})
			require.NoError(t, err)
			target, err = env.DB.CompleteNetworkPolicyTargetJob(ctx, database.CompleteNetworkPolicyTargetJobParams{
				ID: target.ID, JobID: target.ActiveJobID, Generation: target.Generation,
				State: []byte("known state"), AppliedRevisionID: uuid.NullUUID{UUID: revision.ID, Valid: true}, StateRunID: uuid.NullUUID{UUID: source.ID, Valid: true},
				Status: "succeeded", Drifted: true, UpdatedAt: old,
			})
			require.NoError(t, err)
			runID := uuid.New()
			input, err := json.Marshal(provisionerdserver.NetworkPolicyJob{NetworkPolicyRunID: runID})
			require.NoError(t, err)
			seed := database.ProvisionerJob{
				OrganizationID: org.ID, InitiatorID: user.ID, FileID: file.ID, Type: database.ProvisionerJobTypeNetworkPolicy,
				Input: input, CreatedAt: old, UpdatedAt: old,
			}
			if tc.started {
				daemon := dbgen.ProvisionerDaemon(t, env.DB, database.ProvisionerDaemon{OrganizationID: org.ID})
				seed.StartedAt = sql.NullTime{Time: old, Valid: true}
				seed.WorkerID = uuid.NullUUID{UUID: daemon.ID, Valid: true}
			}
			job := dbgen.ProvisionerJob(t, env.DB, env.Pubsub, seed)
			target, err = env.DB.StartNetworkPolicyTargetJob(ctx, database.StartNetworkPolicyTargetJobParams{
				ID: target.ID, JobID: uuid.NullUUID{UUID: job.ID, Valid: true}, DesiredRevisionID: uuid.NullUUID{UUID: revision.ID, Valid: true},
				Generation: target.Generation, Status: "pending", UpdatedAt: old,
			})
			require.NoError(t, err)
			target, err = env.DB.UpdateNetworkPolicyTargetState(ctx, database.UpdateNetworkPolicyTargetStateParams{
				ID: target.ID, JobID: target.ActiveJobID, Generation: target.Generation, State: []byte("known state"), UpdatedAt: old,
			})
			require.NoError(t, err)
			_, err = env.DB.InsertNetworkPolicyRun(ctx, database.InsertNetworkPolicyRunParams{
				ID: runID, JobID: job.ID, PolicyID: policy.ID, TargetID: target.ID, RevisionID: revision.ID, OrganizationID: org.ID, Generation: target.Generation, Operation: tc.operation, CreatedAt: old,
			})
			require.NoError(t, err)

			stats := env.tick(ctx, now)
			require.NoError(t, stats.Error)
			require.Equal(t, []uuid.UUID{job.ID}, stats.TerminatedJobIDs)
			reaped, err := env.DB.GetProvisionerJobByID(ctx, job.ID)
			require.NoError(t, err)
			require.True(t, reaped.CompletedAt.Valid)
			updated, err := env.DB.GetNetworkPolicyTargetByID(ctx, target.ID)
			require.NoError(t, err)
			require.Equal(t, target.State, updated.State)
			require.Equal(t, target.AppliedRevisionID, updated.AppliedRevisionID)
			require.Equal(t, target.Drifted, updated.Drifted)
			require.Equal(t, target.Generation, updated.Generation)
			if tc.quarantine {
				require.Equal(t, "NETWORK_POLICY_STATE_UNKNOWN", reaped.ErrorCode.String)
				require.Contains(t, reaped.Error.String, "requires reconciliation")
				require.Equal(t, target.ActiveJobID, updated.ActiveJobID)
			} else {
				require.Equal(t, "NETWORK_POLICY_STATE_UNCHANGED", reaped.ErrorCode.String)
				require.False(t, updated.ActiveJobID.Valid)
				require.Equal(t, "failed", updated.Status)
				require.Equal(t, reaped.Error.String, updated.LastError)
			}
			nextJob := dbgen.ProvisionerJob(t, env.DB, env.Pubsub, database.ProvisionerJob{
				OrganizationID: org.ID, InitiatorID: user.ID, FileID: file.ID, Type: database.ProvisionerJobTypeNetworkPolicy,
			})
			_, err = env.DB.StartNetworkPolicyTargetJob(ctx, database.StartNetworkPolicyTargetJobParams{
				ID: target.ID, JobID: uuid.NullUUID{UUID: nextJob.ID, Valid: true}, DesiredRevisionID: target.DesiredRevisionID, Generation: target.Generation, Status: "pending", UpdatedAt: now,
			})
			if tc.quarantine {
				require.ErrorIs(t, err, sql.ErrNoRows)
			} else {
				require.NoError(t, err)
			}
			stats = env.tick(ctx, now)
			require.NoError(t, stats.Error)
			require.Empty(t, stats.TerminatedJobIDs)
		})
	}
}

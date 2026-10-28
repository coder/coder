package provisionerdserver

import (
	"database/sql"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/provisionerd/proto"
)

func TestTemplateResourcesCompletedPartialRecovery(t *testing.T) {
	t.Parallel()
	db, ps := dbtestutil.NewDB(t)
	for _, initial := range []bool{false, true} {
		for _, recovery := range []string{"apply", "destroy"} {
			name := recovery
			if initial {
				name += "Initial"
			}
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				f := newTemplateResourcesFixture(t, db, ps, "apply")
				ctx := dbauthz.AsProvisionerd(t.Context())
				if initial {
					state, err := db.CompleteTemplateResourceRun(ctx, database.CompleteTemplateResourceRunParams{
						FoundationID: f.foundation.ID, RunID: f.state.ActiveRunID, Status: "unconfigured", State: []byte{}, Outputs: json.RawMessage(`{}`), UpdatedAt: dbtime.Now(),
					})
					require.NoError(t, err)
					f.state = state
					_, err = db.StartTemplateResourceRun(ctx, database.StartTemplateResourceRunParams{FoundationID: f.foundation.ID, RunID: uuid.NullUUID{UUID: f.run.ID, Valid: true}, UpdatedAt: dbtime.Now()})
					require.NoError(t, err)
				}
				checkpoint := templateResourceTestState(`{"partial":{"value":"not published"}}`, `[{"mode":"managed","instances":[{}]}]`)
				require.NoError(t, f.server.finalizeTemplateResourcesJob(ctx, f.job.ID, templateResourcesJobResult{
					error: "creation provisioner failed", disposition: proto.StateDisposition_RETURNED, state: checkpoint,
				}))
				state, err := db.GetTemplateResourceState(ctx, f.foundation.ID)
				require.NoError(t, err)
				require.Equal(t, "failed", state.Status)
				require.False(t, state.ActiveRunID.Valid)
				require.Equal(t, checkpoint, state.State)
				require.Equal(t, uuid.NullUUID{UUID: f.revision.ID, Valid: true}, state.CheckpointRevisionID)
				require.Equal(t, f.state.AppliedRevisionID, state.AppliedRevisionID)
				require.JSONEq(t, string(f.state.Outputs), string(state.Outputs))

				start := func(action string) database.ProvisionerJob {
					t.Helper()
					runID := uuid.New()
					input, err := json.Marshal(TemplateResourcesJob{TemplateResourceRunID: runID})
					require.NoError(t, err)
					job := dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{
						OrganizationID: f.job.OrganizationID, InitiatorID: f.job.InitiatorID, FileID: f.job.FileID,
						Type: database.ProvisionerJobTypeTemplateResources, Input: input,
						StartedAt: sql.NullTime{Time: dbtime.Now(), Valid: true}, WorkerID: f.job.WorkerID,
					})
					_, err = db.InsertTemplateResourceRun(ctx, database.InsertTemplateResourceRunParams{
						ID: runID, FoundationID: f.foundation.ID, OrganizationID: f.job.OrganizationID,
						RevisionID: f.revision.ID, JobID: job.ID, Action: action, CreatedAt: dbtime.Now(),
					})
					require.NoError(t, err)
					_, err = db.StartTemplateResourceRun(ctx, database.StartTemplateResourceRunParams{
						FoundationID: f.foundation.ID, RunID: uuid.NullUUID{UUID: runID, Valid: true}, UpdatedAt: dbtime.Now(),
					})
					require.NoError(t, err)
					payload, err := f.server.acquireTemplateResourcesJob(ctx, job)
					require.NoError(t, err)
					require.Equal(t, checkpoint, payload.State)
					require.Equal(t, f.revision.ID.String(), payload.GetRevisionId())
					return job
				}

				// A failed pre-apply plan must preserve the partial checkpoint,
				// not restore the older published snapshot or quarantine it.
				preApply := start("apply")
				require.NoError(t, f.server.finalizeTemplateResourcesJob(ctx, preApply.ID, templateResourcesJobResult{error: "plan failed", disposition: proto.StateDisposition_UNCHANGED}))
				afterPlan, err := db.GetTemplateResourceState(ctx, f.foundation.ID)
				require.NoError(t, err)
				require.Equal(t, state.State, afterPlan.State)
				require.Equal(t, state.Status, afterPlan.Status)
				require.Equal(t, state.CheckpointRevisionID, afterPlan.CheckpointRevisionID)
				require.Equal(t, state.AppliedRevisionID, afterPlan.AppliedRevisionID)

				plan := start("plan")
				require.NoError(t, f.server.finalizeTemplateResourcesJob(ctx, plan.ID, templateResourcesJobResult{success: true, disposition: proto.StateDisposition_UNCHANGED}))
				retry := start(recovery)
				finalState := templateResourceTestState(`{}`, `[]`)
				require.NoError(t, f.server.finalizeTemplateResourcesJob(ctx, retry.ID, templateResourcesJobResult{success: true, disposition: proto.StateDisposition_RETURNED, state: finalState}))
				state, err = db.GetTemplateResourceState(ctx, f.foundation.ID)
				require.NoError(t, err)
				require.False(t, state.ActiveRunID.Valid)
				require.Equal(t, finalState, state.State)
				if recovery == "destroy" {
					require.Equal(t, "destroyed", state.Status)
					require.False(t, state.AppliedRevisionID.Valid)
					require.False(t, state.CheckpointRevisionID.Valid)
				} else {
					require.Equal(t, "applied", state.Status)
					require.Equal(t, uuid.NullUUID{UUID: f.revision.ID, Valid: true}, state.AppliedRevisionID)
					require.Equal(t, state.AppliedRevisionID, state.CheckpointRevisionID)
				}
			})
		}
	}
}

func TestTemplateResourcesOutputPublicationFailureKeepsCheckpoint(t *testing.T) {
	t.Parallel()
	db, ps := dbtestutil.NewDB(t)
	f := newTemplateResourcesFixture(t, db, ps, "apply")
	ctx := dbauthz.AsProvisionerd(t.Context())
	checkpoint := templateResourceTestState(`{"large":{"value":"`+strings.Repeat("x", maxTemplateResourceOutputsSize)+`","type":"string"}}`, `[]`)
	require.NoError(t, f.server.finalizeTemplateResourcesJob(ctx, f.job.ID, templateResourcesJobResult{success: true, disposition: proto.StateDisposition_RETURNED, state: checkpoint}))
	state, err := db.GetTemplateResourceState(ctx, f.foundation.ID)
	require.NoError(t, err)
	require.Equal(t, "failed", state.Status)
	require.False(t, state.ActiveRunID.Valid)
	require.Equal(t, checkpoint, state.State)
	require.Equal(t, uuid.NullUUID{UUID: f.revision.ID, Valid: true}, state.CheckpointRevisionID)
	require.Equal(t, f.state.AppliedRevisionID, state.AppliedRevisionID)
	require.JSONEq(t, string(f.state.Outputs), string(state.Outputs))
	job, err := db.GetProvisionerJobByID(ctx, f.job.ID)
	require.NoError(t, err)
	require.True(t, job.CompletedAt.Valid)
	require.Contains(t, job.Error.String, "64 KiB")
	require.NotEqual(t, templateResourcesStateUnknown, job.ErrorCode.String)
}

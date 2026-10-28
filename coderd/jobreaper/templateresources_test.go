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
	"github.com/coder/coder/v2/testutil"
)

func TestDetectorTemplateResourceReservation(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, action     string
		status           string
		started, unknown bool
	}{
		{name: "PendingApply", action: "apply"},
		{name: "HungApply", action: "apply", started: true, unknown: true},
		{name: "PendingDestroy", action: "destroy"},
		{name: "HungDestroy", action: "destroy", started: true, unknown: true},
		{name: "PendingPlan", action: "plan"},
		{name: "HungPlan", action: "plan", started: true},
		{name: "PendingPlanAfterPartialApply", action: "plan", status: "failed"},
		{name: "HungPlanAfterPartialApply", action: "plan", started: true, status: "failed"},
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
			templateFoundation := dbgen.Foundation(t, env.DB, database.Foundation{OrganizationID: template.OrganizationID, TemplateID: uuid.NullUUID{UUID: template.ID, Valid: true}})
			file := dbgen.File(t, env.DB, database.File{CreatedBy: user.ID})
			_, err := env.DB.InsertTemplateResourceState(ctx, database.InsertTemplateResourceStateParams{FoundationID: templateFoundation.ID, OrganizationID: org.ID, UpdatedAt: old})
			require.NoError(t, err)
			revision, err := env.DB.InsertTemplateResourceRevision(ctx, database.InsertTemplateResourceRevisionParams{ID: uuid.New(), FoundationID: templateFoundation.ID, OrganizationID: org.ID, FileID: file.ID, Source: "# immutable", CreatedBy: user.ID, CreatedAt: old})
			require.NoError(t, err)
			previous := dbgen.ProvisionerJob(t, env.DB, nil, database.ProvisionerJob{OrganizationID: org.ID, InitiatorID: user.ID, FileID: file.ID, Type: database.ProvisionerJobTypeTemplateResources, CreatedAt: old, UpdatedAt: old, CompletedAt: sql.NullTime{Time: old, Valid: true}})
			applied, err := env.DB.InsertTemplateResourceRun(ctx, database.InsertTemplateResourceRunParams{ID: uuid.New(), FoundationID: templateFoundation.ID, OrganizationID: org.ID, RevisionID: revision.ID, JobID: previous.ID, Action: "apply", CreatedAt: old})
			require.NoError(t, err)
			state, err := env.DB.StartTemplateResourceRun(ctx, database.StartTemplateResourceRunParams{FoundationID: templateFoundation.ID, RunID: uuid.NullUUID{UUID: applied.ID, Valid: true}, UpdatedAt: old})
			require.NoError(t, err)
			status := tc.status
			if status == "" {
				status = "applied"
			}
			state, err = env.DB.CompleteTemplateResourceRun(ctx, database.CompleteTemplateResourceRunParams{FoundationID: templateFoundation.ID, RunID: state.ActiveRunID, Status: status, State: []byte("retained state"), Outputs: json.RawMessage(`{"value":123}`), AppliedRevisionID: uuid.NullUUID{UUID: revision.ID, Valid: true}, CheckpointRevisionID: uuid.NullUUID{UUID: revision.ID, Valid: true}, UpdatedAt: old})
			require.NoError(t, err)
			seed := database.ProvisionerJob{OrganizationID: org.ID, InitiatorID: user.ID, FileID: file.ID, Type: database.ProvisionerJobTypeTemplateResources, CreatedAt: old, UpdatedAt: old}
			if tc.started {
				daemon := dbgen.ProvisionerDaemon(t, env.DB, database.ProvisionerDaemon{OrganizationID: org.ID})
				seed.StartedAt = sql.NullTime{Time: old, Valid: true}
				seed.WorkerID = uuid.NullUUID{UUID: daemon.ID, Valid: true}
			}
			job := dbgen.ProvisionerJob(t, env.DB, nil, seed)
			run, err := env.DB.InsertTemplateResourceRun(ctx, database.InsertTemplateResourceRunParams{ID: uuid.New(), FoundationID: templateFoundation.ID, OrganizationID: org.ID, RevisionID: revision.ID, JobID: job.ID, Action: tc.action, CreatedAt: old})
			require.NoError(t, err)
			_, err = env.DB.StartTemplateResourceRun(ctx, database.StartTemplateResourceRunParams{FoundationID: templateFoundation.ID, RunID: uuid.NullUUID{UUID: run.ID, Valid: true}, UpdatedAt: old})
			require.NoError(t, err)
			stats := env.tick(ctx, now)
			require.NoError(t, stats.Error)
			require.Contains(t, stats.TerminatedJobIDs, job.ID)
			updated, err := env.DB.GetTemplateResourceState(ctx, templateFoundation.ID)
			require.NoError(t, err)
			require.Equal(t, state.State, updated.State)
			require.Equal(t, state.Outputs, updated.Outputs)
			require.Equal(t, state.AppliedRevisionID, updated.AppliedRevisionID)
			require.Equal(t, state.CheckpointRevisionID, updated.CheckpointRevisionID)
			reaped, err := env.DB.GetProvisionerJobByID(ctx, job.ID)
			require.NoError(t, err)
			require.True(t, reaped.CompletedAt.Valid)
			if tc.unknown {
				require.Equal(t, "unknown", updated.Status)
				require.Equal(t, run.ID, updated.ActiveRunID.UUID)
				require.Equal(t, "TEMPLATE_RESOURCE_STATE_UNKNOWN", reaped.ErrorCode.String)
				require.Contains(t, reaped.Error.String, "Shared resource outcome is unknown; manual reconciliation is required.")
			} else {
				require.Equal(t, status, updated.Status)
				require.False(t, updated.ActiveRunID.Valid)
				require.Equal(t, "TEMPLATE_RESOURCE_STATE_UNCHANGED", reaped.ErrorCode.String)
			}
		})
	}
}

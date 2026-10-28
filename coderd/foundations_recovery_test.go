package coderd_test

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/codersdk"
)

func TestFoundationDestroyInitialFailedApply(t *testing.T) {
	t.Parallel()
	client, db := newTemplateResourceClient(t)
	owner := coderdtest.CreateFirstUser(t, client)
	ctx := t.Context()
	//nolint:gocritic // Simulate a completed partial apply without cloud resources.
	systemCtx := dbauthz.AsSystemRestricted(ctx)
	f, err := client.CreateFoundation(ctx, owner.OrganizationID, codersdk.CreateFoundationRequest{Name: "partial-apply"})
	require.NoError(t, err)
	revision, err := client.CreateFoundationRevision(ctx, owner.OrganizationID, f.ID, codersdk.CreateTemplateResourceRevisionRequest{Source: `resource "terraform_data" "partial" { input = "created before failure" }`})
	require.NoError(t, err)
	apply, err := client.CreateFoundationRun(ctx, owner.OrganizationID, f.ID, codersdk.CreateTemplateResourceRunRequest{RevisionID: revision.ID, Action: "apply"})
	require.NoError(t, err)
	checkpoint := []byte(`{"version":4,"serial":1,"lineage":"partial-apply","outputs":{},"resources":[{"mode":"managed","type":"terraform_data","name":"partial","instances":[{"attributes":{"id":"created-before-failure"}}]}]}`)
	now := dbtime.Now()
	_, err = db.CompleteTemplateResourceRun(systemCtx, database.CompleteTemplateResourceRunParams{
		FoundationID: f.ID, RunID: uuid.NullUUID{UUID: apply.ID, Valid: true}, Status: "failed",
		State: checkpoint, Outputs: json.RawMessage(`{}`), CheckpointRevisionID: uuid.NullUUID{UUID: revision.ID, Valid: true}, UpdatedAt: now,
	})
	require.NoError(t, err)
	require.NoError(t, db.UpdateProvisionerJobWithCompleteByID(systemCtx, database.UpdateProvisionerJobWithCompleteByIDParams{
		ID: apply.Job.ID, UpdatedAt: now, CompletedAt: sql.NullTime{Time: now, Valid: true}, Error: sql.NullString{String: "apply failed after creating one resource", Valid: true},
	}))
	newer, err := client.CreateFoundationRevision(ctx, owner.OrganizationID, f.ID, codersdk.CreateTemplateResourceRevisionRequest{Source: "# unrelated draft"})
	require.NoError(t, err)
	f, err = client.Foundation(ctx, owner.OrganizationID, f.ID)
	require.NoError(t, err)
	require.Equal(t, "failed", f.Status)
	require.Nil(t, f.AppliedRevisionID, "initial failed apply has never published outputs")
	require.Equal(t, &revision.ID, f.CheckpointRevisionID)
	require.Equal(t, &newer.ID, f.CurrentRevisionID)
	require.Equal(t, http.StatusConflict, coderdtest.SDKError(t, client.DeleteFoundation(ctx, owner.OrganizationID, f.ID)).StatusCode(), "partial infrastructure requires cleanup")
	_, err = client.CreateFoundationRun(ctx, owner.OrganizationID, f.ID, codersdk.CreateTemplateResourceRunRequest{RevisionID: newer.ID, Action: "destroy"})
	require.Equal(t, http.StatusConflict, coderdtest.SDKError(t, err).StatusCode(), "destroy cannot use an unrelated draft")
	destroy, err := client.CreateFoundationRun(ctx, owner.OrganizationID, f.ID, codersdk.CreateTemplateResourceRunRequest{RevisionID: revision.ID, Action: "destroy"})
	require.NoError(t, err, "a valid checkpoint permits destroy without an applied revision")
	job, err := db.GetProvisionerJobByID(systemCtx, destroy.Job.ID)
	require.NoError(t, err)
	require.Equal(t, revision.FileID, job.FileID, "cleanup uses the checkpoint-producing archive")

	require.NoError(t, client.CancelFoundationRun(ctx, owner.OrganizationID, f.ID, destroy.ID))
	state, err := db.GetTemplateResourceState(systemCtx, f.ID)
	require.NoError(t, err)
	require.Equal(t, checkpoint, state.State)
	require.Equal(t, revision.ID, state.CheckpointRevisionID.UUID, "cancel before acquisition preserves the cleanup source")
	require.False(t, state.AppliedRevisionID.Valid)
	retry, err := client.CreateFoundationRun(ctx, owner.OrganizationID, f.ID, codersdk.CreateTemplateResourceRunRequest{RevisionID: revision.ID, Action: "destroy"})
	require.NoError(t, err)
	_, err = db.CompleteTemplateResourceRun(systemCtx, database.CompleteTemplateResourceRunParams{
		FoundationID: f.ID, RunID: uuid.NullUUID{UUID: retry.ID, Valid: true}, Status: "destroyed",
		State: []byte(`{"version":4,"resources":[]}`), Outputs: json.RawMessage(`{}`), UpdatedAt: dbtime.Now(),
	})
	require.NoError(t, err)
	require.NoError(t, client.DeleteFoundation(ctx, owner.OrganizationID, f.ID))
}

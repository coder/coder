package database_test

import (
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
)

func TestFoundationConsumerCounts(t *testing.T) {
	t.Parallel()
	db, _ := dbtestutil.NewDB(t)
	ctx := t.Context()
	org := dbgen.Organization(t, db, database.Organization{})
	user := dbgen.User(t, db, database.User{})
	template := dbgen.Template(t, db, database.Template{OrganizationID: org.ID, CreatedBy: user.ID})
	f := dbgen.Foundation(t, db, database.Foundation{OrganizationID: org.ID, TemplateID: uuid.NullUUID{UUID: template.ID, Valid: true}})
	file := dbgen.File(t, db, database.File{CreatedBy: user.ID})
	importJob := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{OrganizationID: org.ID, InitiatorID: user.ID, FileID: file.ID})
	version := dbgen.TemplateVersion(t, db, database.TemplateVersion{TemplateID: uuid.NullUUID{UUID: template.ID, Valid: true}, OrganizationID: org.ID, JobID: importJob.ID, CreatedBy: user.ID})
	now := sql.NullTime{Time: dbtime.Now(), Valid: true}
	var pinnedBuild, stateBuild database.WorkspaceBuild
	for _, tc := range []struct {
		name                         string
		transition                   database.WorkspaceTransition
		started, completed, canceled sql.NullTime
		jobError                     sql.NullString
	}{
		{name: "running", transition: database.WorkspaceTransitionStart, started: now, completed: now},
		{name: "stopped", transition: database.WorkspaceTransitionStop, started: now, completed: now},
		{name: "pending", transition: database.WorkspaceTransitionStart},
		{name: "starting", transition: database.WorkspaceTransitionStart, started: now},
		{name: "stopping", transition: database.WorkspaceTransitionStop, started: now},
		{name: "deleting", transition: database.WorkspaceTransitionDelete, started: now},
		{name: "failed", transition: database.WorkspaceTransitionStart, started: now, completed: now, jobError: sql.NullString{String: "failed", Valid: true}},
		{name: "canceled", transition: database.WorkspaceTransitionStop, started: now, completed: now, canceled: now},
		{name: "deleted-build", transition: database.WorkspaceTransitionDelete, started: now, completed: now},
	} {
		w := dbgen.Workspace(t, db, database.WorkspaceTable{TemplateID: template.ID, OrganizationID: org.ID, OwnerID: user.ID})
		job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{OrganizationID: org.ID, InitiatorID: user.ID, FileID: file.ID, StartedAt: tc.started, CompletedAt: tc.completed, CanceledAt: tc.canceled, Error: tc.jobError})
		transition := tc.transition
		if tc.name == "stopped" {
			transition = database.WorkspaceTransitionStart
		}
		build := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{WorkspaceID: w.ID, TemplateVersionID: version.ID, JobID: job.ID, InitiatorID: user.ID, Transition: transition})
		if tc.name == "running" {
			stateBuild = build
		}
		if tc.name == "stopped" {
			pinnedBuild = build
			// Only the newest build determines the current classification.
			stopJob := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{OrganizationID: org.ID, InitiatorID: user.ID, FileID: file.ID, StartedAt: now, CompletedAt: now})
			dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{WorkspaceID: w.ID, TemplateVersionID: version.ID, JobID: stopJob.ID, InitiatorID: user.ID, Transition: database.WorkspaceTransitionStop, BuildNumber: 2})
		}
	}
	dbgen.Workspace(t, db, database.WorkspaceTable{TemplateID: template.ID, OrganizationID: org.ID, OwnerID: user.ID})
	deleted := dbgen.Workspace(t, db, database.WorkspaceTable{TemplateID: template.ID, OrganizationID: org.ID, OwnerID: user.ID})
	require.NoError(t, db.UpdateWorkspaceDeletedByID(ctx, database.UpdateWorkspaceDeletedByIDParams{ID: deleted.ID, Deleted: true}))
	other := dbgen.Template(t, db, database.Template{OrganizationID: org.ID, CreatedBy: user.ID})
	dbgen.Workspace(t, db, database.WorkspaceTable{TemplateID: other.ID, OrganizationID: org.ID, OwnerID: user.ID})
	counts, err := db.GetFoundationConsumerCounts(ctx, f.ID)
	require.NoError(t, err)
	require.Equal(t, database.GetFoundationConsumerCountsRow{Total: 10, Running: 1, Stopped: 1, Unknown: 8}, counts)
	count, err := db.GetFoundationWorkspaceCount(ctx, f.ID)
	require.NoError(t, err)
	require.Equal(t, count, counts.Total)

	revision, err := db.InsertTemplateResourceRevision(ctx, database.InsertTemplateResourceRevisionParams{ID: uuid.New(), FoundationID: f.ID, OrganizationID: org.ID, FileID: file.ID, CreatedBy: user.ID, CreatedAt: dbtime.Now(), Source: "# historical"})
	require.NoError(t, err)
	require.NoError(t, db.PinWorkspaceBuildTemplateResources(ctx, database.PinWorkspaceBuildTemplateResourcesParams{ID: pinnedBuild.ID, TemplateResourceRevisionID: uuid.NullUUID{UUID: revision.ID, Valid: true}, TemplateResourceOutputs: json.RawMessage(`{}`), StateTemplateResourceOutputs: json.RawMessage(`{}`)}))
	require.NoError(t, db.PinWorkspaceBuildTemplateResources(ctx, database.PinWorkspaceBuildTemplateResourcesParams{ID: stateBuild.ID, TemplateResourceOutputs: json.RawMessage(`{}`), StateTemplateResourceRevisionID: uuid.NullUUID{UUID: revision.ID, Valid: true}, StateTemplateResourceOutputs: json.RawMessage(`{}`)}))
	_, err = db.DeleteFoundation(ctx, database.DeleteFoundationParams{ID: f.ID, UpdatedAt: dbtime.Now()})
	require.NoError(t, err)
	counts, err = db.GetFoundationConsumerCounts(ctx, f.ID)
	require.NoError(t, err)
	require.Equal(t, database.GetFoundationConsumerCountsRow{Total: 2, Running: 1, Stopped: 1}, counts, "historical input and state pins remain consumers after association is cleared")
}

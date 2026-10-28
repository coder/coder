package database_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
)

func TestFoundationPreviewConsumerVersions(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	db, _ := dbtestutil.NewDB(t)
	org := dbgen.Organization(t, db, database.Organization{})
	user := dbgen.User(t, db, database.User{})
	file := dbgen.File(t, db, database.File{CreatedBy: user.ID})
	job := func() database.ProvisionerJob {
		return dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{OrganizationID: org.ID, InitiatorID: user.ID, FileID: file.ID})
	}
	active := dbgen.TemplateVersion(t, db, database.TemplateVersion{OrganizationID: org.ID, CreatedBy: user.ID, JobID: job().ID})
	template := dbgen.Template(t, db, database.Template{OrganizationID: org.ID, CreatedBy: user.ID, ActiveVersionID: active.ID})
	foundation := dbgen.Foundation(t, db, database.Foundation{OrganizationID: org.ID, TemplateID: uuid.NullUUID{UUID: template.ID, Valid: true}})
	revision, err := db.InsertTemplateResourceRevision(ctx, database.InsertTemplateResourceRevisionParams{
		ID: uuid.New(), FoundationID: foundation.ID, OrganizationID: org.ID, FileID: file.ID, CreatedBy: user.ID, CreatedAt: dbtime.Now(),
	})
	require.NoError(t, err)
	workspace := dbgen.Workspace(t, db, database.WorkspaceTable{OrganizationID: org.ID, OwnerID: user.ID, TemplateID: template.ID})
	used := dbgen.TemplateVersion(t, db, database.TemplateVersion{OrganizationID: org.ID, CreatedBy: user.ID, JobID: job().ID})
	dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{WorkspaceID: workspace.ID, TemplateVersionID: used.ID, InitiatorID: user.ID, JobID: job().ID})
	deleted := dbgen.Workspace(t, db, database.WorkspaceTable{OrganizationID: org.ID, OwnerID: user.ID, TemplateID: template.ID, Deleted: true})
	unused := dbgen.TemplateVersion(t, db, database.TemplateVersion{OrganizationID: org.ID, CreatedBy: user.ID, JobID: job().ID})
	dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{WorkspaceID: deleted.ID, TemplateVersionID: unused.ID, InitiatorID: user.ID, JobID: job().ID})

	// Historical pins remain consumers after their template is detached.
	other := dbgen.Template(t, db, database.Template{OrganizationID: org.ID, CreatedBy: user.ID})
	historical := dbgen.Workspace(t, db, database.WorkspaceTable{OrganizationID: org.ID, OwnerID: user.ID, TemplateID: other.ID})
	pinnedVersion := dbgen.TemplateVersion(t, db, database.TemplateVersion{OrganizationID: org.ID, CreatedBy: user.ID, JobID: job().ID})
	build := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{WorkspaceID: historical.ID, TemplateVersionID: pinnedVersion.ID, InitiatorID: user.ID, JobID: job().ID})
	require.NoError(t, db.PinWorkspaceBuildTemplateResources(ctx, database.PinWorkspaceBuildTemplateResourcesParams{
		ID: build.ID, TemplateResourceRevisionID: uuid.NullUUID{UUID: revision.ID, Valid: true}, TemplateResourceOutputs: []byte(`{}`), StateTemplateResourceOutputs: []byte(`{}`),
	}))
	rows, err := db.GetFoundationConsumerTemplateVersions(ctx, foundation.ID)
	require.NoError(t, err)
	var ids []uuid.UUID
	for _, row := range rows {
		ids = append(ids, row.ID)
		require.Equal(t, file.ID, row.FileID)
	}
	require.ElementsMatch(t, []uuid.UUID{active.ID, used.ID, pinnedVersion.ID}, ids)

	run, err := db.InsertTemplateResourceRun(ctx, database.InsertTemplateResourceRunParams{
		ID: uuid.New(), FoundationID: foundation.ID, OrganizationID: org.ID, RevisionID: revision.ID, JobID: job().ID, Action: "plan", CreatedAt: dbtime.Now(),
	})
	require.NoError(t, err)
	require.False(t, run.PlanPreview.Valid)
	require.NoError(t, db.UpdateTemplateResourceRunPlanPreview(ctx, database.UpdateTemplateResourceRunPlanPreviewParams{
		ID: run.ID, PlanPreview: pqtype.NullRawMessage{Valid: true, RawMessage: []byte(`{"available":false}`)},
	}))
	stored, err := db.GetTemplateResourceRunByID(ctx, run.ID)
	require.NoError(t, err)
	require.True(t, stored.PlanPreview.Valid)
	require.JSONEq(t, `{"available":false}`, string(stored.PlanPreview.RawMessage))
}

package dlppolicy_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/dlppolicy"
	"github.com/coder/coder/v2/testutil"
)

func TestForWorkspace(t *testing.T) {
	t.Parallel()

	t.Run("WorkspaceWithPolicy", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		db, _ := dbtestutil.NewDB(t)
		org := dbgen.Organization(t, db, database.Organization{})
		user := dbgen.User(t, db, database.User{})
		tpl := dbgen.Template(t, db, database.Template{
			OrganizationID: org.ID,
			CreatedBy:      user.ID,
		})
		job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			Type:           database.ProvisionerJobTypeTemplateVersionImport,
			OrganizationID: org.ID,
		})
		tv := dbgen.TemplateVersion(t, db, database.TemplateVersion{
			JobID:          job.ID,
			OrganizationID: org.ID,
			TemplateID:     uuid.NullUUID{UUID: tpl.ID, Valid: true},
			CreatedBy:      user.ID,
		})

		_, err := db.InsertTemplateVersionDLPPolicy(ctx, database.InsertTemplateVersionDLPPolicyParams{
			ID:                   uuid.New(),
			TemplateVersionID:    tv.ID,
			Name:                 "strict",
			SshAccess:            true,
			WebTerminalAccess:    false,
			PortForwardingAccess: true,
			AllowedApplications:  []string{"code-server"},
			CreatedAt:            time.Now(),
		})
		require.NoError(t, err)

		ws := dbgen.Workspace(t, db, database.WorkspaceTable{
			OrganizationID: org.ID,
			OwnerID:        user.ID,
			TemplateID:     tpl.ID,
		})
		buildJob := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			Type:           database.ProvisionerJobTypeWorkspaceBuild,
			OrganizationID: org.ID,
		})
		dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
			WorkspaceID:       ws.ID,
			TemplateVersionID: tv.ID,
			BuildNumber:       1,
			JobID:             buildJob.ID,
		})

		got, err := dlppolicy.ForWorkspace(context.Background(), db, ws.ID)
		require.NoError(t, err)
		require.NotNil(t, got)
		require.Equal(t, "strict", got.Name)
		require.True(t, got.SshAccess)
		require.False(t, got.WebTerminalAccess)
	})

	t.Run("WorkspaceWithoutPolicy", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		org := dbgen.Organization(t, db, database.Organization{})
		user := dbgen.User(t, db, database.User{})
		tpl := dbgen.Template(t, db, database.Template{
			OrganizationID: org.ID,
			CreatedBy:      user.ID,
		})
		job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			Type:           database.ProvisionerJobTypeWorkspaceBuild,
			OrganizationID: org.ID,
		})
		tv := dbgen.TemplateVersion(t, db, database.TemplateVersion{
			JobID:          job.ID,
			OrganizationID: org.ID,
			TemplateID:     uuid.NullUUID{UUID: tpl.ID, Valid: true},
			CreatedBy:      user.ID,
		})
		ws := dbgen.Workspace(t, db, database.WorkspaceTable{
			OrganizationID: org.ID,
			OwnerID:        user.ID,
			TemplateID:     tpl.ID,
		})
		buildJob := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			Type:           database.ProvisionerJobTypeWorkspaceBuild,
			OrganizationID: org.ID,
		})
		dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
			WorkspaceID:       ws.ID,
			TemplateVersionID: tv.ID,
			BuildNumber:       1,
			JobID:             buildJob.ID,
		})

		got, err := dlppolicy.ForWorkspace(context.Background(), db, ws.ID)
		require.NoError(t, err)
		require.Nil(t, got)
	})

	t.Run("WorkspaceNotFound", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)

		got, err := dlppolicy.ForWorkspace(context.Background(), db, uuid.New())
		require.NoError(t, err)
		require.Nil(t, got)
	})
}

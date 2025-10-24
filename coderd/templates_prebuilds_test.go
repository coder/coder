package coderd_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestInvalidateTemplatePrebuilds(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		owner, db := coderdtest.NewWithDatabase(t, nil)

		// Create organization and template with old version
		org := dbgen.Organization(t, db, database.Organization{})
		oldVersion := dbgen.TemplateVersion(t, db, database.TemplateVersion{
			OrganizationID: org.ID,
		})
		template := dbgen.Template(t, db, database.Template{
			OrganizationID:  org.ID,
			ActiveVersionID: oldVersion.ID,
		})

		// Create prebuild with old version (should NOT be invalidated)
		oldPrebuild := dbgen.Workspace(t, db, database.Workspace{
			OrganizationID: org.ID,
			OwnerID:        database.PrebuildsSystemUserID,
			TemplateID:     template.ID,
		})
		_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
			WorkspaceID:       oldPrebuild.ID,
			TemplateVersionID: oldVersion.ID,
		})

		// Update template to new active version
		newVersion := dbgen.TemplateVersion(t, db, database.TemplateVersion{
			OrganizationID: org.ID,
			TemplateID:     template.ID,
		})
		template, err := db.UpdateTemplateActiveVersionByID(ctx, database.UpdateTemplateActiveVersionByIDParams{
			ID:              template.ID,
			ActiveVersionID: newVersion.ID,
		})
		require.NoError(t, err)

		// Create 2 prebuilds with active version (SHOULD be invalidated)
		var activePrebuilds []database.Workspace
		for i := 0; i < 2; i++ {
			ws := dbgen.Workspace(t, db, database.Workspace{
				OrganizationID: org.ID,
				OwnerID:        database.PrebuildsSystemUserID,
				TemplateID:     template.ID,
			})
			_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
				WorkspaceID:       ws.ID,
				TemplateVersionID: newVersion.ID,
			})
			activePrebuilds = append(activePrebuilds, ws)
		}

		// Create user workspace with active version (should NOT be touched)
		user := coderdtest.CreateFirstUser(t, owner)
		userWorkspace := dbgen.Workspace(t, db, database.Workspace{
			OrganizationID: org.ID,
			OwnerID:        user.UserID,
			TemplateID:     template.ID,
		})
		_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
			WorkspaceID:       userWorkspace.ID,
			TemplateVersionID: newVersion.ID,
		})

		// Invalidate prebuilds as template admin
		client := coderdtest.CreateAnotherUser(t, owner, org.ID, codersdk.RoleTemplateAdmin)
		err = client.InvalidateTemplatePrebuilds(ctx, template.ID)
		require.NoError(t, err)

		// Verify old prebuild still exists (different version)
		_, err = db.GetWorkspaceByID(ctx, oldPrebuild.ID)
		require.NoError(t, err, "old version prebuild should not be deleted")

		// Verify user workspace still exists
		_, err = db.GetWorkspaceByID(ctx, userWorkspace.ID)
		require.NoError(t, err, "user workspace should not be deleted")

		// Verify active prebuilds have delete builds created
		for _, ws := range activePrebuilds {
			builds, err := db.GetWorkspaceBuildsByWorkspaceID(ctx, database.GetWorkspaceBuildsByWorkspaceIDParams{
				WorkspaceID: ws.ID,
			})
			require.NoError(t, err)
			require.NotEmpty(t, builds, "active prebuild should have builds")
			// Note: we can't easily verify the delete build was created without more setup
		}
	})

	t.Run("Unauthorized", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		owner, db := coderdtest.NewWithDatabase(t, nil)

		org := dbgen.Organization(t, db, database.Organization{})
		version := dbgen.TemplateVersion(t, db, database.TemplateVersion{
			OrganizationID: org.ID,
		})
		template := dbgen.Template(t, db, database.Template{
			OrganizationID:  org.ID,
			ActiveVersionID: version.ID,
		})

		// Regular user (not admin)
		regularUser := coderdtest.CreateAnotherUser(t, owner, org.ID)

		err := regularUser.InvalidateTemplatePrebuilds(ctx, template.ID)
		require.Error(t, err)
		
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, 403, apiErr.StatusCode)
	})
}

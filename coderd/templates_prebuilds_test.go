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

		// Create a template
		org := dbgen.Organization(t, db, database.Organization{})
		version := dbgen.TemplateVersion(t, db, database.TemplateVersion{
			OrganizationID: org.ID,
		})
		template := dbgen.Template(t, db, database.Template{
			OrganizationID:  org.ID,
			ActiveVersionID: version.ID,
		})

		// Create 2 prebuilt workspaces for this template version
		for i := 0; i < 2; i++ {
			_ = dbgen.Workspace(t, db, database.Workspace{
				OrganizationID:    org.ID,
				OwnerID:           database.PrebuildsSystemUserID,
				TemplateID:        template.ID,
				TemplateVersionID: version.ID,
			})
		}

		// Create a regular workspace (should not be touched)
		user := coderdtest.CreateFirstUser(t, owner)
		regularWorkspace := dbgen.Workspace(t, db, database.Workspace{
			OrganizationID:    org.ID,
			OwnerID:           user.UserID,
			TemplateID:        template.ID,
			TemplateVersionID: version.ID,
		})

		// Invalidate prebuilds as template admin
		client := coderdtest.CreateAnotherUser(t, owner, org.ID, codersdk.RoleTemplateAdmin)
		err := client.InvalidateTemplatePrebuilds(ctx, template.ID)
		require.NoError(t, err)

		// Verify regular workspace still exists
		_, err = db.GetWorkspaceByID(ctx, regularWorkspace.ID)
		require.NoError(t, err, "regular workspace should not be deleted")
	})

	t.Run("NoPrebuilds", func(t *testing.T) {
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

		client := coderdtest.CreateAnotherUser(t, owner, org.ID, codersdk.RoleTemplateAdmin)
		err := client.InvalidateTemplatePrebuilds(ctx, template.ID)
		require.NoError(t, err)
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

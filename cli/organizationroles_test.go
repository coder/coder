package cli_test

import (
	"bytes"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/testutil"
)

func TestShowOrganizationRoles(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		ownerClient, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{})
		owner := coderdtest.CreateFirstUser(t, ownerClient)
		client, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.RoleUserAdmin())

		const expectedRole = "test-role"
		dbgen.CustomRole(t, db, database.CustomRole{
			Name:            expectedRole,
			DisplayName:     "Expected",
			SitePermissions: nil,
			OrgPermissions:  nil,
			UserPermissions: nil,
			OrganizationID: uuid.NullUUID{
				UUID:  owner.OrganizationID,
				Valid: true,
			},
		})

		ctx := testutil.Context(t, testutil.WaitMedium)
		inv, root := clitest.New(t, "organization", "roles", "show")
		clitest.SetupConfig(t, client, root)

		buf := new(bytes.Buffer)
		inv.Stdout = buf
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)
		require.Contains(t, buf.String(), expectedRole)
	})
}

package rolestore_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmem"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/rolestore"
	"github.com/coder/coder/v2/testutil"
)

func TestExpandCustomRoleRoles(t *testing.T) {
	t.Parallel()

	db := dbmem.New()

	org := dbgen.Organization(t, db, database.Organization{})

	const roleName = "test-role"
	dbgen.CustomRole(t, db, database.CustomRole{
		Name:            roleName,
		DisplayName:     "",
		SitePermissions: nil,
		OrgPermissions:  nil,
		UserPermissions: nil,
		OrganizationID: uuid.NullUUID{
			UUID:  org.ID,
			Valid: true,
		},
	})

	ctx := testutil.Context(t, testutil.WaitShort)
	roles, err := rolestore.Expand(ctx, db, []string{rbac.RoleName(roleName, org.ID.String())})
	require.NoError(t, err)
	require.Len(t, roles, 1, "role found")
}

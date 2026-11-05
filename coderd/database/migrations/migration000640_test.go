package migrations_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/testutil"
)

// TestMigration000640ClearWorkspaceACLsWhenSharingDisallowed checks that the
// migration removes ACL entries left on workspaces whose organization does
// not allow sharing them, and keeps the others.
func TestMigration000640ClearWorkspaceACLsWhenSharingDisallowed(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	sqlDB := testSQLDB(t)
	stepTo(t, sqlDB, 639)
	ctx := testutil.Context(t, testutil.WaitLong)
	db := database.New(sqlDB)

	sharedUser := dbgen.User(t, db, database.User{})
	acl := database.WorkspaceACL{
		sharedUser.ID.String(): {Permissions: []policy.Action{policy.ActionRead}},
	}

	newWorkspace := func(sharing database.ShareableWorkspaceOwners, serviceAccount bool) database.WorkspaceTable {
		t.Helper()
		org := dbgen.Organization(t, db, database.Organization{})
		_, err := sqlDB.ExecContext(ctx,
			`UPDATE organizations SET shareable_workspace_owners = $1 WHERE id = $2`, sharing, org.ID)
		require.NoError(t, err)
		owner := dbgen.User(t, db, database.User{IsServiceAccount: serviceAccount})
		tpl := dbgen.Template(t, db, database.Template{OrganizationID: org.ID, CreatedBy: owner.ID})
		return dbgen.Workspace(t, db, database.WorkspaceTable{
			OrganizationID: org.ID,
			OwnerID:        owner.ID,
			TemplateID:     tpl.ID,
			UserACL:        acl,
			GroupACL:       database.WorkspaceACL{org.ID.String(): {Permissions: []policy.Action{policy.ActionRead}}},
		})
	}

	everyone := newWorkspace(database.ShareableWorkspaceOwnersEveryone, false)
	none := newWorkspace(database.ShareableWorkspaceOwnersNone, false)
	saModeRegular := newWorkspace(database.ShareableWorkspaceOwnersServiceAccounts, false)
	saModeServiceAccount := newWorkspace(database.ShareableWorkspaceOwnersServiceAccounts, true)

	stepTo(t, sqlDB, 640)

	requireACL := func(ws database.WorkspaceTable, wantShared bool) {
		t.Helper()
		got, err := db.GetWorkspaceACLByID(ctx, ws.ID)
		require.NoError(t, err)
		if wantShared {
			require.Len(t, got.Users, 1)
			require.Len(t, got.Groups, 1)
			return
		}
		require.Empty(t, got.Users)
		require.Empty(t, got.Groups)
	}
	requireACL(everyone, true)
	requireACL(none, false)
	requireACL(saModeRegular, false)
	requireACL(saModeServiceAccount, true)
}

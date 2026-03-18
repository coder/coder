package rolestore_test

import (
	"database/sql"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/rolestore"
	"github.com/coder/coder/v2/testutil"
)

func TestExpandCustomRoleRoles(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)

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
	roles, err := rolestore.Expand(ctx, db, []rbac.RoleIdentifier{{Name: roleName, OrganizationID: org.ID}})
	require.NoError(t, err)
	require.Len(t, roles, 1, "role found")
}

func TestReconcileSystemRole(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		roleName  string
		permsFunc func(rbac.OrgSettings) rbac.OrgRolePermissions
	}{
		{"OrgMember", rbac.RoleOrgMember(), rbac.OrgMemberPermissions},
		{"ServiceAccount", rbac.RoleOrgServiceAccount(), rbac.OrgServiceAccountPermissions},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			db, _ := dbtestutil.NewDB(t)
			org := dbgen.Organization(t, db, database.Organization{})
			ctx := testutil.Context(t, testutil.WaitShort)

			existing, err := database.ExpectOne(db.CustomRoles(ctx, database.CustomRolesParams{
				LookupRoles: []database.NameOrganizationPair{
					{
						Name:           tt.roleName,
						OrganizationID: org.ID,
					},
				},
				IncludeSystemRoles: true,
			}))
			require.NoError(t, err)

			// Zero out permissions to simulate stale state.
			_, err = db.UpdateCustomRole(ctx, database.UpdateCustomRoleParams{
				Name: existing.Name,
				OrganizationID: uuid.NullUUID{
					UUID:  org.ID,
					Valid: true,
				},
				DisplayName:       "",
				SitePermissions:   database.CustomRolePermissions{},
				UserPermissions:   database.CustomRolePermissions{},
				OrgPermissions:    database.CustomRolePermissions{},
				MemberPermissions: database.CustomRolePermissions{},
			})
			require.NoError(t, err)

			stale := existing
			stale.OrgPermissions = database.CustomRolePermissions{}
			stale.MemberPermissions = database.CustomRolePermissions{}

			reconciled, didUpdate, err := rolestore.ReconcileSystemRole(ctx, db, stale, org)
			require.NoError(t, err)
			require.True(t, didUpdate, "expected reconciliation to update stale permissions")

			dbstored, err := database.ExpectOne(db.CustomRoles(ctx, database.CustomRolesParams{
				LookupRoles: []database.NameOrganizationPair{
					{
						Name:           tt.roleName,
						OrganizationID: org.ID,
					},
				},
				IncludeSystemRoles: true,
			}))
			require.NoError(t, err)

			want := tt.permsFunc(rbac.OrgSettings{
				ShareableWorkspaceOwners: rbac.ShareableWorkspaceOwners(org.ShareableWorkspaceOwners),
			})
			require.True(t, rbac.PermissionsEqual(rolestore.ConvertDBPermissions(dbstored.OrgPermissions), want.Org))
			require.True(t, rbac.PermissionsEqual(rolestore.ConvertDBPermissions(dbstored.MemberPermissions), want.Member))
			require.True(t, rbac.PermissionsEqual(rolestore.ConvertDBPermissions(reconciled.OrgPermissions), want.Org))
			require.True(t, rbac.PermissionsEqual(rolestore.ConvertDBPermissions(reconciled.MemberPermissions), want.Member))

			_, didUpdate, err = rolestore.ReconcileSystemRole(ctx, db, reconciled, org)
			require.NoError(t, err)
			require.False(t, didUpdate, "expected no-op reconciliation when permissions are already current")
		})
	}
}

func TestReconcileSystemRoles(t *testing.T) {
	t.Parallel()

	var sqlDB *sql.DB
	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)

	// The DB trigger will create system roles for the org.
	org1 := dbgen.Organization(t, db, database.Organization{})
	org2 := dbgen.Organization(t, db, database.Organization{})

	ctx := testutil.Context(t, testutil.WaitShort)

	_, err := sqlDB.ExecContext(ctx, "UPDATE organizations SET shareable_workspace_owners = 'none' WHERE id = $1", org2.ID)
	require.NoError(t, err)

	// Simulate a missing system role by bypassing the application's
	// safety check in DeleteCustomRole (which prevents deleting
	// system roles).
	res, err := sqlDB.ExecContext(ctx,
		"DELETE FROM custom_roles WHERE name = lower($1) AND organization_id = $2",
		rbac.RoleOrgMember(),
		org1.ID,
	)
	require.NoError(t, err)
	affected, err := res.RowsAffected()
	require.NoError(t, err)
	require.Equal(t, int64(1), affected)

	// Not using testutil.Logger() here because it would fail on the
	// CRITICAL log line due to the deleted custom role.
	err = rolestore.ReconcileSystemRoles(ctx, slog.Make(), db)
	require.NoError(t, err)

	orgs, err := db.GetOrganizations(ctx, database.GetOrganizationsParams{})
	require.NoError(t, err)

	orgByID := make(map[uuid.UUID]database.Organization, len(orgs))
	for _, org := range orgs {
		orgByID[org.ID] = org
	}

	assertOrgMemberRole := func(t *testing.T, orgID uuid.UUID) {
		t.Helper()

		org := orgByID[orgID]
		got, err := database.ExpectOne(db.CustomRoles(ctx, database.CustomRolesParams{
			LookupRoles: []database.NameOrganizationPair{
				{
					Name:           rbac.RoleOrgMember(),
					OrganizationID: orgID,
				},
			},
			IncludeSystemRoles: true,
		}))
		require.NoError(t, err)
		require.True(t, got.IsSystem)

		want := rbac.OrgMemberPermissions(rbac.OrgSettings{ShareableWorkspaceOwners: rbac.ShareableWorkspaceOwners(org.ShareableWorkspaceOwners)})
		require.True(t, rbac.PermissionsEqual(rolestore.ConvertDBPermissions(got.OrgPermissions), want.Org))
		require.True(t, rbac.PermissionsEqual(rolestore.ConvertDBPermissions(got.MemberPermissions), want.Member))
	}

	assertOrgMemberRole(t, org1.ID)
	assertOrgMemberRole(t, org2.ID)
}

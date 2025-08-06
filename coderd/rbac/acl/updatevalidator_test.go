package acl_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/rbac/acl"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestOK(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	o := dbgen.Organization(t, db, database.Organization{})
	g := dbgen.Group(t, db, database.Group{OrganizationID: o.ID})
	u := dbgen.User(t, db, database.User{})
	ctx := testutil.Context(t, testutil.WaitShort)

	update := codersdk.UpdateWorkspaceACL{
		UserRoles: map[string]codersdk.WorkspaceRole{
			u.ID.String(): codersdk.WorkspaceRoleAdmin,
			// An unknown ID is allowed if and only if the specified role is either
			// codersdk.WorkspaceRoleDeleted or codersdk.TemplateRoleDeleted.
			uuid.NewString(): codersdk.WorkspaceRoleDeleted,
		},
		GroupRoles: map[string]codersdk.WorkspaceRole{
			g.ID.String(): codersdk.WorkspaceRoleAdmin,
			// An unknown ID is allowed if and only if the specified role is either
			// codersdk.WorkspaceRoleDeleted or codersdk.TemplateRoleDeleted.
			uuid.NewString(): codersdk.WorkspaceRoleDeleted,
		},
	}
	errors := acl.Validate(ctx, db, coderd.WorkspaceACLUpdateValidator(update))
	require.Empty(t, errors)
}

func TestDeniesUnknownIDs(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitShort)

	update := codersdk.UpdateWorkspaceACL{
		UserRoles: map[string]codersdk.WorkspaceRole{
			uuid.NewString(): codersdk.WorkspaceRoleAdmin,
		},
		GroupRoles: map[string]codersdk.WorkspaceRole{
			uuid.NewString(): codersdk.WorkspaceRoleAdmin,
		},
	}
	errors := acl.Validate(ctx, db, coderd.WorkspaceACLUpdateValidator(update))
	require.Len(t, errors, 2)
	require.Equal(t, errors[0].Field, "group_roles")
	require.ErrorContains(t, errors[0], "does not exist")
	require.Equal(t, errors[1].Field, "user_roles")
	require.ErrorContains(t, errors[1], "does not exist")
}

func TestDeniesUnknownRolesAndInvalidIDs(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitShort)

	update := codersdk.UpdateWorkspaceACL{
		UserRoles: map[string]codersdk.WorkspaceRole{
			"Quifrey": "level 5",
		},
		GroupRoles: map[string]codersdk.WorkspaceRole{
			"apprentices": "level 2",
		},
	}
	errors := acl.Validate(ctx, db, coderd.WorkspaceACLUpdateValidator(update))
	require.Len(t, errors, 4)
	require.Equal(t, errors[0].Field, "group_roles")
	require.ErrorContains(t, errors[0], "role \"level 2\" is not a valid workspace role")
	require.Equal(t, errors[1].Field, "group_roles")
	require.ErrorContains(t, errors[1], "not a valid UUID")
	require.Equal(t, errors[2].Field, "user_roles")
	require.ErrorContains(t, errors[2], "role \"level 5\" is not a valid workspace role")
	require.Equal(t, errors[3].Field, "user_roles")
	require.ErrorContains(t, errors[3], "not a valid UUID")
}

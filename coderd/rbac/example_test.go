package rbac_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/rbac"
)

// TestExample gives some examples on how to use the authz library.
// This serves to test syntax more than functionality.
func TestExample(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	authorizer, err := rbac.NewAuthorizer()
	require.NoError(t, err)
	defaultOrg := uuid.New()

	// user will become an authn object, and can even be a database.User if it
	// fulfills the interface. Until then, use a placeholder.
	user := subject{
		UserID: "alice",
		Roles: []rbac.Role{
			must(rbac.RoleByName(rbac.RoleMember())),
			must(rbac.RoleByName(rbac.RoleOrgAdmin(defaultOrg))),
		},
	}

	//nolint:paralleltest
	t.Run("ReadAllWorkspaces", func(t *testing.T) {
		// To read all workspaces on the site
		err := authorizer.Authorize(ctx, user.UserID, user.Roles, rbac.ActionRead, rbac.ResourceWorkspace.All())
		var _ = err
		require.Error(t, err, "this user cannot read all workspaces")
	})

	//nolint:paralleltest
	t.Run("ReadOrgWorkspaces", func(t *testing.T) {
		// To read all workspaces on the org 'default'
		err := authorizer.Authorize(ctx, user.UserID, user.Roles, rbac.ActionRead, rbac.ResourceWorkspace.InOrg(defaultOrg))
		require.NoError(t, err, "this user can read all org workspaces in 'default'")
	})

	//nolint:paralleltest
	t.Run("ReadMyWorkspace", func(t *testing.T) {
		// Note 'database.Workspace' could fulfill the object interface and be passed in directly
		err := authorizer.Authorize(ctx, user.UserID, user.Roles, rbac.ActionRead, rbac.ResourceWorkspace.InOrg(defaultOrg).WithOwner(user.UserID))
		require.NoError(t, err, "this user can their workspace")

		err = authorizer.Authorize(ctx, user.UserID, user.Roles, rbac.ActionRead, rbac.ResourceWorkspace.InOrg(defaultOrg).WithOwner(user.UserID).WithID("1234"))
		require.NoError(t, err, "this user can read workspace '1234'")
	})
}

func must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}

package authz_test

import (
	"context"
	"testing"

	"github.com/coder/coder/coderd/authz"
	"github.com/stretchr/testify/require"
)

// TestExample gives some examples on how to use the authz library.
// This serves to test syntax more than functionality.
func TestExample(t *testing.T) {
	t.Skip("TODO: unskip when rego is done")
	t.Parallel()
	ctx := context.Background()
	authorizer, err := authz.NewAuthorizer()
	require.NoError(t, err)

	// user will become an authn object, and can even be a database.User if it
	// fulfills the interface. Until then, use a placeholder.
	user := subject{
		UserID: "alice",
		Roles: []authz.Role{
			authz.RoleOrgAdmin("default"),
			authz.RoleMember,
		},
	}

	// TODO: Uncomment all assertions when implementation is done.

	//nolint:paralleltest
	t.Run("ReadAllWorkspaces", func(t *testing.T) {
		// To read all workspaces on the site
		err := authorizer.Authorize(ctx, user.UserID, user.Roles, authz.ResourceWorkspace.All(), authz.ActionRead)
		var _ = err
		// require.Error(t, err, "this user cannot read all workspaces")
	})

	//nolint:paralleltest
	t.Run("ReadOrgWorkspaces", func(t *testing.T) {
		// To read all workspaces on the org 'default'
		err := authorizer.Authorize(ctx, user.UserID, user.Roles, authz.ResourceWorkspace.InOrg("default"), authz.ActionRead)
		require.NoError(t, err, "this user can read all org workspaces in 'default'")
	})

	//nolint:paralleltest
	t.Run("ReadMyWorkspace", func(t *testing.T) {
		// Note 'database.Workspace' could fulfill the object interface and be passed in directly
		err := authorizer.Authorize(ctx, user.UserID, user.Roles, authz.ResourceWorkspace.InOrg("default").WithOwner(user.UserID), authz.ActionRead)
		require.NoError(t, err, "this user can their workspace")

		err = authorizer.Authorize(ctx, user.UserID, user.Roles, authz.ResourceWorkspace.InOrg("default").WithOwner(user.UserID).WithID("1234"), authz.ActionRead)
		require.NoError(t, err, "this user can read workspace '1234'")
	})
}

package authz_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/authz"
)

// TestExample gives some examples on how to use the authz library.
// This serves to test syntax more than functionality.
func TestExample(t *testing.T) {
	t.Skip("TODO: unskip when rego is done")
	t.Parallel()

	// user will become an authn object, and can even be a database.User if it
	// fulfills the interface. Until then, use a placeholder.
	user := authz.SubjectTODO{
		UserID: "alice",
		Roles: []authz.Role{
			authz.RoleOrgAdmin("default"),
			authz.RoleSiteMember,
		},
	}

	// TODO: Uncomment all assertions when implementation is done.

	//nolint:paralleltest
	t.Run("ReadAllWorkspaces", func(t *testing.T) {
		// To read all workspaces on the site
		err := authz.Authorize(user, authz.ResourceWorkspace, authz.ActionRead)
		var _ = err
		// require.Error(t, err, "this user cannot read all workspaces")
	})

	//nolint:paralleltest
	t.Run("ReadOrgWorkspaces", func(t *testing.T) {
		// To read all workspaces on the org 'default'
		err := authz.Authorize(user, authz.ResourceWorkspace.SetOrg("default"), authz.ActionRead)
		require.NoError(t, err, "this user can read all org workspaces in 'default'")
	})

	//nolint:paralleltest
	t.Run("ReadMyWorkspace", func(t *testing.T) {
		// Note 'database.Workspace' could fulfill the object interface and be passed in directly
		err := authz.Authorize(user, authz.ResourceWorkspace.SetOrg("default").SetOwner(user.UserID), authz.ActionRead)
		require.NoError(t, err, "this user can their workspace")

		err = authz.Authorize(user, authz.ResourceWorkspace.SetOrg("default").SetOwner(user.UserID).SetID("1234"), authz.ActionRead)
		require.NoError(t, err, "this user can read workspace '1234'")
	})

	//nolint:paralleltest
	t.Run("CreateNewSiteUser", func(t *testing.T) {
		err := authz.Authorize(user, authz.ResourceUser, authz.ActionCreate)
		var _ = err
		// require.Error(t, err, "this user cannot create new users")
	})
}

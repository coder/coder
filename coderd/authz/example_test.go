package authz_test

import (
	"testing"

	"github.com/coder/coder/coderd/authz"
	"github.com/stretchr/testify/require"
)

// Test_Example gives some examples on how to use the authz library.
// This serves to test syntax more than functionality.
func Test_Example(t *testing.T) {
	t.Parallel()

	// user will become an authn object, and can even be a database.User if it
	// fulfills the interface. Until then, use a placeholder.
	user := authz.SubjectTODO{
		UserID: "alice",
		// No site perms
		Site: []authz.Role{},
		Org: map[string][]authz.Role{
			// Admin of org "default".
			"default": {{Permissions: must(authz.ParsePermissions("+org.*.*.*"))}},
		},
		User: []authz.Role{
			// Site user role
			{Permissions: must(authz.ParsePermissions("+user.*.*.*"))},
		},
	}

	// TODO: Uncomment all assertions when implementation is done.

	t.Run("ReadAllWorkspaces", func(t *testing.T) {
		// To read all workspaces on the site
		err := authz.Authorize(user, authz.ResourceWorkspace, authz.ActionRead)
		var _ = err
		//require.Error(t, err, "this user cannot read all workspaces")
	})

	t.Run("ReadOrgWorkspaces", func(t *testing.T) {
		// To read all workspaces on the org 'default'
		err := authz.Authorize(user, authz.ResourceWorkspace.Org("default"), authz.ActionRead)
		require.NoError(t, err, "this user can read all org workspaces in 'default'")
	})

	t.Run("ReadMyWorkspace", func(t *testing.T) {
		// Note 'database.Workspace' could fulfill the object interface and be passed in directly
		err := authz.Authorize(user, authz.ResourceWorkspace.Org("default").Owner(user.UserID), authz.ActionRead)
		require.NoError(t, err, "this user can their workspace")

		err = authz.Authorize(user, authz.ResourceWorkspace.Org("default").Owner(user.UserID).AsID("1234"), authz.ActionRead)
		require.NoError(t, err, "this user can read workspace '1234'")
	})

	t.Run("CreateNewSiteUser", func(t *testing.T) {
		err := authz.Authorize(user, authz.ResourceUser, authz.ActionCreate)
		var _ = err
		//require.Error(t, err, "this user cannot create new users")
	})
}

func must[r any](v r, err error) r {
	if err != nil {
		panic(err)
	}
	return v
}

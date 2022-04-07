package authz_test

import (
	"testing"

	"github.com/coder/coder/coderd/authz/rbac"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/authz"
)

// TestExample gives some examples on how to use the authz library.
// This serves to test syntax more than functionality.
func TestExample(t *testing.T) {
	t.Parallel()

	// user will become an authn object, and can even be a database.User if it
	// fulfills the interface. Until then, use a placeholder.
	user := authz.SubjectTODO{
		UserID: "alice",
		// No site perms
		Site: []rbac.Role{authz.SiteMember},
		Org: map[string]rbac.Roles{
			// Admin of org "default".
			"default": {authz.OrganizationAdmin},
		},
	}

	// To read all workspaces on the site
	err := authz.Authorize(user, authz.Object{
		ObjectType: authz.Workspaces,
	}, authz.ReadAll)
	require.EqualError(t, err, authz.ErrUnauthorized.Error(), "this user cannot read all workspaces")
}

package coderd_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/testutil"
)

// TestTasksDisabled asserts that a deployment without CODER_ENABLE_AI_TASKS
// exposes no Tasks surface: the routes 404 and no built-in role grants a task
// permission.
//
//nolint:paralleltest // This test reloads the process-wide built-in roles.
func TestTasksDisabled(t *testing.T) {
	rbac.ReloadBuiltinRoles(nil)
	t.Cleanup(func() {
		rbac.ReloadBuiltinRoles(nil)
	})

	ctx := testutil.Context(t, testutil.WaitLong)
	values := coderdtest.DeploymentValues(t)
	values.EnableAITasks = false

	client := coderdtest.New(t, &coderdtest.Options{DeploymentValues: values})
	owner := coderdtest.CreateFirstUser(t, client)

	t.Run("RoutesNotFound", func(t *testing.T) {
		for _, route := range []string{
			"/api/v2/tasks",
			"/api/experimental/tasks",
		} {
			res, err := client.Request(ctx, http.MethodGet, route, nil)
			require.NoError(t, err)
			_ = res.Body.Close()
			require.Equal(t, http.StatusNotFound, res.StatusCode, "route %s should not be registered", route)
		}
	})

	t.Run("NoRoleGrantsTaskPermissions", func(t *testing.T) {
		authorizer := rbac.NewStrictCachingAuthorizer(nil)
		for _, role := range []rbac.RoleIdentifier{
			rbac.RoleOwner(),
			rbac.RoleMember(),
			rbac.ScopedRoleOrgAdmin(owner.OrganizationID),
		} {
			subject := rbac.Subject{
				ID:    owner.UserID.String(),
				Roles: rbac.RoleIdentifiers{rbac.RoleMember(), role},
				Scope: rbac.ScopeAll,
			}
			for _, action := range rbac.ResourceTask.AvailableActions() {
				err := authorizer.Authorize(context.Background(), subject, action, rbac.ResourceTask.WithOwner(owner.UserID.String()).InOrg(owner.OrganizationID))
				require.Error(t, err, "role %s should not be able to %s tasks", role, action)
				require.True(t, rbac.IsUnauthorizedError(err), "role %s action %s: %v", role, action, err)
			}
		}
	})

	// Sanity check that the deny permissions are scoped to tasks only.
	t.Run("OtherPermissionsUnaffected", func(t *testing.T) {
		authorizer := rbac.NewStrictCachingAuthorizer(nil)
		subject := rbac.Subject{
			ID:    owner.UserID.String(),
			Roles: rbac.RoleIdentifiers{rbac.RoleOwner()},
			Scope: rbac.ScopeAll,
		}
		require.NoError(t, authorizer.Authorize(context.Background(), subject, policy.ActionRead, rbac.ResourceWorkspace))
	})
}

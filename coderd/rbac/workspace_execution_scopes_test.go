package rbac_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
)

func TestWorkspaceExecutionPublicScopes(t *testing.T) {
	t.Parallel()
	auth := rbac.NewAuthorizer(prometheus.NewRegistry())
	user, org := uuid.New(), uuid.New()
	resource := rbac.ResourceWorkspaceExecution.WithID(uuid.New()).WithOwner(user.String()).InOrg(org)
	actions := []policy.Action{policy.ActionCreate, policy.ActionRead, policy.ActionSSH, policy.ActionUpdate}
	for _, action := range actions {
		t.Run(string(action), func(t *testing.T) {
			t.Parallel()
			name := rbac.ScopeName("workspace_execution:" + string(action))
			require.True(t, rbac.IsExternalScope(name))
			require.Contains(t, rbac.ExternalScopeNames(), string(name))
			subject := rbac.Subject{ID: user.String(), Roles: rbac.RoleIdentifiers{rbac.RoleMember(), rbac.ScopedRoleOrgWorkspaceAccess(org)}, Scope: name}
			for _, attempt := range actions {
				err := auth.Authorize(t.Context(), subject, attempt, resource)
				if attempt == action {
					require.NoError(t, err)
				} else {
					require.Error(t, err)
				}
			}
			// A narrow scope still intersects the user's own resource permissions.
			require.Error(t, auth.Authorize(t.Context(), subject, action, resource.WithOwner(uuid.NewString())))
			require.Error(t, auth.Authorize(t.Context(), subject, policy.ActionSSH, rbac.ResourceWorkspace.WithOwner(user.String()).InOrg(org)))
		})
	}
	require.False(t, rbac.IsExternalScope("workspace_execution:*"))
	require.False(t, rbac.IsExternalScope("workspace_execution:stop"))
	for _, composite := range []rbac.ScopeName{"coder:workspaces.access", "coder:workspaces.create", "coder:workspaces.operate", "coder:workspaces.delete"} {
		subject := rbac.Subject{ID: user.String(), Roles: rbac.RoleIdentifiers{rbac.RoleOwner()}, Scope: composite}
		for _, action := range actions {
			require.Error(t, auth.Authorize(t.Context(), subject, action, resource), "existing %s unexpectedly grants %s", composite, action)
		}
	}
}

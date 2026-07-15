package license

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
)

// TestWorkspaceCreateIgnoresGroups reinforces the assumption that lets
// CountWorkspaceCapableUsers deduplicate on roles alone and omit groups
// from the evaluation subject: group membership must not change the
// workspace-create outcome for the objects canCreateWorkspace uses.
// Those objects carry no ACLs, and groups only influence authorization
// through object ACL matching, so a subject with groups and one without
// must authorize identically.
//
// If this test fails, the policy has become group-sensitive for
// ACL-less workspace objects. Groups must then be added back to the
// evaluation subject, the dedupe signature, and the
// GetActiveUsersAuthorizationRoles query, or the cached verdicts will
// be shared across users with different authorization outcomes.
func TestWorkspaceCreateIgnoresGroups(t *testing.T) {
	t.Parallel()

	auth := rbac.NewStrictAuthorizer(prometheus.NewRegistry())
	orgID := uuid.New()
	userID := uuid.New()

	roleSets := map[string][]rbac.RoleIdentifier{
		"member only": {rbac.RoleMember()},
		"org member floor": {
			rbac.RoleMember(),
			rbac.ScopedRoleOrgMember(orgID),
		},
		"workspace access": {
			rbac.RoleMember(),
			rbac.ScopedRoleOrgMember(orgID),
			rbac.ScopedRoleOrgWorkspaceAccess(orgID),
		},
		"gateway access only": {
			rbac.RoleMember(),
			rbac.ScopedRoleOrgMember(orgID),
			rbac.ScopedRoleOrgAIGatewayAccess(orgID),
		},
		"creation ban": {
			rbac.RoleMember(),
			rbac.ScopedRoleOrgMember(orgID),
			rbac.ScopedRoleOrgWorkspaceAccess(orgID),
			rbac.ScopedRoleOrgWorkspaceCreationBan(orgID),
		},
		"org admin": {
			rbac.RoleMember(),
			rbac.ScopedRoleOrgAdmin(orgID),
		},
		"owner": {rbac.RoleMember(), rbac.RoleOwner()},
	}

	// The org ID doubles as the Everyone group ID, making it the
	// adversarial group membership for ACL-related rules.
	groups := []string{orgID.String(), uuid.NewString(), uuid.NewString()}

	// The same object shapes canCreateWorkspace evaluates: no ACLs.
	objects := map[string]rbac.Object{
		"any org": rbac.ResourceWorkspace.AnyOrganization().WithOwner(userID.String()),
		"in org":  rbac.ResourceWorkspace.InOrg(orgID).WithOwner(userID.String()),
	}

	for name, roleNames := range roleSets {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			base := rbac.Subject{
				Type:  rbac.SubjectTypeUser,
				ID:    userID.String(),
				Roles: rbac.RoleIdentifiers(roleNames),
				Scope: rbac.ScopeAll,
			}
			withGroups := base
			withGroups.Groups = groups

			for objName, obj := range objects {
				errWithout := auth.Authorize(context.Background(), base, policy.ActionCreate, obj)
				errWith := auth.Authorize(context.Background(), withGroups, policy.ActionCreate, obj)
				require.Equal(t,
					errWithout == nil, errWith == nil,
					"object %q: outcome must not depend on groups (without: %v, with: %v)",
					objName, errWithout, errWith,
				)
			}
		})
	}
}

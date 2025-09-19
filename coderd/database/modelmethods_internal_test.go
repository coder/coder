package database

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
)

func TestAPIKeyScopesExpand(t *testing.T) {
	t.Parallel()
	t.Run("builtins", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name   string
			scopes APIKeyScopes
			want   func(t *testing.T, s rbac.Scope)
		}{
			{
				name:   "all",
				scopes: APIKeyScopes{APIKeyScopeAll},
				want: func(t *testing.T, s rbac.Scope) {
					requirePermission(t, s, rbac.ResourceWildcard.Type, policy.Action(policy.WildcardSymbol))
					requireAllowAll(t, s)
				},
			},
			{
				name:   "application_connect",
				scopes: APIKeyScopes{APIKeyScopeApplicationConnect},
				want: func(t *testing.T, s rbac.Scope) {
					requirePermission(t, s, rbac.ResourceWorkspace.Type, policy.ActionApplicationConnect)
					requireAllowAll(t, s)
				},
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				s, err := tc.scopes.Expand()
				require.NoError(t, err)
				tc.want(t, s)
			})
		}
	})

	t.Run("low_level_pairs", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name   string
			scopes APIKeyScopes
			res    string
			act    policy.Action
		}{
			{name: "workspace:read", scopes: APIKeyScopes{ApiKeyScopeWorkspaceRead}, res: rbac.ResourceWorkspace.Type, act: policy.ActionRead},
			{name: "template:use", scopes: APIKeyScopes{ApiKeyScopeTemplateUse}, res: rbac.ResourceTemplate.Type, act: policy.ActionUse},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				s, err := tc.scopes.Expand()
				require.NoError(t, err)
				requirePermission(t, s, tc.res, tc.act)
				requireAllowAll(t, s)
			})
		}
	})

	t.Run("merge", func(t *testing.T) {
		t.Parallel()
		scopes := APIKeyScopes{APIKeyScopeApplicationConnect, APIKeyScopeAll, ApiKeyScopeWorkspaceRead}
		s, err := scopes.Expand()
		require.NoError(t, err)
		requirePermission(t, s, rbac.ResourceWildcard.Type, policy.Action(policy.WildcardSymbol))
		requirePermission(t, s, rbac.ResourceWorkspace.Type, policy.ActionApplicationConnect)
		requirePermission(t, s, rbac.ResourceWorkspace.Type, policy.ActionRead)
		requireAllowAll(t, s)
	})

	t.Run("empty_defaults_to_all", func(t *testing.T) {
		t.Parallel()
		s, err := (APIKeyScopes{}).Expand()
		require.NoError(t, err)
		requirePermission(t, s, rbac.ResourceWildcard.Type, policy.Action(policy.WildcardSymbol))
		requireAllowAll(t, s)
	})
}

// Helpers
func requirePermission(t *testing.T, s rbac.Scope, resource string, action policy.Action) {
	t.Helper()
	for _, p := range s.Site {
		if p.ResourceType == resource && p.Action == action {
			return
		}
	}
	t.Fatalf("permission not found: %s:%s", resource, action)
}

func requireAllowAll(t *testing.T, s rbac.Scope) {
	t.Helper()
	require.Len(t, s.AllowIDList, 1)
	require.Equal(t, policy.WildcardSymbol, s.AllowIDList[0].ID)
	require.Equal(t, policy.WildcardSymbol, s.AllowIDList[0].Type)
}

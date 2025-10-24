package database

import (
	"testing"

	"github.com/google/uuid"
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
				scopes: APIKeyScopes{ApiKeyScopeCoderAll},
				want: func(t *testing.T, s rbac.Scope) {
					requirePermission(t, s, rbac.ResourceWildcard.Type, policy.Action(policy.WildcardSymbol))
					requireAllowAll(t, s)
				},
			},
			{
				name:   "application_connect",
				scopes: APIKeyScopes{ApiKeyScopeCoderApplicationConnect},
				want: func(t *testing.T, s rbac.Scope) {
					requirePermission(t, s, rbac.ResourceWorkspace.Type, policy.ActionApplicationConnect)
					requireAllowAll(t, s)
				},
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				s, err := tc.scopes.expandRBACScope()
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
				s, err := tc.scopes.expandRBACScope()
				require.NoError(t, err)
				requirePermission(t, s, tc.res, tc.act)
				requireAllowAll(t, s)
			})
		}
	})

	t.Run("merge", func(t *testing.T) {
		t.Parallel()
		scopes := APIKeyScopes{ApiKeyScopeCoderApplicationConnect, ApiKeyScopeCoderAll, ApiKeyScopeWorkspaceRead}
		s, err := scopes.expandRBACScope()
		require.NoError(t, err)
		requirePermission(t, s, rbac.ResourceWildcard.Type, policy.Action(policy.WildcardSymbol))
		requirePermission(t, s, rbac.ResourceWorkspace.Type, policy.ActionApplicationConnect)
		requirePermission(t, s, rbac.ResourceWorkspace.Type, policy.ActionRead)
		requireAllowAll(t, s)
	})

	t.Run("effective_scope_keep_types", func(t *testing.T) {
		t.Parallel()
		workspaceID := uuid.New()

		effective := APIKeyScopeSet{
			Scopes: APIKeyScopes{ApiKeyScopeWorkspaceRead},
			AllowList: AllowList{
				{Type: rbac.ResourceWorkspace.Type, ID: workspaceID.String()},
			},
		}

		expanded, err := effective.Expand()
		require.NoError(t, err)
		require.Len(t, expanded.AllowIDList, 1)
		require.Equal(t, "workspace", expanded.AllowIDList[0].Type)
		require.Equal(t, workspaceID.String(), expanded.AllowIDList[0].ID)
	})

	t.Run("empty_rejected", func(t *testing.T) {
		t.Parallel()
		_, err := (APIKeyScopes{}).expandRBACScope()
		require.Error(t, err)
		require.ErrorContains(t, err, "no scopes provided")
	})

	t.Run("allow_list_overrides", func(t *testing.T) {
		t.Parallel()
		allowID := uuid.NewString()
		set := APIKeyScopes{ApiKeyScopeWorkspaceRead}.WithAllowList(AllowList{
			{Type: rbac.ResourceWorkspace.Type, ID: allowID},
		})
		s, err := set.Expand()
		require.NoError(t, err)
		require.Len(t, s.AllowIDList, 1)
		require.Equal(t, rbac.AllowListElement{Type: rbac.ResourceWorkspace.Type, ID: allowID}, s.AllowIDList[0])
	})

	t.Run("allow_list_wildcard_keeps_merged", func(t *testing.T) {
		t.Parallel()
		set := APIKeyScopes{ApiKeyScopeWorkspaceRead}.WithAllowList(AllowList{
			{Type: policy.WildcardSymbol, ID: policy.WildcardSymbol},
		})
		s, err := set.Expand()
		require.NoError(t, err)
		requirePermission(t, s, rbac.ResourceWorkspace.Type, policy.ActionRead)
		requireAllowAll(t, s)
	})

	t.Run("scope_set_helper", func(t *testing.T) {
		t.Parallel()
		allowID := uuid.NewString()
		key := APIKey{
			Scopes: APIKeyScopes{ApiKeyScopeWorkspaceRead},
			AllowList: AllowList{
				{Type: rbac.ResourceWorkspace.Type, ID: allowID},
			},
		}
		s, err := key.ScopeSet().Expand()
		require.NoError(t, err)
		require.Len(t, s.AllowIDList, 1)
		require.Equal(t, rbac.AllowListElement{Type: rbac.ResourceWorkspace.Type, ID: allowID}, s.AllowIDList[0])
	})
}

func TestAPIKeyScopesCompositeDefaults(t *testing.T) {
	t.Parallel()

	workspaceID := uuid.NewString()
	templateID := uuid.NewString()

	t.Run("workspace_create_adds_template_default", func(t *testing.T) {
		t.Parallel()
		// User creates token with workspace.create scope and only specifies workspace in allow list
		set := APIKeyScopes{"coder:workspaces.create"}.WithAllowList(AllowList{
			{Type: rbac.ResourceWorkspace.Type, ID: workspaceID},
		})

		expanded, err := set.Expand()
		require.NoError(t, err)

		// Should have both workspace (user-specified) and template (default)
		require.Len(t, expanded.AllowIDList, 2)
		require.ElementsMatch(t, []rbac.AllowListElement{
			{Type: rbac.ResourceTemplate.Type, ID: policy.WildcardSymbol},
			{Type: rbac.ResourceWorkspace.Type, ID: workspaceID},
		}, expanded.AllowIDList)
	})

	t.Run("workspace_create_empty_workspace_id", func(t *testing.T) {
		t.Parallel()
		// User creates token with empty workspace ID (for creation checks)
		set := APIKeyScopes{"coder:workspaces.create"}.WithAllowList(AllowList{
			{Type: rbac.ResourceWorkspace.Type, ID: ""},
		})

		expanded, err := set.Expand()
		require.NoError(t, err)

		// Should have both workspace with empty ID and template wildcard
		require.Len(t, expanded.AllowIDList, 2)
		require.ElementsMatch(t, []rbac.AllowListElement{
			{Type: rbac.ResourceTemplate.Type, ID: policy.WildcardSymbol},
			{Type: rbac.ResourceWorkspace.Type, ID: ""},
		}, expanded.AllowIDList)
	})

	t.Run("workspace_create_both_types_present_no_defaults", func(t *testing.T) {
		t.Parallel()
		// User specifies both workspace and template - no defaults added
		set := APIKeyScopes{"coder:workspaces.create"}.WithAllowList(AllowList{
			{Type: rbac.ResourceWorkspace.Type, ID: workspaceID},
			{Type: rbac.ResourceTemplate.Type, ID: templateID},
		})

		expanded, err := set.Expand()
		require.NoError(t, err)

		// Should keep user's specific template ID, not elevate to wildcard
		require.Len(t, expanded.AllowIDList, 2)
		require.ElementsMatch(t, []rbac.AllowListElement{
			{Type: rbac.ResourceTemplate.Type, ID: templateID},
			{Type: rbac.ResourceWorkspace.Type, ID: workspaceID},
		}, expanded.AllowIDList)
	})

	t.Run("workspace_operate_multiple_defaults", func(t *testing.T) {
		t.Parallel()
		// workspace.operate has defaults for template and user
		set := APIKeyScopes{"coder:workspaces.operate"}.WithAllowList(AllowList{
			{Type: rbac.ResourceWorkspace.Type, ID: workspaceID},
		})

		expanded, err := set.Expand()
		require.NoError(t, err)

		// Should add both template and user defaults
		require.Len(t, expanded.AllowIDList, 3)
		require.ElementsMatch(t, []rbac.AllowListElement{
			{Type: rbac.ResourceTemplate.Type, ID: policy.WildcardSymbol},
			{Type: rbac.ResourceUser.Type, ID: policy.WildcardSymbol},
			{Type: rbac.ResourceWorkspace.Type, ID: workspaceID},
		}, expanded.AllowIDList)
	})

	t.Run("multiple_composite_scopes_merge_defaults", func(t *testing.T) {
		t.Parallel()
		// Multiple composite scopes should merge their defaults
		set := APIKeyScopes{"coder:workspaces.create", "coder:workspaces.operate"}.WithAllowList(AllowList{
			{Type: rbac.ResourceWorkspace.Type, ID: workspaceID},
		})

		expanded, err := set.Expand()
		require.NoError(t, err)

		// create has [workspace:, template:*]
		// operate has [template:*, user:*]
		// merged defaults: [workspace:, template:*, user:*]
		// but workspace is already in allow list, so only add template and user
		require.Len(t, expanded.AllowIDList, 3)
		require.ElementsMatch(t, []rbac.AllowListElement{
			{Type: rbac.ResourceTemplate.Type, ID: policy.WildcardSymbol},
			{Type: rbac.ResourceUser.Type, ID: policy.WildcardSymbol},
			{Type: rbac.ResourceWorkspace.Type, ID: workspaceID},
		}, expanded.AllowIDList)
	})

	t.Run("wildcard_allow_list_skips_defaults", func(t *testing.T) {
		t.Parallel()
		// Global wildcard should not be modified by defaults
		set := APIKeyScopes{"coder:workspaces.create"}.WithAllowList(AllowList{
			{Type: policy.WildcardSymbol, ID: policy.WildcardSymbol},
		})

		expanded, err := set.Expand()
		require.NoError(t, err)

		// Should remain wildcard, no defaults added
		requireAllowAll(t, expanded)
	})

	t.Run("low_level_scopes_no_defaults", func(t *testing.T) {
		t.Parallel()
		// Low-level scopes don't have composite defaults
		set := APIKeyScopes{ApiKeyScopeWorkspaceRead}.WithAllowList(AllowList{
			{Type: rbac.ResourceWorkspace.Type, ID: workspaceID},
		})

		expanded, err := set.Expand()
		require.NoError(t, err)

		// No defaults, just the user-specified allow list
		require.Len(t, expanded.AllowIDList, 1)
		require.Equal(t, rbac.AllowListElement{
			Type: rbac.ResourceWorkspace.Type,
			ID:   workspaceID,
		}, expanded.AllowIDList[0])
	})

	t.Run("builtin_scopes_no_defaults", func(t *testing.T) {
		t.Parallel()
		// Built-in scopes like coder:all don't have specific defaults
		set := APIKeyScopes{ApiKeyScopeCoderAll}.WithAllowList(AllowList{
			{Type: rbac.ResourceWorkspace.Type, ID: workspaceID},
		})

		expanded, err := set.Expand()
		require.NoError(t, err)

		// No defaults, just the user-specified allow list
		require.Len(t, expanded.AllowIDList, 1)
		require.Equal(t, rbac.AllowListElement{
			Type: rbac.ResourceWorkspace.Type,
			ID:   workspaceID,
		}, expanded.AllowIDList[0])
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

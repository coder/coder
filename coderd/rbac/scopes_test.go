package rbac_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
)

func TestExpandScope(t *testing.T) {
	t.Parallel()

	t.Run("low_level_pairs", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name     string
			resource string
			action   policy.Action
		}{
			{name: "workspace:start", resource: rbac.ResourceWorkspace.Type, action: policy.ActionWorkspaceStart},
			{name: "workspace:ssh", resource: rbac.ResourceWorkspace.Type, action: policy.ActionSSH},
			{name: "template:use", resource: rbac.ResourceTemplate.Type, action: policy.ActionUse},
			{name: "api_key:read", resource: rbac.ResourceApiKey.Type, action: policy.ActionRead},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				s, err := rbac.ScopeName(tc.name).Expand()
				require.NoError(t, err)

				// site-only single permission
				require.Len(t, s.Site, 1)
				require.Equal(t, tc.resource, s.Site[0].ResourceType)
				require.Equal(t, tc.action, s.Site[0].Action)
				require.Empty(t, s.ByOrgID)
				require.Empty(t, s.User)

				require.Equal(t, []rbac.AllowListElement{rbac.AllowListAll()}, s.AllowIDList)
			})
		}
	})

	t.Run("invalid_low_level", func(t *testing.T) {
		t.Parallel()
		invalid := []string{
			"",                // empty
			"workspace:",      // missing action
			":read",           // missing resource
			"unknown:read",    // unknown resource
			"workspace:bogus", // unknown action
			"a:b:c",           // too many parts
		}
		for _, name := range invalid {
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				_, err := rbac.ScopeName(name).Expand()
				require.Error(t, err)
			})
		}
	})
}

func TestFirstScopeNotCovered(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		allowed   []rbac.ScopeName
		requested []rbac.ScopeName
		want      rbac.ScopeName
		// wantErrContains names the side that could not be expanded.
		wantErrContains string
	}{
		{
			name:      "EmptyRequestIsCovered",
			allowed:   []rbac.ScopeName{"workspace:read"},
			requested: nil,
		},
		{
			name:      "EveryRequestedScopeCovered",
			allowed:   []rbac.ScopeName{"coder:workspaces.access"},
			requested: []rbac.ScopeName{"workspace:read", "workspace:ssh"},
		},
		{
			// The answer names which scope failed, not merely that one did.
			name:      "NamesTheFirstUncovered",
			allowed:   []rbac.ScopeName{"coder:workspaces.access"},
			requested: []rbac.ScopeName{"workspace:read", "workspace:delete", "user_secret:delete"},
			want:      "workspace:delete",
		},
		{
			name:            "UnexpandableRequestedScopeNamed",
			allowed:         []rbac.ScopeName{"coder:workspaces.access"},
			requested:       []rbac.ScopeName{"workspace:read", "not_a_real_scope"},
			want:            "not_a_real_scope",
			wantErrContains: "expand requested scope",
		},
		{
			name:            "UnexpandableAllowedScopeNamed",
			allowed:         []rbac.ScopeName{"not_a_real_scope"},
			requested:       []rbac.ScopeName{"workspace:read"},
			want:            "not_a_real_scope",
			wantErrContains: "expand allowed scope",
		},
		{
			// The alias validates but is not an expandable name, so callers
			// must canonicalize first.
			name:            "LegacyAliasIsNotExpandable",
			allowed:         []rbac.ScopeName{"all"},
			requested:       []rbac.ScopeName{"workspace:read"},
			want:            "all",
			wantErrContains: "expand allowed scope",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := rbac.FirstScopeNotCovered(test.allowed, test.requested)
			if test.wantErrContains != "" {
				require.ErrorContains(t, err, test.wantErrContains)
				require.Equal(t, test.want, got, "an undecidable comparison must name the scope it could not decide")
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.want, got)
		})
	}
}

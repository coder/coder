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

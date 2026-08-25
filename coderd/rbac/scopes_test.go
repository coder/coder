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

func TestScopesCover(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		allowed   []rbac.ScopeName
		requested rbac.ScopeName
		want      bool
		// wantErrContains names the side that could not be expanded. Asserting
		// the side keeps rows that fail through different paths from passing
		// on each other's errors.
		wantErrContains string
	}{
		{
			name:      "IdenticalName",
			allowed:   []rbac.ScopeName{"workspace:read"},
			requested: "workspace:read",
			want:      true,
		},
		{
			// Name matching alone cannot decide this: coder:workspaces.access
			// never mentions workspace:ssh by name, but its expansion covers it.
			name:      "CompositeCoversItsMember",
			allowed:   []rbac.ScopeName{"coder:workspaces.access"},
			requested: "workspace:ssh",
			want:      true,
		},
		{
			// The composite grants no permission on this resource at all.
			name:      "CompositeDoesNotCoverUnrelatedResource",
			allowed:   []rbac.ScopeName{"coder:workspaces.access"},
			requested: "user_secret:delete",
			want:      false,
		},
		{
			// Same resource, action the composite does not grant. Coverage
			// compares the pair, not the resource alone.
			name:      "CompositeDoesNotCoverUngrantedActionOnSameResource",
			allowed:   []rbac.ScopeName{"coder:workspaces.access"},
			requested: "workspace:delete",
			want:      false,
		},
		{
			name:      "AllCoversEverything",
			allowed:   []rbac.ScopeName{rbac.ScopeAll},
			requested: "user_secret:delete",
			want:      true,
		},
		{
			name:      "NarrowScopeDoesNotCoverAll",
			allowed:   []rbac.ScopeName{"workspace:read"},
			requested: rbac.ScopeAll,
			want:      false,
		},
		{
			name:      "ResourceWildcardCoversOneAction",
			allowed:   []rbac.ScopeName{"workspace:*"},
			requested: "workspace:ssh",
			want:      true,
		},
		{
			name:      "OneActionDoesNotCoverResourceWildcard",
			allowed:   []rbac.ScopeName{"workspace:ssh"},
			requested: "workspace:*",
			want:      false,
		},
		{
			// A composite is covered only when every permission it expands
			// to is granted, so a strict subset of them is not enough.
			name:      "PartialUnionDoesNotCoverComposite",
			allowed:   []rbac.ScopeName{"template:read", "file:create"},
			requested: "coder:templates.build",
			want:      false,
		},
		{
			// The allowed side is a union rather than a set of independent
			// candidates, so one composite's permissions may be drawn from
			// several allowed entries at once.
			name:      "UnionOfAllowedScopesCoversComposite",
			allowed:   []rbac.ScopeName{"template:read", "file:*", "provisioner_jobs:read"},
			requested: "coder:templates.build",
			want:      true,
		},
		{
			name:      "EmptyAllowedCoversNothing",
			allowed:   nil,
			requested: "workspace:read",
			want:      false,
		},
		{
			// Not a false: a caller cannot distinguish "known and not
			// covered" from "we could not tell", so an undecidable
			// comparison is surfaced rather than answered.
			name:            "UnknownRequestedScopeErrors",
			allowed:         []rbac.ScopeName{rbac.ScopeAll},
			requested:       "not_a_real_scope",
			wantErrContains: "expand requested scope",
		},
		{
			name:            "UnknownAllowedScopeErrors",
			allowed:         []rbac.ScopeName{"not_a_real_scope"},
			requested:       "workspace:read",
			wantErrContains: "expand allowed scope",
		},
		{
			// The bad name sits behind an entry that already covers the
			// request, which an implementation answering on the first match
			// never reaches.
			name:            "UnknownAllowedScopeErrorsBesideCoveringScope",
			allowed:         []rbac.ScopeName{rbac.ScopeAll, "not_a_real_scope"},
			requested:       "workspace:read",
			wantErrContains: "expand allowed scope",
		},
		{
			// The aliases IsExternalScope accepts are not expandable names,
			// so callers must canonicalize before asking about coverage.
			name:            "NonCanonicalAliasErrorsRequestedAll",
			allowed:         []rbac.ScopeName{rbac.ScopeAll},
			requested:       "all",
			wantErrContains: "expand requested scope",
		},
		{
			name:            "NonCanonicalAliasErrorsRequestedApplicationConnect",
			allowed:         []rbac.ScopeName{rbac.ScopeAll},
			requested:       "application_connect",
			wantErrContains: "expand requested scope",
		},
		{
			// The only row that would catch someone canonicalizing inside the
			// allowed loop, which would silently widen what an allow list
			// accepts without any caller asking for it.
			name:            "NonCanonicalAliasErrorsAllowedApplicationConnect",
			allowed:         []rbac.ScopeName{"application_connect"},
			requested:       rbac.ScopeApplicationConnect,
			wantErrContains: "expand allowed scope",
		},
		{
			name:            "NonCanonicalAliasErrorsAllowedAll",
			allowed:         []rbac.ScopeName{"all"},
			requested:       rbac.ScopeAll,
			wantErrContains: "expand allowed scope",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := rbac.ScopesCover(test.allowed, test.requested)
			if test.wantErrContains != "" {
				require.ErrorContains(t, err, test.wantErrContains)
				require.False(t, got, "an undecided comparison must not report coverage")
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.want, got)
		})
	}
}

// TestCanonicalScopeName pins the alias mapping. Without it the two case arms
// could be swapped, so that requesting `all` persisted application_connect and
// the reverse, and the suite would stay green.
func TestCanonicalScopeName(t *testing.T) {
	t.Parallel()

	require.Equal(t, rbac.ScopeAll, rbac.CanonicalScopeName("all"))
	require.Equal(t, rbac.ScopeApplicationConnect, rbac.CanonicalScopeName("application_connect"))

	// Canonical names and low-level names are returned unchanged, so callers
	// can canonicalize unconditionally.
	require.Equal(t, rbac.ScopeAll, rbac.CanonicalScopeName(rbac.ScopeAll))
	require.Equal(t, rbac.ScopeApplicationConnect, rbac.CanonicalScopeName(rbac.ScopeApplicationConnect))
	require.Equal(t, rbac.ScopeName("workspace:read"), rbac.CanonicalScopeName("workspace:read"))
	require.Equal(t, rbac.ScopeName("not_a_real_scope"), rbac.CanonicalScopeName("not_a_real_scope"))
}

// TestScopesCoverEveryExternalScope asserts the property the OAuth2 allowlist
// check depends on: coder:all is a ceiling over the whole external catalog, and
// every catalog name covers itself. A name that cannot be compared at all would
// otherwise reject every request naming it, which is a rejection no app owner
// could act on.
func TestScopesCoverEveryExternalScope(t *testing.T) {
	t.Parallel()

	// The aliases inherit this without being named here: TestScopeAliases pins
	// that each one canonicalizes to a name this list offers.
	for _, name := range rbac.ExternalScopeNames() {
		scope := rbac.ScopeName(name)

		covered, err := rbac.ScopesCover([]rbac.ScopeName{rbac.ScopeAll}, scope)
		require.NoErrorf(t, err, "coder:all vs %q", scope)
		require.Truef(t, covered, "coder:all must cover %q", scope)

		covered, err = rbac.ScopesCover([]rbac.ScopeName{scope}, scope)
		require.NoErrorf(t, err, "%q vs itself", scope)
		require.Truef(t, covered, "%q must cover itself", scope)
	}
}

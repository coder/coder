package rbac

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/rbac/policy"
)

var (
	workspaceRead         = Permission{ResourceType: "workspace", Action: policy.ActionRead}
	workspaceDelete       = Permission{ResourceType: "workspace", Action: policy.ActionDelete}
	workspaceWildcard     = Permission{ResourceType: "workspace", Action: policy.WildcardSymbol}
	workspaceDeleteNegate = Permission{ResourceType: "workspace", Action: policy.ActionDelete, Negate: true}
	wildcardResourceRead  = Permission{ResourceType: policy.WildcardSymbol, Action: policy.ActionRead}
)

// coverableScope is the shape every ExpandScope result has: site permissions
// only, wildcard allow list, no negatives.
func coverableScope(perms ...Permission) Scope {
	return Scope{
		Role:        Role{Site: perms},
		AllowIDList: []AllowListElement{AllowListAll()},
	}
}

// TestScopeAliases asserts what the shared table exists to guarantee, which no
// test outside this package can: every alias is public, resolves to a public
// name, and resolves to one the RBAC layer can expand. A name IsExternalScope
// calls public but ExpandScope rejects is requestable in name only, and every
// request naming it fails. Iterating the table rather than naming the two
// aliases means a third is covered the day it is added.
func TestScopeAliases(t *testing.T) {
	t.Parallel()

	require.NotEmpty(t, scopeAliases)

	for alias, canonical := range scopeAliases {
		require.Truef(t, IsExternalScope(alias), "alias %q must be public", alias)
		require.Equalf(t, canonical, CanonicalScopeName(alias), "alias %q", alias)

		// An alias is a second spelling of a scope, not a scope of its own.
		require.Truef(t, IsExternalScope(canonical), "canonical %q must be public", canonical)
		_, err := ExpandScope(canonical)
		require.NoErrorf(t, err, "canonical %q must expand", canonical)

		// The alias itself does not expand, which is what makes
		// canonicalization mandatory before storage or coverage rather than a
		// tidying step callers may skip.
		_, err = ExpandScope(alias)
		require.Errorf(t, err, "alias %q must not expand directly", alias)

		// The list a client reads offers the canonical spelling and only that
		// one, so a caller can request a name from it and store what it
		// requested. Listing the alias too would offer two names for one scope,
		// one of which fails to expand once stored.
		require.NotContainsf(t, ExternalScopeNames(), string(alias), "list must omit alias %q", alias)
		require.Containsf(t, ExternalScopeNames(), string(canonical), "list must offer %q", canonical)
	}
}

// TestScopesCoverGuards drives Scope values that no catalog entry produces.
// The guards exist for authority ScopeName inputs cannot express today, so
// ScopesCover cannot reach them and they would otherwise ship unverified.
//
// Each shape runs on both sides of the comparison, with the opposite side
// coverable, so a guard consulted on only one side fails here.
func TestScopesCoverGuards(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		scope   Scope
		wantErr string
	}{
		{
			name:  "SitePermissionsOnly",
			scope: coverableScope(workspaceRead),
		},
		{
			name: "UserPermission",
			scope: Scope{
				Role: Role{
					Site: []Permission{workspaceRead},
					User: []Permission{workspaceRead},
				},
				AllowIDList: []AllowListElement{AllowListAll()},
			},
			wantErr: "grants org or user permissions",
		},
		{
			// The case the guards were added for: a scope granting every
			// workspace action except delete. The permission coverage reads is
			// harmless, and the one it does not read carves delete back out, so
			// comparing on Site alone would answer a request for
			// workspace:delete from a wildcard the scope has already qualified.
			name: "NegativeUserPermission",
			scope: Scope{
				Role: Role{
					Site: []Permission{workspaceWildcard},
					User: []Permission{workspaceDeleteNegate},
				},
				AllowIDList: []AllowListElement{AllowListAll()},
			},
			wantErr: "grants org or user permissions",
		},
		{
			name: "OrgPermission",
			scope: Scope{
				Role: Role{
					Site:    []Permission{workspaceRead},
					ByOrgID: map[string]OrgPermissions{"00000000-0000-0000-0000-000000000001": {}},
				},
				AllowIDList: []AllowListElement{AllowListAll()},
			},
			wantErr: "grants org or user permissions",
		},
		{
			name:    "NegativeSitePermission",
			scope:   coverableScope(workspaceWildcard, workspaceDeleteNegate),
			wantErr: "carries a negative permission",
		},
		{
			name: "NarrowedAllowList",
			scope: Scope{
				Role:        Role{Site: []Permission{workspaceRead}},
				AllowIDList: []AllowListElement{{Type: "workspace", ID: "00000000-0000-0000-0000-000000000002"}},
			},
			wantErr: "carries a resource allow list",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// The opposite side always covers, so a guard error stays
			// distinguishable from an ordinary uncovered result.
			cleanGrant := namedScope{name: "clean_scope", scope: coverableScope(workspaceWildcard)}
			cleanRequest := namedScope{name: "clean_scope", scope: coverableScope(workspaceRead)}
			under := namedScope{name: "test_scope", scope: test.scope}

			sides := []struct {
				side      string
				allowed   []namedScope
				requested namedScope
			}{
				{side: coverageSideRequested, allowed: []namedScope{cleanGrant}, requested: under},
				{side: coverageSideAllowed, allowed: []namedScope{under}, requested: cleanRequest},
			}

			for _, args := range sides {
				side := args.side
				got, err := scopesCoverExpanded(args.allowed, args.requested)
				if test.wantErr == "" {
					require.NoErrorf(t, err, "side %q", side)
					require.Truef(t, got, "side %q", side)
					continue
				}
				require.ErrorContainsf(t, err, test.wantErr, "side %q", side)
				// The side names itself, so an operator reading the error can
				// tell which half of the comparison was undecidable.
				require.ErrorContainsf(t, err, side+` scope "test_scope"`, "side %q", side)
				require.Falsef(t, got, "an undecided comparison must not report coverage, side %q", side)
			}
		})
	}
}

// TestScopesCoverWildcardResourceChecksAction pins that a wildcard resource
// type is not on its own a grant: {*, read} authorizes read on every resource,
// not every action on every resource. No ScopeName expands to that shape, since
// the only wildcard resource the catalog spells is coder:all's {*, *}, so
// permissionCovered could drop its action comparison and every ScopesCover test
// would stay green.
func TestScopesCoverWildcardResourceChecksAction(t *testing.T) {
	t.Parallel()

	allowed := []namedScope{{name: "wildcard_read", scope: coverableScope(wildcardResourceRead)}}

	// The wildcard resource does match an unrelated resource, so the assertion
	// below fails on the action and not on resource matching.
	covered, err := scopesCoverExpanded(allowed, namedScope{name: "workspace_read", scope: coverableScope(workspaceRead)})
	require.NoError(t, err)
	require.True(t, covered)

	covered, err = scopesCoverExpanded(allowed, namedScope{name: "workspace_delete", scope: coverableScope(workspaceDelete)})
	require.NoError(t, err)
	require.False(t, covered, "read on every resource must not cover delete")

	// The mirror: a grant on one resource cannot cover a request for read on
	// every resource. ScopeName inputs reach the action wildcard on the
	// requested side but never the resource wildcard.
	covered, err = scopesCoverExpanded(
		[]namedScope{{name: "workspace_read", scope: coverableScope(workspaceRead)}},
		namedScope{name: "wildcard_read", scope: coverableScope(wildcardResourceRead)},
	)
	require.NoError(t, err)
	require.False(t, covered, "read on one resource must not cover read on every resource")
}

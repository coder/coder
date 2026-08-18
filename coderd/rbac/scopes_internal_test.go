package rbac

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/rbac/policy"
)

var (
	siteRead     = Permission{ResourceType: "workspace", Action: policy.ActionRead}
	siteWildcard = Permission{ResourceType: "workspace", Action: policy.WildcardSymbol}
	siteDeleteNo = Permission{ResourceType: "workspace", Action: policy.ActionDelete, Negate: true}
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
			scope: coverableScope(siteRead),
		},
		{
			name: "UserPermission",
			scope: Scope{
				Role: Role{
					Site: []Permission{siteRead},
					User: []Permission{siteRead},
				},
				AllowIDList: []AllowListElement{AllowListAll()},
			},
			wantErr: "grants org or user permissions",
		},
		{
			// The permission coverage reads is harmless. The one it does not
			// read carves an action back out, so comparing on Site alone would
			// report authority the scope withholds.
			name: "NegativeUserPermission",
			scope: Scope{
				Role: Role{
					Site: []Permission{siteWildcard},
					User: []Permission{siteDeleteNo},
				},
				AllowIDList: []AllowListElement{AllowListAll()},
			},
			wantErr: "grants org or user permissions",
		},
		{
			name: "OrgPermission",
			scope: Scope{
				Role: Role{
					Site:    []Permission{siteRead},
					ByOrgID: map[string]OrgPermissions{"00000000-0000-0000-0000-000000000001": {}},
				},
				AllowIDList: []AllowListElement{AllowListAll()},
			},
			wantErr: "grants org or user permissions",
		},
		{
			name:    "NegativeSitePermission",
			scope:   coverableScope(siteWildcard, siteDeleteNo),
			wantErr: "carries a negative permission",
		},
		{
			name: "NarrowedAllowList",
			scope: Scope{
				Role:        Role{Site: []Permission{siteRead}},
				AllowIDList: []AllowListElement{{Type: "workspace", ID: "00000000-0000-0000-0000-000000000002"}},
			},
			wantErr: "carries a resource allow list",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// The opposite side is chosen so that a coverable scope under test
			// reaches the comparison and answers true: a wildcard grant covers
			// any request, and a workspace:read request is covered by any
			// grant here. That keeps a guard error distinguishable from an
			// ordinary uncovered result.
			cleanGrant := namedScope{name: "clean_scope", scope: coverableScope(siteWildcard)}
			cleanRequest := namedScope{name: "clean_scope", scope: coverableScope(siteRead)}
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

// TestScopesCoverAllowedNegativeDoesNotWiden is the case the guards were added
// for. An allowed scope granting every workspace action except delete must not
// answer a request for delete. Reading its Site permissions alone would, since
// the wildcard matches and the anti-grant sits in a field coverage never reads.
func TestScopesCoverAllowedNegativeDoesNotWiden(t *testing.T) {
	t.Parallel()

	everythingExceptDelete := namedScope{
		name: "workspace_except_delete",
		scope: Scope{
			Role: Role{
				Site: []Permission{siteWildcard},
				User: []Permission{siteDeleteNo},
			},
			AllowIDList: []AllowListElement{AllowListAll()},
		},
	}
	wantDelete := namedScope{
		name:  "workspace:delete",
		scope: coverableScope(Permission{ResourceType: "workspace", Action: policy.ActionDelete}),
	}

	got, err := scopesCoverExpanded([]namedScope{everythingExceptDelete}, wantDelete)
	require.Error(t, err)
	require.False(t, got)
}

package rbac

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/rbac/policy"
)

// TestCheckCoverable drives Scope values that no catalog entry produces. The
// guards exist for authority ScopeName inputs cannot express today, so the
// public function cannot reach them and they would otherwise ship unverified.
func TestCheckCoverable(t *testing.T) {
	t.Parallel()

	siteRead := Permission{ResourceType: "workspace", Action: policy.ActionRead}

	tests := []struct {
		name    string
		scope   Scope
		wantErr string
	}{
		{
			name: "SitePermissionsOnly",
			scope: Scope{
				Role:        Role{Site: []Permission{siteRead}},
				AllowIDList: []AllowListElement{AllowListAll()},
			},
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
					Site: []Permission{{ResourceType: "workspace", Action: policy.WildcardSymbol}},
					User: []Permission{{ResourceType: "workspace", Action: policy.ActionDelete, Negate: true}},
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
			name: "NegativeSitePermission",
			scope: Scope{
				Role: Role{Site: []Permission{
					{ResourceType: "workspace", Action: policy.WildcardSymbol},
					{ResourceType: "workspace", Action: policy.ActionDelete, Negate: true},
				}},
				AllowIDList: []AllowListElement{AllowListAll()},
			},
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

			for _, side := range []string{coverageSideRequested, coverageSideAllowed} {
				err := checkCoverable(test.scope, side, "test_scope")
				if test.wantErr == "" {
					require.NoErrorf(t, err, "side %q", side)
					continue
				}
				require.ErrorContainsf(t, err, test.wantErr, "side %q", side)
				// The side names itself, so an operator reading the error can
				// tell which half of the comparison was undecidable.
				require.ErrorContainsf(t, err, side+` scope "test_scope"`, "side %q", side)
			}
		})
	}
}

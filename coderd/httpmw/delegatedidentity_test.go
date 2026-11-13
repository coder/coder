package httpmw_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
)

func TestDelegatedIdentityScopes(t *testing.T) {
	t.Parallel()

	identity := func(scopes ...string) database.DelegatedIdentity {
		return database.DelegatedIdentity{Scopes: scopes}
	}
	tests := []struct {
		name  string
		chain []database.DelegatedIdentity
		want  []rbac.ScopeName
	}{
		{name: "NotAnIdentity", chain: nil, want: nil},
		{name: "NoScopes", chain: []database.DelegatedIdentity{identity()}, want: []rbac.ScopeName{}},
		{
			name:  "UnderAUser",
			chain: []database.DelegatedIdentity{identity("workspace:read", "template:read")},
			want:  []rbac.ScopeName{"workspace:read", "template:read"},
		},
		{
			name: "ParentLacksOne",
			chain: []database.DelegatedIdentity{
				identity("workspace:read", "template:read"),
				identity("workspace:read"),
			},
			want: []rbac.ScopeName{"workspace:read"},
		},
		{
			name: "ParentHoldsAWildcard",
			chain: []database.DelegatedIdentity{
				identity("workspace:read", "workspace:delete", "template:read"),
				identity("workspace:*"),
			},
			want: []rbac.ScopeName{"workspace:read", "workspace:delete"},
		},
		{
			name: "AWildcardNeedsAWildcard",
			chain: []database.DelegatedIdentity{
				identity("workspace:*"),
				identity("workspace:read", "workspace:delete"),
			},
			want: []rbac.ScopeName{},
		},
		{
			name: "GrandparentLacksAll",
			chain: []database.DelegatedIdentity{
				identity("workspace:read"),
				identity("workspace:read", "template:read"),
				identity(),
			},
			want: []rbac.ScopeName{},
		},
		{
			name: "ParentHoldsAll",
			chain: []database.DelegatedIdentity{
				identity("workspace:read"),
				identity("all"),
			},
			want: []rbac.ScopeName{"workspace:read"},
		},
		{
			name: "UnknownIsNotHeld",
			chain: []database.DelegatedIdentity{
				identity("workspace:read", "not:a-scope"),
				identity("all"),
			},
			want: []rbac.ScopeName{"workspace:read"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, httpmw.DelegatedIdentityScopes(tt.chain))
		})
	}
}

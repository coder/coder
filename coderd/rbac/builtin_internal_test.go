package rbac

import (
	"testing"

	"github.com/google/uuid"
	"github.com/open-policy-agent/opa/ast"
	"github.com/stretchr/testify/require"
)

func BenchmarkRBACValueAllocation(b *testing.B) {
	actor := Subject{
		Roles:  RoleNames{RoleOrgMember(uuid.New()), RoleOrgAdmin(uuid.New()), RoleMember()},
		ID:     uuid.NewString(),
		Scope:  ScopeAll,
		Groups: []string{uuid.NewString(), uuid.NewString(), uuid.NewString()},
	}
	obj := ResourceTemplate.
		WithID(uuid.New()).
		InOrg(uuid.New()).
		WithOwner(uuid.NewString()).
		WithGroupACL(map[string][]Action{
			uuid.NewString(): {ActionRead, ActionCreate},
			uuid.NewString(): {ActionRead, ActionCreate},
			uuid.NewString(): {ActionRead, ActionCreate},
		}).WithACLUserList(map[string][]Action{
		uuid.NewString(): {ActionRead, ActionCreate},
		uuid.NewString(): {ActionRead, ActionCreate},
	})

	jsonSubject := authSubject{
		ID:     actor.ID,
		Roles:  must(actor.Roles.Expand()),
		Groups: actor.Groups,
		Scope:  must(actor.Scope.Expand()),
	}

	b.Run("ManualRegoValue", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, err := regoInputValue(actor, ActionRead, obj)
			require.NoError(b, err)
		}
	})
	b.Run("JSONRegoValue", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, err := ast.InterfaceToValue(jsonSubject)
			require.NoError(b, err)
		}
	})

}

func TestRoleByName(t *testing.T) {
	t.Parallel()

	t.Run("BuiltIns", func(t *testing.T) {
		t.Parallel()
		testCases := []struct {
			Role Role
		}{
			{Role: builtInRoles[owner]("")},
			{Role: builtInRoles[member]("")},
			{Role: builtInRoles[templateAdmin]("")},
			{Role: builtInRoles[userAdmin]("")},
			{Role: builtInRoles[auditor]("")},

			{Role: builtInRoles[orgAdmin](uuid.New().String())},
			{Role: builtInRoles[orgAdmin](uuid.New().String())},
			{Role: builtInRoles[orgAdmin](uuid.New().String())},

			{Role: builtInRoles[orgMember](uuid.New().String())},
			{Role: builtInRoles[orgMember](uuid.New().String())},
			{Role: builtInRoles[orgMember](uuid.New().String())},
		}

		for _, c := range testCases {
			c := c
			t.Run(c.Role.Name, func(t *testing.T) {
				role, err := RoleByName(c.Role.Name)
				require.NoError(t, err, "role exists")
				equalRoles(t, c.Role, role)
			})
		}
	})

	// nolint:paralleltest
	t.Run("Errors", func(t *testing.T) {
		var err error

		_, err = RoleByName("")
		require.Error(t, err, "empty role")

		_, err = RoleByName("too:many:colons")
		require.Error(t, err, "too many colons")

		_, err = RoleByName(orgMember)
		require.Error(t, err, "expect orgID")
	})
}

// SameAs compares 2 roles for equality.
func equalRoles(t *testing.T, a, b Role) {
	require.Equal(t, a.Name, b.Name, "role names")
	require.Equal(t, a.DisplayName, b.DisplayName, "role display names")
	require.ElementsMatch(t, a.Site, b.Site, "site permissions")
	require.ElementsMatch(t, a.User, b.User, "user permissions")
	require.Equal(t, len(a.Org), len(b.Org), "same number of org roles")

	for ak, av := range a.Org {
		bv, ok := b.Org[ak]
		require.True(t, ok, "org permissions missing: %s", ak)
		require.ElementsMatchf(t, av, bv, "org %s permissions", ak)
	}
}

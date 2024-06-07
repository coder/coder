package rbac

import (
	"testing"

	"github.com/google/uuid"
	"github.com/open-policy-agent/opa/ast"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/rbac/policy"
)

// BenchmarkRBACValueAllocation benchmarks the cost of allocating a rego input
// value. By default, `ast.InterfaceToValue` is used to convert the input,
// which uses json marshaling under the hood.
//
// Currently ast.Object.insert() is the slowest part of the process and allocates
// the most amount of bytes. This general approach copies all of our struct
// data and uses a lot of extra memory for handling things like sort order.
// A possible large improvement would be to implement the ast.Value interface directly.
func BenchmarkRBACValueAllocation(b *testing.B) {
	actor := Subject{
		Roles:  RoleNames{ScopedRoleOrgMember(uuid.New()), ScopedRoleOrgAdmin(uuid.New()), RoleMember()},
		ID:     uuid.NewString(),
		Scope:  ScopeAll,
		Groups: []string{uuid.NewString(), uuid.NewString(), uuid.NewString()},
	}
	obj := ResourceTemplate.
		WithID(uuid.New()).
		InOrg(uuid.New()).
		WithOwner(uuid.NewString()).
		WithGroupACL(map[string][]policy.Action{
			uuid.NewString(): {policy.ActionRead, policy.ActionCreate},
			uuid.NewString(): {policy.ActionRead, policy.ActionCreate},
			uuid.NewString(): {policy.ActionRead, policy.ActionCreate},
		}).WithACLUserList(map[string][]policy.Action{
		uuid.NewString(): {policy.ActionRead, policy.ActionCreate},
		uuid.NewString(): {policy.ActionRead, policy.ActionCreate},
	})

	jsonSubject := authSubject{
		ID:     actor.ID,
		Roles:  must(actor.Roles.Expand()),
		Groups: actor.Groups,
		Scope:  must(actor.Scope.Expand()),
	}

	b.Run("ManualRegoValue", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, err := regoInputValue(actor, policy.ActionRead, obj)
			require.NoError(b, err)
		}
	})
	b.Run("JSONRegoValue", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, err := ast.InterfaceToValue(map[string]interface{}{
				"subject": jsonSubject,
				"action":  policy.ActionRead,
				"object":  obj,
			})
			require.NoError(b, err)
		}
	})
}

// TestRegoInputValue ensures the custom rego input parser returns the
// same value as the default json parser. The json parser is always correct,
// and the custom parser is used to reduce allocations. This optimization
// should yield the same results. Anything different is a bug.
func TestRegoInputValue(t *testing.T) {
	t.Parallel()

	// Expand all roles and make sure we have a good copy.
	// This is because these tests modify the roles, and we don't want to
	// modify the original roles.
	roles, err := RoleNames{ScopedRoleOrgMember(uuid.New()), ScopedRoleOrgAdmin(uuid.New()), RoleMember()}.Expand()
	require.NoError(t, err, "failed to expand roles")
	for i := range roles {
		// If all cached values are nil, then the role will not use
		// the shared cached value.
		roles[i].cachedRegoValue = nil
	}

	actor := Subject{
		Roles:  Roles(roles),
		ID:     uuid.NewString(),
		Scope:  ScopeAll,
		Groups: []string{uuid.NewString(), uuid.NewString(), uuid.NewString()},
	}

	obj := ResourceTemplate.
		WithID(uuid.New()).
		InOrg(uuid.New()).
		WithOwner(uuid.NewString()).
		WithGroupACL(map[string][]policy.Action{
			uuid.NewString(): {policy.ActionRead, policy.ActionCreate},
			uuid.NewString(): {policy.ActionRead, policy.ActionCreate},
			uuid.NewString(): {policy.ActionRead, policy.ActionCreate},
		}).WithACLUserList(map[string][]policy.Action{
		uuid.NewString(): {policy.ActionRead, policy.ActionCreate},
		uuid.NewString(): {policy.ActionRead, policy.ActionCreate},
	})

	action := policy.ActionRead

	t.Run("InputValue", func(t *testing.T) {
		t.Parallel()

		// This is the input that would be passed to the rego policy.
		jsonInput := map[string]interface{}{
			"subject": authSubject{
				ID:     actor.ID,
				Roles:  must(actor.Roles.Expand()),
				Groups: actor.Groups,
				Scope:  must(actor.Scope.Expand()),
			},
			"action": action,
			"object": obj,
		}

		manual, err := regoInputValue(actor, action, obj)
		require.NoError(t, err)

		general, err := ast.InterfaceToValue(jsonInput)
		require.NoError(t, err)

		// The custom parser does not set these fields because they are not needed.
		// To ensure the outputs are identical, intentionally overwrite all names
		// to the same values.
		ignoreNames(t, manual)
		ignoreNames(t, general)

		cmp := manual.Compare(general)
		require.Equal(t, 0, cmp, "manual and general input values should be equal")
	})

	t.Run("PartialInputValue", func(t *testing.T) {
		t.Parallel()

		// This is the input that would be passed to the rego policy.
		jsonInput := map[string]interface{}{
			"subject": authSubject{
				ID:     actor.ID,
				Roles:  must(actor.Roles.Expand()),
				Groups: actor.Groups,
				Scope:  must(actor.Scope.Expand()),
			},
			"action": action,
			"object": map[string]interface{}{
				"type": obj.Type,
			},
		}

		manual, err := regoPartialInputValue(actor, action, obj.Type)
		require.NoError(t, err)

		general, err := ast.InterfaceToValue(jsonInput)
		require.NoError(t, err)

		// The custom parser does not set these fields because they are not needed.
		// To ensure the outputs are identical, intentionally overwrite all names
		// to the same values.
		ignoreNames(t, manual)
		ignoreNames(t, general)

		cmp := manual.Compare(general)
		require.Equal(t, 0, cmp, "manual and general input values should be equal")
	})
}

// ignoreNames sets all names to "ignore" to ensure the values are identical.
func ignoreNames(t *testing.T, value ast.Value) {
	t.Helper()

	// Override the names of the roles
	ref := ast.Ref{
		ast.StringTerm("subject"),
		ast.StringTerm("roles"),
	}
	roles, err := value.Find(ref)
	require.NoError(t, err)

	rolesArray, ok := roles.(*ast.Array)
	require.True(t, ok, "roles is expected to be an array")

	rolesArray.Foreach(func(term *ast.Term) {
		obj, _ := term.Value.(ast.Object)
		// Ignore all names
		obj.Insert(ast.StringTerm("name"), ast.StringTerm("ignore"))
		obj.Insert(ast.StringTerm("display_name"), ast.StringTerm("ignore"))
	})

	// Override the names of the scope role
	ref = ast.Ref{
		ast.StringTerm("subject"),
		ast.StringTerm("scope"),
	}
	scope, err := value.Find(ref)
	require.NoError(t, err)

	scopeObj, ok := scope.(ast.Object)
	require.True(t, ok, "scope is expected to be an object")

	scopeObj.Insert(ast.StringTerm("name"), ast.StringTerm("ignore"))
	scopeObj.Insert(ast.StringTerm("display_name"), ast.StringTerm("ignore"))
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

			{Role: builtInRoles[orgAdmin]("4592dac5-0945-42fd-828d-a903957d3dbb")},
			{Role: builtInRoles[orgAdmin]("24c100c5-1920-49c0-8c38-1b640ac4b38c")},
			{Role: builtInRoles[orgAdmin]("4a00f697-0040-4079-b3ce-d24470281a62")},

			{Role: builtInRoles[orgMember]("3293c50e-fa5d-414f-a461-01112a4dfb6f")},
			{Role: builtInRoles[orgMember]("f88dd23d-bdbd-469d-b82e-36ee06c3d1e1")},
			{Role: builtInRoles[orgMember]("02cfd2a5-016c-4d8d-8290-301f5f18023d")},
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

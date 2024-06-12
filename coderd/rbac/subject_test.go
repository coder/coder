package rbac_test

import (
	"testing"

	"github.com/coder/coder/v2/coderd/rbac"
)

func TestSubjectEqual(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		Name     string
		A        rbac.Subject
		B        rbac.Subject
		Expected bool
	}{
		{
			Name:     "Empty",
			A:        rbac.Subject{},
			B:        rbac.Subject{},
			Expected: true,
		},
		{
			Name: "Same",
			A: rbac.Subject{
				ID:     "id",
				Roles:  rbac.RoleIdentifiers{rbac.RoleMember()},
				Groups: []string{"group"},
				Scope:  rbac.ScopeAll,
			},
			B: rbac.Subject{
				ID:     "id",
				Roles:  rbac.RoleIdentifiers{rbac.RoleMember()},
				Groups: []string{"group"},
				Scope:  rbac.ScopeAll,
			},
			Expected: true,
		},
		{
			Name: "DifferentID",
			A: rbac.Subject{
				ID: "id",
			},
			B: rbac.Subject{
				ID: "id2",
			},
			Expected: false,
		},
		{
			Name: "RolesNilVs0",
			A: rbac.Subject{
				Roles: rbac.RoleIdentifiers{},
			},
			B: rbac.Subject{
				Roles: nil,
			},
			Expected: true,
		},
		{
			Name: "GroupsNilVs0",
			A: rbac.Subject{
				Groups: []string{},
			},
			B: rbac.Subject{
				Groups: nil,
			},
			Expected: true,
		},
		{
			Name: "DifferentRoles",
			A: rbac.Subject{
				Roles: rbac.RoleIdentifiers{rbac.RoleMember()},
			},
			B: rbac.Subject{
				Roles: rbac.RoleIdentifiers{rbac.RoleOwner()},
			},
			Expected: false,
		},
		{
			Name: "Different#Roles",
			A: rbac.Subject{
				Roles: rbac.RoleIdentifiers{rbac.RoleMember()},
			},
			B: rbac.Subject{
				Roles: rbac.RoleIdentifiers{rbac.RoleMember(), rbac.RoleOwner()},
			},
			Expected: false,
		},
		{
			Name: "DifferentGroups",
			A: rbac.Subject{
				Groups: []string{"group1"},
			},
			B: rbac.Subject{
				Groups: []string{"group2"},
			},
			Expected: false,
		},
		{
			Name: "Different#Groups",
			A: rbac.Subject{
				Groups: []string{"group1"},
			},
			B: rbac.Subject{
				Groups: []string{"group1", "group2"},
			},
			Expected: false,
		},
		{
			Name: "DifferentScope",
			A: rbac.Subject{
				Scope: rbac.ScopeAll,
			},
			B: rbac.Subject{
				Scope: rbac.ScopeApplicationConnect,
			},
			Expected: false,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()

			actual := tc.A.Equal(tc.B)
			if actual != tc.Expected {
				t.Errorf("expected %v, got %v", tc.Expected, actual)
			}
		})
	}
}

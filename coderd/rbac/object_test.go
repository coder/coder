package rbac_test

import (
	"testing"

	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/util/slice"
)

func TestObjectEqual(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		Name     string
		A        rbac.Object
		B        rbac.Object
		Expected bool
	}{
		{
			Name:     "Empty",
			A:        rbac.Object{},
			B:        rbac.Object{},
			Expected: true,
		},
		{
			Name: "NilVs0",
			A: rbac.Object{
				ACLGroupList: map[string][]policy.Action{},
				ACLUserList:  map[string][]policy.Action{},
			},
			B:        rbac.Object{},
			Expected: true,
		},
		{
			Name: "Same",
			A: rbac.Object{
				ID:           "id",
				Owner:        "owner",
				OrgID:        "orgID",
				Type:         "type",
				ACLUserList:  map[string][]policy.Action{},
				ACLGroupList: map[string][]policy.Action{},
			},
			B: rbac.Object{
				ID:           "id",
				Owner:        "owner",
				OrgID:        "orgID",
				Type:         "type",
				ACLUserList:  map[string][]policy.Action{},
				ACLGroupList: map[string][]policy.Action{},
			},
			Expected: true,
		},
		{
			Name: "DifferentID",
			A: rbac.Object{
				ID: "id",
			},
			B: rbac.Object{
				ID: "id2",
			},
			Expected: false,
		},
		{
			Name: "DifferentOwner",
			A: rbac.Object{
				Owner: "owner",
			},
			B: rbac.Object{
				Owner: "owner2",
			},
			Expected: false,
		},
		{
			Name: "DifferentOrgID",
			A: rbac.Object{
				OrgID: "orgID",
			},
			B: rbac.Object{
				OrgID: "orgID2",
			},
			Expected: false,
		},
		{
			Name: "DifferentType",
			A: rbac.Object{
				Type: "type",
			},
			B: rbac.Object{
				Type: "type2",
			},
			Expected: false,
		},
		{
			Name: "DifferentACLUserList",
			A: rbac.Object{
				ACLUserList: map[string][]policy.Action{
					"user1": {policy.ActionRead},
				},
			},
			B: rbac.Object{
				ACLUserList: map[string][]policy.Action{
					"user2": {policy.ActionRead},
				},
			},
			Expected: false,
		},
		{
			Name: "ACLUserDiff#Actions",
			A: rbac.Object{
				ACLUserList: map[string][]policy.Action{
					"user1": {policy.ActionRead},
				},
			},
			B: rbac.Object{
				ACLUserList: map[string][]policy.Action{
					"user1": {policy.ActionRead, policy.ActionUpdate},
				},
			},
			Expected: false,
		},
		{
			Name: "ACLUserDiffAction",
			A: rbac.Object{
				ACLUserList: map[string][]policy.Action{
					"user1": {policy.ActionRead},
				},
			},
			B: rbac.Object{
				ACLUserList: map[string][]policy.Action{
					"user1": {policy.ActionUpdate},
				},
			},
			Expected: false,
		},
		{
			Name: "ACLUserDiff#Users",
			A: rbac.Object{
				ACLUserList: map[string][]policy.Action{
					"user1": {policy.ActionRead},
				},
			},
			B: rbac.Object{
				ACLUserList: map[string][]policy.Action{
					"user1": {policy.ActionRead},
					"user2": {policy.ActionRead},
				},
			},
			Expected: false,
		},
		{
			Name: "DifferentACLGroupList",
			A: rbac.Object{
				ACLGroupList: map[string][]policy.Action{
					"group1": {policy.ActionRead},
				},
			},
			B: rbac.Object{
				ACLGroupList: map[string][]policy.Action{
					"group2": {policy.ActionRead},
				},
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

// TestAllResources ensures that all resources have a unique type name.
func TestAllResources(t *testing.T) {
	t.Parallel()

	var typeNames []string
	resources := rbac.AllResources()
	for _, r := range resources {
		if r.Type == "" {
			t.Errorf("empty type name: %s", r.Type)
			continue
		}
		if slice.Contains(typeNames, r.Type) {
			t.Errorf("duplicate type name: %s", r.Type)
			continue
		}
		typeNames = append(typeNames, r.Type)
	}
}

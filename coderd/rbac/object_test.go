package rbac_test

import (
	"testing"

	"github.com/coder/coder/coderd/rbac"
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
				ACLGroupList: map[string][]rbac.Action{},
				ACLUserList:  map[string][]rbac.Action{},
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
				ACLUserList:  map[string][]rbac.Action{},
				ACLGroupList: map[string][]rbac.Action{},
			},
			B: rbac.Object{
				ID:           "id",
				Owner:        "owner",
				OrgID:        "orgID",
				Type:         "type",
				ACLUserList:  map[string][]rbac.Action{},
				ACLGroupList: map[string][]rbac.Action{},
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
				ACLUserList: map[string][]rbac.Action{
					"user1": {rbac.ActionRead},
				},
			},
			B: rbac.Object{
				ACLUserList: map[string][]rbac.Action{
					"user2": {rbac.ActionRead},
				},
			},
			Expected: false,
		},
		{
			Name: "ACLUserDiff#Actions",
			A: rbac.Object{
				ACLUserList: map[string][]rbac.Action{
					"user1": {rbac.ActionRead},
				},
			},
			B: rbac.Object{
				ACLUserList: map[string][]rbac.Action{
					"user1": {rbac.ActionRead, rbac.ActionUpdate},
				},
			},
			Expected: false,
		},
		{
			Name: "ACLUserDiffAction",
			A: rbac.Object{
				ACLUserList: map[string][]rbac.Action{
					"user1": {rbac.ActionRead},
				},
			},
			B: rbac.Object{
				ACLUserList: map[string][]rbac.Action{
					"user1": {rbac.ActionUpdate},
				},
			},
			Expected: false,
		},
		{
			Name: "ACLUserDiff#Users",
			A: rbac.Object{
				ACLUserList: map[string][]rbac.Action{
					"user1": {rbac.ActionRead},
				},
			},
			B: rbac.Object{
				ACLUserList: map[string][]rbac.Action{
					"user1": {rbac.ActionRead},
					"user2": {rbac.ActionRead},
				},
			},
			Expected: false,
		},
		{
			Name: "DifferentACLGroupList",
			A: rbac.Object{
				ACLGroupList: map[string][]rbac.Action{
					"group1": {rbac.ActionRead},
				},
			},
			B: rbac.Object{
				ACLGroupList: map[string][]rbac.Action{
					"group2": {rbac.ActionRead},
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

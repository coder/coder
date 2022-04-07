package rbac

import (
	"reflect"
	"testing"
)

type testCaseSingleRoleSingleOperation struct {
	Role
	Resource
	Operation
	Want bool
}

type testCaseSingleRoleMultipleOperations struct {
	Role
	Resource
	Operations
	Want bool
}

type testCaseMultipleRolesSingleOperation struct {
	Roles
	Resource
	Operation
	Want bool
}

type testCaseMultipleRolesMultipleOperations struct {
	Roles
	Resource
	Operations
	Want bool
}

func deepCompare(t *testing.T, got interface{}, want interface{}) {
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got: %v, want: %v", got, want)
	}
}

func shallowCompare(t *testing.T, got bool, want bool) {
	if got != want {
		t.Errorf("got: %v, want: %v", got, want)
	}
}

// Resources.
const (
	Workspace Resource = "environment"
	Image     Resource = "image"
)

// Operations.
const (
	Create    Operation = "create"
	ReadOwn   Operation = "read-own"
	DeleteAll Operation = "delete-all"
)

var testEnforcer = Enforcer{
	Inheritances{
		Admin: Roles{Member},
	},
	RolePermissions{
		Admin: {
			Workspace: Operations{DeleteAll},
		},
		Member: {
			Workspace: Operations{Create, ReadOwn},
		},
	},
}

func Test_Enforcer_GetResourcePermissions(t *testing.T) {
	got := testEnforcer.GetResourcePermissions(Member)
	want := ResourcePermissions{
		Workspace: Operations{Create, ReadOwn},
	}
	deepCompare(t, got, want)

	got = testEnforcer.GetResourcePermissions(Auditor)
	want = nil
	deepCompare(t, got, want)
}

func Test_Enforcer_GetOperations(t *testing.T) {
	got := testEnforcer.GetOperations(Member, Workspace)
	want := Operations{Create, ReadOwn}
	deepCompare(t, got, want)
}

func Test_Enforcer_RoleHasDirectPermission(t *testing.T) {
	testCases := []testCaseSingleRoleSingleOperation{
		{Member, Workspace, Create, true},
		{Member, Workspace, DeleteAll, false},
		{Member, Image, Create, false},
		{Admin, Workspace, Create, false},
	}
	for _, tc := range testCases {
		got := testEnforcer.RoleHasDirectPermission(tc.Role, tc.Resource, tc.Operation)
		shallowCompare(t, got, tc.Want)
	}
}

func Test_Enforcer_RoleHasInheritedPermission(t *testing.T) {
	testCases := []testCaseSingleRoleSingleOperation{
		{Admin, Workspace, Create, true},
		{Admin, Workspace, DeleteAll, false},
		{Member, Image, Create, false},
	}
	for _, tc := range testCases {
		got := testEnforcer.RoleHasInheritedPermission(tc.Role, tc.Resource, tc.Operation)
		shallowCompare(t, got, tc.Want)
	}
}

func Test_Enforcer_RoleHasPermission(t *testing.T) {
	testCases := []testCaseSingleRoleSingleOperation{
		{Member, Workspace, Create, true},
		{Member, Workspace, DeleteAll, false},
		{Admin, Workspace, Create, true},
	}
	for _, tc := range testCases {
		got := testEnforcer.RoleHasPermission(tc.Role, tc.Resource, tc.Operation)
		shallowCompare(t, got, tc.Want)
	}
}

func Test_Enforcer_RoleHasAllPermissions(t *testing.T) {
	testCases := []testCaseSingleRoleMultipleOperations{
		{Admin, Workspace, Operations{Create, DeleteAll}, true},
		{Member, Workspace, Operations{Create, DeleteAll}, false},
	}
	for _, tc := range testCases {
		got := testEnforcer.RoleHasAllPermissions(tc.Role, tc.Resource, tc.Operations)
		shallowCompare(t, got, tc.Want)
	}
}

func Test_Enforcer_RolesHavePermission(t *testing.T) {
	testCases := []testCaseMultipleRolesSingleOperation{
		{Roles{Member, Admin}, Workspace, Create, true},
		{Roles{Member}, Workspace, DeleteAll, false},
		{Roles{Auditor, Admin}, Workspace, Create, true},
	}
	for _, tc := range testCases {
		got := testEnforcer.RolesHavePermission(tc.Roles, tc.Resource, tc.Operation)
		shallowCompare(t, got, tc.Want)
	}
}

func Test_Enforcer_RolesHaveAllPermissions(t *testing.T) {
	testCases := []testCaseMultipleRolesMultipleOperations{
		{Roles{Member, Admin}, Workspace, Operations{Create, DeleteAll}, true},
		{Roles{Member}, Workspace, Operations{Create, DeleteAll}, false},
	}
	for _, tc := range testCases {
		got := testEnforcer.RolesHaveAllPermissions(tc.Roles, tc.Resource, tc.Operations)
		shallowCompare(t, got, tc.Want)
	}
}

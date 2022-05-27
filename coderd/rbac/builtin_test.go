package rbac_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/rbac"
)

type authSubject struct {
	// Name is helpful for test assertions
	Name   string
	UserID string
	Roles  []string
}

func TestRolePermissions(t *testing.T) {
	t.Parallel()

	auth, err := rbac.NewAuthorizer()
	require.NoError(t, err, "new rego authorizer")

	// currentUser is anything that references "me", "mine", or "my".
	currentUser := uuid.New()
	adminID := uuid.New()
	orgID := uuid.New()
	otherOrg := uuid.New()

	// Subjects to user
	memberMe := authSubject{Name: "member_me", UserID: currentUser.String(), Roles: []string{rbac.RoleMember()}}
	orgMemberMe := authSubject{Name: "org_member_me", UserID: currentUser.String(), Roles: []string{rbac.RoleMember(), rbac.RoleOrgMember(orgID)}}

	admin := authSubject{Name: "admin", UserID: adminID.String(), Roles: []string{rbac.RoleMember(), rbac.RoleAdmin()}}
	orgAdmin := authSubject{Name: "org_admin", UserID: adminID.String(), Roles: []string{rbac.RoleMember(), rbac.RoleOrgMember(orgID), rbac.RoleOrgAdmin(orgID)}}

	otherOrgMember := authSubject{Name: "org_member_other", UserID: uuid.NewString(), Roles: []string{rbac.RoleMember(), rbac.RoleOrgMember(otherOrg)}}
	otherOrgAdmin := authSubject{Name: "org_admin_other", UserID: uuid.NewString(), Roles: []string{rbac.RoleMember(), rbac.RoleOrgMember(otherOrg), rbac.RoleOrgAdmin(otherOrg)}}

	// requiredSubjects are required to be asserted in each test case. This is
	// to make sure one is not forgotten.
	requiredSubjects := []authSubject{memberMe, admin, orgMemberMe, orgAdmin, otherOrgAdmin, otherOrgMember}

	testCases := []struct {
		// Name the test case to better locate the failing test case.
		Name     string
		Resource rbac.Object
		Actions  []rbac.Action
		// AuthorizeMap must cover all subjects in 'requiredSubjects'.
		// This map will run an Authorize() check with the resource, action,
		// and subjects. The subjects are split into 2 categories, "true" and
		// "false".
		//		true: Subjects who Authorize should return no error
		//		false: Subjects who Authorize should return forbidden.
		AuthorizeMap map[bool][]authSubject
	}{
		{
			Name:     "MyUser",
			Actions:  []rbac.Action{rbac.ActionRead},
			Resource: rbac.ResourceUser.WithID(currentUser.String()),
			AuthorizeMap: map[bool][]authSubject{
				true:  {admin, memberMe, orgMemberMe, orgAdmin, otherOrgMember, otherOrgAdmin},
				false: {},
			},
		},
		{
			Name:     "AUser",
			Actions:  []rbac.Action{rbac.ActionCreate, rbac.ActionUpdate, rbac.ActionDelete},
			Resource: rbac.ResourceUser,
			AuthorizeMap: map[bool][]authSubject{
				true:  {admin},
				false: {memberMe, orgMemberMe, orgAdmin, otherOrgMember, otherOrgAdmin},
			},
		},
		{
			Name: "MyWorkspaceInOrg",
			// When creating the WithID won't be set, but it does not change the result.
			Actions:  []rbac.Action{rbac.ActionCreate, rbac.ActionRead, rbac.ActionUpdate, rbac.ActionDelete},
			Resource: rbac.ResourceWorkspace.InOrg(orgID).WithOwner(currentUser.String()).WithID(uuid.NewString()),
			AuthorizeMap: map[bool][]authSubject{
				true:  {admin, orgMemberMe, orgAdmin},
				false: {memberMe, otherOrgAdmin, otherOrgMember},
			},
		},
		{
			Name:     "Templates",
			Actions:  []rbac.Action{rbac.ActionCreate, rbac.ActionUpdate, rbac.ActionDelete},
			Resource: rbac.ResourceTemplate.InOrg(orgID).WithID(uuid.NewString()),
			AuthorizeMap: map[bool][]authSubject{
				true:  {admin, orgAdmin},
				false: {memberMe, orgMemberMe, otherOrgAdmin, otherOrgMember},
			},
		},
		{
			Name:     "ReadTemplates",
			Actions:  []rbac.Action{rbac.ActionRead},
			Resource: rbac.ResourceTemplate.InOrg(orgID).WithID(uuid.NewString()),
			AuthorizeMap: map[bool][]authSubject{
				true:  {admin, orgMemberMe, orgAdmin},
				false: {memberMe, otherOrgAdmin, otherOrgMember},
			},
		},
		{
			Name:     "Files",
			Actions:  []rbac.Action{rbac.ActionCreate},
			Resource: rbac.ResourceFile,
			AuthorizeMap: map[bool][]authSubject{
				true:  {admin},
				false: {orgMemberMe, orgAdmin, memberMe, otherOrgAdmin, otherOrgMember},
			},
		},
		{
			Name:     "MyFile",
			Actions:  []rbac.Action{rbac.ActionRead, rbac.ActionUpdate, rbac.ActionDelete},
			Resource: rbac.ResourceFile.WithID(uuid.NewString()).WithOwner(currentUser.String()),
			AuthorizeMap: map[bool][]authSubject{
				true:  {admin, memberMe, orgMemberMe},
				false: {orgAdmin, otherOrgAdmin, otherOrgMember},
			},
		},
		{
			Name:     "CreateOrganizations",
			Actions:  []rbac.Action{rbac.ActionCreate},
			Resource: rbac.ResourceOrganization,
			AuthorizeMap: map[bool][]authSubject{
				true:  {admin},
				false: {orgAdmin, otherOrgAdmin, otherOrgMember, memberMe, orgMemberMe},
			},
		},
		{
			Name:     "Organizations",
			Actions:  []rbac.Action{rbac.ActionUpdate, rbac.ActionDelete},
			Resource: rbac.ResourceOrganization.InOrg(orgID).WithID(orgID.String()),
			AuthorizeMap: map[bool][]authSubject{
				true:  {admin, orgAdmin},
				false: {otherOrgAdmin, otherOrgMember, memberMe, orgMemberMe},
			},
		},
		{
			Name:     "ReadOrganizations",
			Actions:  []rbac.Action{rbac.ActionRead},
			Resource: rbac.ResourceOrganization.InOrg(orgID).WithID(orgID.String()),
			AuthorizeMap: map[bool][]authSubject{
				true:  {admin, orgAdmin, orgMemberMe},
				false: {otherOrgAdmin, otherOrgMember, memberMe},
			},
		},
		{
			Name:     "RoleAssignment",
			Actions:  []rbac.Action{rbac.ActionCreate, rbac.ActionUpdate, rbac.ActionDelete},
			Resource: rbac.ResourceRoleAssignment,
			AuthorizeMap: map[bool][]authSubject{
				true:  {admin},
				false: {orgAdmin, orgMemberMe, otherOrgAdmin, otherOrgMember, memberMe},
			},
		},
		{
			Name:     "ReadRoleAssignment",
			Actions:  []rbac.Action{rbac.ActionRead},
			Resource: rbac.ResourceRoleAssignment,
			AuthorizeMap: map[bool][]authSubject{
				true:  {admin, orgAdmin, orgMemberMe, otherOrgAdmin, otherOrgMember, memberMe},
				false: {},
			},
		},
		{
			Name:     "OrgRoleAssignment",
			Actions:  []rbac.Action{rbac.ActionCreate, rbac.ActionUpdate, rbac.ActionDelete},
			Resource: rbac.ResourceOrgRoleAssignment.InOrg(orgID),
			AuthorizeMap: map[bool][]authSubject{
				true:  {admin, orgAdmin},
				false: {orgMemberMe, otherOrgAdmin, otherOrgMember, memberMe},
			},
		},
		{
			Name:     "ReadOrgRoleAssignment",
			Actions:  []rbac.Action{rbac.ActionRead},
			Resource: rbac.ResourceOrgRoleAssignment.InOrg(orgID),
			AuthorizeMap: map[bool][]authSubject{
				true:  {admin, orgAdmin, orgMemberMe},
				false: {otherOrgAdmin, otherOrgMember, memberMe},
			},
		},
		{
			Name:     "APIKey",
			Actions:  []rbac.Action{rbac.ActionCreate, rbac.ActionRead, rbac.ActionUpdate, rbac.ActionDelete},
			Resource: rbac.ResourceAPIKey.WithOwner(currentUser.String()).WithID(uuid.NewString()),
			AuthorizeMap: map[bool][]authSubject{
				true:  {admin, orgMemberMe, memberMe},
				false: {orgAdmin, otherOrgAdmin, otherOrgMember},
			},
		},
		{
			Name:     "UserData",
			Actions:  []rbac.Action{rbac.ActionCreate, rbac.ActionRead, rbac.ActionUpdate, rbac.ActionDelete},
			Resource: rbac.ResourceUserData.WithOwner(currentUser.String()).WithID(currentUser.String()),
			AuthorizeMap: map[bool][]authSubject{
				true:  {admin, orgMemberMe, memberMe},
				false: {orgAdmin, otherOrgAdmin, otherOrgMember},
			},
		},
		{
			Name:     "ManageOrgMember",
			Actions:  []rbac.Action{rbac.ActionCreate, rbac.ActionUpdate, rbac.ActionDelete},
			Resource: rbac.ResourceOrganizationMember.InOrg(orgID).WithID(uuid.NewString()),
			AuthorizeMap: map[bool][]authSubject{
				true:  {admin, orgAdmin},
				false: {orgMemberMe, memberMe, otherOrgAdmin, otherOrgMember},
			},
		},
		{
			Name:     "ReadOrgMember",
			Actions:  []rbac.Action{rbac.ActionRead},
			Resource: rbac.ResourceOrganizationMember.InOrg(orgID).WithID(uuid.NewString()),
			AuthorizeMap: map[bool][]authSubject{
				true:  {admin, orgAdmin, orgMemberMe},
				false: {memberMe, otherOrgAdmin, otherOrgMember},
			},
		},
	}

	for _, c := range testCases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			remainingSubjs := make(map[string]struct{})
			for _, subj := range requiredSubjects {
				remainingSubjs[subj.Name] = struct{}{}
			}

			for _, action := range c.Actions {
				for result, subjs := range c.AuthorizeMap {
					for _, subj := range subjs {
						delete(remainingSubjs, subj.Name)
						msg := fmt.Sprintf("%s as %q doing %q on %q", c.Name, subj.Name, action, c.Resource.Type)
						err := auth.ByRoleName(context.Background(), subj.UserID, subj.Roles, action, c.Resource)
						if result {
							assert.NoError(t, err, fmt.Sprintf("Should pass: %s", msg))
						} else {
							assert.ErrorContains(t, err, "forbidden", fmt.Sprintf("Should fail: %s", msg))
						}
					}
				}
			}
			require.Empty(t, remainingSubjs, "test should cover all subjects")
		})
	}
}

func TestIsOrgRole(t *testing.T) {
	t.Parallel()
	randomUUID := uuid.New()

	testCases := []struct {
		RoleName string
		OrgRole  bool
		OrgID    string
	}{
		// Not org roles
		{RoleName: rbac.RoleAdmin()},
		{RoleName: rbac.RoleMember()},
		{RoleName: "auditor"},

		{
			RoleName: "a:bad:role",
			OrgRole:  false,
		},
		{
			RoleName: "",
			OrgRole:  false,
		},

		// Org roles
		{
			RoleName: rbac.RoleOrgAdmin(randomUUID),
			OrgRole:  true,
			OrgID:    randomUUID.String(),
		},
		{
			RoleName: rbac.RoleOrgMember(randomUUID),
			OrgRole:  true,
			OrgID:    randomUUID.String(),
		},
		{
			RoleName: "test:example",
			OrgRole:  true,
			OrgID:    "example",
		},
	}

	// nolint:paralleltest
	for _, c := range testCases {
		t.Run(c.RoleName, func(t *testing.T) {
			orgID, ok := rbac.IsOrgRole(c.RoleName)
			require.Equal(t, c.OrgRole, ok, "match expected org role")
			require.Equal(t, c.OrgID, orgID, "match expected org id")
		})
	}
}

func TestListRoles(t *testing.T) {
	t.Parallel()

	siteRoles := rbac.SiteRoles()
	siteRoleNames := make([]string, 0, len(siteRoles))
	for _, role := range siteRoles {
		siteRoleNames = append(siteRoleNames, role.Name)
	}

	// If this test is ever failing, just update the list to the roles
	// expected from the builtin set.
	require.ElementsMatch(t, []string{
		"admin",
		"member",
		"auditor",
	},
		siteRoleNames)

	orgID := uuid.New()
	orgRoles := rbac.OrganizationRoles(orgID)
	orgRoleNames := make([]string, 0, len(orgRoles))
	for _, role := range orgRoles {
		orgRoleNames = append(orgRoleNames, role.Name)
	}

	require.ElementsMatch(t, []string{
		fmt.Sprintf("organization-admin:%s", orgID.String()),
		fmt.Sprintf("organization-member:%s", orgID.String()),
	},
		orgRoleNames)
}

func TestChangeSet(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		Name      string
		From      []string
		To        []string
		ExpAdd    []string
		ExpRemove []string
	}{
		{
			Name: "Empty",
		},
		{
			Name:      "Same",
			From:      []string{"a", "b", "c"},
			To:        []string{"a", "b", "c"},
			ExpAdd:    []string{},
			ExpRemove: []string{},
		},
		{
			Name:      "AllRemoved",
			From:      []string{"a", "b", "c"},
			ExpRemove: []string{"a", "b", "c"},
		},
		{
			Name:   "AllAdded",
			To:     []string{"a", "b", "c"},
			ExpAdd: []string{"a", "b", "c"},
		},
		{
			Name:      "AddAndRemove",
			From:      []string{"a", "b", "c"},
			To:        []string{"a", "b", "d", "e"},
			ExpAdd:    []string{"d", "e"},
			ExpRemove: []string{"c"},
		},
	}

	for _, c := range testCases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			add, remove := rbac.ChangeRoleSet(c.From, c.To)
			require.ElementsMatch(t, c.ExpAdd, add, "expect added")
			require.ElementsMatch(t, c.ExpRemove, remove, "expect removed")
		})
	}
}

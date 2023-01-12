package rbac_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/rbac"
)

type authSubject struct {
	// Name is helpful for test assertions
	Name   string
	UserID string
	Roles  []string
	Groups []string
}

func TestRolePermissions(t *testing.T) {
	t.Parallel()

	auth := rbac.NewAuthorizer(prometheus.NewRegistry())

	// currentUser is anything that references "me", "mine", or "my".
	currentUser := uuid.New()
	adminID := uuid.New()
	templateAdminID := uuid.New()
	orgID := uuid.New()
	otherOrg := uuid.New()

	// Subjects to user
	memberMe := authSubject{Name: "member_me", UserID: currentUser.String(), Roles: []string{rbac.RoleMember()}}
	orgMemberMe := authSubject{Name: "org_member_me", UserID: currentUser.String(), Roles: []string{rbac.RoleMember(), rbac.RoleOrgMember(orgID)}}

	owner := authSubject{Name: "owner", UserID: adminID.String(), Roles: []string{rbac.RoleMember(), rbac.RoleOwner()}}
	orgAdmin := authSubject{Name: "org_admin", UserID: adminID.String(), Roles: []string{rbac.RoleMember(), rbac.RoleOrgMember(orgID), rbac.RoleOrgAdmin(orgID)}}

	otherOrgMember := authSubject{Name: "org_member_other", UserID: uuid.NewString(), Roles: []string{rbac.RoleMember(), rbac.RoleOrgMember(otherOrg)}}
	otherOrgAdmin := authSubject{Name: "org_admin_other", UserID: uuid.NewString(), Roles: []string{rbac.RoleMember(), rbac.RoleOrgMember(otherOrg), rbac.RoleOrgAdmin(otherOrg)}}

	templateAdmin := authSubject{Name: "template-admin", UserID: templateAdminID.String(), Roles: []string{rbac.RoleMember(), rbac.RoleTemplateAdmin()}}
	userAdmin := authSubject{Name: "user-admin", UserID: templateAdminID.String(), Roles: []string{rbac.RoleMember(), rbac.RoleUserAdmin()}}

	// requiredSubjects are required to be asserted in each test case. This is
	// to make sure one is not forgotten.
	requiredSubjects := []authSubject{memberMe, owner, orgMemberMe, orgAdmin, otherOrgAdmin, otherOrgMember, templateAdmin, userAdmin}

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
			Resource: rbac.ResourceUser,
			AuthorizeMap: map[bool][]authSubject{
				true:  {owner, memberMe, orgMemberMe, orgAdmin, otherOrgMember, otherOrgAdmin, templateAdmin, userAdmin},
				false: {},
			},
		},
		{
			Name:     "AUser",
			Actions:  []rbac.Action{rbac.ActionCreate, rbac.ActionUpdate, rbac.ActionDelete},
			Resource: rbac.ResourceUser,
			AuthorizeMap: map[bool][]authSubject{
				true:  {owner, userAdmin},
				false: {memberMe, orgMemberMe, orgAdmin, otherOrgMember, otherOrgAdmin, templateAdmin},
			},
		},
		{
			Name: "ReadMyWorkspaceInOrg",
			// When creating the WithID won't be set, but it does not change the result.
			Actions:  []rbac.Action{rbac.ActionRead},
			Resource: rbac.ResourceWorkspace.InOrg(orgID).WithOwner(currentUser.String()),
			AuthorizeMap: map[bool][]authSubject{
				true:  {owner, orgMemberMe, orgAdmin, templateAdmin},
				false: {memberMe, otherOrgAdmin, otherOrgMember, userAdmin},
			},
		},
		{
			Name: "C_RDMyWorkspaceInOrg",
			// When creating the WithID won't be set, but it does not change the result.
			Actions:  []rbac.Action{rbac.ActionCreate, rbac.ActionUpdate, rbac.ActionDelete},
			Resource: rbac.ResourceWorkspace.InOrg(orgID).WithOwner(currentUser.String()),
			AuthorizeMap: map[bool][]authSubject{
				true:  {owner, orgMemberMe, orgAdmin},
				false: {memberMe, otherOrgAdmin, otherOrgMember, userAdmin, templateAdmin},
			},
		},
		{
			Name: "MyWorkspaceInOrgExecution",
			// When creating the WithID won't be set, but it does not change the result.
			Actions:  []rbac.Action{rbac.ActionCreate, rbac.ActionRead, rbac.ActionUpdate, rbac.ActionDelete},
			Resource: rbac.ResourceWorkspaceExecution.InOrg(orgID).WithOwner(currentUser.String()),
			AuthorizeMap: map[bool][]authSubject{
				true:  {owner, orgAdmin, orgMemberMe},
				false: {memberMe, otherOrgAdmin, otherOrgMember, templateAdmin, userAdmin},
			},
		},
		{
			Name: "MyWorkspaceInOrgAppConnect",
			// When creating the WithID won't be set, but it does not change the result.
			Actions:  []rbac.Action{rbac.ActionCreate, rbac.ActionRead, rbac.ActionUpdate, rbac.ActionDelete},
			Resource: rbac.ResourceWorkspaceApplicationConnect.InOrg(orgID).WithOwner(currentUser.String()),
			AuthorizeMap: map[bool][]authSubject{
				true:  {owner, orgAdmin, orgMemberMe},
				false: {memberMe, otherOrgAdmin, otherOrgMember, templateAdmin, userAdmin},
			},
		},
		{
			Name:     "Templates",
			Actions:  []rbac.Action{rbac.ActionCreate, rbac.ActionUpdate, rbac.ActionDelete},
			Resource: rbac.ResourceTemplate.InOrg(orgID),
			AuthorizeMap: map[bool][]authSubject{
				true:  {owner, orgAdmin, templateAdmin},
				false: {memberMe, orgMemberMe, otherOrgAdmin, otherOrgMember, userAdmin},
			},
		},
		{
			Name:     "ReadTemplates",
			Actions:  []rbac.Action{rbac.ActionRead},
			Resource: rbac.ResourceTemplate.InOrg(orgID),
			AuthorizeMap: map[bool][]authSubject{
				true:  {owner, orgAdmin, templateAdmin},
				false: {memberMe, otherOrgAdmin, otherOrgMember, userAdmin, orgMemberMe},
			},
		},
		{
			Name:     "Files",
			Actions:  []rbac.Action{rbac.ActionCreate},
			Resource: rbac.ResourceFile,
			AuthorizeMap: map[bool][]authSubject{
				true:  {owner, templateAdmin},
				false: {orgMemberMe, orgAdmin, memberMe, otherOrgAdmin, otherOrgMember, userAdmin},
			},
		},
		{
			Name:     "MyFile",
			Actions:  []rbac.Action{rbac.ActionCreate, rbac.ActionRead, rbac.ActionUpdate, rbac.ActionDelete},
			Resource: rbac.ResourceFile.WithOwner(currentUser.String()),
			AuthorizeMap: map[bool][]authSubject{
				true:  {owner, memberMe, orgMemberMe, templateAdmin},
				false: {orgAdmin, otherOrgAdmin, otherOrgMember, userAdmin},
			},
		},
		{
			Name:     "CreateOrganizations",
			Actions:  []rbac.Action{rbac.ActionCreate},
			Resource: rbac.ResourceOrganization,
			AuthorizeMap: map[bool][]authSubject{
				true:  {owner},
				false: {orgAdmin, otherOrgAdmin, otherOrgMember, memberMe, orgMemberMe, templateAdmin, userAdmin},
			},
		},
		{
			Name:     "Organizations",
			Actions:  []rbac.Action{rbac.ActionUpdate, rbac.ActionDelete},
			Resource: rbac.ResourceOrganization.InOrg(orgID),
			AuthorizeMap: map[bool][]authSubject{
				true:  {owner, orgAdmin},
				false: {otherOrgAdmin, otherOrgMember, memberMe, orgMemberMe, templateAdmin, userAdmin},
			},
		},
		{
			Name:     "ReadOrganizations",
			Actions:  []rbac.Action{rbac.ActionRead},
			Resource: rbac.ResourceOrganization.InOrg(orgID),
			AuthorizeMap: map[bool][]authSubject{
				true:  {owner, orgAdmin, orgMemberMe},
				false: {otherOrgAdmin, otherOrgMember, memberMe, templateAdmin, userAdmin},
			},
		},
		{
			Name:     "RoleAssignment",
			Actions:  []rbac.Action{rbac.ActionCreate, rbac.ActionUpdate, rbac.ActionDelete},
			Resource: rbac.ResourceRoleAssignment,
			AuthorizeMap: map[bool][]authSubject{
				true:  {owner, userAdmin},
				false: {orgAdmin, orgMemberMe, otherOrgAdmin, otherOrgMember, memberMe, templateAdmin},
			},
		},
		{
			Name:     "ReadRoleAssignment",
			Actions:  []rbac.Action{rbac.ActionRead},
			Resource: rbac.ResourceRoleAssignment,
			AuthorizeMap: map[bool][]authSubject{
				true:  {owner, orgAdmin, orgMemberMe, otherOrgAdmin, otherOrgMember, memberMe, templateAdmin, userAdmin},
				false: {},
			},
		},
		{
			Name:     "OrgRoleAssignment",
			Actions:  []rbac.Action{rbac.ActionCreate, rbac.ActionUpdate, rbac.ActionDelete},
			Resource: rbac.ResourceOrgRoleAssignment.InOrg(orgID),
			AuthorizeMap: map[bool][]authSubject{
				true:  {owner, orgAdmin},
				false: {orgMemberMe, otherOrgAdmin, otherOrgMember, memberMe, templateAdmin, userAdmin},
			},
		},
		{
			Name:     "ReadOrgRoleAssignment",
			Actions:  []rbac.Action{rbac.ActionRead},
			Resource: rbac.ResourceOrgRoleAssignment.InOrg(orgID),
			AuthorizeMap: map[bool][]authSubject{
				true:  {owner, orgAdmin, orgMemberMe},
				false: {otherOrgAdmin, otherOrgMember, memberMe, templateAdmin, userAdmin},
			},
		},
		{
			Name:     "APIKey",
			Actions:  []rbac.Action{rbac.ActionCreate, rbac.ActionRead, rbac.ActionUpdate, rbac.ActionDelete},
			Resource: rbac.ResourceAPIKey.WithOwner(currentUser.String()),
			AuthorizeMap: map[bool][]authSubject{
				true:  {owner, orgMemberMe, memberMe},
				false: {orgAdmin, otherOrgAdmin, otherOrgMember, templateAdmin, userAdmin},
			},
		},
		{
			Name:     "UserData",
			Actions:  []rbac.Action{rbac.ActionCreate, rbac.ActionRead, rbac.ActionUpdate, rbac.ActionDelete},
			Resource: rbac.ResourceUserData.WithOwner(currentUser.String()),
			AuthorizeMap: map[bool][]authSubject{
				true:  {owner, orgMemberMe, memberMe},
				false: {orgAdmin, otherOrgAdmin, otherOrgMember, templateAdmin, userAdmin},
			},
		},
		{
			Name:     "ManageOrgMember",
			Actions:  []rbac.Action{rbac.ActionCreate, rbac.ActionUpdate, rbac.ActionDelete},
			Resource: rbac.ResourceOrganizationMember.InOrg(orgID),
			AuthorizeMap: map[bool][]authSubject{
				true:  {owner, orgAdmin, userAdmin},
				false: {orgMemberMe, memberMe, otherOrgAdmin, otherOrgMember, templateAdmin},
			},
		},
		{
			Name:     "ReadOrgMember",
			Actions:  []rbac.Action{rbac.ActionRead},
			Resource: rbac.ResourceOrganizationMember.InOrg(orgID),
			AuthorizeMap: map[bool][]authSubject{
				true:  {owner, orgAdmin, orgMemberMe, userAdmin},
				false: {memberMe, otherOrgAdmin, otherOrgMember, templateAdmin},
			},
		},
		{
			Name:    "AllUsersGroupACL",
			Actions: []rbac.Action{rbac.ActionRead},
			Resource: rbac.ResourceTemplate.InOrg(orgID).WithGroupACL(
				map[string][]rbac.Action{
					orgID.String(): {rbac.ActionRead},
				}),

			AuthorizeMap: map[bool][]authSubject{
				true:  {owner, orgAdmin, orgMemberMe, templateAdmin},
				false: {memberMe, otherOrgAdmin, otherOrgMember, userAdmin},
			},
		},
		{
			Name:     "Groups",
			Actions:  []rbac.Action{rbac.ActionRead},
			Resource: rbac.ResourceGroup.InOrg(orgID),
			AuthorizeMap: map[bool][]authSubject{
				true:  {owner, orgAdmin, userAdmin, orgMemberMe},
				false: {memberMe, otherOrgAdmin, otherOrgMember, templateAdmin},
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
						// TODO: scopey
						err := auth.ByRoleName(context.Background(), subj.UserID, subj.Roles, rbac.ScopeAll, subj.Groups, action, c.Resource)
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
		{RoleName: rbac.RoleOwner()},
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
			t.Parallel()
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
	// Always use constant strings, as if the names change, we need to write
	// a SQL migration to change the name on the backend.
	require.ElementsMatch(t, []string{
		"owner",
		"member",
		"auditor",
		"template-admin",
		"user-admin",
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

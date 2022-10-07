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

// BenchmarkRBACFilter benchmarks the rbac.Filter method.
//
//	go test -bench BenchmarkRBACFilter -benchmem -memprofile memprofile.out -cpuprofile profile.out
func BenchmarkRBACFilter(b *testing.B) {
	orgs := []uuid.UUID{
		uuid.MustParse("bf7b72bd-a2b1-4ef2-962c-1d698e0483f6"),
		uuid.MustParse("e4660c6f-b9de-422d-9578-cd888983a795"),
		uuid.MustParse("fb13d477-06f4-42d9-b957-f6b89bd63515"),
	}

	users := []uuid.UUID{
		uuid.MustParse("10d03e62-7703-4df5-a358-4f76577d4e2f"),
		uuid.MustParse("4ca78b1d-f2d2-4168-9d76-cd93b51c6c1e"),
		uuid.MustParse("0632b012-49e0-4d70-a5b3-f4398f1dcd52"),
		uuid.MustParse("70dbaa7a-ea9c-4f68-a781-97b08af8461d"),
	}

	benchCases := []struct {
		Name   string
		Roles  []string
		UserID uuid.UUID
		Scope  rbac.Scope
	}{
		{
			Name:   "NoRoles",
			Roles:  []string{},
			UserID: users[0],
			Scope:  rbac.ScopeAll,
		},
		{
			Name: "Admin",
			// Give some extra roles that an admin might have
			Roles:  []string{rbac.RoleOrgMember(orgs[0]), "auditor", rbac.RoleOwner(), rbac.RoleMember()},
			UserID: users[0],
			Scope:  rbac.ScopeAll,
		},
		{
			Name:   "OrgAdmin",
			Roles:  []string{rbac.RoleOrgMember(orgs[0]), rbac.RoleOrgAdmin(orgs[0]), rbac.RoleMember()},
			UserID: users[0],
			Scope:  rbac.ScopeAll,
		},
		{
			Name: "OrgMember",
			// Member of 2 orgs
			Roles:  []string{rbac.RoleOrgMember(orgs[0]), rbac.RoleOrgMember(orgs[1]), rbac.RoleMember()},
			UserID: users[0],
			Scope:  rbac.ScopeAll,
		},
		{
			Name: "ManyRoles",
			// Admin of many orgs
			Roles: []string{
				rbac.RoleOrgMember(orgs[0]), rbac.RoleOrgAdmin(orgs[0]),
				rbac.RoleOrgMember(orgs[1]), rbac.RoleOrgAdmin(orgs[1]),
				rbac.RoleOrgMember(orgs[2]), rbac.RoleOrgAdmin(orgs[2]),
				rbac.RoleMember(),
			},
			UserID: users[0],
			Scope:  rbac.ScopeAll,
		},
		{
			Name: "AdminWithScope",
			// Give some extra roles that an admin might have
			Roles:  []string{rbac.RoleOrgMember(orgs[0]), "auditor", rbac.RoleOwner(), rbac.RoleMember()},
			UserID: users[0],
			Scope:  rbac.ScopeApplicationConnect,
		},
	}

	authorizer := rbac.NewAuthorizer()
	for _, c := range benchCases {
		b.Run(c.Name, func(b *testing.B) {
			objects := benchmarkSetup(orgs, users, b.N)
			b.ResetTimer()
			allowed, err := rbac.Filter(context.Background(), authorizer, c.UserID.String(), c.Roles, c.Scope, rbac.ActionRead, objects)
			require.NoError(b, err)
			var _ = allowed
		})
	}
}

func benchmarkSetup(orgs []uuid.UUID, users []uuid.UUID, size int) []rbac.Object {
	// Create a "random" but deterministic set of objects.
	objectList := make([]rbac.Object, size)
	for i := range objectList {
		objectList[i] = rbac.ResourceWorkspace.
			InOrg(orgs[i%len(orgs)]).
			WithOwner(users[i%len(users)].String())
	}

	return objectList
}

type authSubject struct {
	// Name is helpful for test assertions
	Name   string
	UserID string
	Roles  []string
}

func TestRolePermissions(t *testing.T) {
	t.Parallel()

	auth := rbac.NewAuthorizer()

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
				true:  {owner, orgMemberMe, orgAdmin, templateAdmin},
				false: {memberMe, otherOrgAdmin, otherOrgMember, userAdmin},
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
			Actions:  []rbac.Action{rbac.ActionRead, rbac.ActionUpdate, rbac.ActionDelete},
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
						err := auth.ByRoleName(context.Background(), subj.UserID, subj.Roles, rbac.ScopeAll, action, c.Resource)
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

package rbac_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
)

type hasAuthSubjects interface {
	Subjects() []authSubject
}

type authSubjectSet []authSubject

func (a authSubjectSet) Subjects() []authSubject { return a }

type authSubject struct {
	// Name is helpful for test assertions
	Name  string
	Actor rbac.Subject
}

func (a authSubject) Subjects() []authSubject { return []authSubject{a} }

// TestBuiltInRoles makes sure our built-in roles are valid by our own policy
// rules. If this is incorrect, that is a mistake.
func TestBuiltInRoles(t *testing.T) {
	t.Parallel()
	for _, r := range rbac.SiteRoles() {
		r := r
		t.Run(r.Identifier.String(), func(t *testing.T) {
			t.Parallel()
			require.NoError(t, r.Valid(), "invalid role")
		})
	}

	for _, r := range rbac.OrganizationRoles(uuid.New()) {
		r := r
		t.Run(r.Identifier.String(), func(t *testing.T) {
			t.Parallel()
			require.NoError(t, r.Valid(), "invalid role")
		})
	}
}

//nolint:tparallel,paralleltest
func TestOwnerExec(t *testing.T) {
	owner := rbac.Subject{
		ID:    uuid.NewString(),
		Roles: rbac.RoleIdentifiers{rbac.RoleMember(), rbac.RoleOwner()},
		Scope: rbac.ScopeAll,
	}

	t.Run("NoExec", func(t *testing.T) {
		rbac.ReloadBuiltinRoles(&rbac.RoleOptions{
			NoOwnerWorkspaceExec: true,
		})
		t.Cleanup(func() { rbac.ReloadBuiltinRoles(nil) })

		auth := rbac.NewStrictCachingAuthorizer(prometheus.NewRegistry())
		// Exec a random workspace
		err := auth.Authorize(context.Background(), owner, policy.ActionSSH,
			rbac.ResourceWorkspace.WithID(uuid.New()).InOrg(uuid.New()).WithOwner(uuid.NewString()))
		require.ErrorAsf(t, err, &rbac.UnauthorizedError{}, "expected unauthorized error")
	})

	t.Run("Exec", func(t *testing.T) {
		rbac.ReloadBuiltinRoles(&rbac.RoleOptions{
			NoOwnerWorkspaceExec: false,
		})
		t.Cleanup(func() { rbac.ReloadBuiltinRoles(nil) })

		auth := rbac.NewStrictCachingAuthorizer(prometheus.NewRegistry())

		// Exec a random workspace
		err := auth.Authorize(context.Background(), owner, policy.ActionSSH,
			rbac.ResourceWorkspace.WithID(uuid.New()).InOrg(uuid.New()).WithOwner(uuid.NewString()))
		require.NoError(t, err, "expected owner can")
	})
}

// nolint:tparallel,paralleltest // subtests share a map, just run sequentially.
func TestRolePermissions(t *testing.T) {
	t.Parallel()

	crud := []policy.Action{policy.ActionCreate, policy.ActionRead, policy.ActionUpdate, policy.ActionDelete}

	auth := rbac.NewStrictCachingAuthorizer(prometheus.NewRegistry())

	// currentUser is anything that references "me", "mine", or "my".
	currentUser := uuid.New()
	adminID := uuid.New()
	templateAdminID := uuid.New()
	userAdminID := uuid.New()
	auditorID := uuid.New()
	orgID := uuid.New()
	otherOrg := uuid.New()
	workspaceID := uuid.New()
	templateID := uuid.New()
	fileID := uuid.New()
	groupID := uuid.New()
	apiKeyID := uuid.New()

	// Subjects to user
	memberMe := authSubject{Name: "member_me", Actor: rbac.Subject{ID: currentUser.String(), Roles: rbac.RoleIdentifiers{rbac.RoleMember()}}}
	orgMemberMe := authSubject{Name: "org_member_me", Actor: rbac.Subject{ID: currentUser.String(), Roles: rbac.RoleIdentifiers{rbac.RoleMember(), rbac.ScopedRoleOrgMember(orgID)}}}
	orgMemberMeBanWorkspace := authSubject{Name: "org_member_me_workspace_ban", Actor: rbac.Subject{ID: currentUser.String(), Roles: rbac.RoleIdentifiers{rbac.RoleMember(), rbac.ScopedRoleOrgMember(orgID), rbac.ScopedRoleOrgWorkspaceCreationBan(orgID)}}}
	groupMemberMe := authSubject{Name: "group_member_me", Actor: rbac.Subject{ID: currentUser.String(), Roles: rbac.RoleIdentifiers{rbac.RoleMember(), rbac.ScopedRoleOrgMember(orgID)}, Groups: []string{groupID.String()}}}

	owner := authSubject{Name: "owner", Actor: rbac.Subject{ID: adminID.String(), Roles: rbac.RoleIdentifiers{rbac.RoleMember(), rbac.RoleOwner()}}}
	templateAdmin := authSubject{Name: "template-admin", Actor: rbac.Subject{ID: templateAdminID.String(), Roles: rbac.RoleIdentifiers{rbac.RoleMember(), rbac.RoleTemplateAdmin()}}}
	userAdmin := authSubject{Name: "user-admin", Actor: rbac.Subject{ID: userAdminID.String(), Roles: rbac.RoleIdentifiers{rbac.RoleMember(), rbac.RoleUserAdmin()}}}
	auditor := authSubject{Name: "auditor", Actor: rbac.Subject{ID: auditorID.String(), Roles: rbac.RoleIdentifiers{rbac.RoleMember(), rbac.RoleAuditor()}}}

	orgAdmin := authSubject{Name: "org_admin", Actor: rbac.Subject{ID: adminID.String(), Roles: rbac.RoleIdentifiers{rbac.RoleMember(), rbac.ScopedRoleOrgMember(orgID), rbac.ScopedRoleOrgAdmin(orgID)}}}
	orgAuditor := authSubject{Name: "org_auditor", Actor: rbac.Subject{ID: auditorID.String(), Roles: rbac.RoleIdentifiers{rbac.RoleMember(), rbac.ScopedRoleOrgMember(orgID), rbac.ScopedRoleOrgAuditor(orgID)}}}
	orgUserAdmin := authSubject{Name: "org_user_admin", Actor: rbac.Subject{ID: templateAdminID.String(), Roles: rbac.RoleIdentifiers{rbac.RoleMember(), rbac.ScopedRoleOrgMember(orgID), rbac.ScopedRoleOrgUserAdmin(orgID)}}}
	orgTemplateAdmin := authSubject{Name: "org_template_admin", Actor: rbac.Subject{ID: userAdminID.String(), Roles: rbac.RoleIdentifiers{rbac.RoleMember(), rbac.ScopedRoleOrgMember(orgID), rbac.ScopedRoleOrgTemplateAdmin(orgID)}}}
	setOrgNotMe := authSubjectSet{orgAdmin, orgAuditor, orgUserAdmin, orgTemplateAdmin}

	otherOrgMember := authSubject{Name: "org_member_other", Actor: rbac.Subject{ID: uuid.NewString(), Roles: rbac.RoleIdentifiers{rbac.RoleMember(), rbac.ScopedRoleOrgMember(otherOrg)}}}
	otherOrgAdmin := authSubject{Name: "org_admin_other", Actor: rbac.Subject{ID: uuid.NewString(), Roles: rbac.RoleIdentifiers{rbac.RoleMember(), rbac.ScopedRoleOrgMember(otherOrg), rbac.ScopedRoleOrgAdmin(otherOrg)}}}
	otherOrgAuditor := authSubject{Name: "org_auditor_other", Actor: rbac.Subject{ID: adminID.String(), Roles: rbac.RoleIdentifiers{rbac.RoleMember(), rbac.ScopedRoleOrgMember(otherOrg), rbac.ScopedRoleOrgAuditor(otherOrg)}}}
	otherOrgUserAdmin := authSubject{Name: "org_user_admin_other", Actor: rbac.Subject{ID: adminID.String(), Roles: rbac.RoleIdentifiers{rbac.RoleMember(), rbac.ScopedRoleOrgMember(otherOrg), rbac.ScopedRoleOrgUserAdmin(otherOrg)}}}
	otherOrgTemplateAdmin := authSubject{Name: "org_template_admin_other", Actor: rbac.Subject{ID: adminID.String(), Roles: rbac.RoleIdentifiers{rbac.RoleMember(), rbac.ScopedRoleOrgMember(otherOrg), rbac.ScopedRoleOrgTemplateAdmin(otherOrg)}}}
	setOtherOrg := authSubjectSet{otherOrgMember, otherOrgAdmin, otherOrgAuditor, otherOrgUserAdmin, otherOrgTemplateAdmin}

	// requiredSubjects are required to be asserted in each test case. This is
	// to make sure one is not forgotten.
	requiredSubjects := []authSubject{
		memberMe, owner,
		orgMemberMe, orgAdmin,
		otherOrgAdmin, otherOrgMember, orgAuditor, orgUserAdmin, orgTemplateAdmin,
		templateAdmin, userAdmin, otherOrgAuditor, otherOrgUserAdmin, otherOrgTemplateAdmin,
	}

	testCases := []struct {
		// Name the test case to better locate the failing test case.
		Name     string
		Resource rbac.Object
		Actions  []policy.Action
		// AuthorizeMap must cover all subjects in 'requiredSubjects'.
		// This map will run an Authorize() check with the resource, action,
		// and subjects. The subjects are split into 2 categories, "true" and
		// "false".
		//		true: Subjects who Authorize should return no error
		//		false: Subjects who Authorize should return forbidden.
		AuthorizeMap map[bool][]hasAuthSubjects
	}{
		{
			Name:     "MyUser",
			Actions:  []policy.Action{policy.ActionRead},
			Resource: rbac.ResourceUserObject(currentUser),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true: {orgMemberMe, owner, memberMe, templateAdmin, userAdmin, orgUserAdmin, otherOrgAdmin, otherOrgUserAdmin, orgAdmin},
				false: {
					orgTemplateAdmin, orgAuditor,
					otherOrgMember, otherOrgAuditor, otherOrgTemplateAdmin,
				},
			},
		},
		{
			Name:     "AUser",
			Actions:  []policy.Action{policy.ActionCreate, policy.ActionUpdate, policy.ActionDelete},
			Resource: rbac.ResourceUser,
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, userAdmin},
				false: {setOtherOrg, setOrgNotMe, memberMe, orgMemberMe, templateAdmin},
			},
		},
		{
			Name: "ReadMyWorkspaceInOrg",
			// When creating the WithID won't be set, but it does not change the result.
			Actions:  []policy.Action{policy.ActionRead},
			Resource: rbac.ResourceWorkspace.WithID(workspaceID).InOrg(orgID).WithOwner(currentUser.String()),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, orgMemberMe, orgAdmin, templateAdmin, orgTemplateAdmin, orgMemberMeBanWorkspace},
				false: {setOtherOrg, memberMe, userAdmin, orgAuditor, orgUserAdmin},
			},
		},
		{
			Name: "UpdateMyWorkspaceInOrg",
			// When creating the WithID won't be set, but it does not change the result.
			Actions:  []policy.Action{policy.ActionUpdate},
			Resource: rbac.ResourceWorkspace.WithID(workspaceID).InOrg(orgID).WithOwner(currentUser.String()),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, orgMemberMe, orgAdmin},
				false: {setOtherOrg, memberMe, userAdmin, templateAdmin, orgTemplateAdmin, orgUserAdmin, orgAuditor},
			},
		},
		{
			Name: "CreateDeleteMyWorkspaceInOrg",
			// When creating the WithID won't be set, but it does not change the result.
			Actions:  []policy.Action{policy.ActionCreate, policy.ActionDelete},
			Resource: rbac.ResourceWorkspace.WithID(workspaceID).InOrg(orgID).WithOwner(currentUser.String()),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, orgMemberMe, orgAdmin},
				false: {setOtherOrg, memberMe, userAdmin, templateAdmin, orgTemplateAdmin, orgUserAdmin, orgAuditor, orgMemberMeBanWorkspace},
			},
		},
		{
			Name: "MyWorkspaceInOrgExecution",
			// When creating the WithID won't be set, but it does not change the result.
			Actions:  []policy.Action{policy.ActionSSH},
			Resource: rbac.ResourceWorkspace.WithID(workspaceID).InOrg(orgID).WithOwner(currentUser.String()),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, orgMemberMe},
				false: {setOtherOrg, setOrgNotMe, memberMe, templateAdmin, userAdmin},
			},
		},
		{
			Name: "MyWorkspaceInOrgAppConnect",
			// When creating the WithID won't be set, but it does not change the result.
			Actions:  []policy.Action{policy.ActionApplicationConnect},
			Resource: rbac.ResourceWorkspace.WithID(workspaceID).InOrg(orgID).WithOwner(currentUser.String()),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, orgMemberMe},
				false: {setOtherOrg, setOrgNotMe, memberMe, templateAdmin, userAdmin},
			},
		},
		{
			Name:     "Templates",
			Actions:  []policy.Action{policy.ActionCreate, policy.ActionUpdate, policy.ActionDelete},
			Resource: rbac.ResourceTemplate.WithID(templateID).InOrg(orgID),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, orgAdmin, templateAdmin, orgTemplateAdmin},
				false: {setOtherOrg, orgUserAdmin, orgAuditor, memberMe, orgMemberMe, userAdmin},
			},
		},
		{
			Name:     "ReadTemplates",
			Actions:  []policy.Action{policy.ActionRead, policy.ActionViewInsights},
			Resource: rbac.ResourceTemplate.InOrg(orgID),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, orgAuditor, orgAdmin, templateAdmin, orgTemplateAdmin},
				false: {setOtherOrg, orgUserAdmin, memberMe, userAdmin, orgMemberMe},
			},
		},
		{
			Name:    "UseTemplates",
			Actions: []policy.Action{policy.ActionUse},
			Resource: rbac.ResourceTemplate.InOrg(orgID).WithGroupACL(map[string][]policy.Action{
				groupID.String(): {policy.ActionUse},
			}),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, orgAdmin, templateAdmin, orgTemplateAdmin, groupMemberMe},
				false: {setOtherOrg, orgAuditor, orgUserAdmin, memberMe, userAdmin, orgMemberMe},
			},
		},
		{
			Name:     "Files",
			Actions:  []policy.Action{policy.ActionCreate},
			Resource: rbac.ResourceFile.WithID(fileID),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true: {owner, templateAdmin},
				// Org template admins can only read org scoped files.
				// File scope is currently not org scoped :cry:
				false: {setOtherOrg, orgTemplateAdmin, orgMemberMe, orgAdmin, memberMe, userAdmin, orgAuditor, orgUserAdmin},
			},
		},
		{
			Name:     "MyFile",
			Actions:  []policy.Action{policy.ActionCreate, policy.ActionRead},
			Resource: rbac.ResourceFile.WithID(fileID).WithOwner(currentUser.String()),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, memberMe, orgMemberMe, templateAdmin},
				false: {setOtherOrg, setOrgNotMe, userAdmin},
			},
		},
		{
			Name:     "CreateOrganizations",
			Actions:  []policy.Action{policy.ActionCreate},
			Resource: rbac.ResourceOrganization,
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner},
				false: {setOtherOrg, setOrgNotMe, memberMe, orgMemberMe, templateAdmin, userAdmin},
			},
		},
		{
			Name:     "Organizations",
			Actions:  []policy.Action{policy.ActionUpdate, policy.ActionDelete},
			Resource: rbac.ResourceOrganization.WithID(orgID).InOrg(orgID),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, orgAdmin},
				false: {setOtherOrg, orgTemplateAdmin, orgUserAdmin, orgAuditor, memberMe, orgMemberMe, templateAdmin, userAdmin},
			},
		},
		{
			Name:     "ReadOrganizations",
			Actions:  []policy.Action{policy.ActionRead},
			Resource: rbac.ResourceOrganization.WithID(orgID).InOrg(orgID),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, orgAdmin, orgMemberMe, templateAdmin, orgTemplateAdmin, auditor, orgAuditor, userAdmin, orgUserAdmin},
				false: {setOtherOrg, memberMe},
			},
		},
		{
			Name:     "CreateUpdateDeleteCustomRole",
			Actions:  []policy.Action{policy.ActionCreate, policy.ActionUpdate, policy.ActionDelete},
			Resource: rbac.ResourceAssignOrgRole,
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner},
				false: {setOtherOrg, setOrgNotMe, userAdmin, orgMemberMe, memberMe, templateAdmin},
			},
		},
		{
			Name:     "RoleAssignment",
			Actions:  []policy.Action{policy.ActionAssign, policy.ActionUnassign},
			Resource: rbac.ResourceAssignRole,
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, userAdmin},
				false: {setOtherOrg, setOrgNotMe, orgMemberMe, memberMe, templateAdmin},
			},
		},
		{
			Name:     "ReadRoleAssignment",
			Actions:  []policy.Action{policy.ActionRead},
			Resource: rbac.ResourceAssignRole,
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {setOtherOrg, setOrgNotMe, owner, orgMemberMe, memberMe, templateAdmin, userAdmin},
				false: {},
			},
		},
		{
			Name:     "OrgRoleAssignment",
			Actions:  []policy.Action{policy.ActionAssign, policy.ActionUnassign},
			Resource: rbac.ResourceAssignOrgRole.InOrg(orgID),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, orgAdmin, userAdmin, orgUserAdmin},
				false: {setOtherOrg, orgMemberMe, memberMe, templateAdmin, orgTemplateAdmin, orgAuditor},
			},
		},
		{
			Name:     "CreateOrgRoleAssignment",
			Actions:  []policy.Action{policy.ActionCreate, policy.ActionUpdate},
			Resource: rbac.ResourceAssignOrgRole.InOrg(orgID),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, orgAdmin},
				false: {setOtherOrg, orgUserAdmin, orgTemplateAdmin, orgAuditor, orgMemberMe, memberMe, templateAdmin, userAdmin},
			},
		},
		{
			Name:     "ReadOrgRoleAssignment",
			Actions:  []policy.Action{policy.ActionRead},
			Resource: rbac.ResourceAssignOrgRole.InOrg(orgID),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, setOrgNotMe, orgMemberMe, userAdmin, templateAdmin},
				false: {setOtherOrg, memberMe},
			},
		},
		{
			Name:     "APIKey",
			Actions:  []policy.Action{policy.ActionCreate, policy.ActionRead, policy.ActionDelete, policy.ActionUpdate},
			Resource: rbac.ResourceApiKey.WithID(apiKeyID).WithOwner(currentUser.String()),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, orgMemberMe, memberMe},
				false: {setOtherOrg, setOrgNotMe, templateAdmin, userAdmin},
			},
		},
		{
			Name: "InboxNotification",
			Actions: []policy.Action{
				policy.ActionCreate, policy.ActionRead, policy.ActionUpdate,
			},
			Resource: rbac.ResourceInboxNotification.WithID(uuid.New()).InOrg(orgID).WithOwner(currentUser.String()),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, orgMemberMe, orgAdmin},
				false: {setOtherOrg, orgUserAdmin, orgTemplateAdmin, orgAuditor, templateAdmin, userAdmin, memberMe},
			},
		},
		{
			Name:     "UserData",
			Actions:  []policy.Action{policy.ActionReadPersonal, policy.ActionUpdatePersonal},
			Resource: rbac.ResourceUserObject(currentUser),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, orgMemberMe, memberMe, userAdmin},
				false: {setOtherOrg, setOrgNotMe, templateAdmin},
			},
		},
		{
			Name:     "ManageOrgMember",
			Actions:  []policy.Action{policy.ActionCreate, policy.ActionUpdate, policy.ActionDelete},
			Resource: rbac.ResourceOrganizationMember.WithID(currentUser).InOrg(orgID).WithOwner(currentUser.String()),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, orgAdmin, userAdmin, orgUserAdmin},
				false: {setOtherOrg, orgTemplateAdmin, orgAuditor, orgMemberMe, memberMe, templateAdmin},
			},
		},
		{
			Name:     "ReadOrgMember",
			Actions:  []policy.Action{policy.ActionRead},
			Resource: rbac.ResourceOrganizationMember.WithID(currentUser).InOrg(orgID).WithOwner(currentUser.String()),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, orgAuditor, orgAdmin, userAdmin, orgMemberMe, templateAdmin, orgUserAdmin, orgTemplateAdmin},
				false: {memberMe, setOtherOrg},
			},
		},
		{
			Name:    "AllUsersGroupACL",
			Actions: []policy.Action{policy.ActionRead},
			Resource: rbac.ResourceTemplate.WithID(templateID).InOrg(orgID).WithGroupACL(
				map[string][]policy.Action{
					orgID.String(): {policy.ActionRead},
				}),

			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, orgAdmin, orgMemberMe, templateAdmin, orgUserAdmin, orgTemplateAdmin, orgAuditor},
				false: {setOtherOrg, memberMe, userAdmin},
			},
		},
		{
			Name:    "Groups",
			Actions: []policy.Action{policy.ActionCreate, policy.ActionDelete, policy.ActionUpdate},
			Resource: rbac.ResourceGroup.WithID(groupID).InOrg(orgID).WithGroupACL(map[string][]policy.Action{
				groupID.String(): {
					policy.ActionRead,
				},
			}),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, orgAdmin, userAdmin, orgUserAdmin},
				false: {setOtherOrg, memberMe, orgMemberMe, templateAdmin, orgTemplateAdmin, groupMemberMe, orgAuditor},
			},
		},
		{
			Name:    "GroupsRead",
			Actions: []policy.Action{policy.ActionRead},
			Resource: rbac.ResourceGroup.WithID(groupID).InOrg(orgID).WithGroupACL(map[string][]policy.Action{
				groupID.String(): {
					policy.ActionRead,
				},
			}),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, orgAdmin, userAdmin, templateAdmin, orgTemplateAdmin, orgUserAdmin, groupMemberMe, orgAuditor},
				false: {setOtherOrg, memberMe, orgMemberMe},
			},
		},
		{
			Name:     "GroupMemberMeRead",
			Actions:  []policy.Action{policy.ActionRead},
			Resource: rbac.ResourceGroupMember.WithID(currentUser).InOrg(orgID).WithOwner(currentUser.String()),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, orgAuditor, orgAdmin, userAdmin, templateAdmin, orgTemplateAdmin, orgUserAdmin, orgMemberMe, groupMemberMe},
				false: {setOtherOrg, memberMe},
			},
		},
		{
			Name:     "GroupMemberOtherRead",
			Actions:  []policy.Action{policy.ActionRead},
			Resource: rbac.ResourceGroupMember.WithID(adminID).InOrg(orgID).WithOwner(adminID.String()),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, orgAuditor, orgAdmin, userAdmin, templateAdmin, orgTemplateAdmin, orgUserAdmin},
				false: {setOtherOrg, memberMe, orgMemberMe, groupMemberMe},
			},
		},
		{
			Name:     "WorkspaceDormant",
			Actions:  append(crud, policy.ActionWorkspaceStop),
			Resource: rbac.ResourceWorkspaceDormant.WithID(uuid.New()).InOrg(orgID).WithOwner(memberMe.Actor.ID),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {orgMemberMe, orgAdmin, owner},
				false: {setOtherOrg, userAdmin, memberMe, templateAdmin, orgTemplateAdmin, orgUserAdmin, orgAuditor},
			},
		},
		{
			Name:     "WorkspaceDormantUse",
			Actions:  []policy.Action{policy.ActionWorkspaceStart, policy.ActionApplicationConnect, policy.ActionSSH},
			Resource: rbac.ResourceWorkspaceDormant.WithID(uuid.New()).InOrg(orgID).WithOwner(memberMe.Actor.ID),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {},
				false: {setOtherOrg, setOrgNotMe, memberMe, userAdmin, orgMemberMe, owner, templateAdmin},
			},
		},
		{
			Name:     "WorkspaceBuild",
			Actions:  []policy.Action{policy.ActionWorkspaceStart, policy.ActionWorkspaceStop},
			Resource: rbac.ResourceWorkspace.WithID(uuid.New()).InOrg(orgID).WithOwner(memberMe.Actor.ID),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, orgAdmin, orgMemberMe},
				false: {setOtherOrg, userAdmin, templateAdmin, memberMe, orgTemplateAdmin, orgUserAdmin, orgAuditor},
			},
		},
		// Some admin style resources
		{
			Name:     "Licenses",
			Actions:  []policy.Action{policy.ActionCreate, policy.ActionRead, policy.ActionDelete},
			Resource: rbac.ResourceLicense,
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner},
				false: {setOtherOrg, setOrgNotMe, memberMe, orgMemberMe, templateAdmin, userAdmin},
			},
		},
		{
			Name:     "DeploymentStats",
			Actions:  []policy.Action{policy.ActionRead},
			Resource: rbac.ResourceDeploymentStats,
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner},
				false: {setOtherOrg, setOrgNotMe, memberMe, orgMemberMe, templateAdmin, userAdmin},
			},
		},
		{
			Name:     "DeploymentConfig",
			Actions:  []policy.Action{policy.ActionRead, policy.ActionUpdate},
			Resource: rbac.ResourceDeploymentConfig,
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner},
				false: {setOtherOrg, setOrgNotMe, memberMe, orgMemberMe, templateAdmin, userAdmin},
			},
		},
		{
			Name:     "DebugInfo",
			Actions:  []policy.Action{policy.ActionRead},
			Resource: rbac.ResourceDebugInfo,
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner},
				false: {setOtherOrg, setOrgNotMe, memberMe, orgMemberMe, templateAdmin, userAdmin},
			},
		},
		{
			Name:     "Replicas",
			Actions:  []policy.Action{policy.ActionRead},
			Resource: rbac.ResourceReplicas,
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner},
				false: {setOtherOrg, setOrgNotMe, memberMe, orgMemberMe, templateAdmin, userAdmin},
			},
		},
		{
			Name:     "TailnetCoordinator",
			Actions:  crud,
			Resource: rbac.ResourceTailnetCoordinator,
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner},
				false: {setOtherOrg, setOrgNotMe, memberMe, orgMemberMe, templateAdmin, userAdmin},
			},
		},
		{
			Name:     "AuditLogs",
			Actions:  []policy.Action{policy.ActionRead, policy.ActionCreate},
			Resource: rbac.ResourceAuditLog,
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner},
				false: {setOtherOrg, setOrgNotMe, memberMe, orgMemberMe, templateAdmin, userAdmin},
			},
		},
		{
			Name:     "ProvisionerDaemons",
			Actions:  []policy.Action{policy.ActionCreate, policy.ActionUpdate, policy.ActionDelete},
			Resource: rbac.ResourceProvisionerDaemon.InOrg(orgID),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, templateAdmin, orgAdmin, orgTemplateAdmin},
				false: {setOtherOrg, orgAuditor, orgUserAdmin, memberMe, orgMemberMe, userAdmin},
			},
		},
		{
			Name:     "ProvisionerDaemonsRead",
			Actions:  []policy.Action{policy.ActionRead},
			Resource: rbac.ResourceProvisionerDaemon.InOrg(orgID),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, templateAdmin, setOrgNotMe, orgMemberMe},
				false: {setOtherOrg, memberMe, userAdmin},
			},
		},
		{
			Name:     "UserProvisionerDaemons",
			Actions:  []policy.Action{policy.ActionCreate, policy.ActionUpdate, policy.ActionDelete},
			Resource: rbac.ResourceProvisionerDaemon.WithOwner(currentUser.String()).InOrg(orgID),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, templateAdmin, orgTemplateAdmin, orgMemberMe, orgAdmin},
				false: {setOtherOrg, memberMe, userAdmin, orgUserAdmin, orgAuditor},
			},
		},
		{
			Name:     "ProvisionerJobs",
			Actions:  []policy.Action{policy.ActionRead},
			Resource: rbac.ResourceProvisionerJobs.InOrg(orgID),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, orgTemplateAdmin, orgAdmin},
				false: {setOtherOrg, memberMe, orgMemberMe, templateAdmin, userAdmin, orgUserAdmin, orgAuditor},
			},
		},
		{
			Name:     "System",
			Actions:  crud,
			Resource: rbac.ResourceSystem,
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner},
				false: {setOtherOrg, setOrgNotMe, memberMe, orgMemberMe, templateAdmin, userAdmin},
			},
		},
		{
			Name:     "Oauth2App",
			Actions:  []policy.Action{policy.ActionCreate, policy.ActionUpdate, policy.ActionDelete},
			Resource: rbac.ResourceOauth2App,
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner},
				false: {setOtherOrg, setOrgNotMe, memberMe, orgMemberMe, templateAdmin, userAdmin},
			},
		},
		{
			Name:     "Oauth2AppRead",
			Actions:  []policy.Action{policy.ActionRead},
			Resource: rbac.ResourceOauth2App,
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, setOrgNotMe, setOtherOrg, memberMe, orgMemberMe, templateAdmin, userAdmin},
				false: {},
			},
		},
		{
			Name:     "Oauth2AppSecret",
			Actions:  crud,
			Resource: rbac.ResourceOauth2AppSecret,
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner},
				false: {setOrgNotMe, setOtherOrg, memberMe, orgMemberMe, templateAdmin, userAdmin},
			},
		},
		{
			Name:     "Oauth2Token",
			Actions:  []policy.Action{policy.ActionCreate, policy.ActionRead, policy.ActionDelete},
			Resource: rbac.ResourceOauth2AppCodeToken,
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner},
				false: {setOrgNotMe, setOtherOrg, memberMe, orgMemberMe, templateAdmin, userAdmin},
			},
		},
		{
			Name:     "WorkspaceProxy",
			Actions:  []policy.Action{policy.ActionCreate, policy.ActionUpdate, policy.ActionDelete},
			Resource: rbac.ResourceWorkspaceProxy,
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner},
				false: {setOrgNotMe, setOtherOrg, memberMe, orgMemberMe, templateAdmin, userAdmin},
			},
		},
		{
			Name:     "WorkspaceProxyRead",
			Actions:  []policy.Action{policy.ActionRead},
			Resource: rbac.ResourceWorkspaceProxy,
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, setOrgNotMe, setOtherOrg, memberMe, orgMemberMe, templateAdmin, userAdmin},
				false: {},
			},
		},
		{
			// Any owner/admin across may access any users' preferences
			// Members may not access other members' preferences
			Name:     "NotificationPreferencesOwn",
			Actions:  []policy.Action{policy.ActionRead, policy.ActionUpdate},
			Resource: rbac.ResourceNotificationPreference.WithOwner(currentUser.String()),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true: {memberMe, orgMemberMe, owner},
				false: {
					userAdmin, orgUserAdmin, templateAdmin,
					orgAuditor, orgTemplateAdmin,
					otherOrgMember, otherOrgAuditor, otherOrgUserAdmin, otherOrgTemplateAdmin,
					orgAdmin, otherOrgAdmin,
				},
			},
		},
		{
			// Any owner/admin may access notification templates
			Name:     "NotificationTemplates",
			Actions:  []policy.Action{policy.ActionRead, policy.ActionUpdate},
			Resource: rbac.ResourceNotificationTemplate,
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true: {owner},
				false: {
					memberMe, orgMemberMe, userAdmin, orgUserAdmin, templateAdmin,
					orgAuditor, orgTemplateAdmin,
					otherOrgMember, otherOrgAuditor, otherOrgUserAdmin, otherOrgTemplateAdmin,
					orgAdmin, otherOrgAdmin,
				},
			},
		},
		{
			Name:     "NotificationMessages",
			Actions:  []policy.Action{policy.ActionCreate, policy.ActionRead, policy.ActionUpdate, policy.ActionDelete},
			Resource: rbac.ResourceNotificationMessage,
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true: {owner},
				false: {
					memberMe, orgMemberMe, otherOrgMember,
					orgAdmin, otherOrgAdmin,
					orgAuditor, otherOrgAuditor,
					templateAdmin, orgTemplateAdmin, otherOrgTemplateAdmin,
					userAdmin, orgUserAdmin, otherOrgUserAdmin,
				},
			},
		},
		{
			// Notification preferences are currently not organization-scoped
			// Any owner/admin may access any users' preferences
			// Members may not access other members' preferences
			Name:     "NotificationPreferencesOtherUser",
			Actions:  []policy.Action{policy.ActionRead, policy.ActionUpdate},
			Resource: rbac.ResourceNotificationPreference.WithOwner(uuid.NewString()), // some other user
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true: {owner},
				false: {
					memberMe, templateAdmin, orgUserAdmin, userAdmin,
					orgAdmin, orgAuditor, orgTemplateAdmin,
					otherOrgMember, otherOrgAuditor, otherOrgUserAdmin, otherOrgTemplateAdmin,
					otherOrgAdmin, orgMemberMe,
				},
			},
		},
		// AnyOrganization tests
		{
			Name:     "CreateOrgMember",
			Actions:  []policy.Action{policy.ActionCreate},
			Resource: rbac.ResourceOrganizationMember.AnyOrganization(),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true: {owner, userAdmin, orgAdmin, otherOrgAdmin, orgUserAdmin, otherOrgUserAdmin},
				false: {
					memberMe, templateAdmin,
					orgTemplateAdmin, orgMemberMe, orgAuditor,
					otherOrgMember, otherOrgAuditor, otherOrgTemplateAdmin,
				},
			},
		},
		{
			Name:     "CreateTemplateAnyOrg",
			Actions:  []policy.Action{policy.ActionCreate},
			Resource: rbac.ResourceTemplate.AnyOrganization(),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true: {owner, templateAdmin, orgTemplateAdmin, otherOrgTemplateAdmin, orgAdmin, otherOrgAdmin},
				false: {
					userAdmin, memberMe,
					orgMemberMe, orgAuditor, orgUserAdmin,
					otherOrgMember, otherOrgAuditor, otherOrgUserAdmin,
				},
			},
		},
		{
			Name:     "CreateWorkspaceAnyOrg",
			Actions:  []policy.Action{policy.ActionCreate},
			Resource: rbac.ResourceWorkspace.AnyOrganization().WithOwner(currentUser.String()),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true: {owner, orgAdmin, otherOrgAdmin, orgMemberMe},
				false: {
					memberMe, userAdmin, templateAdmin,
					orgAuditor, orgUserAdmin, orgTemplateAdmin,
					otherOrgMember, otherOrgAuditor, otherOrgUserAdmin, otherOrgTemplateAdmin,
				},
			},
		},
		{
			Name:     "CryptoKeys",
			Actions:  []policy.Action{policy.ActionCreate, policy.ActionUpdate, policy.ActionDelete, policy.ActionRead},
			Resource: rbac.ResourceCryptoKey,
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner},
				false: {setOtherOrg, setOrgNotMe, memberMe, orgMemberMe, templateAdmin, userAdmin},
			},
		},
		{
			Name:     "IDPSyncSettings",
			Actions:  []policy.Action{policy.ActionRead, policy.ActionUpdate},
			Resource: rbac.ResourceIdpsyncSettings.InOrg(orgID),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true: {owner, orgAdmin, orgUserAdmin, userAdmin},
				false: {
					orgMemberMe, otherOrgAdmin,
					memberMe, templateAdmin,
					orgAuditor, orgTemplateAdmin,
					otherOrgMember, otherOrgAuditor, otherOrgUserAdmin, otherOrgTemplateAdmin,
				},
			},
		},
		{
			Name:     "OrganizationIDPSyncSettings",
			Actions:  []policy.Action{policy.ActionRead, policy.ActionUpdate},
			Resource: rbac.ResourceIdpsyncSettings,
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true: {owner, userAdmin},
				false: {
					orgAdmin, orgUserAdmin,
					orgMemberMe, otherOrgAdmin,
					memberMe, templateAdmin,
					orgAuditor, orgTemplateAdmin,
					otherOrgMember, otherOrgAuditor, otherOrgUserAdmin, otherOrgTemplateAdmin,
				},
			},
		},
		{
			Name:     "ResourceMonitor",
			Actions:  []policy.Action{policy.ActionRead, policy.ActionCreate, policy.ActionUpdate},
			Resource: rbac.ResourceWorkspaceAgentResourceMonitor,
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true: {owner},
				false: {
					memberMe, orgMemberMe, otherOrgMember,
					orgAdmin, otherOrgAdmin,
					orgAuditor, otherOrgAuditor,
					templateAdmin, orgTemplateAdmin, otherOrgTemplateAdmin,
					userAdmin, orgUserAdmin, otherOrgUserAdmin,
				},
			},
		},
	}

	// We expect every permission to be tested above.
	remainingPermissions := make(map[string]map[policy.Action]bool)
	for rtype, perms := range policy.RBACPermissions {
		remainingPermissions[rtype] = make(map[policy.Action]bool)
		for action := range perms.Actions {
			remainingPermissions[rtype][action] = true
		}
	}

	passed := true
	// nolint:tparallel,paralleltest
	for _, c := range testCases {
		c := c
		// nolint:tparallel,paralleltest // These share the same remainingPermissions map
		t.Run(c.Name, func(t *testing.T) {
			remainingSubjs := make(map[string]struct{})
			for _, subj := range requiredSubjects {
				remainingSubjs[subj.Name] = struct{}{}
			}

			for _, action := range c.Actions {
				err := c.Resource.ValidAction(action)
				ok := assert.NoError(t, err, "%q is not a valid action for type %q", action, c.Resource.Type)
				if !ok {
					passed = passed && assert.NoError(t, err, "%q is not a valid action for type %q", action, c.Resource.Type)
					continue
				}

				for result, sets := range c.AuthorizeMap {
					subjs := make([]authSubject, 0)
					for _, set := range sets {
						subjs = append(subjs, set.Subjects()...)
					}
					used := make(map[string]bool)

					for _, subj := range subjs {
						if _, ok := used[subj.Name]; ok {
							assert.False(t, true, "duplicate subject %q", subj.Name)
						}
						used[subj.Name] = true

						delete(remainingSubjs, subj.Name)
						msg := fmt.Sprintf("%s as %q doing %q on %q", c.Name, subj.Name, action, c.Resource.Type)
						// TODO: scopey
						actor := subj.Actor
						// Actor is missing some fields
						if actor.Scope == nil {
							actor.Scope = rbac.ScopeAll
						}

						delete(remainingPermissions[c.Resource.Type], action)
						err := auth.Authorize(context.Background(), actor, action, c.Resource)
						if result {
							passed = passed && assert.NoError(t, err, fmt.Sprintf("Should pass: %s", msg))
						} else {
							passed = passed && assert.ErrorContains(t, err, "forbidden", fmt.Sprintf("Should fail: %s", msg))
						}
					}
				}
			}
			require.Empty(t, remainingSubjs, "test should cover all subjects")
		})
	}

	// Only run these if the tests on top passed. Otherwise, the error output is too noisy.
	if passed {
		for rtype, v := range remainingPermissions {
			// nolint:tparallel,paralleltest // Making a subtest for easier diagnosing failures.
			t.Run(fmt.Sprintf("%s-AllActions", rtype), func(t *testing.T) {
				if len(v) > 0 {
					assert.Equal(t, map[policy.Action]bool{}, v, "remaining permissions should be empty for type %q", rtype)
				}
			})
		}
	}
}

func TestIsOrgRole(t *testing.T) {
	t.Parallel()
	randomUUID, err := uuid.Parse("cad8c09d-c099-4ec7-9263-7d52b1a3997a")
	require.NoError(t, err)

	testCases := []struct {
		Identifier rbac.RoleIdentifier
		OrgRole    bool
		OrgID      uuid.UUID
	}{
		// Not org roles
		{Identifier: rbac.RoleOwner()},
		{Identifier: rbac.RoleMember()},
		{Identifier: rbac.RoleAuditor()},
		{
			Identifier: rbac.RoleIdentifier{},
			OrgRole:    false,
		},

		// Org roles
		{
			Identifier: rbac.ScopedRoleOrgAdmin(randomUUID),
			OrgRole:    true,
			OrgID:      randomUUID,
		},
		{
			Identifier: rbac.ScopedRoleOrgMember(randomUUID),
			OrgRole:    true,
			OrgID:      randomUUID,
		},
	}

	// nolint:paralleltest
	for _, c := range testCases {
		c := c
		t.Run(c.Identifier.String(), func(t *testing.T) {
			t.Parallel()
			ok := c.Identifier.IsOrgRole()
			require.Equal(t, c.OrgRole, ok, "match expected org role")
			require.Equal(t, c.OrgID, c.Identifier.OrganizationID, "match expected org id")
		})
	}
}

func TestListRoles(t *testing.T) {
	t.Parallel()

	siteRoles := rbac.SiteRoles()
	siteRoleNames := make([]string, 0, len(siteRoles))
	for _, role := range siteRoles {
		siteRoleNames = append(siteRoleNames, role.Identifier.Name)
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
		orgRoleNames = append(orgRoleNames, role.Identifier.String())
	}

	require.ElementsMatch(t, []string{
		fmt.Sprintf("organization-admin:%s", orgID.String()),
		fmt.Sprintf("organization-member:%s", orgID.String()),
		fmt.Sprintf("organization-auditor:%s", orgID.String()),
		fmt.Sprintf("organization-user-admin:%s", orgID.String()),
		fmt.Sprintf("organization-template-admin:%s", orgID.String()),
		fmt.Sprintf("organization-workspace-creation-ban:%s", orgID.String()),
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

	convert := func(s []string) rbac.RoleIdentifiers {
		tmp := make([]rbac.RoleIdentifier, 0, len(s))
		for _, e := range s {
			tmp = append(tmp, rbac.RoleIdentifier{Name: e})
		}
		return tmp
	}

	for _, c := range testCases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()

			add, remove := rbac.ChangeRoleSet(convert(c.From), convert(c.To))
			require.ElementsMatch(t, convert(c.ExpAdd), add, "expect added")
			require.ElementsMatch(t, convert(c.ExpRemove), remove, "expect removed")
		})
	}
}

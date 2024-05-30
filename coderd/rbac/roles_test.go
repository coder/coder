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

type authSubject struct {
	// Name is helpful for test assertions
	Name  string
	Actor rbac.Subject
}

// TestBuiltInRoles makes sure our built-in roles are valid by our own policy
// rules. If this is incorrect, that is a mistake.
func TestBuiltInRoles(t *testing.T) {
	t.Parallel()
	for _, r := range rbac.SiteRoles() {
		r := r
		t.Run(r.Name, func(t *testing.T) {
			t.Parallel()
			require.NoError(t, r.Valid(), "invalid role")
		})
	}

	for _, r := range rbac.OrganizationRoles(uuid.New()) {
		r := r
		t.Run(r.Name, func(t *testing.T) {
			t.Parallel()
			require.NoError(t, r.Valid(), "invalid role")
		})
	}
}

//nolint:tparallel,paralleltest
func TestOwnerExec(t *testing.T) {
	owner := rbac.Subject{
		ID:    uuid.NewString(),
		Roles: rbac.RoleNames{rbac.RoleMember(), rbac.RoleOwner()},
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
	orgID := uuid.New()
	otherOrg := uuid.New()
	workspaceID := uuid.New()
	templateID := uuid.New()
	fileID := uuid.New()
	groupID := uuid.New()
	apiKeyID := uuid.New()

	// Subjects to user
	memberMe := authSubject{Name: "member_me", Actor: rbac.Subject{ID: currentUser.String(), Roles: rbac.RoleNames{rbac.RoleMember()}}}
	orgMemberMe := authSubject{Name: "org_member_me", Actor: rbac.Subject{ID: currentUser.String(), Roles: rbac.RoleNames{rbac.RoleMember(), rbac.RoleOrgMember(orgID)}}}

	owner := authSubject{Name: "owner", Actor: rbac.Subject{ID: adminID.String(), Roles: rbac.RoleNames{rbac.RoleMember(), rbac.RoleOwner()}}}
	orgAdmin := authSubject{Name: "org_admin", Actor: rbac.Subject{ID: adminID.String(), Roles: rbac.RoleNames{rbac.RoleMember(), rbac.RoleOrgMember(orgID), rbac.RoleOrgAdmin(orgID)}}}

	otherOrgMember := authSubject{Name: "org_member_other", Actor: rbac.Subject{ID: uuid.NewString(), Roles: rbac.RoleNames{rbac.RoleMember(), rbac.RoleOrgMember(otherOrg)}}}
	otherOrgAdmin := authSubject{Name: "org_admin_other", Actor: rbac.Subject{ID: uuid.NewString(), Roles: rbac.RoleNames{rbac.RoleMember(), rbac.RoleOrgMember(otherOrg), rbac.RoleOrgAdmin(otherOrg)}}}

	templateAdmin := authSubject{Name: "template-admin", Actor: rbac.Subject{ID: templateAdminID.String(), Roles: rbac.RoleNames{rbac.RoleMember(), rbac.RoleTemplateAdmin()}}}
	userAdmin := authSubject{Name: "user-admin", Actor: rbac.Subject{ID: templateAdminID.String(), Roles: rbac.RoleNames{rbac.RoleMember(), rbac.RoleUserAdmin()}}}

	// requiredSubjects are required to be asserted in each test case. This is
	// to make sure one is not forgotten.
	requiredSubjects := []authSubject{memberMe, owner, orgMemberMe, orgAdmin, otherOrgAdmin, otherOrgMember, templateAdmin, userAdmin}

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
		AuthorizeMap map[bool][]authSubject
	}{
		{
			Name:     "MyUser",
			Actions:  []policy.Action{policy.ActionRead},
			Resource: rbac.ResourceUserObject(currentUser),
			AuthorizeMap: map[bool][]authSubject{
				true:  {orgMemberMe, owner, memberMe, templateAdmin, userAdmin},
				false: {otherOrgMember, otherOrgAdmin, orgAdmin},
			},
		},
		{
			Name:     "AUser",
			Actions:  []policy.Action{policy.ActionCreate, policy.ActionUpdate, policy.ActionDelete},
			Resource: rbac.ResourceUser,
			AuthorizeMap: map[bool][]authSubject{
				true:  {owner, userAdmin},
				false: {memberMe, orgMemberMe, orgAdmin, otherOrgMember, otherOrgAdmin, templateAdmin},
			},
		},
		{
			Name: "ReadMyWorkspaceInOrg",
			// When creating the WithID won't be set, but it does not change the result.
			Actions:  []policy.Action{policy.ActionRead},
			Resource: rbac.ResourceWorkspace.WithID(workspaceID).InOrg(orgID).WithOwner(currentUser.String()),
			AuthorizeMap: map[bool][]authSubject{
				true:  {owner, orgMemberMe, orgAdmin, templateAdmin},
				false: {memberMe, otherOrgAdmin, otherOrgMember, userAdmin},
			},
		},
		{
			Name: "C_RDMyWorkspaceInOrg",
			// When creating the WithID won't be set, but it does not change the result.
			Actions:  []policy.Action{policy.ActionCreate, policy.ActionUpdate, policy.ActionDelete},
			Resource: rbac.ResourceWorkspace.WithID(workspaceID).InOrg(orgID).WithOwner(currentUser.String()),
			AuthorizeMap: map[bool][]authSubject{
				true:  {owner, orgMemberMe, orgAdmin},
				false: {memberMe, otherOrgAdmin, otherOrgMember, userAdmin, templateAdmin},
			},
		},
		{
			Name: "MyWorkspaceInOrgExecution",
			// When creating the WithID won't be set, but it does not change the result.
			Actions:  []policy.Action{policy.ActionSSH},
			Resource: rbac.ResourceWorkspace.WithID(workspaceID).InOrg(orgID).WithOwner(currentUser.String()),
			AuthorizeMap: map[bool][]authSubject{
				true:  {owner, orgMemberMe},
				false: {orgAdmin, memberMe, otherOrgAdmin, otherOrgMember, templateAdmin, userAdmin},
			},
		},
		{
			Name: "MyWorkspaceInOrgAppConnect",
			// When creating the WithID won't be set, but it does not change the result.
			Actions:  []policy.Action{policy.ActionApplicationConnect},
			Resource: rbac.ResourceWorkspace.WithID(workspaceID).InOrg(orgID).WithOwner(currentUser.String()),
			AuthorizeMap: map[bool][]authSubject{
				true:  {owner, orgMemberMe},
				false: {memberMe, otherOrgAdmin, otherOrgMember, templateAdmin, userAdmin, orgAdmin},
			},
		},
		{
			Name:     "Templates",
			Actions:  []policy.Action{policy.ActionCreate, policy.ActionUpdate, policy.ActionDelete, policy.ActionViewInsights},
			Resource: rbac.ResourceTemplate.WithID(templateID).InOrg(orgID),
			AuthorizeMap: map[bool][]authSubject{
				true:  {owner, orgAdmin, templateAdmin},
				false: {memberMe, orgMemberMe, otherOrgAdmin, otherOrgMember, userAdmin},
			},
		},
		{
			Name:     "ReadTemplates",
			Actions:  []policy.Action{policy.ActionRead},
			Resource: rbac.ResourceTemplate.InOrg(orgID),
			AuthorizeMap: map[bool][]authSubject{
				true:  {owner, orgAdmin, templateAdmin},
				false: {memberMe, otherOrgAdmin, otherOrgMember, userAdmin, orgMemberMe},
			},
		},
		{
			Name:     "Files",
			Actions:  []policy.Action{policy.ActionCreate},
			Resource: rbac.ResourceFile.WithID(fileID),
			AuthorizeMap: map[bool][]authSubject{
				true:  {owner, templateAdmin},
				false: {orgMemberMe, orgAdmin, memberMe, otherOrgAdmin, otherOrgMember, userAdmin},
			},
		},
		{
			Name:     "MyFile",
			Actions:  []policy.Action{policy.ActionCreate, policy.ActionRead},
			Resource: rbac.ResourceFile.WithID(fileID).WithOwner(currentUser.String()),
			AuthorizeMap: map[bool][]authSubject{
				true:  {owner, memberMe, orgMemberMe, templateAdmin},
				false: {orgAdmin, otherOrgAdmin, otherOrgMember, userAdmin},
			},
		},
		{
			Name:     "CreateOrganizations",
			Actions:  []policy.Action{policy.ActionCreate},
			Resource: rbac.ResourceOrganization,
			AuthorizeMap: map[bool][]authSubject{
				true:  {owner},
				false: {orgAdmin, otherOrgAdmin, otherOrgMember, memberMe, orgMemberMe, templateAdmin, userAdmin},
			},
		},
		{
			Name:     "Organizations",
			Actions:  []policy.Action{policy.ActionUpdate, policy.ActionDelete},
			Resource: rbac.ResourceOrganization.WithID(orgID).InOrg(orgID),
			AuthorizeMap: map[bool][]authSubject{
				true:  {owner, orgAdmin},
				false: {otherOrgAdmin, otherOrgMember, memberMe, orgMemberMe, templateAdmin, userAdmin},
			},
		},
		{
			Name:     "ReadOrganizations",
			Actions:  []policy.Action{policy.ActionRead},
			Resource: rbac.ResourceOrganization.WithID(orgID).InOrg(orgID),
			AuthorizeMap: map[bool][]authSubject{
				true:  {owner, orgAdmin, orgMemberMe, templateAdmin},
				false: {otherOrgAdmin, otherOrgMember, memberMe, userAdmin},
			},
		},
		{
			Name:     "CreateCustomRole",
			Actions:  []policy.Action{policy.ActionCreate},
			Resource: rbac.ResourceAssignRole,
			AuthorizeMap: map[bool][]authSubject{
				true:  {owner},
				false: {userAdmin, orgAdmin, orgMemberMe, otherOrgAdmin, otherOrgMember, memberMe, templateAdmin},
			},
		},
		{
			Name:     "RoleAssignment",
			Actions:  []policy.Action{policy.ActionAssign, policy.ActionDelete},
			Resource: rbac.ResourceAssignRole,
			AuthorizeMap: map[bool][]authSubject{
				true:  {owner, userAdmin},
				false: {orgAdmin, orgMemberMe, otherOrgAdmin, otherOrgMember, memberMe, templateAdmin},
			},
		},
		{
			Name:     "ReadRoleAssignment",
			Actions:  []policy.Action{policy.ActionRead},
			Resource: rbac.ResourceAssignRole,
			AuthorizeMap: map[bool][]authSubject{
				true:  {owner, orgAdmin, orgMemberMe, otherOrgAdmin, otherOrgMember, memberMe, templateAdmin, userAdmin},
				false: {},
			},
		},
		{
			Name:     "OrgRoleAssignment",
			Actions:  []policy.Action{policy.ActionAssign, policy.ActionDelete},
			Resource: rbac.ResourceAssignOrgRole.InOrg(orgID),
			AuthorizeMap: map[bool][]authSubject{
				true:  {owner, orgAdmin},
				false: {orgMemberMe, otherOrgAdmin, otherOrgMember, memberMe, templateAdmin, userAdmin},
			},
		},
		{
			Name:     "ReadOrgRoleAssignment",
			Actions:  []policy.Action{policy.ActionRead},
			Resource: rbac.ResourceAssignOrgRole.InOrg(orgID),
			AuthorizeMap: map[bool][]authSubject{
				true:  {owner, orgAdmin, orgMemberMe},
				false: {otherOrgAdmin, otherOrgMember, memberMe, templateAdmin, userAdmin},
			},
		},
		{
			Name:     "APIKey",
			Actions:  []policy.Action{policy.ActionCreate, policy.ActionRead, policy.ActionDelete, policy.ActionUpdate},
			Resource: rbac.ResourceApiKey.WithID(apiKeyID).WithOwner(currentUser.String()),
			AuthorizeMap: map[bool][]authSubject{
				true:  {owner, orgMemberMe, memberMe},
				false: {orgAdmin, otherOrgAdmin, otherOrgMember, templateAdmin, userAdmin},
			},
		},
		{
			Name:     "UserData",
			Actions:  []policy.Action{policy.ActionReadPersonal, policy.ActionUpdatePersonal},
			Resource: rbac.ResourceUserObject(currentUser),
			AuthorizeMap: map[bool][]authSubject{
				true:  {owner, orgMemberMe, memberMe, userAdmin},
				false: {orgAdmin, otherOrgAdmin, otherOrgMember, templateAdmin},
			},
		},
		{
			Name:     "ManageOrgMember",
			Actions:  []policy.Action{policy.ActionCreate, policy.ActionUpdate, policy.ActionDelete},
			Resource: rbac.ResourceOrganizationMember.WithID(currentUser).InOrg(orgID).WithOwner(currentUser.String()),
			AuthorizeMap: map[bool][]authSubject{
				true:  {owner, orgAdmin, userAdmin},
				false: {orgMemberMe, memberMe, otherOrgAdmin, otherOrgMember, templateAdmin},
			},
		},
		{
			Name:     "ReadOrgMember",
			Actions:  []policy.Action{policy.ActionRead},
			Resource: rbac.ResourceOrganizationMember.WithID(currentUser).InOrg(orgID).WithOwner(currentUser.String()),
			AuthorizeMap: map[bool][]authSubject{
				true:  {owner, orgAdmin, userAdmin, orgMemberMe, templateAdmin},
				false: {memberMe, otherOrgAdmin, otherOrgMember},
			},
		},
		{
			Name:    "AllUsersGroupACL",
			Actions: []policy.Action{policy.ActionRead},
			Resource: rbac.ResourceTemplate.WithID(templateID).InOrg(orgID).WithGroupACL(
				map[string][]policy.Action{
					orgID.String(): {policy.ActionRead},
				}),

			AuthorizeMap: map[bool][]authSubject{
				true:  {owner, orgAdmin, orgMemberMe, templateAdmin},
				false: {memberMe, otherOrgAdmin, otherOrgMember, userAdmin},
			},
		},
		{
			Name:     "Groups",
			Actions:  []policy.Action{policy.ActionCreate, policy.ActionDelete, policy.ActionUpdate},
			Resource: rbac.ResourceGroup.WithID(groupID).InOrg(orgID),
			AuthorizeMap: map[bool][]authSubject{
				true:  {owner, orgAdmin, userAdmin},
				false: {memberMe, otherOrgAdmin, orgMemberMe, otherOrgMember, templateAdmin},
			},
		},
		{
			Name:     "GroupsRead",
			Actions:  []policy.Action{policy.ActionRead},
			Resource: rbac.ResourceGroup.WithID(groupID).InOrg(orgID),
			AuthorizeMap: map[bool][]authSubject{
				true:  {owner, orgAdmin, userAdmin, templateAdmin},
				false: {memberMe, otherOrgAdmin, orgMemberMe, otherOrgMember},
			},
		},
		{
			Name:     "WorkspaceDormant",
			Actions:  append(crud, policy.ActionWorkspaceStop),
			Resource: rbac.ResourceWorkspaceDormant.WithID(uuid.New()).InOrg(orgID).WithOwner(memberMe.Actor.ID),
			AuthorizeMap: map[bool][]authSubject{
				true:  {orgMemberMe, orgAdmin, owner},
				false: {userAdmin, otherOrgAdmin, otherOrgMember, memberMe, templateAdmin},
			},
		},
		{
			Name:     "WorkspaceDormantUse",
			Actions:  []policy.Action{policy.ActionWorkspaceStart, policy.ActionApplicationConnect, policy.ActionSSH},
			Resource: rbac.ResourceWorkspaceDormant.WithID(uuid.New()).InOrg(orgID).WithOwner(memberMe.Actor.ID),
			AuthorizeMap: map[bool][]authSubject{
				true:  {},
				false: {memberMe, orgAdmin, userAdmin, otherOrgAdmin, otherOrgMember, orgMemberMe, owner, templateAdmin},
			},
		},
		{
			Name:     "WorkspaceBuild",
			Actions:  []policy.Action{policy.ActionWorkspaceStart, policy.ActionWorkspaceStop},
			Resource: rbac.ResourceWorkspace.WithID(uuid.New()).InOrg(orgID).WithOwner(memberMe.Actor.ID),
			AuthorizeMap: map[bool][]authSubject{
				true:  {owner, orgAdmin, orgMemberMe},
				false: {userAdmin, otherOrgAdmin, otherOrgMember, templateAdmin, memberMe},
			},
		},
		// Some admin style resources
		{
			Name:     "Licenses",
			Actions:  []policy.Action{policy.ActionCreate, policy.ActionRead, policy.ActionDelete},
			Resource: rbac.ResourceLicense,
			AuthorizeMap: map[bool][]authSubject{
				true:  {owner},
				false: {orgAdmin, otherOrgAdmin, otherOrgMember, memberMe, orgMemberMe, templateAdmin, userAdmin},
			},
		},
		{
			Name:     "DeploymentStats",
			Actions:  []policy.Action{policy.ActionRead},
			Resource: rbac.ResourceDeploymentStats,
			AuthorizeMap: map[bool][]authSubject{
				true:  {owner},
				false: {orgAdmin, otherOrgAdmin, otherOrgMember, memberMe, orgMemberMe, templateAdmin, userAdmin},
			},
		},
		{
			Name:     "DeploymentConfig",
			Actions:  []policy.Action{policy.ActionRead, policy.ActionUpdate},
			Resource: rbac.ResourceDeploymentConfig,
			AuthorizeMap: map[bool][]authSubject{
				true:  {owner},
				false: {orgAdmin, otherOrgAdmin, otherOrgMember, memberMe, orgMemberMe, templateAdmin, userAdmin},
			},
		},
		{
			Name:     "DebugInfo",
			Actions:  []policy.Action{policy.ActionRead},
			Resource: rbac.ResourceDebugInfo,
			AuthorizeMap: map[bool][]authSubject{
				true:  {owner},
				false: {orgAdmin, otherOrgAdmin, otherOrgMember, memberMe, orgMemberMe, templateAdmin, userAdmin},
			},
		},
		{
			Name:     "Replicas",
			Actions:  []policy.Action{policy.ActionRead},
			Resource: rbac.ResourceReplicas,
			AuthorizeMap: map[bool][]authSubject{
				true:  {owner},
				false: {orgAdmin, otherOrgAdmin, otherOrgMember, memberMe, orgMemberMe, templateAdmin, userAdmin},
			},
		},
		{
			Name:     "TailnetCoordinator",
			Actions:  crud,
			Resource: rbac.ResourceTailnetCoordinator,
			AuthorizeMap: map[bool][]authSubject{
				true:  {owner},
				false: {orgAdmin, otherOrgAdmin, otherOrgMember, memberMe, orgMemberMe, templateAdmin, userAdmin},
			},
		},
		{
			Name:     "AuditLogs",
			Actions:  []policy.Action{policy.ActionRead, policy.ActionCreate},
			Resource: rbac.ResourceAuditLog,
			AuthorizeMap: map[bool][]authSubject{
				true:  {owner},
				false: {orgAdmin, otherOrgAdmin, otherOrgMember, memberMe, orgMemberMe, templateAdmin, userAdmin},
			},
		},
		{
			Name:     "ProvisionerDaemons",
			Actions:  []policy.Action{policy.ActionCreate, policy.ActionUpdate, policy.ActionDelete},
			Resource: rbac.ResourceProvisionerDaemon.InOrg(orgID),
			AuthorizeMap: map[bool][]authSubject{
				true:  {owner, templateAdmin, orgAdmin},
				false: {otherOrgAdmin, otherOrgMember, memberMe, orgMemberMe, userAdmin},
			},
		},
		{
			Name:     "ProvisionerDaemonsRead",
			Actions:  []policy.Action{policy.ActionRead},
			Resource: rbac.ResourceProvisionerDaemon.InOrg(orgID),
			AuthorizeMap: map[bool][]authSubject{
				// This should be fixed when multi-org goes live
				true:  {owner, templateAdmin, orgAdmin, otherOrgAdmin, otherOrgMember, memberMe, orgMemberMe, userAdmin},
				false: {},
			},
		},
		{
			Name:     "UserProvisionerDaemons",
			Actions:  []policy.Action{policy.ActionCreate, policy.ActionUpdate, policy.ActionDelete},
			Resource: rbac.ResourceProvisionerDaemon.WithOwner(currentUser.String()).InOrg(orgID),
			AuthorizeMap: map[bool][]authSubject{
				true:  {owner, templateAdmin, orgMemberMe, orgAdmin},
				false: {memberMe, otherOrgAdmin, otherOrgMember, userAdmin},
			},
		},
		{
			Name:     "System",
			Actions:  crud,
			Resource: rbac.ResourceSystem,
			AuthorizeMap: map[bool][]authSubject{
				true:  {owner},
				false: {orgAdmin, otherOrgAdmin, otherOrgMember, memberMe, orgMemberMe, templateAdmin, userAdmin},
			},
		},
		{
			Name:     "Oauth2App",
			Actions:  []policy.Action{policy.ActionCreate, policy.ActionUpdate, policy.ActionDelete},
			Resource: rbac.ResourceOauth2App,
			AuthorizeMap: map[bool][]authSubject{
				true:  {owner},
				false: {orgAdmin, otherOrgAdmin, otherOrgMember, memberMe, orgMemberMe, templateAdmin, userAdmin},
			},
		},
		{
			Name:     "Oauth2AppRead",
			Actions:  []policy.Action{policy.ActionRead},
			Resource: rbac.ResourceOauth2App,
			AuthorizeMap: map[bool][]authSubject{
				true:  {owner, orgAdmin, otherOrgAdmin, otherOrgMember, memberMe, orgMemberMe, templateAdmin, userAdmin},
				false: {},
			},
		},
		{
			Name:     "Oauth2AppSecret",
			Actions:  crud,
			Resource: rbac.ResourceOauth2AppSecret,
			AuthorizeMap: map[bool][]authSubject{
				true:  {owner},
				false: {orgAdmin, otherOrgAdmin, otherOrgMember, memberMe, orgMemberMe, templateAdmin, userAdmin},
			},
		},
		{
			Name:     "Oauth2Token",
			Actions:  []policy.Action{policy.ActionCreate, policy.ActionRead, policy.ActionDelete},
			Resource: rbac.ResourceOauth2AppCodeToken,
			AuthorizeMap: map[bool][]authSubject{
				true:  {owner},
				false: {orgAdmin, otherOrgAdmin, otherOrgMember, memberMe, orgMemberMe, templateAdmin, userAdmin},
			},
		},
		{
			Name:     "WorkspaceProxy",
			Actions:  []policy.Action{policy.ActionCreate, policy.ActionUpdate, policy.ActionDelete},
			Resource: rbac.ResourceWorkspaceProxy,
			AuthorizeMap: map[bool][]authSubject{
				true:  {owner},
				false: {orgAdmin, otherOrgAdmin, otherOrgMember, memberMe, orgMemberMe, templateAdmin, userAdmin},
			},
		},
		{
			Name:     "WorkspaceProxyRead",
			Actions:  []policy.Action{policy.ActionRead},
			Resource: rbac.ResourceWorkspaceProxy,
			AuthorizeMap: map[bool][]authSubject{
				true:  {owner, orgAdmin, otherOrgAdmin, otherOrgMember, memberMe, orgMemberMe, templateAdmin, userAdmin},
				false: {},
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

				for result, subjs := range c.AuthorizeMap {
					for _, subj := range subjs {
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
		c := c
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

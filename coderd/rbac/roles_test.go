package rbac_test

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
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
	for _, r := range rbac.SiteBuiltInRoles() {
		t.Run(r.Identifier.String(), func(t *testing.T) {
			t.Parallel()
			require.NoError(t, r.Valid(), "invalid role")
		})
	}

	for _, r := range rbac.OrganizationRoles(uuid.New()) {
		t.Run(r.Identifier.String(), func(t *testing.T) {
			t.Parallel()
			require.NoError(t, r.Valid(), "invalid role")
		})
	}

	t.Run("agents-access", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, rbac.AgentsAccessRole().Valid(), "invalid role")
	})
}

// permissionGranted checks whether a permission list contains a
// matching entry for the target, accounting for wildcard actions.
// It does not evaluate negations that may override a positive grant.
func permissionGranted(perms []rbac.Permission, target rbac.Permission) bool {
	return slices.ContainsFunc(perms, func(p rbac.Permission) bool {
		return p.Negate == target.Negate &&
			p.ResourceType == target.ResourceType &&
			(p.Action == target.Action || p.Action == policy.WildcardSymbol)
	})
}

func TestOrgSharingPermissions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		permsFunc         func(rbac.OrgSettings) rbac.OrgRolePermissions
		mode              rbac.ShareableWorkspaceOwners
		orgReadMembers    bool
		orgReadGroups     bool
		orgNegateShare    bool
		memberNegateShare bool
	}{
		{"Member/Everyone", rbac.OrgMemberPermissions, rbac.ShareableWorkspaceOwnersEveryone, true, true, false, false},
		{"Member/None", rbac.OrgMemberPermissions, rbac.ShareableWorkspaceOwnersNone, false, false, true, true},
		{"Member/ServiceAccounts", rbac.OrgMemberPermissions, rbac.ShareableWorkspaceOwnersServiceAccounts, true, false, false, true},
		{"ServiceAccount/Everyone", rbac.OrgServiceAccountPermissions, rbac.ShareableWorkspaceOwnersEveryone, true, true, false, false},
		{"ServiceAccount/None", rbac.OrgServiceAccountPermissions, rbac.ShareableWorkspaceOwnersNone, false, false, true, false},
		{"ServiceAccount/ServiceAccounts", rbac.OrgServiceAccountPermissions, rbac.ShareableWorkspaceOwnersServiceAccounts, true, true, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			perms := tt.permsFunc(rbac.OrgSettings{
				ShareableWorkspaceOwners: tt.mode,
			})

			assert.Equal(t, tt.orgReadMembers, permissionGranted(perms.Org, rbac.Permission{
				ResourceType: rbac.ResourceOrganizationMember.Type,
				Action:       policy.ActionRead,
			}), "org read members")

			assert.Equal(t, tt.orgReadGroups, permissionGranted(perms.Org, rbac.Permission{
				ResourceType: rbac.ResourceGroup.Type,
				Action:       policy.ActionRead,
			}), "org read groups")

			assert.Equal(t, tt.orgNegateShare, permissionGranted(perms.Org, rbac.Permission{
				Negate:       true,
				ResourceType: rbac.ResourceWorkspace.Type,
				Action:       policy.ActionShare,
			}), "org negate share")

			assert.Equal(t, tt.memberNegateShare, permissionGranted(perms.Member, rbac.Permission{
				Negate:       true,
				ResourceType: rbac.ResourceWorkspace.Type,
				Action:       policy.ActionShare,
			}), "member negate share")
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

// These were "pared down" in https://github.com/coder/coder/pull/21359 to avoid
// using the now DB-backed organization-member role. As a result, they no longer
// model real-world org-scoped users (who also have organization-member).
//
// For example, `org_auditor` is now expected to be forbidden for
// `assign_org_role:read`, even though in production an org auditor can read
// available org roles via the org-member baseline.
//
// The tests are still useful for unit-testing the built-in roles in isolation.
//
// TODO(geokat): Add an integration test that includes organization-member to
// recover the old test coverage.
//
// nolint:tparallel,paralleltest // subtests share a map, just run sequentially.
func TestRolePermissions(t *testing.T) {
	t.Parallel()

	crud := []policy.Action{policy.ActionCreate, policy.ActionRead, policy.ActionUpdate, policy.ActionDelete}

	auth := rbac.NewStrictAuthorizer(prometheus.NewRegistry())

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
	memberMe := authSubject{Name: "member_me", Actor: rbac.Subject{ID: currentUser.String(), Roles: rbac.RoleIdentifiers{rbac.RoleMember()}, Scope: rbac.ScopeAll}.WithCachedASTValue()}

	owner := authSubject{Name: "owner", Actor: rbac.Subject{ID: adminID.String(), Roles: rbac.RoleIdentifiers{rbac.RoleMember(), rbac.RoleOwner()}, Scope: rbac.ScopeAll}.WithCachedASTValue()}
	templateAdmin := authSubject{Name: "template-admin", Actor: rbac.Subject{ID: templateAdminID.String(), Roles: rbac.RoleIdentifiers{rbac.RoleMember(), rbac.RoleTemplateAdmin()}, Scope: rbac.ScopeAll}.WithCachedASTValue()}
	userAdmin := authSubject{Name: "user-admin", Actor: rbac.Subject{ID: userAdminID.String(), Roles: rbac.RoleIdentifiers{rbac.RoleMember(), rbac.RoleUserAdmin()}, Scope: rbac.ScopeAll}.WithCachedASTValue()}
	auditor := authSubject{Name: "auditor", Actor: rbac.Subject{ID: auditorID.String(), Roles: rbac.RoleIdentifiers{rbac.RoleMember(), rbac.RoleAuditor()}, Scope: rbac.ScopeAll}.WithCachedASTValue()}

	orgAdmin := authSubject{Name: "org_admin", Actor: rbac.Subject{ID: adminID.String(), Roles: rbac.RoleIdentifiers{rbac.RoleMember(), rbac.ScopedRoleOrgAdmin(orgID)}, Scope: rbac.ScopeAll}.WithCachedASTValue()}
	orgAuditor := authSubject{Name: "org_auditor", Actor: rbac.Subject{ID: auditorID.String(), Roles: rbac.RoleIdentifiers{rbac.RoleMember(), rbac.ScopedRoleOrgAuditor(orgID)}, Scope: rbac.ScopeAll}.WithCachedASTValue()}
	orgUserAdmin := authSubject{Name: "org_user_admin", Actor: rbac.Subject{ID: templateAdminID.String(), Roles: rbac.RoleIdentifiers{rbac.RoleMember(), rbac.ScopedRoleOrgUserAdmin(orgID)}, Scope: rbac.ScopeAll}.WithCachedASTValue()}
	orgTemplateAdmin := authSubject{Name: "org_template_admin", Actor: rbac.Subject{ID: userAdminID.String(), Roles: rbac.RoleIdentifiers{rbac.RoleMember(), rbac.ScopedRoleOrgTemplateAdmin(orgID)}, Scope: rbac.ScopeAll}.WithCachedASTValue()}
	orgAdminBanWorkspace := authSubject{Name: "org_admin_workspace_ban", Actor: rbac.Subject{ID: adminID.String(), Roles: rbac.RoleIdentifiers{rbac.RoleMember(), rbac.ScopedRoleOrgAdmin(orgID), rbac.ScopedRoleOrgWorkspaceCreationBan(orgID)}, Scope: rbac.ScopeAll}.WithCachedASTValue()}
	agentsAccessUser := authSubject{Name: "chat_access", Actor: rbac.Subject{ID: currentUser.String(), Roles: rbac.RoleIdentifiers{rbac.RoleMember(), rbac.RoleAgentsAccess()}, Scope: rbac.ScopeAll}.WithCachedASTValue()}
	setOrgNotMe := authSubjectSet{orgAdmin, orgAuditor, orgUserAdmin, orgTemplateAdmin}

	otherOrgAdmin := authSubject{Name: "org_admin_other", Actor: rbac.Subject{ID: uuid.NewString(), Roles: rbac.RoleIdentifiers{rbac.RoleMember(), rbac.ScopedRoleOrgAdmin(otherOrg)}, Scope: rbac.ScopeAll}.WithCachedASTValue()}
	otherOrgAuditor := authSubject{Name: "org_auditor_other", Actor: rbac.Subject{ID: adminID.String(), Roles: rbac.RoleIdentifiers{rbac.RoleMember(), rbac.ScopedRoleOrgAuditor(otherOrg)}, Scope: rbac.ScopeAll}.WithCachedASTValue()}
	otherOrgUserAdmin := authSubject{Name: "org_user_admin_other", Actor: rbac.Subject{ID: adminID.String(), Roles: rbac.RoleIdentifiers{rbac.RoleMember(), rbac.ScopedRoleOrgUserAdmin(otherOrg)}, Scope: rbac.ScopeAll}.WithCachedASTValue()}
	otherOrgTemplateAdmin := authSubject{Name: "org_template_admin_other", Actor: rbac.Subject{ID: adminID.String(), Roles: rbac.RoleIdentifiers{rbac.RoleMember(), rbac.ScopedRoleOrgTemplateAdmin(otherOrg)}, Scope: rbac.ScopeAll}.WithCachedASTValue()}
	setOtherOrg := authSubjectSet{otherOrgAdmin, otherOrgAuditor, otherOrgUserAdmin, otherOrgTemplateAdmin}

	// requiredSubjects are required to be asserted in each test case. This is
	// to make sure one is not forgotten.
	requiredSubjects := []authSubject{
		memberMe, owner, agentsAccessUser,
		orgAdmin, otherOrgAdmin, orgAuditor, orgUserAdmin, orgTemplateAdmin,
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
				true: {owner, memberMe, agentsAccessUser, templateAdmin, userAdmin, orgUserAdmin, otherOrgAdmin, otherOrgUserAdmin, orgAdmin},
				false: {
					orgTemplateAdmin, orgAuditor,
					otherOrgAuditor, otherOrgTemplateAdmin,
				},
			},
		},
		{
			Name:     "AUser",
			Actions:  []policy.Action{policy.ActionCreate, policy.ActionUpdate, policy.ActionDelete},
			Resource: rbac.ResourceUser,
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, userAdmin},
				false: {setOtherOrg, setOrgNotMe, memberMe, agentsAccessUser, templateAdmin},
			},
		},
		{
			Name: "ReadMyWorkspaceInOrg",
			// When creating the WithID won't be set, but it does not change the result.
			Actions:  []policy.Action{policy.ActionRead},
			Resource: rbac.ResourceWorkspace.WithID(workspaceID).InOrg(orgID).WithOwner(currentUser.String()),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, orgAdmin, templateAdmin, orgTemplateAdmin, orgAdminBanWorkspace},
				false: {setOtherOrg, memberMe, agentsAccessUser, userAdmin, orgAuditor, orgUserAdmin},
			},
		},
		{
			Name: "UpdateMyWorkspaceInOrg",
			// When creating the WithID won't be set, but it does not change the result.
			Actions:  []policy.Action{policy.ActionUpdate},
			Resource: rbac.ResourceWorkspace.WithID(workspaceID).InOrg(orgID).WithOwner(currentUser.String()),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, orgAdmin, orgAdminBanWorkspace},
				false: {setOtherOrg, memberMe, agentsAccessUser, userAdmin, templateAdmin, orgTemplateAdmin, orgUserAdmin, orgAuditor},
			},
		},
		{
			Name: "CreateDeleteMyWorkspaceInOrg",
			// When creating the WithID won't be set, but it does not change the result.
			Actions:  []policy.Action{policy.ActionCreate, policy.ActionDelete},
			Resource: rbac.ResourceWorkspace.WithID(workspaceID).InOrg(orgID).WithOwner(currentUser.String()),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, orgAdmin},
				false: {setOtherOrg, memberMe, agentsAccessUser, userAdmin, templateAdmin, orgTemplateAdmin, orgUserAdmin, orgAuditor, orgAdminBanWorkspace},
			},
		},
		{
			Name: "CreateWorkspaceForMembers",
			// When creating the WithID won't be set, but it does not change the result.
			Actions:  []policy.Action{policy.ActionCreate},
			Resource: rbac.ResourceWorkspace.InOrg(orgID).WithOwner(policy.WildcardSymbol),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, orgAdmin},
				false: {setOtherOrg, orgUserAdmin, orgAuditor, memberMe, agentsAccessUser, userAdmin, templateAdmin, orgTemplateAdmin},
			},
		},
		{
			Name: "MyWorkspaceInOrgExecution",
			// When creating the WithID won't be set, but it does not change the result.
			Actions:  []policy.Action{policy.ActionSSH},
			Resource: rbac.ResourceWorkspace.WithID(workspaceID).InOrg(orgID).WithOwner(currentUser.String()),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner},
				false: {setOtherOrg, setOrgNotMe, memberMe, agentsAccessUser, templateAdmin, userAdmin},
			},
		},
		{
			Name: "MyWorkspaceInOrgAppConnect",
			// When creating the WithID won't be set, but it does not change the result.
			Actions:  []policy.Action{policy.ActionApplicationConnect},
			Resource: rbac.ResourceWorkspace.WithID(workspaceID).InOrg(orgID).WithOwner(currentUser.String()),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner},
				false: {setOtherOrg, setOrgNotMe, memberMe, agentsAccessUser, templateAdmin, userAdmin},
			},
		},
		{
			Name:     "CreateDeleteWorkspaceAgent",
			Actions:  []policy.Action{policy.ActionCreateAgent, policy.ActionDeleteAgent},
			Resource: rbac.ResourceWorkspace.WithID(workspaceID).InOrg(orgID).WithOwner(currentUser.String()),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, orgAdmin},
				false: {setOtherOrg, memberMe, agentsAccessUser, userAdmin, templateAdmin, orgTemplateAdmin, orgUserAdmin, orgAuditor, orgAdminBanWorkspace},
			},
		},
		{
			Name:     "UpdateWorkspaceAgent",
			Actions:  []policy.Action{policy.ActionUpdateAgent},
			Resource: rbac.ResourceWorkspace.WithID(workspaceID).InOrg(orgID).WithOwner(currentUser.String()),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, orgAdmin, orgAdminBanWorkspace},
				false: {setOtherOrg, memberMe, agentsAccessUser, userAdmin, templateAdmin, orgTemplateAdmin, orgUserAdmin, orgAuditor},
			},
		},
		{
			Name:    "ShareMyWorkspace",
			Actions: []policy.Action{policy.ActionShare},
			Resource: rbac.ResourceWorkspace.
				WithID(workspaceID).
				InOrg(orgID).
				WithOwner(currentUser.String()),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true: {owner, orgAdmin, orgAdminBanWorkspace},
				false: {
					memberMe, agentsAccessUser, setOtherOrg,
					templateAdmin, userAdmin,
					orgTemplateAdmin, orgUserAdmin, orgAuditor,
				},
			},
		},
		{
			Name:    "ShareWorkspaceDormant",
			Actions: []policy.Action{policy.ActionShare},
			Resource: rbac.ResourceWorkspaceDormant.
				WithID(uuid.New()).
				InOrg(orgID).
				WithOwner(memberMe.Actor.ID),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true: {},
				false: {
					orgAdmin, owner, setOtherOrg,
					userAdmin, memberMe, agentsAccessUser,
					templateAdmin, orgTemplateAdmin, orgUserAdmin, orgAuditor,
					orgAdminBanWorkspace,
				},
			},
		},
		{
			Name:     "Templates",
			Actions:  []policy.Action{policy.ActionCreate, policy.ActionUpdate, policy.ActionDelete},
			Resource: rbac.ResourceTemplate.WithID(templateID).InOrg(orgID),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, orgAdmin, templateAdmin, orgTemplateAdmin},
				false: {setOtherOrg, orgUserAdmin, orgAuditor, memberMe, agentsAccessUser, userAdmin},
			},
		},
		{
			Name:     "ReadTemplates",
			Actions:  []policy.Action{policy.ActionRead, policy.ActionViewInsights},
			Resource: rbac.ResourceTemplate.InOrg(orgID),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, orgAuditor, orgAdmin, templateAdmin, orgTemplateAdmin},
				false: {setOtherOrg, orgUserAdmin, memberMe, agentsAccessUser, userAdmin},
			},
		},
		{
			Name:    "UseTemplates",
			Actions: []policy.Action{policy.ActionUse},
			Resource: rbac.ResourceTemplate.InOrg(orgID).WithGroupACL(map[string][]policy.Action{
				groupID.String(): {policy.ActionUse},
			}),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, orgAdmin, templateAdmin, orgTemplateAdmin},
				false: {setOtherOrg, orgAuditor, orgUserAdmin, memberMe, agentsAccessUser, userAdmin},
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
				false: {setOtherOrg, orgTemplateAdmin, orgAdmin, memberMe, agentsAccessUser, userAdmin, orgAuditor, orgUserAdmin},
			},
		},
		{
			Name:     "MyFile",
			Actions:  []policy.Action{policy.ActionCreate, policy.ActionRead},
			Resource: rbac.ResourceFile.WithID(fileID).WithOwner(currentUser.String()),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, memberMe, agentsAccessUser, templateAdmin},
				false: {setOtherOrg, setOrgNotMe, userAdmin},
			},
		},
		{
			Name:     "CreateOrganizations",
			Actions:  []policy.Action{policy.ActionCreate},
			Resource: rbac.ResourceOrganization,
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner},
				false: {setOtherOrg, setOrgNotMe, memberMe, agentsAccessUser, templateAdmin, userAdmin},
			},
		},
		{
			Name:     "Organizations",
			Actions:  []policy.Action{policy.ActionUpdate, policy.ActionDelete},
			Resource: rbac.ResourceOrganization.WithID(orgID).InOrg(orgID),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, orgAdmin},
				false: {setOtherOrg, orgTemplateAdmin, orgUserAdmin, orgAuditor, memberMe, agentsAccessUser, templateAdmin, userAdmin},
			},
		},
		{
			Name:     "ReadOrganizations",
			Actions:  []policy.Action{policy.ActionRead},
			Resource: rbac.ResourceOrganization.WithID(orgID).InOrg(orgID),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, orgAdmin, templateAdmin, orgTemplateAdmin, auditor, orgAuditor, userAdmin, orgUserAdmin},
				false: {setOtherOrg, memberMe, agentsAccessUser},
			},
		},
		{
			Name:     "CreateUpdateDeleteCustomRole",
			Actions:  []policy.Action{policy.ActionCreate, policy.ActionUpdate, policy.ActionDelete},
			Resource: rbac.ResourceAssignOrgRole,
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner},
				false: {setOtherOrg, setOrgNotMe, userAdmin, memberMe, agentsAccessUser, templateAdmin},
			},
		},
		{
			Name:     "RoleAssignment",
			Actions:  []policy.Action{policy.ActionAssign, policy.ActionUnassign},
			Resource: rbac.ResourceAssignRole,
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, userAdmin},
				false: {setOtherOrg, setOrgNotMe, memberMe, agentsAccessUser, templateAdmin},
			},
		},
		{
			Name:     "ReadRoleAssignment",
			Actions:  []policy.Action{policy.ActionRead},
			Resource: rbac.ResourceAssignRole,
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {setOtherOrg, setOrgNotMe, owner, memberMe, agentsAccessUser, templateAdmin, userAdmin},
				false: {},
			},
		},
		{
			Name:     "OrgRoleAssignment",
			Actions:  []policy.Action{policy.ActionAssign, policy.ActionUnassign},
			Resource: rbac.ResourceAssignOrgRole.InOrg(orgID),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, orgAdmin, userAdmin, orgUserAdmin},
				false: {setOtherOrg, memberMe, agentsAccessUser, templateAdmin, orgTemplateAdmin, orgAuditor},
			},
		},
		{
			Name:     "CreateOrgRoleAssignment",
			Actions:  []policy.Action{policy.ActionCreate, policy.ActionUpdate},
			Resource: rbac.ResourceAssignOrgRole.InOrg(orgID),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, orgAdmin},
				false: {setOtherOrg, orgUserAdmin, orgTemplateAdmin, orgAuditor, memberMe, agentsAccessUser, templateAdmin, userAdmin},
			},
		},
		{
			Name:     "ReadOrgRoleAssignment",
			Actions:  []policy.Action{policy.ActionRead},
			Resource: rbac.ResourceAssignOrgRole.InOrg(orgID),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, orgAdmin, orgUserAdmin, userAdmin, templateAdmin},
				false: {setOtherOrg, memberMe, agentsAccessUser, orgAuditor, orgTemplateAdmin},
			},
		},
		{
			Name:     "APIKey",
			Actions:  []policy.Action{policy.ActionCreate, policy.ActionRead, policy.ActionDelete, policy.ActionUpdate},
			Resource: rbac.ResourceApiKey.WithID(apiKeyID).WithOwner(currentUser.String()),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, memberMe, agentsAccessUser},
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
				true:  {owner, orgAdmin},
				false: {setOtherOrg, orgUserAdmin, orgTemplateAdmin, orgAuditor, templateAdmin, userAdmin, memberMe, agentsAccessUser},
			},
		},
		{
			Name:     "UserData",
			Actions:  []policy.Action{policy.ActionReadPersonal, policy.ActionUpdatePersonal},
			Resource: rbac.ResourceUserObject(currentUser),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, memberMe, agentsAccessUser, userAdmin},
				false: {setOtherOrg, setOrgNotMe, templateAdmin},
			},
		},
		{
			Name:     "ManageOrgMember",
			Actions:  []policy.Action{policy.ActionCreate, policy.ActionUpdate, policy.ActionDelete},
			Resource: rbac.ResourceOrganizationMember.WithID(currentUser).InOrg(orgID).WithOwner(currentUser.String()),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, orgAdmin, userAdmin, orgUserAdmin},
				false: {setOtherOrg, orgTemplateAdmin, orgAuditor, memberMe, agentsAccessUser, templateAdmin},
			},
		},
		{
			Name:     "ReadOrgMember",
			Actions:  []policy.Action{policy.ActionRead},
			Resource: rbac.ResourceOrganizationMember.WithID(currentUser).InOrg(orgID).WithOwner(currentUser.String()),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, orgAuditor, orgAdmin, userAdmin, templateAdmin, orgUserAdmin, orgTemplateAdmin},
				false: {memberMe, agentsAccessUser, setOtherOrg},
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
				true:  {owner, orgAdmin, templateAdmin, orgUserAdmin, orgTemplateAdmin, orgAuditor},
				false: {setOtherOrg, memberMe, agentsAccessUser, userAdmin},
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
				false: {setOtherOrg, memberMe, agentsAccessUser, templateAdmin, orgTemplateAdmin, orgAuditor},
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
				true:  {owner, orgAdmin, userAdmin, templateAdmin, orgTemplateAdmin, orgUserAdmin, orgAuditor},
				false: {setOtherOrg, memberMe, agentsAccessUser},
			},
		},
		{
			Name:     "GroupMemberMeRead",
			Actions:  []policy.Action{policy.ActionRead},
			Resource: rbac.ResourceGroupMember.WithID(currentUser).InOrg(orgID).WithOwner(currentUser.String()),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, orgAuditor, orgAdmin, userAdmin, templateAdmin, orgTemplateAdmin, orgUserAdmin},
				false: {setOtherOrg, memberMe, agentsAccessUser},
			},
		},
		{
			Name:     "GroupMemberOtherRead",
			Actions:  []policy.Action{policy.ActionRead},
			Resource: rbac.ResourceGroupMember.WithID(adminID).InOrg(orgID).WithOwner(adminID.String()),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, orgAuditor, orgAdmin, userAdmin, templateAdmin, orgTemplateAdmin, orgUserAdmin},
				false: {setOtherOrg, memberMe, agentsAccessUser},
			},
		},
		{
			Name:     "WorkspaceDormant",
			Actions:  append(crud, policy.ActionWorkspaceStop, policy.ActionCreateAgent, policy.ActionDeleteAgent, policy.ActionUpdateAgent),
			Resource: rbac.ResourceWorkspaceDormant.WithID(uuid.New()).InOrg(orgID).WithOwner(memberMe.Actor.ID),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {orgAdmin, owner},
				false: {setOtherOrg, userAdmin, memberMe, agentsAccessUser, templateAdmin, orgTemplateAdmin, orgUserAdmin, orgAuditor},
			},
		},
		{
			Name:     "WorkspaceDormantUse",
			Actions:  []policy.Action{policy.ActionWorkspaceStart, policy.ActionApplicationConnect, policy.ActionSSH},
			Resource: rbac.ResourceWorkspaceDormant.WithID(uuid.New()).InOrg(orgID).WithOwner(memberMe.Actor.ID),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {},
				false: {setOtherOrg, setOrgNotMe, memberMe, agentsAccessUser, userAdmin, owner, templateAdmin},
			},
		},
		{
			Name:     "WorkspaceBuild",
			Actions:  []policy.Action{policy.ActionWorkspaceStart, policy.ActionWorkspaceStop},
			Resource: rbac.ResourceWorkspace.WithID(uuid.New()).InOrg(orgID).WithOwner(memberMe.Actor.ID),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, orgAdmin},
				false: {setOtherOrg, userAdmin, templateAdmin, memberMe, agentsAccessUser, orgTemplateAdmin, orgUserAdmin, orgAuditor},
			},
		},
		{
			Name:     "PrebuiltWorkspace",
			Actions:  []policy.Action{policy.ActionUpdate, policy.ActionDelete},
			Resource: rbac.ResourcePrebuiltWorkspace.WithID(uuid.New()).InOrg(orgID).WithOwner(database.PrebuildsSystemUserID.String()),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, orgAdmin, templateAdmin, orgTemplateAdmin},
				false: {setOtherOrg, userAdmin, memberMe, agentsAccessUser, orgUserAdmin, orgAuditor},
			},
		},
		{
			Name:     "Task",
			Actions:  crud,
			Resource: rbac.ResourceTask.WithID(uuid.New()).InOrg(orgID).WithOwner(memberMe.Actor.ID),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, orgAdmin},
				false: {setOtherOrg, userAdmin, templateAdmin, memberMe, agentsAccessUser, orgTemplateAdmin, orgUserAdmin, orgAuditor},
			},
		},
		// Some admin style resources
		{
			Name:     "Licenses",
			Actions:  []policy.Action{policy.ActionCreate, policy.ActionRead, policy.ActionDelete},
			Resource: rbac.ResourceLicense,
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner},
				false: {setOtherOrg, setOrgNotMe, memberMe, agentsAccessUser, templateAdmin, userAdmin},
			},
		},
		{
			Name:     "DeploymentStats",
			Actions:  []policy.Action{policy.ActionRead},
			Resource: rbac.ResourceDeploymentStats,
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner},
				false: {setOtherOrg, setOrgNotMe, memberMe, agentsAccessUser, templateAdmin, userAdmin},
			},
		},
		{
			Name:     "DeploymentConfig",
			Actions:  []policy.Action{policy.ActionRead, policy.ActionUpdate},
			Resource: rbac.ResourceDeploymentConfig,
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner},
				false: {setOtherOrg, setOrgNotMe, memberMe, agentsAccessUser, templateAdmin, userAdmin},
			},
		},
		{
			Name:     "DebugInfo",
			Actions:  []policy.Action{policy.ActionRead},
			Resource: rbac.ResourceDebugInfo,
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner},
				false: {setOtherOrg, setOrgNotMe, memberMe, agentsAccessUser, templateAdmin, userAdmin},
			},
		},
		{
			Name:     "Replicas",
			Actions:  []policy.Action{policy.ActionRead},
			Resource: rbac.ResourceReplicas,
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner},
				false: {setOtherOrg, setOrgNotMe, memberMe, agentsAccessUser, templateAdmin, userAdmin},
			},
		},
		{
			Name:     "TailnetCoordinator",
			Actions:  crud,
			Resource: rbac.ResourceTailnetCoordinator,
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner},
				false: {setOtherOrg, setOrgNotMe, memberMe, agentsAccessUser, templateAdmin, userAdmin},
			},
		},
		{
			Name:     "AuditLogs",
			Actions:  []policy.Action{policy.ActionRead, policy.ActionCreate},
			Resource: rbac.ResourceAuditLog,
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner},
				false: {setOtherOrg, setOrgNotMe, memberMe, agentsAccessUser, templateAdmin, userAdmin},
			},
		},
		{
			Name:     "ProvisionerDaemons",
			Actions:  []policy.Action{policy.ActionCreate, policy.ActionUpdate, policy.ActionDelete},
			Resource: rbac.ResourceProvisionerDaemon.InOrg(orgID),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, templateAdmin, orgAdmin, orgTemplateAdmin},
				false: {setOtherOrg, orgAuditor, orgUserAdmin, memberMe, agentsAccessUser, userAdmin},
			},
		},
		{
			Name:     "ProvisionerDaemonsRead",
			Actions:  []policy.Action{policy.ActionRead},
			Resource: rbac.ResourceProvisionerDaemon.InOrg(orgID),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, templateAdmin, orgAdmin, orgTemplateAdmin},
				false: {setOtherOrg, memberMe, agentsAccessUser, userAdmin, orgAuditor, orgUserAdmin},
			},
		},
		{
			Name:     "UserProvisionerDaemons",
			Actions:  []policy.Action{policy.ActionCreate, policy.ActionUpdate, policy.ActionDelete},
			Resource: rbac.ResourceProvisionerDaemon.WithOwner(currentUser.String()).InOrg(orgID),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, templateAdmin, orgTemplateAdmin, orgAdmin},
				false: {setOtherOrg, memberMe, agentsAccessUser, userAdmin, orgUserAdmin, orgAuditor},
			},
		},
		{
			Name:     "ProvisionerJobs",
			Actions:  []policy.Action{policy.ActionRead, policy.ActionUpdate, policy.ActionCreate},
			Resource: rbac.ResourceProvisionerJobs.InOrg(orgID),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, orgTemplateAdmin, orgAdmin},
				false: {setOtherOrg, memberMe, agentsAccessUser, templateAdmin, userAdmin, orgUserAdmin, orgAuditor},
			},
		},
		{
			Name:     "System",
			Actions:  crud,
			Resource: rbac.ResourceSystem,
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner},
				false: {setOtherOrg, setOrgNotMe, memberMe, agentsAccessUser, templateAdmin, userAdmin},
			},
		},
		{
			Name:     "Oauth2App",
			Actions:  []policy.Action{policy.ActionCreate, policy.ActionUpdate, policy.ActionDelete},
			Resource: rbac.ResourceOauth2App,
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner},
				false: {setOtherOrg, setOrgNotMe, memberMe, agentsAccessUser, templateAdmin, userAdmin},
			},
		},
		{
			Name:     "Oauth2AppRead",
			Actions:  []policy.Action{policy.ActionRead},
			Resource: rbac.ResourceOauth2App,
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, setOrgNotMe, setOtherOrg, memberMe, agentsAccessUser, templateAdmin, userAdmin},
				false: {},
			},
		},
		{
			Name:     "Oauth2AppSecret",
			Actions:  crud,
			Resource: rbac.ResourceOauth2AppSecret,
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner},
				false: {setOrgNotMe, setOtherOrg, memberMe, agentsAccessUser, templateAdmin, userAdmin},
			},
		},
		{
			Name:     "Oauth2Token",
			Actions:  []policy.Action{policy.ActionCreate, policy.ActionRead, policy.ActionDelete},
			Resource: rbac.ResourceOauth2AppCodeToken,
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner},
				false: {setOrgNotMe, setOtherOrg, memberMe, agentsAccessUser, templateAdmin, userAdmin},
			},
		},
		{
			Name:     "WorkspaceProxy",
			Actions:  []policy.Action{policy.ActionCreate, policy.ActionUpdate, policy.ActionDelete},
			Resource: rbac.ResourceWorkspaceProxy,
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner},
				false: {setOrgNotMe, setOtherOrg, memberMe, agentsAccessUser, templateAdmin, userAdmin},
			},
		},
		{
			Name:     "WorkspaceProxyRead",
			Actions:  []policy.Action{policy.ActionRead},
			Resource: rbac.ResourceWorkspaceProxy,
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, setOrgNotMe, setOtherOrg, memberMe, agentsAccessUser, templateAdmin, userAdmin},
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
				true: {memberMe, agentsAccessUser, owner},
				false: {
					userAdmin, orgUserAdmin, templateAdmin,
					orgAuditor, orgTemplateAdmin,
					otherOrgAuditor, otherOrgUserAdmin, otherOrgTemplateAdmin,
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
					memberMe, agentsAccessUser, userAdmin, orgUserAdmin, templateAdmin,
					orgAuditor, orgTemplateAdmin,
					otherOrgAuditor, otherOrgUserAdmin, otherOrgTemplateAdmin,
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
					memberMe, agentsAccessUser,
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
					memberMe, agentsAccessUser, templateAdmin, orgUserAdmin, userAdmin,
					orgAdmin, orgAuditor, orgTemplateAdmin,
					otherOrgAuditor, otherOrgUserAdmin, otherOrgTemplateAdmin,
					otherOrgAdmin,
				},
			},
		},
		// All users can create, read, and delete their own webpush notification subscriptions.
		{
			Name:     "WebpushSubscription",
			Actions:  []policy.Action{policy.ActionCreate, policy.ActionRead, policy.ActionDelete},
			Resource: rbac.ResourceWebpushSubscription.WithOwner(currentUser.String()),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner, memberMe, agentsAccessUser},
				false: {orgAdmin, otherOrgAdmin, orgAuditor, otherOrgAuditor, templateAdmin, orgTemplateAdmin, otherOrgTemplateAdmin, userAdmin, orgUserAdmin, otherOrgUserAdmin},
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
					memberMe, agentsAccessUser, templateAdmin,
					orgTemplateAdmin, orgAuditor,
					otherOrgAuditor, otherOrgTemplateAdmin,
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
					userAdmin, memberMe, agentsAccessUser,
					orgAuditor, orgUserAdmin,
					otherOrgAuditor, otherOrgUserAdmin,
				},
			},
		},
		{
			Name:     "CreateWorkspaceAnyOrg",
			Actions:  []policy.Action{policy.ActionCreate},
			Resource: rbac.ResourceWorkspace.AnyOrganization().WithOwner(currentUser.String()),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true: {owner, orgAdmin, otherOrgAdmin},
				false: {
					memberMe, agentsAccessUser, userAdmin, templateAdmin,
					orgAuditor, orgUserAdmin, orgTemplateAdmin,
					otherOrgAuditor, otherOrgUserAdmin, otherOrgTemplateAdmin,
				},
			},
		},
		{
			Name:     "CryptoKeys",
			Actions:  []policy.Action{policy.ActionCreate, policy.ActionUpdate, policy.ActionDelete, policy.ActionRead},
			Resource: rbac.ResourceCryptoKey,
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner},
				false: {setOtherOrg, setOrgNotMe, memberMe, agentsAccessUser, templateAdmin, userAdmin},
			},
		},
		{
			Name:     "IDPSyncSettings",
			Actions:  []policy.Action{policy.ActionRead, policy.ActionUpdate},
			Resource: rbac.ResourceIdpsyncSettings.InOrg(orgID),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true: {owner, orgAdmin, orgUserAdmin, userAdmin},
				false: {
					otherOrgAdmin,
					memberMe, agentsAccessUser, templateAdmin,
					orgAuditor, orgTemplateAdmin,
					otherOrgAuditor, otherOrgUserAdmin, otherOrgTemplateAdmin,
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
					otherOrgAdmin,
					memberMe, agentsAccessUser, templateAdmin,
					orgAuditor, orgTemplateAdmin,
					otherOrgAuditor, otherOrgUserAdmin, otherOrgTemplateAdmin,
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
					memberMe, agentsAccessUser,
					orgAdmin, otherOrgAdmin,
					orgAuditor, otherOrgAuditor,
					templateAdmin, orgTemplateAdmin, otherOrgTemplateAdmin,
					userAdmin, orgUserAdmin, otherOrgUserAdmin,
				},
			},
		},
		{
			Name:     "WorkspaceAgentDevcontainers",
			Actions:  []policy.Action{policy.ActionCreate},
			Resource: rbac.ResourceWorkspaceAgentDevcontainers,
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true: {owner},
				false: {
					memberMe, agentsAccessUser,
					orgAdmin, otherOrgAdmin,
					orgAuditor, otherOrgAuditor,
					templateAdmin, orgTemplateAdmin, otherOrgTemplateAdmin,
					userAdmin, orgUserAdmin, otherOrgUserAdmin,
				},
			},
		},
		{
			Name:     "ConnectionLogs",
			Actions:  []policy.Action{policy.ActionRead, policy.ActionUpdate},
			Resource: rbac.ResourceConnectionLog,
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true:  {owner},
				false: {setOtherOrg, setOrgNotMe, memberMe, agentsAccessUser, templateAdmin, userAdmin},
			},
		},
		// Only the user themselves can access their own secrets — no one else.
		{
			Name:     "UserSecrets",
			Actions:  []policy.Action{policy.ActionCreate, policy.ActionRead, policy.ActionUpdate, policy.ActionDelete},
			Resource: rbac.ResourceUserSecret.WithOwner(currentUser.String()),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true: {memberMe, agentsAccessUser},
				false: {
					owner, orgAdmin,
					otherOrgAdmin, orgAuditor, orgUserAdmin, orgTemplateAdmin,
					templateAdmin, userAdmin, otherOrgAuditor, otherOrgUserAdmin, otherOrgTemplateAdmin,
				},
			},
		},
		{
			Name:     "UsageEvents",
			Actions:  []policy.Action{policy.ActionCreate, policy.ActionRead, policy.ActionUpdate},
			Resource: rbac.ResourceUsageEvent,
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true: {},
				false: {
					owner,
					memberMe, agentsAccessUser,
					orgAdmin, otherOrgAdmin,
					orgAuditor, otherOrgAuditor,
					templateAdmin, orgTemplateAdmin, otherOrgTemplateAdmin,
					userAdmin, orgUserAdmin, otherOrgUserAdmin,
				},
			},
		},
		{
			// Members can create/update records but can't read them afterwards.
			Name:     "AIBridgeInterceptionsCreateUpdate",
			Actions:  []policy.Action{policy.ActionCreate, policy.ActionUpdate},
			Resource: rbac.ResourceAibridgeInterception.WithOwner(currentUser.String()),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true: {owner, memberMe, agentsAccessUser},
				false: {
					orgAdmin, otherOrgAdmin,
					orgAuditor, otherOrgAuditor,
					templateAdmin, orgTemplateAdmin, otherOrgTemplateAdmin,
					userAdmin, orgUserAdmin, otherOrgUserAdmin,
				},
			},
		},
		{
			// Only owners and site-wide auditors can view interceptions and their sub-resources.
			Name:     "AIBridgeInterceptionsRead",
			Actions:  []policy.Action{policy.ActionRead},
			Resource: rbac.ResourceAibridgeInterception.WithOwner(currentUser.String()),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true: {owner, auditor},
				false: {
					memberMe, agentsAccessUser,
					orgAdmin, otherOrgAdmin,
					orgAuditor, otherOrgAuditor,
					templateAdmin, orgTemplateAdmin, otherOrgTemplateAdmin,
					userAdmin, orgUserAdmin, otherOrgUserAdmin,
				},
			},
		},
		{
			Name:     "BoundaryUsage",
			Actions:  []policy.Action{policy.ActionRead, policy.ActionUpdate, policy.ActionDelete},
			Resource: rbac.ResourceBoundaryUsage,
			AuthorizeMap: map[bool][]hasAuthSubjects{
				false: {owner, setOtherOrg, setOrgNotMe, memberMe, agentsAccessUser, templateAdmin, userAdmin},
			},
		},
		{
			Name:     "ChatUsage",
			Actions:  []policy.Action{policy.ActionCreate, policy.ActionRead, policy.ActionUpdate, policy.ActionDelete},
			Resource: rbac.ResourceChat.WithOwner(currentUser.String()),
			AuthorizeMap: map[bool][]hasAuthSubjects{
				true: {owner, agentsAccessUser},
				false: {
					memberMe,
					orgAdmin, otherOrgAdmin,
					orgAuditor, otherOrgAuditor,
					templateAdmin, orgTemplateAdmin, otherOrgTemplateAdmin,
					userAdmin, orgUserAdmin, otherOrgUserAdmin,
				},
			},
		},
	}
	// Build coverage set from test case definitions statically,
	// so we don't need shared mutable state during execution.
	// This allows subtests to run in parallel.
	coveredPermissions := make(map[string]map[policy.Action]bool)
	for _, c := range testCases {
		for _, action := range c.Actions {
			if coveredPermissions[c.Resource.Type] == nil {
				coveredPermissions[c.Resource.Type] = make(map[policy.Action]bool)
			}
			coveredPermissions[c.Resource.Type][action] = true
		}
	}

	// Check coverage: every permission in policy.RBACPermissions must
	// be covered by at least one test case.
	for rtype, perms := range policy.RBACPermissions {
		t.Run(fmt.Sprintf("%s-AllActions", rtype), func(t *testing.T) {
			t.Parallel()
			for action := range perms.Actions {
				assert.True(t, coveredPermissions[rtype][action],
					"action %q on type %q is not tested", action, rtype)
			}
		})
	}

	for _, c := range testCases {
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()

			remainingSubjs := make(map[string]struct{})
			for _, subj := range requiredSubjects {
				remainingSubjs[subj.Name] = struct{}{}
			}

			for _, action := range c.Actions {
				err := c.Resource.ValidAction(action)
				if !assert.NoError(t, err, "%q is not a valid action for type %q", action, c.Resource.Type) {
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

						err := auth.Authorize(context.Background(), actor, action, c.Resource)
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

	siteRoles := rbac.SiteBuiltInRoles()
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
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()

			add, remove := rbac.ChangeRoleSet(convert(c.From), convert(c.To))
			require.ElementsMatch(t, convert(c.ExpAdd), add, "expect added")
			require.ElementsMatch(t, convert(c.ExpRemove), remove, "expect removed")
		})
	}
}

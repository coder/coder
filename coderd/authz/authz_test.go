package authz_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/authz"
)

// TestAuthorizeBasic test the very basic roles that are commonly used.
func TestAuthorizeBasic(t *testing.T) {
	t.Skip("TODO: unskip when rego is done")
	t.Parallel()
	defOrg := "default"
	defWorkspaceID := "1234"

	user := authz.SubjectTODO{
		UserID: "me",
		Roles:  []authz.Role{authz.RoleSiteMember, authz.OrgMember(defOrg)},
	}

	testAuthorize(t, "Member", user, []authTestCase{
		// Read my own resources
		{resource: authz.ResourceWorkspace.Owner(user.ID()), actions: authz.AllActions(), allow: true},
		// My workspace in my org
		{resource: authz.ResourceProject.Org(defOrg).Owner(user.ID()), actions: authz.AllActions(), allow: true},
		{resource: authz.ResourceProject.Org(defOrg).Owner(user.ID()).AsID(defWorkspaceID), actions: authz.AllActions(), allow: true},

		// Read resources in default org
		{resource: authz.ResourceWorkspace.Org(defOrg), actions: authz.AllActions(), allow: true},
		{resource: authz.ResourceProject.Org(defOrg), actions: authz.AllActions(), allow: true},

		// Objs in other orgs
		{resource: authz.ResourceWorkspace.Org("other"), actions: authz.AllActions(), allow: false},
		// Obj in other org owned by me
		{resource: authz.ResourceProject.Org("other").Owner(user.ID()), actions: authz.AllActions(), allow: false},
		{resource: authz.ResourceProject.Org("other").Owner(user.ID()).AsID(defWorkspaceID), actions: authz.AllActions(), allow: false},

		// Site wide
		{resource: authz.ResourceWorkspace, actions: authz.AllActions(), allow: false},
	})

	user = authz.SubjectTODO{
		UserID: "me",
		Roles:  []authz.Role{authz.RoleDenyAll},
	}

	testAuthorize(t, "DeletedMember", user, []authTestCase{
		// Read my own resources
		{resource: authz.ResourceWorkspace.Owner(user.ID()), actions: authz.AllActions(), allow: false},
		// My workspace in my org
		{resource: authz.ResourceWorkspace.Org(defOrg).Owner(user.ID()), actions: authz.AllActions(), allow: false},

		// Read resources in default org
		{resource: authz.ResourceWorkspace.Org(defOrg), actions: authz.AllActions(), allow: false},
		{resource: authz.ResourceWorkspace.Org(defOrg), actions: authz.AllActions(), allow: false},

		// Objs in other orgs
		{resource: authz.ResourceWorkspace.Org("other"), actions: authz.AllActions(), allow: false},
		// Obj in other org owned by me
		{resource: authz.ResourceProject.Org("other").Owner(user.ID()), actions: authz.AllActions(), allow: false},
		{resource: authz.ResourceProject.Org("other").Owner(user.ID()).AsID("1234"), actions: authz.AllActions(), allow: false},

		// Site wide
		{resource: authz.ResourceWorkspace, actions: authz.AllActions(), allow: false},
	})

	user = authz.SubjectTODO{
		UserID: "me",
		Roles: []authz.Role{
			authz.RoleOrgAdmin(defOrg),
			authz.RoleSiteMember,
		},
	}

	testAuthorize(t, "OrgAdmin", user, []authTestCase{
		// Read my own resources
		{resource: authz.ResourceWorkspace.Owner(user.ID()), actions: authz.AllActions(), allow: true},
		// My workspace in my org
		{resource: authz.ResourceWorkspace.Org(defOrg).Owner(user.ID()), actions: authz.AllActions(), allow: true},
		// Another workspace in my org
		{resource: authz.ResourceWorkspace.Org(defOrg).Owner("other"), actions: authz.AllActions(), allow: true},

		// Read resources in default org
		{resource: authz.ResourceWorkspace.Org(defOrg), actions: authz.AllActions(), allow: true},
		{resource: authz.ResourceProject.Org(defOrg), actions: authz.AllActions(), allow: true},

		// Objs in other orgs
		{resource: authz.ResourceWorkspace.Org("other"), actions: authz.AllActions(), allow: false},
		// Obj in other org owned by me
		{resource: authz.ResourceProject.Org("other").Owner(user.ID()), actions: authz.AllActions(), allow: false},
		{resource: authz.ResourceProject.Org("other").Owner(user.ID()).AsID("1234"), actions: authz.AllActions(), allow: false},

		// Site wide
		{resource: authz.ResourceWorkspace, actions: authz.AllActions(), allow: false},
	})

	user = authz.SubjectTODO{
		UserID: "me",
		Roles: []authz.Role{
			authz.RoleSiteAdmin,
			authz.RoleSiteMember,
		},
	}

	testAuthorize(t, "SiteAdmin", user, []authTestCase{
		// Read my own resources
		{resource: authz.ResourceWorkspace.Owner(user.ID()), actions: authz.AllActions(), allow: true},
		// My workspace in my org
		{resource: authz.ResourceWorkspace.Org(defOrg).Owner(user.ID()), actions: authz.AllActions(), allow: true},
		// Another workspace in my org
		{resource: authz.ResourceWorkspace.Org(defOrg).Owner("other"), actions: authz.AllActions(), allow: true},

		// Read resources in default org
		{resource: authz.ResourceWorkspace.Org(defOrg), actions: authz.AllActions(), allow: true},
		{resource: authz.ResourceProject.Org(defOrg), actions: authz.AllActions(), allow: true},

		// Objs in other orgs
		{resource: authz.ResourceWorkspace.Org("other"), actions: authz.AllActions(), allow: true},
		// Obj in other org owned by me
		{resource: authz.ResourceProject.Org("other").Owner(user.ID()), actions: authz.AllActions(), allow: true},
		{resource: authz.ResourceProject.Org("other").Owner(user.ID()).AsID("1234"), actions: authz.AllActions(), allow: true},

		// Site wide
		{resource: authz.ResourceWorkspace, actions: authz.AllActions(), allow: true},
	})

	// In practice this is a token scope on a regular subject
	user = authz.SubjectTODO{
		UserID: "me",
		Roles: []authz.Role{
			authz.RoleWorkspaceAgent(defWorkspaceID),
		},
	}

	testAuthorize(t, "WorkspaceAgentToken", user, []authTestCase{
		// Read workspace by ID
		{resource: authz.ResourceWorkspace.Org(defOrg).Owner(user.ID()).AsID(defWorkspaceID), actions: []authz.Action{authz.ActionRead}, allow: true},
		// C_UD
		{resource: authz.ResourceWorkspace.Org(defOrg).Owner(user.ID()).AsID(defWorkspaceID), actions: []authz.Action{authz.ActionCreate, authz.ActionUpdate, authz.ActionDelete}, allow: false},

		// another resource type
		{resource: authz.ResourceProject.Org(defOrg).Owner(user.ID()).AsID(defWorkspaceID), actions: authz.AllActions(), allow: false},

		{resource: authz.ResourceWorkspace.Owner(user.ID()), actions: authz.AllActions(), allow: false},
		{resource: authz.ResourceWorkspace.Org(defOrg).Owner(user.ID()), actions: authz.AllActions(), allow: false},
		{resource: authz.ResourceWorkspace.Org(defOrg).Owner("other"), actions: authz.AllActions(), allow: false},

		// Resources in default org
		{resource: authz.ResourceWorkspace.Org(defOrg), actions: authz.AllActions(), allow: false},
		{resource: authz.ResourceProject.Org(defOrg), actions: authz.AllActions(), allow: false},

		// Objs in other orgs
		{resource: authz.ResourceWorkspace.Org("other"), actions: authz.AllActions(), allow: false},
		// Obj in other org owned by me
		{resource: authz.ResourceProject.Org("other").Owner(user.ID()), actions: authz.AllActions(), allow: false},
		{resource: authz.ResourceProject.Org("other").Owner(user.ID()).AsID(defWorkspaceID), actions: authz.AllActions(), allow: false},

		// Site wide
		{resource: authz.ResourceWorkspace, actions: authz.AllActions(), allow: false},
	})

	// In practice this is a token scope on a regular subject
	user = authz.SubjectTODO{
		UserID: "me",
		Roles: []authz.Role{
			{
				Site: []authz.Permission{},
				Org: map[string][]authz.Permission{
					defOrg: {{
						Negate:       false,
						ResourceType: "*",
						ResourceID:   "*",
						Action:       authz.ActionRead,
					}},
				},
				User: []authz.Permission{
					{
						Negate:       false,
						ResourceType: "*",
						ResourceID:   "*",
						Action:       authz.ActionRead,
					},
				},
			},
		},
	}

	testAuthorize(t, "ReadOnly", user, []authTestCase{
		// Read
		{resource: authz.ResourceWorkspace.Org(defOrg).Owner(user.ID()).AsID(defWorkspaceID), actions: []authz.Action{authz.ActionRead}, allow: true},
		{resource: authz.ResourceWorkspace.Org(defOrg).Owner(user.ID()), actions: []authz.Action{authz.ActionRead}, allow: true},
		{resource: authz.ResourceWorkspace.Org(defOrg), actions: []authz.Action{authz.ActionRead}, allow: true},
		{resource: authz.ResourceWorkspace.Owner(user.ID()), actions: []authz.Action{authz.ActionRead}, allow: true},
		{resource: authz.ResourceWorkspace, actions: []authz.Action{authz.ActionRead}, allow: false},

		// Other
		{resource: authz.ResourceWorkspace.Org(defOrg).Owner(user.ID()).AsID(defWorkspaceID), actions: authz.AllActions(), allow: false},
		{resource: authz.ResourceWorkspace.Org(defOrg).Owner(user.ID()), actions: authz.AllActions(), allow: false},
		{resource: authz.ResourceWorkspace.Org(defOrg), actions: authz.AllActions(), allow: false},
		{resource: authz.ResourceWorkspace.Owner(user.ID()), actions: authz.AllActions(), allow: false},
		{resource: authz.ResourceWorkspace, actions: authz.AllActions(), allow: false},
	})
}

type authTestCase struct {
	resource authz.Resource
	actions  []authz.Action
	allow    bool
}

func testAuthorize(t *testing.T, name string, subject authz.Subject, cases []authTestCase) {
	for _, c := range cases {
		t.Run(name, func(t *testing.T) {
			for _, a := range c.actions {
				err := authz.Authorize(subject, c.resource, a)
				if c.allow {
					require.NoError(t, err, "expected no error for testcase action %s", a)
					continue
				}
				require.Error(t, err, "expected unauthorized")
			}
		})
	}
}

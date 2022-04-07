package authz_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/authz"
)

func TestAuthorize(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		subject  authz.Subject
		resource authz.Resource
		actions  []authz.Action
		error    string
	}{
		{
			name: "unauthenticated user cannot perform an action",
			subject: authz.SubjectTODO{
				UserID: "",
				Site:   []authz.Role{authz.RoleNoPerm},
			},
			resource: authz.ResourceWorkspace,
			actions:  []authz.Action{authz.ActionRead, authz.ActionCreate, authz.ActionDelete, authz.ActionUpdate},
			error:    "unauthorized",
		},
		{
			name: "admin can do anything",
			subject: authz.SubjectTODO{
				UserID: "admin",
				Site:   []authz.Role{authz.RoleAllowAll},
			},
			resource: authz.ResourceWorkspace,
			actions:  []authz.Action{authz.ActionRead, authz.ActionCreate, authz.ActionDelete, authz.ActionUpdate},
			error:    "",
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			for _, action := range testCase.actions {
				err := authz.Authorize(testCase.subject, testCase.resource, action)
				if testCase.error == "" {
					require.NoError(t, err, "expected no error for testcase testcase %q action %s", testCase.name, action)
					continue
				}
				require.EqualError(t, err, testCase.error, "unexpected error")
			}
		})
	}
}

// TestAuthorizeBasic test the very basic roles that are commonly used.
func TestAuthorizeBasic(t *testing.T) {
	t.Parallel()
	defOrg := "default"
	defWorkspaceID := "1234"

	user := authz.SubjectTODO{
		UserID: "me",
		Site:   []authz.Role{},
		Org: map[string][]authz.Role{
			defOrg: {},
		},
		User: []authz.Role{authz.RoleAllowAll},
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
		Site:   []authz.Role{authz.RoleBlockAll},
		Org: map[string][]authz.Role{
			defOrg: {},
		},
		User: []authz.Role{authz.RoleAllowAll},
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
		Site:   []authz.Role{},
		Org: map[string][]authz.Role{
			defOrg: {authz.RoleAllowAll},
		},
		User: []authz.Role{authz.RoleAllowAll},
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
		Site:   []authz.Role{authz.RoleAllowAll},
		Org:    map[string][]authz.Role{},
		User:   []authz.Role{authz.RoleAllowAll},
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
		Site:   []authz.Role{},
		Org: map[string][]authz.Role{
			defOrg: {},
		},
		User: []authz.Role{authz.WorkspaceAgentRole(defWorkspaceID)},
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
		Site:   []authz.Role{},
		Org: map[string][]authz.Role{
			defOrg: {authz.RoleReadOnly},
		},
		User: []authz.Role{authz.RoleReadOnly},
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

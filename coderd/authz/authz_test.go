package authz_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/authz"
)

// TestAuthorizeDomain test the very basic roles that are commonly used.
func TestAuthorizeDomain(t *testing.T) {
	t.Skip("TODO: unskip when rego is done")
	t.Parallel()
	defOrg := "default"
	wrkID := "1234"

	user := authz.SubjectTODO{
		UserID: "me",
		Roles:  []authz.Role{authz.RoleSiteMember, authz.RoleOrgMember(defOrg)},
	}

	testAuthorize(t, "Member", user, []authTestCase{
		// Org + me + id
		{resource: authz.ResourceWorkspace.SetOrg(defOrg).SetOwner(user.ID()).SetID(wrkID), actions: authz.AllActions(), allow: true},
		{resource: authz.ResourceWorkspace.SetOrg(defOrg).SetOwner(user.ID()), actions: authz.AllActions(), allow: true},
		{resource: authz.ResourceWorkspace.SetOrg(defOrg).SetID(wrkID), actions: authz.AllActions(), allow: false},
		{resource: authz.ResourceWorkspace.SetOrg(defOrg), actions: authz.AllActions(), allow: false},

		{resource: authz.ResourceWorkspace.SetOwner(user.ID()).SetID(wrkID), actions: authz.AllActions(), allow: true},
		{resource: authz.ResourceWorkspace.SetOwner(user.ID()), actions: authz.AllActions(), allow: true},

		{resource: authz.ResourceWorkspace.SetID(wrkID), actions: authz.AllActions(), allow: false},

		{resource: authz.ResourceWorkspace, actions: authz.AllActions(), allow: false},

		// Other org + me + id
		{resource: authz.ResourceWorkspace.SetOrg("other").SetOwner(user.ID()).SetID(wrkID), actions: authz.AllActions(), allow: false},
		{resource: authz.ResourceWorkspace.SetOrg("other").SetOwner(user.ID()), actions: authz.AllActions(), allow: false},
		{resource: authz.ResourceWorkspace.SetOrg("other").SetID(wrkID), actions: authz.AllActions(), allow: false},
		{resource: authz.ResourceWorkspace.SetOrg("other"), actions: authz.AllActions(), allow: false},

		// Other org + other user + id
		{resource: authz.ResourceWorkspace.SetOrg(defOrg).SetOwner("not-me").SetID(wrkID), actions: authz.AllActions(), allow: false},
		{resource: authz.ResourceWorkspace.SetOrg(defOrg).SetOwner("not-me"), actions: authz.AllActions(), allow: false},

		{resource: authz.ResourceWorkspace.SetOwner("not-me").SetID(wrkID), actions: authz.AllActions(), allow: false},
		{resource: authz.ResourceWorkspace.SetOwner("not-me"), actions: authz.AllActions(), allow: false},

		// Other org + other use + other id
		{resource: authz.ResourceWorkspace.SetOrg("other").SetOwner("not-me").SetID("not-id"), actions: authz.AllActions(), allow: false},
		{resource: authz.ResourceWorkspace.SetOrg("other").SetOwner("not-me"), actions: authz.AllActions(), allow: false},
		{resource: authz.ResourceWorkspace.SetOrg("other").SetID("not-id"), actions: authz.AllActions(), allow: false},
		{resource: authz.ResourceWorkspace.SetOrg("other"), actions: authz.AllActions(), allow: false},

		{resource: authz.ResourceWorkspace.SetOwner("not-me").SetID("not-id"), actions: authz.AllActions(), allow: false},
		{resource: authz.ResourceWorkspace.SetOwner("not-me"), actions: authz.AllActions(), allow: false},

		{resource: authz.ResourceWorkspace.SetID("not-id"), actions: authz.AllActions(), allow: false},
	})

	user = authz.SubjectTODO{
		UserID: "me",
		Roles:  []authz.Role{authz.RoleDenyAll},
	}

	testAuthorize(t, "DeletedMember", user, []authTestCase{
		// Org + me + id
		{resource: authz.ResourceWorkspace.SetOrg(defOrg).SetOwner(user.ID()).SetID(wrkID), actions: authz.AllActions(), allow: false},
		{resource: authz.ResourceWorkspace.SetOrg(defOrg).SetOwner(user.ID()), actions: authz.AllActions(), allow: false},
		{resource: authz.ResourceWorkspace.SetOrg(defOrg).SetID(wrkID), actions: authz.AllActions(), allow: false},
		{resource: authz.ResourceWorkspace.SetOrg(defOrg), actions: authz.AllActions(), allow: false},

		{resource: authz.ResourceWorkspace.SetOwner(user.ID()).SetID(wrkID), actions: authz.AllActions(), allow: false},
		{resource: authz.ResourceWorkspace.SetOwner(user.ID()), actions: authz.AllActions(), allow: false},

		{resource: authz.ResourceWorkspace.SetID(wrkID), actions: authz.AllActions(), allow: false},

		{resource: authz.ResourceWorkspace, actions: authz.AllActions(), allow: false},

		// Other org + me + id
		{resource: authz.ResourceWorkspace.SetOrg("other").SetOwner(user.ID()).SetID(wrkID), actions: authz.AllActions(), allow: false},
		{resource: authz.ResourceWorkspace.SetOrg("other").SetOwner(user.ID()), actions: authz.AllActions(), allow: false},
		{resource: authz.ResourceWorkspace.SetOrg("other").SetID(wrkID), actions: authz.AllActions(), allow: false},
		{resource: authz.ResourceWorkspace.SetOrg("other"), actions: authz.AllActions(), allow: false},

		// Other org + other user + id
		{resource: authz.ResourceWorkspace.SetOrg(defOrg).SetOwner("not-me").SetID(wrkID), actions: authz.AllActions(), allow: false},
		{resource: authz.ResourceWorkspace.SetOrg(defOrg).SetOwner("not-me"), actions: authz.AllActions(), allow: false},

		{resource: authz.ResourceWorkspace.SetOwner("not-me").SetID(wrkID), actions: authz.AllActions(), allow: false},
		{resource: authz.ResourceWorkspace.SetOwner("not-me"), actions: authz.AllActions(), allow: false},

		// Other org + other use + other id
		{resource: authz.ResourceWorkspace.SetOrg("other").SetOwner("not-me").SetID("not-id"), actions: authz.AllActions(), allow: false},
		{resource: authz.ResourceWorkspace.SetOrg("other").SetOwner("not-me"), actions: authz.AllActions(), allow: false},
		{resource: authz.ResourceWorkspace.SetOrg("other").SetID("not-id"), actions: authz.AllActions(), allow: false},
		{resource: authz.ResourceWorkspace.SetOrg("other"), actions: authz.AllActions(), allow: false},

		{resource: authz.ResourceWorkspace.SetOwner("not-me").SetID("not-id"), actions: authz.AllActions(), allow: false},
		{resource: authz.ResourceWorkspace.SetOwner("not-me"), actions: authz.AllActions(), allow: false},

		{resource: authz.ResourceWorkspace.SetID("not-id"), actions: authz.AllActions(), allow: false},
	})

	user = authz.SubjectTODO{
		UserID: "me",
		Roles: []authz.Role{
			authz.RoleOrgAdmin(defOrg),
			authz.RoleSiteMember,
		},
	}

	testAuthorize(t, "OrgAdmin", user, []authTestCase{
		// Org + me + id
		{resource: authz.ResourceWorkspace.SetOrg(defOrg).SetOwner(user.ID()).SetID(wrkID), actions: authz.AllActions(), allow: true},
		{resource: authz.ResourceWorkspace.SetOrg(defOrg).SetOwner(user.ID()), actions: authz.AllActions(), allow: true},
		{resource: authz.ResourceWorkspace.SetOrg(defOrg).SetID(wrkID), actions: authz.AllActions(), allow: true},
		{resource: authz.ResourceWorkspace.SetOrg(defOrg), actions: authz.AllActions(), allow: true},

		{resource: authz.ResourceWorkspace.SetOwner(user.ID()).SetID(wrkID), actions: authz.AllActions(), allow: true},
		{resource: authz.ResourceWorkspace.SetOwner(user.ID()), actions: authz.AllActions(), allow: true},

		{resource: authz.ResourceWorkspace.SetID(wrkID), actions: authz.AllActions(), allow: false},

		{resource: authz.ResourceWorkspace, actions: authz.AllActions(), allow: false},

		// Other org + me + id
		{resource: authz.ResourceWorkspace.SetOrg("other").SetOwner(user.ID()).SetID(wrkID), actions: authz.AllActions(), allow: false},
		{resource: authz.ResourceWorkspace.SetOrg("other").SetOwner(user.ID()), actions: authz.AllActions(), allow: false},
		{resource: authz.ResourceWorkspace.SetOrg("other").SetID(wrkID), actions: authz.AllActions(), allow: false},
		{resource: authz.ResourceWorkspace.SetOrg("other"), actions: authz.AllActions(), allow: false},

		// Other org + other user + id
		{resource: authz.ResourceWorkspace.SetOrg(defOrg).SetOwner("not-me").SetID(wrkID), actions: authz.AllActions(), allow: true},
		{resource: authz.ResourceWorkspace.SetOrg(defOrg).SetOwner("not-me"), actions: authz.AllActions(), allow: true},

		{resource: authz.ResourceWorkspace.SetOwner("not-me").SetID(wrkID), actions: authz.AllActions(), allow: false},
		{resource: authz.ResourceWorkspace.SetOwner("not-me"), actions: authz.AllActions(), allow: false},

		// Other org + other use + other id
		{resource: authz.ResourceWorkspace.SetOrg("other").SetOwner("not-me").SetID("not-id"), actions: authz.AllActions(), allow: false},
		{resource: authz.ResourceWorkspace.SetOrg("other").SetOwner("not-me"), actions: authz.AllActions(), allow: false},
		{resource: authz.ResourceWorkspace.SetOrg("other").SetID("not-id"), actions: authz.AllActions(), allow: false},
		{resource: authz.ResourceWorkspace.SetOrg("other"), actions: authz.AllActions(), allow: false},

		{resource: authz.ResourceWorkspace.SetOwner("not-me").SetID("not-id"), actions: authz.AllActions(), allow: false},
		{resource: authz.ResourceWorkspace.SetOwner("not-me"), actions: authz.AllActions(), allow: false},

		{resource: authz.ResourceWorkspace.SetID("not-id"), actions: authz.AllActions(), allow: false},
	})

	user = authz.SubjectTODO{
		UserID: "me",
		Roles: []authz.Role{
			authz.RoleSiteAdmin,
			authz.RoleSiteMember,
		},
	}

	testAuthorize(t, "SiteAdmin", user, []authTestCase{
		// Org + me + id
		{resource: authz.ResourceWorkspace.SetOrg(defOrg).SetOwner(user.ID()).SetID(wrkID), actions: authz.AllActions(), allow: true},
		{resource: authz.ResourceWorkspace.SetOrg(defOrg).SetOwner(user.ID()), actions: authz.AllActions(), allow: true},
		{resource: authz.ResourceWorkspace.SetOrg(defOrg).SetID(wrkID), actions: authz.AllActions(), allow: true},
		{resource: authz.ResourceWorkspace.SetOrg(defOrg), actions: authz.AllActions(), allow: true},

		{resource: authz.ResourceWorkspace.SetOwner(user.ID()).SetID(wrkID), actions: authz.AllActions(), allow: true},
		{resource: authz.ResourceWorkspace.SetOwner(user.ID()), actions: authz.AllActions(), allow: true},

		{resource: authz.ResourceWorkspace.SetID(wrkID), actions: authz.AllActions(), allow: true},

		{resource: authz.ResourceWorkspace, actions: authz.AllActions(), allow: true},

		// Other org + me + id
		{resource: authz.ResourceWorkspace.SetOrg("other").SetOwner(user.ID()).SetID(wrkID), actions: authz.AllActions(), allow: true},
		{resource: authz.ResourceWorkspace.SetOrg("other").SetOwner(user.ID()), actions: authz.AllActions(), allow: true},
		{resource: authz.ResourceWorkspace.SetOrg("other").SetID(wrkID), actions: authz.AllActions(), allow: true},
		{resource: authz.ResourceWorkspace.SetOrg("other"), actions: authz.AllActions(), allow: true},

		// Other org + other user + id
		{resource: authz.ResourceWorkspace.SetOrg(defOrg).SetOwner("not-me").SetID(wrkID), actions: authz.AllActions(), allow: true},
		{resource: authz.ResourceWorkspace.SetOrg(defOrg).SetOwner("not-me"), actions: authz.AllActions(), allow: true},

		{resource: authz.ResourceWorkspace.SetOwner("not-me").SetID(wrkID), actions: authz.AllActions(), allow: true},
		{resource: authz.ResourceWorkspace.SetOwner("not-me"), actions: authz.AllActions(), allow: true},

		// Other org + other use + other id
		{resource: authz.ResourceWorkspace.SetOrg("other").SetOwner("not-me").SetID("not-id"), actions: authz.AllActions(), allow: true},
		{resource: authz.ResourceWorkspace.SetOrg("other").SetOwner("not-me"), actions: authz.AllActions(), allow: true},
		{resource: authz.ResourceWorkspace.SetOrg("other").SetID("not-id"), actions: authz.AllActions(), allow: true},
		{resource: authz.ResourceWorkspace.SetOrg("other"), actions: authz.AllActions(), allow: true},

		{resource: authz.ResourceWorkspace.SetOwner("not-me").SetID("not-id"), actions: authz.AllActions(), allow: true},
		{resource: authz.ResourceWorkspace.SetOwner("not-me"), actions: authz.AllActions(), allow: true},

		{resource: authz.ResourceWorkspace.SetID("not-id"), actions: authz.AllActions(), allow: true},
	})

	// In practice this is a token scope on a regular subject
	user = authz.SubjectTODO{
		UserID: "me",
		Roles: []authz.Role{
			authz.RoleWorkspaceAgent(wrkID),
		},
	}

	testAuthorize(t, "WorkspaceAgentToken", user, []authTestCase{
		// Org + me + id
		{resource: authz.ResourceWorkspace.SetOrg(defOrg).SetOwner(user.ID()).SetID(wrkID), actions: authz.AllActions(), allow: true},
		{resource: authz.ResourceWorkspace.SetOrg(defOrg).SetOwner(user.ID()), actions: authz.AllActions(), allow: false},
		{resource: authz.ResourceWorkspace.SetOrg(defOrg).SetID(wrkID), actions: authz.AllActions(), allow: true},
		{resource: authz.ResourceWorkspace.SetOrg(defOrg), actions: authz.AllActions(), allow: false},

		{resource: authz.ResourceWorkspace.SetOwner(user.ID()).SetID(wrkID), actions: authz.AllActions(), allow: true},
		{resource: authz.ResourceWorkspace.SetOwner(user.ID()), actions: authz.AllActions(), allow: false},

		{resource: authz.ResourceWorkspace.SetID(wrkID), actions: authz.AllActions(), allow: true},

		{resource: authz.ResourceWorkspace, actions: authz.AllActions(), allow: false},

		// Other org + me + id
		{resource: authz.ResourceWorkspace.SetOrg("other").SetOwner(user.ID()).SetID(wrkID), actions: authz.AllActions(), allow: false},
		{resource: authz.ResourceWorkspace.SetOrg("other").SetOwner(user.ID()), actions: authz.AllActions(), allow: false},
		{resource: authz.ResourceWorkspace.SetOrg("other").SetID(wrkID), actions: authz.AllActions(), allow: false},
		{resource: authz.ResourceWorkspace.SetOrg("other"), actions: authz.AllActions(), allow: false},

		// Other org + other user + id
		{resource: authz.ResourceWorkspace.SetOrg(defOrg).SetOwner("not-me").SetID(wrkID), actions: authz.AllActions(), allow: true},
		{resource: authz.ResourceWorkspace.SetOrg(defOrg).SetOwner("not-me"), actions: authz.AllActions(), allow: false},

		{resource: authz.ResourceWorkspace.SetOwner("not-me").SetID(wrkID), actions: authz.AllActions(), allow: true},
		{resource: authz.ResourceWorkspace.SetOwner("not-me"), actions: authz.AllActions(), allow: false},

		// Other org + other use + other id
		{resource: authz.ResourceWorkspace.SetOrg("other").SetOwner("not-me").SetID("not-id"), actions: authz.AllActions(), allow: false},
		{resource: authz.ResourceWorkspace.SetOrg("other").SetOwner("not-me"), actions: authz.AllActions(), allow: false},
		{resource: authz.ResourceWorkspace.SetOrg("other").SetID("not-id"), actions: authz.AllActions(), allow: false},
		{resource: authz.ResourceWorkspace.SetOrg("other"), actions: authz.AllActions(), allow: false},

		{resource: authz.ResourceWorkspace.SetOwner("not-me").SetID("not-id"), actions: authz.AllActions(), allow: false},
		{resource: authz.ResourceWorkspace.SetOwner("not-me"), actions: authz.AllActions(), allow: false},

		{resource: authz.ResourceWorkspace.SetID("not-id"), actions: authz.AllActions(), allow: false},
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

	testAuthorize(t, "ReadOnly", user,
		cases(func(c authTestCase) authTestCase {
			c.actions = []authz.Action{authz.ActionRead}
			return c
		}, []authTestCase{
			// Read
			// Org + me + id
			{resource: authz.ResourceWorkspace.SetOrg(defOrg).SetOwner(user.ID()).SetID(wrkID), allow: true},
			{resource: authz.ResourceWorkspace.SetOrg(defOrg).SetOwner(user.ID()), allow: true},
			{resource: authz.ResourceWorkspace.SetOrg(defOrg).SetID(wrkID), allow: false},
			{resource: authz.ResourceWorkspace.SetOrg(defOrg), allow: true},

			{resource: authz.ResourceWorkspace.SetOwner(user.ID()).SetID(wrkID), allow: true},
			{resource: authz.ResourceWorkspace.SetOwner(user.ID()), allow: true},

			{resource: authz.ResourceWorkspace.SetID(wrkID), allow: false},

			{resource: authz.ResourceWorkspace, allow: false},

			// Other org + me + id
			{resource: authz.ResourceWorkspace.SetOrg("other").SetOwner(user.ID()).SetID(wrkID), allow: false},
			{resource: authz.ResourceWorkspace.SetOrg("other").SetOwner(user.ID()), allow: false},
			{resource: authz.ResourceWorkspace.SetOrg("other").SetID(wrkID), allow: false},
			{resource: authz.ResourceWorkspace.SetOrg("other"), allow: false},

			// Other org + other user + id
			{resource: authz.ResourceWorkspace.SetOrg(defOrg).SetOwner("not-me").SetID(wrkID), allow: true},
			{resource: authz.ResourceWorkspace.SetOrg(defOrg).SetOwner("not-me"), allow: true},

			{resource: authz.ResourceWorkspace.SetOwner("not-me").SetID(wrkID), allow: false},
			{resource: authz.ResourceWorkspace.SetOwner("not-me"), allow: false},

			// Other org + other use + other id
			{resource: authz.ResourceWorkspace.SetOrg("other").SetOwner("not-me").SetID("not-id"), allow: false},
			{resource: authz.ResourceWorkspace.SetOrg("other").SetOwner("not-me"), allow: false},
			{resource: authz.ResourceWorkspace.SetOrg("other").SetID("not-id"), allow: false},
			{resource: authz.ResourceWorkspace.SetOrg("other"), allow: false},

			{resource: authz.ResourceWorkspace.SetOwner("not-me").SetID("not-id"), allow: false},
			{resource: authz.ResourceWorkspace.SetOwner("not-me"), allow: false},

			{resource: authz.ResourceWorkspace.SetID("not-id"), allow: false},
		}),

		// Pass non-read actions
		cases(func(c authTestCase) authTestCase {
			c.actions = []authz.Action{authz.ActionCreate, authz.ActionUpdate, authz.ActionDelete}
			c.allow = false
			return c
		}, []authTestCase{
			// Read
			// Org + me + id
			{resource: authz.ResourceWorkspace.SetOrg(defOrg).SetOwner(user.ID()).SetID(wrkID)},
			{resource: authz.ResourceWorkspace.SetOrg(defOrg).SetOwner(user.ID())},
			{resource: authz.ResourceWorkspace.SetOrg(defOrg).SetID(wrkID)},
			{resource: authz.ResourceWorkspace.SetOrg(defOrg)},

			{resource: authz.ResourceWorkspace.SetOwner(user.ID()).SetID(wrkID)},
			{resource: authz.ResourceWorkspace.SetOwner(user.ID())},

			{resource: authz.ResourceWorkspace.SetID(wrkID)},

			{resource: authz.ResourceWorkspace},

			// Other org + me + id
			{resource: authz.ResourceWorkspace.SetOrg("other").SetOwner(user.ID()).SetID(wrkID)},
			{resource: authz.ResourceWorkspace.SetOrg("other").SetOwner(user.ID())},
			{resource: authz.ResourceWorkspace.SetOrg("other").SetID(wrkID)},
			{resource: authz.ResourceWorkspace.SetOrg("other")},

			// Other org + other user + id
			{resource: authz.ResourceWorkspace.SetOrg(defOrg).SetOwner("not-me").SetID(wrkID)},
			{resource: authz.ResourceWorkspace.SetOrg(defOrg).SetOwner("not-me")},

			{resource: authz.ResourceWorkspace.SetOwner("not-me").SetID(wrkID)},
			{resource: authz.ResourceWorkspace.SetOwner("not-me")},

			// Other org + other use + other id
			{resource: authz.ResourceWorkspace.SetOrg("other").SetOwner("not-me").SetID("not-id")},
			{resource: authz.ResourceWorkspace.SetOrg("other").SetOwner("not-me")},
			{resource: authz.ResourceWorkspace.SetOrg("other").SetID("not-id")},
			{resource: authz.ResourceWorkspace.SetOrg("other")},

			{resource: authz.ResourceWorkspace.SetOwner("not-me").SetID("not-id")},
			{resource: authz.ResourceWorkspace.SetOwner("not-me")},

			{resource: authz.ResourceWorkspace.SetID("not-id")},
		}))
}

// TestAuthorizeLevels ensures level overrides are acting appropriately
func TestAuthorizeLevels(t *testing.T) {
	defOrg := "default"
	wrkID := "1234"

	user := authz.SubjectTODO{
		UserID: "me",
		Roles: []authz.Role{
			authz.RoleSiteAdmin,
			authz.RoleOrgDenyAll(defOrg),
			{
				Name: "user-deny-all",
				// List out deny permissions explicitly
				Site: []authz.Permission{
					{
						Negate:       true,
						ResourceType: authz.Wildcard,
						ResourceID:   authz.Wildcard,
						Action:       authz.Wildcard,
					},
				},
			},
		},
	}

	testAuthorize(t, "AdminAlwaysAllow", user,
		cases(func(c authTestCase) authTestCase {
			c.actions = authz.AllActions()
			c.allow = true
			return c
		}, []authTestCase{
			// Org + me + id
			{resource: authz.ResourceWorkspace.SetOrg(defOrg).SetOwner(user.ID()).SetID(wrkID)},
			{resource: authz.ResourceWorkspace.SetOrg(defOrg).SetOwner(user.ID())},
			{resource: authz.ResourceWorkspace.SetOrg(defOrg).SetID(wrkID)},
			{resource: authz.ResourceWorkspace.SetOrg(defOrg)},

			{resource: authz.ResourceWorkspace.SetOwner(user.ID()).SetID(wrkID)},
			{resource: authz.ResourceWorkspace.SetOwner(user.ID())},

			{resource: authz.ResourceWorkspace.SetID(wrkID)},

			{resource: authz.ResourceWorkspace},

			// Other org + me + id
			{resource: authz.ResourceWorkspace.SetOrg("other").SetOwner(user.ID()).SetID(wrkID)},
			{resource: authz.ResourceWorkspace.SetOrg("other").SetOwner(user.ID())},
			{resource: authz.ResourceWorkspace.SetOrg("other").SetID(wrkID)},
			{resource: authz.ResourceWorkspace.SetOrg("other")},

			// Other org + other user + id
			{resource: authz.ResourceWorkspace.SetOrg(defOrg).SetOwner("not-me").SetID(wrkID)},
			{resource: authz.ResourceWorkspace.SetOrg(defOrg).SetOwner("not-me")},

			{resource: authz.ResourceWorkspace.SetOwner("not-me").SetID(wrkID)},
			{resource: authz.ResourceWorkspace.SetOwner("not-me")},

			// Other org + other use + other id
			{resource: authz.ResourceWorkspace.SetOrg("other").SetOwner("not-me").SetID("not-id")},
			{resource: authz.ResourceWorkspace.SetOrg("other").SetOwner("not-me")},
			{resource: authz.ResourceWorkspace.SetOrg("other").SetID("not-id")},
			{resource: authz.ResourceWorkspace.SetOrg("other")},

			{resource: authz.ResourceWorkspace.SetOwner("not-me").SetID("not-id")},
			{resource: authz.ResourceWorkspace.SetOwner("not-me")},

			{resource: authz.ResourceWorkspace.SetID("not-id")},
		}))

	user = authz.SubjectTODO{
		UserID: "me",
		Roles: []authz.Role{
			{
				Name: "site-noise",
				Site: []authz.Permission{
					{
						Negate:       true,
						ResourceType: "random",
						ResourceID:   authz.Wildcard,
						Action:       authz.Wildcard,
					},
				},
			},
			authz.RoleOrgAdmin(defOrg),
			{
				Name: "user-deny-all",
				// List out deny permissions explicitly
				Site: []authz.Permission{
					{
						Negate:       true,
						ResourceType: authz.Wildcard,
						ResourceID:   authz.Wildcard,
						Action:       authz.Wildcard,
					},
				},
			},
		},
	}

	testAuthorize(t, "OrgAllowAll", user,
		cases(func(c authTestCase) authTestCase {
			c.actions = authz.AllActions()
			return c
		}, []authTestCase{
			// Org + me + id
			{resource: authz.ResourceWorkspace.SetOrg(defOrg).SetOwner(user.ID()).SetID(wrkID), allow: true},
			{resource: authz.ResourceWorkspace.SetOrg(defOrg).SetOwner(user.ID()), allow: true},
			{resource: authz.ResourceWorkspace.SetOrg(defOrg).SetID(wrkID), allow: true},
			{resource: authz.ResourceWorkspace.SetOrg(defOrg), allow: true},

			{resource: authz.ResourceWorkspace.SetOwner(user.ID()).SetID(wrkID), allow: false},
			{resource: authz.ResourceWorkspace.SetOwner(user.ID()), allow: false},

			{resource: authz.ResourceWorkspace.SetID(wrkID), allow: false},

			{resource: authz.ResourceWorkspace, allow: false},

			// Other org + me + id
			{resource: authz.ResourceWorkspace.SetOrg("other").SetOwner(user.ID()).SetID(wrkID), allow: false},
			{resource: authz.ResourceWorkspace.SetOrg("other").SetOwner(user.ID()), allow: false},
			{resource: authz.ResourceWorkspace.SetOrg("other").SetID(wrkID), allow: false},
			{resource: authz.ResourceWorkspace.SetOrg("other"), allow: false},

			// Other org + other user + id
			{resource: authz.ResourceWorkspace.SetOrg(defOrg).SetOwner("not-me").SetID(wrkID), allow: true},
			{resource: authz.ResourceWorkspace.SetOrg(defOrg).SetOwner("not-me"), allow: true},

			{resource: authz.ResourceWorkspace.SetOwner("not-me").SetID(wrkID), allow: false},
			{resource: authz.ResourceWorkspace.SetOwner("not-me"), allow: false},

			// Other org + other use + other id
			{resource: authz.ResourceWorkspace.SetOrg("other").SetOwner("not-me").SetID("not-id"), allow: false},
			{resource: authz.ResourceWorkspace.SetOrg("other").SetOwner("not-me"), allow: false},
			{resource: authz.ResourceWorkspace.SetOrg("other").SetID("not-id"), allow: false},
			{resource: authz.ResourceWorkspace.SetOrg("other"), allow: false},

			{resource: authz.ResourceWorkspace.SetOwner("not-me").SetID("not-id"), allow: false},
			{resource: authz.ResourceWorkspace.SetOwner("not-me"), allow: false},

			{resource: authz.ResourceWorkspace.SetID("not-id"), allow: false},
		}))
}

func cases(opt func(c authTestCase) authTestCase, cases []authTestCase) []authTestCase {
	if opt == nil {
		return cases
	}
	for i := range cases {
		cases[i] = opt(cases[i])
	}
	return cases
}

type authTestCase struct {
	resource authz.Resource
	actions  []authz.Action
	allow    bool
}

func testAuthorize(t *testing.T, name string, subject authz.Subject, sets ...[]authTestCase) {
	for _, cases := range sets {
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
}

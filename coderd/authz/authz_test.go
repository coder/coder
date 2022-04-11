package authz_test

import (
	"testing"

	"github.com/coder/coder/coderd/authz"
	"github.com/stretchr/testify/require"
)

// TestAuthorizeDomain test the very basic roles that are commonly used.
func TestAuthorizeDomain(t *testing.T) {
	t.Skip("TODO: unskip when rego is done")
	t.Parallel()
	defOrg := "default"
	wrkID := "1234"

	user := authz.SubjectTODO{
		UserID: "me",
		Roles:  []authz.Role{authz.RoleMember, authz.RoleOrgMember(defOrg)},
	}

	testAuthorize(t, "Member", user, []authTestCase{
		// Org + me + id
		{resource: authz.ResourceWorkspace.InOrg(defOrg).WithOwner(user.ID()).WithID(wrkID), actions: allActions(), allow: true},
		{resource: authz.ResourceWorkspace.InOrg(defOrg).WithOwner(user.ID()), actions: allActions(), allow: true},
		{resource: authz.ResourceWorkspace.InOrg(defOrg).WithID(wrkID), actions: allActions(), allow: false},
		{resource: authz.ResourceWorkspace.InOrg(defOrg), actions: allActions(), allow: false},

		{resource: authz.ResourceWorkspace.WithOwner(user.ID()).WithID(wrkID), actions: allActions(), allow: true},
		{resource: authz.ResourceWorkspace.WithOwner(user.ID()), actions: allActions(), allow: true},

		{resource: authz.ResourceWorkspace.WithID(wrkID), actions: allActions(), allow: false},

		{resource: authz.ResourceWorkspace.All(), actions: allActions(), allow: false},

		// Other org + me + id
		{resource: authz.ResourceWorkspace.InOrg("other").WithOwner(user.ID()).WithID(wrkID), actions: allActions(), allow: false},
		{resource: authz.ResourceWorkspace.InOrg("other").WithOwner(user.ID()), actions: allActions(), allow: false},
		{resource: authz.ResourceWorkspace.InOrg("other").WithID(wrkID), actions: allActions(), allow: false},
		{resource: authz.ResourceWorkspace.InOrg("other"), actions: allActions(), allow: false},

		// Other org + other user + id
		{resource: authz.ResourceWorkspace.InOrg(defOrg).WithOwner("not-me").WithID(wrkID), actions: allActions(), allow: false},
		{resource: authz.ResourceWorkspace.InOrg(defOrg).WithOwner("not-me"), actions: allActions(), allow: false},

		{resource: authz.ResourceWorkspace.WithOwner("not-me").WithID(wrkID), actions: allActions(), allow: false},
		{resource: authz.ResourceWorkspace.WithOwner("not-me"), actions: allActions(), allow: false},

		// Other org + other use + other id
		{resource: authz.ResourceWorkspace.InOrg("other").WithOwner("not-me").WithID("not-id"), actions: allActions(), allow: false},
		{resource: authz.ResourceWorkspace.InOrg("other").WithOwner("not-me"), actions: allActions(), allow: false},
		{resource: authz.ResourceWorkspace.InOrg("other").WithID("not-id"), actions: allActions(), allow: false},
		{resource: authz.ResourceWorkspace.InOrg("other"), actions: allActions(), allow: false},

		{resource: authz.ResourceWorkspace.WithOwner("not-me").WithID("not-id"), actions: allActions(), allow: false},
		{resource: authz.ResourceWorkspace.WithOwner("not-me"), actions: allActions(), allow: false},

		{resource: authz.ResourceWorkspace.WithID("not-id"), actions: allActions(), allow: false},
	})

	user = authz.SubjectTODO{
		UserID: "me",
		Roles: []authz.Role{{
			Name: "deny-all",
			// List out deny permissions explicitly
			Site: []authz.Permission{
				{
					Negate:       true,
					ResourceType: authz.Wildcard,
					ResourceID:   authz.Wildcard,
					Action:       authz.Wildcard,
				},
			},
		}},
	}

	testAuthorize(t, "DeletedMember", user, []authTestCase{
		// Org + me + id
		{resource: authz.ResourceWorkspace.InOrg(defOrg).WithOwner(user.ID()).WithID(wrkID), actions: allActions(), allow: false},
		{resource: authz.ResourceWorkspace.InOrg(defOrg).WithOwner(user.ID()), actions: allActions(), allow: false},
		{resource: authz.ResourceWorkspace.InOrg(defOrg).WithID(wrkID), actions: allActions(), allow: false},
		{resource: authz.ResourceWorkspace.InOrg(defOrg), actions: allActions(), allow: false},

		{resource: authz.ResourceWorkspace.WithOwner(user.ID()).WithID(wrkID), actions: allActions(), allow: false},
		{resource: authz.ResourceWorkspace.WithOwner(user.ID()), actions: allActions(), allow: false},

		{resource: authz.ResourceWorkspace.WithID(wrkID), actions: allActions(), allow: false},

		{resource: authz.ResourceWorkspace.All(), actions: allActions(), allow: false},

		// Other org + me + id
		{resource: authz.ResourceWorkspace.InOrg("other").WithOwner(user.ID()).WithID(wrkID), actions: allActions(), allow: false},
		{resource: authz.ResourceWorkspace.InOrg("other").WithOwner(user.ID()), actions: allActions(), allow: false},
		{resource: authz.ResourceWorkspace.InOrg("other").WithID(wrkID), actions: allActions(), allow: false},
		{resource: authz.ResourceWorkspace.InOrg("other"), actions: allActions(), allow: false},

		// Other org + other user + id
		{resource: authz.ResourceWorkspace.InOrg(defOrg).WithOwner("not-me").WithID(wrkID), actions: allActions(), allow: false},
		{resource: authz.ResourceWorkspace.InOrg(defOrg).WithOwner("not-me"), actions: allActions(), allow: false},

		{resource: authz.ResourceWorkspace.WithOwner("not-me").WithID(wrkID), actions: allActions(), allow: false},
		{resource: authz.ResourceWorkspace.WithOwner("not-me"), actions: allActions(), allow: false},

		// Other org + other use + other id
		{resource: authz.ResourceWorkspace.InOrg("other").WithOwner("not-me").WithID("not-id"), actions: allActions(), allow: false},
		{resource: authz.ResourceWorkspace.InOrg("other").WithOwner("not-me"), actions: allActions(), allow: false},
		{resource: authz.ResourceWorkspace.InOrg("other").WithID("not-id"), actions: allActions(), allow: false},
		{resource: authz.ResourceWorkspace.InOrg("other"), actions: allActions(), allow: false},

		{resource: authz.ResourceWorkspace.WithOwner("not-me").WithID("not-id"), actions: allActions(), allow: false},
		{resource: authz.ResourceWorkspace.WithOwner("not-me"), actions: allActions(), allow: false},

		{resource: authz.ResourceWorkspace.WithID("not-id"), actions: allActions(), allow: false},
	})

	user = authz.SubjectTODO{
		UserID: "me",
		Roles: []authz.Role{
			authz.RoleOrgAdmin(defOrg),
			authz.RoleMember,
		},
	}

	testAuthorize(t, "OrgAdmin", user, []authTestCase{
		// Org + me + id
		{resource: authz.ResourceWorkspace.InOrg(defOrg).WithOwner(user.ID()).WithID(wrkID), actions: allActions(), allow: true},
		{resource: authz.ResourceWorkspace.InOrg(defOrg).WithOwner(user.ID()), actions: allActions(), allow: true},
		{resource: authz.ResourceWorkspace.InOrg(defOrg).WithID(wrkID), actions: allActions(), allow: true},
		{resource: authz.ResourceWorkspace.InOrg(defOrg), actions: allActions(), allow: true},

		{resource: authz.ResourceWorkspace.WithOwner(user.ID()).WithID(wrkID), actions: allActions(), allow: true},
		{resource: authz.ResourceWorkspace.WithOwner(user.ID()), actions: allActions(), allow: true},

		{resource: authz.ResourceWorkspace.WithID(wrkID), actions: allActions(), allow: false},

		{resource: authz.ResourceWorkspace.All(), actions: allActions(), allow: false},

		// Other org + me + id
		{resource: authz.ResourceWorkspace.InOrg("other").WithOwner(user.ID()).WithID(wrkID), actions: allActions(), allow: false},
		{resource: authz.ResourceWorkspace.InOrg("other").WithOwner(user.ID()), actions: allActions(), allow: false},
		{resource: authz.ResourceWorkspace.InOrg("other").WithID(wrkID), actions: allActions(), allow: false},
		{resource: authz.ResourceWorkspace.InOrg("other"), actions: allActions(), allow: false},

		// Other org + other user + id
		{resource: authz.ResourceWorkspace.InOrg(defOrg).WithOwner("not-me").WithID(wrkID), actions: allActions(), allow: true},
		{resource: authz.ResourceWorkspace.InOrg(defOrg).WithOwner("not-me"), actions: allActions(), allow: true},

		{resource: authz.ResourceWorkspace.WithOwner("not-me").WithID(wrkID), actions: allActions(), allow: false},
		{resource: authz.ResourceWorkspace.WithOwner("not-me"), actions: allActions(), allow: false},

		// Other org + other use + other id
		{resource: authz.ResourceWorkspace.InOrg("other").WithOwner("not-me").WithID("not-id"), actions: allActions(), allow: false},
		{resource: authz.ResourceWorkspace.InOrg("other").WithOwner("not-me"), actions: allActions(), allow: false},
		{resource: authz.ResourceWorkspace.InOrg("other").WithID("not-id"), actions: allActions(), allow: false},
		{resource: authz.ResourceWorkspace.InOrg("other"), actions: allActions(), allow: false},

		{resource: authz.ResourceWorkspace.WithOwner("not-me").WithID("not-id"), actions: allActions(), allow: false},
		{resource: authz.ResourceWorkspace.WithOwner("not-me"), actions: allActions(), allow: false},

		{resource: authz.ResourceWorkspace.WithID("not-id"), actions: allActions(), allow: false},
	})

	user = authz.SubjectTODO{
		UserID: "me",
		Roles: []authz.Role{
			authz.RoleAdmin,
			authz.RoleMember,
		},
	}

	testAuthorize(t, "SiteAdmin", user, []authTestCase{
		// Org + me + id
		{resource: authz.ResourceWorkspace.InOrg(defOrg).WithOwner(user.ID()).WithID(wrkID), actions: allActions(), allow: true},
		{resource: authz.ResourceWorkspace.InOrg(defOrg).WithOwner(user.ID()), actions: allActions(), allow: true},
		{resource: authz.ResourceWorkspace.InOrg(defOrg).WithID(wrkID), actions: allActions(), allow: true},
		{resource: authz.ResourceWorkspace.InOrg(defOrg), actions: allActions(), allow: true},

		{resource: authz.ResourceWorkspace.WithOwner(user.ID()).WithID(wrkID), actions: allActions(), allow: true},
		{resource: authz.ResourceWorkspace.WithOwner(user.ID()), actions: allActions(), allow: true},

		{resource: authz.ResourceWorkspace.WithID(wrkID), actions: allActions(), allow: true},

		{resource: authz.ResourceWorkspace.All(), actions: allActions(), allow: true},

		// Other org + me + id
		{resource: authz.ResourceWorkspace.InOrg("other").WithOwner(user.ID()).WithID(wrkID), actions: allActions(), allow: true},
		{resource: authz.ResourceWorkspace.InOrg("other").WithOwner(user.ID()), actions: allActions(), allow: true},
		{resource: authz.ResourceWorkspace.InOrg("other").WithID(wrkID), actions: allActions(), allow: true},
		{resource: authz.ResourceWorkspace.InOrg("other"), actions: allActions(), allow: true},

		// Other org + other user + id
		{resource: authz.ResourceWorkspace.InOrg(defOrg).WithOwner("not-me").WithID(wrkID), actions: allActions(), allow: true},
		{resource: authz.ResourceWorkspace.InOrg(defOrg).WithOwner("not-me"), actions: allActions(), allow: true},

		{resource: authz.ResourceWorkspace.WithOwner("not-me").WithID(wrkID), actions: allActions(), allow: true},
		{resource: authz.ResourceWorkspace.WithOwner("not-me"), actions: allActions(), allow: true},

		// Other org + other use + other id
		{resource: authz.ResourceWorkspace.InOrg("other").WithOwner("not-me").WithID("not-id"), actions: allActions(), allow: true},
		{resource: authz.ResourceWorkspace.InOrg("other").WithOwner("not-me"), actions: allActions(), allow: true},
		{resource: authz.ResourceWorkspace.InOrg("other").WithID("not-id"), actions: allActions(), allow: true},
		{resource: authz.ResourceWorkspace.InOrg("other"), actions: allActions(), allow: true},

		{resource: authz.ResourceWorkspace.WithOwner("not-me").WithID("not-id"), actions: allActions(), allow: true},
		{resource: authz.ResourceWorkspace.WithOwner("not-me"), actions: allActions(), allow: true},

		{resource: authz.ResourceWorkspace.WithID("not-id"), actions: allActions(), allow: true},
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
		{resource: authz.ResourceWorkspace.InOrg(defOrg).WithOwner(user.ID()).WithID(wrkID), actions: allActions(), allow: true},
		{resource: authz.ResourceWorkspace.InOrg(defOrg).WithOwner(user.ID()), actions: allActions(), allow: false},
		{resource: authz.ResourceWorkspace.InOrg(defOrg).WithID(wrkID), actions: allActions(), allow: true},
		{resource: authz.ResourceWorkspace.InOrg(defOrg), actions: allActions(), allow: false},

		{resource: authz.ResourceWorkspace.WithOwner(user.ID()).WithID(wrkID), actions: allActions(), allow: true},
		{resource: authz.ResourceWorkspace.WithOwner(user.ID()), actions: allActions(), allow: false},

		{resource: authz.ResourceWorkspace.WithID(wrkID), actions: allActions(), allow: true},

		{resource: authz.ResourceWorkspace.All(), actions: allActions(), allow: false},

		// Other org + me + id
		{resource: authz.ResourceWorkspace.InOrg("other").WithOwner(user.ID()).WithID(wrkID), actions: allActions(), allow: false},
		{resource: authz.ResourceWorkspace.InOrg("other").WithOwner(user.ID()), actions: allActions(), allow: false},
		{resource: authz.ResourceWorkspace.InOrg("other").WithID(wrkID), actions: allActions(), allow: false},
		{resource: authz.ResourceWorkspace.InOrg("other"), actions: allActions(), allow: false},

		// Other org + other user + id
		{resource: authz.ResourceWorkspace.InOrg(defOrg).WithOwner("not-me").WithID(wrkID), actions: allActions(), allow: true},
		{resource: authz.ResourceWorkspace.InOrg(defOrg).WithOwner("not-me"), actions: allActions(), allow: false},

		{resource: authz.ResourceWorkspace.WithOwner("not-me").WithID(wrkID), actions: allActions(), allow: true},
		{resource: authz.ResourceWorkspace.WithOwner("not-me"), actions: allActions(), allow: false},

		// Other org + other use + other id
		{resource: authz.ResourceWorkspace.InOrg("other").WithOwner("not-me").WithID("not-id"), actions: allActions(), allow: false},
		{resource: authz.ResourceWorkspace.InOrg("other").WithOwner("not-me"), actions: allActions(), allow: false},
		{resource: authz.ResourceWorkspace.InOrg("other").WithID("not-id"), actions: allActions(), allow: false},
		{resource: authz.ResourceWorkspace.InOrg("other"), actions: allActions(), allow: false},

		{resource: authz.ResourceWorkspace.WithOwner("not-me").WithID("not-id"), actions: allActions(), allow: false},
		{resource: authz.ResourceWorkspace.WithOwner("not-me"), actions: allActions(), allow: false},

		{resource: authz.ResourceWorkspace.WithID("not-id"), actions: allActions(), allow: false},
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
			{resource: authz.ResourceWorkspace.InOrg(defOrg).WithOwner(user.ID()).WithID(wrkID), allow: true},
			{resource: authz.ResourceWorkspace.InOrg(defOrg).WithOwner(user.ID()), allow: true},
			{resource: authz.ResourceWorkspace.InOrg(defOrg).WithID(wrkID), allow: false},
			{resource: authz.ResourceWorkspace.InOrg(defOrg), allow: true},

			{resource: authz.ResourceWorkspace.WithOwner(user.ID()).WithID(wrkID), allow: true},
			{resource: authz.ResourceWorkspace.WithOwner(user.ID()), allow: true},

			{resource: authz.ResourceWorkspace.WithID(wrkID), allow: false},

			{resource: authz.ResourceWorkspace.All(), allow: false},

			// Other org + me + id
			{resource: authz.ResourceWorkspace.InOrg("other").WithOwner(user.ID()).WithID(wrkID), allow: false},
			{resource: authz.ResourceWorkspace.InOrg("other").WithOwner(user.ID()), allow: false},
			{resource: authz.ResourceWorkspace.InOrg("other").WithID(wrkID), allow: false},
			{resource: authz.ResourceWorkspace.InOrg("other"), allow: false},

			// Other org + other user + id
			{resource: authz.ResourceWorkspace.InOrg(defOrg).WithOwner("not-me").WithID(wrkID), allow: true},
			{resource: authz.ResourceWorkspace.InOrg(defOrg).WithOwner("not-me"), allow: true},

			{resource: authz.ResourceWorkspace.WithOwner("not-me").WithID(wrkID), allow: false},
			{resource: authz.ResourceWorkspace.WithOwner("not-me"), allow: false},

			// Other org + other use + other id
			{resource: authz.ResourceWorkspace.InOrg("other").WithOwner("not-me").WithID("not-id"), allow: false},
			{resource: authz.ResourceWorkspace.InOrg("other").WithOwner("not-me"), allow: false},
			{resource: authz.ResourceWorkspace.InOrg("other").WithID("not-id"), allow: false},
			{resource: authz.ResourceWorkspace.InOrg("other"), allow: false},

			{resource: authz.ResourceWorkspace.WithOwner("not-me").WithID("not-id"), allow: false},
			{resource: authz.ResourceWorkspace.WithOwner("not-me"), allow: false},

			{resource: authz.ResourceWorkspace.WithID("not-id"), allow: false},
		}),

		// Pass non-read actions
		cases(func(c authTestCase) authTestCase {
			c.actions = []authz.Action{authz.ActionCreate, authz.ActionUpdate, authz.ActionDelete}
			c.allow = false
			return c
		}, []authTestCase{
			// Read
			// Org + me + id
			{resource: authz.ResourceWorkspace.InOrg(defOrg).WithOwner(user.ID()).WithID(wrkID)},
			{resource: authz.ResourceWorkspace.InOrg(defOrg).WithOwner(user.ID())},
			{resource: authz.ResourceWorkspace.InOrg(defOrg).WithID(wrkID)},
			{resource: authz.ResourceWorkspace.InOrg(defOrg)},

			{resource: authz.ResourceWorkspace.WithOwner(user.ID()).WithID(wrkID)},
			{resource: authz.ResourceWorkspace.WithOwner(user.ID())},

			{resource: authz.ResourceWorkspace.WithID(wrkID)},

			{resource: authz.ResourceWorkspace.All()},

			// Other org + me + id
			{resource: authz.ResourceWorkspace.InOrg("other").WithOwner(user.ID()).WithID(wrkID)},
			{resource: authz.ResourceWorkspace.InOrg("other").WithOwner(user.ID())},
			{resource: authz.ResourceWorkspace.InOrg("other").WithID(wrkID)},
			{resource: authz.ResourceWorkspace.InOrg("other")},

			// Other org + other user + id
			{resource: authz.ResourceWorkspace.InOrg(defOrg).WithOwner("not-me").WithID(wrkID)},
			{resource: authz.ResourceWorkspace.InOrg(defOrg).WithOwner("not-me")},

			{resource: authz.ResourceWorkspace.WithOwner("not-me").WithID(wrkID)},
			{resource: authz.ResourceWorkspace.WithOwner("not-me")},

			// Other org + other use + other id
			{resource: authz.ResourceWorkspace.InOrg("other").WithOwner("not-me").WithID("not-id")},
			{resource: authz.ResourceWorkspace.InOrg("other").WithOwner("not-me")},
			{resource: authz.ResourceWorkspace.InOrg("other").WithID("not-id")},
			{resource: authz.ResourceWorkspace.InOrg("other")},

			{resource: authz.ResourceWorkspace.WithOwner("not-me").WithID("not-id")},
			{resource: authz.ResourceWorkspace.WithOwner("not-me")},

			{resource: authz.ResourceWorkspace.WithID("not-id")},
		}))
}

// TestAuthorizeLevels ensures level overrides are acting appropriately
func TestAuthorizeLevels(t *testing.T) {
	defOrg := "default"
	wrkID := "1234"

	user := authz.SubjectTODO{
		UserID: "me",
		Roles: []authz.Role{
			authz.RoleAdmin,
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
			c.actions = allActions()
			c.allow = true
			return c
		}, []authTestCase{
			// Org + me + id
			{resource: authz.ResourceWorkspace.InOrg(defOrg).WithOwner(user.ID()).WithID(wrkID)},
			{resource: authz.ResourceWorkspace.InOrg(defOrg).WithOwner(user.ID())},
			{resource: authz.ResourceWorkspace.InOrg(defOrg).WithID(wrkID)},
			{resource: authz.ResourceWorkspace.InOrg(defOrg)},

			{resource: authz.ResourceWorkspace.WithOwner(user.ID()).WithID(wrkID)},
			{resource: authz.ResourceWorkspace.WithOwner(user.ID())},

			{resource: authz.ResourceWorkspace.WithID(wrkID)},

			{resource: authz.ResourceWorkspace.All()},

			// Other org + me + id
			{resource: authz.ResourceWorkspace.InOrg("other").WithOwner(user.ID()).WithID(wrkID)},
			{resource: authz.ResourceWorkspace.InOrg("other").WithOwner(user.ID())},
			{resource: authz.ResourceWorkspace.InOrg("other").WithID(wrkID)},
			{resource: authz.ResourceWorkspace.InOrg("other")},

			// Other org + other user + id
			{resource: authz.ResourceWorkspace.InOrg(defOrg).WithOwner("not-me").WithID(wrkID)},
			{resource: authz.ResourceWorkspace.InOrg(defOrg).WithOwner("not-me")},

			{resource: authz.ResourceWorkspace.WithOwner("not-me").WithID(wrkID)},
			{resource: authz.ResourceWorkspace.WithOwner("not-me")},

			// Other org + other use + other id
			{resource: authz.ResourceWorkspace.InOrg("other").WithOwner("not-me").WithID("not-id")},
			{resource: authz.ResourceWorkspace.InOrg("other").WithOwner("not-me")},
			{resource: authz.ResourceWorkspace.InOrg("other").WithID("not-id")},
			{resource: authz.ResourceWorkspace.InOrg("other")},

			{resource: authz.ResourceWorkspace.WithOwner("not-me").WithID("not-id")},
			{resource: authz.ResourceWorkspace.WithOwner("not-me")},

			{resource: authz.ResourceWorkspace.WithID("not-id")},
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
			c.actions = allActions()
			return c
		}, []authTestCase{
			// Org + me + id
			{resource: authz.ResourceWorkspace.InOrg(defOrg).WithOwner(user.ID()).WithID(wrkID), allow: true},
			{resource: authz.ResourceWorkspace.InOrg(defOrg).WithOwner(user.ID()), allow: true},
			{resource: authz.ResourceWorkspace.InOrg(defOrg).WithID(wrkID), allow: true},
			{resource: authz.ResourceWorkspace.InOrg(defOrg), allow: true},

			{resource: authz.ResourceWorkspace.WithOwner(user.ID()).WithID(wrkID), allow: false},
			{resource: authz.ResourceWorkspace.WithOwner(user.ID()), allow: false},

			{resource: authz.ResourceWorkspace.WithID(wrkID), allow: false},

			{resource: authz.ResourceWorkspace.All(), allow: false},

			// Other org + me + id
			{resource: authz.ResourceWorkspace.InOrg("other").WithOwner(user.ID()).WithID(wrkID), allow: false},
			{resource: authz.ResourceWorkspace.InOrg("other").WithOwner(user.ID()), allow: false},
			{resource: authz.ResourceWorkspace.InOrg("other").WithID(wrkID), allow: false},
			{resource: authz.ResourceWorkspace.InOrg("other"), allow: false},

			// Other org + other user + id
			{resource: authz.ResourceWorkspace.InOrg(defOrg).WithOwner("not-me").WithID(wrkID), allow: true},
			{resource: authz.ResourceWorkspace.InOrg(defOrg).WithOwner("not-me"), allow: true},

			{resource: authz.ResourceWorkspace.WithOwner("not-me").WithID(wrkID), allow: false},
			{resource: authz.ResourceWorkspace.WithOwner("not-me"), allow: false},

			// Other org + other use + other id
			{resource: authz.ResourceWorkspace.InOrg("other").WithOwner("not-me").WithID("not-id"), allow: false},
			{resource: authz.ResourceWorkspace.InOrg("other").WithOwner("not-me"), allow: false},
			{resource: authz.ResourceWorkspace.InOrg("other").WithID("not-id"), allow: false},
			{resource: authz.ResourceWorkspace.InOrg("other"), allow: false},

			{resource: authz.ResourceWorkspace.WithOwner("not-me").WithID("not-id"), allow: false},
			{resource: authz.ResourceWorkspace.WithOwner("not-me"), allow: false},

			{resource: authz.ResourceWorkspace.WithID("not-id"), allow: false},
		}))
}

// cases applies a given function to all test cases. This makes generalities easier to create.
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
	resource authz.Object
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

// allActions is a helper function to return all the possible actions types.
func allActions() []authz.Action {
	return []authz.Action{authz.ActionCreate, authz.ActionRead, authz.ActionUpdate, authz.ActionDelete}
}

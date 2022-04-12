package rbac_test

import (
	"context"
	"encoding/json"
	"testing"

	"golang.org/x/xerrors"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/rbac"
)

// subject is required because rego needs
type subject struct {
	UserID string      `json:"id"`
	Roles  []rbac.Role `json:"roles"`
}

// TestAuthorizeDomain test the very basic roles that are commonly used.
func TestAuthorizeDomain(t *testing.T) {
	t.Parallel()
	defOrg := "default"
	wrkID := "1234"

	user := subject{
		UserID: "me",
		Roles:  []rbac.Role{rbac.RoleMember, rbac.RoleOrgMember(defOrg)},
	}

	testAuthorize(t, "Member", user, []authTestCase{
		// Org + me + id
		{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithOwner(user.UserID).WithID(wrkID), actions: allActions(), allow: true},
		{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithOwner(user.UserID), actions: allActions(), allow: true},
		{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithID(wrkID), actions: allActions(), allow: false},
		{resource: rbac.ResourceWorkspace.InOrg(defOrg), actions: allActions(), allow: false},

		{resource: rbac.ResourceWorkspace.WithOwner(user.UserID).WithID(wrkID), actions: allActions(), allow: true},
		{resource: rbac.ResourceWorkspace.WithOwner(user.UserID), actions: allActions(), allow: true},

		{resource: rbac.ResourceWorkspace.WithID(wrkID), actions: allActions(), allow: false},

		{resource: rbac.ResourceWorkspace.All(), actions: allActions(), allow: false},

		// Other org + me + id
		{resource: rbac.ResourceWorkspace.InOrg("other").WithOwner(user.UserID).WithID(wrkID), actions: allActions(), allow: false},
		{resource: rbac.ResourceWorkspace.InOrg("other").WithOwner(user.UserID), actions: allActions(), allow: false},
		{resource: rbac.ResourceWorkspace.InOrg("other").WithID(wrkID), actions: allActions(), allow: false},
		{resource: rbac.ResourceWorkspace.InOrg("other"), actions: allActions(), allow: false},

		// Other org + other user + id
		{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithOwner("not-me").WithID(wrkID), actions: allActions(), allow: false},
		{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithOwner("not-me"), actions: allActions(), allow: false},

		{resource: rbac.ResourceWorkspace.WithOwner("not-me").WithID(wrkID), actions: allActions(), allow: false},
		{resource: rbac.ResourceWorkspace.WithOwner("not-me"), actions: allActions(), allow: false},

		// Other org + other use + other id
		{resource: rbac.ResourceWorkspace.InOrg("other").WithOwner("not-me").WithID("not-id"), actions: allActions(), allow: false},
		{resource: rbac.ResourceWorkspace.InOrg("other").WithOwner("not-me"), actions: allActions(), allow: false},
		{resource: rbac.ResourceWorkspace.InOrg("other").WithID("not-id"), actions: allActions(), allow: false},
		{resource: rbac.ResourceWorkspace.InOrg("other"), actions: allActions(), allow: false},

		{resource: rbac.ResourceWorkspace.WithOwner("not-me").WithID("not-id"), actions: allActions(), allow: false},
		{resource: rbac.ResourceWorkspace.WithOwner("not-me"), actions: allActions(), allow: false},

		{resource: rbac.ResourceWorkspace.WithID("not-id"), actions: allActions(), allow: false},
	})

	user = subject{
		UserID: "me",
		Roles: []rbac.Role{{
			Name: "deny-all",
			// List out deny permissions explicitly
			Site: []rbac.Permission{
				{
					Negate:       true,
					ResourceType: rbac.WildcardSymbol,
					ResourceID:   rbac.WildcardSymbol,
					Action:       rbac.WildcardSymbol,
				},
			},
		}},
	}

	testAuthorize(t, "DeletedMember", user, []authTestCase{
		// Org + me + id
		{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithOwner(user.UserID).WithID(wrkID), actions: allActions(), allow: false},
		{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithOwner(user.UserID), actions: allActions(), allow: false},
		{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithID(wrkID), actions: allActions(), allow: false},
		{resource: rbac.ResourceWorkspace.InOrg(defOrg), actions: allActions(), allow: false},

		{resource: rbac.ResourceWorkspace.WithOwner(user.UserID).WithID(wrkID), actions: allActions(), allow: false},
		{resource: rbac.ResourceWorkspace.WithOwner(user.UserID), actions: allActions(), allow: false},

		{resource: rbac.ResourceWorkspace.WithID(wrkID), actions: allActions(), allow: false},

		{resource: rbac.ResourceWorkspace.All(), actions: allActions(), allow: false},

		// Other org + me + id
		{resource: rbac.ResourceWorkspace.InOrg("other").WithOwner(user.UserID).WithID(wrkID), actions: allActions(), allow: false},
		{resource: rbac.ResourceWorkspace.InOrg("other").WithOwner(user.UserID), actions: allActions(), allow: false},
		{resource: rbac.ResourceWorkspace.InOrg("other").WithID(wrkID), actions: allActions(), allow: false},
		{resource: rbac.ResourceWorkspace.InOrg("other"), actions: allActions(), allow: false},

		// Other org + other user + id
		{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithOwner("not-me").WithID(wrkID), actions: allActions(), allow: false},
		{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithOwner("not-me"), actions: allActions(), allow: false},

		{resource: rbac.ResourceWorkspace.WithOwner("not-me").WithID(wrkID), actions: allActions(), allow: false},
		{resource: rbac.ResourceWorkspace.WithOwner("not-me"), actions: allActions(), allow: false},

		// Other org + other use + other id
		{resource: rbac.ResourceWorkspace.InOrg("other").WithOwner("not-me").WithID("not-id"), actions: allActions(), allow: false},
		{resource: rbac.ResourceWorkspace.InOrg("other").WithOwner("not-me"), actions: allActions(), allow: false},
		{resource: rbac.ResourceWorkspace.InOrg("other").WithID("not-id"), actions: allActions(), allow: false},
		{resource: rbac.ResourceWorkspace.InOrg("other"), actions: allActions(), allow: false},

		{resource: rbac.ResourceWorkspace.WithOwner("not-me").WithID("not-id"), actions: allActions(), allow: false},
		{resource: rbac.ResourceWorkspace.WithOwner("not-me"), actions: allActions(), allow: false},

		{resource: rbac.ResourceWorkspace.WithID("not-id"), actions: allActions(), allow: false},
	})

	user = subject{
		UserID: "me",
		Roles: []rbac.Role{
			rbac.RoleOrgAdmin(defOrg),
			rbac.RoleMember,
		},
	}

	testAuthorize(t, "OrgAdmin", user, []authTestCase{
		// Org + me + id
		{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithOwner(user.UserID).WithID(wrkID), actions: allActions(), allow: true},
		{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithOwner(user.UserID), actions: allActions(), allow: true},
		{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithID(wrkID), actions: allActions(), allow: true},
		{resource: rbac.ResourceWorkspace.InOrg(defOrg), actions: allActions(), allow: true},

		{resource: rbac.ResourceWorkspace.WithOwner(user.UserID).WithID(wrkID), actions: allActions(), allow: true},
		{resource: rbac.ResourceWorkspace.WithOwner(user.UserID), actions: allActions(), allow: true},

		{resource: rbac.ResourceWorkspace.WithID(wrkID), actions: allActions(), allow: false},

		{resource: rbac.ResourceWorkspace.All(), actions: allActions(), allow: false},

		// Other org + me + id
		{resource: rbac.ResourceWorkspace.InOrg("other").WithOwner(user.UserID).WithID(wrkID), actions: allActions(), allow: false},
		{resource: rbac.ResourceWorkspace.InOrg("other").WithOwner(user.UserID), actions: allActions(), allow: false},
		{resource: rbac.ResourceWorkspace.InOrg("other").WithID(wrkID), actions: allActions(), allow: false},
		{resource: rbac.ResourceWorkspace.InOrg("other"), actions: allActions(), allow: false},

		// Other org + other user + id
		{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithOwner("not-me").WithID(wrkID), actions: allActions(), allow: true},
		{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithOwner("not-me"), actions: allActions(), allow: true},

		{resource: rbac.ResourceWorkspace.WithOwner("not-me").WithID(wrkID), actions: allActions(), allow: false},
		{resource: rbac.ResourceWorkspace.WithOwner("not-me"), actions: allActions(), allow: false},

		// Other org + other use + other id
		{resource: rbac.ResourceWorkspace.InOrg("other").WithOwner("not-me").WithID("not-id"), actions: allActions(), allow: false},
		{resource: rbac.ResourceWorkspace.InOrg("other").WithOwner("not-me"), actions: allActions(), allow: false},
		{resource: rbac.ResourceWorkspace.InOrg("other").WithID("not-id"), actions: allActions(), allow: false},
		{resource: rbac.ResourceWorkspace.InOrg("other"), actions: allActions(), allow: false},

		{resource: rbac.ResourceWorkspace.WithOwner("not-me").WithID("not-id"), actions: allActions(), allow: false},
		{resource: rbac.ResourceWorkspace.WithOwner("not-me"), actions: allActions(), allow: false},

		{resource: rbac.ResourceWorkspace.WithID("not-id"), actions: allActions(), allow: false},
	})

	user = subject{
		UserID: "me",
		Roles: []rbac.Role{
			rbac.RoleAdmin,
			rbac.RoleMember,
		},
	}

	testAuthorize(t, "SiteAdmin", user, []authTestCase{
		// Org + me + id
		{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithOwner(user.UserID).WithID(wrkID), actions: allActions(), allow: true},
		{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithOwner(user.UserID), actions: allActions(), allow: true},
		{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithID(wrkID), actions: allActions(), allow: true},
		{resource: rbac.ResourceWorkspace.InOrg(defOrg), actions: allActions(), allow: true},

		{resource: rbac.ResourceWorkspace.WithOwner(user.UserID).WithID(wrkID), actions: allActions(), allow: true},
		{resource: rbac.ResourceWorkspace.WithOwner(user.UserID), actions: allActions(), allow: true},

		{resource: rbac.ResourceWorkspace.WithID(wrkID), actions: allActions(), allow: true},

		{resource: rbac.ResourceWorkspace.All(), actions: allActions(), allow: true},

		// Other org + me + id
		{resource: rbac.ResourceWorkspace.InOrg("other").WithOwner(user.UserID).WithID(wrkID), actions: allActions(), allow: true},
		{resource: rbac.ResourceWorkspace.InOrg("other").WithOwner(user.UserID), actions: allActions(), allow: true},
		{resource: rbac.ResourceWorkspace.InOrg("other").WithID(wrkID), actions: allActions(), allow: true},
		{resource: rbac.ResourceWorkspace.InOrg("other"), actions: allActions(), allow: true},

		// Other org + other user + id
		{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithOwner("not-me").WithID(wrkID), actions: allActions(), allow: true},
		{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithOwner("not-me"), actions: allActions(), allow: true},

		{resource: rbac.ResourceWorkspace.WithOwner("not-me").WithID(wrkID), actions: allActions(), allow: true},
		{resource: rbac.ResourceWorkspace.WithOwner("not-me"), actions: allActions(), allow: true},

		// Other org + other use + other id
		{resource: rbac.ResourceWorkspace.InOrg("other").WithOwner("not-me").WithID("not-id"), actions: allActions(), allow: true},
		{resource: rbac.ResourceWorkspace.InOrg("other").WithOwner("not-me"), actions: allActions(), allow: true},
		{resource: rbac.ResourceWorkspace.InOrg("other").WithID("not-id"), actions: allActions(), allow: true},
		{resource: rbac.ResourceWorkspace.InOrg("other"), actions: allActions(), allow: true},

		{resource: rbac.ResourceWorkspace.WithOwner("not-me").WithID("not-id"), actions: allActions(), allow: true},
		{resource: rbac.ResourceWorkspace.WithOwner("not-me"), actions: allActions(), allow: true},

		{resource: rbac.ResourceWorkspace.WithID("not-id"), actions: allActions(), allow: true},
	})

	// In practice this is a token scope on a regular subject
	user = subject{
		UserID: "me",
		Roles: []rbac.Role{
			rbac.RoleWorkspaceAgent(wrkID),
		},
	}

	testAuthorize(t, "WorkspaceAgentToken", user,
		// Read Actions
		cases(func(c authTestCase) authTestCase {
			c.actions = []rbac.Action{rbac.ActionRead}
			return c
		}, []authTestCase{
			// Org + me + id
			{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithOwner(user.UserID).WithID(wrkID), allow: true},
			{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithOwner(user.UserID), allow: false},
			{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithID(wrkID), allow: true},
			{resource: rbac.ResourceWorkspace.InOrg(defOrg), allow: false},

			{resource: rbac.ResourceWorkspace.WithOwner(user.UserID).WithID(wrkID), allow: true},
			{resource: rbac.ResourceWorkspace.WithOwner(user.UserID), allow: false},

			{resource: rbac.ResourceWorkspace.WithID(wrkID), allow: true},

			{resource: rbac.ResourceWorkspace.All(), allow: false},

			// Other org + me + id
			{resource: rbac.ResourceWorkspace.InOrg("other").WithOwner(user.UserID).WithID(wrkID), allow: true},
			{resource: rbac.ResourceWorkspace.InOrg("other").WithOwner(user.UserID), allow: false},
			{resource: rbac.ResourceWorkspace.InOrg("other").WithID(wrkID), allow: true},
			{resource: rbac.ResourceWorkspace.InOrg("other"), allow: false},

			// Other org + other user + id
			{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithOwner("not-me").WithID(wrkID), allow: true},
			{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithOwner("not-me"), allow: false},

			{resource: rbac.ResourceWorkspace.WithOwner("not-me").WithID(wrkID), allow: true},
			{resource: rbac.ResourceWorkspace.WithOwner("not-me"), allow: false},

			// Other org + other use + other id
			{resource: rbac.ResourceWorkspace.InOrg("other").WithOwner("not-me").WithID("not-id"), allow: false},
			{resource: rbac.ResourceWorkspace.InOrg("other").WithOwner("not-me"), allow: false},
			{resource: rbac.ResourceWorkspace.InOrg("other").WithID("not-id"), allow: false},
			{resource: rbac.ResourceWorkspace.InOrg("other"), allow: false},

			{resource: rbac.ResourceWorkspace.WithOwner("not-me").WithID("not-id"), allow: false},
			{resource: rbac.ResourceWorkspace.WithOwner("not-me"), allow: false},

			{resource: rbac.ResourceWorkspace.WithID("not-id"), allow: false},
		}),
		// Not read actions
		cases(func(c authTestCase) authTestCase {
			c.actions = []rbac.Action{rbac.ActionCreate, rbac.ActionUpdate, rbac.ActionDelete}
			c.allow = false
			return c
		}, []authTestCase{
			// Org + me + id
			{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithOwner(user.UserID).WithID(wrkID)},
			{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithOwner(user.UserID)},
			{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithID(wrkID)},
			{resource: rbac.ResourceWorkspace.InOrg(defOrg)},

			{resource: rbac.ResourceWorkspace.WithOwner(user.UserID).WithID(wrkID)},
			{resource: rbac.ResourceWorkspace.WithOwner(user.UserID)},

			{resource: rbac.ResourceWorkspace.WithID(wrkID)},

			{resource: rbac.ResourceWorkspace.All()},

			// Other org + me + id
			{resource: rbac.ResourceWorkspace.InOrg("other").WithOwner(user.UserID).WithID(wrkID)},
			{resource: rbac.ResourceWorkspace.InOrg("other").WithOwner(user.UserID)},
			{resource: rbac.ResourceWorkspace.InOrg("other").WithID(wrkID)},
			{resource: rbac.ResourceWorkspace.InOrg("other")},

			// Other org + other user + id
			{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithOwner("not-me").WithID(wrkID)},
			{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithOwner("not-me")},

			{resource: rbac.ResourceWorkspace.WithOwner("not-me").WithID(wrkID)},
			{resource: rbac.ResourceWorkspace.WithOwner("not-me")},

			// Other org + other use + other id
			{resource: rbac.ResourceWorkspace.InOrg("other").WithOwner("not-me").WithID("not-id")},
			{resource: rbac.ResourceWorkspace.InOrg("other").WithOwner("not-me")},
			{resource: rbac.ResourceWorkspace.InOrg("other").WithID("not-id")},
			{resource: rbac.ResourceWorkspace.InOrg("other")},

			{resource: rbac.ResourceWorkspace.WithOwner("not-me").WithID("not-id")},
			{resource: rbac.ResourceWorkspace.WithOwner("not-me")},

			{resource: rbac.ResourceWorkspace.WithID("not-id")},
		}),
	)

	// In practice this is a token scope on a regular subject
	user = subject{
		UserID: "me",
		Roles: []rbac.Role{
			{
				Name: "ReadOnlyOrgAndUser",
				Site: []rbac.Permission{},
				Org: map[string][]rbac.Permission{
					defOrg: {{
						Negate:       false,
						ResourceType: "*",
						ResourceID:   "*",
						Action:       rbac.ActionRead,
					}},
				},
				User: []rbac.Permission{
					{
						Negate:       false,
						ResourceType: "*",
						ResourceID:   "*",
						Action:       rbac.ActionRead,
					},
				},
			},
		},
	}

	testAuthorize(t, "ReadOnly", user,
		cases(func(c authTestCase) authTestCase {
			c.actions = []rbac.Action{rbac.ActionRead}
			return c
		}, []authTestCase{
			// Read
			// Org + me + id
			{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithOwner(user.UserID).WithID(wrkID), allow: true},
			{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithOwner(user.UserID), allow: true},
			{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithID(wrkID), allow: true},
			{resource: rbac.ResourceWorkspace.InOrg(defOrg), allow: true},

			{resource: rbac.ResourceWorkspace.WithOwner(user.UserID).WithID(wrkID), allow: true},
			{resource: rbac.ResourceWorkspace.WithOwner(user.UserID), allow: true},

			{resource: rbac.ResourceWorkspace.WithID(wrkID), allow: false},

			{resource: rbac.ResourceWorkspace.All(), allow: false},

			// Other org + me + id
			{resource: rbac.ResourceWorkspace.InOrg("other").WithOwner(user.UserID).WithID(wrkID), allow: false},
			{resource: rbac.ResourceWorkspace.InOrg("other").WithOwner(user.UserID), allow: false},
			{resource: rbac.ResourceWorkspace.InOrg("other").WithID(wrkID), allow: false},
			{resource: rbac.ResourceWorkspace.InOrg("other"), allow: false},

			// Other org + other user + id
			{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithOwner("not-me").WithID(wrkID), allow: true},
			{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithOwner("not-me"), allow: true},

			{resource: rbac.ResourceWorkspace.WithOwner("not-me").WithID(wrkID), allow: false},
			{resource: rbac.ResourceWorkspace.WithOwner("not-me"), allow: false},

			// Other org + other use + other id
			{resource: rbac.ResourceWorkspace.InOrg("other").WithOwner("not-me").WithID("not-id"), allow: false},
			{resource: rbac.ResourceWorkspace.InOrg("other").WithOwner("not-me"), allow: false},
			{resource: rbac.ResourceWorkspace.InOrg("other").WithID("not-id"), allow: false},
			{resource: rbac.ResourceWorkspace.InOrg("other"), allow: false},

			{resource: rbac.ResourceWorkspace.WithOwner("not-me").WithID("not-id"), allow: false},
			{resource: rbac.ResourceWorkspace.WithOwner("not-me"), allow: false},

			{resource: rbac.ResourceWorkspace.WithID("not-id"), allow: false},
		}),

		// Pass non-read actions
		cases(func(c authTestCase) authTestCase {
			c.actions = []rbac.Action{rbac.ActionCreate, rbac.ActionUpdate, rbac.ActionDelete}
			c.allow = false
			return c
		}, []authTestCase{
			// Read
			// Org + me + id
			{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithOwner(user.UserID).WithID(wrkID)},
			{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithOwner(user.UserID)},
			{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithID(wrkID)},
			{resource: rbac.ResourceWorkspace.InOrg(defOrg)},

			{resource: rbac.ResourceWorkspace.WithOwner(user.UserID).WithID(wrkID)},
			{resource: rbac.ResourceWorkspace.WithOwner(user.UserID)},

			{resource: rbac.ResourceWorkspace.WithID(wrkID)},

			{resource: rbac.ResourceWorkspace.All()},

			// Other org + me + id
			{resource: rbac.ResourceWorkspace.InOrg("other").WithOwner(user.UserID).WithID(wrkID)},
			{resource: rbac.ResourceWorkspace.InOrg("other").WithOwner(user.UserID)},
			{resource: rbac.ResourceWorkspace.InOrg("other").WithID(wrkID)},
			{resource: rbac.ResourceWorkspace.InOrg("other")},

			// Other org + other user + id
			{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithOwner("not-me").WithID(wrkID)},
			{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithOwner("not-me")},

			{resource: rbac.ResourceWorkspace.WithOwner("not-me").WithID(wrkID)},
			{resource: rbac.ResourceWorkspace.WithOwner("not-me")},

			// Other org + other use + other id
			{resource: rbac.ResourceWorkspace.InOrg("other").WithOwner("not-me").WithID("not-id")},
			{resource: rbac.ResourceWorkspace.InOrg("other").WithOwner("not-me")},
			{resource: rbac.ResourceWorkspace.InOrg("other").WithID("not-id")},
			{resource: rbac.ResourceWorkspace.InOrg("other")},

			{resource: rbac.ResourceWorkspace.WithOwner("not-me").WithID("not-id")},
			{resource: rbac.ResourceWorkspace.WithOwner("not-me")},

			{resource: rbac.ResourceWorkspace.WithID("not-id")},
		}))
}

// TestAuthorizeLevels ensures level overrides are acting appropriately
//nolint:paralleltest
func TestAuthorizeLevels(t *testing.T) {
	defOrg := "default"
	wrkID := "1234"

	user := subject{
		UserID: "me",
		Roles: []rbac.Role{
			rbac.RoleAdmin,
			rbac.RoleOrgDenyAll(defOrg),
			{
				Name: "user-deny-all",
				// List out deny permissions explicitly
				User: []rbac.Permission{
					{
						Negate:       true,
						ResourceType: rbac.WildcardSymbol,
						ResourceID:   rbac.WildcardSymbol,
						Action:       rbac.WildcardSymbol,
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
			{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithOwner(user.UserID).WithID(wrkID)},
			{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithOwner(user.UserID)},
			{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithID(wrkID)},
			{resource: rbac.ResourceWorkspace.InOrg(defOrg)},

			{resource: rbac.ResourceWorkspace.WithOwner(user.UserID).WithID(wrkID)},
			{resource: rbac.ResourceWorkspace.WithOwner(user.UserID)},

			{resource: rbac.ResourceWorkspace.WithID(wrkID)},

			{resource: rbac.ResourceWorkspace.All()},

			// Other org + me + id
			{resource: rbac.ResourceWorkspace.InOrg("other").WithOwner(user.UserID).WithID(wrkID)},
			{resource: rbac.ResourceWorkspace.InOrg("other").WithOwner(user.UserID)},
			{resource: rbac.ResourceWorkspace.InOrg("other").WithID(wrkID)},
			{resource: rbac.ResourceWorkspace.InOrg("other")},

			// Other org + other user + id
			{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithOwner("not-me").WithID(wrkID)},
			{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithOwner("not-me")},

			{resource: rbac.ResourceWorkspace.WithOwner("not-me").WithID(wrkID)},
			{resource: rbac.ResourceWorkspace.WithOwner("not-me")},

			// Other org + other use + other id
			{resource: rbac.ResourceWorkspace.InOrg("other").WithOwner("not-me").WithID("not-id")},
			{resource: rbac.ResourceWorkspace.InOrg("other").WithOwner("not-me")},
			{resource: rbac.ResourceWorkspace.InOrg("other").WithID("not-id")},
			{resource: rbac.ResourceWorkspace.InOrg("other")},

			{resource: rbac.ResourceWorkspace.WithOwner("not-me").WithID("not-id")},
			{resource: rbac.ResourceWorkspace.WithOwner("not-me")},

			{resource: rbac.ResourceWorkspace.WithID("not-id")},
		}))

	user = subject{
		UserID: "me",
		Roles: []rbac.Role{
			{
				Name: "site-noise",
				Site: []rbac.Permission{
					{
						Negate:       true,
						ResourceType: "random",
						ResourceID:   rbac.WildcardSymbol,
						Action:       rbac.WildcardSymbol,
					},
				},
			},
			rbac.RoleOrgAdmin(defOrg),
			{
				Name: "user-deny-all",
				// List out deny permissions explicitly
				User: []rbac.Permission{
					{
						Negate:       true,
						ResourceType: rbac.WildcardSymbol,
						ResourceID:   rbac.WildcardSymbol,
						Action:       rbac.WildcardSymbol,
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
			{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithOwner(user.UserID).WithID(wrkID), allow: true},
			{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithOwner(user.UserID), allow: true},
			{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithID(wrkID), allow: true},
			{resource: rbac.ResourceWorkspace.InOrg(defOrg), allow: true},

			{resource: rbac.ResourceWorkspace.WithOwner(user.UserID).WithID(wrkID), allow: false},
			{resource: rbac.ResourceWorkspace.WithOwner(user.UserID), allow: false},

			{resource: rbac.ResourceWorkspace.WithID(wrkID), allow: false},

			{resource: rbac.ResourceWorkspace.All(), allow: false},

			// Other org + me + id
			{resource: rbac.ResourceWorkspace.InOrg("other").WithOwner(user.UserID).WithID(wrkID), allow: false},
			{resource: rbac.ResourceWorkspace.InOrg("other").WithOwner(user.UserID), allow: false},
			{resource: rbac.ResourceWorkspace.InOrg("other").WithID(wrkID), allow: false},
			{resource: rbac.ResourceWorkspace.InOrg("other"), allow: false},

			// Other org + other user + id
			{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithOwner("not-me").WithID(wrkID), allow: true},
			{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithOwner("not-me"), allow: true},

			{resource: rbac.ResourceWorkspace.WithOwner("not-me").WithID(wrkID), allow: false},
			{resource: rbac.ResourceWorkspace.WithOwner("not-me"), allow: false},

			// Other org + other use + other id
			{resource: rbac.ResourceWorkspace.InOrg("other").WithOwner("not-me").WithID("not-id"), allow: false},
			{resource: rbac.ResourceWorkspace.InOrg("other").WithOwner("not-me"), allow: false},
			{resource: rbac.ResourceWorkspace.InOrg("other").WithID("not-id"), allow: false},
			{resource: rbac.ResourceWorkspace.InOrg("other"), allow: false},

			{resource: rbac.ResourceWorkspace.WithOwner("not-me").WithID("not-id"), allow: false},
			{resource: rbac.ResourceWorkspace.WithOwner("not-me"), allow: false},

			{resource: rbac.ResourceWorkspace.WithID("not-id"), allow: false},
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
	resource rbac.Object
	actions  []rbac.Action
	allow    bool
}

func testAuthorize(t *testing.T, name string, subject subject, sets ...[]authTestCase) {
	authorizer, err := rbac.NewAuthorizer()
	require.NoError(t, err)
	for _, cases := range sets {
		for _, c := range cases {
			t.Run(name, func(t *testing.T) {
				for _, a := range c.actions {
					err := authorizer.Authorize(context.Background(), subject.UserID, subject.Roles, a, c.resource)
					if c.allow {
						if err != nil {
							var uerr *rbac.UnauthorizedError
							xerrors.As(err, &uerr)
							d, _ := json.Marshal(uerr.Input())
							t.Logf("input: %s", string(d))
							t.Logf("internal error: %+v", uerr.Internal().Error())
							t.Logf("output: %+v", uerr.Output())
						}
						require.NoError(t, err, "expected no error for testcase action %s", a)
						continue
					}

					if err == nil {
						d, _ := json.Marshal(map[string]interface{}{
							"subject": subject,
							"object":  c.resource,
							"action":  a,
						})
						t.Log(string(d))
					}
					require.Error(t, err, "expected unauthorized")
				}
			})
		}
	}
}

// allActions is a helper function to return all the possible actions types.
func allActions() []rbac.Action {
	return []rbac.Action{rbac.ActionCreate, rbac.ActionRead, rbac.ActionUpdate, rbac.ActionDelete}
}

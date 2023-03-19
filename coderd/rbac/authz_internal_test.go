package rbac

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/testutil"
)

type fakeObject struct {
	Owner    uuid.UUID
	OrgOwner uuid.UUID
	Type     string
	Allowed  bool
}

func (w fakeObject) RBACObject() Object {
	return Object{
		Owner: w.Owner.String(),
		OrgID: w.OrgOwner.String(),
		Type:  w.Type,
	}
}

func TestFilterError(t *testing.T) {
	t.Parallel()
	auth := NewAuthorizer(prometheus.NewRegistry())
	subject := Subject{
		ID:     uuid.NewString(),
		Roles:  RoleNames{},
		Groups: []string{},
		Scope:  ScopeAll,
	}

	_, err := Filter(context.Background(), auth, subject, ActionRead, []Object{ResourceUser, ResourceWorkspace})
	require.ErrorContains(t, err, "object types must be uniform")
}

// TestFilter ensures the filter acts the same as an individual authorize.
// It generates a random set of objects, then runs the Filter batch function
// against the singular Authorize function.
func TestFilter(t *testing.T) {
	t.Parallel()

	orgIDs := make([]uuid.UUID, 10)
	userIDs := make([]uuid.UUID, len(orgIDs))
	for i := range orgIDs {
		orgIDs[i] = uuid.New()
		userIDs[i] = uuid.New()
	}
	objects := make([]fakeObject, 0, len(userIDs)*len(orgIDs))
	for i := range userIDs {
		for j := range orgIDs {
			objects = append(objects, fakeObject{
				Owner:    userIDs[i],
				OrgOwner: orgIDs[j],
				Type:     ResourceWorkspace.Type,
				Allowed:  false,
			})
		}
	}

	testCases := []struct {
		Name       string
		Actor      Subject
		Action     Action
		ObjectType string
	}{
		{
			Name: "NoRoles",
			Actor: Subject{
				ID:    userIDs[0].String(),
				Roles: RoleNames{},
			},
			ObjectType: ResourceWorkspace.Type,
			Action:     ActionRead,
		},
		{
			Name: "Admin",
			Actor: Subject{
				ID:    userIDs[0].String(),
				Roles: RoleNames{RoleOrgMember(orgIDs[0]), "auditor", RoleOwner(), RoleMember()},
			},
			ObjectType: ResourceWorkspace.Type,
			Action:     ActionRead,
		},
		{
			Name: "OrgAdmin",
			Actor: Subject{
				ID:    userIDs[0].String(),
				Roles: RoleNames{RoleOrgMember(orgIDs[0]), RoleOrgAdmin(orgIDs[0]), RoleMember()},
			},
			ObjectType: ResourceWorkspace.Type,
			Action:     ActionRead,
		},
		{
			Name: "OrgMember",
			Actor: Subject{
				ID:    userIDs[0].String(),
				Roles: RoleNames{RoleOrgMember(orgIDs[0]), RoleOrgMember(orgIDs[1]), RoleMember()},
			},
			ObjectType: ResourceWorkspace.Type,
			Action:     ActionRead,
		},
		{
			Name: "ManyRoles",
			Actor: Subject{
				ID: userIDs[0].String(),
				Roles: RoleNames{
					RoleOrgMember(orgIDs[0]), RoleOrgAdmin(orgIDs[0]),
					RoleOrgMember(orgIDs[1]), RoleOrgAdmin(orgIDs[1]),
					RoleOrgMember(orgIDs[2]), RoleOrgAdmin(orgIDs[2]),
					RoleOrgMember(orgIDs[4]),
					RoleOrgMember(orgIDs[5]),
					RoleMember(),
				},
			},
			ObjectType: ResourceWorkspace.Type,
			Action:     ActionRead,
		},
		{
			Name: "SiteMember",
			Actor: Subject{
				ID:    userIDs[0].String(),
				Roles: RoleNames{RoleMember()},
			},
			ObjectType: ResourceUser.Type,
			Action:     ActionRead,
		},
		{
			Name: "ReadOrgs",
			Actor: Subject{
				ID: userIDs[0].String(),
				Roles: RoleNames{
					RoleOrgMember(orgIDs[0]),
					RoleOrgMember(orgIDs[1]),
					RoleOrgMember(orgIDs[2]),
					RoleOrgMember(orgIDs[3]),
					RoleMember(),
				},
			},
			ObjectType: ResourceOrganization.Type,
			Action:     ActionRead,
		},
		{
			Name: "ScopeApplicationConnect",
			Actor: Subject{
				ID:    userIDs[0].String(),
				Roles: RoleNames{RoleOrgMember(orgIDs[0]), "auditor", RoleOwner(), RoleMember()},
			},
			ObjectType: ResourceWorkspace.Type,
			Action:     ActionRead,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			actor := tc.Actor

			localObjects := make([]fakeObject, len(objects))
			copy(localObjects, objects)

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
			defer cancel()
			auth := NewAuthorizer(prometheus.NewRegistry())

			if actor.Scope == nil {
				// Default to ScopeAll
				actor.Scope = ScopeAll
			}

			// Run auth 1 by 1
			var allowedCount int
			for i, obj := range localObjects {
				obj.Type = tc.ObjectType
				err := auth.Authorize(ctx, actor, ActionRead, obj.RBACObject())
				obj.Allowed = err == nil
				if err == nil {
					allowedCount++
				}
				localObjects[i] = obj
			}

			// Run by filter
			list, err := Filter(ctx, auth, actor, tc.Action, localObjects)
			require.NoError(t, err)
			require.Equal(t, allowedCount, len(list), "expected number of allowed")
			for _, obj := range list {
				require.True(t, obj.Allowed, "expected allowed")
			}
		})
	}
}

// TestAuthorizeDomain test the very basic roles that are commonly used.
func TestAuthorizeDomain(t *testing.T) {
	t.Parallel()
	defOrg := uuid.New()
	unuseID := uuid.New()
	allUsersGroup := "Everyone"

	user := Subject{
		ID:     "me",
		Scope:  must(ExpandScope(ScopeAll)),
		Groups: []string{allUsersGroup},
		Roles: Roles{
			must(RoleByName(RoleMember())),
			must(RoleByName(RoleOrgMember(defOrg))),
		},
	}

	testAuthorize(t, "UserACLList", user, []authTestCase{
		{
			resource: ResourceWorkspace.WithOwner(unuseID.String()).InOrg(unuseID).WithACLUserList(map[string][]Action{
				user.ID: AllActions(),
			}),
			actions: AllActions(),
			allow:   true,
		},
		{
			resource: ResourceWorkspace.WithOwner(unuseID.String()).InOrg(unuseID).WithACLUserList(map[string][]Action{
				user.ID: {WildcardSymbol},
			}),
			actions: AllActions(),
			allow:   true,
		},
		{
			resource: ResourceWorkspace.WithOwner(unuseID.String()).InOrg(unuseID).WithACLUserList(map[string][]Action{
				user.ID: {ActionRead, ActionUpdate},
			}),
			actions: []Action{ActionCreate, ActionDelete},
			allow:   false,
		},
		{
			// By default users cannot update templates
			resource: ResourceTemplate.InOrg(defOrg).WithACLUserList(map[string][]Action{
				user.ID: {ActionUpdate},
			}),
			actions: []Action{ActionUpdate},
			allow:   true,
		},
	})

	testAuthorize(t, "GroupACLList", user, []authTestCase{
		{
			resource: ResourceWorkspace.WithOwner(unuseID.String()).InOrg(defOrg).WithGroupACL(map[string][]Action{
				allUsersGroup: AllActions(),
			}),
			actions: AllActions(),
			allow:   true,
		},
		{
			resource: ResourceWorkspace.WithOwner(unuseID.String()).InOrg(defOrg).WithGroupACL(map[string][]Action{
				allUsersGroup: {WildcardSymbol},
			}),
			actions: AllActions(),
			allow:   true,
		},
		{
			resource: ResourceWorkspace.WithOwner(unuseID.String()).InOrg(defOrg).WithGroupACL(map[string][]Action{
				allUsersGroup: {ActionRead, ActionUpdate},
			}),
			actions: []Action{ActionCreate, ActionDelete},
			allow:   false,
		},
		{
			// By default users cannot update templates
			resource: ResourceTemplate.InOrg(defOrg).WithGroupACL(map[string][]Action{
				allUsersGroup: {ActionUpdate},
			}),
			actions: []Action{ActionUpdate},
			allow:   true,
		},
	})

	testAuthorize(t, "Member", user, []authTestCase{
		// Org + me
		{resource: ResourceWorkspace.InOrg(defOrg).WithOwner(user.ID), actions: AllActions(), allow: true},
		{resource: ResourceWorkspace.InOrg(defOrg), actions: AllActions(), allow: false},

		{resource: ResourceWorkspace.WithOwner(user.ID), actions: AllActions(), allow: true},

		{resource: ResourceWorkspace.All(), actions: AllActions(), allow: false},

		// Other org + me
		{resource: ResourceWorkspace.InOrg(unuseID).WithOwner(user.ID), actions: AllActions(), allow: false},
		{resource: ResourceWorkspace.InOrg(unuseID), actions: AllActions(), allow: false},

		// Other org + other user
		{resource: ResourceWorkspace.InOrg(defOrg).WithOwner("not-me"), actions: AllActions(), allow: false},

		{resource: ResourceWorkspace.WithOwner("not-me"), actions: AllActions(), allow: false},

		// Other org + other us
		{resource: ResourceWorkspace.InOrg(unuseID).WithOwner("not-me"), actions: AllActions(), allow: false},
		{resource: ResourceWorkspace.InOrg(unuseID), actions: AllActions(), allow: false},

		{resource: ResourceWorkspace.WithOwner("not-me"), actions: AllActions(), allow: false},
	})

	user = Subject{
		ID:    "me",
		Scope: must(ExpandScope(ScopeAll)),
		Roles: Roles{{
			Name: "deny-all",
			// List out deny permissions explicitly
			Site: []Permission{
				{
					Negate:       true,
					ResourceType: WildcardSymbol,
					Action:       WildcardSymbol,
				},
			},
		}},
	}

	testAuthorize(t, "DeletedMember", user, []authTestCase{
		// Org + me
		{resource: ResourceWorkspace.InOrg(defOrg).WithOwner(user.ID), actions: AllActions(), allow: false},
		{resource: ResourceWorkspace.InOrg(defOrg), actions: AllActions(), allow: false},

		{resource: ResourceWorkspace.WithOwner(user.ID), actions: AllActions(), allow: false},

		{resource: ResourceWorkspace.All(), actions: AllActions(), allow: false},

		// Other org + me
		{resource: ResourceWorkspace.InOrg(unuseID).WithOwner(user.ID), actions: AllActions(), allow: false},
		{resource: ResourceWorkspace.InOrg(unuseID), actions: AllActions(), allow: false},

		// Other org + other user
		{resource: ResourceWorkspace.InOrg(defOrg).WithOwner("not-me"), actions: AllActions(), allow: false},

		{resource: ResourceWorkspace.WithOwner("not-me"), actions: AllActions(), allow: false},

		// Other org + other use
		{resource: ResourceWorkspace.InOrg(unuseID).WithOwner("not-me"), actions: AllActions(), allow: false},
		{resource: ResourceWorkspace.InOrg(unuseID), actions: AllActions(), allow: false},

		{resource: ResourceWorkspace.WithOwner("not-me"), actions: AllActions(), allow: false},
	})

	user = Subject{
		ID:    "me",
		Scope: must(ExpandScope(ScopeAll)),
		Roles: Roles{
			must(RoleByName(RoleOrgAdmin(defOrg))),
			must(RoleByName(RoleMember())),
		},
	}

	testAuthorize(t, "OrgAdmin", user, []authTestCase{
		// Org + me
		{resource: ResourceWorkspace.InOrg(defOrg).WithOwner(user.ID), actions: AllActions(), allow: true},
		{resource: ResourceWorkspace.InOrg(defOrg), actions: AllActions(), allow: true},

		{resource: ResourceWorkspace.WithOwner(user.ID), actions: AllActions(), allow: true},

		{resource: ResourceWorkspace.All(), actions: AllActions(), allow: false},

		// Other org + me
		{resource: ResourceWorkspace.InOrg(unuseID).WithOwner(user.ID), actions: AllActions(), allow: false},
		{resource: ResourceWorkspace.InOrg(unuseID), actions: AllActions(), allow: false},

		// Other org + other user
		{resource: ResourceWorkspace.InOrg(defOrg).WithOwner("not-me"), actions: AllActions(), allow: true},

		{resource: ResourceWorkspace.WithOwner("not-me"), actions: AllActions(), allow: false},

		// Other org + other use
		{resource: ResourceWorkspace.InOrg(unuseID).WithOwner("not-me"), actions: AllActions(), allow: false},
		{resource: ResourceWorkspace.InOrg(unuseID), actions: AllActions(), allow: false},

		{resource: ResourceWorkspace.WithOwner("not-me"), actions: AllActions(), allow: false},
	})

	user = Subject{
		ID:    "me",
		Scope: must(ExpandScope(ScopeAll)),
		Roles: Roles{
			must(RoleByName(RoleOwner())),
			must(RoleByName(RoleMember())),
		},
	}

	testAuthorize(t, "SiteAdmin", user, []authTestCase{
		// Org + me
		{resource: ResourceWorkspace.InOrg(defOrg).WithOwner(user.ID), actions: AllActions(), allow: true},
		{resource: ResourceWorkspace.InOrg(defOrg), actions: AllActions(), allow: true},

		{resource: ResourceWorkspace.WithOwner(user.ID), actions: AllActions(), allow: true},

		{resource: ResourceWorkspace.All(), actions: AllActions(), allow: true},

		// Other org + me
		{resource: ResourceWorkspace.InOrg(unuseID).WithOwner(user.ID), actions: AllActions(), allow: true},
		{resource: ResourceWorkspace.InOrg(unuseID), actions: AllActions(), allow: true},

		// Other org + other user
		{resource: ResourceWorkspace.InOrg(defOrg).WithOwner("not-me"), actions: AllActions(), allow: true},

		{resource: ResourceWorkspace.WithOwner("not-me"), actions: AllActions(), allow: true},

		// Other org + other use
		{resource: ResourceWorkspace.InOrg(unuseID).WithOwner("not-me"), actions: AllActions(), allow: true},
		{resource: ResourceWorkspace.InOrg(unuseID), actions: AllActions(), allow: true},

		{resource: ResourceWorkspace.WithOwner("not-me"), actions: AllActions(), allow: true},
	})

	user = Subject{
		ID:    "me",
		Scope: must(ExpandScope(ScopeApplicationConnect)),
		Roles: Roles{
			must(RoleByName(RoleOrgMember(defOrg))),
			must(RoleByName(RoleMember())),
		},
	}

	testAuthorize(t, "ApplicationToken", user,
		// Create (connect) Actions
		cases(func(c authTestCase) authTestCase {
			c.actions = []Action{ActionCreate}
			return c
		}, []authTestCase{
			// Org + me
			{resource: ResourceWorkspaceApplicationConnect.InOrg(defOrg).WithOwner(user.ID), allow: true},
			{resource: ResourceWorkspaceApplicationConnect.InOrg(defOrg), allow: false},

			{resource: ResourceWorkspaceApplicationConnect.WithOwner(user.ID), allow: true},

			{resource: ResourceWorkspaceApplicationConnect.All(), allow: false},

			// Other org + me
			{resource: ResourceWorkspaceApplicationConnect.InOrg(unuseID).WithOwner(user.ID), allow: false},
			{resource: ResourceWorkspaceApplicationConnect.InOrg(unuseID), allow: false},

			// Other org + other user
			{resource: ResourceWorkspaceApplicationConnect.InOrg(defOrg).WithOwner("not-me"), allow: false},

			{resource: ResourceWorkspaceApplicationConnect.WithOwner("not-me"), allow: false},

			// Other org + other use
			{resource: ResourceWorkspaceApplicationConnect.InOrg(unuseID).WithOwner("not-me"), allow: false},
			{resource: ResourceWorkspaceApplicationConnect.InOrg(unuseID), allow: false},

			{resource: ResourceWorkspaceApplicationConnect.WithOwner("not-me"), allow: false},
		}),
		// Not create actions
		cases(func(c authTestCase) authTestCase {
			c.actions = []Action{ActionRead, ActionUpdate, ActionDelete}
			c.allow = false
			return c
		}, []authTestCase{
			// Org + me
			{resource: ResourceWorkspaceApplicationConnect.InOrg(defOrg).WithOwner(user.ID)},
			{resource: ResourceWorkspaceApplicationConnect.InOrg(defOrg)},

			{resource: ResourceWorkspaceApplicationConnect.WithOwner(user.ID)},

			{resource: ResourceWorkspaceApplicationConnect.All()},

			// Other org + me
			{resource: ResourceWorkspaceApplicationConnect.InOrg(unuseID).WithOwner(user.ID)},
			{resource: ResourceWorkspaceApplicationConnect.InOrg(unuseID)},

			// Other org + other user
			{resource: ResourceWorkspaceApplicationConnect.InOrg(defOrg).WithOwner("not-me")},

			{resource: ResourceWorkspaceApplicationConnect.WithOwner("not-me")},

			// Other org + other use
			{resource: ResourceWorkspaceApplicationConnect.InOrg(unuseID).WithOwner("not-me")},
			{resource: ResourceWorkspaceApplicationConnect.InOrg(unuseID)},

			{resource: ResourceWorkspaceApplicationConnect.WithOwner("not-me")},
		}),
		// Other Objects
		cases(func(c authTestCase) authTestCase {
			c.actions = []Action{ActionCreate, ActionRead, ActionUpdate, ActionDelete}
			c.allow = false
			return c
		}, []authTestCase{
			// Org + me
			{resource: ResourceTemplate.InOrg(defOrg).WithOwner(user.ID)},
			{resource: ResourceTemplate.InOrg(defOrg)},

			{resource: ResourceTemplate.WithOwner(user.ID)},

			{resource: ResourceTemplate.All()},

			// Other org + me
			{resource: ResourceTemplate.InOrg(unuseID).WithOwner(user.ID)},
			{resource: ResourceTemplate.InOrg(unuseID)},

			// Other org + other user
			{resource: ResourceTemplate.InOrg(defOrg).WithOwner("not-me")},

			{resource: ResourceTemplate.WithOwner("not-me")},

			// Other org + other use
			{resource: ResourceTemplate.InOrg(unuseID).WithOwner("not-me")},
			{resource: ResourceTemplate.InOrg(unuseID)},

			{resource: ResourceTemplate.WithOwner("not-me")},
		}),
	)

	// In practice this is a token scope on a regular subject
	user = Subject{
		ID:    "me",
		Scope: must(ExpandScope(ScopeAll)),
		Roles: Roles{
			{
				Name: "ReadOnlyOrgAndUser",
				Site: []Permission{},
				Org: map[string][]Permission{
					defOrg.String(): {{
						Negate:       false,
						ResourceType: "*",
						Action:       ActionRead,
					}},
				},
				User: []Permission{
					{
						Negate:       false,
						ResourceType: "*",
						Action:       ActionRead,
					},
				},
			},
		},
	}

	testAuthorize(t, "ReadOnly", user,
		cases(func(c authTestCase) authTestCase {
			c.actions = []Action{ActionRead}
			return c
		}, []authTestCase{
			// Read
			// Org + me
			{resource: ResourceWorkspace.InOrg(defOrg).WithOwner(user.ID), allow: true},
			{resource: ResourceWorkspace.InOrg(defOrg), allow: true},

			{resource: ResourceWorkspace.WithOwner(user.ID), allow: true},

			{resource: ResourceWorkspace.All(), allow: false},

			// Other org + me
			{resource: ResourceWorkspace.InOrg(unuseID).WithOwner(user.ID), allow: false},
			{resource: ResourceWorkspace.InOrg(unuseID), allow: false},

			// Other org + other user
			{resource: ResourceWorkspace.InOrg(defOrg).WithOwner("not-me"), allow: true},

			{resource: ResourceWorkspace.WithOwner("not-me"), allow: false},

			// Other org + other use
			{resource: ResourceWorkspace.InOrg(unuseID).WithOwner("not-me"), allow: false},
			{resource: ResourceWorkspace.InOrg(unuseID), allow: false},

			{resource: ResourceWorkspace.WithOwner("not-me"), allow: false},
		}),

		// Pass non-read actions
		cases(func(c authTestCase) authTestCase {
			c.actions = []Action{ActionCreate, ActionUpdate, ActionDelete}
			c.allow = false
			return c
		}, []authTestCase{
			// Read
			// Org + me
			{resource: ResourceWorkspace.InOrg(defOrg).WithOwner(user.ID)},
			{resource: ResourceWorkspace.InOrg(defOrg)},

			{resource: ResourceWorkspace.WithOwner(user.ID)},

			{resource: ResourceWorkspace.All()},

			// Other org + me
			{resource: ResourceWorkspace.InOrg(unuseID).WithOwner(user.ID)},
			{resource: ResourceWorkspace.InOrg(unuseID)},

			// Other org + other user
			{resource: ResourceWorkspace.InOrg(defOrg).WithOwner("not-me")},

			{resource: ResourceWorkspace.WithOwner("not-me")},

			// Other org + other use
			{resource: ResourceWorkspace.InOrg(unuseID).WithOwner("not-me")},
			{resource: ResourceWorkspace.InOrg(unuseID)},

			{resource: ResourceWorkspace.WithOwner("not-me")},
		}))
}

// TestAuthorizeLevels ensures level overrides are acting appropriately
func TestAuthorizeLevels(t *testing.T) {
	t.Parallel()
	defOrg := uuid.New()
	unusedID := uuid.New()

	user := Subject{
		ID:    "me",
		Scope: must(ExpandScope(ScopeAll)),
		Roles: Roles{
			must(RoleByName(RoleOwner())),
			{
				Name: "org-deny:" + defOrg.String(),
				Org: map[string][]Permission{
					defOrg.String(): {
						{
							Negate:       true,
							ResourceType: "*",
							Action:       "*",
						},
					},
				},
			},
			{
				Name: "user-deny-all",
				// List out deny permissions explicitly
				User: []Permission{
					{
						Negate:       true,
						ResourceType: WildcardSymbol,
						Action:       WildcardSymbol,
					},
				},
			},
		},
	}

	testAuthorize(t, "AdminAlwaysAllow", user,
		cases(func(c authTestCase) authTestCase {
			c.actions = AllActions()
			c.allow = true
			return c
		}, []authTestCase{
			// Org + me
			{resource: ResourceWorkspace.InOrg(defOrg).WithOwner(user.ID)},
			{resource: ResourceWorkspace.InOrg(defOrg)},

			{resource: ResourceWorkspace.WithOwner(user.ID)},

			{resource: ResourceWorkspace.All()},

			// Other org + me
			{resource: ResourceWorkspace.InOrg(unusedID).WithOwner(user.ID)},
			{resource: ResourceWorkspace.InOrg(unusedID)},

			// Other org + other user
			{resource: ResourceWorkspace.InOrg(defOrg).WithOwner("not-me")},

			{resource: ResourceWorkspace.WithOwner("not-me")},

			// Other org + other use
			{resource: ResourceWorkspace.InOrg(unusedID).WithOwner("not-me")},
			{resource: ResourceWorkspace.InOrg(unusedID)},

			{resource: ResourceWorkspace.WithOwner("not-me")},
		}))

	user = Subject{
		ID:    "me",
		Scope: must(ExpandScope(ScopeAll)),
		Roles: Roles{
			{
				Name: "site-noise",
				Site: []Permission{
					{
						Negate:       true,
						ResourceType: "random",
						Action:       WildcardSymbol,
					},
				},
			},
			must(RoleByName(RoleOrgAdmin(defOrg))),
			{
				Name: "user-deny-all",
				// List out deny permissions explicitly
				User: []Permission{
					{
						Negate:       true,
						ResourceType: WildcardSymbol,
						Action:       WildcardSymbol,
					},
				},
			},
		},
	}

	testAuthorize(t, "OrgAllowAll", user,
		cases(func(c authTestCase) authTestCase {
			c.actions = AllActions()
			return c
		}, []authTestCase{
			// Org + me
			{resource: ResourceWorkspace.InOrg(defOrg).WithOwner(user.ID), allow: true},
			{resource: ResourceWorkspace.InOrg(defOrg), allow: true},

			{resource: ResourceWorkspace.WithOwner(user.ID), allow: false},

			{resource: ResourceWorkspace.All(), allow: false},

			// Other org + me
			{resource: ResourceWorkspace.InOrg(unusedID).WithOwner(user.ID), allow: false},
			{resource: ResourceWorkspace.InOrg(unusedID), allow: false},

			// Other org + other user
			{resource: ResourceWorkspace.InOrg(defOrg).WithOwner("not-me"), allow: true},

			{resource: ResourceWorkspace.WithOwner("not-me"), allow: false},

			// Other org + other use
			{resource: ResourceWorkspace.InOrg(unusedID).WithOwner("not-me"), allow: false},
			{resource: ResourceWorkspace.InOrg(unusedID), allow: false},

			{resource: ResourceWorkspace.WithOwner("not-me"), allow: false},
		}))
}

func TestAuthorizeScope(t *testing.T) {
	t.Parallel()

	defOrg := uuid.New()
	unusedID := uuid.New()
	user := Subject{
		ID:    "me",
		Roles: Roles{must(RoleByName(RoleOwner()))},
		Scope: must(ExpandScope(ScopeApplicationConnect)),
	}

	testAuthorize(t, "Admin_ScopeApplicationConnect", user,
		cases(func(c authTestCase) authTestCase {
			c.actions = []Action{ActionRead, ActionUpdate, ActionDelete}
			return c
		}, []authTestCase{
			{resource: ResourceWorkspace.InOrg(defOrg).WithOwner(user.ID), allow: false},
			{resource: ResourceWorkspace.InOrg(defOrg), allow: false},
			{resource: ResourceWorkspace.WithOwner(user.ID), allow: false},
			{resource: ResourceWorkspace.All(), allow: false},
			{resource: ResourceWorkspace.InOrg(unusedID).WithOwner(user.ID), allow: false},
			{resource: ResourceWorkspace.InOrg(unusedID), allow: false},
			{resource: ResourceWorkspace.InOrg(defOrg).WithOwner("not-me"), allow: false},
			{resource: ResourceWorkspace.WithOwner("not-me"), allow: false},
			{resource: ResourceWorkspace.InOrg(unusedID).WithOwner("not-me"), allow: false},
			{resource: ResourceWorkspace.InOrg(unusedID), allow: false},
			{resource: ResourceWorkspace.WithOwner("not-me"), allow: false},
		}),
		// Allowed by scope:
		[]authTestCase{
			{resource: ResourceWorkspaceApplicationConnect.InOrg(defOrg).WithOwner("not-me"), actions: []Action{ActionCreate}, allow: true},
			{resource: ResourceWorkspaceApplicationConnect.InOrg(defOrg).WithOwner(user.ID), actions: []Action{ActionCreate}, allow: true},
			{resource: ResourceWorkspaceApplicationConnect.InOrg(unusedID).WithOwner("not-me"), actions: []Action{ActionCreate}, allow: true},
		},
	)

	user = Subject{
		ID: "me",
		Roles: Roles{
			must(RoleByName(RoleMember())),
			must(RoleByName(RoleOrgMember(defOrg))),
		},
		Scope: must(ExpandScope(ScopeApplicationConnect)),
	}

	testAuthorize(t, "User_ScopeApplicationConnect", user,
		cases(func(c authTestCase) authTestCase {
			c.actions = []Action{ActionRead, ActionUpdate, ActionDelete}
			c.allow = false
			return c
		}, []authTestCase{
			{resource: ResourceWorkspace.InOrg(defOrg).WithOwner(user.ID)},
			{resource: ResourceWorkspace.InOrg(defOrg)},
			{resource: ResourceWorkspace.WithOwner(user.ID)},
			{resource: ResourceWorkspace.All()},
			{resource: ResourceWorkspace.InOrg(unusedID).WithOwner(user.ID)},
			{resource: ResourceWorkspace.InOrg(unusedID)},
			{resource: ResourceWorkspace.InOrg(defOrg).WithOwner("not-me")},
			{resource: ResourceWorkspace.WithOwner("not-me")},
			{resource: ResourceWorkspace.InOrg(unusedID).WithOwner("not-me")},
			{resource: ResourceWorkspace.InOrg(unusedID)},
			{resource: ResourceWorkspace.WithOwner("not-me")},
		}),
		// Allowed by scope:
		[]authTestCase{
			{resource: ResourceWorkspaceApplicationConnect.InOrg(defOrg).WithOwner(user.ID), actions: []Action{ActionCreate}, allow: true},
			{resource: ResourceWorkspaceApplicationConnect.InOrg(defOrg).WithOwner("not-me"), actions: []Action{ActionCreate}, allow: false},
			{resource: ResourceWorkspaceApplicationConnect.InOrg(unusedID).WithOwner("not-me"), actions: []Action{ActionCreate}, allow: false},
		},
	)

	workspaceID := uuid.New()
	user = Subject{
		ID: "me",
		Roles: Roles{
			must(RoleByName(RoleMember())),
			must(RoleByName(RoleOrgMember(defOrg))),
		},
		Scope: Scope{
			Role: Role{
				Name:        "workspace_agent",
				DisplayName: "Workspace Agent",
				Site: Permissions(map[string][]Action{
					// Only read access for workspaces.
					ResourceWorkspace.Type: {ActionRead},
				}),
				Org:  map[string][]Permission{},
				User: []Permission{},
			},
			AllowIDList: []string{workspaceID.String()},
		},
	}

	testAuthorize(t, "User_WorkspaceAgent", user,
		// Test cases without ID
		cases(func(c authTestCase) authTestCase {
			c.actions = []Action{ActionCreate, ActionUpdate, ActionDelete}
			c.allow = false
			return c
		}, []authTestCase{
			{resource: ResourceWorkspace.InOrg(defOrg).WithOwner(user.ID)},
			{resource: ResourceWorkspace.InOrg(defOrg)},
			{resource: ResourceWorkspace.WithOwner(user.ID)},
			{resource: ResourceWorkspace.All()},
			{resource: ResourceWorkspace.InOrg(unusedID).WithOwner(user.ID)},
			{resource: ResourceWorkspace.InOrg(unusedID)},
			{resource: ResourceWorkspace.InOrg(defOrg).WithOwner("not-me")},
			{resource: ResourceWorkspace.WithOwner("not-me")},
			{resource: ResourceWorkspace.InOrg(unusedID).WithOwner("not-me")},
			{resource: ResourceWorkspace.InOrg(unusedID)},
			{resource: ResourceWorkspace.WithOwner("not-me")},
		}),

		// Test all cases with the workspace id
		cases(func(c authTestCase) authTestCase {
			c.actions = []Action{ActionCreate, ActionUpdate, ActionDelete}
			c.allow = false
			c.resource.WithID(workspaceID)
			return c
		}, []authTestCase{
			{resource: ResourceWorkspace.InOrg(defOrg).WithOwner(user.ID)},
			{resource: ResourceWorkspace.InOrg(defOrg)},
			{resource: ResourceWorkspace.WithOwner(user.ID)},
			{resource: ResourceWorkspace.All()},
			{resource: ResourceWorkspace.InOrg(unusedID).WithOwner(user.ID)},
			{resource: ResourceWorkspace.InOrg(unusedID)},
			{resource: ResourceWorkspace.InOrg(defOrg).WithOwner("not-me")},
			{resource: ResourceWorkspace.WithOwner("not-me")},
			{resource: ResourceWorkspace.InOrg(unusedID).WithOwner("not-me")},
			{resource: ResourceWorkspace.InOrg(unusedID)},
			{resource: ResourceWorkspace.WithOwner("not-me")},
		}),
		// Test cases with random ids. These should always fail from the scope.
		cases(func(c authTestCase) authTestCase {
			c.actions = []Action{ActionRead, ActionCreate, ActionUpdate, ActionDelete}
			c.allow = false
			c.resource.WithID(uuid.New())
			return c
		}, []authTestCase{
			{resource: ResourceWorkspace.InOrg(defOrg).WithOwner(user.ID)},
			{resource: ResourceWorkspace.InOrg(defOrg)},
			{resource: ResourceWorkspace.WithOwner(user.ID)},
			{resource: ResourceWorkspace.All()},
			{resource: ResourceWorkspace.InOrg(unusedID).WithOwner(user.ID)},
			{resource: ResourceWorkspace.InOrg(unusedID)},
			{resource: ResourceWorkspace.InOrg(defOrg).WithOwner("not-me")},
			{resource: ResourceWorkspace.WithOwner("not-me")},
			{resource: ResourceWorkspace.InOrg(unusedID).WithOwner("not-me")},
			{resource: ResourceWorkspace.InOrg(unusedID)},
			{resource: ResourceWorkspace.WithOwner("not-me")},
		}),
		// Allowed by scope:
		[]authTestCase{
			{resource: ResourceWorkspace.WithID(workspaceID).InOrg(defOrg).WithOwner(user.ID), actions: []Action{ActionRead}, allow: true},
			// The scope will return true, but the user perms return false for resources not owned by the user.
			{resource: ResourceWorkspace.WithID(workspaceID).InOrg(defOrg).WithOwner("not-me"), actions: []Action{ActionRead}, allow: false},
			{resource: ResourceWorkspace.WithID(workspaceID).InOrg(unusedID).WithOwner("not-me"), actions: []Action{ActionRead}, allow: false},
		},
	)

	// This scope can only create workspaces
	user = Subject{
		ID: "me",
		Roles: Roles{
			must(RoleByName(RoleMember())),
			must(RoleByName(RoleOrgMember(defOrg))),
		},
		Scope: Scope{
			Role: Role{
				Name:        "create_workspace",
				DisplayName: "Create Workspace",
				Site: Permissions(map[string][]Action{
					// Only read access for workspaces.
					ResourceWorkspace.Type: {ActionCreate},
				}),
				Org:  map[string][]Permission{},
				User: []Permission{},
			},
			// Empty string allow_list is allowed for actions like 'create'
			AllowIDList: []string{""},
		},
	}

	testAuthorize(t, "CreatWorkspaceScope", user,
		// All these cases will fail because a resource ID is set.
		cases(func(c authTestCase) authTestCase {
			c.actions = []Action{ActionCreate, ActionRead, ActionUpdate, ActionDelete}
			c.allow = false
			c.resource.ID = uuid.NewString()
			return c
		}, []authTestCase{
			{resource: ResourceWorkspace.InOrg(defOrg).WithOwner(user.ID)},
			{resource: ResourceWorkspace.InOrg(defOrg)},
			{resource: ResourceWorkspace.WithOwner(user.ID)},
			{resource: ResourceWorkspace.All()},
			{resource: ResourceWorkspace.InOrg(unusedID).WithOwner(user.ID)},
			{resource: ResourceWorkspace.InOrg(unusedID)},
			{resource: ResourceWorkspace.InOrg(defOrg).WithOwner("not-me")},
			{resource: ResourceWorkspace.WithOwner("not-me")},
			{resource: ResourceWorkspace.InOrg(unusedID).WithOwner("not-me")},
			{resource: ResourceWorkspace.InOrg(unusedID)},
			{resource: ResourceWorkspace.WithOwner("not-me")},
		}),

		// Test create allowed by scope:
		[]authTestCase{
			{resource: ResourceWorkspace.InOrg(defOrg).WithOwner(user.ID), actions: []Action{ActionCreate}, allow: true},
			// The scope will return true, but the user perms return false for resources not owned by the user.
			{resource: ResourceWorkspace.InOrg(defOrg).WithOwner("not-me"), actions: []Action{ActionCreate}, allow: false},
			{resource: ResourceWorkspace.InOrg(unusedID).WithOwner("not-me"), actions: []Action{ActionCreate}, allow: false},
		},
	)
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
	resource Object
	actions  []Action
	allow    bool
}

func testAuthorize(t *testing.T, name string, subject Subject, sets ...[]authTestCase) {
	t.Helper()
	authorizer := NewAuthorizer(prometheus.NewRegistry())
	for _, cases := range sets {
		for i, c := range cases {
			c := c
			caseName := fmt.Sprintf("%s/%d", name, i)
			t.Run(caseName, func(t *testing.T) {
				t.Parallel()
				for _, a := range c.actions {
					ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
					t.Cleanup(cancel)

					authError := authorizer.Authorize(ctx, subject, a, c.resource)

					d, _ := json.Marshal(map[string]interface{}{
						// This is not perfect marshal, but it is good enough for debugging this test.
						"subject": authSubject{
							ID:     subject.ID,
							Roles:  must(subject.Roles.Expand()),
							Groups: subject.Groups,
							Scope:  must(subject.Scope.Expand()),
						},
						"object": c.resource,
						"action": a,
					})

					// Logging only
					t.Logf("input: %s", string(d))
					if authError != nil {
						var uerr *UnauthorizedError
						xerrors.As(authError, &uerr)
						t.Logf("internal error: %+v", uerr.Internal().Error())
						t.Logf("output: %+v", uerr.Output())
					}

					if c.allow {
						assert.NoError(t, authError, "expected no error for testcase action %s", a)
					} else {
						assert.Error(t, authError, "expected unauthorized")
					}

					prepared, err := authorizer.Prepare(ctx, subject, a, c.resource.Type)
					require.NoError(t, err, "make prepared authorizer")

					// For unit testing logging and assertions, we want the PartialAuthorizer
					// struct.
					partialAuthz, ok := prepared.(*PartialAuthorizer)
					require.True(t, ok, "prepared authorizer is partial")

					// Ensure the partial can compile to a SQL clause.
					// This does not guarantee that the clause is valid SQL.
					_, err = Compile(ConfigWithACL(), partialAuthz)
					require.NoError(t, err, "compile prepared authorizer")

					// Also check the rego policy can form a valid partial query result.
					// This ensures we can convert the queries into SQL WHERE clauses in the future.
					// If this function returns 'Support' sections, then we cannot convert the query into SQL.
					for _, q := range partialAuthz.partialQueries.Queries {
						t.Logf("query: %+v", q.String())
					}
					for _, s := range partialAuthz.partialQueries.Support {
						t.Logf("support: %+v", s.String())
					}

					require.Equal(t, 0, len(partialAuthz.partialQueries.Support), "expected 0 support rules in scope authorizer")

					partialErr := partialAuthz.Authorize(ctx, c.resource)
					if authError != nil {
						assert.Error(t, partialErr, "partial allowed invalid request  (false positive)")
					} else {
						assert.NoError(t, partialErr, "partial error blocked valid request (false negative)")
					}
				}
			})
		}
	}
}

func must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}

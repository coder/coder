package rbac

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/rbac/regosql"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/testutil"
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

// objectBomb is a wrapper around an Objecter that calls a function when
// RBACObject is called.
type objectBomb struct {
	Objecter
	bomb func()
}

func (o *objectBomb) RBACObject() Object {
	o.bomb()
	return o.Objecter.RBACObject()
}

func TestFilterError(t *testing.T) {
	t.Parallel()
	_ = objectBomb{}

	t.Run("DifferentResourceTypes", func(t *testing.T) {
		t.Parallel()

		auth := NewAuthorizer(prometheus.NewRegistry())
		subject := Subject{
			ID:     uuid.NewString(),
			Roles:  RoleIdentifiers{},
			Groups: []string{},
			Scope:  ScopeAll,
		}

		_, err := Filter(context.Background(), auth, subject, policy.ActionRead, []Object{ResourceUser, ResourceWorkspace})
		require.ErrorContains(t, err, "object types must be uniform")
	})

	t.Run("CancelledContext", func(t *testing.T) {
		t.Parallel()

		auth := &MockAuthorizer{
			AuthorizeFunc: func(ctx context.Context, subject Subject, action policy.Action, object Object) error {
				// Authorize func always returns nil, unless the context is canceled.
				return ctx.Err()
			},
		}

		subject := Subject{
			ID: uuid.NewString(),
			Roles: RoleIdentifiers{
				RoleOwner(),
			},
			Groups: []string{},
			Scope:  ScopeAll,
		}

		t.Run("SmallSet", func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			objects := []Objecter{
				ResourceUser,
				ResourceUser,
				&objectBomb{
					Objecter: ResourceUser,
					bomb:     cancel,
				},
				ResourceUser,
			}

			_, err := Filter(ctx, auth, subject, policy.ActionRead, objects)
			require.ErrorIs(t, err, context.Canceled)
		})

		// Triggers Prepared Authorize
		t.Run("LargeSet", func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			objects := make([]Objecter, 100)
			for i := 0; i < 100; i++ {
				objects[i] = ResourceUser
			}
			objects[20] = &objectBomb{
				Objecter: ResourceUser,
				bomb:     cancel,
			}

			_, err := Filter(ctx, auth, subject, policy.ActionRead, objects)
			require.ErrorIs(t, err, context.Canceled)
		})
	})
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
		Action     policy.Action
		ObjectType string
	}{
		{
			Name: "NoRoles",
			Actor: Subject{
				ID:    userIDs[0].String(),
				Roles: RoleIdentifiers{},
			},
			ObjectType: ResourceWorkspace.Type,
			Action:     policy.ActionRead,
		},
		{
			Name: "Admin",
			Actor: Subject{
				ID:    userIDs[0].String(),
				Roles: RoleIdentifiers{RoleAuditor(), RoleOwner(), RoleMember()},
			},
			ObjectType: ResourceWorkspace.Type,
			Action:     policy.ActionRead,
		},
		{
			Name: "OrgAdmin",
			Actor: Subject{
				ID:    userIDs[0].String(),
				Roles: RoleIdentifiers{ScopedRoleOrgAdmin(orgIDs[0]), RoleMember()},
			},
			ObjectType: ResourceWorkspace.Type,
			Action:     policy.ActionRead,
		},
		{
			Name: "OrgMember",
			Actor: Subject{
				ID:    userIDs[0].String(),
				Roles: RoleIdentifiers{RoleMember()},
			},
			ObjectType: ResourceWorkspace.Type,
			Action:     policy.ActionRead,
		},
		{
			Name: "ManyRoles",
			Actor: Subject{
				ID: userIDs[0].String(),
				Roles: RoleIdentifiers{
					ScopedRoleOrgAdmin(orgIDs[0]),
					ScopedRoleOrgAdmin(orgIDs[1]),
					ScopedRoleOrgAdmin(orgIDs[2]),
					RoleMember(),
				},
			},
			ObjectType: ResourceWorkspace.Type,
			Action:     policy.ActionRead,
		},
		{
			Name: "SiteMember",
			Actor: Subject{
				ID:    userIDs[0].String(),
				Roles: RoleIdentifiers{RoleMember()},
			},
			ObjectType: ResourceUser.Type,
			Action:     policy.ActionRead,
		},
		{
			Name: "ReadOrgs",
			Actor: Subject{
				ID: userIDs[0].String(),
				Roles: RoleIdentifiers{
					RoleMember(),
				},
			},
			ObjectType: ResourceOrganization.Type,
			Action:     policy.ActionRead,
		},
		{
			Name: "ScopeApplicationConnect",
			Actor: Subject{
				ID:    userIDs[0].String(),
				Roles: RoleIdentifiers{RoleAuditor(), RoleOwner(), RoleMember()},
			},
			ObjectType: ResourceWorkspace.Type,
			Action:     policy.ActionRead,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			actor := tc.Actor

			localObjects := make([]fakeObject, len(objects))
			copy(localObjects, objects)

			auth := NewAuthorizer(prometheus.NewRegistry())

			if actor.Scope == nil {
				// Default to ScopeAll
				actor.Scope = ScopeAll
			}

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			// Run auth 1 by 1
			var allowedCount int
			for i, obj := range localObjects {
				obj.Type = tc.ObjectType
				err := auth.Authorize(ctx, actor, policy.ActionRead, obj.RBACObject())
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
	unusedID := uuid.New()
	allUsersGroup := "Everyone"

	// orphanedUser has no organization
	orphanedUser := Subject{
		ID:     "me",
		Scope:  must(ExpandScope(ScopeAll)),
		Groups: []string{},
		Roles: Roles{
			must(RoleByName(RoleMember())),
		},
	}
	testAuthorize(t, "OrphanedUser", orphanedUser, []authTestCase{
		{resource: ResourceWorkspace.InOrg(defOrg).WithOwner(orphanedUser.ID), actions: ResourceWorkspace.AvailableActions(), allow: false},

		// Orphaned user cannot create workspaces in any organization
		{resource: ResourceWorkspace.AnyOrganization().WithOwner(orphanedUser.ID), actions: []policy.Action{policy.ActionCreate}, allow: false},
	})

	user := Subject{
		ID:     "me",
		Scope:  must(ExpandScope(ScopeAll)),
		Groups: []string{allUsersGroup},
		Roles: Roles{
			must(RoleByName(RoleMember())),
			orgMemberRole(defOrg),
		},
	}

	testAuthorize(t, "UserACLList", user, []authTestCase{
		{
			resource: ResourceWorkspace.WithOwner(unusedID.String()).InOrg(unusedID).WithACLUserList(map[string][]policy.Action{
				user.ID: ResourceWorkspace.AvailableActions(),
			}),
			actions: ResourceWorkspace.AvailableActions(),
			allow:   true,
		},
		{
			resource: ResourceWorkspace.WithOwner(unusedID.String()).InOrg(unusedID).WithACLUserList(map[string][]policy.Action{
				user.ID: {policy.WildcardSymbol},
			}),
			actions: ResourceWorkspace.AvailableActions(),
			allow:   true,
		},
		{
			resource: ResourceWorkspace.WithOwner(unusedID.String()).InOrg(unusedID).WithACLUserList(map[string][]policy.Action{
				user.ID: {policy.ActionRead, policy.ActionUpdate},
			}),
			actions: []policy.Action{policy.ActionCreate, policy.ActionDelete},
			allow:   false,
		},
		{
			// By default users cannot update templates
			resource: ResourceTemplate.InOrg(defOrg).WithACLUserList(map[string][]policy.Action{
				user.ID: {policy.ActionUpdate},
			}),
			actions: []policy.Action{policy.ActionUpdate},
			allow:   true,
		},
	})

	testAuthorize(t, "GroupACLList", user, []authTestCase{
		{
			resource: ResourceWorkspace.WithOwner(unusedID.String()).InOrg(defOrg).WithGroupACL(map[string][]policy.Action{
				allUsersGroup: ResourceWorkspace.AvailableActions(),
			}),
			actions: ResourceWorkspace.AvailableActions(),
			allow:   true,
		},
		{
			resource: ResourceWorkspace.WithOwner(unusedID.String()).InOrg(defOrg).WithGroupACL(map[string][]policy.Action{
				allUsersGroup: {policy.WildcardSymbol},
			}),
			actions: ResourceWorkspace.AvailableActions(),
			allow:   true,
		},
		{
			resource: ResourceWorkspace.WithOwner(unusedID.String()).InOrg(defOrg).WithGroupACL(map[string][]policy.Action{
				allUsersGroup: {policy.ActionRead, policy.ActionUpdate},
			}),
			actions: []policy.Action{policy.ActionCreate, policy.ActionDelete},
			allow:   false,
		},
		{
			// By default users cannot update templates
			resource: ResourceTemplate.InOrg(defOrg).WithGroupACL(map[string][]policy.Action{
				allUsersGroup: {policy.ActionUpdate},
			}),
			actions: []policy.Action{policy.ActionUpdate},
			allow:   true,
		},
	})

	testAuthorize(t, "Member", user, []authTestCase{
		// Org + me
		{resource: ResourceWorkspace.InOrg(defOrg).WithOwner(user.ID), actions: ResourceWorkspace.AvailableActions(), allow: true},
		{resource: ResourceWorkspace.InOrg(defOrg), actions: ResourceWorkspace.AvailableActions(), allow: false},

		// AnyOrganization using a user scoped permission
		{resource: ResourceWorkspace.AnyOrganization().WithOwner(user.ID), actions: ResourceWorkspace.AvailableActions(), allow: true},
		{resource: ResourceTemplate.AnyOrganization(), actions: []policy.Action{policy.ActionCreate}, allow: false},

		// No org + me
		{resource: ResourceWorkspace.WithOwner(user.ID), actions: ResourceWorkspace.AvailableActions(), allow: false},

		{resource: ResourceWorkspace.All(), actions: ResourceWorkspace.AvailableActions(), allow: false},

		// Other org + me
		{resource: ResourceWorkspace.InOrg(unusedID).WithOwner(user.ID), actions: ResourceWorkspace.AvailableActions(), allow: false},
		{resource: ResourceWorkspace.InOrg(unusedID), actions: ResourceWorkspace.AvailableActions(), allow: false},

		// Other org + other user
		{resource: ResourceWorkspace.InOrg(defOrg).WithOwner("not-me"), actions: ResourceWorkspace.AvailableActions(), allow: false},

		{resource: ResourceWorkspace.WithOwner("not-me"), actions: ResourceWorkspace.AvailableActions(), allow: false},

		// Other org + other us
		{resource: ResourceWorkspace.InOrg(unusedID).WithOwner("not-me"), actions: ResourceWorkspace.AvailableActions(), allow: false},
		{resource: ResourceWorkspace.InOrg(unusedID), actions: ResourceWorkspace.AvailableActions(), allow: false},

		{resource: ResourceWorkspace.WithOwner("not-me"), actions: ResourceWorkspace.AvailableActions(), allow: false},
	})

	user = Subject{
		ID:    "me",
		Scope: must(ExpandScope(ScopeAll)),
		Roles: Roles{{
			Identifier: RoleIdentifier{Name: "deny-all"},
			// List out deny permissions explicitly
			Site: []Permission{
				{
					Negate:       true,
					ResourceType: policy.WildcardSymbol,
					Action:       policy.WildcardSymbol,
				},
			},
		}},
	}

	testAuthorize(t, "DeletedMember", user, []authTestCase{
		// Org + me
		{resource: ResourceWorkspace.InOrg(defOrg).WithOwner(user.ID), actions: ResourceWorkspace.AvailableActions(), allow: false},
		{resource: ResourceWorkspace.InOrg(defOrg), actions: ResourceWorkspace.AvailableActions(), allow: false},

		{resource: ResourceWorkspace.WithOwner(user.ID), actions: ResourceWorkspace.AvailableActions(), allow: false},

		{resource: ResourceWorkspace.All(), actions: ResourceWorkspace.AvailableActions(), allow: false},

		// Other org + me
		{resource: ResourceWorkspace.InOrg(unusedID).WithOwner(user.ID), actions: ResourceWorkspace.AvailableActions(), allow: false},
		{resource: ResourceWorkspace.InOrg(unusedID), actions: ResourceWorkspace.AvailableActions(), allow: false},

		// Other org + other user
		{resource: ResourceWorkspace.InOrg(defOrg).WithOwner("not-me"), actions: ResourceWorkspace.AvailableActions(), allow: false},

		{resource: ResourceWorkspace.WithOwner("not-me"), actions: ResourceWorkspace.AvailableActions(), allow: false},

		// Other org + other use
		{resource: ResourceWorkspace.InOrg(unusedID).WithOwner("not-me"), actions: ResourceWorkspace.AvailableActions(), allow: false},
		{resource: ResourceWorkspace.InOrg(unusedID), actions: ResourceWorkspace.AvailableActions(), allow: false},

		{resource: ResourceWorkspace.WithOwner("not-me"), actions: ResourceWorkspace.AvailableActions(), allow: false},
	})

	user = Subject{
		ID:    "me",
		Scope: must(ExpandScope(ScopeAll)),
		Roles: Roles{
			must(RoleByName(ScopedRoleOrgAdmin(defOrg))),
			orgMemberRole(defOrg),
			must(RoleByName(RoleMember())),
		},
	}

	workspaceExceptConnect := slice.Omit(ResourceWorkspace.AvailableActions(), policy.ActionApplicationConnect, policy.ActionSSH)
	workspaceConnect := []policy.Action{policy.ActionApplicationConnect, policy.ActionSSH}
	testAuthorize(t, "OrgAdmin", user, []authTestCase{
		{resource: ResourceTemplate.AnyOrganization(), actions: []policy.Action{policy.ActionCreate}, allow: true},

		// Org + me
		{resource: ResourceWorkspace.InOrg(defOrg).WithOwner(user.ID), actions: ResourceWorkspace.AvailableActions(), allow: true},
		{resource: ResourceWorkspace.InOrg(defOrg), actions: workspaceExceptConnect, allow: true},
		{resource: ResourceWorkspace.InOrg(defOrg), actions: workspaceConnect, allow: false},

		// No org + me
		{resource: ResourceWorkspace.WithOwner(user.ID), actions: ResourceWorkspace.AvailableActions(), allow: false},

		{resource: ResourceWorkspace.All(), actions: ResourceWorkspace.AvailableActions(), allow: false},

		// Other org + me
		{resource: ResourceWorkspace.InOrg(unusedID).WithOwner(user.ID), actions: ResourceWorkspace.AvailableActions(), allow: false},
		{resource: ResourceWorkspace.InOrg(unusedID), actions: ResourceWorkspace.AvailableActions(), allow: false},

		// Other org + other user
		{resource: ResourceWorkspace.InOrg(defOrg).WithOwner("not-me"), actions: workspaceExceptConnect, allow: true},
		{resource: ResourceWorkspace.InOrg(defOrg).WithOwner("not-me"), actions: workspaceConnect, allow: false},

		{resource: ResourceWorkspace.WithOwner("not-me"), actions: ResourceWorkspace.AvailableActions(), allow: false},

		// Other org + other user
		{resource: ResourceWorkspace.InOrg(unusedID).WithOwner("not-me"), actions: ResourceWorkspace.AvailableActions(), allow: false},
		{resource: ResourceWorkspace.InOrg(unusedID), actions: ResourceWorkspace.AvailableActions(), allow: false},

		{resource: ResourceWorkspace.WithOwner("not-me"), actions: ResourceWorkspace.AvailableActions(), allow: false},
	})

	user = Subject{
		ID:    "me",
		Scope: must(ExpandScope(ScopeAll)),
		Roles: Roles{
			must(RoleByName(RoleOwner())),
			must(RoleByName(RoleMember())),
		},
	}

	siteAdminWorkspaceActions := slice.Omit(ResourceWorkspace.AvailableActions(), policy.ActionShare)
	testAuthorize(t, "SiteAdmin", user, []authTestCase{
		// Similar to an orphaned user, but has site level perms
		{resource: ResourceTemplate.AnyOrganization(), actions: []policy.Action{policy.ActionCreate}, allow: true},

		// Org + me
		{resource: ResourceWorkspace.InOrg(defOrg).WithOwner(user.ID), actions: siteAdminWorkspaceActions, allow: true},
		{resource: ResourceWorkspace.InOrg(defOrg), actions: siteAdminWorkspaceActions, allow: true},

		{resource: ResourceWorkspace.WithOwner(user.ID), actions: siteAdminWorkspaceActions, allow: true},

		{resource: ResourceWorkspace.All(), actions: siteAdminWorkspaceActions, allow: true},

		// Other org + me
		{resource: ResourceWorkspace.InOrg(unusedID).WithOwner(user.ID), actions: siteAdminWorkspaceActions, allow: true},
		{resource: ResourceWorkspace.InOrg(unusedID), actions: siteAdminWorkspaceActions, allow: true},

		// Other org + other user
		{resource: ResourceWorkspace.InOrg(defOrg).WithOwner("not-me"), actions: siteAdminWorkspaceActions, allow: true},

		{resource: ResourceWorkspace.WithOwner("not-me"), actions: siteAdminWorkspaceActions, allow: true},

		// Other org + other use
		{resource: ResourceWorkspace.InOrg(unusedID).WithOwner("not-me"), actions: siteAdminWorkspaceActions, allow: true},
		{resource: ResourceWorkspace.InOrg(unusedID), actions: siteAdminWorkspaceActions, allow: true},

		{resource: ResourceWorkspace.WithOwner("not-me"), actions: siteAdminWorkspaceActions, allow: true},
	})

	user = Subject{
		ID:    "me",
		Scope: must(ExpandScope(ScopeApplicationConnect)),
		Roles: Roles{
			orgMemberRole(defOrg),
			must(RoleByName(RoleMember())),
		},
	}

	testAuthorize(t, "ApplicationToken", user,
		// Create (connect) Actions
		cases(func(c authTestCase) authTestCase {
			c.actions = []policy.Action{policy.ActionApplicationConnect}
			return c
		}, []authTestCase{
			// Org + me
			{resource: ResourceWorkspace.InOrg(defOrg).WithOwner(user.ID), allow: true},
			{resource: ResourceWorkspace.InOrg(defOrg), allow: false},

			// No org + me
			{resource: ResourceWorkspace.WithOwner(user.ID), allow: false},

			{resource: ResourceWorkspace.All(), allow: false},

			// Other org + me
			{resource: ResourceWorkspace.InOrg(unusedID).WithOwner(user.ID), allow: false},
			{resource: ResourceWorkspace.InOrg(unusedID), allow: false},

			// Other org + other user
			{resource: ResourceWorkspace.InOrg(defOrg).WithOwner("not-me"), allow: false},

			{resource: ResourceWorkspace.WithOwner("not-me"), allow: false},

			// Other org + other use
			{resource: ResourceWorkspace.InOrg(unusedID).WithOwner("not-me"), allow: false},
			{resource: ResourceWorkspace.InOrg(unusedID), allow: false},

			{resource: ResourceWorkspace.WithOwner("not-me"), allow: false},
		}),
		// No ActionApplicationConnect action
		cases(func(c authTestCase) authTestCase {
			c.actions = []policy.Action{policy.ActionRead, policy.ActionUpdate, policy.ActionDelete}
			c.allow = false
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
		}),
		// Other Objects
		cases(func(c authTestCase) authTestCase {
			c.actions = []policy.Action{policy.ActionCreate, policy.ActionRead, policy.ActionUpdate, policy.ActionDelete}
			c.allow = false
			return c
		}, []authTestCase{
			// Org + me
			{resource: ResourceTemplate.InOrg(defOrg).WithOwner(user.ID)},
			{resource: ResourceTemplate.InOrg(defOrg)},

			{resource: ResourceTemplate.WithOwner(user.ID)},

			{resource: ResourceTemplate.All()},

			// Other org + me
			{resource: ResourceTemplate.InOrg(unusedID).WithOwner(user.ID)},
			{resource: ResourceTemplate.InOrg(unusedID)},

			// Other org + other user
			{resource: ResourceTemplate.InOrg(defOrg).WithOwner("not-me")},

			{resource: ResourceTemplate.WithOwner("not-me")},

			// Other org + other use
			{resource: ResourceTemplate.InOrg(unusedID).WithOwner("not-me")},
			{resource: ResourceTemplate.InOrg(unusedID)},

			{resource: ResourceTemplate.WithOwner("not-me")},
		}),
	)

	// In practice this is a token scope on a regular subject
	user = Subject{
		ID:    "me",
		Scope: must(ExpandScope(ScopeAll)),
		Roles: Roles{
			{
				Identifier: RoleIdentifier{Name: "ReadOnlyOrgAndUser"},
				Site:       []Permission{},
				User: []Permission{
					{
						Negate:       false,
						ResourceType: "*",
						Action:       policy.ActionRead,
					},
				},
				ByOrgID: map[string]OrgPermissions{
					defOrg.String(): {
						Org: []Permission{{
							Negate:       false,
							ResourceType: "*",
							Action:       policy.ActionRead,
						}},
						Member: []Permission{},
					},
				},
			},
		},
	}

	testAuthorize(t, "ReadOnly", user,
		cases(func(c authTestCase) authTestCase {
			c.actions = []policy.Action{policy.ActionRead}
			return c
		}, []authTestCase{
			// Read
			// Org + me
			{resource: ResourceWorkspace.InOrg(defOrg).WithOwner(user.ID), allow: true},
			{resource: ResourceWorkspace.InOrg(defOrg), allow: true},

			{resource: ResourceWorkspace.WithOwner(user.ID), allow: true},

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
		}),

		// Pass non-read actions
		cases(func(c authTestCase) authTestCase {
			c.actions = []policy.Action{policy.ActionCreate, policy.ActionUpdate, policy.ActionDelete}
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
				Identifier: RoleIdentifier{Name: "org-deny:", OrganizationID: defOrg},
				ByOrgID: map[string]OrgPermissions{
					defOrg.String(): {
						Org: []Permission{
							{
								Negate:       true,
								ResourceType: "*",
								Action:       "*",
							},
						},
						Member: []Permission{},
					},
				},
			},
			{
				Identifier: RoleIdentifier{Name: "user-deny-all"},
				// List out deny permissions explicitly
				User: []Permission{
					{
						Negate:       true,
						ResourceType: policy.WildcardSymbol,
						Action:       policy.WildcardSymbol,
					},
				},
			},
		},
	}

	testAuthorize(t, "AdminAlwaysAllow", user,
		cases(func(c authTestCase) authTestCase {
			c.actions = slice.Omit(ResourceWorkspace.AvailableActions(), policy.ActionShare)
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
				Identifier: RoleIdentifier{Name: "site-noise"},
				Site: []Permission{
					{
						Negate:       true,
						ResourceType: "random",
						Action:       policy.WildcardSymbol,
					},
				},
			},
			must(RoleByName(ScopedRoleOrgAdmin(defOrg))),
			{
				Identifier: RoleIdentifier{Name: "user-deny-all"},
				// List out deny permissions explicitly
				User: []Permission{
					{
						Negate:       true,
						ResourceType: policy.WildcardSymbol,
						Action:       policy.WildcardSymbol,
					},
				},
			},
		},
	}

	testAuthorize(t, "OrgAllowAll", user,
		cases(func(c authTestCase) authTestCase {
			// SSH and app connect are not implied here.
			c.actions = slice.Omit(ResourceWorkspace.AvailableActions(), policy.ActionApplicationConnect, policy.ActionSSH)
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
			c.actions = []policy.Action{policy.ActionRead, policy.ActionUpdate, policy.ActionDelete}
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
			{resource: ResourceWorkspace.InOrg(defOrg).WithOwner("not-me"), actions: []policy.Action{policy.ActionApplicationConnect}, allow: true},
			{resource: ResourceWorkspace.InOrg(defOrg).WithOwner(user.ID), actions: []policy.Action{policy.ActionApplicationConnect}, allow: true},
			{resource: ResourceWorkspace.InOrg(unusedID).WithOwner("not-me"), actions: []policy.Action{policy.ActionApplicationConnect}, allow: true},
		},
	)

	user = Subject{
		ID: "me",
		Roles: Roles{
			must(RoleByName(RoleMember())),
			orgMemberRole(defOrg),
		},
		Scope: must(ExpandScope(ScopeApplicationConnect)),
	}

	testAuthorize(t, "User_ScopeApplicationConnect", user,
		cases(func(c authTestCase) authTestCase {
			c.actions = []policy.Action{policy.ActionRead, policy.ActionUpdate, policy.ActionDelete}
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
			{resource: ResourceWorkspace.InOrg(defOrg).WithOwner(user.ID), actions: []policy.Action{policy.ActionApplicationConnect}, allow: true},
			{resource: ResourceWorkspace.InOrg(defOrg).WithOwner("not-me"), actions: []policy.Action{policy.ActionApplicationConnect}, allow: false},
			{resource: ResourceWorkspace.InOrg(unusedID).WithOwner("not-me"), actions: []policy.Action{policy.ActionApplicationConnect}, allow: false},
		},
	)

	workspaceID := uuid.New()
	user = Subject{
		ID: "me",
		Roles: Roles{
			must(RoleByName(RoleMember())),
			orgMemberRole(defOrg),
		},
		Scope: Scope{
			Role: Role{
				Identifier:  RoleIdentifier{Name: "workspace_agent"},
				DisplayName: "Workspace Agent",
				Site: Permissions(map[string][]policy.Action{
					// Only read access for workspaces.
					ResourceWorkspace.Type: {policy.ActionRead},
				}),
				User:    []Permission{},
				ByOrgID: map[string]OrgPermissions{},
			},
			AllowIDList: []AllowListElement{{Type: ResourceWorkspace.Type, ID: workspaceID.String()}},
		},
	}

	testAuthorize(t, "User_WorkspaceAgent", user,
		// Test cases without ID
		cases(func(c authTestCase) authTestCase {
			c.actions = []policy.Action{policy.ActionCreate, policy.ActionUpdate, policy.ActionDelete}
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
			c.actions = []policy.Action{policy.ActionCreate, policy.ActionUpdate, policy.ActionDelete}
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
			c.actions = []policy.Action{policy.ActionRead, policy.ActionCreate, policy.ActionUpdate, policy.ActionDelete}
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
			{resource: ResourceWorkspace.WithID(workspaceID).InOrg(defOrg).WithOwner(user.ID), actions: []policy.Action{policy.ActionRead}, allow: true},
			// The scope will return true, but the user perms return false for resources not owned by the user.
			{resource: ResourceWorkspace.WithID(workspaceID).InOrg(defOrg).WithOwner("not-me"), actions: []policy.Action{policy.ActionRead}, allow: false},
			{resource: ResourceWorkspace.WithID(workspaceID).InOrg(unusedID).WithOwner("not-me"), actions: []policy.Action{policy.ActionRead}, allow: false},
		},
	)

	// This scope can only create workspaces
	user = Subject{
		ID: "me",
		Roles: Roles{
			must(RoleByName(RoleMember())),
			orgMemberRole(defOrg),
		},
		Scope: Scope{
			Role: Role{
				Identifier:  RoleIdentifier{Name: "create_workspace"},
				DisplayName: "Create Workspace",
				Site: Permissions(map[string][]policy.Action{
					// Only read access for workspaces.
					ResourceWorkspace.Type: {policy.ActionCreate},
				}),
				User:    []Permission{},
				ByOrgID: map[string]OrgPermissions{},
			},
			// Empty string allow_list is allowed for actions like 'create'
			AllowIDList: []AllowListElement{{
				Type: ResourceWorkspace.Type, ID: "",
			}},
		},
	}

	testAuthorize(t, "CreatWorkspaceScope", user,
		// All these cases will fail because a resource ID is set.
		cases(func(c authTestCase) authTestCase {
			c.actions = []policy.Action{policy.ActionCreate, policy.ActionRead, policy.ActionUpdate, policy.ActionDelete}
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
			{resource: ResourceWorkspace.InOrg(defOrg).WithOwner(user.ID), actions: []policy.Action{policy.ActionCreate}, allow: true},
			// The scope will return true, but the user perms return false for resources not owned by the user.
			{resource: ResourceWorkspace.InOrg(defOrg).WithOwner("not-me"), actions: []policy.Action{policy.ActionCreate}, allow: false},
			{resource: ResourceWorkspace.InOrg(unusedID).WithOwner("not-me"), actions: []policy.Action{policy.ActionCreate}, allow: false},
		},
	)

	meID := uuid.New()
	user = Subject{
		ID: meID.String(),
		Roles: Roles{
			must(RoleByName(RoleMember())),
			orgMemberRole(defOrg),
		},
		Scope: must(ScopeNoUserData.Expand()),
	}

	// Test 1: Verify that no_user_data scope prevents accessing user data
	testAuthorize(t, "ReadPersonalUser", user,
		cases(func(c authTestCase) authTestCase {
			c.actions = ResourceUser.AvailableActions()
			c.allow = false
			c.resource.ID = meID.String()
			return c
		}, []authTestCase{
			{resource: ResourceUser.WithOwner(meID.String()).InOrg(defOrg).WithID(meID)},
		}),
	)

	// Test 2: Verify token can still perform regular member actions that don't involve user data
	testAuthorize(t, "NoUserData_CanStillUseRegularPermissions", user,
		// Test workspace access - should still work
		cases(func(c authTestCase) authTestCase {
			c.actions = []policy.Action{policy.ActionRead}
			c.allow = true
			return c
		}, []authTestCase{
			// Can still read owned workspaces
			{resource: ResourceWorkspace.InOrg(defOrg).WithOwner(user.ID)},
		}),
		// Test workspace create - should still work
		cases(func(c authTestCase) authTestCase {
			c.actions = []policy.Action{policy.ActionCreate}
			c.allow = true
			return c
		}, []authTestCase{
			// Can still create workspaces
			{resource: ResourceWorkspace.InOrg(defOrg).WithOwner(user.ID)},
		}),
	)

	// Test 3: Verify token cannot perform actions outside of member role
	testAuthorize(t, "NoUserData_CannotExceedMemberRole", user,
		cases(func(c authTestCase) authTestCase {
			c.actions = []policy.Action{policy.ActionRead, policy.ActionUpdate, policy.ActionDelete}
			c.allow = false
			return c
		}, []authTestCase{
			// Cannot access other users' workspaces
			{resource: ResourceWorkspace.InOrg(defOrg).WithOwner("other-user")},
			// Cannot access admin resources
			{resource: ResourceOrganization.WithID(defOrg)},
		}),
	)

	// Test setting a scope on the org and the user level
	// This is a bit of a contrived example that would not exist in practice.
	// It combines a specific organization scope with a user scope to verify
	// that both are applied.
	// The test uses the `Owner` role, so by default the user can do everything.
	user = Subject{
		ID: "me",
		Roles: Roles{
			must(RoleByName(RoleOwner())),
			// TODO: There is a __bug__ in the policy.rego. If the user is not a
			//  member of the organization, the org_scope fails. This happens because
			//  the org_allow_set uses "org_members".
			//  This is odd behavior, as without this membership role, the test for
			//  the workspace fails. Maybe scopes should just assume the user
			//  is a member.
			orgMemberRole(defOrg),
		},
		Scope: Scope{
			Role: Role{
				Identifier: RoleIdentifier{
					Name:           "org-and-user-scope",
					OrganizationID: defOrg,
				},
				DisplayName: "OrgAndUserScope",
				Site:        nil,
				User: Permissions(map[string][]policy.Action{
					ResourceUser.Type: {policy.ActionRead},
				}),
				ByOrgID: map[string]OrgPermissions{
					defOrg.String(): {
						Org: Permissions(map[string][]policy.Action{
							ResourceWorkspace.Type: {policy.ActionRead},
						}),
						Member: []Permission{},
					},
				},
			},
			AllowIDList: []AllowListElement{AllowListAll()},
		},
	}

	testAuthorize(t, "OrgAndUserScope", user,
		// Allowed by scope:
		[]authTestCase{
			{resource: ResourceWorkspace.InOrg(defOrg).WithOwner(user.ID), allow: true, actions: []policy.Action{policy.ActionRead}},
			{resource: ResourceUser.WithOwner(user.ID), allow: true, actions: []policy.Action{policy.ActionRead}},
		},
		// Not allowed by scope:
		[]authTestCase{
			{resource: ResourceWorkspace.InOrg(defOrg).WithOwner(user.ID), allow: false, actions: []policy.Action{policy.ActionCreate}},
			{resource: ResourceUser.WithOwner(user.ID), allow: false, actions: []policy.Action{policy.ActionUpdate}},
		},
	)
}

func TestScopeAllowList(t *testing.T) {
	t.Parallel()

	defOrg := uuid.New()

	// Some IDs to use
	wid := uuid.New()
	gid := uuid.New()

	user := Subject{
		ID: "me",
		Roles: Roles{
			must(RoleByName(RoleOwner())),
		},
		Scope: Scope{
			Role: Role{
				Identifier: RoleIdentifier{
					Name:           "AllowList",
					OrganizationID: defOrg,
				},
				DisplayName: "AllowList",
				// Allow almost everything
				Site: allPermsExcept(ResourceUser),
			},
			AllowIDList: []AllowListElement{
				{Type: ResourceWorkspace.Type, ID: wid.String()},
				{Type: ResourceWorkspace.Type, ID: ""}, // Allow to create
				{Type: ResourceTemplate.Type, ID: policy.WildcardSymbol},
				{Type: ResourceGroup.Type, ID: gid.String()},

				// This scope allows all users, but the permissions do not.
				{Type: ResourceUser.Type, ID: policy.WildcardSymbol},
			},
		},
	}

	testAuthorize(t, "AllowList", user,
		// Allowed:
		cases(func(c authTestCase) authTestCase {
			c.allow = true
			return c
		},
			[]authTestCase{
				{resource: ResourceWorkspace.InOrg(defOrg).WithOwner(user.ID).WithID(wid), actions: []policy.Action{policy.ActionRead}},
				// matching on empty id
				{resource: ResourceWorkspace.InOrg(defOrg).WithOwner(user.ID), actions: []policy.Action{policy.ActionCreate}},

				// Template has wildcard ID, so any uuid is allowed, including the empty
				{resource: ResourceTemplate.InOrg(defOrg).WithID(uuid.New()), actions: AllActions()},
				{resource: ResourceTemplate.InOrg(defOrg).WithID(uuid.New()), actions: AllActions()},
				{resource: ResourceTemplate.InOrg(defOrg), actions: AllActions()},

				// Group
				{resource: ResourceGroup.InOrg(defOrg).WithID(gid), actions: []policy.Action{policy.ActionRead}},
			},
		),

		// Not allowed:
		cases(func(c authTestCase) authTestCase {
			c.allow = false
			return c
		},
			[]authTestCase{
				// Has the scope and allow list, but not the permission
				{resource: ResourceUser.WithOwner(user.ID), actions: []policy.Action{policy.ActionRead}},

				// `wid` matches on the uuid, but not the type
				{resource: ResourceGroup.WithID(wid), actions: []policy.Action{policy.ActionRead}},

				// no empty id for the create action
				{resource: ResourceGroup.InOrg(defOrg), actions: []policy.Action{policy.ActionCreate}},
			},
		),
	)

	// Wildcard type
	user = Subject{
		ID: "me",
		Roles: Roles{
			must(RoleByName(RoleOwner())),
		},
		Scope: Scope{
			Role: Role{
				Identifier: RoleIdentifier{
					Name:           "WildcardType",
					OrganizationID: defOrg,
				},
				DisplayName: "WildcardType",
				// Allow almost everything
				Site: allPermsExcept(ResourceUser),
			},
			AllowIDList: []AllowListElement{
				{Type: policy.WildcardSymbol, ID: wid.String()},
			},
		},
	}

	testAuthorize(t, "WildcardType", user,
		// Allowed:
		cases(func(c authTestCase) authTestCase {
			c.allow = true
			return c
		},
			[]authTestCase{
				// anything with the id is ok
				{resource: ResourceWorkspace.InOrg(defOrg).WithOwner(user.ID).WithID(wid), actions: []policy.Action{policy.ActionRead}},
				{resource: ResourceGroup.InOrg(defOrg).WithID(wid), actions: []policy.Action{policy.ActionRead}},
				{resource: ResourceTemplate.InOrg(defOrg).WithID(wid), actions: []policy.Action{policy.ActionRead}},
			},
		),

		// Not allowed:
		cases(func(c authTestCase) authTestCase {
			c.allow = false
			return c
		},
			[]authTestCase{
				// Anything without the id is not allowed
				{resource: ResourceWorkspace.InOrg(defOrg).WithOwner(user.ID), actions: []policy.Action{policy.ActionCreate}},
				{resource: ResourceWorkspace.InOrg(defOrg).WithOwner(user.ID).WithID(uuid.New()), actions: []policy.Action{policy.ActionRead}},
			},
		),
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
	actions  []policy.Action
	allow    bool
}

func testAuthorize(t *testing.T, name string, subject Subject, sets ...[]authTestCase) {
	t.Helper()
	authorizer := NewAuthorizer(prometheus.NewRegistry())
	for i, cases := range sets {
		for j, c := range cases {
			caseName := fmt.Sprintf("%s/Set%d/Case%d", name, i, j)
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
						if xerrors.As(authError, &uerr) {
							t.Logf("internal error: %+v", uerr.Internal().Error())
							t.Logf("output: %+v", uerr.Output())
						}
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
					// If 'AnyOrgOwner' is true, a partial eval does not make sense.
					// Run the partial eval to ensure no panics, but the actual authz
					// response does not matter.
					if !c.resource.AnyOrgOwner {
						if authError != nil {
							assert.Error(t, partialErr, "partial allowed invalid request  (false positive)")
						} else {
							assert.NoError(t, partialErr, "partial error blocked valid request (false negative)")
						}
					}
				}
			})
		}
	}
}

// orgMemberRole returns an organization-member role for RBAC-only tests.
//
// organization-member is now a DB-backed system role (not a built-in role), so
// RoleByName won't resolve it here. Assume the default behavior: workspace
// sharing enabled.
func orgMemberRole(orgID uuid.UUID) Role {
	workspaceSharingDisabled := false
	orgPerms, memberPerms := OrgMemberPermissions(workspaceSharingDisabled)
	return Role{
		Identifier:  ScopedRoleOrgMember(orgID),
		DisplayName: "",
		Site:        []Permission{},
		User:        []Permission{},
		ByOrgID: map[string]OrgPermissions{
			orgID.String(): {
				Org:    orgPerms,
				Member: memberPerms,
			},
		},
	}
}

func must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}

type MockAuthorizer struct {
	AuthorizeFunc func(context.Context, Subject, policy.Action, Object) error
}

var _ Authorizer = (*MockAuthorizer)(nil)

func (d *MockAuthorizer) Authorize(ctx context.Context, s Subject, a policy.Action, o Object) error {
	return d.AuthorizeFunc(ctx, s, a, o)
}

func (d *MockAuthorizer) Prepare(_ context.Context, subject Subject, action policy.Action, _ string) (PreparedAuthorized, error) {
	return &mockPreparedAuthorizer{
		Original: d,
		Subject:  subject,
		Action:   action,
	}, nil
}

var _ PreparedAuthorized = (*mockPreparedAuthorizer)(nil)

// fakePreparedAuthorizer is the prepared version of a FakeAuthorizer. It will
// return the same error as the original FakeAuthorizer.
type mockPreparedAuthorizer struct {
	sync.RWMutex
	Original *MockAuthorizer
	Subject  Subject
	Action   policy.Action
}

func (f *mockPreparedAuthorizer) Authorize(ctx context.Context, object Object) error {
	return f.Original.Authorize(ctx, f.Subject, f.Action, object)
}

// CompileToSQL returns a compiled version of the authorizer that will work for
// in memory databases. This fake version will not work against a SQL database.
func (*mockPreparedAuthorizer) CompileToSQL(_ context.Context, _ regosql.ConvertConfig) (string, error) {
	return "not a valid sql string", nil
}

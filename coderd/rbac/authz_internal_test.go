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
			Roles:  RoleNames{},
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
			Roles: RoleNames{
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
				Roles: RoleNames{},
			},
			ObjectType: ResourceWorkspace.Type,
			Action:     policy.ActionRead,
		},
		{
			Name: "Admin",
			Actor: Subject{
				ID:    userIDs[0].String(),
				Roles: RoleNames{ScopedRoleOrgMember(orgIDs[0]), "auditor", RoleOwner(), RoleMember()},
			},
			ObjectType: ResourceWorkspace.Type,
			Action:     policy.ActionRead,
		},
		{
			Name: "OrgAdmin",
			Actor: Subject{
				ID:    userIDs[0].String(),
				Roles: RoleNames{ScopedRoleOrgMember(orgIDs[0]), ScopedRoleOrgAdmin(orgIDs[0]), RoleMember()},
			},
			ObjectType: ResourceWorkspace.Type,
			Action:     policy.ActionRead,
		},
		{
			Name: "OrgMember",
			Actor: Subject{
				ID:    userIDs[0].String(),
				Roles: RoleNames{ScopedRoleOrgMember(orgIDs[0]), ScopedRoleOrgMember(orgIDs[1]), RoleMember()},
			},
			ObjectType: ResourceWorkspace.Type,
			Action:     policy.ActionRead,
		},
		{
			Name: "ManyRoles",
			Actor: Subject{
				ID: userIDs[0].String(),
				Roles: RoleNames{
					ScopedRoleOrgMember(orgIDs[0]), ScopedRoleOrgAdmin(orgIDs[0]),
					ScopedRoleOrgMember(orgIDs[1]), ScopedRoleOrgAdmin(orgIDs[1]),
					ScopedRoleOrgMember(orgIDs[2]), ScopedRoleOrgAdmin(orgIDs[2]),
					ScopedRoleOrgMember(orgIDs[4]),
					ScopedRoleOrgMember(orgIDs[5]),
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
				Roles: RoleNames{RoleMember()},
			},
			ObjectType: ResourceUser.Type,
			Action:     policy.ActionRead,
		},
		{
			Name: "ReadOrgs",
			Actor: Subject{
				ID: userIDs[0].String(),
				Roles: RoleNames{
					ScopedRoleOrgMember(orgIDs[0]),
					ScopedRoleOrgMember(orgIDs[1]),
					ScopedRoleOrgMember(orgIDs[2]),
					ScopedRoleOrgMember(orgIDs[3]),
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
				Roles: RoleNames{ScopedRoleOrgMember(orgIDs[0]), "auditor", RoleOwner(), RoleMember()},
			},
			ObjectType: ResourceWorkspace.Type,
			Action:     policy.ActionRead,
		},
	}

	for _, tc := range testCases {
		tc := tc
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
	unuseID := uuid.New()
	allUsersGroup := "Everyone"

	user := Subject{
		ID:     "me",
		Scope:  must(ExpandScope(ScopeAll)),
		Groups: []string{allUsersGroup},
		Roles: Roles{
			must(RoleByName(RoleMember())),
			must(RoleByName(ScopedRoleOrgMember(defOrg))),
		},
	}

	testAuthorize(t, "UserACLList", user, []authTestCase{
		{
			resource: ResourceWorkspace.WithOwner(unuseID.String()).InOrg(unuseID).WithACLUserList(map[string][]policy.Action{
				user.ID: ResourceWorkspace.AvailableActions(),
			}),
			actions: ResourceWorkspace.AvailableActions(),
			allow:   true,
		},
		{
			resource: ResourceWorkspace.WithOwner(unuseID.String()).InOrg(unuseID).WithACLUserList(map[string][]policy.Action{
				user.ID: {policy.WildcardSymbol},
			}),
			actions: ResourceWorkspace.AvailableActions(),
			allow:   true,
		},
		{
			resource: ResourceWorkspace.WithOwner(unuseID.String()).InOrg(unuseID).WithACLUserList(map[string][]policy.Action{
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
			resource: ResourceWorkspace.WithOwner(unuseID.String()).InOrg(defOrg).WithGroupACL(map[string][]policy.Action{
				allUsersGroup: ResourceWorkspace.AvailableActions(),
			}),
			actions: ResourceWorkspace.AvailableActions(),
			allow:   true,
		},
		{
			resource: ResourceWorkspace.WithOwner(unuseID.String()).InOrg(defOrg).WithGroupACL(map[string][]policy.Action{
				allUsersGroup: {policy.WildcardSymbol},
			}),
			actions: ResourceWorkspace.AvailableActions(),
			allow:   true,
		},
		{
			resource: ResourceWorkspace.WithOwner(unuseID.String()).InOrg(defOrg).WithGroupACL(map[string][]policy.Action{
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

		{resource: ResourceWorkspace.WithOwner(user.ID), actions: ResourceWorkspace.AvailableActions(), allow: true},

		{resource: ResourceWorkspace.All(), actions: ResourceWorkspace.AvailableActions(), allow: false},

		// Other org + me
		{resource: ResourceWorkspace.InOrg(unuseID).WithOwner(user.ID), actions: ResourceWorkspace.AvailableActions(), allow: false},
		{resource: ResourceWorkspace.InOrg(unuseID), actions: ResourceWorkspace.AvailableActions(), allow: false},

		// Other org + other user
		{resource: ResourceWorkspace.InOrg(defOrg).WithOwner("not-me"), actions: ResourceWorkspace.AvailableActions(), allow: false},

		{resource: ResourceWorkspace.WithOwner("not-me"), actions: ResourceWorkspace.AvailableActions(), allow: false},

		// Other org + other us
		{resource: ResourceWorkspace.InOrg(unuseID).WithOwner("not-me"), actions: ResourceWorkspace.AvailableActions(), allow: false},
		{resource: ResourceWorkspace.InOrg(unuseID), actions: ResourceWorkspace.AvailableActions(), allow: false},

		{resource: ResourceWorkspace.WithOwner("not-me"), actions: ResourceWorkspace.AvailableActions(), allow: false},
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
		{resource: ResourceWorkspace.InOrg(unuseID).WithOwner(user.ID), actions: ResourceWorkspace.AvailableActions(), allow: false},
		{resource: ResourceWorkspace.InOrg(unuseID), actions: ResourceWorkspace.AvailableActions(), allow: false},

		// Other org + other user
		{resource: ResourceWorkspace.InOrg(defOrg).WithOwner("not-me"), actions: ResourceWorkspace.AvailableActions(), allow: false},

		{resource: ResourceWorkspace.WithOwner("not-me"), actions: ResourceWorkspace.AvailableActions(), allow: false},

		// Other org + other use
		{resource: ResourceWorkspace.InOrg(unuseID).WithOwner("not-me"), actions: ResourceWorkspace.AvailableActions(), allow: false},
		{resource: ResourceWorkspace.InOrg(unuseID), actions: ResourceWorkspace.AvailableActions(), allow: false},

		{resource: ResourceWorkspace.WithOwner("not-me"), actions: ResourceWorkspace.AvailableActions(), allow: false},
	})

	user = Subject{
		ID:    "me",
		Scope: must(ExpandScope(ScopeAll)),
		Roles: Roles{
			must(RoleByName(ScopedRoleOrgAdmin(defOrg))),
			must(RoleByName(RoleMember())),
		},
	}

	workspaceExceptConnect := slice.Omit(ResourceWorkspace.AvailableActions(), policy.ActionApplicationConnect, policy.ActionSSH)
	workspaceConnect := []policy.Action{policy.ActionApplicationConnect, policy.ActionSSH}
	testAuthorize(t, "OrgAdmin", user, []authTestCase{
		// Org + me
		{resource: ResourceWorkspace.InOrg(defOrg).WithOwner(user.ID), actions: ResourceWorkspace.AvailableActions(), allow: true},
		{resource: ResourceWorkspace.InOrg(defOrg), actions: workspaceExceptConnect, allow: true},
		{resource: ResourceWorkspace.InOrg(defOrg), actions: workspaceConnect, allow: false},

		{resource: ResourceWorkspace.WithOwner(user.ID), actions: ResourceWorkspace.AvailableActions(), allow: true},

		{resource: ResourceWorkspace.All(), actions: ResourceWorkspace.AvailableActions(), allow: false},

		// Other org + me
		{resource: ResourceWorkspace.InOrg(unuseID).WithOwner(user.ID), actions: ResourceWorkspace.AvailableActions(), allow: false},
		{resource: ResourceWorkspace.InOrg(unuseID), actions: ResourceWorkspace.AvailableActions(), allow: false},

		// Other org + other user
		{resource: ResourceWorkspace.InOrg(defOrg).WithOwner("not-me"), actions: workspaceExceptConnect, allow: true},
		{resource: ResourceWorkspace.InOrg(defOrg).WithOwner("not-me"), actions: workspaceConnect, allow: false},

		{resource: ResourceWorkspace.WithOwner("not-me"), actions: ResourceWorkspace.AvailableActions(), allow: false},

		// Other org + other use
		{resource: ResourceWorkspace.InOrg(unuseID).WithOwner("not-me"), actions: ResourceWorkspace.AvailableActions(), allow: false},
		{resource: ResourceWorkspace.InOrg(unuseID), actions: ResourceWorkspace.AvailableActions(), allow: false},

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

	testAuthorize(t, "SiteAdmin", user, []authTestCase{
		// Org + me
		{resource: ResourceWorkspace.InOrg(defOrg).WithOwner(user.ID), actions: ResourceWorkspace.AvailableActions(), allow: true},
		{resource: ResourceWorkspace.InOrg(defOrg), actions: ResourceWorkspace.AvailableActions(), allow: true},

		{resource: ResourceWorkspace.WithOwner(user.ID), actions: ResourceWorkspace.AvailableActions(), allow: true},

		{resource: ResourceWorkspace.All(), actions: ResourceWorkspace.AvailableActions(), allow: true},

		// Other org + me
		{resource: ResourceWorkspace.InOrg(unuseID).WithOwner(user.ID), actions: ResourceWorkspace.AvailableActions(), allow: true},
		{resource: ResourceWorkspace.InOrg(unuseID), actions: ResourceWorkspace.AvailableActions(), allow: true},

		// Other org + other user
		{resource: ResourceWorkspace.InOrg(defOrg).WithOwner("not-me"), actions: ResourceWorkspace.AvailableActions(), allow: true},

		{resource: ResourceWorkspace.WithOwner("not-me"), actions: ResourceWorkspace.AvailableActions(), allow: true},

		// Other org + other use
		{resource: ResourceWorkspace.InOrg(unuseID).WithOwner("not-me"), actions: ResourceWorkspace.AvailableActions(), allow: true},
		{resource: ResourceWorkspace.InOrg(unuseID), actions: ResourceWorkspace.AvailableActions(), allow: true},

		{resource: ResourceWorkspace.WithOwner("not-me"), actions: ResourceWorkspace.AvailableActions(), allow: true},
	})

	user = Subject{
		ID:    "me",
		Scope: must(ExpandScope(ScopeApplicationConnect)),
		Roles: Roles{
			must(RoleByName(ScopedRoleOrgMember(defOrg))),
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

			{resource: ResourceWorkspace.WithOwner(user.ID), allow: true},

			{resource: ResourceWorkspace.All(), allow: false},

			// Other org + me
			{resource: ResourceWorkspace.InOrg(unuseID).WithOwner(user.ID), allow: false},
			{resource: ResourceWorkspace.InOrg(unuseID), allow: false},

			// Other org + other user
			{resource: ResourceWorkspace.InOrg(defOrg).WithOwner("not-me"), allow: false},

			{resource: ResourceWorkspace.WithOwner("not-me"), allow: false},

			// Other org + other use
			{resource: ResourceWorkspace.InOrg(unuseID).WithOwner("not-me"), allow: false},
			{resource: ResourceWorkspace.InOrg(unuseID), allow: false},

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
			{resource: ResourceWorkspace.InOrg(unuseID).WithOwner(user.ID)},
			{resource: ResourceWorkspace.InOrg(unuseID)},

			// Other org + other user
			{resource: ResourceWorkspace.InOrg(defOrg).WithOwner("not-me")},

			{resource: ResourceWorkspace.WithOwner("not-me")},

			// Other org + other use
			{resource: ResourceWorkspace.InOrg(unuseID).WithOwner("not-me")},
			{resource: ResourceWorkspace.InOrg(unuseID)},

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
						Action:       policy.ActionRead,
					}},
				},
				User: []Permission{
					{
						Negate:       false,
						ResourceType: "*",
						Action:       policy.ActionRead,
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
						ResourceType: policy.WildcardSymbol,
						Action:       policy.WildcardSymbol,
					},
				},
			},
		},
	}

	testAuthorize(t, "AdminAlwaysAllow", user,
		cases(func(c authTestCase) authTestCase {
			c.actions = ResourceWorkspace.AvailableActions()
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
						Action:       policy.WildcardSymbol,
					},
				},
			},
			must(RoleByName(ScopedRoleOrgAdmin(defOrg))),
			{
				Name: "user-deny-all",
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
			must(RoleByName(ScopedRoleOrgMember(defOrg))),
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
			must(RoleByName(ScopedRoleOrgMember(defOrg))),
		},
		Scope: Scope{
			Role: Role{
				Name:        "workspace_agent",
				DisplayName: "Workspace Agent",
				Site: Permissions(map[string][]policy.Action{
					// Only read access for workspaces.
					ResourceWorkspace.Type: {policy.ActionRead},
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
			must(RoleByName(ScopedRoleOrgMember(defOrg))),
		},
		Scope: Scope{
			Role: Role{
				Name:        "create_workspace",
				DisplayName: "Create Workspace",
				Site: Permissions(map[string][]policy.Action{
					// Only read access for workspaces.
					ResourceWorkspace.Type: {policy.ActionCreate},
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

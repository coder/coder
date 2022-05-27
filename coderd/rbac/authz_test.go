package rbac_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/rbac"
)

// subject is required because rego needs
type subject struct {
	UserID string `json:"id"`
	// For the unit test we want to pass in the roles directly, instead of just
	// by name. This allows us to test custom roles that do not exist in the product,
	// but test edge cases of the implementation.
	Roles []rbac.Role `json:"roles"`
}

func TestFilter(t *testing.T) {
	t.Parallel()

	objectList := make([]rbac.Object, 0)
	workspaceList := make([]rbac.Object, 0)
	fileList := make([]rbac.Object, 0)
	for i := 0; i < 10; i++ {
		idxStr := strconv.Itoa(i)
		workspace := rbac.ResourceWorkspace.WithID(idxStr).WithOwner("me")
		file := rbac.ResourceFile.WithID(idxStr).WithOwner("me")

		workspaceList = append(workspaceList, workspace)
		fileList = append(fileList, file)

		objectList = append(objectList, workspace)
		objectList = append(objectList, file)
	}

	// copyList is to prevent tests from sharing the same slice
	copyList := func(list []rbac.Object) []rbac.Object {
		tmp := make([]rbac.Object, len(list))
		copy(tmp, list)
		return tmp
	}

	testCases := []struct {
		Name     string
		List     []rbac.Object
		Expected []rbac.Object
		Auth     func(o rbac.Object) error
	}{
		{
			Name:     "FilterWorkspaceType",
			List:     copyList(objectList),
			Expected: copyList(workspaceList),
			Auth: func(o rbac.Object) error {
				if o.Type != rbac.ResourceWorkspace.Type {
					return xerrors.New("only workspace")
				}
				return nil
			},
		},
		{
			Name:     "FilterFileType",
			List:     copyList(objectList),
			Expected: copyList(fileList),
			Auth: func(o rbac.Object) error {
				if o.Type != rbac.ResourceFile.Type {
					return xerrors.New("only file")
				}
				return nil
			},
		},
		{
			Name:     "FilterAll",
			List:     copyList(objectList),
			Expected: []rbac.Object{},
			Auth: func(o rbac.Object) error {
				return xerrors.New("always fail")
			},
		},
		{
			Name:     "FilterNone",
			List:     copyList(objectList),
			Expected: copyList(objectList),
			Auth: func(o rbac.Object) error {
				return nil
			},
		},
	}

	for _, c := range testCases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			authorizer := fakeAuthorizer{
				AuthFunc: func(_ context.Context, _ string, _ []string, _ rbac.Action, object rbac.Object) error {
					return c.Auth(object)
				},
			}

			filtered := rbac.Filter(context.Background(), authorizer, "me", []string{}, rbac.ActionRead, c.List)
			require.ElementsMatch(t, c.Expected, filtered, "expect same list")
			require.Equal(t, len(c.Expected), len(filtered), "same length list")
		})
	}
}

// TestAuthorizeDomain test the very basic roles that are commonly used.
func TestAuthorizeDomain(t *testing.T) {
	t.Parallel()
	defOrg := uuid.New()
	unuseID := uuid.New()
	wrkID := "1234"

	user := subject{
		UserID: "me",
		Roles: []rbac.Role{
			must(rbac.RoleByName(rbac.RoleMember())),
			must(rbac.RoleByName(rbac.RoleOrgMember(defOrg))),
		},
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
		{resource: rbac.ResourceWorkspace.InOrg(unuseID).WithOwner(user.UserID).WithID(wrkID), actions: allActions(), allow: false},
		{resource: rbac.ResourceWorkspace.InOrg(unuseID).WithOwner(user.UserID), actions: allActions(), allow: false},
		{resource: rbac.ResourceWorkspace.InOrg(unuseID).WithID(wrkID), actions: allActions(), allow: false},
		{resource: rbac.ResourceWorkspace.InOrg(unuseID), actions: allActions(), allow: false},

		// Other org + other user + id
		{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithOwner("not-me").WithID(wrkID), actions: allActions(), allow: false},
		{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithOwner("not-me"), actions: allActions(), allow: false},

		{resource: rbac.ResourceWorkspace.WithOwner("not-me").WithID(wrkID), actions: allActions(), allow: false},
		{resource: rbac.ResourceWorkspace.WithOwner("not-me"), actions: allActions(), allow: false},

		// Other org + other use + other id
		{resource: rbac.ResourceWorkspace.InOrg(unuseID).WithOwner("not-me").WithID("not-id"), actions: allActions(), allow: false},
		{resource: rbac.ResourceWorkspace.InOrg(unuseID).WithOwner("not-me"), actions: allActions(), allow: false},
		{resource: rbac.ResourceWorkspace.InOrg(unuseID).WithID("not-id"), actions: allActions(), allow: false},
		{resource: rbac.ResourceWorkspace.InOrg(unuseID), actions: allActions(), allow: false},

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
		{resource: rbac.ResourceWorkspace.InOrg(unuseID).WithOwner(user.UserID).WithID(wrkID), actions: allActions(), allow: false},
		{resource: rbac.ResourceWorkspace.InOrg(unuseID).WithOwner(user.UserID), actions: allActions(), allow: false},
		{resource: rbac.ResourceWorkspace.InOrg(unuseID).WithID(wrkID), actions: allActions(), allow: false},
		{resource: rbac.ResourceWorkspace.InOrg(unuseID), actions: allActions(), allow: false},

		// Other org + other user + id
		{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithOwner("not-me").WithID(wrkID), actions: allActions(), allow: false},
		{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithOwner("not-me"), actions: allActions(), allow: false},

		{resource: rbac.ResourceWorkspace.WithOwner("not-me").WithID(wrkID), actions: allActions(), allow: false},
		{resource: rbac.ResourceWorkspace.WithOwner("not-me"), actions: allActions(), allow: false},

		// Other org + other use + other id
		{resource: rbac.ResourceWorkspace.InOrg(unuseID).WithOwner("not-me").WithID("not-id"), actions: allActions(), allow: false},
		{resource: rbac.ResourceWorkspace.InOrg(unuseID).WithOwner("not-me"), actions: allActions(), allow: false},
		{resource: rbac.ResourceWorkspace.InOrg(unuseID).WithID("not-id"), actions: allActions(), allow: false},
		{resource: rbac.ResourceWorkspace.InOrg(unuseID), actions: allActions(), allow: false},

		{resource: rbac.ResourceWorkspace.WithOwner("not-me").WithID("not-id"), actions: allActions(), allow: false},
		{resource: rbac.ResourceWorkspace.WithOwner("not-me"), actions: allActions(), allow: false},

		{resource: rbac.ResourceWorkspace.WithID("not-id"), actions: allActions(), allow: false},
	})

	user = subject{
		UserID: "me",
		Roles: []rbac.Role{
			must(rbac.RoleByName(rbac.RoleOrgAdmin(defOrg))),
			must(rbac.RoleByName(rbac.RoleMember())),
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
		{resource: rbac.ResourceWorkspace.InOrg(unuseID).WithOwner(user.UserID).WithID(wrkID), actions: allActions(), allow: false},
		{resource: rbac.ResourceWorkspace.InOrg(unuseID).WithOwner(user.UserID), actions: allActions(), allow: false},
		{resource: rbac.ResourceWorkspace.InOrg(unuseID).WithID(wrkID), actions: allActions(), allow: false},
		{resource: rbac.ResourceWorkspace.InOrg(unuseID), actions: allActions(), allow: false},

		// Other org + other user + id
		{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithOwner("not-me").WithID(wrkID), actions: allActions(), allow: true},
		{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithOwner("not-me"), actions: allActions(), allow: true},

		{resource: rbac.ResourceWorkspace.WithOwner("not-me").WithID(wrkID), actions: allActions(), allow: false},
		{resource: rbac.ResourceWorkspace.WithOwner("not-me"), actions: allActions(), allow: false},

		// Other org + other use + other id
		{resource: rbac.ResourceWorkspace.InOrg(unuseID).WithOwner("not-me").WithID("not-id"), actions: allActions(), allow: false},
		{resource: rbac.ResourceWorkspace.InOrg(unuseID).WithOwner("not-me"), actions: allActions(), allow: false},
		{resource: rbac.ResourceWorkspace.InOrg(unuseID).WithID("not-id"), actions: allActions(), allow: false},
		{resource: rbac.ResourceWorkspace.InOrg(unuseID), actions: allActions(), allow: false},

		{resource: rbac.ResourceWorkspace.WithOwner("not-me").WithID("not-id"), actions: allActions(), allow: false},
		{resource: rbac.ResourceWorkspace.WithOwner("not-me"), actions: allActions(), allow: false},

		{resource: rbac.ResourceWorkspace.WithID("not-id"), actions: allActions(), allow: false},
	})

	user = subject{
		UserID: "me",
		Roles: []rbac.Role{
			must(rbac.RoleByName(rbac.RoleAdmin())),
			must(rbac.RoleByName(rbac.RoleMember())),
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
		{resource: rbac.ResourceWorkspace.InOrg(unuseID).WithOwner(user.UserID).WithID(wrkID), actions: allActions(), allow: true},
		{resource: rbac.ResourceWorkspace.InOrg(unuseID).WithOwner(user.UserID), actions: allActions(), allow: true},
		{resource: rbac.ResourceWorkspace.InOrg(unuseID).WithID(wrkID), actions: allActions(), allow: true},
		{resource: rbac.ResourceWorkspace.InOrg(unuseID), actions: allActions(), allow: true},

		// Other org + other user + id
		{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithOwner("not-me").WithID(wrkID), actions: allActions(), allow: true},
		{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithOwner("not-me"), actions: allActions(), allow: true},

		{resource: rbac.ResourceWorkspace.WithOwner("not-me").WithID(wrkID), actions: allActions(), allow: true},
		{resource: rbac.ResourceWorkspace.WithOwner("not-me"), actions: allActions(), allow: true},

		// Other org + other use + other id
		{resource: rbac.ResourceWorkspace.InOrg(unuseID).WithOwner("not-me").WithID("not-id"), actions: allActions(), allow: true},
		{resource: rbac.ResourceWorkspace.InOrg(unuseID).WithOwner("not-me"), actions: allActions(), allow: true},
		{resource: rbac.ResourceWorkspace.InOrg(unuseID).WithID("not-id"), actions: allActions(), allow: true},
		{resource: rbac.ResourceWorkspace.InOrg(unuseID), actions: allActions(), allow: true},

		{resource: rbac.ResourceWorkspace.WithOwner("not-me").WithID("not-id"), actions: allActions(), allow: true},
		{resource: rbac.ResourceWorkspace.WithOwner("not-me"), actions: allActions(), allow: true},

		{resource: rbac.ResourceWorkspace.WithID("not-id"), actions: allActions(), allow: true},
	})

	// In practice this is a token scope on a regular subject
	user = subject{
		UserID: "me",
		Roles: []rbac.Role{
			{
				Name: fmt.Sprintf("agent-%s", wrkID),
				// This is at the site level to prevent the token from losing access if the user
				// is kicked from the org
				Site: []rbac.Permission{
					{
						Negate:       false,
						ResourceType: rbac.ResourceWorkspace.Type,
						ResourceID:   wrkID,
						Action:       rbac.ActionRead,
					},
				},
			},
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
			{resource: rbac.ResourceWorkspace.InOrg(unuseID).WithOwner(user.UserID).WithID(wrkID), allow: true},
			{resource: rbac.ResourceWorkspace.InOrg(unuseID).WithOwner(user.UserID), allow: false},
			{resource: rbac.ResourceWorkspace.InOrg(unuseID).WithID(wrkID), allow: true},
			{resource: rbac.ResourceWorkspace.InOrg(unuseID), allow: false},

			// Other org + other user + id
			{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithOwner("not-me").WithID(wrkID), allow: true},
			{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithOwner("not-me"), allow: false},

			{resource: rbac.ResourceWorkspace.WithOwner("not-me").WithID(wrkID), allow: true},
			{resource: rbac.ResourceWorkspace.WithOwner("not-me"), allow: false},

			// Other org + other use + other id
			{resource: rbac.ResourceWorkspace.InOrg(unuseID).WithOwner("not-me").WithID("not-id"), allow: false},
			{resource: rbac.ResourceWorkspace.InOrg(unuseID).WithOwner("not-me"), allow: false},
			{resource: rbac.ResourceWorkspace.InOrg(unuseID).WithID("not-id"), allow: false},
			{resource: rbac.ResourceWorkspace.InOrg(unuseID), allow: false},

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
			{resource: rbac.ResourceWorkspace.InOrg(unuseID).WithOwner(user.UserID).WithID(wrkID)},
			{resource: rbac.ResourceWorkspace.InOrg(unuseID).WithOwner(user.UserID)},
			{resource: rbac.ResourceWorkspace.InOrg(unuseID).WithID(wrkID)},
			{resource: rbac.ResourceWorkspace.InOrg(unuseID)},

			// Other org + other user + id
			{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithOwner("not-me").WithID(wrkID)},
			{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithOwner("not-me")},

			{resource: rbac.ResourceWorkspace.WithOwner("not-me").WithID(wrkID)},
			{resource: rbac.ResourceWorkspace.WithOwner("not-me")},

			// Other org + other use + other id
			{resource: rbac.ResourceWorkspace.InOrg(unuseID).WithOwner("not-me").WithID("not-id")},
			{resource: rbac.ResourceWorkspace.InOrg(unuseID).WithOwner("not-me")},
			{resource: rbac.ResourceWorkspace.InOrg(unuseID).WithID("not-id")},
			{resource: rbac.ResourceWorkspace.InOrg(unuseID)},

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
					defOrg.String(): {{
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
			{resource: rbac.ResourceWorkspace.InOrg(unuseID).WithOwner(user.UserID).WithID(wrkID), allow: false},
			{resource: rbac.ResourceWorkspace.InOrg(unuseID).WithOwner(user.UserID), allow: false},
			{resource: rbac.ResourceWorkspace.InOrg(unuseID).WithID(wrkID), allow: false},
			{resource: rbac.ResourceWorkspace.InOrg(unuseID), allow: false},

			// Other org + other user + id
			{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithOwner("not-me").WithID(wrkID), allow: true},
			{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithOwner("not-me"), allow: true},

			{resource: rbac.ResourceWorkspace.WithOwner("not-me").WithID(wrkID), allow: false},
			{resource: rbac.ResourceWorkspace.WithOwner("not-me"), allow: false},

			// Other org + other use + other id
			{resource: rbac.ResourceWorkspace.InOrg(unuseID).WithOwner("not-me").WithID("not-id"), allow: false},
			{resource: rbac.ResourceWorkspace.InOrg(unuseID).WithOwner("not-me"), allow: false},
			{resource: rbac.ResourceWorkspace.InOrg(unuseID).WithID("not-id"), allow: false},
			{resource: rbac.ResourceWorkspace.InOrg(unuseID), allow: false},

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
			{resource: rbac.ResourceWorkspace.InOrg(unuseID).WithOwner(user.UserID).WithID(wrkID)},
			{resource: rbac.ResourceWorkspace.InOrg(unuseID).WithOwner(user.UserID)},
			{resource: rbac.ResourceWorkspace.InOrg(unuseID).WithID(wrkID)},
			{resource: rbac.ResourceWorkspace.InOrg(unuseID)},

			// Other org + other user + id
			{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithOwner("not-me").WithID(wrkID)},
			{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithOwner("not-me")},

			{resource: rbac.ResourceWorkspace.WithOwner("not-me").WithID(wrkID)},
			{resource: rbac.ResourceWorkspace.WithOwner("not-me")},

			// Other org + other use + other id
			{resource: rbac.ResourceWorkspace.InOrg(unuseID).WithOwner("not-me").WithID("not-id")},
			{resource: rbac.ResourceWorkspace.InOrg(unuseID).WithOwner("not-me")},
			{resource: rbac.ResourceWorkspace.InOrg(unuseID).WithID("not-id")},
			{resource: rbac.ResourceWorkspace.InOrg(unuseID)},

			{resource: rbac.ResourceWorkspace.WithOwner("not-me").WithID("not-id")},
			{resource: rbac.ResourceWorkspace.WithOwner("not-me")},

			{resource: rbac.ResourceWorkspace.WithID("not-id")},
		}))
}

// TestAuthorizeLevels ensures level overrides are acting appropriately
//nolint:paralleltest
func TestAuthorizeLevels(t *testing.T) {
	defOrg := uuid.New()
	unusedID := uuid.New()
	wrkID := "1234"

	user := subject{
		UserID: "me",
		Roles: []rbac.Role{
			must(rbac.RoleByName(rbac.RoleAdmin())),
			{
				Name: "org-deny:" + defOrg.String(),
				Org: map[string][]rbac.Permission{
					defOrg.String(): {
						{
							Negate:       true,
							ResourceType: "*",
							ResourceID:   "*",
							Action:       "*",
						},
					},
				},
			},
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
			{resource: rbac.ResourceWorkspace.InOrg(unusedID).WithOwner(user.UserID).WithID(wrkID)},
			{resource: rbac.ResourceWorkspace.InOrg(unusedID).WithOwner(user.UserID)},
			{resource: rbac.ResourceWorkspace.InOrg(unusedID).WithID(wrkID)},
			{resource: rbac.ResourceWorkspace.InOrg(unusedID)},

			// Other org + other user + id
			{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithOwner("not-me").WithID(wrkID)},
			{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithOwner("not-me")},

			{resource: rbac.ResourceWorkspace.WithOwner("not-me").WithID(wrkID)},
			{resource: rbac.ResourceWorkspace.WithOwner("not-me")},

			// Other org + other use + other id
			{resource: rbac.ResourceWorkspace.InOrg(unusedID).WithOwner("not-me").WithID("not-id")},
			{resource: rbac.ResourceWorkspace.InOrg(unusedID).WithOwner("not-me")},
			{resource: rbac.ResourceWorkspace.InOrg(unusedID).WithID("not-id")},
			{resource: rbac.ResourceWorkspace.InOrg(unusedID)},

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
			must(rbac.RoleByName(rbac.RoleOrgAdmin(defOrg))),
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
			{resource: rbac.ResourceWorkspace.InOrg(unusedID).WithOwner(user.UserID).WithID(wrkID), allow: false},
			{resource: rbac.ResourceWorkspace.InOrg(unusedID).WithOwner(user.UserID), allow: false},
			{resource: rbac.ResourceWorkspace.InOrg(unusedID).WithID(wrkID), allow: false},
			{resource: rbac.ResourceWorkspace.InOrg(unusedID), allow: false},

			// Other org + other user + id
			{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithOwner("not-me").WithID(wrkID), allow: true},
			{resource: rbac.ResourceWorkspace.InOrg(defOrg).WithOwner("not-me"), allow: true},

			{resource: rbac.ResourceWorkspace.WithOwner("not-me").WithID(wrkID), allow: false},
			{resource: rbac.ResourceWorkspace.WithOwner("not-me"), allow: false},

			// Other org + other use + other id
			{resource: rbac.ResourceWorkspace.InOrg(unusedID).WithOwner("not-me").WithID("not-id"), allow: false},
			{resource: rbac.ResourceWorkspace.InOrg(unusedID).WithOwner("not-me"), allow: false},
			{resource: rbac.ResourceWorkspace.InOrg(unusedID).WithID("not-id"), allow: false},
			{resource: rbac.ResourceWorkspace.InOrg(unusedID), allow: false},

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

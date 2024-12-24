package dbauthz_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/rbac"
)

// nolint:tparallel
func TestGroupsAuth(t *testing.T) {
	t.Parallel()

	authz := rbac.NewAuthorizer(prometheus.NewRegistry())
	store, _ := dbtestutil.NewDB(t)
	db := dbauthz.New(store, authz, slogtest.Make(t, &slogtest.Options{
		IgnoreErrors: true,
	}), coderdtest.AccessControlStorePointer())

	ownerCtx := dbauthz.As(context.Background(), rbac.Subject{
		ID:     "owner",
		Roles:  rbac.Roles(must(rbac.RoleIdentifiers{rbac.RoleOwner()}.Expand())),
		Groups: []string{},
		Scope:  rbac.ExpandableScope(rbac.ScopeAll),
	})

	org := dbgen.Organization(t, db, database.Organization{})
	group := dbgen.Group(t, db, database.Group{
		OrganizationID: org.ID,
	})

	var users []database.User
	for i := 0; i < 5; i++ {
		user := dbgen.User(t, db, database.User{})
		users = append(users, user)
		err := db.InsertGroupMember(ownerCtx, database.InsertGroupMemberParams{
			UserID:  user.ID,
			GroupID: group.ID,
		})
		require.NoError(t, err)
	}

	totalMembers := len(users)
	testCases := []struct {
		Name            string
		Subject         rbac.Subject
		ReadGroup       bool
		ReadMembers     bool
		MembersExpected int
	}{
		{
			Name: "Owner",
			Subject: rbac.Subject{
				ID:     "owner",
				Roles:  rbac.Roles(must(rbac.RoleIdentifiers{rbac.RoleOwner()}.Expand())),
				Groups: []string{},
				Scope:  rbac.ExpandableScope(rbac.ScopeAll),
			},
			ReadGroup:       true,
			ReadMembers:     true,
			MembersExpected: totalMembers,
		},
		{
			Name: "UserAdmin",
			Subject: rbac.Subject{
				ID:     "useradmin",
				Roles:  rbac.Roles(must(rbac.RoleIdentifiers{rbac.RoleUserAdmin()}.Expand())),
				Groups: []string{},
				Scope:  rbac.ExpandableScope(rbac.ScopeAll),
			},
			ReadGroup:       true,
			ReadMembers:     true,
			MembersExpected: totalMembers,
		},
		{
			Name: "OrgAdmin",
			Subject: rbac.Subject{
				ID:     "orgadmin",
				Roles:  rbac.Roles(must(rbac.RoleIdentifiers{rbac.ScopedRoleOrgAdmin(org.ID)}.Expand())),
				Groups: []string{},
				Scope:  rbac.ExpandableScope(rbac.ScopeAll),
			},
			ReadGroup:       true,
			ReadMembers:     true,
			MembersExpected: totalMembers,
		},
		{
			Name: "OrgUserAdmin",
			Subject: rbac.Subject{
				ID:     "orgUserAdmin",
				Roles:  rbac.Roles(must(rbac.RoleIdentifiers{rbac.ScopedRoleOrgUserAdmin(org.ID)}.Expand())),
				Groups: []string{},
				Scope:  rbac.ExpandableScope(rbac.ScopeAll),
			},
			ReadGroup:       true,
			ReadMembers:     true,
			MembersExpected: totalMembers,
		},
		{
			Name: "GroupMember",
			Subject: rbac.Subject{
				ID:    users[0].ID.String(),
				Roles: rbac.Roles(must(rbac.RoleIdentifiers{rbac.RoleMember(), rbac.ScopedRoleOrgMember(org.ID)}.Expand())),
				Groups: []string{
					group.ID.String(),
				},
				Scope: rbac.ExpandableScope(rbac.ScopeAll),
			},
			ReadGroup:       true,
			ReadMembers:     true,
			MembersExpected: 1,
		},
		{
			// Org admin in the incorrect organization
			Name: "DifferentOrgAdmin",
			Subject: rbac.Subject{
				ID:     "orgadmin",
				Roles:  rbac.Roles(must(rbac.RoleIdentifiers{rbac.ScopedRoleOrgUserAdmin(uuid.New())}.Expand())),
				Groups: []string{},
				Scope:  rbac.ExpandableScope(rbac.ScopeAll),
			},
			ReadGroup:   false,
			ReadMembers: false,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()

			actorCtx := dbauthz.As(context.Background(), tc.Subject)
			_, err := db.GetGroupByID(actorCtx, group.ID)
			if tc.ReadGroup {
				require.NoError(t, err, "group read")
			} else {
				require.Error(t, err, "group read")
			}

			members, err := db.GetGroupMembersByGroupID(actorCtx, group.ID)
			if tc.ReadMembers {
				require.NoError(t, err, "member read")
				require.Len(t, members, tc.MembersExpected, "member count found does not match")
			} else {
				require.Len(t, members, 0, "member count is not 0")
			}
		})
	}
}

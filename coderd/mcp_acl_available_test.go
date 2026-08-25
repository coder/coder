package coderd_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/rbac/rolestore"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestMCPServerConfigACLAvailable(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, db, _ := newMCPClientWithDatabase(t)
	firstUser := coderdtest.CreateFirstUser(t, client)
	config := createMCPServerConfig(t, client, firstUser.OrganizationID, testutil.GetRandomName(t), true)

	needleUser := dbgen.User(t, db, database.User{
		Username: "needle-user-" + testutil.GetRandomName(t),
		Name:     "Needle User",
		Email:    testutil.GetRandomName(t) + "@example.com",
	})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		OrganizationID: firstUser.OrganizationID,
		UserID:         needleUser.ID,
	})
	otherMember := dbgen.User(t, db, database.User{
		Username: "other-user-" + testutil.GetRandomName(t),
		Email:    testutil.GetRandomName(t) + "@example.com",
	})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		OrganizationID: firstUser.OrganizationID,
		UserID:         otherMember.ID,
	})

	needleGroup := dbgen.Group(t, db, database.Group{
		OrganizationID: firstUser.OrganizationID,
		Name:           "needle-group-" + testutil.GetRandomName(t),
		DisplayName:    "Needle Group",
	})
	dbgen.GroupMember(t, db, database.GroupMemberTable{GroupID: needleGroup.ID, UserID: needleUser.ID})
	dbgen.GroupMember(t, db, database.GroupMemberTable{GroupID: needleGroup.ID, UserID: otherMember.ID})

	otherOrganization := dbgen.Organization(t, db, database.Organization{})
	foreignUser := dbgen.User(t, db, database.User{
		Username: "needle-foreign-user-" + testutil.GetRandomName(t),
		Email:    testutil.GetRandomName(t) + "@example.com",
	})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		OrganizationID: otherOrganization.ID,
		UserID:         foreignUser.ID,
	})
	foreignGroup := dbgen.Group(t, db, database.Group{
		OrganizationID: otherOrganization.ID,
		Name:           "needle-foreign-group-" + testutil.GetRandomName(t),
	})

	available, err := client.MCPServerConfigACLAvailable(ctx, firstUser.OrganizationID, config.ID, codersdk.UsersRequest{})
	require.NoError(t, err)
	usersByID := make(map[uuid.UUID]codersdk.ReducedUser, len(available.Users))
	for _, user := range available.Users {
		usersByID[user.ID] = user
	}
	require.Equal(t, needleUser.Username, usersByID[needleUser.ID].Username)
	require.Equal(t, needleUser.Name, usersByID[needleUser.ID].Name)
	require.Equal(t, needleUser.Email, usersByID[needleUser.ID].Email)
	require.Contains(t, usersByID, otherMember.ID)
	require.NotContains(t, usersByID, database.PrebuildsSystemUserID)
	require.NotContains(t, usersByID, foreignUser.ID)

	groupsByID := make(map[uuid.UUID]codersdk.Group, len(available.Groups))
	for _, group := range available.Groups {
		groupsByID[group.ID] = group
		require.Empty(t, group.Members)
	}
	require.Equal(t, needleGroup.Name, groupsByID[needleGroup.ID].Name)
	require.Equal(t, needleGroup.DisplayName, groupsByID[needleGroup.ID].DisplayName)
	require.Equal(t, 2, groupsByID[needleGroup.ID].TotalMemberCount)
	require.Equal(t, 3, groupsByID[firstUser.OrganizationID].TotalMemberCount)
	require.NotContains(t, groupsByID, foreignGroup.ID)

	filtered, err := client.MCPServerConfigACLAvailable(ctx, firstUser.OrganizationID, config.ID, codersdk.UsersRequest{
		SearchQuery: "needle",
		Pagination:  codersdk.Pagination{Limit: 1},
	})
	require.NoError(t, err)
	require.Len(t, filtered.Users, 1)
	require.Equal(t, needleUser.ID, filtered.Users[0].ID)
	require.Len(t, filtered.Groups, 1)
	require.Equal(t, needleGroup.ID, filtered.Groups[0].ID)
	require.Equal(t, 2, filtered.Groups[0].TotalMemberCount)
	require.Empty(t, filtered.Groups[0].Members)

	memberClient, _ := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)
	_, err = memberClient.MCPServerConfigACLAvailable(ctx, firstUser.OrganizationID, config.ID, codersdk.UsersRequest{})
	requireSDKError(t, err, http.StatusNotFound)

	_, err = client.MCPServerConfigACLAvailable(ctx, otherOrganization.ID, config.ID, codersdk.UsersRequest{})
	requireSDKError(t, err, http.StatusNotFound)
}

func TestMCPServerConfigDisabledShareOnlyFetch(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, rawDB, _ := newMCPClientWithDatabase(t)
	firstUser := coderdtest.CreateFirstUser(t, client)

	config := createMCPServerConfig(t, client, firstUser.OrganizationID, testutil.GetRandomName(t), false)

	sharerClient, sharer := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)
	_, err := sharerClient.MCPServerConfigByID(ctx, firstUser.OrganizationID, config.ID)
	requireSDKError(t, err, http.StatusNotFound)

	role, err := rawDB.InsertCustomRole(ctx, database.InsertCustomRoleParams{
		Name:           testutil.GetRandomName(t),
		DisplayName:    "MCP Server Config Sharer",
		OrganizationID: uuid.NullUUID{UUID: firstUser.OrganizationID, Valid: true},
		OrgPermissions: database.CustomRolePermissions{
			{
				ResourceType: rbac.ResourceMCPServerConfig.Type,
				Action:       policy.ActionRead,
			},
			{
				ResourceType: rbac.ResourceMCPServerConfig.Type,
				Action:       policy.ActionShare,
			},
		},
	})
	require.NoError(t, err)
	_, err = client.UpdateOrganizationMemberRoles(ctx, firstUser.OrganizationID, sharer.ID.String(), codersdk.UpdateRoles{
		Roles: []string{role.Name},
	})
	require.NoError(t, err)

	// Share-authorized callers keep access to disabled configs so they can
	// still open them and manage sharing, matching the list behavior.
	fetched, err := sharerClient.MCPServerConfigByID(ctx, firstUser.OrganizationID, config.ID)
	require.NoError(t, err)
	require.Equal(t, config.ID, fetched.ID)
	require.False(t, fetched.Enabled)
	require.Empty(t, fetched.URL)

	configs, err := sharerClient.MCPServerConfigs(ctx, firstUser.OrganizationID)
	require.NoError(t, err)
	require.Len(t, configs, 1)
	require.Equal(t, config.ID, configs[0].ID)
}

func TestMCPServerConfigACLWorkspaceSharingModes(t *testing.T) {
	t.Parallel()

	modes := []database.ShareableWorkspaceOwners{
		database.ShareableWorkspaceOwnersNone,
		database.ShareableWorkspaceOwnersServiceAccounts,
		database.ShareableWorkspaceOwnersEveryone,
	}
	for _, mode := range modes {
		t.Run(string(mode), func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)
			client, rawDB, api := newMCPClientWithDatabase(t)
			firstUser := coderdtest.CreateFirstUser(t, client)
			systemCtx := dbauthz.AsSystemRestricted(ctx)

			organization, err := rawDB.UpdateOrganizationWorkspaceSharingSettings(ctx, database.UpdateOrganizationWorkspaceSharingSettingsParams{
				ID:                       firstUser.OrganizationID,
				ShareableWorkspaceOwners: mode,
				UpdatedAt:                dbtime.Now(),
			})
			require.NoError(t, err)
			_, _, err = rolestore.ReconcileSystemRole(systemCtx, api, database.CustomRole{
				Name: rbac.RoleOrgMember(),
				OrganizationID: uuid.NullUUID{
					UUID:  firstUser.OrganizationID,
					Valid: true,
				},
			}, organization)
			require.NoError(t, err)

			config := createMCPServerConfig(t, client, firstUser.OrganizationID, testutil.GetRandomName(t), true)
			preexistingGroup := dbgen.Group(t, rawDB, database.Group{
				OrganizationID: firstUser.OrganizationID,
				Name:           "preexisting-" + testutil.GetRandomName(t),
			})
			candidateGroup := dbgen.Group(t, rawDB, database.Group{
				OrganizationID: firstUser.OrganizationID,
				Name:           "candidate-" + testutil.GetRandomName(t),
				DisplayName:    "Candidate Group",
			})
			err = client.UpdateMCPServerConfigACL(ctx, firstUser.OrganizationID, config.ID, codersdk.UpdateMCPServerConfigACLRequest{
				GroupRoles: map[string]codersdk.MCPServerConfigRole{
					preexistingGroup.ID.String(): codersdk.MCPServerConfigRoleRead,
				},
			})
			require.NoError(t, err)

			sharerClient, sharer := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)
			_, candidateUser := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)
			dbgen.GroupMember(t, rawDB, database.GroupMemberTable{
				GroupID: candidateGroup.ID,
				UserID:  candidateUser.ID,
			})

			role, err := rawDB.InsertCustomRole(ctx, database.InsertCustomRoleParams{
				Name:           testutil.GetRandomName(t),
				DisplayName:    "MCP Server Config Sharer",
				OrganizationID: uuid.NullUUID{UUID: firstUser.OrganizationID, Valid: true},
				OrgPermissions: database.CustomRolePermissions{
					{
						ResourceType: rbac.ResourceMCPServerConfig.Type,
						Action:       policy.ActionRead,
					},
					{
						ResourceType: rbac.ResourceMCPServerConfig.Type,
						Action:       policy.ActionShare,
					},
				},
			})
			require.NoError(t, err)
			_, err = client.UpdateOrganizationMemberRoles(ctx, firstUser.OrganizationID, sharer.ID.String(), codersdk.UpdateRoles{
				Roles: []string{role.Name},
			})
			require.NoError(t, err)

			initialACL, err := sharerClient.MCPServerConfigACL(ctx, firstUser.OrganizationID, config.ID)
			require.NoError(t, err)
			require.Empty(t, initialACL.Users)
			require.Contains(t, mcpServerConfigACLGroupRoles(initialACL), firstUser.OrganizationID)
			preexistingACLGroup := mcpServerConfigACLGroupByID(t, initialACL, preexistingGroup.ID)
			require.Equal(t, preexistingGroup.Name, preexistingACLGroup.Name)
			require.Empty(t, preexistingACLGroup.Members)

			available, err := sharerClient.MCPServerConfigACLAvailable(ctx, firstUser.OrganizationID, config.ID, codersdk.UsersRequest{})
			require.NoError(t, err)
			availableUser := mcpServerConfigACLAvailableUserByID(t, available, candidateUser.ID)
			require.Equal(t, candidateUser.Username, availableUser.Username)
			availableGroup := mcpServerConfigACLAvailableGroupByID(t, available, candidateGroup.ID)
			require.Equal(t, candidateGroup.DisplayName, availableGroup.DisplayName)
			require.Equal(t, 1, availableGroup.TotalMemberCount)
			require.Empty(t, availableGroup.Members)

			err = sharerClient.UpdateMCPServerConfigACL(ctx, firstUser.OrganizationID, config.ID, codersdk.UpdateMCPServerConfigACLRequest{
				UserRoles: map[string]codersdk.MCPServerConfigRole{
					candidateUser.ID.String(): codersdk.MCPServerConfigRoleRead,
				},
				GroupRoles: map[string]codersdk.MCPServerConfigRole{
					candidateGroup.ID.String():   codersdk.MCPServerConfigRoleRead,
					preexistingGroup.ID.String(): codersdk.MCPServerConfigRoleDeleted,
				},
			})
			require.NoError(t, err)

			updatedACL, err := sharerClient.MCPServerConfigACL(ctx, firstUser.OrganizationID, config.ID)
			require.NoError(t, err)
			require.Equal(t, map[uuid.UUID]codersdk.MCPServerConfigRole{
				candidateUser.ID: codersdk.MCPServerConfigRoleRead,
			}, mcpServerConfigACLUserRoles(updatedACL))
			require.Equal(t, map[uuid.UUID]codersdk.MCPServerConfigRole{
				firstUser.OrganizationID: codersdk.MCPServerConfigRoleRead,
				candidateGroup.ID:        codersdk.MCPServerConfigRoleRead,
			}, mcpServerConfigACLGroupRoles(updatedACL))
		})
	}
}

func newMCPClientWithDatabase(t testing.TB) (client *codersdk.Client, rawDB database.Store, apiDB database.Store) {
	t.Helper()

	db, pubsub := dbtestutil.NewDB(t)
	providerKeys := coderdtest.FakeOpenAICompatProviderAPIKeys(t)
	client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
		Database:            db,
		Pubsub:              pubsub,
		DeploymentValues:    mcpDeploymentValues(t),
		ChatProviderAPIKeys: &providerKeys,
	})
	return client, db, api.Database
}

func mcpServerConfigACLUserRoles(acl codersdk.MCPServerConfigACL) map[uuid.UUID]codersdk.MCPServerConfigRole {
	roles := make(map[uuid.UUID]codersdk.MCPServerConfigRole, len(acl.Users))
	for _, user := range acl.Users {
		roles[user.ID] = user.Role
	}
	return roles
}

func mcpServerConfigACLGroupRoles(acl codersdk.MCPServerConfigACL) map[uuid.UUID]codersdk.MCPServerConfigRole {
	roles := make(map[uuid.UUID]codersdk.MCPServerConfigRole, len(acl.Groups))
	for _, group := range acl.Groups {
		roles[group.ID] = group.Role
	}
	return roles
}

func mcpServerConfigACLGroupByID(t testing.TB, acl codersdk.MCPServerConfigACL, groupID uuid.UUID) codersdk.MCPServerConfigGroup {
	t.Helper()
	for _, group := range acl.Groups {
		if group.ID == groupID {
			return group
		}
	}
	require.FailNow(t, "MCP server config ACL group not found", "group_id=%s", groupID)
	return codersdk.MCPServerConfigGroup{}
}

func mcpServerConfigACLAvailableUserByID(t testing.TB, available codersdk.ACLAvailable, userID uuid.UUID) codersdk.ReducedUser {
	t.Helper()
	for _, user := range available.Users {
		if user.ID == userID {
			return user
		}
	}
	require.FailNow(t, "MCP server config ACL available user not found", "user_id=%s", userID)
	return codersdk.ReducedUser{}
}

func mcpServerConfigACLAvailableGroupByID(t testing.TB, available codersdk.ACLAvailable, groupID uuid.UUID) codersdk.Group {
	t.Helper()
	for _, group := range available.Groups {
		if group.ID == groupID {
			return group
		}
	}
	require.FailNow(t, "MCP server config ACL available group not found", "group_id=%s", groupID)
	return codersdk.Group{}
}

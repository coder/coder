package coderd_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/audit"
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

func TestChatModelACL(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	mAudit := audit.NewMock()
	// Keep a raw store handle so the test can seed a stale
	// cross-organization ACL entry that no authorized caller could create.
	rawDB, pubsub := dbtestutil.NewDB(t)
	adminClient, db := newChatClientWithDatabase(t, func(opts *coderdtest.Options) {
		opts.Auditor = mAudit
		opts.Database = rawDB
		opts.Pubsub = pubsub
	})
	firstUser := coderdtest.CreateFirstUser(t, adminClient.Client)
	model := createChatModel(t, adminClient)
	memberClientRaw, member := coderdtest.CreateAnotherUser(t, adminClient.Client, firstUser.OrganizationID)
	memberClient := codersdk.NewExperimentalClient(memberClientRaw)
	groupMemberClientRaw, groupMember := coderdtest.CreateAnotherUser(t, adminClient.Client, firstUser.OrganizationID)
	groupMemberClient := codersdk.NewExperimentalClient(groupMemberClientRaw)
	group := dbgen.Group(t, db, database.Group{OrganizationID: firstUser.OrganizationID})
	dbgen.GroupMember(t, db, database.GroupMemberTable{GroupID: group.ID, UserID: groupMember.ID})

	initialACL, err := adminClient.ChatModelACL(ctx, firstUser.OrganizationID, model.ID)
	require.NoError(t, err)
	require.Empty(t, initialACL.Users)
	require.Len(t, initialACL.Groups, 1)
	everyone := initialACL.Groups[0]
	require.True(t, everyone.IsEveryone())
	require.Equal(t, firstUser.OrganizationID, everyone.ID)
	require.Equal(t, firstUser.OrganizationID, everyone.OrganizationID)
	require.Equal(t, codersdk.ChatRoleRead, everyone.Role)
	require.Equal(t, 3, everyone.TotalMemberCount)
	require.Empty(t, everyone.Members)

	mAudit.ResetLogs()
	err = adminClient.UpdateChatModelACL(ctx, firstUser.OrganizationID, model.ID, codersdk.UpdateChatModelACLRequest{
		UserRoles: map[string]codersdk.ChatRole{
			member.ID.String(): codersdk.ChatRoleRead,
		},
		GroupRoles: map[string]codersdk.ChatRole{
			firstUser.OrganizationID.String(): codersdk.ChatRoleDeleted,
			group.ID.String():                 codersdk.ChatRoleRead,
		},
	})
	require.NoError(t, err)

	logs := mAudit.AuditLogs()
	require.Len(t, logs, 1)
	require.Equal(t, database.AuditActionWrite, logs[0].Action)
	require.Equal(t, database.ResourceTypeChatModelConfig, logs[0].ResourceType)
	require.Equal(t, model.ID, logs[0].ResourceID)
	require.Equal(t, firstUser.UserID, logs[0].UserID)
	require.Equal(t, firstUser.OrganizationID, logs[0].OrganizationID)
	require.EqualValues(t, http.StatusNoContent, logs[0].StatusCode)

	updatedACL, err := adminClient.ChatModelACL(ctx, firstUser.OrganizationID, model.ID)
	require.NoError(t, err)
	require.Len(t, updatedACL.Users, 1)
	require.Equal(t, member.ID, updatedACL.Users[0].ID)
	require.Equal(t, member.Username, updatedACL.Users[0].Username)
	require.Equal(t, member.Name, updatedACL.Users[0].Name)
	require.Equal(t, member.AvatarURL, updatedACL.Users[0].AvatarURL)
	require.Equal(t, codersdk.ChatRoleRead, updatedACL.Users[0].Role)
	require.Len(t, updatedACL.Groups, 1)
	require.Equal(t, group.ID, updatedACL.Groups[0].ID)
	require.Equal(t, group.Name, updatedACL.Groups[0].Name)
	require.Equal(t, group.DisplayName, updatedACL.Groups[0].DisplayName)
	require.Equal(t, firstUser.OrganizationID, updatedACL.Groups[0].OrganizationID)
	require.Equal(t, 1, updatedACL.Groups[0].TotalMemberCount)
	require.Empty(t, updatedACL.Groups[0].Members)
	require.Equal(t, codersdk.ChatRoleRead, updatedACL.Groups[0].Role)

	_, err = memberClient.ChatModel(ctx, firstUser.OrganizationID, model.ID)
	require.NoError(t, err)
	_, err = groupMemberClient.ChatModel(ctx, firstUser.OrganizationID, model.ID)
	require.NoError(t, err)

	_, err = memberClient.ChatModelACL(ctx, firstUser.OrganizationID, model.ID)
	requireSDKError(t, err, http.StatusNotFound)
	err = memberClient.UpdateChatModelACL(ctx, firstUser.OrganizationID, model.ID, codersdk.UpdateChatModelACLRequest{})
	requireSDKError(t, err, http.StatusNotFound)

	err = adminClient.UpdateChatModelACL(ctx, firstUser.OrganizationID, model.ID, codersdk.UpdateChatModelACLRequest{
		UserRoles: map[string]codersdk.ChatRole{
			member.ID.String(): codersdk.ChatRoleDeleted,
		},
		GroupRoles: map[string]codersdk.ChatRole{
			group.ID.String(): codersdk.ChatRoleDeleted,
		},
	})
	require.NoError(t, err)

	emptyACL, err := adminClient.ChatModelACL(ctx, firstUser.OrganizationID, model.ID)
	require.NoError(t, err)
	require.Empty(t, emptyACL.Users)
	require.Empty(t, emptyACL.Groups)

	foreignOrganization := dbgen.Organization(t, db, database.Organization{})
	foreignGroup := dbgen.Group(t, db, database.Group{OrganizationID: foreignOrganization.ID})
	// Seed through the raw store: a stale cross-organization ACL entry
	// cannot be created through the authorized API surface.
	_, err = rawDB.UpdateChatModelConfigACLByID(ctx, database.UpdateChatModelConfigACLByIDParams{
		GroupACL: database.ChatACL{
			foreignGroup.ID.String(): {Permissions: []policy.Action{policy.ActionRead}},
		},
		UserACL: database.ChatACL{},
		ID:      model.ID,
	})
	require.NoError(t, err)

	scopedACL, err := adminClient.ChatModelACL(ctx, firstUser.OrganizationID, model.ID)
	require.NoError(t, err)
	require.Empty(t, scopedACL.Groups)

	_, err = memberClient.ChatModelACL(ctx, firstUser.OrganizationID, model.ID)
	requireSDKError(t, err, http.StatusNotFound)
	err = memberClient.UpdateChatModelACL(ctx, firstUser.OrganizationID, model.ID, codersdk.UpdateChatModelACLRequest{})
	requireSDKError(t, err, http.StatusNotFound)
	_, err = groupMemberClient.ChatModel(ctx, firstUser.OrganizationID, model.ID)
	requireSDKError(t, err, http.StatusNotFound)

	otherOrganization := dbgen.Organization(t, db, database.Organization{})
	_, err = adminClient.ChatModelACL(ctx, otherOrganization.ID, model.ID)
	requireSDKError(t, err, http.StatusNotFound)
}

func TestChatModelACLAvailable(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	adminClient, db := newChatClientWithDatabase(t)
	firstUser := coderdtest.CreateFirstUser(t, adminClient.Client)
	model := createChatModel(t, adminClient)

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

	available, err := adminClient.ChatModelACLAvailable(ctx, firstUser.OrganizationID, model.ID, codersdk.UsersRequest{})
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

	filtered, err := adminClient.ChatModelACLAvailable(ctx, firstUser.OrganizationID, model.ID, codersdk.UsersRequest{
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

	_, err = adminClient.ChatModelACLAvailable(ctx, otherOrganization.ID, model.ID, codersdk.UsersRequest{})
	requireSDKError(t, err, http.StatusNotFound)
}

// TestChatModelACLWorkspaceSharingModes covers the restrictive workspace
// sharing modes where org members lose directory read permissions. The
// default "everyone" mode is exercised by the other tests in this file.
func TestChatModelACLWorkspaceSharingModes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		mode                   database.ShareableWorkspaceOwners
		memberDirectoryVisible bool
	}{
		{
			mode:                   database.ShareableWorkspaceOwnersServiceAccounts,
			memberDirectoryVisible: true,
		},
		{
			mode:                   database.ShareableWorkspaceOwnersNone,
			memberDirectoryVisible: false,
		},
	}
	for _, test := range tests {
		t.Run(string(test.mode), func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)
			rawDB, pubsub := dbtestutil.NewDB(t)
			opts := newChatTestOptions(t, coderdtest.DeploymentValues(t))
			opts.Database = rawDB
			opts.Pubsub = pubsub
			client, _, api := coderdtest.NewWithAPI(t, opts)
			firstUser := coderdtest.CreateFirstUser(t, client)
			adminClient := codersdk.NewExperimentalClient(client)
			systemCtx := dbauthz.AsSystemRestricted(ctx)

			organization, err := rawDB.UpdateOrganizationWorkspaceSharingSettings(ctx, database.UpdateOrganizationWorkspaceSharingSettingsParams{
				ID:                       firstUser.OrganizationID,
				ShareableWorkspaceOwners: test.mode,
				UpdatedAt:                dbtime.Now(),
			})
			require.NoError(t, err)
			_, _, err = rolestore.ReconcileSystemRole(systemCtx, api.Database, database.CustomRole{
				Name: rbac.RoleOrgMember(),
				OrganizationID: uuid.NullUUID{
					UUID:  firstUser.OrganizationID,
					Valid: true,
				},
			}, organization)
			require.NoError(t, err)

			model := createChatModel(t, adminClient)
			preexistingGroup := dbgen.Group(t, rawDB, database.Group{
				OrganizationID: firstUser.OrganizationID,
				Name:           "preexisting-" + testutil.GetRandomName(t),
			})
			candidateGroup := dbgen.Group(t, rawDB, database.Group{
				OrganizationID: firstUser.OrganizationID,
				Name:           "candidate-" + testutil.GetRandomName(t),
				DisplayName:    "Candidate Group",
			})
			err = adminClient.UpdateChatModelACL(ctx, firstUser.OrganizationID, model.ID, codersdk.UpdateChatModelACLRequest{
				GroupRoles: map[string]codersdk.ChatRole{
					preexistingGroup.ID.String(): codersdk.ChatRoleRead,
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
				DisplayName:    "Chat Model Sharer",
				OrganizationID: uuid.NullUUID{UUID: firstUser.OrganizationID, Valid: true},
				OrgPermissions: database.CustomRolePermissions{
					{
						ResourceType: rbac.ResourceChatModelConfig.Type,
						Action:       policy.ActionRead,
					},
					{
						ResourceType: rbac.ResourceChatModelConfig.Type,
						Action:       policy.ActionShare,
					},
				},
			})
			require.NoError(t, err)
			_, err = client.UpdateOrganizationMemberRoles(ctx, firstUser.OrganizationID, sharer.ID.String(), codersdk.UpdateRoles{
				Roles: []string{role.Name},
			})
			require.NoError(t, err)
			sharerExperimentalClient := codersdk.NewExperimentalClient(sharerClient)

			members, memberDirectoryErr := sharerClient.OrganizationMembersPaginated(ctx, firstUser.OrganizationID, codersdk.UsersRequest{})
			if test.memberDirectoryVisible {
				require.NoError(t, memberDirectoryErr)
				require.True(t, paginatedMembersContain(members, candidateUser.ID))
			} else {
				require.Error(t, memberDirectoryErr)
			}

			// Neither restrictive mode grants org members group directory reads.
			_, groupDirectoryErr := sharerClient.OrganizationGroupsPaginated(ctx, firstUser.OrganizationID, codersdk.PaginatedGroupsRequest{})
			require.Error(t, groupDirectoryErr)

			initialACL, err := sharerExperimentalClient.ChatModelACL(ctx, firstUser.OrganizationID, model.ID)
			require.NoError(t, err)
			require.Empty(t, initialACL.Users)
			require.Equal(t, map[uuid.UUID]codersdk.ChatRole{
				firstUser.OrganizationID: codersdk.ChatRoleRead,
				preexistingGroup.ID:      codersdk.ChatRoleRead,
			}, chatModelACLGroupRoles(initialACL))
			preexistingACLGroup := chatModelACLGroupByID(t, initialACL, preexistingGroup.ID)
			require.Equal(t, preexistingGroup.Name, preexistingACLGroup.Name)
			require.Empty(t, preexistingACLGroup.Members)

			available, err := sharerExperimentalClient.ChatModelACLAvailable(ctx, firstUser.OrganizationID, model.ID, codersdk.UsersRequest{})
			require.NoError(t, err)
			availableUser := aclAvailableUserByID(t, available, candidateUser.ID)
			require.Equal(t, candidateUser.Username, availableUser.Username)
			require.Equal(t, candidateUser.Name, availableUser.Name)
			availableGroup := aclAvailableGroupByID(t, available, candidateGroup.ID)
			require.Equal(t, candidateGroup.Name, availableGroup.Name)
			require.Equal(t, candidateGroup.DisplayName, availableGroup.DisplayName)
			require.Equal(t, 1, availableGroup.TotalMemberCount)
			require.Empty(t, availableGroup.Members)

			err = sharerExperimentalClient.UpdateChatModelACL(ctx, firstUser.OrganizationID, model.ID, codersdk.UpdateChatModelACLRequest{
				UserRoles: map[string]codersdk.ChatRole{
					candidateUser.ID.String(): codersdk.ChatRoleRead,
				},
				GroupRoles: map[string]codersdk.ChatRole{
					candidateGroup.ID.String():   codersdk.ChatRoleRead,
					preexistingGroup.ID.String(): codersdk.ChatRoleDeleted,
				},
			})
			require.NoError(t, err)

			updatedACL, err := sharerExperimentalClient.ChatModelACL(ctx, firstUser.OrganizationID, model.ID)
			require.NoError(t, err)
			require.Equal(t, map[uuid.UUID]codersdk.ChatRole{
				candidateUser.ID: codersdk.ChatRoleRead,
			}, chatModelACLUserRoles(updatedACL))
			require.Equal(t, map[uuid.UUID]codersdk.ChatRole{
				firstUser.OrganizationID: codersdk.ChatRoleRead,
				candidateGroup.ID:        codersdk.ChatRoleRead,
			}, chatModelACLGroupRoles(updatedACL))
			require.Equal(t, candidateUser.Username, updatedACL.Users[0].Username)
			updatedGroup := chatModelACLGroupByID(t, updatedACL, candidateGroup.ID)
			require.Equal(t, 1, updatedGroup.TotalMemberCount)
			require.Empty(t, updatedGroup.Members)

			err = sharerExperimentalClient.UpdateChatModelACL(ctx, firstUser.OrganizationID, model.ID, codersdk.UpdateChatModelACLRequest{
				UserRoles: map[string]codersdk.ChatRole{
					candidateUser.ID.String(): codersdk.ChatRoleDeleted,
				},
				GroupRoles: map[string]codersdk.ChatRole{
					candidateGroup.ID.String(): codersdk.ChatRoleDeleted,
				},
			})
			require.NoError(t, err)

			finalACL, err := sharerExperimentalClient.ChatModelACL(ctx, firstUser.OrganizationID, model.ID)
			require.NoError(t, err)
			require.Empty(t, finalACL.Users)
			require.Equal(t, map[uuid.UUID]codersdk.ChatRole{
				firstUser.OrganizationID: codersdk.ChatRoleRead,
			}, chatModelACLGroupRoles(finalACL))
		})
	}
}

//nolint:tparallel,paralleltest // Subtests share one model ACL and run sequentially.
func TestChatModelACLSparseUpdate(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	adminClient, db := newChatClientWithDatabase(t)
	firstUser := coderdtest.CreateFirstUser(t, adminClient.Client)
	model := createChatModel(t, adminClient)
	_, firstMember := coderdtest.CreateAnotherUser(t, adminClient.Client, firstUser.OrganizationID)
	_, secondMember := coderdtest.CreateAnotherUser(t, adminClient.Client, firstUser.OrganizationID)
	group := dbgen.Group(t, db, database.Group{OrganizationID: firstUser.OrganizationID})

	err := adminClient.UpdateChatModelACL(ctx, firstUser.OrganizationID, model.ID, codersdk.UpdateChatModelACLRequest{
		UserRoles:  map[string]codersdk.ChatRole{firstMember.ID.String(): codersdk.ChatRoleRead},
		GroupRoles: map[string]codersdk.ChatRole{group.ID.String(): codersdk.ChatRoleRead},
	})
	require.NoError(t, err)

	err = adminClient.UpdateChatModelACL(ctx, firstUser.OrganizationID, model.ID, codersdk.UpdateChatModelACLRequest{
		UserRoles: map[string]codersdk.ChatRole{secondMember.ID.String(): codersdk.ChatRoleRead},
	})
	require.NoError(t, err)

	expectedUserRoles := map[uuid.UUID]codersdk.ChatRole{
		firstMember.ID:  codersdk.ChatRoleRead,
		secondMember.ID: codersdk.ChatRoleRead,
	}
	expectedGroupRoles := map[uuid.UUID]codersdk.ChatRole{
		firstUser.OrganizationID: codersdk.ChatRoleRead,
		group.ID:                 codersdk.ChatRoleRead,
	}
	modelACL, err := adminClient.ChatModelACL(ctx, firstUser.OrganizationID, model.ID)
	require.NoError(t, err)
	require.Equal(t, expectedUserRoles, chatModelACLUserRoles(modelACL))
	require.Equal(t, expectedGroupRoles, chatModelACLGroupRoles(modelACL))

	path := fmt.Sprintf(
		"/api/experimental/organizations/%s/chats/models/%s/acl",
		firstUser.OrganizationID,
		model.ID,
	)
	for _, test := range []struct {
		name string
		body json.RawMessage
	}{
		{name: "omitted maps", body: json.RawMessage(`{}`)},
		{name: "empty maps", body: json.RawMessage(`{"user_roles":{},"group_roles":{}}`)},
		{name: "null maps", body: json.RawMessage(`{"user_roles":null,"group_roles":null}`)},
	} {
		t.Run(test.name, func(t *testing.T) {
			res, err := adminClient.Request(ctx, http.MethodPatch, path, test.body)
			require.NoError(t, err)
			defer res.Body.Close()
			require.Equal(t, http.StatusNoContent, res.StatusCode)

			modelACL, err := adminClient.ChatModelACL(ctx, firstUser.OrganizationID, model.ID)
			require.NoError(t, err)
			require.Equal(t, expectedUserRoles, chatModelACLUserRoles(modelACL))
			require.Equal(t, expectedGroupRoles, chatModelACLGroupRoles(modelACL))
		})
	}

	err = adminClient.UpdateChatModelACL(ctx, firstUser.OrganizationID, model.ID, codersdk.UpdateChatModelACLRequest{
		UserRoles: map[string]codersdk.ChatRole{
			firstMember.ID.String(): codersdk.ChatRoleDeleted,
			uuid.NewString():        codersdk.ChatRoleDeleted,
		},
		GroupRoles: map[string]codersdk.ChatRole{
			group.ID.String(): codersdk.ChatRoleDeleted,
		},
	})
	require.NoError(t, err)

	modelACL, err = adminClient.ChatModelACL(ctx, firstUser.OrganizationID, model.ID)
	require.NoError(t, err)
	require.Equal(t, map[uuid.UUID]codersdk.ChatRole{
		secondMember.ID: codersdk.ChatRoleRead,
	}, chatModelACLUserRoles(modelACL))
	require.Equal(t, map[uuid.UUID]codersdk.ChatRole{
		firstUser.OrganizationID: codersdk.ChatRoleRead,
	}, chatModelACLGroupRoles(modelACL))

	err = adminClient.UpdateChatModelACL(ctx, firstUser.OrganizationID, model.ID, codersdk.UpdateChatModelACLRequest{
		UserRoles: map[string]codersdk.ChatRole{firstMember.ID.String(): codersdk.ChatRoleRead},
	})
	require.NoError(t, err)
	//nolint:gocritic // Seeding a former member requires system access.
	err = db.DeleteOrganizationMember(dbauthz.AsSystemRestricted(ctx), database.DeleteOrganizationMemberParams{
		OrganizationID: firstUser.OrganizationID,
		UserID:         firstMember.ID,
	})
	require.NoError(t, err)
	err = adminClient.UpdateChatModelACL(ctx, firstUser.OrganizationID, model.ID, codersdk.UpdateChatModelACLRequest{
		UserRoles: map[string]codersdk.ChatRole{firstMember.ID.String(): codersdk.ChatRoleDeleted},
	})
	require.NoError(t, err)

	otherOrganization := dbgen.Organization(t, db, database.Organization{})
	foreignUser := dbgen.User(t, db, database.User{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		OrganizationID: otherOrganization.ID,
		UserID:         foreignUser.ID,
	})
	foreignGroup := dbgen.Group(t, db, database.Group{OrganizationID: otherOrganization.ID})
	err = adminClient.UpdateChatModelACL(ctx, firstUser.OrganizationID, model.ID, codersdk.UpdateChatModelACLRequest{
		UserRoles:  map[string]codersdk.ChatRole{foreignUser.ID.String(): codersdk.ChatRoleDeleted},
		GroupRoles: map[string]codersdk.ChatRole{foreignGroup.ID.String(): codersdk.ChatRoleDeleted},
	})
	require.NoError(t, err)
}

//nolint:tparallel,paralleltest // Subtests verify the same ACL remains unchanged.
func TestChatModelACLValidationIsAtomic(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	adminClient, db := newChatClientWithDatabase(t)
	firstUser := coderdtest.CreateFirstUser(t, adminClient.Client)
	model := createChatModel(t, adminClient)
	_, member := coderdtest.CreateAnotherUser(t, adminClient.Client, firstUser.OrganizationID)
	err := adminClient.UpdateChatModelACL(ctx, firstUser.OrganizationID, model.ID, codersdk.UpdateChatModelACLRequest{
		UserRoles: map[string]codersdk.ChatRole{member.ID.String(): codersdk.ChatRoleRead},
	})
	require.NoError(t, err)
	initialACL, err := adminClient.ChatModelACL(ctx, firstUser.OrganizationID, model.ID)
	require.NoError(t, err)

	otherOrganization := dbgen.Organization(t, db, database.Organization{})
	foreignUser := dbgen.User(t, db, database.User{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		OrganizationID: otherOrganization.ID,
		UserID:         foreignUser.ID,
	})
	foreignGroup := dbgen.Group(t, db, database.Group{OrganizationID: otherOrganization.ID})
	duplicateBody := json.RawMessage(fmt.Sprintf(
		`{"user_roles":{%q:"read",%q:""}}`,
		member.ID.String(),
		strings.ToUpper(member.ID.String()),
	))

	tests := []struct {
		name string
		body any
	}{
		{
			name: "foreign user",
			body: codersdk.UpdateChatModelACLRequest{
				UserRoles: map[string]codersdk.ChatRole{foreignUser.ID.String(): codersdk.ChatRoleRead},
			},
		},
		{
			name: "foreign group",
			body: codersdk.UpdateChatModelACLRequest{
				GroupRoles: map[string]codersdk.ChatRole{foreignGroup.ID.String(): codersdk.ChatRoleRead},
			},
		},
		{
			name: "missing user",
			body: codersdk.UpdateChatModelACLRequest{
				UserRoles: map[string]codersdk.ChatRole{uuid.NewString(): codersdk.ChatRoleRead},
			},
		},
		{
			name: "missing group",
			body: codersdk.UpdateChatModelACLRequest{
				GroupRoles: map[string]codersdk.ChatRole{uuid.NewString(): codersdk.ChatRoleRead},
			},
		},
		{
			name: "invalid user permission",
			body: codersdk.UpdateChatModelACLRequest{
				UserRoles: map[string]codersdk.ChatRole{member.ID.String(): "write"},
			},
		},
		{
			name: "invalid group permission",
			body: codersdk.UpdateChatModelACLRequest{
				GroupRoles: map[string]codersdk.ChatRole{firstUser.OrganizationID.String(): "share"},
			},
		},
		{name: "duplicate canonical user ID", body: duplicateBody},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			res, err := adminClient.Request(ctx, http.MethodPatch, fmt.Sprintf(
				"/api/experimental/organizations/%s/chats/models/%s/acl",
				firstUser.OrganizationID,
				model.ID,
			), test.body)
			require.NoError(t, err)
			defer res.Body.Close()
			err = codersdk.ReadBodyAsError(res)
			requireSDKError(t, err, http.StatusBadRequest)

			modelACL, err := adminClient.ChatModelACL(ctx, firstUser.OrganizationID, model.ID)
			require.NoError(t, err)
			require.Equal(t, initialACL, modelACL)
		})
	}
}

func TestChatModelACLLockFailure(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	rawDB, pubsub := dbtestutil.NewDB(t)
	store := newFailNextAcquireLockStore(rawDB, 0)
	adminClient := newChatClient(t, func(opts *coderdtest.Options) {
		opts.Database = store
		opts.Pubsub = pubsub
	})
	firstUser := coderdtest.CreateFirstUser(t, adminClient.Client)
	model := createChatModel(t, adminClient)
	initialACL, err := adminClient.ChatModelACL(ctx, firstUser.OrganizationID, model.ID)
	require.NoError(t, err)

	store.lockID = database.GenLockID("chat_model_config_writes:" + firstUser.OrganizationID.String())
	store.failNextAcquireLock.Store(true)
	err = adminClient.UpdateChatModelACL(ctx, firstUser.OrganizationID, model.ID, codersdk.UpdateChatModelACLRequest{})
	requireSDKError(t, err, http.StatusInternalServerError)

	modelACL, err := adminClient.ChatModelACL(ctx, firstUser.OrganizationID, model.ID)
	require.NoError(t, err)
	require.Equal(t, initialACL, modelACL)
}

//nolint:tparallel,paralleltest // Subtests share one model and run sequentially.
func TestChatModelActionScopesWithOrgRead(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	adminClient, db := newChatClientWithDatabase(t)
	firstUser := coderdtest.CreateFirstUser(t, adminClient.Client)
	model := createChatModel(t, adminClient)

	newScopedClient := func(t *testing.T, scopes ...database.APIKeyScope) *codersdk.ExperimentalClient {
		t.Helper()
		_, token := dbgen.APIKey(t, db, database.APIKey{
			UserID: firstUser.UserID,
			Scopes: append(database.APIKeyScopes{"organization:read"}, scopes...),
		})
		client := codersdk.New(
			adminClient.URL,
			codersdk.WithSessionToken(token),
			codersdk.WithHTTPClient(coderdtest.NewIsolatedHTTPClient(adminClient.URL)),
		)
		t.Cleanup(client.HTTPClient.CloseIdleConnections)
		return codersdk.NewExperimentalClient(client)
	}

	t.Run("UpdateWithoutModelRead", func(t *testing.T) {
		updateClient := newScopedClient(t, database.ApiKeyScopeChatModelConfigUpdate)
		updated, err := updateClient.UpdateChatModel(ctx, firstUser.OrganizationID, model.ID, codersdk.UpdateChatModelRequest{
			DisplayName: "Updated without model read scope",
		})
		require.NoError(t, err)
		require.Equal(t, "Updated without model read scope", updated.DisplayName)

		_, err = updateClient.ChatModel(ctx, firstUser.OrganizationID, model.ID)
		requireSDKError(t, err, http.StatusNotFound)
	})

	t.Run("ShareWithoutModelRead", func(t *testing.T) {
		shareClient := newScopedClient(t, database.ApiKeyScopeChatModelConfigShare)
		_, err := shareClient.ChatModelACL(ctx, firstUser.OrganizationID, model.ID)
		require.NoError(t, err)
		err = shareClient.UpdateChatModelACL(ctx, firstUser.OrganizationID, model.ID, codersdk.UpdateChatModelACLRequest{})
		require.NoError(t, err)

		_, err = shareClient.ChatModel(ctx, firstUser.OrganizationID, model.ID)
		requireSDKError(t, err, http.StatusNotFound)
	})
}

func TestChatModelACLShareDenied(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	adminClient := newChatClient(t)
	firstUser := coderdtest.CreateFirstUser(t, adminClient.Client)
	model := createChatModel(t, adminClient)
	memberClientRaw, _ := coderdtest.CreateAnotherUser(t, adminClient.Client, firstUser.OrganizationID)
	memberClient := codersdk.NewExperimentalClient(memberClientRaw)

	_, err := memberClient.ChatModel(ctx, firstUser.OrganizationID, model.ID)
	require.NoError(t, err)
	_, err = memberClient.ChatModelACL(ctx, firstUser.OrganizationID, model.ID)
	requireSDKError(t, err, http.StatusNotFound)
	_, err = memberClient.ChatModelACLAvailable(ctx, firstUser.OrganizationID, model.ID, codersdk.UsersRequest{})
	requireSDKError(t, err, http.StatusNotFound)
	err = memberClient.UpdateChatModelACL(ctx, firstUser.OrganizationID, model.ID, codersdk.UpdateChatModelACLRequest{})
	requireSDKError(t, err, http.StatusNotFound)
}

func paginatedMembersContain(members codersdk.PaginatedMembersResponse, userID uuid.UUID) bool {
	for _, member := range members.Members {
		if member.UserID == userID {
			return true
		}
	}
	return false
}

func chatModelACLGroupByID(t *testing.T, modelACL codersdk.ChatModelACL, groupID uuid.UUID) codersdk.ChatGroup {
	t.Helper()
	for _, group := range modelACL.Groups {
		if group.ID == groupID {
			return group
		}
	}
	require.FailNow(t, "chat model ACL group not found", groupID.String())
	return codersdk.ChatGroup{}
}

func aclAvailableUserByID(t *testing.T, available codersdk.ACLAvailable, userID uuid.UUID) codersdk.ReducedUser {
	t.Helper()
	for _, user := range available.Users {
		if user.ID == userID {
			return user
		}
	}
	require.FailNow(t, "available ACL user not found", userID.String())
	return codersdk.ReducedUser{}
}

func aclAvailableGroupByID(t *testing.T, available codersdk.ACLAvailable, groupID uuid.UUID) codersdk.Group {
	t.Helper()
	for _, group := range available.Groups {
		if group.ID == groupID {
			return group
		}
	}
	require.FailNow(t, "available ACL group not found", groupID.String())
	return codersdk.Group{}
}

func chatModelACLUserRoles(modelACL codersdk.ChatModelACL) map[uuid.UUID]codersdk.ChatRole {
	roles := make(map[uuid.UUID]codersdk.ChatRole, len(modelACL.Users))
	for _, user := range modelACL.Users {
		roles[user.ID] = user.Role
	}
	return roles
}

func chatModelACLGroupRoles(modelACL codersdk.ChatModelACL) map[uuid.UUID]codersdk.ChatRole {
	roles := make(map[uuid.UUID]codersdk.ChatRole, len(modelACL.Groups))
	for _, group := range modelACL.Groups {
		roles[group.ID] = group.Role
	}
	return roles
}

func TestCreateChatModelRejectsACLKeys(t *testing.T) {
	t.Parallel()

	client := newChatClient(t)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)

	for _, test := range []struct {
		name  string
		key   string
		value any
	}{
		{name: "group_acl/object", key: "group_acl", value: map[string]any{}},
		{name: "group_acl/null", key: "group_acl", value: nil},
		{name: "user_acl/object", key: "user_acl", value: map[string]any{}},
		{name: "user_acl/null", key: "user_acl", value: nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			res, err := client.Request(ctx, http.MethodPost, fmt.Sprintf(
				"/api/experimental/organizations/%s/chats/models",
				firstUser.OrganizationID,
			), map[string]any{test.key: test.value})
			require.NoError(t, err)
			defer res.Body.Close()
			err = codersdk.ReadBodyAsError(res)
			sdkErr := requireSDKError(t, err, http.StatusBadRequest)
			require.Contains(t, sdkErr.Message, "nested /acl endpoint")
		})
	}
}

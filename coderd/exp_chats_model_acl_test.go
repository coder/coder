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
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestChatModelACL(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	mAudit := audit.NewMock()
	adminClient, db := newChatClientWithDatabase(t, func(opts *coderdtest.Options) {
		opts.Auditor = mAudit
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
	require.Equal(t, map[string]codersdk.ChatRole{}, initialACL.UserRoles)
	require.Equal(t, map[string]codersdk.ChatRole{
		firstUser.OrganizationID.String(): codersdk.ChatRoleRead,
	}, initialACL.GroupRoles)

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
	require.Equal(t, map[string]codersdk.ChatRole{
		member.ID.String(): codersdk.ChatRoleRead,
	}, updatedACL.UserRoles)
	require.Equal(t, map[string]codersdk.ChatRole{
		group.ID.String(): codersdk.ChatRoleRead,
	}, updatedACL.GroupRoles)

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
	require.Empty(t, emptyACL.UserRoles)
	require.Empty(t, emptyACL.GroupRoles)

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

	expected := codersdk.ChatModelACL{
		UserRoles: map[string]codersdk.ChatRole{
			firstMember.ID.String():  codersdk.ChatRoleRead,
			secondMember.ID.String(): codersdk.ChatRoleRead,
		},
		GroupRoles: map[string]codersdk.ChatRole{
			firstUser.OrganizationID.String(): codersdk.ChatRoleRead,
			group.ID.String():                 codersdk.ChatRoleRead,
		},
	}
	modelACL, err := adminClient.ChatModelACL(ctx, firstUser.OrganizationID, model.ID)
	require.NoError(t, err)
	require.Equal(t, expected, modelACL)

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
			require.Equal(t, expected, modelACL)
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
	require.Equal(t, map[string]codersdk.ChatRole{
		secondMember.ID.String(): codersdk.ChatRoleRead,
	}, modelACL.UserRoles)
	require.Equal(t, map[string]codersdk.ChatRole{
		firstUser.OrganizationID.String(): codersdk.ChatRoleRead,
	}, modelACL.GroupRoles)

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
func TestChatModelActionOnlyScopes(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	adminClient, db := newChatClientWithDatabase(t)
	firstUser := coderdtest.CreateFirstUser(t, adminClient.Client)
	model := createChatModel(t, adminClient)

	newScopedClient := func(t *testing.T, scope database.APIKeyScope) *codersdk.ExperimentalClient {
		t.Helper()
		_, token := dbgen.APIKey(t, db, database.APIKey{
			UserID: firstUser.UserID,
			Scopes: database.APIKeyScopes{scope},
		})
		client := codersdk.New(
			adminClient.URL,
			codersdk.WithSessionToken(token),
			codersdk.WithHTTPClient(coderdtest.NewIsolatedHTTPClient(adminClient.URL)),
		)
		t.Cleanup(client.HTTPClient.CloseIdleConnections)
		return codersdk.NewExperimentalClient(client)
	}

	t.Run("UpdateWithoutRead", func(t *testing.T) {
		updateClient := newScopedClient(t, database.ApiKeyScopeChatModelConfigUpdate)
		updated, err := updateClient.UpdateChatModel(ctx, firstUser.OrganizationID, model.ID, codersdk.UpdateChatModelRequest{
			DisplayName: "Updated with update-only scope",
		})
		require.NoError(t, err)
		require.Equal(t, "Updated with update-only scope", updated.DisplayName)

		_, err = updateClient.ChatModel(ctx, firstUser.OrganizationID, model.ID)
		requireSDKError(t, err, http.StatusNotFound)
	})

	t.Run("ShareWithoutRead", func(t *testing.T) {
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
	err = memberClient.UpdateChatModelACL(ctx, firstUser.OrganizationID, model.ID, codersdk.UpdateChatModelACLRequest{})
	requireSDKError(t, err, http.StatusNotFound)
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

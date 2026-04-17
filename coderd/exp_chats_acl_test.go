package coderd_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func createSharedChat(
	ctx context.Context,
	t *testing.T,
	ownerClient *codersdk.ExperimentalClient,
	orgID uuid.UUID,
	title string,
) codersdk.Chat {
	t.Helper()

	chat, err := ownerClient.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: orgID,
		Content: []codersdk.ChatInputPart{
			{
				Type: codersdk.ChatInputPartTypeText,
				Text: title,
			},
		},
	})
	require.NoError(t, err)
	return chat
}

func TestPatchChatACL_AddsUserAndGroup(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	ownerClient, db := newChatClientWithDatabase(t)
	firstUser := coderdtest.CreateFirstUser(t, ownerClient.Client)
	_ = createChatModelConfig(t, ownerClient)

	_, viewer := coderdtest.CreateAnotherUser(t, ownerClient.Client, firstUser.OrganizationID)

	group := dbgen.Group(t, db, database.Group{OrganizationID: firstUser.OrganizationID})
	dbgen.GroupMember(t, db, database.GroupMemberTable{GroupID: group.ID, UserID: viewer.ID})

	chat := createSharedChat(ctx, t, ownerClient, firstUser.OrganizationID, "acl patch add user+group")

	err := ownerClient.UpdateChatACL(ctx, chat.ID, codersdk.UpdateChatACL{
		UserRoles: map[string]codersdk.ChatRole{
			viewer.ID.String(): codersdk.ChatRoleRead,
		},
		GroupRoles: map[string]codersdk.ChatRole{
			group.ID.String(): codersdk.ChatRoleRead,
		},
	})
	require.NoError(t, err)

	acl, err := ownerClient.ChatACL(ctx, chat.ID)
	require.NoError(t, err)

	require.Len(t, acl.Users, 1)
	require.Equal(t, viewer.ID, acl.Users[0].ID)
	require.Equal(t, codersdk.ChatRoleRead, acl.Users[0].Role)

	require.Len(t, acl.Groups, 1)
	require.Equal(t, group.ID, acl.Groups[0].ID)
	require.Equal(t, codersdk.ChatRoleRead, acl.Groups[0].Role)
}

func TestPatchChatACL_RejectsNonReadRole(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	ownerClient := newChatClient(t)
	firstUser := coderdtest.CreateFirstUser(t, ownerClient.Client)
	_ = createChatModelConfig(t, ownerClient)

	_, viewer := coderdtest.CreateAnotherUser(t, ownerClient.Client, firstUser.OrganizationID)

	chat := createSharedChat(ctx, t, ownerClient, firstUser.OrganizationID, "acl patch bad role")

	err := ownerClient.UpdateChatACL(ctx, chat.ID, codersdk.UpdateChatACL{
		UserRoles: map[string]codersdk.ChatRole{
			viewer.ID.String(): codersdk.ChatRole("admin"),
		},
	})
	requireSDKError(t, err, http.StatusBadRequest)
}

func TestPatchChatACL_SubChatRejected(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	ownerClient, db := newChatClientWithDatabase(t)
	firstUser := coderdtest.CreateFirstUser(t, ownerClient.Client)
	modelConfig := createChatModelConfig(t, ownerClient)

	_, viewer := coderdtest.CreateAnotherUser(t, ownerClient.Client, firstUser.OrganizationID)

	parent := createSharedChat(ctx, t, ownerClient, firstUser.OrganizationID, "root chat for sub-chat patch")

	subChat, err := db.InsertChat(dbauthz.AsSystemRestricted(ctx), database.InsertChatParams{
		OrganizationID:    firstUser.OrganizationID,
		Status:            database.ChatStatusWaiting,
		ClientType:        database.ChatClientTypeUi,
		OwnerID:           firstUser.UserID,
		LastModelConfigID: modelConfig.ID,
		Title:             "sub-chat",
		ParentChatID:      uuid.NullUUID{UUID: parent.ID, Valid: true},
		RootChatID:        uuid.NullUUID{UUID: parent.ID, Valid: true},
	})
	require.NoError(t, err)

	err = ownerClient.UpdateChatACL(ctx, subChat.ID, codersdk.UpdateChatACL{
		UserRoles: map[string]codersdk.ChatRole{
			viewer.ID.String(): codersdk.ChatRoleRead,
		},
	})
	sdkErr := requireSDKError(t, err, http.StatusBadRequest)
	require.Contains(t, sdkErr.Message, "root chats")
}

func TestPatchChatACL_RequiresToolConfirmation(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	ownerClient, db := newChatClientWithDatabase(t)
	firstUser := coderdtest.CreateFirstUser(t, ownerClient.Client)
	modelConfig := createChatModelConfig(t, ownerClient)

	_, viewer := coderdtest.CreateAnotherUser(t, ownerClient.Client, firstUser.OrganizationID)

	chat := createSharedChat(ctx, t, ownerClient, firstUser.OrganizationID, "acl patch tool confirm")

	toolCallPart := codersdk.ChatMessageToolCall(
		"call_abc",
		"demo_tool",
		json.RawMessage(`{"arg":"value"}`),
	)
	content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{toolCallPart})
	require.NoError(t, err)

	_, err = db.InsertChatMessages(dbauthz.AsSystemRestricted(ctx), database.InsertChatMessagesParams{
		ChatID:              chat.ID,
		CreatedBy:           []uuid.UUID{uuid.Nil},
		ModelConfigID:       []uuid.UUID{modelConfig.ID},
		Role:                []database.ChatMessageRole{database.ChatMessageRoleAssistant},
		ContentVersion:      []int16{chatprompt.CurrentContentVersion},
		Content:             []string{string(content.RawMessage)},
		Visibility:          []database.ChatMessageVisibility{database.ChatMessageVisibilityBoth},
		InputTokens:         []int64{0},
		OutputTokens:        []int64{0},
		TotalTokens:         []int64{0},
		ReasoningTokens:     []int64{0},
		CacheCreationTokens: []int64{0},
		CacheReadTokens:     []int64{0},
		ContextLimit:        []int64{0},
		Compressed:          []bool{false},
		TotalCostMicros:     []int64{0},
		RuntimeMs:           []int64{0},
	})
	require.NoError(t, err)

	err = ownerClient.UpdateChatACL(ctx, chat.ID, codersdk.UpdateChatACL{
		UserRoles: map[string]codersdk.ChatRole{
			viewer.ID.String(): codersdk.ChatRoleRead,
		},
	})
	sdkErr := requireSDKError(t, err, http.StatusBadRequest)

	foundConfirmField := false
	for _, v := range sdkErr.Validations {
		if v.Field == "confirm_share_tool_calls" {
			foundConfirmField = true
			break
		}
	}
	require.True(t, foundConfirmField,
		"expected validation error on confirm_share_tool_calls, got: %+v", sdkErr.Validations)

	err = ownerClient.UpdateChatACL(ctx, chat.ID, codersdk.UpdateChatACL{
		UserRoles: map[string]codersdk.ChatRole{
			viewer.ID.String(): codersdk.ChatRoleRead,
		},
		ConfirmShareToolCalls: true,
	})
	require.NoError(t, err)

	acl, err := ownerClient.ChatACL(ctx, chat.ID)
	require.NoError(t, err)
	require.Len(t, acl.Users, 1)
	require.Equal(t, viewer.ID, acl.Users[0].ID)
}

func TestDeleteChatACL_ClearsEntries(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	rawClient := coderdtest.New(t, &coderdtest.Options{
		DeploymentValues: chatDeploymentValues(t),
	})
	ownerClient := codersdk.NewExperimentalClient(rawClient)
	firstUser := coderdtest.CreateFirstUser(t, ownerClient.Client)
	_ = createChatModelConfig(t, ownerClient)

	_, viewer := coderdtest.CreateAnotherUser(t, ownerClient.Client, firstUser.OrganizationID)

	chat := createSharedChat(ctx, t, ownerClient, firstUser.OrganizationID, "delete clears entries")

	err := ownerClient.UpdateChatACL(ctx, chat.ID, codersdk.UpdateChatACL{
		UserRoles: map[string]codersdk.ChatRole{
			viewer.ID.String(): codersdk.ChatRoleRead,
		},
	})
	require.NoError(t, err)

	err = ownerClient.DeleteChatACL(ctx, chat.ID)
	require.NoError(t, err)

	acl, err := ownerClient.ChatACL(ctx, chat.ID)
	require.NoError(t, err)
	require.Empty(t, acl.Users)
	require.Empty(t, acl.Groups)
}

func TestListChats_SharedFilter(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	ownerClient := newChatClient(t)
	firstUser := coderdtest.CreateFirstUser(t, ownerClient.Client)
	_ = createChatModelConfig(t, ownerClient)

	viewerRaw, viewer := coderdtest.CreateAnotherUser(t, ownerClient.Client, firstUser.OrganizationID)
	viewerClient := codersdk.NewExperimentalClient(viewerRaw)

	ownedOnly := createSharedChat(ctx, t, ownerClient, firstUser.OrganizationID, "owned only")
	sharedChat := createSharedChat(ctx, t, ownerClient, firstUser.OrganizationID, "owned and shared")

	err := ownerClient.UpdateChatACL(ctx, sharedChat.ID, codersdk.UpdateChatACL{
		UserRoles: map[string]codersdk.ChatRole{
			viewer.ID.String(): codersdk.ChatRoleRead,
		},
	})
	require.NoError(t, err)

	defaultList, err := viewerClient.ListChats(ctx, nil)
	require.NoError(t, err)
	require.Empty(t, defaultList, "default list should only include owned chats")

	includeList, err := viewerClient.ListChats(ctx, &codersdk.ListChatsOptions{
		Shared: codersdk.ChatSharedFilterInclude,
	})
	require.NoError(t, err)
	includeIDs := chatIDSet(includeList)
	require.Contains(t, includeIDs, sharedChat.ID)
	require.NotContains(t, includeIDs, ownedOnly.ID, "viewer does not own or share the first chat")

	onlyList, err := viewerClient.ListChats(ctx, &codersdk.ListChatsOptions{
		Shared: codersdk.ChatSharedFilterOnly,
	})
	require.NoError(t, err)
	onlyIDs := chatIDSet(onlyList)
	require.Contains(t, onlyIDs, sharedChat.ID)
	require.Len(t, onlyIDs, 1, "viewer has exactly one shared chat")

	ownerList, err := ownerClient.ListChats(ctx, nil)
	require.NoError(t, err)
	ownerIDs := chatIDSet(ownerList)
	require.Contains(t, ownerIDs, ownedOnly.ID)
	require.Contains(t, ownerIDs, sharedChat.ID)

	res, err := viewerClient.Request(ctx, http.MethodGet, "/api/experimental/chats?shared=wat", nil)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusBadRequest, res.StatusCode)
}

func chatIDSet(chats []codersdk.Chat) map[uuid.UUID]struct{} {
	ids := make(map[uuid.UUID]struct{}, len(chats))
	for _, c := range chats {
		ids[c.ID] = struct{}{}
	}
	return ids
}

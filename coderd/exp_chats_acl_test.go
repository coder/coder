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
		UserRoles: map[string]codersdk.ChatShareEntry{
			viewer.ID.String(): {Role: codersdk.ChatRoleRead},
		},
		GroupRoles: map[string]codersdk.ChatShareEntry{
			group.ID.String(): {Role: codersdk.ChatRoleRead},
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
		UserRoles: map[string]codersdk.ChatShareEntry{
			viewer.ID.String(): {Role: codersdk.ChatRole("admin")},
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
		UserRoles: map[string]codersdk.ChatShareEntry{
			viewer.ID.String(): {Role: codersdk.ChatRoleRead},
		},
	})
	sdkErr := requireSDKError(t, err, http.StatusBadRequest)
	require.Contains(t, sdkErr.Message, "root chats")
}

func TestPatchChatACL_StoresShareFlags(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	ownerClient, db := newChatClientWithDatabase(t)
	firstUser := coderdtest.CreateFirstUser(t, ownerClient.Client)
	_ = createChatModelConfig(t, ownerClient)

	_, viewer := coderdtest.CreateAnotherUser(t, ownerClient.Client, firstUser.OrganizationID)

	group := dbgen.Group(t, db, database.Group{OrganizationID: firstUser.OrganizationID})
	dbgen.GroupMember(t, db, database.GroupMemberTable{GroupID: group.ID, UserID: viewer.ID})

	chat := createSharedChat(ctx, t, ownerClient, firstUser.OrganizationID, "acl patch stores share flags")

	err := ownerClient.UpdateChatACL(ctx, chat.ID, codersdk.UpdateChatACL{
		UserRoles: map[string]codersdk.ChatShareEntry{
			viewer.ID.String(): {Role: codersdk.ChatRoleRead, ShareToolCalls: true, ShareAttachments: false},
		},
		GroupRoles: map[string]codersdk.ChatShareEntry{
			group.ID.String(): {Role: codersdk.ChatRoleRead, ShareToolCalls: false, ShareAttachments: true},
		},
	})
	require.NoError(t, err)

	acl, err := ownerClient.ChatACL(ctx, chat.ID)
	require.NoError(t, err)

	require.Len(t, acl.Users, 1)
	require.Equal(t, viewer.ID, acl.Users[0].ID)
	require.True(t, acl.Users[0].ShareToolCalls)
	require.False(t, acl.Users[0].ShareAttachments)

	require.Len(t, acl.Groups, 1)
	require.Equal(t, group.ID, acl.Groups[0].ID)
	require.False(t, acl.Groups[0].ShareToolCalls)
	require.True(t, acl.Groups[0].ShareAttachments)
}

func TestPatchChatACL_DefaultsHideEverything(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	ownerClient := newChatClient(t)
	firstUser := coderdtest.CreateFirstUser(t, ownerClient.Client)
	_ = createChatModelConfig(t, ownerClient)

	_, viewer := coderdtest.CreateAnotherUser(t, ownerClient.Client, firstUser.OrganizationID)

	chat := createSharedChat(ctx, t, ownerClient, firstUser.OrganizationID, "acl patch default flags")

	err := ownerClient.UpdateChatACL(ctx, chat.ID, codersdk.UpdateChatACL{
		UserRoles: map[string]codersdk.ChatShareEntry{
			viewer.ID.String(): {Role: codersdk.ChatRoleRead},
		},
	})
	require.NoError(t, err)

	acl, err := ownerClient.ChatACL(ctx, chat.ID)
	require.NoError(t, err)
	require.Len(t, acl.Users, 1)
	require.False(t, acl.Users[0].ShareToolCalls)
	require.False(t, acl.Users[0].ShareAttachments)
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
		UserRoles: map[string]codersdk.ChatShareEntry{
			viewer.ID.String(): {Role: codersdk.ChatRoleRead},
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
		UserRoles: map[string]codersdk.ChatShareEntry{
			viewer.ID.String(): {Role: codersdk.ChatRoleRead},
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

// insertShareTestAssistantMessage inserts a single assistant message
// with every part type the filter treats specially, plus text and
// reasoning (which must always pass through).
func insertShareTestAssistantMessage(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	chatID, modelConfigID, fileID uuid.UUID,
) {
	t.Helper()

	parts := []codersdk.ChatMessagePart{
		codersdk.ChatMessageText("hello world"),
		codersdk.ChatMessageReasoning("thinking..."),
		codersdk.ChatMessageToolCall("call_abc", "demo_tool", json.RawMessage(`{"arg":"value"}`)),
		codersdk.ChatMessageToolResult("call_abc", "demo_tool", json.RawMessage(`{"ok":true}`), false, false),
		{
			Type:      codersdk.ChatMessagePartTypeFile,
			FileID:    uuid.NullUUID{UUID: fileID, Valid: true},
			MediaType: "text/plain",
		},
		codersdk.ChatMessageFileReference("README.md", 1, 10, "example content"),
		{
			Type:            codersdk.ChatMessagePartTypeContextFile,
			ContextFilePath: "AGENTS.md",
		},
	}
	content, err := chatprompt.MarshalParts(parts)
	require.NoError(t, err)

	_, err = db.InsertChatMessages(dbauthz.AsSystemRestricted(ctx), database.InsertChatMessagesParams{
		ChatID:              chatID,
		CreatedBy:           []uuid.UUID{uuid.Nil},
		ModelConfigID:       []uuid.UUID{modelConfigID},
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
}

// insertSharedChatFile inserts a chat_files row and links it to the chat
// so the owner's Chat.Files is populated.
func insertSharedChatFile(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	orgID, ownerID, chatID uuid.UUID,
) uuid.UUID {
	t.Helper()

	//nolint:gocritic // Using AsChatd to mimic the chatd background worker that normally inserts files.
	chatdCtx := dbauthz.AsChatd(ctx)
	row, err := db.InsertChatFile(chatdCtx, database.InsertChatFileParams{
		OwnerID:        ownerID,
		OrganizationID: orgID,
		Name:           "shared.md",
		Mimetype:       "text/markdown",
		Data:           []byte("# Shared"),
	})
	require.NoError(t, err)
	rejected, err := db.LinkChatFiles(chatdCtx, database.LinkChatFilesParams{
		ChatID:       chatID,
		MaxFileLinks: int32(codersdk.MaxChatFileIDs),
		FileIds:      []uuid.UUID{row.ID},
	})
	require.NoError(t, err)
	require.Equal(t, int32(0), rejected)
	return row.ID
}

// typeCounts builds a multiset of part types keyed by redacted_type
// where applicable, so tests can assert ordering + redaction exactly.
func typeCounts(parts []codersdk.ChatMessagePartForViewer) []string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p.Type == codersdk.ChatMessagePartTypeRedacted {
			out = append(out, "redacted:"+string(p.RedactedType))
			continue
		}
		out = append(out, string(p.Type))
	}
	return out
}

func TestGetChatMessages_OwnerSeesEverything(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	ownerClient, db := newChatClientWithDatabase(t)
	firstUser := coderdtest.CreateFirstUser(t, ownerClient.Client)
	modelConfig := createChatModelConfig(t, ownerClient)

	chat := createSharedChat(ctx, t, ownerClient, firstUser.OrganizationID, "owner sees everything")
	fileID := insertSharedChatFile(ctx, t, db, firstUser.OrganizationID, firstUser.UserID, chat.ID)
	insertShareTestAssistantMessage(ctx, t, db, chat.ID, modelConfig.ID, fileID)

	resp, err := ownerClient.GetChatMessages(ctx, chat.ID, nil)
	require.NoError(t, err)

	assistant := findAssistantMessage(t, resp.Messages)
	types := make([]string, 0, len(assistant.Content))
	for _, p := range assistant.Content {
		types = append(types, string(p.Type))
	}
	require.Equal(t, []string{"text", "reasoning", "tool-call", "tool-result", "file", "file-reference", "context-file"}, types)

	chatRes, err := ownerClient.GetChat(ctx, chat.ID)
	require.NoError(t, err)
	require.Len(t, chatRes.Files, 1)
}

func TestGetChatMessages_SharedViewer_NothingExtra(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	ownerClient, db := newChatClientWithDatabase(t)
	firstUser := coderdtest.CreateFirstUser(t, ownerClient.Client)
	modelConfig := createChatModelConfig(t, ownerClient)

	viewerRaw, viewer := coderdtest.CreateAnotherUser(t, ownerClient.Client, firstUser.OrganizationID)
	viewerClient := codersdk.NewExperimentalClient(viewerRaw)

	chat := createSharedChat(ctx, t, ownerClient, firstUser.OrganizationID, "shared viewer default")
	fileID := insertSharedChatFile(ctx, t, db, firstUser.OrganizationID, firstUser.UserID, chat.ID)
	insertShareTestAssistantMessage(ctx, t, db, chat.ID, modelConfig.ID, fileID)

	err := ownerClient.UpdateChatACL(ctx, chat.ID, codersdk.UpdateChatACL{
		UserRoles: map[string]codersdk.ChatShareEntry{
			viewer.ID.String(): {Role: codersdk.ChatRoleRead},
		},
	})
	require.NoError(t, err)

	resp, err := viewerClient.GetChatMessagesForViewer(ctx, chat.ID, nil)
	require.NoError(t, err)

	assistant := findAssistantMessageForViewer(t, resp.Messages)
	require.Equal(t,
		[]string{
			"text",
			"reasoning",
			"redacted:tool-call",
			"redacted:tool-result",
			"redacted:file",
			"redacted:file-reference",
			"redacted:context-file",
		},
		typeCounts(assistant.Content),
	)

	res, err := viewerClient.Request(ctx, http.MethodGet, "/api/experimental/chats/"+chat.ID.String(), nil)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)
	var chatView codersdk.ChatForViewer
	require.NoError(t, json.NewDecoder(res.Body).Decode(&chatView))
	require.Empty(t, chatView.Files)
}

func TestGetChatMessages_SharedViewer_ToolsOnly(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	ownerClient, db := newChatClientWithDatabase(t)
	firstUser := coderdtest.CreateFirstUser(t, ownerClient.Client)
	modelConfig := createChatModelConfig(t, ownerClient)

	viewerRaw, viewer := coderdtest.CreateAnotherUser(t, ownerClient.Client, firstUser.OrganizationID)
	viewerClient := codersdk.NewExperimentalClient(viewerRaw)

	chat := createSharedChat(ctx, t, ownerClient, firstUser.OrganizationID, "tools only viewer")
	fileID := insertSharedChatFile(ctx, t, db, firstUser.OrganizationID, firstUser.UserID, chat.ID)
	insertShareTestAssistantMessage(ctx, t, db, chat.ID, modelConfig.ID, fileID)

	err := ownerClient.UpdateChatACL(ctx, chat.ID, codersdk.UpdateChatACL{
		UserRoles: map[string]codersdk.ChatShareEntry{
			viewer.ID.String(): {Role: codersdk.ChatRoleRead, ShareToolCalls: true},
		},
	})
	require.NoError(t, err)

	resp, err := viewerClient.GetChatMessagesForViewer(ctx, chat.ID, nil)
	require.NoError(t, err)

	assistant := findAssistantMessageForViewer(t, resp.Messages)
	require.Equal(t,
		[]string{
			"text",
			"reasoning",
			"tool-call",
			"tool-result",
			"redacted:file",
			"redacted:file-reference",
			"redacted:context-file",
		},
		typeCounts(assistant.Content),
	)

	res, err := viewerClient.Request(ctx, http.MethodGet, "/api/experimental/chats/"+chat.ID.String(), nil)
	require.NoError(t, err)
	defer res.Body.Close()
	var chatView codersdk.ChatForViewer
	require.NoError(t, json.NewDecoder(res.Body).Decode(&chatView))
	require.Empty(t, chatView.Files)
}

func TestGetChatMessages_SharedViewer_AttachmentsOnly(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	ownerClient, db := newChatClientWithDatabase(t)
	firstUser := coderdtest.CreateFirstUser(t, ownerClient.Client)
	modelConfig := createChatModelConfig(t, ownerClient)

	viewerRaw, viewer := coderdtest.CreateAnotherUser(t, ownerClient.Client, firstUser.OrganizationID)
	viewerClient := codersdk.NewExperimentalClient(viewerRaw)

	chat := createSharedChat(ctx, t, ownerClient, firstUser.OrganizationID, "attachments only viewer")
	fileID := insertSharedChatFile(ctx, t, db, firstUser.OrganizationID, firstUser.UserID, chat.ID)
	insertShareTestAssistantMessage(ctx, t, db, chat.ID, modelConfig.ID, fileID)

	err := ownerClient.UpdateChatACL(ctx, chat.ID, codersdk.UpdateChatACL{
		UserRoles: map[string]codersdk.ChatShareEntry{
			viewer.ID.String(): {Role: codersdk.ChatRoleRead, ShareAttachments: true},
		},
	})
	require.NoError(t, err)

	resp, err := viewerClient.GetChatMessagesForViewer(ctx, chat.ID, nil)
	require.NoError(t, err)

	assistant := findAssistantMessageForViewer(t, resp.Messages)
	require.Equal(t,
		[]string{
			"text",
			"reasoning",
			"redacted:tool-call",
			"redacted:tool-result",
			"file",
			"file-reference",
			"context-file",
		},
		typeCounts(assistant.Content),
	)

	res, err := viewerClient.Request(ctx, http.MethodGet, "/api/experimental/chats/"+chat.ID.String(), nil)
	require.NoError(t, err)
	defer res.Body.Close()
	var chatView codersdk.ChatForViewer
	require.NoError(t, json.NewDecoder(res.Body).Decode(&chatView))
	require.Len(t, chatView.Files, 1)
}

func TestGetChatMessages_GroupEntryFlags(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	ownerClient, db := newChatClientWithDatabase(t)
	firstUser := coderdtest.CreateFirstUser(t, ownerClient.Client)
	modelConfig := createChatModelConfig(t, ownerClient)

	viewerRaw, viewer := coderdtest.CreateAnotherUser(t, ownerClient.Client, firstUser.OrganizationID)
	viewerClient := codersdk.NewExperimentalClient(viewerRaw)

	group := dbgen.Group(t, db, database.Group{OrganizationID: firstUser.OrganizationID})
	dbgen.GroupMember(t, db, database.GroupMemberTable{GroupID: group.ID, UserID: viewer.ID})

	chat := createSharedChat(ctx, t, ownerClient, firstUser.OrganizationID, "group entry flags")
	fileID := insertSharedChatFile(ctx, t, db, firstUser.OrganizationID, firstUser.UserID, chat.ID)
	insertShareTestAssistantMessage(ctx, t, db, chat.ID, modelConfig.ID, fileID)

	err := ownerClient.UpdateChatACL(ctx, chat.ID, codersdk.UpdateChatACL{
		GroupRoles: map[string]codersdk.ChatShareEntry{
			group.ID.String(): {Role: codersdk.ChatRoleRead, ShareToolCalls: true},
		},
	})
	require.NoError(t, err)

	resp, err := viewerClient.GetChatMessagesForViewer(ctx, chat.ID, nil)
	require.NoError(t, err)

	assistant := findAssistantMessageForViewer(t, resp.Messages)
	require.Equal(t,
		[]string{
			"text",
			"reasoning",
			"tool-call",
			"tool-result",
			"redacted:file",
			"redacted:file-reference",
			"redacted:context-file",
		},
		typeCounts(assistant.Content),
	)
}

func TestGetChatMessages_UnionAcrossEntries(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	ownerClient, db := newChatClientWithDatabase(t)
	firstUser := coderdtest.CreateFirstUser(t, ownerClient.Client)
	modelConfig := createChatModelConfig(t, ownerClient)

	viewerRaw, viewer := coderdtest.CreateAnotherUser(t, ownerClient.Client, firstUser.OrganizationID)
	viewerClient := codersdk.NewExperimentalClient(viewerRaw)

	group := dbgen.Group(t, db, database.Group{OrganizationID: firstUser.OrganizationID})
	dbgen.GroupMember(t, db, database.GroupMemberTable{GroupID: group.ID, UserID: viewer.ID})

	chat := createSharedChat(ctx, t, ownerClient, firstUser.OrganizationID, "union across entries")
	fileID := insertSharedChatFile(ctx, t, db, firstUser.OrganizationID, firstUser.UserID, chat.ID)
	insertShareTestAssistantMessage(ctx, t, db, chat.ID, modelConfig.ID, fileID)

	err := ownerClient.UpdateChatACL(ctx, chat.ID, codersdk.UpdateChatACL{
		UserRoles: map[string]codersdk.ChatShareEntry{
			viewer.ID.String(): {Role: codersdk.ChatRoleRead},
		},
		GroupRoles: map[string]codersdk.ChatShareEntry{
			group.ID.String(): {Role: codersdk.ChatRoleRead, ShareToolCalls: true},
		},
	})
	require.NoError(t, err)

	resp, err := viewerClient.GetChatMessagesForViewer(ctx, chat.ID, nil)
	require.NoError(t, err)

	assistant := findAssistantMessageForViewer(t, resp.Messages)
	require.Equal(t,
		[]string{
			"text",
			"reasoning",
			"tool-call",
			"tool-result",
			"redacted:file",
			"redacted:file-reference",
			"redacted:context-file",
		},
		typeCounts(assistant.Content),
	)
}

func TestStreamChat_SharedViewerFiltersToolParts(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	ownerClient, db := newChatClientWithDatabase(t)
	firstUser := coderdtest.CreateFirstUser(t, ownerClient.Client)
	modelConfig := createChatModelConfig(t, ownerClient)

	viewerRaw, viewer := coderdtest.CreateAnotherUser(t, ownerClient.Client, firstUser.OrganizationID)
	viewerClient := codersdk.NewExperimentalClient(viewerRaw)

	chat := createSharedChat(ctx, t, ownerClient, firstUser.OrganizationID, "stream viewer filter")
	insertShareTestAssistantMessage(ctx, t, db, chat.ID, modelConfig.ID, uuid.New())

	err := ownerClient.UpdateChatACL(ctx, chat.ID, codersdk.UpdateChatACL{
		UserRoles: map[string]codersdk.ChatShareEntry{
			viewer.ID.String(): {Role: codersdk.ChatRoleRead},
		},
	})
	require.NoError(t, err)

	events, closer, err := viewerClient.StreamChatForViewer(ctx, chat.ID, nil)
	require.NoError(t, err)
	defer closer.Close()

	foundRedactedToolPart := false
	for !foundRedactedToolPart {
		select {
		case <-ctx.Done():
			require.FailNow(t, "timed out waiting for redacted tool part on viewer stream")
		case event, ok := <-events:
			require.True(t, ok, "viewer stream closed before expected event")
			require.NotEqual(t, codersdk.ChatStreamEventTypeError, event.Type)

			if event.Type == codersdk.ChatStreamEventTypeMessage &&
				event.Message != nil &&
				event.Message.Role == codersdk.ChatMessageRoleAssistant {
				for _, p := range event.Message.Content {
					require.NotEqual(t, codersdk.ChatMessagePartTypeToolCall, p.Type,
						"viewer should never see an un-redacted tool-call part")
					require.NotEqual(t, codersdk.ChatMessagePartTypeToolResult, p.Type,
						"viewer should never see an un-redacted tool-result part")
					if p.Type == codersdk.ChatMessagePartTypeRedacted &&
						(p.RedactedType == codersdk.ChatMessagePartTypeToolCall ||
							p.RedactedType == codersdk.ChatMessagePartTypeToolResult) {
						foundRedactedToolPart = true
					}
				}
			}
		}
	}
}

func findAssistantMessage(t *testing.T, msgs []codersdk.ChatMessage) codersdk.ChatMessage {
	t.Helper()
	for _, m := range msgs {
		if m.Role == codersdk.ChatMessageRoleAssistant {
			return m
		}
	}
	require.FailNow(t, "no assistant message found")
	return codersdk.ChatMessage{}
}

func findAssistantMessageForViewer(t *testing.T, msgs []codersdk.ChatMessageForViewer) codersdk.ChatMessageForViewer {
	t.Helper()
	for _, m := range msgs {
		if m.Role == codersdk.ChatMessageRoleAssistant {
			return m
		}
	}
	require.FailNow(t, "no assistant message found")
	return codersdk.ChatMessageForViewer{}
}

func chatIDSet(chats []codersdk.Chat) map[uuid.UUID]struct{} {
	ids := make(map[uuid.UUID]struct{}, len(chats))
	for _, c := range chats {
		ids[c.ID] = struct{}{}
	}
	return ids
}

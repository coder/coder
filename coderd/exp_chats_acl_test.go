package coderd_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

var (
	allAssistantParts              = []string{"text", "reasoning", "tool-call", "tool-result", "file", "file-reference", "context-file"}
	textOnlyAssistantParts         = []string{"text", "reasoning"}
	toolAssistantParts             = []string{"text", "reasoning", "tool-call", "tool-result"}
	attachmentAssistantParts       = []string{"text", "reasoning", "file", "file-reference", "context-file"}
	toolLastInjectedContext        = []string{"tool-call"}
	fileLastInjectedContext        = []string{"file"}
	toolAndFileLastInjectedContext = []string{"tool-call", "file"}
)

type chatACLTestEnv struct {
	ctx           context.Context
	ownerClient   *codersdk.ExperimentalClient
	viewerClient  *codersdk.ExperimentalClient
	db            database.Store
	orgID         uuid.UUID
	ownerID       uuid.UUID
	viewerID      uuid.UUID
	groupID       uuid.UUID
	modelConfigID uuid.UUID
}

type chatACLOperationCase struct {
	name   string
	newEnv func(*testing.T) chatACLTestEnv
	act    func(*testing.T, chatACLTestEnv, uuid.UUID)
	assert func(*testing.T, chatACLTestEnv, uuid.UUID)
}

type chatFileAssertion func(*testing.T, chatACLTestEnv, uuid.UUID)

type chatVisibilityCase struct {
	name                    string
	newEnv                  func(*testing.T) chatACLTestEnv
	viewerShare             *codersdk.ChatShareEntry
	groupShare              *codersdk.ChatShareEntry
	asOwner                 bool
	wantMessageParts        []string
	wantFiles               int
	wantLastInjectedContext []string
	verifyViewerFile        chatFileAssertion
}

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
		Content: []codersdk.ChatInputPart{{
			Type: codersdk.ChatInputPartTypeText,
			Text: title,
		}},
	})
	require.NoError(t, err)
	return chat
}

func newChatACLTestEnv(t *testing.T) chatACLTestEnv {
	t.Helper()

	ctx := testutil.Context(t, testutil.WaitLong)
	ownerClient, db := newChatClientWithDatabase(t)
	firstUser := coderdtest.CreateFirstUser(t, ownerClient.Client)
	modelConfig := createChatModelConfig(t, ownerClient)
	viewerRaw, viewer := coderdtest.CreateAnotherUser(t, ownerClient.Client, firstUser.OrganizationID)

	return chatACLTestEnv{
		ctx:           ctx,
		ownerClient:   ownerClient,
		viewerClient:  codersdk.NewExperimentalClient(viewerRaw),
		db:            db,
		orgID:         firstUser.OrganizationID,
		ownerID:       firstUser.UserID,
		viewerID:      viewer.ID,
		modelConfigID: modelConfig.ID,
	}
}

func newGroupedChatACLTestEnv(t *testing.T) chatACLTestEnv {
	t.Helper()

	env := newChatACLTestEnv(t)
	group := dbgen.Group(t, env.db, database.Group{OrganizationID: env.orgID})
	dbgen.GroupMember(t, env.db, database.GroupMemberTable{GroupID: group.ID, UserID: env.viewerID})
	env.groupID = group.ID
	return env
}

func createSharedChatContent(t *testing.T, env chatACLTestEnv, title string) (codersdk.Chat, uuid.UUID) {
	t.Helper()

	chat := createSharedChat(env.ctx, t, env.ownerClient, env.orgID, title)
	fileID := insertSharedChatFile(env.ctx, t, env.db, env.orgID, env.ownerID, chat.ID)
	insertShareTestAssistantMessage(env.ctx, t, env.db, chat.ID, env.modelConfigID, fileID)
	setViewerLastInjectedContext(env.ctx, t, env.db, chat.ID)
	return chat, fileID
}

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

func insertSharedChatFile(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	orgID, ownerID, chatID uuid.UUID,
) uuid.UUID {
	t.Helper()

	//nolint:gocritic
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

func partTypes(parts []codersdk.ChatMessagePart) []string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		out = append(out, string(p.Type))
	}
	return out
}

func setViewerLastInjectedContext(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	chatID uuid.UUID,
) {
	t.Helper()

	parts := []codersdk.ChatMessagePart{
		codersdk.ChatMessageToolCall("call_abc", "demo_tool", json.RawMessage(`{"arg":"value"}`)),
		{
			Type:      codersdk.ChatMessagePartTypeFile,
			FileID:    uuid.NullUUID{UUID: uuid.New(), Valid: true},
			MediaType: "text/plain",
		},
	}
	raw, err := json.Marshal(parts)
	require.NoError(t, err)

	_, err = db.UpdateChatLastInjectedContext(dbauthz.AsSystemRestricted(ctx), database.UpdateChatLastInjectedContextParams{
		ID: chatID,
		LastInjectedContext: pqtype.NullRawMessage{
			RawMessage: raw,
			Valid:      true,
		},
	})
	require.NoError(t, err)
}

func insertSubChat(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	orgID, ownerID, modelConfigID, parentID uuid.UUID,
	title string,
) database.Chat {
	t.Helper()

	subChat, err := db.InsertChat(dbauthz.AsSystemRestricted(ctx), database.InsertChatParams{
		OrganizationID:    orgID,
		Status:            database.ChatStatusWaiting,
		ClientType:        database.ChatClientTypeUi,
		OwnerID:           ownerID,
		LastModelConfigID: modelConfigID,
		Title:             title,
		ParentChatID:      uuid.NullUUID{UUID: parentID, Valid: true},
		RootChatID:        uuid.NullUUID{UUID: parentID, Valid: true},
	})
	require.NoError(t, err)
	return subChat
}

func share(entry codersdk.ChatShareEntry) *codersdk.ChatShareEntry {
	return &entry
}

func applyChatShare(
	t *testing.T,
	env chatACLTestEnv,
	chatID uuid.UUID,
	viewerShare, groupShare *codersdk.ChatShareEntry,
) {
	t.Helper()

	update := codersdk.UpdateChatACL{}
	if viewerShare != nil {
		update.UserRoles = map[string]codersdk.ChatShareEntry{env.viewerID.String(): *viewerShare}
	}
	if groupShare != nil {
		update.GroupRoles = map[string]codersdk.ChatShareEntry{env.groupID.String(): *groupShare}
	}
	require.NoError(t, env.ownerClient.UpdateChatACL(env.ctx, chatID, update))
}

func mustChatACL(
	ctx context.Context,
	t *testing.T,
	client *codersdk.ExperimentalClient,
	chatID uuid.UUID,
) codersdk.ChatACL {
	t.Helper()

	acl, err := client.ChatACL(ctx, chatID)
	require.NoError(t, err)
	return acl
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

func requireSDKErrorOneOf(t *testing.T, err error, statuses ...int) *codersdk.Error {
	t.Helper()
	require.Error(t, err)
	var sdkErr *codersdk.Error
	require.ErrorAs(t, err, &sdkErr)
	require.Contains(t, statuses, sdkErr.StatusCode())
	return sdkErr
}

func denyViewerFile(t *testing.T, env chatACLTestEnv, fileID uuid.UUID) {
	t.Helper()
	_, _, err := env.viewerClient.GetChatFile(env.ctx, fileID)
	requireSDKError(t, err, http.StatusNotFound)
}

func allowViewerFile(t *testing.T, env chatACLTestEnv, fileID uuid.UUID) {
	t.Helper()
	got, contentType, err := env.viewerClient.GetChatFile(env.ctx, fileID)
	require.NoError(t, err)
	require.Equal(t, "text/markdown", contentType)
	require.Equal(t, []byte("# Shared"), got)
}

func assertSubChatACLUpdateRejected(t *testing.T, env chatACLTestEnv, chatID uuid.UUID) {
	t.Helper()

	err := env.ownerClient.UpdateChatACL(env.ctx, chatID, codersdk.UpdateChatACL{
		UserRoles: map[string]codersdk.ChatShareEntry{
			env.viewerID.String(): {Role: codersdk.ChatRoleRead},
		},
	})
	sdkErr := requireSDKError(t, err, http.StatusBadRequest)
	require.Contains(t, sdkErr.Message, "root chats")
}

func runChatVisibilityCase(t *testing.T, tc chatVisibilityCase) {
	t.Helper()

	newEnv := tc.newEnv
	if newEnv == nil {
		newEnv = newChatACLTestEnv
	}
	env := newEnv(t)
	chat, fileID := createSharedChatContent(t, env, tc.name)
	if tc.viewerShare != nil || tc.groupShare != nil {
		applyChatShare(t, env, chat.ID, tc.viewerShare, tc.groupShare)
	}

	client := env.viewerClient
	if tc.asOwner {
		client = env.ownerClient
	}

	resp, err := client.GetChatMessages(env.ctx, chat.ID, nil)
	require.NoError(t, err)
	require.Equal(t, tc.wantMessageParts, partTypes(findAssistantMessage(t, resp.Messages).Content))

	chatView, err := client.GetChat(env.ctx, chat.ID)
	require.NoError(t, err)
	require.Len(t, chatView.Files, tc.wantFiles)
	if tc.wantLastInjectedContext != nil {
		require.Equal(t, tc.wantLastInjectedContext, partTypes(chatView.LastInjectedContext))
	}
	if tc.verifyViewerFile != nil {
		tc.verifyViewerFile(t, env, fileID)
	}
}

func TestPatchChatACL_Operations(t *testing.T) {
	t.Parallel()

	cases := []chatACLOperationCase{
		{
			name:   "AddsUserAndGroup",
			newEnv: newGroupedChatACLTestEnv,
			act: func(t *testing.T, env chatACLTestEnv, chatID uuid.UUID) {
				t.Helper()
				applyChatShare(t, env, chatID,
					share(codersdk.ChatShareEntry{Role: codersdk.ChatRoleRead}),
					share(codersdk.ChatShareEntry{Role: codersdk.ChatRoleRead}),
				)
			},
			assert: func(t *testing.T, env chatACLTestEnv, chatID uuid.UUID) {
				t.Helper()
				acl := mustChatACL(env.ctx, t, env.ownerClient, chatID)
				require.Len(t, acl.Users, 1)
				require.Equal(t, env.viewerID, acl.Users[0].ID)
				require.Equal(t, codersdk.ChatRoleRead, acl.Users[0].Role)
				require.Len(t, acl.Groups, 1)
				require.Equal(t, env.groupID, acl.Groups[0].ID)
				require.Equal(t, codersdk.ChatRoleRead, acl.Groups[0].Role)
			},
		},
		{
			name:   "StoresShareFlags",
			newEnv: newGroupedChatACLTestEnv,
			act: func(t *testing.T, env chatACLTestEnv, chatID uuid.UUID) {
				t.Helper()
				applyChatShare(t, env, chatID,
					share(codersdk.ChatShareEntry{Role: codersdk.ChatRoleRead, ShareToolCalls: true}),
					share(codersdk.ChatShareEntry{Role: codersdk.ChatRoleRead, ShareAttachments: true}),
				)
			},
			assert: func(t *testing.T, env chatACLTestEnv, chatID uuid.UUID) {
				t.Helper()
				acl := mustChatACL(env.ctx, t, env.ownerClient, chatID)
				require.Len(t, acl.Users, 1)
				require.Equal(t, env.viewerID, acl.Users[0].ID)
				require.True(t, acl.Users[0].ShareToolCalls)
				require.False(t, acl.Users[0].ShareAttachments)
				require.Len(t, acl.Groups, 1)
				require.Equal(t, env.groupID, acl.Groups[0].ID)
				require.False(t, acl.Groups[0].ShareToolCalls)
				require.True(t, acl.Groups[0].ShareAttachments)
			},
		},
		{
			name:   "DefaultsHideEverything",
			newEnv: newChatACLTestEnv,
			act: func(t *testing.T, env chatACLTestEnv, chatID uuid.UUID) {
				t.Helper()
				applyChatShare(t, env, chatID, share(codersdk.ChatShareEntry{Role: codersdk.ChatRoleRead}), nil)
			},
			assert: func(t *testing.T, env chatACLTestEnv, chatID uuid.UUID) {
				t.Helper()
				acl := mustChatACL(env.ctx, t, env.ownerClient, chatID)
				require.Len(t, acl.Users, 1)
				require.False(t, acl.Users[0].ShareToolCalls)
				require.False(t, acl.Users[0].ShareAttachments)
			},
		},
		{
			name:   "DeleteClearsEntries",
			newEnv: newChatACLTestEnv,
			act: func(t *testing.T, env chatACLTestEnv, chatID uuid.UUID) {
				t.Helper()
				applyChatShare(t, env, chatID, share(codersdk.ChatShareEntry{Role: codersdk.ChatRoleRead}), nil)
				require.NoError(t, env.ownerClient.DeleteChatACL(env.ctx, chatID))
			},
			assert: func(t *testing.T, env chatACLTestEnv, chatID uuid.UUID) {
				t.Helper()
				acl := mustChatACL(env.ctx, t, env.ownerClient, chatID)
				require.Empty(t, acl.Users)
				require.Empty(t, acl.Groups)
			},
		},
		{
			name:   "CannotChangeOwnRole",
			newEnv: newChatACLTestEnv,
			act: func(t *testing.T, env chatACLTestEnv, chatID uuid.UUID) {
				t.Helper()
				err := env.ownerClient.UpdateChatACL(env.ctx, chatID, codersdk.UpdateChatACL{
					UserRoles: map[string]codersdk.ChatShareEntry{
						env.ownerID.String(): {Role: codersdk.ChatRoleRead},
					},
				})
				sdkErr := requireSDKError(t, err, http.StatusBadRequest)
				require.Contains(t, sdkErr.Message, "Cannot change your own chat sharing role")
			},
		},
		{
			name:   "RemovesEntryViaDeletedRole",
			newEnv: newGroupedChatACLTestEnv,
			act: func(t *testing.T, env chatACLTestEnv, chatID uuid.UUID) {
				t.Helper()
				applyChatShare(t, env, chatID,
					share(codersdk.ChatShareEntry{Role: codersdk.ChatRoleRead}),
					share(codersdk.ChatShareEntry{Role: codersdk.ChatRoleRead}),
				)
				acl := mustChatACL(env.ctx, t, env.ownerClient, chatID)
				require.Len(t, acl.Users, 1)
				require.Len(t, acl.Groups, 1)
				applyChatShare(t, env, chatID,
					share(codersdk.ChatShareEntry{Role: codersdk.ChatRoleDeleted}),
					share(codersdk.ChatShareEntry{Role: codersdk.ChatRoleDeleted}),
				)
			},
			assert: func(t *testing.T, env chatACLTestEnv, chatID uuid.UUID) {
				t.Helper()
				acl := mustChatACL(env.ctx, t, env.ownerClient, chatID)
				require.Empty(t, acl.Users, "ChatRoleDeleted must remove the user entry")
				require.Empty(t, acl.Groups, "ChatRoleDeleted must remove the group entry")
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			env := tc.newEnv(t)
			chat := createSharedChat(env.ctx, t, env.ownerClient, env.orgID, tc.name)
			tc.act(t, env, chat.ID)
			if tc.assert != nil {
				tc.assert(t, env, chat.ID)
			}
		})
	}
}

func TestPatchChatACL_RejectsNonReadRole(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		role   codersdk.ChatRole
		reject bool
	}{
		{name: "admin", role: codersdk.ChatRole("admin"), reject: true},
		{name: "UppercaseREAD", role: codersdk.ChatRole("READ"), reject: true},
		{name: "write", role: codersdk.ChatRole("write"), reject: true},
		{name: "PaddedRead", role: codersdk.ChatRole(" read "), reject: true},
		{name: "SpelledDeleted", role: codersdk.ChatRole("deleted"), reject: true},
		{name: "owner", role: codersdk.ChatRole("owner"), reject: true},
		{name: "read", role: codersdk.ChatRoleRead, reject: false},
	}

	ownerClient := newChatClient(t)
	firstUser := coderdtest.CreateFirstUser(t, ownerClient.Client)
	_ = createChatModelConfig(t, ownerClient)
	_, viewer := coderdtest.CreateAnotherUser(t, ownerClient.Client, firstUser.OrganizationID)

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			chat := createSharedChat(ctx, t, ownerClient, firstUser.OrganizationID, tc.name)
			err := ownerClient.UpdateChatACL(ctx, chat.ID, codersdk.UpdateChatACL{
				UserRoles: map[string]codersdk.ChatShareEntry{
					viewer.ID.String(): {Role: tc.role},
				},
			})
			if tc.reject {
				requireSDKError(t, err, http.StatusBadRequest)
				return
			}
			require.NoError(t, err, "%q must be accepted as a valid role", tc.role)
		})
	}
}

func TestPatchChatACL_SubChatRejected(t *testing.T) {
	t.Parallel()

	env := newChatACLTestEnv(t)
	parent := createSharedChat(env.ctx, t, env.ownerClient, env.orgID, t.Name())
	subChat := insertSubChat(env.ctx, t, env.db, env.orgID, env.ownerID, env.modelConfigID, parent.ID, "sub-chat")
	assertSubChatACLUpdateRejected(t, env, subChat.ID)
}

func TestChatACL_Audited(t *testing.T) {
	t.Parallel()

	newAuditEnv := func(t *testing.T) (context.Context, *audit.MockAuditor, *codersdk.ExperimentalClient, uuid.UUID, uuid.UUID, codersdk.Chat) {
		t.Helper()
		ctx := testutil.Context(t, testutil.WaitLong)
		mAudit := audit.NewMock()
		clientRaw := coderdtest.New(t, &coderdtest.Options{
			DeploymentValues: chatDeploymentValues(t),
			Auditor:          mAudit,
		})
		ownerClient := codersdk.NewExperimentalClient(clientRaw)
		firstUser := coderdtest.CreateFirstUser(t, ownerClient.Client)
		_ = createChatModelConfig(t, ownerClient)
		_, viewer := coderdtest.CreateAnotherUser(t, ownerClient.Client, firstUser.OrganizationID)
		chat := createSharedChat(ctx, t, ownerClient, firstUser.OrganizationID, t.Name())
		return ctx, mAudit, ownerClient, firstUser.OrganizationID, viewer.ID, chat
	}
	assertAudit := func(t *testing.T, mAudit *audit.MockAuditor, orgID, chatID uuid.UUID) {
		t.Helper()
		require.True(t, mAudit.Contains(t, database.AuditLog{
			OrganizationID: orgID,
			ResourceType:   database.ResourceTypeChat,
			ResourceID:     chatID,
			ResourceTarget: chatID.String()[:8],
			Action:         database.AuditActionWrite,
			StatusCode:     http.StatusNoContent,
		}))
	}

	t.Run("Patch", func(t *testing.T) {
		t.Parallel()
		ctx, mAudit, ownerClient, orgID, viewerID, chat := newAuditEnv(t)
		mAudit.ResetLogs()
		require.NoError(t, ownerClient.UpdateChatACL(ctx, chat.ID, codersdk.UpdateChatACL{
			UserRoles: map[string]codersdk.ChatShareEntry{
				viewerID.String(): {Role: codersdk.ChatRoleRead},
			},
		}))
		assertAudit(t, mAudit, orgID, chat.ID)
	})

	t.Run("Delete", func(t *testing.T) {
		t.Parallel()
		ctx, mAudit, ownerClient, orgID, viewerID, chat := newAuditEnv(t)
		require.NoError(t, ownerClient.UpdateChatACL(ctx, chat.ID, codersdk.UpdateChatACL{
			UserRoles: map[string]codersdk.ChatShareEntry{
				viewerID.String(): {Role: codersdk.ChatRoleRead},
			},
		}))
		mAudit.ResetLogs()
		require.NoError(t, ownerClient.DeleteChatACL(ctx, chat.ID))
		assertAudit(t, mAudit, orgID, chat.ID)
	})
}

func TestListChats_Sharing(t *testing.T) {
	t.Parallel()

	chatIDs := func(chats []codersdk.Chat) map[uuid.UUID]struct{} {
		ids := make(map[uuid.UUID]struct{}, len(chats))
		for _, chat := range chats {
			ids[chat.ID] = struct{}{}
		}
		return ids
	}

	t.Run("SharedFilter", func(t *testing.T) {
		t.Parallel()
		env := newChatACLTestEnv(t)
		ownedOnly := createSharedChat(env.ctx, t, env.ownerClient, env.orgID, "owned only")
		sharedChat := createSharedChat(env.ctx, t, env.ownerClient, env.orgID, "owned and shared")
		applyChatShare(t, env, sharedChat.ID, share(codersdk.ChatShareEntry{Role: codersdk.ChatRoleRead}), nil)

		defaultList, err := env.viewerClient.ListChats(env.ctx, nil)
		require.NoError(t, err)
		require.Empty(t, defaultList, "default list should only include owned chats")

		includeList, err := env.viewerClient.ListChats(env.ctx, &codersdk.ListChatsOptions{Shared: codersdk.ChatSharedFilterInclude})
		require.NoError(t, err)
		includeIDs := chatIDs(includeList)
		require.Contains(t, includeIDs, sharedChat.ID)
		require.NotContains(t, includeIDs, ownedOnly.ID, "viewer does not own or share the first chat")

		onlyList, err := env.viewerClient.ListChats(env.ctx, &codersdk.ListChatsOptions{Shared: codersdk.ChatSharedFilterOnly})
		require.NoError(t, err)
		onlyIDs := chatIDs(onlyList)
		require.Contains(t, onlyIDs, sharedChat.ID)
		require.Len(t, onlyIDs, 1, "viewer has exactly one shared chat")

		ownerList, err := env.ownerClient.ListChats(env.ctx, nil)
		require.NoError(t, err)
		ownerIDs := chatIDs(ownerList)
		require.Contains(t, ownerIDs, ownedOnly.ID)
		require.Contains(t, ownerIDs, sharedChat.ID)

		res, err := env.viewerClient.Request(env.ctx, http.MethodGet, "/api/experimental/chats?shared=wat", nil)
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusBadRequest, res.StatusCode)
	})

	t.Run("SharedViewer_RedactsLastInjectedContext", func(t *testing.T) {
		t.Parallel()
		env := newChatACLTestEnv(t)
		chat := createSharedChat(env.ctx, t, env.ownerClient, env.orgID, t.Name())
		setViewerLastInjectedContext(env.ctx, t, env.db, chat.ID)

		child := insertSubChat(env.ctx, t, env.db, env.orgID, env.ownerID, env.modelConfigID, chat.ID, "shared list child")
		childFileID := insertSharedChatFile(env.ctx, t, env.db, env.orgID, env.ownerID, child.ID)
		insertShareTestAssistantMessage(env.ctx, t, env.db, child.ID, env.modelConfigID, childFileID)
		setViewerLastInjectedContext(env.ctx, t, env.db, child.ID)
		applyChatShare(t, env, chat.ID, share(codersdk.ChatShareEntry{Role: codersdk.ChatRoleRead}), nil)

		list, err := env.viewerClient.ListChats(env.ctx, &codersdk.ListChatsOptions{Shared: codersdk.ChatSharedFilterOnly})
		require.NoError(t, err)
		require.Len(t, list, 1)
		require.Equal(t, chat.ID, list[0].ID)
		require.Empty(t, list[0].Files)
		require.Empty(t, list[0].LastInjectedContext)
		require.Len(t, list[0].Children, 1)
		require.Equal(t, child.ID, list[0].Children[0].ID)
		require.Empty(t, list[0].Children[0].Files)
		require.Empty(t, list[0].Children[0].LastInjectedContext)

		chatView, err := env.viewerClient.GetChat(env.ctx, chat.ID)
		require.NoError(t, err)
		require.Equal(t, chatView.Files, list[0].Files)
		require.Equal(t, chatView.LastInjectedContext, list[0].LastInjectedContext)
		require.Len(t, chatView.Children, 1)
		require.Equal(t, child.ID, chatView.Children[0].ID)
		require.Equal(t, chatView.Children, list[0].Children)
	})
}

func TestGetChatMessages_Sharing(t *testing.T) {
	t.Parallel()

	cases := []chatVisibilityCase{
		{
			name:                    "OwnerSeesEverything",
			asOwner:                 true,
			wantMessageParts:        allAssistantParts,
			wantFiles:               1,
			wantLastInjectedContext: toolAndFileLastInjectedContext,
		},
		{
			name:                    "SharedViewer_NothingExtra",
			viewerShare:             share(codersdk.ChatShareEntry{Role: codersdk.ChatRoleRead}),
			wantMessageParts:        textOnlyAssistantParts,
			wantFiles:               0,
			wantLastInjectedContext: []string{},
			verifyViewerFile:        denyViewerFile,
		},
		{
			name:                    "SharedViewer_ToolsOnly",
			viewerShare:             share(codersdk.ChatShareEntry{Role: codersdk.ChatRoleRead, ShareToolCalls: true}),
			wantMessageParts:        toolAssistantParts,
			wantFiles:               0,
			wantLastInjectedContext: toolLastInjectedContext,
		},
		{
			name:                    "SharedViewer_AttachmentsOnly",
			viewerShare:             share(codersdk.ChatShareEntry{Role: codersdk.ChatRoleRead, ShareAttachments: true}),
			wantMessageParts:        attachmentAssistantParts,
			wantFiles:               1,
			wantLastInjectedContext: fileLastInjectedContext,
			verifyViewerFile:        allowViewerFile,
		},
		{
			name:                    "GroupEntryFlags",
			newEnv:                  newGroupedChatACLTestEnv,
			groupShare:              share(codersdk.ChatShareEntry{Role: codersdk.ChatRoleRead, ShareToolCalls: true}),
			wantMessageParts:        toolAssistantParts,
			wantFiles:               0,
			wantLastInjectedContext: toolLastInjectedContext,
		},
		{
			name:                    "UnionAcrossEntries",
			newEnv:                  newGroupedChatACLTestEnv,
			viewerShare:             share(codersdk.ChatShareEntry{Role: codersdk.ChatRoleRead, ShareAttachments: true}),
			groupShare:              share(codersdk.ChatShareEntry{Role: codersdk.ChatRoleRead, ShareToolCalls: true}),
			wantMessageParts:        allAssistantParts,
			wantFiles:               1,
			wantLastInjectedContext: toolAndFileLastInjectedContext,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runChatVisibilityCase(t, tc)
		})
	}
}

func TestStreamChat_SharedViewerFiltersToolParts(t *testing.T) {
	t.Parallel()

	env := newChatACLTestEnv(t)
	chat := createSharedChat(env.ctx, t, env.ownerClient, env.orgID, t.Name())
	insertShareTestAssistantMessage(env.ctx, t, env.db, chat.ID, env.modelConfigID, uuid.New())
	applyChatShare(t, env, chat.ID, share(codersdk.ChatShareEntry{Role: codersdk.ChatRoleRead}), nil)

	events, closer, err := env.viewerClient.StreamChat(env.ctx, chat.ID, nil)
	require.NoError(t, err)
	defer closer.Close()

	for {
		select {
		case <-env.ctx.Done():
			t.Fatal("timed out waiting for assistant stream event")
		case event, ok := <-events:
			require.True(t, ok, "viewer stream closed before expected event")
			require.NotEqual(t, codersdk.ChatStreamEventTypeError, event.Type)
			if event.Type != codersdk.ChatStreamEventTypeMessage || event.Message == nil || event.Message.Role != codersdk.ChatMessageRoleAssistant {
				continue
			}

			types := partTypes(event.Message.Content)
			require.Contains(t, types, string(codersdk.ChatMessagePartTypeText))
			require.Contains(t, types, string(codersdk.ChatMessagePartTypeReasoning))
			require.NotContains(t, types, string(codersdk.ChatMessagePartTypeToolCall))
			require.NotContains(t, types, string(codersdk.ChatMessagePartTypeToolResult))
			require.NotContains(t, types, string(codersdk.ChatMessagePartTypeFile))
			require.NotContains(t, types, string(codersdk.ChatMessagePartTypeFileReference))
			require.NotContains(t, types, string(codersdk.ChatMessagePartTypeContextFile))
			return
		}
	}
}

func TestSubChatAccess_ViewerViaRootACL(t *testing.T) {
	t.Parallel()

	env := newChatACLTestEnv(t)
	root := createSharedChat(env.ctx, t, env.ownerClient, env.orgID, t.Name())
	subChat := insertSubChat(env.ctx, t, env.db, env.orgID, env.ownerID, env.modelConfigID, root.ID, "sub-chat inherits root acl")
	insertShareTestAssistantMessage(env.ctx, t, env.db, subChat.ID, env.modelConfigID, uuid.Nil)
	applyChatShare(t, env, root.ID, share(codersdk.ChatShareEntry{Role: codersdk.ChatRoleRead}), nil)

	_, err := env.viewerClient.GetChat(env.ctx, subChat.ID)
	require.NoError(t, err)
	msgs, err := env.viewerClient.GetChatMessages(env.ctx, subChat.ID, nil)
	require.NoError(t, err)
	_ = findAssistantMessage(t, msgs.Messages)
	assertSubChatACLUpdateRejected(t, env, subChat.ID)
}

//nolint:tparallel,paralleltest
func TestChatSharingDisabled(t *testing.T) {
	t.Run("CanAccessWhenEnabled", func(t *testing.T) {
		env := newChatACLTestEnv(t)
		chat := createSharedChat(env.ctx, t, env.ownerClient, env.orgID, t.Name())
		applyChatShare(t, env, chat.ID, share(codersdk.ChatShareEntry{Role: codersdk.ChatRoleRead}), nil)

		_, err := env.viewerClient.GetChat(env.ctx, chat.ID)
		require.NoError(t, err, "shared viewer must reach chat when sharing is enabled")
	})

	t.Run("NoAccessWhenDisabled", func(t *testing.T) {
		t.Cleanup(func() {
			rbac.ReloadBuiltinRoles(nil)
			rbac.SetChatACLDisabled(false)
		})

		ctx := testutil.Context(t, testutil.WaitLong)
		dv := chatDeploymentValues(t)
		dv.DisableChatSharing = true

		rawClient, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{DeploymentValues: dv})
		ownerClient := codersdk.NewExperimentalClient(rawClient)
		firstUser := coderdtest.CreateFirstUser(t, rawClient)
		_ = createChatModelConfig(t, ownerClient)
		viewerRaw, viewer := coderdtest.CreateAnotherUser(t, rawClient, firstUser.OrganizationID)
		viewerClient := codersdk.NewExperimentalClient(viewerRaw)
		chat := createSharedChat(ctx, t, ownerClient, firstUser.OrganizationID, t.Name())

		//nolint:gocritic
		ownerRoles, err := rbac.RoleIdentifiers{rbac.RoleOwner()}.Expand()
		require.NoError(t, err)
		ownerCtx := dbauthz.As(ctx, rbac.Subject{
			ID:    "owner",
			Roles: rbac.Roles(ownerRoles),
			Scope: rbac.ExpandableScope(rbac.ScopeAll),
		})
		require.NoError(t, db.UpdateChatACLByID(ownerCtx, database.UpdateChatACLByIDParams{
			ID: chat.ID,
			UserACL: database.ChatACL{
				viewer.ID.String(): {Permissions: []policy.Action{policy.ActionRead}},
			},
			GroupACL: database.ChatACL{},
		}))

		_, err = viewerClient.GetChat(ctx, chat.ID)
		requireSDKError(t, err, http.StatusNotFound)

		sharedList, err := viewerClient.ListChats(ctx, &codersdk.ListChatsOptions{Shared: codersdk.ChatSharedFilterInclude})
		require.NoError(t, err)
		for _, listed := range sharedList {
			require.NotEqual(t, chat.ID, listed.ID, "shared chat must not appear in list when sharing is disabled")
		}
	})
}

func TestChatACL_MixedCaseOwnerIDIsRejected(t *testing.T) {
	t.Parallel()

	env := newChatACLTestEnv(t)
	chat := createSharedChat(env.ctx, t, env.ownerClient, env.orgID, t.Name())

	upper := strings.ToUpper(env.ownerID.String())
	require.NotEqual(t, env.ownerID.String(), upper, "owner UUID must differ from uppercase form")

	err := env.ownerClient.UpdateChatACL(env.ctx, chat.ID, codersdk.UpdateChatACL{
		UserRoles: map[string]codersdk.ChatShareEntry{
			upper: {Role: codersdk.ChatRoleRead},
		},
	})
	sdkErr := requireSDKError(t, err, http.StatusBadRequest)
	require.Contains(t, sdkErr.Message, "Cannot change your own chat sharing role")

	acl := mustChatACL(env.ctx, t, env.ownerClient, chat.ID)
	require.Empty(t, acl.Users, "owner entry must not have been stored under a non-canonical key")
}

func TestChatACL_PatchConcurrent(t *testing.T) {
	t.Parallel()

	env := newChatACLTestEnv(t)
	chat := createSharedChat(env.ctx, t, env.ownerClient, env.orgID, t.Name())
	viewerA := env.viewerID
	_, viewerBUser := coderdtest.CreateAnotherUser(t, env.ownerClient.Client, env.orgID)
	viewerB := viewerBUser.ID

	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make(chan error, 2)

	patch := func(userID uuid.UUID) {
		defer wg.Done()
		<-start
		results <- env.ownerClient.UpdateChatACL(env.ctx, chat.ID, codersdk.UpdateChatACL{
			UserRoles: map[string]codersdk.ChatShareEntry{
				userID.String(): {Role: codersdk.ChatRoleRead},
			},
		})
	}

	wg.Add(2)
	go patch(viewerA)
	go patch(viewerB)
	close(start)
	wg.Wait()
	close(results)
	for err := range results {
		require.NoError(t, err)
	}

	acl := mustChatACL(env.ctx, t, env.ownerClient, chat.ID)
	userIDs := make(map[uuid.UUID]struct{}, len(acl.Users))
	for _, u := range acl.Users {
		userIDs[u.ID] = struct{}{}
	}
	require.Contains(t, userIDs, viewerA, "first concurrent update must survive")
	require.Contains(t, userIDs, viewerB, "second concurrent update must survive")
}

func TestChatACL_NonOwnerForbidden(t *testing.T) {
	t.Parallel()

	env := newChatACLTestEnv(t)
	strangerRaw, stranger := coderdtest.CreateAnotherUser(t, env.ownerClient.Client, env.orgID)
	strangerClient := codersdk.NewExperimentalClient(strangerRaw)
	chat := createSharedChat(env.ctx, t, env.ownerClient, env.orgID, t.Name())
	applyChatShare(t, env, chat.ID, share(codersdk.ChatShareEntry{Role: codersdk.ChatRoleRead}), nil)

	acl := mustChatACL(env.ctx, t, env.viewerClient, chat.ID)
	require.Len(t, acl.Users, 1)
	require.Equal(t, env.viewerID, acl.Users[0].ID)

	err := env.viewerClient.UpdateChatACL(env.ctx, chat.ID, codersdk.UpdateChatACL{
		UserRoles: map[string]codersdk.ChatShareEntry{
			stranger.ID.String(): {Role: codersdk.ChatRoleRead},
		},
	})
	requireSDKErrorOneOf(t, err, http.StatusForbidden, http.StatusNotFound)

	err = env.viewerClient.DeleteChatACL(env.ctx, chat.ID)
	requireSDKErrorOneOf(t, err, http.StatusForbidden, http.StatusNotFound)

	_, err = strangerClient.ChatACL(env.ctx, chat.ID)
	requireSDKError(t, err, http.StatusNotFound)
}

package coderd_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/chatd"
	"github.com/coder/coder/v2/coderd/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/coderdtest/oidctest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/externalauth"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

func chatDeploymentValues(t testing.TB) *codersdk.DeploymentValues {
	t.Helper()

	values := coderdtest.DeploymentValues(t)
	values.Experiments = []string{string(codersdk.ExperimentAgents)}
	return values
}

func newChatClient(t testing.TB) *codersdk.Client {
	t.Helper()

	return coderdtest.New(t, &coderdtest.Options{
		DeploymentValues: chatDeploymentValues(t),
	})
}

func newChatClientWithDatabase(t testing.TB) (*codersdk.Client, database.Store) {
	t.Helper()

	return coderdtest.NewWithDatabase(t, &coderdtest.Options{
		DeploymentValues: chatDeploymentValues(t),
	})
}

func requireChatUsageLimitExceededError(
	t *testing.T,
	err error,
	wantSpentMicros int64,
	wantLimitMicros int64,
	wantResetsAt time.Time,
) *codersdk.ChatUsageLimitExceededResponse {
	t.Helper()

	sdkErr, ok := codersdk.AsError(err)
	require.True(t, ok)
	require.Equal(t, http.StatusConflict, sdkErr.StatusCode())
	require.Equal(t, "Chat usage limit exceeded.", sdkErr.Message)

	limitErr := codersdk.ChatUsageLimitExceededFrom(err)
	require.NotNil(t, limitErr)
	require.Equal(t, "Chat usage limit exceeded.", limitErr.Message)
	require.Equal(t, wantSpentMicros, limitErr.SpentMicros)
	require.Equal(t, wantLimitMicros, limitErr.LimitMicros)
	require.True(
		t,
		limitErr.ResetsAt.Equal(wantResetsAt),
		"expected resets_at %s, got %s",
		wantResetsAt.UTC().Format(time.RFC3339),
		limitErr.ResetsAt.UTC().Format(time.RFC3339),
	)

	return limitErr
}

func enableDailyChatUsageLimit(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	limitMicros int64,
) time.Time {
	t.Helper()

	_, err := db.UpsertChatUsageLimitConfig(
		dbauthz.AsSystemRestricted(ctx),
		database.UpsertChatUsageLimitConfigParams{
			Enabled:            true,
			DefaultLimitMicros: limitMicros,
			Period:             string(codersdk.ChatUsageLimitPeriodDay),
		},
	)
	require.NoError(t, err)

	_, periodEnd := chatd.ComputeUsagePeriodBounds(time.Now(), codersdk.ChatUsageLimitPeriodDay)
	return periodEnd
}

func insertAssistantCostMessage(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	chatID uuid.UUID,
	modelConfigID uuid.UUID,
	totalCostMicros int64,
) {
	t.Helper()

	assistantContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageText("assistant"),
	})
	require.NoError(t, err)

	_, err = db.InsertChatMessages(dbauthz.AsSystemRestricted(ctx), database.InsertChatMessagesParams{
		ChatID:              chatID,
		CreatedBy:           []uuid.UUID{uuid.Nil},
		ModelConfigID:       []uuid.UUID{modelConfigID},
		Role:                []database.ChatMessageRole{database.ChatMessageRoleAssistant},
		ContentVersion:      []int16{chatprompt.CurrentContentVersion},
		Content:             []string{string(assistantContent.RawMessage)},
		Visibility:          []database.ChatMessageVisibility{database.ChatMessageVisibilityBoth},
		InputTokens:         []int64{0},
		OutputTokens:        []int64{0},
		TotalTokens:         []int64{0},
		ReasoningTokens:     []int64{0},
		CacheCreationTokens: []int64{0},
		CacheReadTokens:     []int64{0},
		ContextLimit:        []int64{0},
		Compressed:          []bool{false},
		TotalCostMicros:     []int64{totalCostMicros},
		RuntimeMs:           []int64{0},
	})
	require.NoError(t, err)
}

func TestPostChats(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		user := coderdtest.CreateFirstUser(t, client)
		modelConfig := createChatModelConfig(t, client)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "hello from chats route tests",
				},
			},
		})
		require.NoError(t, err)

		require.NotEqual(t, uuid.Nil, chat.ID)
		require.Equal(t, user.UserID, chat.OwnerID)
		require.Equal(t, modelConfig.ID, chat.LastModelConfigID)
		require.Equal(t, "hello from chats route tests", chat.Title)
		require.Equal(t, codersdk.ChatStatusPending, chat.Status)
		require.NotZero(t, chat.CreatedAt)
		require.NotZero(t, chat.UpdatedAt)
		require.Nil(t, chat.WorkspaceID)
		require.NotNil(t, chat.RootChatID)
		require.Equal(t, chat.ID, *chat.RootChatID)

		chatResult, err := client.GetChat(ctx, chat.ID)
		require.NoError(t, err)
		messagesResult, err := client.GetChatMessages(ctx, chat.ID, nil)
		require.NoError(t, err)
		require.Equal(t, chat.ID, chatResult.ID)

		foundUserMessage := false
		for _, message := range messagesResult.Messages {
			if message.Role != codersdk.ChatMessageRoleUser {
				continue
			}
			for _, part := range message.Content {
				if part.Type == codersdk.ChatMessagePartTypeText &&
					part.Text == "hello from chats route tests" {
					foundUserMessage = true
					break
				}
			}
		}
		require.True(t, foundUserMessage)
	})

	t.Run("HidesSystemPromptMessages", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client)
		_ = createChatModelConfig(t, client)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "verify hidden system prompt",
				},
			},
		})
		require.NoError(t, err)

		messagesResult, err := client.GetChatMessages(ctx, chat.ID, nil)
		require.NoError(t, err)
		for _, message := range messagesResult.Messages {
			require.NotEqual(t, codersdk.ChatMessageRoleSystem, message.Role)
		}
	})

	t.Run("WorkspaceNotAccessible", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		adminClient, db := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, adminClient)
		memberClient, _ := coderdtest.CreateAnotherUser(t, adminClient, firstUser.OrganizationID)

		workspaceBuild := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: firstUser.OrganizationID,
			OwnerID:        firstUser.UserID,
		}).WithAgent().Do()

		_, err := memberClient.CreateChat(ctx, codersdk.CreateChatRequest{
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "hello",
				},
			},
			WorkspaceID: &workspaceBuild.Workspace.ID,
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(
			t,
			"Workspace not found or you do not have access to this resource",
			sdkErr.Message,
		)
	})

	t.Run("WorkspaceAccessibleButNoSSH", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		adminClient, db := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, adminClient)
		orgAdminClient, _ := coderdtest.CreateAnotherUser(
			t,
			adminClient,
			firstUser.OrganizationID,
			rbac.ScopedRoleOrgAdmin(firstUser.OrganizationID),
		)

		workspaceBuild := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: firstUser.OrganizationID,
			OwnerID:        firstUser.UserID,
		}).WithAgent().Do()

		_, err := orgAdminClient.CreateChat(ctx, codersdk.CreateChatRequest{
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "hello",
				},
			},
			WorkspaceID: &workspaceBuild.Workspace.ID,
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(
			t,
			"Workspace not found or you do not have access to this resource",
			sdkErr.Message,
		)
	})

	t.Run("WorkspaceNotFound", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client)

		workspaceID := uuid.New()
		_, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "hello",
				},
			},
			WorkspaceID: &workspaceID,
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(
			t,
			"Workspace not found or you do not have access to this resource",
			sdkErr.Message,
		)
	})

	t.Run("WorkspaceSelectsFirstAgent", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		user := coderdtest.CreateFirstUser(t, client)
		modelConfig := createChatModelConfig(t, client)

		workspaceBuild := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: user.OrganizationID,
			OwnerID:        user.UserID,
		}).WithAgent().Do()

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "hello",
				},
			},
			WorkspaceID: &workspaceBuild.Workspace.ID,
		})
		require.NoError(t, err)
		require.NotNil(t, chat.WorkspaceID)
		require.Equal(t, workspaceBuild.Workspace.ID, *chat.WorkspaceID)
		require.Equal(t, modelConfig.ID, chat.LastModelConfigID)
	})

	t.Run("MissingDefaultModelConfig", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client)

		_, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "hello",
				},
			},
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "No default chat model config is configured.", sdkErr.Message)
	})

	t.Run("EmptyContent", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client)

		_, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			Content: nil,
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Content is required.", sdkErr.Message)
		require.Equal(t, "Content cannot be empty.", sdkErr.Detail)
	})

	t.Run("EmptyText", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client)

		_, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "   ",
				},
			},
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Invalid input part.", sdkErr.Message)
		require.Equal(t, "content[0].text cannot be empty.", sdkErr.Detail)
	})

	t.Run("UnsupportedPartType", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client)

		_, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartType("image"),
					Text: "hello",
				},
			},
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Invalid input part.", sdkErr.Message)
		require.Equal(t, `content[0].type "image" is not supported.`, sdkErr.Detail)
	})

	t.Run("UsageLimitExceeded", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		user := coderdtest.CreateFirstUser(t, client)
		modelConfig := createChatModelConfig(t, client)
		wantResetsAt := enableDailyChatUsageLimit(ctx, t, db, 100)

		existingChat, err := db.InsertChat(dbauthz.AsSystemRestricted(ctx), database.InsertChatParams{
			OwnerID:           user.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "existing-limit-chat",
		})
		require.NoError(t, err)

		insertAssistantCostMessage(ctx, t, db, existingChat.ID, modelConfig.ID, 100)

		_, err = client.CreateChat(ctx, codersdk.CreateChatRequest{
			Content: []codersdk.ChatInputPart{{
				Type: codersdk.ChatInputPartTypeText,
				Text: "over limit",
			}},
		})
		requireChatUsageLimitExceededError(t, err, 100, 100, wantResetsAt)
	})
}

func TestListChats(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, client)
		modelConfig := createChatModelConfig(t, client)

		firstChatA, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "first owner chat",
				},
			},
		})
		require.NoError(t, err)

		firstChatB, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "second owner chat",
				},
			},
		})
		require.NoError(t, err)

		memberClient, member := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)
		memberDBChat, err := db.InsertChat(dbauthz.AsSystemRestricted(ctx), database.InsertChatParams{
			OwnerID:           member.ID,
			LastModelConfigID: modelConfig.ID,
			Title:             "member chat only",
		})
		require.NoError(t, err)

		chats, err := client.ListChats(ctx, nil)
		require.NoError(t, err)
		require.Len(t, chats, 2)

		chatIndexes := make(map[uuid.UUID]int, len(chats))
		chatsByID := make(map[uuid.UUID]codersdk.Chat, len(chats))
		for i, chat := range chats {
			chatIndexes[chat.ID] = i
			chatsByID[chat.ID] = chat

			require.Equal(t, firstUser.UserID, chat.OwnerID)
			require.Equal(t, modelConfig.ID, chat.LastModelConfigID)
			require.Equal(t, codersdk.ChatStatusPending, chat.Status)
			require.NotZero(t, chat.CreatedAt)
			require.NotZero(t, chat.UpdatedAt)
			require.Nil(t, chat.ParentChatID)
			require.Nil(t, chat.WorkspaceID)
			require.NotNil(t, chat.RootChatID)
			require.Equal(t, chat.ID, *chat.RootChatID)
			require.NotNil(t, chat.DiffStatus)
			require.Equal(t, chat.ID, chat.DiffStatus.ChatID)
		}

		require.Contains(t, chatsByID, firstChatA.ID)
		require.Contains(t, chatsByID, firstChatB.ID)
		require.NotContains(t, chatsByID, memberDBChat.ID)
		require.Equal(t, "first owner chat", chatsByID[firstChatA.ID].Title)
		require.Equal(t, "second owner chat", chatsByID[firstChatB.ID].Title)

		for i := 1; i < len(chats); i++ {
			require.False(t, chats[i-1].UpdatedAt.Before(chats[i].UpdatedAt))
		}
		if firstChatA.UpdatedAt.After(firstChatB.UpdatedAt) {
			require.Less(t, chatIndexes[firstChatA.ID], chatIndexes[firstChatB.ID])
		}
		if firstChatB.UpdatedAt.After(firstChatA.UpdatedAt) {
			require.Less(t, chatIndexes[firstChatB.ID], chatIndexes[firstChatA.ID])
		}

		memberChats, err := memberClient.ListChats(ctx, nil)
		require.NoError(t, err)
		require.Len(t, memberChats, 1)
		require.Equal(t, memberDBChat.ID, memberChats[0].ID)
		require.Equal(t, member.ID, memberChats[0].OwnerID)
		require.Equal(t, "member chat only", memberChats[0].Title)
		require.NotNil(t, memberChats[0].RootChatID)
		require.Equal(t, memberChats[0].ID, *memberChats[0].RootChatID)
		require.NotNil(t, memberChats[0].DiffStatus)
		require.Equal(t, memberChats[0].ID, memberChats[0].DiffStatus.ChatID)
	})

	t.Run("Unauthenticated", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client)

		unauthenticatedClient := codersdk.New(client.URL)
		_, err := unauthenticatedClient.ListChats(ctx, nil)
		requireSDKError(t, err, http.StatusUnauthorized)
	})

	t.Run("Pagination", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, _ := newChatClientWithDatabase(t)
		_ = coderdtest.CreateFirstUser(t, client)
		_ = createChatModelConfig(t, client)

		// Create 5 chats.
		const totalChats = 5
		createdChats := make([]codersdk.Chat, 0, totalChats)
		for i := 0; i < totalChats; i++ {
			chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
				Content: []codersdk.ChatInputPart{
					{
						Type: codersdk.ChatInputPartTypeText,
						Text: fmt.Sprintf("chat-%d", i),
					},
				},
			})
			require.NoError(t, err)
			createdChats = append(createdChats, chat)
		}

		// Fetch first page with limit=2.
		page1, err := client.ListChats(ctx, &codersdk.ListChatsOptions{
			Pagination: codersdk.Pagination{Limit: 2},
		})
		require.NoError(t, err)
		require.Len(t, page1, 2)

		// Fetch second page using after_id from last item of page 1.
		page2, err := client.ListChats(ctx, &codersdk.ListChatsOptions{
			Pagination: codersdk.Pagination{
				AfterID: uuid.MustParse(page1[len(page1)-1].ID.String()),
				Limit:   2,
			},
		})
		require.NoError(t, err)
		require.Len(t, page2, 2)

		// Ensure page1 and page2 have no overlap.
		page1IDs := make(map[uuid.UUID]struct{})
		for _, c := range page1 {
			page1IDs[c.ID] = struct{}{}
		}
		for _, c := range page2 {
			_, overlap := page1IDs[c.ID]
			require.False(t, overlap, "page2 should not contain items from page1")
		}

		// Fetch third page — should have 1 remaining chat.
		page3, err := client.ListChats(ctx, &codersdk.ListChatsOptions{
			Pagination: codersdk.Pagination{
				AfterID: uuid.MustParse(page2[len(page2)-1].ID.String()),
				Limit:   2,
			},
		})
		require.NoError(t, err)
		require.Len(t, page3, 1)

		// All 5 chats should be accounted for.
		allIDs := make(map[uuid.UUID]struct{})
		for _, c := range append(append(page1, page2...), page3...) {
			allIDs[c.ID] = struct{}{}
		}
		for _, c := range createdChats {
			_, found := allIDs[c.ID]
			require.True(t, found, "chat %s should appear in paginated results", c.ID)
		}

		// Fetch with offset=3, limit=2 — should return 2 chats.
		offsetPage, err := client.ListChats(ctx, &codersdk.ListChatsOptions{
			Pagination: codersdk.Pagination{Offset: 3, Limit: 2},
		})
		require.NoError(t, err)
		require.Len(t, offsetPage, 2)

		// No limit should return all chats.
		allChats, err := client.ListChats(ctx, nil)
		require.NoError(t, err)
		require.Len(t, allChats, totalChats)
	})
}

func TestListChatModels(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client)
		_ = createChatModelConfig(t, client)

		models, err := client.ListChatModels(ctx)
		require.NoError(t, err)

		var openAIProvider *codersdk.ChatModelProvider
		for i := range models.Providers {
			if models.Providers[i].Provider == "openai" {
				openAIProvider = &models.Providers[i]
				break
			}
		}
		require.NotNil(t, openAIProvider)
		require.True(t, openAIProvider.Available)

		foundModel := false
		for _, model := range openAIProvider.Models {
			if model.Provider == "openai" && model.Model == "gpt-4o-mini" {
				foundModel = true
				break
			}
		}
		require.True(t, foundModel)
	})

	t.Run("Unauthenticated", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client)

		unauthenticatedClient := codersdk.New(client.URL)
		_, err := unauthenticatedClient.ListChatModels(ctx)
		requireSDKError(t, err, http.StatusUnauthorized)
	})
}

func TestWatchChats(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client)
		_ = createChatModelConfig(t, client)

		conn, err := client.Dial(ctx, "/api/experimental/chats/watch", nil)
		require.NoError(t, err)
		defer conn.Close(websocket.StatusNormalClosure, "done")

		type watchEvent struct {
			Type codersdk.ServerSentEventType `json:"type"`
			Data json.RawMessage              `json:"data,omitempty"`
		}

		var event watchEvent
		err = wsjson.Read(ctx, conn, &event)
		require.NoError(t, err)
		require.Equal(t, codersdk.ServerSentEventTypePing, event.Type)
		require.True(t, len(event.Data) == 0 || string(event.Data) == "null")

		createdChat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "watch route created event",
				},
			},
		})
		require.NoError(t, err)

		for {
			var update watchEvent
			err = wsjson.Read(ctx, conn, &update)
			require.NoError(t, err)

			if update.Type == codersdk.ServerSentEventTypePing {
				continue
			}
			require.Equal(t, codersdk.ServerSentEventTypeData, update.Type)

			var payload coderdpubsub.ChatEvent
			err = json.Unmarshal(update.Data, &payload)
			require.NoError(t, err)
			if payload.Kind == coderdpubsub.ChatEventKindCreated &&
				payload.Chat.ID == createdChat.ID {
				break
			}
		}
	})

	t.Run("DiffStatusChangeIncludesDiffStatus", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
			DeploymentValues: chatDeploymentValues(t),
		})
		db := api.Database
		user := coderdtest.CreateFirstUser(t, client)
		modelConfig := createChatModelConfig(t, client)

		// Insert a chat and a diff status row.
		chat, err := db.InsertChat(dbauthz.AsSystemRestricted(ctx), database.InsertChatParams{
			OwnerID:           user.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "diff status watch test",
		})
		require.NoError(t, err)

		refreshedAt := time.Now().UTC().Truncate(time.Second)
		staleAt := refreshedAt.Add(time.Hour)
		_, err = db.UpsertChatDiffStatusReference(
			dbauthz.AsSystemRestricted(ctx),
			database.UpsertChatDiffStatusReferenceParams{
				ChatID:          chat.ID,
				Url:             sql.NullString{String: "https://github.com/coder/coder/pull/99", Valid: true},
				GitBranch:       "feature/test",
				GitRemoteOrigin: "git@github.com:coder/coder.git",
				StaleAt:         staleAt,
			},
		)
		require.NoError(t, err)
		_, err = db.UpsertChatDiffStatus(
			dbauthz.AsSystemRestricted(ctx),
			database.UpsertChatDiffStatusParams{
				ChatID:           chat.ID,
				Url:              sql.NullString{String: "https://github.com/coder/coder/pull/99", Valid: true},
				PullRequestState: sql.NullString{String: "open", Valid: true},
				Additions:        42,
				Deletions:        7,
				ChangedFiles:     5,
				RefreshedAt:      refreshedAt,
				StaleAt:          staleAt,
			},
		)
		require.NoError(t, err)

		// Open the watch WebSocket.
		conn, err := client.Dial(ctx, "/api/experimental/chats/watch", nil)
		require.NoError(t, err)
		defer conn.Close(websocket.StatusNormalClosure, "done")

		type watchEvent struct {
			Type codersdk.ServerSentEventType `json:"type"`
			Data json.RawMessage              `json:"data,omitempty"`
		}

		// Read the initial ping.
		var ping watchEvent
		err = wsjson.Read(ctx, conn, &ping)
		require.NoError(t, err)
		require.Equal(t, codersdk.ServerSentEventTypePing, ping.Type)

		// Publish a diff_status_change event via pubsub,
		// mimicking what PublishDiffStatusChange does after
		// it reads the diff status from the DB.
		dbStatus, err := db.GetChatDiffStatusByChatID(dbauthz.AsSystemRestricted(ctx), chat.ID)
		require.NoError(t, err)
		sdkDiffStatus := db2sdk.ChatDiffStatus(chat.ID, &dbStatus)
		event := coderdpubsub.ChatEvent{
			Kind: coderdpubsub.ChatEventKindDiffStatusChange,
			Chat: codersdk.Chat{
				ID:         chat.ID,
				OwnerID:    chat.OwnerID,
				Title:      chat.Title,
				Status:     codersdk.ChatStatus(chat.Status),
				CreatedAt:  chat.CreatedAt,
				UpdatedAt:  chat.UpdatedAt,
				DiffStatus: &sdkDiffStatus,
			},
		}
		payload, err := json.Marshal(event)
		require.NoError(t, err)
		err = api.Pubsub.Publish(coderdpubsub.ChatEventChannel(user.UserID), payload)
		require.NoError(t, err)

		// Read events until we find the diff_status_change.
		for {
			var update watchEvent
			err = wsjson.Read(ctx, conn, &update)
			require.NoError(t, err)

			if update.Type == codersdk.ServerSentEventTypePing {
				continue
			}
			require.Equal(t, codersdk.ServerSentEventTypeData, update.Type)

			var received coderdpubsub.ChatEvent
			err = json.Unmarshal(update.Data, &received)
			require.NoError(t, err)

			if received.Kind != coderdpubsub.ChatEventKindDiffStatusChange ||
				received.Chat.ID != chat.ID {
				continue
			}

			// Verify the event carries the full DiffStatus.
			require.NotNil(t, received.Chat.DiffStatus, "diff_status_change event must include DiffStatus")
			ds := received.Chat.DiffStatus
			require.Equal(t, chat.ID, ds.ChatID)
			require.NotNil(t, ds.URL)
			require.Equal(t, "https://github.com/coder/coder/pull/99", *ds.URL)
			require.NotNil(t, ds.PullRequestState)
			require.Equal(t, "open", *ds.PullRequestState)
			require.EqualValues(t, 42, ds.Additions)
			require.EqualValues(t, 7, ds.Deletions)
			require.EqualValues(t, 5, ds.ChangedFiles)
			break
		}
	})

	t.Run("Unauthenticated", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client)

		unauthenticatedClient := codersdk.New(client.URL)
		res, err := unauthenticatedClient.Request(
			ctx,
			http.MethodGet,
			"/api/experimental/chats/watch",
			nil,
		)
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)
	})
}

func TestListChatProviders(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client)
		_ = createChatModelConfig(t, client)

		providers, err := client.ListChatProviders(ctx)
		require.NoError(t, err)

		var openAIProvider *codersdk.ChatProviderConfig
		for i := range providers {
			if providers[i].Provider == "openai" {
				openAIProvider = &providers[i]
				break
			}
		}
		require.NotNil(t, openAIProvider)
		require.Equal(t, codersdk.ChatProviderConfigSourceDatabase, openAIProvider.Source)
		require.True(t, openAIProvider.Enabled)
		require.True(t, openAIProvider.HasAPIKey)
	})

	t.Run("ForbiddenForOrganizationMember", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		adminClient := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, adminClient)
		memberClient, _ := coderdtest.CreateAnotherUser(t, adminClient, firstUser.OrganizationID)

		_, err := memberClient.ListChatProviders(ctx)
		requireSDKError(t, err, http.StatusForbidden)
	})
}

func TestCreateChatProvider(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client)

		provider, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider:    "openai",
			DisplayName: "OpenAI Primary",
			APIKey:      "test-api-key",
		})
		require.NoError(t, err)
		require.NotEqual(t, uuid.Nil, provider.ID)
		require.Equal(t, "openai", provider.Provider)
		require.Equal(t, "OpenAI Primary", provider.DisplayName)
		require.True(t, provider.Enabled)
		require.True(t, provider.HasAPIKey)
		require.Equal(t, codersdk.ChatProviderConfigSourceDatabase, provider.Source)
	})

	t.Run("InvalidProvider", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client)

		_, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider: "not-a-provider",
			APIKey:   "test-api-key",
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Invalid provider.", sdkErr.Message)
	})

	t.Run("Conflict", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client)

		_, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider: "openai",
			APIKey:   "test-api-key",
		})
		require.NoError(t, err)

		_, err = client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider: "openai",
			APIKey:   "other-api-key",
		})
		sdkErr := requireSDKError(t, err, http.StatusConflict)
		require.Equal(t, "Chat provider already exists.", sdkErr.Message)
	})

	t.Run("ForbiddenForOrganizationMember", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		adminClient := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, adminClient)
		memberClient, _ := coderdtest.CreateAnotherUser(t, adminClient, firstUser.OrganizationID)

		_, err := memberClient.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider: "openai",
			APIKey:   "member-key",
		})
		requireSDKError(t, err, http.StatusForbidden)
	})
}

func TestUpdateChatProvider(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client)

		provider, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider: "openai",
			APIKey:   "test-api-key",
		})
		require.NoError(t, err)

		enabled := false
		baseURL := "https://example.com/v1"
		updated, err := client.UpdateChatProvider(ctx, provider.ID, codersdk.UpdateChatProviderConfigRequest{
			DisplayName: "OpenAI Updated",
			Enabled:     &enabled,
			BaseURL:     &baseURL,
		})
		require.NoError(t, err)
		require.Equal(t, provider.ID, updated.ID)
		require.Equal(t, "OpenAI Updated", updated.DisplayName)
		require.False(t, updated.Enabled)
		require.Equal(t, baseURL, updated.BaseURL)
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client)

		_, err := client.UpdateChatProvider(ctx, uuid.New(), codersdk.UpdateChatProviderConfigRequest{
			DisplayName: "missing",
		})
		requireSDKError(t, err, http.StatusNotFound)
	})

	t.Run("InvalidProviderID", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client)

		res, err := client.Request(
			ctx,
			http.MethodPatch,
			"/api/experimental/chats/providers/not-a-uuid",
			codersdk.UpdateChatProviderConfigRequest{DisplayName: "ignored"},
		)
		require.NoError(t, err)
		defer res.Body.Close()

		err = codersdk.ReadBodyAsError(res)
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Invalid chat provider ID.", sdkErr.Message)
	})

	t.Run("ForbiddenForOrganizationMember", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		adminClient := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, adminClient)
		memberClient, _ := coderdtest.CreateAnotherUser(t, adminClient, firstUser.OrganizationID)

		provider, err := adminClient.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider: "openai",
			APIKey:   "test-api-key",
		})
		require.NoError(t, err)

		_, err = memberClient.UpdateChatProvider(ctx, provider.ID, codersdk.UpdateChatProviderConfigRequest{
			DisplayName: "member update",
		})
		requireSDKError(t, err, http.StatusForbidden)
	})
}

func TestDeleteChatProvider(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client)

		provider, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider: "openai",
			APIKey:   "test-api-key",
		})
		require.NoError(t, err)

		err = client.DeleteChatProvider(ctx, provider.ID)
		require.NoError(t, err)

		providers, err := client.ListChatProviders(ctx)
		require.NoError(t, err)
		for _, listed := range providers {
			require.NotEqual(t, provider.ID, listed.ID)
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client)

		err := client.DeleteChatProvider(ctx, uuid.New())
		requireSDKError(t, err, http.StatusNotFound)
	})

	t.Run("InvalidProviderID", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client)

		res, err := client.Request(
			ctx,
			http.MethodDelete,
			"/api/experimental/chats/providers/not-a-uuid",
			nil,
		)
		require.NoError(t, err)
		defer res.Body.Close()

		err = codersdk.ReadBodyAsError(res)
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Invalid chat provider ID.", sdkErr.Message)
	})

	t.Run("ForbiddenForOrganizationMember", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		adminClient := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, adminClient)
		memberClient, _ := coderdtest.CreateAnotherUser(t, adminClient, firstUser.OrganizationID)

		provider, err := adminClient.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider: "openai",
			APIKey:   "test-api-key",
		})
		require.NoError(t, err)

		err = memberClient.DeleteChatProvider(ctx, provider.ID)
		requireSDKError(t, err, http.StatusForbidden)
	})
}

func TestListChatModelConfigs(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client)
		modelConfig := createChatModelConfig(t, client)

		configs, err := client.ListChatModelConfigs(ctx)
		require.NoError(t, err)
		require.NotEmpty(t, configs)

		found := false
		for _, config := range configs {
			if config.ID == modelConfig.ID {
				found = true
				require.Equal(t, "openai", config.Provider)
				require.Equal(t, "gpt-4o-mini", config.Model)
				require.True(t, config.IsDefault)
			}
		}
		require.True(t, found)
	})

	t.Run("DeserializesLegacyPricingJSON", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, client)

		_, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider: "openai",
			APIKey:   "test-api-key",
		})
		require.NoError(t, err)

		legacyOptions := json.RawMessage(`{"input_price_per_million_tokens":0.15,"output_price_per_million_tokens":0.6,"cache_read_price_per_million_tokens":0.03,"cache_write_price_per_million_tokens":0.3}`)
		storedConfig, err := db.InsertChatModelConfig(dbauthz.AsSystemRestricted(ctx), database.InsertChatModelConfigParams{
			Provider:             "openai",
			Model:                "gpt-4o-mini-legacy",
			DisplayName:          "GPT-4o Mini Legacy",
			CreatedBy:            uuid.NullUUID{UUID: firstUser.UserID, Valid: true},
			UpdatedBy:            uuid.NullUUID{UUID: firstUser.UserID, Valid: true},
			Enabled:              true,
			IsDefault:            false,
			ContextLimit:         4096,
			CompressionThreshold: 80,
			Options:              legacyOptions,
		})
		require.NoError(t, err)

		configs, err := client.ListChatModelConfigs(ctx)
		require.NoError(t, err)
		require.Len(t, configs, 1)
		require.Equal(t, storedConfig.ID, configs[0].ID)
		requireChatModelPricing(t, configs[0].ModelConfig, &codersdk.ChatModelCallConfig{
			Cost: &codersdk.ModelCostConfig{
				InputPricePerMillionTokens:      decRef("0.15"),
				OutputPricePerMillionTokens:     decRef("0.6"),
				CacheReadPricePerMillionTokens:  decRef("0.03"),
				CacheWritePricePerMillionTokens: decRef("0.3"),
			},
		})
	})

	t.Run("SuccessForOrganizationMember", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		adminClient := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, adminClient)
		modelConfig := createChatModelConfig(t, adminClient)
		memberClient, _ := coderdtest.CreateAnotherUser(t, adminClient, firstUser.OrganizationID)

		// Non-admin users should see only enabled model configs.
		configs, err := memberClient.ListChatModelConfigs(ctx)
		require.NoError(t, err)
		require.NotEmpty(t, configs)

		found := false
		for _, config := range configs {
			if config.ID == modelConfig.ID {
				found = true
				require.Equal(t, "openai", config.Provider)
				require.Equal(t, "gpt-4o-mini", config.Model)
			}
		}
		require.True(t, found)
	})
}

func TestCreateChatModelConfig(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client)

		_, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider: "openai",
			APIKey:   "test-api-key",
		})
		require.NoError(t, err)

		contextLimit := int64(4096)
		isDefault := true
		pricing := &codersdk.ChatModelCallConfig{
			Cost: &codersdk.ModelCostConfig{
				InputPricePerMillionTokens:      decRef("0.15"),
				OutputPricePerMillionTokens:     decRef("0.6"),
				CacheReadPricePerMillionTokens:  decRef("0.03"),
				CacheWritePricePerMillionTokens: decRef("0.3"),
			},
		}
		modelConfig, err := client.CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
			Provider:     "openai",
			Model:        "gpt-4o-mini",
			ContextLimit: &contextLimit,
			IsDefault:    &isDefault,
			ModelConfig:  pricing,
		})
		require.NoError(t, err)
		require.NotEqual(t, uuid.Nil, modelConfig.ID)
		require.Equal(t, "openai", modelConfig.Provider)
		require.Equal(t, "gpt-4o-mini", modelConfig.Model)
		require.EqualValues(t, 4096, modelConfig.ContextLimit)
		require.True(t, modelConfig.IsDefault)
		requireChatModelPricing(t, modelConfig.ModelConfig, pricing)

		configs, err := client.ListChatModelConfigs(ctx)
		require.NoError(t, err)
		require.Len(t, configs, 1)
		requireChatModelPricing(t, configs[0].ModelConfig, pricing)
	})

	t.Run("RejectsNegativePricing", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client)

		_, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider: "openai",
			APIKey:   "test-api-key",
		})
		require.NoError(t, err)

		contextLimit := int64(4096)
		_, err = client.CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
			Provider:     "openai",
			Model:        "gpt-4o-mini",
			ContextLimit: &contextLimit,
			ModelConfig: &codersdk.ChatModelCallConfig{
				Cost: &codersdk.ModelCostConfig{
					InputPricePerMillionTokens: decRef("-0.01"),
				},
			},
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Invalid model config.", sdkErr.Message)
		require.Equal(
			t,
			"cost.input_price_per_million_tokens must be greater than or equal to zero",
			sdkErr.Detail,
		)
	})

	t.Run("MissingContextLimit", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client)

		_, err := client.CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
			Provider: "openai",
			Model:    "gpt-4o-mini",
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Context limit is required.", sdkErr.Message)
	})

	t.Run("ProviderNotConfigured", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client)

		contextLimit := int64(4096)
		_, err := client.CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
			Provider:     "openai",
			Model:        "gpt-4o-mini",
			ContextLimit: &contextLimit,
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Chat provider is not configured.", sdkErr.Message)
	})

	t.Run("ForbiddenForOrganizationMember", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		adminClient := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, adminClient)
		memberClient, _ := coderdtest.CreateAnotherUser(t, adminClient, firstUser.OrganizationID)

		_, err := adminClient.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider: "openai",
			APIKey:   "test-api-key",
		})
		require.NoError(t, err)

		contextLimit := int64(4096)
		_, err = memberClient.CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
			Provider:     "openai",
			Model:        "gpt-4o-mini",
			ContextLimit: &contextLimit,
		})
		requireSDKError(t, err, http.StatusForbidden)
	})
}

func TestUpdateChatModelConfig(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client)
		modelConfig := createChatModelConfig(t, client)

		contextLimit := int64(8192)
		pricing := &codersdk.ChatModelCallConfig{
			Cost: &codersdk.ModelCostConfig{
				InputPricePerMillionTokens:      decRef("0.2"),
				OutputPricePerMillionTokens:     decRef("0.8"),
				CacheReadPricePerMillionTokens:  decRef("0.04"),
				CacheWritePricePerMillionTokens: decRef("0.4"),
			},
		}
		updated, err := client.UpdateChatModelConfig(ctx, modelConfig.ID, codersdk.UpdateChatModelConfigRequest{
			DisplayName:  "GPT-4o Mini Updated",
			ContextLimit: &contextLimit,
			ModelConfig:  pricing,
		})
		require.NoError(t, err)
		require.Equal(t, modelConfig.ID, updated.ID)
		require.Equal(t, "GPT-4o Mini Updated", updated.DisplayName)
		require.EqualValues(t, 8192, updated.ContextLimit)
		requireChatModelPricing(t, updated.ModelConfig, pricing)

		configs, err := client.ListChatModelConfigs(ctx)
		require.NoError(t, err)
		require.Len(t, configs, 1)
		requireChatModelPricing(t, configs[0].ModelConfig, pricing)
	})

	t.Run("RejectsNegativePricing", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client)
		modelConfig := createChatModelConfig(t, client)

		_, err := client.UpdateChatModelConfig(ctx, modelConfig.ID, codersdk.UpdateChatModelConfigRequest{
			ModelConfig: &codersdk.ChatModelCallConfig{
				Cost: &codersdk.ModelCostConfig{
					OutputPricePerMillionTokens: decRef("-1.0"),
				},
			},
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Invalid model config.", sdkErr.Message)
		require.Equal(
			t,
			"cost.output_price_per_million_tokens must be greater than or equal to zero",
			sdkErr.Detail,
		)
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client)

		_, err := client.UpdateChatModelConfig(ctx, uuid.New(), codersdk.UpdateChatModelConfigRequest{
			DisplayName: "missing",
		})
		requireSDKError(t, err, http.StatusNotFound)
	})

	t.Run("InvalidContextLimit", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client)
		modelConfig := createChatModelConfig(t, client)

		contextLimit := int64(0)
		_, err := client.UpdateChatModelConfig(ctx, modelConfig.ID, codersdk.UpdateChatModelConfigRequest{
			ContextLimit: &contextLimit,
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Context limit must be greater than zero.", sdkErr.Message)
	})

	t.Run("InvalidModelConfigID", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client)

		res, err := client.Request(
			ctx,
			http.MethodPatch,
			"/api/experimental/chats/model-configs/not-a-uuid",
			codersdk.UpdateChatModelConfigRequest{DisplayName: "ignored"},
		)
		require.NoError(t, err)
		defer res.Body.Close()

		err = codersdk.ReadBodyAsError(res)
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Invalid chat model config ID.", sdkErr.Message)
	})

	t.Run("ForbiddenForOrganizationMember", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		adminClient := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, adminClient)
		memberClient, _ := coderdtest.CreateAnotherUser(t, adminClient, firstUser.OrganizationID)

		modelConfig := createChatModelConfig(t, adminClient)
		_, err := memberClient.UpdateChatModelConfig(ctx, modelConfig.ID, codersdk.UpdateChatModelConfigRequest{
			DisplayName: "member update",
		})
		requireSDKError(t, err, http.StatusForbidden)
	})
}

func TestDeleteChatModelConfig(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client)
		modelConfig := createChatModelConfig(t, client)

		err := client.DeleteChatModelConfig(ctx, modelConfig.ID)
		require.NoError(t, err)

		configs, err := client.ListChatModelConfigs(ctx)
		require.NoError(t, err)
		for _, config := range configs {
			require.NotEqual(t, modelConfig.ID, config.ID)
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client)

		err := client.DeleteChatModelConfig(ctx, uuid.New())
		requireSDKError(t, err, http.StatusNotFound)
	})

	t.Run("InvalidModelConfigID", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client)

		res, err := client.Request(
			ctx,
			http.MethodDelete,
			"/api/experimental/chats/model-configs/not-a-uuid",
			nil,
		)
		require.NoError(t, err)
		defer res.Body.Close()

		err = codersdk.ReadBodyAsError(res)
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Invalid chat model config ID.", sdkErr.Message)
	})

	t.Run("ForbiddenForOrganizationMember", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		adminClient := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, adminClient)
		memberClient, _ := coderdtest.CreateAnotherUser(t, adminClient, firstUser.OrganizationID)

		modelConfig := createChatModelConfig(t, adminClient)
		err := memberClient.DeleteChatModelConfig(ctx, modelConfig.ID)
		requireSDKError(t, err, http.StatusForbidden)
	})
}

func TestGetChat(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client)
		modelConfig := createChatModelConfig(t, client)

		createdChat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "get chat route payload",
				},
			},
		})
		require.NoError(t, err)

		chatResult, err := client.GetChat(ctx, createdChat.ID)
		require.NoError(t, err)
		messagesResult, err := client.GetChatMessages(ctx, createdChat.ID, nil)
		require.NoError(t, err)
		require.Equal(t, createdChat.ID, chatResult.ID)
		require.Equal(t, firstUser.UserID, chatResult.OwnerID)
		require.Equal(t, modelConfig.ID, chatResult.LastModelConfigID)
		require.Equal(t, "get chat route payload", chatResult.Title)
		require.NotZero(t, chatResult.CreatedAt)
		require.NotZero(t, chatResult.UpdatedAt)
		require.NotEmpty(t, messagesResult.Messages)
		require.Empty(t, messagesResult.QueuedMessages)

		foundUserMessage := false
		for _, message := range messagesResult.Messages {
			require.Equal(t, createdChat.ID, message.ChatID)
			require.NotEqual(t, codersdk.ChatMessageRoleSystem, message.Role)
			for _, part := range message.Content {
				if message.Role == codersdk.ChatMessageRoleUser &&
					part.Type == codersdk.ChatMessagePartTypeText &&
					part.Text == "get chat route payload" {
					foundUserMessage = true
				}
			}
		}
		require.True(t, foundUserMessage)
	})

	t.Run("NotFoundForDifferentUser", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client)
		_ = createChatModelConfig(t, client)

		createdChat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "private chat",
				},
			},
		})
		require.NoError(t, err)

		otherClient, _ := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)
		_, err = otherClient.GetChat(ctx, createdChat.ID)
		requireSDKError(t, err, http.StatusNotFound)
	})
}

func TestArchiveChat(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client)
		_ = createChatModelConfig(t, client)

		chatToArchive, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "archive me",
				},
			},
		})
		require.NoError(t, err)

		chatToKeep, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "keep me",
				},
			},
		})
		require.NoError(t, err)

		chatsBeforeArchive, err := client.ListChats(ctx, nil)
		require.NoError(t, err)
		require.Len(t, chatsBeforeArchive, 2)

		err = client.UpdateChat(ctx, chatToArchive.ID, codersdk.UpdateChatRequest{Archived: ptr.Ref(true)})
		require.NoError(t, err)

		// Default (no filter) returns only non-archived chats.
		allChats, err := client.ListChats(ctx, nil)
		require.NoError(t, err)
		require.Len(t, allChats, 1)
		require.Equal(t, chatToKeep.ID, allChats[0].ID)

		// archived:false returns only non-archived chats.
		activeChats, err := client.ListChats(ctx, &codersdk.ListChatsOptions{
			Query: "archived:false",
		})
		require.NoError(t, err)
		require.Len(t, activeChats, 1)
		require.Equal(t, chatToKeep.ID, activeChats[0].ID)
		require.False(t, activeChats[0].Archived)

		// archived:true returns only archived chats.
		archivedChats, err := client.ListChats(ctx, &codersdk.ListChatsOptions{
			Query: "archived:true",
		})
		require.NoError(t, err)
		require.Len(t, archivedChats, 1)
		require.Equal(t, chatToArchive.ID, archivedChats[0].ID)
		require.True(t, archivedChats[0].Archived)
	})
	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client)

		err := client.UpdateChat(ctx, uuid.New(), codersdk.UpdateChatRequest{Archived: ptr.Ref(true)})
		requireSDKError(t, err, http.StatusNotFound)
	})

	t.Run("ArchivesChildren", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		user := coderdtest.CreateFirstUser(t, client)
		modelConfig := createChatModelConfig(t, client)

		// Create a parent chat via the API.
		parentChat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "parent chat",
				},
			},
		})
		require.NoError(t, err)

		// Insert child chats directly via the database.
		child1, err := db.InsertChat(dbauthz.AsSystemRestricted(ctx), database.InsertChatParams{
			OwnerID:           user.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "child 1",
			ParentChatID:      uuid.NullUUID{UUID: parentChat.ID, Valid: true},
			RootChatID:        uuid.NullUUID{UUID: parentChat.ID, Valid: true},
		})
		require.NoError(t, err)

		child2, err := db.InsertChat(dbauthz.AsSystemRestricted(ctx), database.InsertChatParams{
			OwnerID:           user.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "child 2",
			ParentChatID:      uuid.NullUUID{UUID: parentChat.ID, Valid: true},
			RootChatID:        uuid.NullUUID{UUID: parentChat.ID, Valid: true},
		})
		require.NoError(t, err)

		// Archive the parent via the API.
		err = client.UpdateChat(ctx, parentChat.ID, codersdk.UpdateChatRequest{Archived: ptr.Ref(true)})
		require.NoError(t, err)

		// archived:false should exclude the entire archived family.
		activeChats, err := client.ListChats(ctx, &codersdk.ListChatsOptions{
			Query: "archived:false",
		})
		require.NoError(t, err)
		for _, c := range activeChats {
			require.NotEqual(t, parentChat.ID, c.ID, "parent should not appear")
			require.NotEqual(t, child1.ID, c.ID, "child1 should not appear")
			require.NotEqual(t, child2.ID, c.ID, "child2 should not appear")
		}

		// Verify children are archived directly in the DB.
		dbChild1, err := db.GetChatByID(dbauthz.AsSystemRestricted(ctx), child1.ID)
		require.NoError(t, err)
		require.True(t, dbChild1.Archived, "child1 should be archived")

		dbChild2, err := db.GetChatByID(dbauthz.AsSystemRestricted(ctx), child2.ID)
		require.NoError(t, err)
		require.True(t, dbChild2.Archived, "child2 should be archived")
	})
}

func TestUnarchiveChat(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client)
		_ = createChatModelConfig(t, client)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "archive then unarchive me",
				},
			},
		})
		require.NoError(t, err)

		// Archive the chat first.
		err = client.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{Archived: ptr.Ref(true)})
		require.NoError(t, err)

		// Verify it's archived.
		archivedChats, err := client.ListChats(ctx, &codersdk.ListChatsOptions{
			Query: "archived:true",
		})
		require.NoError(t, err)
		require.Len(t, archivedChats, 1)
		require.True(t, archivedChats[0].Archived)
		// Unarchive the chat.
		err = client.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{Archived: ptr.Ref(false)})
		require.NoError(t, err)

		// Verify it's no longer archived.
		activeChats, err := client.ListChats(ctx, &codersdk.ListChatsOptions{
			Query: "archived:false",
		})
		require.NoError(t, err)
		require.Len(t, activeChats, 1)
		require.Equal(t, chat.ID, activeChats[0].ID)
		require.False(t, activeChats[0].Archived)

		// No archived chats remain.
		archivedChats, err = client.ListChats(ctx, &codersdk.ListChatsOptions{
			Query: "archived:true",
		})
		require.NoError(t, err)
		require.Empty(t, archivedChats)
	})

	t.Run("NotArchived", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client)
		_ = createChatModelConfig(t, client)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "not archived",
				},
			},
		})
		require.NoError(t, err)

		// Trying to unarchive a non-archived chat should fail.
		err = client.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{Archived: ptr.Ref(false)})
		requireSDKError(t, err, http.StatusBadRequest)
	})
	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client)

		err := client.UpdateChat(ctx, uuid.New(), codersdk.UpdateChatRequest{Archived: ptr.Ref(false)})
		requireSDKError(t, err, http.StatusNotFound)
	})
}

func TestPostChatMessages(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client)
		_ = createChatModelConfig(t, client)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "initial message for post route test",
				},
			},
		})
		require.NoError(t, err)

		hasTextPart := func(parts []codersdk.ChatMessagePart, want string) bool {
			for _, part := range parts {
				if part.Type == codersdk.ChatMessagePartTypeText && part.Text == want {
					return true
				}
			}
			return false
		}

		messageText := "post message route success " + uuid.NewString()
		created, err := client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: messageText,
				},
			},
		})
		require.NoError(t, err)

		if created.Queued {
			require.Nil(t, created.Message)
			require.NotNil(t, created.QueuedMessage)
			require.Equal(t, chat.ID, created.QueuedMessage.ChatID)
			require.NotZero(t, created.QueuedMessage.ID)
			require.True(t, hasTextPart(created.QueuedMessage.Content, messageText))

			require.Eventually(t, func() bool {
				messagesResult, getErr := client.GetChatMessages(ctx, chat.ID, nil)
				if getErr != nil {
					return false
				}

				for _, queued := range messagesResult.QueuedMessages {
					if queued.ID == created.QueuedMessage.ID &&
						queued.ChatID == chat.ID &&
						hasTextPart(queued.Content, messageText) {
						return true
					}
				}
				for _, message := range messagesResult.Messages {
					if message.Role == codersdk.ChatMessageRoleUser && hasTextPart(message.Content, messageText) {
						return true
					}
				}
				return false
			}, testutil.WaitLong, testutil.IntervalFast)
		} else {
			require.Nil(t, created.QueuedMessage)
			require.NotNil(t, created.Message)
			require.Equal(t, chat.ID, created.Message.ChatID)
			require.Equal(t, codersdk.ChatMessageRoleUser, created.Message.Role)
			require.NotZero(t, created.Message.ID)
			require.True(t, hasTextPart(created.Message.Content, messageText))

			require.Eventually(t, func() bool {
				messagesResult, getErr := client.GetChatMessages(ctx, chat.ID, nil)
				if getErr != nil {
					return false
				}
				for _, message := range messagesResult.Messages {
					if message.ID == created.Message.ID &&
						message.Role == codersdk.ChatMessageRoleUser &&
						hasTextPart(message.Content, messageText) {
						return true
					}
				}
				return false
			}, testutil.WaitLong, testutil.IntervalFast)
		}
	})

	t.Run("EmptyText", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client)
		_ = createChatModelConfig(t, client)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "initial message for validation test",
				},
			},
		})
		require.NoError(t, err)

		_, err = client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "   ",
				},
			},
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Invalid input part.", sdkErr.Message)
		require.Equal(t, "content[0].text cannot be empty.", sdkErr.Detail)
	})

	t.Run("UsageLimitExceeded", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		_ = coderdtest.CreateFirstUser(t, client)
		modelConfig := createChatModelConfig(t, client)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			Content: []codersdk.ChatInputPart{{
				Type: codersdk.ChatInputPartTypeText,
				Text: "initial message for usage-limit test",
			}},
		})
		require.NoError(t, err)

		wantResetsAt := enableDailyChatUsageLimit(ctx, t, db, 100)
		insertAssistantCostMessage(ctx, t, db, chat.ID, modelConfig.ID, 100)

		_, err = client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
			Content: []codersdk.ChatInputPart{{
				Type: codersdk.ChatInputPartTypeText,
				Text: "over limit",
			}},
		})
		requireChatUsageLimitExceededError(t, err, 100, 100, wantResetsAt)
	})

	t.Run("ChatNotFound", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client)
		_ = createChatModelConfig(t, client)

		_, err := client.CreateChatMessage(ctx, uuid.New(), codersdk.CreateChatMessageRequest{
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "hello",
				},
			},
		})
		requireSDKError(t, err, http.StatusNotFound)
	})
}

func TestChatMessageWithFileReferences(t *testing.T) {
	t.Parallel()

	// createChat is a helper that creates a chat so we can post messages to it.
	createChatForTest := func(t *testing.T, client *codersdk.Client) codersdk.Chat {
		t.Helper()
		ctx := testutil.Context(t, testutil.WaitLong)
		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			Content: []codersdk.ChatInputPart{{
				Type: codersdk.ChatInputPartTypeText,
				Text: "initial message",
			}},
		})
		require.NoError(t, err)
		return chat
	}

	t.Run("FileReferenceOnly", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client)
		_ = createChatModelConfig(t, client)
		chat := createChatForTest(t, client)

		created, err := client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
			Content: []codersdk.ChatInputPart{{
				Type:      codersdk.ChatInputPartTypeFileReference,
				FileName:  "main.go",
				StartLine: 10,
				EndLine:   15,
				Content:   "func broken() {}",
			}},
		})
		require.NoError(t, err)

		// File-reference parts are stored as structured parts.
		checkFileRef := func(part codersdk.ChatMessagePart) bool {
			return part.Type == codersdk.ChatMessagePartTypeFileReference &&
				part.FileName == "main.go" &&
				part.StartLine == 10 &&
				part.EndLine == 15 &&
				part.Content == "func broken() {}"
		}

		var found bool
		require.Eventually(t, func() bool {
			messagesResult, getErr := client.GetChatMessages(ctx, chat.ID, nil)
			if getErr != nil {
				return false
			}
			for _, message := range messagesResult.Messages {
				if message.Role != codersdk.ChatMessageRoleUser {
					continue
				}
				for _, part := range message.Content {
					if checkFileRef(part) {
						found = true
						return true
					}
				}
			}
			// The message may have been queued.
			if created.Queued && created.QueuedMessage != nil {
				for _, queued := range messagesResult.QueuedMessages {
					for _, part := range queued.Content {
						if checkFileRef(part) {
							found = true
							return true
						}
					}
				}
			}
			return false
		}, testutil.WaitLong, testutil.IntervalFast)
		require.True(t, found, "expected to find file-reference part in stored message")
	})

	t.Run("FileReferenceSingleLine", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client)
		_ = createChatModelConfig(t, client)
		chat := createChatForTest(t, client)

		created, err := client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
			Content: []codersdk.ChatInputPart{{
				Type:      codersdk.ChatInputPartTypeFileReference,
				FileName:  "lib/utils.ts",
				StartLine: 42,
				EndLine:   42,
				Content:   "const x = 1;",
			}},
		})
		require.NoError(t, err)

		checkFileRef := func(part codersdk.ChatMessagePart) bool {
			return part.Type == codersdk.ChatMessagePartTypeFileReference &&
				part.FileName == "lib/utils.ts" &&
				part.StartLine == 42 &&
				part.EndLine == 42 &&
				part.Content == "const x = 1;"
		}

		require.Eventually(t, func() bool {
			messagesResult, getErr := client.GetChatMessages(ctx, chat.ID, nil)
			if getErr != nil {
				return false
			}
			for _, msg := range messagesResult.Messages {
				for _, part := range msg.Content {
					if checkFileRef(part) {
						return true
					}
				}
			}
			if created.Queued && created.QueuedMessage != nil {
				for _, queued := range messagesResult.QueuedMessages {
					for _, part := range queued.Content {
						if checkFileRef(part) {
							return true
						}
					}
				}
			}
			return false
		}, testutil.WaitLong, testutil.IntervalFast)
	})

	t.Run("FileReferenceWithoutContent", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client)
		_ = createChatModelConfig(t, client)
		chat := createChatForTest(t, client)

		created, err := client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
			Content: []codersdk.ChatInputPart{{
				Type:      codersdk.ChatInputPartTypeFileReference,
				FileName:  "README.md",
				StartLine: 1,
				EndLine:   1,
				// No code content — just a file reference.
			}},
		})
		require.NoError(t, err)

		checkFileRef := func(part codersdk.ChatMessagePart) bool {
			return part.Type == codersdk.ChatMessagePartTypeFileReference &&
				part.FileName == "README.md" &&
				part.StartLine == 1 &&
				part.EndLine == 1 &&
				part.Content == ""
		}

		require.Eventually(t, func() bool {
			messagesResult, getErr := client.GetChatMessages(ctx, chat.ID, nil)
			if getErr != nil {
				return false
			}
			for _, msg := range messagesResult.Messages {
				for _, part := range msg.Content {
					if checkFileRef(part) {
						return true
					}
				}
			}
			if created.Queued && created.QueuedMessage != nil {
				for _, queued := range messagesResult.QueuedMessages {
					for _, part := range queued.Content {
						if checkFileRef(part) {
							return true
						}
					}
				}
			}
			return false
		}, testutil.WaitLong, testutil.IntervalFast)
	})

	t.Run("FileReferenceWithCode", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client)
		_ = createChatModelConfig(t, client)
		chat := createChatForTest(t, client)

		created, err := client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
			Content: []codersdk.ChatInputPart{{
				Type:      codersdk.ChatInputPartTypeFileReference,
				FileName:  "server.go",
				StartLine: 5,
				EndLine:   8,
				Content:   "func main() {\n\tfmt.Println()\n}",
			}},
		})
		require.NoError(t, err)

		checkFileRef := func(part codersdk.ChatMessagePart) bool {
			return part.Type == codersdk.ChatMessagePartTypeFileReference &&
				part.FileName == "server.go" &&
				part.StartLine == 5 &&
				part.EndLine == 8 &&
				part.Content == "func main() {\n\tfmt.Println()\n}"
		}

		require.Eventually(t, func() bool {
			messagesResult, getErr := client.GetChatMessages(ctx, chat.ID, nil)
			if getErr != nil {
				return false
			}
			for _, msg := range messagesResult.Messages {
				for _, part := range msg.Content {
					if checkFileRef(part) {
						return true
					}
				}
			}
			if created.Queued && created.QueuedMessage != nil {
				for _, queued := range messagesResult.QueuedMessages {
					for _, part := range queued.Content {
						if checkFileRef(part) {
							return true
						}
					}
				}
			}
			return false
		}, testutil.WaitLong, testutil.IntervalFast)
	})

	t.Run("InterleavedTextAndFileReferences", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client)
		_ = createChatModelConfig(t, client)
		chat := createChatForTest(t, client)

		created, err := client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "Please review these two issues:",
				},
				{
					Type:      codersdk.ChatInputPartTypeFileReference,
					FileName:  "a.go",
					StartLine: 1,
					EndLine:   3,
					Content:   "line1\nline2\nline3",
				},
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "first issue",
				},
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "and also:",
				},
				{
					Type:      codersdk.ChatInputPartTypeFileReference,
					FileName:  "b.go",
					StartLine: 10,
					EndLine:   10,
					Content:   "return nil",
				},
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "second issue",
				},
			},
		})
		require.NoError(t, err)

		// Verify that all six parts are stored in order with
		// correct types: text, file-reference, text, text,
		// file-reference, text.
		type wantPart struct {
			typ       codersdk.ChatMessagePartType
			text      string
			fileName  string
			startLine int
			endLine   int
			content   string
		}
		want := []wantPart{
			{typ: codersdk.ChatMessagePartTypeText, text: "Please review these two issues:"},
			{typ: codersdk.ChatMessagePartTypeFileReference, fileName: "a.go", startLine: 1, endLine: 3, content: "line1\nline2\nline3"},
			{typ: codersdk.ChatMessagePartTypeText, text: "first issue"},
			{typ: codersdk.ChatMessagePartTypeText, text: "and also:"},
			{typ: codersdk.ChatMessagePartTypeFileReference, fileName: "b.go", startLine: 10, endLine: 10, content: "return nil"},
			{typ: codersdk.ChatMessagePartTypeText, text: "second issue"},
		}

		require.Eventually(t, func() bool {
			messagesResult, getErr := client.GetChatMessages(ctx, chat.ID, nil)
			if getErr != nil {
				return false
			}

			checkParts := func(parts []codersdk.ChatMessagePart) bool {
				if len(parts) != len(want) {
					return false
				}
				for i, w := range want {
					p := parts[i]
					if p.Type != w.typ {
						return false
					}
					switch w.typ {
					case codersdk.ChatMessagePartTypeText:
						if p.Text != w.text {
							return false
						}
					case codersdk.ChatMessagePartTypeFileReference:
						if p.FileName != w.fileName ||
							p.StartLine != w.startLine ||
							p.EndLine != w.endLine ||
							p.Content != w.content {
							return false
						}
					}
				}
				return true
			}

			for _, msg := range messagesResult.Messages {
				if msg.Role == codersdk.ChatMessageRoleUser && checkParts(msg.Content) {
					return true
				}
			}
			if created.Queued && created.QueuedMessage != nil {
				for _, queued := range messagesResult.QueuedMessages {
					if checkParts(queued.Content) {
						return true
					}
				}
			}
			return false
		}, testutil.WaitLong, testutil.IntervalFast)
	})

	t.Run("EmptyFileName", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client)
		_ = createChatModelConfig(t, client)
		chat := createChatForTest(t, client)

		_, err := client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
			Content: []codersdk.ChatInputPart{{
				Type:      codersdk.ChatInputPartTypeFileReference,
				FileName:  "",
				StartLine: 1,
				EndLine:   1,
			}},
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Invalid input part.", sdkErr.Message)
		require.Equal(t, "content[0].file_name cannot be empty for file-reference.", sdkErr.Detail)
	})

	t.Run("CreateChatWithFileReference", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client)
		_ = createChatModelConfig(t, client)

		// File references should also work in the initial CreateChat call.
		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			Content: []codersdk.ChatInputPart{{
				Type:      codersdk.ChatInputPartTypeFileReference,
				FileName:  "bug.py",
				StartLine: 7,
				EndLine:   7,
				Content:   "x = None",
			}},
		})
		require.NoError(t, err)
		require.NotEqual(t, uuid.Nil, chat.ID)

		// Title is derived from the text parts. For file-references
		// the formatted text becomes the title source.
		require.NotEmpty(t, chat.Title)
	})
}

func TestChatMessageWithFiles(t *testing.T) {
	t.Parallel()

	t.Run("FileOnly", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client)
		_ = createChatModelConfig(t, client)

		// Upload a file.
		pngData := append([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, make([]byte, 64)...)
		uploadResp, err := client.UploadChatFile(ctx, firstUser.OrganizationID, "image/png", "test.png", bytes.NewReader(pngData))
		require.NoError(t, err)

		// Create a chat with text first.
		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "initial message",
				},
			},
		})
		require.NoError(t, err)

		// Send a file-only message (no text).
		resp, err := client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
			Content: []codersdk.ChatInputPart{
				{
					Type:   codersdk.ChatInputPartTypeFile,
					FileID: uploadResp.ID,
				},
			},
		})
		require.NoError(t, err)

		// Verify the message was accepted.
		if resp.Queued {
			require.NotNil(t, resp.QueuedMessage)
		} else {
			require.NotNil(t, resp.Message)
			require.Equal(t, codersdk.ChatMessageRoleUser, resp.Message.Role)
		}
	})

	t.Run("TextAndFile", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client)
		_ = createChatModelConfig(t, client)

		// Upload a file.
		pngData := append([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, make([]byte, 64)...)
		uploadResp, err := client.UploadChatFile(ctx, firstUser.OrganizationID, "image/png", "test.png", bytes.NewReader(pngData))
		require.NoError(t, err)

		// Create a chat with text first.
		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "initial message",
				},
			},
		})
		require.NoError(t, err)

		// Send a message with both text and file.
		resp, err := client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "here is an image",
				},
				{
					Type:   codersdk.ChatInputPartTypeFile,
					FileID: uploadResp.ID,
				},
			},
		})
		require.NoError(t, err)

		if resp.Queued {
			require.NotNil(t, resp.QueuedMessage)
		} else {
			require.NotNil(t, resp.Message)
			require.Equal(t, codersdk.ChatMessageRoleUser, resp.Message.Role)
		}

		// Verify file parts omit inline data in the API response.
		messagesResult, err := client.GetChatMessages(ctx, chat.ID, nil)
		require.NoError(t, err)
		for _, msg := range messagesResult.Messages {
			for _, part := range msg.Content {
				if part.Type == codersdk.ChatMessagePartTypeFile {
					require.True(t, part.FileID.Valid, "file part should have a valid file_id")
					require.Equal(t, uploadResp.ID, part.FileID.UUID)
					require.Nil(t, part.Data, "file data should not be sent when file_id is present")
				}
			}
		}
	})

	t.Run("FileOnlyOnCreate", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client)
		_ = createChatModelConfig(t, client)

		// Upload a file.
		pngData := append([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, make([]byte, 64)...)
		uploadResp, err := client.UploadChatFile(ctx, firstUser.OrganizationID, "image/png", "test.png", bytes.NewReader(pngData))
		require.NoError(t, err)

		// Create a new chat with only a file part.
		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			Content: []codersdk.ChatInputPart{
				{
					Type:   codersdk.ChatInputPartTypeFile,
					FileID: uploadResp.ID,
				},
			},
		})
		require.NoError(t, err)

		// With no text, chatTitleFromMessage("") returns "New Chat".
		require.Equal(t, "New Chat", chat.Title)
	})

	t.Run("InvalidFileID", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client)
		_ = createChatModelConfig(t, client)

		// Create a chat with text first.
		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "initial message",
				},
			},
		})
		require.NoError(t, err)

		// Send a message with a non-existent file ID.
		_, err = client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
			Content: []codersdk.ChatInputPart{
				{
					Type:   codersdk.ChatInputPartTypeFile,
					FileID: uuid.New(),
				},
			},
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Invalid input part.", sdkErr.Message)
		require.Contains(t, sdkErr.Detail, "does not exist")
	})
}

func TestPatchChatMessage(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client)
		_ = createChatModelConfig(t, client)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "hello before edit",
				},
			},
		})
		require.NoError(t, err)

		messagesResult, err := client.GetChatMessages(ctx, chat.ID, nil)
		require.NoError(t, err)

		var userMessageID int64
		for _, message := range messagesResult.Messages {
			if message.Role == codersdk.ChatMessageRoleUser {
				userMessageID = message.ID
				break
			}
		}
		require.NotZero(t, userMessageID)

		edited, err := client.EditChatMessage(ctx, chat.ID, userMessageID, codersdk.EditChatMessageRequest{
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "hello after edit",
				},
			},
		})
		require.NoError(t, err)
		// The edited message is soft-deleted and a new one is inserted,
		// so the returned ID will differ from the original.
		require.NotEqual(t, userMessageID, edited.ID)
		require.Equal(t, codersdk.ChatMessageRoleUser, edited.Role)

		foundEditedText := false
		for _, part := range edited.Content {
			if part.Type == codersdk.ChatMessagePartTypeText && part.Text == "hello after edit" {
				foundEditedText = true
			}
		}
		require.True(t, foundEditedText)

		messagesResult, err = client.GetChatMessages(ctx, chat.ID, nil)
		require.NoError(t, err)
		foundEditedInChat := false
		foundOriginalInChat := false
		for _, message := range messagesResult.Messages {
			if message.Role != codersdk.ChatMessageRoleUser {
				continue
			}
			for _, part := range message.Content {
				if part.Type != codersdk.ChatMessagePartTypeText {
					continue
				}
				if part.Text == "hello after edit" {
					foundEditedInChat = true
				}
				if part.Text == "hello before edit" {
					foundOriginalInChat = true
				}
			}
		}
		require.True(t, foundEditedInChat)
		require.False(t, foundOriginalInChat)
	})

	t.Run("PreservesFileID", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client)
		_ = createChatModelConfig(t, client)

		// Upload a file.
		pngData := append([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, make([]byte, 64)...)
		uploadResp, err := client.UploadChatFile(ctx, firstUser.OrganizationID, "image/png", "test.png", bytes.NewReader(pngData))
		require.NoError(t, err)

		// Create a chat with a text + file part.
		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "before edit with file",
				},
				{
					Type:   codersdk.ChatInputPartTypeFile,
					FileID: uploadResp.ID,
				},
			},
		})
		require.NoError(t, err)

		// Find the user message ID.
		messagesResult, err := client.GetChatMessages(ctx, chat.ID, nil)
		require.NoError(t, err)

		var userMessageID int64
		for _, message := range messagesResult.Messages {
			if message.Role == codersdk.ChatMessageRoleUser {
				userMessageID = message.ID
				break
			}
		}
		require.NotZero(t, userMessageID)

		// Edit the message: new text, same file_id.
		edited, err := client.EditChatMessage(ctx, chat.ID, userMessageID, codersdk.EditChatMessageRequest{
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "after edit with file",
				},
				{
					Type:   codersdk.ChatInputPartTypeFile,
					FileID: uploadResp.ID,
				},
			},
		})
		require.NoError(t, err)
		// The edited message is soft-deleted and a new one is inserted,
		// so the returned ID will differ from the original.
		require.NotEqual(t, userMessageID, edited.ID)

		// Assert the edit response preserves the file_id.
		var foundText, foundFile bool
		for _, part := range edited.Content {
			if part.Type == codersdk.ChatMessagePartTypeText && part.Text == "after edit with file" {
				foundText = true
			}
			if part.Type == codersdk.ChatMessagePartTypeFile && part.FileID.Valid && part.FileID.UUID == uploadResp.ID {
				foundFile = true
				require.Nil(t, part.Data, "file data should not be sent when file_id is present")
			}
		}
		require.True(t, foundText, "edited message should contain updated text")
		require.True(t, foundFile, "edited message should preserve file_id")

		// GET the chat messages and verify the file_id persists.
		messagesResult, err = client.GetChatMessages(ctx, chat.ID, nil)
		require.NoError(t, err)

		var foundTextInChat, foundFileInChat bool
		for _, message := range messagesResult.Messages {
			if message.Role != codersdk.ChatMessageRoleUser {
				continue
			}
			for _, part := range message.Content {
				if part.Type == codersdk.ChatMessagePartTypeText && part.Text == "after edit with file" {
					foundTextInChat = true
				}
				if part.Type == codersdk.ChatMessagePartTypeFile && part.FileID.Valid && part.FileID.UUID == uploadResp.ID {
					foundFileInChat = true
					require.Nil(t, part.Data, "file data should not be sent when file_id is present")
				}
			}
		}
		require.True(t, foundTextInChat, "chat should contain edited text")
		require.True(t, foundFileInChat, "chat should preserve file_id after edit")
	})

	t.Run("UsageLimitExceeded", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		_ = coderdtest.CreateFirstUser(t, client)
		modelConfig := createChatModelConfig(t, client)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			Content: []codersdk.ChatInputPart{{
				Type: codersdk.ChatInputPartTypeText,
				Text: "hello before edit",
			}},
		})
		require.NoError(t, err)

		messagesResult, err := client.GetChatMessages(ctx, chat.ID, nil)
		require.NoError(t, err)

		var userMessageID int64
		for _, message := range messagesResult.Messages {
			if message.Role == codersdk.ChatMessageRoleUser {
				userMessageID = message.ID
				break
			}
		}
		require.NotZero(t, userMessageID)

		wantResetsAt := enableDailyChatUsageLimit(ctx, t, db, 100)
		insertAssistantCostMessage(ctx, t, db, chat.ID, modelConfig.ID, 100)

		_, err = client.EditChatMessage(ctx, chat.ID, userMessageID, codersdk.EditChatMessageRequest{
			Content: []codersdk.ChatInputPart{{
				Type: codersdk.ChatInputPartTypeText,
				Text: "edited over limit",
			}},
		})
		requireChatUsageLimitExceededError(t, err, 100, 100, wantResetsAt)
	})

	t.Run("MessageNotFound", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client)
		_ = createChatModelConfig(t, client)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "hello",
				},
			},
		})
		require.NoError(t, err)

		_, err = client.EditChatMessage(ctx, chat.ID, 999999, codersdk.EditChatMessageRequest{
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "edited",
				},
			},
		})
		sdkErr := requireSDKError(t, err, http.StatusNotFound)
		require.Equal(t, "Chat message not found.", sdkErr.Message)
	})

	t.Run("InvalidMessageID", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client)
		_ = createChatModelConfig(t, client)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "hello",
				},
			},
		})
		require.NoError(t, err)

		res, err := client.Request(
			ctx,
			http.MethodPatch,
			fmt.Sprintf("/api/experimental/chats/%s/messages/not-an-int", chat.ID),
			codersdk.EditChatMessageRequest{
				Content: []codersdk.ChatInputPart{
					{
						Type: codersdk.ChatInputPartTypeText,
						Text: "ignored",
					},
				},
			},
		)
		require.NoError(t, err)
		defer res.Body.Close()

		err = codersdk.ReadBodyAsError(res)
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Invalid chat message ID.", sdkErr.Message)
	})
}

func TestStreamChat(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client)
		_ = createChatModelConfig(t, client)

		const initialMessage = "stream chat route initial message"
		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: initialMessage,
				},
			},
		})
		require.NoError(t, err)

		events, closer, err := client.StreamChat(ctx, chat.ID, nil)
		require.NoError(t, err)
		defer closer.Close()

		hasTextPart := func(parts []codersdk.ChatMessagePart, want string) bool {
			for _, part := range parts {
				if part.Type == codersdk.ChatMessagePartTypeText && part.Text == want {
					return true
				}
			}
			return false
		}

		foundInitialUserMessage := false
		for !foundInitialUserMessage {
			select {
			case <-ctx.Done():
				require.FailNow(t, "timed out waiting for expected stream chat event")
			case event, ok := <-events:
				require.True(t, ok, "stream closed before expected event")
				require.Equal(t, chat.ID, event.ChatID)
				require.NotEqual(t, codersdk.ChatStreamEventTypeError, event.Type)

				if event.Type == codersdk.ChatStreamEventTypeMessage &&
					event.Message != nil &&
					event.Message.Role == codersdk.ChatMessageRoleUser &&
					hasTextPart(event.Message.Content, initialMessage) {
					foundInitialUserMessage = true
				}
			}
		}
	})

	t.Run("Unauthenticated", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client)

		unauthenticatedClient := codersdk.New(client.URL)
		res, err := unauthenticatedClient.Request(
			ctx,
			http.MethodGet,
			fmt.Sprintf("/api/experimental/chats/%s/stream", uuid.New()),
			nil,
		)
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)
	})
}

func TestInterruptChat(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		user := coderdtest.CreateFirstUser(t, client)
		modelConfig := createChatModelConfig(t, client)

		chat, err := db.InsertChat(dbauthz.AsSystemRestricted(ctx), database.InsertChatParams{
			OwnerID:           user.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "interrupt route test",
		})
		require.NoError(t, err)

		runningWorkerID := uuid.New()
		chat, err = db.UpdateChatStatus(dbauthz.AsSystemRestricted(ctx), database.UpdateChatStatusParams{
			ID:          chat.ID,
			Status:      database.ChatStatusRunning,
			WorkerID:    uuid.NullUUID{UUID: runningWorkerID, Valid: true},
			StartedAt:   sql.NullTime{Time: time.Now(), Valid: true},
			HeartbeatAt: sql.NullTime{Time: time.Now(), Valid: true},
		})
		require.NoError(t, err)
		require.Equal(t, database.ChatStatusRunning, chat.Status)
		require.True(t, chat.WorkerID.Valid)
		require.True(t, chat.StartedAt.Valid)
		require.True(t, chat.HeartbeatAt.Valid)

		interrupted, err := client.InterruptChat(ctx, chat.ID)
		require.NoError(t, err)
		require.Equal(t, chat.ID, interrupted.ID)
		require.Equal(t, codersdk.ChatStatusWaiting, interrupted.Status)

		persisted, err := db.GetChatByID(dbauthz.AsSystemRestricted(ctx), chat.ID)
		require.NoError(t, err)
		require.Equal(t, database.ChatStatusWaiting, persisted.Status)
		require.False(t, persisted.WorkerID.Valid)
		require.False(t, persisted.StartedAt.Valid)
		require.False(t, persisted.HeartbeatAt.Valid)
	})

	t.Run("ChatNotFound", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client)

		_, err := client.InterruptChat(ctx, uuid.New())
		requireSDKError(t, err, http.StatusNotFound)
	})
}

func TestGetChatDiffStatus(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
			DeploymentValues: chatDeploymentValues(t),
			ExternalAuthConfigs: []*externalauth.Config{
				{
					ID:    "gitlab-test",
					Type:  "gitlab",
					Regex: regexp.MustCompile(`github\.com`),
				},
			},
		})
		db := api.Database

		user := coderdtest.CreateFirstUser(t, client)
		modelConfig := createChatModelConfig(t, client)

		noCachedStatusChat, err := db.InsertChat(dbauthz.AsSystemRestricted(ctx), database.InsertChatParams{
			OwnerID:           user.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "get diff status route no cache",
		})
		require.NoError(t, err)

		noCachedChat, err := client.GetChat(ctx, noCachedStatusChat.ID)
		require.NoError(t, err)
		require.Equal(t, noCachedStatusChat.ID, noCachedChat.ID)
		require.Nil(t, noCachedChat.DiffStatus)

		cachedStatusChat, err := db.InsertChat(dbauthz.AsSystemRestricted(ctx), database.InsertChatParams{
			OwnerID:           user.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "get diff status route cached",
		})
		require.NoError(t, err)

		refreshedAt := time.Now().UTC().Truncate(time.Second)
		staleAt := refreshedAt.Add(time.Hour)
		_, err = db.UpsertChatDiffStatusReference(
			dbauthz.AsSystemRestricted(ctx),
			database.UpsertChatDiffStatusReferenceParams{
				ChatID:          cachedStatusChat.ID,
				Url:             sql.NullString{},
				GitBranch:       "feature/diff-status",
				GitRemoteOrigin: "git@github.com:coder/coder.git",
				StaleAt:         staleAt,
			},
		)
		require.NoError(t, err)

		_, err = db.UpsertChatDiffStatus(
			dbauthz.AsSystemRestricted(ctx),
			database.UpsertChatDiffStatusParams{
				ChatID: cachedStatusChat.ID,
				Url:    sql.NullString{},
				PullRequestState: sql.NullString{
					String: " open ",
					Valid:  true,
				},
				ChangesRequested: true,
				Additions:        11,
				Deletions:        4,
				ChangedFiles:     3,
				RefreshedAt:      refreshedAt,
				StaleAt:          staleAt,
			},
		)
		require.NoError(t, err)

		cachedChat, err := client.GetChat(ctx, cachedStatusChat.ID)
		require.NoError(t, err)
		require.Equal(t, cachedStatusChat.ID, cachedChat.ID)
		require.NotNil(t, cachedChat.DiffStatus)
		cachedStatus := cachedChat.DiffStatus
		require.Equal(t, cachedStatusChat.ID, cachedStatus.ChatID)
		require.NotNil(t, cachedStatus.URL)
		require.Equal(t, "https://github.com/coder/coder/tree/feature/diff-status", *cachedStatus.URL)
		require.NotNil(t, cachedStatus.PullRequestState)
		require.Equal(t, "open", *cachedStatus.PullRequestState)
		require.True(t, cachedStatus.ChangesRequested)
		require.EqualValues(t, 11, cachedStatus.Additions)
		require.EqualValues(t, 4, cachedStatus.Deletions)
		require.EqualValues(t, 3, cachedStatus.ChangedFiles)
		require.NotNil(t, cachedStatus.RefreshedAt)
		require.WithinDuration(t, refreshedAt, *cachedStatus.RefreshedAt, time.Second)
		require.NotNil(t, cachedStatus.StaleAt)
		require.WithinDuration(t, staleAt, *cachedStatus.StaleAt, time.Second)
	})

	t.Run("NotFoundForDifferentUser", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client)
		_ = createChatModelConfig(t, client)

		createdChat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "private chat",
				},
			},
		})
		require.NoError(t, err)

		otherClient, _ := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)
		_, err = otherClient.GetChat(ctx, createdChat.ID)
		requireSDKError(t, err, http.StatusNotFound)
	})

	// Integration test: exercises the full GetChat handler refresh
	// path with a real DB, dbauthz, a mock GitHub API, and an
	// external-auth-linked user. Verifies that a stale chat diff
	// status is refreshed end-to-end via the gitsync worker's
	// Refresh pipeline (provider resolution, token acquisition
	// through external auth, and PR status fetch).
	t.Run("RefreshesStaleStatusWithExternalAuth", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)

		// Mock GitHub API over TLS so the git provider's URL patterns
		// (which require https://) match our PR URLs.
		ghAPI := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch {
			// PR status: GET /repos/{owner}/{repo}/pulls/{number}
			case r.URL.Path == "/repos/testorg/testrepo/pulls/42" && r.URL.Query().Get("per_page") == "":
				_, _ = w.Write([]byte(`{
					"state": "open",
					"merged": false,
					"draft": false,
					"additions": 25,
					"deletions": 7,
					"changed_files": 4,
					"head": {"sha": "abc123"}
				}`))
			// PR reviews: GET /repos/{owner}/{repo}/pulls/{number}/reviews
			case strings.HasSuffix(r.URL.Path, "/reviews"):
				_, _ = w.Write([]byte(`[]`))
			default:
				http.NotFound(w, r)
			}
		}))
		t.Cleanup(ghAPI.Close)

		// The git provider derives webBaseURL from apiBaseURL.
		// For a TLS server at https://127.0.0.1:PORT, webBaseURL
		// is the same, and PR URL patterns match
		// https://127.0.0.1:PORT/{owner}/{repo}/pull/{number}.
		ghWebHost := strings.TrimPrefix(ghAPI.URL, "https://")
		prURL := fmt.Sprintf("https://%s/testorg/testrepo/pull/42", ghWebHost)
		remoteOrigin := fmt.Sprintf("https://%s/testorg/testrepo.git", ghWebHost)

		// Set up a fake OIDC IDP for external auth login.
		const providerID = "test-github"
		fake := oidctest.NewFakeIDP(t, oidctest.WithServing())

		client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
			DeploymentValues: chatDeploymentValues(t),
			ExternalAuthConfigs: []*externalauth.Config{
				fake.ExternalAuthConfig(t, providerID, nil, func(cfg *externalauth.Config) {
					cfg.Type = codersdk.EnhancedExternalAuthProviderGitHub.String()
					// Point the git provider at our mock API server.
					cfg.APIBaseURL = ghAPI.URL
					// Match the remote origin (127.0.0.1 host).
					cfg.Regex = regexp.MustCompile(regexp.QuoteMeta(ghWebHost))
				}),
			},
		})
		db := api.Database

		// Use the TLS mock server's HTTP client (which trusts its
		// self-signed cert) for git provider API calls.
		api.HTTPClient = ghAPI.Client()

		user := coderdtest.CreateFirstUser(t, client)
		modelConfig := createChatModelConfig(t, client)

		// Log in to the external auth provider so the user has an
		// ExternalAuthLink row in the DB. This is what
		// resolveChatGitAccessToken reads via GetExternalAuthLink.
		fake.ExternalLogin(t, client)

		// Insert a chat owned by the user.
		chat, err := db.InsertChat(dbauthz.AsSystemRestricted(ctx), database.InsertChatParams{
			OwnerID:           user.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "rbac integration test",
		})
		require.NoError(t, err)

		// Store a pre-resolved PR URL so the refresh path uses
		// ParsePullRequestURL directly (skipping branch-to-PR
		// resolution, which isn't what we're testing). The status
		// is stale (stale_at in the past) so the handler triggers
		// a full refresh through RefreshChat.
		_, err = db.UpsertChatDiffStatusReference(
			dbauthz.AsSystemRestricted(ctx),
			database.UpsertChatDiffStatusReferenceParams{
				ChatID:          chat.ID,
				Url:             sql.NullString{String: prURL, Valid: true},
				GitBranch:       "feature/rbac-fix",
				GitRemoteOrigin: remoteOrigin,
				StaleAt:         time.Now().Add(-time.Minute),
			},
		)
		require.NoError(t, err)

		// Call GetChat which now resolves diff status inline.
		// This exercises the full code path:
		// resolveChatDiffStatus -> RefreshChat (with
		// AsSystemRestricted) -> Refresher.Refresh ->
		// resolveChatGitAccessToken (GetExternalAuthLink with
		// AsSystemRestricted) -> FetchPullRequestStatus (mock).
		//
		// Without the AsSystemRestricted fix, GetExternalAuthLink
		// would fail under the chatd RBAC context (missing
		// ActionReadPersonal), causing ErrNoTokenAvailable and a
		// refresh failure that silently returns stale data.
		result, err := client.GetChat(ctx, chat.ID)
		require.NoError(t, err)
		require.NotNil(t, result.DiffStatus)
		status := result.DiffStatus

		// The mock GitHub API returned PR #42 with 25 additions,
		// 7 deletions, 4 changed files, state "open".
		require.NotNil(t, status.RefreshedAt, "status should have been refreshed")
		require.NotNil(t, status.PullRequestState)
		require.Equal(t, "open", *status.PullRequestState)
		require.EqualValues(t, 25, status.Additions)
		require.EqualValues(t, 7, status.Deletions)
		require.EqualValues(t, 4, status.ChangedFiles)
		require.NotNil(t, status.URL)
		require.Contains(t, *status.URL, "pull/42")
	})
}

func TestGetChatDiffContents(t *testing.T) {
	t.Parallel()

	t.Run("SuccessWithCachedRepositoryReference", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
			DeploymentValues: chatDeploymentValues(t),
			ExternalAuthConfigs: []*externalauth.Config{
				{
					ID:    "gitlab-test",
					Type:  "gitlab",
					Regex: regexp.MustCompile(`gitlab\.example\.com`),
				},
			},
		})
		db := api.Database
		user := coderdtest.CreateFirstUser(t, client)
		modelConfig := createChatModelConfig(t, client)

		chat, err := db.InsertChat(dbauthz.AsSystemRestricted(ctx), database.InsertChatParams{
			OwnerID:           user.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "diff contents with cached repository reference",
		})
		require.NoError(t, err)

		_, err = db.UpsertChatDiffStatusReference(
			dbauthz.AsSystemRestricted(ctx),
			database.UpsertChatDiffStatusReferenceParams{
				ChatID:          chat.ID,
				Url:             sql.NullString{},
				GitBranch:       "feature/cached-diff",
				GitRemoteOrigin: "https://gitlab.example.com/acme/project.git",
				StaleAt:         time.Now().UTC().Add(time.Hour),
			},
		)
		require.NoError(t, err)

		diffContents, err := client.GetChatDiffContents(ctx, chat.ID)
		require.NoError(t, err)
		require.Equal(t, chat.ID, diffContents.ChatID)
		require.NotNil(t, diffContents.Provider)
		require.Equal(t, "gitlab", *diffContents.Provider)
		require.NotNil(t, diffContents.RemoteOrigin)
		require.Equal(t, "https://gitlab.example.com/acme/project.git", *diffContents.RemoteOrigin)
		require.NotNil(t, diffContents.Branch)
		require.Equal(t, "feature/cached-diff", *diffContents.Branch)
		require.Nil(t, diffContents.PullRequestURL)
		require.Empty(t, diffContents.Diff)
	})

	t.Run("SuccessWithoutCachedReference", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client)
		_ = createChatModelConfig(t, client)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "diff contents test",
				},
			},
		})
		require.NoError(t, err)

		diffContents, err := client.GetChatDiffContents(ctx, chat.ID)
		require.NoError(t, err)
		require.Equal(t, chat.ID, diffContents.ChatID)
		require.Nil(t, diffContents.Provider)
		require.Nil(t, diffContents.RemoteOrigin)
		require.Nil(t, diffContents.Branch)
		require.Nil(t, diffContents.PullRequestURL)
		require.Empty(t, diffContents.Diff)
	})

	t.Run("NotFoundForDifferentUser", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client)
		_ = createChatModelConfig(t, client)

		createdChat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "private chat",
				},
			},
		})
		require.NoError(t, err)

		otherClient, _ := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)
		_, err = otherClient.GetChatDiffContents(ctx, createdChat.ID)
		requireSDKError(t, err, http.StatusNotFound)
	})
}

func TestDeleteChatQueuedMessage(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		user := coderdtest.CreateFirstUser(t, client)
		modelConfig := createChatModelConfig(t, client)

		chat, err := db.InsertChat(dbauthz.AsSystemRestricted(ctx), database.InsertChatParams{
			OwnerID:           user.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "delete queued message route test",
		})
		require.NoError(t, err)

		deleteContent, err := json.Marshal([]codersdk.ChatMessagePart{
			codersdk.ChatMessageText("queued message for delete route"),
		})
		require.NoError(t, err)
		queuedMessage, err := db.InsertChatQueuedMessage(
			dbauthz.AsSystemRestricted(ctx),
			database.InsertChatQueuedMessageParams{
				ChatID:  chat.ID,
				Content: deleteContent,
			},
		)
		require.NoError(t, err)

		res, err := client.Request(
			ctx,
			http.MethodDelete,
			fmt.Sprintf("/api/experimental/chats/%s/queue/%d", chat.ID, queuedMessage.ID),
			nil,
		)
		require.NoError(t, err)
		res.Body.Close()
		require.Equal(t, http.StatusNoContent, res.StatusCode)

		messagesResult, err := client.GetChatMessages(ctx, chat.ID, nil)
		require.NoError(t, err)
		for _, queued := range messagesResult.QueuedMessages {
			require.NotEqual(t, queuedMessage.ID, queued.ID)
		}

		queuedMessages, err := db.GetChatQueuedMessages(dbauthz.AsSystemRestricted(ctx), chat.ID)
		require.NoError(t, err)
		for _, queued := range queuedMessages {
			require.NotEqual(t, queuedMessage.ID, queued.ID)
		}
	})

	t.Run("InvalidQueuedMessageID", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		user := coderdtest.CreateFirstUser(t, client)
		modelConfig := createChatModelConfig(t, client)

		chat, err := db.InsertChat(dbauthz.AsSystemRestricted(ctx), database.InsertChatParams{
			OwnerID:           user.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "delete queued invalid id",
		})
		require.NoError(t, err)

		invalidRes, err := client.Request(
			ctx,
			http.MethodDelete,
			fmt.Sprintf("/api/experimental/chats/%s/queue/not-an-int", chat.ID),
			nil,
		)
		require.NoError(t, err)
		defer invalidRes.Body.Close()

		err = codersdk.ReadBodyAsError(invalidRes)
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Invalid queued message ID.", sdkErr.Message)
		require.Contains(t, sdkErr.Detail, "invalid syntax")
	})
}

func TestPromoteChatQueuedMessage(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		user := coderdtest.CreateFirstUser(t, client)
		modelConfig := createChatModelConfig(t, client)

		chat, err := db.InsertChat(dbauthz.AsSystemRestricted(ctx), database.InsertChatParams{
			OwnerID:           user.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "promote queued message route test",
		})
		require.NoError(t, err)

		const queuedText = "queued message for promote route"
		queuedContent, err := json.Marshal([]codersdk.ChatMessagePart{
			codersdk.ChatMessageText(queuedText),
		})
		require.NoError(t, err)
		queuedMessage, err := db.InsertChatQueuedMessage(
			dbauthz.AsSystemRestricted(ctx),
			database.InsertChatQueuedMessageParams{
				ChatID:  chat.ID,
				Content: queuedContent,
			},
		)
		require.NoError(t, err)

		promoteRes, err := client.Request(
			ctx,
			http.MethodPost,
			fmt.Sprintf("/api/experimental/chats/%s/queue/%d/promote", chat.ID, queuedMessage.ID),
			nil,
		)
		require.NoError(t, err)
		defer promoteRes.Body.Close()
		require.Equal(t, http.StatusOK, promoteRes.StatusCode)

		var promoted codersdk.ChatMessage
		err = json.NewDecoder(promoteRes.Body).Decode(&promoted)
		require.NoError(t, err)
		require.NotZero(t, promoted.ID)
		require.Equal(t, chat.ID, promoted.ChatID)
		require.Equal(t, codersdk.ChatMessageRoleUser, promoted.Role)

		foundPromotedText := false
		for _, part := range promoted.Content {
			if part.Type == codersdk.ChatMessagePartTypeText &&
				part.Text == queuedText {
				foundPromotedText = true
				break
			}
		}
		require.True(t, foundPromotedText)

		messagesResult, err := client.GetChatMessages(ctx, chat.ID, nil)
		require.NoError(t, err)
		for _, queued := range messagesResult.QueuedMessages {
			require.NotEqual(t, queuedMessage.ID, queued.ID)
		}

		queuedMessages, err := db.GetChatQueuedMessages(dbauthz.AsSystemRestricted(ctx), chat.ID)
		require.NoError(t, err)
		for _, queued := range queuedMessages {
			require.NotEqual(t, queuedMessage.ID, queued.ID)
		}
	})

	t.Run("PromotesAlreadyQueuedMessageAfterLimitReached", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		user := coderdtest.CreateFirstUser(t, client)
		modelConfig := createChatModelConfig(t, client)
		enableDailyChatUsageLimit(ctx, t, db, 100)

		chat, err := db.InsertChat(dbauthz.AsSystemRestricted(ctx), database.InsertChatParams{
			OwnerID:           user.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "promote queued usage limit",
		})
		require.NoError(t, err)

		const queuedText = "queued message for promote route"
		queuedContent, err := json.Marshal([]codersdk.ChatMessagePart{
			codersdk.ChatMessageText(queuedText),
		})
		require.NoError(t, err)
		queuedMessage, err := db.InsertChatQueuedMessage(
			dbauthz.AsSystemRestricted(ctx),
			database.InsertChatQueuedMessageParams{
				ChatID:  chat.ID,
				Content: queuedContent,
			},
		)
		require.NoError(t, err)

		insertAssistantCostMessage(ctx, t, db, chat.ID, modelConfig.ID, 100)

		_, err = db.UpdateChatStatus(dbauthz.AsSystemRestricted(ctx), database.UpdateChatStatusParams{
			ID:          chat.ID,
			Status:      database.ChatStatusWaiting,
			WorkerID:    uuid.NullUUID{},
			StartedAt:   sql.NullTime{},
			HeartbeatAt: sql.NullTime{},
			LastError:   sql.NullString{},
		})
		require.NoError(t, err)

		promoteRes, err := client.Request(
			ctx,
			http.MethodPost,
			fmt.Sprintf("/api/experimental/chats/%s/queue/%d/promote", chat.ID, queuedMessage.ID),
			nil,
		)
		require.NoError(t, err)
		defer promoteRes.Body.Close()
		require.Equal(t, http.StatusOK, promoteRes.StatusCode)

		var promoted codersdk.ChatMessage
		err = json.NewDecoder(promoteRes.Body).Decode(&promoted)
		require.NoError(t, err)
		require.NotZero(t, promoted.ID)
		require.Equal(t, chat.ID, promoted.ChatID)
		require.Equal(t, codersdk.ChatMessageRoleUser, promoted.Role)

		foundPromotedText := false
		for _, part := range promoted.Content {
			if part.Type == codersdk.ChatMessagePartTypeText && part.Text == queuedText {
				foundPromotedText = true
				break
			}
		}
		require.True(t, foundPromotedText)

		queuedMessages, err := db.GetChatQueuedMessages(dbauthz.AsSystemRestricted(ctx), chat.ID)
		require.NoError(t, err)
		for _, queued := range queuedMessages {
			require.NotEqual(t, queuedMessage.ID, queued.ID)
		}
	})

	t.Run("InvalidQueuedMessageID", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		user := coderdtest.CreateFirstUser(t, client)
		modelConfig := createChatModelConfig(t, client)

		chat, err := db.InsertChat(dbauthz.AsSystemRestricted(ctx), database.InsertChatParams{
			OwnerID:           user.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "promote queued invalid id",
		})
		require.NoError(t, err)

		invalidRes, err := client.Request(
			ctx,
			http.MethodPost,
			fmt.Sprintf("/api/experimental/chats/%s/queue/not-an-int/promote", chat.ID),
			nil,
		)
		require.NoError(t, err)
		defer invalidRes.Body.Close()

		err = codersdk.ReadBodyAsError(invalidRes)
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Invalid queued message ID.", sdkErr.Message)
		require.Contains(t, sdkErr.Detail, "invalid syntax")
	})
}

func TestChatUsageLimitOverrideRoutes(t *testing.T) {
	t.Parallel()

	t.Run("UpsertUserOverrideRequiresPositiveSpendLimit", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, _ := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, client)
		_, member := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)

		res, err := client.Request(
			ctx,
			http.MethodPut,
			fmt.Sprintf("/api/experimental/chats/usage-limits/overrides/%s", member.ID),
			map[string]any{},
		)
		require.NoError(t, err)
		defer res.Body.Close()

		err = codersdk.ReadBodyAsError(res)
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Invalid chat usage limit override.", sdkErr.Message)
		require.Equal(t, "Spend limit must be greater than 0.", sdkErr.Detail)
	})

	t.Run("UpsertUserOverrideMissingUser", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client)

		_, err := client.UpsertChatUsageLimitOverride(ctx, uuid.New(), codersdk.UpsertChatUsageLimitOverrideRequest{
			SpendLimitMicros: 7_000_000,
		})
		sdkErr := requireSDKError(t, err, http.StatusNotFound)
		require.Equal(t, "User not found.", sdkErr.Message)
	})

	t.Run("DeleteUserOverrideMissingUser", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client)

		err := client.DeleteChatUsageLimitOverride(ctx, uuid.New())
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "User not found.", sdkErr.Message)
	})

	t.Run("DeleteUserOverrideMissingOverride", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client)
		_, member := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)

		err := client.DeleteChatUsageLimitOverride(ctx, member.ID)
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Chat usage limit override not found.", sdkErr.Message)
	})

	t.Run("UpsertGroupOverrideIncludesMemberCount", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, client)
		_, member := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)
		group := dbgen.Group(t, db, database.Group{OrganizationID: firstUser.OrganizationID})
		dbgen.GroupMember(t, db, database.GroupMemberTable{GroupID: group.ID, UserID: member.ID})
		dbgen.GroupMember(t, db, database.GroupMemberTable{GroupID: group.ID, UserID: database.PrebuildsSystemUserID})

		override, err := client.UpsertChatUsageLimitGroupOverride(ctx, group.ID, codersdk.UpsertChatUsageLimitGroupOverrideRequest{
			SpendLimitMicros: 7_000_000,
		})
		require.NoError(t, err)
		require.Equal(t, group.ID, override.GroupID)
		require.EqualValues(t, 1, override.MemberCount)
		require.NotNil(t, override.SpendLimitMicros)
		require.EqualValues(t, 7_000_000, *override.SpendLimitMicros)

		config, err := client.GetChatUsageLimitConfig(ctx)
		require.NoError(t, err)

		var listed *codersdk.ChatUsageLimitGroupOverride
		for i := range config.GroupOverrides {
			if config.GroupOverrides[i].GroupID == group.ID {
				listed = &config.GroupOverrides[i]
				break
			}
		}
		require.NotNil(t, listed)
		require.EqualValues(t, 1, listed.MemberCount)
	})

	t.Run("UpsertGroupOverrideMissingGroup", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client)

		_, err := client.UpsertChatUsageLimitGroupOverride(ctx, uuid.New(), codersdk.UpsertChatUsageLimitGroupOverrideRequest{
			SpendLimitMicros: 7_000_000,
		})
		sdkErr := requireSDKError(t, err, http.StatusNotFound)
		require.Equal(t, "Group not found.", sdkErr.Message)
	})

	t.Run("DeleteGroupOverrideMissingOverride", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, client)
		group := dbgen.Group(t, db, database.Group{OrganizationID: firstUser.OrganizationID})

		err := client.DeleteChatUsageLimitGroupOverride(ctx, group.ID)
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Chat usage limit group override not found.", sdkErr.Message)
	})
}

func TestPostChatFile(t *testing.T) {
	t.Parallel()

	t.Run("Success/PNG", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client)

		// Valid PNG header + padding.
		data := append([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, make([]byte, 64)...)
		resp, err := client.UploadChatFile(ctx, firstUser.OrganizationID, "image/png", "test.png", bytes.NewReader(data))
		require.NoError(t, err)
		require.NotEqual(t, uuid.Nil, resp.ID)
	})

	t.Run("Success/JPEG", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client)

		data := append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, make([]byte, 64)...)
		resp, err := client.UploadChatFile(ctx, firstUser.OrganizationID, "image/jpeg", "test.jpg", bytes.NewReader(data))
		require.NoError(t, err)
		require.NotEqual(t, uuid.Nil, resp.ID)
	})

	t.Run("Success/WebP", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client)

		// WebP: RIFF + 4-byte size + WEBP + padding.
		data := append([]byte("RIFF"), make([]byte, 4)...)
		data = append(data, []byte("WEBP")...)
		data = append(data, make([]byte, 64)...)
		resp, err := client.UploadChatFile(ctx, firstUser.OrganizationID, "image/webp", "test.webp", bytes.NewReader(data))
		require.NoError(t, err)
		require.NotEqual(t, uuid.Nil, resp.ID)
	})

	t.Run("UnsupportedContentType", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client)

		_, err := client.UploadChatFile(ctx, firstUser.OrganizationID, "text/plain", "test.txt", bytes.NewReader([]byte("hello")))
		requireSDKError(t, err, http.StatusBadRequest)
	})

	t.Run("SVGBlocked", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client)

		_, err := client.UploadChatFile(ctx, firstUser.OrganizationID, "image/svg+xml", "test.svg", bytes.NewReader([]byte("<svg></svg>")))
		requireSDKError(t, err, http.StatusBadRequest)
	})

	t.Run("ContentSniffingRejects", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client)

		// Header says PNG but body is plain text.
		_, err := client.UploadChatFile(ctx, firstUser.OrganizationID, "image/png", "test.png", bytes.NewReader([]byte("hello world")))
		requireSDKError(t, err, http.StatusBadRequest)
	})

	t.Run("TooLarge", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client)

		// 10 MB + 1 byte, with valid PNG header to pass MIME check.
		data := make([]byte, 10<<20+1)
		copy(data, []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A})
		_, err := client.UploadChatFile(ctx, firstUser.OrganizationID, "image/png", "test.png", bytes.NewReader(data))
		require.Error(t, err)
	})

	t.Run("MissingOrganization", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		coderdtest.CreateFirstUser(t, client)

		data := append([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, make([]byte, 64)...)
		res, err := client.Request(ctx, http.MethodPost, "/api/experimental/chats/files", bytes.NewReader(data), func(r *http.Request) {
			r.Header.Set("Content-Type", "image/png")
		})
		require.NoError(t, err)
		defer res.Body.Close()
		err = codersdk.ReadBodyAsError(res)
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Contains(t, sdkErr.Message, "Missing organization")
	})

	t.Run("InvalidOrganization", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		coderdtest.CreateFirstUser(t, client)

		data := append([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, make([]byte, 64)...)
		res, err := client.Request(ctx, http.MethodPost, "/api/experimental/chats/files?organization=not-a-uuid", bytes.NewReader(data), func(r *http.Request) {
			r.Header.Set("Content-Type", "image/png")
		})
		require.NoError(t, err)
		defer res.Body.Close()
		err = codersdk.ReadBodyAsError(res)
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Contains(t, sdkErr.Message, "Invalid organization ID")
	})

	t.Run("WrongOrganization", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		coderdtest.CreateFirstUser(t, client)

		data := append([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, make([]byte, 64)...)
		_, err := client.UploadChatFile(ctx, uuid.New(), "image/png", "test.png", bytes.NewReader(data))
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		// dbauthz returns 404 or 500 depending on how the org lookup
		// fails; 403 is also possible. Any non-success code is valid.
		require.GreaterOrEqual(t, sdkErr.StatusCode(), http.StatusBadRequest,
			"expected error status, got %d", sdkErr.StatusCode())
	})

	t.Run("Unauthenticated", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client)

		unauthed := codersdk.New(client.URL)
		data := append([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, make([]byte, 64)...)
		_, err := unauthed.UploadChatFile(ctx, firstUser.OrganizationID, "image/png", "test.png", bytes.NewReader(data))
		requireSDKError(t, err, http.StatusUnauthorized)
	})
}

func TestGetChatFile(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client)

		data := append([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, make([]byte, 64)...)
		uploaded, err := client.UploadChatFile(ctx, firstUser.OrganizationID, "image/png", "test.png", bytes.NewReader(data))
		require.NoError(t, err)

		got, contentType, err := client.GetChatFile(ctx, uploaded.ID)
		require.NoError(t, err)
		require.Equal(t, "image/png", contentType)
		require.Equal(t, data, got)
	})

	t.Run("CacheHeaders", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client)

		data := append([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, make([]byte, 64)...)
		uploaded, err := client.UploadChatFile(ctx, firstUser.OrganizationID, "image/png", "test.png", bytes.NewReader(data))
		require.NoError(t, err)

		res, err := client.Request(ctx, http.MethodGet,
			fmt.Sprintf("/api/experimental/chats/files/%s", uploaded.ID), nil)
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
		require.Equal(t, "private, max-age=31536000, immutable", res.Header.Get("Cache-Control"))
		require.Contains(t, res.Header.Get("Content-Disposition"), "inline")
		require.Contains(t, res.Header.Get("Content-Disposition"), "test.png")
	})

	t.Run("LongFilename", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client)

		longName := strings.Repeat("a", 300) + ".png"
		data := append([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, make([]byte, 64)...)
		uploaded, err := client.UploadChatFile(ctx, firstUser.OrganizationID, "image/png", longName, bytes.NewReader(data))
		require.NoError(t, err)

		res, err := client.Request(ctx, http.MethodGet,
			fmt.Sprintf("/api/experimental/chats/files/%s", uploaded.ID), nil)
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
		// Filename should be truncated to maxChatFileName (255) bytes.
		cd := res.Header.Get("Content-Disposition")
		require.Contains(t, cd, "inline")
		require.Contains(t, cd, strings.Repeat("a", 255))
		require.NotContains(t, cd, strings.Repeat("a", 256))
	})

	t.Run("UnicodeFilename", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client)

		// Upload with a non-ASCII filename using RFC 5987 encoding,
		// which is what the frontend sends for Unicode filenames.
		data := append([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, make([]byte, 64)...)
		uploaded, err := client.UploadChatFile(ctx, firstUser.OrganizationID, "image/png", "スクリーンショット.png", bytes.NewReader(data))
		require.NoError(t, err)

		res, err := client.Request(ctx, http.MethodGet,
			fmt.Sprintf("/api/experimental/chats/files/%s", uploaded.ID), nil)
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
		cd := res.Header.Get("Content-Disposition")
		require.Contains(t, cd, "inline")
		_, params, err := mime.ParseMediaType(cd)
		require.NoError(t, err)
		require.Equal(t, "スクリーンショット.png", params["filename"])
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		coderdtest.CreateFirstUser(t, client)

		_, _, err := client.GetChatFile(ctx, uuid.New())
		requireSDKError(t, err, http.StatusNotFound)
	})

	t.Run("InvalidUUID", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		coderdtest.CreateFirstUser(t, client)

		res, err := client.Request(ctx, http.MethodGet,
			"/api/experimental/chats/files/not-a-uuid", nil)
		require.NoError(t, err)
		defer res.Body.Close()
		err = codersdk.ReadBodyAsError(res)
		requireSDKError(t, err, http.StatusBadRequest)
	})

	t.Run("OtherUserForbidden", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client)

		data := append([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, make([]byte, 64)...)
		uploaded, err := client.UploadChatFile(ctx, firstUser.OrganizationID, "image/png", "test.png", bytes.NewReader(data))
		require.NoError(t, err)

		otherClient, _ := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)
		_, _, err = otherClient.GetChatFile(ctx, uploaded.ID)
		requireSDKError(t, err, http.StatusNotFound)
	})
}

type chatCostTestFixture struct {
	Client            *codersdk.Client
	DB                database.Store
	ModelConfigID     uuid.UUID
	ChatID            uuid.UUID
	EarliestCreatedAt time.Time
	LatestCreatedAt   time.Time
}

// safeOptions returns an explicit time window around the fixture messages to
// avoid app-time/database-time boundary flakes in summary tests.
func (f chatCostTestFixture) safeOptions() codersdk.ChatCostSummaryOptions {
	return codersdk.ChatCostSummaryOptions{
		StartDate: f.EarliestCreatedAt.Add(-time.Minute),
		EndDate:   f.LatestCreatedAt.Add(time.Minute),
	}
}

func seedChatCostFixture(t *testing.T) chatCostTestFixture {
	t.Helper()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, db := newChatClientWithDatabase(t)
	firstUser := coderdtest.CreateFirstUser(t, client)
	modelConfig := createChatModelConfig(t, client)

	chat, err := db.InsertChat(dbauthz.AsSystemRestricted(ctx), database.InsertChatParams{
		OwnerID:           firstUser.UserID,
		LastModelConfigID: modelConfig.ID,
		Title:             "test chat",
	})
	require.NoError(t, err)

	results, err := db.InsertChatMessages(dbauthz.AsSystemRestricted(ctx), database.InsertChatMessagesParams{
		ChatID:              chat.ID,
		CreatedBy:           []uuid.UUID{uuid.Nil, uuid.Nil},
		ModelConfigID:       []uuid.UUID{modelConfig.ID, modelConfig.ID},
		Role:                []database.ChatMessageRole{"assistant", "assistant"},
		Content:             []string{"null", "null"},
		ContentVersion:      []int16{0, 0},
		Visibility:          []database.ChatMessageVisibility{database.ChatMessageVisibilityBoth, database.ChatMessageVisibilityBoth},
		InputTokens:         []int64{100, 100},
		OutputTokens:        []int64{50, 50},
		TotalTokens:         []int64{0, 0},
		ReasoningTokens:     []int64{0, 0},
		CacheCreationTokens: []int64{0, 0},
		CacheReadTokens:     []int64{0, 0},
		ContextLimit:        []int64{0, 0},
		Compressed:          []bool{false, false},
		TotalCostMicros:     []int64{500, 500},
		RuntimeMs:           []int64{0, 0},
	})
	require.NoError(t, err)
	require.Len(t, results, 2)
	earliestCreatedAt := results[0].CreatedAt
	latestCreatedAt := results[0].CreatedAt
	for _, msg := range results {
		if msg.CreatedAt.Before(earliestCreatedAt) {
			earliestCreatedAt = msg.CreatedAt
		}
		if msg.CreatedAt.After(latestCreatedAt) {
			latestCreatedAt = msg.CreatedAt
		}
	}

	return chatCostTestFixture{
		Client:            client,
		DB:                db,
		ModelConfigID:     modelConfig.ID,
		ChatID:            chat.ID,
		EarliestCreatedAt: earliestCreatedAt,
		LatestCreatedAt:   latestCreatedAt,
	}
}

func assertChatCostSummary(t *testing.T, summary codersdk.ChatCostSummary, modelConfigID, chatID uuid.UUID) {
	t.Helper()

	require.Equal(t, int64(1000), summary.TotalCostMicros)
	require.Equal(t, int64(2), summary.PricedMessageCount)
	require.Equal(t, int64(0), summary.UnpricedMessageCount)
	require.Equal(t, int64(200), summary.TotalInputTokens)
	require.Equal(t, int64(100), summary.TotalOutputTokens)

	require.Len(t, summary.ByModel, 1)
	require.Equal(t, modelConfigID, summary.ByModel[0].ModelConfigID)
	require.Equal(t, int64(1000), summary.ByModel[0].TotalCostMicros)
	require.Equal(t, int64(2), summary.ByModel[0].MessageCount)

	require.Len(t, summary.ByChat, 1)
	require.Equal(t, chatID, summary.ByChat[0].RootChatID)
	require.Equal(t, int64(1000), summary.ByChat[0].TotalCostMicros)
	require.Equal(t, int64(2), summary.ByChat[0].MessageCount)
}

func TestChatCostSummary(t *testing.T) {
	t.Parallel()

	t.Run("BasicSummary", func(t *testing.T) {
		t.Parallel()

		f := seedChatCostFixture(t)
		ctx := testutil.Context(t, testutil.WaitLong)

		// Use a window derived from DB timestamps to avoid time boundary flakes.
		summary, err := f.Client.GetChatCostSummary(ctx, "me", f.safeOptions())
		require.NoError(t, err)
		assertChatCostSummary(t, summary, f.ModelConfigID, f.ChatID)
	})
}

func TestChatCostSummary_AfterModelDeletion(t *testing.T) {
	t.Parallel()

	f := seedChatCostFixture(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	options := f.safeOptions()

	// Baseline: use DB-derived timestamps to avoid time boundary flakes.
	summary, err := f.Client.GetChatCostSummary(ctx, "me", options)
	require.NoError(t, err)
	assertChatCostSummary(t, summary, f.ModelConfigID, f.ChatID)

	// Soft-delete the model config.
	err = f.Client.DeleteChatModelConfig(ctx, f.ModelConfigID)
	require.NoError(t, err)

	// Costs must survive the deletion unchanged within the same safe window.
	summary, err = f.Client.GetChatCostSummary(ctx, "me", options)
	require.NoError(t, err)
	assertChatCostSummary(t, summary, f.ModelConfigID, f.ChatID)
}

func TestChatCostSummary_AdminDrilldown(t *testing.T) {
	t.Parallel()

	seedCtx := testutil.Context(t, testutil.WaitLong)
	client, db := newChatClientWithDatabase(t)
	firstUser := coderdtest.CreateFirstUser(t, client)
	memberClient, member := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)
	modelConfig := createChatModelConfig(t, client)

	chat, err := db.InsertChat(dbauthz.AsSystemRestricted(seedCtx), database.InsertChatParams{
		OwnerID:           member.ID,
		LastModelConfigID: modelConfig.ID,
		Title:             "member chat",
	})
	require.NoError(t, err)

	results, err := db.InsertChatMessages(dbauthz.AsSystemRestricted(seedCtx), database.InsertChatMessagesParams{
		ChatID:              chat.ID,
		CreatedBy:           []uuid.UUID{uuid.Nil},
		ModelConfigID:       []uuid.UUID{modelConfig.ID},
		Role:                []database.ChatMessageRole{"assistant"},
		Content:             []string{"null"},
		ContentVersion:      []int16{0},
		Visibility:          []database.ChatMessageVisibility{database.ChatMessageVisibilityBoth},
		InputTokens:         []int64{200},
		OutputTokens:        []int64{100},
		TotalTokens:         []int64{0},
		ReasoningTokens:     []int64{0},
		CacheCreationTokens: []int64{0},
		CacheReadTokens:     []int64{0},
		ContextLimit:        []int64{0},
		Compressed:          []bool{false},
		TotalCostMicros:     []int64{750},
		RuntimeMs:           []int64{0},
	})
	require.NoError(t, err)
	message := results[0]
	options := codersdk.ChatCostSummaryOptions{
		// Pad the DB-assigned timestamp so the query window cannot race it.
		StartDate: message.CreatedAt.Add(-time.Minute),
		EndDate:   message.CreatedAt.Add(time.Minute),
	}

	t.Run("AdminCanDrilldown", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		summary, err := client.GetChatCostSummary(ctx, member.ID.String(), options)
		require.NoError(t, err)
		require.Equal(t, int64(750), summary.TotalCostMicros)
		require.Equal(t, int64(1), summary.PricedMessageCount)
	})

	t.Run("MemberCannotDrilldownOtherUser", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		_, err := memberClient.GetChatCostSummary(ctx, firstUser.UserID.String(), options)
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
	})
}

func TestChatCostUsers(t *testing.T) {
	t.Parallel()

	seedCtx := testutil.Context(t, testutil.WaitLong)
	client, db := newChatClientWithDatabase(t)
	firstUser := coderdtest.CreateFirstUser(t, client)
	memberClient, member := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)
	firstUserRecord, err := db.GetUserByID(dbauthz.AsSystemRestricted(seedCtx), firstUser.UserID)
	require.NoError(t, err)
	modelConfig := createChatModelConfig(t, client)

	adminChat, err := db.InsertChat(dbauthz.AsSystemRestricted(seedCtx), database.InsertChatParams{
		OwnerID:           firstUser.UserID,
		LastModelConfigID: modelConfig.ID,
		Title:             "admin chat",
	})
	require.NoError(t, err)
	_, err = db.InsertChatMessages(dbauthz.AsSystemRestricted(seedCtx), database.InsertChatMessagesParams{
		ChatID:              adminChat.ID,
		CreatedBy:           []uuid.UUID{uuid.Nil},
		ModelConfigID:       []uuid.UUID{modelConfig.ID},
		Role:                []database.ChatMessageRole{"assistant"},
		Content:             []string{"null"},
		ContentVersion:      []int16{0},
		Visibility:          []database.ChatMessageVisibility{database.ChatMessageVisibilityBoth},
		InputTokens:         []int64{100},
		OutputTokens:        []int64{50},
		TotalTokens:         []int64{0},
		ReasoningTokens:     []int64{0},
		CacheCreationTokens: []int64{0},
		CacheReadTokens:     []int64{0},
		ContextLimit:        []int64{0},
		Compressed:          []bool{false},
		TotalCostMicros:     []int64{300},
		RuntimeMs:           []int64{0},
	})
	require.NoError(t, err)

	memberChat, err := db.InsertChat(dbauthz.AsSystemRestricted(seedCtx), database.InsertChatParams{
		OwnerID:           member.ID,
		LastModelConfigID: modelConfig.ID,
		Title:             "member chat",
	})
	require.NoError(t, err)
	_, err = db.InsertChatMessages(dbauthz.AsSystemRestricted(seedCtx), database.InsertChatMessagesParams{
		ChatID:              memberChat.ID,
		CreatedBy:           []uuid.UUID{uuid.Nil},
		ModelConfigID:       []uuid.UUID{modelConfig.ID},
		Role:                []database.ChatMessageRole{"assistant"},
		Content:             []string{"null"},
		ContentVersion:      []int16{0},
		Visibility:          []database.ChatMessageVisibility{database.ChatMessageVisibilityBoth},
		InputTokens:         []int64{200},
		OutputTokens:        []int64{100},
		TotalTokens:         []int64{0},
		ReasoningTokens:     []int64{0},
		CacheCreationTokens: []int64{0},
		CacheReadTokens:     []int64{0},
		ContextLimit:        []int64{0},
		Compressed:          []bool{false},
		TotalCostMicros:     []int64{800},
		RuntimeMs:           []int64{0},
	})
	require.NoError(t, err)

	t.Run("AdminCanListUsers", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		resp, err := client.GetChatCostUsers(ctx, codersdk.ChatCostUsersOptions{})
		require.NoError(t, err)
		require.Equal(t, int64(2), resp.Count)
		require.Len(t, resp.Users, 2)
		require.Equal(t, member.ID, resp.Users[0].UserID)
		require.Equal(t, member.Username, resp.Users[0].Username)
		require.Equal(t, int64(800), resp.Users[0].TotalCostMicros)
		require.Equal(t, int64(1), resp.Users[0].MessageCount)
		require.Equal(t, int64(1), resp.Users[0].ChatCount)
		require.Equal(t, firstUser.UserID, resp.Users[1].UserID)
		require.Equal(t, firstUserRecord.Username, resp.Users[1].Username)
		require.Equal(t, int64(300), resp.Users[1].TotalCostMicros)
	})

	t.Run("AdminCanFilterAndPaginateUsers", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		resp, err := client.GetChatCostUsers(ctx, codersdk.ChatCostUsersOptions{
			Username: member.Username,
			Pagination: codersdk.Pagination{
				Limit:  1,
				Offset: 0,
			},
		})
		require.NoError(t, err)
		require.Equal(t, int64(1), resp.Count)
		require.Len(t, resp.Users, 1)
		require.Equal(t, member.ID, resp.Users[0].UserID)
		require.Equal(t, member.Username, resp.Users[0].Username)
	})

	t.Run("MemberCannotListUsers", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		_, err := memberClient.GetChatCostUsers(ctx, codersdk.ChatCostUsersOptions{})
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusForbidden, sdkErr.StatusCode())
	})
}

func TestChatCostSummary_DateRange(t *testing.T) {
	t.Parallel()

	seedCtx := testutil.Context(t, testutil.WaitLong)
	client, db := newChatClientWithDatabase(t)
	firstUser := coderdtest.CreateFirstUser(t, client)
	modelConfig := createChatModelConfig(t, client)

	chat, err := db.InsertChat(dbauthz.AsSystemRestricted(seedCtx), database.InsertChatParams{
		OwnerID:           firstUser.UserID,
		LastModelConfigID: modelConfig.ID,
		Title:             "date range test",
	})
	require.NoError(t, err)

	_, err = db.InsertChatMessages(dbauthz.AsSystemRestricted(seedCtx), database.InsertChatMessagesParams{
		ChatID:              chat.ID,
		CreatedBy:           []uuid.UUID{uuid.Nil},
		ModelConfigID:       []uuid.UUID{modelConfig.ID},
		Role:                []database.ChatMessageRole{"assistant"},
		Content:             []string{"null"},
		ContentVersion:      []int16{0},
		Visibility:          []database.ChatMessageVisibility{database.ChatMessageVisibilityBoth},
		InputTokens:         []int64{100},
		OutputTokens:        []int64{50},
		TotalTokens:         []int64{0},
		ReasoningTokens:     []int64{0},
		CacheCreationTokens: []int64{0},
		CacheReadTokens:     []int64{0},
		ContextLimit:        []int64{0},
		Compressed:          []bool{false},
		TotalCostMicros:     []int64{500},
		RuntimeMs:           []int64{0},
	})
	require.NoError(t, err)

	now := time.Now()

	t.Run("MessageInRange", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		summary, err := client.GetChatCostSummary(ctx, "me", codersdk.ChatCostSummaryOptions{
			StartDate: now.Add(-time.Hour),
			EndDate:   now.Add(time.Hour),
		})
		require.NoError(t, err)
		require.Equal(t, int64(500), summary.TotalCostMicros)
		require.Equal(t, int64(1), summary.PricedMessageCount)
	})

	t.Run("MessageOutOfRange", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		summary, err := client.GetChatCostSummary(ctx, "me", codersdk.ChatCostSummaryOptions{
			StartDate: now.Add(time.Hour),
			EndDate:   now.Add(2 * time.Hour),
		})
		require.NoError(t, err)
		require.Equal(t, int64(0), summary.TotalCostMicros)
		require.Equal(t, int64(0), summary.PricedMessageCount)
	})
}

func TestChatCostSummary_UnpricedMessages(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, db := newChatClientWithDatabase(t)
	firstUser := coderdtest.CreateFirstUser(t, client)
	modelConfig := createChatModelConfig(t, client)

	chat, err := db.InsertChat(dbauthz.AsSystemRestricted(ctx), database.InsertChatParams{
		OwnerID:           firstUser.UserID,
		LastModelConfigID: modelConfig.ID,
		Title:             "unpriced test",
	})
	require.NoError(t, err)

	pricedResults, err := db.InsertChatMessages(dbauthz.AsSystemRestricted(ctx), database.InsertChatMessagesParams{
		ChatID:              chat.ID,
		CreatedBy:           []uuid.UUID{uuid.Nil},
		ModelConfigID:       []uuid.UUID{modelConfig.ID},
		Role:                []database.ChatMessageRole{"assistant"},
		Content:             []string{"null"},
		ContentVersion:      []int16{0},
		Visibility:          []database.ChatMessageVisibility{database.ChatMessageVisibilityBoth},
		InputTokens:         []int64{100},
		OutputTokens:        []int64{50},
		TotalTokens:         []int64{0},
		ReasoningTokens:     []int64{0},
		CacheCreationTokens: []int64{0},
		CacheReadTokens:     []int64{0},
		ContextLimit:        []int64{0},
		Compressed:          []bool{false},
		TotalCostMicros:     []int64{500},
		RuntimeMs:           []int64{0},
	})
	require.NoError(t, err)
	pricedMessage := pricedResults[0]

	unpricedResults, err := db.InsertChatMessages(dbauthz.AsSystemRestricted(ctx), database.InsertChatMessagesParams{
		ChatID:              chat.ID,
		CreatedBy:           []uuid.UUID{uuid.Nil},
		ModelConfigID:       []uuid.UUID{modelConfig.ID},
		Role:                []database.ChatMessageRole{"assistant"},
		Content:             []string{"null"},
		ContentVersion:      []int16{0},
		Visibility:          []database.ChatMessageVisibility{database.ChatMessageVisibilityBoth},
		InputTokens:         []int64{200},
		OutputTokens:        []int64{75},
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
	unpricedMessage := unpricedResults[0]

	earliestCreatedAt := pricedMessage.CreatedAt
	latestCreatedAt := pricedMessage.CreatedAt
	if unpricedMessage.CreatedAt.Before(earliestCreatedAt) {
		earliestCreatedAt = unpricedMessage.CreatedAt
	}
	if unpricedMessage.CreatedAt.After(latestCreatedAt) {
		latestCreatedAt = unpricedMessage.CreatedAt
	}
	options := codersdk.ChatCostSummaryOptions{
		// Pad the DB-assigned timestamps to avoid time boundary flakes.
		StartDate: earliestCreatedAt.Add(-time.Minute),
		EndDate:   latestCreatedAt.Add(time.Minute),
	}

	summary, err := client.GetChatCostSummary(ctx, "me", options)
	require.NoError(t, err)

	require.Equal(t, int64(500), summary.TotalCostMicros)
	require.Equal(t, int64(1), summary.PricedMessageCount)
	require.Equal(t, int64(1), summary.UnpricedMessageCount)
	require.Equal(t, int64(300), summary.TotalInputTokens)
	require.Equal(t, int64(125), summary.TotalOutputTokens)
}

func requireChatModelPricing(
	t *testing.T,
	actual *codersdk.ChatModelCallConfig,
	expected *codersdk.ChatModelCallConfig,
) {
	t.Helper()
	require.NotNil(t, actual)
	require.NotNil(t, expected)

	require.NotNil(t, actual.Cost)
	require.NotNil(t, expected.Cost)
	require.NotNil(t, actual.Cost.InputPricePerMillionTokens)
	require.NotNil(t, actual.Cost.OutputPricePerMillionTokens)
	require.NotNil(t, actual.Cost.CacheReadPricePerMillionTokens)
	require.NotNil(t, actual.Cost.CacheWritePricePerMillionTokens)

	require.True(t, expected.Cost.InputPricePerMillionTokens.Equal(*actual.Cost.InputPricePerMillionTokens))
	require.True(t, expected.Cost.OutputPricePerMillionTokens.Equal(*actual.Cost.OutputPricePerMillionTokens))
	require.True(t, expected.Cost.CacheReadPricePerMillionTokens.Equal(*actual.Cost.CacheReadPricePerMillionTokens))
	require.True(t, expected.Cost.CacheWritePricePerMillionTokens.Equal(*actual.Cost.CacheWritePricePerMillionTokens))
}

func decRef(value string) *decimal.Decimal {
	d := decimal.RequireFromString(value)
	return &d
}

func TestWatchChatDesktop(t *testing.T) {
	t.Parallel()

	t.Run("NoWorkspace", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client)
		_ = createChatModelConfig(t, client)

		createdChat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "desktop no workspace test",
				},
			},
		})
		require.NoError(t, err)

		// Try to connect to the desktop endpoint — should fail because
		// chat has no workspace.
		res, err := client.Request(
			ctx,
			http.MethodGet,
			fmt.Sprintf("/api/experimental/chats/%s/stream/desktop", createdChat.ID),
			nil,
		)
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusBadRequest, res.StatusCode)
	})
}

func createChatModelConfig(t *testing.T, client *codersdk.Client) codersdk.ChatModelConfig {
	t.Helper()

	ctx := testutil.Context(t, testutil.WaitLong)
	_, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
		Provider: "openai",
		APIKey:   "test-api-key",
	})
	require.NoError(t, err)

	contextLimit := int64(4096)
	isDefault := true
	modelConfig, err := client.CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
		Provider:     "openai",
		Model:        "gpt-4o-mini",
		ContextLimit: &contextLimit,
		IsDefault:    &isDefault,
	})
	require.NoError(t, err)
	return modelConfig
}

//nolint:tparallel,paralleltest // Subtests share a single coderdtest instance.
func TestChatSystemPrompt(t *testing.T) {
	t.Parallel()

	adminClient := newChatClient(t)
	firstUser := coderdtest.CreateFirstUser(t, adminClient)
	memberClient, _ := coderdtest.CreateAnotherUser(t, adminClient, firstUser.OrganizationID)

	t.Run("ReturnsEmptyWhenUnset", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitLong)

		resp, err := adminClient.GetChatSystemPrompt(ctx)
		require.NoError(t, err)
		require.Equal(t, "", resp.SystemPrompt)
	})

	t.Run("AdminCanSet", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitLong)

		err := adminClient.UpdateChatSystemPrompt(ctx, codersdk.ChatSystemPrompt{
			SystemPrompt: "You are a helpful coding assistant.",
		})
		require.NoError(t, err)

		resp, err := adminClient.GetChatSystemPrompt(ctx)
		require.NoError(t, err)
		require.Equal(t, "You are a helpful coding assistant.", resp.SystemPrompt)
	})

	t.Run("AdminCanUnset", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitLong)

		// Unset by sending an empty string.
		err := adminClient.UpdateChatSystemPrompt(ctx, codersdk.ChatSystemPrompt{
			SystemPrompt: "",
		})
		require.NoError(t, err)

		resp, err := adminClient.GetChatSystemPrompt(ctx)
		require.NoError(t, err)
		require.Equal(t, "", resp.SystemPrompt)
	})

	t.Run("NonAdminFails", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitLong)

		err := memberClient.UpdateChatSystemPrompt(ctx, codersdk.ChatSystemPrompt{
			SystemPrompt: "This should fail.",
		})
		requireSDKError(t, err, http.StatusNotFound)
	})

	t.Run("UnauthenticatedFails", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		anonClient := codersdk.New(adminClient.URL)
		_, err := anonClient.GetChatSystemPrompt(ctx)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusUnauthorized, sdkErr.StatusCode())
	})

	t.Run("TooLong", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitLong)

		tooLong := strings.Repeat("a", 131073)
		err := adminClient.UpdateChatSystemPrompt(ctx, codersdk.ChatSystemPrompt{
			SystemPrompt: tooLong,
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "System prompt exceeds maximum length.", sdkErr.Message)
	})
}

func TestChatDesktopEnabled(t *testing.T) {
	t.Parallel()

	t.Run("ReturnsFalseWhenUnset", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		adminClient := newChatClient(t)
		coderdtest.CreateFirstUser(t, adminClient)

		resp, err := adminClient.GetChatDesktopEnabled(ctx)
		require.NoError(t, err)
		require.False(t, resp.EnableDesktop)
	})

	t.Run("AdminCanSetTrue", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		adminClient := newChatClient(t)
		coderdtest.CreateFirstUser(t, adminClient)

		err := adminClient.UpdateChatDesktopEnabled(ctx, codersdk.UpdateChatDesktopEnabledRequest{
			EnableDesktop: true,
		})
		require.NoError(t, err)

		resp, err := adminClient.GetChatDesktopEnabled(ctx)
		require.NoError(t, err)
		require.True(t, resp.EnableDesktop)
	})

	t.Run("AdminCanSetFalse", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		adminClient := newChatClient(t)
		coderdtest.CreateFirstUser(t, adminClient)

		// Set true first, then set false.
		err := adminClient.UpdateChatDesktopEnabled(ctx, codersdk.UpdateChatDesktopEnabledRequest{
			EnableDesktop: true,
		})
		require.NoError(t, err)

		err = adminClient.UpdateChatDesktopEnabled(ctx, codersdk.UpdateChatDesktopEnabledRequest{
			EnableDesktop: false,
		})
		require.NoError(t, err)

		resp, err := adminClient.GetChatDesktopEnabled(ctx)
		require.NoError(t, err)
		require.False(t, resp.EnableDesktop)
	})

	t.Run("NonAdminCanRead", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		adminClient := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, adminClient)
		memberClient, _ := coderdtest.CreateAnotherUser(t, adminClient, firstUser.OrganizationID)

		err := adminClient.UpdateChatDesktopEnabled(ctx, codersdk.UpdateChatDesktopEnabledRequest{
			EnableDesktop: true,
		})
		require.NoError(t, err)

		resp, err := memberClient.GetChatDesktopEnabled(ctx)
		require.NoError(t, err)
		require.True(t, resp.EnableDesktop)
	})

	t.Run("NonAdminWriteFails", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		adminClient := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, adminClient)
		memberClient, _ := coderdtest.CreateAnotherUser(t, adminClient, firstUser.OrganizationID)

		err := memberClient.UpdateChatDesktopEnabled(ctx, codersdk.UpdateChatDesktopEnabledRequest{
			EnableDesktop: true,
		})
		requireSDKError(t, err, http.StatusForbidden)
	})

	t.Run("UnauthenticatedFails", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		adminClient := newChatClient(t)
		coderdtest.CreateFirstUser(t, adminClient)

		anonClient := codersdk.New(adminClient.URL)
		_, err := anonClient.GetChatDesktopEnabled(ctx)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusUnauthorized, sdkErr.StatusCode())
	})
}

func requireSDKError(t *testing.T, err error, expectedStatus int) *codersdk.Error {
	t.Helper()

	var sdkErr *codersdk.Error
	require.ErrorAs(t, err, &sdkErr)
	require.Equal(t, expectedStatus, sdkErr.StatusCode())
	return sdkErr
}

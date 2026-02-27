package coderd_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/externalauth"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
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
		require.Nil(t, chat.WorkspaceAgentID)
		require.NotNil(t, chat.RootChatID)
		require.Equal(t, chat.ID, *chat.RootChatID)

		chatWithMessages, err := client.GetChat(ctx, chat.ID)
		require.NoError(t, err)
		require.Equal(t, chat.ID, chatWithMessages.Chat.ID)

		foundUserMessage := false
		for _, message := range chatWithMessages.Messages {
			if message.Role != "user" {
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

		chatWithMessages, err := client.GetChat(ctx, chat.ID)
		require.NoError(t, err)
		for _, message := range chatWithMessages.Messages {
			require.NotEqual(t, "system", message.Role)
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
		require.NotNil(t, chat.WorkspaceAgentID)
		require.Equal(t, workspaceBuild.Agents[0].ID, *chat.WorkspaceAgentID)
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

		chats, err := client.ListChats(ctx)
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
			require.Nil(t, chat.WorkspaceAgentID)
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

		memberChats, err := memberClient.ListChats(ctx)
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
		_, err := unauthenticatedClient.ListChats(ctx)
		requireSDKError(t, err, http.StatusUnauthorized)
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

	t.Run("ForbiddenForOrganizationMember", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		adminClient := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, adminClient)
		memberClient, _ := coderdtest.CreateAnotherUser(t, adminClient, firstUser.OrganizationID)

		_, err := memberClient.ListChatModelConfigs(ctx)
		requireSDKError(t, err, http.StatusForbidden)
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
		modelConfig, err := client.CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
			Provider:     "openai",
			Model:        "gpt-4o-mini",
			ContextLimit: &contextLimit,
			IsDefault:    &isDefault,
		})
		require.NoError(t, err)
		require.NotEqual(t, uuid.Nil, modelConfig.ID)
		require.Equal(t, "openai", modelConfig.Provider)
		require.Equal(t, "gpt-4o-mini", modelConfig.Model)
		require.EqualValues(t, 4096, modelConfig.ContextLimit)
		require.True(t, modelConfig.IsDefault)
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
		updated, err := client.UpdateChatModelConfig(ctx, modelConfig.ID, codersdk.UpdateChatModelConfigRequest{
			DisplayName:  "GPT-4o Mini Updated",
			ContextLimit: &contextLimit,
		})
		require.NoError(t, err)
		require.Equal(t, modelConfig.ID, updated.ID)
		require.Equal(t, "GPT-4o Mini Updated", updated.DisplayName)
		require.EqualValues(t, 8192, updated.ContextLimit)
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

		chatWithMessages, err := client.GetChat(ctx, createdChat.ID)
		require.NoError(t, err)
		require.Equal(t, createdChat.ID, chatWithMessages.Chat.ID)
		require.Equal(t, firstUser.UserID, chatWithMessages.Chat.OwnerID)
		require.Equal(t, modelConfig.ID, chatWithMessages.Chat.LastModelConfigID)
		require.Equal(t, "get chat route payload", chatWithMessages.Chat.Title)
		require.NotZero(t, chatWithMessages.Chat.CreatedAt)
		require.NotZero(t, chatWithMessages.Chat.UpdatedAt)
		require.NotEmpty(t, chatWithMessages.Messages)
		require.Empty(t, chatWithMessages.QueuedMessages)

		foundUserMessage := false
		for _, message := range chatWithMessages.Messages {
			require.Equal(t, createdChat.ID, message.ChatID)
			require.NotEqual(t, "system", message.Role)
			for _, part := range message.Content {
				if message.Role == "user" &&
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

func TestDeleteChat(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client)
		_ = createChatModelConfig(t, client)

		chatToDelete, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "delete me",
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

		chatsBeforeDelete, err := client.ListChats(ctx)
		require.NoError(t, err)
		require.Len(t, chatsBeforeDelete, 2)

		err = client.DeleteChat(ctx, chatToDelete.ID)
		require.NoError(t, err)

		_, err = client.GetChat(ctx, chatToDelete.ID)
		requireSDKError(t, err, http.StatusNotFound)

		chatsAfterDelete, err := client.ListChats(ctx)
		require.NoError(t, err)
		require.Len(t, chatsAfterDelete, 1)
		require.Equal(t, chatToKeep.ID, chatsAfterDelete[0].ID)
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client)

		err := client.DeleteChat(ctx, uuid.New())
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
				chatWithMessages, getErr := client.GetChat(ctx, chat.ID)
				if getErr != nil {
					return false
				}

				for _, queued := range chatWithMessages.QueuedMessages {
					if queued.ID == created.QueuedMessage.ID &&
						queued.ChatID == chat.ID &&
						hasTextPart(queued.Content, messageText) {
						return true
					}
				}
				for _, message := range chatWithMessages.Messages {
					if message.Role == "user" && hasTextPart(message.Content, messageText) {
						return true
					}
				}
				return false
			}, testutil.WaitLong, testutil.IntervalFast)
		} else {
			require.Nil(t, created.QueuedMessage)
			require.NotNil(t, created.Message)
			require.Equal(t, chat.ID, created.Message.ChatID)
			require.Equal(t, "user", created.Message.Role)
			require.NotZero(t, created.Message.ID)
			require.True(t, hasTextPart(created.Message.Content, messageText))

			require.Eventually(t, func() bool {
				chatWithMessages, getErr := client.GetChat(ctx, chat.ID)
				if getErr != nil {
					return false
				}
				for _, message := range chatWithMessages.Messages {
					if message.ID == created.Message.ID &&
						message.Role == "user" &&
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

		chatWithMessages, err := client.GetChat(ctx, chat.ID)
		require.NoError(t, err)

		var userMessageID int64
		for _, message := range chatWithMessages.Messages {
			if message.Role == "user" {
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
		require.Equal(t, userMessageID, edited.ID)
		require.Equal(t, "user", edited.Role)

		foundEditedText := false
		for _, part := range edited.Content {
			if part.Type == codersdk.ChatMessagePartTypeText && part.Text == "hello after edit" {
				foundEditedText = true
			}
		}
		require.True(t, foundEditedText)

		updatedChat, err := client.GetChat(ctx, chat.ID)
		require.NoError(t, err)
		foundEditedInChat := false
		foundOriginalInChat := false
		for _, message := range updatedChat.Messages {
			if message.Role != "user" {
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

		events, closer, err := client.StreamChat(ctx, chat.ID)
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
					event.Message.Role == "user" &&
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

		noCachedStatus, err := client.GetChatDiffStatus(ctx, noCachedStatusChat.ID)
		require.NoError(t, err)
		require.Equal(t, noCachedStatusChat.ID, noCachedStatus.ChatID)
		require.Nil(t, noCachedStatus.URL)
		require.Nil(t, noCachedStatus.PullRequestState)
		require.False(t, noCachedStatus.ChangesRequested)
		require.Zero(t, noCachedStatus.Additions)
		require.Zero(t, noCachedStatus.Deletions)
		require.Zero(t, noCachedStatus.ChangedFiles)
		require.Nil(t, noCachedStatus.RefreshedAt)
		require.Nil(t, noCachedStatus.StaleAt)

		cachedStatusChat, err := db.InsertChat(dbauthz.AsSystemRestricted(ctx), database.InsertChatParams{
			OwnerID:           user.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "get diff status route cached",
		})
		require.NoError(t, err)

		refreshedAt := time.Date(2026, time.January, 15, 12, 0, 0, 0, time.UTC)
		staleAt := time.Date(2026, time.January, 15, 13, 0, 0, 0, time.UTC)

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

		cachedStatus, err := client.GetChatDiffStatus(ctx, cachedStatusChat.ID)
		require.NoError(t, err)
		require.Equal(t, cachedStatusChat.ID, cachedStatus.ChatID)
		require.NotNil(t, cachedStatus.URL)
		require.Equal(t, "https://github.com/coder/coder/tree/feature%2Fdiff-status", *cachedStatus.URL)
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
		_, err = otherClient.GetChatDiffStatus(ctx, createdChat.ID)
		requireSDKError(t, err, http.StatusNotFound)
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

		queuedMessage, err := db.InsertChatQueuedMessage(
			dbauthz.AsSystemRestricted(ctx),
			database.InsertChatQueuedMessageParams{
				ChatID:  chat.ID,
				Content: []byte(`"queued message for delete route"`),
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

		chatWithMessages, err := client.GetChat(ctx, chat.ID)
		require.NoError(t, err)
		for _, queued := range chatWithMessages.QueuedMessages {
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
		queuedMessage, err := db.InsertChatQueuedMessage(
			dbauthz.AsSystemRestricted(ctx),
			database.InsertChatQueuedMessageParams{
				ChatID:  chat.ID,
				Content: []byte(fmt.Sprintf("%q", queuedText)),
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
		require.Equal(t, "user", promoted.Role)

		foundPromotedText := false
		for _, part := range promoted.Content {
			if part.Type == codersdk.ChatMessagePartTypeText &&
				part.Text == queuedText {
				foundPromotedText = true
				break
			}
		}
		require.True(t, foundPromotedText)

		chatWithMessages, err := client.GetChat(ctx, chat.ID)
		require.NoError(t, err)
		for _, queued := range chatWithMessages.QueuedMessages {
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

func requireSDKError(t *testing.T, err error, expectedStatus int) *codersdk.Error {
	t.Helper()

	var sdkErr *codersdk.Error
	require.ErrorAs(t, err, &sdkErr)
	require.Equal(t, expectedStatus, sdkErr.StatusCode())
	return sdkErr
}

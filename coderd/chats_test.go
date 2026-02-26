package coderd_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestChats(t *testing.T) {
	t.Parallel()

	t.Run("PostChats", func(t *testing.T) {
		t.Parallel()

		t.Run("Success", func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)
			client := coderdtest.New(t, nil)
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
			client := coderdtest.New(t, nil)
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
			adminClient, db := coderdtest.NewWithDatabase(t, nil)
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
			require.Equal(t, "Workspace not found or you do not have access to this resource", sdkErr.Message)
		})

		t.Run("WorkspaceNotFound", func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)
			client := coderdtest.New(t, nil)
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
			require.Equal(t, "Workspace not found or you do not have access to this resource", sdkErr.Message)
		})

		t.Run("WorkspaceSelectsFirstAgent", func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)
			client, db := coderdtest.NewWithDatabase(t, nil)
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
			client := coderdtest.New(t, nil)
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
			client := coderdtest.New(t, nil)
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
			client := coderdtest.New(t, nil)
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
			client := coderdtest.New(t, nil)
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
	})

	t.Run("ListChatModels", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := coderdtest.New(t, nil)
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

	t.Run("ListChatProviders", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := coderdtest.New(t, nil)
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

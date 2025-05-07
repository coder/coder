package coderd_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestChat(t *testing.T) {
	t.Parallel()

	t.Run("ExperimentAgenticChatDisabled", func(t *testing.T) {
		t.Parallel()

		client, _ := coderdtest.NewWithDatabase(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)
		memberClient, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		// Hit the endpoint to get the chat. It should return a 404.
		ctx := testutil.Context(t, testutil.WaitShort)
		_, err := memberClient.ListChats(ctx)
		require.Error(t, err, "list chats should fail")
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr, "request should fail with an SDK error")
		require.Equal(t, http.StatusForbidden, sdkErr.StatusCode())
	})

	t.Run("ChatCRUD", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)
		dv.Experiments = []string{string(codersdk.ExperimentAgenticChat)}
		dv.AI.Value = codersdk.AIConfig{
			Providers: []codersdk.AIProviderConfig{
				{
					Type:    "fake",
					APIKey:  "",
					BaseURL: "http://localhost",
					Models:  []string{"fake-model"},
				},
			},
		}
		client, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{
			DeploymentValues: dv,
		})
		owner := coderdtest.CreateFirstUser(t, client)
		memberClient, memberUser := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		// Seed the database with some data.
		dbChat := dbgen.Chat(t, db, database.Chat{
			OwnerID:   memberUser.ID,
			CreatedAt: dbtime.Now().Add(-time.Hour),
			UpdatedAt: dbtime.Now().Add(-time.Hour),
			Title:     "This is a test chat",
		})
		_ = dbgen.ChatMessage(t, db, database.ChatMessage{
			ChatID:    dbChat.ID,
			CreatedAt: dbtime.Now().Add(-time.Hour),
			Content:   []byte(`[{"content": "Hello world"}]`),
			Model:     "fake model",
			Provider:  "fake",
		})

		ctx := testutil.Context(t, testutil.WaitShort)

		// Listing chats should return the chat we just inserted.
		chats, err := memberClient.ListChats(ctx)
		require.NoError(t, err, "list chats should succeed")
		require.Len(t, chats, 1, "response should have one chat")
		require.Equal(t, dbChat.ID, chats[0].ID, "unexpected chat ID")
		require.Equal(t, dbChat.Title, chats[0].Title, "unexpected chat title")
		require.Equal(t, dbChat.CreatedAt.UTC(), chats[0].CreatedAt.UTC(), "unexpected chat created at")
		require.Equal(t, dbChat.UpdatedAt.UTC(), chats[0].UpdatedAt.UTC(), "unexpected chat updated at")

		// Fetching a single chat by ID should return the same chat.
		chat, err := memberClient.Chat(ctx, dbChat.ID)
		require.NoError(t, err, "get chat should succeed")
		require.Equal(t, chats[0], chat, "get chat should return the same chat")

		// Listing chat messages should return the message we just inserted.
		messages, err := memberClient.ChatMessages(ctx, dbChat.ID)
		require.NoError(t, err, "list chat messages should succeed")
		require.Len(t, messages, 1, "response should have one message")
		require.Equal(t, "Hello world", messages[0].Content, "response should have the correct message content")

		// Creating a new chat will fail because the model does not exist.
		// TODO: Test the message streaming functionality with a mock model.
		// Inserting a chat message will fail due to the model not existing.
		_, err = memberClient.CreateChatMessage(ctx, dbChat.ID, codersdk.CreateChatMessageRequest{
			Model: "echo",
			Message: codersdk.ChatMessage{
				Role:    "user",
				Content: "Hello world",
			},
			Thinking: false,
		})
		require.Error(t, err, "create chat message should fail")
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr, "create chat should fail with an SDK error")
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode(), "create chat should fail with a 400 when model does not exist")

		// Creating a new chat message with malformed content should fail.
		res, err := memberClient.Request(ctx, http.MethodPost, "/api/v2/chats/"+dbChat.ID.String()+"/messages", strings.NewReader(`{malformed json}`))
		require.NoError(t, err)
		defer res.Body.Close()
		apiErr := codersdk.ReadBodyAsError(res)
		require.Contains(t, apiErr.Error(), "Failed to decode chat message")

		_, err = memberClient.CreateChat(ctx)
		require.NoError(t, err, "create chat should succeed")
		chats, err = memberClient.ListChats(ctx)
		require.NoError(t, err, "list chats should succeed")
		require.Len(t, chats, 2, "response should have two chats")
	})
}

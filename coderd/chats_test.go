package coderd_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestChats(t *testing.T) {
	t.Parallel()

	t.Run("Create", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitShort)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			Title: "Test Chat",
		})
		require.NoError(t, err)
		require.Equal(t, "Test Chat", chat.Title)
		require.Equal(t, user.UserID, chat.OwnerID)
		require.Equal(t, codersdk.ChatStatusWaiting, chat.Status)
	})

	t.Run("CreateDefaultTitle", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitShort)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{})
		require.NoError(t, err)
		require.Equal(t, "New Chat", chat.Title)
	})

	t.Run("List", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitShort)

		// Create two chats.
		_, err := client.CreateChat(ctx, codersdk.CreateChatRequest{Title: "Chat 1"})
		require.NoError(t, err)
		_, err = client.CreateChat(ctx, codersdk.CreateChatRequest{Title: "Chat 2"})
		require.NoError(t, err)

		chats, err := client.ListChats(ctx)
		require.NoError(t, err)
		require.Len(t, chats, 2)
	})

	t.Run("Get", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitShort)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{Title: "Test Chat"})
		require.NoError(t, err)

		result, err := client.GetChat(ctx, chat.ID)
		require.NoError(t, err)
		require.Equal(t, chat.ID, result.Chat.ID)
		require.Empty(t, result.Messages)
	})

	t.Run("Delete", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitShort)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{Title: "Test Chat"})
		require.NoError(t, err)

		err = client.DeleteChat(ctx, chat.ID)
		require.NoError(t, err)

		// Verify it's deleted.
		_, err = client.GetChat(ctx, chat.ID)
		require.Error(t, err)
	})

	t.Run("CreateMessage", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitShort)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{Title: "Test Chat"})
		require.NoError(t, err)

		messages, err := client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
			Role:    "user",
			Content: json.RawMessage(`"Hello, AI!"`),
		})
		require.NoError(t, err)
		require.Len(t, messages, 1)
		require.Equal(t, "user", messages[0].Role)

		// Verify message was saved.
		result, err := client.GetChat(ctx, chat.ID)
		require.NoError(t, err)
		require.Len(t, result.Messages, 1)
	})

	t.Run("GetGitChanges", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitShort)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{Title: "Test Chat"})
		require.NoError(t, err)

		// Initially, there should be no git changes.
		changes, err := client.GetChatGitChanges(ctx, chat.ID)
		require.NoError(t, err)
		require.Empty(t, changes)
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitShort)

		// Use a random UUID that doesn't exist.
		randomID := uuid.New()
		_, err := client.GetChat(ctx, randomID)
		require.Error(t, err)
	})
}

package coderd_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func queuedTextContent(t *testing.T, text string) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal([]codersdk.ChatMessagePart{codersdk.ChatMessageText(text)})
	require.NoError(t, err)
	return raw
}

func boolPtr(b bool) *bool { return &b }

// TestPatchChatQueuedMessage covers PATCH /chats/{chat}/queue/{id}: the
// hold and release round trip, the release-from-waiting promotion, and
// the request guards.
func TestPatchChatQueuedMessage(t *testing.T) {
	t.Parallel()

	t.Run("HoldThenReleaseOnErroredChat", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		user := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModel(t, client)

		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    user.OrganizationID,
			OwnerID:           user.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "hold queued message",
			Status:            database.ChatStatusError,
		})
		queued := insertTestChatQueuedMessage(ctx, t, db, chat.ID, queuedTextContent(t, "original"), modelConfig.ID)

		err := client.EditChatQueuedMessage(ctx, chat.ID, queued.ID, codersdk.EditChatQueuedMessageRequest{
			Held: boolPtr(true),
		})
		require.NoError(t, err)

		listed, err := client.GetChatMessages(ctx, chat.ID, nil)
		require.NoError(t, err)
		require.Len(t, listed.QueuedMessages, 1)
		require.NotNil(t, listed.QueuedMessages[0].HeldAt, "held_at is visible in the queue listing")

		// Save: new content and release. The chat is in error, so the
		// row stays queued (E1) instead of being promoted.
		err = client.EditChatQueuedMessage(ctx, chat.ID, queued.ID, codersdk.EditChatQueuedMessageRequest{
			Content: []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "edited"}},
			Held:    boolPtr(false),
		})
		require.NoError(t, err)

		listed, err = client.GetChatMessages(ctx, chat.ID, nil)
		require.NoError(t, err)
		require.Len(t, listed.QueuedMessages, 1, "release from error does not promote")
		saved := listed.QueuedMessages[0]
		require.Nil(t, saved.HeldAt, "release clears held_at")
		require.Len(t, saved.Content, 1)
		require.Equal(t, "edited", saved.Content[0].Text)

		refreshed, err := db.GetChatByID(dbauthz.AsSystemRestricted(ctx), chat.ID)
		require.NoError(t, err)
		require.Equal(t, database.ChatStatusError, refreshed.Status)
	})

	t.Run("ReleaseFromWaitingPromotes", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t, withChatWorkerDisabled)
		user := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModel(t, client)

		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    user.OrganizationID,
			OwnerID:           user.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "release promotes",
			Status:            database.ChatStatusWaiting,
		})
		queued := insertTestChatQueuedMessage(ctx, t, db, chat.ID, queuedTextContent(t, "original"), modelConfig.ID)
		// Waiting with an unheld head is invalid; seed the hold directly
		// so the chat is a legitimate W with a held head.
		_, err := db.UpdateChatQueuedMessageHeld(dbauthz.AsSystemRestricted(ctx), database.UpdateChatQueuedMessageHeldParams{
			ChatID: chat.ID,
			ID:     queued.ID,
			Held:   true,
		})
		require.NoError(t, err)

		err = client.EditChatQueuedMessage(ctx, chat.ID, queued.ID, codersdk.EditChatQueuedMessageRequest{
			Content: []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "edited"}},
			Held:    boolPtr(false),
		})
		require.NoError(t, err)

		// Releasing the held head of an idle chat is a promotion: the
		// edited row is in history and the chat is running.
		listed, err := client.GetChatMessages(ctx, chat.ID, nil)
		require.NoError(t, err)
		require.Empty(t, listed.QueuedMessages, "promoted row is no longer queued")
		require.NotEmpty(t, listed.Messages)
		promoted := listed.Messages[len(listed.Messages)-1]
		require.Equal(t, codersdk.ChatMessageRoleUser, promoted.Role)
		require.Len(t, promoted.Content, 1)
		require.Equal(t, "edited", promoted.Content[0].Text)

		refreshed, err := db.GetChatByID(dbauthz.AsSystemRestricted(ctx), chat.ID)
		require.NoError(t, err)
		require.Equal(t, database.ChatStatusRunning, refreshed.Status)

		// The promoted row is gone; a second hold attempt is a 404, which
		// is what tells a client not to enter edit mode.
		err = client.EditChatQueuedMessage(ctx, chat.ID, queued.ID, codersdk.EditChatQueuedMessageRequest{
			Held: boolPtr(true),
		})
		sdkErr := requireSDKError(t, err, http.StatusNotFound)
		require.Equal(t, "Queued message not found.", sdkErr.Message)
	})

	t.Run("DeleteHeldHeadOnIdleChatStartsNext", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t, withChatWorkerDisabled)
		user := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModel(t, client)

		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    user.OrganizationID,
			OwnerID:           user.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "delete held head",
			Status:            database.ChatStatusWaiting,
		})
		held := insertTestChatQueuedMessage(ctx, t, db, chat.ID, queuedTextContent(t, "held"), modelConfig.ID)
		insertTestChatQueuedMessage(ctx, t, db, chat.ID, queuedTextContent(t, "next"), modelConfig.ID)
		_, err := db.UpdateChatQueuedMessageHeld(dbauthz.AsSystemRestricted(ctx), database.UpdateChatQueuedMessageHeldParams{
			ChatID: chat.ID,
			ID:     held.ID,
			Held:   true,
		})
		require.NoError(t, err)

		// Removing the held head unpauses the row behind it. On an idle
		// chat that row is started, as "send now" would.
		res, err := client.Request(ctx, http.MethodDelete,
			fmt.Sprintf("/api/experimental/chats/%s/queue/%d", chat.ID, held.ID), nil)
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusNoContent, res.StatusCode)

		listed, err := client.GetChatMessages(ctx, chat.ID, nil)
		require.NoError(t, err)
		require.Empty(t, listed.QueuedMessages)
		require.NotEmpty(t, listed.Messages)
		promoted := listed.Messages[len(listed.Messages)-1]
		require.Equal(t, codersdk.ChatMessageRoleUser, promoted.Role)
		require.Equal(t, "next", promoted.Content[0].Text)

		refreshed, err := db.GetChatByID(dbauthz.AsSystemRestricted(ctx), chat.ID)
		require.NoError(t, err)
		require.Equal(t, database.ChatStatusRunning, refreshed.Status)
	})

	t.Run("Guards", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		user := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModel(t, client)

		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    user.OrganizationID,
			OwnerID:           user.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "queued edit guards",
			Status:            database.ChatStatusError,
		})
		queued := insertTestChatQueuedMessage(ctx, t, db, chat.ID, queuedTextContent(t, "original"), modelConfig.ID)

		// Empty request.
		err := client.EditChatQueuedMessage(ctx, chat.ID, queued.ID, codersdk.EditChatQueuedMessageRequest{})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Nothing to edit.", sdkErr.Message)

		// Empty content array. The SDK omits empty slices, so send the
		// raw body.
		res, err := client.Request(ctx, http.MethodPatch,
			fmt.Sprintf("/api/experimental/chats/%s/queue/%d", chat.ID, queued.ID),
			map[string]any{"content": []any{}},
		)
		require.NoError(t, err)
		defer res.Body.Close()
		sdkErr = requireSDKError(t, codersdk.ReadBodyAsError(res), http.StatusBadRequest)
		require.Equal(t, "Content is required.", sdkErr.Message)

		// Invalid ID.
		res, err = client.Request(ctx, http.MethodPatch,
			fmt.Sprintf("/api/experimental/chats/%s/queue/not-an-int", chat.ID),
			codersdk.EditChatQueuedMessageRequest{Held: boolPtr(true)},
		)
		require.NoError(t, err)
		defer res.Body.Close()
		sdkErr = requireSDKError(t, codersdk.ReadBodyAsError(res), http.StatusBadRequest)
		require.Equal(t, "Invalid queued message ID.", sdkErr.Message)

		// Non-owner with update permission on the org's chats.
		adminClientRaw, _ := coderdtest.CreateAnotherUser(t, client.Client, user.OrganizationID, rbac.ScopedRoleOrgAdmin(user.OrganizationID))
		adminClient := codersdk.NewExperimentalClient(adminClientRaw)
		err = adminClient.EditChatQueuedMessage(ctx, chat.ID, queued.ID, codersdk.EditChatQueuedMessageRequest{
			Held: boolPtr(true),
		})
		sdkErr = requireSDKError(t, err, http.StatusForbidden)
		require.Equal(t, "Only the chat owner may edit queued messages.", sdkErr.Message)

		// Non-owner delete: deleting a held head can start the next
		// message under the owner's credentials, so delete is owner-only
		// like promote.
		res, err = adminClientRaw.Request(ctx, http.MethodDelete,
			fmt.Sprintf("/api/experimental/chats/%s/queue/%d", chat.ID, queued.ID), nil)
		require.NoError(t, err)
		defer res.Body.Close()
		sdkErr = requireSDKError(t, codersdk.ReadBodyAsError(res), http.StatusForbidden)
		require.Equal(t, "Only the chat owner may delete queued messages.", sdkErr.Message)

		// Archived chat.
		archived := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    user.OrganizationID,
			OwnerID:           user.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "archived queued edit",
			Status:            database.ChatStatusError,
		})
		archivedQueued := insertTestChatQueuedMessage(ctx, t, db, archived.ID, queuedTextContent(t, "original"), modelConfig.ID)
		_, err = db.ArchiveChatByID(dbauthz.AsSystemRestricted(ctx), archived.ID)
		require.NoError(t, err)
		err = client.EditChatQueuedMessage(ctx, archived.ID, archivedQueued.ID, codersdk.EditChatQueuedMessageRequest{
			Held: boolPtr(true),
		})
		sdkErr = requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Cannot edit queued messages in an archived chat.", sdkErr.Message)
	})
}

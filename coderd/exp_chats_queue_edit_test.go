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

// TestPatchChatQueuedMessage walks the queued-edit flow over HTTP: hold,
// save on a busy chat, resume on a paused chat, delete the paused head,
// and the request guards. State-machine outcomes are proven in
// chatstate; this covers routing, authorization, and error mapping.
func TestPatchChatQueuedMessage(t *testing.T) {
	t.Parallel()

	t.Run("HoldSaveResumeDelete", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t, withChatWorkerDisabled)
		user := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModel(t, client)
		sysCtx := dbauthz.AsSystemRestricted(ctx)

		// Errored chat with two queued rows: hold the head and save it.
		// The chat stays errored; the row keeps its place with new text.
		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID: user.OrganizationID, OwnerID: user.UserID,
			LastModelConfigID: modelConfig.ID, Title: "queued edit", Status: database.ChatStatusError,
		})
		head := insertTestChatQueuedMessage(ctx, t, db, chat.ID, queuedTextContent(t, "original"), modelConfig.ID)
		next := insertTestChatQueuedMessage(ctx, t, db, chat.ID, queuedTextContent(t, "next"), modelConfig.ID)

		require.NoError(t, client.EditChatQueuedMessage(ctx, chat.ID, head.ID, codersdk.EditChatQueuedMessageRequest{Held: boolPtr(true)}))
		listed, err := client.GetChatMessages(ctx, chat.ID, nil)
		require.NoError(t, err)
		require.NotNil(t, listed.QueuedMessages[0].HeldAt, "held_at is visible in the queue listing")

		require.NoError(t, client.EditChatQueuedMessage(ctx, chat.ID, head.ID, codersdk.EditChatQueuedMessageRequest{
			Content: []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "edited"}},
			Held:    boolPtr(false),
		}))
		listed, err = client.GetChatMessages(ctx, chat.ID, nil)
		require.NoError(t, err)
		require.Len(t, listed.QueuedMessages, 2, "release on an errored chat does not promote")
		require.Nil(t, listed.QueuedMessages[0].HeldAt)
		require.Equal(t, "edited", listed.QueuedMessages[0].Content[0].Text)

		// Pause the chat at the head and resume it: the head is sent.
		require.NoError(t, client.EditChatQueuedMessage(ctx, chat.ID, head.ID, codersdk.EditChatQueuedMessageRequest{Held: boolPtr(true)}))
		_, err = db.UpdateChatStatus(sysCtx, database.UpdateChatStatusParams{ID: chat.ID, Status: database.ChatStatusWaiting})
		require.NoError(t, err)
		require.NoError(t, client.EditChatQueuedMessage(ctx, chat.ID, head.ID, codersdk.EditChatQueuedMessageRequest{Held: boolPtr(false)}))
		listed, err = client.GetChatMessages(ctx, chat.ID, nil)
		require.NoError(t, err)
		require.Len(t, listed.QueuedMessages, 1, "the head was sent")
		require.Equal(t, "edited", listed.Messages[len(listed.Messages)-1].Content[0].Text)
		refreshed, err := db.GetChatByID(sysCtx, chat.ID)
		require.NoError(t, err)
		require.Equal(t, database.ChatStatusRunning, refreshed.Status)

		// A hold on a row that was already sent is a 404.
		err = client.EditChatQueuedMessage(ctx, chat.ID, head.ID, codersdk.EditChatQueuedMessageRequest{Held: boolPtr(true)})
		require.Equal(t, "Queued message not found.", requireSDKError(t, err, http.StatusNotFound).Message)

		// Pause at the remaining row and delete it: the chat idles.
		require.NoError(t, client.EditChatQueuedMessage(ctx, chat.ID, next.ID, codersdk.EditChatQueuedMessageRequest{Held: boolPtr(true)}))
		_, err = db.UpdateChatStatus(sysCtx, database.UpdateChatStatusParams{ID: chat.ID, Status: database.ChatStatusWaiting})
		require.NoError(t, err)
		res, err := client.Request(ctx, http.MethodDelete, fmt.Sprintf("/api/experimental/chats/%s/queue/%d", chat.ID, next.ID), nil)
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusNoContent, res.StatusCode)
		refreshed, err = db.GetChatByID(sysCtx, chat.ID)
		require.NoError(t, err)
		require.Equal(t, database.ChatStatusWaiting, refreshed.Status)
	})

	t.Run("Guards", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		user := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModel(t, client)
		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID: user.OrganizationID, OwnerID: user.UserID,
			LastModelConfigID: modelConfig.ID, Title: "queued edit guards", Status: database.ChatStatusError,
		})
		queued := insertTestChatQueuedMessage(ctx, t, db, chat.ID, queuedTextContent(t, "original"), modelConfig.ID)
		path := fmt.Sprintf("/api/experimental/chats/%s/queue/%d", chat.ID, queued.ID)

		err := client.EditChatQueuedMessage(ctx, chat.ID, queued.ID, codersdk.EditChatQueuedMessageRequest{})
		require.Equal(t, "Nothing to edit.", requireSDKError(t, err, http.StatusBadRequest).Message)

		// The SDK omits empty slices, so send the raw body.
		res, err := client.Request(ctx, http.MethodPatch, path, map[string]any{"content": []any{}})
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, "Content is required.", requireSDKError(t, codersdk.ReadBodyAsError(res), http.StatusBadRequest).Message)

		res, err = client.Request(ctx, http.MethodPatch, fmt.Sprintf("/api/experimental/chats/%s/queue/not-an-int", chat.ID), codersdk.EditChatQueuedMessageRequest{Held: boolPtr(true)})
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, "Invalid queued message ID.", requireSDKError(t, codersdk.ReadBodyAsError(res), http.StatusBadRequest).Message)

		// Non-owner with update permission: PATCH and DELETE are
		// owner-only because both can start inference.
		adminRaw, _ := coderdtest.CreateAnotherUser(t, client.Client, user.OrganizationID, rbac.ScopedRoleOrgAdmin(user.OrganizationID))
		err = codersdk.NewExperimentalClient(adminRaw).EditChatQueuedMessage(ctx, chat.ID, queued.ID, codersdk.EditChatQueuedMessageRequest{Held: boolPtr(true)})
		require.Equal(t, "Only the chat owner may edit queued messages.", requireSDKError(t, err, http.StatusForbidden).Message)
		res, err = adminRaw.Request(ctx, http.MethodDelete, path, nil)
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, "Only the chat owner may delete queued messages.", requireSDKError(t, codersdk.ReadBodyAsError(res), http.StatusForbidden).Message)

		_, err = db.ArchiveChatByID(dbauthz.AsSystemRestricted(ctx), chat.ID)
		require.NoError(t, err)
		err = client.EditChatQueuedMessage(ctx, chat.ID, queued.ID, codersdk.EditChatQueuedMessageRequest{Held: boolPtr(true)})
		require.Equal(t, "Cannot edit queued messages in an archived chat.", requireSDKError(t, err, http.StatusBadRequest).Message)
	})
}

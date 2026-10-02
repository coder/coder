package coderd_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/websocket"
)

func queuedTextContent(t *testing.T, text string) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal([]codersdk.ChatMessagePart{codersdk.ChatMessageText(text)})
	require.NoError(t, err)
	return raw
}

const pausedArchiveRefusal = "Cannot archive: a chat in this family is paused at a queued message under edit. Finish editing, send, or remove that message first."

// TestPatchChatQueuedMessage covers routing, authorization, and error
// mapping for the queued-edit endpoint; state outcomes are covered in
// chatstate.
func TestPatchChatQueuedMessage(t *testing.T) {
	t.Parallel()

	t.Run("EditSaveResumeDelete", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t, withChatWorkerDisabled)
		user := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModel(t, client)
		sysCtx := dbauthz.AsSystemRestricted(ctx)

		// Errored chat with two queued rows: edit the head and save it.
		// The chat stays errored.
		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID: user.OrganizationID, OwnerID: user.UserID,
			LastModelConfigID: modelConfig.ID, Title: "queued edit", Status: database.ChatStatusError,
		})
		head := insertTestChatQueuedMessage(ctx, t, db, chat.ID, queuedTextContent(t, "original"), modelConfig.ID)
		next := insertTestChatQueuedMessage(ctx, t, db, chat.ID, queuedTextContent(t, "next"), modelConfig.ID)
		watch, err := client.Dial(ctx, "/api/v2/chats/watch", nil)
		require.NoError(t, err)
		defer watch.Close(websocket.StatusNormalClosure, "done")

		require.NoError(t, client.EditChatQueuedMessage(ctx, chat.ID, head.ID, codersdk.EditChatQueuedMessageRequest{Editing: new(true)}))
		listed, err := client.GetChatMessages(ctx, chat.ID, nil)
		require.NoError(t, err)
		require.NotNil(t, listed.QueuedMessages[0].EditingSince, "editing_since is visible in the queue listing")

		require.NoError(t, client.EditChatQueuedMessage(ctx, chat.ID, head.ID, codersdk.EditChatQueuedMessageRequest{
			Content: []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "edited"}},
			Editing: new(false),
		}))
		listed, err = client.GetChatMessages(ctx, chat.ID, nil)
		require.NoError(t, err)
		require.Len(t, listed.QueuedMessages, 2, "ending the edit on an errored chat does not promote")
		require.Nil(t, listed.QueuedMessages[0].EditingSince)
		require.Equal(t, "edited", listed.QueuedMessages[0].Content[0].Text)
		fetched, err := client.GetChat(ctx, chat.ID)
		require.NoError(t, err)
		require.Equal(t, codersdk.ChatStatusError, fetched.Status, "saving the edit keeps the chat errored")

		// Pause the chat at the head and resume it: the head is sent.
		require.NoError(t, client.EditChatQueuedMessage(ctx, chat.ID, head.ID, codersdk.EditChatQueuedMessageRequest{Editing: new(true)}))
		_, err = db.UpdateChatStatus(sysCtx, database.UpdateChatStatusParams{ID: chat.ID, Status: database.ChatStatusPaused})
		require.NoError(t, err)
		fetched, err = client.GetChat(ctx, chat.ID)
		require.NoError(t, err)
		require.Equal(t, codersdk.ChatStatusPaused, fetched.Status, "paused is visible on the chat itself")
		// Beginning an edit on another row while paused is refused, and
		// so is archiving, with a message that names the paused state.
		err = client.EditChatQueuedMessage(ctx, chat.ID, next.ID, codersdk.EditChatQueuedMessageRequest{Editing: new(true)})
		requireSDKError(t, err, http.StatusConflict)
		err = client.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{Archived: new(true)})
		require.Equal(t, pausedArchiveRefusal, requireSDKError(t, err, http.StatusConflict).Message)
		require.NoError(t, client.EditChatQueuedMessage(ctx, chat.ID, head.ID, codersdk.EditChatQueuedMessageRequest{Editing: new(false)}))
		listed, err = client.GetChatMessages(ctx, chat.ID, nil)
		require.NoError(t, err)
		require.Len(t, listed.QueuedMessages, 1, "the head was sent")
		require.Equal(t, "edited", listed.Messages[len(listed.Messages)-1].Content[0].Text)
		refreshed, err := db.GetChatByID(sysCtx, chat.ID)
		require.NoError(t, err)
		require.Equal(t, database.ChatStatusRunning, refreshed.Status)
		require.Equal(t, codersdk.ChatStatusRunning, waitForChatWatchStatusChangeEvent(ctx, t, watch, chat.ID).Chat.Status,
			"resuming publishes the running status to watchers")

		// Beginning an edit on a row that was already sent is a 404.
		err = client.EditChatQueuedMessage(ctx, chat.ID, head.ID, codersdk.EditChatQueuedMessageRequest{Editing: new(true)})
		require.Equal(t, "Queued message not found.", requireSDKError(t, err, http.StatusNotFound).Message)

		// Pause at the remaining row and delete it: the chat returns to waiting.
		require.NoError(t, client.EditChatQueuedMessage(ctx, chat.ID, next.ID, codersdk.EditChatQueuedMessageRequest{Editing: new(true)}))
		_, err = db.UpdateChatStatus(sysCtx, database.UpdateChatStatusParams{ID: chat.ID, Status: database.ChatStatusPaused})
		require.NoError(t, err)
		res, err := client.Request(ctx, http.MethodDelete, fmt.Sprintf("/api/v2/chats/%s/queue/%d", chat.ID, next.ID), nil)
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusNoContent, res.StatusCode)
		refreshed, err = db.GetChatByID(sysCtx, chat.ID)
		require.NoError(t, err)
		require.Equal(t, database.ChatStatusWaiting, refreshed.Status)
		require.Equal(t, codersdk.ChatStatusWaiting, waitForChatWatchStatusChangeEvent(ctx, t, watch, chat.ID).Chat.Status,
			"deleting the paused head publishes the waiting status to watchers")
	})

	// Overrides sent with content are stored; a later content-only edit
	// keeps them.
	t.Run("OverridesApplyWithContent", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t, withChatWorkerDisabled)
		user := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModel(t, client)
		otherModel := createAdditionalChatModel(t, client, coderdtest.TestChatProviderOpenAICompat, "gpt-4o-mini-queued-edit-"+uuid.NewString())
		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID: user.OrganizationID, OwnerID: user.UserID,
			LastModelConfigID: modelConfig.ID, Title: "queued edit overrides", Status: database.ChatStatusError,
		})
		queued := insertTestChatQueuedMessage(ctx, t, db, chat.ID, queuedTextContent(t, "original"), modelConfig.ID)

		require.NoError(t, client.EditChatQueuedMessage(ctx, chat.ID, queued.ID, codersdk.EditChatQueuedMessageRequest{
			Content:         []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "with overrides"}},
			ModelConfigID:   &otherModel.ID,
			ReasoningEffort: ptr.Ref("high"),
		}))
		listed, err := client.GetChatMessages(ctx, chat.ID, nil)
		require.NoError(t, err)
		require.Len(t, listed.QueuedMessages, 1)
		require.Equal(t, &otherModel.ID, listed.QueuedMessages[0].ModelConfigID)
		require.Equal(t, ptr.Ref("high"), listed.QueuedMessages[0].ReasoningEffort)

		require.NoError(t, client.EditChatQueuedMessage(ctx, chat.ID, queued.ID, codersdk.EditChatQueuedMessageRequest{
			Content: []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "content only"}},
		}))
		listed, err = client.GetChatMessages(ctx, chat.ID, nil)
		require.NoError(t, err)
		require.Equal(t, "content only", listed.QueuedMessages[0].Content[0].Text)
		require.Equal(t, &otherModel.ID, listed.QueuedMessages[0].ModelConfigID, "a content-only edit keeps the model")
		require.Equal(t, ptr.Ref("high"), listed.QueuedMessages[0].ReasoningEffort, "a content-only edit keeps the effort")
	})

	// The archive refusal names the paused state when a child, not the
	// root, is the member that refuses.
	t.Run("ArchiveRefusedByPausedChild", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t, withChatWorkerDisabled)
		user := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModel(t, client)
		sysCtx := dbauthz.AsSystemRestricted(ctx)

		root := dbgen.Chat(t, db, database.Chat{
			OrganizationID: user.OrganizationID, OwnerID: user.UserID,
			LastModelConfigID: modelConfig.ID, Title: "idle root",
		})
		child := dbgen.Chat(t, db, database.Chat{
			OrganizationID: user.OrganizationID, OwnerID: user.UserID,
			LastModelConfigID: modelConfig.ID, Title: "paused child", Status: database.ChatStatusPaused,
			ParentChatID: uuid.NullUUID{UUID: root.ID, Valid: true},
			RootChatID:   uuid.NullUUID{UUID: root.ID, Valid: true},
		})
		queued := insertTestChatQueuedMessage(ctx, t, db, child.ID, queuedTextContent(t, "under edit"), modelConfig.ID)
		_, err := db.UpdateChatQueuedMessageEditing(sysCtx, database.UpdateChatQueuedMessageEditingParams{
			ID: queued.ID, ChatID: child.ID, Editing: true,
		})
		require.NoError(t, err)

		err = client.UpdateChat(ctx, root.ID, codersdk.UpdateChatRequest{Archived: new(true)})
		require.Equal(t, pausedArchiveRefusal, requireSDKError(t, err, http.StatusConflict).Message)
		refreshed, err := db.GetChatByID(sysCtx, root.ID)
		require.NoError(t, err)
		require.False(t, refreshed.Archived)
	})

	// Promoting a stale automation row under edit from paused deletes it
	// and answers 404, and the chat leaves paused with the row behind it.
	t.Run("PromoteStalePausedHead", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t, withChatWorkerDisabled)
		user := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModel(t, client)
		sysCtx := dbauthz.AsSystemRestricted(ctx)

		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID: user.OrganizationID, OwnerID: user.UserID,
			LastModelConfigID: modelConfig.ID, Title: "stale paused head", Status: database.ChatStatusPaused,
		})
		automation := dbgen.ChatAutomation(t, db, database.ChatAutomation{
			OrganizationID: user.OrganizationID, OwnerID: user.UserID, Enabled: true,
		})
		stale, err := db.InsertChatQueuedMessageWithCreator(sysCtx, database.InsertChatQueuedMessageWithCreatorParams{
			ChatID:        chat.ID,
			Content:       queuedTextContent(t, "stale"),
			ModelConfigID: uuid.NullUUID{UUID: modelConfig.ID, Valid: true},
			CreatedBy:     user.UserID,
			AutomationID:  uuid.NullUUID{UUID: automation.ID, Valid: true},
			InputID:       uuid.NullUUID{UUID: uuid.New(), Valid: true},
			// A row from another queue generation fails the guard.
			QueueGeneration: sql.NullInt64{Int64: automation.QueueGeneration + 1, Valid: true},
		})
		require.NoError(t, err)
		_, err = db.UpdateChatQueuedMessageEditing(sysCtx, database.UpdateChatQueuedMessageEditingParams{
			ID: stale.ID, ChatID: chat.ID, Editing: true,
		})
		require.NoError(t, err)
		next := insertTestChatQueuedMessage(ctx, t, db, chat.ID, queuedTextContent(t, "next"), modelConfig.ID)
		watch, err := client.Dial(ctx, "/api/v2/chats/watch", nil)
		require.NoError(t, err)
		defer watch.Close(websocket.StatusNormalClosure, "done")

		res, err := client.Request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/chats/%s/queue/%d/promote", chat.ID, stale.ID), nil)
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusNotFound, res.StatusCode)

		listed, err := client.GetChatMessages(ctx, chat.ID, nil)
		require.NoError(t, err)
		require.Empty(t, listed.QueuedMessages, "the stale row is deleted and the next row is sent")
		require.Equal(t, "next", listed.Messages[len(listed.Messages)-1].Content[0].Text)
		require.NotNil(t, listed.Messages[len(listed.Messages)-1].QueuedMessageID)
		require.Equal(t, next.ID, *listed.Messages[len(listed.Messages)-1].QueuedMessageID)
		require.Equal(t, codersdk.ChatStatusRunning, waitForChatWatchStatusChangeEvent(ctx, t, watch, chat.ID).Chat.Status,
			"leaving paused publishes the running status to watchers")
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
		path := fmt.Sprintf("/api/v2/chats/%s/queue/%d", chat.ID, queued.ID)

		err := client.EditChatQueuedMessage(ctx, chat.ID, queued.ID, codersdk.EditChatQueuedMessageRequest{})
		require.Equal(t, "Nothing to edit.", requireSDKError(t, err, http.StatusBadRequest).Message)

		// Overrides without content return 400, and the request that
		// also sets editing: true does not begin an edit.
		err = client.EditChatQueuedMessage(ctx, chat.ID, queued.ID, codersdk.EditChatQueuedMessageRequest{
			Editing:       new(false),
			ModelConfigID: &modelConfig.ID,
		})
		require.Equal(t, "model_config_id and reasoning_effort require content.", requireSDKError(t, err, http.StatusBadRequest).Message)
		err = client.EditChatQueuedMessage(ctx, chat.ID, queued.ID, codersdk.EditChatQueuedMessageRequest{
			Editing:         new(true),
			ReasoningEffort: ptr.Ref("high"),
		})
		require.Equal(t, "model_config_id and reasoning_effort require content.", requireSDKError(t, err, http.StatusBadRequest).Message)
		afterRefusal, err := db.GetChatQueuedMessageByID(dbauthz.AsSystemRestricted(ctx), database.GetChatQueuedMessageByIDParams{ID: queued.ID, ChatID: chat.ID})
		require.NoError(t, err)
		require.False(t, afterRefusal.EditingSince.Valid, "the refused request does not begin an edit")

		err = client.EditChatQueuedMessage(ctx, chat.ID, queued.ID, codersdk.EditChatQueuedMessageRequest{
			Content:         []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "edited"}},
			ReasoningEffort: ptr.Ref("bogus"),
		})
		require.Equal(t, "Invalid reasoning_effort value.", requireSDKError(t, err, http.StatusBadRequest).Message)

		// The SDK omits empty slices, so send the raw body.
		res, err := client.Request(ctx, http.MethodPatch, path, map[string]any{"content": []any{}})
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, "Content is required.", requireSDKError(t, codersdk.ReadBodyAsError(res), http.StatusBadRequest).Message)

		res, err = client.Request(ctx, http.MethodPatch, fmt.Sprintf("/api/v2/chats/%s/queue/not-an-int", chat.ID), codersdk.EditChatQueuedMessageRequest{Editing: new(true)})
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, "Invalid queued message ID.", requireSDKError(t, codersdk.ReadBodyAsError(res), http.StatusBadRequest).Message)

		// PATCH and DELETE are owner-only.
		adminRaw, _ := coderdtest.CreateAnotherUser(t, client.Client, user.OrganizationID, rbac.ScopedRoleOrgAdmin(user.OrganizationID))
		err = codersdk.NewExperimentalClient(adminRaw).EditChatQueuedMessage(ctx, chat.ID, queued.ID, codersdk.EditChatQueuedMessageRequest{Editing: new(true)})
		require.Equal(t, "Only the chat owner may edit queued messages.", requireSDKError(t, err, http.StatusForbidden).Message)
		res, err = adminRaw.Request(ctx, http.MethodDelete, path, nil)
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, "Only the chat owner may delete queued messages.", requireSDKError(t, codersdk.ReadBodyAsError(res), http.StatusForbidden).Message)

		_, err = db.ArchiveChatByID(dbauthz.AsSystemRestricted(ctx), chat.ID)
		require.NoError(t, err)
		err = client.EditChatQueuedMessage(ctx, chat.ID, queued.ID, codersdk.EditChatQueuedMessageRequest{Editing: new(true)})
		require.Equal(t, "Cannot edit queued messages in an archived chat.", requireSDKError(t, err, http.StatusBadRequest).Message)
	})
}

package chatd

import (
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func createStructuredTestChat(t *testing.T, f *taskTestFixture, initial chatstate.Message) (database.Chat, *chatstate.ChatMachine) {
	t.Helper()
	created, err := chatstate.CreateChat(testutil.Context(t, testutil.WaitShort), f.db, f.pubsub, chatstate.CreateChatInput{
		OrganizationID: f.org.ID, OwnerID: f.user.ID, LastModelConfigID: f.model.ID, Title: "test",
		ClientType: database.ChatClientTypeApi, InitialMessages: []chatstate.Message{initial},
	})
	require.NoError(t, err)
	return created.Chat, chatstate.NewChatMachine(f.db, f.pubsub, created.Chat.ID)
}

func queueTestMessages(t *testing.T, m *chatstate.ChatMachine, messages ...chatstate.Message) {
	t.Helper()
	for _, msg := range messages {
		require.NoError(t, m.Update(testutil.Context(t, testutil.WaitShort), func(tx *chatstate.Tx, _ database.Store) error {
			_, err := tx.SendMessage(chatstate.SendMessageInput{Message: msg, BusyBehavior: chatstate.BusyBehaviorQueue})
			return err
		}))
	}
}

func TestDeleteQueuedStructuredRequest(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name       string
		errorState bool
		structured bool
	}{
		{name: "Running", structured: true},
		{name: "Error", errorState: true, structured: true},
		{name: "Ordinary"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			f := newTaskTestFixture(t)
			chat, m := createStructuredTestChat(t, f, taskUserTextMessage(t, "hello", f.user.ID, f.model.ID, f.apiKey.ID))
			requestID := uuid.New()
			deleted := taskUserTextMessage(t, "queued", f.user.ID, f.model.ID, f.apiKey.ID)
			if tt.structured {
				deleted = structuredUserMessage(t, f, requestID)
			}
			queueTestMessages(t, m, deleted, taskUserTextMessage(t, "kept", f.user.ID, f.model.ID, f.apiKey.ID))
			if tt.errorState {
				f.forceExecutionState(t, chat.ID, database.ChatStatusError, false, sql.NullTime{})
			}
			queued, err := f.db.GetChatQueuedMessagesByPosition(ctx, chat.ID)
			require.NoError(t, err)
			// chatstate rejects anything but receipt rows.
			require.Error(t, m.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
				_, err := tx.DeleteQueuedMessage(chatstate.DeleteQueuedMessageInput{
					QueuedMessageID: queued[0].ID, Receipts: []chatstate.Message{deleted},
				})
				return err
			}))
			before, err := f.db.GetChatByID(ctx, chat.ID)
			require.NoError(t, err)
			historyBefore, _ := structuredHistory(t, f, chat.ID)

			server := &Server{db: f.db, pubsub: f.rawPS, logger: testutil.Logger(t)}
			require.NoError(t, server.DeleteQueued(ctx, chat.ID, queued[0].ID))

			after, err := f.db.GetChatByID(ctx, chat.ID)
			require.NoError(t, err)
			history, _ := structuredHistory(t, f, chat.ID)
			added := history[len(historyBefore):]
			var want []codersdk.ChatStructuredOutput
			if tt.structured {
				want = []codersdk.ChatStructuredOutput{canceledStructuredOutput(requestID, codersdk.ChatStructuredOutputErrorCodeQueueDeleted, queueDeletedStructuredOutputMessage)}
			}
			require.Equal(t, want, receiptOutcomes(added))
			require.Len(t, added, len(want))
			// A receipt-only insert leaves the running turn's history epoch alone.
			require.Equal(t, before.HistoryVersion, after.HistoryVersion)
			require.Equal(t, before.GenerationAttempt, after.GenerationAttempt)
			remaining, err := f.db.GetChatQueuedMessagesByPosition(ctx, chat.ID)
			require.NoError(t, err)
			require.Len(t, remaining, 1)
			require.Equal(t, queued[1].ID, remaining[0].ID)
		})
	}
}

func TestEditSupersedesDiscardedStructuredRequests(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	f := newTaskTestFixture(t)
	target, closed, queuedA, queuedB := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	chat, m := createStructuredTestChat(t, f, structuredUserMessage(t, f, target))
	// A later request in the discarded history is already closed.
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		receipt, err := structuredReceiptMessages(ctx, store, chat.ID, 0, codersdk.ChatStructuredOutput{
			RequestID: closed, Status: codersdk.ChatStructuredOutputStatusSucceeded, Value: json.RawMessage(`{}`),
		})
		if err != nil {
			return err
		}
		_, err = tx.CommitStep(chatstate.CommitStepInput{Messages: append([]chatstate.Message{structuredUserMessage(t, f, closed)}, receipt...)})
		return err
	}))
	queueTestMessages(t, m, structuredUserMessage(t, f, queuedA), taskUserTextMessage(t, "ordinary", f.user.ID, f.model.ID, f.apiKey.ID), structuredUserMessage(t, f, queuedB))
	history, _ := structuredHistory(t, f, chat.ID)
	require.Error(t, m.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		_, err := tx.EditMessage(chatstate.EditMessageInput{
			MessageID: history[0].ID, Content: history[0].Content, Receipts: []chatstate.Message{structuredUserMessage(t, f, target)},
		})
		return err
	}))

	server := &Server{db: f.db, pubsub: f.rawPS, logger: testutil.Logger(t)}
	result, err := server.EditMessage(ctx, EditMessageOptions{
		ChatID: chat.ID, CreatedBy: f.user.ID, EditedMessageID: history[0].ID,
		Content: []codersdk.ChatMessagePart{codersdk.ChatMessageText("edited")},
	})
	require.NoError(t, err)

	// Pending requests close in history order, then queue order, before the
	// replacement; the closed request is never closed again.
	superseded := func(id uuid.UUID) codersdk.ChatStructuredOutput {
		return canceledStructuredOutput(id, codersdk.ChatStructuredOutputErrorCodeSuperseded, supersededStructuredOutputMessage)
	}
	require.Len(t, result.InsertedMessages, 4)
	require.Equal(t, []codersdk.ChatStructuredOutput{superseded(target), superseded(queuedA), superseded(queuedB)}, receiptOutcomes(result.InsertedMessages))
	require.Equal(t, result.Message.ID, result.InsertedMessages[3].ID)
	// History is in ID order, so the response is too.
	after, state := structuredHistory(t, f, chat.ID)
	require.Len(t, after, len(result.InsertedMessages))
	for i, msg := range after {
		require.Equal(t, result.InsertedMessages[i].ID, msg.ID)
	}
	require.False(t, state.Active)
	queued, err := f.db.GetChatQueuedMessagesByPosition(ctx, chat.ID)
	require.NoError(t, err)
	require.Empty(t, queued)
}

func TestArchiveAndClearLeaveStructuredRequests(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	f := newTaskTestFixture(t)
	requestID := uuid.New()
	chat, m := createStructuredTestChat(t, f, structuredUserMessage(t, f, requestID))
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		receipts, err := structuredReceiptMessages(ctx, store, chat.ID, 0, codersdk.ChatStructuredOutput{
			RequestID: requestID, Status: codersdk.ChatStructuredOutputStatusSucceeded, Value: json.RawMessage(`{}`),
		})
		if err != nil {
			return err
		}
		_, err = tx.FinishTurn(chatstate.FinishTurnInput{TerminalMessages: receipts})
		return err
	}))
	_, closed := structuredHistory(t, f, chat.ID)
	require.True(t, closed.Closed)

	// Clearing context neither writes a receipt nor reopens the request.
	server := &Server{db: f.db, pubsub: f.rawPS, logger: testutil.Logger(t)}
	_, err := server.ClearChat(ctx, chat)
	require.NoError(t, err)
	history, cleared := structuredHistory(t, f, chat.ID)
	require.Equal(t, closed, cleared)
	require.Len(t, receiptOutcomes(history), 1)

	// Archiving keeps queued structured requests queued and unanswered.
	queuedID := uuid.New()
	queueTestMessages(t, m, taskUserTextMessage(t, "next", f.user.ID, f.model.ID, f.apiKey.ID), structuredUserMessage(t, f, queuedID))
	f.forceExecutionState(t, chat.ID, database.ChatStatusError, false, sql.NullTime{})
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		_, err := tx.SetArchived(chatstate.SetArchivedInput{Archived: true})
		return err
	}))
	history, _ = structuredHistory(t, f, chat.ID)
	require.Len(t, receiptOutcomes(history), 1)
	queued, err := f.db.GetChatQueuedMessagesByPosition(ctx, chat.ID)
	require.NoError(t, err)
	require.Len(t, queued, 1)
}

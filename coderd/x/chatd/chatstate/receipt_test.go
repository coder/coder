package chatstate_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstructured"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// receiptMessage builds a terminal receipt row for a fresh request.
func receiptMessage(t *testing.T, parts ...codersdk.ChatMessagePart) chatstate.Message {
	t.Helper()
	if parts == nil {
		var err error
		parts, err = chatstructured.ReceiptParts(codersdk.ChatStructuredOutput{
			RequestID: uuid.New(), Status: codersdk.ChatStructuredOutputStatusSucceeded, Value: json.RawMessage(`{"a":1}`),
		})
		require.NoError(t, err)
	}
	content, err := chatprompt.MarshalParts(parts)
	require.NoError(t, err)
	return chatstate.Message{
		Role: database.ChatMessageRoleAssistant, Visibility: database.ChatMessageVisibilityUser,
		ContentVersion: chatprompt.ContentVersionV1, Content: content,
	}
}

func TestTerminalReceipts(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	update := func(m *chatstate.ChatMachine, fn func(tx *chatstate.Tx) error) error {
		return m.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error { return fn(tx) })
	}
	newChat := func() (database.Chat, *chatstate.ChatMachine) {
		created := createTestChat(t, f)
		return created.Chat, chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)
	}
	last := func(chatID uuid.UUID) database.ChatMessage {
		ids := historyMessageIDs(ctx, t, f, chatID)
		msg, err := f.DB.GetChatMessageByID(ctx, ids[len(ids)-1])
		require.NoError(t, err)
		return msg
	}

	// R0: the receipt commits with the turn and does not reset history.
	chat, m := newChat()
	require.NoError(t, update(m, func(tx *chatstate.Tx) error {
		_, err := tx.FinishTurn(chatstate.FinishTurnInput{TerminalMessages: []chatstate.Message{receiptMessage(t)}})
		return err
	}))
	after, err := f.DB.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusWaiting, after.Status)
	require.Equal(t, chat.HistoryVersion, after.HistoryVersion)
	require.Equal(t, after.SnapshotVersion, last(chat.ID).Revision)
	require.Equal(t, database.ChatMessageVisibilityUser, last(chat.ID).Visibility)

	// R1 with a pending tool call: the cancellation and the receipt land
	// before the promoted head, which resets history.
	chat, m = newChat()
	callID := "call_" + uuid.NewString()
	commitAssistantToolCall(t, f, m, nonDynamicAssistantToolCallMessage(t, f.Model.ID, callID))
	sendQueuedMessage(t, f, m, "queued")
	before := historyMessageIDs(ctx, t, f, chat.ID)
	var finish chatstate.FinishTurnResult
	require.NoError(t, update(m, func(tx *chatstate.Tx) (err error) {
		finish, err = tx.FinishTurn(chatstate.FinishTurnInput{TerminalMessages: []chatstate.Message{receiptMessage(t)}})
		return err
	}))
	added := historyMessageIDs(ctx, t, f, chat.ID)[len(before):]
	require.Len(t, added, 3)
	cancel, err := f.DB.GetChatMessageByID(ctx, added[0])
	require.NoError(t, err)
	assertToolResultForCall(t, cancel, callID)
	receipt, err := f.DB.GetChatMessageByID(ctx, added[1])
	require.NoError(t, err)
	require.Equal(t, database.ChatMessageVisibilityUser, receipt.Visibility)
	require.Equal(t, finish.PromotedMessage.ID, added[2])
	require.Equal(t, finish.Chat.SnapshotVersion, finish.Chat.HistoryVersion)

	// FinishError commits the receipt before parking the chat.
	chat, m = newChat()
	require.NoError(t, update(m, func(tx *chatstate.Tx) error {
		_, err := tx.FinishError(chatstate.FinishErrorInput{TerminalMessages: []chatstate.Message{receiptMessage(t)}})
		return err
	}))
	require.Equal(t, database.ChatMessageVisibilityUser, last(chat.ID).Visibility)

	// FinishInterruption takes the receipt as the last partial message, and
	// a receipt does not hide an outstanding tool call.
	interrupted := func() (database.Chat, *chatstate.ChatMachine) {
		chat, m := newChat()
		require.NoError(t, update(m, func(tx *chatstate.Tx) error {
			_, err := tx.Interrupt(chatstate.InterruptInput{Reason: "test"})
			return err
		}))
		return chat, m
	}
	chat, m = interrupted()
	require.NoError(t, update(m, func(tx *chatstate.Tx) error {
		_, err := tx.FinishInterruption(chatstate.FinishInterruptionInput{PartialMessages: []chatstate.Message{receiptMessage(t)}})
		return err
	}))
	require.Equal(t, database.ChatMessageVisibilityUser, last(chat.ID).Visibility)
	chat, m = newChat()
	commitAssistantToolCall(t, f, m, nonDynamicAssistantToolCallMessage(t, f.Model.ID, "call_"+uuid.NewString()))
	require.NoError(t, update(m, func(tx *chatstate.Tx) error {
		_, err := tx.Interrupt(chatstate.InterruptInput{Reason: "test"})
		return err
	}))
	require.ErrorContains(t, update(m, func(tx *chatstate.Tx) error {
		_, err := tx.FinishInterruption(chatstate.FinishInterruptionInput{PartialMessages: []chatstate.Message{receiptMessage(t)}})
		return err
	}), "outstanding tool calls")

	// Anything but a valid receipt is rejected, and a rolled back
	// transaction leaves no receipt.
	notReceipt := receiptMessage(t)
	notReceipt.Visibility = database.ChatMessageVisibilityBoth
	for _, bad := range []chatstate.Message{
		notReceipt,
		receiptMessage(t, codersdk.ChatMessageText("x")),
		receiptMessage(t, codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeStructuredOutputOutcome, StructuredOutputData: json.RawMessage(`{}`)}),
	} {
		chat, m = newChat()
		require.Error(t, update(m, func(tx *chatstate.Tx) error {
			_, err := tx.FinishTurn(chatstate.FinishTurnInput{TerminalMessages: []chatstate.Message{bad}})
			return err
		}))
		require.Equal(t, database.ChatMessageVisibilityBoth, last(chat.ID).Visibility)
	}
	chat, m = newChat()
	require.Error(t, update(m, func(tx *chatstate.Tx) error {
		if _, err := tx.FinishTurn(chatstate.FinishTurnInput{TerminalMessages: []chatstate.Message{receiptMessage(t)}}); err != nil {
			return err
		}
		return xerrors.New("fence failed after insert")
	}))
	require.Equal(t, database.ChatMessageVisibilityBoth, last(chat.ID).Visibility)
}

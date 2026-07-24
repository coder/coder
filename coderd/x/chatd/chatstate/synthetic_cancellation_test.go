package chatstate_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// nonDynamicAssistantToolCallMessage builds an assistant message that
// issues a single tool call against a tool that is NOT in the chat's
// dynamic_tools set. The send-message and edit-message paths use the
// "cancel every outstanding tool call regardless of source" variant
// (dynamicOnly=false), so the cancellation must still fire even for
// non-dynamic tools.
func nonDynamicAssistantToolCallMessage(t *testing.T, modelID uuid.UUID, callID string) chatstate.Message {
	t.Helper()
	raw, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{{
		Type:       codersdk.ChatMessagePartTypeToolCall,
		ToolCallID: callID,
		ToolName:   "non_dynamic_tool",
		Args:       json.RawMessage(`{}`),
	}})
	require.NoError(t, err)
	return chatstate.Message{
		Role:           database.ChatMessageRoleAssistant,
		Content:        raw,
		Visibility:     database.ChatMessageVisibilityBoth,
		ContentVersion: chatprompt.CurrentContentVersion,
		ModelConfigID:  uuid.NullUUID{UUID: modelID, Valid: true},
	}
}

// assertToolResultForCall asserts that msg is a tool-result message
// that resolves a tool call with id wantCallID and is_error=true.
func assertToolResultForCall(t *testing.T, msg database.ChatMessage, wantCallID string) {
	t.Helper()
	require.Equal(t, database.ChatMessageRoleTool, msg.Role)
	parts, err := chatprompt.ParseContent(msg)
	require.NoError(t, err)
	require.NotEmpty(t, parts)
	var found bool
	for _, p := range parts {
		if p.Type != codersdk.ChatMessagePartTypeToolResult {
			continue
		}
		require.Equal(t, wantCallID, p.ToolCallID, "tool-call id matches")
		require.True(t, p.IsError, "synthetic cancellation must be marked is_error=true")
		found = true
	}
	require.True(t, found, "expected at least one tool-result part")
}

// commitAssistantToolCall pushes an assistant message that calls
// `tool_name` with `callID` into history via CommitStep. Returns the
// inserted assistant ChatMessage. Use the dynamic-tools chat fixture
// (createTestChatWithDynamicTools) when dynamicOnly cancellation
// paths are exercised.
func commitAssistantToolCall(
	t *testing.T,
	f *testFixture,
	m *chatstate.ChatMachine,
	msg chatstate.Message,
) database.ChatMessage {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitShort)
	var step chatstate.CommitStepResult
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		var err error
		step, err = tx.CommitStep(chatstate.CommitStepInput{Messages: []chatstate.Message{msg}})
		return err
	}))
	require.Len(t, step.InsertedMessages, 1)
	return step.InsertedMessages[0]
}

// landInW puts a fresh R0 chat into state W (waiting) via FinishTurn.
func landInW(t *testing.T, f *testFixture, m *chatstate.ChatMachine) {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitShort)
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		_, err := tx.FinishTurn(chatstate.FinishTurnInput{})
		return err
	}))
	require.Equal(t, chatstate.StateW, f.classify(ctx, t, m.ChatID()))
}

// landInE0 puts a fresh R0 chat into state E0 (error, empty queue).
func landInE0(t *testing.T, f *testFixture, m *chatstate.ChatMachine) {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitShort)
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		_, err := tx.FinishError(chatstate.FinishErrorInput{
			LastError: pqtype.NullRawMessage{
				RawMessage: json.RawMessage(`{"message":"boom"}`),
				Valid:      true,
			},
		})
		return err
	}))
	require.Equal(t, chatstate.StateE0, f.classify(ctx, t, m.ChatID()))
}

func TestSyntheticCancellation_SendMessageDirect(t *testing.T) {
	t.Parallel()

	t.Run("waiting", func(t *testing.T) {
		t.Parallel()
		testSendMessageDirectWSynthesizesToolCancellations(t)
	})
	t.Run("error", func(t *testing.T) {
		t.Parallel()
		testSendMessageDirectE0SynthesizesToolCancellations(t)
	})
}

// testSendMessageDirectWSynthesizesToolCancellations verifies that
// from W, SendMessage inserts synthetic tool-result rows for every
// outstanding tool call on the last assistant message BEFORE the new
// user message, regardless of whether the tools are dynamic.
func testSendMessageDirectWSynthesizesToolCancellations(t *testing.T) {
	t.Helper()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)
	m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)

	callID := "call_" + uuid.NewString()
	assistant := commitAssistantToolCall(t, f, m,
		nonDynamicAssistantToolCallMessage(t, f.Model.ID, callID))
	require.Equal(t, database.ChatMessageRoleAssistant, assistant.Role)

	// R0 -> W.
	landInW(t, f, m)

	// SendMessage with a fresh user message. The direct-history path
	// must insert a synthetic tool-result (for callID) followed by
	// the new user message.
	var send chatstate.SendMessageResult
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		var err error
		send, err = tx.SendMessage(chatstate.SendMessageInput{
			Message:      userTextMessage("after-cancel", f.User.ID, f.Model.ID),
			BusyBehavior: chatstate.BusyBehaviorQueue,
		})
		return err
	}))

	require.Len(t, send.InsertedMessages, 2, "synthetic cancel + new user")
	assertToolResultForCall(t, send.InsertedMessages[0], callID)
	require.Equal(t, database.ChatMessageRoleUser, send.InsertedMessages[1].Role)
	require.Less(t, send.InsertedMessages[0].ID, send.InsertedMessages[1].ID,
		"synthetic cancel is inserted before the user message")
}

// testSendMessageDirectE0SynthesizesToolCancellations exercises
// the same path from E0.
func testSendMessageDirectE0SynthesizesToolCancellations(t *testing.T) {
	t.Helper()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)
	m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)

	callID := "call_" + uuid.NewString()
	commitAssistantToolCall(t, f, m,
		nonDynamicAssistantToolCallMessage(t, f.Model.ID, callID))

	// R0 -> E0.
	landInE0(t, f, m)

	var send chatstate.SendMessageResult
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		var err error
		send, err = tx.SendMessage(chatstate.SendMessageInput{
			Message:      userTextMessage("after-error", f.User.ID, f.Model.ID),
			BusyBehavior: chatstate.BusyBehaviorQueue,
		})
		return err
	}))

	require.Len(t, send.InsertedMessages, 2)
	assertToolResultForCall(t, send.InsertedMessages[0], callID)
	require.Equal(t, database.ChatMessageRoleUser, send.InsertedMessages[1].Role)
}

func TestSyntheticCancellation_EditMessage(t *testing.T) {
	t.Parallel()

	t.Run("replacement insertion", func(t *testing.T) {
		t.Parallel()
		testEditMessageSynthesizesToolCancellationsBeforeReplacement(t)
	})
}

// testEditMessageSynthesizesToolCancellationsBeforeReplacement
// verifies that EditMessage from a state with an outstanding tool
// call before the edited user message inserts a synthetic
// tool-result before the replacement user message in history.
//
// The scenario is:
//   - user message 1 (initial)
//   - assistant tool-call (outstanding)
//   - user message 2 (the one we will edit)
//
// EditMessage soft-deletes user message 2 and everything after it,
// then synthesizes cancellations for tool calls on the last
// surviving assistant message that have no matching tool-result.
func testEditMessageSynthesizesToolCancellationsBeforeReplacement(t *testing.T) {
	t.Helper()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)
	m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)

	// Build the history described above. CommitStep is happy to insert
	// a mixed batch as long as it stays inside R0.
	callID := "call_" + uuid.NewString()
	assistantTC := nonDynamicAssistantToolCallMessage(t, f.Model.ID, callID)
	secondUser := userTextMessage("second user", f.User.ID, f.Model.ID)
	var step chatstate.CommitStepResult
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		var err error
		step, err = tx.CommitStep(chatstate.CommitStepInput{
			Messages: []chatstate.Message{assistantTC, secondUser},
		})
		return err
	}))
	require.Len(t, step.InsertedMessages, 2)
	secondUserID := step.InsertedMessages[1].ID
	require.Equal(t, database.ChatMessageRoleUser, step.InsertedMessages[1].Role)

	var edit chatstate.EditMessageResult
	editedContent := mustMarshalParts(t, []codersdk.ChatMessagePart{
		codersdk.ChatMessageText("edited"),
	})
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		var err error
		edit, err = tx.EditMessage(chatstate.EditMessageInput{
			MessageID: secondUserID,
			CreatedBy: f.User.ID,
			Content:   editedContent,
		})
		return err
	}))

	require.Len(t, edit.CancellationMessages, 1, "synthetic cancel inserted")
	assertToolResultForCall(t, edit.CancellationMessages[0], callID)
	require.Equal(t, database.ChatMessageRoleUser, edit.ReplacementMessage.Role)
	require.Less(t, edit.CancellationMessages[0].ID, edit.ReplacementMessage.ID,
		"cancellations are inserted before the replacement user message")
}

func TestSyntheticCancellation_PromoteQueuedMessage(t *testing.T) {
	t.Parallel()

	t.Run("error queued message", func(t *testing.T) {
		t.Parallel()
		testPromoteQueuedMessageE1SynthesizesToolCancellations(t)
	})
	t.Run("requires action queued message", func(t *testing.T) {
		t.Parallel()
		testPromoteQueuedMessageA1SynthesizesDynamicToolCancellations(t)
	})
}

// testPromoteQueuedMessageE1SynthesizesToolCancellations verifies
// that promoting a queued message from E1 inserts synthetic
// tool-result rows for outstanding tool calls before the promoted
// user message in history.
func testPromoteQueuedMessageE1SynthesizesToolCancellations(t *testing.T) {
	t.Helper()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)
	m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)

	callID := "call_" + uuid.NewString()
	commitAssistantToolCall(t, f, m,
		nonDynamicAssistantToolCallMessage(t, f.Model.ID, callID))

	// Land in R1 with one queued message.
	queued := sendQueuedMessage(t, f, m, "queued-for-promote")
	require.NotNil(t, queued.QueuedMessage)
	require.Equal(t, chatstate.StateR1, f.classify(ctx, t, created.Chat.ID))

	// R1 -> E1 via FinishError.
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		_, err := tx.FinishError(chatstate.FinishErrorInput{
			LastError: pqtype.NullRawMessage{
				RawMessage: json.RawMessage(`{"message":"boom"}`),
				Valid:      true,
			},
		})
		return err
	}))
	require.Equal(t, chatstate.StateE1, f.classify(ctx, t, created.Chat.ID))

	var promote chatstate.PromoteQueuedMessageResult
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		var err error
		promote, err = tx.PromoteQueuedMessage(chatstate.PromoteQueuedMessageInput{
			QueuedMessageID: queued.QueuedMessage.ID,
		})
		return err
	}))

	require.Len(t, promote.CancellationMessages, 1)
	assertToolResultForCall(t, promote.CancellationMessages[0], callID)
	require.NotNil(t, promote.InsertedMessage)
	require.Equal(t, database.ChatMessageRoleUser, promote.InsertedMessage.Role)
	require.Less(t, promote.CancellationMessages[0].ID, promote.InsertedMessage.ID,
		"cancel is inserted before the promoted user message")
}

// testPromoteQueuedMessageA1SynthesizesDynamicToolCancellations
// verifies that the dynamic outstanding tool call is canceled when
// promoting from A1.
func testPromoteQueuedMessageA1SynthesizesDynamicToolCancellations(t *testing.T) {
	t.Helper()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)

	toolName := "dyn_promote_a1"
	created := createTestChatWithDynamicTools(t, f, toolName)
	m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)

	dynCallID := "call_" + uuid.NewString()
	commitAssistantToolCall(t, f, m,
		assistantToolCallMessage(t, f.Model.ID, toolName, dynCallID))

	// Land in A0.
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		_, err := tx.EnterRequiresAction(chatstate.EnterRequiresActionInput{})
		return err
	}))
	require.Equal(t, chatstate.StateA0, f.classify(ctx, t, created.Chat.ID))

	// A0 -> A1 with one queued user message.
	queued := sendQueuedMessage(t, f, m, "queued-for-a1-promote")
	require.NotNil(t, queued.QueuedMessage)
	require.Equal(t, chatstate.StateA1, f.classify(ctx, t, created.Chat.ID))

	var promote chatstate.PromoteQueuedMessageResult
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		var err error
		promote, err = tx.PromoteQueuedMessage(chatstate.PromoteQueuedMessageInput{
			QueuedMessageID: queued.QueuedMessage.ID,
		})
		return err
	}))

	require.Len(t, promote.CancellationMessages, 1, "dynamic tool call canceled")
	assertToolResultForCall(t, promote.CancellationMessages[0], dynCallID)
	require.NotNil(t, promote.InsertedMessage)
	require.Equal(t, database.ChatMessageRoleUser, promote.InsertedMessage.Role)
}

func TestSyntheticCancellation_FinishTurn(t *testing.T) {
	t.Parallel()

	t.Run("running queued message", func(t *testing.T) {
		t.Parallel()
		testFinishTurnR1SynthesizesToolCancellationsBeforePromotion(t)
	})
}

// testFinishTurnR1SynthesizesToolCancellationsBeforePromotion
// verifies that finishing a turn while a queued message exists
// synthesizes outstanding tool cancellations before promoting the
// queue head into history.
func testFinishTurnR1SynthesizesToolCancellationsBeforePromotion(t *testing.T) {
	t.Helper()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)
	m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)

	callID := "call_" + uuid.NewString()
	commitAssistantToolCall(t, f, m,
		nonDynamicAssistantToolCallMessage(t, f.Model.ID, callID))

	queued := sendQueuedMessage(t, f, m, "queued-for-finish")
	require.NotNil(t, queued.QueuedMessage)
	require.Equal(t, chatstate.StateR1, f.classify(ctx, t, created.Chat.ID))

	beforeIDs := historyMessageIDs(ctx, t, f, created.Chat.ID)

	var finish chatstate.FinishTurnResult
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		var err error
		finish, err = tx.FinishTurn(chatstate.FinishTurnInput{})
		return err
	}))
	require.NotNil(t, finish.PromotedMessage)
	require.Equal(t, database.ChatMessageRoleUser, finish.PromotedMessage.Role)

	afterIDs := historyMessageIDs(ctx, t, f, created.Chat.ID)
	require.Equal(t, len(beforeIDs)+2, len(afterIDs),
		"finish inserts both a tool cancel and the promoted user")

	// The two newly inserted messages are tool-result then user.
	newIDs := afterIDs[len(beforeIDs):]
	cancel, err := f.DB.GetChatMessageByID(ctx, newIDs[0])
	require.NoError(t, err)
	assertToolResultForCall(t, cancel, callID)
	require.Equal(t, finish.PromotedMessage.ID, newIDs[1])
}

func TestSyntheticCancellation_FinishInterruption(t *testing.T) {
	t.Parallel()

	t.Run("interrupting queued message", func(t *testing.T) {
		t.Parallel()
		testFinishInterruptionI1PromotesQueueHead(t)
	})
	t.Run("rejects outstanding dynamic tool calls", func(t *testing.T) {
		t.Parallel()
		testFinishInterruptionRejectsOutstandingToolCalls(t)
	})
}

// testFinishInterruptionI1PromotesQueueHead verifies that
// FinishInterruption from I1 with no outstanding tool calls
// promotes the queue head into history.
func testFinishInterruptionI1PromotesQueueHead(t *testing.T) {
	t.Helper()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)
	m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)

	// Reach R1 with one queued message.
	queued := sendQueuedMessage(t, f, m, "queued-for-interruption")
	require.NotNil(t, queued.QueuedMessage)
	// R1 -> I1 via Interrupt.
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		_, err := tx.Interrupt(chatstate.InterruptInput{Reason: "test"})
		return err
	}))
	require.Equal(t, chatstate.StateI1, f.classify(ctx, t, created.Chat.ID))

	beforeIDs := historyMessageIDs(ctx, t, f, created.Chat.ID)

	var finish chatstate.FinishInterruptionResult
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		var err error
		finish, err = tx.FinishInterruption(chatstate.FinishInterruptionInput{})
		return err
	}))
	require.NotNil(t, finish.PromotedMessage)
	require.Equal(t, database.ChatMessageRoleUser, finish.PromotedMessage.Role)

	afterIDs := historyMessageIDs(ctx, t, f, created.Chat.ID)
	require.Equal(t, len(beforeIDs)+1, len(afterIDs))
	require.Equal(t, chatstate.StateR0, f.classify(ctx, t, created.Chat.ID))
}

// testFinishInterruptionRejectsOutstandingToolCalls verifies that
// FinishInterruption fails (TransitionNotAllowed-shaped) when the
// chat still has an outstanding dynamic tool call after the partial
// commit. The chat must remain in its prior state.
func testFinishInterruptionRejectsOutstandingToolCalls(t *testing.T) {
	t.Helper()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)

	toolName := "dyn_finish_reject"
	created := createTestChatWithDynamicTools(t, f, toolName)
	m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)

	dynCallID := "call_" + uuid.NewString()
	commitAssistantToolCall(t, f, m,
		assistantToolCallMessage(t, f.Model.ID, toolName, dynCallID))

	// R0 -> I0 via Interrupt. Interrupt closes pending dynamic calls
	// when transitioning from A0/A1, but from R0 it does NOT, so the
	// chat keeps its outstanding dynamic call.
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		_, err := tx.Interrupt(chatstate.InterruptInput{Reason: "test"})
		return err
	}))
	require.Equal(t, chatstate.StateI0, f.classify(ctx, t, created.Chat.ID))

	stateBefore := f.classify(ctx, t, created.Chat.ID)
	historyBefore := historyMessageIDs(ctx, t, f, created.Chat.ID)
	publishedBefore := len(f.Pub.channels)

	// FinishInterruption with no partial commits should reject
	// because the dynamic call is still outstanding.
	err := m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		_, err := tx.FinishInterruption(chatstate.FinishInterruptionInput{})
		return err
	})
	require.Error(t, err)
	require.ErrorIs(t, err, chatstate.ErrTransitionNotAllowed)

	require.Equal(t, stateBefore, f.classify(ctx, t, created.Chat.ID), "state unchanged")
	require.Equal(t, historyBefore, historyMessageIDs(ctx, t, f, created.Chat.ID),
		"history unchanged on rejected finish")
	require.Equal(t, publishedBefore, len(f.Pub.channels),
		"failed FinishInterruption publishes nothing")
}

// ensure unused imports don't break the build if any helper is
// removed later.
var _ = context.Background

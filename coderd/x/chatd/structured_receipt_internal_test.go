package chatd

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd/chathooks"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstructured"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestStructuredReceiptMessagesSkipsDuplicates(t *testing.T) {
	t.Parallel()
	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: user.ID, OrganizationID: org.ID})
	dbgen.ChatProvider(t, db, database.ChatProvider{})
	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{IsDefault: true})
	chat := dbgen.Chat(t, db, database.Chat{OwnerID: user.ID, OrganizationID: org.ID, LastModelConfigID: model.ID})
	machine := chatstate.NewChatMachine(db, ps, chat.ID)
	out := codersdk.ChatStructuredOutput{RequestID: uuid.New(), Status: codersdk.ChatStructuredOutputStatusSucceeded, Value: json.RawMessage(`null`)}

	// The first terminal attempt commits one receipt under the chat lock.
	require.NoError(t, machine.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		messages, err := structuredReceiptMessages(ctx, store, chat.ID, 0, out)
		require.NoError(t, err)
		require.Len(t, messages, 1)
		_, err = tx.FinishError(chatstate.FinishErrorInput{TerminalMessages: messages})
		return err
	}))
	// A later attempt for the same request adds nothing; another request does.
	require.NoError(t, machine.Update(ctx, func(_ *chatstate.Tx, store database.Store) error {
		messages, err := structuredReceiptMessages(ctx, store, chat.ID, 0, out)
		require.NoError(t, err)
		require.Empty(t, messages)
		other := out
		other.RequestID = uuid.New()
		messages, err = structuredReceiptMessages(ctx, store, chat.ID, 0, other)
		require.NoError(t, err)
		require.Len(t, messages, 1)
		return nil
	}))
	history, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: chat.ID})
	require.NoError(t, err)
	require.Len(t, history, 1)
	got, err := chatstate.ReceiptRowOutcome(history[0])
	require.NoError(t, err)
	require.Equal(t, out, got)
}

// receiptRow builds a structured output receipt row. A succeeded receipt's
// fallback text contains receiptCanary, so a test can prove a reader never
// surfaced it.
func receiptRow(t *testing.T, id int64, status codersdk.ChatStructuredOutputStatus) database.ChatMessage {
	t.Helper()
	out := codersdk.ChatStructuredOutput{RequestID: uuid.New(), Status: status, Value: json.RawMessage(`"` + receiptCanary + `"`)}
	if status != codersdk.ChatStructuredOutputStatusSucceeded {
		out.Value = nil
		out.Error = &codersdk.ChatStructuredOutputError{Code: codersdk.ChatStructuredOutputErrorCodeGenerationFailed, Message: "failed"}
	}
	parts, err := chatstructured.ReceiptParts(out)
	require.NoError(t, err)
	msg := dbMessage(t, id, database.ChatMessageRoleAssistant, false, parts...)
	msg.Visibility = database.ChatMessageVisibilityUser
	require.True(t, chatstate.IsReceiptRow(msg))
	return msg
}

const receiptCanary = "RECEIPT_CANARY"

func TestHistoryReadersSkipStructuredOutputReceipts(t *testing.T) {
	t.Parallel()
	user := func(id int64, text string) database.ChatMessage {
		return dbMessage(t, id, database.ChatMessageRoleUser, false, codersdk.ChatMessageText(text))
	}
	answer := func(id int64, text string) database.ChatMessage {
		return dbMessage(t, id, database.ChatMessageRoleAssistant, false, codersdk.ChatMessageText(text))
	}
	call := dbMessage(t, 2, database.ChatMessageRoleAssistant, false, codersdk.ChatMessageToolCall("call-1", "execute", json.RawMessage(`{}`)))
	result := dbMessage(t, 3, database.ChatMessageRoleTool, false, codersdk.ChatMessageToolResult("call-1", "execute", json.RawMessage(`{}`), false, false))
	succeeded := codersdk.ChatStructuredOutputStatusSucceeded
	failed := codersdk.ChatStructuredOutputStatusFailed

	// user, assistant tool call, tool result, receipt: one step, and the
	// turn is not complete because the tool result still needs an answer.
	history := []database.ChatMessage{user(1, "q"), call, result, receiptRow(t, 4, succeeded)}
	require.Equal(t, 1, currentTurnStepCount(history))
	complete, err := currentHistoryComplete(history)
	require.NoError(t, err)
	require.False(t, complete)

	// A receipt after an unanswered tool call does not hide the call.
	local, _, err := unresolvedToolCallsFromHistory([]database.ChatMessage{user(1, "q"), call, receiptRow(t, 3, failed)}, nil)
	require.NoError(t, err)
	require.Len(t, local, 1)
	require.Equal(t, "call-1", local[0].ToolCallID)

	// A receipt carries no usage and nothing to compact.
	onlyReceipt := []database.ChatMessage{user(1, "q"), receiptRow(t, 2, failed)}
	_, ok := firstUncompressedAssistantAfter(onlyReceipt, -1)
	require.False(t, ok)
	require.False(t, hasUncompressedMessageAfter(onlyReceipt, 0))
	require.Equal(t, chathooks.SessionStartSourceStartup, chathooks.SessionStartSource(onlyReceipt))

	// Deliberately unchanged: a receipt ends the pending-user segment like
	// the turn it closes. Compaction replays that segment as model-only user
	// rows, so it must never contain a receipt.
	require.Equal(t, 4, pendingUserSegmentStart([]database.ChatMessage{
		user(1, "q"), answer(2, "a"), user(3, "p1"), receiptRow(t, 4, failed), user(5, "p2"),
	}))

	// Status labels, titles, summaries and subagent reports use the model's
	// own text: receipt fallback text duplicates the output or names only
	// an error code.
	finished := []database.ChatMessage{user(1, "q"), answer(2, "final answer"), receiptRow(t, 3, succeeded)}
	require.Equal(t, "final answer", latestAssistantText(finished))
	require.Equal(t, []manualTitleTurn{{role: "user", text: "q"}, {role: "assistant", text: "final answer"}}, extractManualTitleTurns(finished, nil))
	require.NotContains(t, renderChatSummaryTranscript(finished), receiptCanary)
	input, ok := titleInput(database.Chat{}, onlyReceipt, nil)
	require.True(t, ok)
	require.Equal(t, "q", input)

	store := dbmock.NewMockStore(gomock.NewController(t))
	store.EXPECT().GetChatMessagesByChatID(gomock.Any(), gomock.Any()).Return(finished, nil)
	report, err := latestSubagentAssistantMessage(context.Background(), store, uuid.New())
	require.NoError(t, err)
	require.Equal(t, "final answer", report)
}

package chatstate_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func flaggedMessage(f *testFixture, text string) chatstate.Message {
	m := userTextMessage(text, f.User.ID, f.Model.ID)
	m.PromptHooksPending = true
	return m
}

// TestPromptHooksPendingSurvivesTheQueue checks that a message keeps its
// flag from its first insert to the history row that a turn starts from.
func TestPromptHooksPendingSurvivesTheQueue(t *testing.T) {
	t.Parallel()

	send := func(t *testing.T, f *testFixture, m *chatstate.ChatMachine, behavior chatstate.BusyBehavior) chatstate.SendMessageResult {
		t.Helper()
		var result chatstate.SendMessageResult
		require.NoError(t, m.Update(testutil.Context(t, testutil.WaitShort), func(tx *chatstate.Tx, _ database.Store) error {
			var err error
			result, err = tx.SendMessage(chatstate.SendMessageInput{
				Message:      flaggedMessage(f, "flagged"),
				BusyBehavior: behavior,
				MaxQueueSize: codersdk.DefaultChatMaxQueuedMessagesPerChat,
			})
			return err
		}))
		return result
	}

	t.Run("Idle", func(t *testing.T) {
		t.Parallel()
		f := newTestFixture(t)
		seeded := seedState(t, f, chatstate.StateW)
		result := send(t, f, chatstate.NewChatMachine(f.DB, f.Pub, seeded.chatID), chatstate.BusyBehaviorQueue)
		require.Nil(t, result.QueuedMessage)
		require.Len(t, result.InsertedMessages, 1)
		require.True(t, result.InsertedMessages[0].PromptHooksPending)
	})

	t.Run("Queued", func(t *testing.T) {
		t.Parallel()
		f := newTestFixture(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		created := createTestChat(t, f)
		m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)

		queued := send(t, f, m, chatstate.BusyBehaviorQueue)
		require.NotNil(t, queued.QueuedMessage)
		require.True(t, queued.QueuedMessage.PromptHooksPending)

		var promoted []database.ChatMessage
		require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
			result, err := tx.FinishTurn(chatstate.FinishTurnInput{})
			promoted = result.PromotedMessages
			return err
		}))
		require.Len(t, promoted, 1)
		require.True(t, requireChatMessageByID(ctx, t, f, promoted[0].ID).PromptHooksPending)
	})
}

// TestPromptHooksPendingRowsAreUserVisible checks that a flagged row is a
// user message that both the user and the model see. chatd finds flagged
// rows in the history that the user sees, so any other row would reach the
// model before its hooks ran.
func TestPromptHooksPendingRowsAreUserVisible(t *testing.T) {
	t.Parallel()
	tests := map[string]func(*chatstate.Message){
		"AssistantRole":   func(m *chatstate.Message) { m.Role = database.ChatMessageRoleAssistant },
		"ModelVisibility": func(m *chatstate.Message) { m.Visibility = database.ChatMessageVisibilityModel },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newTestFixture(t)
			ctx := testutil.Context(t, testutil.WaitShort)
			created := createTestChat(t, f)
			message := flaggedMessage(f, "flagged")
			mutate(&message)
			err := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID).Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
				_, err := tx.CommitStep(chatstate.CommitStepInput{Messages: []chatstate.Message{message}})
				return err
			})
			require.ErrorContains(t, err, "prompt_hooks_pending")
		})
	}
}

// TestResolvePromptHooksRejectsMisuse covers what the transition matrix does
// not: new content without a version, and a second resolution, which must be
// loud and must not look like sql.ErrNoRows, which chatd treats as a deleted
// chat.
func TestResolvePromptHooksRejectsMisuse(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)
	flagged := dbgen.ChatMessage(t, f.DB, database.ChatMessage{
		ChatID:             created.Chat.ID,
		Content:            userTextMessage("hello", f.User.ID, f.Model.ID).Content,
		PromptHooksPending: true,
	})
	m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)
	resolve := func(input chatstate.ResolvePromptHooksInput) error {
		input.MessageID = flagged.ID
		return m.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
			_, err := tx.ResolvePromptHooks(input)
			return err
		})
	}

	rewritten, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{codersdk.ChatMessageText("rewritten")})
	require.NoError(t, err)
	require.ErrorContains(t, resolve(chatstate.ResolvePromptHooksInput{Content: rewritten}), "content version")
	require.NoError(t, resolve(chatstate.ResolvePromptHooksInput{}))
	require.ErrorIs(t, resolve(chatstate.ResolvePromptHooksInput{}), chatstate.ErrPromptHooksNotPending)
}

func TestClearContextClearsPromptHooksPending(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	seeded := seedState(t, f, chatstate.StateW)
	flagged := dbgen.ChatMessage(t, f.DB, database.ChatMessage{
		ChatID:             seeded.chatID,
		Content:            userTextMessage("hidden", f.User.ID, f.Model.ID).Content,
		PromptHooksPending: true,
	})

	require.NoError(t, chatstate.NewChatMachine(f.DB, f.Pub, seeded.chatID).Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		_, err := tx.ClearContext(chatstate.ClearContextInput{Messages: clearBoundaryMessages(t)})
		return err
	}))
	require.False(t, requireChatMessageByID(ctx, t, f, flagged.ID).PromptHooksPending)
}

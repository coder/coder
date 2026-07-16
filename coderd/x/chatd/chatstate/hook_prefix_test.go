package chatstate_test

import (
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

// A user_prompt_submit hook prefix for a queued prompt must stay off
// active history until that prompt is promoted, then land immediately
// before it with the prompt's turn ID.
func TestQueuedHookPrefixDeferredUntilPromotion(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)

	created := createTestChat(t, f)
	m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)
	baseHistory := activeHistoryIDs(ctx, t, f, created.Chat.ID)

	turnID := uuid.New()
	prompt := userTextMessage("queued prompt", f.User.ID, f.Model.ID)
	prompt.TurnID = uuid.NullUUID{UUID: turnID, Valid: true}
	prefixContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{codersdk.ChatMessageText("hook context")})
	require.NoError(t, err)
	prefix := chatstate.Message{
		Role:           database.ChatMessageRoleUser,
		Content:        prefixContent,
		Visibility:     database.ChatMessageVisibilityModel,
		ModelConfigID:  uuid.NullUUID{UUID: f.Model.ID, Valid: true},
		ContentVersion: chatprompt.CurrentContentVersion,
	}

	var send chatstate.SendMessageResult
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		var err error
		send, err = tx.SendMessage(chatstate.SendMessageInput{
			Message:        prompt,
			PrefixMessages: []chatstate.Message{prefix},
			BusyBehavior:   chatstate.BusyBehaviorQueue,
		})
		return err
	}))
	require.NotNil(t, send.QueuedMessage)
	require.True(t, send.QueuedMessage.HookPrefix.Valid)
	require.Equal(t, baseHistory, activeHistoryIDs(ctx, t, f, created.Chat.ID),
		"queueing a prompt must not change active history")

	var finish chatstate.FinishTurnResult
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		var err error
		finish, err = tx.FinishTurn(chatstate.FinishTurnInput{})
		return err
	}))
	require.NotNil(t, finish.PromotedMessage)

	// activeHistoryIDs sees user-visible rows only; the model-visible
	// prefix must be fetched from the prompt-side query.
	history := activeHistoryIDs(ctx, t, f, created.Chat.ID)
	require.Len(t, history, len(baseHistory)+1, "promotion inserts the prompt")
	promptRow := requireChatMessageByID(ctx, t, f, history[len(history)-1])
	require.Equal(t, finish.PromotedMessage.ID, promptRow.ID)
	assertChatMessageText(t, promptRow, "queued prompt")

	promptMessages, err := f.DB.GetChatMessagesForPromptByChatID(ctx, created.Chat.ID)
	require.NoError(t, err)
	prefixRow := promptMessages[len(promptMessages)-2]
	require.Equal(t, database.ChatMessageVisibilityModel, prefixRow.Visibility)
	assertChatMessageText(t, prefixRow, "hook context")
	require.Equal(t, uuid.NullUUID{UUID: turnID, Valid: true}, prefixRow.TurnID,
		"prefix adopts the queued prompt's turn ID")
	require.Equal(t, promptRow.ID, promptMessages[len(promptMessages)-1].ID,
		"prefix lands immediately before the promoted prompt")
}

// A user_prompt_submit tool policy for a queued prompt must not touch
// chats.hook_allowed_tools until that prompt is promoted.
func TestQueuedHookAllowedToolsDeferredUntilPromotion(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)

	created := createTestChat(t, f)
	m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)

	policy := pqtype.NullRawMessage{RawMessage: []byte(`["read_file"]`), Valid: true}
	prompt := userTextMessage("queued prompt", f.User.ID, f.Model.ID)
	prompt.TurnID = uuid.NullUUID{UUID: uuid.New(), Valid: true}

	var send chatstate.SendMessageResult
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		var err error
		send, err = tx.SendMessage(chatstate.SendMessageInput{
			Message:          prompt,
			HookAllowedTools: policy,
			BusyBehavior:     chatstate.BusyBehaviorQueue,
		})
		return err
	}))
	require.NotNil(t, send.QueuedMessage)
	require.True(t, send.QueuedMessage.HookAllowedTools.Valid)

	chat, err := f.DB.GetChatByID(ctx, created.Chat.ID)
	require.NoError(t, err)
	require.False(t, chat.HookAllowedTools.Valid,
		"queueing a prompt must not change the chat tool policy")

	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		_, err := tx.FinishTurn(chatstate.FinishTurnInput{})
		return err
	}))

	chat, err = f.DB.GetChatByID(ctx, created.Chat.ID)
	require.NoError(t, err)
	require.True(t, chat.HookAllowedTools.Valid, "promotion applies the queued prompt's tool policy")
	require.JSONEq(t, `["read_file"]`, string(chat.HookAllowedTools.RawMessage))
}

package chatstate_test

import (
	"context"
	"testing"
	"unicode"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/testutil"
)

// Matrix cases for DeleteQueuedMessagesByAutomation. A layout spells the
// queue of a case, head first: 'a' and 'b' are messages of the two
// automations that the transition removes, 'o' is a message of another
// automation, and 'h' is a human message. An upper-case letter puts the
// row under edit.

// automationQueueBase maps each state with a queue to the empty-queue
// state that its seed starts from.
var automationQueueBase = map[chatstate.ExecutionState]chatstate.ExecutionState{
	chatstate.StateE1:   chatstate.StateE0,
	chatstate.StateE1P:  chatstate.StateE0,
	chatstate.StateR1:   chatstate.StateR0,
	chatstate.StateR1P:  chatstate.StateR0,
	chatstate.StateI1:   chatstate.StateI0,
	chatstate.StateI1P:  chatstate.StateI0,
	chatstate.StateA1:   chatstate.StateA0,
	chatstate.StateA1P:  chatstate.StateA0,
	chatstate.StateP:    chatstate.StateR0,
	chatstate.StateXE1:  chatstate.StateE0,
	chatstate.StateXE1P: chatstate.StateE0,
}

func applyDeleteQueuedMessagesByAutomation(automationIDs ...uuid.UUID) applierFn {
	return func(t *testing.T, _ *testFixture, tx *chatstate.Tx, _ seededChat, _ chatstate.ExecutionState, result *transitionCaseResult) error {
		t.Helper()
		var err error
		result.deleteQueuedMessagesByAutomation, err = tx.DeleteQueuedMessagesByAutomation(chatstate.DeleteQueuedMessagesByAutomationInput{
			AutomationIDs: automationIDs,
		})
		return err
	}
}

func automationQueueCase(from, want chatstate.ExecutionState, layout string) transitionCaseSpec {
	automationIDs := map[rune]uuid.UUID{'a': uuid.New(), 'b': uuid.New(), 'o': uuid.New()}
	// The seed fills these and the assertion of the same case reads them.
	kept := []int64{}
	var keptEditingID, removed int64

	seed := func(t *testing.T, f *testFixture, from chatstate.ExecutionState) seededChat {
		t.Helper()
		ctx := testutil.Context(t, testutil.WaitShort)
		base, hasQueue := automationQueueBase[from]
		if !hasQueue {
			base = from
		}
		seeded := seedState(t, f, base)
		message := userTextMessage("queued", f.User.ID, f.Model.ID)
		for _, kind := range layout {
			params := database.InsertChatQueuedMessageWithCreatorParams{
				ChatID:        seeded.chatID,
				Content:       message.Content.RawMessage,
				ModelConfigID: message.ModelConfigID,
				CreatedBy:     f.User.ID,
				BusyBehavior:  database.ChatBusyBehaviorQueue,
			}
			source := unicode.ToLower(kind)
			if automationID, ok := automationIDs[source]; ok {
				params.AutomationID = uuid.NullUUID{UUID: automationID, Valid: true}
				params.InputID = uuid.NullUUID{UUID: uuid.New(), Valid: true}
			}
			row, err := f.DB.InsertChatQueuedMessageWithCreator(ctx, params)
			require.NoError(t, err)
			if unicode.IsUpper(kind) {
				beginQueuedMessageEdit(ctx, t, f, seeded.chatID, row.ID)
			}
			if source == 'a' || source == 'b' {
				removed++
				continue
			}
			kept = append(kept, row.ID)
			if unicode.IsUpper(kind) {
				keptEditingID = row.ID
			}
		}
		switch from {
		case chatstate.StateP:
			pauseSeeded(t, f, seeded)
		case chatstate.StateXE1, chatstate.StateXE1P:
			m := chatstate.NewChatMachine(f.DB, f.Pub, seeded.chatID)
			require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
				_, err := tx.SetArchived(chatstate.SetArchivedInput{Archived: true})
				return err
			}))
		}
		return seeded
	}

	return transitionCaseSpec{
		transition: chatstate.TransitionDeleteQueuedMessagesByAutomation,
		from:       from,
		want:       want,
		seed:       seed,
		apply:      applyDeleteQueuedMessagesByAutomation(automationIDs['a'], automationIDs['b']),
		assert: func(ctx context.Context, t *testing.T, f *testFixture, seeded seededChat, base snapshotBaseline, result transitionCaseResult) {
			got := result.deleteQueuedMessagesByAutomation
			require.Equal(t, removed, got.Deleted, "every message of the two automations is removed")

			// Leaving P for a running state promotes the next row.
			promoted := 0
			if from == chatstate.StateP && (want == chatstate.StateR0 || want == chatstate.StateR1) {
				promoted = 1
			}
			require.Equal(t, kept[promoted:], queuedIDsByPosition(ctx, t, f, seeded.chatID),
				"the other queued messages keep their order")
			assertQueueEditMarkers(ctx, t, f, seeded.chatID, keptEditingID)
			require.Len(t, newActiveMessageIDs(base, activeHistoryIDs(ctx, t, f, seeded.chatID)), promoted,
				"history changes only by a promotion")

			chat := f.readChat(ctx, t, seeded.chatID)
			if from == chatstate.StateP && want != chatstate.StateP {
				require.NotNil(t, got.Chat, "leaving P returns the updated chat")
				require.Equal(t, chat.Status, got.Chat.Status)
				return
			}
			require.Nil(t, got.Chat)
			require.Equal(t, base.chat.Status, chat.Status, "the status stays outside P")
			require.Equal(t, base.chat.Archived, chat.Archived)
		},
	}
}

func automationQueueMatrixCases() []transitionCaseSpec {
	var cases []transitionCaseSpec
	// Without a queue the transition changes nothing.
	for _, state := range []chatstate.ExecutionState{
		chatstate.StateW, chatstate.StateE0, chatstate.StateR0, chatstate.StateI0,
		chatstate.StateA0, chatstate.StateXW, chatstate.StateXE0,
	} {
		cases = append(cases, automationQueueCase(state, state, ""))
	}
	// The head that is left decides the state, as for DeleteQueuedMessage.
	for _, family := range []struct{ empty, ready, blocked chatstate.ExecutionState }{
		{chatstate.StateE0, chatstate.StateE1, chatstate.StateE1P},
		{chatstate.StateR0, chatstate.StateR1, chatstate.StateR1P},
		{chatstate.StateI0, chatstate.StateI1, chatstate.StateI1P},
		{chatstate.StateA0, chatstate.StateA1, chatstate.StateA1P},
		{chatstate.StateXE0, chatstate.StateXE1, chatstate.StateXE1P},
	} {
		cases = append(cases,
			automationQueueCase(family.ready, family.empty, "ab"),
			automationQueueCase(family.ready, family.ready, "aoh"),
			automationQueueCase(family.ready, family.blocked, "aH"),
			automationQueueCase(family.blocked, family.empty, "A"),
			automationQueueCase(family.blocked, family.ready, "Ah"),
			automationQueueCase(family.blocked, family.blocked, "Hab"),
		)
	}
	// P leaves paused when the blocked head goes.
	return append(cases,
		automationQueueCase(chatstate.StateP, chatstate.StateP, "Ha"),
		automationQueueCase(chatstate.StateP, chatstate.StateW, "Aa"),
		automationQueueCase(chatstate.StateP, chatstate.StateR0, "Ah"),
		automationQueueCase(chatstate.StateP, chatstate.StateR1, "Aho"),
	)
}

// TestDeleteQueuedMessagesByAutomationKeepsOtherChats runs the transition on
// one chat. A message of the same automation in another chat stays, because
// that chat is not locked.
func TestDeleteQueuedMessagesByAutomationKeepsOtherChats(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	automationID := uuid.New()
	message := userTextMessage("queued", f.User.ID, f.Model.ID)
	var chatIDs [2]uuid.UUID
	for i := range chatIDs {
		chatIDs[i] = seedState(t, f, chatstate.StateR0).chatID
		_, err := f.DB.InsertChatQueuedMessageWithCreator(ctx, database.InsertChatQueuedMessageWithCreatorParams{
			ChatID:        chatIDs[i],
			Content:       message.Content.RawMessage,
			CreatedBy:     f.User.ID,
			BusyBehavior:  database.ChatBusyBehaviorQueue,
			AutomationID:  uuid.NullUUID{UUID: automationID, Valid: true},
			InputID:       uuid.NullUUID{UUID: uuid.New(), Valid: true},
			ModelConfigID: message.ModelConfigID,
		})
		require.NoError(t, err)
	}

	var result transitionCaseResult
	require.NoError(t, chatstate.NewChatMachine(f.DB, f.Pub, chatIDs[0]).Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		return applyDeleteQueuedMessagesByAutomation(automationID)(t, f, tx, seededChat{}, chatstate.StateR1, &result)
	}))

	require.EqualValues(t, 1, result.deleteQueuedMessagesByAutomation.Deleted)
	require.Empty(t, queuedIDsByPosition(ctx, t, f, chatIDs[0]))
	require.Len(t, queuedIDsByPosition(ctx, t, f, chatIDs[1]), 1)
}

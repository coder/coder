package chatstate_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/testutil"
)

// Matrix cases for queue holds. A held head hides the queue from the
// classifier, so the "0" states (W, E0, R0, I0, A0) can physically
// carry queued rows. These builders seed that shape and cover the
// EditQueuedMessage transition plus the DeleteQueuedMessage and
// PromoteQueuedMessage cells that only exist because of holds.

const (
	// scenarioHold marks EditQueuedMessage cases that set the hold on
	// the queue head, collapsing a "1" state into its "0" sibling.
	scenarioHold scenario = "hold"
	// scenarioRelease marks EditQueuedMessage cases that clear the
	// hold on the head. From W the exposed head is promoted.
	scenarioRelease scenario = "release"
	// scenarioContent marks EditQueuedMessage cases that only replace
	// the row's content and leave the classified state unchanged.
	scenarioContent scenario = "content"
	// scenarioHeldHead marks DeleteQueuedMessage and
	// PromoteQueuedMessage cases seeded with a held head and no rows
	// behind it.
	scenarioHeldHead scenario = "held_head"
	// scenarioHeldHeadWithTail marks the same cases seeded with
	// unheld rows behind the held head.
	scenarioHeldHeadWithTail scenario = "held_head_with_tail"
	// scenarioAroundHold marks the PromoteQueuedMessage case that
	// targets an unheld row behind a held head.
	scenarioAroundHold scenario = "around_hold"
)

// holdQueuedMessage sets held_at directly. Seeding bypasses the state
// machine on purpose; the transition under test is what must go
// through it.
func holdQueuedMessage(ctx context.Context, t *testing.T, f *testFixture, chatID uuid.UUID, id int64) {
	t.Helper()
	_, err := f.DB.UpdateChatQueuedMessageHeld(ctx, database.UpdateChatQueuedMessageHeldParams{
		ChatID: chatID,
		ID:     id,
		Held:   true,
	})
	require.NoError(t, err)
}

// seedHeldHead seeds one of W, E0, R0, I0, or A0 with a held queue head
// and extraUnheld unheld rows behind it. queuedMessageIDs[0] is the held
// head; the rest follow in queue order.
func seedHeldHead(t *testing.T, f *testFixture, from chatstate.ExecutionState, extraUnheld int) seededChat {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitShort)
	rows := 1 + extraUnheld

	var seeded seededChat
	switch from {
	case chatstate.StateA0:
		seeded = seedAOrA1(t, f, rows, "seed_tool_a0_held")
	case chatstate.StateW, chatstate.StateE0, chatstate.StateR0, chatstate.StateI0:
		created := createTestChat(t, f)
		m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)
		var ids []int64
		var bodies []string
		for i := 0; i < rows; i++ {
			body := fmt.Sprintf("queued-held-%s-%d", from, i)
			var sm chatstate.SendMessageResult
			if from == chatstate.StateI0 && i == rows-1 {
				sm = sendInterruptMessage(t, f, m, body)
			} else {
				sm = sendQueuedMessage(t, f, m, body)
			}
			require.NotNil(t, sm.QueuedMessage)
			ids = append(ids, sm.QueuedMessage.ID)
			bodies = append(bodies, body)
		}
		seeded = seededChat{
			chatID:               created.Chat.ID,
			exists:               true,
			initialUserMessageID: firstUserMessageID(ctx, t, f, created.Chat.ID),
			queuedMessageIDs:     ids,
			queuedMessageBodies:  bodies,
		}
	default:
		t.Fatalf("seedHeldHead: %s is not a held-head state", from)
	}

	holdQueuedMessage(ctx, t, f, seeded.chatID, seeded.queuedMessageIDs[0])

	m := chatstate.NewChatMachine(f.DB, f.Pub, seeded.chatID)
	switch from {
	case chatstate.StateW:
		// R0 (held head) -> W.
		require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
			_, err := tx.FinishTurn(chatstate.FinishTurnInput{})
			return err
		}))
	case chatstate.StateE0:
		require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
			_, err := tx.FinishError(chatstate.FinishErrorInput{
				LastError: pqtype.NullRawMessage{
					RawMessage: json.RawMessage(`{"message":"boom"}`),
					Valid:      true,
				},
			})
			return err
		}))
	}
	return seeded
}

func applyEditQueuedMessage(t *testing.T, _ *testFixture, tx *chatstate.Tx, seeded seededChat, _ chatstate.ExecutionState, result *transitionCaseResult) error {
	t.Helper()
	var targetQueueID int64
	if len(seeded.queuedMessageIDs) > 0 {
		targetQueueID = seeded.queuedMessageIDs[0]
	}
	held := true
	var err error
	result.editQueuedMessage, err = tx.EditQueuedMessage(chatstate.EditQueuedMessageInput{
		QueuedMessageID: targetQueueID,
		Held:            &held,
	})
	return err
}

// editQueuedHoldCase holds the head of a single-row "1" state, landing
// in the "0" sibling with the row still queued.
func editQueuedHoldCase(from, want chatstate.ExecutionState) transitionCaseSpec {
	return transitionCaseSpec{
		transition: chatstate.TransitionEditQueuedMessage,
		from:       from,
		want:       want,
		scenario:   scenarioHold,
		apply:      applyEditQueuedMessage,
		assert: func(ctx context.Context, t *testing.T, f *testFixture, seeded seededChat, base snapshotBaseline, result transitionCaseResult) {
			after, err := f.DB.GetChatByID(ctx, seeded.chatID)
			require.NoError(t, err)
			require.Equal(t, base.chat.Status, after.Status, "holding does not change status")
			require.Equal(t, base.chat.LastError, after.LastError, "holding preserves last_error")
			require.Greater(t, after.QueueVersion, base.queueVersion, "holding advances queue_version")
			require.Equal(t, base.queueIDs, queuedIDsByPosition(ctx, t, f, seeded.chatID), "holding preserves the queue")
			require.Equal(t, base.historyIDs, activeHistoryIDs(ctx, t, f, seeded.chatID), "holding does not touch history")
			row := requireQueuedMessageByID(ctx, t, f, seeded.chatID, seeded.queuedMessageIDs[0])
			require.True(t, row.HeldAt.Valid, "head is held")
			require.True(t, result.editQueuedMessage.QueuedMessage.HeldAt.Valid)
			require.Nil(t, result.editQueuedMessage.PromotedMessage, "holding never promotes")
		},
	}
}

// editQueuedContentCase replaces the content of the head row without
// touching the hold. The classified state is unchanged. For "0" states
// the head is held.
func editQueuedContentCase(from chatstate.ExecutionState) transitionCaseSpec {
	const newBody = "edited-queued-body"
	spec := transitionCaseSpec{
		transition: chatstate.TransitionEditQueuedMessage,
		from:       from,
		want:       from,
		scenario:   scenarioContent,
		apply: func(t *testing.T, _ *testFixture, tx *chatstate.Tx, seeded seededChat, _ chatstate.ExecutionState, result *transitionCaseResult) error {
			t.Helper()
			var err error
			result.editQueuedMessage, err = tx.EditQueuedMessage(chatstate.EditQueuedMessageInput{
				QueuedMessageID: seeded.queuedMessageIDs[0],
				Content:         userMessageContent(t, newBody),
			})
			return err
		},
		assert: func(ctx context.Context, t *testing.T, f *testFixture, seeded seededChat, base snapshotBaseline, result transitionCaseResult) {
			after, err := f.DB.GetChatByID(ctx, seeded.chatID)
			require.NoError(t, err)
			require.Equal(t, base.chat.Status, after.Status, "content edit does not change status")
			require.Greater(t, after.QueueVersion, base.queueVersion, "content edit advances queue_version")
			require.Equal(t, base.queueIDs, queuedIDsByPosition(ctx, t, f, seeded.chatID), "content edit preserves the queue")
			require.Equal(t, base.historyIDs, activeHistoryIDs(ctx, t, f, seeded.chatID), "content edit does not touch history")
			row := requireQueuedMessageByID(ctx, t, f, seeded.chatID, seeded.queuedMessageIDs[0])
			assertQueuedMessageText(t, row, newBody)
			assertQueuedMessageText(t, result.editQueuedMessage.QueuedMessage, newBody)
			wantHeld := isZeroQueueState(from)
			require.Equal(t, wantHeld, row.HeldAt.Valid, "content edit leaves the hold as it was")
			require.True(t, row.ModelConfigID.Valid, "content edit keeps the model override")
			require.Nil(t, result.editQueuedMessage.PromotedMessage)
		},
	}
	if isZeroQueueState(from) {
		spec.seed = func(t *testing.T, f *testFixture, from chatstate.ExecutionState) seededChat {
			return seedHeldHead(t, f, from, 0)
		}
	}
	return spec
}

// editQueuedReleaseCase clears the hold on the head of a "0" state
// seeded with extraUnheld rows behind it. From W the head is promoted
// into history in the same transaction; elsewhere it becomes the
// promotable head and the chat lands in the "1" sibling.
func editQueuedReleaseCase(from, want chatstate.ExecutionState, extraUnheld int) transitionCaseSpec {
	sc := scenarioRelease
	if extraUnheld > 0 {
		sc = scenario(string(scenarioRelease) + "_with_tail")
	}
	return transitionCaseSpec{
		transition: chatstate.TransitionEditQueuedMessage,
		from:       from,
		want:       want,
		scenario:   sc,
		seed: func(t *testing.T, f *testFixture, from chatstate.ExecutionState) seededChat {
			return seedHeldHead(t, f, from, extraUnheld)
		},
		apply: func(t *testing.T, _ *testFixture, tx *chatstate.Tx, seeded seededChat, _ chatstate.ExecutionState, result *transitionCaseResult) error {
			t.Helper()
			held := false
			var err error
			result.editQueuedMessage, err = tx.EditQueuedMessage(chatstate.EditQueuedMessageInput{
				QueuedMessageID: seeded.queuedMessageIDs[0],
				Held:            &held,
			})
			return err
		},
		assert: func(ctx context.Context, t *testing.T, f *testFixture, seeded seededChat, base snapshotBaseline, result transitionCaseResult) {
			after, err := f.DB.GetChatByID(ctx, seeded.chatID)
			require.NoError(t, err)
			headID := seeded.queuedMessageIDs[0]
			require.False(t, result.editQueuedMessage.QueuedMessage.HeldAt.Valid, "release clears held_at")
			if from == chatstate.StateW {
				require.NotNil(t, result.editQueuedMessage.PromotedMessage, "release from W promotes the head")
				require.Equal(t, database.ChatStatusRunning, after.Status)
				require.False(t, after.LastError.Valid, "promotion clears last_error")
				requireQueuedMessageDeleted(ctx, t, f, seeded.chatID, headID)
				require.Equal(t, base.queueIDs[1:], queuedIDsByPosition(ctx, t, f, seeded.chatID), "rows behind the head stay queued")
				promoted := requireChatMessageByID(ctx, t, f, result.editQueuedMessage.PromotedMessage.ID)
				assertChatMessageText(t, promoted, seeded.queuedMessageBodies[0])
				require.Contains(t, newActiveMessageIDs(base, activeHistoryIDs(ctx, t, f, seeded.chatID)), promoted.ID)
				return
			}
			require.Nil(t, result.editQueuedMessage.PromotedMessage, "release outside W does not promote")
			require.Equal(t, base.chat.Status, after.Status, "release outside W keeps the status")
			require.Equal(t, base.chat.LastError, after.LastError, "release outside W keeps last_error")
			require.Equal(t, base.queueIDs, queuedIDsByPosition(ctx, t, f, seeded.chatID), "release keeps the queue")
			require.Equal(t, base.historyIDs, activeHistoryIDs(ctx, t, f, seeded.chatID), "release does not touch history")
			row := requireQueuedMessageByID(ctx, t, f, seeded.chatID, headID)
			require.False(t, row.HeldAt.Valid)
		},
	}
}

// heldDeleteQueuedCase deletes the held head of a "0" state. From W
// with unheld rows behind it the next row is promoted.
func heldDeleteQueuedCase(from, want chatstate.ExecutionState, extraUnheld int) transitionCaseSpec {
	sc := scenarioHeldHead
	if extraUnheld > 0 {
		sc = scenarioHeldHeadWithTail
	}
	return transitionCaseSpec{
		transition: chatstate.TransitionDeleteQueuedMessage,
		from:       from,
		want:       want,
		scenario:   sc,
		seed: func(t *testing.T, f *testFixture, from chatstate.ExecutionState) seededChat {
			return seedHeldHead(t, f, from, extraUnheld)
		},
		apply: applyDeleteQueuedMessage,
		assert: func(ctx context.Context, t *testing.T, f *testFixture, seeded seededChat, base snapshotBaseline, result transitionCaseResult) {
			after, err := f.DB.GetChatByID(ctx, seeded.chatID)
			require.NoError(t, err)
			headID := seeded.queuedMessageIDs[0]
			require.Equal(t, headID, result.deleteQueuedMessage.DeletedQueuedMessage.ID)
			requireQueuedMessageDeleted(ctx, t, f, seeded.chatID, headID)
			remaining := queuedIDsByPosition(ctx, t, f, seeded.chatID)
			if from == chatstate.StateW && extraUnheld > 0 {
				require.NotNil(t, result.deleteQueuedMessage.PromotedMessage, "deleting a held head from W promotes the exposed head")
				require.Equal(t, database.ChatStatusRunning, after.Status)
				require.Equal(t, base.queueIDs[2:], remaining, "the exposed head left the queue")
				promoted := requireChatMessageByID(ctx, t, f, result.deleteQueuedMessage.PromotedMessage.ID)
				assertChatMessageText(t, promoted, seeded.queuedMessageBodies[1])
				return
			}
			require.Nil(t, result.deleteQueuedMessage.PromotedMessage)
			require.Equal(t, base.chat.Status, after.Status, "delete keeps the status")
			require.Equal(t, base.queueIDs[1:], remaining, "delete removes exactly the head")
			require.Equal(t, base.historyIDs, activeHistoryIDs(ctx, t, f, seeded.chatID), "delete does not touch history")
		},
	}
}

// heldPromoteQueuedCase runs "send now" on a "0" state with a held
// head. targetIdx 0 targets the held head itself (its hold is
// released); a later index targets an unheld row behind it, which goes
// around the hold. W/E0/A0 pop the target into history; R0/I0 reorder
// and land in I1.
func heldPromoteQueuedCase(from, want chatstate.ExecutionState, extraUnheld, targetIdx int) transitionCaseSpec {
	sc := scenarioHeldHead
	switch {
	case targetIdx > 0:
		sc = scenarioAroundHold
	case extraUnheld > 0:
		sc = scenarioHeldHeadWithTail
	}
	return transitionCaseSpec{
		transition: chatstate.TransitionPromoteQueuedMessage,
		from:       from,
		want:       want,
		scenario:   sc,
		seed: func(t *testing.T, f *testFixture, from chatstate.ExecutionState) seededChat {
			return seedHeldHead(t, f, from, extraUnheld)
		},
		apply: func(t *testing.T, _ *testFixture, tx *chatstate.Tx, seeded seededChat, _ chatstate.ExecutionState, result *transitionCaseResult) error {
			t.Helper()
			require.Less(t, targetIdx, len(seeded.queuedMessageIDs))
			var err error
			result.promoteQueuedMessage, err = tx.PromoteQueuedMessage(chatstate.PromoteQueuedMessageInput{
				QueuedMessageID: seeded.queuedMessageIDs[targetIdx],
			})
			return err
		},
		assert: func(ctx context.Context, t *testing.T, f *testFixture, seeded seededChat, base snapshotBaseline, result transitionCaseResult) {
			after, err := f.DB.GetChatByID(ctx, seeded.chatID)
			require.NoError(t, err)
			targetID := seeded.queuedMessageIDs[targetIdx]
			require.Equal(t, targetID, result.promoteQueuedMessage.QueuedMessage.ID)
			require.False(t, result.promoteQueuedMessage.QueuedMessage.HeldAt.Valid, "promote releases the target's hold")
			remaining := queuedIDsByPosition(ctx, t, f, seeded.chatID)
			switch from {
			case chatstate.StateW, chatstate.StateE0, chatstate.StateA0:
				require.NotNil(t, result.promoteQueuedMessage.InsertedMessage, "promote pops the target into history")
				require.Equal(t, database.ChatStatusRunning, after.Status)
				require.False(t, after.LastError.Valid, "promote clears last_error")
				requireQueuedMessageDeleted(ctx, t, f, seeded.chatID, targetID)
				require.Equal(t, remainingExcluding(base.queueIDs, targetIdx), remaining)
				promoted := requireChatMessageByID(ctx, t, f, result.promoteQueuedMessage.InsertedMessage.ID)
				assertChatMessageText(t, promoted, seeded.queuedMessageBodies[targetIdx])
			case chatstate.StateR0, chatstate.StateI0:
				require.Nil(t, result.promoteQueuedMessage.InsertedMessage, "promote from a busy state does not insert history")
				require.Equal(t, database.ChatStatusInterrupting, after.Status)
				require.Equal(t, base.historyIDs, activeHistoryIDs(ctx, t, f, seeded.chatID))
				require.NotEmpty(t, remaining)
				require.Equal(t, targetID, remaining[0], "target is now the head")
				head := requireQueuedMessageByID(ctx, t, f, seeded.chatID, targetID)
				require.False(t, head.HeldAt.Valid, "the new head is promotable")
				if targetIdx > 0 {
					// The held row is still held, just no longer first.
					heldRow := requireQueuedMessageByID(ctx, t, f, seeded.chatID, seeded.queuedMessageIDs[0])
					require.True(t, heldRow.HeldAt.Valid, "going around the hold leaves it in place")
				}
			default:
				t.Fatalf("unexpected from state %s", from)
			}
		},
	}
}

// isZeroQueueState reports whether s is one of the states whose
// classifier sees no promotable head.
func isZeroQueueState(s chatstate.ExecutionState) bool {
	switch s {
	case chatstate.StateW, chatstate.StateE0, chatstate.StateR0, chatstate.StateI0, chatstate.StateA0:
		return true
	}
	return false
}

// heldQueueMatrixCases enumerates every matrix cell that exists because
// of holds, plus the EditQueuedMessage cells on the "1" states. The
// cases mirror queueTransitionRows: one row per status, both queue
// variants, the same outputs.
func heldQueueMatrixCases() []transitionCaseSpec {
	// zero is the variant whose head is held; one is the variant with a
	// promotable head. Waiting has no "1" variant: releasing or deleting
	// a held head there promotes the exposed head, covered below.
	statuses := []struct{ zero, one chatstate.ExecutionState }{
		{chatstate.StateE0, chatstate.StateE1},
		{chatstate.StateR0, chatstate.StateR1},
		{chatstate.StateI0, chatstate.StateI1},
		{chatstate.StateA0, chatstate.StateA1},
	}

	cases := []transitionCaseSpec{
		editQueuedContentCase(chatstate.StateW),
		editQueuedReleaseCase(chatstate.StateW, chatstate.StateR0, 0),
		editQueuedReleaseCase(chatstate.StateW, chatstate.StateR1, 1),
		heldDeleteQueuedCase(chatstate.StateW, chatstate.StateW, 0),
		heldDeleteQueuedCase(chatstate.StateW, chatstate.StateR0, 1),
		heldDeleteQueuedCase(chatstate.StateW, chatstate.StateR1, 2),
		heldPromoteQueuedCase(chatstate.StateW, chatstate.StateR0, 0, 0),
		heldPromoteQueuedCase(chatstate.StateW, chatstate.StateR1, 1, 0),
	}
	for _, s := range statuses {
		cases = append(cases,
			// Hold the head of the "1" variant; content edits leave
			// either variant where it is.
			editQueuedHoldCase(s.one, s.zero),
			editQueuedContentCase(s.zero),
			editQueuedContentCase(s.one),
			// Release or delete the held head: the "1" variant appears
			// when an unheld row is exposed.
			editQueuedReleaseCase(s.zero, s.one, 0),
			heldDeleteQueuedCase(s.zero, s.zero, 0),
			heldDeleteQueuedCase(s.zero, s.one, 1),
		)
	}

	// PromoteQueuedMessage from the "0" variants. Idle-like statuses pop
	// the target into history; busy statuses reorder it to the head and
	// interrupt. Targeting a row behind the held head goes around it.
	cases = append(cases,
		heldPromoteQueuedCase(chatstate.StateE0, chatstate.StateR0, 0, 0),
		heldPromoteQueuedCase(chatstate.StateE0, chatstate.StateR1, 1, 0),
		heldPromoteQueuedCase(chatstate.StateA0, chatstate.StateR0, 0, 0),
		heldPromoteQueuedCase(chatstate.StateA0, chatstate.StateR1, 1, 0),
		heldPromoteQueuedCase(chatstate.StateR0, chatstate.StateI1, 0, 0),
		heldPromoteQueuedCase(chatstate.StateR0, chatstate.StateI1, 1, 1),
		heldPromoteQueuedCase(chatstate.StateI0, chatstate.StateI1, 0, 0),
	)
	return cases
}

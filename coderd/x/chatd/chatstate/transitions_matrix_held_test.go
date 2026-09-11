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
	// hold on the head, exposing it as promotable.
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
// seeded with extraUnheld rows behind it. The head becomes promotable
// and the chat lands in the "1" sibling. Not valid from W; see
// editQueuedReleaseIdleHeadRefusedCase.
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
		apply: applyReleaseHead,
		assert: func(ctx context.Context, t *testing.T, f *testFixture, seeded seededChat, base snapshotBaseline, result transitionCaseResult) {
			after, err := f.DB.GetChatByID(ctx, seeded.chatID)
			require.NoError(t, err)
			headID := seeded.queuedMessageIDs[0]
			require.False(t, result.editQueuedMessage.QueuedMessage.HeldAt.Valid, "release clears held_at")
			require.Equal(t, base.chat.Status, after.Status, "release keeps the status")
			require.Equal(t, base.chat.LastError, after.LastError, "release keeps last_error")
			require.Equal(t, base.queueIDs, queuedIDsByPosition(ctx, t, f, seeded.chatID), "release keeps the queue")
			require.Equal(t, base.historyIDs, activeHistoryIDs(ctx, t, f, seeded.chatID), "release does not touch history")
			row := requireQueuedMessageByID(ctx, t, f, seeded.chatID, headID)
			require.False(t, row.HeldAt.Valid)
		},
	}
}

func applyReleaseHead(t *testing.T, _ *testFixture, tx *chatstate.Tx, seeded seededChat, _ chatstate.ExecutionState, result *transitionCaseResult) error {
	t.Helper()
	held := false
	var err error
	result.editQueuedMessage, err = tx.EditQueuedMessage(chatstate.EditQueuedMessageInput{
		QueuedMessageID: seeded.queuedMessageIDs[0],
		Held:            &held,
	})
	return err
}

// editQueuedReleaseIdleHeadRefusedCase clears the hold on the head of
// an idle chat. That would leave W with a promotable head, so Update
// rolls the transaction back and nothing is written; releasing that
// row is PromoteQueuedMessage's job.
func editQueuedReleaseIdleHeadRefusedCase() transitionCaseSpec {
	return transitionCaseSpec{
		transition: chatstate.TransitionEditQueuedMessage,
		from:       chatstate.StateW,
		want:       chatstate.StateW,
		scenario:   scenario(string(scenarioRelease) + "_refused"),
		seed: func(t *testing.T, f *testFixture, from chatstate.ExecutionState) seededChat {
			return seedHeldHead(t, f, from, 0)
		},
		apply: applyReleaseHead,
		assertFailure: func(ctx context.Context, t *testing.T, f *testFixture, seeded seededChat, base snapshotBaseline, err error) {
			require.ErrorIs(t, err, chatstate.ErrInvalidResultState)
			assertNoMutationOrPublish(ctx, t, f, seeded.chatID, base)
		},
	}
}

// editQueuedMoveHoldCase holds a row behind the held head. A chat has
// one held row, so the hold moves: the old head is released and, being
// promotable again, puts the chat back in the "1" variant.
func editQueuedMoveHoldCase(from, want chatstate.ExecutionState) transitionCaseSpec {
	return transitionCaseSpec{
		transition: chatstate.TransitionEditQueuedMessage,
		from:       from,
		want:       want,
		scenario:   scenario(string(scenarioHold) + "_moved"),
		seed: func(t *testing.T, f *testFixture, from chatstate.ExecutionState) seededChat {
			return seedHeldHead(t, f, from, 1)
		},
		apply: func(t *testing.T, _ *testFixture, tx *chatstate.Tx, seeded seededChat, _ chatstate.ExecutionState, result *transitionCaseResult) error {
			t.Helper()
			held := true
			var err error
			result.editQueuedMessage, err = tx.EditQueuedMessage(chatstate.EditQueuedMessageInput{
				QueuedMessageID: seeded.queuedMessageIDs[1],
				Held:            &held,
			})
			return err
		},
		assert: func(ctx context.Context, t *testing.T, f *testFixture, seeded seededChat, base snapshotBaseline, result transitionCaseResult) {
			require.True(t, result.editQueuedMessage.QueuedMessage.HeldAt.Valid)
			require.Equal(t, base.queueIDs, queuedIDsByPosition(ctx, t, f, seeded.chatID), "moving the hold keeps the order")
			require.Equal(t, base.historyIDs, activeHistoryIDs(ctx, t, f, seeded.chatID))
			head := requireQueuedMessageByID(ctx, t, f, seeded.chatID, seeded.queuedMessageIDs[0])
			require.False(t, head.HeldAt.Valid, "the previous hold is released")
		},
	}
}

// heldDeleteQueuedCase deletes the held head of a "0" state. With
// unheld rows behind it the next row becomes the promotable head and
// the chat lands in the "1" sibling. Not valid from W with a tail; see
// heldDeleteIdleHeadRefusedCase.
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
			require.Equal(t, base.chat.Status, after.Status, "delete keeps the status")
			require.Equal(t, base.queueIDs[1:], queuedIDsByPosition(ctx, t, f, seeded.chatID), "delete removes exactly the head")
			require.Equal(t, base.historyIDs, activeHistoryIDs(ctx, t, f, seeded.chatID), "delete does not touch history")
		},
	}
}

// heldDeleteIdleHeadRefusedCase deletes the held head of an idle chat
// that has a row behind it. That would leave W with a promotable head,
// so Update rolls the transaction back; the caller promotes the
// successor first.
func heldDeleteIdleHeadRefusedCase() transitionCaseSpec {
	return transitionCaseSpec{
		transition: chatstate.TransitionDeleteQueuedMessage,
		from:       chatstate.StateW,
		want:       chatstate.StateW,
		scenario:   scenario(string(scenarioHeldHeadWithTail) + "_refused"),
		seed: func(t *testing.T, f *testFixture, from chatstate.ExecutionState) seededChat {
			return seedHeldHead(t, f, from, 1)
		},
		apply: applyDeleteQueuedMessage,
		assertFailure: func(ctx context.Context, t *testing.T, f *testFixture, seeded seededChat, base snapshotBaseline, err error) {
			require.ErrorIs(t, err, chatstate.ErrInvalidResultState)
			assertNoMutationOrPublish(ctx, t, f, seeded.chatID, base)
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

// heldSendMessageCase covers the SendMessage outputs that exist only
// because a held head hides the queue: a queued send from a "0" state
// appends behind the held head and stays in that "0" variant (or moves
// to the busy status's "0" variant on interrupt), instead of landing
// in the "1" sibling the unheld shape produces.
func heldSendMessageCase(from, want chatstate.ExecutionState, busy chatstate.BusyBehavior) transitionCaseSpec {
	sc := scenario(string(scenarioQueue) + "_behind_held_head")
	apply := applySendMessageQueue
	if busy == chatstate.BusyBehaviorInterrupt {
		sc = scenario(string(scenarioInterrupt) + "_behind_held_head")
		apply = applySendMessageInterrupt
	}
	return transitionCaseSpec{
		transition: chatstate.TransitionSendMessage,
		from:       from,
		want:       want,
		scenario:   sc,
		seed: func(t *testing.T, f *testFixture, from chatstate.ExecutionState) seededChat {
			return seedHeldHead(t, f, from, 0)
		},
		apply: apply,
		assert: func(ctx context.Context, t *testing.T, f *testFixture, seeded seededChat, base snapshotBaseline, result transitionCaseResult) {
			require.NotNil(t, result.sendMessage.QueuedMessage, "the send is queued")
			for _, inserted := range result.sendMessage.InsertedMessages {
				// An interrupt from requires_action inserts synthetic
				// tool cancellations; no user message may be promoted.
				require.NotEqual(t, database.ChatMessageRoleUser, inserted.Role, "nothing is promoted past the held head")
			}
			wantQueue := append(append([]int64{}, base.queueIDs...), result.sendMessage.QueuedMessage.ID)
			require.Equal(t, wantQueue, queuedIDsByPosition(ctx, t, f, seeded.chatID), "the send lands behind the held head")
			afterHistory := activeHistoryIDs(ctx, t, f, seeded.chatID)
			require.Equal(t, base.historyIDs, afterHistory[:len(base.historyIDs)], "existing history is untouched")
			head := requireQueuedMessageByID(ctx, t, f, seeded.chatID, seeded.queuedMessageIDs[0])
			require.True(t, head.HeldAt.Valid, "the head stays held")
		},
	}
}

// heldSendMessageE1Case covers SendMessage from E1 whose queue is an
// unheld head followed by a held row. The send appends and promotes
// the head as usual; the held row is then the head, so the chat lands
// in R0 rather than R1.
func heldSendMessageE1Case() transitionCaseSpec {
	return transitionCaseSpec{
		transition: chatstate.TransitionSendMessage,
		from:       chatstate.StateE1,
		want:       chatstate.StateR0,
		scenario:   scenario(string(scenarioQueue) + "_held_behind_head"),
		seed: func(t *testing.T, f *testFixture, _ chatstate.ExecutionState) seededChat {
			// E0 with a held head and one row behind it, then move the
			// hold to the second row: the head is promotable, so E1.
			seeded := seedHeldHead(t, f, chatstate.StateE0, 1)
			ctx := testutil.Context(t, testutil.WaitShort)
			_, err := f.DB.UpdateChatQueuedMessageHeld(ctx, database.UpdateChatQueuedMessageHeldParams{
				ChatID: seeded.chatID, ID: seeded.queuedMessageIDs[0], Held: false,
			})
			require.NoError(t, err)
			holdQueuedMessage(ctx, t, f, seeded.chatID, seeded.queuedMessageIDs[1])
			return seeded
		},
		apply: applySendMessageQueue,
		assert: func(ctx context.Context, t *testing.T, f *testFixture, seeded seededChat, base snapshotBaseline, result transitionCaseResult) {
			after, err := f.DB.GetChatByID(ctx, seeded.chatID)
			require.NoError(t, err)
			require.Equal(t, database.ChatStatusRunning, after.Status)
			require.NotNil(t, result.sendMessage.QueuedMessage)
			require.Len(t, result.sendMessage.InsertedMessages, 1, "the previous head is promoted")
			assertChatMessageText(t, requireChatMessageByID(ctx, t, f, result.sendMessage.InsertedMessages[0].ID), seeded.queuedMessageBodies[0])
			wantQueue := []int64{seeded.queuedMessageIDs[1], result.sendMessage.QueuedMessage.ID}
			require.Equal(t, wantQueue, queuedIDsByPosition(ctx, t, f, seeded.chatID))
			head := requireQueuedMessageByID(ctx, t, f, seeded.chatID, seeded.queuedMessageIDs[1])
			require.True(t, head.HeldAt.Valid, "the held row is now the head")
		},
	}
}

// heldQueueMatrixCases enumerates every matrix cell that exists because
// of holds, plus the EditQueuedMessage cells on the "1" states, and the
// two W rewrites the transitions refuse.
func heldQueueMatrixCases() []transitionCaseSpec {
	return []transitionCaseSpec{
		// EditQueuedMessage: hold the head of a "1" state.
		editQueuedHoldCase(chatstate.StateE1, chatstate.StateE0),
		editQueuedHoldCase(chatstate.StateR1, chatstate.StateR0),
		editQueuedHoldCase(chatstate.StateI1, chatstate.StateI0),
		editQueuedHoldCase(chatstate.StateA1, chatstate.StateA0),

		// EditQueuedMessage: content only, state unchanged.
		editQueuedContentCase(chatstate.StateW),
		editQueuedContentCase(chatstate.StateE0),
		editQueuedContentCase(chatstate.StateE1),
		editQueuedContentCase(chatstate.StateR0),
		editQueuedContentCase(chatstate.StateR1),
		editQueuedContentCase(chatstate.StateI0),
		editQueuedContentCase(chatstate.StateI1),
		editQueuedContentCase(chatstate.StateA0),
		editQueuedContentCase(chatstate.StateA1),

		// EditQueuedMessage: release the held head. From W releasing the
		// head is refused; moving the hold off it is refused for the same
		// reason and covered by TestHold_MoveHoldOffIdleHeadRefused.
		editQueuedReleaseIdleHeadRefusedCase(),
		editQueuedReleaseCase(chatstate.StateE0, chatstate.StateE1, 0),
		editQueuedReleaseCase(chatstate.StateR0, chatstate.StateR1, 0),
		editQueuedReleaseCase(chatstate.StateI0, chatstate.StateI1, 0),
		editQueuedReleaseCase(chatstate.StateA0, chatstate.StateA1, 0),

		// EditQueuedMessage: move the hold from the head to the row
		// behind it.
		editQueuedMoveHoldCase(chatstate.StateE0, chatstate.StateE1),
		editQueuedMoveHoldCase(chatstate.StateR0, chatstate.StateR1),
		editQueuedMoveHoldCase(chatstate.StateI0, chatstate.StateI1),
		editQueuedMoveHoldCase(chatstate.StateA0, chatstate.StateA1),

		// DeleteQueuedMessage on a held head. From W a row behind the
		// head would become promotable, so only the last row may go.
		heldDeleteQueuedCase(chatstate.StateW, chatstate.StateW, 0),
		heldDeleteIdleHeadRefusedCase(),
		heldDeleteQueuedCase(chatstate.StateE0, chatstate.StateE0, 0),
		heldDeleteQueuedCase(chatstate.StateE0, chatstate.StateE1, 1),
		heldDeleteQueuedCase(chatstate.StateR0, chatstate.StateR0, 0),
		heldDeleteQueuedCase(chatstate.StateR0, chatstate.StateR1, 1),
		heldDeleteQueuedCase(chatstate.StateI0, chatstate.StateI0, 0),
		heldDeleteQueuedCase(chatstate.StateI0, chatstate.StateI1, 1),
		heldDeleteQueuedCase(chatstate.StateA0, chatstate.StateA0, 0),
		heldDeleteQueuedCase(chatstate.StateA0, chatstate.StateA1, 1),

		// PromoteQueuedMessage on a held head.
		heldPromoteQueuedCase(chatstate.StateW, chatstate.StateR0, 0, 0),
		heldPromoteQueuedCase(chatstate.StateW, chatstate.StateR1, 1, 0),
		heldPromoteQueuedCase(chatstate.StateE0, chatstate.StateR0, 0, 0),
		heldPromoteQueuedCase(chatstate.StateE0, chatstate.StateR1, 1, 0),
		heldPromoteQueuedCase(chatstate.StateA0, chatstate.StateR0, 0, 0),
		heldPromoteQueuedCase(chatstate.StateA0, chatstate.StateR1, 1, 0),
		heldPromoteQueuedCase(chatstate.StateR0, chatstate.StateI1, 0, 0),
		heldPromoteQueuedCase(chatstate.StateR0, chatstate.StateI1, 1, 1),
		heldPromoteQueuedCase(chatstate.StateI0, chatstate.StateI1, 0, 0),

		// SendMessage with a held head: the send queues behind it and
		// the queue stays hidden, so the busy "0" variants are outputs.
		heldSendMessageCase(chatstate.StateR0, chatstate.StateR0, chatstate.BusyBehaviorQueue),
		heldSendMessageCase(chatstate.StateR0, chatstate.StateI0, chatstate.BusyBehaviorInterrupt),
		heldSendMessageCase(chatstate.StateI0, chatstate.StateI0, chatstate.BusyBehaviorQueue),
		heldSendMessageCase(chatstate.StateA0, chatstate.StateA0, chatstate.BusyBehaviorQueue),
		heldSendMessageCase(chatstate.StateA0, chatstate.StateR0, chatstate.BusyBehaviorInterrupt),
		heldSendMessageE1Case(),
	}
}

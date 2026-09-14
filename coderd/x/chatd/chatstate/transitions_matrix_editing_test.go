package chatstate_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/testutil"
)

// Matrix cases for queued-message edits: the boundary transitions that can land
// in P, EditQueuedMessage on the states that have rows, and P's own
// row. The harness asserts the post-state and the snapshot bump; the
// cases below add only the fact that distinguishes each cell.

const (
	scenarioEditingHead scenario = "editing_head"
	scenarioMoveEdit    scenario = "move_edit"
	scenarioRelease     scenario = "release"
	scenarioRefused     scenario = "refused"
)

// beginQueuedMessageEdit sets editing_since directly, bypassing the state machine,
// so the transition under test is the only one exercised.
func beginQueuedMessageEdit(ctx context.Context, t *testing.T, f *testFixture, chatID uuid.UUID, id int64) {
	t.Helper()
	_, err := f.DB.UpdateChatQueuedMessageEditing(ctx, database.UpdateChatQueuedMessageEditingParams{
		ChatID:  chatID,
		ID:      id,
		Editing: true,
	})
	require.NoError(t, err)
}

// seedEditingHead seeds a "1" state (E1, R1, I1, A1) whose head is under edit,
// with extra rows behind it.
func seedEditingHead(t *testing.T, f *testFixture, from chatstate.ExecutionState, extra int) seededChat {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitShort)
	rows := 1 + extra

	var seeded seededChat
	switch from {
	case chatstate.StateA1:
		seeded = seedAOrA1(t, f, rows, "seed_tool_a1_editing")
	case chatstate.StateE1, chatstate.StateR1, chatstate.StateI1:
		created := createTestChat(t, f)
		m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)
		var ids []int64
		var bodies []string
		for i := 0; i < rows; i++ {
			body := fmt.Sprintf("queued-editing-%s-%d", from, i)
			var sm chatstate.SendMessageResult
			if from == chatstate.StateI1 && i == rows-1 {
				sm = sendInterruptMessage(t, f, m, body)
			} else {
				sm = sendQueuedMessage(t, f, m, body)
			}
			require.NotNil(t, sm.QueuedMessage)
			ids = append(ids, sm.QueuedMessage.ID)
			bodies = append(bodies, body)
		}
		if from == chatstate.StateE1 {
			require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
				_, err := tx.FinishError(chatstate.FinishErrorInput{
					LastError: pqtype.NullRawMessage{RawMessage: json.RawMessage(`{"message":"boom"}`), Valid: true},
				})
				return err
			}))
		}
		seeded = seededChat{
			chatID:               created.Chat.ID,
			exists:               true,
			initialUserMessageID: firstUserMessageID(ctx, t, f, created.Chat.ID),
			queuedMessageIDs:     ids,
			queuedMessageBodies:  bodies,
		}
	default:
		t.Fatalf("seedEditingHead: %s has no rows", from)
	}
	beginQueuedMessageEdit(ctx, t, f, seeded.chatID, seeded.queuedMessageIDs[0])
	return seeded
}

// seedPaused seeds P: a head under edit with extra rows behind it,
// after FinishTurn.
func seedPaused(t *testing.T, f *testFixture, extra int) seededChat {
	t.Helper()
	seeded := seedEditingHead(t, f, chatstate.StateR1, extra)
	m := chatstate.NewChatMachine(f.DB, f.Pub, seeded.chatID)
	require.NoError(t, m.Update(testutil.Context(t, testutil.WaitShort), func(tx *chatstate.Tx, _ database.Store) error {
		_, err := tx.FinishTurn(chatstate.FinishTurnInput{})
		return err
	}))
	return seeded
}

func applyEditQueuedMessage(t *testing.T, _ *testFixture, tx *chatstate.Tx, seeded seededChat, _ chatstate.ExecutionState, result *transitionCaseResult) error {
	t.Helper()
	var targetQueueID int64
	if len(seeded.queuedMessageIDs) > 0 {
		targetQueueID = seeded.queuedMessageIDs[0]
	}
	editing := true
	var err error
	result.editQueuedMessage, err = tx.EditQueuedMessage(chatstate.EditQueuedMessageInput{
		QueuedMessageID: targetQueueID,
		Editing:         &editing,
	})
	return err
}

func applyEditingEdit(idx int, editing bool) applierFn {
	return func(t *testing.T, _ *testFixture, tx *chatstate.Tx, seeded seededChat, _ chatstate.ExecutionState, result *transitionCaseResult) error {
		t.Helper()
		var err error
		result.editQueuedMessage, err = tx.EditQueuedMessage(chatstate.EditQueuedMessageInput{
			QueuedMessageID: seeded.queuedMessageIDs[idx],
			Editing:         &editing,
		})
		return err
	}
}

// editingCase is the shared shape of every editing case: seed, apply, then
// check the remaining queue ids and which row is editing.
func editingCase(tr chatstate.Transition, from, want chatstate.ExecutionState, sc scenario, seed func(*testing.T, *testFixture) seededChat, apply applierFn, wantQueue func(seeded seededChat) []int64, wantEditingIdx int) transitionCaseSpec {
	return transitionCaseSpec{
		transition: tr,
		from:       from,
		want:       want,
		scenario:   sc,
		seed:       func(t *testing.T, f *testFixture, _ chatstate.ExecutionState) seededChat { return seed(t, f) },
		apply:      apply,
		assert: func(ctx context.Context, t *testing.T, f *testFixture, seeded seededChat, _ snapshotBaseline, _ transitionCaseResult) {
			require.Equal(t, wantQueue(seeded), queuedIDsByPosition(ctx, t, f, seeded.chatID))
			for i, id := range seeded.queuedMessageIDs {
				row, err := f.DB.GetChatQueuedMessageByID(ctx, database.GetChatQueuedMessageByIDParams{ID: id, ChatID: seeded.chatID})
				if errors.Is(err, sql.ErrNoRows) {
					continue // promoted or deleted
				}
				require.NoError(t, err)
				require.Equal(t, i == wantEditingIdx, row.EditingSince.Valid, "editing on row %d", i)
			}
		},
	}
}

func allRows(s seededChat) []int64  { return s.queuedMessageIDs }
func tailRows(s seededChat) []int64 { return append([]int64{}, s.queuedMessageIDs[1:]...) }
func noRows(seededChat) []int64     { return []int64{} }

// editingQueueMatrixCases enumerates every matrix cell that exists because
// of queued-message edits.
func editingQueueMatrixCases() []transitionCaseSpec {
	seedEditing := func(from chatstate.ExecutionState, extra int) func(*testing.T, *testFixture) seededChat {
		return func(t *testing.T, f *testFixture) seededChat { return seedEditingHead(t, f, from, extra) }
	}
	seedP := func(extra int) func(*testing.T, *testFixture) seededChat {
		return func(t *testing.T, f *testFixture) seededChat { return seedPaused(t, f, extra) }
	}
	cases := []transitionCaseSpec{
		// Boundaries with a head under edit pause instead of promoting.
		editingCase(chatstate.TransitionFinishTurn, chatstate.StateR1, chatstate.StateP, scenarioEditingHead, seedEditing(chatstate.StateR1, 0), applyFinishTurn, allRows, 0),
		editingCase(chatstate.TransitionFinishInterruption, chatstate.StateI1, chatstate.StateP, scenarioEditingHead, seedEditing(chatstate.StateI1, 0), applyFinishInterruption, allRows, 0),
		sendBehindEditingCase(chatstate.StateE1, seedEditing(chatstate.StateE1, 0)),
		// P: a send queues behind the head under edit.
		sendBehindEditingCase(chatstate.StateP, seedP(0)),
		// P: content edit keeps the pause; releasing the head resumes.
		editingCase(chatstate.TransitionEditQueuedMessage, chatstate.StateP, chatstate.StateP, scenarioEditingHead, seedP(0), applyContentEdit, allRows, 0),
		editingCase(chatstate.TransitionEditQueuedMessage, chatstate.StateP, chatstate.StateR0, scenarioRelease, seedP(0), applyEditingEdit(0, false), noRows, -1),
		editingCase(chatstate.TransitionEditQueuedMessage, chatstate.StateP, chatstate.StateR1, scenarioRelease, seedP(1), applyEditingEdit(0, false), tailRows, -1),
		// P: deleting the head resumes with the next row, or idles.
		editingCase(chatstate.TransitionDeleteQueuedMessage, chatstate.StateP, chatstate.StateW, scenarioEditingHead, seedP(0), applyDeleteQueuedMessage, noRows, -1),
		editingCase(chatstate.TransitionDeleteQueuedMessage, chatstate.StateP, chatstate.StateR0, scenarioEditingHead, seedP(1), applyDeleteQueuedMessage, noRows, -1),
		editingCase(chatstate.TransitionDeleteQueuedMessage, chatstate.StateP, chatstate.StateR1, scenarioEditingHead, seedP(2), applyDeleteQueuedMessage,
			func(s seededChat) []int64 { return append([]int64{}, s.queuedMessageIDs[2:]...) }, -1),
		// P: send now on the head under edit, or on the row behind it.
		editingCase(chatstate.TransitionPromoteQueuedMessage, chatstate.StateP, chatstate.StateR0, scenarioEditingHead, seedP(0), applyPromoteQueuedMessage, noRows, -1),
		editingCase(chatstate.TransitionPromoteQueuedMessage, chatstate.StateP, chatstate.StateR1, scenarioEditingHead, seedP(1), applyPromoteQueuedMessage, tailRows, -1),
		// P: a history edit clears the queue.
		editingCase(chatstate.TransitionEditMessage, chatstate.StateP, chatstate.StateR0, scenarioEditingHead, seedP(1), applyEditMessage, noRows, -1),
	}
	// Edit on the states that have rows: re-begin the edit on the head;
	// the state does not change.
	for _, from := range []chatstate.ExecutionState{chatstate.StateE1, chatstate.StateR1, chatstate.StateI1, chatstate.StateA1} {
		cases = append(cases,
			editingCase(chatstate.TransitionEditQueuedMessage, from, from, scenarioEditingHead, seedEditing(from, 1), applyEditingEdit(0, true), allRows, 0),
			editingCase(chatstate.TransitionEditQueuedMessage, from, from, scenarioMoveEdit, seedEditing(from, 1), applyEditingEdit(1, true), allRows, 1),
		)
	}
	// P refuses to move the edit off its head.
	cases = append(cases, transitionCaseSpec{
		transition: chatstate.TransitionEditQueuedMessage,
		from:       chatstate.StateP,
		want:       chatstate.StateP,
		scenario:   scenarioRefused,
		seed:       func(t *testing.T, f *testFixture, _ chatstate.ExecutionState) seededChat { return seedPaused(t, f, 1) },
		apply:      applyEditingEdit(1, true),
		assertFailure: func(ctx context.Context, t *testing.T, f *testFixture, seeded seededChat, base snapshotBaseline, err error) {
			require.ErrorIs(t, err, chatstate.ErrPausedQueuedHeadUnderEdit)
			assertNoMutationOrPublish(ctx, t, f, seeded.chatID, base)
		},
	})
	return cases
}

// sendBehindEditingCase: a queued send lands behind the head under edit; the
// state does not change.
func sendBehindEditingCase(from chatstate.ExecutionState, seed func(*testing.T, *testFixture) seededChat) transitionCaseSpec {
	return transitionCaseSpec{
		transition: chatstate.TransitionSendMessage,
		from:       from,
		want:       from,
		scenario:   scenarioEditingHead,
		seed:       func(t *testing.T, f *testFixture, _ chatstate.ExecutionState) seededChat { return seed(t, f) },
		apply:      applySendMessageQueue,
		assert: func(ctx context.Context, t *testing.T, f *testFixture, seeded seededChat, base snapshotBaseline, result transitionCaseResult) {
			require.NotNil(t, result.sendMessage.QueuedMessage)
			require.Empty(t, result.sendMessage.InsertedMessages, "nothing is promoted past the head under edit")
			want := append(append([]int64{}, seeded.queuedMessageIDs...), result.sendMessage.QueuedMessage.ID)
			require.Equal(t, want, queuedIDsByPosition(ctx, t, f, seeded.chatID))
			require.Equal(t, base.historyIDs, activeHistoryIDs(ctx, t, f, seeded.chatID))
		},
	}
}

func applyContentEdit(t *testing.T, _ *testFixture, tx *chatstate.Tx, seeded seededChat, _ chatstate.ExecutionState, result *transitionCaseResult) error {
	t.Helper()
	var err error
	result.editQueuedMessage, err = tx.EditQueuedMessage(chatstate.EditQueuedMessageInput{
		QueuedMessageID: seeded.queuedMessageIDs[0],
		Content:         userMessageContent(t, "edited-queued-body"),
	})
	return err
}

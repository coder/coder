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

// Matrix cases for queue holds. A hold is consulted only at turn
// boundaries, so the only new state is P (waiting with a held head) and
// the only changed cells are the boundary transitions that can land
// there, EditQueuedMessage on the states that have rows, and P's own
// row. The harness asserts the post-state and the snapshot bump; the
// cases below add only the fact that distinguishes each cell.

const (
	scenarioHeldHead scenario = "held_head"
	scenarioMoveHold scenario = "move_hold"
	scenarioRelease  scenario = "release"
	scenarioRefused  scenario = "refused"
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

// seedHeldHead seeds a "1" state (E1, R1, I1, A1) whose head is held,
// with extra unheld rows behind it.
func seedHeldHead(t *testing.T, f *testFixture, from chatstate.ExecutionState, extra int) seededChat {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitShort)
	rows := 1 + extra

	var seeded seededChat
	switch from {
	case chatstate.StateA1:
		seeded = seedAOrA1(t, f, rows, "seed_tool_a1_held")
	case chatstate.StateE1, chatstate.StateR1, chatstate.StateI1:
		created := createTestChat(t, f)
		m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)
		var ids []int64
		var bodies []string
		for i := 0; i < rows; i++ {
			body := fmt.Sprintf("queued-held-%s-%d", from, i)
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
		t.Fatalf("seedHeldHead: %s has no rows", from)
	}
	holdQueuedMessage(ctx, t, f, seeded.chatID, seeded.queuedMessageIDs[0])
	return seeded
}

// seedPaused seeds P: a held head with extra unheld rows behind it, on
// a chat whose turn has finished.
func seedPaused(t *testing.T, f *testFixture, extra int) seededChat {
	t.Helper()
	seeded := seedHeldHead(t, f, chatstate.StateR1, extra)
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
	held := true
	var err error
	result.editQueuedMessage, err = tx.EditQueuedMessage(chatstate.EditQueuedMessageInput{
		QueuedMessageID: targetQueueID,
		Held:            &held,
	})
	return err
}

func applyHeldEdit(idx int, held bool) applierFn {
	return func(t *testing.T, _ *testFixture, tx *chatstate.Tx, seeded seededChat, _ chatstate.ExecutionState, result *transitionCaseResult) error {
		t.Helper()
		var err error
		result.editQueuedMessage, err = tx.EditQueuedMessage(chatstate.EditQueuedMessageInput{
			QueuedMessageID: seeded.queuedMessageIDs[idx],
			Held:            &held,
		})
		return err
	}
}

// heldCase is the shared shape of every hold case: seed, apply, then
// check the queue ids left and which row is held.
func heldCase(tr chatstate.Transition, from, want chatstate.ExecutionState, sc scenario, seed func(*testing.T, *testFixture) seededChat, apply applierFn, wantQueue func(seeded seededChat) []int64, wantHeldIdx int) transitionCaseSpec {
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
				require.Equal(t, i == wantHeldIdx, row.HeldAt.Valid, "hold on row %d", i)
			}
		},
	}
}

func allRows(s seededChat) []int64  { return s.queuedMessageIDs }
func tailRows(s seededChat) []int64 { return append([]int64{}, s.queuedMessageIDs[1:]...) }
func noRows(seededChat) []int64     { return []int64{} }

// heldQueueMatrixCases enumerates every matrix cell that exists because
// of holds.
func heldQueueMatrixCases() []transitionCaseSpec {
	seedHeld := func(from chatstate.ExecutionState, extra int) func(*testing.T, *testFixture) seededChat {
		return func(t *testing.T, f *testFixture) seededChat { return seedHeldHead(t, f, from, extra) }
	}
	seedP := func(extra int) func(*testing.T, *testFixture) seededChat {
		return func(t *testing.T, f *testFixture) seededChat { return seedPaused(t, f, extra) }
	}
	cases := []transitionCaseSpec{
		// Boundaries with a held head pause instead of promoting.
		heldCase(chatstate.TransitionFinishTurn, chatstate.StateR1, chatstate.StateP, scenarioHeldHead, seedHeld(chatstate.StateR1, 0), applyFinishTurn, allRows, 0),
		heldCase(chatstate.TransitionFinishInterruption, chatstate.StateI1, chatstate.StateP, scenarioHeldHead, seedHeld(chatstate.StateI1, 0), applyFinishInterruption, allRows, 0),
		sendBehindHeldCase(chatstate.StateE1, seedHeld(chatstate.StateE1, 0)),
		// P: a send queues behind the held head.
		sendBehindHeldCase(chatstate.StateP, seedP(0)),
		// P: content edit keeps the pause; releasing the head resumes.
		heldCase(chatstate.TransitionEditQueuedMessage, chatstate.StateP, chatstate.StateP, scenarioHeldHead, seedP(0), applyContentEdit, allRows, 0),
		heldCase(chatstate.TransitionEditQueuedMessage, chatstate.StateP, chatstate.StateR0, scenarioRelease, seedP(0), applyHeldEdit(0, false), noRows, -1),
		heldCase(chatstate.TransitionEditQueuedMessage, chatstate.StateP, chatstate.StateR1, scenarioRelease, seedP(1), applyHeldEdit(0, false), tailRows, -1),
		// P: deleting the head resumes with the next row, or idles.
		heldCase(chatstate.TransitionDeleteQueuedMessage, chatstate.StateP, chatstate.StateW, scenarioHeldHead, seedP(0), applyDeleteQueuedMessage, noRows, -1),
		heldCase(chatstate.TransitionDeleteQueuedMessage, chatstate.StateP, chatstate.StateR0, scenarioHeldHead, seedP(1), applyDeleteQueuedMessage, noRows, -1),
		heldCase(chatstate.TransitionDeleteQueuedMessage, chatstate.StateP, chatstate.StateR1, scenarioHeldHead, seedP(2), applyDeleteQueuedMessage,
			func(s seededChat) []int64 { return append([]int64{}, s.queuedMessageIDs[2:]...) }, -1),
		// P: send now on the held head, or on the row behind it.
		heldCase(chatstate.TransitionPromoteQueuedMessage, chatstate.StateP, chatstate.StateR0, scenarioHeldHead, seedP(0), applyPromoteQueuedMessage, noRows, -1),
		heldCase(chatstate.TransitionPromoteQueuedMessage, chatstate.StateP, chatstate.StateR1, scenarioHeldHead, seedP(1), applyPromoteQueuedMessage, tailRows, -1),
		// P: a history edit clears the queue.
		heldCase(chatstate.TransitionEditMessage, chatstate.StateP, chatstate.StateR0, scenarioHeldHead, seedP(1), applyEditMessage, noRows, -1),
	}
	// Edit on the states that have rows: hold the head (already held
	// by the seed, so this is the idempotent re-hold) and the state does
	// not change.
	for _, from := range []chatstate.ExecutionState{chatstate.StateE1, chatstate.StateR1, chatstate.StateI1, chatstate.StateA1} {
		cases = append(cases,
			heldCase(chatstate.TransitionEditQueuedMessage, from, from, scenarioHeldHead, seedHeld(from, 1), applyHeldEdit(0, true), allRows, 0),
			heldCase(chatstate.TransitionEditQueuedMessage, from, from, scenarioMoveHold, seedHeld(from, 1), applyHeldEdit(1, true), allRows, 1),
		)
	}
	// P refuses to move the hold off its head.
	cases = append(cases, transitionCaseSpec{
		transition: chatstate.TransitionEditQueuedMessage,
		from:       chatstate.StateP,
		want:       chatstate.StateP,
		scenario:   scenarioRefused,
		seed:       func(t *testing.T, f *testFixture, _ chatstate.ExecutionState) seededChat { return seedPaused(t, f, 1) },
		apply:      applyHeldEdit(1, true),
		assertFailure: func(ctx context.Context, t *testing.T, f *testFixture, seeded seededChat, base snapshotBaseline, err error) {
			require.ErrorIs(t, err, chatstate.ErrPausedHeadMustResume)
			assertNoMutationOrPublish(ctx, t, f, seeded.chatID, base)
		},
	})
	return cases
}

// sendBehindHeldCase: a queued send lands behind the held head and
// nothing is promoted, so the state does not change.
func sendBehindHeldCase(from chatstate.ExecutionState, seed func(*testing.T, *testFixture) seededChat) transitionCaseSpec {
	return transitionCaseSpec{
		transition: chatstate.TransitionSendMessage,
		from:       from,
		want:       from,
		scenario:   scenarioHeldHead,
		seed:       func(t *testing.T, f *testFixture, _ chatstate.ExecutionState) seededChat { return seed(t, f) },
		apply:      applySendMessageQueue,
		assert: func(ctx context.Context, t *testing.T, f *testFixture, seeded seededChat, base snapshotBaseline, result transitionCaseResult) {
			require.NotNil(t, result.sendMessage.QueuedMessage)
			require.Empty(t, result.sendMessage.InsertedMessages, "nothing is promoted past the held head")
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

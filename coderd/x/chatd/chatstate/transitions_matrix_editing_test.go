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

// Matrix cases for queued-message edits: the boundary transitions that
// reach P, EditQueuedMessage between each "1" state and its blocked-head
// sibling, and every transition admitted from P. The harness asserts the
// post-state and the snapshot bump; the cases below add only the fact
// that distinguishes each cell.

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

// seedBlockedHead seeds the blocked-head sibling of a "1" state (E1, R1,
// I1, A1): the head is under edit, with extra rows behind it.
func seedBlockedHead(t *testing.T, f *testFixture, from chatstate.ExecutionState, extra int) seededChat {
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
		t.Fatalf("seedBlockedHead: %s has no rows", from)
	}
	beginQueuedMessageEdit(ctx, t, f, seeded.chatID, seeded.queuedMessageIDs[0])
	seeded.editingQueuedID = seeded.queuedMessageIDs[0]
	return seeded
}

// seedPaused seeds P: a head under edit with extra rows behind it,
// after FinishTurn from R1P.
func seedPaused(t *testing.T, f *testFixture, extra int) seededChat {
	t.Helper()
	seeded := seedBlockedHead(t, f, chatstate.StateR1, extra)
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

func applySetEditing(idx int, editing bool) applierFn {
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

func applyPromoteQueuedMessageAt(idx int) applierFn {
	return func(t *testing.T, _ *testFixture, tx *chatstate.Tx, seeded seededChat, _ chatstate.ExecutionState, result *transitionCaseResult) error {
		t.Helper()
		var err error
		result.promoteQueuedMessage, err = tx.PromoteQueuedMessage(chatstate.PromoteQueuedMessageInput{
			QueuedMessageID: seeded.queuedMessageIDs[idx],
		})
		return err
	}
}

func applyDeleteQueuedMessageAt(idx int) applierFn {
	return func(t *testing.T, _ *testFixture, tx *chatstate.Tx, seeded seededChat, _ chatstate.ExecutionState, result *transitionCaseResult) error {
		t.Helper()
		var err error
		result.deleteQueuedMessage, err = tx.DeleteQueuedMessage(chatstate.DeleteQueuedMessageInput{
			QueuedMessageID: seeded.queuedMessageIDs[idx],
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
func headOnly(s seededChat) []int64 { return s.queuedMessageIDs[:1] }

// editingQueueMatrixCases enumerates every matrix cell that exists because
// of queued-message edits.
func editingQueueMatrixCases() []transitionCaseSpec {
	seedBlocked := func(from chatstate.ExecutionState, extra int) func(*testing.T, *testFixture) seededChat {
		return func(t *testing.T, f *testFixture) seededChat { return seedBlockedHead(t, f, from, extra) }
	}
	seedP := func(extra int) func(*testing.T, *testFixture) seededChat {
		return func(t *testing.T, f *testFixture) seededChat { return seedPaused(t, f, extra) }
	}
	seedMulti := func(from chatstate.ExecutionState) func(*testing.T, *testFixture) seededChat {
		return func(t *testing.T, f *testFixture) seededChat { return seedStateMultiQueued(t, f, from) }
	}
	seedMultiEditingBehind := func(from chatstate.ExecutionState) func(*testing.T, *testFixture) seededChat {
		return func(t *testing.T, f *testFixture) seededChat {
			return withEditingRow(seedStateMultiQueued, 1)(t, f, from)
		}
	}
	cases := []transitionCaseSpec{
		// Turn boundaries with a blocked head pause the chat.
		editingCase(chatstate.TransitionFinishTurn, chatstate.StateR1P, chatstate.StateP, "", seedBlocked(chatstate.StateR1, 0), applyFinishTurn, allRows, 0),
		editingCase(chatstate.TransitionFinishInterruption, chatstate.StateI1P, chatstate.StateP, "", seedBlocked(chatstate.StateI1, 0), applyFinishInterruption, allRows, 0),
		// A send queues behind the blocked head whatever its busy behavior.
		sendBehindEditingCase(chatstate.StateE1P, seedBlocked(chatstate.StateE1, 0), scenarioQueue, applySendMessageQueue),
		sendBehindEditingCase(chatstate.StateE1P, seedBlocked(chatstate.StateE1, 0), scenarioInterrupt, applySendMessageInterrupt),
		sendBehindEditingCase(chatstate.StateP, seedP(0), scenarioQueue, applySendMessageQueue),
		sendBehindEditingCase(chatstate.StateP, seedP(0), scenarioInterrupt, applySendMessageInterrupt),
		// P: content edit keeps the pause; ending the head's edit resumes.
		editingCase(chatstate.TransitionEditQueuedMessage, chatstate.StateP, chatstate.StateP, scenarioContentEdit, seedP(0), applyContentEdit, allRows, 0),
		editingCase(chatstate.TransitionEditQueuedMessage, chatstate.StateP, chatstate.StateR0, scenarioEndEdit, seedP(0), applySetEditing(0, false), noRows, -1),
		editingCase(chatstate.TransitionEditQueuedMessage, chatstate.StateP, chatstate.StateR1, scenarioEndEdit, seedP(1), applySetEditing(0, false), tailRows, -1),
		// P: deleting the head resumes with the next row, or sets W when the
		// queue empties; deleting a row behind the head keeps the pause.
		editingCase(chatstate.TransitionDeleteQueuedMessage, chatstate.StateP, chatstate.StateW, "", seedP(0), applyDeleteQueuedMessage, noRows, -1),
		editingCase(chatstate.TransitionDeleteQueuedMessage, chatstate.StateP, chatstate.StateR0, "", seedP(1), applyDeleteQueuedMessage, noRows, -1),
		editingCase(chatstate.TransitionDeleteQueuedMessage, chatstate.StateP, chatstate.StateR1, "", seedP(2), applyDeleteQueuedMessage,
			func(s seededChat) []int64 { return append([]int64{}, s.queuedMessageIDs[2:]...) }, -1),
		editingCase(chatstate.TransitionDeleteQueuedMessage, chatstate.StateP, chatstate.StateP, scenarioTargetBehindHead, seedP(1), applyDeleteQueuedMessageAt(1), headOnly, 0),
		// P: promoting the blocked head ends its edit and sends it; promoting
		// the row behind it sends that row and leaves the head blocked, so
		// the resumed turn runs as R1P.
		editingCase(chatstate.TransitionPromoteQueuedMessage, chatstate.StateP, chatstate.StateR0, "", seedP(0), applyPromoteQueuedMessage, noRows, -1),
		editingCase(chatstate.TransitionPromoteQueuedMessage, chatstate.StateP, chatstate.StateR1, "", seedP(1), applyPromoteQueuedMessage, tailRows, -1),
		editingCase(chatstate.TransitionPromoteQueuedMessage, chatstate.StateP, chatstate.StateR1P, scenarioTargetBehindHead, seedP(1), applyPromoteQueuedMessageAt(1), headOnly, 0),
		// P: a history edit clears the queue.
		editingCase(chatstate.TransitionEditMessage, chatstate.StateP, chatstate.StateR0, "", seedP(1), applyEditMessage, noRows, -1),
	}
	// Busy and error states with rows: the edit marker on the head is the
	// only difference between a "1" state and its blocked-head sibling.
	// Beginning an edit on the head blocks it; moving or ending that edit
	// makes it ready again; an edit behind the head changes nothing.
	for _, blocked := range []chatstate.ExecutionState{chatstate.StateE1P, chatstate.StateR1P, chatstate.StateI1P, chatstate.StateA1P} {
		ready := readyHeadOf[blocked]
		cases = append(cases,
			editingCase(chatstate.TransitionEditQueuedMessage, ready, blocked, scenarioBeginEdit, seedMulti(ready), applySetEditing(0, true), allRows, 0),
			editingCase(chatstate.TransitionEditQueuedMessage, ready, ready, scenarioTargetBehindHead, seedMulti(ready), applySetEditing(1, true), allRows, 1),
			editingCase(chatstate.TransitionEditQueuedMessage, ready, ready, scenarioEndEdit, seedMultiEditingBehind(ready), applySetEditing(1, false), allRows, -1),
			editingCase(chatstate.TransitionEditQueuedMessage, blocked, blocked, scenarioBeginEdit, seedBlocked(ready, 1), applySetEditing(0, true), allRows, 0),
			editingCase(chatstate.TransitionEditQueuedMessage, blocked, blocked, scenarioContentEdit, seedBlocked(ready, 1), applyContentEdit, allRows, 0),
			editingCase(chatstate.TransitionEditQueuedMessage, blocked, ready, scenarioMoveEdit, seedBlocked(ready, 1), applySetEditing(1, true), allRows, 1),
			editingCase(chatstate.TransitionEditQueuedMessage, blocked, ready, scenarioEndEdit, seedBlocked(ready, 1), applySetEditing(0, false), allRows, -1),
		)
	}
	// P refuses to move the edit off its head.
	cases = append(cases, transitionCaseSpec{
		transition: chatstate.TransitionEditQueuedMessage,
		from:       chatstate.StateP,
		want:       chatstate.StateP,
		scenario:   scenarioRefused,
		seed:       func(t *testing.T, f *testFixture, _ chatstate.ExecutionState) seededChat { return seedPaused(t, f, 1) },
		apply:      applySetEditing(1, true),
		assertFailure: func(ctx context.Context, t *testing.T, f *testFixture, seeded seededChat, base snapshotBaseline, err error) {
			require.ErrorIs(t, err, chatstate.ErrPausedQueuedHeadUnderEdit)
			assertNoMutationOrPublish(ctx, t, f, seeded.chatID, base)
		},
	})
	return cases
}

// sendBehindEditingCase: a send is appended behind the blocked head; the
// state, history, and stored error do not change.
func sendBehindEditingCase(from chatstate.ExecutionState, seed func(*testing.T, *testFixture) seededChat, sc scenario, apply applierFn) transitionCaseSpec {
	return transitionCaseSpec{
		transition: chatstate.TransitionSendMessage,
		from:       from,
		want:       from,
		scenario:   sc,
		seed:       func(t *testing.T, f *testFixture, _ chatstate.ExecutionState) seededChat { return seed(t, f) },
		apply:      apply,
		assert: func(ctx context.Context, t *testing.T, f *testFixture, seeded seededChat, base snapshotBaseline, result transitionCaseResult) {
			require.NotNil(t, result.sendMessage.QueuedMessage)
			require.Empty(t, result.sendMessage.InsertedMessages, "nothing is promoted past the blocked head")
			want := append(append([]int64{}, seeded.queuedMessageIDs...), result.sendMessage.QueuedMessage.ID)
			require.Equal(t, want, queuedIDsByPosition(ctx, t, f, seeded.chatID))
			require.Equal(t, base.historyIDs, activeHistoryIDs(ctx, t, f, seeded.chatID))
			assertQueueEditMarkers(ctx, t, f, seeded.chatID, seeded.editingQueuedID)
			after, err := f.DB.GetChatByID(ctx, seeded.chatID)
			require.NoError(t, err)
			require.Equal(t, base.chat.Status, after.Status, "a send behind a blocked head keeps the status")
			require.Equal(t, base.chat.LastError, after.LastError, "a send behind a blocked head keeps last_error")
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

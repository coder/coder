package chatstate_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/testutil"
)

// RequestCompaction lifecycle tests.
//
// The compaction_requested_at marker is one-shot by construction:
// executionStateUpdate clears it unless a transition explicitly
// carries it forward. These tests pin the intended carriers
// (ownership changes, queue appends) and the intended consumers
// (compaction commit, turn-terminal transitions).

func requestCompaction(t *testing.T, f *testFixture) (uuid.UUID, *chatstate.ChatMachine) {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitShort)
	seeded := seedState(t, f, chatstate.StateW)
	m := chatstate.NewChatMachine(f.DB, f.Pub, seeded.chatID)

	worker := uuid.New()
	runner := uuid.New()
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		_, err := tx.Acquire(chatstate.AcquireInput{WorkerID: worker, RunnerID: runner})
		return err
	}))
	stale, err := f.DB.IsChatHeartbeatStale(ctx, database.IsChatHeartbeatStaleParams{
		ChatID:       seeded.chatID,
		RunnerID:     runner,
		StaleSeconds: chatstate.HeartbeatStaleSeconds,
	})
	require.NoError(t, err)
	require.False(t, stale, "owned runner heartbeat must be fresh")
	ownershipBefore := f.Pub.ownershipPublishCount()

	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		_, err := tx.RequestCompaction(chatstate.RequestCompactionInput{})
		return err
	}))
	chat := f.readChat(ctx, t, seeded.chatID)
	require.True(t, chat.CompactionRequestedAt.Valid, "request must set the marker")
	require.Equal(t, database.ChatStatusRunning, chat.Status)
	require.False(t, chat.WorkerID.Valid, "request must clear worker_id")
	require.False(t, chat.RunnerID.Valid, "request must clear runner_id")
	require.Equal(t, ownershipBefore+1, f.Pub.ownershipPublishCount(),
		"cleared ownership must publish an ownership hint")
	return seeded.chatID, m
}

func TestRequestCompaction_ClearsOwnershipAndPublishesHint(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	requestCompaction(t, f)
}

// TestRequestCompaction_PreservedByAcquireAndQueueAppend verifies the
// marker survives worker acquisition and queued message appends: both
// happen between the request and the compaction commit in normal
// operation.
func TestRequestCompaction_PreservedByAcquireAndQueueAppend(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	chatID, m := requestCompaction(t, f)

	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		_, err := tx.Acquire(chatstate.AcquireInput{WorkerID: uuid.New(), RunnerID: uuid.New()})
		return err
	}))
	chat := f.readChat(ctx, t, chatID)
	require.True(t, chat.CompactionRequestedAt.Valid, "Acquire preserves the marker")

	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		_, err := tx.SendMessage(chatstate.SendMessageInput{
			Message:      userTextMessage("queued while compacting", f.User.ID, f.Model.ID),
			BusyBehavior: chatstate.BusyBehaviorQueue,
		})
		return err
	}))
	chat = f.readChat(ctx, t, chatID)
	require.True(t, chat.CompactionRequestedAt.Valid, "queue append preserves the marker")
}

// TestRequestCompaction_ConsumedByCommitStep verifies the compaction
// commit path clears the marker exactly once, and that a plain
// CommitStep without ConsumeCompactionRequest leaves it alone.
func TestRequestCompaction_ConsumedByCommitStep(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	chatID, m := requestCompaction(t, f)

	assistant := userTextMessage("mid-step", f.User.ID, f.Model.ID)
	assistant.Role = database.ChatMessageRoleAssistant
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		_, err := tx.CommitStep(chatstate.CommitStepInput{
			Messages: []chatstate.Message{assistant},
		})
		return err
	}))
	chat := f.readChat(ctx, t, chatID)
	require.True(t, chat.CompactionRequestedAt.Valid,
		"CommitStep without ConsumeCompactionRequest preserves the marker")

	summary := userTextMessage("summary", f.User.ID, f.Model.ID)
	summary.Role = database.ChatMessageRoleAssistant
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		_, err := tx.CommitStep(chatstate.CommitStepInput{
			Messages:                 []chatstate.Message{summary},
			ConsumeCompactionRequest: true,
		})
		return err
	}))
	chat = f.readChat(ctx, t, chatID)
	require.False(t, chat.CompactionRequestedAt.Valid,
		"CommitStep with ConsumeCompactionRequest clears the marker")
}

// TestRequestCompaction_ClearedOnTerminalTransitions verifies that
// every turn-terminal transition reachable from a pending request
// clears the marker so it never replays on a later turn.
func TestRequestCompaction_ClearedOnTerminalTransitions(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		apply func(tx *chatstate.Tx) error
	}{
		{
			name: "FinishTurn",
			apply: func(tx *chatstate.Tx) error {
				_, err := tx.FinishTurn(chatstate.FinishTurnInput{})
				return err
			},
		},
		{
			name: "FinishError",
			apply: func(tx *chatstate.Tx) error {
				_, err := tx.FinishError(chatstate.FinishErrorInput{
					LastError: pqtype.NullRawMessage{
						RawMessage: json.RawMessage(`{"message":"boom"}`),
						Valid:      true,
					},
				})
				return err
			},
		},
		{
			name: "Interrupt",
			apply: func(tx *chatstate.Tx) error {
				_, err := tx.Interrupt(chatstate.InterruptInput{})
				return err
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newTestFixture(t)
			ctx := testutil.Context(t, testutil.WaitShort)
			chatID, m := requestCompaction(t, f)

			require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
				return tc.apply(tx)
			}))
			chat := f.readChat(ctx, t, chatID)
			require.False(t, chat.CompactionRequestedAt.Valid,
				"%s must clear the compaction request marker", tc.name)
		})
	}
}

// TestRequestCompaction_ClearedByNewTurn verifies transitions that
// start a fresh turn (direct sends, edits) drop a pending request:
// the new turn's history supersedes the compaction the user asked for.
func TestRequestCompaction_ClearedByNewTurn(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	chatID, m := requestCompaction(t, f)

	// Finish the pending turn (clears), then re-request and edit.
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		_, err := tx.FinishTurn(chatstate.FinishTurnInput{})
		return err
	}))
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		_, err := tx.RequestCompaction(chatstate.RequestCompactionInput{})
		return err
	}))

	target := firstUserMessageID(ctx, t, f, chatID)
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		_, err := tx.EditMessage(chatstate.EditMessageInput{
			MessageID: target,
			CreatedBy: f.User.ID,
			Content:   userTextMessage("edited", f.User.ID, f.Model.ID).Content,
		})
		return err
	}))
	chat := f.readChat(ctx, t, chatID)
	require.False(t, chat.CompactionRequestedAt.Valid,
		"EditMessage starts a new turn and must clear the marker")
}

// TestRequestCompaction_RejectedWhenBusyOrArchived pins the matrix
// boundaries callers rely on for 409 mapping: only W admits the
// transition.
func TestRequestCompaction_RejectedWhenBusyOrArchived(t *testing.T) {
	t.Parallel()

	for _, from := range []chatstate.ExecutionState{
		chatstate.StateR0, chatstate.StateE0, chatstate.StateXW,
	} {
		t.Run(string(from), func(t *testing.T) {
			t.Parallel()
			f := newTestFixture(t)
			ctx := testutil.Context(t, testutil.WaitShort)
			seeded := seedState(t, f, from)
			m := chatstate.NewChatMachine(f.DB, f.Pub, seeded.chatID)

			err := m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
				_, rerr := tx.RequestCompaction(chatstate.RequestCompactionInput{})
				return rerr
			})
			require.ErrorIs(t, err, chatstate.ErrTransitionNotAllowed)
			chat := f.readChat(ctx, t, seeded.chatID)
			require.False(t, chat.CompactionRequestedAt.Valid)
		})
	}
}

package chatstate_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/testutil"
)

// Idle-state resumes must clear ownership so worker acquisition
// re-admits the chat under the capacity cap.
func TestResumeFromIdleClearsOwnership(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		state chatstate.ExecutionState
		apply func(tx *chatstate.Tx, f *testFixture, seeded seededChat) error
	}{
		{
			name:  "SendMessageFromWaiting",
			state: chatstate.StateW,
			apply: func(tx *chatstate.Tx, f *testFixture, _ seededChat) error {
				_, err := tx.SendMessage(chatstate.SendMessageInput{
					Message:      userTextMessage("resume", f.User.ID, f.Model.ID),
					BusyBehavior: chatstate.BusyBehaviorQueue,
				})
				return err
			},
		},
		{
			name:  "SendMessageFromErrorWithQueue",
			state: chatstate.StateE1,
			apply: func(tx *chatstate.Tx, f *testFixture, _ seededChat) error {
				_, err := tx.SendMessage(chatstate.SendMessageInput{
					Message:      userTextMessage("resume", f.User.ID, f.Model.ID),
					BusyBehavior: chatstate.BusyBehaviorQueue,
				})
				return err
			},
		},
		{
			name:  "EditMessageFromWaiting",
			state: chatstate.StateW,
			apply: func(tx *chatstate.Tx, f *testFixture, seeded seededChat) error {
				_, err := tx.EditMessage(chatstate.EditMessageInput{
					MessageID: seeded.initialUserMessageID,
					CreatedBy: f.User.ID,
					Content:   userTextMessage("edited", f.User.ID, f.Model.ID).Content,
				})
				return err
			},
		},
		{
			name:  "PromoteQueuedMessageFromErrorWithQueue",
			state: chatstate.StateE1,
			apply: func(tx *chatstate.Tx, f *testFixture, seeded seededChat) error {
				_, err := tx.PromoteQueuedMessage(chatstate.PromoteQueuedMessageInput{
					QueuedMessageID: seeded.queuedMessageIDs[0],
				})
				return err
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newTestFixture(t)
			ctx := testutil.Context(t, testutil.WaitShort)
			seeded := seedState(t, f, tc.state)
			m := chatstate.NewChatMachine(f.DB, f.Pub, seeded.chatID)

			require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
				_, err := tx.Acquire(chatstate.AcquireInput{WorkerID: uuid.New(), RunnerID: uuid.New()})
				return err
			}))
			ownershipBefore := f.Pub.ownershipPublishCount()

			require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
				return tc.apply(tx, f, seeded)
			}))

			chat := f.readChat(ctx, t, seeded.chatID)
			require.Equal(t, database.ChatStatusRunning, chat.Status)
			require.False(t, chat.WorkerID.Valid, "idle resume must clear worker_id")
			require.False(t, chat.RunnerID.Valid, "idle resume must clear runner_id")
			require.Equal(t, ownershipBefore+1, f.Pub.ownershipPublishCount(),
				"cleared ownership must publish an ownership hint")
		})
	}
}

func TestEditMessageFromRunningKeepsOwnership(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	seeded := seedState(t, f, chatstate.StateR0)
	m := chatstate.NewChatMachine(f.DB, f.Pub, seeded.chatID)

	worker := uuid.New()
	runner := uuid.New()
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		_, err := tx.Acquire(chatstate.AcquireInput{WorkerID: worker, RunnerID: runner})
		return err
	}))

	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		_, err := tx.EditMessage(chatstate.EditMessageInput{
			MessageID: seeded.initialUserMessageID,
			CreatedBy: f.User.ID,
			Content:   userTextMessage("edited", f.User.ID, f.Model.ID).Content,
		})
		return err
	}))

	chat := f.readChat(ctx, t, seeded.chatID)
	require.Equal(t, database.ChatStatusRunning, chat.Status)
	require.Equal(t, uuid.NullUUID{UUID: worker, Valid: true}, chat.WorkerID,
		"running resume keeps worker_id")
	require.Equal(t, uuid.NullUUID{UUID: runner, Valid: true}, chat.RunnerID,
		"running resume keeps runner_id")
}

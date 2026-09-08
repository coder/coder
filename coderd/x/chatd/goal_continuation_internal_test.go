package chatd

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestGoalHistoryEnabledRequiresGoalRow(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	f := newWorkerTestFixture(t)
	chat, _ := setupGoalTurn(ctx, t, f)
	starter := newGoalTaskStarter(t, f)

	enabled, err := starter.goalHistoryEnabled(ctx, f.db, chat)
	require.NoError(t, err)
	require.False(t, enabled, "a chat that never had a goal must skip the hidden-history read")

	insertActiveGoal(ctx, t, f, chat.ID)
	enabled, err = starter.goalHistoryEnabled(ctx, f.db, chat)
	require.NoError(t, err)
	require.True(t, enabled)
}

func setupGoalTurn(ctx context.Context, t *testing.T, f *workerTestFixture) (database.Chat, chatWorkerTaskStartInput) {
	t.Helper()
	chat := f.createRunningChat(t)
	workerID := uuid.New()
	runnerID := uuid.New()
	chat = acquireChat(t, f, chat.ID, workerID, runnerID)
	machine := chatstate.NewChatMachine(f.db, f.pubsub, chat.ID)
	require.NoError(t, machine.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		_, err := tx.CommitStep(chatstate.CommitStepInput{
			Messages: []chatstate.Message{assistantTextMessage(t, "done", f.model.ID)},
		})
		return err
	}))
	chat, err := f.db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	return chat, chatWorkerTaskStartInput{
		TaskID:            uuid.New(),
		ChatID:            chat.ID,
		WorkerID:          workerID,
		RunnerID:          runnerID,
		HistoryVersion:    chat.HistoryVersion,
		GenerationAttempt: chat.GenerationAttempt,
		Status:            chat.Status,
		StopNudges:        &stopNudgeTracker{},
	}
}

func insertActiveGoal(ctx context.Context, t *testing.T, f *workerTestFixture, rootChatID uuid.UUID) database.ChatGoal {
	t.Helper()
	goal, err := f.db.InsertActiveChatGoal(dbauthz.AsSystemRestricted(ctx), database.InsertActiveChatGoalParams{
		RootChatID:      rootChatID,
		Objective:       "finish the work",
		CreatedByUserID: f.user.ID,
	})
	require.NoError(t, err)
	return goal
}

func newGoalTaskStarter(t *testing.T, f *workerTestFixture) *taskStarter {
	t.Helper()
	logger := testutil.Logger(t)
	clock := quartz.NewReal()
	// The finish paths spawn detached finalize goroutines through
	// Server.goInflight, so the server needs a lifecycle context and
	// config cache even in tests.
	server := &Server{
		ctx:         context.Background(),
		db:          f.db,
		pubsub:      f.pubsub,
		logger:      logger,
		clock:       clock,
		configCache: newChatConfigCache(context.Background(), f.db, clock),
		experiments: codersdk.Experiments{codersdk.ExperimentChatGoals},
	}
	return &taskStarter{
		server: server,
		opts: chatWorkerOptions{
			Store:  f.db,
			Pubsub: f.pubsub,
			Logger: logger,
			Clock:  clock,
		},
		routeStateHint: func(context.Context, runnerStateUpdate) {},
	}
}

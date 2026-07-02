package chatd //nolint:testpackage // Exercises unexported generation reminder helpers.

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestInsertGoalCompletionReminder(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	f := newWorkerTestFixture(t)
	chat, input := setupGoalReminderTurn(ctx, t, f)
	goal := insertActiveGoal(ctx, t, f, chat.ID)
	starter := newGoalReminderTaskStarter(t, f)
	machine := chatstate.NewChatMachine(f.db, f.pubsub, chat.ID)

	inserted, err := starter.insertGoalCompletionReminder(ctx, machine, input, generationPrepared{
		ModelConfigID: f.model.ID,
		GoalReminder:  &generationGoalReminder{GoalID: goal.ID},
	})
	require.NoError(t, err)
	require.True(t, inserted)

	messages, err := f.db.GetChatMessagesForPromptByChatID(ctx, chat.ID)
	require.NoError(t, err)
	require.Len(t, messages, 3)
	reminder := messages[2]
	require.Equal(t, database.ChatMessageRoleUser, reminder.Role)
	require.Equal(t, database.ChatMessageVisibilityModel, reminder.Visibility)
	parts, err := chatprompt.ParseContent(reminder)
	require.NoError(t, err)
	text := textFromParts(parts)
	require.Contains(t, text, goal.ID.String())
	require.Contains(t, text, "call complete_goal now")
}

func TestInsertGoalCompletionReminderSkipsQueuedUserMessage(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	f := newWorkerTestFixture(t)
	chat, input := setupGoalReminderTurn(ctx, t, f)
	goal := insertActiveGoal(ctx, t, f, chat.ID)
	machine := chatstate.NewChatMachine(f.db, f.pubsub, chat.ID)
	require.NoError(t, machine.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		_, err := tx.SendMessage(chatstate.SendMessageInput{
			Message:      userTextMessage(t, "queued", f.user.ID, f.model.ID, f.apiKey.ID),
			BusyBehavior: chatstate.BusyBehaviorQueue,
		})
		return err
	}))

	starter := newGoalReminderTaskStarter(t, f)
	inserted, err := starter.insertGoalCompletionReminder(ctx, machine, input, generationPrepared{
		ModelConfigID: f.model.ID,
		GoalReminder:  &generationGoalReminder{GoalID: goal.ID},
	})
	require.NoError(t, err)
	require.False(t, inserted)

	messages, err := f.db.GetChatMessagesForPromptByChatID(ctx, chat.ID)
	require.NoError(t, err)
	require.Len(t, messages, 2)
}

func TestInsertGoalCompletionReminderCountsCompactedReminder(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	f := newWorkerTestFixture(t)
	chat, input := setupGoalReminderTurn(ctx, t, f)
	goal := insertActiveGoal(ctx, t, f, chat.ID)
	starter := newGoalReminderTaskStarter(t, f)
	machine := chatstate.NewChatMachine(f.db, f.pubsub, chat.ID)

	inserted, err := starter.insertGoalCompletionReminder(ctx, machine, input, generationPrepared{
		ModelConfigID: f.model.ID,
		GoalReminder:  &generationGoalReminder{GoalID: goal.ID},
	})
	require.NoError(t, err)
	require.True(t, inserted)

	// Commit a compaction step. Its compressed summary hides the
	// reminder from the prompt window, so reminder accounting must not
	// rely on GetChatMessagesForPromptByChatID alone.
	compaction, err := buildCompactionMessages(buildCompactionMessagesInput{
		modelConfigID: f.model.ID,
		toolCallID:    uuid.NewString(),
		toolName:      "compact",
		compaction: compactionOutcome{
			SystemSummary: "summary",
			SummaryReport: "summary report",
		},
	})
	require.NoError(t, err)
	require.NoError(t, machine.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		_, err := tx.CommitStep(chatstate.CommitStepInput{
			Messages: compaction.Messages,
		})
		return err
	}))
	chat, err = f.db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	input.HistoryVersion = chat.HistoryVersion
	input.GenerationAttempt = chat.GenerationAttempt

	inserted, err = starter.insertGoalCompletionReminder(ctx, machine, input, generationPrepared{
		ModelConfigID: f.model.ID,
		GoalReminder:  &generationGoalReminder{GoalID: goal.ID},
	})
	require.NoError(t, err)
	require.False(t, inserted)

	hidden, err := f.db.GetChatHiddenUserMessagesByChatID(ctx, chat.ID)
	require.NoError(t, err)
	reminders := 0
	for _, msg := range hidden {
		if isGoalCompletionReminderMessageBestEffort(msg) {
			reminders++
		}
	}
	require.Equal(t, 1, reminders)
}

func setupGoalReminderTurn(ctx context.Context, t *testing.T, f *workerTestFixture) (database.Chat, chatWorkerTaskStartInput) {
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

func newGoalReminderTaskStarter(t *testing.T, f *workerTestFixture) *taskStarter {
	t.Helper()
	logger := testutil.Logger(t)
	server := &Server{
		db:          f.db,
		pubsub:      f.pubsub,
		logger:      logger,
		experiments: codersdk.Experiments{codersdk.ExperimentChatGoals},
	}
	return &taskStarter{
		server: server,
		opts: chatWorkerOptions{
			Store:  f.db,
			Pubsub: f.pubsub,
			Logger: logger,
		},
		routeStateHint: func(context.Context, runnerStateUpdate) {},
	}
}

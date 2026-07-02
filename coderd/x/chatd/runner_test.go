package chatd //nolint:testpackage // Uses unexported chatworker helpers.

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestRunner_IgnoresDuplicateStateNotifications(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	chat := f.createRunningChat(t)
	starter := newBlockingTaskStarter(false)
	startWorker(t, testOptions(t, f, starter))
	starter.waitCall(t, TaskKindGeneration, chat.ID)
	latest, err := f.db.GetChatByID(testutil.Context(t, testutil.WaitShort), chat.ID)
	require.NoError(t, err)

	publishChatUpdate(t, f, latest)
	publishChatUpdate(t, f, latest)
	starter.assertNoCall(t)
}

func TestRunner_CancelsActiveTaskWhenHistoryChanges(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	chat := f.createRunningChat(t)
	starter := newBlockingTaskStarter(false)
	startWorker(t, testOptions(t, f, starter))
	first := starter.waitCall(t, TaskKindGeneration, chat.ID)

	updated := commitAssistantStep(t, f, chat.ID, "first step")
	require.Greater(t, updated.HistoryVersion, first.input.HistoryVersion)
	requireTaskCanceled(t, first)
	require.NotErrorIs(t, context.Cause(first.ctx), errTaskTimeout)
	second := starter.waitCall(t, TaskKindGeneration, chat.ID)
	require.Equal(t, updated.HistoryVersion, second.input.HistoryVersion)
}

func TestRunner_CancelsActiveTaskWhenStatusChanges(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	chat := f.createRunningChat(t)
	starter := newBlockingTaskStarter(false)
	startWorker(t, testOptions(t, f, starter))
	first := starter.waitCall(t, TaskKindGeneration, chat.ID)

	updated := interruptChat(t, f, chat.ID)
	require.Equal(t, database.ChatStatusInterrupting, updated.Status)
	requireTaskCanceled(t, first)
	second := starter.waitCall(t, TaskKindInterrupt, chat.ID)
	require.Equal(t, updated.HistoryVersion, second.input.HistoryVersion)
}

func TestRunner_CleansUpOnOwnershipTakeover(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	chat := f.createRunningChat(t)
	starter := newBlockingTaskStarter(false)
	startWorker(t, testOptions(t, f, starter))
	first := starter.waitCall(t, TaskKindGeneration, chat.ID)

	acquireChat(t, f, chat.ID, uuid.New(), uuid.New())
	requireTaskCanceled(t, first)
	starter.assertNoCall(t)
}

func TestRunner_SerializesReplacementTasksForSameHistoryAndStatus(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	chat := f.createRunningChat(t)
	starter := newBlockingTaskStarter(true)
	defer starter.releaseAll()
	startWorker(t, testOptions(t, f, starter))
	first := starter.waitCall(t, TaskKindGeneration, chat.ID)

	forceExecutionStateAndPublish(t, f, chat.ID, database.ChatStatusInterrupting, false)
	starter.waitCall(t, TaskKindInterrupt, chat.ID)
	forceExecutionStateAndPublish(t, f, chat.ID, database.ChatStatusRunning, false)
	starter.assertNoCall(t)

	starter.release(t, 0)
	replacement := starter.waitCall(t, TaskKindGeneration, chat.ID)
	require.Equal(t, first.input.HistoryVersion, replacement.input.HistoryVersion)
}

func TestRunner_AllowsReplacementForDifferentHistoryOrStatus(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	chat := f.createRunningChat(t)
	starter := newBlockingTaskStarter(true)
	defer starter.releaseAll()
	startWorker(t, testOptions(t, f, starter))
	first := starter.waitCall(t, TaskKindGeneration, chat.ID)

	updated := commitAssistantStep(t, f, chat.ID, "different history")
	second := starter.waitCall(t, TaskKindGeneration, chat.ID)
	require.Greater(t, second.input.HistoryVersion, first.input.HistoryVersion)
	require.Equal(t, updated.HistoryVersion, second.input.HistoryVersion)
}

func TestRunner_TaskTimeoutRetries(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	chat := f.createRunningChat(t)
	clock := quartz.NewMock(t).WithLogger(quartz.NoOpLogger)
	timeoutTrap := clock.Trap().AfterFunc("chatworker", "task-timeout-generation")
	starter := newBlockingTaskStarter(false)
	opts := testOptions(t, f, starter)
	opts.Clock = clock
	opts.TaskRetryInitialBackoff = time.Minute
	opts.TaskRetryMaxBackoff = time.Minute
	startWorker(t, opts)

	timeoutTrap.MustWait(testutil.Context(t, testutil.WaitLong)).MustRelease(testutil.Context(t, testutil.WaitLong))
	timeoutTrap.Close()
	first := starter.waitCall(t, TaskKindGeneration, chat.ID)
	retryTrap := clock.Trap().NewTimer("chatworker", "task-retry-generation")
	defer retryTrap.Close()

	ctx := testutil.Context(t, testutil.WaitLong)
	clock.Advance(defaultTaskTimeout).MustWait(ctx)
	retryTrap.MustWait(ctx).MustRelease(ctx)
	require.ErrorIs(t, context.Cause(first.ctx), errTaskTimeout)
	clock.Advance(time.Minute).MustWait(ctx)
	second := starter.waitCall(t, TaskKindGeneration, chat.ID)
	require.Equal(t, first.input.HistoryVersion, second.input.HistoryVersion)
}

func TestWorker_RoutesDatabaseSyncStateToActiveRunner(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	chat := f.createRunningChat(t)
	clock := quartz.NewMock(t).WithLogger(quartz.NoOpLogger)
	starter := newBlockingTaskStarter(false)
	opts := testOptions(t, f, starter)
	opts.Clock = clock
	opts.RunnerSyncInterval = time.Minute
	startWorker(t, opts)
	first := starter.waitCall(t, TaskKindGeneration, chat.ID)

	forceExecutionState(t, f, chat.ID, database.ChatStatusInterrupting, false)
	clock.Advance(time.Minute).MustWait(testutil.Context(t, testutil.WaitLong))
	requireTaskCanceled(t, first)
	starter.waitCall(t, TaskKindInterrupt, chat.ID)
}

func TestWorker_CleanupStopsRoutingAndCancelsTasks(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	chat := f.createRunningChat(t)
	starter := newBlockingTaskStarter(false)
	startWorker(t, testOptions(t, f, starter))
	first := starter.waitCall(t, TaskKindGeneration, chat.ID)

	latest := acquireChat(t, f, chat.ID, uuid.New(), uuid.New())
	requireTaskCanceled(t, first)
	publishChatUpdate(t, f, latest)
	starter.assertNoCall(t)
}

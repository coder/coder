package chatd //nolint:testpackage // Uses unexported chatworker helpers.

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/aibridge"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestRunner_IgnoresDuplicateStateNotifications(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	chat := f.createRunningChat(t)
	starter := newBlockingTaskStarter(false)
	startWorker(t, testOptions(t, f, starter))
	starter.waitCall(t, taskKindGeneration, chat.ID)
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
	first := starter.waitCall(t, taskKindGeneration, chat.ID)

	updated := commitAssistantStep(t, f, chat.ID, "first step")
	require.Greater(t, updated.HistoryVersion, first.input.HistoryVersion)
	requireTaskCanceled(t, first)
	require.NotErrorIs(t, context.Cause(first.ctx), errTaskTimeout)
	second := starter.waitCall(t, taskKindGeneration, chat.ID)
	require.Equal(t, updated.HistoryVersion, second.input.HistoryVersion)
	require.Same(t, first.input.SessionStart, second.input.SessionStart)
}

func TestRunner_CancelsActiveTaskWhenStatusChanges(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	chat := f.createRunningChat(t)
	starter := newBlockingTaskStarter(false)
	startWorker(t, testOptions(t, f, starter))
	first := starter.waitCall(t, taskKindGeneration, chat.ID)

	updated := interruptChat(t, f, chat.ID)
	require.Equal(t, database.ChatStatusInterrupting, updated.Status)
	requireTaskCanceled(t, first)
	second := starter.waitCall(t, taskKindInterrupt, chat.ID)
	require.Equal(t, updated.HistoryVersion, second.input.HistoryVersion)
}

func TestRunner_CleansUpOnOwnershipTakeover(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	chat := f.createRunningChat(t)
	starter := newBlockingTaskStarter(false)
	startWorker(t, testOptions(t, f, starter))
	first := starter.waitCall(t, taskKindGeneration, chat.ID)

	acquireChat(t, f, chat.ID, uuid.New(), uuid.New())
	requireTaskCanceled(t, first)
	starter.assertNoCall(t)
}

func TestRunner_SerializesReplacementTasksForSameHistoryAndStatus(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	chat := f.createRunningChat(t)
	starter := newBlockingTaskStarter(true)
	worker := startWorker(t, testOptions(t, f, starter))
	t.Cleanup(func() {
		// Stop recovery before releasing tasks that ignore cancellation.
		// startWorker's cleanup joins the worker after these gates open.
		worker.cancel()
		starter.releaseAll()
	})
	first := starter.waitCall(t, taskKindGeneration, chat.ID)

	forceExecutionStateAndPublish(t, f, chat.ID, database.ChatStatusInterrupting, false)
	starter.waitCall(t, taskKindInterrupt, chat.ID)
	forceExecutionStateAndPublish(t, f, chat.ID, database.ChatStatusRunning, false)
	starter.assertNoCall(t)

	starter.release(t, 0)
	replacement := starter.waitCall(t, taskKindGeneration, chat.ID)
	require.Equal(t, first.input.HistoryVersion, replacement.input.HistoryVersion)
}

func TestRunner_AllowsReplacementForDifferentHistoryOrStatus(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	chat := f.createRunningChat(t)
	starter := newBlockingTaskStarter(true)
	worker := startWorker(t, testOptions(t, f, starter))
	t.Cleanup(func() {
		// Stop recovery before releasing tasks that ignore cancellation.
		// startWorker's cleanup joins the worker after these gates open.
		worker.cancel()
		starter.releaseAll()
	})
	first := starter.waitCall(t, taskKindGeneration, chat.ID)

	updated := commitAssistantStep(t, f, chat.ID, "different history")
	second := starter.waitCall(t, taskKindGeneration, chat.ID)
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
	first := starter.waitCall(t, taskKindGeneration, chat.ID)
	retryTrap := clock.Trap().NewTimer("chatworker", "task-retry-generation")
	defer retryTrap.Close()

	ctx := testutil.Context(t, testutil.WaitLong)
	clock.Advance(defaultTaskTimeout).MustWait(ctx)
	retryTrap.MustWait(ctx).MustRelease(ctx)
	require.ErrorIs(t, context.Cause(first.ctx), errTaskTimeout)
	clock.Advance(time.Minute).MustWait(ctx)
	second := starter.waitCall(t, taskKindGeneration, chat.ID)
	require.Equal(t, first.input.HistoryVersion, second.input.HistoryVersion)
}

func TestWorker_RoutesDatabaseSyncStateToActiveRunner(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	chat := f.createRunningChat(t)
	clock := quartz.NewMock(t).WithLogger(quartz.NoOpLogger)
	syncTrap := clock.Trap().NewTicker("chatworker", "runner-sync")
	defer syncTrap.Close()
	starter := newBlockingTaskStarter(false)
	opts := testOptions(t, f, starter)
	opts.Clock = clock
	opts.RunnerSyncInterval = time.Minute
	startWorker(t, opts)
	first := starter.waitCall(t, taskKindGeneration, chat.ID)
	syncTrap.MustWait(testutil.Context(t, testutil.WaitLong)).MustRelease(testutil.Context(t, testutil.WaitLong))
	before, err := f.db.GetChatByID(testutil.Context(t, testutil.WaitLong), chat.ID)
	require.NoError(t, err)

	updated := forceExecutionState(t, f, chat.ID, database.ChatStatusInterrupting, false)
	require.Greater(t, updated.SnapshotVersion, before.SnapshotVersion)
	require.Equal(t, before.HistoryVersion, updated.HistoryVersion)
	starter.assertNoCall(t)
	require.NoError(t, first.ctx.Err(), "unpublished change must wait for periodic sync")
	clock.Advance(time.Minute).MustWait(testutil.Context(t, testutil.WaitLong))
	requireTaskCanceled(t, first)
	starter.waitCall(t, taskKindInterrupt, chat.ID)
}

func TestWorker_CleanupStopsRoutingAndCancelsTasks(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	chat := f.createRunningChat(t)
	starter := newBlockingTaskStarter(false)
	startWorker(t, testOptions(t, f, starter))
	first := starter.waitCall(t, taskKindGeneration, chat.ID)

	latest := acquireChat(t, f, chat.ID, uuid.New(), uuid.New())
	requireTaskCanceled(t, first)
	publishChatUpdate(t, f, latest)
	starter.assertNoCall(t)
}

func TestRunner_CompletionRecovery(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		status   database.ChatStatus
		archived bool
		history  int64
		kind     taskKind
	}{
		{name: "EqualSnapshotUnchangedWork", status: database.ChatStatusRunning, history: 5, kind: taskKindGeneration},
		{name: "EqualSnapshotAdvancedHistory", status: database.ChatStatusRunning, history: 6, kind: taskKindGeneration},
		{name: "Interrupt", status: database.ChatStatusInterrupting, history: 5, kind: taskKindInterrupt},
		{name: "RequiresAction", status: database.ChatStatusRequiresAction, history: 5, kind: taskKindRequiresActionTimeout},
		{name: "Archived", status: database.ChatStatusRunning, archived: true, history: 5, kind: taskKindAbandon},
		{name: "Waiting", status: database.ChatStatusWaiting, history: 5, kind: taskKindAbandon},
		{name: "Error", status: database.ChatStatusError, history: 5, kind: taskKindAbandon},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r, starter, store, _, chat := newRunnerRecoveryTest(t)
			runRecoveryTestRunner(t, r, store, chat)
			first := starter.waitCall(t, taskKindGeneration, chat.ID)
			starter.release(t, 0)
			// No clock advance or state notification can cause this read.
			read := testutil.RequireReceive(r.ctx, t, store.reads)
			chat.Status, chat.Archived, chat.HistoryVersion = tc.status, tc.archived, tc.history
			chat.GenerationAttempt = 7
			chat.RequiresActionDeadlineAt = sql.NullTime{Time: time.Now().Add(time.Hour), Valid: true}
			read.reply <- runnerRefreshResult{chat: chat}
			second := starter.waitCall(t, tc.kind, chat.ID)
			require.NotEqual(t, first.input.TaskID, second.input.TaskID)
			require.Equal(t, chat.HistoryVersion, second.input.HistoryVersion)
			require.Equal(t, chat.GenerationAttempt, second.input.GenerationAttempt)
			require.Equal(t, chat.RequiresActionDeadlineAt, second.input.RequiresActionDeadlineAt)
			require.ErrorIs(t, read.ctx.Err(), context.Canceled)
			starter.assertNoCall(t)
		})
	}
}

func TestRunner_CompletionRecoveryCleanupBeforeSelect(t *testing.T) {
	t.Parallel()
	r, starter, store, _, chat := newRunnerRecoveryTest(t)
	r.processState(stateUpdateFromChat(chat))
	first := starter.waitCall(t, taskKindGeneration, chat.ID)
	id := finishRecoveryTestTask(t, r, starter, 0)
	chat.SnapshotVersion++
	r.processState(stateUpdateFromChat(chat))
	require.False(t, r.activeTaskSet)
	require.Equal(t, id, r.recoveryTaskID)
	require.Empty(t, r.tasks)
	require.Empty(t, r.tasksByIndex)
	requireTaskCanceled(t, first)
	r.scheduleRecovery()
	read := testutil.RequireReceive(r.ctx, t, store.reads)
	read.reply <- runnerRefreshResult{chat: chat}
	r.applyRefresh(testutil.RequireReceive(r.ctx, t, r.refreshResults))
	second := starter.waitCall(t, taskKindGeneration, chat.ID)
	require.NotEqual(t, first.input.TaskID, second.input.TaskID)
	require.Equal(t, 100*time.Millisecond, r.recoveryDelay)
}

func TestRunner_CompletionRecoverySupersededTask(t *testing.T) {
	t.Parallel()
	r, starter, store, _, chat := newRunnerRecoveryTest(t)
	starter.ignoreCancel = true
	t.Cleanup(func() {
		r.rec.cancel()
		starter.releaseAll()
	})
	r.processState(stateUpdateFromChat(chat))
	first := starter.waitCall(t, taskKindGeneration, chat.ID)
	oldID := r.activeTaskID
	chat.SnapshotVersion++
	chat.HistoryVersion++
	r.processState(stateUpdateFromChat(chat))
	second := starter.waitCall(t, taskKindGeneration, chat.ID)
	r.recoveryDelay = 200 * time.Millisecond
	starter.release(t, 0)
	testutil.TryReceive(r.ctx, t, r.tasks[oldID].done)
	r.removeFinishedTasks()
	r.scheduleRecovery()
	require.Equal(t, taskInstanceID(second.input.TaskID), r.activeTaskID)
	require.True(t, r.activeTaskSet)
	require.Zero(t, r.recoveryTaskID)
	require.False(t, r.refreshInFlight)
	require.Empty(t, store.reads)
	require.Equal(t, 200*time.Millisecond, r.recoveryDelay)
	require.NoError(t, second.ctx.Err())
	requireTaskCanceled(t, first)
	starter.assertNoCall(t)
}

func TestRunner_CompletionRecoveryObsoleteRefresh(t *testing.T) {
	t.Parallel()
	for _, finishes := range []bool{false, true} {
		t.Run(fmt.Sprintf("ReplacementFinishes=%t", finishes), func(t *testing.T) {
			t.Parallel()
			r, starter, store, _, chat := newRunnerRecoveryTest(t)
			r.processState(stateUpdateFromChat(chat))
			starter.waitCall(t, taskKindGeneration, chat.ID)
			finishRecoveryTestTask(t, r, starter, 0)
			r.removeFinishedTasks()
			r.scheduleRecovery()
			readA := testutil.RequireReceive(r.ctx, t, store.reads)

			newChat := chat
			newChat.SnapshotVersion++
			newChat.HistoryVersion++
			r.processState(stateUpdateFromChat(newChat))
			second := starter.waitCall(t, taskKindGeneration, chat.ID)
			require.True(t, r.refreshInFlight)
			r.scheduleRecovery()
			require.Empty(t, store.reads, "the outstanding read must be consumed before another starts")
			require.Zero(t, r.recoveryTaskID)
			if finishes {
				finishRecoveryTestTask(t, r, starter, 1)
				// Leave completion unrecognized until A's result is applied.
			}
			readA.reply <- runnerRefreshResult{chat: chat}
			r.applyRefresh(testutil.RequireReceive(r.ctx, t, r.refreshResults))
			require.Equal(t, stateUpdateFromChat(newChat), r.latestState)
			if !finishes {
				require.True(t, r.activeTaskSet)
				require.Equal(t, taskInstanceID(second.input.TaskID), r.activeTaskID)
				require.NoError(t, second.ctx.Err())
				starter.assertNoCall(t)
				return
			}
			require.False(t, r.activeTaskSet)
			require.Equal(t, taskInstanceID(second.input.TaskID), r.recoveryTaskID)
			r.scheduleRecovery()
			readB := testutil.RequireReceive(r.ctx, t, store.reads)
			readB.reply <- runnerRefreshResult{chat: newChat}
			r.applyRefresh(testutil.RequireReceive(r.ctx, t, r.refreshResults))
			third := starter.waitCall(t, taskKindGeneration, chat.ID)
			require.Equal(t, newChat.HistoryVersion, third.input.HistoryVersion)
		})
	}
}

func TestRunner_CompletionRecoveryBackoff(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"DatabaseError", "ReadTimeout", "NilExit", "ExpectedExit", "SnapshotAndAttemptOnly"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			r, starter, store, clock, chat := newRunnerRecoveryTest(t)
			if mode == "ExpectedExit" {
				r.opts.TaskStarter = runnerExitTaskStarter{chatWorkerTaskStarter: starter}
			}
			trap := clock.Trap().NewTimer("chatworker", "runner-recovery")
			defer trap.Close()
			runRecoveryTestRunner(t, r, store, chat)
			starter.waitCall(t, taskKindGeneration, chat.ID)
			starter.release(t, 0)
			for i, delay := range []time.Duration{100 * time.Millisecond, 200 * time.Millisecond, 400 * time.Millisecond, 400 * time.Millisecond} {
				read := testutil.RequireReceive(r.ctx, t, store.reads)
				switch mode {
				case "ReadTimeout":
					clock.Advance(runnerRefreshTimeout).MustWait(r.ctx)
					testutil.TryReceive(r.ctx, t, read.ctx.Done())
					require.ErrorIs(t, context.Cause(read.ctx), context.DeadlineExceeded)
				case "DatabaseError":
					read.reply <- runnerRefreshResult{err: xerrors.New("database unavailable")}
				default:
					if mode == "SnapshotAndAttemptOnly" {
						chat.SnapshotVersion++
						chat.GenerationAttempt++
						r.rec.stateCh <- stateUpdateFromChat(chat)
					}
					read.reply <- runnerRefreshResult{chat: chat}
					call := starter.waitCall(t, taskKindGeneration, chat.ID)
					require.Equal(t, chat.GenerationAttempt, call.input.GenerationAttempt)
					starter.release(t, i+1)
				}
				wait := trap.MustWait(r.ctx)
				require.Equal(t, delay, wait.Duration)
				wait.MustRelease(r.ctx)
				starter.assertNoCall(t)
				require.Empty(t, store.reads)
				clock.Advance(delay).MustWait(r.ctx)
			}
			// Expiry must launch a read, not indefinitely rearm the delay.
			testutil.RequireReceive(r.ctx, t, store.reads)
		})
	}
}

func TestRunner_CompletionRecoveryWorkChangeCancelsBackoff(t *testing.T) {
	t.Parallel()
	for _, field := range []string{"History", "Status", "Archived"} {
		t.Run(field, func(t *testing.T) {
			t.Parallel()
			r, starter, store, clock, chat := newRunnerRecoveryTest(t)
			trap := clock.Trap().NewTimer("chatworker", "runner-recovery")
			defer trap.Close()
			runRecoveryTestRunner(t, r, store, chat)
			starter.waitCall(t, taskKindGeneration, chat.ID)
			starter.release(t, 0)
			read := testutil.RequireReceive(r.ctx, t, store.reads)
			read.reply <- runnerRefreshResult{err: xerrors.New("database unavailable")}
			trap.MustWait(r.ctx).MustRelease(r.ctx)
			chat.SnapshotVersion++
			kind := taskKindGeneration
			switch field {
			case "History":
				chat.HistoryVersion++
			case "Status":
				chat.Status, kind = database.ChatStatusInterrupting, taskKindInterrupt
			case "Archived":
				chat.Archived, kind = true, taskKindAbandon
			}
			r.rec.stateCh <- stateUpdateFromChat(chat)
			starter.waitCall(t, kind, chat.ID)
			// Only the task's own timeout should remain scheduled.
			delay, ok := clock.Peek()
			require.True(t, ok)
			require.Equal(t, defaultTaskTimeout, delay)
			starter.release(t, 1)
			// Required-work changes restore immediate recovery.
			read = testutil.RequireReceive(r.ctx, t, store.reads)
			read.reply <- runnerRefreshResult{chat: chat}
			starter.waitCall(t, kind, chat.ID)
		})
	}
}

func TestRunner_CompletionRecoveryRejectsStaleRows(t *testing.T) {
	t.Parallel()
	for _, field := range []string{"Snapshot", "History"} {
		t.Run(field, func(t *testing.T) {
			t.Parallel()
			r, starter, store, clock, chat := newRunnerRecoveryTest(t)
			r.processState(stateUpdateFromChat(chat))
			starter.waitCall(t, taskKindGeneration, chat.ID)
			id := finishRecoveryTestTask(t, r, starter, 0)
			r.removeFinishedTasks()
			r.scheduleRecovery()
			read := testutil.RequireReceive(r.ctx, t, store.reads)
			stale := chat
			if field == "Snapshot" {
				stale.SnapshotVersion--
			} else {
				stale.SnapshotVersion++
				stale.HistoryVersion--
			}
			read.reply <- runnerRefreshResult{chat: stale}
			r.applyRefresh(testutil.RequireReceive(r.ctx, t, r.refreshResults))
			require.Equal(t, stateUpdateFromChat(chat), r.latestState)
			require.False(t, r.activeTaskSet)
			require.Equal(t, id, r.recoveryTaskID)
			r.scheduleRecovery()
			delay, ok := clock.Peek()
			require.True(t, ok)
			require.Equal(t, 100*time.Millisecond, delay)
			starter.assertNoCall(t)
		})
	}
}

func TestRunner_CompletionRecoveryLateHints(t *testing.T) {
	t.Parallel()
	r, starter, store, _, chat := newRunnerRecoveryTest(t)
	r.processState(stateUpdateFromChat(chat))
	starter.waitCall(t, taskKindGeneration, chat.ID)
	id := finishRecoveryTestTask(t, r, starter, 0)
	stale := chat
	stale.WorkerID = uuid.NullUUID{}
	stale.HistoryVersion--
	r.processState(stateUpdateFromChat(stale))
	stale.SnapshotVersion--
	r.processState(stateUpdateFromChat(stale))
	require.False(t, r.stopping)
	require.Equal(t, stateUpdateFromChat(chat), r.latestState)
	r.removeFinishedTasks()
	require.Equal(t, id, r.recoveryTaskID)
	r.scheduleRecovery()
	read := testutil.RequireReceive(r.ctx, t, store.reads)
	read.reply <- runnerRefreshResult{chat: chat}
	r.applyRefresh(testutil.RequireReceive(r.ctx, t, r.refreshResults))
	starter.waitCall(t, taskKindGeneration, chat.ID)
}

func TestRunner_CompletionRecoveryCleanup(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"Deleted", "WorkerChanged", "RunnerChanged", "OwnershipHint"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			r, starter, store, _, chat := newRunnerRecoveryTest(t)
			r.processState(stateUpdateFromChat(chat))
			starter.waitCall(t, taskKindGeneration, chat.ID)
			finishRecoveryTestTask(t, r, starter, 0)
			r.removeFinishedTasks()
			r.scheduleRecovery()
			read := testutil.RequireReceive(r.ctx, t, store.reads)
			result := runnerRefreshResult{chat: chat}
			switch mode {
			case "Deleted":
				result.err = sql.ErrNoRows
			case "WorkerChanged":
				result.chat.WorkerID.UUID = uuid.New()
			case "RunnerChanged":
				result.chat.RunnerID.UUID = uuid.New()
			case "OwnershipHint":
				lost := chat
				lost.SnapshotVersion++
				lost.WorkerID = uuid.NullUUID{}
				r.processState(stateUpdateFromChat(lost))
			}
			read.reply <- result
			r.applyRefresh(testutil.RequireReceive(r.ctx, t, r.refreshResults))
			require.Equal(t, r.rec.key, testutil.RequireReceive(r.ctx, t, r.mgr.cleanupReqCh))
			require.True(t, r.stopping)
			require.NoError(t, r.ctx.Err(), "manager cancellation has not arrived")
			chat.SnapshotVersion += 2
			chat.HistoryVersion++
			r.processState(stateUpdateFromChat(chat))
			r.scheduleRecovery()
			require.False(t, r.activeTaskSet)
			require.False(t, r.refreshInFlight)
			require.Nil(t, r.recoveryTimer)
			starter.assertNoCall(t)
		})
	}
}

func TestRunner_CompletionRecoveryShutdown(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"Timer", "Read", "ResultDelivery"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			r, starter, store, clock, chat := newRunnerRecoveryTest(t)
			trap := clock.Trap().NewTimer("chatworker", "runner-recovery")
			defer trap.Close()
			runRecoveryTestRunner(t, r, store, chat)
			starter.waitCall(t, taskKindGeneration, chat.ID)
			starter.release(t, 0)
			read := testutil.RequireReceive(r.ctx, t, store.reads)
			if mode == "Timer" {
				read.reply <- runnerRefreshResult{err: xerrors.New("database unavailable")}
				trap.MustWait(r.ctx).MustRelease(r.ctx)
			}
			r.rec.cancel()
			if mode == "ResultDelivery" {
				read.reply <- runnerRefreshResult{chat: chat}
			}
			testutil.TryReceive(testutil.Context(t, testutil.WaitLong), t, r.rec.done)
			require.ErrorIs(t, read.ctx.Err(), context.Canceled)
			require.Nil(t, r.recoveryTimer)
			_, pending := clock.Peek()
			require.False(t, pending)
			starter.assertNoCall(t)
		})
	}
}

func TestRunner_CompletionRecoveryCanceledResult(t *testing.T) {
	t.Parallel()
	r, starter, store, clock, chat := newRunnerRecoveryTest(t)
	r.processState(stateUpdateFromChat(chat))
	starter.waitCall(t, taskKindGeneration, chat.ID)
	finishRecoveryTestTask(t, r, starter, 0)
	r.removeFinishedTasks()
	r.scheduleRecovery()
	read := testutil.RequireReceive(r.ctx, t, store.reads)
	read.reply <- runnerRefreshResult{chat: chat}
	result := testutil.RequireReceive(r.ctx, t, r.refreshResults)
	r.rec.cancel()
	r.applyRefresh(result)
	r.scheduleRecovery()
	require.False(t, r.activeTaskSet)
	require.False(t, r.refreshInFlight)
	require.Nil(t, r.recoveryTimer)
	_, pending := clock.Peek()
	require.False(t, pending)
	starter.assertNoCall(t)
}

func TestRunner_CompletionRecoveryShutdownWithFullResults(t *testing.T) {
	t.Parallel()
	r, starter, store, _, chat := newRunnerRecoveryTest(t)
	r.processState(stateUpdateFromChat(chat))
	starter.waitCall(t, taskKindGeneration, chat.ID)
	finishRecoveryTestTask(t, r, starter, 0)
	r.removeFinishedTasks()
	r.scheduleRecovery()
	read := testutil.RequireReceive(r.ctx, t, store.reads)
	// Block result delivery to exercise its cancellation alternative.
	r.refreshResults <- runnerRefreshResult{}
	read.reply <- runnerRefreshResult{chat: chat}
	testutil.TryReceive(r.ctx, t, read.ctx.Done())
	r.rec.cancel()
	joined := make(chan struct{})
	go func() {
		r.refreshWG.Wait()
		close(joined)
	}()
	testutil.TryReceive(testutil.Context(t, testutil.WaitLong), t, joined)
	require.Len(t, r.refreshResults, 1)
	starter.assertNoCall(t)
}

func TestRunner_CompletionRecoveryRealGeneration(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	f := newWorkerTestFixture(t)
	require.NotNil(t, f.sqlDB, "history-fence regression requires PostgreSQL triggers")
	clock := quartz.NewMock(t).WithLogger(quartz.NoOpLogger)
	firstEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	var calls atomic.Int32
	providerURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("completion recovery")
		}
		if calls.Add(1) == 1 {
			close(firstEntered)
			select {
			case <-releaseFirst:
			case <-req.Context().Done():
			case <-ctx.Done():
			}
			return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("stale response must not commit")...)
		}
		return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("recovered response")...)
	})
	provider := dbgen.ChatProvider(t, f.db, database.ChatProvider{
		Provider: "openai-compat", DisplayName: "recovery", BaseUrl: providerURL,
	})
	model := dbgen.ChatModelConfig(t, f.db, database.ChatModelConfig{
		AIProviderID: uuid.NullUUID{UUID: provider.ID, Valid: true}, OrganizationID: f.org.ID,
	})
	var transport atomic.Pointer[aibridge.TransportFactory]
	var factory aibridge.TransportFactory = chattest.NewMockAIBridgeTransport(t, providerURL)
	transport.Store(&factory)
	sink := testutil.NewFakeSink(t)
	server := New(f.pubsub, Config{
		Logger: sink.Logger(), Database: f.db, ReplicaID: uuid.New(), Clock: clock,
		Experiments: codersdk.ExperimentsKnown, AIBridgeTransportFactory: &transport,
	})
	t.Cleanup(func() { require.NoError(t, server.Close()) })
	created, err := server.CreateChat(ctx, CreateOptions{
		OrganizationID: f.org.ID, OwnerID: f.user.ID, ModelConfigID: model.ID,
		Title: "completion recovery", InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	// Construct the real runner and manager directly so stateCh can be an
	// unbuffered acceptance barrier. Neither task execution nor the runner
	// event loop is replaced, and the manager's periodic loops remain active.
	runnerCtx, cancel := context.WithCancel(ctx)
	opts := server.chatWorker.opts
	mgr := newRunnerManager(runnerCtx, server, opts)
	opts.TaskStarter, err = newTaskStarter(server, opts, mgr.RouteStateHint, mgr.requestCleanup)
	require.NoError(t, err)
	done := make(chan struct{})
	rec := &runnerRecord{
		key: runnerKey{ChatID: created.ID, RunnerID: uuid.New()}, workerID: opts.WorkerID,
		cancel: cancel, done: done, stateCh: make(chan runnerStateUpdate),
	}
	acquireChat(t, f, created.ID, rec.workerID, rec.key.RunnerID)
	mgr.runners[rec.key] = rec
	mgr.runnersByChat[created.ID] = map[uuid.UUID]*runnerRecord{rec.key.RunnerID: rec}
	store := &runnerGenerationRefreshStore{Store: f.db, refreshed: make(chan database.Chat, 16)}
	opts.Store = store
	r := newRunner(runnerCtx, mgr, rec, opts)
	syncTrap := clock.Trap().NewTicker("chatworker", "runner-sync")
	defer syncTrap.Close()
	mgr.start()
	mgr.wg.Go(func() {
		defer close(done)
		r.run()
	})
	t.Cleanup(func() {
		cancel()
		mgr.wait()
	})
	syncTrap.MustWait(ctx).MustRelease(ctx)
	testutil.TryReceive(ctx, t, firstEntered)
	before, err := f.db.GetChatByID(ctx, created.ID)
	require.NoError(t, err)
	require.Positive(t, before.GenerationAttempt)
	require.Greater(t, before.SnapshotVersion, before.HistoryVersion)

	// The second unbuffered send cannot complete until processState has
	// finished accepting the first. Both carry the actual attempt row; the
	// second is a duplicate, not an artificial version or work transition.
	state := stateUpdateFromChat(before)
	testutil.RequireSend(ctx, t, rec.stateCh, state)
	testutil.RequireSend(ctx, t, rec.stateCh, state)
	require.Empty(t, store.refreshed)

	result, err := f.sqlDB.ExecContext(ctx, `
		UPDATE chat_messages
		SET content = '[{"type":"text","text":"hello after out-of-band edit"}]'::jsonb
		WHERE chat_id = $1 AND role = 'user' AND NOT deleted
	`, created.ID)
	require.NoError(t, err)
	affected, err := result.RowsAffected()
	require.NoError(t, err)
	require.Equal(t, int64(1), affected)
	mutated, err := f.db.GetChatByID(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, before.SnapshotVersion, mutated.SnapshotVersion)
	require.Equal(t, mutated.SnapshotVersion, mutated.HistoryVersion)
	require.Greater(t, mutated.HistoryVersion, before.HistoryVersion)
	require.Zero(t, mutated.GenerationAttempt)

	// No further state hint, clock advance, or snapshot allocation occurs in
	// the test. A real history fence must reject the blocked response, then
	// active-task completion must cause this database read.
	close(releaseFirst)
	refreshed := testutil.RequireReceive(ctx, t, store.refreshed)
	require.Equal(t, mutated.SnapshotVersion, refreshed.SnapshotVersion)
	require.Equal(t, mutated.HistoryVersion, refreshed.HistoryVersion)
	require.Equal(t, database.ChatStatusRunning, refreshed.Status)
	require.Equal(t, mutated.RunnerID, refreshed.RunnerID)
	var fenceExit string
	for _, entry := range sink.Entries(func(e slog.SinkEntry) bool { return e.Message == "chatworker task exited" }) {
		fields := fmt.Sprint(entry.Fields)
		if strings.Contains(fields, "chat history version mismatch") {
			fenceExit = fields
			break
		}
	}
	require.Contains(t, fenceExit, "expected_non_retryable_exit")
	require.Contains(t, fenceExit, "chat history version mismatch")

	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		chat, err := f.db.GetChatByID(ctx, created.ID)
		return err == nil && chat.Status == database.ChatStatusWaiting && !chat.WorkerID.Valid && !chat.RunnerID.Valid
	}, testutil.IntervalFast, "successor must commit and abandon without a periodic sync")
	messages, err := f.db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: created.ID})
	require.NoError(t, err)
	var users, assistants int
	for _, message := range messages {
		switch message.Role {
		case database.ChatMessageRoleUser:
			users++
			require.Contains(t, string(message.Content.RawMessage), "hello after out-of-band edit")
		case database.ChatMessageRoleAssistant:
			assistants++
			require.Contains(t, string(message.Content.RawMessage), "recovered response")
		}
		require.NotContains(t, string(message.Content.RawMessage), "stale response must not commit")
	}
	require.Equal(t, 1, users, "no recovery message was sent")
	require.Equal(t, 1, assistants)
	require.Equal(t, int32(2), calls.Load())
	final, err := f.db.GetChatByID(ctx, created.ID)
	require.NoError(t, err)
	require.False(t, final.LastError.Valid)
}

func TestRunner_CompletionRecoveryBootstrapFailure(t *testing.T) {
	t.Parallel()
	for _, failure := range []string{"Subscribe", "Read"} {
		t.Run(failure, func(t *testing.T) {
			t.Parallel()
			r, starter, store, clock, _ := newRunnerRecoveryTest(t)
			failureErr := xerrors.New("bootstrap unavailable")
			if failure == "Subscribe" {
				r.opts.Pubsub = runnerSubscribeErrorPubsub{chatWorkerPubsub: r.opts.Pubsub, err: failureErr}
			} else {
				r.opts.Store = runnerBootstrapStore{getChat: func(context.Context, uuid.UUID) (database.Chat, error) {
					return database.Chat{}, failureErr
				}}
			}

			r.run()
			require.True(t, r.stopping)
			require.Equal(t, r.rec.key, testutil.RequireReceive(r.ctx, t, r.mgr.cleanupReqCh))
			require.NoError(t, r.ctx.Err(), "cleanup request must precede manager cancellation")
			require.False(t, r.hasAcceptedState)
			require.False(t, r.activeTaskSet)
			require.Empty(t, r.tasks)
			require.False(t, r.refreshInFlight)
			require.Nil(t, r.recoveryTimer)
			r.scheduleRecovery()
			require.Empty(t, store.reads)
			_, pending := clock.Peek()
			require.False(t, pending)
			starter.assertNoCall(t)
		})
	}
}

func TestRunner_CompletionRecoveryCanceledDuringBootstrap(t *testing.T) {
	t.Parallel()
	r, starter, store, clock, chat := newRunnerRecoveryTest(t)
	r.opts.Store = runnerBootstrapStore{getChat: func(context.Context, uuid.UUID) (database.Chat, error) {
		// A successful read can race shutdown. The loop must honor cancellation
		// even though bootstrap returns a valid owned row.
		r.rec.cancel()
		return chat, nil
	}}

	r.run()
	require.ErrorIs(t, r.ctx.Err(), context.Canceled)
	require.False(t, r.hasAcceptedState)
	require.False(t, r.activeTaskSet)
	require.Empty(t, r.tasks)
	require.False(t, r.refreshInFlight)
	require.Empty(t, store.reads)
	require.Empty(t, r.mgr.cleanupReqCh)
	_, pending := clock.Peek()
	require.False(t, pending)
	starter.assertNoCall(t)
}

func TestRunner_CompletionRecoverySpawnGuard(t *testing.T) {
	t.Parallel()
	for _, condition := range []string{"Stopping", "Canceled"} {
		t.Run(condition, func(t *testing.T) {
			t.Parallel()
			r, starter, store, clock, chat := newRunnerRecoveryTest(t)
			pending := taskInstanceID(uuid.New())
			r.recoveryTaskID = pending
			r.recoveryDelay = r.opts.TaskRetryInitialBackoff
			if condition == "Stopping" {
				r.requestCleanup()
				require.NoError(t, r.ctx.Err(), "manager cancellation has not arrived")
			} else {
				r.rec.cancel()
			}

			r.spawnForState(stateUpdateFromChat(chat))
			require.False(t, r.activeTaskSet)
			require.Empty(t, r.tasks)
			require.Empty(t, r.tasksByIndex)
			require.Equal(t, pending, r.recoveryTaskID)
			require.Equal(t, r.opts.TaskRetryInitialBackoff, r.recoveryDelay)
			r.scheduleRecovery()
			require.False(t, r.refreshInFlight)
			require.Empty(t, store.reads)
			require.Nil(t, r.recoveryTimer)
			_, scheduled := clock.Peek()
			require.False(t, scheduled)
			starter.assertNoCall(t)
		})
	}
}

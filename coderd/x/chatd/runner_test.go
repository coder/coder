package chatd //nolint:testpackage // Uses unexported chatworker helpers.

import (
	"context"
	"database/sql"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/aibridge"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	dbpubsub "github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
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
	cancelWorker := worker.cancel
	t.Cleanup(func() {
		// Stop retries before releasing tasks that ignore cancellation.
		// startWorker's cleanup joins the worker after these gates open.
		cancelWorker()
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
	cancelWorker := worker.cancel
	t.Cleanup(func() {
		// Stop retries before releasing tasks that ignore cancellation.
		// startWorker's cleanup joins the worker after these gates open.
		cancelWorker()
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

// Handoff must retain work without a notification or maintenance tick.
// Changed work gets a successor; unchanged work retains the task identity.
func TestRunner_TaskHandoff(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		status   database.ChatStatus
		archived bool
		kind     taskKind
	}{
		{name: "Running", status: database.ChatStatusRunning, kind: taskKindGeneration},
		{name: "Interrupting", status: database.ChatStatusInterrupting, kind: taskKindInterrupt},
		{name: "RequiresAction", status: database.ChatStatusRequiresAction, kind: taskKindRequiresActionTimeout},
		{name: "Waiting", status: database.ChatStatusWaiting, kind: taskKindAbandon},
		{name: "Error", status: database.ChatStatusError, kind: taskKindAbandon},
		{name: "Archived", status: database.ChatStatusRunning, archived: true, kind: taskKindAbandon},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			f := newWorkerTestFixture(t)
			var chat database.Chat
			if tc.status == database.ChatStatusRequiresAction {
				chat = f.createRequiresActionChat(t)
			} else {
				chat = f.createRunningChat(t)
				if tc.status == database.ChatStatusInterrupting {
					chat = forceExecutionState(t, f, chat.ID, tc.status, false)
				}
			}
			starter := newBlockingTaskStarter(false)
			store := newGatedChatStore(f.db, chat.ID)
			clock := quartz.NewMock(t).WithLogger(quartz.NoOpLogger)
			opts := testOptions(t, f, starter)
			opts.Store, opts.Clock = store, clock
			trap := clock.Trap().NewTimer("chatworker")
			defer trap.Close()
			startWorker(t, opts)
			first := starter.waitCall(t, "", uuid.Nil)
			current, err := f.db.GetChatByID(ctx, chat.ID)
			require.NoError(t, err)
			if tc.kind == taskKindAbandon {
				current = forceExecutionState(t, f, chat.ID, tc.status, tc.archived)
			} else {
				require.Equal(t, tc.kind, first.kind)
			}
			store.enabled.Store(true)
			starter.release(t, 0)
			read := testutil.RequireReceive(ctx, t, store.reads)
			require.NoError(t, read.err)
			require.Equal(t, current.SnapshotVersion, read.chat.SnapshotVersion)
			require.Equal(t, current.HistoryVersion, read.chat.HistoryVersion)
			testutil.RequireSend(ctx, t, read.release, nil)
			if tc.kind != taskKindAbandon {
				wait := trap.MustWait(ctx)
				require.Equal(t, defaultTaskRetryInitialBackoff, wait.Duration)
				wait.MustRelease(ctx)
				clock.Advance(wait.Duration).MustWait(ctx)
			}
			second := starter.waitCall(t, "", uuid.Nil)
			require.Equal(t, tc.kind, second.kind)
			require.Equal(t, chat.ID, second.input.ChatID)
			if tc.kind == taskKindAbandon {
				require.NotEqual(t, first.input.TaskID, second.input.TaskID)
			} else {
				require.Equal(t, first.input.TaskID, second.input.TaskID)
			}
			require.Equal(t, first.input.RunnerID, second.input.RunnerID)
			require.Equal(t, current.HistoryVersion, second.input.HistoryVersion)
			require.Equal(t, current.RequiresActionDeadlineAt, second.input.RequiresActionDeadlineAt)
			require.Same(t, first.input.SessionStart, second.input.SessionStart)
			// Abandonment also retains responsibility after a nil return.
			if tc.kind == taskKindAbandon {
				starter.release(t, 1)
				read = testutil.RequireReceive(ctx, t, store.reads)
				testutil.RequireSend(ctx, t, read.release, nil)
				wait := trap.MustWait(ctx)
				require.Equal(t, defaultTaskRetryInitialBackoff, wait.Duration)
				wait.MustRelease(ctx)
				clock.Advance(wait.Duration).MustWait(ctx)
				third := starter.waitCall(t, "", uuid.Nil)
				require.Equal(t, taskKindAbandon, third.kind)
				require.Equal(t, second.input.TaskID, third.input.TaskID)
			}
		})
	}
}

func TestRunner_HandoffBackoff(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"NilExit", "ExpectedExit", "DatabaseError", "SnapshotOnly", "NormalizedMaximum"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			f := newWorkerTestFixture(t)
			chat := f.createRunningChat(t)
			starter := newBlockingTaskStarter(false)
			if mode == "ExpectedExit" {
				starter.exitErr = errTaskExpectedExit
			}
			store := newGatedChatStore(f.db, chat.ID)
			clock := quartz.NewMock(t).WithLogger(quartz.NoOpLogger)
			trap := clock.Trap().NewTimer("chatworker", "task-retry-generation")
			defer trap.Close()
			opts := testOptions(t, f, starter)
			opts.Clock, opts.Store = clock, store
			opts.TaskRetryInitialBackoff = 100 * time.Millisecond
			opts.TaskRetryMaxBackoff = 400 * time.Millisecond
			if mode == "NormalizedMaximum" {
				opts.TaskRetryMaxBackoff = time.Millisecond
			}
			worker := startWorker(t, opts)
			first := starter.waitCall(t, "", uuid.Nil)
			store.enabled.Store(true)
			starter.release(t, 0)
			for index, delay := range []time.Duration{100 * time.Millisecond, 200 * time.Millisecond, 400 * time.Millisecond, 400 * time.Millisecond} {
				if mode == "NormalizedMaximum" {
					delay = 100 * time.Millisecond
				}
				read := testutil.RequireReceive(ctx, t, store.reads)
				require.NoError(t, read.ctx.Err(), "operation return must leave the handoff parent live")
				if mode == "DatabaseError" {
					testutil.RequireSend(ctx, t, read.release, xerrors.New("database unavailable"))
				} else {
					testutil.RequireSend(ctx, t, read.release, nil)
				}
				wait := trap.MustWait(ctx)
				require.Equal(t, delay, wait.Duration)
				require.Empty(t, starter.callCh, "no operation may run before its backoff")
				wait.MustRelease(ctx)
				clock.Advance(delay).MustWait(ctx)
				if mode == "DatabaseError" {
					continue
				}
				next := starter.waitCall(t, "", uuid.Nil)
				require.Equal(t, taskKindGeneration, next.kind)
				require.Equal(t, first.input.TaskID, next.input.TaskID)
				require.Equal(t, first.input.RunnerID, next.input.RunnerID)
				if mode == "SnapshotOnly" {
					machine := chatstate.NewChatMachine(f.db, f.pubsub, chat.ID)
					require.NoError(t, machine.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
						_, err := tx.RecordGenerationAttempt(chatstate.RecordGenerationAttemptInput{})
						return err
					}))
				}
				starter.release(t, index+1)
			}
			read := testutil.RequireReceive(ctx, t, store.reads)
			testutil.RequireSend(ctx, t, read.release, nil)
			wait := trap.MustWait(ctx)
			require.Empty(t, starter.callCh, "failed reads must not replay the operation")
			wait.MustRelease(ctx)
			clock.Advance(wait.Duration).MustWait(ctx)
			next := starter.waitCall(t, "", uuid.Nil)
			require.Equal(t, first.input.TaskID, next.input.TaskID)
			require.Equal(t, int32(1), store.maxActive.Load())
			require.NoError(t, worker.Close())
			require.Empty(t, starter.callCh)
		})
	}
}

func TestRunner_HandoffReadStaysLiveUntilShutdown(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	f := newWorkerTestFixture(t)
	chat := f.createRunningChat(t)
	starter := newBlockingTaskStarter(false)
	store := newGatedChatStore(f.db, chat.ID)
	clock := quartz.NewMock(t).WithLogger(quartz.NoOpLogger)
	opts := testOptions(t, f, starter)
	opts.Clock, opts.Store = clock, store
	worker := startWorker(t, opts)
	starter.waitCall(t, taskKindGeneration, chat.ID)
	store.enabled.Store(true)
	starter.release(t, 0)
	read := testutil.RequireReceive(ctx, t, store.reads)
	clock.Advance(2 * defaultTaskTimeout).MustWait(ctx)
	require.NoError(t, read.ctx.Err(), "handoff uses the task parent, without a read or operation timeout")
	require.Empty(t, starter.callCh)
	require.Empty(t, store.reads)
	require.NoError(t, worker.Close())
	testutil.TryReceive(ctx, t, read.ctx.Done())
	testutil.TryReceive(ctx, t, read.done)
	require.Empty(t, starter.callCh)
}

func TestRunner_ShutdownDuringHandoffBackoff(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	f := newWorkerTestFixture(t)
	chat := f.createRunningChat(t)
	starter := newBlockingTaskStarter(false)
	store := newGatedChatStore(f.db, chat.ID)
	clock := quartz.NewMock(t).WithLogger(quartz.NoOpLogger)
	trap := clock.Trap().NewTimer("chatworker", "task-retry-generation")
	defer trap.Close()
	opts := testOptions(t, f, starter)
	opts.Clock, opts.Store = clock, store
	worker := startWorker(t, opts)
	starter.waitCall(t, taskKindGeneration, chat.ID)
	store.enabled.Store(true)
	starter.release(t, 0)
	read := testutil.RequireReceive(ctx, t, store.reads)
	testutil.RequireSend(ctx, t, read.release, nil)
	wait := trap.MustWait(ctx)
	wait.MustRelease(ctx)
	require.NoError(t, worker.Close())
	clock.Advance(wait.Duration).MustWait(ctx)
	require.Empty(t, starter.callCh)
	require.Empty(t, store.reads)
}

func TestRunner_WorkChangeSupersedesHandoffBackoff(t *testing.T) {
	t.Parallel()
	for _, field := range []string{"History", "Status", "Archived"} {
		t.Run(field, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			f := newWorkerTestFixture(t)
			chat := f.createRunningChat(t)
			starter := newBlockingTaskStarter(false)
			store := newGatedChatStore(f.db, chat.ID)
			clock := quartz.NewMock(t).WithLogger(quartz.NoOpLogger)
			trap := clock.Trap().NewTimer("chatworker", "task-retry-generation")
			defer trap.Close()
			opts := testOptions(t, f, starter)
			opts.Clock, opts.Store = clock, store
			startWorker(t, opts)
			starter.waitCall(t, "", uuid.Nil)
			store.enabled.Store(true)
			starter.release(t, 0)
			read := testutil.RequireReceive(ctx, t, store.reads)
			testutil.RequireSend(ctx, t, read.release, xerrors.New("database unavailable"))
			trap.MustWait(ctx).MustRelease(ctx)
			kind := taskKindGeneration
			switch field {
			case "History":
				chat = commitAssistantStep(t, f, chat.ID, "new history")
			case "Status":
				chat = interruptChat(t, f, chat.ID)
				kind = taskKindInterrupt
			case "Archived":
				chat = forceExecutionStateAndPublish(t, f, chat.ID, database.ChatStatusRunning, true)
				kind = taskKindAbandon
			}
			next := starter.waitCall(t, "", uuid.Nil)
			require.Equal(t, kind, next.kind)
			require.Equal(t, chat.HistoryVersion, next.input.HistoryVersion)
			starter.release(t, 1)
			// The successor reads immediately, without the prior task backoff.
			read = testutil.RequireReceive(ctx, t, store.reads)
			require.NoError(t, read.ctx.Err())
			require.Equal(t, chat.HistoryVersion, read.chat.HistoryVersion)
			require.Empty(t, starter.callCh)
		})
	}
}

func TestRunner_ObsoleteReadDoesNotReplaceNewWork(t *testing.T) {
	t.Parallel()
	for _, outcome := range []string{"StaleSnapshot", "NotFound"} {
		t.Run(outcome, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			f := newWorkerTestFixture(t)
			chat := f.createRunningChat(t)
			starter := newBlockingTaskStarter(false)
			store := newGatedChatStore(f.db, chat.ID)
			store.ignoreCancel = true
			clock := quartz.NewMock(t).WithLogger(quartz.NoOpLogger)
			opts := testOptions(t, f, starter)
			opts.Clock, opts.Store = clock, store
			worker := startWorker(t, opts)
			cancelWorker := worker.cancel
			t.Cleanup(func() {
				cancelWorker()
				close(store.releaseAll)
			})
			starter.waitCall(t, "", uuid.Nil)
			store.enabled.Store(true)
			starter.release(t, 0)
			oldRead := testutil.RequireReceive(ctx, t, store.reads)
			updated := commitAssistantStep(t, f, chat.ID, "replacement history")
			second := starter.waitCall(t, "", uuid.Nil)
			require.Equal(t, updated.HistoryVersion, second.input.HistoryVersion)
			testutil.TryReceive(ctx, t, oldRead.ctx.Done())
			starter.release(t, 1)
			newRead := testutil.RequireReceive(ctx, t, store.reads)
			require.Equal(t, int32(2), store.maxActive.Load(), "a superseded read must not block the successor handoff")
			require.NoError(t, newRead.ctx.Err())
			if outcome == "NotFound" {
				// A late absence would request cleanup if delivered to a successor.
				testutil.RequireSend(ctx, t, oldRead.release, sql.ErrNoRows)
			} else {
				testutil.RequireSend(ctx, t, oldRead.release, nil)
			}
			testutil.TryReceive(ctx, t, oldRead.done)
			require.Equal(t, updated.HistoryVersion, newRead.chat.HistoryVersion)
			trap := clock.Trap().NewTimer("chatworker", "task-retry-generation")
			defer trap.Close()
			testutil.RequireSend(ctx, t, newRead.release, nil)
			wait := trap.MustWait(ctx)
			require.NoError(t, newRead.ctx.Err(), "the obsolete result must not cancel the successor")
			wait.MustRelease(ctx)
			clock.Advance(wait.Duration).MustWait(ctx)
			third := starter.waitCall(t, "", uuid.Nil)
			require.Equal(t, taskKindGeneration, third.kind)
			require.Equal(t, updated.HistoryVersion, third.input.HistoryVersion)
			require.Equal(t, second.input.TaskID, third.input.TaskID)
			require.NoError(t, worker.Close())
			require.Empty(t, starter.callCh)
		})
	}
}

func TestRunner_HandoffOwnershipAndShutdown(t *testing.T) {
	t.Parallel()
	for _, outcome := range []string{"OwnershipLost", "NotFound", "Shutdown", "ShutdownDelayedRead"} {
		t.Run(outcome, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			f := newWorkerTestFixture(t)
			chat := f.createRunningChat(t)
			starter := newBlockingTaskStarter(false)
			store := newGatedChatStore(f.db, chat.ID)
			clock := quartz.NewMock(t).WithLogger(quartz.NoOpLogger)
			opts := testOptions(t, f, starter)
			opts.Clock, opts.Store = clock, store
			store.ignoreCancel = outcome == "ShutdownDelayedRead"
			worker := startWorker(t, opts)
			cancelWorker := worker.cancel
			t.Cleanup(func() {
				cancelWorker()
				close(store.releaseAll)
			})
			starter.waitCall(t, "", uuid.Nil)
			store.enabled.Store(true)
			if outcome == "OwnershipLost" {
				// Commit ownership without delivering a hint to this worker.
				quiet := *f
				quiet.pubsub = dbpubsub.NewInMemory()
				acquireChat(t, &quiet, chat.ID, uuid.New(), uuid.New())
			}
			starter.release(t, 0)
			read := testutil.RequireReceive(ctx, t, store.reads)
			switch outcome {
			case "ShutdownDelayedRead":
				closed := make(chan error, 1)
				go func() { closed <- worker.Close() }()
				testutil.TryReceive(ctx, t, read.ctx.Done())
				testutil.RequireSend(ctx, t, read.release, nil)
				require.NoError(t, testutil.RequireReceive(ctx, t, closed))
			case "Shutdown":
				require.NoError(t, worker.Close())
				testutil.TryReceive(ctx, t, read.ctx.Done())
			case "NotFound":
				testutil.RequireSend(ctx, t, read.release, sql.ErrNoRows)
			case "OwnershipLost":
				testutil.RequireSend(ctx, t, read.release, nil)
			}
			if outcome != "Shutdown" && outcome != "ShutdownDelayedRead" {
				// idle is manager-locked and is reached only after runner cleanup.
				testutil.Eventually(ctx, t, func(context.Context) bool { return worker.manager.idle() }, testutil.IntervalFast)
				require.NoError(t, worker.Close())
			}
			require.Empty(t, starter.callCh)
			require.Zero(t, store.active.Load())
		})
	}
}

// A database read must not roll back an accepted watermark even when it
// belongs to the current task. Replay a previously read, coherent database row.
func TestRunner_RejectsStaleCurrentTaskSnapshot(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	f := newWorkerTestFixture(t)
	chat := f.createRunningChat(t)
	starter := newBlockingTaskStarter(false)
	store := newGatedChatStore(f.db, chat.ID)
	store.enabled.Store(true)
	clock := quartz.NewMock(t).WithLogger(quartz.NoOpLogger)
	trap := clock.Trap().NewTimer("chatworker", "task-retry-generation")
	defer trap.Close()
	opts := testOptions(t, f, starter)
	opts.Clock, opts.Store = clock, store
	worker := startWorker(t, opts)
	bootstrap := testutil.RequireReceive(ctx, t, store.reads)
	stale := bootstrap.chat
	quiet := *f
	quiet.pubsub = dbpubsub.NewInMemory()
	current := forceExecutionState(t, &quiet, chat.ID, database.ChatStatusRunning, false)
	require.Greater(t, current.SnapshotVersion, stale.SnapshotVersion)
	require.Equal(t, stale.HistoryVersion, current.HistoryVersion)
	// The newer real row seeds bootstrap's watermark synchronously.
	bootstrap.chat = current
	testutil.RequireSend(ctx, t, bootstrap.release, nil)
	first := starter.waitCall(t, "", uuid.Nil)
	starter.release(t, 0)
	read := testutil.RequireReceive(ctx, t, store.reads)
	read.chat = stale
	testutil.RequireSend(ctx, t, read.release, nil)
	wait := trap.MustWait(ctx)
	require.Equal(t, defaultTaskRetryInitialBackoff, wait.Duration)
	wait.MustRelease(ctx)
	clock.Advance(wait.Duration).MustWait(ctx)
	read = testutil.RequireReceive(ctx, t, store.reads)
	require.Empty(t, starter.callCh, "a rejected row must retry only the read")
	require.Equal(t, current.SnapshotVersion, read.chat.SnapshotVersion)
	testutil.RequireSend(ctx, t, read.release, nil)
	wait = trap.MustWait(ctx)
	require.Equal(t, 2*defaultTaskRetryInitialBackoff, wait.Duration)
	require.Empty(t, starter.callCh)
	wait.MustRelease(ctx)
	clock.Advance(wait.Duration).MustWait(ctx)
	second := starter.waitCall(t, "", uuid.Nil)
	require.Equal(t, first.input.TaskID, second.input.TaskID)
	require.Equal(t, first.input.RunnerID, second.input.RunnerID)
	require.NoError(t, worker.Close())
	require.Empty(t, starter.callCh)
}

func TestRunner_BootstrapFailureAndShutdown(t *testing.T) {
	t.Parallel()
	for _, outcome := range []string{"DatabaseError", "Shutdown", "ShutdownDelayedRead"} {
		t.Run(outcome, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			f := newWorkerTestFixture(t)
			chat := f.createRunningChat(t)
			starter := newBlockingTaskStarter(false)
			store := newGatedChatStore(f.db, chat.ID)
			store.enabled.Store(true)
			store.ignoreCancel = outcome == "ShutdownDelayedRead"
			opts := testOptions(t, f, starter)
			opts.Store = store
			opts.Clock = quartz.NewMock(t).WithLogger(quartz.NoOpLogger)
			worker := startWorker(t, opts)
			cancelWorker := worker.cancel
			t.Cleanup(func() {
				cancelWorker()
				close(store.releaseAll)
			})
			read := testutil.RequireReceive(ctx, t, store.reads)
			switch outcome {
			case "DatabaseError":
				testutil.RequireSend(ctx, t, read.release, xerrors.New("bootstrap database unavailable"))
				testutil.Eventually(ctx, t, func(context.Context) bool { return worker.manager.idle() }, testutil.IntervalFast)
				require.NoError(t, worker.Close())
			case "Shutdown":
				require.NoError(t, worker.Close())
				testutil.TryReceive(ctx, t, read.ctx.Done())
			case "ShutdownDelayedRead":
				closed := make(chan error, 1)
				go func() { closed <- worker.Close() }()
				testutil.TryReceive(ctx, t, read.ctx.Done())
				testutil.RequireSend(ctx, t, read.release, nil)
				require.NoError(t, testutil.RequireReceive(ctx, t, closed))
			}
			require.Empty(t, starter.callCh, "bootstrap failure must never invoke a task")
			require.Zero(t, store.active.Load())
		})
	}
}

func TestRunner_RealGenerationRecoversHistoryFence(t *testing.T) {
	t.Parallel()
	for _, phase := range []string{"BootstrapSnapshot", "InFlightResponse"} {
		t.Run(phase, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			f := newWorkerTestFixture(t)
			clock := quartz.NewMock(t).WithLogger(quartz.NoOpLogger)
			firstRequest := make(chan struct{})
			releaseResponse := make(chan struct{})
			recoveredRequest := make(chan string, 2)
			var requests atomic.Int32
			providerURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
				if !req.Stream {
					return chattest.OpenAINonStreamingResponse("history fence")
				}
				if requests.Add(1) == 1 && phase == "InFlightResponse" {
					close(firstRequest)
					select {
					case <-releaseResponse:
					case <-req.Context().Done():
					case <-ctx.Done():
					}
					return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("stale response must not commit")...)
				}
				select {
				case recoveredRequest <- string(req.RawBody):
				case <-req.Context().Done():
				}
				return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("recovered response")...)
			})
			provider := dbgen.ChatProvider(t, f.db, database.ChatProvider{
				Provider: "openai-compat", DisplayName: "history fence", BaseUrl: providerURL,
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
				Title: "history fence", InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
			})
			require.NoError(t, err)
			store := newGatedChatStore(f.db, created.ID)
			// Keep API and preparation reads on the real store; only the worker
			// dependency delays root reads. Transactional generation stays real.
			server.chatWorker.opts.Store = store
			if phase == "BootstrapSnapshot" {
				store.enabled.Store(true)
			}
			starter, err := newTaskStarter(server, server.chatWorker.opts,
				func(ctx context.Context, state runnerStateUpdate) {
					server.chatWorker.manager.RouteStateHint(ctx, state)
				},
				func(ctx context.Context, key runnerKey) {
					server.chatWorker.manager.requestCleanup(ctx, key)
				},
			)
			require.NoError(t, err)
			results := make(chan error, 16)
			server.chatWorker.opts.TaskStarter = &generationResultTaskStarter{
				chatWorkerTaskStarter: starter,
				results:               results,
			}
			server.Start()
			var before database.Chat
			var bootstrap *gatedChatRead
			if phase == "BootstrapSnapshot" {
				bootstrap = testutil.RequireReceive(ctx, t, store.reads)
				require.NoError(t, bootstrap.err)
				before = bootstrap.chat
			} else {
				testutil.TryReceive(ctx, t, firstRequest)
				before, err = f.db.GetChatByID(ctx, created.ID)
				require.NoError(t, err)
				require.Positive(t, before.GenerationAttempt)
				store.enabled.Store(true)
			}
			require.Greater(t, before.SnapshotVersion, before.HistoryVersion)
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
			if bootstrap != nil {
				// Bootstrap accepts its captured snapshot before entering the loop.
				testutil.RequireSend(ctx, t, bootstrap.release, nil)
			} else {
				close(releaseResponse)
			}
			// No clock advance, recovery message, or new state hint drives this.
			read := testutil.RequireReceive(ctx, t, store.reads)
			require.Equal(t, mutated.SnapshotVersion, read.chat.SnapshotVersion)
			require.Equal(t, mutated.HistoryVersion, read.chat.HistoryVersion)
			require.Equal(t, before.RunnerID, read.chat.RunnerID)
			store.enabled.Store(false)
			testutil.RequireSend(ctx, t, read.release, nil)
			testutil.Eventually(ctx, t, func(ctx context.Context) bool {
				chat, err := f.db.GetChatByID(ctx, created.ID)
				return err == nil && chat.Status == database.ChatStatusWaiting && !chat.WorkerID.Valid && !chat.RunnerID.Valid
			}, testutil.IntervalFast)
			fenceExit := testutil.RequireReceive(ctx, t, results)
			require.ErrorIs(t, fenceExit, errTaskExpectedExit)
			require.NotErrorIs(t, fenceExit, errTaskRetryable)
			require.ErrorContains(t, fenceExit, "chat history version mismatch")
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
			require.Equal(t, 1, users)
			require.Equal(t, 1, assistants)
			wantRequests := int32(1)
			if phase == "InFlightResponse" {
				wantRequests = 2
			}
			require.Equal(t, wantRequests, requests.Load())
			require.Contains(t, testutil.RequireReceive(ctx, t, recoveredRequest), "hello after out-of-band edit")
			final, err := f.db.GetChatByID(ctx, created.ID)
			require.NoError(t, err)
			require.False(t, final.LastError.Valid)
		})
	}
}

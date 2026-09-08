package chatd //nolint:testpackage // Uses unexported chatworker helpers.

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

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
			starter, err := newTaskStarter(server, server.chatWorker.opts)
			require.NoError(t, err)
			results := make(chan error, 16)
			generationStarted := make(chan chatWorkerTaskStartInput, 1)
			releaseGeneration := make(chan struct{})
			var invocations atomic.Int32
			server.chatWorker.opts.TaskStarter = &generationResultTaskStarter{
				chatWorkerTaskStarter: starter,
				results:               results,
				beforeGeneration: func(ctx context.Context, input chatWorkerTaskStartInput) {
					if phase != "BootstrapSnapshot" || invocations.Add(1) != 1 {
						return
					}
					generationStarted <- input
					select {
					case <-releaseGeneration:
					case <-ctx.Done():
					}
				},
			}
			server.Start()
			if phase == "BootstrapSnapshot" {
				input := testutil.RequireReceive(ctx, t, generationStarted)
				require.Equal(t, created.ID, input.ChatID)
			} else {
				testutil.TryReceive(ctx, t, firstRequest)
			}
			before, err := f.db.GetChatByID(ctx, created.ID)
			require.NoError(t, err)
			if phase == "InFlightResponse" {
				require.Positive(t, before.GenerationAttempt)
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
			if phase == "BootstrapSnapshot" {
				close(releaseGeneration)
			} else {
				close(releaseResponse)
			}
			// Recovery must not require a clock tick or state notification.
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

func TestRunner_CompletedOperationRereadsCurrentWork(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name        string
		status      database.ChatStatus
		kind        taskKind
		exitErr     error
		panicOnExit bool
	}{
		{name: "Generation", status: database.ChatStatusRunning, kind: taskKindGeneration},
		{name: "ExpectedExit", status: database.ChatStatusRunning, kind: taskKindGeneration, exitErr: errTaskExpectedExit},
		{name: "UnexpectedError", status: database.ChatStatusRunning, kind: taskKindGeneration, exitErr: xerrors.New("operation failed")},
		{name: "Panic", status: database.ChatStatusRunning, kind: taskKindGeneration, panicOnExit: true},
		{name: "Interrupt", status: database.ChatStatusInterrupting, kind: taskKindInterrupt},
		{name: "RequiresAction", status: database.ChatStatusRequiresAction, kind: taskKindRequiresActionTimeout},
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
				chat = forceExecutionState(t, f, chat.ID, tc.status, false)
			}
			clock := quartz.NewMock(t).WithLogger(quartz.NoOpLogger)
			starter := newBlockingTaskStarter(false)
			starter.exitErr, starter.panicOnExit = tc.exitErr, tc.panicOnExit
			opts := testOptions(t, f, starter)
			opts.Clock = clock
			startWorker(t, opts)
			first := starter.waitCall(t, tc.kind, chat.ID)

			// Commit without publishing so only completion can discover it.
			changedStatus := database.ChatStatusRunning
			changedKind := taskKindGeneration
			if tc.status == database.ChatStatusRunning {
				changedStatus, changedKind = database.ChatStatusInterrupting, taskKindInterrupt
			}
			updated := forceExecutionState(t, f, chat.ID, changedStatus, false)
			if tc.name == "UnexpectedError" || tc.name == "Panic" {
				trap := clock.Trap().NewTimer("chatworker", "task-retry-generation")
				defer trap.Close()
				starter.release(t, 0)
				wait := trap.MustWait(ctx)
				require.Empty(t, starter.callCh)
				wait.MustRelease(ctx)
				clock.Advance(wait.Duration).MustWait(ctx)
			} else {
				starter.release(t, 0)
			}
			next := starter.waitCall(t, changedKind, chat.ID)
			require.Equal(t, updated.HistoryVersion, next.input.HistoryVersion)
			require.Equal(t, first.input.RunnerID, next.input.RunnerID)
			require.Same(t, first.input.SessionStart, next.input.SessionStart)
			require.NoError(t, ctx.Err())
		})
	}
}

func TestRunner_NoProgressBackoff(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"NilExit", "ExpectedExit", "UnexpectedError", "Panic", "SnapshotOnly", "NormalizedMaximum"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			f := newWorkerTestFixture(t)
			chat := f.createRunningChat(t)
			starter := newBlockingTaskStarter(false)
			switch mode {
			case "ExpectedExit":
				starter.exitErr = errTaskExpectedExit
			case "UnexpectedError":
				starter.exitErr = xerrors.New("operation failed")
			case "Panic":
				starter.panicOnExit = true
			}
			clock := quartz.NewMock(t).WithLogger(quartz.NoOpLogger)
			trap := clock.Trap().NewTimer("chatworker", "task-retry-generation")
			defer trap.Close()
			opts := testOptions(t, f, starter)
			opts.Clock = clock
			opts.TaskRetryInitialBackoff = 100 * time.Millisecond
			opts.TaskRetryMaxBackoff = 400 * time.Millisecond
			if mode == "NormalizedMaximum" {
				opts.TaskRetryMaxBackoff = time.Millisecond
			}
			worker := startWorker(t, opts)
			first := starter.waitCall(t, taskKindGeneration, chat.ID)
			for index, delay := range []time.Duration{100 * time.Millisecond, 200 * time.Millisecond, 400 * time.Millisecond, 400 * time.Millisecond} {
				if mode == "NormalizedMaximum" {
					delay = 100 * time.Millisecond
				}
				if mode == "SnapshotOnly" {
					forceExecutionState(t, f, chat.ID, database.ChatStatusRunning, false)
				}
				starter.release(t, index)
				wait := trap.MustWait(ctx)
				require.Equal(t, delay, wait.Duration)
				require.Empty(t, starter.callCh, "no operation may run before its backoff")
				wait.MustRelease(ctx)
				clock.Advance(delay).MustWait(ctx)
				next := starter.waitCall(t, taskKindGeneration, chat.ID)
				require.Equal(t, first.input.HistoryVersion, next.input.HistoryVersion)
				require.Equal(t, first.input.RunnerID, next.input.RunnerID)
			}
			require.NoError(t, worker.Close())
			require.Empty(t, starter.callCh)
		})
	}
}

func TestRunner_WorkChangeSupersedesBackoff(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	f := newWorkerTestFixture(t)
	chat := f.createRunningChat(t)
	clock := quartz.NewMock(t).WithLogger(quartz.NoOpLogger)
	trap := clock.Trap().NewTimer("chatworker", "task-retry-generation")
	defer trap.Close()
	starter := newBlockingTaskStarter(false)
	opts := testOptions(t, f, starter)
	opts.Clock = clock
	startWorker(t, opts)
	starter.waitCall(t, taskKindGeneration, chat.ID)
	starter.release(t, 0)
	trap.MustWait(ctx).MustRelease(ctx)
	updated := forceExecutionStateAndPublish(t, f, chat.ID, database.ChatStatusInterrupting, false)
	next := starter.waitCall(t, taskKindInterrupt, chat.ID)
	require.Equal(t, updated.HistoryVersion, next.input.HistoryVersion)
}

func TestRunner_ShutdownDuringBackoff(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	f := newWorkerTestFixture(t)
	chat := f.createRunningChat(t)
	clock := quartz.NewMock(t).WithLogger(quartz.NoOpLogger)
	trap := clock.Trap().NewTimer("chatworker", "task-retry-generation")
	defer trap.Close()
	starter := newBlockingTaskStarter(false)
	opts := testOptions(t, f, starter)
	opts.Clock = clock
	worker := startWorker(t, opts)
	starter.waitCall(t, taskKindGeneration, chat.ID)
	starter.release(t, 0)
	trap.MustWait(ctx).MustRelease(ctx)
	require.NoError(t, worker.Close())
	clock.Advance(defaultTaskRetryMaxBackoff).MustWait(ctx)
	require.Empty(t, starter.callCh)
}

func TestRunner_CompletedOperationReleasesIdleChat(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		status   database.ChatStatus
		archived bool
	}{
		{name: "Waiting", status: database.ChatStatusWaiting},
		{name: "Error", status: database.ChatStatusError},
		{name: "Archived", status: database.ChatStatusRunning, archived: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			f := newWorkerTestFixture(t)
			chat := f.createRunningChat(t)
			clock := quartz.NewMock(t).WithLogger(quartz.NoOpLogger)
			starter := newBlockingTaskStarter(false)
			opts := testOptions(t, f, starter)
			opts.Clock = clock
			startWorker(t, opts)
			starter.waitCall(t, taskKindGeneration, chat.ID)
			forceExecutionState(t, f, chat.ID, tc.status, tc.archived)
			starter.release(t, 0)
			testutil.Eventually(ctx, t, func(ctx context.Context) bool {
				latest, err := f.db.GetChatByID(ctx, chat.ID)
				return err == nil && !latest.WorkerID.Valid && !latest.RunnerID.Valid
			}, testutil.IntervalFast)
			require.Empty(t, starter.callCh)
		})
	}
}

func TestRunnerExecutor_CancelsRead(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	executor := runnerExecutor{ctx: ctx}
	defer executor.close()
	started := make(chan struct{})
	old := executor.start(nil, func(ctx context.Context) runnerActivityResult {
		close(started)
		<-ctx.Done()
		return runnerActivityResult{err: ctx.Err()}
	})
	testutil.TryReceive(ctx, t, started)
	current := executor.start(nil, func(context.Context) runnerActivityResult {
		return runnerActivityResult{released: true}
	})
	require.ErrorIs(t, testutil.RequireReceive(ctx, t, old).err, context.Canceled)
	require.True(t, testutil.RequireReceive(ctx, t, current).released)
}

func TestRunnerExecutor_PanicReturnsResult(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	executor := runnerExecutor{ctx: ctx}
	defer executor.close()
	result := executor.start(nil, func(context.Context) runnerActivityResult {
		panic("read failed")
	})
	require.ErrorContains(t, testutil.RequireReceive(ctx, t, result).err, "chatworker task panic: read failed")
}

func TestRunnerExecutor_LateCompletionKeepsOwnResult(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	executor := runnerExecutor{ctx: ctx}
	defer executor.close()
	oldGate, currentGate := make(chan struct{}), make(chan struct{})
	defer close(currentGate)
	defer close(oldGate)
	started := make(chan struct{})
	old := executor.start(nil, func(context.Context) runnerActivityResult {
		close(started)
		testutil.TryReceive(ctx, t, oldGate)
		return runnerActivityResult{chat: database.Chat{SnapshotVersion: 1}}
	})
	testutil.TryReceive(ctx, t, started)
	current := executor.start(nil, func(context.Context) runnerActivityResult {
		testutil.TryReceive(ctx, t, currentGate)
		return runnerActivityResult{chat: database.Chat{SnapshotVersion: 2}}
	})
	testutil.RequireSend(ctx, t, oldGate, struct{}{})
	require.Equal(t, int64(1), testutil.RequireReceive(ctx, t, old).chat.SnapshotVersion)
	require.Empty(t, current, "old completion must not satisfy the current invocation")
	testutil.RequireSend(ctx, t, currentGate, struct{}{})
	require.Equal(t, int64(2), testutil.RequireReceive(ctx, t, current).chat.SnapshotVersion)
}

func TestRunnerExecutor_CanceledBeforeInvocation(t *testing.T) {
	t.Parallel()
	waitCtx := testutil.Context(t, testutil.WaitLong)
	ctx, cancel := context.WithCancel(waitCtx)
	cancel()
	executor := runnerExecutor{ctx: ctx}
	defer executor.close()
	result := executor.start(nil, func(context.Context) runnerActivityResult {
		t.Error("canceled executor must not invoke work")
		return runnerActivityResult{}
	})
	require.ErrorIs(t, testutil.RequireReceive(waitCtx, t, result).err, context.Canceled)
}

func TestRunnerExecutor_CancelsQueuedSameWork(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	executor := runnerExecutor{ctx: ctx, locked: make(map[localWorkKey]chan struct{})}
	defer executor.close()
	gate := make(chan struct{})
	defer close(gate)
	started := make(chan struct{})
	key := localWorkKey{historyVersion: 1, status: database.ChatStatusRunning}
	first := executor.start(&key, func(context.Context) runnerActivityResult {
		close(started)
		testutil.TryReceive(ctx, t, gate)
		return runnerActivityResult{}
	})
	testutil.TryReceive(ctx, t, started)
	queued := executor.start(&key, func(context.Context) runnerActivityResult {
		t.Error("same work must not run before its predecessor drains")
		return runnerActivityResult{}
	})
	otherKey := localWorkKey{historyVersion: 2, status: database.ChatStatusRunning}
	current := executor.start(&otherKey, func(context.Context) runnerActivityResult {
		return runnerActivityResult{released: true}
	})
	require.ErrorIs(t, testutil.RequireReceive(ctx, t, queued).err, context.Canceled)
	require.True(t, testutil.RequireReceive(ctx, t, current).released)
	testutil.RequireSend(ctx, t, gate, struct{}{})
	require.NoError(t, testutil.RequireReceive(ctx, t, first).err)
}

func TestRunner_TaskTimeoutClassification(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		err   error
		retry bool
	}{
		{name: "Success"},
		{name: "ExpectedExit", err: errTaskExpectedExit},
		{name: "UnexpectedError", err: xerrors.New("operation failed"), retry: true},
		{name: "ExpectedRetryable", err: taskRetryableError{err: errTaskExpectedExit}, retry: true},
		{name: "ExpectedCanceled", err: errors.Join(errTaskExpectedExit, context.Canceled), retry: true},
		{name: "ExpectedDeadline", err: errors.Join(errTaskExpectedExit, context.DeadlineExceeded), retry: true},
		{name: "ExpectedTimeout", err: errors.Join(errTaskExpectedExit, errTaskTimeout), retry: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			f := newWorkerTestFixture(t)
			chat := f.createRunningChat(t)
			clock := quartz.NewMock(t).WithLogger(quartz.NoOpLogger)
			starter := newBlockingTaskStarter(true)
			starter.exitErr = tc.err
			opts := testOptions(t, f, starter)
			opts.Clock = clock
			worker := startWorker(t, opts)
			cancelWorker := worker.cancel
			t.Cleanup(func() {
				cancelWorker()
				starter.releaseAll()
			})
			first := starter.waitCall(t, taskKindGeneration, chat.ID)
			forceExecutionState(t, f, chat.ID, database.ChatStatusInterrupting, false)
			clock.Advance(defaultTaskTimeout).MustWait(ctx)
			require.ErrorIs(t, context.Cause(first.ctx), errTaskTimeout)
			if tc.retry {
				trap := clock.Trap().NewTimer("chatworker", "task-retry-generation")
				defer trap.Close()
				starter.release(t, 0)
				wait := trap.MustWait(ctx)
				require.Empty(t, starter.callCh)
				wait.MustRelease(ctx)
				clock.Advance(wait.Duration).MustWait(ctx)
			} else {
				starter.release(t, 0)
			}
			starter.waitCall(t, taskKindInterrupt, chat.ID)
		})
	}
}

func TestRunnerExecutor_TaskTimeoutCancelsDatabaseQuery(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	f := newWorkerTestFixture(t)
	clock := quartz.NewMock(t).WithLogger(quartz.NoOpLogger)
	executor := runnerExecutor{ctx: ctx}
	defer executor.close()
	started := make(chan struct{})
	causes := make(chan error, 1)
	result := executor.start(nil, func(ctx context.Context) runnerActivityResult {
		attemptCtx, cancel := taskAttemptContext(ctx, clock, taskKindGeneration)
		defer cancel()
		close(started)
		<-attemptCtx.Done()
		causes <- context.Cause(attemptCtx)
		_, err := f.db.GetDatabaseNow(attemptCtx)
		return runnerActivityResult{err: normalizeTaskTransitionError(err, "db query")}
	})
	testutil.TryReceive(ctx, t, started)
	clock.Advance(defaultTaskTimeout).MustWait(ctx)
	err := testutil.RequireReceive(ctx, t, result).err
	require.ErrorIs(t, err, context.Canceled)
	require.ErrorIs(t, err, errTaskExpectedExit)
	require.NotErrorIs(t, err, errTaskTimeout)
	require.ErrorIs(t, testutil.RequireReceive(ctx, t, causes), errTaskTimeout)
}

func TestRunner_CancellationBeforeOperation(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	f := newWorkerTestFixture(t)
	f.createRunningChat(t)
	clock := quartz.NewMock(t).WithLogger(quartz.NoOpLogger)
	trap := clock.Trap().AfterFunc("chatworker", "task-timeout-generation")
	defer trap.Close()
	starter := newRecordingTaskStarter()
	opts := testOptions(t, f, starter)
	opts.Clock = clock
	worker := startWorker(t, opts)
	wait := trap.MustWait(ctx)
	worker.cancel()
	wait.MustRelease(ctx)
	require.NoError(t, worker.Close())
	require.Empty(t, starter.callCh, "cancellation before invocation must prevent operation")
}

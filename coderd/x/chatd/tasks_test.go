//nolint:testpackage // These tests exercise package-private task seams.
package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	dbpubsub "github.com/coder/coder/v2/coderd/database/pubsub"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatretry"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/coderd/x/chatd/messagepartbuffer"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestRetryWrapper_ExpectedExitsDoNotRetry(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	sink := testutil.NewFakeSink(t)
	calls := 0
	err := runTaskWithRetry(ctx, retryWrapperOptions{
		clock:        quartz.NewMock(t),
		logger:       sink.Logger(),
		initialDelay: time.Second,
		maxDelay:     time.Second,
	}, taskKindInterrupt, retryWrapperTaskInfo{}, func(context.Context) error {
		calls++
		return errTaskExpectedExit
	})
	require.NoError(t, err)
	require.Equal(t, 1, calls)
	require.Empty(t, entriesWithMessage(sink, "chatworker task retrying"))
}

func TestRetryWrapper_UnexpectedErrorsRetry(t *testing.T) {
	t.Parallel()

	clock := quartz.NewMock(t)
	trap := clock.Trap().NewTimer("chatworker", "task-retry-requires_action_timeout")
	defer trap.Close()
	ctx := testutil.Context(t, testutil.WaitLong)
	sink := testutil.NewFakeSink(t)
	calls := 0
	done := make(chan error, 1)
	go func() {
		done <- runTaskWithRetry(ctx, retryWrapperOptions{
			clock:        clock,
			logger:       sink.Logger(),
			initialDelay: time.Minute,
			maxDelay:     time.Minute,
		}, taskKindRequiresActionTimeout, retryWrapperTaskInfo{}, func(context.Context) error {
			calls++
			if calls == 1 {
				return xerrors.New("database unavailable")
			}
			return nil
		})
	}()

	trap.MustWait(ctx).MustRelease(ctx)
	clock.Advance(time.Minute).MustWait(ctx)
	require.NoError(t, <-done)
	require.Equal(t, 2, calls)
	entries := entriesWithMessage(sink, "chatworker task retrying")
	require.Len(t, entries, 1)
	require.Equal(t, string(taskKindRequiresActionTimeout), sinkFieldValue(t, entries[0].Fields, "task_kind"))
	require.Equal(t, time.Minute.String(), sinkFieldValue(t, entries[0].Fields, "delay"))
	require.Contains(t, sinkFieldValue(t, entries[0].Fields, "error"), "database unavailable")
}

func TestRetryWrapper_PanicsRetry(t *testing.T) {
	t.Parallel()

	clock := quartz.NewMock(t)
	trap := clock.Trap().NewTimer("chatworker", "task-retry-generation")
	defer trap.Close()
	ctx := testutil.Context(t, testutil.WaitLong)
	sink := testutil.NewFakeSink(t)
	calls := 0
	done := make(chan error, 1)
	go func() {
		done <- runTaskWithRetry(ctx, retryWrapperOptions{
			clock:        clock,
			logger:       sink.Logger(),
			initialDelay: time.Minute,
			maxDelay:     time.Minute,
		}, taskKindGeneration, retryWrapperTaskInfo{}, func(context.Context) error {
			calls++
			if calls == 1 {
				panic("database unavailable")
			}
			return nil
		})
	}()

	trap.MustWait(ctx).MustRelease(ctx)
	clock.Advance(time.Minute).MustWait(ctx)
	require.NoError(t, <-done)
	require.Equal(t, 2, calls)
	entries := entriesWithMessage(sink, "chatworker task retrying")
	require.Len(t, entries, 1)
	require.Contains(t, sinkFieldValue(t, entries[0].Fields, "error"), "chatworker task panic: database unavailable")
}

// database/sql returns ctx.Err() from ctxDriverQuery, not
// context.Cause(ctx). This test checks that the retry logic
// doesn't classify such an error as an expected exit when
// task timeout is the cause of the cancellation.
func TestRetryWrapper_TaskTimeoutDBQueryCancellationRetries(t *testing.T) {
	t.Parallel()

	f := newTaskTestFixture(t)
	clock := quartz.NewMock(t)
	timeoutTrap := clock.Trap().AfterFunc("chatworker", "task-timeout-generation")
	retryTrap := clock.Trap().NewTimer("chatworker", "task-retry-generation")
	defer retryTrap.Close()
	ctx := testutil.Context(t, testutil.WaitLong)
	sink := testutil.NewFakeSink(t)
	calls := 0
	firstCallStarted := make(chan struct{})
	var firstQueryErr error
	var firstQueryCause error
	done := make(chan error, 1)
	go func() {
		done <- runTaskWithRetry(ctx, retryWrapperOptions{
			clock:        clock,
			logger:       sink.Logger(),
			initialDelay: time.Minute,
			maxDelay:     time.Minute,
		}, taskKindGeneration, retryWrapperTaskInfo{}, func(ctx context.Context) error {
			calls++
			if calls == 1 {
				close(firstCallStarted)
				<-ctx.Done()
				_, err := f.db.GetDatabaseNow(ctx)
				firstQueryErr = err
				firstQueryCause = context.Cause(ctx)
				return normalizeTaskTransitionError(err, "db query")
			}
			return nil
		})
	}()

	timeoutTrap.MustWait(ctx).MustRelease(ctx)
	timeoutTrap.Close()
	<-firstCallStarted
	clock.Advance(defaultTaskTimeout).MustWait(ctx)
	retryTrap.MustWait(ctx).MustRelease(ctx)
	clock.Advance(time.Minute).MustWait(ctx)
	require.NoError(t, <-done)
	require.Equal(t, 2, calls)
	require.ErrorIs(t, firstQueryErr, context.Canceled)
	require.NotErrorIs(t, firstQueryErr, errTaskTimeout)
	require.ErrorIs(t, firstQueryCause, errTaskTimeout)
	entries := entriesWithMessage(sink, "chatworker task retrying")
	require.Len(t, entries, 1)
	require.Contains(t, sinkFieldValue(t, entries[0].Fields, "error"), errTaskTimeout.Error())
	require.Contains(t, sinkFieldValue(t, entries[0].Fields, "error"), context.Canceled.Error())
}

func TestRetryWrapper_ContextCancellationDoesNotRetryOrLog(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(testutil.Context(t, testutil.WaitLong))
	cancel()
	sink := testutil.NewFakeSink(t)
	calls := 0
	original := xerrors.New("database unavailable")
	err := runTaskWithRetry(ctx, retryWrapperOptions{
		clock:        quartz.NewMock(t),
		logger:       sink.Logger(),
		initialDelay: time.Second,
		maxDelay:     time.Second,
	}, taskKindGeneration, retryWrapperTaskInfo{}, func(context.Context) error {
		calls++
		return original
	})
	require.NoError(t, err)
	require.Equal(t, 1, calls)
	require.Empty(t, entriesWithMessage(sink, "chatworker task retrying"))
}

func TestNormalizeTaskErrors_ContextCancellationIsExpectedExit(t *testing.T) {
	t.Parallel()

	err := normalizeTaskInfrastructureError(context.Canceled, "lock chat")
	require.ErrorIs(t, err, errTaskExpectedExit)
	require.ErrorIs(t, err, context.Canceled)
	require.NotErrorIs(t, err, errTaskRetryable)
	require.NotErrorIs(t, err, errTaskTimeout)

	err = normalizeTaskTransitionError(context.Canceled, "commit chat")
	require.ErrorIs(t, err, errTaskExpectedExit)
	require.ErrorIs(t, err, context.Canceled)
	require.NotErrorIs(t, err, errTaskRetryable)
	require.NotErrorIs(t, err, errTaskTimeout)
}

func entriesWithMessage(sink *testutil.FakeSink, message string) []slog.SinkEntry {
	return sink.Entries(func(e slog.SinkEntry) bool { return e.Message == message })
}

func sinkFieldValue(t *testing.T, fields slog.Map, name string) string {
	t.Helper()
	for _, f := range fields {
		if f.Name == name {
			return fmt.Sprint(f.Value)
		}
	}
	t.Fatalf("missing log field %q", name)
	return ""
}

func TestInterruptTask_FinishInterruptionOnly(t *testing.T) {
	t.Parallel()

	f := newTaskTestFixture(t)
	chat := f.createRunningChat(t)
	workerID := uuid.New()
	runnerID := uuid.New()
	acquired := f.acquireChat(t, chat.ID, workerID, runnerID)
	recorder := newTaskSideEffectRecorder()
	starter := newTestTaskStarter(t, f, recorder)
	buffer := starter.opts.MessagePartBuffer
	key := messagepartbuffer.Key{
		ChatID:            chat.ID,
		HistoryVersion:    acquired.HistoryVersion,
		GenerationAttempt: acquired.GenerationAttempt,
	}
	require.NoError(t, buffer.CreateEpisode(key))
	require.NoError(t, buffer.AddPart(key, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageText("partial answer")))
	interrupting := f.interruptChat(t, chat.ID)
	require.Equal(t, database.ChatStatusInterrupting, interrupting.Status)

	err := starter.StartInterrupt(testutil.Context(t, testutil.WaitLong), chatWorkerTaskStartInput{
		ChatID:            chat.ID,
		WorkerID:          workerID,
		RunnerID:          runnerID,
		HistoryVersion:    interrupting.HistoryVersion,
		GenerationAttempt: interrupting.GenerationAttempt,
		Status:            database.ChatStatusInterrupting,
	})
	require.NoError(t, err)

	latest, err := f.db.GetChatByID(testutil.Context(t, testutil.WaitShort), chat.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusRunning, latest.Status)
	recorder.requireStateHint(t, chat.ID, latest.SnapshotVersion, database.ChatStatusRunning)
	recorder.requireInterruptionOutcome(t, chat.ID, database.ChatStatusRunning)
	recorder.requireCleanupCount(t, 0)
	f.requireWatchEvent(t, chat.ID, codersdk.ChatWatchEventKindStatusChange)

	messages, err := f.db.GetChatMessagesByChatID(testutil.Context(t, testutil.WaitShort), database.GetChatMessagesByChatIDParams{ChatID: chat.ID})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(messages), 3)
	parts, err := chatprompt.ParseContent(messages[len(messages)-2])
	require.NoError(t, err)
	require.Equal(t, []codersdk.ChatMessagePart{codersdk.ChatMessageText("partial answer")}, parts)
	require.Equal(t, database.ChatMessageRoleUser, messages[len(messages)-1].Role)
}

func TestInterruptTask_StaleFenceExits(t *testing.T) {
	t.Parallel()

	f := newTaskTestFixture(t)
	chat := f.createRunningChat(t)
	workerID := uuid.New()
	runnerID := uuid.New()
	f.acquireChat(t, chat.ID, workerID, runnerID)
	interrupting := f.interruptChat(t, chat.ID)
	otherWorkerID := uuid.New()
	otherRunnerID := uuid.New()
	f.acquireChat(t, chat.ID, otherWorkerID, otherRunnerID)
	recorder := newTaskSideEffectRecorder()
	starter := newTestTaskStarter(t, f, recorder)

	err := starter.StartInterrupt(testutil.Context(t, testutil.WaitLong), chatWorkerTaskStartInput{
		ChatID:            chat.ID,
		WorkerID:          workerID,
		RunnerID:          runnerID,
		HistoryVersion:    interrupting.HistoryVersion,
		GenerationAttempt: interrupting.GenerationAttempt,
		Status:            database.ChatStatusInterrupting,
	})
	require.ErrorIs(t, err, errTaskExpectedExit)

	latest, err := f.db.GetChatByID(testutil.Context(t, testutil.WaitShort), chat.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusInterrupting, latest.Status)
	require.Equal(t, otherWorkerID, latest.WorkerID.UUID)
	require.Equal(t, otherRunnerID, latest.RunnerID.UUID)
	recorder.requireStateHintCount(t, 0)
	f.requireNoWatchEvents(t)
}

func TestInterruptTask_MissingEpisodePersistsNilPartials(t *testing.T) {
	t.Parallel()

	f := newTaskTestFixture(t)
	chat := f.createRunningChat(t)
	workerID := uuid.New()
	runnerID := uuid.New()
	f.acquireChat(t, chat.ID, workerID, runnerID)
	interrupting := f.forceExecutionState(t, chat.ID, database.ChatStatusInterrupting, false, sql.NullTime{})
	recorder := newTaskSideEffectRecorder()
	starter := newTestTaskStarter(t, f, recorder)

	err := starter.StartInterrupt(testutil.Context(t, testutil.WaitLong), chatWorkerTaskStartInput{
		ChatID:            chat.ID,
		WorkerID:          workerID,
		RunnerID:          runnerID,
		HistoryVersion:    interrupting.HistoryVersion,
		GenerationAttempt: interrupting.GenerationAttempt,
		Status:            database.ChatStatusInterrupting,
	})
	require.NoError(t, err)

	latest, err := f.db.GetChatByID(testutil.Context(t, testutil.WaitShort), chat.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusWaiting, latest.Status)
	recorder.requireInterruptionOutcome(t, chat.ID, database.ChatStatusWaiting)
	messages, err := f.db.GetChatMessagesByChatID(testutil.Context(t, testutil.WaitShort), database.GetChatMessagesByChatIDParams{ChatID: chat.ID})
	require.NoError(t, err)
	require.Len(t, messages, 1)
	recorder.requireStateHint(t, chat.ID, latest.SnapshotVersion, database.ChatStatusWaiting)
}

func TestInterruptTask_BufferedPartsBecomePartialMessages(t *testing.T) {
	t.Parallel()

	f := newTaskTestFixture(t)
	chat := f.createRunningChat(t)
	workerID := uuid.New()
	runnerID := uuid.New()
	acquired := f.acquireChat(t, chat.ID, workerID, runnerID)
	recorder := newTaskSideEffectRecorder()
	starter := newTestTaskStarter(t, f, recorder)
	buffer := starter.opts.MessagePartBuffer
	key := messagepartbuffer.Key{ChatID: chat.ID, HistoryVersion: acquired.HistoryVersion, GenerationAttempt: acquired.GenerationAttempt}
	require.NoError(t, buffer.CreateEpisode(key))
	callID := "call_" + uuid.NewString()
	require.NoError(t, buffer.AddPart(key, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessagePart{
		Type:       codersdk.ChatMessagePartTypeToolCall,
		ToolCallID: callID,
		ToolName:   "local_tool",
		Args:       json.RawMessage(`{"value":1}`),
	}))
	interrupting := f.interruptChat(t, chat.ID)

	err := starter.StartInterrupt(testutil.Context(t, testutil.WaitLong), chatWorkerTaskStartInput{
		ChatID:            chat.ID,
		WorkerID:          workerID,
		RunnerID:          runnerID,
		HistoryVersion:    interrupting.HistoryVersion,
		GenerationAttempt: interrupting.GenerationAttempt,
		Status:            database.ChatStatusInterrupting,
	})
	require.NoError(t, err)

	messages, err := f.db.GetChatMessagesByChatID(testutil.Context(t, testutil.WaitShort), database.GetChatMessagesByChatIDParams{ChatID: chat.ID})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(messages), 4)
	assistant := messages[len(messages)-3]
	tool := messages[len(messages)-2]
	require.Equal(t, database.ChatMessageRoleAssistant, assistant.Role)
	require.Equal(t, database.ChatMessageRoleTool, tool.Role)
	toolParts, err := chatprompt.ParseContent(tool)
	require.NoError(t, err)
	require.Len(t, toolParts, 1)
	require.Equal(t, codersdk.ChatMessagePartTypeToolResult, toolParts[0].Type)
	require.Equal(t, callID, toolParts[0].ToolCallID)
	require.True(t, toolParts[0].IsError)
}

func TestRequiresActionTimeout_ExpiredCancelsOnly(t *testing.T) {
	t.Parallel()

	f := newTaskTestFixture(t)
	chat := f.createRequiresActionChat(t)
	workerID := uuid.New()
	runnerID := uuid.New()
	acquired := f.acquireChat(t, chat.ID, workerID, runnerID)
	expired := f.setRequiresActionDeadline(t, chat.ID, sql.NullTime{Time: time.Now().Add(-time.Minute), Valid: true})
	recorder := newTaskSideEffectRecorder()
	starter := newTestTaskStarter(t, f, recorder)

	err := starter.StartRequiresActionTimeout(testutil.Context(t, testutil.WaitLong), chatWorkerTaskStartInput{
		ChatID:                   chat.ID,
		WorkerID:                 workerID,
		RunnerID:                 runnerID,
		HistoryVersion:           acquired.HistoryVersion,
		Status:                   database.ChatStatusRequiresAction,
		RequiresActionDeadlineAt: expired.RequiresActionDeadlineAt,
	})
	require.NoError(t, err)

	latest, err := f.db.GetChatByID(testutil.Context(t, testutil.WaitShort), chat.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusRunning, latest.Status)
	require.False(t, latest.RequiresActionDeadlineAt.Valid)
	recorder.requireStateHint(t, chat.ID, latest.SnapshotVersion, database.ChatStatusRunning)
	f.requireWatchEvent(t, chat.ID, codersdk.ChatWatchEventKindStatusChange)
}

func TestRequiresActionTimeout_NullDeadlineCancelsImmediately(t *testing.T) {
	t.Parallel()

	f := newTaskTestFixture(t)
	chat := f.createRequiresActionChat(t)
	workerID := uuid.New()
	runnerID := uuid.New()
	acquired := f.acquireChat(t, chat.ID, workerID, runnerID)
	nullDeadline := f.setRequiresActionDeadline(t, chat.ID, sql.NullTime{})
	recorder := newTaskSideEffectRecorder()
	starter := newTestTaskStarter(t, f, recorder)

	err := starter.StartRequiresActionTimeout(testutil.Context(t, testutil.WaitLong), chatWorkerTaskStartInput{
		ChatID:                   chat.ID,
		WorkerID:                 workerID,
		RunnerID:                 runnerID,
		HistoryVersion:           acquired.HistoryVersion,
		Status:                   database.ChatStatusRequiresAction,
		RequiresActionDeadlineAt: nullDeadline.RequiresActionDeadlineAt,
	})
	require.NoError(t, err)

	latest, err := f.db.GetChatByID(testutil.Context(t, testutil.WaitShort), chat.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusRunning, latest.Status)
	recorder.requireStateHint(t, chat.ID, latest.SnapshotVersion, database.ChatStatusRunning)
}

func TestRequiresActionTimeout_StaleFenceExitsAfterToolResult(t *testing.T) {
	t.Parallel()

	f := newTaskTestFixture(t)
	chat := f.createRequiresActionChat(t)
	workerID := uuid.New()
	runnerID := uuid.New()
	acquired := f.acquireChat(t, chat.ID, workerID, runnerID)
	expired := f.setRequiresActionDeadline(t, chat.ID, sql.NullTime{Time: time.Now().Add(-time.Minute), Valid: true})
	f.forceExecutionState(t, chat.ID, database.ChatStatusRunning, false, sql.NullTime{})
	recorder := newTaskSideEffectRecorder()
	starter := newTestTaskStarter(t, f, recorder)

	err := starter.StartRequiresActionTimeout(testutil.Context(t, testutil.WaitLong), chatWorkerTaskStartInput{
		ChatID:                   chat.ID,
		WorkerID:                 workerID,
		RunnerID:                 runnerID,
		HistoryVersion:           acquired.HistoryVersion,
		Status:                   database.ChatStatusRequiresAction,
		RequiresActionDeadlineAt: expired.RequiresActionDeadlineAt,
	})
	require.ErrorIs(t, err, errTaskExpectedExit)

	latest, err := f.db.GetChatByID(testutil.Context(t, testutil.WaitShort), chat.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusRunning, latest.Status)
	recorder.requireStateHintCount(t, 0)
	f.requireNoWatchEvents(t)
}

func TestAbandonTask_AbandonOnly(t *testing.T) {
	t.Parallel()

	f := newTaskTestFixture(t)
	chat := f.createRunningChat(t)
	workerID := uuid.New()
	runnerID := uuid.New()
	acquired := f.acquireChat(t, chat.ID, workerID, runnerID)
	recorder := newTaskSideEffectRecorder()
	starter := newTestTaskStarter(t, f, recorder)

	err := starter.StartAbandon(testutil.Context(t, testutil.WaitLong), chatWorkerTaskStartInput{
		ChatID:         chat.ID,
		WorkerID:       workerID,
		RunnerID:       runnerID,
		HistoryVersion: acquired.HistoryVersion,
		Status:         database.ChatStatusRunning,
	})
	require.NoError(t, err)

	latest, err := f.db.GetChatByID(testutil.Context(t, testutil.WaitShort), chat.ID)
	require.NoError(t, err)
	require.False(t, latest.WorkerID.Valid)
	require.False(t, latest.RunnerID.Valid)
	recorder.requireCleanup(t, chat.ID, runnerID)
	recorder.requireStateHintCount(t, 0)
	f.requireNoWatchEvents(t)
}

func TestAbandonTask_OwnershipMismatchRequestsCleanup(t *testing.T) {
	t.Parallel()

	f := newTaskTestFixture(t)
	chat := f.createRunningChat(t)
	workerID := uuid.New()
	runnerID := uuid.New()
	f.acquireChat(t, chat.ID, workerID, runnerID)
	otherWorkerID := uuid.New()
	otherRunnerID := uuid.New()
	latestOwner := f.acquireChat(t, chat.ID, otherWorkerID, otherRunnerID)
	recorder := newTaskSideEffectRecorder()
	starter := newTestTaskStarter(t, f, recorder)

	err := starter.StartAbandon(testutil.Context(t, testutil.WaitLong), chatWorkerTaskStartInput{
		ChatID:         chat.ID,
		WorkerID:       workerID,
		RunnerID:       runnerID,
		HistoryVersion: latestOwner.HistoryVersion,
		Status:         database.ChatStatusRunning,
	})
	require.NoError(t, err)

	latest, err := f.db.GetChatByID(testutil.Context(t, testutil.WaitShort), chat.ID)
	require.NoError(t, err)
	require.Equal(t, otherWorkerID, latest.WorkerID.UUID)
	require.Equal(t, otherRunnerID, latest.RunnerID.UUID)
	recorder.requireCleanup(t, chat.ID, runnerID)
}

func TestAbandonTask_StaleStatusFenceExits(t *testing.T) {
	t.Parallel()

	f := newTaskTestFixture(t)
	chat := f.createRunningChat(t)
	workerID := uuid.New()
	runnerID := uuid.New()
	acquired := f.acquireChat(t, chat.ID, workerID, runnerID)
	f.forceExecutionState(t, chat.ID, database.ChatStatusInterrupting, false, sql.NullTime{})
	recorder := newTaskSideEffectRecorder()
	starter := newTestTaskStarter(t, f, recorder)

	err := starter.StartAbandon(testutil.Context(t, testutil.WaitLong), chatWorkerTaskStartInput{
		ChatID:         chat.ID,
		WorkerID:       workerID,
		RunnerID:       runnerID,
		HistoryVersion: acquired.HistoryVersion,
		Status:         database.ChatStatusWaiting,
	})
	require.ErrorIs(t, err, errTaskExpectedExit)

	latest, err := f.db.GetChatByID(testutil.Context(t, testutil.WaitShort), chat.ID)
	require.NoError(t, err)
	require.True(t, latest.WorkerID.Valid)
	require.True(t, latest.RunnerID.Valid)
	require.Equal(t, database.ChatStatusInterrupting, latest.Status)
	recorder.requireCleanupCount(t, 0)
}

func TestGenerationTask_RecordRetryState(t *testing.T) {
	t.Parallel()

	f := newTaskTestFixture(t)
	chat := f.createRunningChat(t)
	workerID := uuid.New()
	runnerID := uuid.New()
	acquired := f.acquireChat(t, chat.ID, workerID, runnerID)
	recorder := newTaskSideEffectRecorder()
	starter := newTestTaskStarter(t, f, recorder)

	attempt, err := starter.beginGenerationAttempt(
		testutil.Context(t, testutil.WaitLong),
		chatstate.NewChatMachine(f.db, f.pubsub, chat.ID),
		chatWorkerTaskStartInput{
			ChatID:         chat.ID,
			WorkerID:       workerID,
			RunnerID:       runnerID,
			HistoryVersion: acquired.HistoryVersion,
			Status:         database.ChatStatusRunning,
		},
	)
	require.NoError(t, err)
	attempt.closeEpisode()
	require.Equal(t, int64(1), attempt.number)
	before, err := f.db.GetChatByID(testutil.Context(t, testutil.WaitShort), chat.ID)
	require.NoError(t, err)
	require.False(t, before.RetryState.Valid)

	decision, err := starter.recordGenerationRetry(
		testutil.Context(t, testutil.WaitLong),
		chatstate.NewChatMachine(f.db, f.pubsub, chat.ID),
		chatWorkerTaskStartInput{
			ChatID:         chat.ID,
			WorkerID:       workerID,
			RunnerID:       runnerID,
			HistoryVersion: acquired.HistoryVersion,
			Status:         database.ChatStatusRunning,
		},
		chaterror.ClassifiedError{
			Message:    "OpenAI is rate limiting requests.",
			Kind:       codersdk.ChatErrorKindRateLimit,
			Provider:   "openai",
			Retryable:  true,
			StatusCode: 429,
		},
	)
	require.NoError(t, err)
	require.True(t, decision.retry)
	require.Equal(t, int64(1), decision.generationAttempt)
	require.Equal(t, chatretry.Delay(0), decision.delay)

	latest, err := f.db.GetChatByID(testutil.Context(t, testutil.WaitShort), chat.ID)
	require.NoError(t, err)
	require.True(t, latest.RetryState.Valid)
	require.Equal(t, latest.SnapshotVersion, latest.RetryStateVersion)
	require.Greater(t, latest.RetryStateVersion, before.RetryStateVersion)
	require.Equal(t, before.GenerationAttempt, latest.GenerationAttempt)
	recorder.requireStateHintCount(t, 0)

	var retryPayload codersdk.ChatStreamRetry
	require.NoError(t, json.Unmarshal(latest.RetryState.RawMessage, &retryPayload))
	require.Equal(t, 1, retryPayload.Attempt)
	require.Equal(t, chatretry.Delay(0).Milliseconds(), retryPayload.DelayMs)
	require.Equal(t, "OpenAI is rate limiting requests.", retryPayload.Error)
	require.Equal(t, codersdk.ChatErrorKindRateLimit, retryPayload.Kind)
	require.Equal(t, "openai", retryPayload.Provider)
	require.Equal(t, 429, retryPayload.StatusCode)
	require.False(t, retryPayload.RetryingAt.IsZero())
}

func TestGenerationTask_RecordRetryStateUsesDurableGenerationAttempt(t *testing.T) {
	t.Parallel()

	f := newTaskTestFixture(t)
	chat := f.createRunningChat(t)
	workerID := uuid.New()
	runnerID := uuid.New()
	acquired := f.acquireChat(t, chat.ID, workerID, runnerID)
	starter := newTestTaskStarter(t, f, newTaskSideEffectRecorder())
	machine := chatstate.NewChatMachine(f.db, f.pubsub, chat.ID)

	for range 3 {
		attempt, err := starter.beginGenerationAttempt(
			testutil.Context(t, testutil.WaitLong),
			machine,
			chatWorkerTaskStartInput{
				ChatID:         chat.ID,
				WorkerID:       workerID,
				RunnerID:       runnerID,
				HistoryVersion: acquired.HistoryVersion,
				Status:         database.ChatStatusRunning,
			},
		)
		require.NoError(t, err)
		attempt.closeEpisode()
		require.Positive(t, attempt.number)
	}

	decision, err := starter.recordGenerationRetry(
		testutil.Context(t, testutil.WaitLong),
		machine,
		chatWorkerTaskStartInput{
			ChatID:         chat.ID,
			WorkerID:       workerID,
			RunnerID:       runnerID,
			HistoryVersion: acquired.HistoryVersion,
			Status:         database.ChatStatusRunning,
		},
		chaterror.ClassifiedError{
			Message:   "OpenAI is temporarily unavailable.",
			Kind:      codersdk.ChatErrorKindTimeout,
			Provider:  "openai",
			Retryable: true,
		},
	)
	require.NoError(t, err)
	require.True(t, decision.retry)
	require.Equal(t, int64(3), decision.generationAttempt)
	require.Equal(t, chatretry.Delay(2), decision.delay)

	latest, err := f.db.GetChatByID(testutil.Context(t, testutil.WaitShort), chat.ID)
	require.NoError(t, err)
	var retryPayload codersdk.ChatStreamRetry
	require.NoError(t, json.Unmarshal(latest.RetryState.RawMessage, &retryPayload))
	require.Equal(t, 3, retryPayload.Attempt)
	require.Equal(t, chatretry.Delay(2).Milliseconds(), retryPayload.DelayMs)
}

func TestGenerationTask_RecordRetryStateClearedByNextAttempt(t *testing.T) {
	t.Parallel()

	f := newTaskTestFixture(t)
	chat := f.createRunningChat(t)
	workerID := uuid.New()
	runnerID := uuid.New()
	acquired := f.acquireChat(t, chat.ID, workerID, runnerID)
	starter := newTestTaskStarter(t, f, newTaskSideEffectRecorder())
	machine := chatstate.NewChatMachine(f.db, f.pubsub, chat.ID)
	input := chatWorkerTaskStartInput{
		ChatID:         chat.ID,
		WorkerID:       workerID,
		RunnerID:       runnerID,
		HistoryVersion: acquired.HistoryVersion,
		Status:         database.ChatStatusRunning,
	}

	attempt, err := starter.beginGenerationAttempt(testutil.Context(t, testutil.WaitLong), machine, input)
	require.NoError(t, err)
	attempt.closeEpisode()
	require.Equal(t, int64(1), attempt.number)
	_, err = starter.recordGenerationRetry(
		testutil.Context(t, testutil.WaitLong),
		machine,
		input,
		chaterror.ClassifiedError{
			Message:   "OpenAI is temporarily unavailable.",
			Kind:      codersdk.ChatErrorKindTimeout,
			Provider:  "openai",
			Retryable: true,
		},
	)
	require.NoError(t, err)
	withRetry, err := f.db.GetChatByID(testutil.Context(t, testutil.WaitShort), chat.ID)
	require.NoError(t, err)
	require.True(t, withRetry.RetryState.Valid)

	attempt, err = starter.beginGenerationAttempt(testutil.Context(t, testutil.WaitLong), machine, input)
	require.NoError(t, err)
	attempt.closeEpisode()
	require.Equal(t, int64(2), attempt.number)
	after, err := f.db.GetChatByID(testutil.Context(t, testutil.WaitShort), chat.ID)
	require.NoError(t, err)
	require.False(t, after.RetryState.Valid)
	require.Equal(t, after.SnapshotVersion, after.RetryStateVersion)
	require.Greater(t, after.RetryStateVersion, withRetry.RetryStateVersion)
}

func TestGenerationTask_RecordRetryStateStaleFenceExits(t *testing.T) {
	t.Parallel()

	f := newTaskTestFixture(t)
	chat := f.createRunningChat(t)
	workerID := uuid.New()
	runnerID := uuid.New()
	acquired := f.acquireChat(t, chat.ID, workerID, runnerID)
	starter := newTestTaskStarter(t, f, newTaskSideEffectRecorder())
	machine := chatstate.NewChatMachine(f.db, f.pubsub, chat.ID)
	attempt, err := starter.beginGenerationAttempt(
		testutil.Context(t, testutil.WaitLong),
		machine,
		chatWorkerTaskStartInput{
			ChatID:         chat.ID,
			WorkerID:       workerID,
			RunnerID:       runnerID,
			HistoryVersion: acquired.HistoryVersion,
			Status:         database.ChatStatusRunning,
		},
	)
	require.NoError(t, err)
	attempt.closeEpisode()
	require.Equal(t, int64(1), attempt.number)

	otherWorkerID := uuid.New()
	otherRunnerID := uuid.New()
	f.acquireChat(t, chat.ID, otherWorkerID, otherRunnerID)
	_, err = starter.recordGenerationRetry(
		testutil.Context(t, testutil.WaitLong),
		machine,
		chatWorkerTaskStartInput{
			ChatID:         chat.ID,
			WorkerID:       workerID,
			RunnerID:       runnerID,
			HistoryVersion: acquired.HistoryVersion,
			Status:         database.ChatStatusRunning,
		},
		chaterror.ClassifiedError{
			Message:   "OpenAI is temporarily unavailable.",
			Kind:      codersdk.ChatErrorKindTimeout,
			Provider:  "openai",
			Retryable: true,
		},
	)
	require.ErrorIs(t, err, errTaskExpectedExit)
	latest, err := f.db.GetChatByID(testutil.Context(t, testutil.WaitShort), chat.ID)
	require.NoError(t, err)
	require.False(t, latest.RetryState.Valid)
	require.Equal(t, otherWorkerID, latest.WorkerID.UUID)
	require.Equal(t, otherRunnerID, latest.RunnerID.UUID)
}

func TestRunner_StartsRealInterruptTask(t *testing.T) {
	t.Parallel()

	f := newTaskTestFixture(t)
	chat := f.createRunningChat(t)
	worker := startRealTaskWorker(t, f)
	waitOwnedChat(t, f, chat.ID, worker.chatWorkerID())

	interrupting := f.interruptChat(t, chat.ID)
	require.Equal(t, database.ChatStatusInterrupting, interrupting.Status)
	testutil.Eventually(testutil.Context(t, testutil.WaitLong), t, func(ctx context.Context) bool {
		latest, err := f.db.GetChatByID(ctx, chat.ID)
		return err == nil && latest.Status == database.ChatStatusRunning
	}, testutil.IntervalFast)
	latest, err := f.db.GetChatByID(testutil.Context(t, testutil.WaitShort), chat.ID)
	require.NoError(t, err)
	require.Equal(t, worker.chatWorkerID(), latest.WorkerID.UUID)
	f.requireWatchEvent(t, chat.ID, codersdk.ChatWatchEventKindStatusChange)
}

func TestRunner_StartsRealRequiresActionTimeoutTask(t *testing.T) {
	t.Parallel()

	f := newTaskTestFixture(t)
	chat := f.createRequiresActionChat(t)
	f.setRequiresActionDeadline(t, chat.ID, sql.NullTime{Time: time.Now().Add(-time.Minute), Valid: true})
	worker := startRealTaskWorker(t, f)

	testutil.Eventually(testutil.Context(t, testutil.WaitLong), t, func(ctx context.Context) bool {
		latest, err := f.db.GetChatByID(ctx, chat.ID)
		return err == nil && latest.Status == database.ChatStatusRunning && latest.WorkerID.Valid && latest.WorkerID.UUID == worker.chatWorkerID()
	}, testutil.IntervalFast)
	latest, err := f.db.GetChatByID(testutil.Context(t, testutil.WaitShort), chat.ID)
	require.NoError(t, err)
	require.True(t, latest.RunnerID.Valid)
	f.requireWatchEvent(t, chat.ID, codersdk.ChatWatchEventKindStatusChange)
}

func TestRunner_StartsRealAbandonTask(t *testing.T) {
	t.Parallel()

	f := newTaskTestFixture(t)
	chat := f.createRunningChat(t)
	worker := startRealTaskWorker(t, f)
	waitOwnedChat(t, f, chat.ID, worker.chatWorkerID())

	updated := f.forceExecutionState(t, chat.ID, database.ChatStatusError, false, sql.NullTime{})
	f.publishChatUpdate(t, updated)
	testutil.Eventually(testutil.Context(t, testutil.WaitLong), t, func(ctx context.Context) bool {
		latest, err := f.db.GetChatByID(ctx, chat.ID)
		return err == nil && !latest.WorkerID.Valid && !latest.RunnerID.Valid
	}, testutil.IntervalFast)
}

type taskTestFixture struct {
	db     database.Store
	pubsub *taskRecordingPubsub
	rawPS  dbpubsub.Pubsub
	sqlDB  *sql.DB
	user   database.User
	org    database.Organization
	model  database.ChatModelConfig
	apiKey database.APIKey
}

func newTaskTestFixture(t *testing.T) *taskTestFixture {
	t.Helper()
	db, ps, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: user.ID, OrganizationID: org.ID})
	dbgen.ChatProvider(t, db, database.ChatProvider{
		Provider:    "openai",
		DisplayName: "openai",
		BaseUrl:     "http://example.invalid",
	})
	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{IsDefault: true})
	apiKey, _ := dbgen.APIKey(t, db, database.APIKey{UserID: user.ID})
	return &taskTestFixture{db: db, pubsub: newTaskRecordingPubsub(ps), rawPS: ps, sqlDB: sqlDB, user: user, org: org, model: model, apiKey: apiKey}
}

func (f *taskTestFixture) createRunningChat(t *testing.T) database.Chat {
	t.Helper()
	res, err := chatstate.CreateChat(testutil.Context(t, testutil.WaitShort), f.db, f.pubsub, chatstate.CreateChatInput{
		OrganizationID:    f.org.ID,
		OwnerID:           f.user.ID,
		LastModelConfigID: f.model.ID,
		Title:             "test",
		ClientType:        database.ChatClientTypeApi,
		InitialMessages:   []chatstate.Message{taskUserTextMessage(t, "hello", f.user.ID, f.model.ID, f.apiKey.ID)},
	})
	require.NoError(t, err)
	f.pubsub.clear()
	return res.Chat
}

func (f *taskTestFixture) createRequiresActionChat(t *testing.T) database.Chat {
	t.Helper()
	toolName := "dynamic_" + uuid.NewString()
	dynamicTools, err := json.Marshal([]codersdk.DynamicTool{{
		Name:        toolName,
		Description: "test tool",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}})
	require.NoError(t, err)
	res, err := chatstate.CreateChat(testutil.Context(t, testutil.WaitShort), f.db, f.pubsub, chatstate.CreateChatInput{
		OrganizationID:    f.org.ID,
		OwnerID:           f.user.ID,
		LastModelConfigID: f.model.ID,
		Title:             "test",
		ClientType:        database.ChatClientTypeApi,
		DynamicTools:      pqtype.NullRawMessage{RawMessage: dynamicTools, Valid: true},
		InitialMessages:   []chatstate.Message{taskUserTextMessage(t, "hello", f.user.ID, f.model.ID, f.apiKey.ID)},
	})
	require.NoError(t, err)
	machine := chatstate.NewChatMachine(f.db, f.pubsub, res.Chat.ID)
	require.NoError(t, machine.Update(testutil.Context(t, testutil.WaitShort), func(tx *chatstate.Tx, store database.Store) error {
		_, err := tx.CommitStep(chatstate.CommitStepInput{Messages: []chatstate.Message{taskAssistantToolCallMessage(t, f.model.ID, toolName)}})
		return err
	}))
	require.NoError(t, machine.Update(testutil.Context(t, testutil.WaitShort), func(tx *chatstate.Tx, store database.Store) error {
		_, err := tx.EnterRequiresAction(chatstate.EnterRequiresActionInput{})
		return err
	}))
	chat, err := f.db.GetChatByID(testutil.Context(t, testutil.WaitShort), res.Chat.ID)
	require.NoError(t, err)
	f.pubsub.clear()
	return chat
}

func (f *taskTestFixture) acquireChat(t *testing.T, chatID uuid.UUID, workerID uuid.UUID, runnerID uuid.UUID) database.Chat {
	t.Helper()
	machine := chatstate.NewChatMachine(f.db, f.pubsub, chatID)
	require.NoError(t, machine.Update(testutil.Context(t, testutil.WaitShort), func(tx *chatstate.Tx, store database.Store) error {
		_, err := tx.Acquire(chatstate.AcquireInput{WorkerID: workerID, RunnerID: runnerID})
		return err
	}))
	chat, err := f.db.GetChatByID(testutil.Context(t, testutil.WaitShort), chatID)
	require.NoError(t, err)
	f.pubsub.clear()
	return chat
}

func (f *taskTestFixture) interruptChat(t *testing.T, chatID uuid.UUID) database.Chat {
	t.Helper()
	machine := chatstate.NewChatMachine(f.db, f.pubsub, chatID)
	require.NoError(t, machine.Update(testutil.Context(t, testutil.WaitShort), func(tx *chatstate.Tx, store database.Store) error {
		_, err := tx.SendMessage(chatstate.SendMessageInput{
			Message:      taskUserTextMessage(t, "interrupt", f.user.ID, f.model.ID, f.apiKey.ID),
			BusyBehavior: chatstate.BusyBehaviorInterrupt,
		})
		return err
	}))
	chat, err := f.db.GetChatByID(testutil.Context(t, testutil.WaitShort), chatID)
	require.NoError(t, err)
	f.pubsub.clear()
	return chat
}

func (f *taskTestFixture) forceExecutionState(t *testing.T, chatID uuid.UUID, status database.ChatStatus, archived bool, deadline sql.NullTime) database.Chat {
	t.Helper()
	var updated database.Chat
	require.NoError(t, f.db.InTx(func(store database.Store) error {
		if _, err := store.LockChatAndBumpSnapshotVersion(testutil.Context(t, testutil.WaitShort), chatID); err != nil {
			return err
		}
		chat, err := store.GetChatByID(testutil.Context(t, testutil.WaitShort), chatID)
		if err != nil {
			return err
		}
		updated, err = store.UpdateChatExecutionState(testutil.Context(t, testutil.WaitShort), database.UpdateChatExecutionStateParams{
			ID:                       chat.ID,
			Status:                   status,
			Archived:                 archived,
			WorkerID:                 chat.WorkerID,
			RunnerID:                 chat.RunnerID,
			LastError:                chat.LastError,
			RequiresActionDeadlineAt: deadline,
		})
		return err
	}, nil))
	f.pubsub.clear()
	return updated
}

func (f *taskTestFixture) setRequiresActionDeadline(t *testing.T, chatID uuid.UUID, deadline sql.NullTime) database.Chat {
	t.Helper()
	chat, err := f.db.GetChatByID(testutil.Context(t, testutil.WaitShort), chatID)
	require.NoError(t, err)
	return f.forceExecutionState(t, chatID, chat.Status, chat.Archived, deadline)
}

func (f *taskTestFixture) publishChatUpdate(t *testing.T, chat database.Chat) {
	t.Helper()
	msg := coderdpubsub.ChatStateUpdateMessage{
		SnapshotVersion:   chat.SnapshotVersion,
		HistoryVersion:    chat.HistoryVersion,
		QueueVersion:      chat.QueueVersion,
		RetryStateVersion: chat.RetryStateVersion,
		GenerationAttempt: chat.GenerationAttempt,
		Status:            string(chat.Status),
		Archived:          chat.Archived,
	}
	if chat.WorkerID.Valid {
		id := chat.WorkerID.UUID
		msg.WorkerID = &id
	}
	if chat.RunnerID.Valid {
		id := chat.RunnerID.UUID
		msg.RunnerID = &id
	}
	payload, err := json.Marshal(msg)
	require.NoError(t, err)
	require.NoError(t, f.pubsub.Publish(coderdpubsub.ChatStateUpdateChannel(chat.ID), payload))
}

func (f *taskTestFixture) requireWatchEvent(t *testing.T, chatID uuid.UUID, kind codersdk.ChatWatchEventKind) {
	t.Helper()
	// Watch events are published after the corresponding database update
	// commits, so poll instead of asserting on a single snapshot.
	testutil.Eventually(testutil.Context(t, testutil.WaitLong), t, func(_ context.Context) bool {
		for _, event := range f.pubsub.watchEvents(t) {
			if event.Kind == kind && event.Chat.ID == chatID {
				return true
			}
		}
		return false
	}, testutil.IntervalFast)
}

func (f *taskTestFixture) requireNoWatchEvents(t *testing.T) {
	t.Helper()
	require.Empty(t, f.pubsub.watchEvents(t))
}

func taskUserTextMessage(t *testing.T, text string, createdBy uuid.UUID, modelConfigID uuid.UUID, apiKeyID string) chatstate.Message {
	t.Helper()
	raw, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{codersdk.ChatMessageText(text)})
	require.NoError(t, err)
	return chatstate.Message{
		Role:           database.ChatMessageRoleUser,
		Content:        raw,
		Visibility:     database.ChatMessageVisibilityBoth,
		ContentVersion: chatprompt.CurrentContentVersion,
		CreatedBy:      uuid.NullUUID{UUID: createdBy, Valid: true},
		ModelConfigID:  uuid.NullUUID{UUID: modelConfigID, Valid: true},
		APIKeyID:       sql.NullString{String: apiKeyID, Valid: apiKeyID != ""},
	}
}

func taskAssistantToolCallMessage(t *testing.T, modelConfigID uuid.UUID, toolName string) chatstate.Message {
	t.Helper()
	raw, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{{
		Type:       codersdk.ChatMessagePartTypeToolCall,
		ToolCallID: "call_" + uuid.NewString(),
		ToolName:   toolName,
		Args:       json.RawMessage(`{}`),
	}})
	require.NoError(t, err)
	return chatstate.Message{
		Role:           database.ChatMessageRoleAssistant,
		Content:        raw,
		Visibility:     database.ChatMessageVisibilityBoth,
		ContentVersion: chatprompt.CurrentContentVersion,
		ModelConfigID:  uuid.NullUUID{UUID: modelConfigID, Valid: true},
	}
}

type taskPublishedEvent struct {
	channel string
	payload []byte
}

type taskRecordingPubsub struct {
	inner dbpubsub.Pubsub
	mu    sync.Mutex
	sent  []taskPublishedEvent
}

func newTaskRecordingPubsub(inner dbpubsub.Pubsub) *taskRecordingPubsub {
	return &taskRecordingPubsub{inner: inner}
}

func (p *taskRecordingPubsub) Publish(channel string, payload []byte) error {
	p.mu.Lock()
	p.sent = append(p.sent, taskPublishedEvent{channel: channel, payload: append([]byte(nil), payload...)})
	p.mu.Unlock()
	return p.inner.Publish(channel, payload)
}

func (p *taskRecordingPubsub) SubscribeWithErr(channel string, listener dbpubsub.ListenerWithErr) (func(), error) {
	return p.inner.SubscribeWithErr(channel, listener)
}

func (p *taskRecordingPubsub) clear() {
	p.mu.Lock()
	p.sent = nil
	p.mu.Unlock()
}

func (p *taskRecordingPubsub) events() []taskPublishedEvent {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]taskPublishedEvent(nil), p.sent...)
}

func (p *taskRecordingPubsub) watchEvents(t *testing.T) []codersdk.ChatWatchEvent {
	t.Helper()
	events := p.events()
	out := make([]codersdk.ChatWatchEvent, 0)
	for _, event := range events {
		var payload codersdk.ChatWatchEvent
		if err := json.Unmarshal(event.payload, &payload); err != nil {
			continue
		}
		if event.channel != coderdpubsub.ChatWatchEventChannel(payload.Chat.OwnerID) {
			continue
		}
		out = append(out, payload)
	}
	return out
}

func startRealTaskWorker(t *testing.T, f *taskTestFixture) *chatWorker {
	t.Helper()
	buffer := messagepartbuffer.New(messagepartbuffer.Options{})
	t.Cleanup(buffer.Close)
	worker, err := newChatWorker(newUnstartedServer(t, f.rawPS, f.db), chatWorkerOptions{
		WorkerID:                   uuid.New(),
		Store:                      f.db,
		Pubsub:                     f.pubsub,
		Logger:                     slog.Make(),
		MessagePartBuffer:          buffer,
		AcquisitionInterval:        time.Hour,
		AcquisitionBatchSize:       10,
		RunnerSyncInterval:         time.Hour,
		HeartbeatInterval:          time.Hour,
		HeartbeatCleanupInterval:   time.Hour,
		HeartbeatStaleSeconds:      30,
		StateChannelSize:           16,
		RunnerManagerChannelSize:   16,
		AcquisitionWakeChannelSize: 1,
		TaskRetryInitialBackoff:    time.Millisecond,
		TaskRetryMaxBackoff:        time.Millisecond,
	})
	require.NoError(t, err)
	require.NoError(t, worker.Start(context.Background()))
	t.Cleanup(func() { require.NoError(t, worker.Close()) })
	return worker
}

func waitOwnedChat(t *testing.T, f *taskTestFixture, chatID uuid.UUID, workerID uuid.UUID) database.Chat {
	t.Helper()
	var latest database.Chat
	testutil.Eventually(testutil.Context(t, testutil.WaitLong), t, func(ctx context.Context) bool {
		chat, err := f.db.GetChatByID(ctx, chatID)
		if err != nil {
			return false
		}
		latest = chat
		return chat.WorkerID.Valid && chat.WorkerID.UUID == workerID && chat.RunnerID.Valid
	}, testutil.IntervalFast)
	return latest
}

type taskSideEffectRecorder struct {
	mu         sync.Mutex
	hints      []runnerStateUpdate
	cleanups   []runnerKey
	interrupts []interruptionOutcome
}

func newTaskSideEffectRecorder() *taskSideEffectRecorder {
	return &taskSideEffectRecorder{}
}

func (r *taskSideEffectRecorder) routeStateHint(_ context.Context, state runnerStateUpdate) {
	r.mu.Lock()
	r.hints = append(r.hints, state)
	r.mu.Unlock()
}

func (r *taskSideEffectRecorder) requestCleanup(_ context.Context, key runnerKey) {
	r.mu.Lock()
	r.cleanups = append(r.cleanups, key)
	r.mu.Unlock()
}

func (r *taskSideEffectRecorder) afterInterruptionOutcome(_ context.Context, outcome interruptionOutcome) error {
	r.mu.Lock()
	r.interrupts = append(r.interrupts, outcome)
	r.mu.Unlock()
	return nil
}

func (r *taskSideEffectRecorder) requireStateHint(t *testing.T, chatID uuid.UUID, snapshot int64, status database.ChatStatus) {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, hint := range r.hints {
		if hint.ChatID == chatID && hint.SnapshotVersion == snapshot && hint.Status == status {
			return
		}
	}
	t.Fatalf("missing state hint chat_id=%s snapshot=%d status=%s hints=%v", chatID, snapshot, status, r.hints)
}

func (r *taskSideEffectRecorder) requireStateHintCount(t *testing.T, count int) {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	require.Len(t, r.hints, count)
}

func (r *taskSideEffectRecorder) requireCleanup(t *testing.T, chatID uuid.UUID, runnerID uuid.UUID) {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, cleanup := range r.cleanups {
		if cleanup.ChatID == chatID && cleanup.RunnerID == runnerID {
			return
		}
	}
	t.Fatalf("missing cleanup chat_id=%s runner_id=%s cleanups=%v", chatID, runnerID, r.cleanups)
}

func (r *taskSideEffectRecorder) requireCleanupCount(t *testing.T, count int) {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	require.Len(t, r.cleanups, count)
}

func (r *taskSideEffectRecorder) requireInterruptionOutcome(t *testing.T, chatID uuid.UUID, status database.ChatStatus) {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, outcome := range r.interrupts {
		if outcome.Chat.ID == chatID && outcome.Chat.Status == status {
			return
		}
	}
	t.Fatalf("missing interruption outcome chat_id=%s status=%s outcomes=%v", chatID, status, r.interrupts)
}

func newTestTaskStarter(t *testing.T, f *taskTestFixture, recorder *taskSideEffectRecorder) *taskStarter {
	t.Helper()
	buffer := messagepartbuffer.New(messagepartbuffer.Options{})
	t.Cleanup(buffer.Close)
	starter, err := newTaskStarter(newUnstartedServer(t, f.rawPS, f.db), chatWorkerOptions{
		Store:                   f.db,
		Pubsub:                  f.pubsub,
		Logger:                  slog.Make(),
		Clock:                   quartz.NewReal(),
		MessagePartBuffer:       buffer,
		TaskRetryInitialBackoff: time.Millisecond,
		TaskRetryMaxBackoff:     time.Millisecond,
	}, recorder.routeStateHint, recorder.requestCleanup)
	require.NoError(t, err)
	starter.afterInterruptionOutcome = recorder.afterInterruptionOutcome
	return starter
}

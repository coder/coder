//nolint:testpackage // These tests exercise package-private task seams.
package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
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
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/coderd/x/chatd/messagepartbuffer"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
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

func TestInterruptTask_PartialAssistantKeepsAttemptRuntime(t *testing.T) {
	t.Parallel()

	f := newTaskTestFixture(t)
	chat := f.createRunningChat(t)
	workerID := uuid.New()
	runnerID := uuid.New()
	acquired := f.acquireChat(t, chat.ID, workerID, runnerID)
	recorder := newTaskSideEffectRecorder()
	clock := quartz.NewMock(t)
	starter := newTestTaskStarterWithClock(t, f, recorder, clock)
	buffer := starter.opts.MessagePartBuffer
	key := messagepartbuffer.Key{
		ChatID:            chat.ID,
		HistoryVersion:    acquired.HistoryVersion,
		GenerationAttempt: acquired.GenerationAttempt,
	}
	require.NoError(t, buffer.CreateEpisode(key))
	// Prompt preparation and attempt bookkeeping run before the
	// provider stream opens and must not be billed.
	clock.Advance(3 * time.Second)
	require.NoError(t, buffer.StartModelInvocation(key))
	require.NoError(t, buffer.AddPart(key, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageText("partial answer")))
	clock.Advance(1500 * time.Millisecond)
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
	require.GreaterOrEqual(t, len(messages), 3)
	assistant := messages[len(messages)-2]
	require.Equal(t, database.ChatMessageRoleAssistant, assistant.Role)
	require.Equal(t, sql.NullInt64{Int64: 1500, Valid: true}, assistant.RuntimeMs)
}

func TestInterruptTask_PartialAssistantWithoutModelInvocationHasNoRuntime(t *testing.T) {
	t.Parallel()

	f := newTaskTestFixture(t)
	chat := f.createRunningChat(t)
	workerID := uuid.New()
	runnerID := uuid.New()
	acquired := f.acquireChat(t, chat.ID, workerID, runnerID)
	recorder := newTaskSideEffectRecorder()
	clock := quartz.NewMock(t)
	starter := newTestTaskStarterWithClock(t, f, recorder, clock)
	buffer := starter.opts.MessagePartBuffer
	key := messagepartbuffer.Key{
		ChatID:            chat.ID,
		HistoryVersion:    acquired.HistoryVersion,
		GenerationAttempt: acquired.GenerationAttempt,
	}
	require.NoError(t, buffer.CreateEpisode(key))
	require.NoError(t, buffer.AddPart(key, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageText("partial answer")))
	clock.Advance(1500 * time.Millisecond)
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
	require.GreaterOrEqual(t, len(messages), 3)
	assistant := messages[len(messages)-2]
	require.Equal(t, database.ChatMessageRoleAssistant, assistant.Role)
	require.False(t, assistant.RuntimeMs.Valid)
}

type interruptedBatch struct {
	chat     database.Chat
	starter  *taskStarter
	clock    *quartz.Mock
	key      messagepartbuffer.Key
	workerID uuid.UUID
	runnerID uuid.UUID
}

func interruptedBatchFixture(
	t *testing.T,
	f *taskTestFixture,
	calls []codersdk.ChatMessagePart,
) interruptedBatch {
	t.Helper()
	chat := f.createRunningChat(t)
	raw, err := chatprompt.MarshalParts(calls)
	require.NoError(t, err)
	machine := chatstate.NewChatMachine(f.db, f.pubsub, chat.ID)
	require.NoError(t, machine.Update(testutil.Context(t, testutil.WaitShort), func(tx *chatstate.Tx, _ database.Store) error {
		_, err := tx.CommitStep(chatstate.CommitStepInput{Messages: []chatstate.Message{{
			Role:           database.ChatMessageRoleAssistant,
			Content:        raw,
			Visibility:     database.ChatMessageVisibilityBoth,
			ContentVersion: chatprompt.CurrentContentVersion,
			ModelConfigID:  uuid.NullUUID{UUID: f.model.ID, Valid: true},
		}}})
		return err
	}))
	workerID := uuid.New()
	runnerID := uuid.New()
	acquired := f.acquireChat(t, chat.ID, workerID, runnerID)
	recorder := newTaskSideEffectRecorder()
	clock := quartz.NewMock(t)
	starter := newTestTaskStarterWithClock(t, f, recorder, clock)
	key := messagepartbuffer.Key{
		ChatID:            chat.ID,
		HistoryVersion:    acquired.HistoryVersion,
		GenerationAttempt: acquired.GenerationAttempt,
	}
	require.NoError(t, starter.opts.MessagePartBuffer.CreateEpisode(key))
	return interruptedBatch{
		chat:     chat,
		starter:  starter,
		clock:    clock,
		key:      key,
		workerID: workerID,
		runnerID: runnerID,
	}
}

func (b interruptedBatch) interrupt(t *testing.T, f *taskTestFixture) []database.ChatMessage {
	t.Helper()
	interrupting := f.interruptChat(t, b.chat.ID)
	err := b.starter.StartInterrupt(testutil.Context(t, testutil.WaitLong), chatWorkerTaskStartInput{
		ChatID:            b.chat.ID,
		WorkerID:          b.workerID,
		RunnerID:          b.runnerID,
		HistoryVersion:    interrupting.HistoryVersion,
		GenerationAttempt: interrupting.GenerationAttempt,
		Status:            database.ChatStatusInterrupting,
	})
	require.NoError(t, err)
	messages, err := f.db.GetChatMessagesByChatID(testutil.Context(t, testutil.WaitShort), database.GetChatMessagesByChatIDParams{ChatID: b.chat.ID})
	require.NoError(t, err)
	return messages
}

func findToolResultMessage(t *testing.T, messages []database.ChatMessage, toolCallID string) database.ChatMessage {
	t.Helper()
	for _, msg := range messages {
		if msg.Role != database.ChatMessageRoleTool {
			continue
		}
		parts, err := chatprompt.ParseContent(msg)
		require.NoError(t, err)
		for _, part := range parts {
			if part.Type == codersdk.ChatMessagePartTypeToolResult && part.ToolCallID == toolCallID {
				return msg
			}
		}
	}
	t.Fatalf("no tool result message for call %s", toolCallID)
	return database.ChatMessage{}
}

// batchUsageRecords returns the model-only tool batch usage rows for the
// chat. Cancellation rows are user-visible, so the dedicated usage record
// only appears in the model-visibility history.
func batchUsageRecords(t *testing.T, f *taskTestFixture, chatID uuid.UUID) []database.ChatMessage {
	t.Helper()
	rows, err := f.db.GetChatMessagesForPromptByChatID(testutil.Context(t, testutil.WaitShort), chatID)
	require.NoError(t, err)
	var records []database.ChatMessage
	for _, msg := range rows {
		if msg.Role != database.ChatMessageRoleTool || msg.Visibility != database.ChatMessageVisibilityModel {
			continue
		}
		records = append(records, msg)
	}
	return records
}

func requireSingleBatchUsageRecord(t *testing.T, f *taskTestFixture, chatID uuid.UUID, billedMs int64, billedCalls int) {
	t.Helper()
	records := batchUsageRecords(t, f, chatID)
	require.Len(t, records, 1)
	require.Equal(t, sql.NullInt64{Int64: billedMs, Valid: true}, records[0].RuntimeMs)
	parts, err := chatprompt.ParseContent(records[0])
	require.NoError(t, err)
	require.Len(t, parts, 1)
	require.Equal(t, toolBatchUsagePartType, parts[0].Type)
	require.JSONEq(t,
		fmt.Sprintf(`{"billed_ms":%d,"billed_calls":%d}`, billedMs, billedCalls),
		string(parts[0].Result),
	)
}

func TestInterruptTask_ToolBatchBillsPartialWindowOnUsageRecord(t *testing.T) {
	t.Parallel()

	f := newTaskTestFixture(t)
	execCallID := "call_" + uuid.NewString()
	waitCallID := "call_" + uuid.NewString()
	batch := interruptedBatchFixture(t, f, []codersdk.ChatMessagePart{
		{Type: codersdk.ChatMessagePartTypeToolCall, ToolCallID: execCallID, ToolName: "execute", Args: json.RawMessage(`{}`)},
		{Type: codersdk.ChatMessagePartTypeToolCall, ToolCallID: waitCallID, ToolName: "wait_agent", Args: json.RawMessage(`{}`)},
	})
	buffer := batch.starter.opts.MessagePartBuffer

	// Keep advances below the buffer's 15-second cleanup tick.
	batch.clock.Advance(2 * time.Second)
	require.NoError(t, buffer.RecordToolStart(batch.key, 0, batch.clock.Now()))
	require.NoError(t, buffer.RecordToolStart(batch.key, 1, batch.clock.Now()))
	// Record a live completion without publishing a tool result.
	batch.clock.Advance(3 * time.Second)
	require.NoError(t, buffer.RecordToolCompletion(batch.key, 0, batch.clock.Now()))
	batch.clock.Advance(5 * time.Second)

	messages := batch.interrupt(t, f)
	execRow := findToolResultMessage(t, messages, execCallID)
	require.False(t, execRow.RuntimeMs.Valid)
	waitRow := findToolResultMessage(t, messages, waitCallID)
	require.False(t, waitRow.RuntimeMs.Valid)
	requireSingleBatchUsageRecord(t, f, batch.chat.ID, 3_000, 1)
}

func TestInterruptTask_RunningBilledToolBillsUpToInterrupt(t *testing.T) {
	t.Parallel()

	f := newTaskTestFixture(t)
	execCallID := "call_" + uuid.NewString()
	batch := interruptedBatchFixture(t, f, []codersdk.ChatMessagePart{
		{Type: codersdk.ChatMessagePartTypeToolCall, ToolCallID: execCallID, ToolName: "execute", Args: json.RawMessage(`{}`)},
	})

	batch.clock.Advance(2 * time.Second)
	require.NoError(t, batch.starter.opts.MessagePartBuffer.RecordToolStart(batch.key, 0, batch.clock.Now()))
	batch.clock.Advance(7 * time.Second)

	messages := batch.interrupt(t, f)
	execRow := findToolResultMessage(t, messages, execCallID)
	require.False(t, execRow.RuntimeMs.Valid)
	requireSingleBatchUsageRecord(t, f, batch.chat.ID, 7_000, 1)
}

func TestInterruptTask_UnbilledOnlyBatchBillsNothingOnInterrupt(t *testing.T) {
	t.Parallel()

	f := newTaskTestFixture(t)
	waitCallID := "call_" + uuid.NewString()
	batch := interruptedBatchFixture(t, f, []codersdk.ChatMessagePart{
		{Type: codersdk.ChatMessagePartTypeToolCall, ToolCallID: waitCallID, ToolName: "wait_agent", Args: json.RawMessage(`{}`)},
	})

	batch.clock.Advance(2 * time.Second)
	require.NoError(t, batch.starter.opts.MessagePartBuffer.RecordToolStart(batch.key, 0, batch.clock.Now()))
	batch.clock.Advance(10 * time.Second)

	messages := batch.interrupt(t, f)
	waitRow := findToolResultMessage(t, messages, waitCallID)
	require.False(t, waitRow.RuntimeMs.Valid)
	require.Empty(t, batchUsageRecords(t, f, batch.chat.ID))
}

func TestInterruptTask_UnstartedSerialCallBillsNothingOnInterrupt(t *testing.T) {
	t.Parallel()

	f := newTaskTestFixture(t)
	waitCallID := "call_" + uuid.NewString()
	serialCallID := "call_" + uuid.NewString()
	batch := interruptedBatchFixture(t, f, []codersdk.ChatMessagePart{
		{Type: codersdk.ChatMessagePartTypeToolCall, ToolCallID: waitCallID, ToolName: "wait_agent", Args: json.RawMessage(`{}`)},
		{Type: codersdk.ChatMessagePartTypeToolCall, ToolCallID: serialCallID, ToolName: "execute", Args: json.RawMessage(`{}`)},
	})
	buffer := batch.starter.opts.MessagePartBuffer

	batch.clock.Advance(2 * time.Second)
	require.NoError(t, buffer.RecordToolStart(batch.key, 0, batch.clock.Now()))
	batch.clock.Advance(10 * time.Second)

	messages := batch.interrupt(t, f)
	for _, msg := range messages {
		if msg.Role == database.ChatMessageRoleTool {
			require.False(t, msg.RuntimeMs.Valid,
				"no cancellation row may bill: the billed call never began executing")
		}
	}
	require.Empty(t, batchUsageRecords(t, f, batch.chat.ID),
		"no usage record may bill: the billed call never began executing")
}

func TestInterruptTask_StartedSerialCallBillsFromItsOwnStart(t *testing.T) {
	t.Parallel()

	f := newTaskTestFixture(t)
	execCallID := "call_" + uuid.NewString()
	waitCallID := "call_" + uuid.NewString()
	serialCallID := "call_" + uuid.NewString()
	batch := interruptedBatchFixture(t, f, []codersdk.ChatMessagePart{
		{Type: codersdk.ChatMessagePartTypeToolCall, ToolCallID: execCallID, ToolName: "execute", Args: json.RawMessage(`{}`)},
		{Type: codersdk.ChatMessagePartTypeToolCall, ToolCallID: waitCallID, ToolName: "wait_agent", Args: json.RawMessage(`{}`)},
		{Type: codersdk.ChatMessagePartTypeToolCall, ToolCallID: serialCallID, ToolName: "execute", Args: json.RawMessage(`{}`)},
	})
	buffer := batch.starter.opts.MessagePartBuffer

	batch.clock.Advance(2 * time.Second)
	require.NoError(t, buffer.RecordToolStart(batch.key, 0, batch.clock.Now()))
	require.NoError(t, buffer.RecordToolStart(batch.key, 1, batch.clock.Now()))
	batch.clock.Advance(3 * time.Second)
	require.NoError(t, buffer.RecordToolCompletion(batch.key, 0, batch.clock.Now()))
	batch.clock.Advance(3 * time.Second)
	require.NoError(t, buffer.RecordToolCompletion(batch.key, 1, batch.clock.Now()))
	require.NoError(t, buffer.RecordToolStart(batch.key, 2, batch.clock.Now()))
	batch.clock.Advance(2 * time.Second)

	messages := batch.interrupt(t, f)
	serialRow := findToolResultMessage(t, messages, serialCallID)
	require.False(t, serialRow.RuntimeMs.Valid)
	execRow := findToolResultMessage(t, messages, execCallID)
	require.False(t, execRow.RuntimeMs.Valid)
	waitRow := findToolResultMessage(t, messages, waitCallID)
	require.False(t, waitRow.RuntimeMs.Valid)
	// Completed execute [2s,5s] plus the serial call's own window
	// [8s,10s]: the gap and the unbilled wait_agent do not count.
	requireSingleBatchUsageRecord(t, f, batch.chat.ID, 5_000, 2)
}

func TestInterruptTask_DuplicateCallIDsKeepOccurrenceStates(t *testing.T) {
	t.Parallel()

	f := newTaskTestFixture(t)
	dupCallID := "call_" + uuid.NewString()
	batch := interruptedBatchFixture(t, f, []codersdk.ChatMessagePart{
		{Type: codersdk.ChatMessagePartTypeToolCall, ToolCallID: dupCallID, ToolName: "execute", Args: json.RawMessage(`{}`)},
		{Type: codersdk.ChatMessagePartTypeToolCall, ToolCallID: dupCallID, ToolName: "execute", Args: json.RawMessage(`{}`)},
	})
	buffer := batch.starter.opts.MessagePartBuffer

	batch.clock.Advance(2 * time.Second)
	require.NoError(t, buffer.RecordToolStart(batch.key, 0, batch.clock.Now()))
	require.NoError(t, buffer.RecordToolStart(batch.key, 1, batch.clock.Now()))
	batch.clock.Advance(3 * time.Second)
	require.NoError(t, buffer.RecordToolCompletion(batch.key, 0, batch.clock.Now()))
	batch.clock.Advance(3 * time.Second)

	messages := batch.interrupt(t, f)
	for _, msg := range messages {
		if msg.Role == database.ChatMessageRoleTool {
			require.False(t, msg.RuntimeMs.Valid)
		}
	}
	// The still-running occurrence bills the full window exactly once.
	requireSingleBatchUsageRecord(t, f, batch.chat.ID, 6_000, 2)
}

func TestInterruptTask_DuplicateCallIDCompletionStampsOwnOccurrence(t *testing.T) {
	t.Parallel()

	f := newTaskTestFixture(t)
	dupCallID := "call_" + uuid.NewString()
	batch := interruptedBatchFixture(t, f, []codersdk.ChatMessagePart{
		{Type: codersdk.ChatMessagePartTypeToolCall, ToolCallID: dupCallID, ToolName: "execute", Args: json.RawMessage(`{}`)},
		{Type: codersdk.ChatMessagePartTypeToolCall, ToolCallID: dupCallID, ToolName: "wait_agent", Args: json.RawMessage(`{}`)},
	})
	buffer := batch.starter.opts.MessagePartBuffer

	batch.clock.Advance(2 * time.Second)
	require.NoError(t, buffer.RecordToolStart(batch.key, 0, batch.clock.Now()))
	require.NoError(t, buffer.RecordToolStart(batch.key, 1, batch.clock.Now()))
	batch.clock.Advance(3 * time.Second)
	require.NoError(t, buffer.RecordToolCompletion(batch.key, 1, batch.clock.Now()))
	batch.clock.Advance(3 * time.Second)

	messages := batch.interrupt(t, f)
	for _, msg := range messages {
		if msg.Role == database.ChatMessageRoleTool {
			require.False(t, msg.RuntimeMs.Valid)
		}
	}
	// The running execute occurrence bills to the interrupt; wait_agent's
	// early completion must not end it at 3s.
	requireSingleBatchUsageRecord(t, f, batch.chat.ID, 6_000, 1)
}

func TestInterruptTask_RejectedDuplicateIDDoesNotStealDispatchedOccurrence(t *testing.T) {
	t.Parallel()

	f := newTaskTestFixture(t)
	dupCallID := "call_" + uuid.NewString()
	batch := interruptedBatchFixture(t, f, []codersdk.ChatMessagePart{
		{Type: codersdk.ChatMessagePartTypeToolCall, ToolCallID: dupCallID, ToolName: "execute", Args: json.RawMessage(`{}`)},
		{Type: codersdk.ChatMessagePartTypeToolCall, ToolCallID: dupCallID, ToolName: "wait_agent", Args: json.RawMessage(`{}`)},
	})

	batch.clock.Advance(2 * time.Second)
	require.NoError(t, batch.starter.opts.MessagePartBuffer.RecordToolStart(batch.key, 1, batch.clock.Now()))
	batch.clock.Advance(10 * time.Second)

	messages := batch.interrupt(t, f)
	for _, msg := range messages {
		if msg.Role == database.ChatMessageRoleTool {
			require.False(t, msg.RuntimeMs.Valid)
		}
	}
	require.Empty(t, batchUsageRecords(t, f, batch.chat.ID),
		"no usage record may bill: only the unbilled wait_agent occurrence ran")
}

func TestInterruptTask_RejectedCallBillsNothingOnInterrupt(t *testing.T) {
	t.Parallel()

	f := newTaskTestFixture(t)
	execCallID := "call_" + uuid.NewString()
	waitCallID := "call_" + uuid.NewString()
	batch := interruptedBatchFixture(t, f, []codersdk.ChatMessagePart{
		{Type: codersdk.ChatMessagePartTypeToolCall, ToolCallID: execCallID, ToolName: "execute", Args: json.RawMessage(`{}`)},
		{Type: codersdk.ChatMessagePartTypeToolCall, ToolCallID: waitCallID, ToolName: "wait_agent", Args: json.RawMessage(`{}`)},
	})

	batch.clock.Advance(2 * time.Second)
	require.NoError(t, batch.starter.opts.MessagePartBuffer.RecordToolStart(batch.key, 1, batch.clock.Now()))
	batch.clock.Advance(10 * time.Second)

	messages := batch.interrupt(t, f)
	execRow := findToolResultMessage(t, messages, execCallID)
	require.False(t, execRow.RuntimeMs.Valid)
	waitRow := findToolResultMessage(t, messages, waitCallID)
	require.False(t, waitRow.RuntimeMs.Valid)
	require.Empty(t, batchUsageRecords(t, f, batch.chat.ID))
}

func TestInterruptTask_ToolCancellationWithoutLiveBatchHasNoRuntime(t *testing.T) {
	t.Parallel()

	f := newTaskTestFixture(t)
	execCallID := "call_" + uuid.NewString()
	batch := interruptedBatchFixture(t, f, []codersdk.ChatMessagePart{
		{Type: codersdk.ChatMessagePartTypeToolCall, ToolCallID: execCallID, ToolName: "execute", Args: json.RawMessage(`{}`)},
	})

	batch.clock.Advance(10 * time.Second)

	messages := batch.interrupt(t, f)
	execRow := findToolResultMessage(t, messages, execCallID)
	require.False(t, execRow.RuntimeMs.Valid)
	require.Empty(t, batchUsageRecords(t, f, batch.chat.ID))
}

// executeInterrupt is an interrupted batch whose chat is bound to a
// workspace agent that dials conn. The tool call message is backdated by
// an hour, so a tool call age of at least an hour comes from the
// database clock. No message part episode exists for the interrupt, as
// after an ownership change; createEpisode adds one.
type executeInterrupt struct {
	interruptedBatch
	messageID int64
	conn      *agentconnmock.MockAgentConn
	dials     *atomic.Int32
	releases  *atomic.Int32
}

// Execute arguments whose deadline an hour-old tool call is within, and
// past, and of a background call.
const (
	executeWithinDeadline = `{"command":"make test","timeout":"2h"}`
	executePastDeadline   = `{"command":"make test","timeout":"10m"}`
	executeBackground     = `{"command":"make dev","run_in_background":true}`
)

func newExecuteInterrupt(t *testing.T, f *taskTestFixture, calls []codersdk.ChatMessagePart, dialErr error) executeInterrupt {
	t.Helper()
	batch := interruptedBatchFixture(t, f, calls)
	ctx := testutil.Context(t, testutil.WaitShort)
	_, err := f.sqlDB.ExecContext(ctx,
		`UPDATE chat_messages SET created_at = created_at - interval '1 hour' WHERE chat_id = $1 AND role = 'assistant'`,
		batch.chat.ID)
	require.NoError(t, err)
	messages, err := f.db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: batch.chat.ID})
	require.NoError(t, err)
	assistant := messages[len(messages)-1]
	require.Equal(t, database.ChatMessageRoleAssistant, assistant.Role)
	// Updating a message moves the chat's history version and generation
	// attempt, so the interrupt reads a new episode.
	chat, err := f.db.GetChatByID(ctx, batch.chat.ID)
	require.NoError(t, err)
	batch.key = messagepartbuffer.Key{
		ChatID:            chat.ID,
		HistoryVersion:    chat.HistoryVersion,
		GenerationAttempt: chat.GenerationAttempt,
	}

	workspace, build, agent := seedWorkspaceBinding(t, f.db, f.user.ID)
	_, err = f.db.UpdateChatWorkspaceBinding(ctx, database.UpdateChatWorkspaceBindingParams{
		ID:          batch.chat.ID,
		WorkspaceID: uuid.NullUUID{UUID: workspace.ID, Valid: true},
		BuildID:     uuid.NullUUID{UUID: build.ID, Valid: true},
		AgentID:     uuid.NullUUID{UUID: agent.ID, Valid: true},
	})
	require.NoError(t, err)

	conn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
	conn.EXPECT().SetExtraHeaders(gomock.Any()).AnyTimes()
	var dials, releases atomic.Int32
	batch.starter.server.agentConnFn = func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
		dials.Add(1)
		assert.Equal(t, agent.ID, agentID)
		if dialErr != nil {
			return nil, nil, dialErr
		}
		return conn, func() { releases.Add(1) }, nil
	}
	return executeInterrupt{
		interruptedBatch: batch,
		messageID:        assistant.ID,
		conn:             conn,
		dials:            &dials,
		releases:         &releases,
	}
}

// createEpisode creates the interrupted generation attempt's episode, as
// on the replica that ran it.
func (e executeInterrupt) createEpisode(t *testing.T) {
	t.Helper()
	require.NoError(t, e.starter.opts.MessagePartBuffer.CreateEpisode(e.key))
}

func (e executeInterrupt) processID(callID string) string {
	return workspacesdk.ToolCallUUID(e.chat.ID, e.messageID, callID).String()
}

func (e executeInterrupt) startInput(t *testing.T, f *taskTestFixture) chatWorkerTaskStartInput {
	t.Helper()
	interrupting := f.interruptChat(t, e.chat.ID)
	return chatWorkerTaskStartInput{
		ChatID:            e.chat.ID,
		WorkerID:          e.workerID,
		RunnerID:          e.runnerID,
		HistoryVersion:    interrupting.HistoryVersion,
		GenerationAttempt: interrupting.GenerationAttempt,
		Status:            database.ChatStatusInterrupting,
	}
}

// expectCancel expects one cancel request for callID that carries the
// tool call and returns resp and err.
func (e executeInterrupt) expectCancel(t *testing.T, callID string, resp workspacesdk.CancelProcessResponse, err error) {
	t.Helper()
	e.conn.EXPECT().CancelProcess(gomock.Any(), e.processID(callID)).
		DoAndReturn(func(ctx context.Context, _ string) (workspacesdk.CancelProcessResponse, error) {
			e.assertToolCall(ctx, t, callID)
			return resp, err
		})
}

func (e executeInterrupt) assertToolCall(ctx context.Context, t *testing.T, callID string) {
	tc, ok := workspacesdk.ToolCallFromContext(ctx)
	if assert.True(t, ok, "cancel request must carry the tool call") {
		assert.Equal(t, e.messageID, tc.MessageID)
		assert.Equal(t, callID, tc.ID)
		assert.GreaterOrEqual(t, tc.Age, time.Hour)
	}
}

func executeToolCall(callID, args string) codersdk.ChatMessagePart {
	return codersdk.ChatMessagePart{
		Type:       codersdk.ChatMessagePartTypeToolCall,
		ToolCallID: callID,
		ToolName:   chattool.ExecuteToolName,
		Args:       json.RawMessage(args),
	}
}

// agentTransportError is the error shape agent requests return when the
// request got no response.
func agentTransportError(err error) error {
	return &url.Error{Op: http.MethodPost, URL: "http://agent/api/v0/processes", Err: err}
}

func toolResults(t *testing.T, messages []database.ChatMessage, callID string) []codersdk.ChatMessagePart {
	t.Helper()
	var results []codersdk.ChatMessagePart
	for _, msg := range messages {
		if msg.Role != database.ChatMessageRoleTool {
			continue
		}
		parts, err := chatprompt.ParseContent(msg)
		require.NoError(t, err)
		for _, part := range parts {
			if part.Type == codersdk.ChatMessagePartTypeToolResult && part.ToolCallID == callID {
				results = append(results, part)
			}
		}
	}
	return results
}

// singleToolResult returns the only committed result for callID.
func singleToolResult(t *testing.T, messages []database.ChatMessage, callID string) codersdk.ChatMessagePart {
	t.Helper()
	results := toolResults(t, messages, callID)
	require.Len(t, results, 1, "tool call %s must have exactly one result", callID)
	return results[0]
}

func requireGenericInterruptResult(t *testing.T, part codersdk.ChatMessagePart) {
	t.Helper()
	require.True(t, part.IsError)
	require.JSONEq(t, fmt.Sprintf(`{"error":%q}`, interruptedToolResultErrorMessage), string(part.Result))
}

func requireExecuteResult(t *testing.T, part codersdk.ChatMessagePart) chattool.ExecuteResult {
	t.Helper()
	// The model sees every field only when the result is not an error
	// result, as for every execute result.
	require.False(t, part.IsError)
	var result chattool.ExecuteResult
	require.NoError(t, json.Unmarshal(part.Result, &result), string(part.Result))
	return result
}

// TestInterruptTask_ExecuteResults runs without a message part episode,
// as after an ownership change, so only the execute deadline decides
// whether a foreground call is canceled.
func TestInterruptTask_ExecuteResults(t *testing.T) {
	t.Parallel()

	intPtr := func(v int) *int { return &v }
	tests := []struct {
		name string
		args string
		// snapshot is set when a non-blocking output request is expected,
		// answered with output and outputErr.
		snapshot  bool
		output    workspacesdk.ProcessOutputResponse
		outputErr error
		// noCancel is set when no cancel request is expected.
		noCancel bool
		resp     workspacesdk.CancelProcessResponse
		err      error
		dialErr  error
		// generic is set when the call keeps today's interrupted result.
		generic bool
		// want is compared without Error, which must contain wantError.
		want      chattool.ExecuteResult
		wantError string
		// wantUUID is set when Error must name the tool call UUID.
		wantUUID bool
		// wantBackground is set when BackgroundProcessID must be the tool
		// call UUID.
		wantBackground bool
	}{
		{
			name:      "CanceledAfterStart",
			args:      executeWithinDeadline,
			resp:      workspacesdk.CancelProcessResponse{Started: true, Canceled: true, Output: "partial", ExitCode: intPtr(137), AgeMs: 12_000},
			want:      chattool.ExecuteResult{Output: "partial", ExitCode: 137, WallDurationMs: 12_000},
			wantError: "canceled by the user after 12s.",
		},
		{
			name:      "CanceledWithoutExitCode",
			args:      executeWithinDeadline,
			resp:      workspacesdk.CancelProcessResponse{Started: true, Canceled: true, Output: "partial", AgeMs: 1_500},
			want:      chattool.ExecuteResult{Output: "partial", ExitCode: -1, WallDurationMs: 1_500},
			wantError: "canceled by the user after 1.5s.",
		},
		{
			name: "AlreadyExited",
			args: executeWithinDeadline,
			resp: workspacesdk.CancelProcessResponse{Started: true, Output: "PASS", ExitCode: intPtr(0), AgeMs: 3_000},
			want: chattool.ExecuteResult{Success: true, Output: "PASS", ExitCode: 0, WallDurationMs: 3_000},
		},
		{
			name: "AlreadyExitedWithFailure",
			args: executeWithinDeadline,
			resp: workspacesdk.CancelProcessResponse{Started: true, Output: "FAIL", ExitCode: intPtr(2), AgeMs: 3_000},
			want: chattool.ExecuteResult{Output: "FAIL", ExitCode: 2, WallDurationMs: 3_000},
		},
		{
			name:      "NotStarted",
			args:      executeWithinDeadline,
			resp:      workspacesdk.CancelProcessResponse{},
			wantError: "not run: the command was canceled before the workspace agent received it, or the agent failed to start it.",
		},
		{
			name: "AgentStartedAfterToolCall",
			args: executeWithinDeadline,
			err: &workspacesdk.ToolCallError{
				Response: codersdk.Response{Message: "agent started after the tool call"},
				Code:     workspacesdk.ToolCallErrorAgentStartedAfterToolCall,
			},
			wantError: "outcome unknown: the workspace agent restarted after this tool call",
		},
		{
			name:    "OldAgentWithoutCancelRoute",
			args:    executeWithinDeadline,
			err:     codersdk.NewTestError(http.StatusNotFound, http.MethodPost, "/api/v0/processes/x/cancel"),
			generic: true,
		},
		{
			name:    "OtherErrorResponse",
			args:    executeWithinDeadline,
			err:     codersdk.NewTestError(http.StatusInternalServerError, http.MethodPost, "/api/v0/processes/x/cancel"),
			generic: true,
		},
		{
			name: "StaleToolCall",
			args: executeWithinDeadline,
			err: &workspacesdk.ToolCallError{
				Response: codersdk.Response{Message: "stale"},
				Code:     workspacesdk.ToolCallErrorStale,
			},
			generic: true,
		},
		{
			name:      "TransportError",
			args:      executeWithinDeadline,
			err:       agentTransportError(xerrors.New("connection reset by peer")),
			wantError: "outcome unknown: the workspace agent could not be reached",
			wantUUID:  true,
		},
		{
			name:      "UnreadableResponse",
			args:      executeWithinDeadline,
			err:       xerrors.New("unexpected EOF"),
			wantError: "outcome unknown: the workspace agent answered the cancel request, but its response could not be read",
			wantUUID:  true,
		},
		{
			name:      "NoConnection",
			args:      executeWithinDeadline,
			dialErr:   xerrors.New("dial failed"),
			noCancel:  true,
			wantError: "outcome unknown: the workspace agent could not be reached",
			wantUUID:  true,
		},
		{
			name:           "PastDeadlineStillRunning",
			args:           executePastDeadline,
			snapshot:       true,
			output:         workspacesdk.ProcessOutputResponse{Running: true, Output: "still going", AgeMs: 11 * 60_000},
			noCancel:       true,
			want:           chattool.ExecuteResult{Output: "still going", ExitCode: -1, WallDurationMs: 11 * 60_000},
			wantError:      "command timed out after 10m0s",
			wantBackground: true,
		},
		{
			// The process started late, so its own deadline is still ahead.
			name:      "PastDeadlineRunningWithinProcessDeadline",
			args:      executePastDeadline,
			snapshot:  true,
			output:    workspacesdk.ProcessOutputResponse{Running: true, Output: "partial", AgeMs: 5 * 60_000},
			resp:      workspacesdk.CancelProcessResponse{Started: true, Canceled: true, Output: "partial", ExitCode: intPtr(137), AgeMs: 5 * 60_000},
			want:      chattool.ExecuteResult{Output: "partial", ExitCode: 137, WallDurationMs: 5 * 60_000},
			wantError: "canceled by the user after 5m0s.",
		},
		{
			name:     "PastDeadlineExited",
			args:     executePastDeadline,
			snapshot: true,
			output:   workspacesdk.ProcessOutputResponse{Output: "PASS", ExitCode: intPtr(0), AgeMs: 3_000},
			resp:     workspacesdk.CancelProcessResponse{Started: true, Output: "PASS", ExitCode: intPtr(0), AgeMs: 3_000},
			want:     chattool.ExecuteResult{Success: true, Output: "PASS", ExitCode: 0, WallDurationMs: 3_000},
		},
		{
			// The snapshot would keep the process running if the error were
			// ignored; without an answer it is left running either way.
			name:      "PastDeadlineOutputError",
			args:      executePastDeadline,
			snapshot:  true,
			output:    workspacesdk.ProcessOutputResponse{Running: true, AgeMs: 11 * 60_000},
			outputErr: agentTransportError(xerrors.New("connection reset by peer")),
			noCancel:  true,
			wantError: "outcome unknown: the workspace agent could not be reached to check whether the command is past its timeout, so it was not canceled",
			wantUUID:  true,
		},
		{
			name:      "PastDeadlineNotFound",
			args:      executePastDeadline,
			snapshot:  true,
			outputErr: codersdk.NewTestError(http.StatusNotFound, http.MethodGet, "/api/v0/processes/x/output"),
			err:       codersdk.NewTestError(http.StatusNotFound, http.MethodPost, "/api/v0/processes/x/cancel"),
			generic:   true,
		},
		{
			name:           "BackgroundFound",
			args:           executeBackground,
			snapshot:       true,
			output:         workspacesdk.ProcessOutputResponse{Running: true, AgeMs: 2 * 60_000},
			noCancel:       true,
			want:           chattool.ExecuteResult{Success: true, Backgrounded: true},
			wantBackground: true,
		},
		{
			name:           "BackgroundTrailingAmpersandFound",
			args:           `{"command":"make dev &"}`,
			snapshot:       true,
			output:         workspacesdk.ProcessOutputResponse{Running: true},
			noCancel:       true,
			want:           chattool.ExecuteResult{Success: true, Backgrounded: true},
			wantBackground: true,
		},
		{
			// Also what an agent without tool call support answers, since it
			// picked another process ID.
			name:      "BackgroundNotFound",
			args:      executeBackground,
			snapshot:  true,
			outputErr: codersdk.NewTestError(http.StatusNotFound, http.MethodGet, "/api/v0/processes/x/output"),
			noCancel:  true,
			generic:   true,
		},
		{
			name:      "BackgroundOutputError",
			args:      executeBackground,
			snapshot:  true,
			outputErr: agentTransportError(xerrors.New("connection reset by peer")),
			noCancel:  true,
			generic:   true,
		},
		{
			name:     "BackgroundNoConnection",
			args:     executeBackground,
			dialErr:  xerrors.New("dial failed"),
			noCancel: true,
			generic:  true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			f := newTaskTestFixture(t)
			callID := "call_" + uuid.NewString()
			e := newExecuteInterrupt(t, f, []codersdk.ChatMessagePart{executeToolCall(callID, tc.args)}, tc.dialErr)
			if tc.snapshot {
				e.conn.EXPECT().ProcessOutput(gomock.Any(), e.processID(callID), nil).Return(tc.output, tc.outputErr)
			}
			if !tc.noCancel {
				e.expectCancel(t, callID, tc.resp, tc.err)
			}

			part := singleToolResult(t, e.interrupt(t, f), callID)
			if tc.dialErr == nil {
				assert.EqualValues(t, 1, e.dials.Load())
				assert.EqualValues(t, 1, e.releases.Load(), "the connection must be released")
			} else {
				// A failed dial is retried once after validating the agent.
				assert.Positive(t, e.dials.Load())
			}
			if tc.generic {
				requireGenericInterruptResult(t, part)
				return
			}
			result := requireExecuteResult(t, part)
			assert.Contains(t, result.Error, tc.wantError)
			if tc.wantError == "" {
				assert.Empty(t, result.Error)
			}
			if tc.wantUUID {
				assert.Contains(t, result.Error, e.processID(callID))
			}
			want := tc.want
			if tc.wantBackground {
				want.BackgroundProcessID = e.processID(callID)
			}
			result.Error = ""
			assert.Equal(t, want, result)
		})
	}
}

// TestInterruptTask_ExecuteCallsWithoutCancel covers unresolved calls the
// interrupt must not cancel: they keep today's result and no agent
// connection is made.
func TestInterruptTask_ExecuteCallsWithoutCancel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		calls func(callID string) []codersdk.ChatMessagePart
	}{
		{
			name: "UnparsableArgs",
			calls: func(callID string) []codersdk.ChatMessagePart {
				return []codersdk.ChatMessagePart{executeToolCall(callID, `{"command":1}`)}
			},
		},
		{
			// The tool rejects the timeout before starting a process.
			name: "InvalidTimeout",
			calls: func(callID string) []codersdk.ChatMessagePart {
				return []codersdk.ChatMessagePart{executeToolCall(callID, `{"command":"make test","timeout":"soon"}`)}
			},
		},
		{
			name: "NonExecuteTool",
			calls: func(callID string) []codersdk.ChatMessagePart {
				return []codersdk.ChatMessagePart{{Type: codersdk.ChatMessagePartTypeToolCall, ToolCallID: callID, ToolName: "wait_agent", Args: json.RawMessage(`{}`)}}
			},
		},
		{
			// Both calls share one tool call UUID, and canceling it would
			// kill the background process.
			name: "DuplicateIDOnlyOneQualifies",
			calls: func(callID string) []codersdk.ChatMessagePart {
				return []codersdk.ChatMessagePart{
					executeToolCall(callID, executeWithinDeadline),
					executeToolCall(callID, executeBackground),
				}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			f := newTaskTestFixture(t)
			callID := "call_" + uuid.NewString()
			calls := tc.calls(callID)
			e := newExecuteInterrupt(t, f, calls, nil)

			results := toolResults(t, e.interrupt(t, f), callID)
			require.Len(t, results, len(calls))
			for _, result := range results {
				requireGenericInterruptResult(t, result)
			}
			assert.Zero(t, e.dials.Load())
		})
	}
}

// TestInterruptTask_CancelIgnoresEpisodeBuffer covers the usual interrupt:
// the canceled generation goroutine records the call's completion and
// publishes its result before interrupt handling reads the episode. The
// call is still canceled and the cancel answer is its only result.
func TestInterruptTask_CancelIgnoresEpisodeBuffer(t *testing.T) {
	t.Parallel()

	f := newTaskTestFixture(t)
	callID := "call_" + uuid.NewString()
	e := newExecuteInterrupt(t, f, []codersdk.ChatMessagePart{executeToolCall(callID, executeWithinDeadline)}, nil)
	e.createEpisode(t)
	buffer := e.starter.opts.MessagePartBuffer
	require.NoError(t, buffer.RecordToolStart(e.key, 0, e.clock.Now()))
	require.NoError(t, buffer.RecordToolCompletion(e.key, 0, e.clock.Now()))
	canceledResult := codersdk.ChatMessageToolResult(callID, chattool.ExecuteToolName, json.RawMessage(`{"error":"context canceled"}`), false, false)
	require.NoError(t, buffer.AddPart(e.key, codersdk.ChatMessageRoleTool, canceledResult))
	e.expectCancel(t, callID, workspacesdk.CancelProcessResponse{Started: true, Canceled: true, Output: "partial", AgeMs: 2_000}, nil)

	result := requireExecuteResult(t, singleToolResult(t, e.interrupt(t, f), callID))
	assert.Equal(t, "partial", result.Output)
	assert.Contains(t, result.Error, "canceled by the user after 2s.")
}

// TestInterruptTask_CancelsExecuteCallsInParallel requires both cancel
// requests to be in flight at once, and keeps billing and the results of
// other calls in the batch as they are today.
func TestInterruptTask_CancelsExecuteCallsInParallel(t *testing.T) {
	t.Parallel()

	f := newTaskTestFixture(t)
	firstCallID := "call_" + uuid.NewString()
	secondCallID := "call_" + uuid.NewString()
	waitCallID := "call_" + uuid.NewString()
	e := newExecuteInterrupt(t, f, []codersdk.ChatMessagePart{
		executeToolCall(firstCallID, executeWithinDeadline),
		executeToolCall(secondCallID, executeWithinDeadline),
		{Type: codersdk.ChatMessagePartTypeToolCall, ToolCallID: waitCallID, ToolName: "wait_agent", Args: json.RawMessage(`{}`)},
	}, nil)
	e.createEpisode(t)
	buffer := e.starter.opts.MessagePartBuffer
	e.clock.Advance(2 * time.Second)
	require.NoError(t, buffer.RecordToolStart(e.key, 0, e.clock.Now()))
	require.NoError(t, buffer.RecordToolStart(e.key, 1, e.clock.Now()))
	require.NoError(t, buffer.RecordToolStart(e.key, 2, e.clock.Now()))
	e.clock.Advance(5 * time.Second)

	testCtx := testutil.Context(t, testutil.WaitShort)
	bothArrived := make(chan struct{})
	var arrivals atomic.Int32
	cancel := func(callID string, output string) func(context.Context, string) (workspacesdk.CancelProcessResponse, error) {
		return func(ctx context.Context, _ string) (workspacesdk.CancelProcessResponse, error) {
			e.assertToolCall(ctx, t, callID)
			if arrivals.Add(1) == 2 {
				close(bothArrived)
			}
			select {
			case <-bothArrived:
			case <-testCtx.Done():
				return workspacesdk.CancelProcessResponse{}, xerrors.New("the other cancel request never arrived")
			}
			return workspacesdk.CancelProcessResponse{Started: true, Canceled: true, Output: output, AgeMs: 5_000}, nil
		}
	}
	e.conn.EXPECT().CancelProcess(gomock.Any(), e.processID(firstCallID)).DoAndReturn(cancel(firstCallID, "first"))
	e.conn.EXPECT().CancelProcess(gomock.Any(), e.processID(secondCallID)).DoAndReturn(cancel(secondCallID, "second"))

	messages := e.interrupt(t, f)
	for callID, output := range map[string]string{firstCallID: "first", secondCallID: "second"} {
		result := requireExecuteResult(t, singleToolResult(t, messages, callID))
		assert.Equal(t, output, result.Output)
		assert.Contains(t, result.Error, "canceled by the user after 5s.")
	}
	requireGenericInterruptResult(t, singleToolResult(t, messages, waitCallID))
	assert.EqualValues(t, 1, e.dials.Load())
	// Both execute calls ran [2s,7s]; wait_agent is unbilled.
	requireSingleBatchUsageRecord(t, f, e.chat.ID, 5_000, 2)
}

// TestInterruptTask_CancelBoundYieldsUnknown shows that an agent that
// never answers yields an unknown result once the bound passes.
func TestInterruptTask_CancelBoundYieldsUnknown(t *testing.T) {
	t.Parallel()

	f := newTaskTestFixture(t)
	callID := "call_" + uuid.NewString()
	e := newExecuteInterrupt(t, f, []codersdk.ChatMessagePart{executeToolCall(callID, executeWithinDeadline)}, nil)
	ctx := testutil.Context(t, testutil.WaitLong)
	// Move the message part buffer off the mock clock, so the bound's
	// timer is the only event on it and one Advance reaches it. The
	// buffer's cleanup ticker is the clock's only ticker, and it stops
	// without tags.
	tickerStop := e.clock.Trap().TickerStop()
	e.starter.opts.MessagePartBuffer.Close()
	tickerStop.MustWait(ctx).MustRelease(ctx)
	tickerStop.Close()
	buffer := messagepartbuffer.New(messagepartbuffer.Options{})
	t.Cleanup(buffer.Close)
	e.starter.opts.MessagePartBuffer = buffer

	bound := e.clock.Trap().AfterFunc("chatworker", "interrupt_cancel")
	defer bound.Close()
	arrived := make(chan struct{})
	e.conn.EXPECT().CancelProcess(gomock.Any(), e.processID(callID)).
		DoAndReturn(func(ctx context.Context, _ string) (workspacesdk.CancelProcessResponse, error) {
			close(arrived)
			<-ctx.Done()
			return workspacesdk.CancelProcessResponse{}, agentTransportError(ctx.Err())
		})

	advanced := make(chan struct{})
	go func() {
		defer close(advanced)
		call, err := bound.Wait(ctx)
		if !assert.NoError(t, err) {
			return
		}
		assert.Equal(t, interruptCancelTimeout, call.Duration)
		if !assert.NoError(t, call.Release(ctx)) {
			return
		}
		select {
		case <-arrived:
		case <-ctx.Done():
			return
		}
		assert.NoError(t, e.clock.Advance(interruptCancelTimeout).Wait(ctx))
	}()

	result := requireExecuteResult(t, singleToolResult(t, e.interrupt(t, f), callID))
	<-advanced
	assert.Contains(t, result.Error, "outcome unknown: the workspace agent could not be reached")
	assert.Contains(t, result.Error, e.processID(callID))
}

// TestInterruptTask_ParentCanceledDuringCancelCommitsNothing ends the
// interrupt task while its cancel request is in flight.
func TestInterruptTask_ParentCanceledDuringCancelCommitsNothing(t *testing.T) {
	t.Parallel()

	f := newTaskTestFixture(t)
	callID := "call_" + uuid.NewString()
	e := newExecuteInterrupt(t, f, []codersdk.ChatMessagePart{executeToolCall(callID, executeWithinDeadline)}, nil)
	input := e.startInput(t, f)
	ctx, cancel := context.WithCancel(testutil.Context(t, testutil.WaitLong))
	defer cancel()
	e.conn.EXPECT().CancelProcess(gomock.Any(), e.processID(callID)).
		DoAndReturn(func(reqCtx context.Context, _ string) (workspacesdk.CancelProcessResponse, error) {
			cancel()
			<-reqCtx.Done()
			return workspacesdk.CancelProcessResponse{}, agentTransportError(reqCtx.Err())
		})

	err := e.starter.StartInterrupt(ctx, input)
	require.ErrorIs(t, err, errTaskExpectedExit)

	checkCtx := testutil.Context(t, testutil.WaitShort)
	messages, err := f.db.GetChatMessagesByChatID(checkCtx, database.GetChatMessagesByChatIDParams{ChatID: e.chat.ID})
	require.NoError(t, err)
	assert.Empty(t, toolResults(t, messages, callID))
	chat, err := f.db.GetChatByID(checkCtx, e.chat.ID)
	require.NoError(t, err)
	assert.Equal(t, database.ChatStatusInterrupting, chat.Status)
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
	waitOwnedChat(t, f, chat.ID, worker.opts.WorkerID)

	f.interruptChat(t, chat.ID)
	testutil.Eventually(testutil.Context(t, testutil.WaitLong), t, func(ctx context.Context) bool {
		latest, err := f.db.GetChatByID(ctx, chat.ID)
		return err == nil && latest.Status == database.ChatStatusRunning
	}, testutil.IntervalFast)
	latest, err := f.db.GetChatByID(testutil.Context(t, testutil.WaitShort), chat.ID)
	require.NoError(t, err)
	require.Equal(t, worker.opts.WorkerID, latest.WorkerID.UUID)
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
		return err == nil && latest.Status == database.ChatStatusRunning && latest.WorkerID.Valid && latest.WorkerID.UUID == worker.opts.WorkerID
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
	waitOwnedChat(t, f, chat.ID, worker.opts.WorkerID)

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
	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
		IsDefault:      true,
		OrganizationID: org.ID,
	})
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
	f.pubsub.clear()
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
	return newTestTaskStarterWithClock(t, f, recorder, quartz.NewReal())
}

// newTestTaskStarterWithClock shares the clock between the starter and its
// message part buffer, mirroring production wiring.
func newTestTaskStarterWithClock(t *testing.T, f *taskTestFixture, recorder *taskSideEffectRecorder, clock quartz.Clock) *taskStarter {
	t.Helper()
	buffer := messagepartbuffer.New(messagepartbuffer.Options{Clock: clock})
	t.Cleanup(buffer.Close)
	starter, err := newTaskStarter(newUnstartedServer(t, f.rawPS, f.db), chatWorkerOptions{
		Store:                   f.db,
		Pubsub:                  f.pubsub,
		Logger:                  slog.Make(),
		Clock:                   clock,
		MessagePartBuffer:       buffer,
		TaskRetryInitialBackoff: time.Millisecond,
		TaskRetryMaxBackoff:     time.Millisecond,
	}, recorder.routeStateHint, recorder.requestCleanup)
	require.NoError(t, err)
	starter.afterInterruptionOutcome = recorder.afterInterruptionOutcome
	return starter
}

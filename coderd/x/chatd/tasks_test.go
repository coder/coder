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

	recorder.requireInterruptionOutcome(t, chat.ID, database.ChatStatusRunning)

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

	f.requireNoWatchEvents(t)
}

func TestRunnerManager_Release(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"Waiting", "Error", "Archived", "OwnershipMismatch", "MissingChat", "HistoryChanged", "QueueChanged", "StatusChanged", "ArchiveChanged", "Runnable", "Canceled"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			f := newTaskTestFixture(t)
			chat := f.createRunningChat(t)
			workerID, runnerID := uuid.New(), uuid.New()
			f.acquireChat(t, chat.ID, workerID, runnerID)
			status := database.ChatStatusWaiting
			archived := false
			switch mode {
			case "Error":
				status = database.ChatStatusError
			case "Archived":
				status, archived = database.ChatStatusRunning, true
			case "Runnable":
				status = database.ChatStatusRunning
			}
			idle := f.forceExecutionState(t, chat.ID, status, archived, sql.NullTime{})
			state := stateUpdateFromChat(idle)
			wantReleased := true
			switch mode {
			case "OwnershipMismatch":
				f.acquireChat(t, chat.ID, uuid.New(), uuid.New())
			case "MissingChat":
				_, err := f.sqlDB.ExecContext(ctx, `DELETE FROM chats WHERE id = $1`, chat.ID)
				require.NoError(t, err)
			case "HistoryChanged":
				_, err := f.sqlDB.ExecContext(ctx, `UPDATE chat_messages SET content = '[{"type":"text","text":"edited"}]'::jsonb WHERE chat_id = $1 AND role = 'user'`, chat.ID)
				require.NoError(t, err)
				wantReleased = false
			case "QueueChanged":
				_, err := f.sqlDB.ExecContext(ctx, `UPDATE chats SET queue_version = queue_version + 1 WHERE id = $1`, chat.ID)
				require.NoError(t, err)
				wantReleased = false
			case "StatusChanged":
				f.forceExecutionState(t, chat.ID, database.ChatStatusInterrupting, false, sql.NullTime{})
				wantReleased = false
			case "ArchiveChanged":
				f.forceExecutionState(t, chat.ID, status, true, sql.NullTime{})
				wantReleased = false
			case "Runnable", "Canceled":
				wantReleased = false
			}
			before, beforeErr := f.db.GetChatByID(ctx, chat.ID)
			if mode != "MissingChat" {
				require.NoError(t, beforeErr)
			}
			manager := newRunnerManager(ctx, newUnstartedServer(t, f.rawPS, f.db), chatWorkerOptions{
				Store: f.db, Pubsub: f.pubsub, Logger: testutil.Logger(t),
			})
			rec := &runnerRecord{key: runnerKey{ChatID: chat.ID, RunnerID: runnerID}, workerID: workerID}
			releaseCtx := ctx
			if mode == "Canceled" {
				var cancel context.CancelFunc
				releaseCtx, cancel = context.WithCancel(ctx)
				cancel()
			}
			released, err := manager.release(releaseCtx, rec, state)
			if mode == "Canceled" {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, wantReleased, released)
			latest, err := f.db.GetChatByID(ctx, chat.ID)
			if mode == "MissingChat" {
				require.ErrorIs(t, err, sql.ErrNoRows)
				return
			}
			require.NoError(t, err)
			if !wantReleased || mode == "OwnershipMismatch" {
				require.Equal(t, before.WorkerID, latest.WorkerID)
				require.Equal(t, before.RunnerID, latest.RunnerID)
				require.Equal(t, before.SnapshotVersion, latest.SnapshotVersion, "skipped release must roll back its snapshot bump")
			} else {
				require.False(t, latest.WorkerID.Valid)
				require.False(t, latest.RunnerID.Valid)
			}
			f.requireNoWatchEvents(t)
		})
	}
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

func TestRunner_ReleasesIdleChat(t *testing.T) {
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
	interrupts []interruptionOutcome
}

func newTaskSideEffectRecorder() *taskSideEffectRecorder {
	return &taskSideEffectRecorder{}
}

func (r *taskSideEffectRecorder) afterInterruptionOutcome(_ context.Context, outcome interruptionOutcome) error {
	r.mu.Lock()
	r.interrupts = append(r.interrupts, outcome)
	r.mu.Unlock()
	return nil
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
	})
	require.NoError(t, err)
	starter.afterInterruptionOutcome = recorder.afterInterruptionOutcome
	return starter
}

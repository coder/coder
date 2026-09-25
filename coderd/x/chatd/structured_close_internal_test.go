package chatd

import (
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstructured"
	"github.com/coder/coder/v2/coderd/x/chatd/messagepartbuffer"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// structuredUserMessage is a user turn requesting structured output, followed
// by any extra parts.
func structuredUserMessage(t *testing.T, f *taskTestFixture, requestID uuid.UUID, extra ...codersdk.ChatMessagePart) chatstate.Message {
	t.Helper()
	req, err := chatstructured.EncodeRequestPart(chatstructured.Request{RequestID: requestID, Name: "report", Schema: json.RawMessage(`{"type":"object"}`)})
	require.NoError(t, err)
	msg := taskUserTextMessage(t, "structured", f.user.ID, f.model.ID, f.apiKey.ID)
	msg.Content, err = chatprompt.MarshalParts(append([]codersdk.ChatMessagePart{codersdk.ChatMessageText("structured"), req}, extra...))
	require.NoError(t, err)
	return msg
}

// structuredHistory returns the chat's history and the state that
// reconstruction reports for it.
func structuredHistory(t *testing.T, f *taskTestFixture, chatID uuid.UUID) ([]database.ChatMessage, chatstructured.ActiveRequestState) {
	t.Helper()
	history, err := f.db.GetChatMessagesByChatID(testutil.Context(t, testutil.WaitShort), database.GetChatMessagesByChatIDParams{ChatID: chatID})
	require.NoError(t, err)
	rows := make([]chatstructured.Row, 0, len(history))
	for _, msg := range history {
		parts, err := chatprompt.ParseContent(msg)
		require.NoError(t, err)
		rows = append(rows, chatstructured.Row{ID: msg.ID, Role: codersdk.ChatMessageRole(msg.Role), Visibility: chatstructured.Visibility(msg.Visibility), Parts: parts})
	}
	state, _ := chatstructured.ActiveRequest(rows)
	return history, state
}

func receiptOutcomes(history []database.ChatMessage) []codersdk.ChatStructuredOutput {
	var out []codersdk.ChatStructuredOutput
	for _, msg := range history {
		if o, err := chatstate.ReceiptRowOutcome(msg); err == nil {
			out = append(out, o)
		}
	}
	return out
}

func TestInterruptClosesActiveStructuredRequest(t *testing.T) {
	t.Parallel()
	requestID, queuedID := uuid.New(), uuid.New()
	interrupted := codersdk.ChatStructuredOutput{
		RequestID: requestID, Status: codersdk.ChatStructuredOutputStatusCanceled,
		Error: &codersdk.ChatStructuredOutputError{Code: codersdk.ChatStructuredOutputErrorCodeInterrupted, Message: interruptedStructuredOutputMessage},
	}
	candidate, err := chatstructured.EncodeControlPart(chatstructured.Control{Kind: chatstructured.ControlCandidate, RequestID: requestID, Value: json.RawMessage(`{"a":1}`)})
	require.NoError(t, err)
	duplicate, err := chatstructured.EncodeRequestPart(chatstructured.Request{RequestID: uuid.New(), Name: "other", Schema: json.RawMessage(`{}`)})
	require.NoError(t, err)

	for _, tt := range []struct {
		name string
		// initial is the first user turn; nil starts an ordinary chat.
		initial func(*testing.T, *taskTestFixture) chatstate.Message
		// closed commits a receipt for the request before the interrupt.
		closed bool
		// queued interrupts with a queued structured request instead of in place.
		queued     bool
		staleFence bool
		want       []codersdk.ChatStructuredOutput
	}{
		{name: "InPlace", want: []codersdk.ChatStructuredOutput{interrupted}},
		{name: "QueuedRequestStaysOpen", queued: true, want: []codersdk.ChatStructuredOutput{interrupted}},
		{name: "OrdinaryChat", initial: func(t *testing.T, f *taskTestFixture) chatstate.Message {
			return taskUserTextMessage(t, "hello", f.user.ID, f.model.ID, f.apiKey.ID)
		}},
		{name: "AlreadyClosed", closed: true},
		{name: "CorruptHistory", initial: func(t *testing.T, f *taskTestFixture) chatstate.Message {
			return structuredUserMessage(t, f, requestID, duplicate)
		}},
		{name: "StaleFence", staleFence: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			f := newTaskTestFixture(t)
			initial := structuredUserMessage(t, f, requestID)
			if tt.initial != nil {
				initial = tt.initial(t, f)
			}
			created, err := chatstate.CreateChat(ctx, f.db, f.pubsub, chatstate.CreateChatInput{
				OrganizationID: f.org.ID, OwnerID: f.user.ID, LastModelConfigID: f.model.ID, Title: "test",
				ClientType: database.ChatClientTypeApi, InitialMessages: []chatstate.Message{initial},
			})
			require.NoError(t, err)
			chatID := created.Chat.ID
			workerID, runnerID := uuid.New(), uuid.New()
			f.acquireChat(t, chatID, workerID, runnerID)
			machine := chatstate.NewChatMachine(f.db, f.pubsub, chatID)
			// A committed candidate must never turn an interrupt into a success.
			content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{candidate})
			require.NoError(t, err)
			seed := []chatstate.Message{{Role: database.ChatMessageRoleAssistant, Visibility: database.ChatMessageVisibilityModel, ContentVersion: chatprompt.CurrentContentVersion, Content: content}}
			if tt.closed {
				seed = seed[:0]
			}
			require.NoError(t, machine.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
				if tt.closed {
					failed := codersdk.ChatStructuredOutput{RequestID: requestID, Status: codersdk.ChatStructuredOutputStatusFailed, Error: &codersdk.ChatStructuredOutputError{Code: codersdk.ChatStructuredOutputErrorCodeNotProduced, Message: "none"}}
					if seed, err = structuredReceiptMessages(ctx, store, chatID, 0, failed); err != nil {
						return err
					}
				}
				if tt.initial != nil {
					return nil
				}
				_, err := tx.CommitStep(chatstate.CommitStepInput{Messages: seed})
				return err
			}))
			seeded, err := f.db.GetChatByID(ctx, chatID)
			require.NoError(t, err)
			key := messagepartbuffer.Key{ChatID: chatID, HistoryVersion: seeded.HistoryVersion, GenerationAttempt: seeded.GenerationAttempt}
			starter := newTestTaskStarter(t, f, newTaskSideEffectRecorder())
			require.NoError(t, starter.opts.MessagePartBuffer.CreateEpisode(key))
			require.NoError(t, starter.opts.MessagePartBuffer.AddPart(key, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageText("partial answer")))
			var interrupting database.Chat
			if tt.queued {
				require.NoError(t, machine.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
					_, err := tx.SendMessage(chatstate.SendMessageInput{Message: structuredUserMessage(t, f, queuedID), BusyBehavior: chatstate.BusyBehaviorInterrupt})
					return err
				}))
				interrupting, err = f.db.GetChatByID(ctx, chatID)
				require.NoError(t, err)
			} else {
				interrupting = f.forceExecutionState(t, chatID, database.ChatStatusInterrupting, false, sql.NullTime{})
			}
			if tt.staleFence {
				f.acquireChat(t, chatID, uuid.New(), uuid.New())
			}
			before, _ := structuredHistory(t, f, chatID)

			err = starter.StartInterrupt(ctx, chatWorkerTaskStartInput{
				ChatID: chatID, WorkerID: workerID, RunnerID: runnerID, HistoryVersion: interrupting.HistoryVersion,
				GenerationAttempt: interrupting.GenerationAttempt, Status: database.ChatStatusInterrupting,
			})
			history, state := structuredHistory(t, f, chatID)
			if tt.staleFence {
				require.ErrorIs(t, err, errTaskExpectedExit)
				require.Equal(t, before, history)
				return
			}
			require.NoError(t, err)
			added := history[len(before):]
			require.Equal(t, tt.want, receiptOutcomes(added))
			if tt.want == nil {
				return
			}
			// The receipt follows the partial answer and precedes any promotion.
			receipt := len(added) - 1
			if tt.queued {
				receipt--
				require.Equal(t, database.ChatMessageRoleUser, added[receipt+1].Role)
				require.True(t, state.Active)
				require.Equal(t, queuedID, state.Request.RequestID)
				require.False(t, state.Closed)
			} else {
				require.True(t, state.Closed)
				require.Equal(t, added[receipt].ID, state.OutcomeRowID)
			}
			require.True(t, chatstate.IsReceiptRow(added[receipt]))
			parts, err := chatprompt.ParseContent(added[receipt-1])
			require.NoError(t, err)
			require.Equal(t, []codersdk.ChatMessagePart{codersdk.ChatMessageText("partial answer")}, parts)
		})
	}
}

func TestReconcileClosesActiveStructuredRequest(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	f := newTaskTestFixture(t)
	requestID := uuid.New()
	toolName := "dynamic_" + uuid.NewString()
	dynamicTools, err := json.Marshal([]codersdk.DynamicTool{{Name: toolName, Description: "test tool", InputSchema: json.RawMessage(`{"type":"object"}`)}})
	require.NoError(t, err)
	created, err := chatstate.CreateChat(ctx, f.db, f.pubsub, chatstate.CreateChatInput{
		OrganizationID: f.org.ID, OwnerID: f.user.ID, LastModelConfigID: f.model.ID, Title: "test", ClientType: database.ChatClientTypeApi,
		DynamicTools:    pqtype.NullRawMessage{RawMessage: dynamicTools, Valid: true},
		InitialMessages: []chatstate.Message{structuredUserMessage(t, f, requestID)},
	})
	require.NoError(t, err)
	chat := created.Chat
	machine := chatstate.NewChatMachine(f.db, f.pubsub, chat.ID)
	require.NoError(t, machine.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		_, err := tx.CommitStep(chatstate.CommitStepInput{Messages: []chatstate.Message{taskAssistantToolCallMessage(t, f.model.ID, toolName)}})
		return err
	}))
	// Waiting with a queued message is invalid.
	queued, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{codersdk.ChatMessageText("queued")})
	require.NoError(t, err)
	_, err = f.db.InsertChatQueuedMessage(ctx, database.InsertChatQueuedMessageParams{ChatID: chat.ID, Content: queued.RawMessage})
	require.NoError(t, err)
	f.forceExecutionState(t, chat.ID, database.ChatStatusWaiting, false, sql.NullTime{})
	before, _ := structuredHistory(t, f, chat.ID)

	// chatstate rejects terminal messages that are not receipts.
	require.Error(t, machine.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		_, err := tx.ReconcileInvalidState(chatstate.ReconcileInvalidStateInput{
			TerminalMessages: []chatstate.Message{taskUserTextMessage(t, "x", f.user.ID, f.model.ID, f.apiKey.ID)},
		})
		return err
	}))
	unchanged, _ := structuredHistory(t, f, chat.ID)
	require.Equal(t, before, unchanged)

	server := &Server{db: f.db, pubsub: f.rawPS, logger: testutil.Logger(t)}
	reconciled, err := server.ReconcileInvalidStateChat(ctx, chat)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusError, reconciled.Status)
	count, err := f.db.CountChatQueuedMessages(ctx, chat.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	history, state := structuredHistory(t, f, chat.ID)
	added := history[len(before):]
	require.Len(t, added, 2)
	require.Equal(t, database.ChatMessageRoleTool, added[0].Role)
	require.Equal(t, []codersdk.ChatStructuredOutput{{
		RequestID: requestID, Status: codersdk.ChatStructuredOutputStatusFailed,
		Error: &codersdk.ChatStructuredOutputError{Code: codersdk.ChatStructuredOutputErrorCodeGenerationFailed, Message: reconciledStructuredOutputMessage},
	}}, receiptOutcomes(added[1:]))
	require.True(t, state.Closed)
}

// Promoting a queued message out of requires_action ends the waiting turn,
// so its open structured output request closes as interrupted before the
// promoted message.
func TestPromoteQueuedClosesRequiresActionStructuredRequest(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	f := newTaskTestFixture(t)
	requestID, toolName := uuid.New(), "dynamic_"+uuid.NewString()
	dynamicTools, err := json.Marshal([]codersdk.DynamicTool{{Name: toolName, Description: "test tool", InputSchema: json.RawMessage(`{"type":"object"}`)}})
	require.NoError(t, err)
	created, err := chatstate.CreateChat(ctx, f.db, f.pubsub, chatstate.CreateChatInput{
		OrganizationID: f.org.ID, OwnerID: f.user.ID, LastModelConfigID: f.model.ID, Title: "test", ClientType: database.ChatClientTypeApi,
		DynamicTools:    pqtype.NullRawMessage{RawMessage: dynamicTools, Valid: true},
		InitialMessages: []chatstate.Message{structuredUserMessage(t, f, requestID)},
	})
	require.NoError(t, err)
	machine := chatstate.NewChatMachine(f.db, f.pubsub, created.Chat.ID)
	require.NoError(t, machine.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		if _, err := tx.CommitStep(chatstate.CommitStepInput{Messages: []chatstate.Message{taskAssistantToolCallMessage(t, f.model.ID, toolName)}}); err != nil {
			return err
		}
		_, err := tx.EnterRequiresAction(chatstate.EnterRequiresActionInput{})
		return err
	}))
	queueTestMessages(t, machine, taskUserTextMessage(t, "queued", f.user.ID, f.model.ID, f.apiKey.ID))
	queued, err := f.db.GetChatQueuedMessages(ctx, created.Chat.ID)
	require.NoError(t, err)

	server := &Server{db: f.db, pubsub: f.rawPS, logger: testutil.Logger(t)}
	result, err := server.PromoteQueued(ctx, PromoteQueuedOptions{ChatID: created.Chat.ID, QueuedMessageID: queued[0].ID})
	require.NoError(t, err)
	history, _ := structuredHistory(t, f, created.Chat.ID)
	require.Equal(t, result.PromotedMessage.ID, history[len(history)-1].ID)
	require.Equal(t, []codersdk.ChatStructuredOutput{canceledStructuredOutput(requestID, codersdk.ChatStructuredOutputErrorCodeInterrupted, interruptedStructuredOutputMessage)},
		receiptOutcomes(history[len(history)-2:len(history)-1]))
}

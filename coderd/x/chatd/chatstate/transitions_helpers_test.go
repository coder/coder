package chatstate_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// seededChat is the shared output of seedState. Some transition tests
// need extra context beyond the chat ID (for example, the queued
// message ID to delete, or the message ID to edit), so this struct
// surfaces what each state was seeded with.

type seededChat struct {
	chatID                 uuid.UUID
	exists                 bool
	initialUserMessageID   int64
	assistantToolCallMsgID int64
	queuedMessageIDs       []int64
	// queuedMessageBodies is parallel to queuedMessageIDs and records
	// the text body each queued message was seeded with. Cases that
	// promote queued messages into history use this to assert the
	// promoted message content matches what was originally queued.
	queuedMessageBodies    []string
	queuedMessageCreatedBy []uuid.UUID
	dynamicToolName        string
	pendingToolCallID      string
	pendingToolCallIDs     []string
}

// dynamicToolJSON returns the canonical [{name,description,input_schema}]
// payload used to seed dynamic_tools on a chat. Tests that need
// pending dynamic tool calls (A0, A1) reuse this and reference the
// returned tool name in their assistant tool-call message.
func dynamicToolJSON(name string) []byte {
	tools := []codersdk.DynamicTool{{
		Name:        name,
		Description: "test tool",
		InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
	}}
	raw, err := json.Marshal(tools)
	if err != nil {
		panic(err)
	}
	return raw
}

// assistantToolCallMessage builds a chatstate.Message for an
// assistant message that issues one tool call against the supplied
// dynamic tool name. The tool-call ID is unique per call so multiple
// messages do not collide.
func assistantToolCallMessage(t *testing.T, modelID uuid.UUID, toolName, callID string) chatstate.Message {
	t.Helper()
	raw, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{{
		Type:       codersdk.ChatMessagePartTypeToolCall,
		ToolCallID: callID,
		ToolName:   toolName,
		Args:       json.RawMessage(`{}`),
	}})
	require.NoError(t, err)
	return chatstate.Message{
		Role:           database.ChatMessageRoleAssistant,
		Content:        raw,
		Visibility:     database.ChatMessageVisibilityBoth,
		ContentVersion: chatprompt.CurrentContentVersion,
		ModelConfigID:  uuid.NullUUID{UUID: modelID, Valid: true},
	}
}

func mixedAssistantToolCallMessage(t *testing.T, modelID uuid.UUID, dynamicTool, dynCallID, nonDynCallID string) chatstate.Message {
	t.Helper()
	parts := []codersdk.ChatMessagePart{
		{
			Type:       codersdk.ChatMessagePartTypeToolCall,
			ToolCallID: dynCallID,
			ToolName:   dynamicTool,
			Args:       json.RawMessage(`{}`),
		},
		{
			Type:       codersdk.ChatMessagePartTypeToolCall,
			ToolCallID: nonDynCallID,
			ToolName:   "non_dynamic_tool",
			Args:       json.RawMessage(`{}`),
		},
	}
	raw, err := chatprompt.MarshalParts(parts)
	require.NoError(t, err)
	return chatstate.Message{
		Role:           database.ChatMessageRoleAssistant,
		Content:        raw,
		Visibility:     database.ChatMessageVisibilityBoth,
		ContentVersion: chatprompt.CurrentContentVersion,
		ModelConfigID:  uuid.NullUUID{UUID: modelID, Valid: true},
	}
}

// createTestChatWithDynamicTools mirrors createTestChat but seeds the
// chat with a non-empty dynamic_tools blob so EnterRequiresAction,
// CompleteRequiresAction, and CancelRequiresAction can find pending
// dynamic tool calls.
func createTestChatWithDynamicTools(t *testing.T, f *testFixture, toolName string) chatstate.CreateChatResult {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitShort)
	res, err := chatstate.CreateChat(ctx, f.DB, f.Pub, chatstate.CreateChatInput{
		OrganizationID:    f.Org.ID,
		OwnerID:           f.User.ID,
		LastModelConfigID: f.Model.ID,
		Title:             "test",
		ClientType:        database.ChatClientTypeApi,
		DynamicTools: pqtype.NullRawMessage{
			RawMessage: dynamicToolJSON(toolName),
			Valid:      true,
		},
		InitialMessages: []chatstate.Message{
			userTextMessage("hello", f.User.ID, f.Model.ID),
		},
	})
	require.NoError(t, err)
	return res
}

// seedAOrA1 seeds a chat into A0 (queuedExtras=0) or A1
// (queuedExtras>=1) with a real pending dynamic tool call. Used by
// cases that need A0 or A1 with a configurable queue cardinality.
func seedAOrA1(t *testing.T, f *testFixture, queuedExtras int, namePrefix string) seededChat {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitShort)
	toolName := namePrefix
	callID := "call_" + uuid.NewString()
	created := createTestChatWithDynamicTools(t, f, toolName)
	m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)
	var step chatstate.CommitStepResult
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		var err error
		step, err = tx.CommitStep(chatstate.CommitStepInput{
			Messages: []chatstate.Message{
				assistantToolCallMessage(t, f.Model.ID, toolName, callID),
			},
		})
		return err
	}))
	require.Len(t, step.InsertedMessages, 1)
	// R0 -> A0.
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		_, err := tx.EnterRequiresAction(chatstate.EnterRequiresActionInput{})
		return err
	}))
	var (
		queuedIDs    []int64
		queuedBodies []string
	)
	for i := 0; i < queuedExtras; i++ {
		body := fmt.Sprintf("queued-%s-%d", namePrefix, i)
		sm := sendQueuedMessage(t, f, m, body)
		require.NotNil(t, sm.QueuedMessage)
		queuedIDs = append(queuedIDs, sm.QueuedMessage.ID)
		queuedBodies = append(queuedBodies, body)
	}
	return seededChat{
		chatID:                 created.Chat.ID,
		exists:                 true,
		initialUserMessageID:   firstUserMessageID(ctx, t, f, created.Chat.ID),
		assistantToolCallMsgID: step.InsertedMessages[0].ID,
		queuedMessageIDs:       queuedIDs,
		queuedMessageBodies:    queuedBodies,
		dynamicToolName:        toolName,
		pendingToolCallID:      callID,
	}
}

// seedState seeds a chat into the supplied execution state and
// returns identifying handles useful for downstream assertions. For
// [chatstate.StateN] the returned chatID is a fresh UUID that does
// not exist in the database. Multi-queued seeds (for E1, R1, I1,
// A1 with 2 queued messages, and Invalid with a non-empty queue) live in
// seedStateMultiQueued.
func seedState(t *testing.T, f *testFixture, state chatstate.ExecutionState) seededChat {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitShort)

	switch state {
	case chatstate.StateN:
		return seededChat{chatID: uuid.New(), exists: false}

	case chatstate.StateR0:
		created := createTestChat(t, f)
		initial := firstUserMessageID(ctx, t, f, created.Chat.ID)
		return seededChat{
			chatID:               created.Chat.ID,
			exists:               true,
			initialUserMessageID: initial,
		}

	case chatstate.StateW:
		created := createTestChat(t, f)
		m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)
		require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
			_, err := tx.FinishTurn(chatstate.FinishTurnInput{})
			return err
		}))
		return seededChat{
			chatID:               created.Chat.ID,
			exists:               true,
			initialUserMessageID: firstUserMessageID(ctx, t, f, created.Chat.ID),
		}

	case chatstate.StateE0:
		created := createTestChat(t, f)
		m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)
		require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
			_, err := tx.FinishError(chatstate.FinishErrorInput{
				LastError: pqtype.NullRawMessage{
					RawMessage: json.RawMessage(`{"message":"boom"}`),
					Valid:      true,
				},
			})
			return err
		}))
		return seededChat{
			chatID:               created.Chat.ID,
			exists:               true,
			initialUserMessageID: firstUserMessageID(ctx, t, f, created.Chat.ID),
		}

	case chatstate.StateE1:
		created := createTestChat(t, f)
		m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)
		// R0 -> R1
		queuedBody := "queued-for-E1"
		queued := sendQueuedMessage(t, f, m, queuedBody)
		require.NotNil(t, queued.QueuedMessage)
		// R1 -> E1
		require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
			_, err := tx.FinishError(chatstate.FinishErrorInput{
				LastError: pqtype.NullRawMessage{
					RawMessage: json.RawMessage(`{"message":"boom"}`),
					Valid:      true,
				},
			})
			return err
		}))
		return seededChat{
			chatID:               created.Chat.ID,
			exists:               true,
			initialUserMessageID: firstUserMessageID(ctx, t, f, created.Chat.ID),
			queuedMessageIDs:     []int64{queued.QueuedMessage.ID},
			queuedMessageBodies:  []string{queuedBody},
		}

	case chatstate.StateR1:
		created := createTestChat(t, f)
		m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)
		queuedBody := "queued-for-R1"
		queued := sendQueuedMessage(t, f, m, queuedBody)
		require.NotNil(t, queued.QueuedMessage)
		return seededChat{
			chatID:               created.Chat.ID,
			exists:               true,
			initialUserMessageID: firstUserMessageID(ctx, t, f, created.Chat.ID),
			queuedMessageIDs:     []int64{queued.QueuedMessage.ID},
			queuedMessageBodies:  []string{queuedBody},
		}

	case chatstate.StateI0:
		created := createTestChat(t, f)
		m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)
		require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
			_, err := tx.Interrupt(chatstate.InterruptInput{Reason: "seed"})
			return err
		}))
		return seededChat{
			chatID:               created.Chat.ID,
			exists:               true,
			initialUserMessageID: firstUserMessageID(ctx, t, f, created.Chat.ID),
		}

	case chatstate.StateI1:
		created := createTestChat(t, f)
		m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)
		// R0 -> I1: SendMessage with interrupt behavior queues the
		// message and sets status to interrupting.
		queuedBody := "queued-for-I1"
		sm := sendInterruptMessage(t, f, m, queuedBody)
		require.NotNil(t, sm.QueuedMessage)
		return seededChat{
			chatID:               created.Chat.ID,
			exists:               true,
			initialUserMessageID: firstUserMessageID(ctx, t, f, created.Chat.ID),
			queuedMessageIDs:     []int64{sm.QueuedMessage.ID},
			queuedMessageBodies:  []string{queuedBody},
		}

	case chatstate.StateA0:
		return seedAOrA1(t, f, 0, "seed_tool_a0")

	case chatstate.StateA1:
		return seedAOrA1(t, f, 1, "seed_tool_a1")

	case chatstate.StateXW:
		created := createTestChat(t, f)
		m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)
		require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
			_, err := tx.FinishTurn(chatstate.FinishTurnInput{})
			return err
		}))
		require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
			_, err := tx.SetArchived(chatstate.SetArchivedInput{Archived: true})
			return err
		}))
		return seededChat{
			chatID:               created.Chat.ID,
			exists:               true,
			initialUserMessageID: firstUserMessageID(ctx, t, f, created.Chat.ID),
		}

	case chatstate.StateXE0:
		created := createTestChat(t, f)
		m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)
		require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
			_, err := tx.FinishError(chatstate.FinishErrorInput{
				LastError: pqtype.NullRawMessage{
					RawMessage: json.RawMessage(`{"message":"boom"}`),
					Valid:      true,
				},
			})
			return err
		}))
		require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
			_, err := tx.SetArchived(chatstate.SetArchivedInput{Archived: true})
			return err
		}))
		return seededChat{
			chatID:               created.Chat.ID,
			exists:               true,
			initialUserMessageID: firstUserMessageID(ctx, t, f, created.Chat.ID),
		}

	case chatstate.StateXE1:
		created := createTestChat(t, f)
		m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)
		queuedBody := "queued-for-XE1"
		queued := sendQueuedMessage(t, f, m, queuedBody)
		require.NotNil(t, queued.QueuedMessage)
		require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
			_, err := tx.FinishError(chatstate.FinishErrorInput{
				LastError: pqtype.NullRawMessage{
					RawMessage: json.RawMessage(`{"message":"boom"}`),
					Valid:      true,
				},
			})
			return err
		}))
		require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
			_, err := tx.SetArchived(chatstate.SetArchivedInput{Archived: true})
			return err
		}))
		return seededChat{
			chatID:               created.Chat.ID,
			exists:               true,
			initialUserMessageID: firstUserMessageID(ctx, t, f, created.Chat.ID),
			queuedMessageIDs:     []int64{queued.QueuedMessage.ID},
			queuedMessageBodies:  []string{queuedBody},
		}

	case chatstate.StateInvalid:
		created := createTestChat(t, f)
		// Force running + archived, a deliberately invalid
		// combination per the classifier.
		_, err := f.DB.UpdateChatExecutionState(ctx, database.UpdateChatExecutionStateParams{
			ID:                       created.Chat.ID,
			Status:                   database.ChatStatusRunning,
			Archived:                 true,
			WorkerID:                 created.Chat.WorkerID,
			RunnerID:                 created.Chat.RunnerID,
			LastError:                created.Chat.LastError,
			RequiresActionDeadlineAt: created.Chat.RequiresActionDeadlineAt,
		})
		require.NoError(t, err)
		return seededChat{
			chatID:               created.Chat.ID,
			exists:               true,
			initialUserMessageID: firstUserMessageID(ctx, t, f, created.Chat.ID),
		}
	}
	t.Fatalf("seedState: unsupported execution state %s", state)
	return seededChat{}
}

// seedStateMultiQueued seeds a state with two queued messages. Used
// by cases that need the post-mutation queue to remain non-empty.
// Supported states: E1, R1, I1, A1.
func seedStateMultiQueued(t *testing.T, f *testFixture, state chatstate.ExecutionState) seededChat {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitShort)
	switch state {
	case chatstate.StateE1:
		created := createTestChat(t, f)
		m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)
		firstBody := "queued-e1-a"
		first := sendQueuedMessage(t, f, m, firstBody)
		require.NotNil(t, first.QueuedMessage)
		secondBody := "queued-e1-b"
		second := sendQueuedMessage(t, f, m, secondBody)
		require.NotNil(t, second.QueuedMessage)
		require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
			_, err := tx.FinishError(chatstate.FinishErrorInput{
				LastError: pqtype.NullRawMessage{
					RawMessage: json.RawMessage(`{"message":"boom"}`),
					Valid:      true,
				},
			})
			return err
		}))
		return seededChat{
			chatID:               created.Chat.ID,
			exists:               true,
			initialUserMessageID: firstUserMessageID(ctx, t, f, created.Chat.ID),
			queuedMessageIDs:     []int64{first.QueuedMessage.ID, second.QueuedMessage.ID},
			queuedMessageBodies:  []string{firstBody, secondBody},
		}

	case chatstate.StateR1:
		created := createTestChat(t, f)
		m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)
		firstBody := "queued-r1-a"
		first := sendQueuedMessage(t, f, m, firstBody)
		require.NotNil(t, first.QueuedMessage)
		secondBody := "queued-r1-b"
		second := sendQueuedMessage(t, f, m, secondBody)
		require.NotNil(t, second.QueuedMessage)
		return seededChat{
			chatID:               created.Chat.ID,
			exists:               true,
			initialUserMessageID: firstUserMessageID(ctx, t, f, created.Chat.ID),
			queuedMessageIDs:     []int64{first.QueuedMessage.ID, second.QueuedMessage.ID},
			queuedMessageBodies:  []string{firstBody, secondBody},
		}

	case chatstate.StateI1:
		created := createTestChat(t, f)
		m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)
		firstBody := "queued-i1-a"
		first := sendQueuedMessage(t, f, m, firstBody)
		require.NotNil(t, first.QueuedMessage)
		// R1 -> I1 via interrupt-mode SendMessage queues a second
		// message and flips status to interrupting.
		secondBody := "queued-i1-b"
		second := sendInterruptMessage(t, f, m, secondBody)
		require.NotNil(t, second.QueuedMessage)
		return seededChat{
			chatID:               created.Chat.ID,
			exists:               true,
			initialUserMessageID: firstUserMessageID(ctx, t, f, created.Chat.ID),
			queuedMessageIDs:     []int64{first.QueuedMessage.ID, second.QueuedMessage.ID},
			queuedMessageBodies:  []string{firstBody, secondBody},
		}

	case chatstate.StateA1:
		return seedAOrA1(t, f, 2, "seed_tool_a1_multi")
	}
	t.Fatalf("seedStateMultiQueued: unsupported execution state %s", state)
	return seededChat{}
}

// seedA1WithMixedOutstandingToolCalls seeds A1 with one queued message
// and one assistant message carrying both a dynamic and non-dynamic
// outstanding tool call. It is used by PromoteQueuedMessage(A1) to
// prove all tool calls are closed before inserting the promoted user.
func seedA1WithMixedOutstandingToolCalls(t *testing.T, f *testFixture, queuedExtras int, namePrefix string) seededChat {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitShort)
	toolName := namePrefix
	dynCallID := "call_" + uuid.NewString()
	nonDynCallID := "call_" + uuid.NewString()
	created := createTestChatWithDynamicTools(t, f, toolName)
	m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)
	var step chatstate.CommitStepResult
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		var err error
		step, err = tx.CommitStep(chatstate.CommitStepInput{
			Messages: []chatstate.Message{
				mixedAssistantToolCallMessage(t, f.Model.ID, toolName, dynCallID, nonDynCallID),
			},
		})
		return err
	}))
	require.Len(t, step.InsertedMessages, 1)
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		_, err := tx.EnterRequiresAction(chatstate.EnterRequiresActionInput{})
		return err
	}))
	var (
		queuedIDs       []int64
		queuedBodies    []string
		queuedCreatedBy []uuid.UUID
	)
	for i := range queuedExtras {
		body := fmt.Sprintf("queued-%s-%d", namePrefix, i)
		createdBy := uuid.New()
		queued, err := f.DB.InsertChatQueuedMessageWithCreator(ctx, database.InsertChatQueuedMessageWithCreatorParams{
			ChatID:        created.Chat.ID,
			Content:       userMessageContent(t, body),
			ModelConfigID: uuid.NullUUID{UUID: f.Model.ID, Valid: true},
			CreatedBy:     createdBy,
		})
		require.NoError(t, err)
		queuedIDs = append(queuedIDs, queued.ID)
		queuedBodies = append(queuedBodies, body)
		queuedCreatedBy = append(queuedCreatedBy, createdBy)
	}
	return seededChat{
		chatID:                 created.Chat.ID,
		exists:                 true,
		initialUserMessageID:   firstUserMessageID(ctx, t, f, created.Chat.ID),
		assistantToolCallMsgID: step.InsertedMessages[0].ID,
		queuedMessageIDs:       queuedIDs,
		queuedMessageBodies:    queuedBodies,
		queuedMessageCreatedBy: queuedCreatedBy,
		dynamicToolName:        toolName,
		pendingToolCallID:      dynCallID,
		pendingToolCallIDs:     []string{dynCallID, nonDynCallID},
	}
}

// seedInvalidWithQueue seeds Invalid with a single queued message so
// ReconcileInvalidState lands in E1 (non-empty queue) instead of E0.
func seedInvalidWithQueue(t *testing.T, f *testFixture) seededChat {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)
	m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)
	queuedBody := "queued-invalid"
	queued := sendQueuedMessage(t, f, m, queuedBody)
	require.NotNil(t, queued.QueuedMessage)
	// Force the deliberately invalid running + archived combo on
	// top of the queue.
	chat, err := f.DB.GetChatByID(ctx, created.Chat.ID)
	require.NoError(t, err)
	_, err = f.DB.UpdateChatExecutionState(ctx, database.UpdateChatExecutionStateParams{
		ID:                       chat.ID,
		Status:                   database.ChatStatusRunning,
		Archived:                 true,
		WorkerID:                 chat.WorkerID,
		RunnerID:                 chat.RunnerID,
		LastError:                chat.LastError,
		RequiresActionDeadlineAt: chat.RequiresActionDeadlineAt,
	})
	require.NoError(t, err)
	return seededChat{
		chatID:               chat.ID,
		exists:               true,
		initialUserMessageID: firstUserMessageID(ctx, t, f, chat.ID),
		queuedMessageIDs:     []int64{queued.QueuedMessage.ID},
		queuedMessageBodies:  []string{queuedBody},
	}
}

// firstUserMessageID returns the lowest-id non-deleted user message
// on the chat. Most transition tests reuse this when they need a
// user message to edit.
func firstUserMessageID(ctx context.Context, t *testing.T, f *testFixture, chatID uuid.UUID) int64 {
	t.Helper()
	msgs, err := f.DB.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
		ChatID: chatID,
	})
	require.NoError(t, err)
	for _, m := range msgs {
		if m.Role == database.ChatMessageRoleUser && !m.Deleted {
			return m.ID
		}
	}
	t.Fatalf("firstUserMessageID: chat %s has no user messages", chatID)
	return 0
}

func firstAssistantMessageID(ctx context.Context, t *testing.T, f *testFixture, chatID uuid.UUID) int64 {
	t.Helper()
	msgs, err := f.DB.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
		ChatID: chatID,
	})
	require.NoError(t, err)
	for _, m := range msgs {
		if m.Role == database.ChatMessageRoleAssistant && !m.Deleted {
			return m.ID
		}
	}
	t.Fatalf("firstAssistantMessageID: chat %s has no assistant messages", chatID)
	return 0
}

// seedForEnterRequiresAction extends seedState for R0 and R1 with a
// chat that has dynamic_tools plus an assistant tool-call message in
// history. EnterRequiresAction's precondition rejects R0/R1 without
// pending dynamic tool calls, so the generic seedState path will not
// do. Other states fall through to the default seedState.
func seedForEnterRequiresAction(t *testing.T, f *testFixture, state chatstate.ExecutionState) seededChat {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitShort)
	switch state {
	case chatstate.StateR0:
		toolName := "ra_tool_r0"
		callID := "call_" + uuid.NewString()
		created := createTestChatWithDynamicTools(t, f, toolName)
		m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)
		var step chatstate.CommitStepResult
		require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
			var err error
			step, err = tx.CommitStep(chatstate.CommitStepInput{
				Messages: []chatstate.Message{
					assistantToolCallMessage(t, f.Model.ID, toolName, callID),
				},
			})
			return err
		}))
		require.Len(t, step.InsertedMessages, 1)
		return seededChat{
			chatID:                 created.Chat.ID,
			exists:                 true,
			initialUserMessageID:   firstUserMessageID(ctx, t, f, created.Chat.ID),
			assistantToolCallMsgID: step.InsertedMessages[0].ID,
			dynamicToolName:        toolName,
			pendingToolCallID:      callID,
			pendingToolCallIDs:     []string{callID},
		}
	case chatstate.StateR1:
		toolName := "ra_tool_r1"
		callID := "call_" + uuid.NewString()
		created := createTestChatWithDynamicTools(t, f, toolName)
		m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)
		var step chatstate.CommitStepResult
		require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
			var err error
			step, err = tx.CommitStep(chatstate.CommitStepInput{
				Messages: []chatstate.Message{
					assistantToolCallMessage(t, f.Model.ID, toolName, callID),
				},
			})
			return err
		}))
		// R0 -> R1 with a queued message.
		queuedBody := "queued-for-RA-r1"
		sm := sendQueuedMessage(t, f, m, queuedBody)
		require.NotNil(t, sm.QueuedMessage)
		return seededChat{
			chatID:                 created.Chat.ID,
			exists:                 true,
			initialUserMessageID:   firstUserMessageID(ctx, t, f, created.Chat.ID),
			assistantToolCallMsgID: step.InsertedMessages[0].ID,
			queuedMessageIDs:       []int64{sm.QueuedMessage.ID},
			queuedMessageBodies:    []string{queuedBody},
			dynamicToolName:        toolName,
			pendingToolCallID:      callID,
			pendingToolCallIDs:     []string{callID},
		}
	}
	return seedState(t, f, state)
}

// activeHistoryIDs returns the ids of non-deleted history messages
// for the chat in row-id order. Useful for verifying CommitStep,
// EditMessage replacement, and PromoteQueuedMessage head insertion.
func activeHistoryIDs(ctx context.Context, t *testing.T, f *testFixture, chatID uuid.UUID) []int64 {
	t.Helper()
	msgs, err := f.DB.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
		ChatID: chatID,
	})
	require.NoError(t, err)
	out := make([]int64, 0, len(msgs))
	for _, m := range msgs {
		if !m.Deleted {
			out = append(out, m.ID)
		}
	}
	return out
}

func requireChatMessageByID(ctx context.Context, t *testing.T, f *testFixture, id int64) database.ChatMessage {
	t.Helper()
	msg, err := f.DB.GetChatMessageByID(ctx, id)
	require.NoError(t, err)
	return msg
}

func requireQueuedMessageByID(ctx context.Context, t *testing.T, f *testFixture, chatID uuid.UUID, id int64) database.ChatQueuedMessage {
	t.Helper()
	msg, err := f.DB.GetChatQueuedMessageByID(ctx, database.GetChatQueuedMessageByIDParams{
		ID:     id,
		ChatID: chatID,
	})
	require.NoError(t, err)
	return msg
}

func requireQueuedMessageDeleted(ctx context.Context, t *testing.T, f *testFixture, chatID uuid.UUID, id int64) {
	t.Helper()
	_, err := f.DB.GetChatQueuedMessageByID(ctx, database.GetChatQueuedMessageByIDParams{
		ID:     id,
		ChatID: chatID,
	})
	require.Error(t, err)
}

func assertFetchedUserMessage(ctx context.Context, t *testing.T, f *testFixture, msg database.ChatMessage) database.ChatMessage {
	t.Helper()
	fetched := requireChatMessageByID(ctx, t, f, msg.ID)
	require.Equal(t, msg.ChatID, fetched.ChatID)
	require.Equal(t, database.ChatMessageRoleUser, fetched.Role)
	require.True(t, fetched.CreatedBy.Valid)
	require.Equal(t, f.User.ID, fetched.CreatedBy.UUID)
	require.True(t, fetched.ModelConfigID.Valid)
	require.Equal(t, f.Model.ID, fetched.ModelConfigID.UUID)
	require.Equal(t, chatprompt.CurrentContentVersion, fetched.ContentVersion)
	return fetched
}

func assertFetchedQueuedMessage(ctx context.Context, t *testing.T, f *testFixture, chatID uuid.UUID, queued database.ChatQueuedMessage) database.ChatQueuedMessage {
	t.Helper()
	fetched := requireQueuedMessageByID(ctx, t, f, chatID, queued.ID)
	require.Equal(t, chatID, fetched.ChatID)
	require.Equal(t, f.User.ID, fetched.CreatedBy)
	require.True(t, fetched.ModelConfigID.Valid)
	require.Equal(t, f.Model.ID, fetched.ModelConfigID.UUID)
	require.NotEmpty(t, fetched.Content)
	return fetched
}

func newActiveMessageIDs(base snapshotBaseline, after []int64) []int64 {
	seen := make(map[int64]struct{}, len(base.historyIDs))
	for _, id := range base.historyIDs {
		seen[id] = struct{}{}
	}
	out := make([]int64, 0, len(after))
	for _, id := range after {
		if _, ok := seen[id]; !ok {
			out = append(out, id)
		}
	}
	return out
}

// assertToolResultForCallNoError asserts that msg is a tool-result
// message that resolves a tool call with id wantCallID, is_error=false,
// and that the result JSON matches wantResultJSON. Complements
// assertToolResultForCall in synthetic_cancellation_test.go which
// asserts is_error=true.
func assertToolResultForCallNoError(t *testing.T, msg database.ChatMessage, wantCallID, wantResultJSON string) {
	t.Helper()
	require.Equal(t, database.ChatMessageRoleTool, msg.Role)
	parts, err := chatprompt.ParseContent(msg)
	require.NoError(t, err)
	require.NotEmpty(t, parts)
	var found bool
	for _, p := range parts {
		if p.Type != codersdk.ChatMessagePartTypeToolResult {
			continue
		}
		require.Equal(t, wantCallID, p.ToolCallID, "tool-call id matches")
		require.False(t, p.IsError, "CompleteRequiresAction tool result must not be is_error")
		require.JSONEq(t, wantResultJSON, string(p.Result), "CompleteRequiresAction tool result JSON matches submitted output")
		found = true
	}
	require.True(t, found, "expected at least one tool-result part")
}

// assertChatMessageText asserts that the persisted content of msg
// decodes to a single text part with the supplied body. Used by
// matrix cases that need to verify the actual text submitted via
// SendMessage / EditMessage / CommitStep, or the text that was
// promoted out of the queue into history.
func assertChatMessageText(t *testing.T, msg database.ChatMessage, want string) {
	t.Helper()
	parts, err := chatprompt.ParseContent(msg)
	require.NoError(t, err, "parse chat message content")
	require.Len(t, parts, 1, "expected exactly one content part")
	require.Equal(t, codersdk.ChatMessagePartTypeText, parts[0].Type,
		"expected a text content part")
	require.Equal(t, want, parts[0].Text, "unexpected chat message text")
}

// assertQueuedMessageText asserts that the JSON content of queued
// decodes to a single text part with the supplied body. Used by
// matrix cases that need to verify the body inserted into
// chat_queued_messages via SendMessage.
func assertQueuedMessageText(t *testing.T, queued database.ChatQueuedMessage, want string) {
	t.Helper()
	assertQueuedMessageContent(t, queued.Content, want)
}

func assertQueuedMessageContent(t *testing.T, content json.RawMessage, want string) {
	t.Helper()
	var parts []codersdk.ChatMessagePart
	require.NoError(t, json.Unmarshal(content, &parts), "unmarshal queued content")
	require.Len(t, parts, 1, "expected exactly one queued content part")
	require.Equal(t, codersdk.ChatMessagePartTypeText, parts[0].Type,
		"expected a text content part")
	require.Equal(t, want, parts[0].Text, "unexpected queued message text")
}

// assertQueueBodiesInOrder fetches the queued messages for the chat
// in queue order and asserts each row's text body matches the
// supplied bodies. Used by matrix cases that need to verify the
// remaining queue content after a promote / finish-turn /
// finish-interruption.
func assertQueueBodiesInOrder(ctx context.Context, t *testing.T, f *testFixture, chatID uuid.UUID, want []string) {
	t.Helper()
	rows, err := f.DB.GetChatQueuedMessagesByPosition(ctx, chatID)
	require.NoError(t, err)
	require.Len(t, rows, len(want), "queue length must match expected bodies")
	for i, r := range rows {
		assertQueuedMessageContent(t, r.Content, want[i])
	}
}

// snapshotBaseline records the chat's snapshot_version and the
// publisher's recorded channel count immediately before a transition
// runs. Tests use it to verify either a single snapshot bump and one
// chat:update on success, or zero mutation and zero publishes on
// failure.
type snapshotBaseline struct {
	exists            bool
	chat              database.Chat
	snapshot          int64
	historyVersion    int64
	queueVersion      int64
	retryStateVersion int64
	generationAttempt int64
	queueCount        int64
	queueIDs          []int64
	historyIDs        []int64
	channels          int
}

func captureBaseline(ctx context.Context, t *testing.T, f *testFixture, seeded seededChat) snapshotBaseline {
	t.Helper()
	base := snapshotBaseline{
		exists:   seeded.exists,
		channels: len(f.Pub.channels),
	}
	if !seeded.exists {
		return base
	}
	chat, err := f.DB.GetChatByID(ctx, seeded.chatID)
	require.NoError(t, err)
	base.chat = chat
	base.snapshot = chat.SnapshotVersion
	base.historyVersion = chat.HistoryVersion
	base.queueVersion = chat.QueueVersion
	base.retryStateVersion = chat.RetryStateVersion
	base.generationAttempt = chat.GenerationAttempt
	base.queueIDs = queuedIDsByPosition(ctx, t, f, seeded.chatID)
	count, err := f.DB.CountChatQueuedMessages(ctx, seeded.chatID)
	require.NoError(t, err)
	base.queueCount = count
	base.historyIDs = activeHistoryIDs(ctx, t, f, seeded.chatID)
	return base
}

// assertSnapshotBumpedOnce asserts that one Update committed; that is,
// snapshot_version advanced by exactly one and the publisher saw at
// least one chat:update on the per-chat channel after the baseline.
func assertSnapshotBumpedOnce(ctx context.Context, t *testing.T, f *testFixture, chatID uuid.UUID, base snapshotBaseline) {
	t.Helper()
	after, err := f.DB.GetChatByID(ctx, chatID)
	require.NoError(t, err)
	require.Equal(t, base.snapshot+1, after.SnapshotVersion, "snapshot_version must bump exactly once")
	channel := coderdpubsub.ChatStateUpdateChannel(chatID)
	found := false
	for _, c := range f.Pub.channels[base.channels:] {
		if c == channel {
			found = true
			break
		}
	}
	require.True(t, found, "expected one chat:update on %s after commit", channel)
}

// assertNoMutationOrPublish asserts a failed transition rolled back
// the automatic snapshot bump and published nothing.
func assertNoMutationOrPublish(ctx context.Context, t *testing.T, f *testFixture, chatID uuid.UUID, base snapshotBaseline) {
	t.Helper()
	require.Equal(t, base.channels, len(f.Pub.channels), "failed transition must not publish")
	if base.exists {
		after, err := f.DB.GetChatByID(ctx, chatID)
		require.NoError(t, err)
		require.Equal(t, base.snapshot, after.SnapshotVersion, "failed transition must not advance snapshot_version")
	}
}

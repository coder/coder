package coderd_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// withChatWorkerDisabled turns off the chat daemon's background worker
// so every test in this file observes synchronous chatstate endpoint
// behavior deterministically. Without it the worker races the tests:
// it can finish a turn (running -> waiting), promote queued messages,
// or commit steps concurrently with the driveChatTo* fixtures.
func withChatWorkerDisabled(o *coderdtest.Options) {
	o.ChatWorkerDisabled = true
}

// driveChatToWaiting transitions the chat from `running` (its initial
// state per the RFC) to `waiting` by running chatstate.FinishTurn.
// Tests use this when they need to exercise endpoint behavior that
// only succeeds from idle execution states (W, E0).
func driveChatToWaiting(ctx context.Context, t *testing.T, api *coderd.API, chatID uuid.UUID) {
	t.Helper()
	chatdCtx := dbauthz.AsChatd(ctx) //nolint:gocritic // Test fixture mirrors chatd background transitions.
	machine := chatstate.NewChatMachine(api.Database, api.Pubsub, chatID)
	require.NoError(t, machine.Update(chatdCtx, func(tx *chatstate.Tx, store database.Store) error {
		_, err := tx.FinishTurn(chatstate.FinishTurnInput{})
		return err
	}))
}

// driveChatToRequiresAction commits an assistant message with a single
// dynamic tool_call part and then transitions the chat to
// `requires_action`. The tool_call_id returned lets the caller
// assemble a valid SubmitToolResultsRequest.
func driveChatToRequiresAction(
	ctx context.Context,
	t *testing.T,
	api *coderd.API,
	chat codersdk.Chat,
	toolName string,
) (toolCallID string) {
	t.Helper()
	chatdCtx := dbauthz.AsChatd(ctx) //nolint:gocritic // Test fixture mirrors chatd background transitions.

	toolCallID = "call-" + uuid.NewString()
	assistantContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageText("dispatching dynamic tool"),
		{
			Type:       codersdk.ChatMessagePartTypeToolCall,
			ToolCallID: toolCallID,
			ToolName:   toolName,
			Args:       json.RawMessage(`{}`),
		},
	})
	require.NoError(t, err)

	machine := chatstate.NewChatMachine(api.Database, api.Pubsub, chat.ID)
	require.NoError(t, machine.Update(chatdCtx, func(tx *chatstate.Tx, store database.Store) error {
		_, err := tx.CommitStep(chatstate.CommitStepInput{
			Messages: []chatstate.Message{{
				Role:           database.ChatMessageRoleAssistant,
				Content:        assistantContent,
				Visibility:     database.ChatMessageVisibilityBoth,
				ModelConfigID:  uuid.NullUUID{UUID: chat.LastModelConfigID, Valid: true},
				ContentVersion: chatprompt.CurrentContentVersion,
			}},
		})
		if err != nil {
			return err
		}
		_, err = tx.EnterRequiresAction(chatstate.EnterRequiresActionInput{})
		return err
	}))
	return toolCallID
}

// TestPostChatsStartsRunning verifies the RFC-mandated `running`
// initial status surfaced by the create-chat endpoint.
func TestPostChatsStartsRunning(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, api := newChatClientWithAPI(t, withChatWorkerDisabled)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	_ = createChatModelConfig(t, client)

	chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: firstUser.OrganizationID,
		Content: []codersdk.ChatInputPart{{
			Type: codersdk.ChatInputPartTypeText,
			Text: "hello",
		}},
	})
	require.NoError(t, err)
	require.Equal(t, codersdk.ChatStatusRunning, chat.Status,
		"new chats must start in `running` per chatd RFC")

	// Re-reading also reports `running` because the chat row is
	// authoritative and no worker has advanced it.
	gotChat, err := client.GetChat(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, codersdk.ChatStatusRunning, gotChat.Status)
	require.NotNil(t, api.Pubsub)
}

// TestArchiveChatStateTransitions covers the two RFC-mandated archive
// behaviors at the endpoint contract level: archiving from an idle
// chat (W) succeeds, and archiving from an active chat (R0) returns
// a state conflict and leaves the chat unarchived.
func TestArchiveChatStateTransitions(t *testing.T) {
	t.Parallel()

	t.Run("IdleSucceeds", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, api := newChatClientWithAPI(t, withChatWorkerDisabled)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content:        []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "archive me"}},
		})
		require.NoError(t, err)

		driveChatToWaiting(ctx, t, api, chat.ID)

		err = client.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{Archived: ptr.Ref(true)})
		require.NoError(t, err)

		got, err := client.GetChat(ctx, chat.ID)
		require.NoError(t, err)
		require.True(t, got.Archived)
	})

	t.Run("ActiveChatReturnsConflict", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t, withChatWorkerDisabled)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content:        []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "no archive"}},
		})
		require.NoError(t, err)

		err = client.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{Archived: ptr.Ref(true)})
		requireSDKError(t, err, http.StatusConflict)

		got, err := client.GetChat(ctx, chat.ID)
		require.NoError(t, err)
		require.False(t, got.Archived, "active chat must remain unarchived after a conflict")
	})
}

// TestPostChatMessagesBusyInterrupt verifies that a busy-interrupt
// send returns a queued response and leaves the chat in `interrupting`
// from the endpoint's perspective.
func TestPostChatMessagesBusyInterrupt(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client := newChatClient(t, withChatWorkerDisabled)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	_ = createChatModelConfig(t, client)

	chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: firstUser.OrganizationID,
		Content:        []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "hello"}},
	})
	require.NoError(t, err)
	require.Equal(t, codersdk.ChatStatusRunning, chat.Status)

	// CreateChat leaves the chat in `running`; an interrupt-style
	// follow-up should land it in `interrupting`.
	resp, err := client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
		Content:      []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "stop"}},
		BusyBehavior: codersdk.ChatBusyBehaviorInterrupt,
	})
	require.NoError(t, err)
	require.True(t, resp.Queued, "busy interrupt must return queued=true")
	require.NotNil(t, resp.QueuedMessage)

	got, err := client.GetChat(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, codersdk.ChatStatusInterrupting, got.Status,
		"busy interrupt send must land the chat in `interrupting`")
}

// TestDeleteChatQueuedMessageMissingReturns404 covers the new
// chatstate-driven 404 path for missing queued IDs. The chat must
// have at least one queued message so the request is in a state where
// DeleteQueuedMessage is allowed; the looked-up ID then mismatches
// and the endpoint returns 404 instead of a state-conflict 409.
func TestDeleteChatQueuedMessageMissingReturns404(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client := newChatClient(t, withChatWorkerDisabled)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	_ = createChatModelConfig(t, client)

	chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: firstUser.OrganizationID,
		Content:        []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "hello"}},
	})
	require.NoError(t, err)

	// Seed one queued message via the public endpoint (the chat
	// starts in R0, so a queue send lands in R1).
	_, err = client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
		Content:      []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "queued"}},
		BusyBehavior: codersdk.ChatBusyBehaviorQueue,
	})
	require.NoError(t, err)

	res, err := client.Request(
		ctx,
		http.MethodDelete,
		fmt.Sprintf("/api/experimental/chats/%s/queue/99999999", chat.ID),
		nil,
	)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusNotFound, res.StatusCode)
}

// TestDeleteChatQueuedMessageEmptyQueueReturnsConflict covers the
// state-conflict 409 path when the chat has no queued messages.
func TestDeleteChatQueuedMessageEmptyQueueReturnsConflict(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client := newChatClient(t, withChatWorkerDisabled)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	_ = createChatModelConfig(t, client)

	chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: firstUser.OrganizationID,
		Content:        []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "hello"}},
	})
	require.NoError(t, err)

	res, err := client.Request(
		ctx,
		http.MethodDelete,
		fmt.Sprintf("/api/experimental/chats/%s/queue/99999999", chat.ID),
		nil,
	)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusConflict, res.StatusCode)
}

// TestPromoteChatQueuedMessageMissingReturns404 mirrors the delete
// test for the promote endpoint: with a non-empty queue, an unknown
// queued-message ID returns 404 rather than a 409.
func TestPromoteChatQueuedMessageMissingReturns404(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client := newChatClient(t, withChatWorkerDisabled)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	_ = createChatModelConfig(t, client)

	chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: firstUser.OrganizationID,
		Content:        []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "hello"}},
	})
	require.NoError(t, err)

	// Seed one queued message so the promote transition is allowed.
	_, err = client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
		Content:      []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "queued"}},
		BusyBehavior: codersdk.ChatBusyBehaviorQueue,
	})
	require.NoError(t, err)

	res, err := client.Request(
		ctx,
		http.MethodPost,
		fmt.Sprintf("/api/experimental/chats/%s/queue/99999999/promote", chat.ID),
		nil,
	)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusNotFound, res.StatusCode)
}

// TestPromoteChatQueuedMessageEmptyQueueReturnsConflict verifies the
// state-conflict 409 path when the chat has no queued messages.
func TestPromoteChatQueuedMessageEmptyQueueReturnsConflict(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client := newChatClient(t, withChatWorkerDisabled)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	_ = createChatModelConfig(t, client)

	chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: firstUser.OrganizationID,
		Content:        []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "hello"}},
	})
	require.NoError(t, err)

	res, err := client.Request(
		ctx,
		http.MethodPost,
		fmt.Sprintf("/api/experimental/chats/%s/queue/99999999/promote", chat.ID),
		nil,
	)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusConflict, res.StatusCode)
}

// TestInterruptChatIdleReturnsConflict verifies that interrupting an
// idle chat is now rejected. The fixture composes chatstate
// transitions to reach the W state without depending on the
// background worker.
func TestInterruptChatIdleReturnsConflict(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, api := newChatClientWithAPI(t, withChatWorkerDisabled)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	_ = createChatModelConfig(t, client)

	chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: firstUser.OrganizationID,
		Content:        []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "interrupt me"}},
	})
	require.NoError(t, err)

	driveChatToWaiting(ctx, t, api, chat.ID)

	_, err = client.InterruptChat(ctx, chat.ID)
	requireSDKError(t, err, http.StatusConflict)
}

// TestSubmitToolResultsWrongStateReturnsConflict covers the wrong
// chat-status response when the chat is not in requires_action.
func TestSubmitToolResultsWrongStateReturnsConflict(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client := newChatClient(t, withChatWorkerDisabled)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	_ = createChatModelConfig(t, client)

	chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: firstUser.OrganizationID,
		Content:        []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "hello"}},
	})
	require.NoError(t, err)
	require.Equal(t, codersdk.ChatStatusRunning, chat.Status)

	err = client.SubmitToolResults(ctx, chat.ID, codersdk.SubmitToolResultsRequest{
		Results: []codersdk.ToolResult{{
			ToolCallID: "unknown-call",
			Output:     json.RawMessage(`{}`),
		}},
	})
	requireSDKError(t, err, http.StatusConflict)
}

// TestSubmitToolResultsRequiresActionSucceeds drives a chat into
// requires_action with a single dynamic tool call and verifies a
// matching SubmitToolResults call returns 204 with the tool result
// persisted.
func TestSubmitToolResultsRequiresActionSucceeds(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, api := newChatClientWithAPI(t, withChatWorkerDisabled)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	_ = createChatModelConfig(t, client)

	dynamicTools := []codersdk.DynamicTool{{
		Name:        "echo",
		Description: "test echo tool",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}}
	chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID:     firstUser.OrganizationID,
		Content:            []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "hello"}},
		UnsafeDynamicTools: dynamicTools,
	})
	require.NoError(t, err)

	toolCallID := driveChatToRequiresAction(ctx, t, api, chat, "echo")

	err = client.SubmitToolResults(ctx, chat.ID, codersdk.SubmitToolResultsRequest{
		Results: []codersdk.ToolResult{{
			ToolCallID: toolCallID,
			Output:     json.RawMessage(`{"ok":true}`),
		}},
	})
	require.NoError(t, err)

	// The tool result must be persisted as a visible tool message.
	got, err := client.GetChatMessages(ctx, chat.ID, nil)
	require.NoError(t, err)
	foundToolResult := false
	for _, msg := range got.Messages {
		if msg.Role != codersdk.ChatMessageRoleTool {
			continue
		}
		for _, part := range msg.Content {
			if part.Type == codersdk.ChatMessagePartTypeToolResult && part.ToolCallID == toolCallID {
				foundToolResult = true
				break
			}
		}
	}
	require.True(t, foundToolResult, "tool result message must be visible in chat history")
}

// TestPatchChatArchiveChildRejected verifies that PATCH /api/experimental/chats/{child}
// with archived=true returns the root-only error regardless of the
// child's current archived value, and does not change archive state on
// any family member.
func TestPatchChatArchiveChildRejected(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, db, api := newChatClientWithAPIAndDatabase(t, withChatWorkerDisabled)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	modelConfig := createChatModelConfig(t, client)

	root, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: firstUser.OrganizationID,
		Content:        []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "root"}},
	})
	require.NoError(t, err)
	driveChatToWaiting(ctx, t, api, root.ID)

	// Sibling child A and B; both unarchived.
	childA := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    firstUser.OrganizationID,
		OwnerID:           firstUser.UserID,
		LastModelConfigID: modelConfig.ID,
		Title:             "child-a",
		Status:            database.ChatStatusWaiting,
		ParentChatID:      uuid.NullUUID{UUID: root.ID, Valid: true},
		RootChatID:        uuid.NullUUID{UUID: root.ID, Valid: true},
	})
	childB := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    firstUser.OrganizationID,
		OwnerID:           firstUser.UserID,
		LastModelConfigID: modelConfig.ID,
		Title:             "child-b",
		Status:            database.ChatStatusWaiting,
		ParentChatID:      uuid.NullUUID{UUID: root.ID, Valid: true},
		RootChatID:        uuid.NullUUID{UUID: root.ID, Valid: true},
	})

	err = client.UpdateChat(ctx, childA.ID, codersdk.UpdateChatRequest{Archived: ptr.Ref(true)})
	requireSDKError(t, err, http.StatusBadRequest)

	for _, id := range []uuid.UUID{root.ID, childA.ID, childB.ID} {
		got, gerr := loadChatRow(ctx, db, id)
		require.NoError(t, gerr)
		require.False(t, got.Archived, "no family member may flip archive state after a rejected child archive")
	}
}

// TestPatchChatUnarchiveChildRejected verifies that PATCH /api/experimental/chats/{child}
// with archived=false on an archived family is rejected with the
// root-only error and leaves every family member archived. The child
// already matches the requested value? No, the family is archived;
// we are asking to unarchive a child individually.
func TestPatchChatUnarchiveChildRejected(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, db, api := newChatClientWithAPIAndDatabase(t, withChatWorkerDisabled)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	modelConfig := createChatModelConfig(t, client)

	root, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: firstUser.OrganizationID,
		Content:        []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "root"}},
	})
	require.NoError(t, err)
	driveChatToWaiting(ctx, t, api, root.ID)

	childA := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    firstUser.OrganizationID,
		OwnerID:           firstUser.UserID,
		LastModelConfigID: modelConfig.ID,
		Title:             "child-a",
		Status:            database.ChatStatusWaiting,
		ParentChatID:      uuid.NullUUID{UUID: root.ID, Valid: true},
		RootChatID:        uuid.NullUUID{UUID: root.ID, Valid: true},
	})
	childB := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    firstUser.OrganizationID,
		OwnerID:           firstUser.UserID,
		LastModelConfigID: modelConfig.ID,
		Title:             "child-b",
		Status:            database.ChatStatusWaiting,
		ParentChatID:      uuid.NullUUID{UUID: root.ID, Valid: true},
		RootChatID:        uuid.NullUUID{UUID: root.ID, Valid: true},
	})

	// Archive the whole family via the root.
	err = client.UpdateChat(ctx, root.ID, codersdk.UpdateChatRequest{Archived: ptr.Ref(true)})
	require.NoError(t, err)
	for _, id := range []uuid.UUID{root.ID, childA.ID, childB.ID} {
		got, gerr := loadChatRow(ctx, db, id)
		require.NoError(t, gerr)
		require.True(t, got.Archived, "precondition: family archived after root archive")
	}

	// Unarchiving a child must be rejected.
	err = client.UpdateChat(ctx, childA.ID, codersdk.UpdateChatRequest{Archived: ptr.Ref(false)})
	requireSDKError(t, err, http.StatusBadRequest)

	for _, id := range []uuid.UUID{root.ID, childA.ID, childB.ID} {
		got, gerr := loadChatRow(ctx, db, id)
		require.NoError(t, gerr)
		require.True(t, got.Archived, "no family member may flip archive state after a rejected child unarchive")
	}
}

// TestPatchChatArchiveRootRollsBackWhenChildCannotArchive verifies the
// family-archive atomicity guarantee surfaced through the endpoint:
// when a child is in a state that rejects SetArchived (running here),
// the whole cascade rolls back and no family member changes archive
// state.
func TestPatchChatArchiveRootRollsBackWhenChildCannotArchive(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, db, api := newChatClientWithAPIAndDatabase(t, withChatWorkerDisabled)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	modelConfig := createChatModelConfig(t, client)

	root, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: firstUser.OrganizationID,
		Content:        []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "root"}},
	})
	require.NoError(t, err)
	driveChatToWaiting(ctx, t, api, root.ID)

	// Child is running (R0) which is NOT archive-eligible.
	child := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    firstUser.OrganizationID,
		OwnerID:           firstUser.UserID,
		LastModelConfigID: modelConfig.ID,
		Title:             "child",
		Status:            database.ChatStatusRunning,
		ParentChatID:      uuid.NullUUID{UUID: root.ID, Valid: true},
		RootChatID:        uuid.NullUUID{UUID: root.ID, Valid: true},
	})

	err = client.UpdateChat(ctx, root.ID, codersdk.UpdateChatRequest{Archived: ptr.Ref(true)})
	requireSDKError(t, err, http.StatusConflict)

	for _, id := range []uuid.UUID{root.ID, child.ID} {
		got, gerr := loadChatRow(ctx, db, id)
		require.NoError(t, gerr)
		require.False(t, got.Archived, "rolled-back family archive must not leave any member archived")
	}
}

// TestPostChatMessagesInvalidStateReturnsSharedResponse drives a chat
// into the chatstate-invalid state (waiting with a queued backlog)
// and asserts the shared invalid-state response. This is the
// representative endpoint required by the review.
func TestPostChatMessagesInvalidStateReturnsSharedResponse(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, _, api := newChatClientWithAPIAndDatabase(t, withChatWorkerDisabled)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	_ = createChatModelConfig(t, client)

	chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: firstUser.OrganizationID,
		Content:        []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "hello"}},
	})
	require.NoError(t, err)

	// Drive the chat to an invalid combination: status=waiting (W),
	// archived=false, and a queued message. ClassifyExecutionState
	// returns StateInvalid for (waiting, queue=true).
	driveChatToInvalidWaitingWithQueue(ctx, t, api, chat.ID)

	_, err = client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
		Content: []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "send"}},
	})
	sdkErr := requireSDKError(t, err, http.StatusConflict)
	require.Equal(t, "Chat is in an invalid state.", sdkErr.Message,
		"invalid-state endpoint response uses the shared message")
}

// TestPostChatToolResultsInvalidStateReturnsSharedResponse drives a
// chat into the chatstate-invalid state and asserts that the tool
// results endpoint returns the shared invalid-state response instead
// of the old "Chat is not waiting for tool results." status-conflict
// message. This locks the fix that removes the endpoint fast-path
// and routes invalid chats through the chatstate-backed transaction.
func TestPostChatToolResultsInvalidStateReturnsSharedResponse(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, _, api := newChatClientWithAPIAndDatabase(t, withChatWorkerDisabled)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	_ = createChatModelConfig(t, client)

	chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: firstUser.OrganizationID,
		Content:        []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "hello"}},
	})
	require.NoError(t, err)

	// Drive the chat to an invalid combination so the tool-results
	// endpoint must surface the shared invalid-state response rather
	// than the requires_action status conflict.
	driveChatToInvalidWaitingWithQueue(ctx, t, api, chat.ID)

	err = client.SubmitToolResults(ctx, chat.ID, codersdk.SubmitToolResultsRequest{
		Results: []codersdk.ToolResult{{
			ToolCallID: "call-irrelevant",
			Output:     json.RawMessage(`{}`),
		}},
	})
	sdkErr := requireSDKError(t, err, http.StatusConflict)
	require.Equal(t, "Chat is in an invalid state.", sdkErr.Message,
		"tool-results invalid-state response uses the shared message")
}

// TestReconcileInvalidChatStateSucceeds drives a chat into the
// chatstate-invalid combination (waiting with a queued backlog) and
// verifies the reconcile endpoint moves it into a valid error state
// while preserving the queued message.
func TestReconcileInvalidChatStateSucceeds(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, db, api := newChatClientWithAPIAndDatabase(t, withChatWorkerDisabled)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	_ = createChatModelConfig(t, client)

	chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: firstUser.OrganizationID,
		Content:        []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "hello"}},
	})
	require.NoError(t, err)

	// Drive the chat to an invalid combination: status=waiting (W),
	// archived=false, with a queued message. ClassifyExecutionState
	// returns StateInvalid for (waiting, queue=true).
	driveChatToInvalidWaitingWithQueue(ctx, t, api, chat.ID)

	reconciled, err := client.ReconcileInvalidChatState(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, chat.ID, reconciled.ID)
	require.Equal(t, codersdk.ChatStatusError, reconciled.Status)

	// The persisted row must reflect a valid error state with the
	// queued message preserved (E1) and a populated last_error.
	persisted, err := loadChatRow(ctx, db, chat.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusError, persisted.Status)
	require.False(t, persisted.Archived)
	require.True(t, persisted.LastError.Valid)

	queueCount, err := db.CountChatQueuedMessages(dbauthz.AsChatd(ctx), chat.ID)
	require.NoError(t, err)
	require.Equal(t, int64(1), queueCount, "queued message is preserved by reconcile")
}

// TestReconcileInvalidChatStateNotInvalidReturnsConflict verifies that
// reconciling a chat that is in a valid execution state is rejected
// with a 409 conflict.
func TestReconcileInvalidChatStateNotInvalidReturnsConflict(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client := newChatClient(t, withChatWorkerDisabled)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	_ = createChatModelConfig(t, client)

	// A freshly created chat starts in the valid running state (R0).
	chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: firstUser.OrganizationID,
		Content:        []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "hello"}},
	})
	require.NoError(t, err)
	require.Equal(t, codersdk.ChatStatusRunning, chat.Status)

	_, err = client.ReconcileInvalidChatState(ctx, chat.ID)
	sdkErr := requireSDKError(t, err, http.StatusConflict)
	require.Equal(t, "Chat is not in an invalid state.", sdkErr.Message)
}

// TestReconcileInvalidChatStateNotFound verifies the reconcile
// endpoint returns 404 for a chat that does not exist.
func TestReconcileInvalidChatStateNotFound(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client := newChatClient(t, withChatWorkerDisabled)
	_ = coderdtest.CreateFirstUser(t, client.Client)

	_, err := client.ReconcileInvalidChatState(ctx, uuid.New())
	requireSDKError(t, err, http.StatusNotFound)
}

// loadChatRow reads a chat row directly through dbauthz.AsChatd so
// endpoint tests verify side effects with the daemon's narrower
// permission set.
func loadChatRow(ctx context.Context, db database.Store, id uuid.UUID) (database.Chat, error) {
	chatdCtx := dbauthz.AsChatd(ctx) //nolint:gocritic // Test fixture reads rows with chatd permissions.
	return db.GetChatByID(chatdCtx, id)
}

// driveChatToInvalidWaitingWithQueue forces a chat into the
// chatstate-invalid combination (status=waiting, archived=false,
// queue non-empty) by writing directly through the database. This is
// an intentional invalid fixture: chatstate transitions reject
// driving toward this combination, so AsChatd is not used here.
func driveChatToInvalidWaitingWithQueue(
	ctx context.Context,
	t *testing.T,
	api *coderd.API,
	chatID uuid.UUID,
) {
	t.Helper()
	sysCtx := dbauthz.AsSystemRestricted(ctx) //nolint:gocritic // Test fixture writes invalid combination by design.

	// Seed the queue with one row attributed to the chat owner. The
	// content is a minimal valid JSON payload; only the row's
	// presence matters for ClassifyExecutionState. The owner_id is
	// filled from the chat row by the SQL.
	rawContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageText("queued"),
	})
	require.NoError(t, err)
	_, err = api.Database.InsertChatQueuedMessage(sysCtx, database.InsertChatQueuedMessageParams{
		ChatID:        chatID,
		Content:       rawContent.RawMessage,
		ModelConfigID: uuid.NullUUID{},
	})
	require.NoError(t, err)

	// Flip the chat's status to waiting via a raw execution-state
	// update. This bypasses the transition matrix to produce the
	// (waiting, queued) invalid pairing.
	_, err = api.Database.UpdateChatExecutionState(sysCtx, database.UpdateChatExecutionStateParams{
		ID:       chatID,
		Status:   database.ChatStatusWaiting,
		Archived: false,
	})
	require.NoError(t, err)
}

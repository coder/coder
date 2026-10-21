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
	_ = createChatModel(t, client)

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
		_ = createChatModel(t, client)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content:        []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "archive me"}},
		})
		require.NoError(t, err)

		driveChatToWaiting(ctx, t, api, chat.ID)

		err = client.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{Archived: new(true)})
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
		_ = createChatModel(t, client)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content:        []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "no archive"}},
		})
		require.NoError(t, err)

		err = client.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{Archived: new(true)})
		requireSDKError(t, err, http.StatusConflict)

		got, err := client.GetChat(ctx, chat.ID)
		require.NoError(t, err)
		require.False(t, got.Archived, "active chat must remain unarchived after a conflict")
	})
}

func TestPostChatMessagesBusyBehavior(t *testing.T) {
	t.Parallel()

	client := newChatClient(t, withChatWorkerDisabled)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	_ = createChatModel(t, client)

	for _, tc := range []struct {
		name         string
		busyBehavior codersdk.ChatBusyBehavior
		wantRow      codersdk.ChatBusyBehavior
		wantDetail   string
	}{
		{name: "DefaultSteers", wantRow: codersdk.ChatBusyBehaviorSteer},
		{name: "Steer", busyBehavior: codersdk.ChatBusyBehaviorSteer, wantRow: codersdk.ChatBusyBehaviorSteer},
		{name: "Queue", busyBehavior: codersdk.ChatBusyBehaviorQueue, wantRow: codersdk.ChatBusyBehaviorQueue},
		{
			name:         "InterruptRejected",
			busyBehavior: "interrupt",
			wantDetail:   `Must be "steer" or "queue". To interrupt the chat, call POST /api/v2/chats/{chat}/interrupt and then send the message.`,
		},
		{name: "UnknownRejected", busyBehavior: "bogus", wantDetail: `Must be "steer" or "queue".`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)
			chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
				OrganizationID: firstUser.OrganizationID,
				Content:        []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "hello"}},
			})
			require.NoError(t, err)
			require.Equal(t, codersdk.ChatStatusRunning, chat.Status)

			resp, err := client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
				Content:      []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "follow up"}},
				BusyBehavior: tc.busyBehavior,
			})
			var wantQueued []codersdk.ChatBusyBehavior
			if tc.wantDetail != "" {
				sdkErr := requireSDKError(t, err, http.StatusBadRequest)
				require.Equal(t, "Invalid busy_behavior value.", sdkErr.Message)
				require.Equal(t, tc.wantDetail, sdkErr.Detail)
			} else {
				require.NoError(t, err)
				require.True(t, resp.Queued)
				require.NotNil(t, resp.QueuedMessage)
				require.Equal(t, tc.wantRow, resp.QueuedMessage.BusyBehavior)
				wantQueued = []codersdk.ChatBusyBehavior{tc.wantRow}
			}

			messages, err := client.GetChatMessages(ctx, chat.ID, nil)
			require.NoError(t, err)
			gotQueued := make([]codersdk.ChatBusyBehavior, 0, len(messages.QueuedMessages))
			for _, queued := range messages.QueuedMessages {
				gotQueued = append(gotQueued, queued.BusyBehavior)
			}
			require.ElementsMatch(t, wantQueued, gotQueued)

			got, err := client.GetChat(ctx, chat.ID)
			require.NoError(t, err)
			require.Equal(t, codersdk.ChatStatusRunning, got.Status)
		})
	}
}

// Missing-ID cases need a non-empty queue to pass the transition check.
func TestChatQueueEndpointErrors(t *testing.T) {
	t.Parallel()

	client := newChatClient(t, withChatWorkerDisabled)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	_ = createChatModel(t, client)

	for _, tc := range []struct {
		name       string
		method     string
		pathSuffix string
		queueFirst bool
		wantStatus int
	}{
		{name: "DeleteMissingReturns404", method: http.MethodDelete, queueFirst: true, wantStatus: http.StatusNotFound},
		{name: "DeleteEmptyQueueReturnsConflict", method: http.MethodDelete, wantStatus: http.StatusConflict},
		{name: "PromoteMissingReturns404", method: http.MethodPost, pathSuffix: "/promote", queueFirst: true, wantStatus: http.StatusNotFound},
		{name: "PromoteEmptyQueueReturnsConflict", method: http.MethodPost, pathSuffix: "/promote", wantStatus: http.StatusConflict},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)
			chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
				OrganizationID: firstUser.OrganizationID,
				Content:        []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "hello"}},
			})
			require.NoError(t, err)
			var queued *codersdk.ChatQueuedMessage
			if tc.queueFirst {
				sent, err := client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
					Content:      []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "queued"}},
					BusyBehavior: codersdk.ChatBusyBehaviorQueue,
				})
				require.NoError(t, err)
				require.NotNil(t, sent.QueuedMessage)
				queued = sent.QueuedMessage
			}

			res, err := client.Request(ctx, tc.method, fmt.Sprintf("/api/v2/chats/%s/queue/99999999%s", chat.ID, tc.pathSuffix), nil)
			require.NoError(t, err)
			defer res.Body.Close()
			require.Equal(t, tc.wantStatus, res.StatusCode)

			if queued != nil {
				got, err := client.GetChatMessages(ctx, chat.ID, nil)
				require.NoError(t, err)
				require.Len(t, got.QueuedMessages, 1)
				require.Equal(t, queued.ID, got.QueuedMessages[0].ID)
			}
		})
	}
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
	_ = createChatModel(t, client)

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
	_ = createChatModel(t, client)

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
	_ = createChatModel(t, client)

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

func TestPatchChatChildArchive(t *testing.T) {
	t.Parallel()

	client, db, api := newChatClientWithAPIAndDatabase(t, withChatWorkerDisabled)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	modelConfig := createChatModel(t, client)

	createFamily := func(ctx context.Context, t *testing.T, childStatuses ...database.ChatStatus) []uuid.UUID {
		t.Helper()
		root, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content:        []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "root"}},
		})
		require.NoError(t, err)
		driveChatToWaiting(ctx, t, api, root.ID)

		family := []uuid.UUID{root.ID}
		for _, status := range childStatuses {
			child := dbgen.Chat(t, db, database.Chat{
				OrganizationID:    firstUser.OrganizationID,
				OwnerID:           firstUser.UserID,
				LastModelConfigID: modelConfig.ID,
				Status:            status,
				ParentChatID:      uuid.NullUUID{UUID: root.ID, Valid: true},
				RootChatID:        uuid.NullUUID{UUID: root.ID, Valid: true},
			})
			family = append(family, child.ID)
		}
		return family
	}
	requireArchived := func(ctx context.Context, t *testing.T, family []uuid.UUID, want bool, msg string) {
		t.Helper()
		for _, id := range family {
			got, err := loadChatRow(ctx, db, id)
			require.NoError(t, err)
			require.Equal(t, want, got.Archived, msg)
		}
	}

	t.Run("ArchiveChildRejected", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		family := createFamily(ctx, t, database.ChatStatusWaiting, database.ChatStatusWaiting)

		err := client.UpdateChat(ctx, family[1], codersdk.UpdateChatRequest{Archived: new(true)})
		requireSDKError(t, err, http.StatusBadRequest)
		requireArchived(ctx, t, family, false, "no family member may flip archive state after a rejected child archive")
	})

	t.Run("UnarchiveChildRejected", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		family := createFamily(ctx, t, database.ChatStatusWaiting, database.ChatStatusWaiting)
		err := client.UpdateChat(ctx, family[0], codersdk.UpdateChatRequest{Archived: new(true)})
		require.NoError(t, err)
		requireArchived(ctx, t, family, true, "precondition: family archived after root archive")

		err = client.UpdateChat(ctx, family[1], codersdk.UpdateChatRequest{Archived: new(false)})
		requireSDKError(t, err, http.StatusBadRequest)
		requireArchived(ctx, t, family, true, "no family member may flip archive state after a rejected child unarchive")
	})

	t.Run("RootArchiveRollsBackWhenChildCannotArchive", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		family := createFamily(ctx, t, database.ChatStatusRunning)

		err := client.UpdateChat(ctx, family[0], codersdk.UpdateChatRequest{Archived: new(true)})
		requireSDKError(t, err, http.StatusConflict)
		requireArchived(ctx, t, family, false, "rolled-back family archive must not leave any member archived")
	})
}

func TestInvalidChatStateEndpoints(t *testing.T) {
	t.Parallel()

	client, db, api := newChatClientWithAPIAndDatabase(t, withChatWorkerDisabled)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	_ = createChatModel(t, client)

	createChat := func(ctx context.Context, t *testing.T) codersdk.Chat {
		t.Helper()
		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content:        []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "hello"}},
		})
		require.NoError(t, err)
		return chat
	}

	for _, tc := range []struct {
		name string
		call func(ctx context.Context, chatID uuid.UUID) error
	}{
		{
			name: "PostChatMessagesReturnsSharedResponse",
			call: func(ctx context.Context, chatID uuid.UUID) error {
				_, err := client.CreateChatMessage(ctx, chatID, codersdk.CreateChatMessageRequest{
					Content: []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "send"}},
				})
				return err
			},
		},
		{
			name: "PostChatToolResultsReturnsSharedResponse",
			call: func(ctx context.Context, chatID uuid.UUID) error {
				return client.SubmitToolResults(ctx, chatID, codersdk.SubmitToolResultsRequest{
					Results: []codersdk.ToolResult{{ToolCallID: "call-irrelevant", Output: json.RawMessage(`{}`)}},
				})
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)
			chat := createChat(ctx, t)
			driveChatToInvalidWaitingWithQueue(ctx, t, api, chat.ID)

			sdkErr := requireSDKError(t, tc.call(ctx, chat.ID), http.StatusConflict)
			require.Equal(t, "Chat is in an invalid state.", sdkErr.Message)
		})
	}

	t.Run("ReconcileSucceeds", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		chat := createChat(ctx, t)
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
	})

	t.Run("ReconcileNotInvalidReturnsConflict", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		// A freshly created chat starts in the valid running state (R0).
		chat := createChat(ctx, t)
		require.Equal(t, codersdk.ChatStatusRunning, chat.Status)

		_, err := client.ReconcileInvalidChatState(ctx, chat.ID)
		sdkErr := requireSDKError(t, err, http.StatusConflict)
		require.Equal(t, "Chat is not in an invalid state.", sdkErr.Message)
	})

	t.Run("ReconcileNotFound", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		_, err := client.ReconcileInvalidChatState(ctx, uuid.New())
		requireSDKError(t, err, http.StatusNotFound)
	})
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

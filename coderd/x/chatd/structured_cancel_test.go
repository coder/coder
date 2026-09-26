package chatd_test

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstructured"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// Deleting a queued structured request while an ordinary turn waits on a
// dynamic tool records the receipt for REST and stream readers without
// disturbing the paused turn.
func TestDeleteQueuedStructuredRequestDuringToolWait(t *testing.T) {
	t.Parallel()
	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	var streamed atomic.Int32
	var mu sync.Mutex
	var resumed []chattest.OpenAIMessage
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		if streamed.Add(1) == 1 {
			return chattest.OpenAIStreamingResponse(chattest.OpenAIToolCallChunk("wait_tool", `{}`))
		}
		mu.Lock()
		resumed = append([]chattest.OpenAIMessage(nil), req.Messages...)
		mu.Unlock()
		return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("turn done")...)
	})
	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	factory := chattest.NewMockAIBridgeTransport(t, openAIURL)
	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(factory)
	})
	tools, err := json.Marshal([]mcp.Tool{{Name: "wait_tool", InputSchema: map[string]any{"type": "object"}}})
	require.NoError(t, err)
	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID, OwnerID: user.ID, Title: "tool-wait", ModelConfigID: model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("call the tool")},
		DynamicTools:       tools,
	})
	require.NoError(t, err)

	// The ordinary turn pauses on the dynamic tool call.
	var paused database.Chat
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		paused, err = db.GetChatByID(ctx, chat.ID)
		return err == nil && (paused.Status == database.ChatStatusRequiresAction || paused.Status == database.ChatStatusError)
	}, testutil.IntervalFast)
	require.Equal(t, database.ChatStatusRequiresAction, paused.Status, chatLastErrorMessage(paused.LastError))
	history, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: chat.ID})
	require.NoError(t, err)
	parts, err := chatprompt.ParseContent(history[len(history)-1])
	require.NoError(t, err)
	toolCallID := parts[0].ToolCallID

	// Queue a structured request behind the paused turn, then delete it.
	requestID := uuid.New()
	request, err := chatstructured.EncodeRequestPart(chatstructured.Request{RequestID: requestID, Name: "report", Schema: json.RawMessage(`{}`)})
	require.NoError(t, err)
	content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{codersdk.ChatMessageText("structured"), request})
	require.NoError(t, err)
	require.NoError(t, chatstate.NewChatMachine(db, ps, chat.ID).Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		_, err := tx.SendMessage(chatstate.SendMessageInput{BusyBehavior: chatstate.BusyBehaviorQueue, Message: chatstate.Message{
			Role: database.ChatMessageRoleUser, Visibility: database.ChatMessageVisibilityBoth, Content: content,
			ContentVersion: chatprompt.CurrentContentVersion, CreatedBy: uuid.NullUUID{UUID: user.ID, Valid: true},
			ModelConfigID: uuid.NullUUID{UUID: model.ID, Valid: true},
		}})
		return err
	}))
	queued, err := db.GetChatQueuedMessagesByPosition(ctx, chat.ID)
	require.NoError(t, err)
	_, events, cancel, ok := server.Subscribe(ctx, chat.ID, nil, 0)
	require.True(t, ok)
	defer cancel()
	require.NoError(t, server.DeleteQueued(ctx, chat.ID, queued[0].ID))

	// REST reads the receipt row; the stream delivers it live while the turn
	// is still paused with its history epoch and attempt unchanged.
	history, err = db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: chat.ID})
	require.NoError(t, err)
	receipt := history[len(history)-1]
	outcome, err := chatstate.ReceiptRowOutcome(receipt)
	require.NoError(t, err)
	require.Equal(t, requestID, outcome.RequestID)
	for streamedReceipt := false; !streamedReceipt; {
		select {
		case event := <-events:
			streamedReceipt = event.Type == codersdk.ChatStreamEventTypeMessage && event.Message != nil && event.Message.ID == receipt.ID
		case <-ctx.Done():
			t.Fatal("receipt never reached the stream")
		}
	}
	stillPaused, err := db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusRequiresAction, stillPaused.Status)
	require.Equal(t, paused.HistoryVersion, stillPaused.HistoryVersion)
	require.Equal(t, paused.GenerationAttempt, stillPaused.GenerationAttempt)

	// The receipt hides neither the pending call nor anything the model sees.
	require.NoError(t, server.SubmitToolResults(ctx, chatd.SubmitToolResultsOptions{
		ChatID: chat.ID, UserID: user.ID, ModelConfigID: paused.LastModelConfigID,
		Results: []codersdk.ToolResult{{ToolCallID: toolCallID, Output: json.RawMessage(`{"ok":true}`)}},
	}))
	var done database.Chat
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		done, err = db.GetChatByID(ctx, chat.ID)
		return err == nil && (done.Status == database.ChatStatusWaiting || done.Status == database.ChatStatusError)
	}, testutil.IntervalFast)
	require.Equal(t, database.ChatStatusWaiting, done.Status, chatLastErrorMessage(done.LastError))
	require.EqualValues(t, 2, streamed.Load())
	mu.Lock()
	defer mu.Unlock()
	for _, msg := range resumed {
		require.NotContains(t, msg.Content, string(codersdk.ChatStructuredOutputErrorCodeQueueDeleted))
	}
	final, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: chat.ID, AfterID: receipt.ID})
	require.NoError(t, err)
	require.Len(t, final, 2)
	require.Equal(t, database.ChatMessageRoleTool, final[0].Role)
	text, err := chatprompt.ParseContent(final[1])
	require.NoError(t, err)
	require.Contains(t, text[0].Text, "turn done")
}

package coderd_test

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstructured"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// REST and the stream project structured output request IDs and receipts
// identically, and a promoted queued message keeps its request ID.
func TestChatMessagesProjectStructuredOutput(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	client, db := newChatClientWithDatabase(t, withChatWorkerDisabled)
	user := coderdtest.CreateFirstUser(t, client.Client)
	modelConfig := createChatModel(t, client)
	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID: user.OrganizationID, OwnerID: user.UserID, LastModelConfigID: modelConfig.ID,
		Title: "structured output projection", Status: database.ChatStatusError,
	})
	requestID := uuid.New()
	request, err := chatstructured.EncodeRequestPart(chatstructured.Request{RequestID: requestID, Name: "report", Schema: json.RawMessage(`{}`)})
	require.NoError(t, err)
	content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{codersdk.ChatMessageText("question"), request})
	require.NoError(t, err)
	outcome := codersdk.ChatStructuredOutput{RequestID: requestID, Status: codersdk.ChatStructuredOutputStatusSucceeded, Value: json.RawMessage(`{"a":1}`)}
	receiptParts, err := chatstructured.ReceiptParts(outcome)
	require.NoError(t, err)
	receiptContent, err := chatprompt.MarshalParts(receiptParts)
	require.NoError(t, err)
	userRow := dbgen.ChatMessage(t, db, database.ChatMessage{ChatID: chat.ID, Role: database.ChatMessageRoleUser, Content: content})
	receiptRow := dbgen.ChatMessage(t, db, database.ChatMessage{
		ChatID: chat.ID, Role: database.ChatMessageRoleAssistant, Visibility: database.ChatMessageVisibilityUser,
		ContentVersion: chatprompt.ContentVersionV1, Content: receiptContent,
	})
	queued := insertTestChatQueuedMessage(ctx, t, db, chat.ID, content.RawMessage, modelConfig.ID)

	rest, err := client.GetChatMessages(ctx, chat.ID, nil)
	require.NoError(t, err)
	restJSON := make(map[int64]string)
	for _, msg := range rest.Messages {
		encoded, err := json.Marshal(msg)
		require.NoError(t, err)
		restJSON[msg.ID] = string(encoded)
		switch msg.ID {
		case userRow.ID:
			require.Equal(t, &requestID, msg.StructuredOutputRequestID)
		case receiptRow.ID:
			require.NotNil(t, msg.StructuredOutput)
			require.Equal(t, outcome.Status, msg.StructuredOutput.Status)
			require.JSONEq(t, string(outcome.Value), string(msg.StructuredOutput.Value))
		}
	}
	require.Len(t, rest.QueuedMessages, 1)
	require.Equal(t, &requestID, rest.QueuedMessages[0].StructuredOutputRequestID)
	restQueued, err := json.Marshal(rest.QueuedMessages)
	require.NoError(t, err)

	events, closer, err := client.StreamChat(ctx, chat.ID, nil)
	require.NoError(t, err)
	defer closer.Close()
	streamed := make(map[int64]string)
	var streamedQueue string
	for streamed[userRow.ID] == "" || streamed[receiptRow.ID] == "" || streamedQueue == "" {
		select {
		case <-ctx.Done():
			require.FailNow(t, "timed out waiting for the stream snapshot")
		case event, ok := <-events:
			require.True(t, ok, "stream closed before the snapshot")
			if event.Message != nil {
				encoded, err := json.Marshal(event.Message)
				require.NoError(t, err)
				streamed[event.Message.ID] = string(encoded)
			}
			if event.Type == codersdk.ChatStreamEventTypeQueueUpdate {
				encoded, err := json.Marshal(event.QueuedMessages)
				require.NoError(t, err)
				streamedQueue = string(encoded)
			}
		}
	}
	for _, id := range []int64{userRow.ID, receiptRow.ID} {
		require.JSONEq(t, restJSON[id], streamed[id])
	}
	require.JSONEq(t, string(restQueued), streamedQueue)

	res, err := client.Request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/chats/%s/queue/%d/promote", chat.ID, queued.ID), nil)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusAccepted, res.StatusCode)
	promoted, err := client.GetChatMessages(ctx, chat.ID, nil)
	require.NoError(t, err)
	var promotedIDs []*uuid.UUID
	for _, msg := range promoted.Messages {
		if msg.ID > receiptRow.ID && msg.Role == codersdk.ChatMessageRoleUser {
			promotedIDs = append(promotedIDs, msg.StructuredOutputRequestID)
		}
	}
	require.Equal(t, []*uuid.UUID{&requestID}, promotedIDs)
}

func withStructuredOutput(o *coderdtest.Options) {
	o.DeploymentValues.Experiments = append(o.DeploymentValues.Experiments, string(codersdk.ExperimentChatStructuredOutput))
}

// Create requests with a response format are validated before any side
// effect, run a full structured turn, and lock the formatted input.
func TestPostChatsResponseFormat(t *testing.T) {
	t.Parallel()
	format := &codersdk.ChatResponseFormat{Type: codersdk.ChatResponseFormatTypeJSONSchema, JSONSchema: &codersdk.ChatResponseFormatJSONSchema{
		Name: "report", Schema: json.RawMessage(`{"type":"object","properties":{"v":{"type":"string"}},"required":["v"]}`),
	}}
	create := func(org uuid.UUID, format *codersdk.ChatResponseFormat, plan codersdk.ChatPlanMode) codersdk.CreateChatRequest {
		return codersdk.CreateChatRequest{
			OrganizationID: org, Content: []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "q"}},
			ResponseFormat: format, PlanMode: plan,
		}
	}

	t.Run("Disabled", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t, withChatWorkerDisabled)
		user := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModel(t, client)
		_, err := client.CreateChat(ctx, create(user.OrganizationID, format, ""))
		require.Equal(t, "response_format", requireSDKError(t, err, http.StatusBadRequest).Validations[0].Field)
		_, err = client.CreateChat(ctx, create(user.OrganizationID, &codersdk.ChatResponseFormat{Type: codersdk.ChatResponseFormatTypeText}, ""))
		require.NoError(t, err)
		chats, err := client.ListChats(ctx, nil)
		require.NoError(t, err)
		require.Len(t, chats, 1)
	})

	t.Run("Rejected", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t, withChatWorkerDisabled, withStructuredOutput)
		user := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModel(t, client)
		for extra, field := range map[string]string{
			`{"type":"text","strict":true}`:                                   "response_format",
			`{"type":"json_schema","json_schema":{"name":"a b","schema":{}}}`: "response_format.json_schema.name",
		} {
			res, err := client.Request(ctx, http.MethodPost, "/api/v2/chats", json.RawMessage(fmt.Sprintf(`{"organization_id":%q,"content":[{"type":"text","text":"q"}],"response_format":%s}`, user.OrganizationID, extra)))
			require.NoError(t, err)
			sdkErr := requireSDKError(t, codersdk.ReadBodyAsError(res), http.StatusBadRequest)
			require.Equal(t, []codersdk.ValidationError{{Field: field, Detail: sdkErr.Validations[0].Detail}}, sdkErr.Validations)
			_ = res.Body.Close()
		}
		_, err := client.CreateChat(ctx, create(user.OrganizationID, format, codersdk.ChatPlanModePlan))
		require.Equal(t, "response_format", requireSDKError(t, err, http.StatusBadRequest).Validations[0].Field)
		chats, err := client.ListChats(ctx, nil)
		require.NoError(t, err)
		require.Empty(t, chats)
	})

	t.Run("Completes", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		var streamed atomic.Int32
		baseURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
			if !req.Stream {
				return chattest.OpenAINonStreamingResponse("title")
			}
			if streamed.Add(1) == 1 {
				return chattest.OpenAIStreamingResponse(chattest.OpenAIToolCallChunk(chatstructured.FinalizerToolName, `{"output":{"v":"ok"}}`))
			}
			return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
		})
		client := newChatClient(t, withStructuredOutput)
		user := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelWithBaseURL(t, client, baseURL)
		chat, err := client.CreateChat(ctx, create(user.OrganizationID, format, ""))
		require.NoError(t, err)
		var messages codersdk.ChatMessagesResponse
		testutil.Eventually(ctx, t, func(ctx context.Context) bool {
			messages, err = client.GetChatMessages(ctx, chat.ID, nil)
			return err == nil && slices.ContainsFunc(messages.Messages, func(m codersdk.ChatMessage) bool { return m.StructuredOutput != nil })
		}, testutil.IntervalFast)
		var requestID *uuid.UUID
		var out *codersdk.ChatStructuredOutput
		for _, msg := range messages.Messages {
			requestID, out = cmp.Or(requestID, msg.StructuredOutputRequestID), cmp.Or(out, msg.StructuredOutput)
		}
		require.NotNil(t, requestID)
		require.Equal(t, *requestID, out.RequestID)
		require.Equal(t, codersdk.ChatStructuredOutputStatusSucceeded, out.Status)
		require.JSONEq(t, `{"v":"ok"}`, string(out.Value))
	})

	t.Run("LocksFormattedInput", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t, withChatWorkerDisabled, withStructuredOutput)
		user := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModel(t, client)
		chat, err := client.CreateChat(ctx, create(user.OrganizationID, format, ""))
		require.NoError(t, err)
		messages, err := client.GetChatMessages(ctx, chat.ID, nil)
		require.NoError(t, err)
		idx := slices.IndexFunc(messages.Messages, func(m codersdk.ChatMessage) bool { return m.StructuredOutputRequestID != nil })
		require.GreaterOrEqual(t, idx, 0)
		plan := codersdk.ChatPlanModePlan
		requireSDKError(t, client.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{PlanMode: &plan}), http.StatusConflict)
		_, err = client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
			Content: []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "more"}}, PlanMode: &plan,
		})
		requireSDKError(t, err, http.StatusConflict)
		_, err = client.EditChatMessage(ctx, chat.ID, messages.Messages[idx].ID, codersdk.EditChatMessageRequest{
			Content: []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "edited"}},
		})
		requireSDKError(t, err, http.StatusConflict)
	})
}

package coderd_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstructured"
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

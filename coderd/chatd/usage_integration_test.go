package chatd_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/chatd"
	"github.com/coder/coder/v2/coderd/chatd/chattest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestPersistStepNormalizesCachedInput(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	replica := chatd.New(chatd.Config{
		Logger:                     logger,
		Database:                   db,
		ReplicaID:                  uuid.New(),
		Pubsub:                     ps,
		PendingChatAcquireInterval: 10 * time.Millisecond,
		InFlightChatStaleAfter:     testutil.WaitSuperLong,
	})
	t.Cleanup(func() {
		require.NoError(t, replica.Close())
	})

	usage := chattest.OpenAICompletionUsage{
		PromptTokens:     1000,
		CompletionTokens: 200,
		TotalTokens:      1200,
		PromptTokensDetails: &chattest.OpenAIPromptTokensDetails{
			CachedTokens: 900,
		},
	}
	response := chattest.OpenAINonStreamingResponseWithUsage("cached reply", usage)

	openAIServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Helper()
		if r.URL.Path != "/responses" {
			http.NotFound(w, r)
			return
		}

		var req chattest.OpenAIRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))

		if !req.Stream {
			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(response.Response))
			return
		}

		flusher, ok := w.(http.Flusher)
		require.True(t, ok)
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)

		writeSSE := func(v any) {
			t.Helper()
			data, err := json.Marshal(v)
			require.NoError(t, err)
			_, err = fmt.Fprintf(w, "data: %s\n\n", data)
			require.NoError(t, err)
			flusher.Flush()
		}

		itemID := "msg_cached_input"
		writeSSE(responses.ResponseOutputItemAddedEvent{
			OutputIndex: 0,
			Item: responses.ResponseOutputItemUnion{
				ID:   itemID,
				Type: "message",
			},
		})
		writeSSE(map[string]any{
			"type":          "response.output_text.delta",
			"item_id":       itemID,
			"output_index":  0,
			"content_index": 0,
			"delta":         response.Response.Choices[0].Message.Content,
		})
		writeSSE(responses.ResponseTextDoneEvent{
			ItemID:      itemID,
			OutputIndex: 0,
			Text:        response.Response.Choices[0].Message.Content,
		})
		writeSSE(responses.ResponseOutputItemDoneEvent{
			OutputIndex: 0,
			Item: responses.ResponseOutputItemUnion{
				ID:   itemID,
				Type: "message",
			},
		})
		writeSSE(responses.ResponseCompletedEvent{
			Response: responses.Response{
				Usage: responses.ResponseUsage{
					InputTokens:  int64(response.Response.Usage.PromptTokens),
					OutputTokens: int64(response.Response.Usage.CompletionTokens),
					TotalTokens:  int64(response.Response.Usage.TotalTokens),
					InputTokensDetails: responses.ResponseUsageInputTokensDetails{
						CachedTokens: int64(response.Response.Usage.PromptTokensDetails.CachedTokens),
					},
				},
			},
		})
	}))
	t.Cleanup(openAIServer.Close)

	user, model := seedChatDependencies(ctx, t, db)
	setOpenAIProviderBaseURL(ctx, t, db, openAIServer.URL)

	chat, err := replica.CreateChat(ctx, chatd.CreateOptions{
		OwnerID:            user.ID,
		Title:              "cached-input",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	var assistant database.ChatMessage
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		fromDB, err := db.GetChatByID(ctx, chat.ID)
		if err != nil || fromDB.Status != database.ChatStatusWaiting || fromDB.WorkerID.Valid {
			return false
		}

		messages, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
			ChatID:  chat.ID,
			AfterID: 0,
		})
		if err != nil || len(messages) < 2 {
			return false
		}

		for _, message := range messages {
			if message.Role != database.ChatMessageRoleAssistant {
				continue
			}
			assistant = message
			return message.InputTokens.Valid && message.OutputTokens.Valid && message.CacheReadTokens.Valid
		}
		return false
	}, testutil.IntervalFast)

	require.True(t, assistant.InputTokens.Valid)
	require.True(t, assistant.OutputTokens.Valid)
	require.True(t, assistant.CacheReadTokens.Valid)
	require.True(t, assistant.TotalTokens.Valid)
	require.Equal(t, int64(100), assistant.InputTokens.Int64)
	require.Equal(t, int64(200), assistant.OutputTokens.Int64)
	require.Equal(t, int64(900), assistant.CacheReadTokens.Int64)
	require.Equal(t, int64(1200), assistant.TotalTokens.Int64)

	// Verify cost was calculated using normalized InputTokens, not raw.
	// seedChatDependencies stores an empty pricing config, so cost stays NULL.
	require.False(t, assistant.TotalCostMicros.Valid, "empty pricing config should yield NULL (unpriced) cost")
}

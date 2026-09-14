package chatd_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/x/agenthooks"
	"github.com/coder/coder/v2/testutil"
)

func TestStopHookNoOpFinishesTurn(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
	})
	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	var stopCalls atomic.Int32
	consumer := stopConsumer(t, func() (int, string) {
		stopCalls.Add(1)
		return http.StatusOK, `{}`
	})
	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
		cfg.HookDispatcher = newHookDispatcher(t, db, consumer)
	})
	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		Title:          "stop-noop",
		ModelConfigID:  model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("finish normally"),
		},
	})
	require.NoError(t, err)
	testutil.Eventually(ctx, t, func(context.Context) bool {
		return stopCalls.Load() == 1
	}, testutil.IntervalFast)
	waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)
	require.Equal(t, int32(1), stopCalls.Load())
}

func TestStopHookNudgeContinuesOnce(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	var modelCalls atomic.Int32
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		switch modelCalls.Add(1) {
		case 1:
			return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("first answer")...)
		case 2:
			var found bool
			for _, message := range req.Messages {
				found = found || strings.Contains(message.Content, "continue please")
			}
			require.True(t, found)
			return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("second answer")...)
		default:
			require.FailNow(t, "stop nudge exceeded continuation cap")
			return chattest.OpenAIStreamingResponse()
		}
	})
	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	var stopCalls atomic.Int32
	consumer := stopConsumer(t, func() (int, string) {
		stopCalls.Add(1)
		return http.StatusOK, `{"model_context":"continue please"}`
	})
	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
		cfg.HookDispatcher = newHookDispatcher(t, db, consumer)
	})
	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		Title:          "stop-nudge",
		ModelConfigID:  model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("continue once"),
		},
	})
	require.NoError(t, err)
	testutil.Eventually(ctx, t, func(context.Context) bool {
		return stopCalls.Load() == 2
	}, testutil.IntervalFast)
	waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)
	require.Equal(t, int32(2), modelCalls.Load())
	require.Equal(t, int32(2), stopCalls.Load())

	promptMessages, err := db.GetChatMessagesForPromptByChatID(ctx, chat.ID)
	require.NoError(t, err)
	var contextRows int
	for _, message := range promptMessages {
		parts, err := chatprompt.ParseContent(message)
		require.NoError(t, err)
		if len(parts) == 1 && parts[0].Text == "continue please" {
			contextRows++
			require.Equal(t, database.ChatMessageVisibilityModel, message.Visibility)
		}
	}
	require.Equal(t, 2, contextRows)
}

func TestStopHookDispatchFailureErrorsChat(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
	})
	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	consumer := stopConsumer(t, func() (int, string) {
		return http.StatusInternalServerError, ""
	})
	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
		cfg.HookDispatcher = newHookDispatcher(t, db, consumer)
	})
	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		Title:          "stop-failure",
		ModelConfigID:  model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("fail on stop"),
		},
	})
	require.NoError(t, err)
	failed := waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusError)
	lastError := chatLastErrorMessage(failed.LastError)
	require.Contains(t, lastError, "hook dispatch failed: stop: http_error")
}

func stopConsumer(t *testing.T, response func() (int, string)) *httptest.Server {
	t.Helper()
	consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request agenthooks.Request
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		if request.Type != agenthooks.EventStop {
			_, err := w.Write([]byte(`{}`))
			require.NoError(t, err)
			return
		}
		status, body := response()
		w.WriteHeader(status)
		if body != "" {
			_, err := w.Write([]byte(body))
			require.NoError(t, err)
		}
	}))
	t.Cleanup(consumer.Close)
	return consumer
}

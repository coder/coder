package chatd_test

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

type dropBlockSent struct {
	beta  bool
	field bool
}

type dropBlockRecorder struct {
	mu   sync.Mutex
	sent []dropBlockSent
}

func (r *dropBlockRecorder) record(req *chattest.AnthropicRequest) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sent = append(r.sent, dropBlockSent{
		beta:  strings.Contains(req.Header.Get("Anthropic-Beta"), "thinking-binding-controls-2026-08-01"),
		field: strings.Contains(string(req.Thinking), `"prefix_mismatch_behavior":"drop_block"`),
	})
	return len(r.sent)
}

func (r *dropBlockRecorder) snapshot() []dropBlockSent {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]dropBlockSent(nil), r.sent...)
}

func thinkingBindingError() chattest.AnthropicResponse {
	return chattest.AnthropicResponse{Error: &chattest.ErrorResponse{
		StatusCode: http.StatusBadRequest,
		Type:       "invalid_request_error",
		Message:    "messages.1.content.0: Invalid `signature` in `thinking` block. The block is bound to a different conversation.",
	}}
}

func TestThinkingBinding_RetriesWithDropBlock(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	var recorder dropBlockRecorder
	anthropicURL := chattest.NewAnthropic(t, func(req *chattest.AnthropicRequest) chattest.AnthropicResponse {
		if !req.Stream {
			return chattest.AnthropicNonStreamingResponse("title")
		}
		if recorder.record(req) == 1 {
			return thinkingBindingError()
		}
		return chattest.AnthropicStreamingResponse(chattest.AnthropicTextChunks("ok")...)
	})
	user, org, model := seedAnthropicChatDependencies(t, db, anthropicURL)
	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, anthropicURL, chattest.WithPreservePath()))
	})

	chat := createChatThroughServer(ctx, t, db, server, org.ID, user.ID, model.ID, "hello")
	latest := waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)
	require.False(t, latest.LastError.Valid)
	require.Equal(t, []dropBlockSent{{}, {beta: true, field: true}}, recorder.snapshot())

	_, err := server.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:    chat.ID,
		CreatedBy: user.ID,
		Content:   []codersdk.ChatMessagePart{codersdk.ChatMessageText("again")},
	})
	require.NoError(t, err)
	testutil.Eventually(ctx, t, func(context.Context) bool {
		return len(recorder.snapshot()) == 3
	}, testutil.IntervalFast)
	latest = waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)
	require.False(t, latest.LastError.Valid)
	require.Equal(t, []dropBlockSent{{}, {beta: true, field: true}, {beta: true, field: true}}, recorder.snapshot())
}

func TestThinkingBinding_RetriesOnlyOnce(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	var recorder dropBlockRecorder
	anthropicURL := chattest.NewAnthropic(t, func(req *chattest.AnthropicRequest) chattest.AnthropicResponse {
		if !req.Stream {
			return chattest.AnthropicNonStreamingResponse("title")
		}
		recorder.record(req)
		return thinkingBindingError()
	})
	user, org, model := seedAnthropicChatDependencies(t, db, anthropicURL)
	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, anthropicURL, chattest.WithPreservePath()))
	})

	chat := createChatThroughServer(ctx, t, db, server, org.ID, user.ID, model.ID, "hello")
	latest := waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusError)
	require.True(t, latest.LastError.Valid)
	require.Equal(t, []dropBlockSent{{}, {beta: true, field: true}}, recorder.snapshot())
}

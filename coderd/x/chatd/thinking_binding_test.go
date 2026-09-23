package chatd_test

import (
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

func TestThinkingBinding_RetriesOnceWithDropBlock(t *testing.T) {
	t.Parallel()

	type dropBlockSent struct {
		beta  bool
		field bool
	}

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	var (
		mu   sync.Mutex
		sent []dropBlockSent
	)
	anthropicURL := chattest.NewAnthropic(t, func(req *chattest.AnthropicRequest) chattest.AnthropicResponse {
		if !req.Stream {
			return chattest.AnthropicNonStreamingResponse("title")
		}
		mu.Lock()
		sent = append(sent, dropBlockSent{
			beta:  strings.Contains(req.Header.Get("Anthropic-Beta"), "thinking-binding-controls-2026-08-01"),
			field: strings.Contains(string(req.Thinking), `"prefix_mismatch_behavior":"drop_block"`),
		})
		n := len(sent)
		mu.Unlock()
		if n == 2 {
			return chattest.AnthropicStreamingResponse(chattest.AnthropicTextChunks("ok")...)
		}
		return chattest.AnthropicResponse{Error: &chattest.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Type:       "invalid_request_error",
			Message:    "messages.1.content.0: Invalid `signature` in `thinking` block. The block is bound to a different conversation.",
		}}
	})
	user, org, model := seedAnthropicChatDependencies(t, db, anthropicURL)
	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, anthropicURL, chattest.WithPreservePath()))
	})

	chat := createChatThroughServer(ctx, t, db, server, org.ID, user.ID, model.ID, "hello")
	latest := waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)
	require.False(t, latest.LastError.Valid)

	_, err := server.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:    chat.ID,
		CreatedBy: user.ID,
		Content:   []codersdk.ChatMessagePart{codersdk.ChatMessageText("again")},
	})
	require.NoError(t, err)
	latest = waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusError)
	require.True(t, latest.LastError.Valid)

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, []dropBlockSent{{}, {beta: true, field: true}, {beta: true, field: true}}, sent)
}

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
	"github.com/coder/coder/v2/testutil"
)

func TestThinkingBinding_RetriesWithDropBlock(t *testing.T) {
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
		n, dropBlock := len(sent), sent[len(sent)-1].field
		mu.Unlock()
		if n == 4 || n == 6 {
			return chattest.AnthropicStreamingResponse(chattest.AnthropicTextChunks("ok")...)
		}
		if dropBlock {
			return chattest.AnthropicResponse{Error: &chattest.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Type:       "invalid_request_error",
				Message:    "thinking.block_binding: Extra inputs are not permitted",
			}}
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

	// Chat 1's drop_block retry fails, so chat 2 still starts without
	// drop_block. Chat 2's retry succeeds, so chat 3 sends drop_block
	// first. That is rejected, and the retry without it succeeds and
	// clears the hint, so chat 4 starts without drop_block again.
	for _, want := range []database.ChatStatus{database.ChatStatusError, database.ChatStatusWaiting, database.ChatStatusWaiting, database.ChatStatusError} {
		chat := createChatThroughServer(ctx, t, db, server, org.ID, user.ID, model.ID, "hello")
		waitForChatStatus(ctx, t, db, chat.ID, want)
	}

	mu.Lock()
	defer mu.Unlock()
	withDropBlock := dropBlockSent{beta: true, field: true}
	require.Equal(t, []dropBlockSent{{}, withDropBlock, {}, withDropBlock, withDropBlock, {}, {}, withDropBlock}, sent)
}

package chatd

import (
	"net/http"
	"strings"
	"sync"
	"testing"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/testutil"
)

func TestThinkingDropBlockModel_RetriesWithEffort(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	requests := make(chan chattest.AnthropicRequest, 2)
	url := chattest.NewAnthropic(t, func(req *chattest.AnthropicRequest) chattest.AnthropicResponse {
		requests <- *req
		if len(requests) == 1 {
			return chattest.AnthropicResponse{Error: &chattest.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Type:       "invalid_request_error",
				Message:    "The block is bound to a different conversation.",
			}}
		}
		return chattest.AnthropicStreamingResponse(chattest.AnthropicTextChunks("ok")...)
	})
	client, err := fantasyanthropic.New(fantasyanthropic.WithAPIKey("test-key"), fantasyanthropic.WithBaseURL(url))
	require.NoError(t, err)
	lm, err := client.LanguageModel(ctx, "claude-opus-5-5")
	require.NoError(t, err)

	server := &Server{logger: slogtest.Make(t, nil)}
	model := server.withThinkingDropBlock(lm, uuid.New(), uuid.New())
	effort := fantasyanthropic.EffortHigh
	stream, err := model.Stream(ctx, fantasy.Call{
		Prompt:          fantasy.Prompt{fantasy.NewUserMessage("hello")},
		ProviderOptions: fantasy.ProviderOptions{fantasyanthropic.Name: &fantasyanthropic.ProviderOptions{Effort: &effort}},
	})
	require.NoError(t, err)
	for part := range stream {
		require.NotEqual(t, fantasy.StreamPartTypeError, part.Type, part.Error)
	}

	testutil.RequireReceive(ctx, t, requests)
	retry := testutil.RequireReceive(ctx, t, requests)
	require.JSONEq(t, `{"type":"adaptive","block_binding":{"prefix_mismatch_behavior":"drop_block"}}`, string(retry.Thinking))
	require.Equal(t, thinkingBindingBeta, retry.Header.Get("Anthropic-Beta"))
}

func TestThinkingDropBlockModel_UpdatesHint(t *testing.T) {
	t.Parallel()

	type dropBlockSent struct {
		beta  bool
		field bool
	}

	ctx := testutil.Context(t, testutil.WaitLong)
	var (
		mu   sync.Mutex
		sent []dropBlockSent
	)
	url := chattest.NewAnthropic(t, func(req *chattest.AnthropicRequest) chattest.AnthropicResponse {
		mu.Lock()
		sent = append(sent, dropBlockSent{
			beta:  strings.Contains(req.Header.Get("Anthropic-Beta"), thinkingBindingBeta),
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
	client, err := fantasyanthropic.New(fantasyanthropic.WithAPIKey("test-key"), fantasyanthropic.WithBaseURL(url))
	require.NoError(t, err)
	lm, err := client.LanguageModel(ctx, "claude-opus-5-5")
	require.NoError(t, err)

	server := &Server{logger: slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})}
	providerID := uuid.New()
	streamFails := func() bool {
		model := server.withThinkingDropBlock(lm, providerID, uuid.New())
		stream, err := model.Stream(ctx, fantasy.Call{
			Prompt: fantasy.Prompt{fantasy.NewUserMessage("hello")},
		})
		require.NoError(t, err)
		failed := false
		for part := range stream {
			failed = failed || part.Type == fantasy.StreamPartTypeError
		}
		return failed
	}

	// Call 1's drop_block retry fails, so call 2 still starts without
	// drop_block. Call 2's retry succeeds, so call 3 sends drop_block
	// first. That is rejected, and the retry without it succeeds and
	// clears the hint, so call 4 starts without drop_block again.
	for _, wantFailure := range []bool{true, false, false, true} {
		require.Equal(t, wantFailure, streamFails())
	}

	mu.Lock()
	defer mu.Unlock()
	withDropBlock := dropBlockSent{beta: true, field: true}
	require.Equal(t, []dropBlockSent{{}, withDropBlock, {}, withDropBlock, withDropBlock, {}, {}, withDropBlock}, sent)
}

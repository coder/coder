package chatd

import (
	"net/http"
	"testing"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
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
	model := server.withThinkingDropBlock(chatprovider.NewModel(lm, nil), uuid.New(), codersdk.ChatModelCallConfig{}, uuid.New())
	effort := fantasyanthropic.EffortHigh
	stream, err := model.LanguageModel().Stream(ctx, fantasy.Call{
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

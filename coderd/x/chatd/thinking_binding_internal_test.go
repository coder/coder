package chatd

import (
	"net/http"
	"strings"
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
	require.JSONEq(t, `{"type":"adaptive","display":"summarized","block_binding":{"prefix_mismatch_behavior":"drop_block"}}`, string(retry.Thinking))
	require.Equal(t, thinkingBindingBeta, retry.Header.Get("Anthropic-Beta"))
}

func TestThinkingDropBlockModel_HintedRetriesOnlyDropBlockRejections(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name          string
		rejection     string
		wantDropBlock []bool
		wantErr       bool
		wantHint      bool
	}{
		{
			name:          "OtherError",
			rejection:     "messages.9.content.43.image.source.base64.data: At least one of the image dimensions exceed max allowed size for many-image requests: 3000 pixels",
			wantDropBlock: []bool{true},
			wantErr:       true,
			wantHint:      true,
		},
		{
			name:          "BetaRejected",
			rejection:     "Unexpected value(s) `thinking-binding-controls-2026-08-01` for the `anthropic-beta` header. Please consult our documentation at platform.claude.com/docs or try again without the header.",
			wantDropBlock: []bool{true, false},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitShort)
			requests := make(chan chattest.AnthropicRequest, 2)
			url := chattest.NewAnthropic(t, func(req *chattest.AnthropicRequest) chattest.AnthropicResponse {
				requests <- *req
				if strings.Contains(string(req.Thinking), thinkingBindingDropBlock) {
					return chattest.AnthropicResponse{Error: &chattest.ErrorResponse{
						StatusCode: http.StatusBadRequest,
						Type:       "invalid_request_error",
						Message:    tc.rejection,
					}}
				}
				return chattest.AnthropicStreamingResponse(chattest.AnthropicTextChunks("ok")...)
			})
			client, err := fantasyanthropic.New(fantasyanthropic.WithAPIKey("test-key"), fantasyanthropic.WithBaseURL(url))
			require.NoError(t, err)
			lm, err := client.LanguageModel(ctx, "claude-opus-5-5")
			require.NoError(t, err)

			server := &Server{logger: slogtest.Make(t, nil)}
			key := thinkingDropBlockKey{providerID: uuid.New(), model: "claude-opus-5-5"}
			server.thinkingDropBlock.Store(key, struct{}{})
			model := server.withThinkingDropBlock(chatprovider.NewModel(lm, nil), key.providerID, codersdk.ChatModelCallConfig{}, uuid.New())
			stream, err := model.LanguageModel().Stream(ctx, fantasy.Call{
				Prompt: fantasy.Prompt{fantasy.NewUserMessage("hello")},
			})
			require.NoError(t, err)
			var streamErr error
			for part := range stream {
				if part.Type == fantasy.StreamPartTypeError {
					streamErr = part.Error
				}
			}

			if tc.wantErr {
				require.ErrorContains(t, streamErr, tc.rejection)
			} else {
				require.NoError(t, streamErr)
			}
			var dropBlock []bool
			for len(requests) > 0 {
				req := <-requests
				dropBlock = append(dropBlock, strings.Contains(string(req.Thinking), thinkingBindingDropBlock))
			}
			require.Equal(t, tc.wantDropBlock, dropBlock)
			_, hinted := server.thinkingDropBlock.Load(key)
			require.Equal(t, tc.wantHint, hinted)
		})
	}
}

package chatd

import (
	"testing"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/testutil"
)

func TestApplyThinkingDropBlock_WithEffort(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	requests := make(chan chattest.AnthropicRequest, 1)
	url := chattest.NewAnthropic(t, func(req *chattest.AnthropicRequest) chattest.AnthropicResponse {
		requests <- *req
		return chattest.AnthropicNonStreamingResponse("ok")
	})
	client, err := fantasyanthropic.New(fantasyanthropic.WithAPIKey("test-key"), fantasyanthropic.WithBaseURL(url))
	require.NoError(t, err)
	lm, err := client.LanguageModel(ctx, "claude-opus-5-5")
	require.NoError(t, err)

	effort := fantasyanthropic.EffortHigh
	resolved := resolvedModelCall{
		model:           chatprovider.NewModel(lm, nil),
		providerOptions: fantasy.ProviderOptions{fantasyanthropic.Name: &fantasyanthropic.ProviderOptions{Effort: &effort}},
	}
	resolved.applyThinkingDropBlock()
	call := resolved.newCall()
	call.Prompt = fantasy.Prompt{fantasy.NewUserMessage("hello")}
	_, err = lm.Generate(ctx, call)
	require.NoError(t, err)

	req := testutil.RequireReceive(ctx, t, requests)
	require.JSONEq(t, `{"type":"adaptive","block_binding":{"prefix_mismatch_behavior":"drop_block"}}`, string(req.Thinking))
	require.Equal(t, thinkingBindingBeta, req.Header.Get("Anthropic-Beta"))
}

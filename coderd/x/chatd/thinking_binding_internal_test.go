package chatd

import (
	"testing"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestApplyThinkingDropBlock(t *testing.T) {
	t.Parallel()

	const dropBlock = `"block_binding":{"prefix_mismatch_behavior":"drop_block"}`
	effort := fantasyanthropic.EffortHigh
	tests := []struct {
		name         string
		options      *fantasyanthropic.ProviderOptions
		callConfig   codersdk.ChatModelCallConfig
		wantThinking string
		wantBeta     string
	}{
		{
			name:         "Effort",
			options:      &fantasyanthropic.ProviderOptions{Effort: &effort},
			wantThinking: `{"type":"adaptive",` + dropBlock + `}`,
			wantBeta:     thinkingBindingBeta,
		},
		{
			name: "OmittedThinkingKeepsBetas",
			callConfig: codersdk.ChatModelCallConfig{
				ProviderOptions: &codersdk.ChatModelProviderOptions{
					Anthropic: &codersdk.ChatModelAnthropicProviderOptions{Context1MEnabled: ptr.Ref(true)},
				},
			},
			wantThinking: `{"type":"adaptive",` + dropBlock + `}`,
			wantBeta:     chatprovider.AnthropicBetaContext1M + "," + thinkingBindingBeta,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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

			resolved := resolvedModelCall{model: chatprovider.NewModel(lm, nil), clientCallConfig: tt.callConfig}
			if tt.options != nil {
				resolved.providerOptions = fantasy.ProviderOptions{fantasyanthropic.Name: tt.options}
			}
			resolved.applyThinkingDropBlock()
			call := resolved.newCall()
			call.Prompt = fantasy.Prompt{fantasy.NewUserMessage("hello")}
			_, err = lm.Generate(ctx, call)
			require.NoError(t, err)

			req := testutil.RequireReceive(ctx, t, requests)
			require.JSONEq(t, tt.wantThinking, string(req.Thinking))
			require.Equal(t, tt.wantBeta, req.Header.Get("Anthropic-Beta"))
		})
	}
}

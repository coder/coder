package chatprovider_test

import (
	"encoding/json"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
)

// Exercise the production provider construction and pinned serializer. Budget
// translation is valid legacy behavior but cannot fulfill an exact effort.
func TestAnthropicExactEffortWire(t *testing.T) {
	t.Parallel()
	for _, modelID := range []string{"claude-opus-4-5", "claude-sonnet-4-5", "claude-haiku-4-5"} {
		t.Run(modelID, func(t *testing.T) {
			t.Parallel()
			for _, effort := range []string{"high"} {
				t.Run(effort, func(t *testing.T) {
					t.Parallel()
					requests := make(chan *chattest.AnthropicRequest, 1)
					url := chattest.NewAnthropic(t, func(req *chattest.AnthropicRequest) chattest.AnthropicResponse {
						requests <- req
						return chattest.AnthropicNonStreamingResponse("done")
					})
					config := codersdk.ChatModelCallConfig{ReasoningEffort: effortConfig("high", "high")}
					model, err := chatprovider.ModelFromConfig("anthropic", modelID, chatprovider.ProviderAPIKeys{
						ByProvider:        map[string]string{"anthropic": "test-key"},
						BaseURLByProvider: map[string]string{"anthropic": url},
					}, chatprovider.UserAgent(), nil, nil, &config)
					require.NoError(t, err)
					_, err = model.LanguageModel().Generate(t.Context(), fantasy.Call{
						Prompt:          []fantasy.Message{{Role: fantasy.MessageRoleUser, Content: []fantasy.MessagePart{fantasy.TextPart{Text: "hello"}}}},
						MaxOutputTokens: new(int64(20000)),
						ProviderOptions: chatprovider.ProviderOptionsForCall(model, config, &effort),
					})
					require.NoError(t, err)
					request := <-requests
					var output struct {
						Effort string `json:"effort"`
					}
					if len(request.OutputConfig) != 0 {
						require.NoError(t, json.Unmarshal(request.OutputConfig, &output))
					}
					resolved, err := chatprovider.ResolveExactReasoningEffort("anthropic", modelID, config, &effort)
					if modelID == "claude-opus-4-5" {
						require.Equal(t, effort, output.Effort)
						require.NoError(t, err)
						require.Equal(t, effort, *resolved)
					} else {
						require.Empty(t, output.Effort)
						var thinking struct {
							Type   string `json:"type"`
							Budget int64  `json:"budget_tokens"`
						}
						require.NoError(t, json.Unmarshal(request.Thinking, &thinking))
						require.Equal(t, "enabled", thinking.Type)
						require.Positive(t, thinking.Budget)
						require.ErrorContains(t, err, "cannot be applied exactly")
						require.Nil(t, resolved)
						require.Empty(t, chatprovider.ExactReasoningEfforts("anthropic", modelID, config))
					}
				})
			}
		})
	}
}

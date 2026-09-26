package chatd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
)

func TestResolveModelCallReasoningModeWire(t *testing.T) {
	t.Parallel()
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("Stream=%t", stream), func(t *testing.T) {
			t.Parallel()
			db := dbmock.NewMockStore(gomock.NewController(t))
			provider := aibridgeTestAIProvider(uuid.New(), "primary-openai", database.AIProviderTypeOpenai)
			db.EXPECT().GetAIProviderByID(gomock.Any(), provider.ID).Return(provider, nil).AnyTimes()
			db.EXPECT().GetAIProviders(gomock.Any(), gomock.Any()).Return([]database.AIProvider{provider}, nil).AnyTimes()
			server := titleOverrideTestServer(db, slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}))
			seen := make(chan map[string]any, 1)
			factory := &aibridgeTestFactory{rt: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				require.Equal(t, "/v1/responses", req.URL.Path)
				var body map[string]any
				require.NoError(t, json.NewDecoder(req.Body).Decode(&body))
				seen <- body
				response := &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body: io.NopCloser(strings.NewReader(
						`{"id":"resp_test","object":"response","status":"completed","model":"gpt-5.6-sol","output":[{"id":"msg_test","type":"message","role":"assistant","content":[{"type":"output_text","text":"hello"}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`)),
					Request: req,
				}
				if streaming, _ := body["stream"].(bool); streaming {
					raw, err := io.ReadAll(response.Body)
					require.NoError(t, err)
					event, err := json.Marshal(map[string]any{"type": "response.completed", "response": json.RawMessage(raw)})
					require.NoError(t, err)
					response.Body = io.NopCloser(strings.NewReader(fmt.Sprintf("data: %s\n\n", event)))
					response.Header.Set("Content-Type", "text/event-stream")
				}
				return response, nil
			})}
			server.aibridgeTransportFactory = aibridgeTestFactoryPointer(factory)
			chat := database.Chat{ID: uuid.New(), OwnerID: uuid.New()}
			var baseline map[string]any
			for _, mode := range []string{"", "pro", ""} {
				config := titleOverrideModelConfig("gpt-5.6-sol", true)
				config.AIProviderID = uuid.NullUUID{UUID: provider.ID, Valid: true}
				callConfig := codersdk.ChatModelCallConfig{
					ReasoningEffort: &codersdk.ChatModelReasoningEffortConfig{Default: ptr.Ref(codersdk.ChatModelReasoningEffortHigh), Max: ptr.Ref(codersdk.ChatModelReasoningEffortHigh)},
					ProviderOptions: &codersdk.ChatModelProviderOptions{OpenAI: &codersdk.ChatModelOpenAIProviderOptions{
						ReasoningSummary: ptr.Ref("detailed"), ServiceTier: ptr.Ref("priority"),
					}},
				}
				if mode != "" {
					callConfig.ProviderOptions.OpenAI.ReasoningMode = ptr.Ref(mode)
				}
				var err error
				config.Options, err = json.Marshal(callConfig)
				require.NoError(t, err)
				chat.LastModelConfigID = config.ID
				db.EXPECT().GetEnabledChatModelConfigByID(gomock.Any(), config.ID).Return(config, nil)
				resolved, err := server.resolveModelCall(t.Context(), modelCallSpec{
					purpose: "chat", chat: chat, requestedEffort: ptr.Ref("medium"), buildOptions: modelBuildOptions{ActiveAPIKeyID: uuid.NewString()},
				})
				require.NoError(t, err)
				call := resolved.newCall()
				call.Prompt = []fantasy.Message{{Role: fantasy.MessageRoleUser, Content: []fantasy.MessagePart{fantasy.TextPart{Text: "hello"}}}}
				if stream {
					parts, err := resolved.model.LanguageModel().Stream(t.Context(), call)
					require.NoError(t, err)
					finished := false
					for part := range parts {
						require.NoError(t, part.Error)
						finished = finished || part.Type == fantasy.StreamPartTypeFinish
					}
					require.True(t, finished)
				} else {
					_, err = resolved.model.LanguageModel().Generate(t.Context(), call)
					require.NoError(t, err)
				}
				got := <-seen
				reasoning, ok := got["reasoning"].(map[string]any)
				require.True(t, ok)
				require.Equal(t, "medium", reasoning["effort"])
				require.NotContains(t, got, "reasoning.mode")
				require.Equal(t, "detailed", reasoning["summary"])
				require.Equal(t, "priority", got["service_tier"])
				require.Equal(t, config.Model, got["model"])
				if mode == "" {
					require.NotContains(t, reasoning, "mode")
				} else {
					require.Equal(t, mode, reasoning["mode"])
				}
				delete(reasoning, "mode")
				if baseline == nil {
					baseline = got
				} else {
					require.Equal(t, baseline, got)
				}

				if mode == "pro" {
					modelProvider, modelName, ok := chattool.DefaultComputerUseModel(codersdk.ChatComputerUseProviderOpenAI)
					require.True(t, ok)
					fixed, err := server.resolveModelCall(t.Context(), modelCallSpec{
						purpose: "computer_use", chat: chat,
						fixedModel:   &fixedModelCall{providerType: modelProvider, modelName: modelName, callConfig: callConfig},
						buildOptions: modelBuildOptions{ActiveAPIKeyID: uuid.NewString()},
					})
					require.NoError(t, err)
					fixedCall := fixed.newCall()
					fixedCall.Prompt = call.Prompt
					_, err = fixed.model.LanguageModel().Generate(t.Context(), fixedCall)
					require.NoError(t, err)
					got := <-seen
					reasoning, ok := got["reasoning"].(map[string]any)
					require.True(t, ok)
					require.NotContains(t, reasoning, "mode")
					require.Equal(t, modelName, got["model"])
				}
			}
		})
	}
}

package chatd

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"charm.land/fantasy"
	fantasyopenai "charm.land/fantasy/providers/openai"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/mock/gomock"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func modelCallSentinelOptions(t *testing.T, user string) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(codersdk.ChatModelCallConfig{
		ProviderOptions: &codersdk.ChatModelProviderOptions{
			OpenAI: &codersdk.ChatModelOpenAIProviderOptions{
				User: ptr.Ref(user),
			},
		},
	})
	require.NoError(t, err)
	return raw
}

// The transport decides which of the two OpenAI option shapes derivation
// produces, so both are accepted.
func requireOpenAIUserOption(t *testing.T, options fantasy.ProviderOptions, user string) {
	t.Helper()
	switch opts := options[fantasyopenai.Name].(type) {
	case *fantasyopenai.ResponsesProviderOptions:
		require.NotNil(t, opts.User)
		require.Equal(t, user, *opts.User)
	case *fantasyopenai.ProviderOptions:
		require.NotNil(t, opts.User)
		require.Equal(t, user, *opts.User)
	default:
		t.Fatalf("unexpected openai provider options type %T", opts)
	}
}

func requireOpenAIReasoningEffort(t *testing.T, options fantasy.ProviderOptions, effort string) {
	t.Helper()
	switch opts := options[fantasyopenai.Name].(type) {
	case *fantasyopenai.ResponsesProviderOptions:
		require.NotNil(t, opts.ReasoningEffort)
		require.Equal(t, fantasyopenai.ReasoningEffort(effort), *opts.ReasoningEffort)
	case *fantasyopenai.ProviderOptions:
		require.NotNil(t, opts.ReasoningEffort)
		require.Equal(t, fantasyopenai.ReasoningEffort(effort), *opts.ReasoningEffort)
	default:
		t.Fatalf("unexpected openai provider options type %T", opts)
	}
}

func TestResolveModelCallDerivesProviderOptions(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	chat, _ := titleOverrideTestChatAndMessages(t)
	providerID := uuid.New()
	config := titleOverrideModelConfig("gpt-4o-mini", true)
	config.AIProviderID = uuid.NullUUID{UUID: providerID, Valid: true}
	config.Options = modelCallSentinelOptions(t, "summary-options-sentinel")
	chat.LastModelConfigID = config.ID

	db.EXPECT().GetEnabledChatModelConfigByID(gomock.Any(), config.ID).Return(config, nil)
	db.EXPECT().GetAIProviderByID(gomock.Any(), providerID).Return(aibridgeTestAIProvider(providerID, "primary-openai", database.AIProviderTypeOpenai), nil).AnyTimes()
	db.EXPECT().GetAIProviderKeysByProviderID(gomock.Any(), providerID).Return([]database.AIProviderKey{{
		ProviderID: providerID,
		APIKey:     "test-key",
	}}, nil).AnyTimes()

	server := titleOverrideTestServer(db, logger)
	resolved, err := server.resolveModelCall(ctx, modelCallSpec{
		purpose:      "chat_summary",
		chat:         chat,
		buildOptions: modelBuildOptions{ActiveAPIKeyID: uuid.NewString()},
	})
	require.NoError(t, err)
	requireOpenAIUserOption(t, resolved.providerOptions, "summary-options-sentinel")
	requireOpenAIUserOption(t, summaryObjectCall(resolved).ProviderOptions, "summary-options-sentinel")
}

func TestResolveModelCallStageModel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		providerType    database.AIProviderType
		reasoningEffort *codersdk.ChatModelReasoningEffortConfig
		requestedEffort *string
		wantEffort      string
	}{
		{
			name:            "ClampedToMax",
			providerType:    database.AIProviderTypeOpenai,
			reasoningEffort: &codersdk.ChatModelReasoningEffortConfig{Default: ptr.Ref("low"), Max: ptr.Ref("medium")},
			requestedEffort: ptr.Ref("high"),
			wantEffort:      "medium",
		},
		{
			name:         "EmptyWithoutConfig",
			providerType: database.AIProviderTypeOpenai,
			wantEffort:   "",
		},
		{
			// The model name resolves to openai, so the provider type
			// must come from the configured provider.
			name:            "CopilotProviderType",
			providerType:    database.AIProviderTypeCopilot,
			reasoningEffort: &codersdk.ChatModelReasoningEffortConfig{Default: ptr.Ref("low")},
			wantEffort:      "low",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitShort)
			ctrl := gomock.NewController(t)
			db := dbmock.NewMockStore(ctrl)
			logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
			chat, _ := titleOverrideTestChatAndMessages(t)
			providerID := uuid.New()
			config := titleOverrideModelConfig("gpt-5", true)
			config.AIProviderID = uuid.NullUUID{UUID: providerID, Valid: true}
			options, err := json.Marshal(codersdk.ChatModelCallConfig{ReasoningEffort: tt.reasoningEffort})
			require.NoError(t, err)
			config.Options = options
			chat.LastModelConfigID = config.ID

			db.EXPECT().GetEnabledChatModelConfigByID(gomock.Any(), config.ID).Return(config, nil)
			db.EXPECT().GetAIProviderByID(gomock.Any(), providerID).Return(aibridgeTestAIProvider(providerID, "primary", tt.providerType), nil).AnyTimes()
			db.EXPECT().GetAIProviderKeysByProviderID(gomock.Any(), providerID).Return([]database.AIProviderKey{{
				ProviderID: providerID,
				APIKey:     "test-key",
			}}, nil).AnyTimes()

			tracer, recorder := newStageTestTracer(t)
			server := titleOverrideTestServer(db, logger)
			server.stages = tracer
			server.aibridgeTransportFactory = aibridgeTestFactoryPointer(&aibridgeTestFactory{
				rt: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: http.StatusBadRequest,
						Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"rejected"}}`)),
						Request:    req,
					}, nil
				}),
			})
			resolved, err := server.resolveModelCall(ctx, modelCallSpec{
				purpose:         "chat_turn",
				chat:            chat,
				requestedEffort: tt.requestedEffort,
				buildOptions:    modelBuildOptions{ActiveAPIKeyID: uuid.NewString()},
			})
			require.NoError(t, err)
			require.Equal(t, tt.wantEffort, resolved.resolvedEffort)
			// The wire provider is the one the built client reports.
			require.NotEmpty(t, resolved.model.Provider())
			wantStageModel := chatloop.StageModel{Provider: resolved.model.Provider(), ProviderType: string(tt.providerType), Model: "gpt-5", Effort: tt.wantEffort}
			require.Equal(t, wantStageModel, resolved.stageModel())
			if tt.providerType == database.AIProviderTypeOpenai && tt.wantEffort != "" {
				requireOpenAIReasoningEffort(t, resolved.providerOptions, tt.wantEffort)
			}

			_, err = resolved.model.LanguageModel().Generate(ctx, fantasy.Call{Prompt: []fantasy.Message{{
				Role: fantasy.MessageRoleUser, Content: []fantasy.MessagePart{fantasy.TextPart{Text: "hello"}},
			}}})
			require.Error(t, err)
			ended := recorder.Ended()
			require.Len(t, ended, 1)
			require.Equal(t, string(chatloop.StageProviderAttempt), ended[0].Name())
			require.Contains(t, ended[0].Attributes(), attribute.String(chatloop.AttrProvider, wantStageModel.Provider))
			require.Contains(t, ended[0].Attributes(), attribute.String(chatloop.AttrProviderType, wantStageModel.ProviderType))
			require.Contains(t, ended[0].Attributes(), attribute.String(chatloop.AttrModel, wantStageModel.Model))
			if wantStageModel.Effort != "" {
				require.Contains(t, ended[0].Attributes(), attribute.String(chatloop.AttrReasoningEffort, wantStageModel.Effort))
			}
		})
	}
}

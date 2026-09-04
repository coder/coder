package chatd

import (
	"encoding/json"
	"testing"

	"charm.land/fantasy"
	fantasyopenai "charm.land/fantasy/providers/openai"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
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

func TestResolveModelCallResolvedEffort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		reasoningEffort *codersdk.ChatModelReasoningEffortConfig
		requestedEffort *string
		wantEffort      string
	}{
		{
			name:            "ClampedToMax",
			reasoningEffort: &codersdk.ChatModelReasoningEffortConfig{Default: ptr.Ref("low"), Max: ptr.Ref("medium")},
			requestedEffort: ptr.Ref("high"),
			wantEffort:      "medium",
		},
		{
			name:       "EmptyWithoutConfig",
			wantEffort: "",
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
			db.EXPECT().GetAIProviderByID(gomock.Any(), providerID).Return(aibridgeTestAIProvider(providerID, "primary-openai", database.AIProviderTypeOpenai), nil).AnyTimes()
			db.EXPECT().GetAIProviderKeysByProviderID(gomock.Any(), providerID).Return([]database.AIProviderKey{{
				ProviderID: providerID,
				APIKey:     "test-key",
			}}, nil).AnyTimes()

			server := titleOverrideTestServer(db, logger)
			resolved, err := server.resolveModelCall(ctx, modelCallSpec{
				purpose:         "chat_turn",
				chat:            chat,
				requestedEffort: tt.requestedEffort,
				buildOptions:    modelBuildOptions{ActiveAPIKeyID: uuid.NewString()},
			})
			require.NoError(t, err)
			require.Equal(t, tt.wantEffort, resolved.resolvedEffort)
			require.Equal(t, chatloop.StageModel{Model: "gpt-5", Effort: tt.wantEffort}, resolved.stageModel())
			if tt.wantEffort != "" {
				requireOpenAIReasoningEffort(t, resolved.providerOptions, tt.wantEffort)
			}
		})
	}
}

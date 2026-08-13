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

	db.EXPECT().GetChatModelConfigByID(gomock.Any(), config.ID).Return(config, nil)
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

package chatd

import (
	"encoding/json"
	"testing"

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

// modelCallSentinelOptions builds config options whose OpenAI user field acts
// as a sentinel: its presence in a request proves provider options were
// derived from the config, and its absence proves they were omitted.
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

// TestChatModelSpecOmitsProviderOptions locks the historical omission for
// whole-chat summaries and status labels: the resolver must not derive
// provider options even when the config declares them.
func TestChatModelSpecOmitsProviderOptions(t *testing.T) {
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
	resolved, err := server.resolveModelCall(ctx, chatModelSpec("chat_summary", chat, modelBuildOptions{ActiveAPIKeyID: uuid.NewString()}))
	require.NoError(t, err)
	require.Nil(t, resolved.providerOptions)
	require.Nil(t, summaryObjectCall(resolved).ProviderOptions)
}

package chatd

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/testutil"
)

func TestResolveSummaryGenerationModelOverride_Unset(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	chat, _ := titleOverrideTestChatAndMessages(t)

	db.EXPECT().GetChatSummaryGenerationModelOverride(gomock.Any()).Return("", nil)

	server := titleOverrideTestServer(db, logger)
	config, model, _, _, overrideSet, err := server.resolveSummaryGenerationModelOverride(
		ctx,
		chat,
		chatprovider.ProviderAPIKeys{ByProvider: map[string]string{"openai": "test-key"}},
		modelBuildOptions{},
	)
	require.NoError(t, err)
	require.False(t, overrideSet)
	require.Nil(t, model)
	require.Equal(t, database.ChatModelConfig{}, config)
}

func TestResolveSummaryGenerationModelOverride_MalformedFallsThrough(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	chat, _ := titleOverrideTestChatAndMessages(t)

	db.EXPECT().GetChatSummaryGenerationModelOverride(gomock.Any()).Return("not-a-uuid", nil)

	server := titleOverrideTestServer(db, logger)
	config, model, _, _, overrideSet, err := server.resolveSummaryGenerationModelOverride(
		ctx,
		chat,
		chatprovider.ProviderAPIKeys{ByProvider: map[string]string{"openai": "test-key"}},
		modelBuildOptions{},
	)
	require.NoError(t, err)
	require.False(t, overrideSet)
	require.Nil(t, model)
	require.Equal(t, database.ChatModelConfig{}, config)
}

func TestResolveSummaryGenerationModelOverride_SetUsable(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	chat, _ := titleOverrideTestChatAndMessages(t)
	overrideConfig := titleOverrideModelConfig("gpt-4.1", true)

	db.EXPECT().GetChatSummaryGenerationModelOverride(gomock.Any()).Return(overrideConfig.ID.String(), nil)
	db.EXPECT().GetChatModelConfigByID(gomock.Any(), overrideConfig.ID).Return(overrideConfig, nil)
	db.EXPECT().GetAIProviders(gomock.Any(), gomock.Any()).Return([]database.AIProvider{{Type: database.AIProviderTypeOpenai, Enabled: true}}, nil)
	db.EXPECT().GetAIProviderKeysByProviderIDs(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()

	server := titleOverrideTestServer(db, logger)
	config, model, _, _, overrideSet, err := server.resolveSummaryGenerationModelOverride(
		ctx,
		chat,
		chatprovider.ProviderAPIKeys{ByProvider: map[string]string{"openai": "test-key"}},
		modelBuildOptions{},
	)
	require.NoError(t, err)
	require.True(t, overrideSet)
	require.NotNil(t, model)
	require.Equal(t, overrideConfig, config)
}

func TestResolveSummaryGenerationModelOverride_SetUnusableHardFails(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	chat, _ := titleOverrideTestChatAndMessages(t)
	// A disabled config is treated as unavailable.
	overrideConfig := titleOverrideModelConfig("gpt-4.1", false)

	db.EXPECT().GetChatSummaryGenerationModelOverride(gomock.Any()).Return(overrideConfig.ID.String(), nil)
	db.EXPECT().GetChatModelConfigByID(gomock.Any(), overrideConfig.ID).Return(overrideConfig, nil)

	server := titleOverrideTestServer(db, logger)
	config, model, _, _, overrideSet, err := server.resolveSummaryGenerationModelOverride(
		ctx,
		chat,
		chatprovider.ProviderAPIKeys{ByProvider: map[string]string{"openai": "test-key"}},
		modelBuildOptions{},
	)
	// overrideSet is true even on a hard failure so the caller skips generation
	// instead of falling back to the chat model.
	require.Error(t, err)
	require.True(t, overrideSet)
	require.ErrorContains(t, err, "summary generation model override is unavailable")
	require.Nil(t, model)
	require.Equal(t, database.ChatModelConfig{}, config)
}

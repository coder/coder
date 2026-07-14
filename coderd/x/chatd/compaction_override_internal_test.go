package chatd

import (
	"database/sql"
	"encoding/json"
	"testing"

	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestCompactionOverrideProviderOptions(t *testing.T) {
	t.Parallel()

	model := &chattest.FakeModel{ProviderName: "anthropic", ModelName: "claude-3-5-haiku"}

	t.Run("NoOptions", func(t *testing.T) {
		t.Parallel()
		opts, err := compactionOverrideProviderOptions(model, database.ChatModelConfig{})
		require.NoError(t, err)
		require.Nil(t, opts)
	})

	t.Run("ReasoningEffort", func(t *testing.T) {
		t.Parallel()
		effort := "low"
		options, err := json.Marshal(codersdk.ChatModelCallConfig{
			ReasoningEffort: &codersdk.ChatModelReasoningEffortConfig{
				Default: &effort,
				Max:     &effort,
			},
		})
		require.NoError(t, err)
		opts, err := compactionOverrideProviderOptions(model, database.ChatModelConfig{Options: options})
		require.NoError(t, err)
		anthropicOpts, ok := opts[fantasyanthropic.Name].(*fantasyanthropic.ProviderOptions)
		require.True(t, ok)
		require.NotNil(t, anthropicOpts.Effort)
		require.Equal(t, fantasyanthropic.Effort("low"), *anthropicOpts.Effort)
	})

	t.Run("MalformedOptions", func(t *testing.T) {
		t.Parallel()
		_, err := compactionOverrideProviderOptions(model, database.ChatModelConfig{Options: []byte("{")})
		require.ErrorContains(t, err, "parse compaction model override call config")
	})
}

func TestResolveCompactionOverrideConfig_Unset(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	chat, _ := titleOverrideTestChatAndMessages(t)

	db.EXPECT().GetChatCompactionModelOverride(gomock.Any()).Return("", nil)

	server := titleOverrideTestServer(db, logger)
	override, err := server.resolveCompactionOverrideConfig(ctx, chat)
	require.NoError(t, err)
	require.Nil(t, override)
}

func TestResolveCompactionOverrideConfig_ReadDBError(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	chat, _ := titleOverrideTestChatAndMessages(t)

	db.EXPECT().GetChatCompactionModelOverride(gomock.Any()).Return("", sql.ErrConnDone)

	server := titleOverrideTestServer(db, logger)
	override, err := server.resolveCompactionOverrideConfig(ctx, chat)
	require.Error(t, err)
	require.ErrorContains(t, err, "read compaction model override")
	require.Nil(t, override)
}

func TestResolveCompactionOverrideConfig_MalformedFallsBack(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	chat, _ := titleOverrideTestChatAndMessages(t)

	db.EXPECT().GetChatCompactionModelOverride(gomock.Any()).Return("not-a-uuid", nil)

	server := titleOverrideTestServer(db, logger)
	override, err := server.resolveCompactionOverrideConfig(ctx, chat)
	require.NoError(t, err)
	require.Nil(t, override)
}

func TestResolveCompactionOverrideConfig_DeletedConfigFallsBack(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	chat, _ := titleOverrideTestChatAndMessages(t)
	missingID := uuid.New()

	db.EXPECT().GetChatCompactionModelOverride(gomock.Any()).Return(missingID.String(), nil)
	db.EXPECT().GetChatModelConfigByID(gomock.Any(), missingID).Return(database.ChatModelConfig{}, sql.ErrNoRows)

	server := titleOverrideTestServer(db, logger)
	override, err := server.resolveCompactionOverrideConfig(ctx, chat)
	require.NoError(t, err)
	require.Nil(t, override)
}

func TestResolveCompactionOverrideConfig_DisabledConfigFallsBack(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	chat, _ := titleOverrideTestChatAndMessages(t)
	overrideConfig := titleOverrideModelConfig("gpt-4.1", false)

	db.EXPECT().GetChatCompactionModelOverride(gomock.Any()).Return(overrideConfig.ID.String(), nil)
	db.EXPECT().GetChatModelConfigByID(gomock.Any(), overrideConfig.ID).Return(overrideConfig, nil)

	server := titleOverrideTestServer(db, logger)
	override, err := server.resolveCompactionOverrideConfig(ctx, chat)
	require.NoError(t, err)
	require.Nil(t, override)
}

func TestResolveCompactionOverrideConfig_MissingCredentialsFallsBack(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	chat, _ := titleOverrideTestChatAndMessages(t)
	overrideConfig := titleOverrideModelConfig("gpt-4.1", true)
	providerID := uuid.New()
	overrideConfig.AIProviderID = uuid.NullUUID{UUID: providerID, Valid: true}

	db.EXPECT().GetChatCompactionModelOverride(gomock.Any()).Return(overrideConfig.ID.String(), nil)
	db.EXPECT().GetChatModelConfigByID(gomock.Any(), overrideConfig.ID).Return(overrideConfig, nil)
	db.EXPECT().GetAIProviderByID(gomock.Any(), providerID).Return(database.AIProvider{
		ID:      providerID,
		Type:    database.AIProviderTypeOpenai,
		Enabled: true,
	}, nil).AnyTimes()
	db.EXPECT().GetAIProviderKeysByProviderID(gomock.Any(), providerID).Return(nil, nil).AnyTimes()

	server := titleOverrideTestServer(db, logger)
	override, err := server.resolveCompactionOverrideConfig(ctx, chat)
	require.NoError(t, err)
	require.Nil(t, override)
}

func TestCompactionOverride_SetUsable(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	chat, _ := titleOverrideTestChatAndMessages(t)
	overrideConfig := titleOverrideModelConfig("gpt-4.1", true)
	providerID := uuid.New()
	overrideConfig.AIProviderID = uuid.NullUUID{UUID: providerID, Valid: true}

	db.EXPECT().GetChatCompactionModelOverride(gomock.Any()).Return(overrideConfig.ID.String(), nil)
	db.EXPECT().GetChatModelConfigByID(gomock.Any(), overrideConfig.ID).Return(overrideConfig, nil)
	db.EXPECT().GetAIProviderByID(gomock.Any(), providerID).Return(aibridgeTestAIProvider(providerID, "primary-openai", database.AIProviderTypeOpenai), nil).AnyTimes()
	db.EXPECT().GetAIProviderKeysByProviderID(gomock.Any(), providerID).Return([]database.AIProviderKey{{
		ProviderID: providerID,
		APIKey:     "test-key",
	}}, nil).AnyTimes()

	server := titleOverrideTestServer(db, logger)
	resolved, err := server.resolveCompactionOverrideConfig(ctx, chat)
	require.NoError(t, err)
	require.NotNil(t, resolved)
	require.Equal(t, overrideConfig.ID, resolved.Config.ID)

	override, err := server.buildCompactionOverrideModel(
		ctx,
		chat,
		resolved.Config,
		modelBuildOptions{ActiveAPIKeyID: uuid.NewString()},
	)
	require.NoError(t, err)
	require.NotNil(t, override.model)
	require.Equal(t, overrideConfig.ID, override.modelConfig.ID)
	require.Equal(t, "openai", override.resolvedProvider)
	require.Equal(t, "gpt-4.1", override.resolvedModel)
	// Prepare-time identity must match the built client's so
	// still-over-limit metrics land on the same series.
	require.Equal(t, override.resolvedProvider, resolved.ResolvedProvider)
	require.Equal(t, override.resolvedModel, resolved.ResolvedModel)
}

package chatd //nolint:testpackage // Tests internal title override helpers.

import (
	"context"
	"database/sql"
	"sync/atomic"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestMaybeGenerateChatTitle_TitleGenerationOverrideUnset(t *testing.T) {
	t.Parallel()

	t.Run("uses preferred model before fallback", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
		chat, messages := titleOverrideTestChatAndMessages(t)
		wantTitle := "Preferred title"

		var requestCount atomic.Int32
		serverURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
			requestCount.Add(1)
			require.Equal(t, preferredTitleModels[1].model, req.Model)
			return chattest.OpenAINonStreamingResponse(`{"title":"` + wantTitle + `"}`)
		})
		keys := titleOverrideOpenAIKeys(serverURL)
		fallbackModel := &chattest.FakeModel{
			GenerateObjectFn: func(context.Context, fantasy.ObjectCall) (*fantasy.ObjectResponse, error) {
				t.Fatal("fallback model should not be called when preferred model works")
				return nil, xerrors.New("unexpected fallback model call")
			},
		}

		db.EXPECT().GetChatTitleGenerationModelOverride(gomock.Any()).Return("", nil)
		db.EXPECT().UpdateChatTitleByID(gomock.Any(), database.UpdateChatTitleByIDParams{
			ID:    chat.ID,
			Title: wantTitle,
		}).Return(chatWithTitle(chat, wantTitle), nil)

		generated := &generatedChatTitle{}
		server := titleOverrideTestServer(db, logger)
		server.maybeGenerateChatTitle(
			ctx,
			chat,
			messages,
			"openai",
			"fallback-chat-model",
			fallbackModel,
			keys,
			generated,
			logger,
			nil,
		)

		require.Equal(t, int32(1), requestCount.Load())
		gotTitle, ok := generated.Load()
		require.True(t, ok)
		require.Equal(t, wantTitle, gotTitle)
	})

	t.Run("falls back to chat model when preferred models are unavailable", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
		chat, messages := titleOverrideTestChatAndMessages(t)
		wantTitle := "Fallback title"

		var fallbackCalls atomic.Int32
		fallbackModel := &chattest.FakeModel{
			GenerateObjectFn: func(context.Context, fantasy.ObjectCall) (*fantasy.ObjectResponse, error) {
				fallbackCalls.Add(1)
				return &fantasy.ObjectResponse{
					Object: map[string]any{"title": wantTitle},
				}, nil
			},
		}

		db.EXPECT().GetChatTitleGenerationModelOverride(gomock.Any()).Return("", nil)
		db.EXPECT().UpdateChatTitleByID(gomock.Any(), database.UpdateChatTitleByIDParams{
			ID:    chat.ID,
			Title: wantTitle,
		}).Return(chatWithTitle(chat, wantTitle), nil)

		generated := &generatedChatTitle{}
		server := titleOverrideTestServer(db, logger)
		server.maybeGenerateChatTitle(
			ctx,
			chat,
			messages,
			"openai",
			"fallback-chat-model",
			fallbackModel,
			chatprovider.ProviderAPIKeys{},
			generated,
			logger,
			nil,
		)

		require.Equal(t, int32(1), fallbackCalls.Load())
		gotTitle, ok := generated.Load()
		require.True(t, ok)
		require.Equal(t, wantTitle, gotTitle)
	})
}

func TestMaybeGenerateChatTitle_TitleGenerationOverrideReadDBError(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	chat, messages := titleOverrideTestChatAndMessages(t)
	wantTitle := "Fallback title"

	var fallbackCalls atomic.Int32
	fallbackModel := &chattest.FakeModel{
		GenerateObjectFn: func(context.Context, fantasy.ObjectCall) (*fantasy.ObjectResponse, error) {
			fallbackCalls.Add(1)
			return &fantasy.ObjectResponse{
				Object: map[string]any{"title": wantTitle},
			}, nil
		},
	}

	db.EXPECT().GetChatTitleGenerationModelOverride(gomock.Any()).Return("", sql.ErrConnDone)
	db.EXPECT().UpdateChatTitleByID(gomock.Any(), database.UpdateChatTitleByIDParams{
		ID:    chat.ID,
		Title: wantTitle,
	}).Return(chatWithTitle(chat, wantTitle), nil)

	generated := &generatedChatTitle{}
	server := titleOverrideTestServer(db, logger)
	server.maybeGenerateChatTitle(
		ctx,
		chat,
		messages,
		"openai",
		"fallback-chat-model",
		fallbackModel,
		chatprovider.ProviderAPIKeys{},
		generated,
		logger,
		nil,
	)

	require.Equal(t, int32(1), fallbackCalls.Load())
	gotTitle, ok := generated.Load()
	require.True(t, ok)
	require.Equal(t, wantTitle, gotTitle)
}

func TestMaybeGenerateChatTitle_TitleGenerationOverrideMalformedFallsThrough(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	chat, messages := titleOverrideTestChatAndMessages(t)
	wantTitle := "Fallback title"

	var fallbackCalls atomic.Int32
	fallbackModel := &chattest.FakeModel{
		GenerateObjectFn: func(context.Context, fantasy.ObjectCall) (*fantasy.ObjectResponse, error) {
			fallbackCalls.Add(1)
			return &fantasy.ObjectResponse{
				Object: map[string]any{"title": wantTitle},
			}, nil
		},
	}

	db.EXPECT().GetChatTitleGenerationModelOverride(gomock.Any()).Return("not-a-uuid", nil)
	db.EXPECT().UpdateChatTitleByID(gomock.Any(), database.UpdateChatTitleByIDParams{
		ID:    chat.ID,
		Title: wantTitle,
	}).Return(chatWithTitle(chat, wantTitle), nil)

	generated := &generatedChatTitle{}
	server := titleOverrideTestServer(db, logger)
	server.maybeGenerateChatTitle(
		ctx,
		chat,
		messages,
		"openai",
		"fallback-chat-model",
		fallbackModel,
		chatprovider.ProviderAPIKeys{},
		generated,
		logger,
		nil,
	)

	require.Equal(t, int32(1), fallbackCalls.Load())
	gotTitle, ok := generated.Load()
	require.True(t, ok)
	require.Equal(t, wantTitle, gotTitle)
}

func TestMaybeGenerateChatTitle_TitleGenerationOverrideSetUsable(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	chat, messages := titleOverrideTestChatAndMessages(t)
	overrideConfig := titleOverrideModelConfig("gpt-4.1", true)
	wantTitle := "Override title"

	var requestCount atomic.Int32
	serverURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		requestCount.Add(1)
		require.Equal(t, overrideConfig.Model, req.Model)
		return chattest.OpenAINonStreamingResponse(`{"title":"` + wantTitle + `"}`)
	})
	keys := titleOverrideOpenAIKeys(serverURL)
	fallbackModel := &chattest.FakeModel{
		GenerateObjectFn: func(context.Context, fantasy.ObjectCall) (*fantasy.ObjectResponse, error) {
			t.Fatal("fallback model should not be called when override is usable")
			return nil, xerrors.New("unexpected fallback model call")
		},
	}

	db.EXPECT().GetChatTitleGenerationModelOverride(gomock.Any()).Return(overrideConfig.ID.String(), nil)
	db.EXPECT().GetChatModelConfigByID(gomock.Any(), overrideConfig.ID).Return(overrideConfig, nil)
	db.EXPECT().GetEnabledChatProviders(gomock.Any()).Return([]database.ChatProvider{{Provider: "openai"}}, nil)
	db.EXPECT().UpdateChatTitleByID(gomock.Any(), database.UpdateChatTitleByIDParams{
		ID:    chat.ID,
		Title: wantTitle,
	}).Return(chatWithTitle(chat, wantTitle), nil)

	generated := &generatedChatTitle{}
	server := titleOverrideTestServer(db, logger)
	server.maybeGenerateChatTitle(
		ctx,
		chat,
		messages,
		"openai",
		"fallback-chat-model",
		fallbackModel,
		keys,
		generated,
		logger,
		nil,
	)

	require.Equal(t, int32(1), requestCount.Load())
	gotTitle, ok := generated.Load()
	require.True(t, ok)
	require.Equal(t, wantTitle, gotTitle)
}

func TestMaybeGenerateChatTitle_TitleGenerationOverrideSetUnusableSkips(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	chat, messages := titleOverrideTestChatAndMessages(t)
	overrideConfig := titleOverrideModelConfig("gpt-4.1", false)
	fallbackModel := &chattest.FakeModel{
		GenerateObjectFn: func(context.Context, fantasy.ObjectCall) (*fantasy.ObjectResponse, error) {
			t.Fatal("fallback model should not be called when override is unusable")
			return nil, xerrors.New("unexpected fallback model call")
		},
	}

	db.EXPECT().GetChatTitleGenerationModelOverride(gomock.Any()).Return(overrideConfig.ID.String(), nil)
	db.EXPECT().GetChatModelConfigByID(gomock.Any(), overrideConfig.ID).Return(overrideConfig, nil)

	generated := &generatedChatTitle{}
	server := titleOverrideTestServer(db, logger)
	server.maybeGenerateChatTitle(
		ctx,
		chat,
		messages,
		"openai",
		"fallback-chat-model",
		fallbackModel,
		chatprovider.ProviderAPIKeys{},
		generated,
		logger,
		nil,
	)

	_, ok := generated.Load()
	require.False(t, ok)
}

func TestMaybeGenerateChatTitle_TitleGenerationOverrideCallFailureSkipsFallback(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	chat, messages := titleOverrideTestChatAndMessages(t)
	overrideConfig := titleOverrideModelConfig("gpt-4.1", true)

	var requestCount atomic.Int32
	serverURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		requestCount.Add(1)
		require.Equal(t, overrideConfig.Model, req.Model)
		return chattest.OpenAINonStreamingResponse(`{"title":""}`)
	})
	keys := titleOverrideOpenAIKeys(serverURL)
	fallbackModel := &chattest.FakeModel{
		GenerateObjectFn: func(context.Context, fantasy.ObjectCall) (*fantasy.ObjectResponse, error) {
			t.Fatal("fallback model should not be called after override call failure")
			return nil, xerrors.New("unexpected fallback model call")
		},
	}

	db.EXPECT().GetChatTitleGenerationModelOverride(gomock.Any()).Return(overrideConfig.ID.String(), nil)
	db.EXPECT().GetChatModelConfigByID(gomock.Any(), overrideConfig.ID).Return(overrideConfig, nil)
	db.EXPECT().GetEnabledChatProviders(gomock.Any()).Return([]database.ChatProvider{{Provider: "openai"}}, nil)

	generated := &generatedChatTitle{}
	server := titleOverrideTestServer(db, logger)
	server.maybeGenerateChatTitle(
		ctx,
		chat,
		messages,
		"openai",
		"fallback-chat-model",
		fallbackModel,
		keys,
		generated,
		logger,
		nil,
	)

	require.Equal(t, int32(1), requestCount.Load())
	_, ok := generated.Load()
	require.False(t, ok)
}

func TestResolveManualTitleCandidates_TitleGenerationOverrideUnset(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	chat, _ := titleOverrideTestChatAndMessages(t)
	preferredConfig := database.ChatModelConfig{
		ID:       uuid.New(),
		Provider: preferredTitleModels[1].provider,
		Model:    preferredTitleModels[1].model,
		Enabled:  true,
	}
	chat.LastModelConfigID = preferredConfig.ID

	db.EXPECT().GetChatTitleGenerationModelOverride(gomock.Any()).Return("", nil)
	db.EXPECT().GetEnabledChatModelConfigs(gomock.Any()).Return([]database.ChatModelConfig{
		{Provider: "openai", Model: "gpt-4.1", Enabled: true},
		preferredConfig,
	}, nil)
	// The fallback path also runs (it resolves the chat's own model
	// as the last-resort candidate). With LastModelConfigID set to
	// preferredConfig.ID, the configCache hits GetChatModelConfigByID
	// once and the resulting fallback candidate is deduplicated by
	// the seen-by-id guard.
	db.EXPECT().GetChatModelConfigByID(gomock.Any(), preferredConfig.ID).Return(preferredConfig, nil)

	server := titleOverrideTestServer(db, logger)
	candidates, configs, err := server.resolveManualTitleCandidates(
		ctx,
		db,
		chat,
		chatprovider.ProviderAPIKeys{ByProvider: map[string]string{"openai": "test-key"}},
	)
	require.NoError(t, err)
	require.Len(t, candidates, 1, "fallback candidate must be deduped because seen[preferredConfig.ID] is true")
	require.Equal(t, len(candidates), len(configs))
	require.Equal(t, preferredConfig.ID, candidates[0].configID.UUID)
	require.Equal(t, preferredConfig, configs[0])
}

func TestResolveManualTitleCandidates_TitleGenerationOverrideReadDBError(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	chat, _ := titleOverrideTestChatAndMessages(t)
	preferredConfig := database.ChatModelConfig{
		ID:       uuid.New(),
		Provider: preferredTitleModels[1].provider,
		Model:    preferredTitleModels[1].model,
		Enabled:  true,
	}
	chat.LastModelConfigID = preferredConfig.ID

	db.EXPECT().GetChatTitleGenerationModelOverride(gomock.Any()).Return("", sql.ErrConnDone)
	db.EXPECT().GetEnabledChatModelConfigs(gomock.Any()).Return([]database.ChatModelConfig{
		{Provider: "openai", Model: "gpt-4.1", Enabled: true},
		preferredConfig,
	}, nil)
	db.EXPECT().GetChatModelConfigByID(gomock.Any(), preferredConfig.ID).Return(preferredConfig, nil)

	server := titleOverrideTestServer(db, logger)
	candidates, configs, err := server.resolveManualTitleCandidates(
		ctx,
		db,
		chat,
		chatprovider.ProviderAPIKeys{ByProvider: map[string]string{"openai": "test-key"}},
	)
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	require.Equal(t, preferredConfig.ID, candidates[0].configID.UUID)
	require.Equal(t, preferredConfig, configs[0])
}

func TestResolveManualTitleCandidates_TitleGenerationOverrideSetUsable(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	chat, _ := titleOverrideTestChatAndMessages(t)
	overrideConfig := titleOverrideModelConfig("gpt-4.1", true)

	db.EXPECT().GetChatTitleGenerationModelOverride(gomock.Any()).Return(overrideConfig.ID.String(), nil)
	db.EXPECT().GetChatModelConfigByID(gomock.Any(), overrideConfig.ID).Return(overrideConfig, nil)
	db.EXPECT().GetEnabledChatProviders(gomock.Any()).Return([]database.ChatProvider{{Provider: "openai"}}, nil)

	server := titleOverrideTestServer(db, logger)
	candidates, configs, err := server.resolveManualTitleCandidates(
		ctx,
		db,
		chat,
		chatprovider.ProviderAPIKeys{ByProvider: map[string]string{"openai": "test-key"}},
	)
	require.NoError(t, err)
	// Override is exclusive: a single candidate with the override
	// config and nothing else.
	require.Len(t, candidates, 1)
	require.Equal(t, overrideConfig.ID, candidates[0].configID.UUID)
	require.Equal(t, []database.ChatModelConfig{overrideConfig}, configs)
}

func TestResolveManualTitleCandidates_TitleGenerationOverrideMissingCredentials(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	chat, _ := titleOverrideTestChatAndMessages(t)
	overrideConfig := titleOverrideModelConfig("gpt-4.1", true)

	db.EXPECT().GetChatTitleGenerationModelOverride(gomock.Any()).Return(overrideConfig.ID.String(), nil)
	db.EXPECT().GetChatModelConfigByID(gomock.Any(), overrideConfig.ID).Return(overrideConfig, nil)
	db.EXPECT().GetEnabledChatProviders(gomock.Any()).Return([]database.ChatProvider{{Provider: "openai"}}, nil)

	server := titleOverrideTestServer(db, logger)
	candidates, configs, err := server.resolveManualTitleCandidates(
		ctx,
		db,
		chat,
		chatprovider.ProviderAPIKeys{},
	)
	require.Error(t, err)
	require.ErrorContains(t, err, "resolve manual title generation model override")
	require.ErrorContains(t, err, "credentials are unavailable")
	require.Nil(t, candidates)
	require.Nil(t, configs)
}

func TestResolveManualTitleCandidates_TitleGenerationOverrideSetUnusable(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	chat, _ := titleOverrideTestChatAndMessages(t)
	overrideConfig := titleOverrideModelConfig("gpt-4.1", false)

	db.EXPECT().GetChatTitleGenerationModelOverride(gomock.Any()).Return(overrideConfig.ID.String(), nil)
	db.EXPECT().GetChatModelConfigByID(gomock.Any(), overrideConfig.ID).Return(overrideConfig, nil)

	server := titleOverrideTestServer(db, logger)
	candidates, configs, err := server.resolveManualTitleCandidates(
		ctx,
		db,
		chat,
		chatprovider.ProviderAPIKeys{ByProvider: map[string]string{"openai": "test-key"}},
	)
	require.Error(t, err)
	require.ErrorContains(t, err, "resolve manual title generation model override")
	require.ErrorContains(t, err, "title generation model override is unavailable")
	require.Nil(t, candidates)
	require.Nil(t, configs)
}

func titleOverrideTestChatAndMessages(t *testing.T) (database.Chat, []database.ChatMessage) {
	t.Helper()

	userPrompt := "review pull request 123 and fix comments"
	chat := database.Chat{
		ID:      uuid.New(),
		OwnerID: uuid.New(),
		Title:   fallbackChatTitle(userPrompt),
	}
	message := mustChatMessage(
		t,
		database.ChatMessageRoleUser,
		database.ChatMessageVisibilityBoth,
		codersdk.ChatMessageText(userPrompt),
	)
	message.ID = 1
	return chat, []database.ChatMessage{message}
}

func titleOverrideTestServer(db database.Store, logger slog.Logger) *Server {
	return &Server{
		db:          db,
		logger:      logger,
		configCache: newChatConfigCache(context.Background(), db, quartz.NewReal()),
	}
}

func titleOverrideModelConfig(model string, enabled bool) database.ChatModelConfig {
	return database.ChatModelConfig{
		ID:       uuid.New(),
		Provider: "openai",
		Model:    model,
		Enabled:  enabled,
	}
}

func titleOverrideOpenAIKeys(serverURL string) chatprovider.ProviderAPIKeys {
	return chatprovider.ProviderAPIKeys{
		ByProvider: map[string]string{
			"openai": "test-key",
		},
		BaseURLByProvider: map[string]string{
			"openai": serverURL,
		},
	}
}

func chatWithTitle(chat database.Chat, title string) database.Chat {
	chat.Title = title
	return chat
}

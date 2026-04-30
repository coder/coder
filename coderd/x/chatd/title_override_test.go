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
		db.EXPECT().UpdateChatByID(gomock.Any(), database.UpdateChatByIDParams{
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
		db.EXPECT().UpdateChatByID(gomock.Any(), database.UpdateChatByIDParams{
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
	db.EXPECT().UpdateChatByID(gomock.Any(), database.UpdateChatByIDParams{
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
	db.EXPECT().UpdateChatByID(gomock.Any(), database.UpdateChatByIDParams{
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
	db.EXPECT().UpdateChatByID(gomock.Any(), database.UpdateChatByIDParams{
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

func TestResolveManualTitleModel_TitleGenerationOverrideUnset(t *testing.T) {
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

	db.EXPECT().GetChatTitleGenerationModelOverride(gomock.Any()).Return("", nil)
	db.EXPECT().GetEnabledChatModelConfigs(gomock.Any()).Return([]database.ChatModelConfig{
		{Provider: "openai", Model: "gpt-4.1", Enabled: true},
		preferredConfig,
	}, nil)

	server := titleOverrideTestServer(db, logger)
	model, gotConfig, err := server.resolveManualTitleModel(
		ctx,
		db,
		chat,
		chatprovider.ProviderAPIKeys{ByProvider: map[string]string{"openai": "test-key"}},
	)
	require.NoError(t, err)
	require.NotNil(t, model)
	require.Equal(t, preferredConfig, gotConfig)
}

func TestResolveManualTitleModel_TitleGenerationOverrideReadDBError(t *testing.T) {
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

	db.EXPECT().GetChatTitleGenerationModelOverride(gomock.Any()).Return("", sql.ErrConnDone)
	db.EXPECT().GetEnabledChatModelConfigs(gomock.Any()).Return([]database.ChatModelConfig{
		{Provider: "openai", Model: "gpt-4.1", Enabled: true},
		preferredConfig,
	}, nil)

	server := titleOverrideTestServer(db, logger)
	model, gotConfig, err := server.resolveManualTitleModel(
		ctx,
		db,
		chat,
		chatprovider.ProviderAPIKeys{ByProvider: map[string]string{"openai": "test-key"}},
	)
	require.NoError(t, err)
	require.NotNil(t, model)
	require.Equal(t, preferredConfig, gotConfig)
}

func TestResolveManualTitleModel_TitleGenerationOverrideSetUsable(t *testing.T) {
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
	model, gotConfig, err := server.resolveManualTitleModel(
		ctx,
		db,
		chat,
		chatprovider.ProviderAPIKeys{ByProvider: map[string]string{"openai": "test-key"}},
	)
	require.NoError(t, err)
	require.NotNil(t, model)
	require.Equal(t, overrideConfig, gotConfig)
}

func TestResolveManualTitleModel_TitleGenerationOverrideMissingCredentials(t *testing.T) {
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
	model, gotConfig, err := server.resolveManualTitleModel(
		ctx,
		db,
		chat,
		chatprovider.ProviderAPIKeys{},
	)
	require.Error(t, err)
	require.ErrorContains(t, err, "resolve manual title generation model override")
	require.ErrorContains(t, err, "credentials are unavailable")
	require.Nil(t, model)
	require.Equal(t, database.ChatModelConfig{}, gotConfig)
}

func TestResolveManualTitleModel_TitleGenerationOverrideSetUnusable(t *testing.T) {
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
	model, gotConfig, err := server.resolveManualTitleModel(
		ctx,
		db,
		chat,
		chatprovider.ProviderAPIKeys{ByProvider: map[string]string{"openai": "test-key"}},
	)
	require.Error(t, err)
	require.ErrorContains(t, err, "resolve manual title generation model override")
	require.ErrorContains(t, err, "title generation model override is unavailable")
	require.Nil(t, model)
	require.Equal(t, database.ChatModelConfig{}, gotConfig)
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

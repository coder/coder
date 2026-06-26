package chatd

import (
	"context"
	"database/sql"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/aibridge"
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
			resolvedModelRoute{},
			keys,
			modelBuildOptions{},
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
			resolvedModelRoute{},
			chatprovider.ProviderAPIKeys{},
			modelBuildOptions{},
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
		resolvedModelRoute{},
		chatprovider.ProviderAPIKeys{},
		modelBuildOptions{},
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
		resolvedModelRoute{},
		chatprovider.ProviderAPIKeys{},
		modelBuildOptions{},
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
	providerID := uuid.New()
	overrideConfig.AIProviderID = uuid.NullUUID{UUID: providerID, Valid: true}
	wantTitle := "Override title"

	var requestCount atomic.Int32
	serverURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		requestCount.Add(1)
		require.Equal(t, overrideConfig.Model, req.Model)
		return chattest.OpenAINonStreamingResponse(`{"title":"` + wantTitle + `"}`)
	})
	provider := database.AIProvider{
		ID:      providerID,
		Type:    database.AIProviderTypeOpenai,
		Enabled: true,
		BaseUrl: serverURL,
	}
	fallbackModel := &chattest.FakeModel{
		GenerateObjectFn: func(context.Context, fantasy.ObjectCall) (*fantasy.ObjectResponse, error) {
			t.Fatal("fallback model should not be called when override is usable")
			return nil, xerrors.New("unexpected fallback model call")
		},
	}

	db.EXPECT().GetChatTitleGenerationModelOverride(gomock.Any()).Return(overrideConfig.ID.String(), nil)
	db.EXPECT().GetChatModelConfigByID(gomock.Any(), overrideConfig.ID).Return(overrideConfig, nil)
	db.EXPECT().GetAIProviderByID(gomock.Any(), providerID).Return(provider, nil).AnyTimes()
	db.EXPECT().GetAIProviderKeysByProviderID(gomock.Any(), providerID).Return([]database.AIProviderKey{{
		ProviderID: providerID,
		APIKey:     "test-key",
	}}, nil).Times(2)
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
		resolvedModelRoute{},
		chatprovider.ProviderAPIKeys{},
		modelBuildOptions{},
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
		resolvedModelRoute{},
		chatprovider.ProviderAPIKeys{},
		modelBuildOptions{},
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
	db.EXPECT().GetAIProviders(gomock.Any(), gomock.Any()).Return([]database.AIProvider{{Type: database.AIProviderTypeOpenai, Enabled: true}}, nil)
	db.EXPECT().GetAIProviderKeysByProviderIDs(gomock.Any(), []uuid.UUID{uuid.Nil}).Return(nil, nil)

	generated := &generatedChatTitle{}
	server := titleOverrideTestServer(db, logger)
	server.maybeGenerateChatTitle(
		ctx,
		chat,
		messages,
		"openai",
		"fallback-chat-model",
		fallbackModel,
		resolvedModelRoute{},
		keys,
		modelBuildOptions{},
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
	model, gotConfig, _, err := server.resolveManualTitleModel(
		ctx,
		db,
		chat,
		chatprovider.ProviderAPIKeys{ByProvider: map[string]string{"openai": "test-key"}},
		modelBuildOptions{},
	)
	require.NoError(t, err)
	require.NotNil(t, model)
	require.Equal(t, preferredConfig, gotConfig)
}

func TestResolveManualTitleModel_TitleGenerationOverrideUnsetAIProvider(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	chat, _ := titleOverrideTestChatAndMessages(t)
	providerID := uuid.New()
	preferredConfig := database.ChatModelConfig{
		ID:           uuid.New(),
		Provider:     preferredTitleModels[1].provider,
		AIProviderID: uuid.NullUUID{UUID: providerID, Valid: true},
		Model:        preferredTitleModels[1].model,
		Enabled:      true,
	}
	serverURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		t.Fatal("model construction should not call the provider")
		return chattest.OpenAIResponse{}
	})
	provider := database.AIProvider{
		ID:      providerID,
		Type:    database.AIProviderTypeOpenai,
		Enabled: true,
		BaseUrl: serverURL,
	}

	db.EXPECT().GetChatTitleGenerationModelOverride(gomock.Any()).Return("", nil)
	db.EXPECT().GetEnabledChatModelConfigs(gomock.Any()).Return([]database.ChatModelConfig{
		preferredConfig,
	}, nil)
	db.EXPECT().GetAIProviderByID(gomock.Any(), providerID).Return(provider, nil)
	db.EXPECT().GetAIProviderKeysByProviderID(gomock.Any(), providerID).Return([]database.AIProviderKey{{
		ProviderID: providerID,
		APIKey:     "test-key",
	}}, nil)

	server := titleOverrideTestServer(db, logger)
	model, gotConfig, gotKeys, err := server.resolveManualTitleModel(
		ctx,
		db,
		chat,
		chatprovider.ProviderAPIKeys{},
		modelBuildOptions{},
	)
	require.NoError(t, err)
	require.NotNil(t, model)
	require.Equal(t, preferredConfig, gotConfig)
	require.Equal(t, "test-key", gotKeys.APIKey("openai"))
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
	model, gotConfig, _, err := server.resolveManualTitleModel(
		ctx,
		db,
		chat,
		chatprovider.ProviderAPIKeys{ByProvider: map[string]string{"openai": "test-key"}},
		modelBuildOptions{},
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
	db.EXPECT().GetAIProviders(gomock.Any(), gomock.Any()).Return([]database.AIProvider{{Type: database.AIProviderTypeOpenai, Enabled: true}}, nil)
	db.EXPECT().GetAIProviderKeysByProviderIDs(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()

	server := titleOverrideTestServer(db, logger)
	model, gotConfig, _, err := server.resolveManualTitleModel(
		ctx,
		db,
		chat,
		chatprovider.ProviderAPIKeys{ByProvider: map[string]string{"openai": "test-key"}},
		modelBuildOptions{},
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
	db.EXPECT().GetAIProviders(gomock.Any(), gomock.Any()).Return([]database.AIProvider{{Type: database.AIProviderTypeOpenai, Enabled: true}}, nil)
	db.EXPECT().GetAIProviderKeysByProviderIDs(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()

	server := titleOverrideTestServer(db, logger)
	model, gotConfig, _, err := server.resolveManualTitleModel(
		ctx,
		db,
		chat,
		chatprovider.ProviderAPIKeys{},
		modelBuildOptions{},
	)
	require.Error(t, err)
	require.ErrorContains(t, err, "resolve manual title generation model override")
	require.ErrorContains(t, err, "credentials are unavailable")
	require.Nil(t, model)
	require.Equal(t, database.ChatModelConfig{}, gotConfig)
}

func TestGenerateManualTitleCandidate_ActiveAPIKeyIDFallback(t *testing.T) {
	t.Parallel()

	contextAPIKeyID := uuid.NewString()
	messageAPIKeyID := uuid.NewString()
	shadowedContextAPIKeyID := uuid.NewString()
	tests := []struct {
		name            string
		messageAPIKeyID string
		contextAPIKeyID string
		wantAPIKeyID    string
		wantErrContains string
	}{
		{
			name:            "ContextFallback",
			contextAPIKeyID: contextAPIKeyID,
			wantAPIKeyID:    contextAPIKeyID,
		},
		{
			name:            "MessageTakesPrecedence",
			messageAPIKeyID: messageAPIKeyID,
			contextAPIKeyID: shadowedContextAPIKeyID,
			wantAPIKeyID:    messageAPIKeyID,
		},
		{
			name:            "NoKeyAnywhereFailsClosed",
			wantErrContains: "AI Gateway routing requires the active turn API key ID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitShort)
			if tt.contextAPIKeyID != "" {
				ctx = aibridge.WithDelegatedAPIKeyID(ctx, tt.contextAPIKeyID)
			}
			ctrl := gomock.NewController(t)
			db := dbmock.NewMockStore(ctrl)
			logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
			chat, messages := titleOverrideTestChatAndMessages(t)
			chat.OrganizationID = uuid.New()
			if tt.messageAPIKeyID != "" {
				messages[0] = withChatMessageAPIKeyID(messages[0], tt.messageAPIKeyID)
			}
			overrideConfig := titleOverrideModelConfig("gpt-4.1", true)
			providerID := uuid.New()
			overrideConfig.AIProviderID = uuid.NullUUID{UUID: providerID, Valid: true}
			provider := database.AIProvider{
				ID:      providerID,
				Name:    "primary-openai",
				Type:    database.AIProviderTypeOpenai,
				Enabled: true,
			}
			wantTitle := "Context title"
			seenAPIKeyID := make(chan string, 1)
			factory := &aibridgeTestFactory{rt: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				apiKeyID, _ := aibridge.DelegatedAPIKeyIDFromContext(req.Context())
				seenAPIKeyID <- apiKeyID
				text := strconv.Quote(`{"title":"` + wantTitle + `"}`)
				body := `{"id":"resp_test","object":"response","created_at":0,"status":"completed","model":"gpt-4.1","output":[{"id":"msg_test","type":"message","role":"assistant","content":[{"type":"output_text","text":` + text + `}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(strings.NewReader(body)),
					Request:    req,
				}, nil
			})}

			db.EXPECT().GetChatUsageLimitConfig(gomock.Any()).Return(database.ChatUsageLimitConfig{}, sql.ErrNoRows)
			db.EXPECT().GetChatMessagesByChatIDAscPaginated(gomock.Any(), database.GetChatMessagesByChatIDAscPaginatedParams{
				ChatID:   chat.ID,
				AfterID:  0,
				LimitVal: manualTitleMessageWindowLimit,
			}).Return(messages, nil)
			db.EXPECT().GetChatMessagesByChatIDDescPaginated(gomock.Any(), database.GetChatMessagesByChatIDDescPaginatedParams{
				ChatID:   chat.ID,
				BeforeID: 0,
				LimitVal: manualTitleMessageWindowLimit,
			}).Return(nil, nil)
			db.EXPECT().GetChatTitleGenerationModelOverride(gomock.Any()).Return(overrideConfig.ID.String(), nil)
			db.EXPECT().GetChatModelConfigByID(gomock.Any(), overrideConfig.ID).Return(overrideConfig, nil)
			db.EXPECT().GetAIProviderByID(gomock.Any(), providerID).Return(provider, nil).AnyTimes()
			db.EXPECT().GetAIProviderKeysByProviderID(gomock.Any(), providerID).Return([]database.AIProviderKey{{
				ProviderID: providerID,
				APIKey:     "test-key",
			}}, nil).AnyTimes()

			server := titleOverrideTestServer(db, logger)
			server.aiGatewayRoutingEnabled = true
			server.aibridgeTransportFactory = aibridgeTestFactoryPointer(factory)
			result, err := server.generateManualTitleCandidate(ctx, db, chat, chatprovider.ProviderAPIKeys{})
			if tt.wantErrContains != "" {
				require.ErrorContains(t, err, tt.wantErrContains)
				return
			}
			require.NoError(t, err)
			require.Equal(t, wantTitle, result.title)
			require.True(t, result.hasMessages)
			require.Equal(t, tt.wantAPIKeyID, result.activeAPIKeyID)
			require.Equal(t, tt.wantAPIKeyID, testutil.RequireReceive(ctx, t, seenAPIKeyID))
		})
	}
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
	model, gotConfig, _, err := server.resolveManualTitleModel(
		ctx,
		db,
		chat,
		chatprovider.ProviderAPIKeys{ByProvider: map[string]string{"openai": "test-key"}},
		modelBuildOptions{},
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

package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"charm.land/fantasy"
	fantasyopenai "charm.land/fantasy/providers/openai"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/aibridge"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestMaybeGenerateChatTitle_TitleGenerationOverrideUnset(t *testing.T) {
	t.Parallel()

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
			nil,
			"openai",
			database.ChatModelConfig{Model: "fallback-chat-model"},
			fallbackModel,
			aiGatewayModelRoute{},
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
		nil,
		"openai",
		database.ChatModelConfig{Model: "fallback-chat-model"},
		fallbackModel,
		aiGatewayModelRoute{},
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
		nil,
		"openai",
		database.ChatModelConfig{Model: "fallback-chat-model"},
		fallbackModel,
		aiGatewayModelRoute{},
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
	overrideConfig := titleOverrideModelConfig("gpt-5", true)
	providerID := uuid.New()
	overrideConfig.AIProviderID = uuid.NullUUID{UUID: providerID, Valid: true}
	options, err := json.Marshal(codersdk.ChatModelCallConfig{
		ReasoningEffort: &codersdk.ChatModelReasoningEffortConfig{
			Default: ptr.Ref(codersdk.ChatModelReasoningEffortLow),
			Max:     ptr.Ref(codersdk.ChatModelReasoningEffortHigh),
		},
	})
	require.NoError(t, err)
	overrideConfig.Options = options
	wantTitle := "Override title"

	var requestCount atomic.Int32
	factory := &aibridgeTestFactory{rt: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requestCount.Add(1)
		bodyBytes, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		var raw map[string]any
		require.NoError(t, json.Unmarshal(bodyBytes, &raw))
		require.Equal(t, string(fantasyopenai.ReasoningEffortHigh), raw["reasoning"].(map[string]any)["effort"])
		text := strconv.Quote(`{"title":"` + wantTitle + `"}`)
		body := `{"id":"resp_test","object":"response","created_at":0,"status":"completed","model":"gpt-5","output":[{"id":"msg_test","type":"message","role":"assistant","content":[{"type":"output_text","text":` + text + `}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    req,
		}, nil
	})}
	provider := database.AIProvider{
		ID:      providerID,
		Name:    "primary-openai",
		Type:    database.AIProviderTypeOpenai,
		Enabled: true,
	}
	fallbackModel := &chattest.FakeModel{
		GenerateObjectFn: func(context.Context, fantasy.ObjectCall) (*fantasy.ObjectResponse, error) {
			t.Fatal("fallback model should not be called when override is usable")
			return nil, xerrors.New("unexpected fallback model call")
		},
	}

	db.EXPECT().GetChatTitleGenerationModelOverride(gomock.Any()).Return(overrideConfig.ID.String()+":xhigh", nil)
	db.EXPECT().GetChatModelConfigByID(gomock.Any(), overrideConfig.ID).Return(overrideConfig, nil)
	db.EXPECT().GetAIProviderByID(gomock.Any(), providerID).Return(provider, nil).AnyTimes()
	db.EXPECT().GetAIProviderKeysByProviderID(gomock.Any(), providerID).Return([]database.AIProviderKey{{
		ProviderID: providerID,
		APIKey:     "test-key",
	}}, nil).AnyTimes()
	db.EXPECT().UpdateChatTitleByID(gomock.Any(), database.UpdateChatTitleByIDParams{
		ID:    chat.ID,
		Title: wantTitle,
	}).Return(chatWithTitle(chat, wantTitle), nil)

	generated := &generatedChatTitle{}
	server := titleOverrideTestServer(db, logger)
	server.aibridgeTransportFactory = aibridgeTestFactoryPointer(factory)
	server.maybeGenerateChatTitle(
		ctx,
		chat,
		messages,
		nil,
		"openai",
		database.ChatModelConfig{Model: "fallback-chat-model"},
		fallbackModel,
		aiGatewayModelRoute{},
		modelBuildOptions{ActiveAPIKeyID: uuid.NewString()},
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
		nil,
		"openai",
		database.ChatModelConfig{Model: "fallback-chat-model"},
		fallbackModel,
		aiGatewayModelRoute{},
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
	providerID := uuid.New()
	overrideConfig.AIProviderID = uuid.NullUUID{UUID: providerID, Valid: true}

	var requestCount atomic.Int32
	factory := &aibridgeTestFactory{rt: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requestCount.Add(1)
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
	})}
	fallbackModel := &chattest.FakeModel{
		GenerateObjectFn: func(context.Context, fantasy.ObjectCall) (*fantasy.ObjectResponse, error) {
			t.Fatal("fallback model should not be called after override call failure")
			return nil, xerrors.New("unexpected fallback model call")
		},
	}

	db.EXPECT().GetChatTitleGenerationModelOverride(gomock.Any()).Return(overrideConfig.ID.String(), nil)
	db.EXPECT().GetChatModelConfigByID(gomock.Any(), overrideConfig.ID).Return(overrideConfig, nil)
	db.EXPECT().GetAIProviderByID(gomock.Any(), providerID).Return(aibridgeTestAIProvider(providerID, "primary-openai", database.AIProviderTypeOpenai), nil).AnyTimes()
	db.EXPECT().GetAIProviderKeysByProviderID(gomock.Any(), providerID).Return([]database.AIProviderKey{{
		ProviderID: providerID,
		APIKey:     "test-key",
	}}, nil).AnyTimes()

	generated := &generatedChatTitle{}
	server := titleOverrideTestServer(db, logger)
	server.aibridgeTransportFactory = aibridgeTestFactoryPointer(factory)
	server.maybeGenerateChatTitle(
		ctx,
		chat,
		messages,
		nil,
		"openai",
		database.ChatModelConfig{Model: "fallback-chat-model"},
		fallbackModel,
		aiGatewayModelRoute{},
		modelBuildOptions{ActiveAPIKeyID: uuid.NewString()},
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
	providerID := uuid.New()
	preferredConfig := database.ChatModelConfig{
		ID:           uuid.New(),
		AIProviderID: uuid.NullUUID{UUID: providerID, Valid: true},
		Model:        preferredTitleModels[1].model,
		Enabled:      true,
	}

	db.EXPECT().GetChatTitleGenerationModelOverride(gomock.Any()).Return("", nil)
	db.EXPECT().GetEnabledChatModelConfigs(gomock.Any()).Return([]database.GetEnabledChatModelConfigsRow{
		{ChatModelConfig: database.ChatModelConfig{Model: "gpt-4.1", Enabled: true}, Provider: "openai"},
		{ChatModelConfig: preferredConfig, Provider: preferredTitleModels[1].provider},
	}, nil)
	db.EXPECT().GetAIProviderByID(gomock.Any(), providerID).Return(aibridgeTestAIProvider(providerID, "primary-openai", database.AIProviderTypeOpenai), nil).AnyTimes()

	server := titleOverrideTestServer(db, logger)
	model, gotConfig, err := server.resolveManualTitleModel(
		ctx,
		db,
		chat,
		modelBuildOptions{ActiveAPIKeyID: uuid.NewString()},
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
		Name:    "primary-openai",
		Type:    database.AIProviderTypeOpenai,
		Enabled: true,
		BaseUrl: serverURL,
	}

	db.EXPECT().GetChatTitleGenerationModelOverride(gomock.Any()).Return("", nil)
	db.EXPECT().GetEnabledChatModelConfigs(gomock.Any()).Return([]database.GetEnabledChatModelConfigsRow{
		{ChatModelConfig: preferredConfig, Provider: preferredTitleModels[1].provider},
	}, nil)
	db.EXPECT().GetAIProviderByID(gomock.Any(), providerID).Return(provider, nil)
	db.EXPECT().GetAIProviderKeysByProviderID(gomock.Any(), providerID).Return([]database.AIProviderKey{{
		ProviderID: providerID,
		APIKey:     "test-key",
	}}, nil).AnyTimes()

	server := titleOverrideTestServer(db, logger)
	model, gotConfig, err := server.resolveManualTitleModel(
		ctx,
		db,
		chat,
		modelBuildOptions{ActiveAPIKeyID: uuid.NewString()},
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
	providerID := uuid.New()
	preferredConfig := database.ChatModelConfig{
		ID:           uuid.New(),
		AIProviderID: uuid.NullUUID{UUID: providerID, Valid: true},
		Model:        preferredTitleModels[1].model,
		Enabled:      true,
	}

	db.EXPECT().GetChatTitleGenerationModelOverride(gomock.Any()).Return("", sql.ErrConnDone)
	db.EXPECT().GetEnabledChatModelConfigs(gomock.Any()).Return([]database.GetEnabledChatModelConfigsRow{
		{ChatModelConfig: database.ChatModelConfig{Model: "gpt-4.1", Enabled: true}, Provider: "openai"},
		{ChatModelConfig: preferredConfig, Provider: preferredTitleModels[1].provider},
	}, nil)
	db.EXPECT().GetAIProviderByID(gomock.Any(), providerID).Return(aibridgeTestAIProvider(providerID, "primary-openai", database.AIProviderTypeOpenai), nil).AnyTimes()

	server := titleOverrideTestServer(db, logger)
	model, gotConfig, err := server.resolveManualTitleModel(
		ctx,
		db,
		chat,
		modelBuildOptions{ActiveAPIKeyID: uuid.NewString()},
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
	providerID := uuid.New()
	overrideConfig.AIProviderID = uuid.NullUUID{UUID: providerID, Valid: true}

	db.EXPECT().GetChatTitleGenerationModelOverride(gomock.Any()).Return(overrideConfig.ID.String(), nil)
	db.EXPECT().GetChatModelConfigByID(gomock.Any(), overrideConfig.ID).Return(overrideConfig, nil)
	db.EXPECT().GetAIProviderByID(gomock.Any(), providerID).Return(aibridgeTestAIProvider(providerID, "primary-openai", database.AIProviderTypeOpenai), nil).AnyTimes()
	db.EXPECT().GetAIProviderKeysByProviderID(gomock.Any(), providerID).Return([]database.AIProviderKey{{
		ProviderID: providerID,
		APIKey:     "test-key",
	}}, nil).AnyTimes()

	server := titleOverrideTestServer(db, logger)
	model, gotConfig, err := server.resolveManualTitleModel(
		ctx,
		db,
		chat,
		modelBuildOptions{ActiveAPIKeyID: uuid.NewString()},
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
	providerID := uuid.New()
	overrideConfig.AIProviderID = uuid.NullUUID{UUID: providerID, Valid: true}
	provider := database.AIProvider{
		ID:      providerID,
		Type:    database.AIProviderTypeOpenai,
		Enabled: true,
	}

	db.EXPECT().GetChatTitleGenerationModelOverride(gomock.Any()).Return(overrideConfig.ID.String(), nil)
	db.EXPECT().GetChatModelConfigByID(gomock.Any(), overrideConfig.ID).Return(overrideConfig, nil)
	db.EXPECT().GetAIProviderByID(gomock.Any(), providerID).Return(provider, nil).AnyTimes()
	db.EXPECT().GetAIProviderKeysByProviderID(gomock.Any(), providerID).Return(nil, nil).AnyTimes()

	server := titleOverrideTestServer(db, logger)
	model, gotConfig, err := server.resolveManualTitleModel(
		ctx,
		db,
		chat,
		modelBuildOptions{},
	)
	require.Error(t, err)
	require.ErrorContains(t, err, "resolve manual title generation model override")
	require.ErrorContains(t, err, "credentials are unavailable")
	require.Nil(t, model)
	require.Equal(t, database.ChatModelConfig{}, gotConfig)
}

func TestGenerateManualTitleCandidate_UsesSyntheticAPIKey(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	chat, messages := titleOverrideTestChatAndMessages(t)
	chat.OrganizationID = uuid.New()
	overrideConfig := titleOverrideModelConfig("gpt-4.1", true)
	providerID := uuid.New()
	overrideConfig.AIProviderID = uuid.NullUUID{UUID: providerID, Valid: true}
	provider := database.AIProvider{
		ID:      providerID,
		Name:    "primary-openai",
		Type:    database.AIProviderTypeOpenai,
		Enabled: true,
	}
	apiKeyID := uuid.NewString()
	wantTitle := "Synthetic title"
	seenAPIKeyID := make(chan string, 1)
	factory := &aibridgeTestFactory{rt: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		delegatedID, _ := aibridge.DelegatedAPIKeyIDFromContext(req.Context())
		seenAPIKeyID <- delegatedID
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
	db.EXPECT().GetChatGatewayAPIKey(gomock.Any(), database.GetChatGatewayAPIKeyParams{
		UserID:    chat.OwnerID,
		TokenName: GatewayTokenName(chat.OwnerID),
	}).Return(database.APIKey{
		ID:        apiKeyID,
		UserID:    chat.OwnerID,
		ExpiresAt: time.Now().Add(48 * time.Hour),
	}, nil)
	db.EXPECT().GetChatTitleGenerationModelOverride(gomock.Any()).Return(overrideConfig.ID.String(), nil)
	db.EXPECT().GetChatModelConfigByID(gomock.Any(), overrideConfig.ID).Return(overrideConfig, nil)
	db.EXPECT().GetAIProviderByID(gomock.Any(), providerID).Return(provider, nil).AnyTimes()
	db.EXPECT().GetAIProviderKeysByProviderID(gomock.Any(), providerID).Return([]database.AIProviderKey{{
		ProviderID: providerID,
		APIKey:     "test-key",
	}}, nil).AnyTimes()

	server := titleOverrideTestServer(db, logger)
	server.clock = quartz.NewReal()
	server.aibridgeTransportFactory = aibridgeTestFactoryPointer(factory)
	title, err := server.generateManualTitleCandidate(ctx, db, chat)
	require.NoError(t, err)
	require.Equal(t, wantTitle, title)
	require.Equal(t, apiKeyID, testutil.RequireReceive(ctx, t, seenAPIKeyID))
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
		modelBuildOptions{},
	)
	require.Error(t, err)
	require.ErrorContains(t, err, "resolve manual title generation model override")
	require.ErrorContains(t, err, "title generation model override is unavailable")
	require.Nil(t, model)
	require.Equal(t, database.ChatModelConfig{}, gotConfig)
}

func TestParseModelOverride(t *testing.T) {
	t.Parallel()

	modelConfigID := uuid.New()
	tests := []struct {
		name       string
		raw        string
		wantID     uuid.UUID
		wantEffort *string
		wantOK     bool
	}{
		{name: "Empty", raw: "", wantOK: true},
		{name: "Whitespace", raw: " \t\n ", wantOK: true},
		{name: "IDOnly", raw: modelConfigID.String(), wantID: modelConfigID, wantOK: true},
		{name: "IDWithEffort", raw: modelConfigID.String() + ":high", wantID: modelConfigID, wantEffort: ptr.Ref("high"), wantOK: true},
		{name: "IDEmptyEffort", raw: modelConfigID.String() + ":", wantOK: false},
		{name: "OuterWhitespace", raw: " \t" + modelConfigID.String() + ":high\n ", wantID: modelConfigID, wantEffort: ptr.Ref("high"), wantOK: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok := parseModelOverride(tt.raw)

			require.Equal(t, tt.wantOK, ok)
			if !tt.wantOK {
				return
			}
			require.Equal(t, tt.wantID, got.modelConfigID)
			require.Equal(t, tt.wantEffort, got.reasoningEffort)
		})
	}
}

func titleOverrideTestChatAndMessages(t *testing.T) (database.Chat, []database.ChatMessage) {
	t.Helper()

	userPrompt := "review pull request 123 and fix comments"
	chat := database.Chat{
		ID:      uuid.New(),
		OwnerID: uuid.New(),
		Title:   chatprompt.FallbackTitle(userPrompt),
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
	factory := &aibridgeTestFactory{rt: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
	})}
	return &Server{
		db:                       db,
		logger:                   logger,
		configCache:              newChatConfigCache(context.Background(), db, quartz.NewReal()),
		aibridgeTransportFactory: aibridgeTestFactoryPointer(factory),
	}
}

func titleOverrideModelConfig(model string, enabled bool) database.ChatModelConfig {
	return database.ChatModelConfig{
		ID:      uuid.New(),
		Model:   model,
		Enabled: enabled,
	}
}

func chatWithTitle(chat database.Chat, title string) database.Chat {
	chat.Title = title
	return chat
}

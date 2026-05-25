package chatd

import (
	"database/sql"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/aibridge"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
)

type aibridgeTestFactory struct {
	providerName string
	source       aibridge.Source
	err          error
	rt           http.RoundTripper
}

func (f *aibridgeTestFactory) TransportFor(providerName string, source aibridge.Source) (http.RoundTripper, error) {
	f.providerName = providerName
	f.source = source
	if f.err != nil {
		return nil, f.err
	}
	return f.rt, nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func aibridgeTestFactoryPointer(factory aibridge.TransportFactory) *atomic.Pointer[aibridge.TransportFactory] {
	var ptr atomic.Pointer[aibridge.TransportFactory]
	ptr.Store(&factory)
	return &ptr
}

func aibridgeTestAIProvider(providerID uuid.UUID, providerName string, providerType database.AIProviderType) *database.AIProvider {
	return &database.AIProvider{
		ID:      providerID,
		Name:    providerName,
		Type:    providerType,
		Enabled: true,
	}
}

func aibridgeTestRoute(aiProvider *database.AIProvider) resolvedModelRoute {
	return resolvedModelRoute{AIGateway: &aiGatewayModelRoute{
		Provider:     *aiProvider,
		OriginalHint: string(aiProvider.Type),
	}}
}

func aibridgeTestRequest(chat database.Chat, model string) modelClientRequest {
	return modelClientRequest{
		Chat:      chat,
		ModelName: model,
		UserAgent: chatprovider.UserAgent(),
	}
}

func TestAIBridgeProviderFormatMapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		providerType database.AIProviderType
		wantProvider string
		wantBaseURL  string
	}{
		{name: "OpenAI", providerType: database.AiProviderTypeOpenai, wantProvider: "openai", wantBaseURL: "http://coder-aibridge/v1"},
		{name: "Anthropic", providerType: database.AiProviderTypeAnthropic, wantProvider: "anthropic", wantBaseURL: "http://coder-aibridge"},
		{name: "Bedrock", providerType: database.AiProviderTypeBedrock, wantProvider: "anthropic", wantBaseURL: "http://coder-aibridge"},
		{name: "Google", providerType: database.AiProviderTypeGoogle, wantProvider: "openai-compat", wantBaseURL: "http://coder-aibridge/v1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			config := fantasyConfigForAIBridge(tt.providerType)
			require.Equal(t, tt.wantProvider, config.ProviderHint)
			require.Equal(t, tt.wantBaseURL, config.Keys.BaseURL(config.ProviderHint))
			require.Equal(t, aibridgePlaceholderAPIKey, config.Keys.APIKey(config.ProviderHint))
		})
	}
}

func TestAIBridgeModelProviderInputUsesLocalPlaceholderKey(t *testing.T) {
	t.Parallel()

	config := fantasyConfigForAIBridge(database.AiProviderTypeOpenai)
	require.Equal(t, "openai", config.ProviderHint)
	require.Equal(t, aibridgePlaceholderAPIKey, config.Keys.APIKey(config.ProviderHint))
	require.Equal(t, "http://coder-aibridge/v1", config.Keys.BaseURL(config.ProviderHint))
}

func TestResolveModelRouteForConfigPreservesBaseURL(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	ownerID := uuid.New()
	providerID := uuid.New()
	baseURL := "https://openai.example.com/v1"

	db.EXPECT().GetAIProviderByID(gomock.Any(), providerID).Return(database.AIProvider{
		ID:      providerID,
		Type:    database.AiProviderTypeOpenai,
		Name:    "primary-openai",
		Enabled: true,
		BaseUrl: baseURL,
	}, nil)
	db.EXPECT().GetAIProviderKeysByProviderID(gomock.Any(), providerID).Return([]database.AIProviderKey{{
		ProviderID: providerID,
		APIKey:     "provider-key",
	}}, nil)

	server := &Server{db: db}
	route, err := server.resolveModelRouteForConfig(ctx, ownerID, database.ChatModelConfig{
		Provider:     "openai",
		AIProviderID: uuid.NullUUID{UUID: providerID, Valid: true},
	}, chatprovider.ProviderAPIKeys{})
	require.NoError(t, err)
	require.NotNil(t, route.Direct)
	require.Nil(t, route.AIGateway)
	require.Equal(t, "openai", route.Direct.ProviderHint)
	require.Equal(t, "provider-key", route.Direct.Keys.APIKey("openai"))
	require.Equal(t, baseURL, route.Direct.Keys.BaseURL("openai"))
}

func TestAIGatewayProviderAuthForUser(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	ownerID := uuid.New()
	providerID := uuid.New()
	provider := database.AIProvider{ID: providerID, Type: database.AiProviderTypeOpenai, Enabled: true}

	t.Run("OpenAIUserKey", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		db.EXPECT().GetUserAIProviderKeyByProviderID(gomock.Any(), database.GetUserAIProviderKeyByProviderIDParams{
			UserID:       ownerID,
			AIProviderID: providerID,
		}).Return(database.UserAiProviderKey{APIKey: "sk-user"}, nil)

		server := &Server{db: db, allowBYOK: true}
		auth, err := server.aiGatewayProviderAuthForUser(ctx, ownerID, provider, aiGatewayRequestFormatOpenAI)
		require.NoError(t, err)
		require.True(t, auth.PreserveProviderAuth)
		require.Equal(t, "Bearer sk-user", auth.Headers["Authorization"])
		require.Empty(t, auth.Headers["X-Api-Key"])
	})

	t.Run("AnthropicUserKey", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		db.EXPECT().GetUserAIProviderKeyByProviderID(gomock.Any(), database.GetUserAIProviderKeyByProviderIDParams{
			UserID:       ownerID,
			AIProviderID: providerID,
		}).Return(database.UserAiProviderKey{APIKey: "sk-user"}, nil)

		server := &Server{db: db, allowBYOK: true}
		auth, err := server.aiGatewayProviderAuthForUser(ctx, ownerID, provider, aiGatewayRequestFormatAnthropic)
		require.NoError(t, err)
		require.True(t, auth.PreserveProviderAuth)
		require.Equal(t, "sk-user", auth.Headers["X-Api-Key"])
		require.Empty(t, auth.Headers["Authorization"])
	})

	t.Run("NoUserKey", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		db.EXPECT().GetUserAIProviderKeyByProviderID(gomock.Any(), database.GetUserAIProviderKeyByProviderIDParams{
			UserID:       ownerID,
			AIProviderID: providerID,
		}).Return(database.UserAiProviderKey{}, sql.ErrNoRows)

		server := &Server{db: db, allowBYOK: true}
		auth, err := server.aiGatewayProviderAuthForUser(ctx, ownerID, provider, aiGatewayRequestFormatOpenAI)
		require.NoError(t, err)
		require.False(t, auth.PreserveProviderAuth)
		require.Empty(t, auth.Headers)
	})

	t.Run("BYOKDisabled", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		server := &Server{db: db, allowBYOK: false}
		auth, err := server.aiGatewayProviderAuthForUser(ctx, ownerID, provider, aiGatewayRequestFormatOpenAI)
		require.NoError(t, err)
		require.False(t, auth.PreserveProviderAuth)
		require.Empty(t, auth.Headers)
	})
}

func TestResolveModelRouteForConfigAIGatewayProviderAuth(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	ownerID := uuid.New()
	providerID := uuid.New()
	provider := database.AIProvider{
		ID:      providerID,
		Type:    database.AiProviderTypeOpenai,
		Name:    "primary-openai",
		Enabled: true,
	}
	modelConfig := database.ChatModelConfig{
		ID:           uuid.New(),
		Model:        "gpt-4",
		Provider:     "openai",
		AIProviderID: uuid.NullUUID{UUID: providerID, Valid: true},
	}

	t.Run("UserKey", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		db.EXPECT().GetAIProviderByID(gomock.Any(), providerID).Return(provider, nil)
		db.EXPECT().GetUserAIProviderKeyByProviderID(gomock.Any(), database.GetUserAIProviderKeyByProviderIDParams{
			UserID:       ownerID,
			AIProviderID: providerID,
		}).Return(database.UserAiProviderKey{APIKey: "sk-user"}, nil)

		server := &Server{db: db, aiGatewayRoutingEnabled: true, allowBYOK: true}
		route, err := server.resolveModelRouteForConfig(ctx, ownerID, modelConfig, chatprovider.ProviderAPIKeys{})
		require.NoError(t, err)
		require.Nil(t, route.Direct)
		require.NotNil(t, route.AIGateway)
		require.True(t, route.AIGateway.ProviderAuth.PreserveProviderAuth)
		require.Equal(t, "Bearer sk-user", route.AIGateway.ProviderAuth.Headers["Authorization"])
	})

	t.Run("CentralProviderCredentialsNotForwarded", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		db.EXPECT().GetAIProviderByID(gomock.Any(), providerID).Return(provider, nil)

		server := &Server{db: db, aiGatewayRoutingEnabled: true, allowBYOK: false}
		route, err := server.resolveModelRouteForConfig(ctx, ownerID, modelConfig, chatprovider.ProviderAPIKeys{})
		require.NoError(t, err)
		require.Nil(t, route.Direct)
		require.NotNil(t, route.AIGateway)
		require.False(t, route.AIGateway.ProviderAuth.PreserveProviderAuth)
		require.Empty(t, route.AIGateway.ProviderAuth.Headers)
	})
}

func TestAIGatewayModelForwardsProviderAuth(t *testing.T) {
	t.Parallel()

	type seenRequest struct {
		authorization string
		xAPIKey       string
		preserve      bool
		apiKeyID      string
		path          string
	}
	newServer := func(t *testing.T, provider database.AIProvider, auth aiGatewayProviderAuth, seen chan seenRequest) (*Server, resolvedModelRoute) {
		factory := &aibridgeTestFactory{rt: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			apiKeyID, _ := aibridge.DelegatedAPIKeyIDFromContext(req.Context())
			seen <- seenRequest{
				authorization: req.Header.Get("Authorization"),
				xAPIKey:       req.Header.Get("X-Api-Key"),
				preserve:      aibridge.PreserveProviderAuthFromContext(req.Context()),
				apiKeyID:      apiKeyID,
				path:          req.URL.Path,
			}
			body := `{"id":"resp_test","object":"response","created_at":0,"status":"completed","model":"gpt-4","output":[{"id":"msg_test","type":"message","role":"assistant","content":[{"type":"output_text","text":"hello"}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`
			if provider.Type == database.AiProviderTypeAnthropic {
				body = `{"id":"msg_test","type":"message","role":"assistant","model":"claude-haiku-4-5","content":[{"type":"text","text":"hello"}],"stop_reason":"end_turn","stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":1}}`
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(body)),
				Request:    req,
			}, nil
		})}
		server := &Server{
			aiGatewayRoutingEnabled:  true,
			aibridgeTransportFactory: aibridgeTestFactoryPointer(factory),
		}
		route := resolvedModelRoute{AIGateway: &aiGatewayModelRoute{
			Provider:     provider,
			OriginalHint: string(provider.Type),
			ProviderAuth: auth,
		}}
		return server, route
	}

	t.Run("OpenAI", func(t *testing.T) {
		t.Parallel()

		seen := make(chan seenRequest, 1)
		provider := *aibridgeTestAIProvider(uuid.New(), "primary-openai", database.AiProviderTypeOpenai)
		server, route := newServer(t, provider, aiGatewayProviderAuth{
			Headers:              map[string]string{"Authorization": "Bearer sk-user"},
			PreserveProviderAuth: true,
		}, seen)
		apiKeyID := uuid.NewString()
		model, err := server.newModel(t.Context(), aibridgeTestRequest(database.Chat{ID: uuid.New(), OwnerID: uuid.New()}, "gpt-4"), route, modelBuildOptions{ActiveAPIKeyID: apiKeyID, RecordHTTP: true})
		require.NoError(t, err)
		_, _ = model.Generate(t.Context(), fantasy.Call{Prompt: []fantasy.Message{{Role: fantasy.MessageRoleUser, Content: []fantasy.MessagePart{fantasy.TextPart{Text: "hello"}}}}})

		got := <-seen
		require.Equal(t, "Bearer sk-user", got.authorization)
		require.Empty(t, got.xAPIKey)
		require.True(t, got.preserve)
		require.Equal(t, apiKeyID, got.apiKeyID)
		require.Equal(t, "/v1/responses", got.path)
	})

	t.Run("Anthropic", func(t *testing.T) {
		t.Parallel()

		seen := make(chan seenRequest, 1)
		provider := *aibridgeTestAIProvider(uuid.New(), "primary-anthropic", database.AiProviderTypeAnthropic)
		server, route := newServer(t, provider, aiGatewayProviderAuth{
			Headers:              map[string]string{"X-Api-Key": "sk-user"},
			PreserveProviderAuth: true,
		}, seen)
		apiKeyID := uuid.NewString()
		model, err := server.newModel(t.Context(), aibridgeTestRequest(database.Chat{ID: uuid.New(), OwnerID: uuid.New()}, "claude-haiku-4-5"), route, modelBuildOptions{ActiveAPIKeyID: apiKeyID})
		require.NoError(t, err)
		_, _ = model.Generate(t.Context(), fantasy.Call{Prompt: []fantasy.Message{{Role: fantasy.MessageRoleUser, Content: []fantasy.MessagePart{fantasy.TextPart{Text: "hello"}}}}})

		got := <-seen
		require.Empty(t, got.authorization)
		require.Equal(t, "sk-user", got.xAPIKey)
		require.True(t, got.preserve)
		require.Equal(t, apiKeyID, got.apiKeyID)
		require.Equal(t, "/v1/messages", got.path)
	})

	t.Run("NoUserKeyScrubsPlaceholder", func(t *testing.T) {
		t.Parallel()

		seen := make(chan seenRequest, 1)
		provider := *aibridgeTestAIProvider(uuid.New(), "primary-openai", database.AiProviderTypeOpenai)
		server, route := newServer(t, provider, aiGatewayProviderAuth{}, seen)
		apiKeyID := uuid.NewString()
		model, err := server.newModel(t.Context(), aibridgeTestRequest(database.Chat{ID: uuid.New(), OwnerID: uuid.New()}, "gpt-4"), route, modelBuildOptions{ActiveAPIKeyID: apiKeyID})
		require.NoError(t, err)
		_, _ = model.Generate(t.Context(), fantasy.Call{Prompt: []fantasy.Message{{Role: fantasy.MessageRoleUser, Content: []fantasy.MessagePart{fantasy.TextPart{Text: "hello"}}}}})

		got := <-seen
		require.Empty(t, got.authorization)
		require.Empty(t, got.xAPIKey)
		require.False(t, got.preserve)
		require.Equal(t, apiKeyID, got.apiKeyID)
	})
}

func TestActiveTurnAPIKeyIDFromMessages(t *testing.T) {
	t.Parallel()

	oldKeyID := uuid.NewString()
	currentKeyID := uuid.NewString()
	tests := []struct {
		name     string
		messages []database.ChatMessage
		wantKey  string
		wantOK   bool
	}{
		{
			name: "CurrentUserMessage",
			messages: []database.ChatMessage{
				{ID: 1, Role: database.ChatMessageRoleUser, Visibility: database.ChatMessageVisibilityBoth, APIKeyID: sqlNullString(oldKeyID)},
				{ID: 2, Role: database.ChatMessageRoleAssistant, Visibility: database.ChatMessageVisibilityBoth},
				{ID: 3, Role: database.ChatMessageRoleUser, Visibility: database.ChatMessageVisibilityBoth, APIKeyID: sqlNullString(currentKeyID)},
			},
			wantKey: currentKeyID,
			wantOK:  true,
		},
		{
			name: "MissingCurrentUserAPIKeyDoesNotFallBack",
			messages: []database.ChatMessage{
				{ID: 1, Role: database.ChatMessageRoleUser, Visibility: database.ChatMessageVisibilityBoth, APIKeyID: sqlNullString(oldKeyID)},
				{ID: 2, Role: database.ChatMessageRoleUser, Visibility: database.ChatMessageVisibilityBoth},
			},
		},
		{
			name: "SkipsModelOnlyUserMessages",
			messages: []database.ChatMessage{
				{ID: 1, Role: database.ChatMessageRoleUser, Visibility: database.ChatMessageVisibilityBoth, APIKeyID: sqlNullString(oldKeyID)},
				{ID: 2, Role: database.ChatMessageRoleUser, Visibility: database.ChatMessageVisibilityModel, APIKeyID: sqlNullString(currentKeyID)},
			},
			wantKey: oldKeyID,
			wantOK:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotKey, gotOK := activeTurnAPIKeyIDFromMessages(tt.messages)
			require.Equal(t, tt.wantOK, gotOK)
			require.Equal(t, tt.wantKey, gotKey)
			ctx := contextWithActiveTurnAPIKeyID(t.Context(), tt.messages)
			ctxKey, ctxOK := aibridge.DelegatedAPIKeyIDFromContext(ctx)
			require.Equal(t, tt.wantOK, ctxOK)
			require.Equal(t, tt.wantKey, ctxKey)
		})
	}
}

func TestActiveTurnContextUsesPromptMessages(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := t.Context()
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{})
	chat := dbgen.Chat(t, db, database.Chat{OrganizationID: org.ID, OwnerID: user.ID, LastModelConfigID: model.ID})
	oldKey, _ := dbgen.APIKey(t, db, database.APIKey{UserID: user.ID})
	currentKey, _ := dbgen.APIKey(t, db, database.APIKey{UserID: user.ID})
	modelOnlyKey, _ := dbgen.APIKey(t, db, database.APIKey{UserID: user.ID})

	dbgen.ChatMessage(t, db, database.ChatMessage{
		ChatID:        chat.ID,
		CreatedBy:     uuid.NullUUID{UUID: user.ID, Valid: true},
		ModelConfigID: uuid.NullUUID{UUID: model.ID, Valid: true},
		Role:          database.ChatMessageRoleUser,
		Visibility:    database.ChatMessageVisibilityBoth,
		APIKeyID:      sqlNullString(oldKey.ID),
	})
	dbgen.ChatMessage(t, db, database.ChatMessage{
		ChatID:        chat.ID,
		CreatedBy:     uuid.NullUUID{UUID: user.ID, Valid: true},
		ModelConfigID: uuid.NullUUID{UUID: model.ID, Valid: true},
		Role:          database.ChatMessageRoleSystem,
		Visibility:    database.ChatMessageVisibilityModel,
		Compressed:    true,
	})
	dbgen.ChatMessage(t, db, database.ChatMessage{
		ChatID:        chat.ID,
		CreatedBy:     uuid.NullUUID{UUID: user.ID, Valid: true},
		ModelConfigID: uuid.NullUUID{UUID: model.ID, Valid: true},
		Role:          database.ChatMessageRoleUser,
		Visibility:    database.ChatMessageVisibilityBoth,
		APIKeyID:      sqlNullString(currentKey.ID),
	})
	dbgen.ChatMessage(t, db, database.ChatMessage{
		ChatID:        chat.ID,
		CreatedBy:     uuid.NullUUID{UUID: user.ID, Valid: true},
		ModelConfigID: uuid.NullUUID{UUID: model.ID, Valid: true},
		Role:          database.ChatMessageRoleUser,
		Visibility:    database.ChatMessageVisibilityModel,
		APIKeyID:      sqlNullString(modelOnlyKey.ID),
	})

	messages, err := db.GetChatMessagesForPromptByChatID(ctx, chat.ID)
	require.NoError(t, err)
	ctx = contextWithActiveTurnAPIKeyID(ctx, messages)
	gotKey, ok := aibridge.DelegatedAPIKeyIDFromContext(ctx)
	require.True(t, ok)
	require.Equal(t, currentKey.ID, gotKey)
}

func sqlNullString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: value != ""}
}

func TestAIBridgeRoutingFailClosed(t *testing.T) {
	t.Parallel()

	providerID := uuid.New()
	chat := database.Chat{ID: uuid.New(), OwnerID: uuid.New()}
	aiProvider := aibridgeTestAIProvider(providerID, "primary-openai", database.AiProviderTypeOpenai)

	t.Run("NilFactory", func(t *testing.T) {
		t.Parallel()
		server := &Server{aiGatewayRoutingEnabled: true}
		ctx := aibridge.WithDelegatedAPIKeyID(t.Context(), uuid.NewString())
		_, err := server.newModel(ctx, aibridgeTestRequest(chat, "gpt-4"), aibridgeTestRoute(aiProvider), modelBuildOptions{ActiveAPIKeyID: uuid.NewString()})
		require.ErrorContains(t, err, "transport factory")
	})

	t.Run("FactoryError", func(t *testing.T) {
		t.Parallel()
		factory := &aibridgeTestFactory{err: xerrors.New("boom")}
		server := &Server{
			aiGatewayRoutingEnabled:  true,
			aibridgeTransportFactory: aibridgeTestFactoryPointer(factory),
		}
		ctx := aibridge.WithDelegatedAPIKeyID(t.Context(), uuid.NewString())
		_, err := server.newModel(ctx, aibridgeTestRequest(chat, "gpt-4"), aibridgeTestRoute(aiProvider), modelBuildOptions{ActiveAPIKeyID: uuid.NewString()})
		require.ErrorContains(t, err, "boom")
	})

	t.Run("MissingProviderName", func(t *testing.T) {
		t.Parallel()
		server := &Server{aiGatewayRoutingEnabled: true}
		missingNameProvider := aibridgeTestAIProvider(providerID, "", database.AiProviderTypeOpenai)
		_, err := server.newModel(t.Context(), aibridgeTestRequest(chat, "gpt-4"), aibridgeTestRoute(missingNameProvider), modelBuildOptions{ActiveAPIKeyID: uuid.NewString()})
		require.ErrorContains(t, err, "AI provider name")
	})

	t.Run("MissingAPIKeyID", func(t *testing.T) {
		t.Parallel()
		factory := &aibridgeTestFactory{rt: roundTripFunc(func(*http.Request) (*http.Response, error) {
			t.Fatal("transport must not be used without an API key ID")
			return nil, xerrors.New("unreachable")
		})}
		server := &Server{
			aiGatewayRoutingEnabled:  true,
			aibridgeTransportFactory: aibridgeTestFactoryPointer(factory),
		}
		_, err := server.newModel(t.Context(), aibridgeTestRequest(chat, "gpt-4"), aibridgeTestRoute(aiProvider), modelBuildOptions{})
		require.ErrorContains(t, err, "active turn API key ID")
	})

	t.Run("StaticModel", func(t *testing.T) {
		t.Parallel()
		server := &Server{aiGatewayRoutingEnabled: true}
		_, err := server.newModel(t.Context(), aibridgeTestRequest(chat, "gpt-4"), resolvedModelRoute{AIGateway: &aiGatewayModelRoute{}}, modelBuildOptions{ActiveAPIKeyID: uuid.NewString()})
		require.ErrorContains(t, err, "concrete AI provider")
	})
}

func TestDirectModelBuildDoesNotRequireActiveAPIKeyID(t *testing.T) {
	t.Parallel()

	server := &Server{}
	model, err := server.newModel(t.Context(), modelClientRequest{
		Chat:      database.Chat{ID: uuid.New(), OwnerID: uuid.New()},
		ModelName: "gpt-4",
		UserAgent: chatprovider.UserAgent(),
	}, resolvedModelRoute{Direct: &directModelRoute{
		ProviderHint: "openai",
		Keys:         chatprovider.ProviderAPIKeys{OpenAI: "sk-test"},
	}}, modelBuildOptions{})
	require.NoError(t, err)
	require.NotNil(t, model)
}

func TestAIBridgeComputerUseModelUsesRoute(t *testing.T) {
	t.Parallel()

	providerID := uuid.New()
	apiKeyID := uuid.NewString()
	factory := &aibridgeTestFactory{rt: roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("computer use model construction must not send a request")
		return nil, xerrors.New("unreachable")
	})}
	chat := database.Chat{ID: uuid.New(), OwnerID: uuid.New()}
	server := &Server{
		aiGatewayRoutingEnabled:  true,
		aibridgeTransportFactory: aibridgeTestFactoryPointer(factory),
	}
	provider := chattool.ComputerUseProviderOpenAI
	modelProvider, modelName, ok := chattool.DefaultComputerUseModel(provider)
	require.True(t, ok)

	ctx := aibridge.WithDelegatedAPIKeyID(t.Context(), "context-key-must-be-ignored")
	model, debugEnabled, resolvedProvider, resolvedModel, err := server.resolveComputerUseModel(
		ctx,
		chat,
		aibridgeTestRoute(aibridgeTestAIProvider(providerID, "primary-openai", database.AiProviderTypeOpenai)),
		provider,
		modelProvider,
		modelName,
		modelBuildOptions{ActiveAPIKeyID: apiKeyID},
	)
	require.NoError(t, err)
	require.NotNil(t, model)
	require.False(t, debugEnabled)
	require.Equal(t, chattool.ComputerUseProviderOpenAI, resolvedProvider)
	require.Equal(t, modelName, resolvedModel)
	require.Equal(t, "primary-openai", factory.providerName)
	require.Equal(t, aibridge.SourceAgents, factory.source)
}

func TestAIBridgeDelegatedContextPropagation(t *testing.T) {
	t.Parallel()

	providerID := uuid.New()
	apiKeyID := uuid.NewString()
	type seenRequest struct {
		apiKeyID string
		ok       bool
		path     string
	}
	seen := make(chan seenRequest, 1)
	factory := &aibridgeTestFactory{rt: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		gotAPIKeyID, ok := aibridge.DelegatedAPIKeyIDFromContext(req.Context())
		seen <- seenRequest{
			apiKeyID: gotAPIKeyID,
			ok:       ok,
			path:     req.URL.Path,
		}
		body := `{"id":"resp_test","object":"response","created_at":0,"status":"completed","model":"gpt-4","output":[{"id":"msg_test","type":"message","role":"assistant","content":[{"type":"output_text","text":"hello"}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    req,
		}, nil
	})}
	chat := database.Chat{ID: uuid.New(), OwnerID: uuid.New()}
	server := &Server{
		aiGatewayRoutingEnabled:  true,
		aibridgeTransportFactory: aibridgeTestFactoryPointer(factory),
	}

	ctx := aibridge.WithDelegatedAPIKeyID(t.Context(), "context-key-must-be-ignored")
	model, err := server.newModel(ctx, aibridgeTestRequest(chat, "gpt-4"), aibridgeTestRoute(aibridgeTestAIProvider(providerID, "primary-openai", database.AiProviderTypeOpenai)), modelBuildOptions{ActiveAPIKeyID: apiKeyID, RecordHTTP: true})
	require.NoError(t, err)
	_, err = model.Generate(t.Context(), fantasy.Call{Prompt: []fantasy.Message{{
		Role:    fantasy.MessageRoleUser,
		Content: []fantasy.MessagePart{fantasy.TextPart{Text: "hello"}},
	}}})
	require.NoError(t, err)

	got := <-seen
	require.Equal(t, "primary-openai", factory.providerName)
	require.Equal(t, aibridge.SourceAgents, factory.source)
	require.True(t, got.ok)
	require.Equal(t, "/v1/responses", got.path)
	require.Equal(t, apiKeyID, got.apiKeyID)
}

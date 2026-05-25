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

func aibridgeTestModelRoute(providerID uuid.UUID, providerName string, providerType database.AIProviderType) modelRoute {
	return modelRouteFromAIProvider(database.AIProvider{
		ID:      providerID,
		Name:    providerName,
		Type:    providerType,
		Enabled: true,
	})
}

func aibridgeTestDBWithAPIKeyID(t testing.TB, chatID uuid.UUID, apiKeyID string) *dbmock.MockStore {
	t.Helper()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	db.EXPECT().GetLatestChatUserMessageAPIKeyID(gomock.Any(), chatID).Return(sql.NullString{
		String: apiKeyID,
		Valid:  apiKeyID != "",
	}, nil).AnyTimes()
	return db
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
			provider, keys := aibridgeModelProviderInput(tt.providerType)
			require.Equal(t, tt.wantProvider, provider)
			require.Equal(t, tt.wantBaseURL, keys.BaseURL(provider))
			require.Equal(t, aibridgePlaceholderAPIKey, keys.APIKey(provider))
		})
	}
}

func TestAIBridgeModelProviderInputUsesLocalPlaceholderKey(t *testing.T) {
	t.Parallel()

	provider, keys := aibridgeModelProviderInput(database.AiProviderTypeOpenai)
	require.Equal(t, "openai", provider)
	require.Equal(t, aibridgePlaceholderAPIKey, keys.APIKey(provider))
	require.Equal(t, "http://coder-aibridge/v1", keys.BaseURL(provider))
}

func TestResolveModelConfigProviderHintKeysAndRoutePreservesBaseURL(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	ownerID := uuid.New()
	providerID := uuid.New()
	providerName := "primary-openai"
	baseURL := "https://openai.example.com/v1"

	db.EXPECT().GetAIProviderByID(gomock.Any(), providerID).Return(database.AIProvider{
		ID:      providerID,
		Type:    database.AiProviderTypeOpenai,
		Name:    providerName,
		Enabled: true,
		BaseUrl: baseURL,
	}, nil)
	db.EXPECT().GetAIProviderKeysByProviderID(gomock.Any(), providerID).Return([]database.AIProviderKey{{
		ProviderID: providerID,
		APIKey:     "provider-key",
	}}, nil)

	server := &Server{db: db}
	providerHint, keys, route, err := server.resolveModelConfigProviderHintKeysAndRoute(ctx, ownerID, database.ChatModelConfig{
		Provider:     "openai",
		AIProviderID: uuid.NullUUID{UUID: providerID, Valid: true},
	}, chatprovider.ProviderAPIKeys{})
	require.NoError(t, err)
	require.Equal(t, "openai", providerHint)
	require.Equal(t, providerID, route.aiProvider.ID)
	require.Equal(t, providerName, route.aiProvider.Name)
	require.Equal(t, baseURL, route.aiProvider.BaseUrl)
	require.Equal(t, "provider-key", keys.APIKey("openai"))
	require.Equal(t, baseURL, keys.BaseURL("openai"))
}

func TestGetLatestChatUserMessageAPIKeyIDUsesSystemSummaryBoundary(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := t.Context()
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{})
	chat := dbgen.Chat(t, db, database.Chat{OrganizationID: org.ID, OwnerID: user.ID, LastModelConfigID: model.ID})

	oldKey, _ := dbgen.APIKey(t, db, database.APIKey{UserID: user.ID})
	latestKey, _ := dbgen.APIKey(t, db, database.APIKey{UserID: user.ID})
	modelOnlyKey, _ := dbgen.APIKey(t, db, database.APIKey{UserID: user.ID})
	oldKeyID := oldKey.ID
	latestKeyID := latestKey.ID
	dbgen.ChatMessage(t, db, database.ChatMessage{
		ChatID:        chat.ID,
		CreatedBy:     uuid.NullUUID{UUID: user.ID, Valid: true},
		ModelConfigID: uuid.NullUUID{UUID: model.ID, Valid: true},
		Role:          database.ChatMessageRoleUser,
		Visibility:    database.ChatMessageVisibilityBoth,
		APIKeyID:      sql.NullString{String: oldKeyID, Valid: true},
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
		APIKeyID:      sql.NullString{String: latestKeyID, Valid: true},
	})
	dbgen.ChatMessage(t, db, database.ChatMessage{
		ChatID:        chat.ID,
		CreatedBy:     uuid.NullUUID{UUID: user.ID, Valid: true},
		ModelConfigID: uuid.NullUUID{UUID: model.ID, Valid: true},
		Role:          database.ChatMessageRoleAssistant,
		Visibility:    database.ChatMessageVisibilityModel,
		Compressed:    true,
	})
	dbgen.ChatMessage(t, db, database.ChatMessage{
		ChatID:        chat.ID,
		CreatedBy:     uuid.NullUUID{UUID: user.ID, Valid: true},
		ModelConfigID: uuid.NullUUID{UUID: model.ID, Valid: true},
		Role:          database.ChatMessageRoleUser,
		Visibility:    database.ChatMessageVisibilityModel,
		APIKeyID:      sql.NullString{String: modelOnlyKey.ID, Valid: true},
	})

	got, err := db.GetLatestChatUserMessageAPIKeyID(ctx, chat.ID)
	require.NoError(t, err)
	require.True(t, got.Valid)
	require.Equal(t, latestKeyID, got.String)
}

func TestGetLatestChatUserMessageAPIKeyIDWithoutSummaryUsesLatestUser(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := t.Context()
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{})
	chat := dbgen.Chat(t, db, database.Chat{OrganizationID: org.ID, OwnerID: user.ID, LastModelConfigID: model.ID})
	oldKey, _ := dbgen.APIKey(t, db, database.APIKey{UserID: user.ID})
	latestKey, _ := dbgen.APIKey(t, db, database.APIKey{UserID: user.ID})

	dbgen.ChatMessage(t, db, database.ChatMessage{
		ChatID:        chat.ID,
		CreatedBy:     uuid.NullUUID{UUID: user.ID, Valid: true},
		ModelConfigID: uuid.NullUUID{UUID: model.ID, Valid: true},
		Role:          database.ChatMessageRoleUser,
		Visibility:    database.ChatMessageVisibilityBoth,
		APIKeyID:      sql.NullString{String: oldKey.ID, Valid: true},
	})
	dbgen.ChatMessage(t, db, database.ChatMessage{
		ChatID:        chat.ID,
		CreatedBy:     uuid.NullUUID{UUID: user.ID, Valid: true},
		ModelConfigID: uuid.NullUUID{UUID: model.ID, Valid: true},
		Role:          database.ChatMessageRoleUser,
		Visibility:    database.ChatMessageVisibilityBoth,
		APIKeyID:      sql.NullString{String: latestKey.ID, Valid: true},
	})

	got, err := db.GetLatestChatUserMessageAPIKeyID(ctx, chat.ID)
	require.NoError(t, err)
	require.True(t, got.Valid)
	require.Equal(t, latestKey.ID, got.String)
}

func TestGetLatestChatUserMessageAPIKeyIDSkipsUserMessagesWithoutAPIKey(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := t.Context()
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{})
	chat := dbgen.Chat(t, db, database.Chat{OrganizationID: org.ID, OwnerID: user.ID, LastModelConfigID: model.ID})
	userKey, _ := dbgen.APIKey(t, db, database.APIKey{UserID: user.ID})

	dbgen.ChatMessage(t, db, database.ChatMessage{
		ChatID:        chat.ID,
		CreatedBy:     uuid.NullUUID{UUID: user.ID, Valid: true},
		ModelConfigID: uuid.NullUUID{UUID: model.ID, Valid: true},
		Role:          database.ChatMessageRoleUser,
		Visibility:    database.ChatMessageVisibilityBoth,
		APIKeyID:      sql.NullString{String: userKey.ID, Valid: true},
	})
	dbgen.ChatMessage(t, db, database.ChatMessage{
		ChatID:        chat.ID,
		ModelConfigID: uuid.NullUUID{UUID: model.ID, Valid: true},
		Role:          database.ChatMessageRoleUser,
		Visibility:    database.ChatMessageVisibilityBoth,
	})

	got, err := db.GetLatestChatUserMessageAPIKeyID(ctx, chat.ID)
	require.NoError(t, err)
	require.True(t, got.Valid)
	require.Equal(t, userKey.ID, got.String)
}

func TestAIBridgeRoutingFailClosed(t *testing.T) {
	t.Parallel()

	providerID := uuid.New()
	chat := database.Chat{ID: uuid.New(), OwnerID: uuid.New()}
	route := aibridgeTestModelRoute(providerID, "primary-openai", database.AiProviderTypeOpenai)

	t.Run("NilFactory", func(t *testing.T) {
		t.Parallel()
		server := &Server{db: aibridgeTestDBWithAPIKeyID(t, chat.ID, "api-key-id"), aiGatewayRoutingEnabled: true}
		_, _, err := server.newModelFromConfig(t.Context(), chat, "openai", "gpt-4", chatprovider.ProviderAPIKeys{}, chatprovider.UserAgent(), nil, route)
		require.ErrorContains(t, err, "transport factory")
	})

	t.Run("FactoryError", func(t *testing.T) {
		t.Parallel()
		factory := &aibridgeTestFactory{err: xerrors.New("boom")}
		server := &Server{
			db:                       aibridgeTestDBWithAPIKeyID(t, chat.ID, "api-key-id"),
			aiGatewayRoutingEnabled:  true,
			aibridgeTransportFactory: aibridgeTestFactoryPointer(factory),
		}
		_, _, err := server.newModelFromConfig(t.Context(), chat, "openai", "gpt-4", chatprovider.ProviderAPIKeys{}, chatprovider.UserAgent(), nil, route)
		require.ErrorContains(t, err, "boom")
	})

	t.Run("MissingProviderName", func(t *testing.T) {
		t.Parallel()
		server := &Server{aiGatewayRoutingEnabled: true}
		missingNameRoute := aibridgeTestModelRoute(providerID, "", database.AiProviderTypeOpenai)
		_, _, err := server.newModelFromConfig(t.Context(), chat, "openai", "gpt-4", chatprovider.ProviderAPIKeys{}, chatprovider.UserAgent(), nil, missingNameRoute)
		require.ErrorContains(t, err, "AI provider name")
	})

	t.Run("MissingAPIKeyID", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		db.EXPECT().GetLatestChatUserMessageAPIKeyID(gomock.Any(), chat.ID).Return(sql.NullString{}, nil)
		factory := &aibridgeTestFactory{rt: roundTripFunc(func(*http.Request) (*http.Response, error) {
			t.Fatal("transport must not be used without an API key ID")
			return nil, xerrors.New("unreachable")
		})}
		server := &Server{
			db:                       db,
			aiGatewayRoutingEnabled:  true,
			aibridgeTransportFactory: aibridgeTestFactoryPointer(factory),
		}
		_, _, err := server.newModelFromConfig(t.Context(), chat, "openai", "gpt-4", chatprovider.ProviderAPIKeys{}, chatprovider.UserAgent(), nil, route)
		require.ErrorContains(t, err, "active turn API key ID")
	})

	t.Run("StaticModel", func(t *testing.T) {
		t.Parallel()
		server := &Server{aiGatewayRoutingEnabled: true}
		_, _, err := server.newModelFromConfig(t.Context(), chat, "openai", "gpt-4", chatprovider.ProviderAPIKeys{}, chatprovider.UserAgent(), nil, modelRoute{})
		require.ErrorContains(t, err, "concrete AI provider")
	})
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
		db:                       aibridgeTestDBWithAPIKeyID(t, chat.ID, apiKeyID),
		aiGatewayRoutingEnabled:  true,
		aibridgeTransportFactory: aibridgeTestFactoryPointer(factory),
	}
	provider := chattool.ComputerUseProviderOpenAI
	modelProvider, modelName, ok := chattool.DefaultComputerUseModel(provider)
	require.True(t, ok)

	model, debugEnabled, resolvedProvider, resolvedModel, err := server.resolveComputerUseModel(
		t.Context(),
		chat,
		chatprovider.ProviderAPIKeys{},
		provider,
		modelProvider,
		modelName,
		aibridgeTestModelRoute(providerID, "primary-openai", database.AiProviderTypeOpenai),
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
		db:                       aibridgeTestDBWithAPIKeyID(t, chat.ID, apiKeyID),
		aiGatewayRoutingEnabled:  true,
		aibridgeTransportFactory: aibridgeTestFactoryPointer(factory),
	}

	model, _, err := server.newModelFromConfig(t.Context(), chat, "openai", "gpt-4", chatprovider.ProviderAPIKeys{}, chatprovider.UserAgent(), nil, aibridgeTestModelRoute(providerID, "primary-openai", database.AiProviderTypeOpenai))
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

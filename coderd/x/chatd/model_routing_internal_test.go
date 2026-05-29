package chatd

import (
	"database/sql"
	"encoding/json"
	"fmt"
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

func aibridgeTestAIProvider(providerID uuid.UUID, providerName string, providerType database.AIProviderType) database.AIProvider {
	return database.AIProvider{
		ID:      providerID,
		Name:    providerName,
		Type:    providerType,
		Enabled: true,
	}
}

func aibridgeTestRoute(aiProvider database.AIProvider) resolvedModelRoute {
	return newAIGatewayModelRoute(aiProvider, string(aiProvider.Type), aiGatewayProviderAuth{})
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
	require.Equal(t, modelRouteKindDirect, route.kind)
	require.Equal(t, "openai", route.direct.ProviderHint)
	require.Equal(t, "provider-key", route.direct.Keys.APIKey("openai"))
	require.Equal(t, baseURL, route.direct.Keys.BaseURL("openai"))
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
		require.Empty(t, auth.Headers)
	})

	t.Run("BYOKDisabled", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		server := &Server{db: db, allowBYOK: false}
		auth, err := server.aiGatewayProviderAuthForUser(ctx, ownerID, provider, aiGatewayRequestFormatOpenAI)
		require.NoError(t, err)
		require.Empty(t, auth.Headers)
	})
}

func TestAIGatewayProviderAuthRedactsFormatting(t *testing.T) {
	t.Parallel()

	auth := aiGatewayProviderAuth{Headers: map[string]string{
		"Authorization": "Bearer sk-user",
		"X-Api-Key":     "sk-user",
	}}
	for _, formatted := range []string{
		fmt.Sprint(auth),
		fmt.Sprintf("%+v", auth),
		fmt.Sprintf("%#v", auth),
	} {
		require.NotContains(t, formatted, "sk-user")
		require.NotContains(t, formatted, "Bearer sk-user")
		require.Contains(t, formatted, "redacted")
	}
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
		require.Equal(t, modelRouteKindAIGateway, route.kind)
		require.Equal(t, "Bearer sk-user", route.aiGateway.ProviderAuth.Headers["Authorization"])
	})

	t.Run("CentralProviderCredentialsNotForwarded", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		db.EXPECT().GetAIProviderByID(gomock.Any(), providerID).Return(provider, nil)

		server := &Server{db: db, aiGatewayRoutingEnabled: true, allowBYOK: false}
		route, err := server.resolveModelRouteForConfig(ctx, ownerID, modelConfig, chatprovider.ProviderAPIKeys{})
		require.NoError(t, err)
		require.Equal(t, modelRouteKindAIGateway, route.kind)
		require.Empty(t, route.aiGateway.ProviderAuth.Headers)
	})
}

func TestAIGatewayModelForwardsProviderAuth(t *testing.T) {
	t.Parallel()

	type seenRequest struct {
		authorization string
		xAPIKey       string
		coderToken    string
		apiKeyID      string
		path          string
	}
	newServer := func(t *testing.T, provider database.AIProvider, auth aiGatewayProviderAuth, seen chan seenRequest) (*Server, resolvedModelRoute) {
		factory := &aibridgeTestFactory{rt: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			apiKeyID, _ := aibridge.DelegatedAPIKeyIDFromContext(req.Context())
			seen <- seenRequest{
				authorization: req.Header.Get("Authorization"),
				xAPIKey:       req.Header.Get("X-Api-Key"),
				coderToken:    req.Header.Get(aibridge.HeaderCoderToken),
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
		route := newAIGatewayModelRoute(provider, string(provider.Type), auth)
		return server, route
	}

	t.Run("OpenAI", func(t *testing.T) {
		t.Parallel()

		seen := make(chan seenRequest, 1)
		provider := aibridgeTestAIProvider(uuid.New(), "primary-openai", database.AiProviderTypeOpenai)
		server, route := newServer(t, provider, aiGatewayProviderAuth{
			Headers: map[string]string{"Authorization": "Bearer sk-user"},
		}, seen)
		apiKeyID := uuid.NewString()
		model, err := server.newModel(t.Context(), aibridgeTestRequest(database.Chat{ID: uuid.New(), OwnerID: uuid.New()}, "gpt-4"), route, modelBuildOptions{ActiveAPIKeyID: apiKeyID, RecordHTTP: true})
		require.NoError(t, err)
		_, err = model.Generate(t.Context(), fantasy.Call{Prompt: []fantasy.Message{{Role: fantasy.MessageRoleUser, Content: []fantasy.MessagePart{fantasy.TextPart{Text: "hello"}}}}})
		require.NoError(t, err)

		got := <-seen
		require.Equal(t, "Bearer sk-user", got.authorization)
		require.Empty(t, got.xAPIKey)
		require.Equal(t, aibridgeDelegatedBYOKMarker, got.coderToken)
		require.Equal(t, apiKeyID, got.apiKeyID)
		require.Equal(t, "/v1/responses", got.path)
	})

	t.Run("Anthropic", func(t *testing.T) {
		t.Parallel()

		seen := make(chan seenRequest, 1)
		provider := aibridgeTestAIProvider(uuid.New(), "primary-anthropic", database.AiProviderTypeAnthropic)
		server, route := newServer(t, provider, aiGatewayProviderAuth{
			Headers: map[string]string{"X-Api-Key": "sk-user"},
		}, seen)
		apiKeyID := uuid.NewString()
		model, err := server.newModel(t.Context(), aibridgeTestRequest(database.Chat{ID: uuid.New(), OwnerID: uuid.New()}, "claude-haiku-4-5"), route, modelBuildOptions{ActiveAPIKeyID: apiKeyID})
		require.NoError(t, err)
		_, err = model.Generate(t.Context(), fantasy.Call{Prompt: []fantasy.Message{{Role: fantasy.MessageRoleUser, Content: []fantasy.MessagePart{fantasy.TextPart{Text: "hello"}}}}})
		require.NoError(t, err)

		got := <-seen
		require.Equal(t, "sk-user", got.xAPIKey)
		require.Equal(t, aibridgeDelegatedBYOKMarker, got.coderToken)
		require.Equal(t, apiKeyID, got.apiKeyID)
		require.Equal(t, "/v1/messages", got.path)
	})

	t.Run("NoUserKeyLeavesPlaceholderForAIBridged", func(t *testing.T) {
		t.Parallel()

		seen := make(chan seenRequest, 1)
		provider := aibridgeTestAIProvider(uuid.New(), "primary-openai", database.AiProviderTypeOpenai)
		server, route := newServer(t, provider, aiGatewayProviderAuth{}, seen)
		apiKeyID := uuid.NewString()
		model, err := server.newModel(t.Context(), aibridgeTestRequest(database.Chat{ID: uuid.New(), OwnerID: uuid.New()}, "gpt-4"), route, modelBuildOptions{ActiveAPIKeyID: apiKeyID})
		require.NoError(t, err)
		_, err = model.Generate(t.Context(), fantasy.Call{Prompt: []fantasy.Message{{Role: fantasy.MessageRoleUser, Content: []fantasy.MessagePart{fantasy.TextPart{Text: "hello"}}}}})
		require.NoError(t, err)

		got := <-seen
		require.Equal(t, "Bearer "+aibridgePlaceholderAPIKey, got.authorization)
		require.Empty(t, got.xAPIKey)
		require.Empty(t, got.coderToken)
		require.Equal(t, apiKeyID, got.apiKeyID)
	})
}

func TestAIGatewayRoundTripperPreservesSessionHeaders(t *testing.T) {
	t.Parallel()

	seen := make(chan http.Header, 1)
	rt := &aiGatewayRoundTripper{
		base: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			seen <- req.Header.Clone()
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{},
				Body:       io.NopCloser(strings.NewReader("")),
				Request:    req,
			}, nil
		}),
		apiKeyID: uuid.NewString(),
	}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://coder-aibridge/v1/responses", nil)
	require.NoError(t, err)
	req.Header.Set(chatprovider.HeaderCoderOwnerID, "owner-id")
	req.Header.Set(chatprovider.HeaderCoderChatID, "chat-id")
	req.Header.Set(chatprovider.HeaderCoderSubchatID, "subchat-id")
	req.Header.Set(chatprovider.HeaderCoderWorkspaceID, "workspace-id")

	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	got := <-seen
	require.Equal(t, "owner-id", got.Get(chatprovider.HeaderCoderOwnerID))
	require.Equal(t, "chat-id", got.Get(chatprovider.HeaderCoderChatID))
	require.Equal(t, "subchat-id", got.Get(chatprovider.HeaderCoderSubchatID))
	require.Equal(t, "workspace-id", got.Get(chatprovider.HeaderCoderWorkspaceID))
}

func TestAIGatewayQuickgenPromptSerializesFinalAssistantMessage(t *testing.T) {
	t.Parallel()

	const quickgenInput = "summarize failed workspace build logs"
	tests := []struct {
		name         string
		providerType database.AIProviderType
		modelName    string
		wantPath     string
	}{
		{
			name:         "OpenAIResponses",
			providerType: database.AiProviderTypeOpenai,
			modelName:    "gpt-4",
			wantPath:     "/v1/responses",
		},
		{
			name:         "AnthropicMessages",
			providerType: database.AiProviderTypeAnthropic,
			modelName:    "claude-haiku-4-5",
			wantPath:     "/v1/messages",
		},
		{
			name:         "OpenAICompatibleChatCompletions",
			providerType: database.AiProviderTypeGoogle,
			modelName:    "gemini-2.5-flash",
			wantPath:     "/v1/chat/completions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			provider := aibridgeTestAIProvider(uuid.New(), "primary-"+tt.name, tt.providerType)
			route := aibridgeTestRoute(provider)
			model, requests := newCapturedAIGatewayModel(
				t,
				provider,
				tt.modelName,
				aibridgeQuickgenTitleResponse(tt.providerType),
			)

			title, _, err := generateStructuredTitleWithUsagePromptMode(
				t.Context(),
				model,
				titleGenerationPrompt,
				quickgenInput,
				quickgenPromptModeForRoute(route),
			)
			require.NoError(t, err)
			require.Equal(t, "Failed workspace logs", title)

			got := <-requests
			require.Equal(t, tt.wantPath, got.path)
			switch tt.providerType {
			case database.AiProviderTypeOpenai:
				assertAIGatewayFinalRole(t, got.body, "input", "assistant", quickgenInput)
			case database.AiProviderTypeAnthropic:
				assertAIGatewayFinalRole(t, got.body, "messages", "assistant", quickgenInput)
			default:
				assertAIGatewayFinalRole(t, got.body, "messages", "assistant", quickgenInput)
				require.Equal(t, "required", got.body["tool_choice"])
			}
		})
	}
}

func TestAIGatewayChatTurnSerializesFinalUserMessage(t *testing.T) {
	t.Parallel()

	const userInput = "please inspect the failing test"
	model, requests := newCapturedAIGatewayModel(
		t,
		aibridgeTestAIProvider(uuid.New(), "primary-openai", database.AiProviderTypeOpenai),
		"gpt-4",
		`{"id":"resp_test","object":"response","created_at":0,"status":"completed","model":"gpt-4","output":[{"id":"msg_test","type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`,
	)

	_, err := model.Generate(t.Context(), fantasy.Call{Prompt: fantasy.Prompt{
		{
			Role: fantasy.MessageRoleSystem,
			Content: []fantasy.MessagePart{
				fantasy.TextPart{Text: "You are a coding assistant."},
			},
		},
		{
			Role: fantasy.MessageRoleUser,
			Content: []fantasy.MessagePart{
				fantasy.TextPart{Text: userInput},
			},
		},
	}})
	require.NoError(t, err)

	got := <-requests
	require.Equal(t, "/v1/responses", got.path)
	assertAIGatewayFinalRole(t, got.body, "input", "user", userInput)
}

type capturedAIGatewayRequest struct {
	path string
	body map[string]any
}

func newCapturedAIGatewayModel(
	t *testing.T,
	provider database.AIProvider,
	modelName string,
	responseBody string,
) (fantasy.LanguageModel, <-chan capturedAIGatewayRequest) {
	t.Helper()

	requests := make(chan capturedAIGatewayRequest, 1)
	factory := &aibridgeTestFactory{rt: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		var body map[string]any
		require.NoError(t, json.NewDecoder(req.Body).Decode(&body))
		requests <- capturedAIGatewayRequest{path: req.URL.Path, body: body}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(responseBody)),
			Request:    req,
		}, nil
	})}
	server := &Server{
		aiGatewayRoutingEnabled:  true,
		aibridgeTransportFactory: aibridgeTestFactoryPointer(factory),
	}
	model, err := server.newModel(
		t.Context(),
		aibridgeTestRequest(database.Chat{ID: uuid.New(), OwnerID: uuid.New()}, modelName),
		aibridgeTestRoute(provider),
		modelBuildOptions{ActiveAPIKeyID: uuid.NewString()},
	)
	require.NoError(t, err)
	return model, requests
}

func aibridgeQuickgenTitleResponse(providerType database.AIProviderType) string {
	switch providerType {
	case database.AiProviderTypeOpenai:
		return `{"id":"resp_test","object":"response","created_at":0,"status":"completed","model":"gpt-4","output":[{"id":"msg_test","type":"message","role":"assistant","content":[{"type":"output_text","text":"{\"title\":\"Failed workspace logs\"}"}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`
	case database.AiProviderTypeAnthropic:
		return `{"id":"msg_test","type":"message","role":"assistant","model":"claude-haiku-4-5","content":[{"type":"tool_use","id":"toolu_test","name":"propose_title","input":{"title":"Failed workspace logs"}}],"stop_reason":"tool_use","stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":1}}`
	default:
		return `{"id":"chatcmpl_test","object":"chat.completion","created":0,"model":"gemini-2.5-flash","choices":[{"index":0,"message":{"role":"assistant","content":"","tool_calls":[{"id":"call_structured_output","type":"function","function":{"name":"propose_title","arguments":"{\"title\":\"Failed workspace logs\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`
	}
}

func assertAIGatewayFinalRole(
	t *testing.T,
	body map[string]any,
	messageKey string,
	role string,
	text string,
) {
	t.Helper()

	messages, ok := body[messageKey].([]any)
	require.True(t, ok)
	require.NotEmpty(t, messages)
	last, ok := messages[len(messages)-1].(map[string]any)
	require.True(t, ok)
	require.Equal(t, role, last["role"])
	requireRoleText(t, messages, "user", text)
}

func requireRoleText(t *testing.T, messages []any, role string, text string) {
	t.Helper()

	for _, message := range messages {
		messageMap, ok := message.(map[string]any)
		if !ok || messageMap["role"] != role {
			continue
		}
		if messageContentContainsText(messageMap["content"], text) {
			return
		}
	}
	t.Fatalf("no %s message contained %q", role, text)
}

func messageContentContainsText(value any, text string) bool {
	switch typed := value.(type) {
	case string:
		return strings.Contains(typed, text)
	case []any:
		for _, item := range typed {
			itemMap, ok := item.(map[string]any)
			if !ok {
				continue
			}
			itemText, ok := itemMap["text"].(string)
			if ok && strings.Contains(itemText, text) {
				return true
			}
		}
	}
	return false
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
		_, err := server.newModel(t.Context(), aibridgeTestRequest(chat, "gpt-4"), aibridgeTestRoute(aiProvider), modelBuildOptions{ActiveAPIKeyID: uuid.NewString()})
		require.ErrorContains(t, err, "transport factory")
	})

	t.Run("FactoryError", func(t *testing.T) {
		t.Parallel()
		factory := &aibridgeTestFactory{err: xerrors.New("boom")}
		server := &Server{
			aiGatewayRoutingEnabled:  true,
			aibridgeTransportFactory: aibridgeTestFactoryPointer(factory),
		}
		_, err := server.newModel(t.Context(), aibridgeTestRequest(chat, "gpt-4"), aibridgeTestRoute(aiProvider), modelBuildOptions{ActiveAPIKeyID: uuid.NewString()})
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
		_, err := server.newModel(t.Context(), aibridgeTestRequest(chat, "gpt-4"), newAIGatewayModelRoute(database.AIProvider{}, "", aiGatewayProviderAuth{}), modelBuildOptions{ActiveAPIKeyID: uuid.NewString()})
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
	}, newDirectModelRoute("openai", chatprovider.ProviderAPIKeys{OpenAI: "sk-test"}), modelBuildOptions{})
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

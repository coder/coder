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
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
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

func aibridgeTestRoute(aiProvider database.AIProvider) aiGatewayModelRoute {
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
		{name: "OpenAI", providerType: database.AIProviderTypeOpenai, wantProvider: "openai", wantBaseURL: "http://coder-aibridge/v1"},
		{name: "Anthropic", providerType: database.AIProviderTypeAnthropic, wantProvider: "anthropic", wantBaseURL: "http://coder-aibridge"},
		{name: "Bedrock", providerType: database.AIProviderTypeBedrock, wantProvider: "anthropic", wantBaseURL: "http://coder-aibridge"},
		{name: "Google", providerType: database.AIProviderTypeGoogle, wantProvider: "openai-compat", wantBaseURL: "http://coder-aibridge/v1"},
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
		Type:    database.AIProviderTypeOpenai,
		Name:    "primary-openai",
		Enabled: true,
		BaseUrl: baseURL,
	}, nil)

	server := &Server{db: db}
	route, err := server.resolveModelRouteForConfig(ctx, ownerID, database.ChatModelConfig{
		AIProviderID: uuid.NullUUID{UUID: providerID, Valid: true},
	})
	require.NoError(t, err)
	require.Equal(t, "openai", route.ModelProviderHint)
	require.Equal(t, providerID, route.Provider.ID)
	require.Equal(t, baseURL, route.Provider.BaseUrl)
}

func TestAIGatewayProviderAuthForUser(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	ownerID := uuid.New()
	providerID := uuid.New()
	provider := database.AIProvider{ID: providerID, Type: database.AIProviderTypeOpenai, Enabled: true}

	t.Run("OpenAIUserKey", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		db.EXPECT().GetUserAIProviderKeyByProviderID(gomock.Any(), database.GetUserAIProviderKeyByProviderIDParams{
			UserID:       ownerID,
			AIProviderID: providerID,
		}).Return(database.UserAIProviderKey{APIKey: "sk-user"}, nil)

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
		}).Return(database.UserAIProviderKey{APIKey: "sk-user"}, nil)

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
		}).Return(database.UserAIProviderKey{}, sql.ErrNoRows)

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
		Type:    database.AIProviderTypeOpenai,
		Name:    "primary-openai",
		Enabled: true,
	}
	modelConfig := database.ChatModelConfig{
		ID:           uuid.New(),
		Model:        "gpt-4",
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
		}).Return(database.UserAIProviderKey{APIKey: "sk-user"}, nil)

		server := &Server{db: db, allowBYOK: true}
		route, err := server.resolveModelRouteForConfig(ctx, ownerID, modelConfig)
		require.NoError(t, err)
		require.Equal(t, "Bearer sk-user", route.ProviderAuth.Headers["Authorization"])
	})

	t.Run("CentralProviderCredentialsNotForwarded", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		db.EXPECT().GetAIProviderByID(gomock.Any(), providerID).Return(provider, nil)

		server := &Server{db: db, allowBYOK: false}
		route, err := server.resolveModelRouteForConfig(ctx, ownerID, modelConfig)
		require.NoError(t, err)
		require.Empty(t, route.ProviderAuth.Headers)
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
	newServer := func(t *testing.T, provider database.AIProvider, auth aiGatewayProviderAuth, seen chan seenRequest) (*Server, aiGatewayModelRoute) {
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
			if provider.Type == database.AIProviderTypeAnthropic {
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
			aibridgeTransportFactory: aibridgeTestFactoryPointer(factory),
		}
		route := newAIGatewayModelRoute(provider, string(provider.Type), auth)
		return server, route
	}

	t.Run("OpenAI", func(t *testing.T) {
		t.Parallel()

		seen := make(chan seenRequest, 1)
		provider := aibridgeTestAIProvider(uuid.New(), "primary-openai", database.AIProviderTypeOpenai)
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
		provider := aibridgeTestAIProvider(uuid.New(), "primary-anthropic", database.AIProviderTypeAnthropic)
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
		provider := aibridgeTestAIProvider(uuid.New(), "primary-openai", database.AIProviderTypeOpenai)
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

func sqlNullString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: value != ""}
}

func TestAIBridgeRoutingFailClosed(t *testing.T) {
	t.Parallel()

	providerID := uuid.New()
	chat := database.Chat{ID: uuid.New(), OwnerID: uuid.New()}
	aiProvider := aibridgeTestAIProvider(providerID, "primary-openai", database.AIProviderTypeOpenai)

	t.Run("NilFactory", func(t *testing.T) {
		t.Parallel()
		server := &Server{}
		_, err := server.newModel(t.Context(), aibridgeTestRequest(chat, "gpt-4"), aibridgeTestRoute(aiProvider), modelBuildOptions{ActiveAPIKeyID: uuid.NewString()})
		require.ErrorContains(t, err, "transport factory")
	})

	t.Run("FactoryError", func(t *testing.T) {
		t.Parallel()
		factory := &aibridgeTestFactory{err: xerrors.New("boom")}
		server := &Server{
			aibridgeTransportFactory: aibridgeTestFactoryPointer(factory),
		}
		_, err := server.newModel(t.Context(), aibridgeTestRequest(chat, "gpt-4"), aibridgeTestRoute(aiProvider), modelBuildOptions{ActiveAPIKeyID: uuid.NewString()})
		require.ErrorContains(t, err, "boom")
	})

	t.Run("MissingProviderName", func(t *testing.T) {
		t.Parallel()
		server := &Server{}
		missingNameProvider := aibridgeTestAIProvider(providerID, "", database.AIProviderTypeOpenai)
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
			aibridgeTransportFactory: aibridgeTestFactoryPointer(factory),
		}
		_, err := server.newModel(t.Context(), aibridgeTestRequest(chat, "gpt-4"), aibridgeTestRoute(aiProvider), modelBuildOptions{})
		require.ErrorContains(t, err, "active turn API key ID")

		classified := chaterror.Classify(err)
		require.Equal(t, codersdk.ChatErrorKindMissingKey, classified.Kind,
			"production path must return a pre-classified missing_key error")
		require.False(t, classified.Retryable)
	})

	t.Run("OpenRouterMisconfiguredAsOpenAI", func(t *testing.T) {
		t.Parallel()
		factory := &aibridgeTestFactory{rt: roundTripFunc(func(*http.Request) (*http.Response, error) {
			t.Fatal("transport must not be used for invalid provider config")
			return nil, xerrors.New("unreachable")
		})}
		server := &Server{
			aibridgeTransportFactory: aibridgeTestFactoryPointer(factory),
		}
		provider := aibridgeTestAIProvider(providerID, "openrouter", database.AIProviderTypeOpenai)
		_, err := server.newModel(
			t.Context(),
			aibridgeTestRequest(chat, "anthropic/claude-opus-4.6"),
			aibridgeTestRoute(provider),
			modelBuildOptions{ActiveAPIKeyID: uuid.NewString()},
		)
		require.ErrorContains(t, err, "does not support slash-namespaced models")
		classified := chaterror.Classify(err)
		require.Equal(t, codersdk.ChatErrorKindConfig, classified.Kind)
		require.False(t, classified.Retryable)
	})

	t.Run("StaticModel", func(t *testing.T) {
		t.Parallel()
		server := &Server{}
		_, err := server.newModel(t.Context(), aibridgeTestRequest(chat, "gpt-4"), newAIGatewayModelRoute(database.AIProvider{}, "", aiGatewayProviderAuth{}), modelBuildOptions{ActiveAPIKeyID: uuid.NewString()})
		require.ErrorContains(t, err, "concrete AI provider")
	})
}

func TestAIBridgeGatewayProviderTypesPreserveSlashModelID(t *testing.T) {
	t.Parallel()

	const modelName = "anthropic/claude-opus-4.6"
	tests := []struct {
		name         string
		providerName string
		providerType database.AIProviderType
	}{
		{
			name:         "OpenRouter",
			providerName: "openrouter",
			providerType: database.AIProviderTypeOpenrouter,
		},
		{
			name:         "OpenAICompat",
			providerName: "openai-compatible-relay",
			providerType: database.AIProviderTypeOpenaiCompat,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			type seenRequest struct {
				model string
				path  string
			}
			seen := make(chan seenRequest, 1)
			factory := &aibridgeTestFactory{rt: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				body, err := io.ReadAll(req.Body)
				require.NoError(t, err)
				var payload struct {
					Model string `json:"model"`
				}
				require.NoError(t, json.Unmarshal(body, &payload))
				seen <- seenRequest{model: payload.Model, path: req.URL.Path}

				var responsePayload map[string]any
				if strings.Contains(req.URL.Path, "/responses") {
					responsePayload = map[string]any{
						"id":         "resp_test",
						"object":     "response",
						"created_at": 0,
						"status":     "completed",
						"model":      modelName,
						"output": []map[string]any{{
							"id":      "msg_test",
							"type":    "message",
							"role":    "assistant",
							"content": []map[string]any{{"type": "output_text", "text": "hello"}},
						}},
						"usage": map[string]any{"input_tokens": 1, "output_tokens": 1, "total_tokens": 2},
					}
				} else {
					responsePayload = map[string]any{
						"id":      "chatcmpl-test",
						"object":  "chat.completion",
						"created": 0,
						"model":   modelName,
						"choices": []map[string]any{{
							"index":         0,
							"message":       map[string]any{"role": "assistant", "content": "hello"},
							"finish_reason": "stop",
						}},
						"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
					}
				}
				responseBody, err := json.Marshal(responsePayload)
				require.NoError(t, err)
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(strings.NewReader(string(responseBody))),
					Request:    req,
				}, nil
			})}
			chat := database.Chat{ID: uuid.New(), OwnerID: uuid.New()}
			server := &Server{
				aibridgeTransportFactory: aibridgeTestFactoryPointer(factory),
			}

			model, err := server.newModel(
				t.Context(),
				aibridgeTestRequest(chat, modelName),
				aibridgeTestRoute(aibridgeTestAIProvider(uuid.New(), tt.providerName, tt.providerType)),
				modelBuildOptions{ActiveAPIKeyID: uuid.NewString()},
			)
			require.NoError(t, err)
			_, err = model.Generate(t.Context(), fantasy.Call{Prompt: []fantasy.Message{{
				Role:    fantasy.MessageRoleUser,
				Content: []fantasy.MessagePart{fantasy.TextPart{Text: "hello"}},
			}}})
			require.NoError(t, err)

			got := <-seen
			require.NotEmpty(t, got.path)
			require.Equal(t, modelName, got.model)
			require.Equal(t, tt.providerName, factory.providerName)
			require.Equal(t, aibridge.SourceAgents, factory.source)
		})
	}
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
		aibridgeTransportFactory: aibridgeTestFactoryPointer(factory),
	}
	provider := codersdk.ChatComputerUseProviderOpenAI
	modelProvider, modelName, ok := chattool.DefaultComputerUseModel(provider)
	require.True(t, ok)

	ctx := aibridge.WithDelegatedAPIKeyID(t.Context(), "context-key-must-be-ignored")
	model, debugEnabled, resolvedProvider, resolvedModel, err := server.resolveComputerUseModel(
		ctx,
		chat,
		aibridgeTestRoute(aibridgeTestAIProvider(providerID, "primary-openai", database.AIProviderTypeOpenai)),
		provider,
		modelProvider,
		modelName,
		modelBuildOptions{ActiveAPIKeyID: apiKeyID},
	)
	require.NoError(t, err)
	require.NotNil(t, model)
	require.False(t, debugEnabled)
	require.EqualValues(t, codersdk.ChatComputerUseProviderOpenAI, resolvedProvider)
	require.Equal(t, modelName, resolvedModel)
	require.Equal(t, "primary-openai", factory.providerName)
	require.Equal(t, aibridge.SourceAgents, factory.source)
}

func TestResolveComputerUseModel_AIGatewayMissingAPIKeyID(t *testing.T) {
	t.Parallel()

	providerID := uuid.New()
	factory := &aibridgeTestFactory{rt: roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("transport must not be used without an API key ID")
		return nil, xerrors.New("unreachable")
	})}
	chat := database.Chat{ID: uuid.New(), OwnerID: uuid.New()}
	server := &Server{
		aibridgeTransportFactory: aibridgeTestFactoryPointer(factory),
	}
	provider := codersdk.ChatComputerUseProviderOpenAI
	modelProvider, modelName, ok := chattool.DefaultComputerUseModel(provider)
	require.True(t, ok)

	model, debugEnabled, resolvedProvider, resolvedModel, err := server.resolveComputerUseModel(
		t.Context(),
		chat,
		aibridgeTestRoute(aibridgeTestAIProvider(providerID, "primary-openai", database.AIProviderTypeOpenai)),
		provider,
		modelProvider,
		modelName,
		modelBuildOptions{}, // no ActiveAPIKeyID
	)
	require.Error(t, err)
	require.Nil(t, model)
	require.False(t, debugEnabled)
	require.Empty(t, resolvedProvider)
	require.Empty(t, resolvedModel)
	require.Contains(t, err.Error(), `resolve computer use model for provider "openai" model "gpt-5.5"`)
	require.Contains(t, err.Error(), "active turn API key ID")
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
		aibridgeTransportFactory: aibridgeTestFactoryPointer(factory),
	}

	ctx := aibridge.WithDelegatedAPIKeyID(t.Context(), "context-key-must-be-ignored")
	model, err := server.newModel(ctx, aibridgeTestRequest(chat, "gpt-4"), aibridgeTestRoute(aibridgeTestAIProvider(providerID, "primary-openai", database.AIProviderTypeOpenai)), modelBuildOptions{ActiveAPIKeyID: apiKeyID, RecordHTTP: true})
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

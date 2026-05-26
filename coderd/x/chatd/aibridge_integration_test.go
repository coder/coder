package chatd_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/aibridge"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	dbpubsub "github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestChatProviderRoutingIntegration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                    string
		aiGatewayRoutingEnabled bool
		allowBYOK               bool
		centralKey              string
		userKey                 string
		gateway                 *providerRoutingGatewayExpectation
		provider                providerRoutingProviderExpectation
	}{
		{
			name:                    "AI Gateway routing with BYOK",
			aiGatewayRoutingEnabled: true,
			allowBYOK:               true,
			centralKey:              "sk-central-gateway-byok",
			userKey:                 "sk-user-byok",
			provider: providerRoutingProviderExpectation{
				authorization: "Bearer sk-user-byok",
			},
			gateway: &providerRoutingGatewayExpectation{
				coderToken: "sk-user-byok",
			},
		},
		{
			name:                    "AI Gateway routing without BYOK",
			aiGatewayRoutingEnabled: true,
			allowBYOK:               false,
			centralKey:              "sk-central-gateway-no-byok",
			userKey:                 "sk-user-ignored-gateway",
			provider: providerRoutingProviderExpectation{
				authorization: "Bearer coder-aibridge",
			},
			gateway: &providerRoutingGatewayExpectation{
				authorization: "Bearer coder-aibridge",
			},
		},
		{
			name:                    "direct routing with BYOK",
			aiGatewayRoutingEnabled: false,
			allowBYOK:               true,
			centralKey:              "sk-central-direct-byok",
			userKey:                 "sk-user-direct-byok",
			provider: providerRoutingProviderExpectation{
				authorization: "Bearer sk-user-direct-byok",
			},
		},
		{
			name:                    "direct routing without BYOK",
			aiGatewayRoutingEnabled: false,
			allowBYOK:               false,
			centralKey:              "sk-central-direct",
			userKey:                 "sk-user-ignored-direct",
			provider: providerRoutingProviderExpectation{
				authorization: "Bearer sk-central-direct",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)
			db, ps := dbtestutil.NewDB(t)
			assistantText := "provider routing response for " + strings.ToLower(tt.name)
			providerRequests := &providerRoutingProviderRecorder{}
			openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
				providerRequests.record(req)
				if !req.Stream {
					return chattest.OpenAINonStreamingResponse(`{"title":"Provider Routing"}`)
				}
				return chattest.OpenAIStreamingResponse(
					chattest.OpenAITextChunks(assistantText)...,
				)
			})

			user, org, provider, model := seedProviderRoutingChatDependencies(
				t,
				db,
				openAIURL,
				tt.centralKey,
			)
			if tt.userKey != "" {
				_, err := db.UpsertUserAIProviderKey(ctx, database.UpsertUserAIProviderKeyParams{
					ID:           uuid.New(),
					UserID:       user.ID,
					AIProviderID: provider.ID,
					APIKey:       tt.userKey,
				})
				require.NoError(t, err)
			}
			apiKey, _ := dbgen.APIKey(t, db, database.APIKey{UserID: user.ID})

			var gatewayRequests *providerRoutingGatewayRecorder
			unexpectedGateway := &unexpectedProviderRoutingGatewayFactory{}
			var factoryPtr *atomic.Pointer[aibridge.TransportFactory]
			if tt.gateway != nil {
				gatewayRequests = newProviderRoutingGatewayRecorder(t, openAIURL)
				factoryPtr = providerRoutingFactoryPointer(gatewayRequests)
			} else {
				factoryPtr = providerRoutingFactoryPointer(unexpectedGateway)
			}

			server := newProviderRoutingTestServer(
				t,
				db,
				ps,
				tt.allowBYOK,
				tt.aiGatewayRoutingEnabled,
				factoryPtr,
			)
			chat, err := server.CreateChat(ctx, chatd.CreateOptions{
				OrganizationID: org.ID,
				OwnerID:        user.ID,
				Title:          uniqueResponsesTitle(t, "provider-routing"),
				ModelConfigID:  model.ID,
				APIKeyID:       apiKey.ID,
				InitialUserContent: []codersdk.ChatMessagePart{
					codersdk.ChatMessageText("say hello"),
				},
			})
			require.NoError(t, err)

			waitForChatProcessed(ctx, t, db, chat.ID, server)
			requireProviderRoutingChatSucceeded(ctx, t, db, chat.ID, assistantText)

			providerMainRequests := providerRequests.streamingRequests()
			require.Len(t, providerMainRequests, 1)
			require.Equal(t, "/responses", providerMainRequests[0].path)
			require.Empty(t, providerMainRequests[0].xAPIKey)
			require.Equal(t, tt.provider.authorization, providerMainRequests[0].authorization)
			if !tt.allowBYOK {
				require.NotContains(t, providerMainRequests[0].authorization, tt.userKey)
			}

			if tt.gateway != nil {
				gatewayMainRequests := gatewayRequests.streamingRequests()
				require.Len(t, gatewayMainRequests, 1)
				gatewayRequest := gatewayMainRequests[0]
				require.Equal(t, provider.Name, gatewayRequest.providerName)
				require.Equal(t, aibridge.SourceAgents, gatewayRequest.source)
				require.Equal(t, "/v1/responses", gatewayRequest.path)
				require.Equal(t, tt.gateway.authorization, gatewayRequest.authorization)
				require.Empty(t, gatewayRequest.xAPIKey)
				require.Equal(t, tt.gateway.coderToken, gatewayRequest.coderToken)
				require.True(t, gatewayRequest.delegatedAPIKeyIDSet)
				require.Equal(t, apiKey.ID, gatewayRequest.delegatedAPIKeyID)
				if tt.userKey != "" && !tt.allowBYOK {
					require.NotContains(t, gatewayRequest.authorization, tt.userKey)
					require.NotContains(t, gatewayRequest.xAPIKey, tt.userKey)
				}
			} else {
				require.Zero(t, unexpectedGateway.calls.Load())
			}
		})
	}
}

type providerRoutingGatewayExpectation struct {
	authorization string
	coderToken    string
}

type providerRoutingProviderExpectation struct {
	authorization string
}

type providerRoutingProviderRequest struct {
	path          string
	authorization string
	xAPIKey       string
	stream        bool
}

type providerRoutingProviderRecorder struct {
	mu       sync.Mutex
	requests []providerRoutingProviderRequest
}

func (r *providerRoutingProviderRecorder) record(req *chattest.OpenAIRequest) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = append(r.requests, providerRoutingProviderRequest{
		path:          req.URL.Path,
		authorization: req.Header.Get("Authorization"),
		xAPIKey:       req.Header.Get("X-Api-Key"),
		stream:        req.Stream,
	})
}

func (r *providerRoutingProviderRecorder) streamingRequests() []providerRoutingProviderRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	requests := make([]providerRoutingProviderRequest, 0, len(r.requests))
	for _, request := range r.requests {
		if request.stream {
			requests = append(requests, request)
		}
	}
	return requests
}

type providerRoutingGatewayRequest struct {
	providerName         string
	source               aibridge.Source
	path                 string
	authorization        string
	xAPIKey              string
	stream               bool
	delegatedAPIKeyID    string
	delegatedAPIKeyIDSet bool
	coderToken           string
}

type providerRoutingGatewayRecorder struct {
	target *url.URL
	mu     sync.Mutex
	seen   []providerRoutingGatewayRequest
}

func newProviderRoutingGatewayRecorder(t *testing.T, target string) *providerRoutingGatewayRecorder {
	t.Helper()
	targetURL, err := url.Parse(target)
	require.NoError(t, err)
	return &providerRoutingGatewayRecorder{target: targetURL}
}

func (r *providerRoutingGatewayRecorder) TransportFor(providerName string, source aibridge.Source) (http.RoundTripper, error) {
	return roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body, err := readRoundTripBody(req)
		if err != nil {
			return nil, err
		}
		var decoded struct {
			Stream bool `json:"stream"`
		}
		if err := json.Unmarshal(body, &decoded); err != nil {
			return nil, err
		}
		delegatedAPIKeyID, delegatedAPIKeyIDSet := aibridge.DelegatedAPIKeyIDFromContext(req.Context())

		r.mu.Lock()
		r.seen = append(r.seen, providerRoutingGatewayRequest{
			providerName:         providerName,
			source:               source,
			path:                 req.URL.Path,
			authorization:        req.Header.Get("Authorization"),
			xAPIKey:              req.Header.Get("X-Api-Key"),
			stream:               decoded.Stream,
			delegatedAPIKeyID:    delegatedAPIKeyID,
			delegatedAPIKeyIDSet: delegatedAPIKeyIDSet,
			coderToken:           req.Header.Get(aibridge.HeaderCoderToken),
		})
		r.mu.Unlock()

		forwardedURL, err := rewriteProviderRoutingGatewayURL(r.target, req.URL)
		if err != nil {
			return nil, err
		}
		forwarded := req.Clone(req.Context())
		forwarded.URL = forwardedURL
		forwarded.Host = ""
		if coderToken := forwarded.Header.Get(aibridge.HeaderCoderToken); coderToken != "" {
			forwarded.Header.Del(aibridge.HeaderCoderToken)
			forwarded.Header.Set("Authorization", "Bearer "+coderToken)
		}
		forwarded.Body = io.NopCloser(bytes.NewReader(body))
		forwarded.ContentLength = int64(len(body))
		forwarded.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(body)), nil
		}
		return http.DefaultTransport.RoundTrip(forwarded)
	}), nil
}

func (r *providerRoutingGatewayRecorder) streamingRequests() []providerRoutingGatewayRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	requests := make([]providerRoutingGatewayRequest, 0, len(r.seen))
	for _, request := range r.seen {
		if request.stream {
			requests = append(requests, request)
		}
	}
	return requests
}

type unexpectedProviderRoutingGatewayFactory struct {
	calls atomic.Int32
}

func (f *unexpectedProviderRoutingGatewayFactory) TransportFor(string, aibridge.Source) (http.RoundTripper, error) {
	f.calls.Add(1)
	return roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusTeapot,
			Header:     http.Header{"Content-Type": []string{"text/plain"}},
			Body:       io.NopCloser(strings.NewReader("unexpected AI Gateway route")),
		}, nil
	}), nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func providerRoutingFactoryPointer(factory aibridge.TransportFactory) *atomic.Pointer[aibridge.TransportFactory] {
	var ptr atomic.Pointer[aibridge.TransportFactory]
	ptr.Store(&factory)
	return &ptr
}

func readRoundTripBody(req *http.Request) ([]byte, error) {
	if req.Body == nil {
		return nil, nil
	}
	body, err := io.ReadAll(req.Body)
	_ = req.Body.Close()
	if err != nil {
		return nil, err
	}
	return body, nil
}

func rewriteProviderRoutingGatewayURL(target *url.URL, original *url.URL) (*url.URL, error) {
	path, ok := strings.CutPrefix(original.Path, "/v1/")
	if !ok {
		return nil, xerrors.Errorf("expected gateway path to start with /v1/: %s", original.Path)
	}
	forwarded := *original
	forwarded.Scheme = target.Scheme
	forwarded.Host = target.Host
	forwarded.Path = "/" + path
	return &forwarded, nil
}

func newProviderRoutingTestServer(
	t *testing.T,
	db database.Store,
	ps dbpubsub.Pubsub,
	allowBYOK bool,
	aiGatewayRoutingEnabled bool,
	factoryPtr *atomic.Pointer[aibridge.TransportFactory],
) *chatd.Server {
	t.Helper()
	return newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.PendingChatAcquireInterval = testutil.WaitLong
		cfg.AllowBYOK = allowBYOK
		cfg.AllowBYOKSet = true
		cfg.AIGatewayRoutingEnabled = aiGatewayRoutingEnabled
		cfg.AIBridgeTransportFactory = factoryPtr
	})
}

func seedProviderRoutingChatDependencies(
	t *testing.T,
	db database.Store,
	baseURL string,
	centralKey string,
) (database.User, database.Organization, database.AIProvider, database.ChatModelConfig) {
	t.Helper()

	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         user.ID,
		OrganizationID: org.ID,
	})
	provider := dbgen.AIProviderWithOptionalKey(t, db, database.AIProvider{
		Type:    database.AiProviderTypeOpenai,
		Name:    "provider-routing-" + testutil.GetRandomNameHyphenated(t),
		BaseUrl: baseURL,
	}, centralKey)
	model := insertProviderRoutingModelConfig(t, db, user.ID, provider)
	return user, org, provider, model
}

func insertProviderRoutingModelConfig(
	t *testing.T,
	db database.Store,
	userID uuid.UUID,
	provider database.AIProvider,
) database.ChatModelConfig {
	t.Helper()
	store := false
	webSearchEnabled := false
	options, err := json.Marshal(codersdk.ChatModelCallConfig{
		ProviderOptions: &codersdk.ChatModelProviderOptions{
			OpenAI: &codersdk.ChatModelOpenAIProviderOptions{
				Store:            &store,
				WebSearchEnabled: &webSearchEnabled,
			},
		},
	})
	require.NoError(t, err)
	return dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
		Provider:     string(provider.Type),
		Model:        "gpt-4o",
		DisplayName:  "Provider Routing",
		CreatedBy:    uuid.NullUUID{UUID: userID, Valid: true},
		UpdatedBy:    uuid.NullUUID{UUID: userID, Valid: true},
		Options:      options,
		AIProviderID: uuid.NullUUID{UUID: provider.ID, Valid: true},
	})
}

func requireProviderRoutingChatSucceeded(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	chatID uuid.UUID,
	wantText string,
) {
	t.Helper()
	requireResponsesChatWaiting(ctx, t, db, chatID)

	messages, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
		ChatID:  chatID,
		AfterID: 0,
	})
	require.NoError(t, err)
	var assistantText string
	for _, message := range messages {
		if message.Role != database.ChatMessageRoleAssistant {
			continue
		}
		parts, err := chatprompt.ParseContent(message)
		require.NoError(t, err)
		for _, part := range parts {
			if part.Type == codersdk.ChatMessagePartTypeText {
				assistantText += part.Text
			}
		}
	}
	require.Contains(t, assistantText, wantText)
}

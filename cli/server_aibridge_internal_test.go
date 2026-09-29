package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/aibridge/headers"
	"github.com/coder/coder/v2/aibridge/recorder"
	"github.com/coder/coder/v2/coderd/aibridged/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/serpent"
)

func TestBuildProviderFromProtoSetsAPIDumpDir(t *testing.T) {
	t.Parallel()

	const dumpDir = "/tmp/coder-aibridge-dumps"

	tests := []struct {
		name         string
		provider     *proto.AIProvider
		expectedType string
	}{
		{
			name: "OpenAI",
			provider: &proto.AIProvider{
				Enabled: true,
				Type:    string(database.AIProviderTypeOpenai),
				Name:    "openai",
				BaseUrl: "https://api.openai.com/",
			},
			expectedType: aibridge.ProviderOpenAI,
		},
		{
			name: "Anthropic",
			provider: &proto.AIProvider{
				Enabled: true,
				Type:    string(database.AIProviderTypeAnthropic),
				Name:    "anthropic",
				BaseUrl: "https://api.anthropic.com/",
			},
			expectedType: aibridge.ProviderAnthropic,
		},
		{
			name: "Copilot",
			provider: &proto.AIProvider{
				Enabled: true,
				Type:    string(database.AIProviderTypeCopilot),
				Name:    "copilot",
				BaseUrl: "https://api.githubcopilot.com/",
			},
			expectedType: aibridge.ProviderCopilot,
		},
		{
			name: "Azure",
			provider: &proto.AIProvider{
				Enabled: true,
				Type:    string(database.AIProviderTypeAzure),
				Name:    "azure",
				BaseUrl: "https://example.openai.azure.com/",
			},
			expectedType: aibridge.ProviderOpenAI,
		},
		{
			name: "Google",
			provider: &proto.AIProvider{
				Enabled: true,
				Type:    string(database.AIProviderTypeGoogle),
				Name:    "google",
				BaseUrl: "https://generativelanguage.googleapis.com/v1beta/openai/",
			},
			expectedType: aibridge.ProviderOpenAI,
		},
		{
			name: "OpenAICompat",
			provider: &proto.AIProvider{
				Enabled: true,
				Type:    string(database.AIProviderTypeOpenaiCompat),
				Name:    "openai-compat",
				BaseUrl: "https://compat.example.com/v1/",
			},
			expectedType: aibridge.ProviderOpenAI,
		},
		{
			name: "OpenRouter",
			provider: &proto.AIProvider{
				Enabled: true,
				Type:    string(database.AIProviderTypeOpenrouter),
				Name:    "openrouter",
				BaseUrl: "https://openrouter.ai/api/v1/",
			},
			expectedType: aibridge.ProviderOpenAI,
		},
		{
			name: "Vercel",
			provider: &proto.AIProvider{
				Enabled: true,
				Type:    string(database.AIProviderTypeVercel),
				Name:    "vercel",
				BaseUrl: "https://api.v0.dev/v1/",
			},
			expectedType: aibridge.ProviderOpenAI,
		},
		{
			name: "Bedrock",
			provider: &proto.AIProvider{
				Enabled: true,
				Type:    string(database.AIProviderTypeBedrock),
				Name:    "bedrock",
				BaseUrl: "https://bedrock-runtime.us-east-1.amazonaws.com/",
				Bedrock: &proto.AIProviderKindBedrock{
					Region:          "us-east-1",
					AccessKey:       "AKID",
					AccessKeySecret: "secret",
					Model:           "anthropic.claude-3-5-sonnet-20241022-v2:0",
					SmallFastModel:  "anthropic.claude-3-5-haiku-20241022-v1:0",
				},
			},
			expectedType: aibridge.ProviderBedrock,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			provider, err := buildProvider(t.Context(), protoToProviderSpec(tt.provider), codersdk.AIBridgeConfig{
				AllowBYOK:  serpent.Bool(true),
				APIDumpDir: serpent.String(dumpDir),
			}, nil)
			require.NoError(t, err)
			assert.Equal(t, dumpDir, provider.APIDumpDir())
			assert.Equal(t, tt.expectedType, provider.Type())
		})
	}
}

func TestBuildProviderActorHeaders(t *testing.T) {
	t.Parallel()

	const (
		actorID          = "authenticated-user"
		actorUsername    = "authenticated-username"
		clientID         = "client-user"
		clientName       = "client-username"
		customIDHeader   = "X-Downstream-User-ID"
		customNameHeader = "X-Downstream-Username"
	)

	tests := []struct {
		name                string
		providerType        database.AIProviderType
		sendActorHeaders    bool
		actorHeaderID       string
		actorHeaderName     string
		wantCustomHeaders   bool
		wantStandardHeaders bool
	}{
		{
			name:                "disabled with configured destinations",
			providerType:        database.AIProviderTypeOpenai,
			actorHeaderID:       customIDHeader,
			actorHeaderName:     customNameHeader,
			wantStandardHeaders: true,
		},
		{
			name:             "enabled with empty destinations",
			providerType:     database.AIProviderTypeOpenai,
			sendActorHeaders: true,
		},
		{
			name:              "enabled with selected destinations",
			providerType:      database.AIProviderTypeOpenai,
			sendActorHeaders:  true,
			actorHeaderID:     customIDHeader,
			actorHeaderName:   customNameHeader,
			wantCustomHeaders: true,
		},
		{
			name:                "Copilot ignores configured destinations",
			providerType:        database.AIProviderTypeCopilot,
			sendActorHeaders:    true,
			actorHeaderID:       customIDHeader,
			actorHeaderName:     customNameHeader,
			wantStandardHeaders: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			upstreamHeaders := make(chan http.Header, 1)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				upstreamHeaders <- r.Header.Clone()
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":"chatcmpl-test","object":"chat.completion","choices":[]}`))
			}))
			t.Cleanup(upstream.Close)

			provider, err := buildProvider(t.Context(), aiProviderSpec{
				Type:    tt.providerType,
				Name:    string(tt.providerType),
				Enabled: true,
				BaseURL: upstream.URL,
				Keys:    []string{"upstream-key"},
			}, codersdk.AIBridgeConfig{
				SendActorHeaders:    serpent.Bool(tt.sendActorHeaders),
				ActorHeaderID:       serpent.String(tt.actorHeaderID),
				ActorHeaderUsername: serpent.String(tt.actorHeaderName),
			}, nil)
			require.NoError(t, err)

			request := httptest.NewRequest(http.MethodPost, provider.RoutePrefix()+"/chat/completions", bytes.NewBufferString(`{"model":"gpt-4","messages":[],"stream":false}`))
			request = request.WithContext(aibridge.AsActor(request.Context(), actorID, aibridge.Metadata{"Username": actorUsername}))
			request.Header.Set("Authorization", "Bearer client-key")
			request.Header.Set(headers.ActorIDHeader(), clientID)
			request.Header.Set(headers.ActorMetadataHeader("Username"), clientName)

			interceptor, err := provider.CreateInterceptor(httptest.NewRecorder(), request, noop.NewTracerProvider().Tracer("test"))
			require.NoError(t, err)
			interceptor.Setup(slog.Make(), recorder.NewLogRecorder(slog.Make(), nil), nil)

			processRequest := httptest.NewRequest(http.MethodPost, provider.RoutePrefix()+"/chat/completions", bytes.NewBufferString(`{"model":"gpt-4","messages":[],"stream":false}`)).WithContext(request.Context())
			response := httptest.NewRecorder()
			require.NoError(t, interceptor.ProcessRequest(response, processRequest))

			receivedHeaders := testutil.TryReceive(testutil.Context(t, testutil.WaitShort), t, upstreamHeaders)
			if tt.wantCustomHeaders {
				assert.Equal(t, actorID, receivedHeaders.Get(customIDHeader))
				assert.Equal(t, actorUsername, receivedHeaders.Get(customNameHeader))
			} else {
				assert.NotContains(t, receivedHeaders, http.CanonicalHeaderKey(customIDHeader))
				assert.NotContains(t, receivedHeaders, customNameHeader)
			}
			if tt.wantStandardHeaders {
				assert.Equal(t, clientID, receivedHeaders.Get(headers.ActorIDHeader()))
				assert.Equal(t, clientName, receivedHeaders.Get(headers.ActorMetadataHeader("Username")))
			} else {
				assert.NotContains(t, receivedHeaders, http.CanonicalHeaderKey(headers.ActorIDHeader()))
				assert.NotContains(t, receivedHeaders, http.CanonicalHeaderKey(headers.ActorMetadataHeader("Username")))
			}
		})
	}
}

func TestBuildProviderFromProtoBedrockWithoutSettings(t *testing.T) {
	t.Parallel()

	_, err := buildProvider(t.Context(), protoToProviderSpec(&proto.AIProvider{
		Enabled: true,
		Type:    string(database.AIProviderTypeBedrock),
		Name:    "bedrock-no-settings",
		BaseUrl: "https://bedrock-runtime.us-east-1.amazonaws.com/",
	}), codersdk.AIBridgeConfig{
		AllowBYOK: serpent.Bool(true),
	}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bedrock provider has no bedrock credentials configured")
}

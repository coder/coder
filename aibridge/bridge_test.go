package aibridge_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/internal/testutil"
	"github.com/coder/coder/v2/aibridge/provider"
)

var bridgeTestTracer = otel.Tracer("bridge_test")

func TestValidateProviders(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)

	tests := []struct {
		name      string
		providers []provider.Provider
		expectErr string
	}{
		{
			name: "all_supported_providers",
			providers: []provider.Provider{
				aibridge.NewOpenAIProvider(config.OpenAI{Name: "openai", BaseURL: "https://api.openai.com/v1/"}),
				aibridge.NewAnthropicProvider(config.Anthropic{Name: "anthropic", BaseURL: "https://api.anthropic.com/"}, nil),
				aibridge.NewCopilotProvider(config.Copilot{Name: "copilot", BaseURL: "https://api.individual.githubcopilot.com"}),
				aibridge.NewCopilotProvider(config.Copilot{Name: "copilot-business", BaseURL: "https://api.business.githubcopilot.com"}),
				aibridge.NewCopilotProvider(config.Copilot{Name: "copilot-enterprise", BaseURL: "https://api.enterprise.githubcopilot.com"}),
			},
		},
		{
			name: "default_names_and_base_urls",
			providers: []provider.Provider{
				aibridge.NewOpenAIProvider(config.OpenAI{}),
				aibridge.NewAnthropicProvider(config.Anthropic{}, nil),
				aibridge.NewCopilotProvider(config.Copilot{}),
			},
		},
		{
			name: "multiple_copilot_instances",
			providers: []provider.Provider{
				aibridge.NewCopilotProvider(config.Copilot{}),
				aibridge.NewCopilotProvider(config.Copilot{Name: "copilot-business", BaseURL: "https://api.business.githubcopilot.com"}),
				aibridge.NewCopilotProvider(config.Copilot{Name: "copilot-enterprise", BaseURL: "https://api.enterprise.githubcopilot.com"}),
			},
		},
		{
			name: "name_with_slashes",
			providers: []provider.Provider{
				aibridge.NewCopilotProvider(config.Copilot{Name: "copilot/business", BaseURL: "https://api.business.githubcopilot.com"}),
			},
			expectErr: "invalid provider name",
		},
		{
			name: "name_with_spaces",
			providers: []provider.Provider{
				aibridge.NewCopilotProvider(config.Copilot{Name: "copilot business", BaseURL: "https://api.business.githubcopilot.com"}),
			},
			expectErr: "invalid provider name",
		},
		{
			name: "name_with_uppercase",
			providers: []provider.Provider{
				aibridge.NewCopilotProvider(config.Copilot{Name: "Copilot", BaseURL: "https://api.business.githubcopilot.com"}),
			},
			expectErr: "invalid provider name",
		},
		{
			name: "unique_names",
			providers: []provider.Provider{
				aibridge.NewCopilotProvider(config.Copilot{Name: "copilot", BaseURL: "https://api.individual.githubcopilot.com"}),
				aibridge.NewCopilotProvider(config.Copilot{Name: "copilot-business", BaseURL: "https://api.business.githubcopilot.com"}),
			},
		},
		{
			name: "duplicate_base_url_different_names",
			providers: []provider.Provider{
				aibridge.NewCopilotProvider(config.Copilot{Name: "copilot", BaseURL: "https://api.individual.githubcopilot.com"}),
				aibridge.NewCopilotProvider(config.Copilot{Name: "copilot-business", BaseURL: "https://api.individual.githubcopilot.com"}),
			},
		},
		{
			name: "duplicate_name",
			providers: []provider.Provider{
				aibridge.NewCopilotProvider(config.Copilot{Name: "copilot", BaseURL: "https://api.individual.githubcopilot.com"}),
				aibridge.NewCopilotProvider(config.Copilot{Name: "copilot", BaseURL: "https://api.business.githubcopilot.com"}),
			},
			expectErr: "duplicate provider name",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := aibridge.NewRequestBridge(t.Context(), tc.providers, nil, nil, logger, nil, bridgeTestTracer)
			if tc.expectErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestPassthroughRoutesForProviders(t *testing.T) {
	t.Parallel()

	upstreamRespBody := "upstream response"
	tests := []struct {
		name        string
		baseURLPath string
		requestPath string
		provider    func(string) provider.Provider
		expectPath  string
	}{
		{
			name:        "openAI_no_base_path",
			requestPath: "/openai/v1/conversations",
			provider: func(baseURL string) provider.Provider {
				return aibridge.NewOpenAIProvider(config.OpenAI{BaseURL: baseURL})
			},
			expectPath: "/conversations",
		},
		{
			name:        "openAI_with_base_path",
			baseURLPath: "/v1",
			requestPath: "/openai/v1/conversations",
			provider: func(baseURL string) provider.Provider {
				return aibridge.NewOpenAIProvider(config.OpenAI{BaseURL: baseURL})
			},
			expectPath: "/v1/conversations",
		},
		{
			name:        "anthropic_no_base_path",
			requestPath: "/anthropic/v1/models",
			provider: func(baseURL string) provider.Provider {
				return aibridge.NewAnthropicProvider(config.Anthropic{BaseURL: baseURL}, nil)
			},
			expectPath: "/v1/models",
		},
		{
			name:        "anthropic_with_base_path",
			baseURLPath: "/v1",
			requestPath: "/anthropic/v1/models",
			provider: func(baseURL string) provider.Provider {
				return aibridge.NewAnthropicProvider(config.Anthropic{BaseURL: baseURL}, nil)
			},
			expectPath: "/v1/v1/models",
		},
		{
			name:        "copilot_no_base_path",
			requestPath: "/copilot/models",
			provider: func(baseURL string) provider.Provider {
				return aibridge.NewCopilotProvider(config.Copilot{BaseURL: baseURL})
			},
			expectPath: "/models",
		},
		{
			name:        "copilot_with_base_path",
			baseURLPath: "/v1",
			requestPath: "/copilot/models",
			provider: func(baseURL string) provider.Provider {
				return aibridge.NewCopilotProvider(config.Copilot{BaseURL: baseURL})
			},
			expectPath: "/v1/models",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			logger := slogtest.Make(t, nil)

			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, tc.expectPath, r.URL.Path)
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(upstreamRespBody))
			}))
			t.Cleanup(upstream.Close)

			rec := testutil.MockRecorder{}
			prov := tc.provider(upstream.URL + tc.baseURLPath)
			bridge, err := aibridge.NewRequestBridge(t.Context(), []provider.Provider{prov}, &rec, nil, logger, nil, bridgeTestTracer)
			require.NoError(t, err)

			req := httptest.NewRequest("", tc.requestPath, nil)
			resp := httptest.NewRecorder()
			bridge.ServeHTTP(resp, req)

			assert.Equal(t, http.StatusOK, resp.Code)
			assert.Contains(t, resp.Body.String(), upstreamRespBody)
		})
	}
}

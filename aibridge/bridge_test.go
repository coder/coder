package aibridge_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/aibridge/aibridgetest"
	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/internal/testutil"
	"github.com/coder/coder/v2/aibridge/provider"
	codertestutil "github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

var bridgeTestTracer = otel.Tracer("bridge_test")

// TestRequestBridgeShutdownAdmissionRace deterministically interleaves request
// admission with Shutdown using the `serve_admission` quartz trap.
func TestRequestBridgeShutdownAdmissionRace(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	release := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-release
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(upstream.Close)

	clk := quartz.NewMock(t)
	trap := clk.Trap().Now("serve_admission")
	defer trap.Close()

	rec := testutil.MockRecorder{}
	prov := aibridge.NewOpenAIProvider(config.OpenAI{BaseURL: upstream.URL})
	bridge, err := aibridge.NewRequestBridge(ctx, []provider.Provider{prov}, &rec, nil, logger, nil, bridgeTestTracer, aibridge.WithClock(clk))
	require.NoError(t, err)

	serve := func(done chan struct{}) {
		defer close(done)
		bridge.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/openai/v1/conversations", nil))
	}

	// Request 1: admit past the trap; it then blocks in the upstream, holding
	// the inflight WaitGroup (counter == 1).
	req1 := make(chan struct{})
	go serve(req1)
	trap.MustWait(ctx).MustRelease(ctx)

	// Request 2: park at the trap, having passed the closed check but before
	// inflightWG.Add.
	req2 := make(chan struct{})
	go serve(req2)
	call2 := trap.MustWait(ctx)

	// Shutdown closes and waits on the inflight WaitGroup (held by request 1).
	shutdown := make(chan struct{})
	go func() {
		defer close(shutdown)
		_ = bridge.Shutdown(context.Background())
	}()

	// Releasing request 2 races its inflightWG.Add against Shutdown's Wait.
	call2.MustRelease(ctx)

	// Let both requests complete so Shutdown can finish.
	close(release)
	_ = codertestutil.TryReceive(ctx, t, req1)
	_ = codertestutil.TryReceive(ctx, t, req2)
	_ = codertestutil.TryReceive(ctx, t, shutdown)
}

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
				aibridgetest.NewAnthropicProvider(t, config.Anthropic{Name: "anthropic", BaseURL: "https://api.anthropic.com/"}, nil),
				aibridge.NewCopilotProvider(config.Copilot{Name: "copilot", BaseURL: "https://api.individual.githubcopilot.com"}),
				aibridge.NewCopilotProvider(config.Copilot{Name: "copilot-business", BaseURL: "https://api.business.githubcopilot.com"}),
				aibridge.NewCopilotProvider(config.Copilot{Name: "copilot-enterprise", BaseURL: "https://api.enterprise.githubcopilot.com"}),
			},
		},
		{
			name: "default_names_and_base_urls",
			providers: []provider.Provider{
				aibridge.NewOpenAIProvider(config.OpenAI{}),
				aibridgetest.NewAnthropicProvider(t, config.Anthropic{}, nil),
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
		provider    func(*testing.T, string) provider.Provider
		expectPath  string
	}{
		{
			name:        "openAI_no_base_path",
			requestPath: "/openai/v1/conversations",
			provider: func(_ *testing.T, baseURL string) provider.Provider {
				return aibridge.NewOpenAIProvider(config.OpenAI{BaseURL: baseURL})
			},
			expectPath: "/conversations",
		},
		{
			name:        "openAI_with_base_path",
			baseURLPath: "/v1",
			requestPath: "/openai/v1/conversations",
			provider: func(_ *testing.T, baseURL string) provider.Provider {
				return aibridge.NewOpenAIProvider(config.OpenAI{BaseURL: baseURL})
			},
			expectPath: "/v1/conversations",
		},
		{
			name:        "anthropic_no_base_path",
			requestPath: "/anthropic/v1/models",
			provider: func(t *testing.T, baseURL string) provider.Provider {
				return aibridgetest.NewAnthropicProvider(t, config.Anthropic{BaseURL: baseURL}, nil)
			},
			expectPath: "/v1/models",
		},
		{
			name:        "anthropic_with_base_path",
			baseURLPath: "/v1",
			requestPath: "/anthropic/v1/models",
			provider: func(t *testing.T, baseURL string) provider.Provider {
				return aibridgetest.NewAnthropicProvider(t, config.Anthropic{BaseURL: baseURL}, nil)
			},
			expectPath: "/v1/v1/models",
		},
		{
			name:        "copilot_no_base_path",
			requestPath: "/copilot/models",
			provider: func(_ *testing.T, baseURL string) provider.Provider {
				return aibridge.NewCopilotProvider(config.Copilot{BaseURL: baseURL})
			},
			expectPath: "/models",
		},
		{
			name:        "copilot_with_base_path",
			baseURLPath: "/v1",
			requestPath: "/copilot/models",
			provider: func(_ *testing.T, baseURL string) provider.Provider {
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
			prov := tc.provider(t, upstream.URL+tc.baseURLPath)
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

func TestRequestBodySizeLimit(t *testing.T) {
	t.Parallel()

	newOpenAI := func(_ *testing.T, baseURL string) provider.Provider {
		return aibridge.NewOpenAIProvider(config.OpenAI{Name: "openai", BaseURL: baseURL})
	}
	newAnthropic := func(t *testing.T, baseURL string) provider.Provider {
		return aibridgetest.NewAnthropicProvider(t, config.Anthropic{Name: "anthropic", BaseURL: baseURL}, nil)
	}
	newCopilot := func(_ *testing.T, baseURL string) provider.Provider {
		return aibridge.NewCopilotProvider(config.Copilot{Name: "copilot", BaseURL: baseURL})
	}

	// Each body is a well-formed, schema-valid request for its provider, with
	// an oversized message content that pushes it past the 32 MiB limit.
	filler := strings.Repeat("A", 32<<20)
	chatCompletionsBody := fmt.Appendf(nil, `{"model":"gpt-4","messages":[{"role":"user","content":"%s"}]}`, filler)
	responsesBody := fmt.Appendf(nil, `{"model":"gpt-4","input":"%s"}`, filler)
	messagesBody := fmt.Appendf(nil, `{"model":"claude-3-5-sonnet-latest","max_tokens":1024,"messages":[{"role":"user","content":"%s"}]}`, filler)

	tests := []struct {
		name     string
		provider func(*testing.T, string) provider.Provider
		path     string
		body     []byte
	}{
		{name: "openai_passthrough", provider: newOpenAI, path: "/openai/v1/models", body: chatCompletionsBody},
		{name: "openai_chat_completions", provider: newOpenAI, path: "/openai/v1/chat/completions", body: chatCompletionsBody},
		{name: "openai_responses", provider: newOpenAI, path: "/openai/v1/responses", body: responsesBody},
		{name: "anthropic_passthrough", provider: newAnthropic, path: "/anthropic/v1/models", body: messagesBody},
		{name: "anthropic_messages", provider: newAnthropic, path: "/anthropic/v1/messages", body: messagesBody},
		{name: "copilot_passthrough", provider: newCopilot, path: "/copilot/models", body: chatCompletionsBody},
		{name: "copilot_chat_completions", provider: newCopilot, path: "/copilot/chat/completions", body: chatCompletionsBody},
		{name: "copilot_responses", provider: newCopilot, path: "/copilot/responses", body: responsesBody},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			logger := slogtest.Make(t, nil)

			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.ReadAll(r.Body)
				w.WriteHeader(http.StatusOK)
			}))
			t.Cleanup(upstream.Close)

			prov := tc.provider(t, upstream.URL)
			bridge, err := aibridge.NewRequestBridge(
				t.Context(),
				[]provider.Provider{prov},
				nil, nil, logger, nil, bridgeTestTracer,
			)
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPost, tc.path, bytes.NewReader(tc.body))
			// Unknown Content-Length
			req.ContentLength = -1
			// Copilot's bridged route checks Authorization before reading the
			// body, so provide a token to reach the read path.
			req.Header.Set("Authorization", "Bearer test-key")
			resp := httptest.NewRecorder()
			bridge.ServeHTTP(resp, req)

			assert.Equal(t, http.StatusRequestEntityTooLarge, resp.Code)
			assert.Contains(t, resp.Body.String(), "Request body too large")
		})
	}
}

// TestDisabledProviderHandler asserts that requests to a disabled
// provider return a 503 with an ErrorCodeProviderDisabled body and
// that a sibling enabled provider keeps routing normally.
func TestDisabledProviderHandler(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("upstream-reached"))
	}))
	t.Cleanup(upstream.Close)

	enabled := aibridge.NewOpenAIProvider(config.OpenAI{Name: "enabled-openai", BaseURL: upstream.URL})
	disabled := aibridge.NewDisabledProviderStub("disabled-openai", "openai")
	bridge, err := aibridge.NewRequestBridge(
		t.Context(),
		[]provider.Provider{enabled, disabled},
		nil, nil, logger, nil, bridgeTestTracer,
	)
	require.NoError(t, err)

	for _, tc := range []struct {
		name string
		path string
	}{
		{name: "Bridged", path: "/disabled-openai/v1/chat/completions"},
		{name: "Passthrough", path: "/disabled-openai/v1/models"},
		{name: "Unknown", path: "/disabled-openai/anything/else"},
	} {
		t.Run("DisabledProviderReturnsSentinel/"+tc.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodPost, tc.path, nil)
			resp := httptest.NewRecorder()
			bridge.ServeHTTP(resp, req)

			assert.Equal(t, http.StatusServiceUnavailable, resp.Code)
			assert.Contains(t, resp.Body.String(), aibridge.ErrorCodeProviderDisabled)
			assert.Contains(t, resp.Body.String(), "disabled-openai")
		})
	}

	t.Run("EnabledProviderUnaffected", func(t *testing.T) {
		t.Parallel()

		req := httptest.NewRequest(http.MethodGet, "/enabled-openai/v1/models", nil)
		resp := httptest.NewRecorder()
		bridge.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusOK, resp.Code)
		assert.Equal(t, "upstream-reached", resp.Body.String())
	})
}

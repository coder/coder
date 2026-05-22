package chatcompletions

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/intercept"
	"github.com/coder/coder/v2/aibridge/internal/testutil"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/mcp"
	"github.com/coder/coder/v2/aibridge/utils"
	"github.com/coder/quartz"
)

// Test that when the upstream provider returns an error before streaming starts,
// the error status code and body are correctly relayed to the client.
func TestStreamingInterception_RelaysUpstreamErrorToClient(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		statusCode     int
		responseBody   string
		expectedErrStr string
		expectedBody   string
	}{
		{
			name:           "bad request error",
			statusCode:     http.StatusBadRequest,
			responseBody:   `{"error":{"message":"Invalid request","type":"invalid_request_error","code":"invalid_request"}}`,
			expectedErrStr: "Invalid request",
			expectedBody:   "invalid_request",
		},
		{
			name:           "rate limit error",
			statusCode:     http.StatusTooManyRequests,
			responseBody:   `{"error":{"message":"Rate limit exceeded","type":"rate_limit_error","code":"rate_limit_exceeded"}}`,
			expectedErrStr: "Rate limit exceeded",
			expectedBody:   "rate_limit",
		},
		{
			name:           "internal server error",
			statusCode:     http.StatusInternalServerError,
			responseBody:   `{"error":{"message":"Internal server error","type":"server_error","code":"internal_error"}}`,
			expectedErrStr: "Internal server error",
			expectedBody:   "server_error",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup a mock server that returns an error immediately (before any streaming)
			mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("x-should-retry", "false")
				w.WriteHeader(tc.statusCode)
				_, _ = w.Write([]byte(tc.responseBody))
			}))
			t.Cleanup(mockServer.Close)

			// Create interceptor with mock server URL
			cfg := config.OpenAI{
				BaseURL: mockServer.URL,
				Key:     "test-key",
			}

			req := &ChatCompletionNewParamsWrapper{
				ChatCompletionNewParams: openai.ChatCompletionNewParams{
					Model: "gpt-4",
					Messages: []openai.ChatCompletionMessageParamUnion{
						openai.UserMessage("hello"),
					},
				},
				Stream: true,
			}

			// Create test request
			w := httptest.NewRecorder()
			httpReq := httptest.NewRequest(http.MethodPost, "/chat/completions", nil)

			tracer := otel.Tracer("test")
			interceptor := NewStreamingInterceptor(uuid.New(), req, config.ProviderOpenAI, cfg, httpReq.Header, "Authorization", tracer, intercept.CredentialInfo{})

			logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: false}).Leveled(slog.LevelDebug)
			interceptor.Setup(logger, &testutil.MockRecorder{}, nil)

			// Process the request
			err := interceptor.ProcessRequest(w, httpReq)

			// Verify error was returned
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.expectedErrStr)

			// Verify status code was written to response
			assert.Equal(t, tc.statusCode, w.Code, "expected status code to be relayed to client")

			// Verify error body contains expected error info
			body := w.Body.String()
			assert.Contains(t, body, tc.expectedBody, "expected error type in response body")
		})
	}
}

// OpenAI-shaped SSE body for a successful streaming response.
const streamingSuccessBody = `data: {"id":"chatcmpl-01","object":"chat.completion.chunk","created":1234567890,"model":"gpt-4","choices":[{"index":0,"delta":{"role":"assistant","content":"Hello"},"finish_reason":null}]}

data: {"id":"chatcmpl-01","object":"chat.completion.chunk","created":1234567890,"model":"gpt-4","choices":[{"index":0,"delta":{"content":"!"},"finish_reason":null}]}

data: {"id":"chatcmpl-01","object":"chat.completion.chunk","created":1234567890,"model":"gpt-4","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}

data: [DONE]

`

func TestStreamingInterception_KeyFailover(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// Centralized pool keys. Empty when byokKey is set.
		keys []string
		// BYOK key. Empty when keys is set.
		byokKey string
		// Scripted upstream responses keyed by bearer token.
		responses            map[string]upstreamResponse
		expectedRequestCount int32
		expectedStatusCode   int
		expectedRetryAfter   string
		// Expected key states after the request, by index in keys.
		expectedKeyStates []keypool.KeyState
	}{
		{
			// Given: 1 valid key returning a successful stream.
			// Then: 1 request, 200 response, key remains valid.
			name: "single_valid_key",
			keys: []string{"k0"},
			responses: map[string]upstreamResponse{
				"k0": {
					statusCode: http.StatusOK,
					headers:    map[string]string{"Content-Type": "text/event-stream"},
					body:       streamingSuccessBody,
				},
			},
			expectedRequestCount: 1,
			expectedStatusCode:   http.StatusOK,
			expectedKeyStates:    []keypool.KeyState{keypool.KeyStateValid},
		},
		{
			// Given: 2 keys; key-0 returns 429 pre-stream, key-1
			// streams successfully.
			// Then: 2 requests, 200 response, key-0 temporary, key-1 valid.
			name: "failover_after_429",
			keys: []string{"k0", "k1"},
			responses: map[string]upstreamResponse{
				"k0": {
					statusCode: http.StatusTooManyRequests,
					headers:    map[string]string{"Retry-After": "5"},
					body:       rateLimitBody,
				},
				"k1": {
					statusCode: http.StatusOK,
					headers:    map[string]string{"Content-Type": "text/event-stream"},
					body:       streamingSuccessBody,
				},
			},
			expectedRequestCount: 2,
			expectedStatusCode:   http.StatusOK,
			expectedKeyStates: []keypool.KeyState{
				keypool.KeyStateTemporary,
				keypool.KeyStateValid,
			},
		},
		{
			// Given: 2 keys; key-0 returns 401 pre-stream, key-1
			// streams successfully.
			// Then: 2 requests, 200 response, key-0 permanent, key-1 valid.
			name: "failover_after_401",
			keys: []string{"k0", "k1"},
			responses: map[string]upstreamResponse{
				"k0": {statusCode: http.StatusUnauthorized, body: authErrorBody},
				"k1": {
					statusCode: http.StatusOK,
					headers:    map[string]string{"Content-Type": "text/event-stream"},
					body:       streamingSuccessBody,
				},
			},
			expectedRequestCount: 2,
			expectedStatusCode:   http.StatusOK,
			expectedKeyStates: []keypool.KeyState{
				keypool.KeyStatePermanent,
				keypool.KeyStateValid,
			},
		},
		{
			// Given: 2 keys; key-0 returns 403 pre-stream, key-1 streams.
			// Then: 2 requests, 200 response, key-0 permanent, key-1 valid.
			name: "failover_after_403",
			keys: []string{"k0", "k1"},
			responses: map[string]upstreamResponse{
				"k0": {statusCode: http.StatusForbidden, body: authErrorBody},
				"k1": {
					statusCode: http.StatusOK,
					headers:    map[string]string{"Content-Type": "text/event-stream"},
					body:       streamingSuccessBody,
				},
			},
			expectedRequestCount: 2,
			expectedStatusCode:   http.StatusOK,
			expectedKeyStates: []keypool.KeyState{
				keypool.KeyStatePermanent,
				keypool.KeyStateValid,
			},
		},
		{
			// Given: 3 keys; all return 429 pre-stream with
			// cooldowns 5s, 3s, 10s.
			// Then: 3 requests, 429 response with smallest
			// Retry-After, all keys temporary.
			name: "all_keys_rate_limited",
			keys: []string{"k0", "k1", "k2"},
			responses: map[string]upstreamResponse{
				"k0": {
					statusCode: http.StatusTooManyRequests,
					headers:    map[string]string{"Retry-After": "5"},
					body:       rateLimitBody,
				},
				"k1": {
					statusCode: http.StatusTooManyRequests,
					headers:    map[string]string{"Retry-After": "3"},
					body:       rateLimitBody,
				},
				"k2": {
					statusCode: http.StatusTooManyRequests,
					headers:    map[string]string{"Retry-After": "10"},
					body:       rateLimitBody,
				},
			},
			expectedRequestCount: 3,
			expectedStatusCode:   http.StatusTooManyRequests,
			expectedRetryAfter:   "3",
			expectedKeyStates: []keypool.KeyState{
				keypool.KeyStateTemporary,
				keypool.KeyStateTemporary,
				keypool.KeyStateTemporary,
			},
		},
		{
			// Given: 2 keys; both return 401 pre-stream.
			// Then: 2 requests, 502 api_error response, both keys permanent.
			name: "all_keys_unauthorized",
			keys: []string{"k0", "k1"},
			responses: map[string]upstreamResponse{
				"k0": {statusCode: http.StatusUnauthorized, body: authErrorBody},
				"k1": {statusCode: http.StatusUnauthorized, body: authErrorBody},
			},
			expectedRequestCount: 2,
			expectedStatusCode:   http.StatusBadGateway,
			expectedKeyStates: []keypool.KeyState{
				keypool.KeyStatePermanent,
				keypool.KeyStatePermanent,
			},
		},
		{
			// Given: 2 keys; key-0 returns 500 pre-stream.
			// Then: 1 request, 500 response, both keys remain valid.
			name: "server_error_no_failover",
			keys: []string{"k0", "k1"},
			responses: map[string]upstreamResponse{
				"k0": {statusCode: http.StatusInternalServerError, body: serverErrorBody},
			},
			expectedRequestCount: 1,
			expectedStatusCode:   http.StatusInternalServerError,
			expectedKeyStates: []keypool.KeyState{
				keypool.KeyStateValid,
				keypool.KeyStateValid,
			},
		},
		{
			// Given: BYOK with a single key returning 429.
			// Then: 1 request, 429 response, no failover, upstream
			// Retry-After propagated to the client.
			name:    "byok_no_failover",
			byokKey: "user-byok",
			responses: map[string]upstreamResponse{
				"user-byok": {
					statusCode: http.StatusTooManyRequests,
					headers: map[string]string{
						"Retry-After": "5",
						// BYOK doesn't set MaxRetries(0);
						// suppress SDK retries to test a
						// single attempt.
						"x-should-retry": "false",
					},
					body: rateLimitBody,
				},
			},
			expectedRequestCount: 1,
			expectedStatusCode:   http.StatusTooManyRequests,
			expectedRetryAfter:   "5",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Mock upstream: counts requests and returns
			// scripted responses keyed by bearer token. An
			// unmapped key falls through to 500 so misconfigured
			// cases surface via the status assertion.
			var requestCount atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestCount.Add(1)
				_, _ = io.Copy(io.Discard, r.Body)
				resp, ok := tc.responses[utils.ExtractBearerToken(r.Header.Get("Authorization"))]
				if !ok {
					resp = upstreamResponse{statusCode: http.StatusInternalServerError}
				}
				for hk, hv := range resp.headers {
					w.Header().Set(hk, hv)
				}
				w.WriteHeader(resp.statusCode)
				_, _ = w.Write([]byte(resp.body))
			}))
			t.Cleanup(upstream.Close)

			cfg := config.OpenAI{BaseURL: upstream.URL + "/"}
			var pool *keypool.Pool
			if len(tc.keys) > 0 {
				var err error
				pool, err = keypool.New(tc.keys, quartz.NewMock(t))
				require.NoError(t, err)
				cfg.KeyPool = pool
			} else if tc.byokKey != "" {
				cfg.Key = tc.byokKey
			}

			interceptor := NewStreamingInterceptor(
				uuid.New(),
				newRequestParams(true),
				config.ProviderOpenAI,
				cfg,
				http.Header{},
				"Authorization",
				otel.Tracer("streaming_test"),
				intercept.NewCredentialInfo(intercept.CredentialKindCentralized, ""),
			)
			interceptor.Setup(slog.Make(), &testutil.MockRecorder{}, nil)

			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			w := httptest.NewRecorder()
			err := interceptor.ProcessRequest(w, req)
			if tc.expectedStatusCode == http.StatusOK {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}

			assert.Equal(t, tc.expectedRequestCount, requestCount.Load(), "upstream request count")
			assert.Equal(t, tc.expectedStatusCode, w.Code, "response status code")
			assert.Equal(t, tc.expectedRetryAfter, w.Header().Get("Retry-After"), "Retry-After header")
			if pool != nil {
				assert.Equal(t, tc.expectedKeyStates, pool.PoolState(), "key states")
			}
		})
	}
}

// SSE bodies covering an agentic-continuation flow.
const (
	// First response: a tool_calls delta referencing the
	// injected "test_tool". Triggers the agentic continuation
	// loop.
	toolUseStreamBody = `data: {"id":"chatcmpl-01","object":"chat.completion.chunk","created":1234567890,"model":"gpt-4","choices":[{"index":0,"delta":{"role":"assistant","content":null,"tool_calls":[{"index":0,"id":"call_01","type":"function","function":{"name":"test_tool","arguments":""}}]},"finish_reason":null}]}

data: {"id":"chatcmpl-01","object":"chat.completion.chunk","created":1234567890,"model":"gpt-4","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{}"}}]},"finish_reason":null}]}

data: {"id":"chatcmpl-01","object":"chat.completion.chunk","created":1234567890,"model":"gpt-4","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}

data: [DONE]

`

	// Second response (after the tool result is sent back):
	// a plain text completion that ends the loop.
	textStreamBody = `data: {"id":"chatcmpl-02","object":"chat.completion.chunk","created":1234567890,"model":"gpt-4","choices":[{"index":0,"delta":{"role":"assistant","content":"done"},"finish_reason":null}]}

data: {"id":"chatcmpl-02","object":"chat.completion.chunk","created":1234567890,"model":"gpt-4","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":15,"completion_tokens":3,"total_tokens":18}}

data: [DONE]

`
)

// TestStreamingInterception_AgenticLoopFailover covers the
// scenarios that span an agentic-loop continuation: the initial
// client request and the subsequent tool-call continuation can
// each fail over independently. Each iteration gets its own
// walker.
func TestStreamingInterception_AgenticLoopFailover(t *testing.T) {
	t.Parallel()

	sseHeaders := map[string]string{"Content-Type": "text/event-stream"}

	tests := []struct {
		name string
		// Scripted upstream responses consumed in order of
		// upstream request.
		responses            []upstreamResponse
		expectedRequestCount int32
		expectedSeenKeys     []string
		// Substring expected in the response body. Either a
		// success marker (e.g. "done") or an error marker
		// (e.g. "rate_limit_error").
		expectedBodyContains string
		// True when the error must be relayed as an SSE event.
		expectErrorAsSSEEvent bool
		// True when ProcessRequest is expected to return an
		// error (e.g. all keys exhausted).
		expectedErr       bool
		expectedKeyStates []keypool.KeyState
	}{
		{
			// Given: 2 keys; both upstream calls succeed on key-0.
			// Then: 2 requests, success body, both keys remain valid.
			name: "happy_path",
			responses: []upstreamResponse{
				{statusCode: http.StatusOK, headers: sseHeaders, body: toolUseStreamBody},
				{statusCode: http.StatusOK, headers: sseHeaders, body: textStreamBody},
			},
			expectedRequestCount:  2,
			expectedSeenKeys:      []string{"k0", "k0"},
			expectedBodyContains:  "done",
			expectErrorAsSSEEvent: false,
			expectedKeyStates: []keypool.KeyState{
				keypool.KeyStateValid,
				keypool.KeyStateValid,
			},
		},
		{
			// Given: 2 keys; key-0 succeeds initially, then 429s
			// during the agentic continuation, key-1 succeeds.
			// Then: 3 requests, success body, key-0 temporary,
			// key-1 valid.
			name: "agentic_failover_to_k1",
			responses: []upstreamResponse{
				{statusCode: http.StatusOK, headers: sseHeaders, body: toolUseStreamBody},
				{
					statusCode: http.StatusTooManyRequests,
					headers:    map[string]string{"Retry-After": "5"},
					body:       rateLimitBody,
				},
				{statusCode: http.StatusOK, headers: sseHeaders, body: textStreamBody},
			},
			expectedRequestCount:  3,
			expectedSeenKeys:      []string{"k0", "k0", "k1"},
			expectedBodyContains:  "done",
			expectErrorAsSSEEvent: false,
			expectedKeyStates: []keypool.KeyState{
				keypool.KeyStateTemporary,
				keypool.KeyStateValid,
			},
		},
		{
			// Given: 2 keys; key-0 succeeds initially, then both
			// keys 429 during the agentic continuation.
			// Then: 3 requests, error injected as SSE event, both
			// keys temporary.
			name: "agentic_all_keys_fail",
			responses: []upstreamResponse{
				{statusCode: http.StatusOK, headers: sseHeaders, body: toolUseStreamBody},
				{
					statusCode: http.StatusTooManyRequests,
					headers:    map[string]string{"Retry-After": "5"},
					body:       rateLimitBody,
				},
				{
					statusCode: http.StatusTooManyRequests,
					headers:    map[string]string{"Retry-After": "3"},
					body:       rateLimitBody,
				},
			},
			expectedRequestCount:  3,
			expectedSeenKeys:      []string{"k0", "k0", "k1"},
			expectedBodyContains:  "all configured keys are rate-limited",
			expectErrorAsSSEEvent: true,
			expectedErr:           true,
			expectedKeyStates: []keypool.KeyState{
				keypool.KeyStateTemporary,
				keypool.KeyStateTemporary,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var requestCount atomic.Int32
			var seenKeysMu sync.Mutex
			var seenKeys []string

			// Mock upstream: returns scripted responses in order,
			// records each request's bearer token for assertions.
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				idx := int(requestCount.Add(1)) - 1
				seenKeysMu.Lock()
				seenKeys = append(seenKeys, utils.ExtractBearerToken(r.Header.Get("Authorization")))
				seenKeysMu.Unlock()
				_, _ = io.Copy(io.Discard, r.Body)

				if idx >= len(tc.responses) {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				resp := tc.responses[idx]
				for hk, hv := range resp.headers {
					w.Header().Set(hk, hv)
				}
				w.WriteHeader(resp.statusCode)
				_, _ = w.Write([]byte(resp.body))
			}))
			t.Cleanup(upstream.Close)

			pool, err := keypool.New([]string{"k0", "k1"}, quartz.NewMock(t))
			require.NoError(t, err)

			cfg := config.OpenAI{
				BaseURL: upstream.URL + "/",
				KeyPool: pool,
			}

			interceptor := NewStreamingInterceptor(
				uuid.New(),
				newRequestParams(true),
				config.ProviderOpenAI,
				cfg,
				http.Header{},
				"Authorization",
				otel.Tracer("streaming_test"),
				intercept.NewCredentialInfo(intercept.CredentialKindCentralized, ""),
			)

			// Mock proxy with a tool the upstream's tool_calls
			// chunks will reference. The stub caller returns a
			// fixed text result.
			proxy := &mockServerProxier{
				tools: []*mcp.Tool{
					{
						Client:     stubToolCaller{},
						ID:         "test_tool",
						Name:       "test_tool",
						ServerName: "coder",
						Logger:     slog.Make(),
					},
				},
			}
			interceptor.Setup(slog.Make(), &testutil.MockRecorder{}, proxy)

			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			w := httptest.NewRecorder()
			err = interceptor.ProcessRequest(w, req)
			if tc.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, tc.expectedRequestCount, requestCount.Load(), "upstream request count")
			body := w.Body.String()
			assert.Contains(t, body, tc.expectedBodyContains, "response body")
			if tc.expectErrorAsSSEEvent {
				// SSE was opened before the failure, so the body
				// must start with stream chunks, not a direct
				// HTTP error body.
				assert.True(t, strings.HasPrefix(body, "data: "), "body must start with SSE chunks")
			}

			seenKeysMu.Lock()
			defer seenKeysMu.Unlock()
			assert.Equal(t, tc.expectedSeenKeys, seenKeys, "seen keys")
			assert.Equal(t, tc.expectedKeyStates, pool.PoolState(), "key states")
		})
	}
}

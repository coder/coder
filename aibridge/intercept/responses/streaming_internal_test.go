package responses

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/intercept"
	"github.com/coder/coder/v2/aibridge/internal/testutil"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/mcp"
	"github.com/coder/coder/v2/aibridge/utils"
	"github.com/coder/quartz"
)

// Streaming request body for the OpenAI Responses API.
const streamingRequestBody = `{"input":"hi","model":"gpt-4o-mini","stream":true}`

// OpenAI Responses API SSE body for a successful streaming response.
const streamingSuccessBody = `event: response.created
data: {"type":"response.created","response":{"id":"resp_01","object":"response","status":"in_progress"},"sequence_number":0}

event: response.completed
data: {"type":"response.completed","response":{"id":"resp_01","object":"response","status":"completed","model":"gpt-4o-mini","output":[{"type":"message","id":"msg_01","role":"assistant","content":[{"type":"output_text","text":"Hello!"}]}],"usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15}},"sequence_number":1}

`

func TestStreamingResponsesInterceptor_KeyFailover(t *testing.T) {
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
		// Expected credential hint after ProcessRequest: last
		// attempted key for centralized, user key from initial request for BYOK.
		expectedCredentialHint string
	}{
		{
			// Given: 1 valid key returning a successful stream.
			// Then: 1 request, 200 response, key remains valid.
			name: "single_valid_key",
			keys: []string{"k0-long-key"},
			responses: map[string]upstreamResponse{
				"k0-long-key": {
					statusCode: http.StatusOK,
					headers:    map[string]string{"Content-Type": "text/event-stream"},
					body:       streamingSuccessBody,
				},
			},
			expectedRequestCount:   1,
			expectedStatusCode:     http.StatusOK,
			expectedKeyStates:      []keypool.KeyState{keypool.KeyStateValid},
			expectedCredentialHint: utils.MaskSecret("k0-long-key"),
		},
		{
			// Given: 2 keys; key-0 returns 429 pre-stream, key-1
			// streams successfully.
			// Then: 2 requests, 200 response, key-0 temporary, key-1 valid.
			name: "failover_after_429",
			keys: []string{"k0-long-key", "k1-long-key"},
			responses: map[string]upstreamResponse{
				"k0-long-key": {
					statusCode: http.StatusTooManyRequests,
					headers:    map[string]string{"Retry-After": "5"},
					body:       rateLimitBody,
				},
				"k1-long-key": {
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
			expectedCredentialHint: utils.MaskSecret("k1-long-key"),
		},
		{
			// Given: 2 keys; key-0 returns 401 pre-stream, key-1
			// streams successfully.
			// Then: 2 requests, 200 response, key-0 permanent, key-1 valid.
			name: "failover_after_401",
			keys: []string{"k0-long-key", "k1-long-key"},
			responses: map[string]upstreamResponse{
				"k0-long-key": {statusCode: http.StatusUnauthorized, body: authErrorBody},
				"k1-long-key": {
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
			expectedCredentialHint: utils.MaskSecret("k1-long-key"),
		},
		{
			// Given: 2 keys; key-0 returns 403 pre-stream, key-1 streams.
			// Then: 2 requests, 200 response, key-0 permanent, key-1 valid.
			name: "failover_after_403",
			keys: []string{"k0-long-key", "k1-long-key"},
			responses: map[string]upstreamResponse{
				"k0-long-key": {statusCode: http.StatusForbidden, body: authErrorBody},
				"k1-long-key": {
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
			expectedCredentialHint: utils.MaskSecret("k1-long-key"),
		},
		{
			// Given: 3 keys; all return 429 pre-stream with
			// cooldowns 5s, 3s, 10s.
			// Then: 3 requests, 429 response with smallest
			// Retry-After, all keys temporary.
			name: "all_keys_rate_limited",
			keys: []string{"k0-long-key", "k1-long-key", "k2-long-key"},
			responses: map[string]upstreamResponse{
				"k0-long-key": {
					statusCode: http.StatusTooManyRequests,
					headers:    map[string]string{"Retry-After": "5"},
					body:       rateLimitBody,
				},
				"k1-long-key": {
					statusCode: http.StatusTooManyRequests,
					headers:    map[string]string{"Retry-After": "3"},
					body:       rateLimitBody,
				},
				"k2-long-key": {
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
			expectedCredentialHint: utils.MaskSecret("k2-long-key"),
		},
		{
			// Given: 2 keys; both return 401 pre-stream.
			// Then: 2 requests, 502 api_error response, both keys permanent.
			name: "all_keys_unauthorized",
			keys: []string{"k0-long-key", "k1-long-key"},
			responses: map[string]upstreamResponse{
				"k0-long-key": {statusCode: http.StatusUnauthorized, body: authErrorBody},
				"k1-long-key": {statusCode: http.StatusUnauthorized, body: authErrorBody},
			},
			expectedRequestCount: 2,
			expectedStatusCode:   http.StatusBadGateway,
			expectedKeyStates: []keypool.KeyState{
				keypool.KeyStatePermanent,
				keypool.KeyStatePermanent,
			},
			expectedCredentialHint: utils.MaskSecret("k1-long-key"),
		},
		{
			// Given: 2 keys; key-0 returns 500 pre-stream.
			// Then: 1 request, 500 response, both keys remain valid.
			name: "server_error_no_failover",
			keys: []string{"k0-long-key", "k1-long-key"},
			responses: map[string]upstreamResponse{
				"k0-long-key": {statusCode: http.StatusInternalServerError, body: serverErrorBody},
			},
			expectedRequestCount: 1,
			expectedStatusCode:   http.StatusInternalServerError,
			expectedKeyStates: []keypool.KeyState{
				keypool.KeyStateValid,
				keypool.KeyStateValid,
			},
			expectedCredentialHint: utils.MaskSecret("k0-long-key"),
		},
		{
			// Given: BYOK with a single key returning 429.
			// Then: 1 request, 429 response, no failover.
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
			expectedRequestCount:   1,
			expectedStatusCode:     http.StatusTooManyRequests,
			expectedCredentialHint: utils.MaskSecret("user-byok"),
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
			credInfo := intercept.NewCredentialInfo(intercept.CredentialKindCentralized, "")
			var pool *keypool.Pool
			if len(tc.keys) > 0 {
				var err error
				pool, err = keypool.New(tc.keys, quartz.NewMock(t))
				require.NoError(t, err)
				cfg.KeyPool = pool
			} else if tc.byokKey != "" {
				cfg.Key = tc.byokKey
				credInfo = intercept.NewCredentialInfo(intercept.CredentialKindBYOK, tc.byokKey)
			}

			payload, err := NewRequestPayload([]byte(streamingRequestBody))
			require.NoError(t, err)

			interceptor := NewStreamingInterceptor(
				uuid.New(),
				payload,
				config.ProviderOpenAI,
				cfg,
				http.Header{},
				"Authorization",
				otel.Tracer("streaming_test"),
				credInfo,
			)
			interceptor.Setup(slog.Make(), &testutil.MockRecorder{}, nil)

			req := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			w := httptest.NewRecorder()
			err = interceptor.ProcessRequest(w, req)
			if tc.expectedStatusCode == http.StatusOK {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}

			assert.Equal(t, tc.expectedRequestCount, requestCount.Load(), "upstream request count")
			assert.Equal(t, tc.expectedStatusCode, w.Code, "response status code")
			assert.Equal(t, tc.expectedRetryAfter, w.Header().Get("Retry-After"), "Retry-After header")
			assert.Equal(t, tc.expectedCredentialHint, interceptor.Credential().Hint, "credential hint")
			if pool != nil {
				assert.Equal(t, tc.expectedKeyStates, pool.PoolState(), "key states")
			}
		})
	}
}

// SSE bodies covering an agentic-continuation flow.
const (
	// First response: a function_call output referencing the
	// injected "test_tool". Triggers the agentic continuation
	// loop.
	toolUseStreamBody = `event: response.created
data: {"type":"response.created","response":{"id":"resp_01","object":"response","status":"in_progress"},"sequence_number":0}

event: response.completed
data: {"type":"response.completed","response":{"id":"resp_01","object":"response","status":"completed","model":"gpt-4o-mini","output":[{"type":"function_call","id":"fc_01","call_id":"call_01","name":"test_tool","arguments":"{}","status":"completed"}],"usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15}},"sequence_number":1}

`

	// Second response (after the tool result is sent back):
	// a plain text message that ends the loop.
	textStreamBody = `event: response.created
data: {"type":"response.created","response":{"id":"resp_02","object":"response","status":"in_progress"},"sequence_number":0}

event: response.completed
data: {"type":"response.completed","response":{"id":"resp_02","object":"response","status":"completed","model":"gpt-4o-mini","output":[{"type":"message","id":"msg_02","role":"assistant","content":[{"type":"output_text","text":"done"}]}],"usage":{"input_tokens":15,"output_tokens":3,"total_tokens":18}},"sequence_number":1}

`
)

// TestStreamingResponsesInterceptor_AgenticLoopFailover covers
// the scenarios that span an agentic-loop continuation: the
// initial client request and the subsequent tool-call
// continuation can each fail over independently. Each iteration
// gets its own walker.
func TestStreamingResponsesInterceptor_AgenticLoopFailover(t *testing.T) {
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
		// True when ProcessRequest is expected to return an
		// error (e.g. all keys exhausted).
		expectedErr       bool
		expectedKeyStates []keypool.KeyState
		// Expected credential hint after ProcessRequest: hint of the
		// last attempted key across all agentic-loop iterations.
		expectedCredentialHint string
	}{
		{
			// Given: 2 keys; both upstream calls succeed on key-0.
			// Then: 2 requests, success body, both keys remain valid.
			name: "happy_path",
			responses: []upstreamResponse{
				{statusCode: http.StatusOK, headers: sseHeaders, body: toolUseStreamBody},
				{statusCode: http.StatusOK, headers: sseHeaders, body: textStreamBody},
			},
			expectedRequestCount: 2,
			expectedSeenKeys:     []string{"k0-long-key", "k0-long-key"},
			expectedBodyContains: "done",
			expectedKeyStates: []keypool.KeyState{
				keypool.KeyStateValid,
				keypool.KeyStateValid,
			},
			expectedCredentialHint: utils.MaskSecret("k0-long-key"),
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
			expectedRequestCount: 3,
			expectedSeenKeys:     []string{"k0-long-key", "k0-long-key", "k1-long-key"},
			expectedBodyContains: "done",
			expectedKeyStates: []keypool.KeyState{
				keypool.KeyStateTemporary,
				keypool.KeyStateValid,
			},
			expectedCredentialHint: utils.MaskSecret("k1-long-key"),
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
			expectedRequestCount: 3,
			expectedSeenKeys:     []string{"k0-long-key", "k0-long-key", "k1-long-key"},
			expectedBodyContains: "all configured keys are rate-limited",
			expectedErr:          true,
			expectedKeyStates: []keypool.KeyState{
				keypool.KeyStateTemporary,
				keypool.KeyStateTemporary,
			},
			expectedCredentialHint: utils.MaskSecret("k1-long-key"),
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

			pool, err := keypool.New([]string{"k0-long-key", "k1-long-key"}, quartz.NewMock(t))
			require.NoError(t, err)

			cfg := config.OpenAI{
				BaseURL: upstream.URL + "/",
				KeyPool: pool,
			}

			payload, err := NewRequestPayload([]byte(streamingRequestBody))
			require.NoError(t, err)

			interceptor := NewStreamingInterceptor(
				uuid.New(),
				payload,
				config.ProviderOpenAI,
				cfg,
				http.Header{},
				"Authorization",
				otel.Tracer("streaming_test"),
				intercept.NewCredentialInfo(intercept.CredentialKindCentralized, ""),
			)

			// Mock proxy with a tool the upstream's function_call
			// response will reference. The stub caller returns a
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

			req := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
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
			assert.Equal(t, tc.expectedCredentialHint, interceptor.Credential().Hint, "credential hint")

			seenKeysMu.Lock()
			defer seenKeysMu.Unlock()
			assert.Equal(t, tc.expectedSeenKeys, seenKeys, "seen keys")
			assert.Equal(t, tc.expectedKeyStates, pool.PoolState(), "key states")
		})
	}
}

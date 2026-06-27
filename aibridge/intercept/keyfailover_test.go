package intercept_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/sjson"
	"go.opentelemetry.io/otel"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/fixtures"
	"github.com/coder/coder/v2/aibridge/intercept"
	"github.com/coder/coder/v2/aibridge/intercept/chatcompletions"
	"github.com/coder/coder/v2/aibridge/intercept/messages"
	"github.com/coder/coder/v2/aibridge/intercept/responses"
	"github.com/coder/coder/v2/aibridge/internal/testutil"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/metrics"
	"github.com/coder/coder/v2/aibridge/utils"
	"github.com/coder/coder/v2/coderd/coderdtest/promhelp"
	codertestutil "github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// interceptorCase parameterizes the failover tests over the interceptors. It
// captures the per-API differences (request shape, auth header, and route) so a
// single set of scenarios runs against every one.
type interceptorCase struct {
	// name labels the subtest.
	name string
	// provider is the provider name used to build the key pool and to label its
	// failover metrics.
	provider string
	// path is the route the interceptor handles.
	path string
	// authHeader is the header the upstream key is carried in. It is also used
	// to read the key back off a recorded upstream request.
	authHeader string
	// fixture returns the txtar fixture for the given mode. When agentic is true
	// it returns the injected-tool fixture, whose first response calls a tool and
	// whose second is the final answer, otherwise the simple success fixture.
	fixture func(streaming, agentic bool) []byte
	// agenticStreamErrorEvent is the SSE marker a mid-loop pool exhaustion
	// produces once the agentic stream has started. It is empty for responses,
	// which buffers agentic events and writes the error status directly instead,
	// like the blocking path.
	agenticStreamErrorEvent string
	// streamDoneEvent is the terminal SSE event a completed streaming response
	// emits. A successful agentic continuation streams the final response, so its
	// presence confirms that response reached the client.
	streamDoneEvent string
	// newInterceptor builds an interceptor pointed at upstreamURL. pool is the
	// centralized key pool, or nil for BYOK, in which case byokKey is the
	// user-supplied key.
	newInterceptor func(t *testing.T, streaming bool, upstreamURL string, reqBody []byte, pool *keypool.Pool, byokKey string) intercept.Interceptor
}

// interceptorCases is the set of interceptors the failover tests run against,
// one entry per supported API.
var interceptorCases = []interceptorCase{
	{
		name:       "messages",
		provider:   config.ProviderAnthropic,
		path:       "/v1/messages",
		authHeader: "X-Api-Key",
		fixture: func(_, agentic bool) []byte {
			if agentic {
				return fixtures.AntSingleInjectedTool
			}
			return fixtures.AntSimple
		},
		agenticStreamErrorEvent: "event: error",
		streamDoneEvent:         "event: message_stop",
		newInterceptor: func(t *testing.T, streaming bool, upstreamURL string, reqBody []byte, pool *keypool.Pool, byokKey string) intercept.Interceptor {
			var cred intercept.Credential
			if pool != nil {
				cred = &intercept.CentralizedPool{Pool: pool, Header: "X-Api-Key"}
			} else {
				cred = intercept.BYOK{Secret: byokKey, Header: "X-Api-Key"}
			}
			cfg := intercept.Config{
				ProviderName: config.ProviderAnthropic,
				BaseURL:      upstreamURL + "/",
			}

			payload, err := messages.NewRequestPayload(reqBody)
			require.NoError(t, err)

			id, tracer := uuid.New(), otel.Tracer("keyfailover")
			if streaming {
				return messages.NewStreamingInterceptor(id, payload, cfg, cred, nil, nil, http.Header{}, tracer)
			}
			return messages.NewBlockingInterceptor(id, payload, cfg, cred, nil, nil, http.Header{}, tracer)
		},
	},
	{
		name:       "chatcompletions",
		provider:   config.ProviderOpenAI,
		path:       "/v1/chat/completions",
		authHeader: "Authorization",
		fixture: func(_, agentic bool) []byte {
			if agentic {
				return fixtures.OaiChatSingleInjectedTool
			}
			return fixtures.OaiChatSimple
		},
		agenticStreamErrorEvent: `data: {"error"`,
		streamDoneEvent:         "data: [DONE]",
		newInterceptor: func(t *testing.T, streaming bool, upstreamURL string, reqBody []byte, pool *keypool.Pool, byokKey string) intercept.Interceptor {
			var cred intercept.Credential
			if pool != nil {
				cred = &intercept.CentralizedPool{Pool: pool, Header: "Authorization"}
			} else {
				cred = intercept.BYOK{Secret: byokKey, Header: "Authorization"}
			}
			cfg := intercept.Config{
				ProviderName: config.ProviderOpenAI,
				BaseURL:      upstreamURL + "/",
			}

			var req chatcompletions.ChatCompletionNewParamsWrapper
			require.NoError(t, json.Unmarshal(reqBody, &req))

			id, tracer := uuid.New(), otel.Tracer("keyfailover")
			if streaming {
				return chatcompletions.NewStreamingInterceptor(id, &req, cfg, cred, http.Header{}, tracer)
			}
			return chatcompletions.NewBlockingInterceptor(id, &req, cfg, cred, http.Header{}, tracer)
		},
	},
	{
		name:       "responses",
		provider:   config.ProviderOpenAI,
		path:       "/v1/responses",
		authHeader: "Authorization",
		fixture: func(streaming, agentic bool) []byte {
			switch {
			case streaming && agentic:
				return fixtures.OaiResponsesStreamingSingleInjectedTool
			case streaming:
				return fixtures.OaiResponsesStreamingSimple
			case agentic:
				return fixtures.OaiResponsesBlockingSingleInjectedTool
			default:
				return fixtures.OaiResponsesBlockingSimple
			}
		},
		streamDoneEvent: "event: response.completed",
		newInterceptor: func(t *testing.T, streaming bool, upstreamURL string, reqBody []byte, pool *keypool.Pool, byokKey string) intercept.Interceptor {
			var cred intercept.Credential
			if pool != nil {
				cred = &intercept.CentralizedPool{Pool: pool, Header: "Authorization"}
			} else {
				cred = intercept.BYOK{Secret: byokKey, Header: "Authorization"}
			}
			cfg := intercept.Config{
				ProviderName: config.ProviderOpenAI,
				BaseURL:      upstreamURL + "/",
			}

			payload, err := responses.NewRequestPayload(reqBody)
			require.NoError(t, err)

			id, tracer := uuid.New(), otel.Tracer("keyfailover")
			if streaming {
				return responses.NewStreamingInterceptor(id, payload, cfg, cred, http.Header{}, tracer)
			}
			return responses.NewBlockingInterceptor(id, payload, cfg, cred, http.Header{}, tracer)
		},
	},
}

// TestInterception_KeyFailover verifies that, within a single interception, the
// centralized key pool fails over across keys (temporary on 429, permanent on
// 401/403) and reports exhaustion, for every interceptor in both blocking and
// streaming mode.
func TestInterception_KeyFailover(t *testing.T) {
	t.Parallel()

	const (
		k0, k1, k2 = "k0-long-key", "k1-long-key", "k2-long-key"
		byokKey    = "user-byok-key"
	)
	errResp := testutil.NewErrorResponse

	tests := []struct {
		name    string
		keys    []string
		byokKey string
		// responses builds the upstream responses in call order. success is the
		// interceptor's fixture success response, so each case only specifies
		// the error responses that drive failover.
		responses            func(success testutil.UpstreamResponse) []testutil.UpstreamResponse
		expectedStatus       int
		expectedRetryAfter   string
		expectedKeyStates    []keypool.KeyState
		expectedSeenKeys     []string
		expectedBodyContains string
		// Expected key_pool_state_transitions_total counts by reason.
		expectedTransitions map[string]int
		// Expected key_pool_exhaustions_total counts by outcome.
		expectedExhaustions map[string]int
	}{
		{
			// One valid key succeeds on the first attempt.
			name:              "single_valid_key",
			keys:              []string{k0},
			responses:         func(s testutil.UpstreamResponse) []testutil.UpstreamResponse { return []testutil.UpstreamResponse{s} },
			expectedStatus:    http.StatusOK,
			expectedKeyStates: []keypool.KeyState{keypool.KeyStateValid},
			expectedSeenKeys:  []string{k0},
		},
		{
			// A 429 marks the key temporary and fails over to the next one.
			name: "failover_after_429",
			keys: []string{k0, k1},
			responses: func(s testutil.UpstreamResponse) []testutil.UpstreamResponse {
				return []testutil.UpstreamResponse{errResp(http.StatusTooManyRequests, "5"), s}
			},
			expectedStatus:      http.StatusOK,
			expectedKeyStates:   []keypool.KeyState{keypool.KeyStateTemporary, keypool.KeyStateValid},
			expectedSeenKeys:    []string{k0, k1},
			expectedTransitions: map[string]int{"rate_limited": 1},
		},
		{
			// A 401 marks the key permanent and fails over to the next one.
			name: "failover_after_401",
			keys: []string{k0, k1},
			responses: func(s testutil.UpstreamResponse) []testutil.UpstreamResponse {
				return []testutil.UpstreamResponse{errResp(http.StatusUnauthorized, ""), s}
			},
			expectedStatus:      http.StatusOK,
			expectedKeyStates:   []keypool.KeyState{keypool.KeyStatePermanent, keypool.KeyStateValid},
			expectedSeenKeys:    []string{k0, k1},
			expectedTransitions: map[string]int{"unauthorized": 1},
		},
		{
			// A 403 marks the key permanent and fails over to the next one.
			name: "failover_after_403",
			keys: []string{k0, k1},
			responses: func(s testutil.UpstreamResponse) []testutil.UpstreamResponse {
				return []testutil.UpstreamResponse{errResp(http.StatusForbidden, ""), s}
			},
			expectedStatus:      http.StatusOK,
			expectedKeyStates:   []keypool.KeyState{keypool.KeyStatePermanent, keypool.KeyStateValid},
			expectedSeenKeys:    []string{k0, k1},
			expectedTransitions: map[string]int{"forbidden": 1},
		},
		{
			// Every key is rate-limited, so the pool is exhausted and the
			// smallest remaining cooldown is reported.
			name: "all_keys_rate_limited",
			keys: []string{k0, k1, k2},
			responses: func(testutil.UpstreamResponse) []testutil.UpstreamResponse {
				return []testutil.UpstreamResponse{
					errResp(http.StatusTooManyRequests, "5"),
					errResp(http.StatusTooManyRequests, "3"),
					errResp(http.StatusTooManyRequests, "10"),
				}
			},
			expectedStatus:       http.StatusTooManyRequests,
			expectedRetryAfter:   "3",
			expectedBodyContains: "all configured keys are rate-limited",
			expectedKeyStates: []keypool.KeyState{
				keypool.KeyStateTemporary,
				keypool.KeyStateTemporary,
				keypool.KeyStateTemporary,
			},
			expectedSeenKeys:    []string{k0, k1, k2},
			expectedTransitions: map[string]int{"rate_limited": 3},
			expectedExhaustions: map[string]int{"rate_limited": 1},
		},
		{
			// Every key is unauthorized, so the pool is permanently exhausted.
			name: "all_keys_unauthorized",
			keys: []string{k0, k1},
			responses: func(testutil.UpstreamResponse) []testutil.UpstreamResponse {
				return []testutil.UpstreamResponse{
					errResp(http.StatusUnauthorized, ""),
					errResp(http.StatusUnauthorized, ""),
				}
			},
			expectedStatus:      http.StatusBadGateway,
			expectedKeyStates:   []keypool.KeyState{keypool.KeyStatePermanent, keypool.KeyStatePermanent},
			expectedSeenKeys:    []string{k0, k1},
			expectedTransitions: map[string]int{"unauthorized": 2},
			expectedExhaustions: map[string]int{"auth_failed": 1},
		},
		{
			// A 500 is not a key-specific failure, so it does not fail over.
			name: "server_error_no_failover",
			keys: []string{k0, k1},
			responses: func(testutil.UpstreamResponse) []testutil.UpstreamResponse {
				return []testutil.UpstreamResponse{errResp(http.StatusInternalServerError, "")}
			},
			expectedStatus:    http.StatusInternalServerError,
			expectedKeyStates: []keypool.KeyState{keypool.KeyStateValid, keypool.KeyStateValid},
			expectedSeenKeys:  []string{k0},
		},
		{
			// BYOK requests carry a user key and never fail over.
			name:    "byok_no_failover",
			byokKey: byokKey,
			responses: func(testutil.UpstreamResponse) []testutil.UpstreamResponse {
				return []testutil.UpstreamResponse{errResp(http.StatusTooManyRequests, "5")}
			},
			expectedStatus:     http.StatusTooManyRequests,
			expectedRetryAfter: "5",
			expectedSeenKeys:   []string{byokKey},
		},
	}

	for _, ic := range interceptorCases {
		for _, mode := range []string{"blocking", "streaming"} {
			streaming := mode == "streaming"
			for _, tc := range tests {
				t.Run(ic.name+"/"+mode+"/"+tc.name, func(t *testing.T) {
					t.Parallel()

					reg := prometheus.NewRegistry()
					m := metrics.NewMetrics(reg)
					var pool *keypool.Pool
					if len(tc.keys) > 0 {
						var err error
						pool, err = keypool.New(ic.provider, tc.keys, quartz.NewMock(t), m)
						require.NoError(t, err)
					}

					fixture := fixtures.Parse(t, ic.fixture(streaming, false))
					reqBody := fixture.Request()
					if streaming {
						var err error
						reqBody, err = sjson.SetBytes(reqBody, "stream", true)
						require.NoError(t, err)
					}
					upstream := testutil.NewMockUpstream(t.Context(), t, tc.responses(testutil.NewFixtureResponse(fixture))...)

					interceptor := ic.newInterceptor(t, streaming, upstream.URL, reqBody, pool, tc.byokKey)
					interceptor.Setup(slog.Make(), &testutil.MockRecorder{}, nil)

					req := httptest.NewRequest(http.MethodPost, ic.path, nil)
					w := httptest.NewRecorder()
					err := interceptor.ProcessRequest(w, req)
					if tc.expectedStatus == http.StatusOK {
						require.NoError(t, err)
					} else {
						require.Error(t, err)
					}

					assert.Equal(t, tc.expectedStatus, w.Code, "response status code")
					assert.Equal(t, tc.expectedRetryAfter, w.Header().Get("Retry-After"), "Retry-After header")
					if pool != nil {
						assert.Equal(t, tc.expectedKeyStates, pool.PoolState(), "key states")
					}

					var seenKeys []string
					for _, r := range upstream.ReceivedRequests() {
						seenKeys = append(seenKeys, testutil.KeyFromHeader(ic.authHeader, r.Header))
					}
					assert.Equal(t, tc.expectedSeenKeys, seenKeys, "seen keys")

					if len(tc.expectedSeenKeys) > 0 {
						assert.Equal(t, utils.MaskSecret(tc.expectedSeenKeys[len(tc.expectedSeenKeys)-1]),
							interceptor.Credential().Hint(), "credential hint")
					}
					if tc.expectedBodyContains != "" {
						assert.Contains(t, w.Body.String(), tc.expectedBodyContains, "response body")
					}

					// A centralized interception records one failover-attempts
					// observation, labeled with the provider, summing the keys
					// tried (one per upstream attempt). BYOK has no pool, so none.
					if pool != nil {
						hist := promhelp.HistogramValue(t, reg, "key_pool_failover_attempts",
							prometheus.Labels{"provider": ic.provider})
						assert.Equal(t, uint64(1), hist.GetSampleCount())
						assert.Equal(t, float64(len(tc.expectedSeenKeys)), hist.GetSampleSum())
					} else {
						assert.Nil(t, promhelp.MetricValue(t, reg, "key_pool_failover_attempts",
							prometheus.Labels{"provider": ic.provider}))
					}

					gathered, err := reg.Gather()
					require.NoError(t, err)
					// One transition per marked key, by reason.
					for _, reason := range []string{"rate_limited", "unauthorized", "forbidden"} {
						if want := tc.expectedTransitions[reason]; want > 0 {
							assert.True(t, codertestutil.PromCounterHasValue(t, gathered, float64(want), "key_pool_state_transitions_total", ic.provider, reason))
						} else {
							assert.False(t, codertestutil.PromCounterGathered(t, gathered, "key_pool_state_transitions_total", ic.provider, reason))
						}
					}
					// Exhaustion outcome when no usable key remains.
					for _, outcome := range []string{"rate_limited", "auth_failed"} {
						if want := tc.expectedExhaustions[outcome]; want > 0 {
							assert.True(t, codertestutil.PromCounterHasValue(t, gathered, float64(want), "key_pool_exhaustions_total", outcome, ic.provider))
						} else {
							assert.False(t, codertestutil.PromCounterGathered(t, gathered, "key_pool_exhaustions_total", outcome, ic.provider))
						}
					}
				})
			}
		}
	}
}

// TestInterception_AgenticLoopFailover covers the scenarios that span an
// agentic-loop continuation: the initial client request and the subsequent
// tool-call continuation can each fail over independently, in both blocking and
// streaming mode. Each iteration gets its own walker.
func TestInterception_AgenticLoopFailover(t *testing.T) {
	t.Parallel()

	const k0, k1 = "k0-long-key", "k1-long-key"
	errResp := testutil.NewErrorResponse

	tests := []struct {
		name string
		keys []string
		// responses builds the upstream responses in call order. toolCall is the
		// tool_use response and final is the response after the tool result.
		responses            func(toolCall, final testutil.UpstreamResponse) []testutil.UpstreamResponse
		expectedStatus       int
		expectedRetryAfter   string
		expectedKeyStates    []keypool.KeyState
		expectedSeenKeys     []string
		expectedBodyContains string
		// Expected key_pool_state_transitions_total counts by reason.
		expectedTransitions map[string]int
		// Expected key_pool_exhaustions_total counts by outcome.
		expectedExhaustions map[string]int
		// expectErr is true when ProcessRequest returns an error because the
		// pool is exhausted.
		expectErr bool
	}{
		{
			// Both upstream calls succeed on the first key.
			name: "happy_path",
			keys: []string{k0, k1},
			responses: func(toolCall, final testutil.UpstreamResponse) []testutil.UpstreamResponse {
				return []testutil.UpstreamResponse{toolCall, final}
			},
			expectedStatus:    http.StatusOK,
			expectedKeyStates: []keypool.KeyState{keypool.KeyStateValid, keypool.KeyStateValid},
			expectedSeenKeys:  []string{k0, k0},
		},
		{
			// The continuation is rate-limited on the first key and fails over
			// to the second.
			name: "agentic_failover_to_k1",
			keys: []string{k0, k1},
			responses: func(toolCall, final testutil.UpstreamResponse) []testutil.UpstreamResponse {
				return []testutil.UpstreamResponse{toolCall, errResp(http.StatusTooManyRequests, "5"), final}
			},
			expectedStatus:      http.StatusOK,
			expectedKeyStates:   []keypool.KeyState{keypool.KeyStateTemporary, keypool.KeyStateValid},
			expectedSeenKeys:    []string{k0, k0, k1},
			expectedTransitions: map[string]int{"rate_limited": 1},
		},
		{
			// The continuation is rate-limited on every key, exhausting the pool.
			name: "agentic_all_keys_fail",
			keys: []string{k0, k1},
			responses: func(toolCall, _ testutil.UpstreamResponse) []testutil.UpstreamResponse {
				return []testutil.UpstreamResponse{
					toolCall,
					errResp(http.StatusTooManyRequests, "5"),
					errResp(http.StatusTooManyRequests, "3"),
				}
			},
			expectedStatus:       http.StatusTooManyRequests,
			expectedRetryAfter:   "3",
			expectedBodyContains: "all configured keys are rate-limited",
			expectedKeyStates:    []keypool.KeyState{keypool.KeyStateTemporary, keypool.KeyStateTemporary},
			expectedSeenKeys:     []string{k0, k0, k1},
			expectedTransitions:  map[string]int{"rate_limited": 2},
			expectedExhaustions:  map[string]int{"rate_limited": 1},
			expectErr:            true,
		},
	}

	for _, ic := range interceptorCases {
		for _, mode := range []string{"blocking", "streaming"} {
			streaming := mode == "streaming"
			for _, tc := range tests {
				t.Run(ic.name+"/"+mode+"/"+tc.name, func(t *testing.T) {
					t.Parallel()

					reg := prometheus.NewRegistry()
					m := metrics.NewMetrics(reg)
					pool, err := keypool.New(ic.provider, tc.keys, quartz.NewMock(t), m)
					require.NoError(t, err)

					fixture := fixtures.Parse(t, ic.fixture(streaming, true))
					reqBody := fixture.Request()
					if streaming {
						reqBody, err = sjson.SetBytes(reqBody, "stream", true)
						require.NoError(t, err)
					}
					toolCall, final := testutil.NewFixtureResponse(fixture), testutil.NewFixtureToolResponse(fixture)
					upstream := testutil.NewMockUpstream(t.Context(), t, tc.responses(toolCall, final)...)

					interceptor := ic.newInterceptor(t, streaming, upstream.URL, reqBody, pool, "")
					interceptor.Setup(slog.Make(), &testutil.MockRecorder{}, &testutil.MockServerProxier{ResolveAnyTool: true})

					req := httptest.NewRequest(http.MethodPost, ic.path, nil)
					w := httptest.NewRecorder()
					err = interceptor.ProcessRequest(w, req)
					if tc.expectErr {
						require.Error(t, err)
					} else {
						require.NoError(t, err)
					}

					// Once streaming has started, exhaustion is relayed as an SSE
					// error event under a 200.
					wantStatus, wantRetryAfter := tc.expectedStatus, tc.expectedRetryAfter
					if streaming && tc.expectErr && ic.agenticStreamErrorEvent != "" {
						wantStatus, wantRetryAfter = http.StatusOK, ""
					}
					assert.Equal(t, wantStatus, w.Code, "response status code")
					assert.Equal(t, wantRetryAfter, w.Header().Get("Retry-After"), "Retry-After header")
					if streaming && tc.expectErr && ic.agenticStreamErrorEvent != "" {
						assert.Contains(t, w.Body.String(), ic.agenticStreamErrorEvent, "exhaustion relayed as SSE event")
					}
					if streaming && !tc.expectErr {
						assert.Contains(t, w.Body.String(), ic.streamDoneEvent, "final response streamed to client")
					}
					assert.Equal(t, tc.expectedKeyStates, pool.PoolState(), "key states")

					var seenKeys []string
					for _, r := range upstream.ReceivedRequests() {
						seenKeys = append(seenKeys, testutil.KeyFromHeader(ic.authHeader, r.Header))
					}
					assert.Equal(t, tc.expectedSeenKeys, seenKeys, "seen keys")

					if len(tc.expectedSeenKeys) > 0 {
						assert.Equal(t, utils.MaskSecret(tc.expectedSeenKeys[len(tc.expectedSeenKeys)-1]),
							interceptor.Credential().Hint(), "credential hint")
					}
					if tc.expectedBodyContains != "" {
						assert.Contains(t, w.Body.String(), tc.expectedBodyContains, "response body")
					}

					// One observation per interception, summing keys tried across
					// all agentic-loop iterations (one per upstream attempt).
					hist := promhelp.HistogramValue(t, reg, "key_pool_failover_attempts",
						prometheus.Labels{"provider": ic.provider})
					assert.Equal(t, uint64(1), hist.GetSampleCount())
					assert.Equal(t, float64(len(tc.expectedSeenKeys)), hist.GetSampleSum())

					gathered, err := reg.Gather()
					require.NoError(t, err)
					// One transition per marked key, by reason.
					for _, reason := range []string{"rate_limited", "unauthorized", "forbidden"} {
						if want := tc.expectedTransitions[reason]; want > 0 {
							assert.True(t, codertestutil.PromCounterHasValue(t, gathered, float64(want), "key_pool_state_transitions_total", ic.provider, reason))
						} else {
							assert.False(t, codertestutil.PromCounterGathered(t, gathered, "key_pool_state_transitions_total", ic.provider, reason))
						}
					}
					// Exhaustion outcome when no usable key remains.
					for _, outcome := range []string{"rate_limited", "auth_failed"} {
						if want := tc.expectedExhaustions[outcome]; want > 0 {
							assert.True(t, codertestutil.PromCounterHasValue(t, gathered, float64(want), "key_pool_exhaustions_total", outcome, ic.provider))
						} else {
							assert.False(t, codertestutil.PromCounterGathered(t, gathered, "key_pool_exhaustions_total", outcome, ic.provider))
						}
					}
				})
			}
		}
	}
}

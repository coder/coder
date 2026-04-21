package integrationtest //nolint:testpackage // tests unexported internals

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	promtest "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/aibridge/config"
	"github.com/coder/aibridge/metrics"
	"github.com/coder/aibridge/provider"
)

// Common response bodies for circuit breaker tests.
const (
	anthropicRateLimitError = `{"type":"error","error":{"type":"rate_limit_error","message":"rate limited"}}`
	openAIRateLimitError    = `{"error":{"type":"rate_limit_error","message":"rate limited","code":"rate_limit_exceeded"}}`
)

func anthropicSuccessResponse(model string) string {
	return fmt.Sprintf(`{"id":"msg_01","type":"message","role":"assistant","content":[{"type":"text","text":"Hello!"}],"model":%q,"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":5}}`, model)
}

func openAISuccessResponse(model string) string {
	return fmt.Sprintf(`{"id":"chatcmpl-123","object":"chat.completion","created":1677652288,"model":%q,"choices":[{"index":0,"message":{"role":"assistant","content":"Hello!"},"finish_reason":"stop"}],"usage":{"prompt_tokens":9,"completion_tokens":12,"total_tokens":21}}`, model)
}

// TestCircuitBreaker_FullRecoveryCycle tests the complete circuit breaker lifecycle:
// closed → open (after consecutive failures) → half-open (after timeout) → closed (after successful request)
func TestCircuitBreaker_FullRecoveryCycle(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name           string
		errorBody      string
		successBody    string
		requestBody    string
		headers        http.Header
		path           string
		createProvider func(baseURL string, cbConfig *config.CircuitBreaker) provider.Provider
		expectProvider string
		expectEndpoint string
		expectModel    string
	}

	tests := []testCase{
		{
			name:           "Anthropic",
			expectProvider: config.ProviderAnthropic,
			expectEndpoint: "/v1/messages",
			expectModel:    "claude-sonnet-4-20250514",
			errorBody:      anthropicRateLimitError,
			successBody:    anthropicSuccessResponse("claude-sonnet-4-20250514"),
			requestBody:    `{"model":"claude-sonnet-4-20250514","max_tokens":1024,"messages":[{"role":"user","content":"hi"}]}`,
			headers: http.Header{
				"x-api-key":         {"test"},
				"anthropic-version": {"2023-06-01"},
			},
			path: pathAnthropicMessages,
			createProvider: func(baseURL string, cbConfig *config.CircuitBreaker) provider.Provider {
				return provider.NewAnthropic(config.Anthropic{
					BaseURL:        baseURL,
					Key:            "test-key",
					CircuitBreaker: cbConfig,
				}, nil)
			},
		},
		{
			name:           "OpenAI",
			expectProvider: config.ProviderOpenAI,
			expectEndpoint: "/v1/chat/completions",
			expectModel:    "gpt-4o",
			errorBody:      openAIRateLimitError,
			successBody:    openAISuccessResponse("gpt-4o"),
			requestBody:    `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`,
			headers:        http.Header{"Authorization": {"Bearer test-key"}},
			path:           pathOpenAIChatCompletions,
			createProvider: func(baseURL string, cbConfig *config.CircuitBreaker) provider.Provider {
				return provider.NewOpenAI(config.OpenAI{
					BaseURL:        baseURL,
					Key:            "test-key",
					CircuitBreaker: cbConfig,
				})
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var upstreamCalls atomic.Int32
			var shouldFail atomic.Bool
			shouldFail.Store(true)

			// Mock upstream that returns 429 or 200 based on shouldFail flag.
			// x-should-retry: false is required to disable SDK automatic retries (default MaxRetries=2).
			mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				upstreamCalls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("x-should-retry", "false")
				if shouldFail.Load() {
					w.WriteHeader(http.StatusTooManyRequests)
					_, _ = w.Write([]byte(tc.errorBody))
				} else {
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(tc.successBody))
				}
			}))
			defer mockUpstream.Close()

			m := metrics.NewMetrics(prometheus.NewRegistry())

			// Create provider with circuit breaker config
			cbConfig := &config.CircuitBreaker{
				FailureThreshold: 2,
				Interval:         time.Minute,
				Timeout:          50 * time.Millisecond,
				MaxRequests:      1,
			}

			ctx := t.Context()
			bridgeServer := newBridgeTestServer(ctx, t, mockUpstream.URL,
				withCustomProvider(tc.createProvider(mockUpstream.URL, cbConfig)),
				withMetrics(m),
				withActor("test-user-id", nil),
			)

			doRequest := func() int {
				resp, err := bridgeServer.makeRequest(t, http.MethodPost, tc.path, []byte(tc.requestBody), tc.headers)
				require.NoError(t, err)
				_, err = io.ReadAll(resp.Body)
				require.NoError(t, err)
				require.NoError(t, resp.Body.Close())
				return resp.StatusCode
			}

			// Phase 1: Trip the circuit breaker
			// First FailureThreshold requests hit upstream, get 429
			for i := uint32(0); i < cbConfig.FailureThreshold; i++ {
				status := doRequest()
				assert.Equal(t, http.StatusTooManyRequests, status)
			}
			//nolint:gosec // G115: test constant, no overflow risk
			assert.Equal(t, int32(cbConfig.FailureThreshold), upstreamCalls.Load())

			// Phase 2: Verify circuit is open
			// Request should be blocked by circuit breaker (no upstream call)
			status := doRequest()
			assert.Equal(t, http.StatusServiceUnavailable, status)
			//nolint:gosec // G115: test constant, no overflow risk
			assert.Equal(t, int32(cbConfig.FailureThreshold), upstreamCalls.Load(), "No new upstream call when circuit is open")

			// Verify metrics show circuit is open
			trips := promtest.ToFloat64(m.CircuitBreakerTrips.WithLabelValues(tc.expectProvider, tc.expectEndpoint, tc.expectModel))
			assert.Equal(t, 1.0, trips, "CircuitBreakerTrips should be 1")

			state := promtest.ToFloat64(m.CircuitBreakerState.WithLabelValues(tc.expectProvider, tc.expectEndpoint, tc.expectModel))
			assert.Equal(t, 1.0, state, "CircuitBreakerState should be 1 (open)")

			rejects := promtest.ToFloat64(m.CircuitBreakerRejects.WithLabelValues(tc.expectProvider, tc.expectEndpoint, tc.expectModel))
			assert.Equal(t, 1.0, rejects, "CircuitBreakerRejects should be 1")

			// Phase 3: Wait for timeout to transition to half-open
			time.Sleep(cbConfig.Timeout + 10*time.Millisecond)

			// Switch upstream to return success
			shouldFail.Store(false)

			// Phase 4: Recovery - request in half-open state should succeed and close circuit
			upstreamCallsBefore := upstreamCalls.Load()
			status = doRequest()
			assert.Equal(t, http.StatusOK, status, "Request should succeed in half-open state")
			assert.Equal(t, upstreamCallsBefore+1, upstreamCalls.Load(), "Request should reach upstream in half-open state")

			// Verify circuit is now closed
			state = promtest.ToFloat64(m.CircuitBreakerState.WithLabelValues(tc.expectProvider, tc.expectEndpoint, tc.expectModel))
			assert.Equal(t, 0.0, state, "CircuitBreakerState should be 0 (closed) after recovery")

			// Phase 5: Verify circuit is fully functional again
			// Multiple requests should all succeed and reach upstream
			for i := 0; i < 3; i++ {
				status = doRequest()
				assert.Equal(t, http.StatusOK, status, "Request should succeed after circuit closes")
			}

			// All requests should have reached upstream
			assert.Equal(t, upstreamCallsBefore+4, upstreamCalls.Load(), "All requests should reach upstream after circuit closes")

			// Rejects count should not have increased
			rejects = promtest.ToFloat64(m.CircuitBreakerRejects.WithLabelValues(tc.expectProvider, tc.expectEndpoint, tc.expectModel))
			assert.Equal(t, 1.0, rejects, "CircuitBreakerRejects should still be 1 (no new rejects)")
		})
	}
}

// TestCircuitBreaker_HalfOpenFailure tests that a failed request in half-open state
// returns the circuit to open: closed → open → half-open → open
func TestCircuitBreaker_HalfOpenFailure(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name           string
		errorBody      string
		requestBody    string
		headers        http.Header
		path           string
		createProvider func(baseURL string, cbConfig *config.CircuitBreaker) provider.Provider
		expectProvider string
		expectEndpoint string
		expectModel    string
	}

	tests := []testCase{
		{
			name:           "Anthropic",
			expectProvider: config.ProviderAnthropic,
			expectEndpoint: "/v1/messages",
			expectModel:    "claude-sonnet-4-20250514",
			errorBody:      anthropicRateLimitError,
			requestBody:    `{"model":"claude-sonnet-4-20250514","max_tokens":1024,"messages":[{"role":"user","content":"hi"}]}`,
			headers: http.Header{
				"x-api-key":         {"test"},
				"anthropic-version": {"2023-06-01"},
			},
			path: pathAnthropicMessages,
			createProvider: func(baseURL string, cbConfig *config.CircuitBreaker) provider.Provider {
				return provider.NewAnthropic(config.Anthropic{
					BaseURL:        baseURL,
					Key:            "test-key",
					CircuitBreaker: cbConfig,
				}, nil)
			},
		},
		{
			name:           "OpenAI",
			expectProvider: config.ProviderOpenAI,
			expectEndpoint: "/v1/chat/completions",
			expectModel:    "gpt-4o",
			errorBody:      openAIRateLimitError,
			requestBody:    `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`,
			headers:        http.Header{"Authorization": {"Bearer test-key"}},
			path:           pathOpenAIChatCompletions,
			createProvider: func(baseURL string, cbConfig *config.CircuitBreaker) provider.Provider {
				return provider.NewOpenAI(config.OpenAI{
					BaseURL:        baseURL,
					Key:            "test-key",
					CircuitBreaker: cbConfig,
				})
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var upstreamCalls atomic.Int32

			// Mock upstream that always returns 429.
			mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				upstreamCalls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("x-should-retry", "false")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(tc.errorBody))
			}))
			defer mockUpstream.Close()

			m := metrics.NewMetrics(prometheus.NewRegistry())

			cbConfig := &config.CircuitBreaker{
				FailureThreshold: 2,
				Interval:         time.Minute,
				Timeout:          50 * time.Millisecond,
				MaxRequests:      1,
			}

			ctx := t.Context()
			bridgeServer := newBridgeTestServer(ctx, t, mockUpstream.URL,
				withCustomProvider(tc.createProvider(mockUpstream.URL, cbConfig)),
				withMetrics(m),
				withActor("test-user-id", nil),
			)

			doRequest := func() int {
				resp, err := bridgeServer.makeRequest(t, http.MethodPost, tc.path, []byte(tc.requestBody), tc.headers)
				require.NoError(t, err)
				_, err = io.ReadAll(resp.Body)
				require.NoError(t, err)
				require.NoError(t, resp.Body.Close())
				return resp.StatusCode
			}

			// Phase 1: Trip the circuit
			for i := uint32(0); i < cbConfig.FailureThreshold; i++ {
				status := doRequest()
				assert.Equal(t, http.StatusTooManyRequests, status)
			}

			// Verify circuit is open
			status := doRequest()
			assert.Equal(t, http.StatusServiceUnavailable, status)

			trips := promtest.ToFloat64(m.CircuitBreakerTrips.WithLabelValues(tc.expectProvider, tc.expectEndpoint, tc.expectModel))
			assert.Equal(t, 1.0, trips, "CircuitBreakerTrips should be 1")

			// Phase 2: Wait for half-open state
			time.Sleep(cbConfig.Timeout + 10*time.Millisecond)

			// Phase 3: Request in half-open state fails, circuit should re-open
			upstreamCallsBefore := upstreamCalls.Load()
			status = doRequest()
			assert.Equal(t, http.StatusTooManyRequests, status, "Request should fail in half-open state")
			assert.Equal(t, upstreamCallsBefore+1, upstreamCalls.Load(), "Request should reach upstream in half-open state")

			// Circuit should be open again - next request should be rejected immediately
			status = doRequest()
			assert.Equal(t, http.StatusServiceUnavailable, status, "Circuit should be open again after half-open failure")
			assert.Equal(t, upstreamCallsBefore+1, upstreamCalls.Load(), "Request should NOT reach upstream when circuit re-opens")

			// Verify metrics: trips should be 2 now (tripped twice)
			trips = promtest.ToFloat64(m.CircuitBreakerTrips.WithLabelValues(tc.expectProvider, tc.expectEndpoint, tc.expectModel))
			assert.Equal(t, 2.0, trips, "CircuitBreakerTrips should be 2 after half-open failure")

			state := promtest.ToFloat64(m.CircuitBreakerState.WithLabelValues(tc.expectProvider, tc.expectEndpoint, tc.expectModel))
			assert.Equal(t, 1.0, state, "CircuitBreakerState should be 1 (open) after half-open failure")
		})
	}
}

// TestCircuitBreaker_HalfOpenMaxRequests tests that MaxRequests limits concurrent
// requests in half-open state. Requests beyond the limit should be rejected.
func TestCircuitBreaker_HalfOpenMaxRequests(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name           string
		errorBody      string
		successBody    string
		requestBody    string
		headers        http.Header
		path           string
		createProvider func(baseURL string, cbConfig *config.CircuitBreaker) provider.Provider
		expectProvider string
		expectEndpoint string
		expectModel    string
	}

	tests := []testCase{
		{
			name:           "Anthropic",
			expectProvider: config.ProviderAnthropic,
			expectEndpoint: "/v1/messages",
			expectModel:    "claude-sonnet-4-20250514",
			errorBody:      anthropicRateLimitError,
			successBody:    anthropicSuccessResponse("claude-sonnet-4-20250514"),
			requestBody:    `{"model":"claude-sonnet-4-20250514","max_tokens":1024,"messages":[{"role":"user","content":"hi"}]}`,
			headers: http.Header{
				"x-api-key":         {"test"},
				"anthropic-version": {"2023-06-01"},
			},
			path: pathAnthropicMessages,
			createProvider: func(baseURL string, cbConfig *config.CircuitBreaker) provider.Provider {
				return provider.NewAnthropic(config.Anthropic{
					BaseURL:        baseURL,
					Key:            "test-key",
					CircuitBreaker: cbConfig,
				}, nil)
			},
		},
		{
			name:           "OpenAI",
			expectProvider: config.ProviderOpenAI,
			expectEndpoint: "/v1/chat/completions",
			expectModel:    "gpt-4o",
			errorBody:      openAIRateLimitError,
			successBody:    openAISuccessResponse("gpt-4o"),
			requestBody:    `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`,
			headers:        http.Header{"Authorization": {"Bearer test-key"}},
			path:           pathOpenAIChatCompletions,
			createProvider: func(baseURL string, cbConfig *config.CircuitBreaker) provider.Provider {
				return provider.NewOpenAI(config.OpenAI{
					BaseURL:        baseURL,
					Key:            "test-key",
					CircuitBreaker: cbConfig,
				})
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var upstreamCalls atomic.Int32
			var shouldFail atomic.Bool
			shouldFail.Store(true)

			// Upstream is slow to ensure concurrent requests overlap in half-open state.
			mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				upstreamCalls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("x-should-retry", "false")
				if shouldFail.Load() {
					w.WriteHeader(http.StatusTooManyRequests)
					_, _ = w.Write([]byte(tc.errorBody))
				} else {
					// Slow response to ensure requests overlap
					time.Sleep(100 * time.Millisecond)
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(tc.successBody))
				}
			}))
			defer mockUpstream.Close()

			m := metrics.NewMetrics(prometheus.NewRegistry())

			const maxRequests = 2
			cbConfig := &config.CircuitBreaker{
				FailureThreshold: 2,
				Interval:         time.Minute,
				Timeout:          50 * time.Millisecond,
				MaxRequests:      maxRequests, // Allow only 2 concurrent requests in half-open
			}

			ctx := t.Context()
			bridgeServer := newBridgeTestServer(ctx, t, mockUpstream.URL,
				withCustomProvider(tc.createProvider(mockUpstream.URL, cbConfig)),
				withMetrics(m),
				withActor("test-user-id", nil),
			)

			doRequest := func() int {
				resp, err := bridgeServer.makeRequest(t, http.MethodPost, tc.path, []byte(tc.requestBody), tc.headers)
				require.NoError(t, err)
				_, err = io.ReadAll(resp.Body)
				require.NoError(t, err)
				require.NoError(t, resp.Body.Close())
				return resp.StatusCode
			}

			// Phase 1: Trip the circuit
			for i := uint32(0); i < cbConfig.FailureThreshold; i++ {
				status := doRequest()
				assert.Equal(t, http.StatusTooManyRequests, status)
			}

			// Verify circuit is open
			status := doRequest()
			assert.Equal(t, http.StatusServiceUnavailable, status)

			// Phase 2: Wait for half-open state and switch upstream to success
			time.Sleep(cbConfig.Timeout + 10*time.Millisecond)
			shouldFail.Store(false)
			upstreamCalls.Store(0)

			// Phase 3: Send concurrent requests (more than MaxRequests)
			const totalRequests = 5
			var wg sync.WaitGroup
			responses := make(chan int, totalRequests)

			for i := 0; i < totalRequests; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					status := doRequest()
					responses <- status
				}()
			}

			wg.Wait()
			close(responses)

			// Count results
			var successCount, rejectedCount int
			for status := range responses {
				switch status {
				case http.StatusOK:
					successCount++
				case http.StatusServiceUnavailable:
					rejectedCount++
				}
			}

			// Verify only MaxRequests reached upstream
			assert.Equal(t, int32(maxRequests), upstreamCalls.Load(),
				"Only MaxRequests (%d) should reach upstream in half-open state", maxRequests)

			// Verify request counts
			assert.Equal(t, maxRequests, successCount,
				"Only %d requests should succeed (MaxRequests)", maxRequests)
			assert.Equal(t, totalRequests-maxRequests, rejectedCount,
				"%d requests should be rejected (ErrTooManyRequests)", totalRequests-maxRequests)

			// Verify rejects metric increased
			rejects := promtest.ToFloat64(m.CircuitBreakerRejects.WithLabelValues(tc.expectProvider, tc.expectEndpoint, tc.expectModel))
			assert.Equal(t, float64(1+totalRequests-maxRequests), rejects,
				"CircuitBreakerRejects should include half-open rejections")
		})
	}
}

// TestCircuitBreaker_PerModelIsolation tests that circuit breakers are independent per model.
// Rate limits on one model should not affect other models on the same endpoint.
func TestCircuitBreaker_PerModelIsolation(t *testing.T) {
	t.Parallel()

	var sonnetCalls, haikuCalls atomic.Int32
	var sonnetShouldFail atomic.Bool
	sonnetShouldFail.Store(true)

	// Mock upstream that returns different responses based on model in request
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("x-should-retry", "false")

		if strings.Contains(string(body), "claude-sonnet-4-20250514") {
			sonnetCalls.Add(1)
			if sonnetShouldFail.Load() {
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(anthropicRateLimitError))
			} else {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(anthropicSuccessResponse("claude-sonnet-4-20250514")))
			}
		} else if strings.Contains(string(body), "claude-3-5-haiku-20241022") {
			haikuCalls.Add(1)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(anthropicSuccessResponse("claude-3-5-haiku-20241022")))
		}
	}))
	defer mockUpstream.Close()

	m := metrics.NewMetrics(prometheus.NewRegistry())

	cbConfig := &config.CircuitBreaker{
		FailureThreshold: 2,
		Interval:         time.Minute,
		Timeout:          500 * time.Millisecond,
		MaxRequests:      1,
	}
	ctx := t.Context()
	bridgeServer := newBridgeTestServer(ctx, t, mockUpstream.URL,
		withCustomProvider(provider.NewAnthropic(config.Anthropic{
			BaseURL:        mockUpstream.URL,
			Key:            "test-key",
			CircuitBreaker: cbConfig,
		}, nil)),
		withMetrics(m),
		withActor("test-user-id", nil),
	)

	doRequest := func(model string) int {
		body := fmt.Sprintf(`{"model":%q,"max_tokens":1024,"messages":[{"role":"user","content":"hi"}]}`, model)
		resp, err := bridgeServer.makeRequest(t, http.MethodPost, pathAnthropicMessages, []byte(body), http.Header{
			"x-api-key":         {"test"},
			"anthropic-version": {"2023-06-01"},
		})
		require.NoError(t, err)
		_, err = io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
		return resp.StatusCode
	}

	// Phase 1: Trip the circuit for sonnet model
	for i := uint32(0); i < cbConfig.FailureThreshold; i++ {
		status := doRequest("claude-sonnet-4-20250514")
		assert.Equal(t, http.StatusTooManyRequests, status)
	}
	//nolint:gosec // G115: test constant, no overflow risk
	assert.Equal(t, int32(cbConfig.FailureThreshold), sonnetCalls.Load())

	// Verify sonnet circuit is open
	status := doRequest("claude-sonnet-4-20250514")
	assert.Equal(t, http.StatusServiceUnavailable, status, "Sonnet circuit should be open")
	//nolint:gosec // G115: test constant, no overflow risk
	assert.Equal(t, int32(cbConfig.FailureThreshold), sonnetCalls.Load(), "No new sonnet calls when circuit is open")

	// Verify sonnet metrics show circuit is open
	sonnetTrips := promtest.ToFloat64(m.CircuitBreakerTrips.WithLabelValues(config.ProviderAnthropic, "/v1/messages", "claude-sonnet-4-20250514"))
	assert.Equal(t, 1.0, sonnetTrips, "Sonnet CircuitBreakerTrips should be 1")

	sonnetState := promtest.ToFloat64(m.CircuitBreakerState.WithLabelValues(config.ProviderAnthropic, "/v1/messages", "claude-sonnet-4-20250514"))
	assert.Equal(t, 1.0, sonnetState, "Sonnet CircuitBreakerState should be 1 (open)")

	// Phase 2: Haiku model should still work (independent circuit)
	status = doRequest("claude-3-5-haiku-20241022")
	assert.Equal(t, http.StatusOK, status, "Haiku should succeed while sonnet circuit is open")
	assert.Equal(t, int32(1), haikuCalls.Load(), "Haiku call should reach upstream")

	// Make multiple haiku requests - all should succeed
	for i := 0; i < 3; i++ {
		status = doRequest("claude-3-5-haiku-20241022")
		assert.Equal(t, http.StatusOK, status, "Haiku should continue to succeed")
	}
	assert.Equal(t, int32(4), haikuCalls.Load(), "All haiku calls should reach upstream")

	// Verify haiku circuit is still closed (no trips)
	haikuTrips := promtest.ToFloat64(m.CircuitBreakerTrips.WithLabelValues(config.ProviderAnthropic, "/v1/messages", "claude-3-5-haiku-20241022"))
	assert.Equal(t, 0.0, haikuTrips, "Haiku CircuitBreakerTrips should be 0")

	haikuState := promtest.ToFloat64(m.CircuitBreakerState.WithLabelValues(config.ProviderAnthropic, "/v1/messages", "claude-3-5-haiku-20241022"))
	assert.Equal(t, 0.0, haikuState, "Haiku CircuitBreakerState should be 0 (closed)")

	// Phase 3: Sonnet recovers after timeout
	time.Sleep(cbConfig.Timeout + 10*time.Millisecond)
	sonnetShouldFail.Store(false)

	status = doRequest("claude-sonnet-4-20250514")
	assert.Equal(t, http.StatusOK, status, "Sonnet should recover after timeout")

	// Verify sonnet circuit is now closed
	sonnetState = promtest.ToFloat64(m.CircuitBreakerState.WithLabelValues(config.ProviderAnthropic, "/v1/messages", "claude-sonnet-4-20250514"))
	assert.Equal(t, 0.0, sonnetState, "Sonnet CircuitBreakerState should be 0 (closed) after recovery")
}

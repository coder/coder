package integrationtest

import (
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/sjson"

	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/aibridge/aibridgetest"
	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/fixtures"
	"github.com/coder/coder/v2/aibridge/internal/testutil"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/provider"
	"github.com/coder/quartz"
)

// TestOpenAI_KeyFailover verifies that a pool's key state
// persists across distinct client requests for both OpenAI APIs
// (chat completions and responses), in both blocking and
// streaming modes. A key marked temporary on request 1 is
// skipped on request 2 without a wasted upstream attempt.
func TestOpenAI_KeyFailover(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		fixture   []byte
		path      string
		streaming bool
	}{
		{
			name:      "chatcompletions_blocking",
			fixture:   fixtures.OaiChatSimple,
			path:      pathOpenAIChatCompletions,
			streaming: false,
		},
		{
			name:      "chatcompletions_streaming",
			fixture:   fixtures.OaiChatSimple,
			path:      pathOpenAIChatCompletions,
			streaming: true,
		},
		{
			name:      "responses_blocking",
			fixture:   fixtures.OaiResponsesBlockingSimple,
			path:      pathOpenAIResponses,
			streaming: false,
		},
		{
			name:      "responses_streaming",
			fixture:   fixtures.OaiResponsesStreamingSimple,
			path:      pathOpenAIResponses,
			streaming: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fix := fixtures.Parse(t, tc.fixture)

			pool, err := keypool.New(config.ProviderOpenAI, []string{"k0", "k1"}, quartz.NewMock(t), nil)
			require.NoError(t, err)

			// Sequential upstream responses: request 1 fails over
			// from k0 to k1 (calls 1-2), and request 2 goes straight
			// to k1 (call 3).
			upstream := testutil.NewMockUpstream(t.Context(), t,
				testutil.NewErrorResponse(http.StatusTooManyRequests, "60"),
				testutil.NewFixtureResponse(fix),
				testutil.NewFixtureResponse(fix),
			)

			bridgeServer := newBridgeTestServer(t.Context(), t, upstream.URL,
				withCustomProvider(provider.NewOpenAI(config.OpenAI{
					BaseURL: upstream.URL,
					KeyPool: pool,
				})),
			)

			requestBody, err := sjson.SetBytes(fix.Request(), "stream", tc.streaming)
			require.NoError(t, err)

			// Request 1: walker starts at k0, fails over to k1
			// after 429.
			resp, err := bridgeServer.makeRequest(t, http.MethodPost, tc.path, requestBody)
			require.NoError(t, err)
			_, _ = io.Copy(io.Discard, resp.Body)
			require.NoError(t, resp.Body.Close())
			require.Equal(t, http.StatusOK, resp.StatusCode)

			// Request 2: walker skips the now-temporary k0 and
			// goes straight to k1 (1 upstream call, not 2).
			resp, err = bridgeServer.makeRequest(t, http.MethodPost, tc.path, requestBody)
			require.NoError(t, err)
			_, _ = io.Copy(io.Discard, resp.Body)
			require.NoError(t, resp.Body.Close())
			require.Equal(t, http.StatusOK, resp.StatusCode)

			var seenKeys []string
			for _, r := range upstream.ReceivedRequests() {
				seenKeys = append(seenKeys, testutil.KeyFromHeader("Authorization", r.Header))
			}
			// Request 1: 2 calls (k0 then k1). Request 2: 1 call (k1).
			assert.Equal(t, []string{"k0", "k1", "k1"}, seenKeys, "seen keys")

			// Pool state persists: k0 temporary, k1 valid.
			assert.Equal(t, []keypool.KeyState{
				keypool.KeyStateTemporary,
				keypool.KeyStateValid,
			}, pool.PoolState(), "key states")
		})
	}
}

// TestAnthropic_KeyFailover verifies that a pool's key state
// persists across distinct client requests: a key marked
// temporary on request 1 is still skipped on request 2 without
// a wasted upstream attempt.
func TestAnthropic_KeyFailover(t *testing.T) {
	t.Parallel()

	fix := fixtures.Parse(t, fixtures.AntSimple)

	tests := []struct {
		name      string
		streaming bool
	}{
		{
			name:      "blocking",
			streaming: false,
		},
		{
			name:      "streaming",
			streaming: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			pool, err := keypool.New(config.ProviderAnthropic, []string{"k0", "k1"}, quartz.NewMock(t), nil)
			require.NoError(t, err)

			// Sequential upstream responses: request 1 fails over
			// from k0 to k1 (calls 1-2), and request 2 goes straight
			// to k1 (call 3).
			upstream := testutil.NewMockUpstream(t.Context(), t,
				testutil.NewErrorResponse(http.StatusTooManyRequests, "60"),
				testutil.NewFixtureResponse(fix),
				testutil.NewFixtureResponse(fix),
			)

			bridgeServer := newBridgeTestServer(t.Context(), t, upstream.URL,
				withCustomProvider(aibridgetest.NewAnthropicProvider(t, config.Anthropic{
					BaseURL: upstream.URL,
					KeyPool: pool,
				}, nil)),
			)

			requestBody, err := sjson.SetBytes(fix.Request(), "stream", tc.streaming)
			require.NoError(t, err)

			// Request 1: walker starts at k0, fails over to k1
			// after 429.
			resp, err := bridgeServer.makeRequest(t, http.MethodPost, pathAnthropicMessages, requestBody)
			require.NoError(t, err)
			_, _ = io.Copy(io.Discard, resp.Body)
			require.NoError(t, resp.Body.Close())
			require.Equal(t, http.StatusOK, resp.StatusCode)

			// Request 2: walker skips the now-temporary k0 and
			// goes straight to k1 (1 upstream call, not 2).
			resp, err = bridgeServer.makeRequest(t, http.MethodPost, pathAnthropicMessages, requestBody)
			require.NoError(t, err)
			_, _ = io.Copy(io.Discard, resp.Body)
			require.NoError(t, resp.Body.Close())
			require.Equal(t, http.StatusOK, resp.StatusCode)

			var seenKeys []string
			for _, r := range upstream.ReceivedRequests() {
				seenKeys = append(seenKeys, testutil.KeyFromHeader("X-Api-Key", r.Header))
			}
			// Request 1: 2 calls (k0 then k1). Request 2: 1 call (k1).
			assert.Equal(t, []string{"k0", "k1", "k1"}, seenKeys, "seen keys")

			// Pool state persists: k0 temporary, k1 valid.
			assert.Equal(t, []keypool.KeyState{
				keypool.KeyStateTemporary,
				keypool.KeyStateValid,
			}, pool.PoolState(), "key states")
		})
	}
}

// TestKeyPool_StateSharing verifies that a key marked unavailable
// by a bridged route is observed in the same state by every other
// route that shares the provider's pool, including other bridged
// routes and passthrough routes. Both paths walk the same
// *keypool.Pool, so state set in one must be visible to all.
func TestKeyPool_StateSharing(t *testing.T) {
	t.Parallel()

	// Parse fixtures once so table rows can reference them.
	fixAnt := fixtures.Parse(t, fixtures.AntSimple)
	fixOaiChat := fixtures.Parse(t, fixtures.OaiChatSimple)
	fixOaiResp := fixtures.Parse(t, fixtures.OaiResponsesBlockingSimple)

	type requestStep struct {
		method string
		path   string
		body   []byte // nil for GET /models passthrough route.
	}

	tests := []struct {
		name              string
		providerName      string
		newProvider       func(baseURL string, pool *keypool.Pool) aibridge.Provider
		upstreamResponses []testutil.UpstreamResponse
		requests          []requestStep
		expectedSeenKeys  []string
	}{
		{
			// Bridged route fails over k0->k1 (calls 1-2), then
			// the passthrough route hits k1 directly (call 3).
			name:         "anthropic",
			providerName: config.ProviderAnthropic,
			newProvider: func(baseURL string, pool *keypool.Pool) aibridge.Provider {
				return aibridgetest.NewAnthropicProvider(t, config.Anthropic{BaseURL: baseURL, KeyPool: pool}, nil)
			},
			upstreamResponses: []testutil.UpstreamResponse{
				testutil.NewErrorResponse(http.StatusTooManyRequests, "60"),
				testutil.NewFixtureResponse(fixAnt),
				{Blocking: []byte("{}")},
			},
			requests: []requestStep{
				{method: http.MethodPost, path: pathAnthropicMessages, body: fixAnt.Request()},
				{method: http.MethodGet, path: "/anthropic/v1/models"},
			},
			expectedSeenKeys: []string{"k0", "k1", "k1"},
		},
		{
			// Bridged chat completions route fails over k0->k1
			// (calls 1-2), bridged responses route hits k1
			// directly (call 3), then the passthrough route hits
			// k1 directly (call 4).
			name:         "openai",
			providerName: config.ProviderOpenAI,
			newProvider: func(baseURL string, pool *keypool.Pool) aibridge.Provider {
				return provider.NewOpenAI(config.OpenAI{BaseURL: baseURL, KeyPool: pool})
			},
			upstreamResponses: []testutil.UpstreamResponse{
				testutil.NewErrorResponse(http.StatusTooManyRequests, "60"),
				testutil.NewFixtureResponse(fixOaiChat),
				testutil.NewFixtureResponse(fixOaiResp),
				{Blocking: []byte("{}")},
			},
			requests: []requestStep{
				{method: http.MethodPost, path: pathOpenAIChatCompletions, body: fixOaiChat.Request()},
				{method: http.MethodPost, path: pathOpenAIResponses, body: fixOaiResp.Request()},
				{method: http.MethodGet, path: "/openai/v1/models"},
			},
			expectedSeenKeys: []string{"k0", "k1", "k1", "k1"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			pool, err := keypool.New(tc.providerName, []string{"k0", "k1"}, quartz.NewMock(t), nil)
			require.NoError(t, err)

			upstream := testutil.NewMockUpstream(t.Context(), t, tc.upstreamResponses...)

			prov := tc.newProvider(upstream.URL, pool)
			bridgeServer := newBridgeTestServer(t.Context(), t, upstream.URL,
				withCustomProvider(prov),
			)

			// Every request returns 200 to the client: the first
			// fails over from k0 (429) to k1 (200) and subsequent
			// requests skip the now-temporary k0 and hit k1
			// directly.
			for _, req := range tc.requests {
				resp, err := bridgeServer.makeRequest(t, req.method, req.path, req.body)
				require.NoError(t, err)
				_, _ = io.Copy(io.Discard, resp.Body)
				require.NoError(t, resp.Body.Close())
				require.Equal(t, http.StatusOK, resp.StatusCode)
			}

			var seenKeys []string
			for _, r := range upstream.ReceivedRequests() {
				seenKeys = append(seenKeys, testutil.KeyFromHeader(prov.AuthHeader(), r.Header))
			}
			assert.Equal(t, tc.expectedSeenKeys, seenKeys, "seen keys")

			// Pool state persists across bridged and passthrough routes.
			assert.Equal(t, []keypool.KeyState{
				keypool.KeyStateTemporary,
				keypool.KeyStateValid,
			}, pool.PoolState(), "key states")
		})
	}
}

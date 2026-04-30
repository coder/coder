package integrationtest //nolint:testpackage // tests unexported internals

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/sjson"

	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/fixtures"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/provider"
	"github.com/coder/quartz"
)

// TestAnthropic_KeyFailover verifies that a pool's key state
// persists across distinct client requests: a key marked
// temporary on request 1 is still skipped on request 2 without
// a wasted upstream attempt.
func TestAnthropic_KeyFailover(t *testing.T) {
	t.Parallel()

	fix := fixtures.Parse(t, fixtures.AntSimple)

	tests := []struct {
		name         string
		streaming    bool
		successBody  []byte
		successCType string
	}{
		{
			name:         "blocking",
			streaming:    false,
			successBody:  fix.NonStreaming(),
			successCType: "application/json",
		},
		{
			name:         "streaming",
			streaming:    true,
			successBody:  fix.Streaming(),
			successCType: "text/event-stream",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			pool, err := keypool.New([]string{"k0", "k1"}, quartz.NewMock(t))
			require.NoError(t, err)

			var requestCount atomic.Int32
			var seenKeysMu sync.Mutex
			var seenKeys []string

			// Mock upstream: k0 always returns 429, k1 returns
			// the per-test success body.
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestCount.Add(1)
				key := r.Header.Get("X-Api-Key")
				seenKeysMu.Lock()
				seenKeys = append(seenKeys, key)
				seenKeysMu.Unlock()
				_, _ = io.Copy(io.Discard, r.Body)

				switch key {
				case "k0":
					w.Header().Set("Content-Type", "application/json")
					w.Header().Set("Retry-After", "60")
					w.WriteHeader(http.StatusTooManyRequests)
					_, _ = fmt.Fprint(w, `{"type":"error","error":{"type":"rate_limit_error","message":"rate limited"}}`)
				case "k1":
					w.Header().Set("Content-Type", tc.successCType)
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write(tc.successBody)
				default:
					w.WriteHeader(http.StatusInternalServerError)
				}
			}))
			t.Cleanup(upstream.Close)

			bridgeServer := newBridgeTestServer(t.Context(), t, upstream.URL,
				withCustomProvider(provider.NewAnthropic(config.Anthropic{
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

			seenKeysMu.Lock()
			defer seenKeysMu.Unlock()
			// Request 1: 2 calls (k0 then k1). Request 2: 1 call (k1).
			assert.Equal(t, int32(3), requestCount.Load(), "upstream request count")
			assert.Equal(t, []string{"k0", "k1", "k1"}, seenKeys, "seen keys")

			// Pool state persists: k0 temporary, k1 valid.
			assert.Equal(t, []keypool.KeyState{
				keypool.KeyStateTemporary,
				keypool.KeyStateValid,
			}, pool.PoolState(), "key states")
		})
	}
}

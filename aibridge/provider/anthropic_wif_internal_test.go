package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	anthropiccfg "github.com/anthropics/anthropic-sdk-go/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/intercept"
	"github.com/coder/quartz"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func okResponse() *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(`{"data":[]}`)),
	}
}

func unauthorizedResponse() *http.Response {
	return &http.Response{
		StatusCode: http.StatusUnauthorized,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(`{"error":"revoked"}`)),
	}
}

// newFakeTokenEndpoint serves POST /v1/oauth/token, minting tok-1, tok-2,
// ... on successive exchanges. The returned counter reports how many
// exchanges were performed.
func newFakeTokenEndpoint(t *testing.T) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var exchanges atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/oauth/token" {
			t.Errorf("unexpected path %q", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "test-jwt", body["assertion"])
		assert.Equal(t, "fdrl_test", body["federation_rule_id"])
		n := exchanges.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "tok-" + string(rune('0'+n)),
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
	}))
	t.Cleanup(srv.Close)
	return srv, &exchanges
}

func newTestWIFSource(t *testing.T, baseURL string) *wifTokenSource {
	t.Helper()
	return newWIFTokenSource(config.AnthropicWIF{
		FederationRuleID: "fdrl_test",
		OrganizationID:   "org-test",
		IdentityToken: func(context.Context) (string, error) {
			return "test-jwt", nil
		},
	}, baseURL)
}

func TestWIFPassthroughTransport(t *testing.T) {
	t.Parallel()

	t.Run("InjectsFederationTokenAndCaches", func(t *testing.T) {
		t.Parallel()
		srv, exchanges := newFakeTokenEndpoint(t)

		var seen []*http.Request
		transport := &wifAuthTransport{
			source: newTestWIFSource(t, srv.URL),
			inner: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				seen = append(seen, r)
				return okResponse(), nil
			}),
		}

		for range 2 {
			req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
			resp, err := transport.RoundTrip(req)
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, resp.StatusCode)
			_ = resp.Body.Close()
		}

		require.Len(t, seen, 2)
		for _, r := range seen {
			assert.Equal(t, "Bearer tok-1", r.Header.Get("Authorization"))
			assert.Equal(t, anthropiccfg.OAuthAPIBetaHeader, r.Header.Get(intercept.AnthropicBetaHeaderName))
		}
		// The second request must reuse the cached token.
		assert.EqualValues(t, 1, exchanges.Load())
	})

	t.Run("AppendsToExistingBetaHeader", func(t *testing.T) {
		t.Parallel()
		srv, _ := newFakeTokenEndpoint(t)

		var seen *http.Request
		transport := &wifAuthTransport{
			source: newTestWIFSource(t, srv.URL),
			inner: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				seen = r
				return okResponse(), nil
			}),
		}

		req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
		req.Header.Set(intercept.AnthropicBetaHeaderName, "token-counting-2024-11-01")
		resp, err := transport.RoundTrip(req)
		require.NoError(t, err)
		_ = resp.Body.Close()

		assert.Equal(t, "token-counting-2024-11-01,"+anthropiccfg.OAuthAPIBetaHeader,
			seen.Header.Get(intercept.AnthropicBetaHeaderName))
		// The caller's request must not be mutated.
		assert.Equal(t, "token-counting-2024-11-01", req.Header.Get(intercept.AnthropicBetaHeaderName))
		assert.Empty(t, req.Header.Get("Authorization"))
	})

	t.Run("SkipsRequestsWithExistingCredentials", func(t *testing.T) {
		t.Parallel()
		srv, exchanges := newFakeTokenEndpoint(t)

		var seen *http.Request
		transport := &wifAuthTransport{
			source: newTestWIFSource(t, srv.URL),
			inner: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				seen = r
				return okResponse(), nil
			}),
		}

		req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
		req.Header.Set("X-Api-Key", "byok-or-pool-key")
		resp, err := transport.RoundTrip(req)
		require.NoError(t, err)
		_ = resp.Body.Close()

		assert.Equal(t, "byok-or-pool-key", seen.Header.Get("X-Api-Key"))
		assert.Empty(t, seen.Header.Get("Authorization"))
		assert.EqualValues(t, 0, exchanges.Load(), "credentialed requests must not trigger an exchange")
	})

	t.Run("RetriesOnceOn401WithFreshToken", func(t *testing.T) {
		t.Parallel()
		srv, exchanges := newFakeTokenEndpoint(t)

		var tokens []string
		var bodies []string
		transport := &wifAuthTransport{
			source: newTestWIFSource(t, srv.URL),
			inner: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				tokens = append(tokens, r.Header.Get("Authorization"))
				b, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				bodies = append(bodies, string(b))
				if len(tokens) == 1 {
					return unauthorizedResponse(), nil
				}
				return okResponse(), nil
			}),
		}

		req := httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", strings.NewReader(`{"model":"claude"}`))
		resp, err := transport.RoundTrip(req)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		_ = resp.Body.Close()

		require.Equal(t, []string{"Bearer tok-1", "Bearer tok-2"}, tokens)
		require.Equal(t, []string{`{"model":"claude"}`, `{"model":"claude"}`}, bodies, "the buffered body must be replayed on retry")
		assert.EqualValues(t, 2, exchanges.Load())
	})

	t.Run("SurfacesOriginal401WhenExchangeRepeatsToken", func(t *testing.T) {
		t.Parallel()
		// A token endpoint that always mints the same token: retrying the
		// upstream request with it is pointless, so the original 401 must
		// be surfaced.
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "tok-static",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		}))
		t.Cleanup(srv.Close)

		var calls int
		transport := &wifAuthTransport{
			source: newTestWIFSource(t, srv.URL),
			inner: roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				return unauthorizedResponse(), nil
			}),
		}

		req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
		resp, err := transport.RoundTrip(req)
		require.NoError(t, err)
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		_ = resp.Body.Close()
		assert.Equal(t, 1, calls, "a repeated token must not be retried")
	})
}

func TestWIFTokenSource_RefreshMargin(t *testing.T) {
	t.Parallel()

	// Each subtest gets its own token endpoint and source so they can
	// run in parallel without sharing cache state.
	newSource := func(t *testing.T) (*wifTokenSource, *atomic.Int64) {
		t.Helper()
		srv, exchanges := newFakeTokenEndpoint(t)
		source := newTestWIFSource(t, srv.URL)
		source.clock = quartz.NewMock(t)
		return source, exchanges
	}

	ctx := context.Background()

	t.Run("WithinMarginReExchanges", func(t *testing.T) {
		t.Parallel()
		source, exchanges := newSource(t)
		source.token = "tok-stale"
		source.expiresAt = source.clock.Now().Add(wifTokenRefreshMargin / 2)
		tok, err := source.Token(ctx)
		require.NoError(t, err)
		assert.NotEqual(t, "tok-stale", tok)
		assert.EqualValues(t, 1, exchanges.Load())
	})

	t.Run("OutsideMarginUsesCache", func(t *testing.T) {
		t.Parallel()
		source, exchanges := newSource(t)
		source.token = "tok-fresh"
		source.expiresAt = source.clock.Now().Add(2 * wifTokenRefreshMargin)
		tok, err := source.Token(ctx)
		require.NoError(t, err)
		assert.Equal(t, "tok-fresh", tok)
		assert.EqualValues(t, 0, exchanges.Load(), "a fresh token must not trigger an exchange")
	})

	t.Run("NoExpiryCachesUntilInvalidated", func(t *testing.T) {
		t.Parallel()
		source, _ := newSource(t)
		source.token = "tok-forever"
		source.expiresAt = time.Time{}
		tok, err := source.Token(ctx)
		require.NoError(t, err)
		assert.Equal(t, "tok-forever", tok)

		source.invalidate("tok-forever")
		tok, err = source.Token(ctx)
		require.NoError(t, err)
		assert.NotEqual(t, "tok-forever", tok)
	})

	t.Run("InvalidateIgnoresSupersededToken", func(t *testing.T) {
		t.Parallel()
		source, _ := newSource(t)
		source.token = "tok-current"
		source.expiresAt = time.Time{}
		source.invalidate("tok-older")
		assert.Equal(t, "tok-current", source.token)
	})
}

func TestAnthropic_WrapPassthroughTransport(t *testing.T) {
	t.Parallel()

	inner := roundTripFunc(func(*http.Request) (*http.Response, error) { return okResponse(), nil })

	t.Run("NonWIFReturnsInnerUnchanged", func(t *testing.T) {
		t.Parallel()
		p := newTestAnthropic(t, config.Anthropic{}, nil)
		wrapped := p.WrapPassthroughTransport(inner)
		assert.IsType(t, roundTripFunc(nil), wrapped)
	})

	t.Run("WIFWrapsTransport", func(t *testing.T) {
		t.Parallel()
		p := newTestAnthropic(t, config.Anthropic{
			WIF: &config.AnthropicWIF{
				FederationRuleID: "fdrl_test",
				OrganizationID:   "org-test",
				IdentityToken: func(context.Context) (string, error) {
					return "test-jwt", nil
				},
			},
		}, nil)
		wrapped := p.WrapPassthroughTransport(inner)
		assert.IsType(t, &wifAuthTransport{}, wrapped)
	})
}

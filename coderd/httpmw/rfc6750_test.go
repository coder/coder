package httpmw_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// TestRFC6750BearerTokenAuthentication tests that RFC 6750 bearer tokens work correctly
// for authentication, including both Authorization header and access_token query parameter methods.
//
//nolint:tparallel,paralleltest // Subtests share a DB; run sequentially to avoid Windows DB cleanup flake.
func TestRFC6750BearerTokenAuthentication(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)

	// Create a test user and API key
	user := dbgen.User(t, db, database.User{})

	// Create an OAuth2 provider app token (which should work with bearer token authentication)
	key, token := dbgen.APIKey(t, db, database.APIKey{
		UserID:    user.ID,
		ExpiresAt: dbtime.Now().Add(testutil.WaitLong),
	})

	cfg := httpmw.ExtractAPIKeyConfig{
		DB: db,
	}

	testHandler := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		apiKey := httpmw.APIKey(r)
		require.Equal(t, key.ID, apiKey.ID)
		rw.WriteHeader(http.StatusOK)
	})

	t.Run("AuthorizationBearerHeader", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		rw := httptest.NewRecorder()

		httpmw.ExtractAPIKeyMW(cfg)(testHandler).ServeHTTP(rw, req)

		require.Equal(t, http.StatusOK, rw.Code)
	})

	t.Run("AccessTokenQueryParameter", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test?access_token="+url.QueryEscape(token), nil)

		rw := httptest.NewRecorder()

		httpmw.ExtractAPIKeyMW(cfg)(testHandler).ServeHTTP(rw, req)

		require.Equal(t, http.StatusOK, rw.Code)
	})

	t.Run("BearerTokenPriorityAfterCustomMethods", func(t *testing.T) {
		// Create a different token for custom header
		customKey, customToken := dbgen.APIKey(t, db, database.APIKey{
			UserID:    user.ID,
			ExpiresAt: dbtime.Now().Add(testutil.WaitLong),
		})

		// Create handler that checks which token was used
		priorityHandler := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			apiKey := httpmw.APIKey(r)
			// Should use the custom header token, not the bearer token
			require.Equal(t, customKey.ID, apiKey.ID)
			rw.WriteHeader(http.StatusOK)
		})

		req := httptest.NewRequest("GET", "/test", nil)
		// Set both custom header and bearer header - custom should win
		req.Header.Set(codersdk.SessionTokenHeader, customToken)
		req.Header.Set("Authorization", "Bearer "+token)

		rw := httptest.NewRecorder()

		httpmw.ExtractAPIKeyMW(cfg)(priorityHandler).ServeHTTP(rw, req)

		require.Equal(t, http.StatusOK, rw.Code)
	})

	t.Run("InvalidBearerToken", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", "Bearer invalid-token")

		rw := httptest.NewRecorder()

		httpmw.ExtractAPIKeyMW(cfg)(testHandler).ServeHTTP(rw, req)

		require.Equal(t, http.StatusUnauthorized, rw.Code)

		// Check that WWW-Authenticate header is present
		wwwAuth := rw.Header().Get("WWW-Authenticate")
		require.NotEmpty(t, wwwAuth)
		require.Contains(t, wwwAuth, "Bearer")
		require.Contains(t, wwwAuth, `realm="coder"`)
		require.Contains(t, wwwAuth, "invalid_token")
	})

	t.Run("ExpiredBearerToken", func(t *testing.T) {
		// Create an expired token
		_, expiredToken := dbgen.APIKey(t, db, database.APIKey{
			UserID:    user.ID,
			ExpiresAt: dbtime.Now().Add(-testutil.WaitShort), // Expired
		})

		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", "Bearer "+expiredToken)

		rw := httptest.NewRecorder()

		httpmw.ExtractAPIKeyMW(cfg)(testHandler).ServeHTTP(rw, req)

		require.Equal(t, http.StatusUnauthorized, rw.Code)

		// Check that WWW-Authenticate header contains expired error
		wwwAuth := rw.Header().Get("WWW-Authenticate")
		require.NotEmpty(t, wwwAuth)
		require.Contains(t, wwwAuth, "Bearer")
		require.Contains(t, wwwAuth, `realm="coder"`)
		require.Contains(t, wwwAuth, "expired")
	})

	t.Run("MissingBearerToken", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		// No authentication provided

		rw := httptest.NewRecorder()

		httpmw.ExtractAPIKeyMW(cfg)(testHandler).ServeHTTP(rw, req)

		require.Equal(t, http.StatusUnauthorized, rw.Code)

		// Check that WWW-Authenticate header is present
		wwwAuth := rw.Header().Get("WWW-Authenticate")
		require.NotEmpty(t, wwwAuth)
		require.Contains(t, wwwAuth, "Bearer")
		require.Contains(t, wwwAuth, `realm="coder"`)
	})
}

// TestAPITokenFromRequest tests the RFC 6750 bearer token extraction directly
func TestAPITokenFromRequest(t *testing.T) {
	t.Parallel()

	token := "test-token-value"
	customToken := "custom-token"
	cookieToken := "cookie-token"

	tests := []struct {
		name     string
		setupReq func(*http.Request)
		expected string
	}{
		{
			name: "AuthorizationBearerHeader",
			setupReq: func(req *http.Request) {
				req.Header.Set("Authorization", "Bearer "+token)
			},
			expected: token,
		},
		{
			name: "AccessTokenQueryParameter",
			setupReq: func(req *http.Request) {
				q := req.URL.Query()
				q.Set("access_token", token)
				req.URL.RawQuery = q.Encode()
			},
			expected: token,
		},
		{
			name: "CustomMethodsPriorityOverBearer",
			setupReq: func(req *http.Request) {
				req.Header.Set(codersdk.SessionTokenHeader, customToken)
				req.Header.Set("Authorization", "Bearer "+token)
			},
			expected: customToken,
		},
		{
			name: "CookiePriorityOverBearer",
			setupReq: func(req *http.Request) {
				req.AddCookie(&http.Cookie{
					Name:  codersdk.SessionTokenCookie,
					Value: cookieToken,
				})
				req.Header.Set("Authorization", "Bearer "+token)
			},
			expected: cookieToken,
		},
		{
			name: "NoTokenReturnsEmpty",
			setupReq: func(req *http.Request) {
				// No authentication provided
			},
			expected: "",
		},
		{
			name: "InvalidAuthorizationHeaderIgnored",
			setupReq: func(req *http.Request) {
				req.Header.Set("Authorization", "Basic dXNlcjpwYXNz") // Basic auth, not Bearer
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest("GET", "/test", nil)
			tt.setupReq(req)

			extractedToken := httpmw.APITokenFromRequest(req)
			require.Equal(t, tt.expected, extractedToken)
		})
	}
}

//nolint:tparallel,paralleltest // Subtests share a DB; run sequentially to avoid Windows DB cleanup flake.
func TestOAuth2ProviderTokenInQueryString(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)

	user := dbgen.User(t, db, database.User{})
	app := dbgen.OAuth2ProviderApp(t, db, database.OAuth2ProviderApp{})
	oauthKey, oauthToken := dbgen.APIKey(t, db, database.APIKey{
		UserID:    user.ID,
		LoginType: database.LoginTypeOAuth2ProviderApp,
		ExpiresAt: dbtime.Now().Add(testutil.WaitLong),
		// Old enough that an accepted request would update it.
		LastUsed: dbtime.Now().Add(-2 * time.Hour),
	})
	dbgen.OAuth2ProviderAppToken(t, db, database.OAuth2ProviderAppToken{
		AppID:    app.ID,
		APIKeyID: oauthKey.ID,
		UserID:   user.ID,
	})
	sessionKey, sessionToken := dbgen.APIKey(t, db, database.APIKey{
		UserID:    user.ID,
		ExpiresAt: dbtime.Now().Add(testutil.WaitLong),
	})

	cfg := httpmw.ExtractAPIKeyConfig{DB: db}

	// withSink gives a subtest its own logger so it can assert on the
	// refusal warning without seeing entries from sibling subtests.
	withSink := func(t *testing.T) (httpmw.ExtractAPIKeyConfig, *testutil.FakeSink) {
		sink := testutil.NewFakeSink(t)
		sinkCfg := cfg
		sinkCfg.Logger = sink.Logger()
		return sinkCfg, sink
	}
	warnings := func(sink *testutil.FakeSink) []slog.SinkEntry {
		return sink.Entries(func(e slog.SinkEntry) bool { return e.Level == slog.LevelWarn })
	}
	fieldValue := func(e slog.SinkEntry, name string) any {
		for _, f := range e.Fields {
			if f.Name == name {
				return f.Value
			}
		}
		return nil
	}

	handlerExpecting := func(want database.APIKey) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			require.Equal(t, want.ID, httpmw.APIKey(r).ID)
			rw.WriteHeader(http.StatusOK)
		})
	}

	requireIgnored := func(t *testing.T, rw *httptest.ResponseRecorder) {
		require.Equal(t, http.StatusUnauthorized, rw.Code)
		wwwAuth := rw.Header().Get("WWW-Authenticate")
		require.True(t, strings.HasPrefix(wwwAuth, `Bearer realm="coder"`), wwwAuth)
		require.NotContains(t, wwwAuth, "error=")
		var resp codersdk.Response
		require.NoError(t, json.Unmarshal(rw.Body.Bytes(), &resp))
		require.Equal(t, "OAuth2 access token in the URL query string was ignored.", resp.Message)
		require.Contains(t, resp.Detail, "Authorization header")
	}

	t.Run("AccessTokenQuery", func(t *testing.T) {
		sinkCfg, sink := withSink(t)
		req := httptest.NewRequest("GET", "/test?access_token="+url.QueryEscape(oauthToken), nil)
		rw := httptest.NewRecorder()
		httpmw.ExtractAPIKeyMW(sinkCfg)(handlerExpecting(oauthKey)).ServeHTTP(rw, req)
		requireIgnored(t, rw)

		ctx := testutil.Context(t, testutil.WaitShort)
		key, err := db.GetAPIKeyByID(ctx, oauthKey.ID)
		require.NoError(t, err)
		require.Equal(t, oauthKey.LastUsed, key.LastUsed)
		require.Equal(t, oauthKey.ExpiresAt, key.ExpiresAt)

		warns := warnings(sink)
		require.Len(t, warns, 1)
		require.Equal(t, "oauth2 access token ignored: sent in the URL query string", warns[0].Message)
		require.Equal(t, oauthKey.ID, fieldValue(warns[0], "api_key_id"))
		require.Equal(t, user.ID, fieldValue(warns[0], "user_id"))
		require.Equal(t, app.ID, fieldValue(warns[0], "app_id"))
	})

	t.Run("SessionTokenQuery", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test?"+codersdk.SessionTokenCookie+"="+url.QueryEscape(oauthToken), nil)
		rw := httptest.NewRecorder()
		httpmw.ExtractAPIKeyMW(cfg)(handlerExpecting(oauthKey)).ServeHTTP(rw, req)
		requireIgnored(t, rw)
	})

	t.Run("PaddedAccessTokenQuery", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test?access_token="+url.QueryEscape(" "+oauthToken+" "), nil)
		rw := httptest.NewRecorder()
		httpmw.ExtractAPIKeyMW(cfg)(handlerExpecting(oauthKey)).ServeHTTP(rw, req)
		requireIgnored(t, rw)
	})

	t.Run("BearerHeader", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", "Bearer "+oauthToken)
		rw := httptest.NewRecorder()
		httpmw.ExtractAPIKeyMW(cfg)(handlerExpecting(oauthKey)).ServeHTTP(rw, req)
		require.Equal(t, http.StatusOK, rw.Code)
	})

	t.Run("SessionTokenHeader", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set(codersdk.SessionTokenHeader, oauthToken)
		rw := httptest.NewRecorder()
		httpmw.ExtractAPIKeyMW(cfg)(handlerExpecting(oauthKey)).ServeHTTP(rw, req)
		require.Equal(t, http.StatusOK, rw.Code)
	})

	t.Run("QueryAndBearerHeader", func(t *testing.T) {
		sinkCfg, sink := withSink(t)
		req := httptest.NewRequest("GET", "/test?access_token="+url.QueryEscape(oauthToken), nil)
		req.Header.Set("Authorization", "Bearer "+oauthToken)
		rw := httptest.NewRecorder()
		httpmw.ExtractAPIKeyMW(sinkCfg)(handlerExpecting(oauthKey)).ServeHTTP(rw, req)
		require.Equal(t, http.StatusOK, rw.Code)
		require.Empty(t, warnings(sink))
	})

	t.Run("QueryAndSessionTokenHeader", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test?"+codersdk.SessionTokenCookie+"="+url.QueryEscape(oauthToken), nil)
		req.Header.Set(codersdk.SessionTokenHeader, oauthToken)
		rw := httptest.NewRecorder()
		httpmw.ExtractAPIKeyMW(cfg)(handlerExpecting(oauthKey)).ServeHTTP(rw, req)
		require.Equal(t, http.StatusOK, rw.Code)
	})

	t.Run("QueryAndCookie", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test?access_token="+url.QueryEscape(oauthToken), nil)
		req.AddCookie(&http.Cookie{Name: codersdk.SessionTokenCookie, Value: oauthToken})
		rw := httptest.NewRecorder()
		httpmw.ExtractAPIKeyMW(cfg)(handlerExpecting(oauthKey)).ServeHTTP(rw, req)
		require.Equal(t, http.StatusOK, rw.Code)
	})

	t.Run("FirstPartySessionTokenQuery", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test?"+codersdk.SessionTokenCookie+"="+url.QueryEscape(sessionToken), nil)
		rw := httptest.NewRecorder()
		httpmw.ExtractAPIKeyMW(cfg)(handlerExpecting(sessionKey)).ServeHTTP(rw, req)
		require.Equal(t, http.StatusOK, rw.Code)
	})

	t.Run("OptionalRoute", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test?access_token="+url.QueryEscape(oauthToken), nil)
		rw := httptest.NewRecorder()
		optionalCfg := cfg
		optionalCfg.Optional = true
		httpmw.ExtractAPIKeyMW(optionalCfg)(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			_, ok := httpmw.APIKeyOptional(r)
			require.False(t, ok)
			rw.WriteHeader(http.StatusOK)
		})).ServeHTTP(rw, req)
		require.Equal(t, http.StatusOK, rw.Code)
	})

	t.Run("CustomSessionTokenFunc", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test?"+codersdk.SessionTokenCookie+"="+url.QueryEscape(oauthToken), nil)
		rw := httptest.NewRecorder()
		customCfg := cfg
		customCfg.SessionTokenFunc = func(r *http.Request) string {
			return r.URL.Query().Get(codersdk.SessionTokenCookie)
		}
		httpmw.ExtractAPIKeyMW(customCfg)(handlerExpecting(oauthKey)).ServeHTTP(rw, req)
		requireIgnored(t, rw)
	})
}

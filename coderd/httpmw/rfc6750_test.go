package httpmw_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

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

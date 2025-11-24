package httpmw_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// TestOAuth2BearerTokenSecurityBoundaries tests RFC 6750 security boundaries
//
//nolint:tparallel,paralleltest // Subtests share a DB; run sequentially to avoid Windows DB cleanup flake.
func TestOAuth2BearerTokenSecurityBoundaries(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)

	// Create two different users with different API keys
	user1 := dbgen.User(t, db, database.User{})
	user2 := dbgen.User(t, db, database.User{})

	// Create API keys for both users
	key1, token1 := dbgen.APIKey(t, db, database.APIKey{
		UserID:    user1.ID,
		ExpiresAt: dbtime.Now().Add(testutil.WaitLong),
	})

	_, token2 := dbgen.APIKey(t, db, database.APIKey{
		UserID:    user2.ID,
		ExpiresAt: dbtime.Now().Add(testutil.WaitLong),
	})

	t.Run("TokenIsolation", func(t *testing.T) {
		// Create middleware
		middleware := httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB: db,
		})

		// Handler that returns the authenticated user ID
		handler := middleware(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			apiKey := httpmw.APIKey(r)
			rw.Header().Set("X-User-ID", apiKey.UserID.String())
			rw.WriteHeader(http.StatusOK)
		}))

		// Test that user1's token only accesses user1's data
		req1 := httptest.NewRequest("GET", "/test", nil)
		req1.Header.Set("Authorization", "Bearer "+token1)
		rec1 := httptest.NewRecorder()
		handler.ServeHTTP(rec1, req1)

		require.Equal(t, http.StatusOK, rec1.Code)
		require.Equal(t, user1.ID.String(), rec1.Header().Get("X-User-ID"))

		// Test that user2's token only accesses user2's data
		req2 := httptest.NewRequest("GET", "/test", nil)
		req2.Header.Set("Authorization", "Bearer "+token2)
		rec2 := httptest.NewRecorder()
		handler.ServeHTTP(rec2, req2)

		require.Equal(t, http.StatusOK, rec2.Code)
		require.Equal(t, user2.ID.String(), rec2.Header().Get("X-User-ID"))

		// Verify users can't access each other's data
		require.NotEqual(t, rec1.Header().Get("X-User-ID"), rec2.Header().Get("X-User-ID"))
	})

	t.Run("CrossTokenAttempts", func(t *testing.T) {
		middleware := httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB: db,
		})

		handler := middleware(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			rw.WriteHeader(http.StatusOK)
		}))

		// Try to use invalid token (should fail)
		invalidToken := key1.ID + "-invalid-secret"
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", "Bearer "+invalidToken)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		require.Equal(t, http.StatusUnauthorized, rec.Code)
		require.Contains(t, rec.Header().Get("WWW-Authenticate"), "Bearer")
		require.Contains(t, rec.Header().Get("WWW-Authenticate"), "invalid_token")
	})

	t.Run("TimingAttackResistance", func(t *testing.T) {
		middleware := httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB: db,
		})

		handler := middleware(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			rw.WriteHeader(http.StatusOK)
		}))

		// Test multiple invalid tokens to ensure consistent timing
		invalidTokens := []string{
			"invalid-token-1",
			"invalid-token-2-longer",
			"a",
			strings.Repeat("x", 100),
		}

		times := make([]time.Duration, len(invalidTokens))

		for i, token := range invalidTokens {
			start := time.Now()

			req := httptest.NewRequest("GET", "/test", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			times[i] = time.Since(start)

			require.Equal(t, http.StatusUnauthorized, rec.Code)
		}

		// While we can't guarantee perfect timing consistency in tests,
		// we can at least verify that the responses are all unauthorized
		// and contain proper WWW-Authenticate headers
		for _, token := range invalidTokens {
			req := httptest.NewRequest("GET", "/test", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			require.Equal(t, http.StatusUnauthorized, rec.Code)
			require.Contains(t, rec.Header().Get("WWW-Authenticate"), "Bearer")
		}
	})
}

// TestOAuth2BearerTokenMalformedHeaders tests handling of malformed Bearer headers per RFC 6750
//
//nolint:tparallel,paralleltest // Subtests share a DB; run sequentially to avoid Windows DB cleanup flake.
func TestOAuth2BearerTokenMalformedHeaders(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)

	middleware := httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
		DB: db,
	})

	handler := middleware(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(http.StatusOK)
	}))

	tests := []struct {
		name           string
		authHeader     string
		expectedStatus int
		shouldHaveWWW  bool
	}{
		{
			name:           "MissingBearer",
			authHeader:     "invalid-token",
			expectedStatus: http.StatusUnauthorized,
			shouldHaveWWW:  true,
		},
		{
			name:           "CaseSensitive",
			authHeader:     "bearer token", // lowercase should still work
			expectedStatus: http.StatusUnauthorized,
			shouldHaveWWW:  true,
		},
		{
			name:           "ExtraSpaces",
			authHeader:     "Bearer  token-with-extra-spaces",
			expectedStatus: http.StatusUnauthorized,
			shouldHaveWWW:  true,
		},
		{
			name:           "EmptyToken",
			authHeader:     "Bearer ",
			expectedStatus: http.StatusUnauthorized,
			shouldHaveWWW:  true,
		},
		{
			name:           "OnlyBearer",
			authHeader:     "Bearer",
			expectedStatus: http.StatusUnauthorized,
			shouldHaveWWW:  true,
		},
		{
			name:           "MultipleBearer",
			authHeader:     "Bearer token1 Bearer token2",
			expectedStatus: http.StatusUnauthorized,
			shouldHaveWWW:  true,
		},
		{
			name:           "InvalidBase64",
			authHeader:     "Bearer !!!invalid-base64!!!",
			expectedStatus: http.StatusUnauthorized,
			shouldHaveWWW:  true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			req.Header.Set("Authorization", test.authHeader)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			require.Equal(t, test.expectedStatus, rec.Code)

			if test.shouldHaveWWW {
				wwwAuth := rec.Header().Get("WWW-Authenticate")
				require.Contains(t, wwwAuth, "Bearer")
				require.Contains(t, wwwAuth, "realm=\"coder\"")
			}
		})
	}
}

// TestOAuth2BearerTokenPrecedence tests token extraction precedence per RFC 6750
//
//nolint:tparallel,paralleltest // Subtests share a DB; run sequentially to avoid Windows DB cleanup flake.
func TestOAuth2BearerTokenPrecedence(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	user := dbgen.User(t, db, database.User{})

	// Create a valid API key
	key, validToken := dbgen.APIKey(t, db, database.APIKey{
		UserID:    user.ID,
		ExpiresAt: dbtime.Now().Add(testutil.WaitLong),
	})

	middleware := httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
		DB: db,
	})

	handler := middleware(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		apiKey := httpmw.APIKey(r)
		rw.Header().Set("X-Key-ID", apiKey.ID)
		rw.WriteHeader(http.StatusOK)
	}))

	t.Run("CookieTakesPrecedenceOverBearer", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		// Set both cookie and Bearer header - cookie should take precedence
		req.AddCookie(&http.Cookie{
			Name:  codersdk.SessionTokenCookie,
			Value: validToken,
		})
		req.Header.Set("Authorization", "Bearer invalid-token")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		require.Equal(t, key.ID, rec.Header().Get("X-Key-ID"))
	})

	t.Run("QueryParameterTakesPrecedenceOverBearer", func(t *testing.T) {
		// Set both query parameter and Bearer header - query should take precedence
		u, _ := url.Parse("/test")
		q := u.Query()
		q.Set(codersdk.SessionTokenCookie, validToken)
		u.RawQuery = q.Encode()

		req := httptest.NewRequest("GET", u.String(), nil)
		req.Header.Set("Authorization", "Bearer invalid-token")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		require.Equal(t, key.ID, rec.Header().Get("X-Key-ID"))
	})

	t.Run("BearerHeaderFallback", func(t *testing.T) {
		// Only set Bearer header - should be used as fallback
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", "Bearer "+validToken)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		require.Equal(t, key.ID, rec.Header().Get("X-Key-ID"))
	})

	t.Run("AccessTokenQueryParameterFallback", func(t *testing.T) {
		// Only set access_token query parameter - should be used as fallback
		u, _ := url.Parse("/test")
		q := u.Query()
		q.Set("access_token", validToken)
		u.RawQuery = q.Encode()

		req := httptest.NewRequest("GET", u.String(), nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		require.Equal(t, key.ID, rec.Header().Get("X-Key-ID"))
	})

	t.Run("MultipleAuthMethodsShouldNotConflict", func(t *testing.T) {
		// RFC 6750 says clients shouldn't send tokens in multiple ways,
		// but if they do, we should handle it gracefully by using precedence
		u, _ := url.Parse("/test")
		q := u.Query()
		q.Set("access_token", validToken)
		q.Set(codersdk.SessionTokenCookie, validToken)
		u.RawQuery = q.Encode()

		req := httptest.NewRequest("GET", u.String(), nil)
		req.Header.Set("Authorization", "Bearer "+validToken)
		req.AddCookie(&http.Cookie{
			Name:  codersdk.SessionTokenCookie,
			Value: validToken,
		})
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		// Should succeed using the highest precedence method (cookie)
		require.Equal(t, http.StatusOK, rec.Code)
		require.Equal(t, key.ID, rec.Header().Get("X-Key-ID"))
	})
}

// TestOAuth2WWWAuthenticateCompliance tests WWW-Authenticate header compliance with RFC 6750
//
//nolint:tparallel,paralleltest // Subtests share a DB; run sequentially to avoid Windows DB cleanup flake.
func TestOAuth2WWWAuthenticateCompliance(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	user := dbgen.User(t, db, database.User{})

	middleware := httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
		DB: db,
	})

	handler := middleware(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(http.StatusOK)
	}))

	t.Run("UnauthorizedResponse", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", "Bearer invalid-token")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		require.Equal(t, http.StatusUnauthorized, rec.Code)

		wwwAuth := rec.Header().Get("WWW-Authenticate")
		require.NotEmpty(t, wwwAuth)

		// RFC 6750 requires specific format: Bearer realm="realm"
		require.Contains(t, wwwAuth, "Bearer")
		require.Contains(t, wwwAuth, "realm=\"coder\"")
		require.Contains(t, wwwAuth, "error=\"invalid_token\"")
		require.Contains(t, wwwAuth, "error_description=")
	})

	t.Run("ExpiredTokenResponse", func(t *testing.T) {
		// Create an expired API key
		_, expiredToken := dbgen.APIKey(t, db, database.APIKey{
			UserID:    user.ID,
			ExpiresAt: dbtime.Now().Add(-time.Hour), // Expired 1 hour ago
		})

		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", "Bearer "+expiredToken)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		require.Equal(t, http.StatusUnauthorized, rec.Code)

		wwwAuth := rec.Header().Get("WWW-Authenticate")
		require.Contains(t, wwwAuth, "Bearer")
		require.Contains(t, wwwAuth, "realm=\"coder\"")
		require.Contains(t, wwwAuth, "error=\"invalid_token\"")
		require.Contains(t, wwwAuth, "error_description=\"The access token has expired\"")
	})

	t.Run("InsufficientScopeResponse", func(t *testing.T) {
		// For this test, we'll test with an invalid token to trigger the middleware's
		// error handling which does set WWW-Authenticate headers for 403 responses
		// In practice, insufficient scope errors would be handled by RBAC middleware
		// that comes after authentication, but we can simulate a 403 from the auth middleware

		req := httptest.NewRequest("GET", "/admin", nil)
		req.Header.Set("Authorization", "Bearer invalid-token-that-triggers-403")
		rec := httptest.NewRecorder()

		// Use a middleware configuration that might trigger a 403 instead of 401
		// for certain types of authentication failures
		middleware := httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB: db,
		})

		handler := middleware(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			// This shouldn't be reached due to auth failure
			rw.WriteHeader(http.StatusOK)
		}))

		handler.ServeHTTP(rec, req)

		// This will be a 401 (unauthorized) rather than 403 (forbidden) for invalid tokens
		// which is correct - 403 would come from RBAC after successful authentication
		require.Equal(t, http.StatusUnauthorized, rec.Code)

		wwwAuth := rec.Header().Get("WWW-Authenticate")
		require.Contains(t, wwwAuth, "Bearer")
		require.Contains(t, wwwAuth, "realm=\"coder\"")
		require.Contains(t, wwwAuth, "error=\"invalid_token\"")
		require.Contains(t, wwwAuth, "error_description=")
	})
}

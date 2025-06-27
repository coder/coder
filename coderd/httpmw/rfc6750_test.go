package httpmw_test

import (
	"context"
	"crypto/sha256"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// defaultIPAddressForTests returns a default IP address for test API keys
func defaultIPAddressForTests() pqtype.Inet {
	return pqtype.Inet{
		IPNet: net.IPNet{
			IP:   net.IPv4(127, 0, 0, 1),
			Mask: net.IPv4Mask(255, 255, 255, 255),
		},
		Valid: true,
	}
}

// TestRFC6750BearerTokenAuthentication tests that RFC 6750 bearer tokens work correctly
// for authentication, including both Authorization header and access_token query parameter methods.
func TestRFC6750BearerTokenAuthentication(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	t.Cleanup(cancel)

	db, _ := dbtestutil.NewDB(t)

	// Create a test user and API key
	user := dbgen.User(t, db, database.User{})

	// Create an OAuth2 provider app token (which should work with bearer token authentication)
	keyID, keySecret := randomAPIKeyParts()
	hashedSecret := sha256.Sum256([]byte(keySecret))

	key, err := db.InsertAPIKey(ctx, database.InsertAPIKeyParams{
		ID:              keyID,
		UserID:          user.ID,
		HashedSecret:    hashedSecret[:],
		IPAddress:       defaultIPAddressForTests(),
		LastUsed:        dbtime.Now(),
		ExpiresAt:       dbtime.Now().Add(testutil.WaitLong),
		CreatedAt:       dbtime.Now(),
		UpdatedAt:       dbtime.Now(),
		LoginType:       database.LoginTypePassword,
		Scope:           database.APIKeyScopeAll,
		LifetimeSeconds: int64(testutil.WaitLong.Seconds()),
	})
	require.NoError(t, err)

	token := keyID + "-" + keySecret

	cfg := httpmw.ExtractAPIKeyConfig{
		DB: db,
	}

	testHandler := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		apiKey := httpmw.APIKey(r)
		require.Equal(t, key.ID, apiKey.ID)
		rw.WriteHeader(http.StatusOK)
	})

	t.Run("AuthorizationBearerHeader", func(t *testing.T) {
		t.Parallel()

		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		rw := httptest.NewRecorder()

		httpmw.ExtractAPIKeyMW(cfg)(testHandler).ServeHTTP(rw, req)

		require.Equal(t, http.StatusOK, rw.Code)
	})

	t.Run("AccessTokenQueryParameter", func(t *testing.T) {
		t.Parallel()

		req := httptest.NewRequest("GET", "/test?access_token="+url.QueryEscape(token), nil)

		rw := httptest.NewRecorder()

		httpmw.ExtractAPIKeyMW(cfg)(testHandler).ServeHTTP(rw, req)

		require.Equal(t, http.StatusOK, rw.Code)
	})

	t.Run("BearerTokenPriorityAfterCustomMethods", func(t *testing.T) {
		t.Parallel()

		// Create a different token for custom header
		customKeyID, customKeySecret := randomAPIKeyParts()
		customHashedSecret := sha256.Sum256([]byte(customKeySecret))

		customKey, err := db.InsertAPIKey(ctx, database.InsertAPIKeyParams{
			ID:              customKeyID,
			UserID:          user.ID,
			HashedSecret:    customHashedSecret[:],
			IPAddress:       defaultIPAddressForTests(),
			LastUsed:        dbtime.Now(),
			ExpiresAt:       dbtime.Now().Add(testutil.WaitLong),
			CreatedAt:       dbtime.Now(),
			UpdatedAt:       dbtime.Now(),
			LoginType:       database.LoginTypePassword,
			Scope:           database.APIKeyScopeAll,
			LifetimeSeconds: int64(testutil.WaitLong.Seconds()),
		})
		require.NoError(t, err)

		customToken := customKeyID + "-" + customKeySecret

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
		t.Parallel()

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
		t.Parallel()

		// Create an expired token
		expiredKeyID, expiredKeySecret := randomAPIKeyParts()
		expiredHashedSecret := sha256.Sum256([]byte(expiredKeySecret))

		_, err := db.InsertAPIKey(ctx, database.InsertAPIKeyParams{
			ID:              expiredKeyID,
			UserID:          user.ID,
			HashedSecret:    expiredHashedSecret[:],
			IPAddress:       defaultIPAddressForTests(),
			LastUsed:        dbtime.Now(),
			ExpiresAt:       dbtime.Now().Add(-testutil.WaitShort), // Expired
			CreatedAt:       dbtime.Now(),
			UpdatedAt:       dbtime.Now(),
			LoginType:       database.LoginTypePassword,
			Scope:           database.APIKeyScopeAll,
			LifetimeSeconds: int64(testutil.WaitLong.Seconds()),
		})
		require.NoError(t, err)

		expiredToken := expiredKeyID + "-" + expiredKeySecret

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
		t.Parallel()

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

	t.Run("AuthorizationBearerHeader", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		extractedToken := httpmw.APITokenFromRequest(req)
		require.Equal(t, token, extractedToken)
	})

	t.Run("AccessTokenQueryParameter", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest("GET", "/test?access_token="+url.QueryEscape(token), nil)

		extractedToken := httpmw.APITokenFromRequest(req)
		require.Equal(t, token, extractedToken)
	})

	t.Run("CustomMethodsPriorityOverBearer", func(t *testing.T) {
		t.Parallel()
		customToken := "custom-token"

		req := httptest.NewRequest("GET", "/test", nil)
		// Set both custom header and bearer token - custom should win
		req.Header.Set(codersdk.SessionTokenHeader, customToken)
		req.Header.Set("Authorization", "Bearer "+token)

		extractedToken := httpmw.APITokenFromRequest(req)
		require.Equal(t, customToken, extractedToken)
	})

	t.Run("CookiePriorityOverBearer", func(t *testing.T) {
		t.Parallel()
		cookieToken := "cookie-token"

		req := httptest.NewRequest("GET", "/test", nil)
		req.AddCookie(&http.Cookie{
			Name:  codersdk.SessionTokenCookie,
			Value: cookieToken,
		})
		req.Header.Set("Authorization", "Bearer "+token)

		extractedToken := httpmw.APITokenFromRequest(req)
		require.Equal(t, cookieToken, extractedToken)
	})

	t.Run("NoTokenReturnsEmpty", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest("GET", "/test", nil)

		extractedToken := httpmw.APITokenFromRequest(req)
		require.Empty(t, extractedToken)
	})

	t.Run("InvalidAuthorizationHeaderIgnored", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", "Basic dXNlcjpwYXNz") // Basic auth, not Bearer

		extractedToken := httpmw.APITokenFromRequest(req)
		require.Empty(t, extractedToken)
	})
}

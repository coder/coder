package httpmw_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/jwtutils"
	"github.com/coder/coder/v2/codersdk"
)

func TestHealthcheckOrSessionAuth(t *testing.T) {
	t.Parallel()

	signingKey := jwtutils.StaticKey{
		ID:  uuid.New().String(),
		Key: make([]byte, 64),
	}
	// Fill key with deterministic non-zero bytes.
	for i := range signingKey.Key.([]byte) {
		signingKey.Key.([]byte)[i] = byte(i + 1)
	}

	mintToken := func(t *testing.T, subject string, expiry time.Time) string {
		t.Helper()
		claims := jwtutils.RegisteredClaims{
			Subject: subject,
			Expiry:  jwt.NewNumericDate(expiry),
		}
		tok, err := jwtutils.Sign(t.Context(), signingKey, claims)
		require.NoError(t, err)
		return tok
	}

	// sessionCalled tracks whether the fallback session middleware
	// was invoked.
	newSessionMW := func(called *bool) func(http.Handler) http.Handler {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
				*called = true
				next.ServeHTTP(rw, r)
			})
		}
	}

	// rbacCalled tracks whether the fallback RBAC middleware was
	// invoked.
	newRBACMW := func(called *bool) func(http.Handler) http.Handler {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
				*called = true
				next.ServeHTTP(rw, r)
			})
		}
	}

	handler := http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		rw.WriteHeader(http.StatusOK)
	})

	t.Run("ValidJWT", func(t *testing.T) {
		t.Parallel()

		var sessionCalled, rbacCalled bool
		mw := httpmw.HealthcheckOrSessionAuth(
			signingKey,
			newSessionMW(&sessionCalled),
			newRBACMW(&rbacCalled),
		)

		token := mintToken(t, "healthcheck", time.Now().Add(time.Hour))
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set(codersdk.SessionTokenHeader, token)
		rw := httptest.NewRecorder()

		mw(handler).ServeHTTP(rw, r)

		require.Equal(t, http.StatusOK, rw.Code)
		require.False(t, sessionCalled, "session auth should be skipped for valid JWT")
		require.False(t, rbacCalled, "RBAC should be skipped for valid JWT")
	})

	t.Run("ExpiredJWT", func(t *testing.T) {
		t.Parallel()

		var sessionCalled, rbacCalled bool
		mw := httpmw.HealthcheckOrSessionAuth(
			signingKey,
			newSessionMW(&sessionCalled),
			newRBACMW(&rbacCalled),
		)

		token := mintToken(t, "healthcheck", time.Now().Add(-time.Hour))
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set(codersdk.SessionTokenHeader, token)
		rw := httptest.NewRecorder()

		mw(handler).ServeHTTP(rw, r)

		require.Equal(t, http.StatusOK, rw.Code)
		require.True(t, sessionCalled, "session auth should be used for expired JWT")
		require.True(t, rbacCalled, "RBAC should be used for expired JWT")
	})

	t.Run("WrongSubject", func(t *testing.T) {
		t.Parallel()

		var sessionCalled, rbacCalled bool
		mw := httpmw.HealthcheckOrSessionAuth(
			signingKey,
			newSessionMW(&sessionCalled),
			newRBACMW(&rbacCalled),
		)

		token := mintToken(t, "not-healthcheck", time.Now().Add(time.Hour))
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set(codersdk.SessionTokenHeader, token)
		rw := httptest.NewRecorder()

		mw(handler).ServeHTTP(rw, r)

		require.Equal(t, http.StatusOK, rw.Code)
		require.True(t, sessionCalled, "session auth should be used for wrong subject")
		require.True(t, rbacCalled, "RBAC should be used for wrong subject")
	})

	t.Run("GarbageToken", func(t *testing.T) {
		t.Parallel()

		var sessionCalled, rbacCalled bool
		mw := httpmw.HealthcheckOrSessionAuth(
			signingKey,
			newSessionMW(&sessionCalled),
			newRBACMW(&rbacCalled),
		)

		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set(codersdk.SessionTokenHeader, "not-a-jwt")
		rw := httptest.NewRecorder()

		mw(handler).ServeHTTP(rw, r)

		require.Equal(t, http.StatusOK, rw.Code)
		require.True(t, sessionCalled, "session auth should be used for garbage token")
		require.True(t, rbacCalled, "RBAC should be used for garbage token")
	})

	t.Run("NoToken", func(t *testing.T) {
		t.Parallel()

		var sessionCalled, rbacCalled bool
		mw := httpmw.HealthcheckOrSessionAuth(
			signingKey,
			newSessionMW(&sessionCalled),
			newRBACMW(&rbacCalled),
		)

		r := httptest.NewRequest(http.MethodGet, "/", nil)
		rw := httptest.NewRecorder()

		mw(handler).ServeHTTP(rw, r)

		require.Equal(t, http.StatusOK, rw.Code)
		require.True(t, sessionCalled, "session auth should be used when no token")
		require.True(t, rbacCalled, "RBAC should be used when no token")
	})

	t.Run("WrongSigningKey", func(t *testing.T) {
		t.Parallel()

		// Sign with a different key than what the middleware uses.
		wrongKey := jwtutils.StaticKey{
			ID:  uuid.New().String(),
			Key: make([]byte, 64),
		}
		for i := range wrongKey.Key.([]byte) {
			wrongKey.Key.([]byte)[i] = byte(i + 100)
		}
		claims := jwtutils.RegisteredClaims{
			Subject: "healthcheck",
			Expiry:  jwt.NewNumericDate(time.Now().Add(time.Hour)),
		}
		token, err := jwtutils.Sign(t.Context(), wrongKey, claims)
		require.NoError(t, err)

		var sessionCalled, rbacCalled bool
		mw := httpmw.HealthcheckOrSessionAuth(
			signingKey,
			newSessionMW(&sessionCalled),
			newRBACMW(&rbacCalled),
		)

		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set(codersdk.SessionTokenHeader, token)
		rw := httptest.NewRecorder()

		mw(handler).ServeHTTP(rw, r)

		require.Equal(t, http.StatusOK, rw.Code)
		require.True(t, sessionCalled, "session auth should be used for wrong key")
		require.True(t, rbacCalled, "RBAC should be used for wrong key")
	})
}

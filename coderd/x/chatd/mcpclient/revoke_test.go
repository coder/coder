package mcpclient_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
)

// revokeRequest carries a captured revocation request from the
// httptest handler goroutine to the test goroutine.
type revokeRequest struct {
	form      map[string][]string
	basicUser string
	basicPass string
	basicSet  bool
}

func captureRevoke(t *testing.T, got chan<- revokeRequest) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		user, pass, ok := r.BasicAuth()
		got <- revokeRequest{form: r.PostForm, basicUser: user, basicPass: pass, basicSet: ok}
		w.WriteHeader(http.StatusOK)
	}
}

func TestRevokeOAuth2Token(t *testing.T) {
	t.Parallel()

	t.Run("NoRevocationURL", func(t *testing.T) {
		t.Parallel()

		revoked, err := mcpclient.RevokeOAuth2Token(
			context.Background(),
			nil,
			database.MCPServerConfig{OAuth2ClientID: "cid"},
			database.MCPServerUserToken{AccessToken: "at", RefreshToken: "rt"},
		)
		require.NoError(t, err)
		require.False(t, revoked)
	})

	t.Run("RevokesRefreshToken", func(t *testing.T) {
		t.Parallel()

		got := make(chan revokeRequest, 1)
		srv := httptest.NewServer(captureRevoke(t, got))
		defer srv.Close()

		revoked, err := mcpclient.RevokeOAuth2Token(
			context.Background(),
			srv.Client(),
			database.MCPServerConfig{
				OAuth2ClientID:      "cid",
				OAuth2RevocationURL: srv.URL,
			},
			database.MCPServerUserToken{AccessToken: "at", RefreshToken: "rt"},
		)
		require.NoError(t, err)
		require.True(t, revoked)
		c := <-got
		require.Equal(t, []string{"rt"}, c.form["token"])
		require.Equal(t, []string{"refresh_token"}, c.form["token_type_hint"])
		require.Equal(t, []string{"cid"}, c.form["client_id"])
		// Public clients must not authenticate.
		require.False(t, c.basicSet)
		require.NotContains(t, c.form, "client_secret")
	})

	t.Run("AccessTokenFallbackWithBasicAuth", func(t *testing.T) {
		t.Parallel()

		got := make(chan revokeRequest, 1)
		srv := httptest.NewServer(captureRevoke(t, got))
		defer srv.Close()

		revoked, err := mcpclient.RevokeOAuth2Token(
			context.Background(),
			srv.Client(),
			database.MCPServerConfig{
				OAuth2ClientID:      "cid",
				OAuth2ClientSecret:  "secret",
				OAuth2RevocationURL: srv.URL,
			},
			database.MCPServerUserToken{AccessToken: "at"},
		)
		require.NoError(t, err)
		require.True(t, revoked)
		c := <-got
		require.Equal(t, []string{"at"}, c.form["token"])
		require.Equal(t, []string{"access_token"}, c.form["token_type_hint"])
		// Confidential clients use client_secret_basic, not form fields.
		require.True(t, c.basicSet)
		require.Equal(t, "cid", c.basicUser)
		require.Equal(t, "secret", c.basicPass)
		require.NotContains(t, c.form, "client_secret")
	})

	t.Run("AccessTokenFallbackAfterRefreshRejected", func(t *testing.T) {
		t.Parallel()

		got := make(chan revokeRequest, 2)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.NoError(t, r.ParseForm())
			got <- revokeRequest{form: r.PostForm}
			// Reject refresh-token revocation like a provider that
			// only supports access tokens (unsupported_token_type).
			if r.PostForm.Get("token_type_hint") == "refresh_token" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		revoked, err := mcpclient.RevokeOAuth2Token(
			context.Background(),
			srv.Client(),
			database.MCPServerConfig{
				OAuth2ClientID:      "cid",
				OAuth2RevocationURL: srv.URL,
			},
			database.MCPServerUserToken{AccessToken: "at", RefreshToken: "rt"},
		)
		require.NoError(t, err)
		require.True(t, revoked)
		first := <-got
		require.Equal(t, []string{"rt"}, first.form["token"])
		require.Equal(t, []string{"refresh_token"}, first.form["token_type_hint"])
		second := <-got
		require.Equal(t, []string{"at"}, second.form["token"])
		require.Equal(t, []string{"access_token"}, second.form["token_type_hint"])
	})

	t.Run("NoTokenMaterial", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			t.Error("provider must not be called without token material")
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		revoked, err := mcpclient.RevokeOAuth2Token(
			context.Background(),
			srv.Client(),
			database.MCPServerConfig{
				OAuth2ClientID:      "cid",
				OAuth2RevocationURL: srv.URL,
			},
			database.MCPServerUserToken{},
		)
		require.NoError(t, err)
		require.False(t, revoked)
	})

	t.Run("ProviderError", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("SECRET-ECHO " + strings.Repeat("x", 2048)))
		}))
		defer srv.Close()

		revoked, err := mcpclient.RevokeOAuth2Token(
			context.Background(),
			srv.Client(),
			database.MCPServerConfig{
				OAuth2ClientID:      "cid",
				OAuth2RevocationURL: srv.URL,
			},
			database.MCPServerUserToken{AccessToken: "at", RefreshToken: "rt"},
		)
		require.Error(t, err)
		require.False(t, revoked)
		require.Contains(t, err.Error(), "HTTP 500 for the refresh token")
		require.Contains(t, err.Error(), "HTTP 500 for the access token")
		// The provider body may echo request secrets and must not
		// surface in the error.
		require.NotContains(t, err.Error(), "SECRET-ECHO")
	})
}

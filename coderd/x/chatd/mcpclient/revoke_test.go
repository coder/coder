package mcpclient_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
)

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

	t.Run("NoContentIsSuccess", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}))
		defer srv.Close()

		revoked, err := mcpclient.RevokeOAuth2Token(
			context.Background(),
			srv.Client(),
			database.MCPServerConfig{
				OAuth2ClientID:      "cid",
				OAuth2RevocationURL: srv.URL,
			},
			database.MCPServerUserToken{AccessToken: "at"},
		)
		require.NoError(t, err)
		require.True(t, revoked)
	})

	t.Run("AcceptedIsNotSuccess", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusAccepted)
		}))
		defer srv.Close()

		revoked, err := mcpclient.RevokeOAuth2Token(
			context.Background(),
			srv.Client(),
			database.MCPServerConfig{
				OAuth2ClientID:      "cid",
				OAuth2RevocationURL: srv.URL,
			},
			database.MCPServerUserToken{AccessToken: "at"},
		)
		require.ErrorContains(t, err, "HTTP 202")
		require.False(t, revoked)
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
		// Basic auth must not be mixed with body client_id (RFC 6749 2.3.1).
		require.True(t, c.basicSet)
		require.Equal(t, "cid", c.basicUser)
		require.Equal(t, "secret", c.basicPass)
		require.NotContains(t, c.form, "client_id")
		require.NotContains(t, c.form, "client_secret")
	})

	t.Run("AccessTokenFallbackAfterUnsupportedTokenType", func(t *testing.T) {
		t.Parallel()

		got := make(chan revokeRequest, 2)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.NoError(t, r.ParseForm())
			got <- revokeRequest{form: r.PostForm}
			if r.PostForm.Get("token_type_hint") == "refresh_token" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"unsupported_token_type"}`))
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

	t.Run("NoFallbackWithoutUnsupportedTokenType", func(t *testing.T) {
		t.Parallel()

		var calls atomic.Int64
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			calls.Add(1)
			w.WriteHeader(http.StatusUnauthorized)
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
		require.Contains(t, err.Error(), "HTTP 401")
		// No access-token fallback: it could mask a live refresh token.
		require.EqualValues(t, 1, calls.Load())
	})

	t.Run("FallbackAlsoFails", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.NoError(t, r.ParseForm())
			if r.PostForm.Get("token_type_hint") == "refresh_token" {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"unsupported_token_type"}`))
				return
			}
			w.WriteHeader(http.StatusServiceUnavailable)
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
		require.Contains(t, err.Error(), "HTTP 400 for the refresh token")
		require.Contains(t, err.Error(), "HTTP 503 for the access token")
	})

	t.Run("RejectsNonHTTPSEndpoint", func(t *testing.T) {
		t.Parallel()

		// Loopback is exempt only for plain http; hostless forms
		// parse but can never be POSTed to.
		for u, wantErr := range map[string]string{
			"http://revoke.example.com/revoke": "must use https",
			"ftp://localhost/revoke":           "must use https",
			"https:/revoke":                    "has no host",
			"https:///revoke":                  "has no host",
		} {
			revoked, err := mcpclient.RevokeOAuth2Token(
				context.Background(),
				nil,
				database.MCPServerConfig{
					OAuth2ClientID:      "cid",
					OAuth2RevocationURL: u,
				},
				database.MCPServerUserToken{AccessToken: "at", RefreshToken: "rt"},
			)
			require.Error(t, err, u)
			require.False(t, revoked, u)
			require.Contains(t, err.Error(), wantErr, u)
		}
	})

	t.Run("RejectsPlaintextRedirect", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "http://revoke.example.com/revoke", http.StatusTemporaryRedirect)
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
		require.Contains(t, err.Error(), "must use https")
	})

	t.Run("RejectsBodyDroppingRedirect", func(t *testing.T) {
		t.Parallel()

		target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Returns 200 to the bodyless GET produced by the redirect.
			require.NoError(t, r.ParseForm())
			require.Empty(t, r.PostForm.Get("token"))
			w.WriteHeader(http.StatusOK)
		}))
		defer target.Close()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, target.URL, http.StatusFound)
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
		require.Contains(t, err.Error(), "dropped the POST body")
	})

	t.Run("RejectsCrossHostRedirect", func(t *testing.T) {
		t.Parallel()

		// CheckRedirect rejects before the attacker host is dialed.
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "https://attacker.example.com/collect?token=reflected-token", http.StatusTemporaryRedirect)
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
		require.Contains(t, err.Error(), "must stay on origin")
		require.NotContains(t, err.Error(), "/collect")
		require.NotContains(t, err.Error(), "reflected-token")
	})

	t.Run("FollowsLoopbackRedirect", func(t *testing.T) {
		t.Parallel()

		got := make(chan revokeRequest, 1)
		target := httptest.NewServer(captureRevoke(t, got))
		defer target.Close()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
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
		c := <-got
		require.Equal(t, []string{"rt"}, c.form["token"])
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
		require.Contains(t, err.Error(), "HTTP 500")
		// The secret-echoing body must not surface in the error.
		require.NotContains(t, err.Error(), "SECRET-ECHO")
	})
}

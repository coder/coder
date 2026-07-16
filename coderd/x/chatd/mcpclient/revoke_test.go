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

		var gotForm map[string][]string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.NoError(t, r.ParseForm())
			gotForm = r.PostForm
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
		require.Equal(t, []string{"rt"}, gotForm["token"])
		require.Equal(t, []string{"refresh_token"}, gotForm["token_type_hint"])
		require.Equal(t, []string{"cid"}, gotForm["client_id"])
		require.NotContains(t, gotForm, "client_secret")
	})

	t.Run("AccessTokenFallback", func(t *testing.T) {
		t.Parallel()

		var gotForm map[string][]string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.NoError(t, r.ParseForm())
			gotForm = r.PostForm
			w.WriteHeader(http.StatusOK)
		}))
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
		require.Equal(t, []string{"at"}, gotForm["token"])
		require.Equal(t, []string{"access_token"}, gotForm["token_type_hint"])
		require.Equal(t, []string{"secret"}, gotForm["client_secret"])
	})

	t.Run("ProviderError", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(strings.Repeat("x", 2048)))
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
		require.Less(t, len(err.Error()), 1024)
	})
}

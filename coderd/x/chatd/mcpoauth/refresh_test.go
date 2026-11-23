package mcpoauth_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpoauth"
)

func TestIsPermanentRefreshError(t *testing.T) {
	t.Parallel()

	retrieveErr := func(code string, status int) error {
		body, err := json.Marshal(map[string]string{"error": code})
		require.NoError(t, err)
		return &oauth2.RetrieveError{
			Response:  &http.Response{StatusCode: status},
			Body:      body,
			ErrorCode: code,
		}
	}

	cases := []struct {
		name      string
		err       error
		permanent bool
	}{
		{"InvalidGrant", retrieveErr("invalid_grant", http.StatusBadRequest), true},
		{"BadRefreshToken", retrieveErr("bad_refresh_token", http.StatusOK), true},
		{"WrappedInvalidGrant", xerrors.Errorf("refresh: %w", retrieveErr("invalid_grant", http.StatusBadRequest)), true},
		{"InvalidClient", retrieveErr("invalid_client", http.StatusUnauthorized), false},
		{"UnauthorizedClient", retrieveErr("unauthorized_client", http.StatusBadRequest), false},
		{"ServerError", retrieveErr("", http.StatusInternalServerError), false},
		{"RateLimited", retrieveErr("", http.StatusTooManyRequests), false},
		{"PlainError", xerrors.New("connection refused"), false},
		{"Nil", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.permanent, mcpoauth.IsPermanentRefreshError(tc.err))
		})
	}
}

func TestRefreshFailureReason(t *testing.T) {
	t.Parallel()

	require.Equal(t, "boom", mcpoauth.RefreshFailureReason(xerrors.New("boom")))

	long := strings.Repeat("x", 1000)
	reason := mcpoauth.RefreshFailureReason(xerrors.New(long))
	require.Len(t, reason, 400)
}

func TestRefreshOAuth2TokenInvalidGrant(t *testing.T) {
	t.Parallel()

	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"grant revoked"}`))
	}))
	defer tokenSrv.Close()

	cfg := database.MCPServerConfig{
		OAuth2ClientID: "cid",
		OAuth2TokenURL: tokenSrv.URL,
	}
	tok := database.MCPServerUserToken{
		AccessToken:  "expired",
		RefreshToken: "refresh",
		TokenType:    "Bearer",
		Expiry:       sql.NullTime{Time: time.Now().Add(-time.Hour), Valid: true},
	}

	_, err := mcpoauth.RefreshOAuth2Token(context.Background(), testMCPHTTPClient(nil), cfg, tok)
	require.Error(t, err)
	require.True(t, mcpoauth.IsPermanentRefreshError(err))
}

func TestRefreshOAuth2TokenRejectsCrossOriginRedirect(t *testing.T) {
	t.Parallel()

	var targetHits atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		targetHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"stolen","token_type":"Bearer"}`))
	}))
	defer target.Close()
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer tokenSrv.Close()

	cfg := database.MCPServerConfig{
		OAuth2ClientID:     "cid",
		OAuth2ClientSecret: "secret",
		OAuth2TokenURL:     tokenSrv.URL,
	}
	tok := database.MCPServerUserToken{
		AccessToken:  "expired",
		RefreshToken: "refresh",
		TokenType:    "Bearer",
		Expiry:       sql.NullTime{Time: time.Now().Add(-time.Hour), Valid: true},
	}

	_, err := mcpoauth.RefreshOAuth2Token(context.Background(), testMCPHTTPClient(tokenSrv.Client()), cfg, tok)
	require.Error(t, err)
	require.Zero(t, targetHits.Load())
}

func TestRefreshOAuth2TokenResourceIndicator(t *testing.T) {
	t.Parallel()

	const resource = "https://mcp.example.com/mcp"
	cases := []struct {
		name     string
		resource string
	}{
		{name: "Set", resource: resource},
		{name: "Unset"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			forms := make(chan url.Values, 1)
			tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !assert.NoError(t, r.ParseForm()) {
					http.Error(w, "bad form", http.StatusBadRequest)
					return
				}
				// Auth style auto-detection may retry a rejected request.
				select {
				case forms <- r.PostForm:
				default:
				}
				w.Header().Set("Content-Type", "application/json")
				if r.PostForm.Get("resource") != tc.resource {
					w.WriteHeader(http.StatusBadRequest)
					_, _ = w.Write([]byte(`{"error":"invalid_target"}`))
					return
				}
				_, _ = w.Write([]byte(`{"access_token":"fresh","token_type":"Bearer","expires_in":3600}`))
			}))
			defer tokenSrv.Close()

			cfg := database.MCPServerConfig{
				OAuth2ClientID: "cid",
				OAuth2TokenURL: tokenSrv.URL,
				OAuth2Resource: tc.resource,
			}
			tok := database.MCPServerUserToken{
				AccessToken:  "expired",
				RefreshToken: "refresh",
				TokenType:    "Bearer",
				Expiry:       sql.NullTime{Time: time.Now().Add(-time.Hour), Valid: true},
			}

			result, err := mcpoauth.RefreshOAuth2Token(context.Background(), testMCPHTTPClient(nil), cfg, tok)
			require.NoError(t, err)
			require.True(t, result.Refreshed)
			require.Equal(t, "fresh", result.AccessToken)
			// The provider did not rotate, so the stored refresh token is kept.
			require.Equal(t, "refresh", result.RefreshToken)

			form := <-forms
			require.Equal(t, "refresh_token", form.Get("grant_type"))
			require.Equal(t, "refresh", form.Get("refresh_token"))
			require.False(t, form.Has("scope"))
			if tc.resource == "" {
				require.False(t, form.Has("resource"))
			}
		})
	}
}

func TestRefreshOAuth2TokenWithoutRequest(t *testing.T) {
	t.Parallel()

	var hits atomic.Int64
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"fresh","token_type":"Bearer","expires_in":3600}`))
	}))
	t.Cleanup(tokenSrv.Close)

	cfg := database.MCPServerConfig{
		OAuth2ClientID: "cid",
		OAuth2TokenURL: tokenSrv.URL,
		OAuth2Resource: "https://mcp.example.com/mcp",
	}

	t.Run("ValidToken", func(t *testing.T) {
		t.Parallel()
		expiry := time.Now().Add(time.Hour)
		result, err := mcpoauth.RefreshOAuth2Token(context.Background(), testMCPHTTPClient(nil), cfg, database.MCPServerUserToken{
			AccessToken:  "still-valid",
			RefreshToken: "refresh",
			TokenType:    "Bearer",
			Expiry:       sql.NullTime{Time: expiry, Valid: true},
		})
		require.NoError(t, err)
		require.False(t, result.Refreshed)
		require.Equal(t, "still-valid", result.AccessToken)
		require.Equal(t, "refresh", result.RefreshToken)
		require.Equal(t, "Bearer", result.TokenType)
		require.True(t, expiry.Equal(result.Expiry))
	})

	t.Run("ExpiredWithoutRefreshToken", func(t *testing.T) {
		t.Parallel()
		_, err := mcpoauth.RefreshOAuth2Token(context.Background(), testMCPHTTPClient(nil), cfg, database.MCPServerUserToken{
			AccessToken: "expired",
			TokenType:   "Bearer",
			Expiry:      sql.NullTime{Time: time.Now().Add(-time.Hour), Valid: true},
		})
		require.Error(t, err)
	})

	t.Cleanup(func() {
		require.Zero(t, hits.Load(), "no token request may be made")
	})
}

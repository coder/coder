package mcpclient_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
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
			require.Equal(t, tc.permanent, mcpclient.IsPermanentRefreshError(tc.err))
		})
	}
}

func TestRefreshFailureReason(t *testing.T) {
	t.Parallel()

	require.Equal(t, "boom", mcpclient.RefreshFailureReason(xerrors.New("boom")))

	long := strings.Repeat("x", 1000)
	reason := mcpclient.RefreshFailureReason(xerrors.New(long))
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

	_, err := mcpclient.RefreshOAuth2Token(context.Background(), cfg, tok)
	require.Error(t, err)
	require.True(t, mcpclient.IsPermanentRefreshError(err))
}

func TestBuildAuthHeadersSkipsFailedToken(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	cfg := database.MCPServerConfig{
		ID:       uuid.New(),
		Slug:     "revoked",
		AuthType: "oauth2",
	}

	t.Run("FailureReasonSet", func(t *testing.T) {
		t.Parallel()
		headers := mcpclient.BuildAuthHeadersForTest(
			context.Background(), logger, cfg,
			map[uuid.UUID]database.MCPServerUserToken{
				cfg.ID: {
					MCPServerConfigID:         cfg.ID,
					AccessToken:               "leftover",
					OauthRefreshFailureReason: "invalid_grant",
				},
			},
			uuid.New(), nil,
		)
		require.NotContains(t, headers, "Authorization")
	})

	t.Run("HealthyToken", func(t *testing.T) {
		t.Parallel()
		headers := mcpclient.BuildAuthHeadersForTest(
			context.Background(), logger, cfg,
			map[uuid.UUID]database.MCPServerUserToken{
				cfg.ID: {
					MCPServerConfigID: cfg.ID,
					AccessToken:       "valid",
					TokenType:         "Bearer",
				},
			},
			uuid.New(), nil,
		)
		require.Equal(t, "Bearer valid", headers["Authorization"])
	})
}

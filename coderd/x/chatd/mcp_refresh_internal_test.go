package chatd

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
)

func invalidGrantServer(t *testing.T, hits *atomic.Int64) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits != nil {
			hits.Add(1)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func expiredMCPToken(cfgID uuid.UUID) database.MCPServerUserToken {
	return database.MCPServerUserToken{
		ID:                uuid.New(),
		MCPServerConfigID: cfgID,
		UserID:            uuid.New(),
		AccessToken:       "expired-access",
		RefreshToken:      "dead-refresh",
		TokenType:         "Bearer",
		Expiry:            sql.NullTime{Time: time.Now().Add(-time.Hour), Valid: true},
		UpdatedAt:         time.Now().Add(-time.Hour),
	}
}

func TestRefreshMCPTokenPermanentFailure(t *testing.T) {
	t.Parallel()

	t.Run("MarksTokenAndClearsAuth", func(t *testing.T) {
		t.Parallel()
		tokenSrv := invalidGrantServer(t, nil)

		cfg := database.MCPServerConfig{
			ID:             uuid.New(),
			Slug:           "revoked",
			AuthType:       "oauth2",
			OAuth2ClientID: "cid",
			OAuth2TokenURL: tokenSrv.URL,
		}
		tok := expiredMCPToken(cfg.ID)

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		marked := tok
		marked.AccessToken = ""
		marked.RefreshToken = ""
		marked.Expiry = sql.NullTime{}
		marked.OauthRefreshFailureReason = "invalid_grant"
		db.EXPECT().
			MarkMCPServerUserTokenRefreshFailure(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, arg database.MarkMCPServerUserTokenRefreshFailureParams) (database.MCPServerUserToken, error) {
				require.Equal(t, tok.ID, arg.ID)
				require.Equal(t, tok.UpdatedAt, arg.UpdatedAt)
				require.Contains(t, arg.OauthRefreshFailureReason, "invalid_grant")
				return marked, nil
			})

		server := &Server{db: db}
		result, err := server.refreshMCPTokenIfNeeded(
			context.Background(), slogtest.Make(t, nil), cfg, tok,
		)
		require.NoError(t, err)
		require.Empty(t, result.AccessToken)
		require.Empty(t, result.RefreshToken)
		require.NotEmpty(t, result.OauthRefreshFailureReason)
	})

	t.Run("OptimisticLockLossUsesWinnerRow", func(t *testing.T) {
		t.Parallel()
		tokenSrv := invalidGrantServer(t, nil)

		cfg := database.MCPServerConfig{
			ID:             uuid.New(),
			Slug:           "raced",
			AuthType:       "oauth2",
			OAuth2ClientID: "cid",
			OAuth2TokenURL: tokenSrv.URL,
		}
		tok := expiredMCPToken(cfg.ID)
		winner := tok
		winner.AccessToken = "fresh-access"
		winner.RefreshToken = "fresh-refresh"
		winner.Expiry = sql.NullTime{Time: time.Now().Add(time.Hour), Valid: true}

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		db.EXPECT().
			MarkMCPServerUserTokenRefreshFailure(gomock.Any(), gomock.Any()).
			Return(database.MCPServerUserToken{}, sql.ErrNoRows)
		db.EXPECT().
			GetMCPServerUserToken(gomock.Any(), database.GetMCPServerUserTokenParams{
				MCPServerConfigID: tok.MCPServerConfigID,
				UserID:            tok.UserID,
			}).
			Return(winner, nil)

		server := &Server{db: db}
		result, err := server.refreshMCPTokenIfNeeded(
			context.Background(), slogtest.Make(t, nil), cfg, tok,
		)
		require.NoError(t, err)
		require.Equal(t, "fresh-access", result.AccessToken)
		require.Empty(t, result.OauthRefreshFailureReason)
	})

	t.Run("PersistFailureStillClearsAuth", func(t *testing.T) {
		t.Parallel()
		tokenSrv := invalidGrantServer(t, nil)

		cfg := database.MCPServerConfig{
			ID:             uuid.New(),
			Slug:           "db-down",
			AuthType:       "oauth2",
			OAuth2ClientID: "cid",
			OAuth2TokenURL: tokenSrv.URL,
		}
		tok := expiredMCPToken(cfg.ID)

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		db.EXPECT().
			MarkMCPServerUserTokenRefreshFailure(gomock.Any(), gomock.Any()).
			Return(database.MCPServerUserToken{}, sql.ErrConnDone)

		server := &Server{db: db}
		result, err := server.refreshMCPTokenIfNeeded(
			context.Background(), slogtest.Make(t, nil), cfg, tok,
		)
		require.NoError(t, err)
		require.Empty(t, result.AccessToken)
		require.Empty(t, result.RefreshToken)
		require.NotEmpty(t, result.OauthRefreshFailureReason)
	})

	t.Run("TransientFailureKeepsToken", func(t *testing.T) {
		t.Parallel()
		tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		t.Cleanup(tokenSrv.Close)

		cfg := database.MCPServerConfig{
			ID:             uuid.New(),
			Slug:           "flaky",
			AuthType:       "oauth2",
			OAuth2ClientID: "cid",
			OAuth2TokenURL: tokenSrv.URL,
		}
		tok := expiredMCPToken(cfg.ID)

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)

		server := &Server{db: db}
		result, err := server.refreshMCPTokenIfNeeded(
			context.Background(), slogtest.Make(t, nil), cfg, tok,
		)
		require.Error(t, err)
		require.Equal(t, tok.AccessToken, result.AccessToken)
		require.Equal(t, tok.RefreshToken, result.RefreshToken)
		require.Empty(t, result.OauthRefreshFailureReason)
	})
}

func TestRefreshMCPTokenDeletedDuringRefresh(t *testing.T) {
	t.Parallel()

	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"fresh-access","refresh_token":"fresh-refresh","token_type":"Bearer","expires_in":3600}`))
	}))
	t.Cleanup(tokenSrv.Close)

	cfg := database.MCPServerConfig{
		ID:             uuid.New(),
		Slug:           "disconnected",
		AuthType:       "oauth2",
		OAuth2ClientID: "cid",
		OAuth2TokenURL: tokenSrv.URL,
	}
	tok := expiredMCPToken(cfg.ID)

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	db.EXPECT().
		UpdateMCPServerUserTokenFromRefresh(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, arg database.UpdateMCPServerUserTokenFromRefreshParams) (database.MCPServerUserToken, error) {
			require.Equal(t, tok.ID, arg.ID)
			require.Equal(t, tok.UpdatedAt, arg.UpdatedAt)
			return database.MCPServerUserToken{}, sql.ErrNoRows
		})
	db.EXPECT().
		GetMCPServerUserToken(gomock.Any(), database.GetMCPServerUserTokenParams{
			MCPServerConfigID: tok.MCPServerConfigID,
			UserID:            tok.UserID,
		}).
		Return(database.MCPServerUserToken{}, sql.ErrNoRows)

	server := &Server{db: db}
	result, err := server.refreshMCPTokenIfNeeded(
		context.Background(), slogtest.Make(t, nil), cfg, tok,
	)
	require.NoError(t, err)
	require.Empty(t, result.AccessToken)
	require.Empty(t, result.RefreshToken)
	require.Empty(t, result.OauthRefreshFailureReason)
}

func TestRefreshExpiredMCPTokensSkipsFailedTokens(t *testing.T) {
	t.Parallel()

	var hits atomic.Int64
	tokenSrv := invalidGrantServer(t, &hits)

	cfg := database.MCPServerConfig{
		ID:             uuid.New(),
		Slug:           "revoked",
		AuthType:       "oauth2",
		OAuth2ClientID: "cid",
		OAuth2TokenURL: tokenSrv.URL,
	}
	tok := expiredMCPToken(cfg.ID)
	tok.OauthRefreshFailureReason = "invalid_grant"

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	server := &Server{db: db}
	result := server.refreshExpiredMCPTokens(
		context.Background(), slogtest.Make(t, nil),
		[]database.MCPServerConfig{cfg},
		[]database.MCPServerUserToken{tok},
	)
	require.Len(t, result, 1)
	require.Equal(t, tok, result[0])
	require.EqualValues(t, 0, hits.Load(), "provider must not be called for failed tokens")
}

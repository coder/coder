package mcpoauth_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpoauth"
	"github.com/coder/coder/v2/testutil"
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

// heldLeaseSignalStore closes held once an acquire finds the refresh
// lease held by another caller.
type heldLeaseSignalStore struct {
	database.Store
	once sync.Once
	held chan struct{}
}

func (s *heldLeaseSignalStore) AcquireMCPServerUserTokenRefreshLease(ctx context.Context, arg database.AcquireMCPServerUserTokenRefreshLeaseParams) (database.MCPServerUserToken, error) {
	tok, err := s.Store.AcquireMCPServerUserTokenRefreshLease(ctx, arg)
	if database.IsCheckViolation(err) {
		s.once.Do(func() { close(s.held) })
	}
	return tok, err
}

// TestRefreshOAuth2TokenWithLeaseAfterHeldLease covers a caller that waits
// while another caller holds the refresh lease, then finds the row
// unchanged under the lease. The other caller did not change the row, so
// this caller reports a temporary failure without calling the token
// endpoint, instead of retrying it in series after every such holder.
func TestRefreshOAuth2TokenWithLeaseAfterHeldLease(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		expiry time.Duration
		opts   mcpoauth.LeasedRefreshOptions
	}{
		{
			name:   "Expired",
			expiry: -time.Minute,
			opts:   mcpoauth.LeasedRefreshOptions{WaitForLease: true},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db, _ := dbtestutil.NewDB(t)
			ctx := testutil.Context(t, testutil.WaitLong)
			var calls atomic.Int64
			tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.WriteHeader(http.StatusServiceUnavailable)
			}))
			t.Cleanup(tokenSrv.Close)
			cfg, tok := seedMCPToken(t, db, tokenSrv.URL, "access", "refresh", tc.expiry)

			// Another caller holds the lease and leaves the row unchanged.
			held, err := db.AcquireMCPServerUserTokenRefreshLease(ctx, database.AcquireMCPServerUserTokenRefreshLeaseParams{
				ID:        tok.ID,
				TimeoutMs: time.Minute.Milliseconds(),
			})
			require.NoError(t, err)
			store := &heldLeaseSignalStore{Store: db, held: make(chan struct{})}
			type refreshed struct {
				result mcpoauth.LeasedRefresh
				err    error
			}
			done := make(chan refreshed, 1)
			go func() {
				result, err := mcpoauth.RefreshOAuth2TokenWithLease(ctx, slogtest.Make(t, nil), store, testMCPHTTPClient(nil), cfg, tok, tc.opts)
				done <- refreshed{result: result, err: err}
			}()
			select {
			case <-store.held:
			case <-ctx.Done():
				t.Fatal("the refresh never found the lease held")
			}
			require.NoError(t, db.ReleaseMCPServerUserTokenRefreshLease(ctx, database.ReleaseMCPServerUserTokenRefreshLeaseParams{
				ID:                    tok.ID,
				RefreshLeaseExpiresAt: held.RefreshLeaseExpiresAt,
			}))
			var got refreshed
			select {
			case got = <-done:
			case <-ctx.Done():
				t.Fatal("the refresh never returned")
			}

			require.NoError(t, got.err)
			require.Zero(t, calls.Load(), "token endpoint calls")
			require.Equal(t, mcpoauth.LeasedRefreshFailed, got.result.Outcome)
			require.Error(t, got.result.RefreshErr)
			row, err := db.GetMCPServerUserTokenByID(ctx, tok.ID)
			require.NoError(t, err)
			require.Equal(t, "access", row.AccessToken)
			require.Equal(t, "refresh", row.RefreshToken)
			require.Empty(t, row.OauthRefreshFailureReason)
			require.False(t, row.RefreshLeaseExpiresAt.Valid, "lease released")
		})
	}
}

// seedMCPToken stores a token that expires after expiresIn for a new user,
// on a new oauth2 config whose token endpoint is tokenURL.
func seedMCPToken(
	t *testing.T,
	db database.Store,
	tokenURL, accessToken, refreshToken string,
	expiresIn time.Duration,
) (database.MCPServerConfig, database.MCPServerUserToken) {
	t.Helper()
	owner := dbgen.User(t, db, database.User{})
	cfg := dbgen.MCPServerConfig(t, db, database.MCPServerConfig{
		AuthType:       "oauth2",
		OAuth2ClientID: "cid",
		OAuth2TokenURL: tokenURL,
		CreatedBy:      uuid.NullUUID{UUID: owner.ID, Valid: true},
	})
	tok := dbgen.MCPServerUserToken(t, db, database.MCPServerUserToken{
		MCPServerConfigID: cfg.ID,
		UserID:            owner.ID,
		AccessToken:       accessToken,
		RefreshToken:      refreshToken,
		Expiry:            sql.NullTime{Time: time.Now().Add(expiresIn), Valid: true},
	})
	return cfg, tok
}

package mcpclient_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
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

	_, err := mcpclient.RefreshOAuth2Token(context.Background(), testMCPHTTPClient(nil), cfg, tok)
	require.Error(t, err)
	require.True(t, mcpclient.IsPermanentRefreshError(err))
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

	_, err := mcpclient.RefreshOAuth2Token(context.Background(), testMCPHTTPClient(tokenSrv.Client()), cfg, tok)
	require.Error(t, err)
	require.Zero(t, targetHits.Load())
}

func TestBuildAuthHeadersSkipsFailedToken(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	cfg := mcpclient.Server{
		ID:       uuid.New(),
		Slug:     "revoked",
		UserAuth: mcpclient.UserAuthOAuth2,
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
			uuid.New(), nil, nil,
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
			uuid.New(), nil, nil,
		)
		require.Equal(t, "Bearer valid", headers["Authorization"])
	})
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
				// A public client sends client_id in the body, not Basic auth.
				assert.Empty(t, r.Header.Get("Authorization"))
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

			result, err := mcpclient.RefreshOAuth2Token(context.Background(), testMCPHTTPClient(nil), cfg, tok)
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
		result, err := mcpclient.RefreshOAuth2Token(context.Background(), testMCPHTTPClient(nil), cfg, database.MCPServerUserToken{
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
		_, err := mcpclient.RefreshOAuth2Token(context.Background(), testMCPHTTPClient(nil), cfg, database.MCPServerUserToken{
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

// TestRefreshUserTokenOneRefresher refreshes one expired token from three
// callers at once while the first one's provider call is held, then from a
// later caller. The provider rotates refresh tokens and reports any reuse of
// a spent one.
func TestRefreshUserTokenOneRefresher(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name        string
		refresh     string
		wantAccess  string
		wantFailure bool
		wantGrants  int64
		// saveFails fails the first call's save with a database error. Its
		// claim then stays until it lapses, so that row has no other callers.
		saveFails bool
		// cancelFirst cancels the first caller during its provider call.
		cancelFirst bool
	}{
		{name: "Rotates", refresh: "refresh-0", wantAccess: "access-1", wantGrants: 1},
		{name: "RevokedGrant", refresh: "revoked", wantFailure: true},
		{name: "ProviderDown", refresh: "down", wantAccess: "expired-access"},
		{name: "Disconnected", refresh: "disconnect", wantGrants: 1},
		{name: "Reconnected", refresh: "reconnect", wantAccess: "reconnected-access", wantGrants: 1},
		{name: "SameAccessToken", refresh: "same-access", wantAccess: "expired-access", wantGrants: 1},
		{name: "SaveFails", refresh: "refresh-0", wantAccess: "expired-access", wantGrants: 1, saveFails: true},
		{name: "CallerCanceled", refresh: "down", wantAccess: "expired-access", cancelFirst: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			clock := quartz.NewMock(t)
			poll := clock.Trap().NewTimer("RefreshUserToken", "poll")
			store, _ := dbtestutil.NewDB(t)
			db := store
			if tc.saveFails {
				db = failingSaveStore{store}
			}
			authzDB := dbauthz.New(db, rbac.NewStrictCachingAuthorizer(prometheus.NewRegistry()), slogtest.Make(t, nil), nil)
			user := dbgen.User(t, store, database.User{})

			var requests, grants, reuses atomic.Int64
			var spent sync.Map
			release := make(chan struct{})
			var cfg database.MCPServerConfig
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if requests.Add(1) == 1 {
					select {
					case <-release:
					case <-ctx.Done():
					}
				}
				w.Header().Set("Content-Type", "application/json")
				refresh := r.FormValue("refresh_token")
				_, reused := spent.LoadOrStore(refresh, true)
				if reused && refresh != "down" && refresh != "revoked" {
					reuses.Add(1)
				}
				switch {
				case refresh == "down":
					w.WriteHeader(http.StatusInternalServerError)
				case refresh == "revoked" || reused:
					w.WriteHeader(http.StatusBadRequest)
					_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
				default:
					if refresh == "disconnect" || refresh == "reconnect" {
						assert.NoError(t, store.DeleteMCPServerUserToken(ctx, database.DeleteMCPServerUserTokenParams{MCPServerConfigID: cfg.ID, UserID: user.ID}))
					}
					if refresh == "reconnect" {
						_, err := store.UpsertMCPServerUserToken(ctx, database.UpsertMCPServerUserTokenParams{MCPServerConfigID: cfg.ID, UserID: user.ID, AccessToken: "reconnected-access", TokenType: "Bearer"})
						assert.NoError(t, err)
					}
					n := grants.Add(1)
					access := fmt.Sprintf("access-%d", n)
					if refresh == "same-access" {
						access = "expired-access"
					}
					_, _ = fmt.Fprintf(w, `{"access_token":%q,"refresh_token":"refresh-%d","token_type":"Bearer","expires_in":3600}`, access, n)
				}
			}))
			t.Cleanup(provider.Close)
			cfg = dbgen.MCPServerConfig(t, store, database.MCPServerConfig{AuthType: "oauth2", OAuth2ClientID: "cid", OAuth2TokenURL: provider.URL})
			tok, err := store.UpsertMCPServerUserToken(ctx, database.UpsertMCPServerUserTokenParams{
				MCPServerConfigID: cfg.ID,
				UserID:            user.ID,
				AccessToken:       "expired-access",
				RefreshToken:      tc.refresh,
				TokenType:         "Bearer",
				Expiry:            sql.NullTime{Time: time.Now().Add(-time.Hour), Valid: true},
			})
			require.NoError(t, err)
			refresh := func(ctx context.Context) (database.MCPServerUserToken, error) {
				return mcpclient.RefreshUserToken(dbauthz.AsChatd(ctx), slogtest.Make(t, nil), clock, authzDB, testMCPHTTPClient(nil), cfg, tok)
			}
			results := make(chan database.MCPServerUserToken, 3)
			call := func(ctx context.Context, wantErr bool) {
				got, err := refresh(ctx)
				assert.Equal(t, wantErr, err != nil, "error: %v", err)
				results <- got
			}

			firstCtx, cancelFirst := context.WithCancel(ctx)
			defer cancelFirst()
			go call(firstCtx, tc.saveFails)
			testutil.Eventually(ctx, t, func(context.Context) bool { return requests.Load() == 1 }, testutil.IntervalFast)
			// The other callers wait for the first instead of spending the
			// refresh token again.
			waiters := 2
			if tc.saveFails {
				waiters = 0
			}
			for range waiters {
				go call(ctx, false)
				poll.MustWait(ctx).MustRelease(ctx)
			}
			if tc.cancelFirst {
				cancelFirst()
			}
			close(release)
			rows := []database.MCPServerUserToken{testutil.RequireReceive(ctx, t, results)}
			sent := requests.Load()
			// When the first call ends, the waiters return at their next poll
			// with what it left, without a provider call of their own.
			if waiters > 0 {
				_, w := clock.AdvanceNext()
				w.MustWait(ctx)
			}
			for range waiters {
				rows = append(rows, testutil.RequireReceive(ctx, t, results))
			}
			require.Equal(t, sent, requests.Load(), "a waiting caller reached the provider")
			poll.Close()
			// A later caller does not wait, also after a provider failure.
			if !tc.saveFails {
				late, err := refresh(ctx)
				require.NoError(t, err)
				rows = append(rows, late)
			}
			for _, got := range rows {
				require.Equal(t, tc.wantAccess, got.AccessToken)
				require.Equal(t, tc.wantFailure, got.OauthRefreshFailureReason != "")
			}
			require.Equal(t, tc.wantGrants, grants.Load())
			require.Zero(t, reuses.Load(), "a spent refresh token was sent again")
		})
	}
}

// TestRefreshUserTokenReleaseKeepsNewerClaim fails caller A's provider call
// after a reconnect stored a token that is already due and caller B claimed
// it. A's release must leave B's claim alone, so a later caller C waits for
// B instead of spending B's refresh token a second time.
func TestRefreshUserTokenReleaseKeepsNewerClaim(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(testutil.Context(t, testutil.WaitLong))
	defer cancel()
	clock := quartz.NewMock(t)
	poll := clock.Trap().NewTimer("RefreshUserToken", "poll")
	defer poll.Close()
	store, _ := dbtestutil.NewDB(t)
	authzDB := dbauthz.New(store, rbac.NewStrictCachingAuthorizer(prometheus.NewRegistry()), slogtest.Make(t, nil), nil)
	user := dbgen.User(t, store, database.User{})

	failA, finishB := make(chan struct{}), make(chan struct{})
	sent := make(chan string, 8)
	var reconnectedSends atomic.Int64
	wait := func(ch chan struct{}) {
		select {
		case <-ch:
		case <-ctx.Done():
		}
	}
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		refresh := r.FormValue("refresh_token")
		sent <- refresh
		if refresh == "original" {
			wait(failA)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if reconnectedSends.Add(1) > 1 {
			assert.Fail(t, "the reconnected refresh token was sent twice")
			cancel()
			return
		}
		wait(finishB)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"b-access","refresh_token":"b-refresh","token_type":"Bearer","expires_in":3600}`))
	}))
	t.Cleanup(provider.Close)
	cfg := dbgen.MCPServerConfig(t, store, database.MCPServerConfig{AuthType: "oauth2", OAuth2ClientID: "cid", OAuth2TokenURL: provider.URL})
	upsert := func(refresh string) database.MCPServerUserToken {
		tok, err := store.UpsertMCPServerUserToken(ctx, database.UpsertMCPServerUserTokenParams{
			MCPServerConfigID: cfg.ID, UserID: user.ID, AccessToken: "expired-access", RefreshToken: refresh, TokenType: "Bearer",
			Expiry: sql.NullTime{Time: time.Now().Add(-time.Hour), Valid: true},
		})
		require.NoError(t, err)
		return tok
	}
	tok := upsert("original")
	results := make(chan database.MCPServerUserToken, 3)
	call := func() {
		got, err := mcpclient.RefreshUserToken(dbauthz.AsChatd(ctx), slogtest.Make(t, nil), clock, authzDB, testMCPHTTPClient(nil), cfg, tok)
		assert.NoError(t, err)
		results <- got
	}

	go call() // A
	require.Equal(t, "original", testutil.RequireReceive(ctx, t, sent))
	upsert("reconnected")
	go call() // B
	require.Equal(t, "reconnected", testutil.RequireReceive(ctx, t, sent))
	close(failA)
	testutil.RequireReceive(ctx, t, results)
	go call() // C
	poll.MustWait(ctx).MustRelease(ctx)
	close(finishB)
	require.Equal(t, "b-access", testutil.RequireReceive(ctx, t, results).AccessToken)
	_, w := clock.AdvanceNext()
	w.MustWait(ctx)
	require.Equal(t, "b-access", testutil.RequireReceive(ctx, t, results).AccessToken)
}

// failingSaveStore fails saving a refreshed token with a database error.
type failingSaveStore struct{ database.Store }

func (failingSaveStore) UpdateMCPServerUserTokenFromRefresh(context.Context, database.UpdateMCPServerUserTokenFromRefreshParams) (database.MCPServerUserToken, error) {
	return database.MCPServerUserToken{}, xerrors.New("connection reset")
}

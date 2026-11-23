package chatd

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/safedial"
)

// loopbackMCPServer permits local httptest token endpoints through
// the SSRF guard.
func loopbackMCPServer(db database.Store) *Server {
	return &Server{
		db: db,
		mcpHTTPClient: safedial.NewHTTPClient(nil, safedial.WithAllowedPrefixes(
			netip.MustParsePrefix("127.0.0.0/8"),
			netip.MustParsePrefix("::1/128"),
		)),
	}
}

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

// expectRefreshLease makes the mock grant the refresh lease on tok and
// expects the release of exactly that lease. It returns the leased row.
func expectRefreshLease(t *testing.T, db *dbmock.MockStore, tok database.MCPServerUserToken) database.MCPServerUserToken {
	t.Helper()
	leased := tok
	leased.RefreshLeaseExpiresAt = sql.NullTime{Time: time.Now().Add(time.Minute), Valid: true}
	db.EXPECT().
		AcquireMCPServerUserTokenRefreshLease(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, arg database.AcquireMCPServerUserTokenRefreshLeaseParams) (database.MCPServerUserToken, error) {
			require.Equal(t, tok.ID, arg.ID)
			return leased, nil
		})
	db.EXPECT().
		ReleaseMCPServerUserTokenRefreshLease(gomock.Any(), database.ReleaseMCPServerUserTokenRefreshLeaseParams{
			ID:                    tok.ID,
			RefreshLeaseExpiresAt: leased.RefreshLeaseExpiresAt,
		}).
		Return(nil)
	return leased
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
		leased := expectRefreshLease(t, db, tok)
		marked := tok
		marked.AccessToken = ""
		marked.RefreshToken = ""
		marked.Expiry = sql.NullTime{}
		marked.OauthRefreshFailureReason = "invalid_grant"
		db.EXPECT().
			MarkMCPServerUserTokenRefreshFailure(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, arg database.MarkMCPServerUserTokenRefreshFailureParams) (database.MCPServerUserToken, error) {
				require.Equal(t, tok.ID, arg.ID)
				require.Equal(t, leased.UpdatedAt, arg.UpdatedAt)
				require.Equal(t, leased.RefreshLeaseExpiresAt.Time, arg.Lease)
				require.Contains(t, arg.OauthRefreshFailureReason, "invalid_grant")
				return marked, nil
			})

		server := loopbackMCPServer(db)
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
		expectRefreshLease(t, db, tok)
		db.EXPECT().
			MarkMCPServerUserTokenRefreshFailure(gomock.Any(), gomock.Any()).
			Return(database.MCPServerUserToken{}, sql.ErrNoRows)
		db.EXPECT().
			GetMCPServerUserToken(gomock.Any(), database.GetMCPServerUserTokenParams{
				MCPServerConfigID: tok.MCPServerConfigID,
				UserID:            tok.UserID,
			}).
			Return(winner, nil)

		server := loopbackMCPServer(db)
		result, err := server.refreshMCPTokenIfNeeded(
			context.Background(), slogtest.Make(t, nil), cfg, tok,
		)
		require.NoError(t, err)
		require.Equal(t, "fresh-access", result.AccessToken)
		require.Empty(t, result.OauthRefreshFailureReason)
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
		expectRefreshLease(t, db, tok)

		server := loopbackMCPServer(db)
		result, err := server.refreshMCPTokenIfNeeded(
			context.Background(), slogtest.Make(t, nil), cfg, tok,
		)
		require.NoError(t, err)
		require.Equal(t, tok.AccessToken, result.AccessToken)
		require.Equal(t, tok.RefreshToken, result.RefreshToken)
		require.Empty(t, result.OauthRefreshFailureReason)
	})
}

func TestRefreshMCPTokenDatabaseError(t *testing.T) {
	t.Parallel()

	refreshed := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"fresh-access","refresh_token":"fresh-refresh","token_type":"Bearer","expires_in":3600}`))
	}
	invalidGrant := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
	}

	for _, tc := range []struct {
		name     string
		provider http.HandlerFunc
		expect   func(t *testing.T, db *dbmock.MockStore, tok database.MCPServerUserToken)
	}{
		{
			name:     "AcquireLease",
			provider: refreshed,
			expect: func(_ *testing.T, db *dbmock.MockStore, _ database.MCPServerUserToken) {
				db.EXPECT().
					AcquireMCPServerUserTokenRefreshLease(gomock.Any(), gomock.Any()).
					Return(database.MCPServerUserToken{}, sql.ErrConnDone)
			},
		},
		{
			name:     "PersistRefresh",
			provider: refreshed,
			expect: func(t *testing.T, db *dbmock.MockStore, tok database.MCPServerUserToken) {
				expectRefreshLease(t, db, tok)
				db.EXPECT().
					UpdateMCPServerUserTokenFromRefresh(gomock.Any(), gomock.Any()).
					Return(database.MCPServerUserToken{}, sql.ErrConnDone)
			},
		},
		{
			name:     "ReadAfterRefreshConflict",
			provider: refreshed,
			expect: func(t *testing.T, db *dbmock.MockStore, tok database.MCPServerUserToken) {
				expectRefreshLease(t, db, tok)
				db.EXPECT().
					UpdateMCPServerUserTokenFromRefresh(gomock.Any(), gomock.Any()).
					Return(database.MCPServerUserToken{}, sql.ErrNoRows)
				db.EXPECT().
					GetMCPServerUserToken(gomock.Any(), gomock.Any()).
					Return(database.MCPServerUserToken{}, sql.ErrConnDone)
			},
		},
		{
			name:     "MarkRefreshFailure",
			provider: invalidGrant,
			expect: func(t *testing.T, db *dbmock.MockStore, tok database.MCPServerUserToken) {
				expectRefreshLease(t, db, tok)
				db.EXPECT().
					MarkMCPServerUserTokenRefreshFailure(gomock.Any(), gomock.Any()).
					Return(database.MCPServerUserToken{}, sql.ErrConnDone)
			},
		},
		{
			name:     "ReadAfterMarkConflict",
			provider: invalidGrant,
			expect: func(t *testing.T, db *dbmock.MockStore, tok database.MCPServerUserToken) {
				expectRefreshLease(t, db, tok)
				db.EXPECT().
					MarkMCPServerUserTokenRefreshFailure(gomock.Any(), gomock.Any()).
					Return(database.MCPServerUserToken{}, sql.ErrNoRows)
				db.EXPECT().
					GetMCPServerUserToken(gomock.Any(), gomock.Any()).
					Return(database.MCPServerUserToken{}, sql.ErrConnDone)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tokenSrv := httptest.NewServer(tc.provider)
			t.Cleanup(tokenSrv.Close)

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
			tc.expect(t, db, tok)

			server := loopbackMCPServer(db)
			_, err := server.refreshExpiredMCPTokens(
				context.Background(), slogtest.Make(t, nil),
				[]database.MCPServerConfig{cfg},
				[]database.MCPServerUserToken{tok},
			)
			require.ErrorIs(t, err, sql.ErrConnDone)
		})
	}
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
	leased := expectRefreshLease(t, db, tok)
	db.EXPECT().
		UpdateMCPServerUserTokenFromRefresh(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, arg database.UpdateMCPServerUserTokenFromRefreshParams) (database.MCPServerUserToken, error) {
			require.Equal(t, tok.ID, arg.ID)
			require.Equal(t, leased.UpdatedAt, arg.UpdatedAt)
			require.Equal(t, leased.RefreshLeaseExpiresAt.Time, arg.Lease)
			return database.MCPServerUserToken{}, sql.ErrNoRows
		})
	db.EXPECT().
		GetMCPServerUserToken(gomock.Any(), database.GetMCPServerUserTokenParams{
			MCPServerConfigID: tok.MCPServerConfigID,
			UserID:            tok.UserID,
		}).
		Return(database.MCPServerUserToken{}, sql.ErrNoRows)

	server := loopbackMCPServer(db)
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

	server := loopbackMCPServer(db)
	result, err := server.refreshExpiredMCPTokens(
		context.Background(), slogtest.Make(t, nil),
		[]database.MCPServerConfig{cfg},
		[]database.MCPServerUserToken{tok},
	)
	require.NoError(t, err)
	require.Len(t, result, 1)
	require.Equal(t, tok, result[0])
	require.EqualValues(t, 0, hits.Load(), "provider must not be called for failed tokens")
}

// writeSignalStore closes written once the first refresh save or failure
// mark has returned, so a fake token endpoint can order its answers after
// another caller's write.
type writeSignalStore struct {
	database.Store
	once    sync.Once
	written chan struct{}
}

func (s *writeSignalStore) UpdateMCPServerUserTokenFromRefresh(ctx context.Context, arg database.UpdateMCPServerUserTokenFromRefreshParams) (database.MCPServerUserToken, error) {
	tok, err := s.Store.UpdateMCPServerUserTokenFromRefresh(ctx, arg)
	s.once.Do(func() { close(s.written) })
	return tok, err
}

func (s *writeSignalStore) MarkMCPServerUserTokenRefreshFailure(ctx context.Context, arg database.MarkMCPServerUserTokenRefreshFailureParams) (database.MCPServerUserToken, error) {
	tok, err := s.Store.MarkMCPServerUserTokenRefreshFailure(ctx, arg)
	s.once.Do(func() { close(s.written) })
	return tok, err
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

// rotatingTokenEndpoint is an OAuth token endpoint for one grant that
// issues a new refresh token on every successful refresh. Every request
// first waits at gate. When two refreshes overlap, it answers them in the
// order that loses a token without the refresh lease:
//   - strict rotation accepts only the current refresh token, and holds
//     the successful answer until the rejected caller has marked the
//     token dead;
//   - grace rotation, like Cloudflare workers-oauth-provider, also
//     accepts the previous refresh token, and holds the second answer
//     until the first caller has saved its token, which the second
//     rotation then makes stale.
type rotatingTokenEndpoint struct {
	gate    *chattest.Gate
	grace   bool
	grant   *chattest.TokenGrant
	written <-chan struct{}

	mu       sync.Mutex
	calls    int
	answered int
}

func (e *rotatingTokenEndpoint) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	e.mu.Lock()
	e.calls++
	e.mu.Unlock()
	if e.gate.Wait(r.Context()) != nil {
		return
	}
	waitForWrite := func() bool {
		select {
		case <-e.written:
			return true
		case <-r.Context().Done():
			return false
		}
	}
	overlapping := e.gate.Arrivals() > 1

	if e.grace {
		e.mu.Lock()
		e.answered++
		later := e.answered > 1
		e.mu.Unlock()
		if later && !waitForWrite() {
			return
		}
	}

	newAccess, newRefresh, accepted := e.grant.Rotate(r.PostFormValue("refresh_token"))

	w.Header().Set("Content-Type", "application/json")
	if !accepted {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"Invalid refresh token"}`))
		return
	}
	if !e.grace && overlapping && !waitForWrite() {
		return
	}
	_, _ = fmt.Fprintf(w, `{"access_token":%q,"refresh_token":%q,"token_type":"Bearer","expires_in":3600}`, newAccess, newRefresh)
}

func (e *rotatingTokenEndpoint) stats() (calls int, lastIssued string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.calls, e.grant.Current()
}

// seedExpiredMCPToken stores an expired token for a new user on a new
// oauth2 config whose token endpoint is tokenURL.
func seedExpiredMCPToken(
	t *testing.T,
	db database.Store,
	tokenURL, accessToken, refreshToken string,
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
		Expiry:            sql.NullTime{Time: time.Now().Add(-time.Minute), Valid: true},
	})
	return cfg, tok
}

func newAuthzStore(t *testing.T, db database.Store) database.Store {
	t.Helper()
	return dbauthz.New(db, rbac.NewStrictCachingAuthorizer(prometheus.NewRegistry()), slogtest.Make(t, nil), nil)
}

func TestRefreshMCPTokenConcurrentRefreshesShareOneCall(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		grace bool
	}{
		{name: "StrictRotation"},
		{name: "GraceRotation", grace: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := dbauthz.AsChatd(testutil.Context(t, testutil.WaitLong))
			rawDB, _ := dbtestutil.NewDB(t)
			written := make(chan struct{})
			store := &heldLeaseSignalStore{
				Store: &writeSignalStore{Store: newAuthzStore(t, rawDB), written: written},
				held:  make(chan struct{}),
			}
			gate := chattest.NewGate()
			defer gate.Open()
			endpoint := &rotatingTokenEndpoint{gate: gate, grace: tc.grace, grant: chattest.NewTokenGrant("", "refresh-0", tc.grace), written: written}
			tokenSrv := httptest.NewServer(endpoint)
			t.Cleanup(tokenSrv.Close)
			cfg, tok := seedExpiredMCPToken(t, rawDB, tokenSrv.URL, "access-0", "refresh-0")

			server := loopbackMCPServer(store)
			logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
			results := make([]database.MCPServerUserToken, 2)
			var wg sync.WaitGroup
			for i := range results {
				wg.Go(func() {
					var err error
					results[i], err = server.refreshMCPTokenIfNeeded(ctx, logger, cfg, tok)
					assert.NoError(t, err)
				})
			}
			// Hold the first token request until the second caller finds the
			// lease held. Without the lease, the second caller's token request
			// arrives instead, and the endpoint answers both in the order
			// that loses a token.
			select {
			case <-gate.Arrived(1):
			case <-ctx.Done():
				t.Fatal("no token request arrived")
			}
			select {
			case <-store.held:
			case <-gate.Arrived(2):
			case <-ctx.Done():
				t.Fatal("the second caller neither found the lease held nor called the token endpoint")
			}
			gate.Open()
			wg.Wait()

			final, err := rawDB.GetMCPServerUserTokenByID(ctx, tok.ID)
			require.NoError(t, err)
			calls, lastIssued := endpoint.stats()
			require.Equal(t, 1, calls, "token endpoint calls")
			require.Equal(t, lastIssued, final.RefreshToken)
			require.Empty(t, final.OauthRefreshFailureReason)
			require.False(t, final.RefreshLeaseExpiresAt.Valid, "lease released")
			// One caller returns the saved row, the other the row it read
			// under the lease after that save. Equal updated_at values mean
			// neither the later acquire nor either release wrote it.
			for _, result := range results {
				require.Equal(t, lastIssued, result.RefreshToken)
				require.Truef(t, final.UpdatedAt.Equal(result.UpdatedAt),
					"updated_at changed after the save: saved %s, now %s", result.UpdatedAt, final.UpdatedAt)
			}

			if tc.grace {
				// The stored refresh token is the one the endpoint accepts
				// next.
				_, err = rawDB.UpsertMCPServerUserToken(ctx, database.UpsertMCPServerUserTokenParams{
					MCPServerConfigID: final.MCPServerConfigID,
					UserID:            final.UserID,
					AccessToken:       final.AccessToken,
					RefreshToken:      final.RefreshToken,
					TokenType:         final.TokenType,
					Expiry:            sql.NullTime{Time: time.Now().Add(-time.Minute), Valid: true},
				})
				require.NoError(t, err)
				expired, err := rawDB.GetMCPServerUserTokenByID(ctx, tok.ID)
				require.NoError(t, err)
				next, err := server.refreshMCPTokenIfNeeded(ctx, logger, cfg, expired)
				require.NoError(t, err)
				calls, lastIssued = endpoint.stats()
				require.Equal(t, 2, calls, "token endpoint calls")
				require.Equal(t, lastIssued, next.RefreshToken)
				require.Empty(t, next.OauthRefreshFailureReason)
			}
		})
	}
}

func TestRefreshMCPTokenDisconnectDuringLease(t *testing.T) {
	t.Parallel()

	ctx := dbauthz.AsChatd(testutil.Context(t, testutil.WaitLong))
	rawDB, _ := dbtestutil.NewDB(t)
	var tok database.MCPServerUserToken
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The user disconnects while chatd holds the lease.
		assert.NoError(t, rawDB.DeleteMCPServerUserToken(r.Context(), database.DeleteMCPServerUserTokenParams{
			MCPServerConfigID: tok.MCPServerConfigID,
			UserID:            tok.UserID,
		}))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"fresh-access","refresh_token":"fresh-refresh","token_type":"Bearer","expires_in":3600}`))
	}))
	t.Cleanup(tokenSrv.Close)
	cfg, seeded := seedExpiredMCPToken(t, rawDB, tokenSrv.URL, "expired-access", "old-refresh")
	tok = seeded

	// Under dbauthz, releasing the lease on the deleted row fails its
	// authorization fetch.
	server := loopbackMCPServer(newAuthzStore(t, rawDB))
	result, err := server.refreshMCPTokenIfNeeded(ctx, slogtest.Make(t, nil), cfg, tok)
	require.NoError(t, err)
	require.Equal(t, tok.ID, result.ID)
	require.Empty(t, result.AccessToken)
	require.Empty(t, result.RefreshToken)
	require.False(t, result.Expiry.Valid)
	require.Empty(t, result.OauthRefreshFailureReason)
}

func TestRefreshMCPTokenValidTokenTakesNoLease(t *testing.T) {
	t.Parallel()

	var hits atomic.Int64
	tokenSrv := invalidGrantServer(t, &hits)
	cfg := database.MCPServerConfig{
		ID:             uuid.New(),
		Slug:           "fresh",
		AuthType:       "oauth2",
		OAuth2ClientID: "cid",
		OAuth2TokenURL: tokenSrv.URL,
	}
	tok := expiredMCPToken(cfg.ID)
	tok.Expiry = sql.NullTime{Time: time.Now().Add(time.Hour), Valid: true}

	// The strict mock fails the test on any query, the lease included.
	db := dbmock.NewMockStore(gomock.NewController(t))
	server := loopbackMCPServer(db)
	result, err := server.refreshMCPTokenIfNeeded(context.Background(), slogtest.Make(t, nil), cfg, tok)
	require.NoError(t, err)
	require.Equal(t, tok, result)
	require.EqualValues(t, 0, hits.Load())
}

func TestRefreshMCPTokenSavesRotatedRefreshToken(t *testing.T) {
	t.Parallel()

	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"same-access","refresh_token":"rotated-refresh","token_type":"Bearer","expires_in":3600}`))
	}))
	t.Cleanup(tokenSrv.Close)

	ctx := dbauthz.AsChatd(testutil.Context(t, testutil.WaitLong))
	rawDB, _ := dbtestutil.NewDB(t)
	cfg, tok := seedExpiredMCPToken(t, rawDB, tokenSrv.URL, "same-access", "old-refresh")

	server := loopbackMCPServer(newAuthzStore(t, rawDB))
	_, err := server.refreshMCPTokenIfNeeded(ctx, slogtest.Make(t, nil), cfg, tok)
	require.NoError(t, err)

	row, err := rawDB.GetMCPServerUserTokenByID(ctx, tok.ID)
	require.NoError(t, err)
	require.Equal(t, "same-access", row.AccessToken)
	require.Equal(t, "rotated-refresh", row.RefreshToken)
	require.True(t, row.Expiry.Time.After(time.Now()), "expiry moves forward")
}

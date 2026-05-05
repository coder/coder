package externalauth_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/oauth2"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/coderdtest/oidctest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/externalauth"
	"github.com/coder/coder/v2/coderd/promoauth"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestRefreshToken(t *testing.T) {
	t.Parallel()
	expired := time.Now().Add(time.Hour * -1)

	t.Run("NoRefreshExpired", func(t *testing.T) {
		t.Parallel()
		fake, config, link := setupOauth2Test(t, testConfig{
			FakeIDPOpts: []oidctest.FakeIDPOpt{
				oidctest.WithRefresh(func(_ string) error {
					t.Error("refresh on the IDP was called, but NoRefresh was set")
					return xerrors.New("should not be called")
				}),
				// The IDP should not be contacted since the token is expired. An expired
				// token with 'NoRefresh' should early abort.
				oidctest.WithDynamicUserInfo(func(_ string) (jwt.MapClaims, error) {
					t.Error("token was validated, but it was expired and this should never have happened.")
					return nil, xerrors.New("should not be called")
				}),
			},
			ExternalAuthOpt: func(cfg *externalauth.Config) {
				cfg.NoRefresh = true
			},
		})

		ctx := oidc.ClientContext(context.Background(), fake.HTTPClient(nil))
		// Expire the link
		link.OAuthExpiry = expired

		_, err := config.RefreshToken(ctx, nil, link)
		require.Error(t, err)
		require.True(t, externalauth.IsInvalidTokenError(err))
		require.Contains(t, err.Error(), "refreshing is either disabled or refreshing failed")
	})

	// NoRefreshNoExpiry tests that an oauth token without an expiry is always valid.
	// The "validate url" should be hit, but the refresh endpoint should not.
	t.Run("NoRefreshNoExpiry", func(t *testing.T) {
		t.Parallel()

		validated := false
		fake, config, link := setupOauth2Test(t, testConfig{
			FakeIDPOpts: []oidctest.FakeIDPOpt{
				oidctest.WithRefresh(func(_ string) error {
					t.Error("refresh on the IDP was called, but NoRefresh was set")
					return xerrors.New("should not be called")
				}),
				oidctest.WithDynamicUserInfo(func(_ string) (jwt.MapClaims, error) {
					validated = true
					return jwt.MapClaims{}, nil
				}),
			},
			ExternalAuthOpt: func(cfg *externalauth.Config) {
				cfg.NoRefresh = true
			},
		})

		ctx := oidc.ClientContext(context.Background(), fake.HTTPClient(nil))

		// Zero time used
		link.OAuthExpiry = time.Time{}

		_, err := config.RefreshToken(ctx, nil, link)
		require.NoError(t, err)
		require.True(t, validated, "token should have been validated")
	})

	t.Run("FalseIfTokenSourceFails", func(t *testing.T) {
		t.Parallel()
		config := &externalauth.Config{
			InstrumentedOAuth2Config: &testutil.OAuth2Config{
				TokenSourceFunc: func() (*oauth2.Token, error) {
					return nil, xerrors.New("failure")
				},
			},
		}

		_, err := config.RefreshToken(context.Background(), nil, database.ExternalAuthLink{
			OAuthExpiry: expired,
		})
		require.Error(t, err)
		require.True(t, externalauth.IsInvalidTokenError(err))
		require.Contains(t, err.Error(), "failure")
	})

	t.Run("ValidateServerError", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		mDB := dbmock.NewMockStore(ctrl)
		mDB.EXPECT().UpdateExternalAuthLink(gomock.Any(), gomock.Any()).
			Return(database.ExternalAuthLink{}, nil).AnyTimes()

		const staticError = "static error"
		validated := false
		fake, config, link := setupOauth2Test(t, testConfig{
			FakeIDPOpts: []oidctest.FakeIDPOpt{
				oidctest.WithDynamicUserInfo(func(_ string) (jwt.MapClaims, error) {
					validated = true
					return jwt.MapClaims{}, xerrors.New(staticError)
				}),
			},
			ExternalAuthOpt: func(cfg *externalauth.Config) {
			},
		})

		ctx := oidc.ClientContext(context.Background(), fake.HTTPClient(nil))
		link.OAuthExpiry = expired

		_, err := config.RefreshToken(ctx, mDB, link)
		require.ErrorContains(t, err, staticError)
		// Unsure if this should be the correct behavior. It's an invalid token because
		// 'ValidateToken()' failed with a runtime error. This was the previous behavior,
		// so not going to change it.
		require.False(t, externalauth.IsInvalidTokenError(err))
		require.True(t, validated, "token should have been attempted to be validated")
	})

	// RefreshRetries tests that refresh token retry behavior works as expected.
	// If a refresh token fails because the token itself is invalid, no more
	// refresh attempts should ever happen. An invalid refresh token does
	// not magically become valid at some point in the future.
	t.Run("RefreshRetries", func(t *testing.T) {
		t.Parallel()

		var refreshErr *oauth2.RetrieveError

		ctrl := gomock.NewController(t)
		mDB := dbmock.NewMockStore(ctrl)

		refreshCount := 0
		fake, config, link := setupOauth2Test(t, testConfig{
			FakeIDPOpts: []oidctest.FakeIDPOpt{
				oidctest.WithRefresh(func(_ string) error {
					refreshCount++
					return refreshErr
				}),
				// The IDP should not be contacted since the token is expired and
				// refresh attempts will fail.
				oidctest.WithDynamicUserInfo(func(_ string) (jwt.MapClaims, error) {
					t.Error("token was validated, but it was expired and this should never have happened.")
					return nil, xerrors.New("should not be called")
				}),
			},
			ExternalAuthOpt: func(cfg *externalauth.Config) {},
		})

		ctx := oidc.ClientContext(context.Background(), fake.HTTPClient(nil))
		// Expire the link
		link.OAuthExpiry = expired

		// Make the failure a server internal error. Not related to the token
		// This should be retried since this error is temporary.
		refreshErr = &oauth2.RetrieveError{
			Response: &http.Response{
				StatusCode: http.StatusInternalServerError,
			},
			ErrorCode: "internal_error",
		}
		totalRefreshes := 0
		for i := 0; i < 3; i++ {
			// Each loop will hit the temporary error and retry.
			_, err := config.RefreshToken(ctx, mDB, link)
			require.Error(t, err)
			totalRefreshes++
			require.True(t, externalauth.IsInvalidTokenError(err))
			require.Equal(t, refreshCount, totalRefreshes)
		}

		// Try again with a bad refresh token error. This will invalidate the
		// refresh token, and not retry again. Expect DB calls to check for
		// concurrent refresh (GetExternalAuthLink) and then remove the refresh token.
		mDB.EXPECT().GetExternalAuthLink(gomock.Any(), gomock.Any()).Return(link, nil).Times(1)
		mDB.EXPECT().UpdateExternalAuthLinkRefreshToken(gomock.Any(), gomock.Any()).Return(nil).Times(1)
		refreshErr = &oauth2.RetrieveError{ // github error
			Response: &http.Response{
				StatusCode: http.StatusOK,
			},
			ErrorCode: "bad_refresh_token",
		}
		_, err := config.RefreshToken(ctx, mDB, link)
		require.Error(t, err)
		totalRefreshes++
		require.True(t, externalauth.IsInvalidTokenError(err))
		require.Equal(t, refreshCount, totalRefreshes)

		// When the refresh token is empty, no api calls should be made
		link.OAuthRefreshToken = "" // mock'd db, so manually set the token to ''
		_, err = config.RefreshToken(ctx, mDB, link)
		require.Error(t, err)
		require.True(t, externalauth.IsInvalidTokenError(err))
		require.Equal(t, refreshCount, totalRefreshes)
	})

	// ConcurrentRefreshRace tests that when multiple concurrent requests
	// race to refresh the same token, the loser does not poison the
	// database with a cached "bad_refresh_token" failure. This
	// reproduces the issue described in coder/coder#17069 where
	// providers with single-use refresh tokens (e.g., GitHub Apps)
	// reject the second refresh attempt, and the resulting error was
	// incorrectly cached.
	t.Run("ConcurrentRefreshRace", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		mDB := dbmock.NewMockStore(ctrl)

		fake, config, link := setupOauth2Test(t, testConfig{
			FakeIDPOpts: []oidctest.FakeIDPOpt{
				oidctest.WithRefresh(func(_ string) error {
					return &oauth2.RetrieveError{
						Response: &http.Response{
							StatusCode: http.StatusOK,
						},
						ErrorCode: "bad_refresh_token",
					}
				}),
			},
			ExternalAuthOpt: func(cfg *externalauth.Config) {},
		})

		ctx := oidc.ClientContext(context.Background(), fake.HTTPClient(nil))
		link.OAuthExpiry = time.Now().Add(time.Hour * -1)

		// Simulate a concurrent winner: when the loser re-reads the
		// DB, the refresh token has changed (the winner stored a new
		// one). The loser should return the updated link instead of
		// caching the failure.
		winnerLink := link
		winnerLink.OAuthRefreshToken = "winner-refresh-token"
		winnerLink.OAuthAccessToken = "winner-access-token"
		mDB.EXPECT().GetExternalAuthLink(gomock.Any(), database.GetExternalAuthLinkParams{
			ProviderID: link.ProviderID,
			UserID:     link.UserID,
		}).Return(winnerLink, nil).Times(1)

		// UpdateExternalAuthLinkRefreshToken should NOT be called
		// because the re-read detected the concurrent refresh.

		result, err := config.RefreshToken(ctx, mDB, link)
		require.NoError(t, err, "loser should succeed using the winner's token")
		require.Equal(t, "winner-access-token", result.OAuthAccessToken)
		require.Equal(t, "winner-refresh-token", result.OAuthRefreshToken)
	})

	// ValidateFailure tests if the token is no longer valid with a 401 response.
	t.Run("ValidateFailure", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		mDB := dbmock.NewMockStore(ctrl)
		mDB.EXPECT().UpdateExternalAuthLink(gomock.Any(), gomock.Any()).
			Return(database.ExternalAuthLink{}, nil).AnyTimes()

		const staticError = "static error"
		validated := false
		fake, config, link := setupOauth2Test(t, testConfig{
			FakeIDPOpts: []oidctest.FakeIDPOpt{
				oidctest.WithDynamicUserInfo(func(_ string) (jwt.MapClaims, error) {
					validated = true
					return jwt.MapClaims{}, oidctest.StatusError(http.StatusUnauthorized, xerrors.New(staticError))
				}),
			},
			ExternalAuthOpt: func(cfg *externalauth.Config) {
			},
		})

		ctx := oidc.ClientContext(context.Background(), fake.HTTPClient(nil))
		link.OAuthExpiry = expired

		_, err := config.RefreshToken(ctx, mDB, link)
		require.ErrorContains(t, err, "token failed to validate")
		require.True(t, externalauth.IsInvalidTokenError(err))
		require.True(t, validated, "token should have been attempted to be validated")
	})

	t.Run("ValidateRetryGitHub", func(t *testing.T) {
		t.Parallel()

		const staticError = "static error"
		validateCalls := 0
		fake, config, link := setupOauth2Test(t, testConfig{
			FakeIDPOpts: []oidctest.FakeIDPOpt{
				oidctest.WithRefresh(func(_ string) error {
					t.Error("refresh on the IDP was called, but the token is not expired")
					return xerrors.New("should not be called")
				}),
				oidctest.WithDynamicUserInfo(func(_ string) (jwt.MapClaims, error) {
					validateCalls++
					// Make the first call return a 401, subsequent calls should return a 200.
					if validateCalls > 1 {
						return jwt.MapClaims{}, nil
					}
					return jwt.MapClaims{}, oidctest.StatusError(http.StatusUnauthorized, xerrors.New(staticError))
				}),
			},
			ExternalAuthOpt: func(cfg *externalauth.Config) {
				cfg.Type = codersdk.EnhancedExternalAuthProviderGitHub.String()
			},
		})

		ctx := oidc.ClientContext(context.Background(), fake.HTTPClient(nil))
		// Unlimited lifetime, this is what GitHub returns tokens as
		link.OAuthExpiry = time.Time{}

		_, err := config.RefreshToken(ctx, nil, link)
		require.NoError(t, err)
		require.Equal(t, 2, validateCalls, "token should have been attempted to be validated more than once")
	})

	t.Run("ValidateNoUpdate", func(t *testing.T) {
		t.Parallel()

		validateCalls := 0
		fake, config, link := setupOauth2Test(t, testConfig{
			FakeIDPOpts: []oidctest.FakeIDPOpt{
				oidctest.WithRefresh(func(_ string) error {
					t.Error("refresh on the IDP was called, but the token is not expired")
					return xerrors.New("should not be called")
				}),
				oidctest.WithDynamicUserInfo(func(_ string) (jwt.MapClaims, error) {
					validateCalls++
					return jwt.MapClaims{}, nil
				}),
			},
			ExternalAuthOpt: func(cfg *externalauth.Config) {
				cfg.Type = codersdk.EnhancedExternalAuthProviderGitHub.String()
			},
		})

		ctx := oidc.ClientContext(context.Background(), fake.HTTPClient(nil))

		_, err := config.RefreshToken(ctx, nil, link)
		require.NoError(t, err)
		require.Equal(t, 1, validateCalls, "token is validated")
	})

	// A token update comes from a refresh.
	t.Run("Updates", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		validateCalls := 0
		refreshCalls := 0
		fake, config, link := setupOauth2Test(t, testConfig{
			FakeIDPOpts: []oidctest.FakeIDPOpt{
				oidctest.WithRefresh(func(_ string) error {
					refreshCalls++
					return nil
				}),
				oidctest.WithDynamicUserInfo(func(_ string) (jwt.MapClaims, error) {
					validateCalls++
					return jwt.MapClaims{}, nil
				}),
			},
			ExternalAuthOpt: func(cfg *externalauth.Config) {
				cfg.Type = codersdk.EnhancedExternalAuthProviderGitHub.String()
			},
			DB: db,
		})

		ctx := oidc.ClientContext(context.Background(), fake.HTTPClient(nil))
		// Force a refresh
		link.OAuthExpiry = expired

		updated, err := config.RefreshToken(ctx, db, link)
		require.NoError(t, err)
		require.Equal(t, 1, validateCalls, "token is validated")
		require.Equal(t, 1, refreshCalls, "token is refreshed")
		require.NotEqualf(t, link.OAuthAccessToken, updated.OAuthAccessToken, "token is updated")
		dbLink, err := db.GetExternalAuthLink(context.Background(), database.GetExternalAuthLinkParams{
			ProviderID: link.ProviderID,
			UserID:     link.UserID,
		})
		require.NoError(t, err)
		require.Equal(t, updated.OAuthAccessToken, dbLink.OAuthAccessToken, "token is updated in the DB")
	})
	t.Run("WithExtra", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		fake, config, link := setupOauth2Test(t, testConfig{
			FakeIDPOpts: []oidctest.FakeIDPOpt{
				oidctest.WithMutateToken(func(token map[string]interface{}) {
					token["authed_user"] = map[string]interface{}{
						"access_token": token["access_token"],
					}
				}),
			},
			ExternalAuthOpt: func(cfg *externalauth.Config) {
				cfg.Type = codersdk.EnhancedExternalAuthProviderSlack.String()
				cfg.ExtraTokenKeys = []string{"authed_user"}
				cfg.ValidateURL = ""
			},
			DB: db,
		})

		ctx := oidc.ClientContext(context.Background(), fake.HTTPClient(nil))
		// Force a refresh
		link.OAuthExpiry = expired

		updated, err := config.RefreshToken(ctx, db, link)
		require.NoError(t, err)

		require.True(t, updated.OAuthExtra.Valid)
		extra := map[string]interface{}{}
		require.NoError(t, json.Unmarshal(updated.OAuthExtra.RawMessage, &extra))
		mapping, ok := extra["authed_user"].(map[string]interface{})
		require.True(t, ok)
		require.Equal(t, updated.OAuthAccessToken, mapping["access_token"])
	})

	// SaveBeforeValidate tests that a successfully refreshed token is
	// persisted to the DB even when post-refresh validation fails. This
	// prevents the data-loss scenario where GitHub rotates the refresh
	// token on use but the new token is silently discarded because a
	// rate-limited validation endpoint returns 403.
	t.Run("SaveBeforeValidate", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)

		// simulateRateLimit controls whether the validate endpoint
		// returns 403 (true) or 200 (false).
		var simulateRateLimit atomic.Bool
		simulateRateLimit.Store(true)

		var refreshCalls atomic.Int64
		fake, config, link := setupOauth2Test(t, testConfig{
			FakeIDPOpts: []oidctest.FakeIDPOpt{
				oidctest.WithRefresh(func(_ string) error {
					refreshCalls.Add(1)
					return nil
				}),
				oidctest.WithDynamicUserInfo(func(_ string) (jwt.MapClaims, error) {
					if simulateRateLimit.Load() {
						return jwt.MapClaims{}, oidctest.StatusError(http.StatusForbidden, xerrors.New("rate limit exceeded"))
					}
					return jwt.MapClaims{}, nil
				}),
			},
			ExternalAuthOpt: func(cfg *externalauth.Config) {
				cfg.Type = codersdk.EnhancedExternalAuthProviderGitHub.String()
			},
			DB: db,
		})

		ctx := oidc.ClientContext(context.Background(), fake.HTTPClient(nil))

		oldAccessToken := link.OAuthAccessToken
		oldRefreshToken := link.OAuthRefreshToken

		// Expire the token to force a refresh.
		link.OAuthExpiry = expired

		// First call: refresh succeeds, validation fails (403).
		_, err := config.RefreshToken(ctx, db, link)
		require.Error(t, err, "expected error because validation returned 403")
		require.True(t, externalauth.IsInvalidTokenError(err))
		require.Equal(t, int64(1), refreshCalls.Load(), "IDP refresh should have been called exactly once")

		// Critical assertion: the DB must contain the NEW tokens from the
		// successful refresh, not the old (now-stale) ones.
		dbLink, err := db.GetExternalAuthLink(context.Background(), database.GetExternalAuthLinkParams{
			ProviderID: link.ProviderID,
			UserID:     link.UserID,
		})
		require.NoError(t, err)
		require.NotEqual(t, oldAccessToken, dbLink.OAuthAccessToken,
			"DB should have the new access token from the successful refresh")
		require.NotEqual(t, oldRefreshToken, dbLink.OAuthRefreshToken,
			"DB should have the new refresh token (old one was rotated by the IDP)")

		// Second call: uses the saved token from DB, no re-refresh.
		// The saved token has a future expiry, so TokenSource should return
		// it without contacting the IDP. Validation should succeed now.
		simulateRateLimit.Store(false)
		updated, err := config.RefreshToken(ctx, db, dbLink)
		require.NoError(t, err, "second call should succeed because rate limit lifted")
		require.Equal(t, int64(1), refreshCalls.Load(),
			"IDP refresh should NOT have been called again; the saved token is not expired")
		require.Equal(t, dbLink.OAuthAccessToken, updated.OAuthAccessToken,
			"returned token should match what was saved in the DB")
	})

	// SaveBeforeValidate_ContextCanceled verifies the early DB save
	// uses a detached context. The parent context is canceled inside
	// the refresh hook (after TokenSource.Token() but before the DB
	// write), and the test asserts the new token is still persisted.
	t.Run("SaveBeforeValidate_ContextCanceled", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)

		var refreshCalls atomic.Int64
		cancelOnRefresh, cancel := context.WithCancel(context.Background())
		defer cancel()

		fake, config, link := setupOauth2Test(t, testConfig{
			FakeIDPOpts: []oidctest.FakeIDPOpt{
				oidctest.WithRefresh(func(_ string) error {
					refreshCalls.Add(1)
					// Cancel the parent context after refresh succeeds
					// but before the DB save and validation.
					cancel()
					return nil
				}),
				oidctest.WithDynamicUserInfo(func(_ string) (jwt.MapClaims, error) {
					return jwt.MapClaims{}, nil
				}),
			},
			ExternalAuthOpt: func(cfg *externalauth.Config) {
				cfg.Type = codersdk.EnhancedExternalAuthProviderGitHub.String()
			},
			DB: db,
		})

		ctx := oidc.ClientContext(cancelOnRefresh, fake.HTTPClient(nil))

		oldAccessToken := link.OAuthAccessToken
		oldRefreshToken := link.OAuthRefreshToken
		link.OAuthExpiry = expired

		_, err := config.RefreshToken(ctx, db, link)
		require.NoError(t, err)
		require.Equal(t, int64(1), refreshCalls.Load())

		dbLink, err := db.GetExternalAuthLink(context.Background(), database.GetExternalAuthLinkParams{
			ProviderID: link.ProviderID,
			UserID:     link.UserID,
		})
		require.NoError(t, err)
		require.NotEqual(t, oldAccessToken, dbLink.OAuthAccessToken,
			"DB should have the new access token despite context cancellation")
		require.NotEqual(t, oldRefreshToken, dbLink.OAuthRefreshToken,
			"DB should have the new refresh token despite context cancellation")
	})

	// SaveBeforeValidate_RateLimited tests the full path: refresh
	// succeeds, early save persists the token, validation returns
	// rate-limited optimistic true, and RefreshToken returns success
	// with no InvalidTokenError. Uses httptest.NewServer for the
	// validate endpoint to set rate-limit headers that the FakeIDP's
	// WithDynamicUserInfo hook cannot control.
	t.Run("SaveBeforeValidate_RateLimited", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)

		var refreshCalls atomic.Int64
		// rateLimitValidate returns 403 with rate-limit headers.
		rateLimitValidate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("X-RateLimit-Limit", "5000")
			w.WriteHeader(http.StatusForbidden)
		}))
		t.Cleanup(rateLimitValidate.Close)

		fake, config, link := setupOauth2Test(t, testConfig{
			FakeIDPOpts: []oidctest.FakeIDPOpt{
				oidctest.WithRefresh(func(_ string) error {
					refreshCalls.Add(1)
					return nil
				}),
			},
			ExternalAuthOpt: func(cfg *externalauth.Config) {
				cfg.Type = codersdk.EnhancedExternalAuthProviderGitHub.String()
				cfg.ValidateURL = rateLimitValidate.URL
			},
			DB: db,
		})

		// Use a real HTTP transport for non-IDP requests so the
		// validate request can reach the httptest server.
		ctx := oidc.ClientContext(context.Background(), fake.HTTPClient(&http.Client{
			Transport: http.DefaultTransport,
		}))

		oldAccessToken := link.OAuthAccessToken
		oldRefreshToken := link.OAuthRefreshToken

		// Expire the token to force a refresh.
		link.OAuthExpiry = expired

		// RefreshToken should succeed: the IDP refresh works, the
		// early save persists the token, and ValidateToken returns
		// (true, nil, nil) because the 403 has rate-limit headers.
		updated, err := config.RefreshToken(ctx, db, link)
		require.NoError(t, err, "RefreshToken should succeed when validation is rate-limited")
		require.Equal(t, int64(1), refreshCalls.Load(), "IDP refresh should have been called")
		require.NotEqual(t, oldAccessToken, updated.OAuthAccessToken,
			"returned token should be the new one from the refresh")

		// Verify the DB has the new token.
		dbLink, err := db.GetExternalAuthLink(context.Background(), database.GetExternalAuthLinkParams{
			ProviderID: link.ProviderID,
			UserID:     link.UserID,
		})
		require.NoError(t, err)
		require.Equal(t, updated.OAuthAccessToken, dbLink.OAuthAccessToken,
			"DB should have the refreshed access token")
		require.NotEqual(t, oldRefreshToken, dbLink.OAuthRefreshToken,
			"DB should have the new refresh token (old one was rotated by the IDP)")
	})

	// SaveBeforeValidate_DBError tests that when the early DB save
	// fails after a successful IDP refresh, the error is surfaced
	// as a non-InvalidTokenError. This is a degraded state (token
	// issued by IDP but not persisted), and callers should see a
	// real error, not a "please re-authenticate" prompt.
	t.Run("SaveBeforeValidate_DBError", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		mDB := dbmock.NewMockStore(ctrl)

		fake, config, link := setupOauth2Test(t, testConfig{
			FakeIDPOpts: []oidctest.FakeIDPOpt{
				oidctest.WithRefresh(func(_ string) error {
					return nil
				}),
			},
			ExternalAuthOpt: func(cfg *externalauth.Config) {
				cfg.Type = codersdk.EnhancedExternalAuthProviderGitHub.String()
			},
		})

		ctx := oidc.ClientContext(context.Background(), fake.HTTPClient(nil))
		link.OAuthExpiry = expired

		mDB.EXPECT().
			UpdateExternalAuthLink(gomock.Any(), gomock.Any()).
			Return(database.ExternalAuthLink{}, xerrors.New("db connection lost"))

		_, err := config.RefreshToken(ctx, mDB, link)
		require.Error(t, err)
		require.Contains(t, err.Error(), "persist refreshed token")
		require.False(t, externalauth.IsInvalidTokenError(err),
			"DB errors should not be treated as invalid token")
	})

	// OptimisticLockPreventsStaleOverwrite verifies that the
	// UpdateExternalAuthLinkRefreshToken WHERE clause prevents a
	// stale caller from overwriting a valid refresh token saved
	// by a concurrent winner.
	t.Run("OptimisticLockPreventsStaleOverwrite", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)

		fake, config, link := setupOauth2Test(t, testConfig{
			FakeIDPOpts: []oidctest.FakeIDPOpt{
				oidctest.WithRefresh(func(_ string) error {
					return nil
				}),
				oidctest.WithDynamicUserInfo(func(_ string) (jwt.MapClaims, error) {
					return jwt.MapClaims{}, nil
				}),
			},
			ExternalAuthOpt: func(cfg *externalauth.Config) {
				cfg.Type = codersdk.EnhancedExternalAuthProviderGitHub.String()
			},
			DB: db,
		})

		ctx := oidc.ClientContext(context.Background(), fake.HTTPClient(nil))

		// Snapshot the original tokens before any refresh.
		oldRefreshToken := link.OAuthRefreshToken

		// Expire the token to force a refresh.
		link.OAuthExpiry = expired

		// Caller A: refresh and save successfully.
		updated, err := config.RefreshToken(ctx, db, link)
		require.NoError(t, err)
		require.NotEqual(t, oldRefreshToken, updated.OAuthRefreshToken,
			"caller A should have a new refresh token")

		// Caller B had a stale read of the original link. It tries to
		// destroy the refresh token using the OLD refresh token in the
		// optimistic lock. Because caller A already wrote a different
		// refresh token, this WHERE clause matches nothing.
		err = db.UpdateExternalAuthLinkRefreshToken(ctx, database.UpdateExternalAuthLinkRefreshTokenParams{
			OauthRefreshFailureReason: "simulated failure from stale caller B",
			OAuthRefreshToken:         "",
			OAuthRefreshTokenKeyID:    "",
			UpdatedAt:                 dbtime.Now(),
			ProviderID:                link.ProviderID,
			UserID:                    link.UserID,
			OldOauthRefreshToken:      oldRefreshToken,
		})
		require.NoError(t, err, "optimistic lock write should not error, it is a no-op")

		// Verify DB still has caller A's valid token.
		dbLink, err := db.GetExternalAuthLink(context.Background(), database.GetExternalAuthLinkParams{
			ProviderID: link.ProviderID,
			UserID:     link.UserID,
		})
		require.NoError(t, err)
		require.Equal(t, updated.OAuthAccessToken, dbLink.OAuthAccessToken,
			"caller A's access token should still be in DB")
		require.Equal(t, updated.OAuthRefreshToken, dbLink.OAuthRefreshToken,
			"caller A's refresh token should still be in DB")
		require.Empty(t, dbLink.OauthRefreshFailureReason,
			"caller B's failure reason should not have been written")
	})
}

func TestValidateToken(t *testing.T) {
	t.Parallel()

	// These tests use httptest.NewServer to control response headers
	// (X-RateLimit-Remaining, Retry-After) that the FakeIDP's
	// WithDynamicUserInfo hook does not expose.

	newValidateConfig := func(t *testing.T, validateURL string) *externalauth.Config {
		t.Helper()
		f := promoauth.NewFactory(prometheus.NewRegistry())
		return &externalauth.Config{
			InstrumentedOAuth2Config: f.New("test-validate", &oauth2.Config{}),
			ID:                       "test-validate",
			Type:                     codersdk.EnhancedExternalAuthProviderGitHub.String(),
			ValidateURL:              validateURL,
		}
	}

	newToken := func() *oauth2.Token {
		return &oauth2.Token{
			AccessToken: "test-access-token",
			Expiry:      time.Now().Add(time.Hour),
		}
	}

	// RateLimitRemaining: 403 with X-RateLimit-Remaining: 0 should be
	// treated as rate-limited, not as an invalid token.
	t.Run("RateLimitRemaining", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("X-RateLimit-Limit", "5000")
			w.WriteHeader(http.StatusForbidden)
		}))
		t.Cleanup(srv.Close)

		config := newValidateConfig(t, srv.URL)
		valid, user, err := config.ValidateToken(context.Background(), newToken())

		require.NoError(t, err)
		assert.True(t, valid, "rate-limited 403 should be treated as optimistically valid")
		assert.Nil(t, user)
	})

	// RetryAfter: 403 with Retry-After header (secondary rate limit)
	// should be treated as rate-limited.
	t.Run("RetryAfter", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Retry-After", "60")
			w.WriteHeader(http.StatusForbidden)
		}))
		t.Cleanup(srv.Close)

		config := newValidateConfig(t, srv.URL)
		valid, user, err := config.ValidateToken(context.Background(), newToken())

		require.NoError(t, err)
		assert.True(t, valid, "rate-limited 403 with Retry-After should be optimistically valid")
		assert.Nil(t, user)
	})

	// Forbidden_WithNonZeroRateLimit: a 403 with non-zero
	// X-RateLimit-Remaining is a genuine token revocation, not a
	// rate limit. GitHub includes X-RateLimit-* headers on all
	// authenticated responses; the value matters, not the presence.
	t.Run("Forbidden_WithNonZeroRateLimit", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("X-RateLimit-Remaining", "5000")
			w.Header().Set("X-RateLimit-Limit", "5000")
			w.WriteHeader(http.StatusForbidden)
		}))
		t.Cleanup(srv.Close)

		config := newValidateConfig(t, srv.URL)
		valid, user, err := config.ValidateToken(context.Background(), newToken())

		require.NoError(t, err)
		assert.False(t, valid, "403 with non-zero rate limit remaining means token is invalid")
		assert.Nil(t, user)
	})

	// Forbidden_NoRateLimitHeaders: a plain 403 without rate-limit
	// headers is a genuine token revocation / permission error.
	t.Run("Forbidden_NoRateLimitHeaders", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		}))
		t.Cleanup(srv.Close)

		config := newValidateConfig(t, srv.URL)
		valid, user, err := config.ValidateToken(context.Background(), newToken())

		require.NoError(t, err)
		assert.False(t, valid, "plain 403 without rate-limit headers means token is invalid")
		assert.Nil(t, user)
	})

	// Unauthorized: 401 is always a token revocation regardless of
	// rate-limit headers.
	t.Run("Unauthorized", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))
		t.Cleanup(srv.Close)

		config := newValidateConfig(t, srv.URL)
		valid, user, err := config.ValidateToken(context.Background(), newToken())

		require.NoError(t, err)
		assert.False(t, valid, "401 always means token is invalid")
		assert.Nil(t, user)
	})

	// Unauthorized_WithRateLimitHeaders: 401 is always a revocation,
	// even when rate-limit headers are present. Locks the ordering
	// invariant that the 401 branch precedes the rate-limit check.
	t.Run("Unauthorized_WithRateLimitHeaders", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("Retry-After", "60")
			w.WriteHeader(http.StatusUnauthorized)
		}))
		t.Cleanup(srv.Close)

		config := newValidateConfig(t, srv.URL)
		valid, user, err := config.ValidateToken(context.Background(), newToken())

		require.NoError(t, err)
		assert.False(t, valid, "401 is always invalid, even with rate-limit headers")
		assert.Nil(t, user)
	})

	// TooManyRequests: 429 is treated optimistically, same as a
	// rate-limited 403. GitHub can return either status code for
	// rate limits.
	t.Run("TooManyRequests", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
		}))
		t.Cleanup(srv.Close)

		config := newValidateConfig(t, srv.URL)
		valid, user, err := config.ValidateToken(context.Background(), newToken())

		require.NoError(t, err)
		assert.True(t, valid, "429 should be treated as optimistically valid")
		assert.Nil(t, user)
	})
}

func TestRevokeToken(t *testing.T) {
	t.Parallel()

	t.Run("RevokeTokenRFC_OK", func(t *testing.T) {
		t.Parallel()
		var link database.ExternalAuthLink
		var config *externalauth.Config
		fake, config, link := setupOauth2Test(t, testConfig{
			FakeIDPOpts: []oidctest.FakeIDPOpt{
				oidctest.WithRevokeTokenRFC(func() (int, error) {
					return http.StatusOK, nil
				}),
			},
		})

		ctx := oidc.ClientContext(testutil.Context(t, testutil.WaitLong), fake.HTTPClient(nil))
		revoked, err := config.RevokeToken(ctx, link)
		require.NoError(t, err)
		require.True(t, revoked)
	})

	t.Run("RevokeTokenRFC_WrongBearer", func(t *testing.T) {
		t.Parallel()
		fake, config, link := setupOauth2Test(t, testConfig{
			FakeIDPOpts: []oidctest.FakeIDPOpt{
				oidctest.WithRevokeTokenRFC(func() (int, error) {
					return http.StatusOK, nil
				}),
			},
		})

		link.OAuthAccessToken += "wrong_token"
		ctx := oidc.ClientContext(testutil.Context(t, testutil.WaitLong), fake.HTTPClient(nil))
		revoked, err := config.RevokeToken(ctx, link)
		require.Error(t, err)
		require.Contains(t, err.Error(), "token validation failed")
		require.False(t, revoked)
	})

	t.Run("RevokeTokenRFC_WrongURL", func(t *testing.T) {
		t.Parallel()
		fake, config, link := setupOauth2Test(t, testConfig{
			FakeIDPOpts: []oidctest.FakeIDPOpt{
				oidctest.WithRevokeTokenRFC(func() (int, error) {
					return http.StatusOK, nil
				}),
			},
		})

		config.RevokeURL = "%"
		ctx := oidc.ClientContext(testutil.Context(t, testutil.WaitLong), fake.HTTPClient(nil))
		revoked, err := config.RevokeToken(ctx, link)
		require.Error(t, err)
		require.ErrorContains(t, err, "invalid URL escape")
		require.False(t, revoked)
	})

	t.Run("RevokeTokenRFC_Timeout", func(t *testing.T) {
		t.Parallel()
		revokeExited := make(chan bool, 1)
		testTimeout := make(chan bool, 1)
		handlerDone := make(chan bool)

		go func() {
			time.Sleep(5 * time.Second)
			testTimeout <- true
		}()

		fake, config, link := setupOauth2Test(t, testConfig{
			FakeIDPOpts: []oidctest.FakeIDPOpt{
				oidctest.WithRevokeTokenRFC(func() (int, error) {
					defer func() {
						handlerDone <- true
					}()

					select {
					case <-testTimeout:
						t.Error("test timeout reached before context timeout")
						return http.StatusOK, nil
					case <-revokeExited:
						return http.StatusOK, nil
					}
				}),
				oidctest.WithServing(),
			},
		})

		ctx := oidc.ClientContext(testutil.Context(t, testutil.WaitLong), fake.HTTPClient(nil))
		config.RevokeTimeout = time.Millisecond * 10
		revoked, err := config.RevokeToken(ctx, link)
		revokeExited <- true
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.False(t, revoked)
		_ = testutil.RequireReceive(ctx, t, handlerDone)
	})

	t.Run("RevokeTokenGitHub_OK", func(t *testing.T) {
		t.Parallel()
		clientID := "clientID"
		clientSecret := "clientSecret"
		fake, config, link := setupOauth2Test(t, testConfig{
			FakeIDPOpts: []oidctest.FakeIDPOpt{
				oidctest.WithRevokeTokenGitHub(func() (int, error) {
					return http.StatusNoContent, nil
				}),
				oidctest.WithStaticCredentials(clientID, clientSecret),
				oidctest.WithServing(),
			},
		})

		config.Type = codersdk.EnhancedExternalAuthProviderGitHub.String()
		config.ClientID = clientID
		config.ClientSecret = clientSecret
		ctx := oidc.ClientContext(testutil.Context(t, testutil.WaitLong), fake.HTTPClient(nil))
		revoked, err := config.RevokeToken(ctx, link)
		require.NoError(t, err)
		require.True(t, revoked)
	})

	t.Run("RevokeTokenGitHub_WrongAuth", func(t *testing.T) {
		t.Parallel()
		clientID := "clientID"
		clientSecret := "clientSecret"
		fake, config, link := setupOauth2Test(t, testConfig{
			FakeIDPOpts: []oidctest.FakeIDPOpt{
				oidctest.WithRevokeTokenGitHub(func() (int, error) {
					return http.StatusNoContent, nil
				}),
				oidctest.WithStaticCredentials(clientID, clientSecret),
				oidctest.WithServing(),
			},
		})

		config.Type = codersdk.EnhancedExternalAuthProviderGitHub.String()
		config.ClientID = clientID + "bad"
		config.ClientSecret = clientSecret
		ctx := oidc.ClientContext(testutil.Context(t, testutil.WaitLong), fake.HTTPClient(nil))
		revoked, err := config.RevokeToken(ctx, link)
		require.Error(t, err)
		require.Contains(t, err.Error(), "basic auth failed")
		require.False(t, revoked)
	})
}

func TestExchangeWithClientSecret(t *testing.T) {
	t.Parallel()
	instrument := promoauth.NewFactory(prometheus.NewRegistry())
	// This ensures a provider that requires the custom
	// client secret exchange works.
	configs, err := externalauth.ConvertConfig(instrument, []codersdk.ExternalAuthConfig{{
		// JFrog just happens to require this custom type.

		Type:         codersdk.EnhancedExternalAuthProviderJFrog.String(),
		ClientID:     "id",
		ClientSecret: "secret",
	}}, &url.URL{})
	require.NoError(t, err)
	config := configs[0]

	client := &http.Client{
		Transport: roundTripper(func(req *http.Request) (*http.Response, error) {
			require.Equal(t, "Bearer secret", req.Header.Get("Authorization"))
			rec := httptest.NewRecorder()
			rec.WriteHeader(http.StatusOK)
			body, err := json.Marshal(&oauth2.Token{
				AccessToken: "bananas",
			})
			if err != nil {
				return nil, err
			}
			_, err = rec.Write(body)
			return rec.Result(), err
		}),
	}

	_, err = config.Exchange(context.WithValue(context.Background(), oauth2.HTTPClient, client), "code")
	require.NoError(t, err)
}

func TestTokenRevocationResponseOk(t *testing.T) {
	t.Parallel()

	ghType := codersdk.EnhancedExternalAuthProviderGitHub.String()
	rfcType := codersdk.EnhancedExternalAuthProviderAzureDevops.String()
	tests := []struct {
		name string
		conf *externalauth.Config
		resp http.Response
		want bool
	}{
		{
			name: "GH_bad",
			conf: &externalauth.Config{Type: ghType},
			resp: http.Response{StatusCode: http.StatusOK},
			want: false,
		},
		{
			name: "GH_ok",
			conf: &externalauth.Config{Type: ghType},
			resp: http.Response{StatusCode: http.StatusNoContent},
			want: true,
		},
		{
			name: "RFC_ok",
			conf: &externalauth.Config{Type: rfcType},
			resp: http.Response{StatusCode: http.StatusOK},
			want: true,
		},
		{
			name: "RFC_bad",
			conf: &externalauth.Config{Type: rfcType},
			resp: http.Response{StatusCode: http.StatusNoContent},
			want: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := tc.conf.TokenRevocationResponseOk(&tc.resp)
			if tc.want != got {
				t.Errorf("unexpected response success, got: %v want: %v", got, tc.want)
			}
		})
	}
}

func TestConvertYAML(t *testing.T) {
	t.Parallel()

	instrument := promoauth.NewFactory(prometheus.NewRegistry())
	for _, tc := range []struct {
		Name   string
		Input  []codersdk.ExternalAuthConfig
		Output []*externalauth.Config
		Error  string
	}{{
		Name: "InvalidID",
		Input: []codersdk.ExternalAuthConfig{{
			Type: string(codersdk.EnhancedExternalAuthProviderGitHub),
			ID:   "$hi$",
		}},
		Error: "doesn't have a valid id",
	}, {
		Name: "NoClientID",
		Input: []codersdk.ExternalAuthConfig{{
			Type: string(codersdk.EnhancedExternalAuthProviderGitHub),
		}},
		Error: "client_id must be provided",
	}, {
		Name: "DuplicateType",
		Input: []codersdk.ExternalAuthConfig{{
			Type:         string(codersdk.EnhancedExternalAuthProviderGitHub),
			ClientID:     "example",
			ClientSecret: "example",
		}, {
			Type:         string(codersdk.EnhancedExternalAuthProviderGitHub),
			ClientID:     "example-2",
			ClientSecret: "example-2",
		}},
		Error: "multiple github external auth providers provided",
	}, {
		Name: "InvalidRegex",
		Input: []codersdk.ExternalAuthConfig{{
			Type:         string(codersdk.EnhancedExternalAuthProviderGitHub),
			ClientID:     "example",
			ClientSecret: "example",
			Regex:        `\K`,
		}},
		Error: "compile regex for external auth provider",
	}, {
		Name: "NoDeviceURL",
		Input: []codersdk.ExternalAuthConfig{{
			Type:         string(codersdk.EnhancedExternalAuthProviderGitLab),
			ClientID:     "example",
			ClientSecret: "example",
			DeviceFlow:   true,
		}},
		Error: "device auth url must be provided",
	}} {
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			output, err := externalauth.ConvertConfig(instrument, tc.Input, &url.URL{})
			if tc.Error != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.Error)
				return
			}
			require.Equal(t, tc.Output, output)
		})
	}

	t.Run("CustomScopesAndEndpoint", func(t *testing.T) {
		t.Parallel()
		config, err := externalauth.ConvertConfig(instrument, []codersdk.ExternalAuthConfig{{
			Type:         string(codersdk.EnhancedExternalAuthProviderGitLab),
			ClientID:     "id",
			ClientSecret: "secret",
			AuthURL:      "https://auth.com",
			TokenURL:     "https://token.com",
			Scopes:       []string{"read"},
		}}, &url.URL{})
		require.NoError(t, err)
		require.Equal(t, "https://auth.com?client_id=id&redirect_uri=%2Fexternal-auth%2Fgitlab%2Fcallback&response_type=code&scope=read", config[0].AuthCodeURL(""))
	})

	t.Run("RevokeTimeoutSet", func(t *testing.T) {
		t.Parallel()
		configs, err := externalauth.ConvertConfig(instrument, []codersdk.ExternalAuthConfig{{
			Type:         string(codersdk.EnhancedExternalAuthProviderGitLab),
			ClientID:     "id",
			ClientSecret: "secret",
		}}, &url.URL{})
		require.NoError(t, err)
		require.Equal(t, 10*time.Second, configs[0].RevokeTimeout)
	})
}

// TestConstantQueryParams verifies a constant query parameter can be set in the
// "authenticate" url for external auth applications, and it will be carried forward
// to actual auth requests.
// This unit test was specifically created for Auth0 which can set an
// audience query parameter in it's /authorize endpoint.
func TestConstantQueryParams(t *testing.T) {
	t.Parallel()
	const constantQueryParamKey = "audience"
	const constantQueryParamValue = "foobar"
	constantQueryParam := fmt.Sprintf("%s=%s", constantQueryParamKey, constantQueryParamValue)
	fake, config, _ := setupOauth2Test(t, testConfig{
		FakeIDPOpts: []oidctest.FakeIDPOpt{
			oidctest.WithMiddlewares(func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
					if strings.Contains(request.URL.Path, "authorize") {
						// Assert has the audience query param
						assert.Equal(t, request.URL.Query().Get(constantQueryParamKey), constantQueryParamValue)
					}
					next.ServeHTTP(writer, request)
				})
			}),
		},
		CoderOIDCConfigOpts: []func(cfg *coderd.OIDCConfig){
			func(cfg *coderd.OIDCConfig) {
				// Include a constant query parameter.
				authURL, err := url.Parse(cfg.OAuth2Config.(*oauth2.Config).Endpoint.AuthURL)
				require.NoError(t, err)

				authURL.RawQuery = url.Values{constantQueryParamKey: []string{constantQueryParamValue}}.Encode()
				cfg.OAuth2Config.(*oauth2.Config).Endpoint.AuthURL = authURL.String()
				require.Contains(t, cfg.OAuth2Config.(*oauth2.Config).Endpoint.AuthURL, constantQueryParam)
				cfg.PKCEMethods = []promoauth.Oauth2PKCEChallengeMethod{promoauth.PKCEChallengeMethodSha256}
			},
		},
	})

	callbackCalled := false
	fake.SetCoderdCallbackHandler(func(writer http.ResponseWriter, request *http.Request) {
		// Just record the callback was hit, and the auth succeeded.
		callbackCalled = true
	})

	// Verify the AuthURL endpoint contains the constant query parameter and is a valid URL.
	// It should look something like:
	//	http://127.0.0.1:<port>>/oauth2/authorize?
	//		audience=foobar&
	//		client_id=d<uuid>&
	//		redirect_uri=<redirect>&
	//		response_type=code&
	//		scope=openid+email+profile&
	//		state=state
	const state = "state"
	rawAuthURL := config.AuthCodeURL(state)
	// Parsing the url is not perfect. It allows imperfections like the query
	// params having 2 question marks '?a=foo?b=bar'.
	// So use it to validate, then verify the raw url is as expected.
	authURL, err := url.Parse(rawAuthURL)
	require.NoError(t, err)
	require.Equal(t, authURL.Query().Get(constantQueryParamKey), constantQueryParamValue)
	// We are not using a real server, so it fakes https://coder.com
	require.Equal(t, authURL.Scheme, "https")
	// Validate the raw URL.
	// Double check only 1 '?' exists. Url parsing allows multiple '?' in the query string.
	require.Equal(t, strings.Count(rawAuthURL, "?"), 1)

	// Actually run an auth request. Although it says OIDC, the flow is the same
	// for oauth2.
	//nolint:bodyclose
	resp := fake.OIDCCallback(t, state, jwt.MapClaims{})
	require.True(t, callbackCalled)
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

type testConfig struct {
	FakeIDPOpts         []oidctest.FakeIDPOpt
	CoderOIDCConfigOpts []func(cfg *coderd.OIDCConfig)
	ExternalAuthOpt     func(cfg *externalauth.Config)
	// If DB is passed in, the link will be inserted into the DB.
	DB database.Store
}

// setupTest will configure a fake IDP and a externalauth.Config for testing.
// The Fake's userinfo endpoint is used for validating tokens.
// No http servers are started so use the fake IDP's HTTPClient to make requests.
// The returned token is a fully valid token for the IDP. Feel free to manipulate it
// to test different scenarios.
func setupOauth2Test(t *testing.T, settings testConfig) (*oidctest.FakeIDP, *externalauth.Config, database.ExternalAuthLink) {
	t.Helper()

	if settings.ExternalAuthOpt == nil {
		settings.ExternalAuthOpt = func(_ *externalauth.Config) {}
	}

	const providerID = "test-idp"
	fake := oidctest.NewFakeIDP(t,
		append([]oidctest.FakeIDPOpt{oidctest.WithPKCE()}, settings.FakeIDPOpts...)...,
	)

	f := promoauth.NewFactory(prometheus.NewRegistry())
	cid, cs := fake.AppCredentials()
	config := &externalauth.Config{
		InstrumentedOAuth2Config: f.New("test-oauth2",
			fake.OIDCConfig(t, nil, settings.CoderOIDCConfigOpts...)),
		ID:                            providerID,
		ClientID:                      cid,
		ClientSecret:                  cs,
		ValidateURL:                   fake.WellknownConfig().UserInfoURL,
		RevokeURL:                     fake.WellknownConfig().RevokeURL,
		RevokeTimeout:                 1 * time.Second,
		CodeChallengeMethodsSupported: []promoauth.Oauth2PKCEChallengeMethod{promoauth.PKCEChallengeMethodSha256},
	}
	settings.ExternalAuthOpt(config)

	oauthToken, err := fake.GenerateAuthenticatedToken(jwt.MapClaims{
		"email": "test@coder.com",
	})
	require.NoError(t, err)

	now := time.Now()
	link := database.ExternalAuthLink{
		ProviderID:        providerID,
		UserID:            uuid.New(),
		CreatedAt:         now,
		UpdatedAt:         now,
		OAuthAccessToken:  oauthToken.AccessToken,
		OAuthRefreshToken: oauthToken.RefreshToken,
		// The caller can manually expire this if they want.
		OAuthExpiry: now.Add(time.Hour),
	}

	if settings.DB != nil {
		// Feel free to insert additional things like the user, etc if required.
		link, err = settings.DB.InsertExternalAuthLink(context.Background(), database.InsertExternalAuthLinkParams{
			ProviderID:        link.ProviderID,
			UserID:            link.UserID,
			CreatedAt:         link.CreatedAt,
			UpdatedAt:         link.UpdatedAt,
			OAuthAccessToken:  link.OAuthAccessToken,
			OAuthRefreshToken: link.OAuthRefreshToken,
			OAuthExpiry:       link.OAuthExpiry,
		})
		require.NoError(t, err, "failed to insert link into DB")
	}

	return fake, config, link
}

func TestApplyDefaultsToConfig_CaseInsensitive(t *testing.T) {
	t.Parallel()

	instrument := promoauth.NewFactory(prometheus.NewRegistry())
	accessURL, err := url.Parse("https://coder.example.com")
	require.NoError(t, err)

	for _, tc := range []struct {
		Name string
		Type string
	}{
		{Name: "GitHub", Type: "GitHub"},
		{Name: "GITLAB", Type: "GITLAB"},
		{Name: "Gitea", Type: "Gitea"},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			configs, err := externalauth.ConvertConfig(
				instrument,
				[]codersdk.ExternalAuthConfig{{
					Type:         tc.Type,
					ClientID:     "test-id",
					ClientSecret: "test-secret",
				}},
				accessURL,
			)
			require.NoError(t, err)
			require.Len(t, configs, 1)
			// Defaults should have been applied despite mixed-case Type.
			assert.NotEmpty(t, configs[0].AuthCodeURL("state"), "auth URL should be populated from defaults")
		})
	}
}

type roundTripper func(req *http.Request) (*http.Response, error)

func (r roundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return r(req)
}

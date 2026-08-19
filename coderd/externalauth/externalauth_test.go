package externalauth_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/oauth2"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/singleflight"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogjson"
	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/coderdtest/oidctest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/externalauth"
	"github.com/coder/coder/v2/coderd/externalauth/gitprovider"
	"github.com/coder/coder/v2/coderd/promoauth"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestConfigGitMemoizesProvider(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)

	const etag = `"config-git-memo-etag"`
	var conditionalRequests atomic.Int64

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if inm := r.Header.Get("If-None-Match"); inm != "" {
			conditionalRequests.Add(1)
			assert.Equal(t, etag, inm)
			w.Header().Set("ETag", etag)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("ETag", etag)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	cfg := &externalauth.Config{
		Type:       string(codersdk.EnhancedExternalAuthProviderGitHub),
		APIBaseURL: srv.URL + "/api/v3",
		HTTPClient: srv.Client(),
	}

	gp1, err := cfg.Git()
	require.NoError(t, err)
	require.NotNil(t, gp1)

	branch := gitprovider.BranchRef{Owner: "owner", Repo: "repo", Branch: "feat"}

	// Cold poll: populates the provider's ETag cache.
	_, err = gp1.ResolveBranchPullRequest(ctx, "test-token", branch)
	require.NoError(t, err)

	// Re-resolve the provider, as the worker does on every poll.
	gp2, err := cfg.Git()
	require.NoError(t, err)
	require.NotNil(t, gp2)
	assert.Same(t, gp1, gp2, "Git must return the same provider instance so its ETag cache survives across calls")

	_, err = gp2.ResolveBranchPullRequest(ctx, "test-token", branch)
	require.NoError(t, err)

	assert.Equal(t, int64(1), conditionalRequests.Load(), "second poll should have revalidated with If-None-Match using the cache from the first poll")
}

func TestConfigGitRetriesOnConstructorError(t *testing.T) {
	t.Parallel()

	cfg := &externalauth.Config{
		Type:       string(codersdk.EnhancedExternalAuthProviderGitLab),
		APIBaseURL: "://invalid",
	}

	_, err1 := cfg.Git()
	require.Error(t, err1)

	_, err2 := cfg.Git()
	require.Error(t, err2)
	// A memoized error would be the same instance; a retried
	// construction produces a fresh error each call.
	require.NotErrorIs(t, err2, err1, "construction errors must be retried, not memoized")
}

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
				// Should abort before entering the group.
				cfg.RefreshGroup = nil
			},
			ExternalAuthLinkOpts: func(link *database.ExternalAuthLink) {
				link.OAuthExpiry = expired
			},
		})

		mDB := mockDB(t)

		// There should be no database calls since we return early.
		ctx := oidc.ClientContext(testutil.Context(t, testutil.WaitLong), fake.HTTPClient(nil))
		_, err := config.RefreshToken(ctx, mDB, link)
		require.Error(t, err)
		require.True(t, externalauth.IsInvalidTokenError(err))
		require.Contains(t, err.Error(), "token expired and refreshing is disabled")
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

		mDB := mockDB(t)

		ctx := oidc.ClientContext(testutil.Context(t, testutil.WaitLong), fake.HTTPClient(nil))

		// Zero time used
		link.OAuthExpiry = time.Time{}

		// Since the token is not expired, no refresh lease will be acquired and
		// it will only be validated.
		_, err := config.RefreshToken(ctx, mDB, link)
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
			RefreshGroup: new(singleflight.Group),
		}

		link := database.ExternalAuthLink{
			OAuthExpiry: expired,
		}

		mDB := mockDB(t, withLease(link))

		ctx := testutil.Context(t, testutil.WaitLong)
		_, err := config.RefreshToken(ctx, mDB, link)
		require.Error(t, err)
		require.True(t, externalauth.IsInvalidTokenError(err))
		require.Contains(t, err.Error(), "failure")
	})

	t.Run("ValidateServerError", func(t *testing.T) {
		t.Parallel()

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

		mDB := mockDB(t,
			withLease(link),
			withUpdatePassthrough())

		ctx := oidc.ClientContext(testutil.Context(t, testutil.WaitLong), fake.HTTPClient(nil))
		link.OAuthExpiry = expired

		_, err := config.RefreshToken(ctx, mDB, link)
		require.ErrorContains(t, err, staticError)
		// Unsure if this should be the correct behavior. It's an invalid token because
		// 'ValidateToken()' failed with a runtime error. This was the previous behavior,
		// so not going to change it.
		require.False(t, externalauth.IsInvalidTokenError(err))
		require.True(t, validated, "token should have been attempted to be validated")
	})

	// RefreshRetries tests that refresh token external retry behavior works as
	// expected.  If a refresh token fails because the token itself is invalid, no
	// more refresh attempts should ever happen. An invalid refresh token does not
	// magically become valid at some point in the future.
	//
	// Internal retries are disabled in this subtest via a negative
	// RefreshRetryTimeout so each RefreshToken call results in exactly one
	// IDP refresh attempt. The RefreshTokenWithBackoff subtest covers the
	// retry-with-backoff path.
	t.Run("RefreshRetries", func(t *testing.T) {
		t.Parallel()

		var refreshErr *oauth2.RetrieveError
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
			ExternalAuthOpt: func(cfg *externalauth.Config) {
				// Negative timeout disables retries (1 IDP call per RefreshToken).
				// A tiny positive timeout is unreliable on coarse-clock platforms
				// (Windows).
				cfg.RefreshRetryTimeout = -1
			},
			ExternalAuthLinkOpts: func(link *database.ExternalAuthLink) {
				link.OAuthExpiry = expired
			},
		})

		// Allow acquiring and releasing the lease for all the temporary error
		// attempts, the bad refresh token attempt, and then finally the last
		// attempt with no refresh token set.
		mDB := mockDB(t,
			withLease(link),
			withLease(link),
			withLease(link),
			withLease(link))

		ctx := oidc.ClientContext(testutil.Context(t, testutil.WaitLong), fake.HTTPClient(nil))

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

		// The final attempt will be a permanent error and we should see the
		// database update with the error.  Need to extract it from the mock call
		// like this rather than use the returned link from RefreshToken as it does
		// not return the updated link in the error case.
		mDB.EXPECT().UpdateExternalAuthLink(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, p database.UpdateExternalAuthLinkParams) (database.ExternalAuthLink, error) {
				link = database.ExternalAuthLink{
					ProviderID:       p.ProviderID,
					UserID:           p.UserID,
					OAuthAccessToken: p.OAuthAccessToken,
					// This should be zeroed out.
					OAuthRefreshToken:         p.OAuthRefreshToken,
					OAuthExpiry:               p.OAuthExpiry,
					OauthRefreshFailureReason: p.OauthRefreshFailureReason,
				}
				return link, nil
			}).Times(1)

		refreshErr = &oauth2.RetrieveError{ // github error
			Response: &http.Response{
				StatusCode: http.StatusOK,
			},
			ErrorCode: "bad_refresh_token",
		}

		zeroedLink, err := config.RefreshToken(ctx, mDB, link)
		require.Error(t, err)
		totalRefreshes++
		require.True(t, externalauth.IsInvalidTokenError(err))
		require.Equal(t, refreshCount, totalRefreshes)
		// Although the fully updated link with the error is not returned, it does
		// zero out the token.
		require.Empty(t, zeroedLink.OAuthRefreshToken)

		// Databasae link should have a reason and no refresh token.
		require.NotEmpty(t, link.OauthRefreshFailureReason)
		require.Empty(t, link.OAuthRefreshToken)

		// Once more, this time with the zeroed-out refresh token due to the bad
		// refresh error.
		withLease(link)(mDB)

		// When the refresh token is empty, no api calls should be made
		_, err = config.RefreshToken(ctx, mDB, link)
		require.Error(t, err)
		require.True(t, externalauth.IsInvalidTokenError(err))
		require.Equal(t, refreshCount, totalRefreshes)
		// It should return the original cached error, not "no refresh token".
		require.ErrorContains(t, err, "a long while ago")
		require.ErrorContains(t, err, "bad_refresh_token")
	})

	// RefreshTokenWithBackoff tests that refreshes which fail with transient
	// errors (HTTP 5xx, 429, network errors) are retried with exponential
	// backoff so a temporary upstream glitch does not force users to
	// re-authenticate. After enough successful retries, RefreshToken should
	// return a valid token without surfacing the transient error.
	t.Run("RefreshTokenWithBackoff", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)

		const failuresBeforeSuccess = 3
		var refreshCalls atomic.Int64
		fake, config, link := setupOauth2Test(t, testConfig{
			FakeIDPOpts: []oidctest.FakeIDPOpt{
				oidctest.WithRefresh(func(_ string) error {
					// Fail the first N attempts with a transient 5xx, then succeed.
					if refreshCalls.Add(1) <= failuresBeforeSuccess {
						return &oauth2.RetrieveError{
							Response:  &http.Response{StatusCode: http.StatusInternalServerError},
							ErrorCode: "server_error",
						}
					}
					return nil
				}),
			},
			ExternalAuthOpt: func(cfg *externalauth.Config) {
				cfg.Type = codersdk.EnhancedExternalAuthProviderGitHub.String()
				// Tight backoffs keep the test fast.
				cfg.RefreshRetryInitialBackoff = time.Millisecond
				cfg.RefreshRetryMaxBackoff = 5 * time.Millisecond
				cfg.RefreshRetryTimeout = 5 * time.Second
			},
			DB: db,
			ExternalAuthLinkOpts: func(link *database.ExternalAuthLink) {
				link.OAuthExpiry = expired
			},
		})

		ctx := oidc.ClientContext(testutil.Context(t, testutil.WaitLong), fake.HTTPClient(nil))
		oldAccessToken := link.OAuthAccessToken

		updated, err := config.RefreshToken(ctx, db, link)
		require.NoError(t, err, "transient errors should be retried until success")
		require.Equal(t, int64(failuresBeforeSuccess+1), refreshCalls.Load(),
			"refresh should have been retried until the IDP returned success")
		require.NotEqual(t, oldAccessToken, updated.OAuthAccessToken,
			"a new access token should have been issued")
	})

	// RefreshTokenBackoffPermanentError verifies that errors classified as
	// permanent by isFailedRefresh (e.g. "bad_refresh_token") are not
	// retried. Retrying a permanent failure wastes the refresh quota and,
	// on providers with single-use refresh tokens, can mask a legitimate
	// concurrent winner with repeated "bad_refresh_token" responses.
	t.Run("RefreshTokenBackoffPermanentError", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)

		var refreshCalls atomic.Int64
		fake, config, link := setupOauth2Test(t, testConfig{
			FakeIDPOpts: []oidctest.FakeIDPOpt{
				oidctest.WithRefresh(func(_ string) error {
					refreshCalls.Add(1)
					return &oauth2.RetrieveError{
						Response:  &http.Response{StatusCode: http.StatusOK},
						ErrorCode: "bad_refresh_token",
					}
				}),
			},
			ExternalAuthOpt: func(cfg *externalauth.Config) {
				cfg.Type = codersdk.EnhancedExternalAuthProviderGitHub.String()
				// Generous backoff: a regression that incorrectly retried
				// would re-run the failing refresh many times and the test
				// would fail on the call-count assertion below.
				cfg.RefreshRetryInitialBackoff = time.Millisecond
				cfg.RefreshRetryMaxBackoff = 5 * time.Millisecond
				cfg.RefreshRetryTimeout = time.Second
			},
			ExternalAuthLinkOpts: func(link *database.ExternalAuthLink) {
				link.OAuthExpiry = expired
			},
			DB: db,
		})

		ctx := oidc.ClientContext(testutil.Context(t, testutil.WaitLong), fake.HTTPClient(nil))
		_, err := config.RefreshToken(ctx, db, link)
		require.Error(t, err)
		require.True(t, externalauth.IsInvalidTokenError(err))
		require.Equal(t, int64(1), refreshCalls.Load(),
			"permanent failures should not be retried")
	})

	// ConcurrentRefreshGroup tests that when requests try to refresh a token
	// while another request is pending, they wait on the first caller and share
	// the result instead of all attempting to perform the refresh.
	t.Run("ConcurrentRefreshGroup", func(t *testing.T) {
		t.Parallel()

		parallelRequests := 5
		ch := make(chan string)
		refreshedToken := &oauth2.Token{
			AccessToken:  "winner-access-token",
			RefreshToken: "winner-refresh-token",
			Expiry:       time.Now().Add(time.Hour),
		}

		var refreshCalls atomic.Int64
		config := &externalauth.Config{
			InstrumentedOAuth2Config: &testutil.OAuth2Config{
				// The first call to refresh will succeed and all others will fail.  The
				// first will wait for all callers to join the group before returning.
				TokenSourceFunc: func() (*oauth2.Token, error) {
					if refreshCalls.Add(1) == 1 {
						// Wait for all the other calls to be subscribed, to prevent
						// the test from flaking.
						subscribed := 1
						for {
							<-ch
							subscribed++
							if subscribed >= parallelRequests {
								return refreshedToken, nil
							}
						}
					}
					return nil, xerrors.New("bad_refresh_token")
				},
			},
			RefreshGroup: &group{
				notify: ch,
			},
		}

		link := database.ExternalAuthLink{OAuthExpiry: expired}

		// Only one call should try to acquire and release a lease and only one call
		// should update the link.
		mDB := mockDB(t,
			withLease(link),
			withUpdatePassthrough())

		// When we fire off all requests in parallel...
		ctx := testutil.Context(t, testutil.WaitLong)
		var eg errgroup.Group
		results := make([]database.ExternalAuthLink, parallelRequests)
		for i := range parallelRequests {
			eg.Go(func() error {
				result, err := config.RefreshToken(ctx, mDB, link)
				results[i] = result
				return err
			})
		}

		// No call should error.
		err := eg.Wait()
		require.NoError(t, err)

		// All calls should have picked up the winning token.
		for i := range parallelRequests {
			require.Equal(t, refreshedToken.AccessToken, results[i].OAuthAccessToken)
			require.Equal(t, refreshedToken.RefreshToken, results[i].OAuthRefreshToken)
		}

		// Only one refresh call should have actually been made.
		require.Equal(t, int64(1), refreshCalls.Load())
	})

	// ConcurrentRefreshRace tests what happens a request reads the refresh token
	// from the database, then another request finishes and updates the token and
	// releases the refresh group lock before this request can join that group.
	//
	// This request would fail with `bad_refresh_token` for providers that have
	// single-use refresh tokens.  It should instead re-read the token from the
	// database to check whether the token was updated by another request and
	// returns that rather than incorrectly recording in the database that the
	// request failed.
	t.Run("ConcurrentRefreshRace", func(t *testing.T) {
		t.Parallel()

		fake, config, link := setupOauth2Test(t, testConfig{
			FakeIDPOpts: []oidctest.FakeIDPOpt{
				oidctest.WithRefresh(func(_ string) error {
					return xerrors.New("should not reach this")
				}),
			},
			ExternalAuthLinkOpts: func(link *database.ExternalAuthLink) {
				link.OAuthExpiry = expired
			},
		})

		ctx := oidc.ClientContext(testutil.Context(t, testutil.WaitLong), fake.HTTPClient(nil))

		winnerLink := link
		winnerLink.OAuthRefreshToken = "winner-refresh-token"
		winnerLink.OAuthAccessToken = "winner-access-token"

		// Simulate that another caller updated the link.
		// UpdateExternalAuthLinkRefreshToken should NOT be called because trying to
		// get the lease detected the nearly-concurrent refresh.  It should instead
		// return the winning token.
		mDB := mockDB(t, withLease(winnerLink))
		result, err := config.RefreshToken(ctx, mDB, link)
		require.NoError(t, err, "loser should succeed using the winner's token")
		require.Equal(t, winnerLink.OAuthAccessToken, result.OAuthAccessToken)
		require.Equal(t, winnerLink.OAuthRefreshToken, result.OAuthRefreshToken)
	})

	// ConcurrentContextCancel tests that if one request is canceled, it does not
	// cancel other requests waiting on it.
	t.Run("ConcurrentContextCanceled", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		parallelRequests := 5
		ch := make(chan string)

		var refreshCalls atomic.Int64
		ctx := testutil.Context(t, testutil.WaitLong)
		cancelOnRefresh, cancel := context.WithCancel(ctx)
		defer cancel()

		// Use to know when the first call has started the group, so we know which
		// context we can cancel.
		listening := make(chan struct{})

		fake, config, link := setupOauth2Test(t, testConfig{
			FakeIDPOpts: []oidctest.FakeIDPOpt{
				oidctest.WithRefresh(func(_ string) error {
					if refreshCalls.Add(1) == 1 {
						close(listening)
						// Wait for all the other calls to be subscribed, to prevent
						// the test from flaking.
						subscribed := 1
						for {
							<-ch
							subscribed++
							if subscribed >= parallelRequests {
								// Cancel the parent context after refresh succeeds
								// but before the DB save and validation.
								cancel()
								return nil
							}
						}
					}
					return xerrors.New("should not reach this")
				}),
				oidctest.WithDynamicUserInfo(func(_ string) (jwt.MapClaims, error) {
					return jwt.MapClaims{}, nil
				}),
			},
			ExternalAuthOpt: func(cfg *externalauth.Config) {
				cfg.Type = codersdk.EnhancedExternalAuthProviderGitHub.String()
				cfg.RefreshGroup = &group{notify: ch}
			},
			DB: db,
			ExternalAuthLinkOpts: func(link *database.ExternalAuthLink) {
				link.OAuthExpiry = expired
			},
		})

		oldAccessToken := link.OAuthAccessToken
		oldRefreshToken := link.OAuthRefreshToken

		var wg sync.WaitGroup
		// Start the first call with the cancelable context.
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx := oidc.ClientContext(cancelOnRefresh, fake.HTTPClient(nil))
			_, err := config.RefreshToken(ctx, db, link)
			assert.ErrorIs(t, err, context.Canceled)
		}()

		// Wait for it to start the group, to make sure the callback above is
		// canceling the right context (if we fire them all at once, any one of them
		// could start the group).
		<-listening

		// Now we can fire off the remaining requests.
		for range parallelRequests - 1 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				ctx := oidc.ClientContext(ctx, fake.HTTPClient(nil))
				result, err := config.RefreshToken(ctx, db, link)
				assert.NoError(t, err)
				assert.NotEqual(t, oldAccessToken, result.OAuthAccessToken)
				assert.NotEqual(t, oldRefreshToken, result.OAuthRefreshToken)
			}()
		}

		wg.Wait()

		// DB link should have been updated.
		dbLink, err := db.GetExternalAuthLink(ctx, database.GetExternalAuthLinkParams{
			ProviderID: link.ProviderID,
			UserID:     link.UserID,
		})
		require.NoError(t, err)
		require.NotEqual(t, oldAccessToken, dbLink.OAuthAccessToken,
			"DB should have the new access token despite context cancellation")
		require.NotEqual(t, oldRefreshToken, dbLink.OAuthRefreshToken,
			"DB should have the new refresh token despite context cancellation")

		// Only one refresh call should have actually been made.
		require.Equal(t, int64(1), refreshCalls.Load())
	})

	t.Run("LeaseAcquisitionError", func(t *testing.T) {
		t.Parallel()

		fake, config, link := setupOauth2Test(t, testConfig{
			FakeIDPOpts: []oidctest.FakeIDPOpt{
				oidctest.WithRefresh(func(_ string) error {
					return nil
				}),
			},
			ExternalAuthOpt: func(cfg *externalauth.Config) {},
			ExternalAuthLinkOpts: func(link *database.ExternalAuthLink) {
				link.OAuthExpiry = expired
			},
		})

		mDB := mockDB(t, withLeaseErrors(link, xerrors.New("acquire error"), nil))

		ctx := oidc.ClientContext(testutil.Context(t, testutil.WaitLong), fake.HTTPClient(nil))
		_, err := config.RefreshToken(ctx, mDB, link)
		require.Error(t, err)
		require.ErrorContains(t, err, "acquire error")
	})

	t.Run("ReturnsReleaseError", func(t *testing.T) {
		t.Parallel()

		fake, config, link := setupOauth2Test(t, testConfig{
			FakeIDPOpts: []oidctest.FakeIDPOpt{
				oidctest.WithRefresh(func(_ string) error {
					return nil
				}),
			},
			ExternalAuthOpt: func(cfg *externalauth.Config) {},
			ExternalAuthLinkOpts: func(link *database.ExternalAuthLink) {
				link.OAuthExpiry = expired
			},
		})

		mDB := mockDB(t,
			withLeaseErrors(link, nil, xerrors.New("release error")),
			withUpdatePassthrough())

		// Although the refresh was successful, an error is still returned due to
		// the release having failed.
		ctx := oidc.ClientContext(testutil.Context(t, testutil.WaitLong), fake.HTTPClient(nil))
		refreshed, err := config.RefreshToken(ctx, mDB, link)
		require.Error(t, err)
		require.ErrorContains(t, err, "release error")
		require.NotEqual(t, link.OAuthAccessToken, refreshed.OAuthAccessToken)
		require.NotEqual(t, link.OAuthRefreshToken, refreshed.OAuthRefreshToken)
	})

	t.Run("ReturnsCombinedWithReleaseError", func(t *testing.T) {
		t.Parallel()

		fake, config, link := setupOauth2Test(t, testConfig{
			FakeIDPOpts: []oidctest.FakeIDPOpt{
				oidctest.WithRefresh(func(_ string) error {
					return nil
				}),
			},
			ExternalAuthOpt: func(cfg *externalauth.Config) {},
			ExternalAuthLinkOpts: func(link *database.ExternalAuthLink) {
				link.OAuthExpiry = expired
			},
		})

		mDB := mockDB(t,
			withLeaseErrors(link, nil, xerrors.New("release error")),
			withUpdateError(xerrors.New("update error")))

		// Both the release and update errors should be returned.
		ctx := oidc.ClientContext(testutil.Context(t, testutil.WaitLong), fake.HTTPClient(nil))
		_, err := config.RefreshToken(ctx, mDB, link)
		require.Error(t, err)
		require.ErrorContains(t, err, "release error")
		require.ErrorContains(t, err, "update error")
	})

	// ValidateFailure tests if the token is no longer valid with a 401 response.
	t.Run("ValidateFailure", func(t *testing.T) {
		t.Parallel()

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

		ctx := oidc.ClientContext(testutil.Context(t, testutil.WaitLong), fake.HTTPClient(nil))
		link.OAuthExpiry = expired

		mDB := mockDB(t,
			withLease(link),
			withUpdatePassthrough())

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
			ExternalAuthLinkOpts: func(link *database.ExternalAuthLink) {
				// Unlimited lifetime, this is what GitHub returns tokens as.
				link.OAuthExpiry = time.Time{}
			},
		})

		// Since the token is not expired, no lock or refresh lease will be acquired
		// and it will only be validated.
		mDB := mockDB(t)

		ctx := oidc.ClientContext(testutil.Context(t, testutil.WaitLong), fake.HTTPClient(nil))
		_, err := config.RefreshToken(ctx, mDB, link)
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

		// Since the token is not expired, no lock or refresh lease will be acquired
		// and it will only be validated.
		mDB := mockDB(t)

		ctx := oidc.ClientContext(testutil.Context(t, testutil.WaitLong), fake.HTTPClient(nil))
		_, err := config.RefreshToken(ctx, mDB, link)
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
			ExternalAuthLinkOpts: func(link *database.ExternalAuthLink) {
				link.OAuthExpiry = expired
			},
		})

		ctx := oidc.ClientContext(testutil.Context(t, testutil.WaitLong), fake.HTTPClient(nil))
		updated, err := config.RefreshToken(ctx, db, link)
		require.NoError(t, err)
		require.Equal(t, 1, validateCalls, "token is validated")
		require.Equal(t, 1, refreshCalls, "token is refreshed")
		require.NotEqualf(t, link.OAuthAccessToken, updated.OAuthAccessToken, "token is updated")
		dbLink, err := db.GetExternalAuthLink(ctx, database.GetExternalAuthLinkParams{
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
			ExternalAuthLinkOpts: func(link *database.ExternalAuthLink) {
				link.OAuthExpiry = expired
			},
		})

		ctx := oidc.ClientContext(testutil.Context(t, testutil.WaitLong), fake.HTTPClient(nil))

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
			ExternalAuthLinkOpts: func(link *database.ExternalAuthLink) {
				link.OAuthExpiry = expired
			},
		})

		ctx := oidc.ClientContext(testutil.Context(t, testutil.WaitLong), fake.HTTPClient(nil))

		oldAccessToken := link.OAuthAccessToken
		oldRefreshToken := link.OAuthRefreshToken

		// First call: refresh succeeds, validation fails (403).
		_, err := config.RefreshToken(ctx, db, link)
		require.Error(t, err, "expected error because validation returned 403")
		require.True(t, externalauth.IsInvalidTokenError(err))
		require.Equal(t, int64(1), refreshCalls.Load(), "IDP refresh should have been called exactly once")

		// Critical assertion: the DB must contain the NEW tokens from the
		// successful refresh, not the old (now-stale) ones.
		dbLink, err := db.GetExternalAuthLink(ctx, database.GetExternalAuthLinkParams{
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
		ctx := testutil.Context(t, testutil.WaitLong)
		cancelOnRefresh, cancel := context.WithCancel(ctx)
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
			ExternalAuthLinkOpts: func(link *database.ExternalAuthLink) {
				link.OAuthExpiry = expired
			},
		})

		oldAccessToken := link.OAuthAccessToken
		oldRefreshToken := link.OAuthRefreshToken

		octx := oidc.ClientContext(cancelOnRefresh, fake.HTTPClient(nil))
		_, err := config.RefreshToken(octx, db, link)
		require.ErrorIs(t, err, context.Canceled)

		require.Eventually(t, func() bool {
			dbLink, err := db.GetExternalAuthLink(ctx, database.GetExternalAuthLinkParams{
				ProviderID: link.ProviderID,
				UserID:     link.UserID,
			})
			if err != nil {
				return false
			}
			return err == nil &&
				dbLink.OAuthAccessToken != oldAccessToken &&
				dbLink.OAuthRefreshToken != oldRefreshToken
		}, testutil.WaitShort, testutil.IntervalFast, "never saw refresh token db updated")

		require.Equal(t, int64(1), refreshCalls.Load())
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
			ExternalAuthLinkOpts: func(link *database.ExternalAuthLink) {
				link.OAuthExpiry = expired
			},
		})

		// Use a real HTTP transport for non-IDP requests so the
		// validate request can reach the httptest server.
		ctx := oidc.ClientContext(testutil.Context(t, testutil.WaitLong), fake.HTTPClient(&http.Client{
			Transport: http.DefaultTransport,
		}))

		oldAccessToken := link.OAuthAccessToken
		oldRefreshToken := link.OAuthRefreshToken

		// RefreshToken should succeed: the IDP refresh works, the
		// early save persists the token, and ValidateToken returns
		// (true, nil, nil) because the 403 has rate-limit headers.
		updated, err := config.RefreshToken(ctx, db, link)
		require.NoError(t, err, "RefreshToken should succeed when validation is rate-limited")
		require.Equal(t, int64(1), refreshCalls.Load(), "IDP refresh should have been called")
		require.NotEqual(t, oldAccessToken, updated.OAuthAccessToken,
			"returned token should be the new one from the refresh")

		// Verify the DB has the new token.
		dbLink, err := db.GetExternalAuthLink(ctx, database.GetExternalAuthLinkParams{
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

		fake, config, link := setupOauth2Test(t, testConfig{
			FakeIDPOpts: []oidctest.FakeIDPOpt{
				oidctest.WithRefresh(func(_ string) error {
					return nil
				}),
			},
			ExternalAuthOpt: func(cfg *externalauth.Config) {
				cfg.Type = codersdk.EnhancedExternalAuthProviderGitHub.String()
			},
			ExternalAuthLinkOpts: func(link *database.ExternalAuthLink) {
				link.OAuthExpiry = expired
			},
		})

		mDB := mockDB(t,
			withLease(link),
			withUpdateError(xerrors.New("db connection lost")))

		ctx := oidc.ClientContext(testutil.Context(t, testutil.WaitLong), fake.HTTPClient(nil))

		_, err := config.RefreshToken(ctx, mDB, link)
		require.Error(t, err)
		require.Contains(t, err.Error(), "persist refreshed token")
		require.False(t, externalauth.IsInvalidTokenError(err),
			"DB errors should not be treated as invalid token")
	})

	t.Run("WaitsForConcurrentReplicaOK", func(t *testing.T) {
		t.Parallel()

		fake, config, link := setupOauth2Test(t, testConfig{
			FakeIDPOpts: []oidctest.FakeIDPOpt{
				oidctest.WithRefresh(func(_ string) error {
					return xerrors.New("should not be called")
				}),
			},
			ExternalAuthOpt: func(cfg *externalauth.Config) {
				cfg.Type = codersdk.EnhancedExternalAuthProviderGitHub.String()
				// Faster polling for faster tests.
				cfg.RefreshLeaseInitialBackoff = time.Millisecond
				cfg.RefreshLeaseMaxBackoff = time.Millisecond
			},
			ExternalAuthLinkOpts: func(link *database.ExternalAuthLink) {
				link.OAuthExpiry = expired
			},
		})

		winnerLink := link
		winnerLink.OAuthAccessToken = "winner-access-token"
		winnerLink.OAuthRefreshToken = "winner-refresh-token"

		// Simulate another replica already having a lease.  On the second attempt,
		// simulate the token having been refreshed by the other replica.  That
		// refreshed token should be returned instead of trying to initiate a new
		// refresh.
		mDB := mockDB(t,
			withLeaseErrors(link, &pq.Error{
				Code:       pq.ErrorCode("23514"), // check_violation
				Constraint: "external_auth_link_active_lease",
			}, nil),
			withLease(winnerLink))

		ctx := oidc.ClientContext(testutil.Context(t, testutil.WaitLong), fake.HTTPClient(nil))

		updated, err := config.RefreshToken(ctx, mDB, link)
		require.NoError(t, err)
		require.Equal(t, winnerLink.OAuthAccessToken, updated.OAuthAccessToken)
		require.Equal(t, winnerLink.OAuthRefreshToken, updated.OAuthRefreshToken)
	})

	t.Run("WaitsForConcurrentReplicaError", func(t *testing.T) {
		t.Parallel()

		fake, config, link := setupOauth2Test(t, testConfig{
			FakeIDPOpts: []oidctest.FakeIDPOpt{
				oidctest.WithRefresh(func(_ string) error {
					return xerrors.New("should not be called")
				}),
			},
			ExternalAuthOpt: func(cfg *externalauth.Config) {
				cfg.Type = codersdk.EnhancedExternalAuthProviderGitHub.String()
				// Faster polling for faster tests.
				cfg.RefreshLeaseInitialBackoff = time.Millisecond
				cfg.RefreshLeaseMaxBackoff = time.Millisecond
			},
			ExternalAuthLinkOpts: func(link *database.ExternalAuthLink) {
				link.OAuthExpiry = expired
			},
		})

		ctx := oidc.ClientContext(testutil.Context(t, testutil.WaitLong), fake.HTTPClient(nil))

		winnerLink := link
		winnerLink.OAuthRefreshToken = ""
		winnerLink.OauthRefreshFailureReason = "failed to refresh"

		// Simulate another replica already having a lease.  On the second attempt,
		// simulate the token having been failed to be refreshed by the other
		// replica.  That error should be returned instead of trying to initiate a
		// new refresh.
		mDB := mockDB(t,
			withLeaseErrors(link, &pq.Error{
				Code:       pq.ErrorCode("23514"), // check_violation
				Constraint: "external_auth_link_active_lease",
			}, nil),
			withLease(winnerLink))

		_, err := config.RefreshToken(ctx, mDB, link)
		require.Error(t, err)
		require.ErrorContains(t, err, winnerLink.OauthRefreshFailureReason)
	})

	t.Run("AcquireLeaseAtomicity", func(t *testing.T) {
		t.Parallel()
		if testing.Short() {
			t.SkipNow()
		}

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)

		one := dbtestutil.StartTx(t, db, nil)
		two := dbtestutil.StartTx(t, db, nil)

		user := dbgen.User(t, db, database.User{})
		link := dbgen.ExternalAuthLink(t, db, database.ExternalAuthLink{
			UserID: user.ID,
		})

		// The winner acquires the lease inside an open transaction, holding the
		// row lock without committing.
		acquired, err := one.AcquireExternalAuthLinkRefreshLease(ctx, database.AcquireExternalAuthLinkRefreshLeaseParams{
			ProviderID: link.ProviderID,
			UserID:     link.UserID,
			TimeoutMs:  time.Hour.Milliseconds(),
		})
		require.NoError(t, err)
		require.True(t, acquired.RefreshLeaseExpiresAt.Valid)

		// The loser's UPDATE targets the same row and must block on the winner's
		// uncommitted row lock rather than return anything.
		loserErr := make(chan error, 1)
		go func() {
			_, err := two.AcquireExternalAuthLinkRefreshLease(ctx, database.AcquireExternalAuthLinkRefreshLeaseParams{
				ProviderID: link.ProviderID,
				UserID:     link.UserID,
				TimeoutMs:  time.Hour.Milliseconds(),
			})
			loserErr <- err
		}()

		select {
		case err := <-loserErr:
			t.Fatalf("loser returned %v before the winner committed; expected it to block on the row lock", err)
		case <-time.After(testutil.IntervalMedium):
		}

		// Commit the winner. The loser unblocks, re-checks its predicate against
		// the committed row, and errors with the active lease check violation.
		require.NoError(t, one.Done())
		err = testutil.RequireReceive(ctx, t, loserErr)
		require.True(t, database.IsCheckViolation(err, "external_auth_link_active_lease"))

		// The stored lease is the winner's, untouched by the loser.
		final, err := db.GetExternalAuthLink(ctx, database.GetExternalAuthLinkParams{
			ProviderID: link.ProviderID,
			UserID:     link.UserID,
		})
		require.NoError(t, err)
		require.Equal(t, final.RefreshLeaseExpiresAt.Valid, acquired.RefreshLeaseExpiresAt.Valid)
	})

	t.Run("LeaseDeletedLink", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)

		fake, config, link := setupOauth2Test(t, testConfig{
			FakeIDPOpts: []oidctest.FakeIDPOpt{
				oidctest.WithRefresh(func(_ string) error {
					return nil
				}),
			},
			ExternalAuthOpt: func(cfg *externalauth.Config) {
				cfg.Type = codersdk.EnhancedExternalAuthProviderGitHub.String()
				// Faster polling for faster tests.
				cfg.RefreshLeaseInitialBackoff = time.Millisecond
				cfg.RefreshLeaseMaxBackoff = time.Millisecond
			},
			ExternalAuthLinkOpts: func(link *database.ExternalAuthLink) {
				link.OAuthExpiry = expired
			},
		})

		ctx := oidc.ClientContext(testutil.Context(t, testutil.WaitLong), fake.HTTPClient(nil))
		_, err := config.RefreshToken(ctx, db, link)
		require.ErrorIs(t, err, sql.ErrNoRows)
	})

	t.Run("OverridesStaleLease", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)

		fake, config, link := setupOauth2Test(t, testConfig{
			DB: db,
			FakeIDPOpts: []oidctest.FakeIDPOpt{
				oidctest.WithRefresh(func(_ string) error {
					return nil
				}),
			},
			ExternalAuthOpt: func(cfg *externalauth.Config) {
				cfg.Type = codersdk.EnhancedExternalAuthProviderGitHub.String()
				// Faster polling for faster tests.
				cfg.RefreshLeaseInitialBackoff = time.Millisecond
				cfg.RefreshLeaseMaxBackoff = time.Millisecond
			},
			ExternalAuthLinkOpts: func(link *database.ExternalAuthLink) {
				link.OAuthExpiry = expired
			},
		})

		// Simulate another replica having a stale lease.
		ctx := testutil.Context(t, testutil.WaitLong)
		_, err := db.AcquireExternalAuthLinkRefreshLease(ctx, database.AcquireExternalAuthLinkRefreshLeaseParams{
			ProviderID: link.ProviderID,
			UserID:     link.UserID,
			TimeoutMs:  -time.Hour.Milliseconds(),
		})
		require.NoError(t, err)

		ctx = oidc.ClientContext(ctx, fake.HTTPClient(nil))
		_, err = config.RefreshToken(ctx, db, link)
		require.NoError(t, err)
	})

	// OptimisticLockPreventsStaleOverwrite verifies that the
	// UpdateExternalAuthLink WHERE clause prevents a stale caller from
	// overwriting a valid refresh token saved by a concurrent winner.
	t.Run("OptimisticLockPreventsStaleOverwrite", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		wait := make(chan struct{})

		fake, config, link := setupOauth2Test(t, testConfig{
			FakeIDPOpts: []oidctest.FakeIDPOpt{
				oidctest.WithRefresh(func(_ string) error {
					wait <- struct{}{}
					<-wait
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
			ExternalAuthLinkOpts: func(link *database.ExternalAuthLink) {
				link.OAuthExpiry = expired
			},
		})

		ctx := oidc.ClientContext(testutil.Context(t, testutil.WaitLong), fake.HTTPClient(nil))

		// Snapshot the original tokens before any refresh.
		oldRefreshToken := link.OAuthRefreshToken

		var (
			updated database.ExternalAuthLink
			err     error
		)

		// Caller A begins a refresh and takes the lease.
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			updated, err = config.RefreshToken(ctx, db, link)
			assert.NoError(t, err)
			assert.NotEqual(t, oldRefreshToken, updated.OAuthRefreshToken,
				"caller A should have a new refresh token")
		}()

		// Once caller A has the lease, simulate a caller B trying to update the
		// link with an error.  It should fail because caller A has the lease.
		<-wait
		_, err = db.UpdateExternalAuthLink(ctx, database.UpdateExternalAuthLinkParams{
			ProviderID: link.ProviderID,
			UserID:     link.UserID,
			// Write the error an 8lear the token.
			OauthRefreshFailureReason: "simulated failure from stale caller B",
			OAuthRefreshToken:         "",
			UpdatedAt:                 dbtime.Now(),
			// This should prevent the write because it does not match.
			RefreshLeaseExpiresAt: sql.NullTime{Time: dbtime.Now().Add(time.Hour), Valid: true},
			// Preserve then.
			OAuthAccessToken: link.OAuthAccessToken,
			OAuthExpiry:      link.OAuthExpiry,
			OAuthExtra:       link.OAuthExtra,
		})
		require.ErrorIs(t, err, sql.ErrNoRows)

		// Let caller A finish.
		close(wait)
		wg.Wait()

		// Verify DB still has caller A's valid token.
		dbLink, err := db.GetExternalAuthLink(ctx, database.GetExternalAuthLinkParams{
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

// TestRefreshTokenWithScopes verifies the refresh path echoes Config.Scopes on
// the token-endpoint request and preserves the prior refresh_token when the
// authorization server omits a new one (RFC 6749 §6).
func TestRefreshTokenWithScopes(t *testing.T) {
	t.Parallel()

	// fakeAS returns an http.Client + a pointer the test can read after
	// RefreshToken returns. The roundTripper captures the form body of every
	// outbound request and replies with tokenJSON to refresh requests.
	fakeAS := func(t *testing.T, tokenJSON []byte) (*http.Client, *url.Values) {
		t.Helper()
		captured := &url.Values{}
		client := &http.Client{Transport: roundTripper(func(req *http.Request) (*http.Response, error) {
			body, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			values, err := url.ParseQuery(string(body))
			require.NoError(t, err)
			if values.Get("grant_type") == "refresh_token" {
				*captured = values
			}
			rec := httptest.NewRecorder()
			rec.Header().Set("Content-Type", "application/json")
			rec.WriteHeader(http.StatusOK)
			_, err = rec.Write(tokenJSON)
			return rec.Result(), err
		})}
		return client, captured
	}

	newConfig := func(t *testing.T, scopes []string) *externalauth.Config {
		t.Helper()
		instrument := promoauth.NewFactory(prometheus.NewRegistry())
		configs, err := externalauth.ConvertConfig(context.Background(), testutil.Logger(t), instrument, []codersdk.ExternalAuthConfig{{
			ID:           "test",
			Type:         codersdk.EnhancedExternalAuthProviderAzureDevopsEntra.String(),
			ClientID:     "id",
			ClientSecret: "secret",
			AuthURL:      "https://login.microsoftonline.com/tenant/oauth2/authorize",
			TokenURL:     "https://login.microsoftonline.com/tenant/oauth2/token",
			Scopes:       scopes,
		}}, &url.URL{Scheme: "https", Host: "coder.example.com"}, nil)
		require.NoError(t, err)
		return configs[0]
	}

	expired := dbtime.Now().Add(-time.Hour)

	t.Run("EchoesConfiguredScopesOnRefresh", func(t *testing.T) {
		t.Parallel()
		client, captured := fakeAS(t,
			[]byte(`{"access_token":"new","refresh_token":"new-r","token_type":"bearer","expires_in":3600}`))
		cfg := newConfig(t, []string{"openid", "offline_access", "api://app/session:role-any"})

		ctx := context.WithValue(testutil.Context(t, testutil.WaitLong), oauth2.HTTPClient, client)
		link := database.ExternalAuthLink{
			OAuthAccessToken:  "old",
			OAuthRefreshToken: "old-r",
			OAuthExpiry:       expired,
		}
		mDB := mockDB(t,
			withLease(link),
			withUpdatePassthrough())
		_, err := cfg.RefreshToken(ctx, mDB, link)
		require.NoError(t, err)

		require.Equal(t, "refresh_token", captured.Get("grant_type"))
		require.Equal(t, "old-r", captured.Get("refresh_token"))
		require.Equal(t, "openid offline_access api://app/session:role-any", captured.Get("scope"),
			"refresh request must echo configured scopes joined by space")
	})

	t.Run("OmitsScopeParamWhenScopesEmpty", func(t *testing.T) {
		t.Parallel()
		client, captured := fakeAS(t,
			[]byte(`{"access_token":"new","refresh_token":"new-r","token_type":"bearer","expires_in":3600}`))
		cfg := newConfig(t, nil)

		ctx := context.WithValue(testutil.Context(t, testutil.WaitLong), oauth2.HTTPClient, client)
		link := database.ExternalAuthLink{
			OAuthAccessToken:  "old",
			OAuthRefreshToken: "old-r",
			OAuthExpiry:       expired,
		}
		mDB := mockDB(t,
			withLease(link),
			withUpdatePassthrough())
		_, err := cfg.RefreshToken(ctx, mDB, link)
		require.NoError(t, err)

		require.Equal(t, "refresh_token", captured.Get("grant_type"))
		require.Equal(t, "old-r", captured.Get("refresh_token"))
		require.Empty(t, captured.Get("scope"),
			"refresh request must not send a scope param when Config.Scopes is empty")
	})

	t.Run("PreservesPriorRefreshTokenWhenASOmitsNewOne", func(t *testing.T) {
		t.Parallel()
		// Token response intentionally omits refresh_token.
		client, _ := fakeAS(t,
			[]byte(`{"access_token":"new","token_type":"bearer","expires_in":3600}`))
		cfg := newConfig(t, nil)

		ctx := context.WithValue(testutil.Context(t, testutil.WaitLong), oauth2.HTTPClient, client)
		link := database.ExternalAuthLink{
			OAuthAccessToken:  "old",
			OAuthRefreshToken: "prior-r",
			OAuthExpiry:       expired,
		}
		mDB := mockDB(t,
			withLease(link),
			withUpdatePassthrough())
		link, err := cfg.RefreshToken(ctx, mDB, link)
		require.NoError(t, err)
		require.Equal(t, "prior-r", link.OAuthRefreshToken,
			"prior refresh_token must be preserved when AS omits a new one (RFC 6749 §6)")
	})

	t.Run("AcceptsRotatedRefreshTokenWhenASReturnsOne", func(t *testing.T) {
		t.Parallel()
		client, _ := fakeAS(t,
			[]byte(`{"access_token":"new","refresh_token":"rotated-r","token_type":"bearer","expires_in":3600}`))
		cfg := newConfig(t, nil)

		ctx := context.WithValue(testutil.Context(t, testutil.WaitLong), oauth2.HTTPClient, client)
		link := database.ExternalAuthLink{
			OAuthAccessToken:  "old",
			OAuthRefreshToken: "prior-r",
			OAuthExpiry:       expired,
		}
		mDB := mockDB(t,
			withLease(link),
			withUpdatePassthrough())
		link, err := cfg.RefreshToken(ctx, mDB, link)
		require.NoError(t, err)
		require.Equal(t, "rotated-r", link.OAuthRefreshToken,
			"rotated refresh_token from AS must be persisted")
	})
}

func TestValidateToken(t *testing.T) {
	t.Parallel()

	// These tests use httptest.NewServer to control response headers
	// (X-RateLimit-Remaining, Retry-After) that the FakeIDP's
	// WithDynamicUserInfo hook does not expose.

	const providerName = "test-validate"

	// newLoggedConfig returns a config plus the buffer capturing its logs.
	newLoggedConfig := func(t *testing.T, validateURL string) (*externalauth.Config, *bytes.Buffer) {
		t.Helper()
		f := promoauth.NewFactory(prometheus.NewRegistry())
		logs := &bytes.Buffer{}
		logger := slog.Make(slogjson.Sink(logs)).Leveled(slog.LevelDebug)
		// ConvertConfig wires the named logger as production does.
		configs, err := externalauth.ConvertConfig(context.Background(), logger, f, []codersdk.ExternalAuthConfig{{
			ID:           providerName,
			Type:         codersdk.EnhancedExternalAuthProviderGitHub.String(),
			ClientID:     "id",
			ClientSecret: "secret",
			ValidateURL:  validateURL,
		}}, &url.URL{}, nil)
		require.NoError(t, err)
		return configs[0], logs
	}

	type logEntry struct {
		Level  string `json:"level"`
		Msg    string `json:"msg"`
		Fields struct {
			ProviderType string `json:"provider_type"`
			StatusCode   int    `json:"status_code"`
			Reason       string `json:"reason"`
			Suppressed   int64  `json:"suppressed"`
		} `json:"fields"`
	}

	// rateLimitWarnings returns only the rate-limited-validation warnings.
	rateLimitWarnings := func(t *testing.T, logs string) []logEntry {
		t.Helper()
		var out []logEntry
		for _, line := range strings.Split(strings.TrimSpace(logs), "\n") {
			if line == "" {
				continue
			}
			var entry logEntry
			require.NoError(t, json.Unmarshal([]byte(line), &entry))
			if strings.Contains(entry.Msg, "validation endpoint rate-limited") {
				out = append(out, entry)
			}
		}
		return out
	}

	// requireRateLimitLog asserts exactly one WARN line with the given
	// status code and reason.
	requireRateLimitLog := func(t *testing.T, logs string, wantStatus int, wantReason string) {
		t.Helper()
		warnings := rateLimitWarnings(t, logs)
		require.Len(t, warnings, 1, "expected exactly one rate-limit warning, got: %q", logs)
		entry := warnings[0]
		assert.Equal(t, "WARN", entry.Level)
		assert.Equal(t, codersdk.EnhancedExternalAuthProviderGitHub.String(), entry.Fields.ProviderType)
		assert.Equal(t, wantStatus, entry.Fields.StatusCode)
		assert.Equal(t, wantReason, entry.Fields.Reason)
		assert.EqualValues(t, 0, entry.Fields.Suppressed,
			"a lone warning should report no suppressed occurrences")
	}

	newToken := func() *oauth2.Token {
		return &oauth2.Token{
			AccessToken: "test-access-token",
			Expiry:      time.Now().Add(time.Hour),
		}
	}

	// newValidateCtx returns a context carrying a dedicated http.Client per
	// subtest. Without this, parallel subtests share http.DefaultTransport,
	// and httptest.Server.Close() calls http.DefaultTransport.CloseIdleConnections
	// which can break in-flight requests of sibling subtests.
	newValidateCtx := func(t *testing.T) context.Context {
		t.Helper()
		tp := &http.Transport{}
		t.Cleanup(tp.CloseIdleConnections)
		return oidc.ClientContext(testutil.Context(t, testutil.WaitLong), &http.Client{Transport: tp})
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

		config, logs := newLoggedConfig(t, srv.URL)
		valid, user, err := config.ValidateToken(newValidateCtx(t), newToken())

		require.NoError(t, err)
		assert.True(t, valid, "rate-limited 403 should be treated as optimistically valid")
		assert.Nil(t, user)
		requireRateLimitLog(t, logs.String(), http.StatusForbidden, "rate_limit_headers")
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

		config, logs := newLoggedConfig(t, srv.URL)
		valid, user, err := config.ValidateToken(newValidateCtx(t), newToken())

		require.NoError(t, err)
		assert.True(t, valid, "rate-limited 403 with Retry-After should be optimistically valid")
		assert.Nil(t, user)
		requireRateLimitLog(t, logs.String(), http.StatusForbidden, "rate_limit_headers")
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

		config, logs := newLoggedConfig(t, srv.URL)
		valid, user, err := config.ValidateToken(newValidateCtx(t), newToken())

		require.NoError(t, err)
		assert.False(t, valid, "403 with non-zero rate limit remaining means token is invalid")
		assert.Nil(t, user)
		assert.Empty(t, rateLimitWarnings(t, logs.String()), "a genuine revocation should not log a rate-limit warning")
	})

	// Forbidden_NoRateLimitHeaders: a plain 403 without rate-limit
	// headers is a genuine token revocation / permission error.
	t.Run("Forbidden_NoRateLimitHeaders", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		}))
		t.Cleanup(srv.Close)

		config, logs := newLoggedConfig(t, srv.URL)
		valid, user, err := config.ValidateToken(newValidateCtx(t), newToken())

		require.NoError(t, err)
		assert.False(t, valid, "plain 403 without rate-limit headers means token is invalid")
		assert.Nil(t, user)
		assert.Empty(t, rateLimitWarnings(t, logs.String()), "a plain 403 should not log a rate-limit warning")
	})

	// Unauthorized: 401 is always a token revocation regardless of
	// rate-limit headers.
	t.Run("Unauthorized", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))
		t.Cleanup(srv.Close)

		config, _ := newLoggedConfig(t, srv.URL)
		valid, user, err := config.ValidateToken(newValidateCtx(t), newToken())

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

		config, _ := newLoggedConfig(t, srv.URL)
		valid, user, err := config.ValidateToken(newValidateCtx(t), newToken())

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

		config, logs := newLoggedConfig(t, srv.URL)
		valid, user, err := config.ValidateToken(newValidateCtx(t), newToken())

		require.NoError(t, err)
		assert.True(t, valid, "429 should be treated as optimistically valid")
		assert.Nil(t, user)
		requireRateLimitLog(t, logs.String(), http.StatusTooManyRequests, "status_code")
	})

	// Throttled: repeated rate-limited validations within the throttle
	// interval emit a single warning.
	t.Run("Throttled", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
		}))
		t.Cleanup(srv.Close)

		config, logs := newLoggedConfig(t, srv.URL)
		ctx := newValidateCtx(t)
		for range 3 {
			valid, _, err := config.ValidateToken(ctx, newToken())
			require.NoError(t, err)
			assert.True(t, valid)
		}
		requireRateLimitLog(t, logs.String(), http.StatusTooManyRequests, "status_code")
	})

	t.Run("Confirmed", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		}))
		t.Cleanup(srv.Close)

		config, logs := newLoggedConfig(t, srv.URL)
		valid, _, err := config.ValidateToken(newValidateCtx(t), newToken())

		require.NoError(t, err)
		assert.True(t, valid, "200 means the provider confirmed the token")
		assert.Empty(t, rateLimitWarnings(t, logs.String()), "a confirmed validation should not log a rate-limit warning")
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
		handlerStarted := make(chan bool, 1)
		revokeExited := make(chan bool, 1)

		fake, config, link := setupOauth2Test(t, testConfig{
			FakeIDPOpts: []oidctest.FakeIDPOpt{
				oidctest.WithRevokeTokenRFC(func() (int, error) {
					handlerStarted <- true
					<-revokeExited
					return http.StatusOK, nil
				}),
				oidctest.WithServing(),
			},
		})

		// Always unblock the handler so it can return. Must be
		// registered after setupOauth2Test so LIFO runs it first.
		t.Cleanup(func() {
			select {
			case revokeExited <- true:
			default:
			}
		})

		ctx := oidc.ClientContext(testutil.Context(t, testutil.WaitLong), fake.HTTPClient(nil))
		// A short timeout forces the request's deadline to fire while
		// the handler is blocked in-flight, exercising the revoke
		// timeout path.
		config.RevokeTimeout = 100 * time.Millisecond
		revoked, err := config.RevokeToken(ctx, link)
		// Make sure request has reached the handler before asserting.
		// NOTE: if this flakes again, increase config.RevokeTimeout.
		select {
		case <-handlerStarted:
		default:
			t.Fatal("RevokeToken returned before revoke handler started")
		}
		revokeExited <- true
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.False(t, revoked)
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
	configs, err := externalauth.ConvertConfig(context.Background(), testutil.Logger(t), instrument, []codersdk.ExternalAuthConfig{{
		// JFrog just happens to require this custom type.

		Type:         codersdk.EnhancedExternalAuthProviderJFrog.String(),
		ClientID:     "id",
		ClientSecret: "secret",
	}}, &url.URL{}, nil)
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

	ctx := testutil.Context(t, testutil.WaitLong)
	_, err = config.Exchange(context.WithValue(ctx, oauth2.HTTPClient, client), "code")
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
			output, err := externalauth.ConvertConfig(context.Background(), testutil.Logger(t), instrument, tc.Input, &url.URL{}, nil)
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
		client := new(http.Client)
		config, err := externalauth.ConvertConfig(context.Background(), testutil.Logger(t), instrument, []codersdk.ExternalAuthConfig{{
			Type:         string(codersdk.EnhancedExternalAuthProviderGitLab),
			ClientID:     "id",
			ClientSecret: "secret",
			AuthURL:      "https://auth.com",
			TokenURL:     "https://token.com",
			RedirectURL:  "https://redirect.com",
			Scopes:       []string{"read"},
		}}, &url.URL{Scheme: "https", Host: "default.com"}, client)
		require.NoError(t, err)
		require.Equal(t, "https://auth.com?client_id=id&redirect_uri=https%3A%2F%2Fredirect.com%2Fexternal-auth%2Fgitlab%2Fcallback&response_type=code&scope=read", config[0].AuthCodeURL(""))
		assert.Same(t, client, config[0].HTTPClient, "ConvertConfig must wire the provided client onto every Config")
	})

	t.Run("RevokeTimeoutSet", func(t *testing.T) {
		t.Parallel()
		configs, err := externalauth.ConvertConfig(context.Background(), testutil.Logger(t), instrument, []codersdk.ExternalAuthConfig{{
			Type:         string(codersdk.EnhancedExternalAuthProviderGitLab),
			ClientID:     "id",
			ClientSecret: "secret",
		}}, &url.URL{}, nil)
		require.NoError(t, err)
		require.Equal(t, 10*time.Second, configs[0].RevokeTimeout)
	})

	t.Run("SelfHostedGitLabAPIBaseURL", func(t *testing.T) {
		t.Parallel()
		configs, err := externalauth.ConvertConfig(context.Background(), testutil.Logger(t), instrument, []codersdk.ExternalAuthConfig{{
			Type:         string(codersdk.EnhancedExternalAuthProviderGitLab),
			ClientID:     "id",
			ClientSecret: "secret",
			AuthURL:      "https://gitlab.corp.com/oauth/authorize",
			TokenURL:     "https://gitlab.corp.com/oauth/token",
		}}, &url.URL{}, nil)
		require.NoError(t, err)
		require.Len(t, configs, 1)
		require.Equal(t, "https://gitlab.corp.com/api/v4", configs[0].APIBaseURL)
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
	// ExternalAuthLinkOpts can be used to manipulate the link inserted into the
	// DB when the DB is provided.
	ExternalAuthLinkOpts func(link *database.ExternalAuthLink)
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
		RefreshGroup:                  new(singleflight.Group),
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
	if settings.ExternalAuthLinkOpts != nil {
		settings.ExternalAuthLinkOpts(&link)
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

type mockDBOption func(*dbmock.MockStore)

// withLease wraps withLeaseErrors with nil errors.
func withLease(link database.ExternalAuthLink) mockDBOption {
	return withLeaseErrors(link, nil, nil)
}

// withLeaseErrors expects that a lease is acquired and that same lease is then
// released.  If acquireErr is non-nil then a release is not expected and
// releaseErr goes unused.
func withLeaseErrors(link database.ExternalAuthLink, acquireErr, releaseErr error) mockDBOption {
	var lease sql.NullTime
	return func(mDB *dbmock.MockStore) {
		mDB.EXPECT().AcquireExternalAuthLinkRefreshLease(
			gomock.Any(),
			gomock.Cond(func(params database.AcquireExternalAuthLinkRefreshLeaseParams) bool {
				return params.ProviderID == link.ProviderID &&
					params.UserID == link.UserID &&
					params.TimeoutMs > 0
			})).DoAndReturn(func(_ context.Context, params database.AcquireExternalAuthLinkRefreshLeaseParams) (database.ExternalAuthLink, error) {
			if acquireErr == nil {
				// Return the same link but now with a lease attached.
				lease = sql.NullTime{Valid: true, Time: dbtime.Now().Add(time.Duration(params.TimeoutMs) * time.Millisecond)}
				leasedLink := link
				leasedLink.RefreshLeaseExpiresAt = lease
				return leasedLink, nil
			}
			return database.ExternalAuthLink{}, acquireErr
		}).Times(1)
		if acquireErr == nil {
			mDB.EXPECT().ReleaseExternalAuthLinkRefreshLease(gomock.Any(), gomock.Cond(func(params database.ReleaseExternalAuthLinkRefreshLeaseParams) bool {
				return params.ProviderID == link.ProviderID &&
					params.UserID == link.UserID &&
					params.RefreshLeaseExpiresAt.Valid &&
					// Should have passed the same lease back in.
					params.RefreshLeaseExpiresAt.Time.Equal(lease.Time)
			})).Return(releaseErr).Times(1)
		}
	}
}

// withUpdateError expects an update to be attempted and returns an error for
// that update.
func withUpdateError(err error) mockDBOption {
	return func(mDB *dbmock.MockStore) {
		mDB.EXPECT().UpdateExternalAuthLink(gomock.Any(), gomock.Any()).
			Return(database.ExternalAuthLink{}, err).Times(1)
	}
}

// withUpdatePassthrough echoes the UpdateExternalAuthLink params back as a
// populated ExternalAuthLink, letting the test read what RefreshToken decided
// to persist, if anything.
func withUpdatePassthrough() mockDBOption {
	return func(mDB *dbmock.MockStore) {
		mDB.EXPECT().UpdateExternalAuthLink(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, p database.UpdateExternalAuthLinkParams) (database.ExternalAuthLink, error) {
				return database.ExternalAuthLink{
					ProviderID:        p.ProviderID,
					UserID:            p.UserID,
					OAuthAccessToken:  p.OAuthAccessToken,
					OAuthRefreshToken: p.OAuthRefreshToken,
					OAuthExpiry:       p.OAuthExpiry,
				}, nil
			}).Times(1)
	}
}

func mockDB(t *testing.T, opts ...mockDBOption) *dbmock.MockStore {
	t.Helper()
	ctrl := gomock.NewController(t)
	mDB := dbmock.NewMockStore(ctrl)
	for _, opt := range opts {
		opt(mDB)
	}
	return mDB
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
				context.Background(),
				testutil.Logger(t),
				instrument,
				[]codersdk.ExternalAuthConfig{{
					Type:         tc.Type,
					ClientID:     "test-id",
					ClientSecret: "test-secret",
				}},
				accessURL,
				nil,
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

var _ externalauth.SingleflightGroup = (*group)(nil)

// The following has been copied from x/sync/singleflight but has been modified
// to notify when callers join the group so the tests can be deterministic.

// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// errGoexit indicates runtime.Goexit was called in
// the user-given function.
var errGoexit = xerrors.New("runtime.Goexit was called")

// A panicError is an arbitrary value recovered from a panic
// with the stack trace during the execution of the given function.
type panicError struct {
	value any
	stack []byte
}

// Error implements error interface.
func (p *panicError) Error() string {
	return fmt.Sprintf("%v\n\n%s", p.value, p.stack)
}

func (p *panicError) Unwrap() error {
	err, ok := p.value.(error)
	if !ok {
		return nil
	}

	return err
}

func newPanicError(v any) error {
	stack := debug.Stack()

	// The first line of the stack trace is of the form "goroutine N [status]:"
	// but by the time the panic reaches Do the goroutine may no longer exist
	// and its status will have changed. Trim out the misleading line.
	if line := bytes.IndexByte(stack, '\n'); line >= 0 {
		stack = stack[line+1:]
	}
	return &panicError{value: v, stack: stack}
}

// call is an in-flight or completed singleflight.Do call
type call struct {
	wg sync.WaitGroup

	// These fields are written once before the WaitGroup is done
	// and are only read after the WaitGroup is done.
	val any
	err error

	// These fields are read and written with the singleflight
	// mutex held before the WaitGroup is done, and are read but
	// not written after the WaitGroup is done.
	dups  int
	chans []chan<- singleflight.Result
}

// group represents a class of work and forms a namespace in
// which units of work can be executed with duplicate suppression.
type group struct {
	mu     sync.Mutex       // protects m
	m      map[string]*call // lazily initialized
	notify chan string
}

// DoChan is like Do but returns a channel that will receive the
// results when they are ready.
//
// The returned channel will not be closed.
func (g *group) DoChan(key string, fn func() (any, error)) <-chan singleflight.Result {
	ch := make(chan singleflight.Result, 1)
	g.mu.Lock()
	if g.m == nil {
		g.m = make(map[string]*call)
	}
	if c, ok := g.m[key]; ok {
		c.dups++
		c.chans = append(c.chans, ch)
		g.notify <- key
		g.mu.Unlock()
		return ch
	}
	c := &call{chans: []chan<- singleflight.Result{ch}}
	c.wg.Add(1)
	g.m[key] = c
	g.mu.Unlock()

	go g.doCall(c, key, fn)

	return ch
}

// doCall handles the single call for a key.
func (g *group) doCall(c *call, key string, fn func() (any, error)) {
	normalReturn := false
	recovered := false

	// use double-defer to distinguish panic from runtime.Goexit,
	// more details see https://golang.org/cl/134395
	defer func() {
		// the given function invoked runtime.Goexit
		if !normalReturn && !recovered {
			c.err = errGoexit
		}

		g.mu.Lock()
		defer g.mu.Unlock()
		c.wg.Done()
		if g.m[key] == c {
			delete(g.m, key)
		}

		//nolint:errorlint // Avoid changing the original code.
		if e, ok := c.err.(*panicError); ok {
			// In order to prevent the waiting channels from being blocked forever,
			// needs to ensure that this panic cannot be recovered.
			//nolint:revive // Avoid changing the original code.
			if len(c.chans) > 0 {
				go panic(e)
				select {} // Keep this goroutine around so that it will appear in the crash dump.
			} else {
				panic(e)
			}
		} else if c.err == errGoexit { //nolint:revive // Avoid changing the original code.
			// Already in the process of goexit, no need to call again
		} else {
			// Normal return
			for _, ch := range c.chans {
				ch <- singleflight.Result{Val: c.val, Err: c.err, Shared: c.dups > 0}
			}
		}
	}()

	func() {
		defer func() {
			if !normalReturn {
				// Ideally, we would wait to take a stack trace until we've determined
				// whether this is a panic or a runtime.Goexit.
				//
				// Unfortunately, the only way we can distinguish the two is to see
				// whether the recover stopped the goroutine from terminating, and by
				// the time we know that, the part of the stack trace relevant to the
				// panic has been discarded.
				if r := recover(); r != nil {
					c.err = newPanicError(r)
				}
			}
		}()

		c.val, c.err = fn()
		normalReturn = true
	}()

	if !normalReturn {
		recovered = true
	}
}

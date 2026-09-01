package httpmw_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/exp/slices"
	"golang.org/x/oauth2"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/apikey"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/httpmw/loggermw"
	"github.com/coder/coder/v2/coderd/httpmw/loggermw/loggermock"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/cryptorand"
	"github.com/coder/coder/v2/testutil"
)

func randomAPIKeyParts() (id string, secret string, hashedSecret []byte) {
	id, _ = cryptorand.String(10)
	secret, hashedSecret, _ = apikey.GenerateSecret(22)
	return id, secret, hashedSecret
}

// softDeleteUserKeepRows soft-deletes a user while keeping their
// dependent rows (api_keys, user_links): delete_deleted_user_resources
// would purge them, so suppress triggers for this transaction with
// session_replication_role = replica, which is a plain per-transaction
// GUC (no DDL, no ACCESS EXCLUSIVE lock on users that could stall other
// parallel tests when CODER_PG_CONNECTION_URL points every test at one
// shared database). This reconstructs the orphaned credentials the
// middleware must reject (rows that survived cleanup past a race, a
// restored backup, an insert that bypassed trigger_insert_apikeys).
func softDeleteUserKeepRows(t *testing.T, sqlDB *sql.DB, userID uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	tx, err := sqlDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer tx.Rollback() //nolint:errcheck // no-op after commit
	_, err = tx.ExecContext(ctx, `SET LOCAL session_replication_role = replica`)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `UPDATE users SET deleted = true WHERE id = $1`, userID)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
}

func TestAPIKey(t *testing.T) {
	t.Parallel()

	// assertActorOk asserts all the properties of the user auth are ok.
	assertActorOk := func(t *testing.T, r *http.Request) {
		t.Helper()

		actor, ok := dbauthz.ActorFromContext(r.Context())
		assert.True(t, ok, "dbauthz actor ok")
		if ok {
			_, err := actor.Roles.Expand()
			assert.NoError(t, err, "actor roles ok")

			_, err = actor.Scope.Expand()
			assert.NoError(t, err, "actor scope ok")

			err = actor.RegoValueOk()
			assert.NoError(t, err, "actor rego ok")
		}

		auth, ok := httpmw.UserAuthorizationOptional(r.Context())
		assert.True(t, ok, "httpmw auth ok")
		if ok {
			_, err := auth.Roles.Expand()
			assert.NoError(t, err, "auth roles ok")

			_, err = auth.Scope.Expand()
			assert.NoError(t, err, "auth scope ok")

			err = auth.RegoValueOk()
			assert.NoError(t, err, "auth rego ok")
		}
	}

	successHandler := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		// Only called if the API key passes through the handler.
		httpapi.Write(context.Background(), rw, http.StatusOK, codersdk.Response{
			Message: "It worked!",
		})
	})

	t.Run("NoCookie", func(t *testing.T) {
		t.Parallel()
		var (
			db, _ = dbtestutil.NewDB(t)
			r     = httptest.NewRequest("GET", "/", nil)
			rw    = httptest.NewRecorder()
		)
		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: false,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)
	})

	t.Run("NoCookieRedirects", func(t *testing.T) {
		t.Parallel()
		var (
			db, _ = dbtestutil.NewDB(t)
			r     = httptest.NewRequest("GET", "/", nil)
			rw    = httptest.NewRecorder()
		)
		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: true,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		location, err := res.Location()
		require.NoError(t, err)
		require.NotEmpty(t, location.Query().Get("message"))
		require.Equal(t, http.StatusSeeOther, res.StatusCode)
	})

	t.Run("InvalidFormat", func(t *testing.T) {
		t.Parallel()
		var (
			db, _ = dbtestutil.NewDB(t)
			r     = httptest.NewRequest("GET", "/", nil)
			rw    = httptest.NewRecorder()
		)
		r.Header.Set(codersdk.SessionTokenHeader, "test-wow-hello")

		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: false,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)
	})

	t.Run("InvalidIDLength", func(t *testing.T) {
		t.Parallel()
		var (
			db, _ = dbtestutil.NewDB(t)
			r     = httptest.NewRequest("GET", "/", nil)
			rw    = httptest.NewRecorder()
		)
		r.Header.Set(codersdk.SessionTokenHeader, "test-wow")

		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: false,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)
	})

	t.Run("InvalidSecretLength", func(t *testing.T) {
		t.Parallel()
		var (
			db, _ = dbtestutil.NewDB(t)
			r     = httptest.NewRequest("GET", "/", nil)
			rw    = httptest.NewRecorder()
		)
		r.Header.Set(codersdk.SessionTokenHeader, "testtestid-wow")

		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: false,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		var (
			db, _         = dbtestutil.NewDB(t)
			id, secret, _ = randomAPIKeyParts()
			r             = httptest.NewRequest("GET", "/", nil)
			rw            = httptest.NewRecorder()
		)
		r.Header.Set(codersdk.SessionTokenHeader, fmt.Sprintf("%s-%s", id, secret))

		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: false,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)
	})

	t.Run("GetAPIKeyByIDInternalError", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		id, secret, _ := randomAPIKeyParts()
		r := httptest.NewRequest("GET", "/", nil)
		rw := httptest.NewRecorder()
		r.Header.Set(codersdk.SessionTokenHeader, fmt.Sprintf("%s-%s", id, secret))

		db.EXPECT().GetAPIKeyByID(gomock.Any(), id).Return(database.APIKey{}, xerrors.New("db unavailable"))

		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: false,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusInternalServerError, res.StatusCode)

		var resp codersdk.Response
		require.NoError(t, json.NewDecoder(res.Body).Decode(&resp))
		require.NotEqual(t, httpmw.SignedOutErrorMessage, resp.Message)
		require.Contains(t, resp.Detail, "Internal error fetching API key by id")
	})

	t.Run("UserLinkNotFound", func(t *testing.T) {
		t.Parallel()
		var (
			db, _ = dbtestutil.NewDB(t)
			r     = httptest.NewRequest("GET", "/", nil)
			rw    = httptest.NewRecorder()
			user  = dbgen.User(t, db, database.User{
				LoginType: database.LoginTypeGithub,
			})
			// Intentionally not inserting any user link
			_, token = dbgen.APIKey(t, db, database.APIKey{
				UserID:    user.ID,
				LoginType: user.LoginType,
			})
		)
		r.Header.Set(codersdk.SessionTokenHeader, token)
		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: false,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)
		var resp codersdk.Response
		require.NoError(t, json.NewDecoder(res.Body).Decode(&resp))
		require.Equal(t, resp.Message, httpmw.SignedOutErrorMessage)
	})

	t.Run("DeletedUser", func(t *testing.T) {
		t.Parallel()
		var (
			db, _, sqlDB = dbtestutil.NewDBWithSQLDB(t)
			r            = httptest.NewRequest("GET", "/", nil)
			rw           = httptest.NewRecorder()
			user         = dbgen.User(t, db, database.User{})
			// LastUsed is old enough that an accepted request would
			// rewrite it, pinning that rejection precedes the write.
			sentAPIKey, token = dbgen.APIKey(t, db, database.APIKey{
				UserID:   user.ID,
				LastUsed: dbtime.Now().AddDate(0, 0, -1),
			})
		)
		softDeleteUserKeepRows(t, sqlDB, user.ID)
		ctx := testutil.Context(t, testutil.WaitShort)
		deletedUser, err := db.GetUserByID(ctx, user.ID)
		require.NoError(t, err)
		require.True(t, deletedUser.Deleted)

		r.Header.Set(codersdk.SessionTokenHeader, token)
		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: false,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)
		var resp codersdk.Response
		require.NoError(t, json.NewDecoder(res.Body).Decode(&resp))
		require.Equal(t, httpmw.SignedOutErrorMessage, resp.Message)
		require.Equal(t, "API key belongs to a deleted user.", resp.Detail)

		// Reject-before-write: the rejected request must not bump
		// api_keys.last_used / expires_at or users.last_seen_at.
		gotAPIKey, err := db.GetAPIKeyByID(ctx, sentAPIKey.ID)
		require.NoError(t, err)
		require.Equal(t, sentAPIKey.LastUsed, gotAPIKey.LastUsed)
		require.Equal(t, sentAPIKey.ExpiresAt, gotAPIKey.ExpiresAt)
		gotUser, err := db.GetUserByID(ctx, user.ID)
		require.NoError(t, err)
		require.Equal(t, deletedUser.LastSeenAt, gotUser.LastSeenAt)

		// Soft 401: an optional-auth route must continue unauthenticated
		// instead of failing the whole request the way a hard error does.
		optionalR := httptest.NewRequest("GET", "/", nil)
		optionalR.Header.Set(codersdk.SessionTokenHeader, token)
		optionalRW := httptest.NewRecorder()
		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB:       db,
			Optional: true,
		})(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			_, ok := httpmw.APIKeyOptional(r)
			assert.False(t, ok, "optional-auth route must stay unauthenticated")
			rw.WriteHeader(http.StatusOK)
		})).ServeHTTP(optionalRW, optionalR)
		optionalRes := optionalRW.Result()
		defer optionalRes.Body.Close()
		require.Equal(t, http.StatusOK, optionalRes.StatusCode)
	})

	t.Run("DeletedUserExpiredOAuth", func(t *testing.T) {
		t.Parallel()
		var (
			db, _, sqlDB = dbtestutil.NewDBWithSQLDB(t)
			r            = httptest.NewRequest("GET", "/", nil)
			rw           = httptest.NewRecorder()
			user         = dbgen.User(t, db, database.User{})
			// The API key itself is valid, but the OAuth link is
			// expired: pre-fix, the middleware would call the IdP with
			// the deleted user's refresh token and rewrite user_links
			// before rejecting the subject.
			sentAPIKey, token = dbgen.APIKey(t, db, database.APIKey{
				UserID:    user.ID,
				LastUsed:  dbtime.Now().AddDate(0, 0, -1),
				LoginType: database.LoginTypeOIDC,
			})
			sentLink = dbgen.UserLink(t, db, database.UserLink{
				UserID:      user.ID,
				LoginType:   database.LoginTypeOIDC,
				OAuthExpiry: dbtime.Now().AddDate(0, 0, -1),
			})
		)
		softDeleteUserKeepRows(t, sqlDB, user.ID)

		var idpCalls atomic.Int64
		r.Header.Set(codersdk.SessionTokenHeader, token)
		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB: db,
			OAuth2Configs: &httpmw.OAuth2Configs{
				OIDC: &testutil.OAuth2Config{
					TokenSourceFunc: func() (*oauth2.Token, error) {
						idpCalls.Add(1)
						return &oauth2.Token{
							AccessToken:  "wow",
							RefreshToken: "moo",
							Expiry:       dbtime.Now().AddDate(0, 0, 1),
						}, nil
					},
				},
			},
			RedirectToLogin: false,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)
		var resp codersdk.Response
		require.NoError(t, json.NewDecoder(res.Body).Decode(&resp))
		require.Equal(t, httpmw.SignedOutErrorMessage, resp.Message)
		require.Equal(t, "API key belongs to a deleted user.", resp.Detail)

		// Reject-before-write: the deleted user's refresh token must
		// never reach the IdP and user_links must stay untouched.
		require.EqualValues(t, 0, idpCalls.Load(), "deleted-user key must not reach the IdP")
		ctx := testutil.Context(t, testutil.WaitShort)
		gotLink, err := db.GetUserLinkByUserIDLoginType(ctx, database.GetUserLinkByUserIDLoginTypeParams{
			UserID:    user.ID,
			LoginType: database.LoginTypeOIDC,
		})
		require.NoError(t, err)
		require.Equal(t, sentLink.OAuthAccessToken, gotLink.OAuthAccessToken)
		require.Equal(t, sentLink.OAuthRefreshToken, gotLink.OAuthRefreshToken)
		require.Equal(t, sentLink.OAuthExpiry, gotLink.OAuthExpiry)
		gotAPIKey, err := db.GetAPIKeyByID(ctx, sentAPIKey.ID)
		require.NoError(t, err)
		require.Equal(t, sentAPIKey.LastUsed, gotAPIKey.LastUsed)
		require.Equal(t, sentAPIKey.ExpiresAt, gotAPIKey.ExpiresAt)
	})

	t.Run("InvalidSecret", func(t *testing.T) {
		t.Parallel()
		var (
			db, _ = dbtestutil.NewDB(t)
			r     = httptest.NewRequest("GET", "/", nil)
			rw    = httptest.NewRecorder()
			user  = dbgen.User(t, db, database.User{})

			// Use a different secret so they don't match!
			hashed   = sha256.Sum256([]byte("differentsecret"))
			_, token = dbgen.APIKey(t, db, database.APIKey{
				UserID:       user.ID,
				HashedSecret: hashed[:],
			})
		)
		r.Header.Set(codersdk.SessionTokenHeader, token)
		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: false,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)
	})

	t.Run("Expired", func(t *testing.T) {
		t.Parallel()
		var (
			db, _    = dbtestutil.NewDB(t)
			user     = dbgen.User(t, db, database.User{})
			_, token = dbgen.APIKey(t, db, database.APIKey{
				UserID:    user.ID,
				ExpiresAt: time.Now().Add(time.Hour * -1),
			})

			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)
		r.Header.Set(codersdk.SessionTokenHeader, token)

		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: false,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)

		var apiRes codersdk.Response
		dec := json.NewDecoder(res.Body)
		_ = dec.Decode(&apiRes)
		require.True(t, strings.HasPrefix(apiRes.Detail, "API key expired"))
	})

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		var (
			db, _             = dbtestutil.NewDB(t)
			user              = dbgen.User(t, db, database.User{})
			sentAPIKey, token = dbgen.APIKey(t, db, database.APIKey{
				UserID:    user.ID,
				ExpiresAt: dbtime.Now().AddDate(0, 0, 1),
			})

			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)
		r.Header.Set(codersdk.SessionTokenHeader, token)

		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: false,
		})(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			// Checks that it exists on the context!
			_ = httpmw.APIKey(r)
			assertActorOk(t, r)
			httpapi.Write(r.Context(), rw, http.StatusOK, codersdk.Response{
				Message: "It worked!",
			})
		})).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)

		gotAPIKey, err := db.GetAPIKeyByID(r.Context(), sentAPIKey.ID)
		require.NoError(t, err)

		require.Equal(t, sentAPIKey.ExpiresAt, gotAPIKey.ExpiresAt)
	})

	t.Run("ValidWithScope", func(t *testing.T) {
		t.Parallel()
		var (
			db, _    = dbtestutil.NewDB(t)
			user     = dbgen.User(t, db, database.User{})
			_, token = dbgen.APIKey(t, db, database.APIKey{
				UserID:    user.ID,
				ExpiresAt: dbtime.Now().AddDate(0, 0, 1),
				Scopes:    database.APIKeyScopes{database.ApiKeyScopeCoderApplicationConnect},
			})

			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)
		r.AddCookie(&http.Cookie{
			Name:  codersdk.SessionTokenCookie,
			Value: token,
		})

		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: false,
		})(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			// Checks that it exists on the context!
			apiKey := httpmw.APIKey(r)
			assert.Equal(t, database.ApiKeyScopeCoderApplicationConnect, apiKey.Scopes[0])
			assertActorOk(t, r)

			httpapi.Write(r.Context(), rw, http.StatusOK, codersdk.Response{
				Message: "it worked!",
			})
		})).ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
	})

	t.Run("QueryParameter", func(t *testing.T) {
		t.Parallel()
		var (
			db, _    = dbtestutil.NewDB(t)
			user     = dbgen.User(t, db, database.User{})
			_, token = dbgen.APIKey(t, db, database.APIKey{
				UserID:    user.ID,
				ExpiresAt: dbtime.Now().AddDate(0, 0, 1),
			})

			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)
		q := r.URL.Query()
		q.Add(codersdk.SessionTokenCookie, token)
		r.URL.RawQuery = q.Encode()

		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: false,
		})(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			// Checks that it exists on the context!
			_ = httpmw.APIKey(r)
			assertActorOk(t, r)

			httpapi.Write(r.Context(), rw, http.StatusOK, codersdk.Response{
				Message: "It worked!",
			})
		})).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
	})

	t.Run("ValidUpdateLastUsed", func(t *testing.T) {
		t.Parallel()
		var (
			db, _             = dbtestutil.NewDB(t)
			user              = dbgen.User(t, db, database.User{})
			sentAPIKey, token = dbgen.APIKey(t, db, database.APIKey{
				UserID:    user.ID,
				LastUsed:  dbtime.Now().AddDate(0, 0, -1),
				ExpiresAt: dbtime.Now().AddDate(0, 0, 1),
			})

			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)
		r.Header.Set(codersdk.SessionTokenHeader, token)

		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: false,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)

		gotAPIKey, err := db.GetAPIKeyByID(r.Context(), sentAPIKey.ID)
		require.NoError(t, err)

		require.NotEqual(t, sentAPIKey.LastUsed, gotAPIKey.LastUsed)
		require.Equal(t, sentAPIKey.ExpiresAt, gotAPIKey.ExpiresAt)
	})

	t.Run("ValidUpdateExpiry", func(t *testing.T) {
		t.Parallel()
		var (
			db, _             = dbtestutil.NewDB(t)
			user              = dbgen.User(t, db, database.User{})
			sentAPIKey, token = dbgen.APIKey(t, db, database.APIKey{
				UserID:    user.ID,
				LastUsed:  dbtime.Now(),
				ExpiresAt: dbtime.Now().Add(time.Minute),
			})

			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)
		r.Header.Set(codersdk.SessionTokenHeader, token)

		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: false,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)

		gotAPIKey, err := db.GetAPIKeyByID(r.Context(), sentAPIKey.ID)
		require.NoError(t, err)

		require.Equal(t, sentAPIKey.LastUsed, gotAPIKey.LastUsed)
		require.NotEqual(t, sentAPIKey.ExpiresAt, gotAPIKey.ExpiresAt)
	})

	t.Run("TokenNoExpiryRefresh", func(t *testing.T) {
		t.Parallel()
		var (
			db, _             = dbtestutil.NewDB(t)
			user              = dbgen.User(t, db, database.User{})
			sentAPIKey, token = dbgen.APIKey(t, db, database.APIKey{
				UserID:    user.ID,
				LastUsed:  dbtime.Now(),
				ExpiresAt: dbtime.Now().Add(time.Minute),
				LoginType: database.LoginTypeToken,
			})

			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)
		r.Header.Set(codersdk.SessionTokenHeader, token)

		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: false,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)

		gotAPIKey, err := db.GetAPIKeyByID(r.Context(), sentAPIKey.ID)
		require.NoError(t, err)

		// Programmatic tokens honor a fixed lifetime, so the expiry must not be
		// extended on use even though it is within the refresh window.
		require.Equal(t, sentAPIKey.ExpiresAt, gotAPIKey.ExpiresAt)
	})

	t.Run("NoRefresh", func(t *testing.T) {
		t.Parallel()
		var (
			db, _             = dbtestutil.NewDB(t)
			user              = dbgen.User(t, db, database.User{})
			sentAPIKey, token = dbgen.APIKey(t, db, database.APIKey{
				UserID:    user.ID,
				LastUsed:  dbtime.Now().AddDate(0, 0, -1),
				ExpiresAt: dbtime.Now().AddDate(0, 0, 1),
			})

			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)
		r.Header.Set(codersdk.SessionTokenHeader, token)

		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB:                          db,
			RedirectToLogin:             false,
			DisableSessionExpiryRefresh: true,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)

		gotAPIKey, err := db.GetAPIKeyByID(r.Context(), sentAPIKey.ID)
		require.NoError(t, err)

		require.NotEqual(t, sentAPIKey.LastUsed, gotAPIKey.LastUsed)
		require.Equal(t, sentAPIKey.ExpiresAt, gotAPIKey.ExpiresAt)
	})

	t.Run("OAuthNotExpired", func(t *testing.T) {
		t.Parallel()
		var (
			db, _             = dbtestutil.NewDB(t)
			user              = dbgen.User(t, db, database.User{})
			sentAPIKey, token = dbgen.APIKey(t, db, database.APIKey{
				UserID:    user.ID,
				LastUsed:  dbtime.Now(),
				ExpiresAt: dbtime.Now().AddDate(0, 0, 1),
				LoginType: database.LoginTypeGithub,
			})
			_ = dbgen.UserLink(t, db, database.UserLink{
				UserID:    user.ID,
				LoginType: database.LoginTypeGithub,
			})

			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)
		r.Header.Set(codersdk.SessionTokenHeader, token)

		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: false,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)

		gotAPIKey, err := db.GetAPIKeyByID(r.Context(), sentAPIKey.ID)
		require.NoError(t, err)

		require.Equal(t, sentAPIKey.LastUsed, gotAPIKey.LastUsed)
		require.Equal(t, sentAPIKey.ExpiresAt, gotAPIKey.ExpiresAt)
	})

	t.Run("APIKeyExpiredOAuthExpired", func(t *testing.T) {
		t.Parallel()
		var (
			db, _             = dbtestutil.NewDB(t)
			user              = dbgen.User(t, db, database.User{})
			sentAPIKey, token = dbgen.APIKey(t, db, database.APIKey{
				UserID:    user.ID,
				LastUsed:  dbtime.Now().AddDate(0, 0, -1),
				ExpiresAt: dbtime.Now().AddDate(0, 0, -1),
				LoginType: database.LoginTypeOIDC,
			})
			_ = dbgen.UserLink(t, db, database.UserLink{
				UserID:      user.ID,
				LoginType:   database.LoginTypeOIDC,
				OAuthExpiry: dbtime.Now().AddDate(0, 0, -1),
			})

			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)
		r.Header.Set(codersdk.SessionTokenHeader, token)

		// Include a valid oauth token for refreshing. If this token is invalid,
		// it is difficult to tell an auth failure from an expired api key, or
		// an expired oauth key.
		oauthToken := &oauth2.Token{
			AccessToken:  "wow",
			RefreshToken: "moo",
			Expiry:       dbtime.Now().AddDate(0, 0, 1),
		}
		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB: db,
			OAuth2Configs: &httpmw.OAuth2Configs{
				OIDC: &testutil.OAuth2Config{
					Token: oauthToken,
				},
			},
			RedirectToLogin: false,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)

		gotAPIKey, err := db.GetAPIKeyByID(r.Context(), sentAPIKey.ID)
		require.NoError(t, err)

		require.Equal(t, sentAPIKey.LastUsed, gotAPIKey.LastUsed)
		require.Equal(t, sentAPIKey.ExpiresAt, gotAPIKey.ExpiresAt)
	})

	t.Run("APIKeyExpiredOAuthNotExpired", func(t *testing.T) {
		t.Parallel()
		var (
			db, _             = dbtestutil.NewDB(t)
			user              = dbgen.User(t, db, database.User{})
			sentAPIKey, token = dbgen.APIKey(t, db, database.APIKey{
				UserID:    user.ID,
				LastUsed:  dbtime.Now().AddDate(0, 0, -1),
				ExpiresAt: dbtime.Now().AddDate(0, 0, -1),
				LoginType: database.LoginTypeOIDC,
			})
			_ = dbgen.UserLink(t, db, database.UserLink{
				UserID:    user.ID,
				LoginType: database.LoginTypeOIDC,
			})

			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)
		r.Header.Set(codersdk.SessionTokenHeader, token)

		oauthToken := &oauth2.Token{
			AccessToken:  "wow",
			RefreshToken: "moo",
			Expiry:       dbtime.Now().AddDate(0, 0, 1),
		}
		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB: db,
			OAuth2Configs: &httpmw.OAuth2Configs{
				OIDC: &testutil.OAuth2Config{
					Token: oauthToken,
				},
			},
			RedirectToLogin: false,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)

		gotAPIKey, err := db.GetAPIKeyByID(r.Context(), sentAPIKey.ID)
		require.NoError(t, err)

		require.Equal(t, sentAPIKey.LastUsed, gotAPIKey.LastUsed)
		require.Equal(t, sentAPIKey.ExpiresAt, gotAPIKey.ExpiresAt)
	})

	t.Run("OAuthRefresh", func(t *testing.T) {
		t.Parallel()
		var (
			db, _             = dbtestutil.NewDB(t)
			user              = dbgen.User(t, db, database.User{})
			sentAPIKey, token = dbgen.APIKey(t, db, database.APIKey{
				UserID:    user.ID,
				LastUsed:  dbtime.Now(),
				ExpiresAt: dbtime.Now().AddDate(0, 0, 1),
				LoginType: database.LoginTypeGithub,
			})
			_ = dbgen.UserLink(t, db, database.UserLink{
				UserID:            user.ID,
				LoginType:         database.LoginTypeGithub,
				OAuthRefreshToken: "hello",
				OAuthExpiry:       dbtime.Now().AddDate(0, 0, -1),
			})

			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)
		r.Header.Set(codersdk.SessionTokenHeader, token)

		oauthToken := &oauth2.Token{
			AccessToken:  "wow",
			RefreshToken: "moo",
			Expiry:       dbtestutil.NowInDefaultTimezone().AddDate(0, 0, 1),
		}
		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB: db,
			OAuth2Configs: &httpmw.OAuth2Configs{
				Github: &testutil.OAuth2Config{
					Token: oauthToken,
				},
			},
			RedirectToLogin: false,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)

		gotAPIKey, err := db.GetAPIKeyByID(r.Context(), sentAPIKey.ID)
		require.NoError(t, err)

		require.Equal(t, sentAPIKey.LastUsed, gotAPIKey.LastUsed)
		// Note that OAuth expiry is independent of APIKey expiry, so an OIDC refresh DOES NOT affect the expiry of the
		// APIKey
		require.Equal(t, sentAPIKey.ExpiresAt, gotAPIKey.ExpiresAt)

		gotLink, err := db.GetUserLinkByUserIDLoginType(r.Context(), database.GetUserLinkByUserIDLoginTypeParams{
			UserID:    user.ID,
			LoginType: database.LoginTypeGithub,
		})
		require.NoError(t, err)
		require.Equal(t, gotLink.OAuthRefreshToken, "moo")
	})

	t.Run("OAuthExpiredNoRefresh", func(t *testing.T) {
		t.Parallel()
		var (
			ctx               = testutil.Context(t, testutil.WaitShort)
			db, _             = dbtestutil.NewDB(t)
			user              = dbgen.User(t, db, database.User{})
			sentAPIKey, token = dbgen.APIKey(t, db, database.APIKey{
				UserID:    user.ID,
				LastUsed:  dbtime.Now(),
				ExpiresAt: dbtime.Now().AddDate(0, 0, 1),
				LoginType: database.LoginTypeGithub,
			})

			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)
		_, err := db.InsertUserLink(ctx, database.InsertUserLinkParams{
			UserID:           user.ID,
			LoginType:        database.LoginTypeGithub,
			OAuthExpiry:      dbtime.Now().AddDate(0, 0, -1),
			OAuthAccessToken: "letmein",
		})
		require.NoError(t, err)

		r.Header.Set(codersdk.SessionTokenHeader, token)

		oauthToken := &oauth2.Token{
			AccessToken:  "wow",
			RefreshToken: "moo",
			Expiry:       dbtime.Now().AddDate(0, 0, 1),
		}
		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB: db,
			OAuth2Configs: &httpmw.OAuth2Configs{
				Github: &testutil.OAuth2Config{
					Token: oauthToken,
				},
			},
			RedirectToLogin: false,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)

		gotAPIKey, err := db.GetAPIKeyByID(r.Context(), sentAPIKey.ID)
		require.NoError(t, err)

		require.Equal(t, sentAPIKey.LastUsed, gotAPIKey.LastUsed)
		require.Equal(t, sentAPIKey.ExpiresAt, gotAPIKey.ExpiresAt)
	})

	t.Run("RemoteIPUpdates", func(t *testing.T) {
		t.Parallel()
		var (
			db, _             = dbtestutil.NewDB(t)
			user              = dbgen.User(t, db, database.User{})
			sentAPIKey, token = dbgen.APIKey(t, db, database.APIKey{
				UserID:    user.ID,
				LastUsed:  dbtime.Now().AddDate(0, 0, -1),
				ExpiresAt: dbtime.Now().AddDate(0, 0, 1),
			})

			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)
		r.RemoteAddr = "1.1.1.1"
		r.Header.Set(codersdk.SessionTokenHeader, token)

		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: false,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)

		gotAPIKey, err := db.GetAPIKeyByID(r.Context(), sentAPIKey.ID)
		require.NoError(t, err)

		require.Equal(t, "1.1.1.1", gotAPIKey.IPAddress.IPNet.IP.String())
	})

	t.Run("RedirectToLogin", func(t *testing.T) {
		t.Parallel()
		var (
			db, _ = dbtestutil.NewDB(t)
			r     = httptest.NewRequest("GET", "/", nil)
			rw    = httptest.NewRecorder()
		)

		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: true,
		})(successHandler).ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusSeeOther, res.StatusCode)
		u, err := res.Location()
		require.NoError(t, err)
		require.Equal(t, "/login", u.Path)
	})

	t.Run("Optional", func(t *testing.T) {
		t.Parallel()
		var (
			db, _ = dbtestutil.NewDB(t)
			r     = httptest.NewRequest("GET", "/", nil)
			rw    = httptest.NewRecorder()

			count   atomic.Int64
			handler = http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
				count.Add(1)

				apiKey, ok := httpmw.APIKeyOptional(r)
				assert.False(t, ok)
				assert.Zero(t, apiKey)

				rw.WriteHeader(http.StatusOK)
			})
		)

		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: false,
			Optional:        true,
		})(handler).ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
		require.EqualValues(t, 1, count.Load())
	})

	t.Run("Tokens", func(t *testing.T) {
		t.Parallel()
		var (
			db, _             = dbtestutil.NewDB(t)
			user              = dbgen.User(t, db, database.User{})
			sentAPIKey, token = dbgen.APIKey(t, db, database.APIKey{
				UserID:    user.ID,
				LastUsed:  dbtime.Now(),
				ExpiresAt: dbtime.Now().AddDate(0, 0, 1),
				LoginType: database.LoginTypeToken,
			})

			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)
		r.Header.Set(codersdk.SessionTokenHeader, token)

		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: false,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)

		gotAPIKey, err := db.GetAPIKeyByID(r.Context(), sentAPIKey.ID)
		require.NoError(t, err)

		require.Equal(t, sentAPIKey.LastUsed, gotAPIKey.LastUsed)
		require.Equal(t, sentAPIKey.ExpiresAt, gotAPIKey.ExpiresAt)
		require.Equal(t, sentAPIKey.LoginType, gotAPIKey.LoginType)
	})

	t.Run("MissingConfig", func(t *testing.T) {
		t.Parallel()
		var (
			db, _    = dbtestutil.NewDB(t)
			user     = dbgen.User(t, db, database.User{})
			_, token = dbgen.APIKey(t, db, database.APIKey{
				UserID:    user.ID,
				LastUsed:  dbtime.Now(),
				ExpiresAt: dbtime.Now().AddDate(0, 0, 1),
				LoginType: database.LoginTypeOIDC,
			})
			_ = dbgen.UserLink(t, db, database.UserLink{
				UserID:            user.ID,
				LoginType:         database.LoginTypeOIDC,
				OAuthRefreshToken: "random",
				// expired
				OAuthExpiry: time.Now().Add(time.Hour * -1),
			})

			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)
		r.Header.Set(codersdk.SessionTokenHeader, token)

		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: false,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusInternalServerError, res.StatusCode)
		out, _ := io.ReadAll(res.Body)
		require.Contains(t, string(out), "Unable to refresh")
	})

	t.Run("CustomRoles", func(t *testing.T) {
		t.Parallel()
		var (
			db, _      = dbtestutil.NewDB(t)
			org        = dbgen.Organization(t, db, database.Organization{})
			customRole = dbgen.CustomRole(t, db, database.CustomRole{
				Name:           "custom-role",
				OrgPermissions: []database.CustomRolePermission{},
				OrganizationID: uuid.NullUUID{
					UUID:  org.ID,
					Valid: true,
				},
			})
			user = dbgen.User(t, db, database.User{
				RBACRoles: []string{},
			})
			_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
				UserID:         user.ID,
				OrganizationID: org.ID,
				CreatedAt:      time.Time{},
				UpdatedAt:      time.Time{},
				Roles: []string{
					rbac.RoleOrgAdmin(),
					customRole.Name,
				},
			})
			_, token = dbgen.APIKey(t, db, database.APIKey{
				UserID:    user.ID,
				ExpiresAt: dbtime.Now().AddDate(0, 0, 1),
			})

			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)
		r.Header.Set(codersdk.SessionTokenHeader, token)

		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: false,
		})(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			assertActorOk(t, r)

			auth := httpmw.UserAuthorization(r.Context())

			roles, err := auth.Roles.Expand()
			assert.NoError(t, err, "expand user roles")
			// Assert built in org role
			assert.True(t, slices.ContainsFunc(roles, func(role rbac.Role) bool {
				return role.Identifier.Name == rbac.RoleOrgAdmin() && role.Identifier.OrganizationID == org.ID
			}), "org admin role")
			// Assert custom role
			assert.True(t, slices.ContainsFunc(roles, func(role rbac.Role) bool {
				return role.Identifier.Name == customRole.Name && role.Identifier.OrganizationID == org.ID
			}), "custom org role")

			httpapi.Write(r.Context(), rw, http.StatusOK, codersdk.Response{
				Message: "It worked!",
			})
		})).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
	})

	// There is no sql foreign key constraint to require all assigned roles
	// still exist in the database. We need to handle deleted roles.
	t.Run("RoleNotExists", func(t *testing.T) {
		t.Parallel()
		var (
			roleNotExistsName = "role-not-exists"
			db, _             = dbtestutil.NewDB(t)
			org               = dbgen.Organization(t, db, database.Organization{})
			user              = dbgen.User(t, db, database.User{
				RBACRoles: []string{
					// Also provide an org not exists. In practice this makes no sense
					// to store org roles in the user table, but there is no org to
					// store it in. So just throw this here for even more unexpected
					// behavior handling!
					rbac.RoleIdentifier{Name: roleNotExistsName, OrganizationID: uuid.New()}.String(),
				},
			})
			_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
				UserID:         user.ID,
				OrganizationID: org.ID,
				CreatedAt:      time.Time{},
				UpdatedAt:      time.Time{},
				Roles: []string{
					rbac.RoleOrgAdmin(),
					roleNotExistsName,
				},
			})
			_, token = dbgen.APIKey(t, db, database.APIKey{
				UserID:    user.ID,
				ExpiresAt: dbtime.Now().AddDate(0, 0, 1),
			})

			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)
		r.Header.Set(codersdk.SessionTokenHeader, token)

		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: false,
		})(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			assertActorOk(t, r)
			auth := httpmw.UserAuthorization(r.Context())

			roles, err := auth.Roles.Expand()
			assert.NoError(t, err, "expand user roles")
			// Assert built in org role
			assert.True(t, slices.ContainsFunc(roles, func(role rbac.Role) bool {
				return role.Identifier.Name == rbac.RoleOrgAdmin() && role.Identifier.OrganizationID == org.ID
			}), "org admin role")

			// Assert the role-not-exists is not returned
			assert.False(t, slices.ContainsFunc(roles, func(role rbac.Role) bool {
				return role.Identifier.Name == roleNotExistsName
			}), "role should not exist")

			httpapi.Write(r.Context(), rw, http.StatusOK, codersdk.Response{
				Message: "It worked!",
			})
		})).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
	})

	t.Run("LogsAPIKeyID", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name           string
			expired        bool
			expectedStatus int
		}{
			{
				name:           "OnSuccess",
				expired:        false,
				expectedStatus: http.StatusOK,
			},
			{
				name:           "OnFailure",
				expired:        true,
				expectedStatus: http.StatusUnauthorized,
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				var (
					db, _  = dbtestutil.NewDB(t)
					user   = dbgen.User(t, db, database.User{})
					expiry = dbtime.Now().AddDate(0, 0, 1)
				)
				if tc.expired {
					expiry = dbtime.Now().AddDate(0, 0, -1)
				}
				sentAPIKey, token := dbgen.APIKey(t, db, database.APIKey{
					UserID:    user.ID,
					ExpiresAt: expiry,
				})

				var (
					ctrl       = gomock.NewController(t)
					mockLogger = loggermock.NewMockRequestLogger(ctrl)
					r          = httptest.NewRequest("GET", "/", nil)
					rw         = httptest.NewRecorder()
				)
				r.Header.Set(codersdk.SessionTokenHeader, token)

				// Expect WithAuthContext to be called (from dbauthz.As).
				mockLogger.EXPECT().WithAuthContext(gomock.Any()).AnyTimes()
				// Expect WithFields to be called with api_key_id field regardless of success/failure.
				mockLogger.EXPECT().WithFields(
					slog.F("api_key_id", sentAPIKey.ID),
				).Times(1)

				// Add the mock logger to the context.
				ctx := loggermw.WithRequestLogger(r.Context(), mockLogger)
				r = r.WithContext(ctx)

				httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
					DB:              db,
					RedirectToLogin: false,
				})(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
					if tc.expired {
						t.Error("handler should not be called on auth failure")
					}
					httpapi.Write(r.Context(), rw, http.StatusOK, codersdk.Response{
						Message: "It worked!",
					})
				})).ServeHTTP(rw, r)

				res := rw.Result()
				defer res.Body.Close()
				require.Equal(t, tc.expectedStatus, res.StatusCode)
			})
		}
	})
}

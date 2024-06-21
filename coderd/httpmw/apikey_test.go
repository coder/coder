package httpmw_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/slices"
	"golang.org/x/oauth2"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmem"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/cryptorand"
	"github.com/coder/coder/v2/testutil"
)

func randomAPIKeyParts() (id string, secret string) {
	id, _ = cryptorand.String(10)
	secret, _ = cryptorand.String(22)
	return id, secret
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

		auth, ok := httpmw.UserAuthorizationOptional(r)
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
			db = dbmem.New()
			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
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
			db = dbmem.New()
			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
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
			db = dbmem.New()
			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
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
			db = dbmem.New()
			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
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
			db = dbmem.New()
			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
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
			db         = dbmem.New()
			id, secret = randomAPIKeyParts()
			r          = httptest.NewRequest("GET", "/", nil)
			rw         = httptest.NewRecorder()
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

	t.Run("UserLinkNotFound", func(t *testing.T) {
		t.Parallel()
		var (
			db   = dbmem.New()
			r    = httptest.NewRequest("GET", "/", nil)
			rw   = httptest.NewRecorder()
			user = dbgen.User(t, db, database.User{
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

	t.Run("InvalidSecret", func(t *testing.T) {
		t.Parallel()
		var (
			db   = dbmem.New()
			r    = httptest.NewRequest("GET", "/", nil)
			rw   = httptest.NewRecorder()
			user = dbgen.User(t, db, database.User{})

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
			db       = dbmem.New()
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
			db                = dbmem.New()
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
			db       = dbmem.New()
			user     = dbgen.User(t, db, database.User{})
			_, token = dbgen.APIKey(t, db, database.APIKey{
				UserID:    user.ID,
				ExpiresAt: dbtime.Now().AddDate(0, 0, 1),
				Scope:     database.APIKeyScopeApplicationConnect,
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
			assert.Equal(t, database.APIKeyScopeApplicationConnect, apiKey.Scope)
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
			db       = dbmem.New()
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
			db                = dbmem.New()
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
			db                = dbmem.New()
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

	t.Run("NoRefresh", func(t *testing.T) {
		t.Parallel()
		var (
			db                = dbmem.New()
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
			db                = dbmem.New()
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

	t.Run("OAuthRefresh", func(t *testing.T) {
		t.Parallel()
		var (
			db                = dbmem.New()
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
		require.Equal(t, http.StatusOK, res.StatusCode)

		gotAPIKey, err := db.GetAPIKeyByID(r.Context(), sentAPIKey.ID)
		require.NoError(t, err)

		require.Equal(t, sentAPIKey.LastUsed, gotAPIKey.LastUsed)
		require.Equal(t, oauthToken.Expiry, gotAPIKey.ExpiresAt)
	})

	t.Run("RemoteIPUpdates", func(t *testing.T) {
		t.Parallel()
		var (
			db                = dbmem.New()
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

		require.Equal(t, net.ParseIP("1.1.1.1"), gotAPIKey.IPAddress.IPNet.IP)
	})

	t.Run("RedirectToLogin", func(t *testing.T) {
		t.Parallel()
		var (
			db = dbmem.New()
			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
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
			db = dbmem.New()
			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()

			count   int64
			handler = http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
				atomic.AddInt64(&count, 1)

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
		require.EqualValues(t, 1, atomic.LoadInt64(&count))
	})

	t.Run("Tokens", func(t *testing.T) {
		t.Parallel()
		var (
			db                = dbmem.New()
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
			db       = dbmem.New()
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
			db         = dbmem.New()
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

			auth := httpmw.UserAuthorization(r)

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
			db                = dbmem.New()
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
			auth := httpmw.UserAuthorization(r)

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
}

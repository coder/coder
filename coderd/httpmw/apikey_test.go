package httpmw_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/databasefake"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/cryptorand"
)

func randomAPIKeyParts() (id string, secret string) {
	id, _ = cryptorand.String(10)
	secret, _ = cryptorand.String(22)
	return id, secret
}

func TestAPIKey(t *testing.T) {
	t.Parallel()

	successHandler := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		// Only called if the API key passes through the handler.
		httpapi.Write(context.Background(), rw, http.StatusOK, codersdk.Response{
			Message: "It worked!",
		})
	})

	t.Run("NoCookie", func(t *testing.T) {
		t.Parallel()
		var (
			db = databasefake.New()
			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)
		httpmw.ExtractAPIKey(httpmw.ExtractAPIKeyConfig{
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
			db = databasefake.New()
			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)
		httpmw.ExtractAPIKey(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: true,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		location, err := res.Location()
		require.NoError(t, err)
		require.NotEmpty(t, location.Query().Get("message"))
		require.Equal(t, http.StatusTemporaryRedirect, res.StatusCode)
	})

	t.Run("InvalidFormat", func(t *testing.T) {
		t.Parallel()
		var (
			db = databasefake.New()
			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)
		r.Header.Set(codersdk.SessionCustomHeader, "test-wow-hello")

		httpmw.ExtractAPIKey(httpmw.ExtractAPIKeyConfig{
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
			db = databasefake.New()
			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)
		r.Header.Set(codersdk.SessionCustomHeader, "test-wow")

		httpmw.ExtractAPIKey(httpmw.ExtractAPIKeyConfig{
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
			db = databasefake.New()
			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)
		r.Header.Set(codersdk.SessionCustomHeader, "testtestid-wow")

		httpmw.ExtractAPIKey(httpmw.ExtractAPIKeyConfig{
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
			db         = databasefake.New()
			id, secret = randomAPIKeyParts()
			r          = httptest.NewRequest("GET", "/", nil)
			rw         = httptest.NewRecorder()
		)
		r.Header.Set(codersdk.SessionCustomHeader, fmt.Sprintf("%s-%s", id, secret))

		httpmw.ExtractAPIKey(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: false,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)
	})

	t.Run("InvalidSecret", func(t *testing.T) {
		t.Parallel()
		var (
			db         = databasefake.New()
			id, secret = randomAPIKeyParts()
			r          = httptest.NewRequest("GET", "/", nil)
			rw         = httptest.NewRecorder()
			user       = createUser(r.Context(), t, db)
		)
		r.Header.Set(codersdk.SessionCustomHeader, fmt.Sprintf("%s-%s", id, secret))

		// Use a different secret so they don't match!
		hashed := sha256.Sum256([]byte("differentsecret"))
		_, err := db.InsertAPIKey(r.Context(), database.InsertAPIKeyParams{
			ID:           id,
			HashedSecret: hashed[:],
			UserID:       user.ID,
			LoginType:    database.LoginTypePassword,
			Scope:        database.APIKeyScopeAll,
		})
		require.NoError(t, err)
		httpmw.ExtractAPIKey(httpmw.ExtractAPIKeyConfig{
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
			db         = databasefake.New()
			id, secret = randomAPIKeyParts()
			hashed     = sha256.Sum256([]byte(secret))
			r          = httptest.NewRequest("GET", "/", nil)
			rw         = httptest.NewRecorder()
			user       = createUser(r.Context(), t, db)
		)
		r.Header.Set(codersdk.SessionCustomHeader, fmt.Sprintf("%s-%s", id, secret))

		_, err := db.InsertAPIKey(r.Context(), database.InsertAPIKeyParams{
			ID:           id,
			HashedSecret: hashed[:],
			UserID:       user.ID,
			LoginType:    database.LoginTypePassword,
			Scope:        database.APIKeyScopeAll,
		})
		require.NoError(t, err)
		httpmw.ExtractAPIKey(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: false,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)
	})

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		var (
			db         = databasefake.New()
			id, secret = randomAPIKeyParts()
			hashed     = sha256.Sum256([]byte(secret))
			r          = httptest.NewRequest("GET", "/", nil)
			rw         = httptest.NewRecorder()
			user       = createUser(r.Context(), t, db)
		)
		r.Header.Set(codersdk.SessionCustomHeader, fmt.Sprintf("%s-%s", id, secret))

		sentAPIKey, err := db.InsertAPIKey(r.Context(), database.InsertAPIKeyParams{
			ID:           id,
			HashedSecret: hashed[:],
			ExpiresAt:    database.Now().AddDate(0, 0, 1),
			UserID:       user.ID,
			LoginType:    database.LoginTypePassword,
			Scope:        database.APIKeyScopeAll,
		})
		require.NoError(t, err)
		httpmw.ExtractAPIKey(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: false,
		})(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			// Checks that it exists on the context!
			_ = httpmw.APIKey(r)
			httpapi.Write(r.Context(), rw, http.StatusOK, codersdk.Response{
				Message: "It worked!",
			})
		})).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)

		gotAPIKey, err := db.GetAPIKeyByID(r.Context(), id)
		require.NoError(t, err)

		require.Equal(t, sentAPIKey.ExpiresAt, gotAPIKey.ExpiresAt)
	})

	t.Run("ValidWithScope", func(t *testing.T) {
		t.Parallel()
		var (
			db         = databasefake.New()
			id, secret = randomAPIKeyParts()
			hashed     = sha256.Sum256([]byte(secret))
			r          = httptest.NewRequest("GET", "/", nil)
			rw         = httptest.NewRecorder()
			user       = createUser(r.Context(), t, db)
		)
		r.AddCookie(&http.Cookie{
			Name:  codersdk.SessionTokenKey,
			Value: fmt.Sprintf("%s-%s", id, secret),
		})

		_, err := db.InsertAPIKey(r.Context(), database.InsertAPIKeyParams{
			ID:           id,
			UserID:       user.ID,
			HashedSecret: hashed[:],
			ExpiresAt:    database.Now().AddDate(0, 0, 1),
			LoginType:    database.LoginTypePassword,
			Scope:        database.APIKeyScopeApplicationConnect,
		})
		require.NoError(t, err)

		httpmw.ExtractAPIKey(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: false,
		})(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			// Checks that it exists on the context!
			apiKey := httpmw.APIKey(r)
			assert.Equal(t, database.APIKeyScopeApplicationConnect, apiKey.Scope)

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
			db         = databasefake.New()
			id, secret = randomAPIKeyParts()
			hashed     = sha256.Sum256([]byte(secret))
			r          = httptest.NewRequest("GET", "/", nil)
			rw         = httptest.NewRecorder()
			user       = createUser(r.Context(), t, db)
		)
		q := r.URL.Query()
		q.Add(codersdk.SessionTokenKey, fmt.Sprintf("%s-%s", id, secret))
		r.URL.RawQuery = q.Encode()

		_, err := db.InsertAPIKey(r.Context(), database.InsertAPIKeyParams{
			ID:           id,
			HashedSecret: hashed[:],
			ExpiresAt:    database.Now().AddDate(0, 0, 1),
			UserID:       user.ID,
			LoginType:    database.LoginTypePassword,
			Scope:        database.APIKeyScopeAll,
		})
		require.NoError(t, err)
		httpmw.ExtractAPIKey(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: false,
		})(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			// Checks that it exists on the context!
			_ = httpmw.APIKey(r)
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
			db         = databasefake.New()
			id, secret = randomAPIKeyParts()
			hashed     = sha256.Sum256([]byte(secret))
			r          = httptest.NewRequest("GET", "/", nil)
			rw         = httptest.NewRecorder()
			user       = createUser(r.Context(), t, db)
		)
		r.Header.Set(codersdk.SessionCustomHeader, fmt.Sprintf("%s-%s", id, secret))

		sentAPIKey, err := db.InsertAPIKey(r.Context(), database.InsertAPIKeyParams{
			ID:           id,
			HashedSecret: hashed[:],
			LastUsed:     database.Now().AddDate(0, 0, -1),
			ExpiresAt:    database.Now().AddDate(0, 0, 1),
			UserID:       user.ID,
			LoginType:    database.LoginTypePassword,
			Scope:        database.APIKeyScopeAll,
		})
		require.NoError(t, err)
		httpmw.ExtractAPIKey(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: false,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)

		gotAPIKey, err := db.GetAPIKeyByID(r.Context(), id)
		require.NoError(t, err)

		require.NotEqual(t, sentAPIKey.LastUsed, gotAPIKey.LastUsed)
		require.Equal(t, sentAPIKey.ExpiresAt, gotAPIKey.ExpiresAt)
	})

	t.Run("ValidUpdateExpiry", func(t *testing.T) {
		t.Parallel()
		var (
			db         = databasefake.New()
			id, secret = randomAPIKeyParts()
			hashed     = sha256.Sum256([]byte(secret))
			r          = httptest.NewRequest("GET", "/", nil)
			rw         = httptest.NewRecorder()
			user       = createUser(r.Context(), t, db)
		)
		r.Header.Set(codersdk.SessionCustomHeader, fmt.Sprintf("%s-%s", id, secret))

		sentAPIKey, err := db.InsertAPIKey(r.Context(), database.InsertAPIKeyParams{
			ID:           id,
			HashedSecret: hashed[:],
			LastUsed:     database.Now(),
			ExpiresAt:    database.Now().Add(time.Minute),
			UserID:       user.ID,
			LoginType:    database.LoginTypePassword,
			Scope:        database.APIKeyScopeAll,
		})
		require.NoError(t, err)
		httpmw.ExtractAPIKey(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: false,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)

		gotAPIKey, err := db.GetAPIKeyByID(r.Context(), id)
		require.NoError(t, err)

		require.Equal(t, sentAPIKey.LastUsed, gotAPIKey.LastUsed)
		require.NotEqual(t, sentAPIKey.ExpiresAt, gotAPIKey.ExpiresAt)
	})

	t.Run("OAuthNotExpired", func(t *testing.T) {
		t.Parallel()
		var (
			db         = databasefake.New()
			id, secret = randomAPIKeyParts()
			hashed     = sha256.Sum256([]byte(secret))
			r          = httptest.NewRequest("GET", "/", nil)
			rw         = httptest.NewRecorder()
			user       = createUser(r.Context(), t, db)
		)
		r.Header.Set(codersdk.SessionCustomHeader, fmt.Sprintf("%s-%s", id, secret))

		sentAPIKey, err := db.InsertAPIKey(r.Context(), database.InsertAPIKeyParams{
			ID:           id,
			HashedSecret: hashed[:],
			LoginType:    database.LoginTypeGithub,
			LastUsed:     database.Now(),
			ExpiresAt:    database.Now().AddDate(0, 0, 1),
			UserID:       user.ID,
			Scope:        database.APIKeyScopeAll,
		})
		require.NoError(t, err)

		_, err = db.InsertUserLink(r.Context(), database.InsertUserLinkParams{
			UserID:    user.ID,
			LoginType: database.LoginTypeGithub,
		})
		require.NoError(t, err)

		httpmw.ExtractAPIKey(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: false,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)

		gotAPIKey, err := db.GetAPIKeyByID(r.Context(), id)
		require.NoError(t, err)

		require.Equal(t, sentAPIKey.LastUsed, gotAPIKey.LastUsed)
		require.Equal(t, sentAPIKey.ExpiresAt, gotAPIKey.ExpiresAt)
	})

	t.Run("OAuthRefresh", func(t *testing.T) {
		t.Parallel()
		var (
			db         = databasefake.New()
			id, secret = randomAPIKeyParts()
			hashed     = sha256.Sum256([]byte(secret))
			r          = httptest.NewRequest("GET", "/", nil)
			rw         = httptest.NewRecorder()
			user       = createUser(r.Context(), t, db)
		)
		r.Header.Set(codersdk.SessionCustomHeader, fmt.Sprintf("%s-%s", id, secret))

		sentAPIKey, err := db.InsertAPIKey(r.Context(), database.InsertAPIKeyParams{
			ID:           id,
			HashedSecret: hashed[:],
			LoginType:    database.LoginTypeGithub,
			LastUsed:     database.Now(),
			UserID:       user.ID,
			Scope:        database.APIKeyScopeAll,
		})
		require.NoError(t, err)
		_, err = db.InsertUserLink(r.Context(), database.InsertUserLinkParams{
			UserID:            user.ID,
			LoginType:         database.LoginTypeGithub,
			OAuthExpiry:       database.Now().AddDate(0, 0, -1),
			OAuthRefreshToken: "hello",
		})
		require.NoError(t, err)

		token := &oauth2.Token{
			AccessToken:  "wow",
			RefreshToken: "moo",
			Expiry:       database.Now().AddDate(0, 0, 1),
		}
		httpmw.ExtractAPIKey(httpmw.ExtractAPIKeyConfig{
			DB: db,
			OAuth2Configs: &httpmw.OAuth2Configs{
				Github: &oauth2Config{
					tokenSource: oauth2TokenSource(func() (*oauth2.Token, error) {
						return token, nil
					}),
				},
			},
			RedirectToLogin: false,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)

		gotAPIKey, err := db.GetAPIKeyByID(r.Context(), id)
		require.NoError(t, err)

		require.Equal(t, sentAPIKey.LastUsed, gotAPIKey.LastUsed)
		require.Equal(t, token.Expiry, gotAPIKey.ExpiresAt)
	})

	t.Run("RemoteIPUpdates", func(t *testing.T) {
		t.Parallel()
		var (
			db         = databasefake.New()
			id, secret = randomAPIKeyParts()
			hashed     = sha256.Sum256([]byte(secret))
			r          = httptest.NewRequest("GET", "/", nil)
			rw         = httptest.NewRecorder()
			user       = createUser(r.Context(), t, db)
		)
		r.RemoteAddr = "1.1.1.1"
		r.Header.Set(codersdk.SessionCustomHeader, fmt.Sprintf("%s-%s", id, secret))

		_, err := db.InsertAPIKey(r.Context(), database.InsertAPIKeyParams{
			ID:           id,
			HashedSecret: hashed[:],
			LastUsed:     database.Now().AddDate(0, 0, -1),
			ExpiresAt:    database.Now().AddDate(0, 0, 1),
			UserID:       user.ID,
			LoginType:    database.LoginTypePassword,
			Scope:        database.APIKeyScopeAll,
		})
		require.NoError(t, err)
		httpmw.ExtractAPIKey(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: false,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)

		gotAPIKey, err := db.GetAPIKeyByID(r.Context(), id)
		require.NoError(t, err)

		require.Equal(t, net.ParseIP("1.1.1.1"), gotAPIKey.IPAddress.IPNet.IP)
	})

	t.Run("RedirectToLogin", func(t *testing.T) {
		t.Parallel()
		var (
			db = databasefake.New()
			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)

		httpmw.ExtractAPIKey(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: true,
		})(successHandler).ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusTemporaryRedirect, res.StatusCode)
		u, err := res.Location()
		require.NoError(t, err)
		require.Equal(t, "/login", u.Path)
	})

	t.Run("Optional", func(t *testing.T) {
		t.Parallel()
		var (
			db = databasefake.New()
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

		httpmw.ExtractAPIKey(httpmw.ExtractAPIKeyConfig{
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
			db         = databasefake.New()
			id, secret = randomAPIKeyParts()
			hashed     = sha256.Sum256([]byte(secret))
			r          = httptest.NewRequest("GET", "/", nil)
			rw         = httptest.NewRecorder()
			user       = createUser(r.Context(), t, db)
		)
		r.Header.Set(codersdk.SessionCustomHeader, fmt.Sprintf("%s-%s", id, secret))

		sentAPIKey, err := db.InsertAPIKey(r.Context(), database.InsertAPIKeyParams{
			ID:           id,
			HashedSecret: hashed[:],
			LoginType:    database.LoginTypeToken,
			LastUsed:     database.Now(),
			ExpiresAt:    database.Now().AddDate(0, 0, 1),
			UserID:       user.ID,
			Scope:        database.APIKeyScopeAll,
		})
		require.NoError(t, err)

		httpmw.ExtractAPIKey(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: false,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)

		gotAPIKey, err := db.GetAPIKeyByID(r.Context(), id)
		require.NoError(t, err)

		require.Equal(t, sentAPIKey.LastUsed, gotAPIKey.LastUsed)
		require.Equal(t, sentAPIKey.ExpiresAt, gotAPIKey.ExpiresAt)
		require.Equal(t, sentAPIKey.LoginType, gotAPIKey.LoginType)
	})
}

func createUser(ctx context.Context, t *testing.T, db database.Store, opts ...func(u *database.InsertUserParams)) database.User {
	insert := database.InsertUserParams{
		ID:             uuid.New(),
		Email:          "email@coder.com",
		Username:       "username",
		HashedPassword: []byte{},
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		RBACRoles:      []string{},
		LoginType:      database.LoginTypePassword,
	}
	for _, opt := range opts {
		opt(&insert)
	}
	user, err := db.InsertUser(ctx, insert)
	require.NoError(t, err, "create user")
	return user
}

type oauth2Config struct {
	tokenSource oauth2TokenSource
}

func (o *oauth2Config) TokenSource(context.Context, *oauth2.Token) oauth2.TokenSource {
	return o.tokenSource
}

func (*oauth2Config) AuthCodeURL(string, ...oauth2.AuthCodeOption) string {
	return ""
}

func (*oauth2Config) Exchange(context.Context, string, ...oauth2.AuthCodeOption) (*oauth2.Token, error) {
	return &oauth2.Token{}, nil
}

type oauth2TokenSource func() (*oauth2.Token, error)

func (o oauth2TokenSource) Token() (*oauth2.Token, error) {
	return o()
}

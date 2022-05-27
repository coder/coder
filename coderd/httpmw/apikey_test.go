package httpmw_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/databasefake"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
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
		httpapi.Write(rw, http.StatusOK, httpapi.Response{
			Message: "it worked!",
		})
	})

	t.Run("NoCookie", func(t *testing.T) {
		t.Parallel()
		var (
			db = databasefake.New()
			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)
		httpmw.ExtractAPIKey(db, nil)(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)
	})

	t.Run("InvalidFormat", func(t *testing.T) {
		t.Parallel()
		var (
			db = databasefake.New()
			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)
		r.AddCookie(&http.Cookie{
			Name:  httpmw.SessionTokenKey,
			Value: "test-wow-hello",
		})

		httpmw.ExtractAPIKey(db, nil)(successHandler).ServeHTTP(rw, r)
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
		r.AddCookie(&http.Cookie{
			Name:  httpmw.SessionTokenKey,
			Value: "test-wow",
		})

		httpmw.ExtractAPIKey(db, nil)(successHandler).ServeHTTP(rw, r)
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
		r.AddCookie(&http.Cookie{
			Name:  httpmw.SessionTokenKey,
			Value: "testtestid-wow",
		})

		httpmw.ExtractAPIKey(db, nil)(successHandler).ServeHTTP(rw, r)
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
		r.AddCookie(&http.Cookie{
			Name:  httpmw.SessionTokenKey,
			Value: fmt.Sprintf("%s-%s", id, secret),
		})

		httpmw.ExtractAPIKey(db, nil)(successHandler).ServeHTTP(rw, r)
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
		)
		r.AddCookie(&http.Cookie{
			Name:  httpmw.SessionTokenKey,
			Value: fmt.Sprintf("%s-%s", id, secret),
		})

		// Use a different secret so they don't match!
		hashed := sha256.Sum256([]byte("differentsecret"))
		_, err := db.InsertAPIKey(r.Context(), database.InsertAPIKeyParams{
			ID:           id,
			HashedSecret: hashed[:],
		})
		require.NoError(t, err)
		httpmw.ExtractAPIKey(db, nil)(successHandler).ServeHTTP(rw, r)
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
		)
		r.AddCookie(&http.Cookie{
			Name:  httpmw.SessionTokenKey,
			Value: fmt.Sprintf("%s-%s", id, secret),
		})

		_, err := db.InsertAPIKey(r.Context(), database.InsertAPIKeyParams{
			ID:           id,
			HashedSecret: hashed[:],
		})
		require.NoError(t, err)
		httpmw.ExtractAPIKey(db, nil)(successHandler).ServeHTTP(rw, r)
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
		)
		r.AddCookie(&http.Cookie{
			Name:  httpmw.SessionTokenKey,
			Value: fmt.Sprintf("%s-%s", id, secret),
		})

		sentAPIKey, err := db.InsertAPIKey(r.Context(), database.InsertAPIKeyParams{
			ID:           id,
			HashedSecret: hashed[:],
			ExpiresAt:    database.Now().AddDate(0, 0, 1),
		})
		require.NoError(t, err)
		httpmw.ExtractAPIKey(db, nil)(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			// Checks that it exists on the context!
			_ = httpmw.APIKey(r)
			httpapi.Write(rw, http.StatusOK, httpapi.Response{
				Message: "it worked!",
			})
		})).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)

		gotAPIKey, err := db.GetAPIKeyByID(r.Context(), id)
		require.NoError(t, err)

		require.Equal(t, sentAPIKey.ExpiresAt, gotAPIKey.ExpiresAt)
	})

	t.Run("QueryParameter", func(t *testing.T) {
		t.Parallel()
		var (
			db         = databasefake.New()
			id, secret = randomAPIKeyParts()
			hashed     = sha256.Sum256([]byte(secret))
			r          = httptest.NewRequest("GET", "/", nil)
			rw         = httptest.NewRecorder()
		)
		q := r.URL.Query()
		q.Add(httpmw.SessionTokenKey, fmt.Sprintf("%s-%s", id, secret))
		r.URL.RawQuery = q.Encode()

		_, err := db.InsertAPIKey(r.Context(), database.InsertAPIKeyParams{
			ID:           id,
			HashedSecret: hashed[:],
			ExpiresAt:    database.Now().AddDate(0, 0, 1),
		})
		require.NoError(t, err)
		httpmw.ExtractAPIKey(db, nil)(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			// Checks that it exists on the context!
			_ = httpmw.APIKey(r)
			httpapi.Write(rw, http.StatusOK, httpapi.Response{
				Message: "it worked!",
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
		)
		r.AddCookie(&http.Cookie{
			Name:  httpmw.SessionTokenKey,
			Value: fmt.Sprintf("%s-%s", id, secret),
		})

		sentAPIKey, err := db.InsertAPIKey(r.Context(), database.InsertAPIKeyParams{
			ID:           id,
			HashedSecret: hashed[:],
			LastUsed:     database.Now().AddDate(0, 0, -1),
			ExpiresAt:    database.Now().AddDate(0, 0, 1),
		})
		require.NoError(t, err)
		httpmw.ExtractAPIKey(db, nil)(successHandler).ServeHTTP(rw, r)
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
		)
		r.AddCookie(&http.Cookie{
			Name:  httpmw.SessionTokenKey,
			Value: fmt.Sprintf("%s-%s", id, secret),
		})

		sentAPIKey, err := db.InsertAPIKey(r.Context(), database.InsertAPIKeyParams{
			ID:           id,
			HashedSecret: hashed[:],
			LastUsed:     database.Now(),
			ExpiresAt:    database.Now().Add(time.Minute),
		})
		require.NoError(t, err)
		httpmw.ExtractAPIKey(db, nil)(successHandler).ServeHTTP(rw, r)
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
		)
		r.AddCookie(&http.Cookie{
			Name:  httpmw.SessionTokenKey,
			Value: fmt.Sprintf("%s-%s", id, secret),
		})

		sentAPIKey, err := db.InsertAPIKey(r.Context(), database.InsertAPIKeyParams{
			ID:           id,
			HashedSecret: hashed[:],
			LoginType:    database.LoginTypeGithub,
			LastUsed:     database.Now(),
			ExpiresAt:    database.Now().AddDate(0, 0, 1),
		})
		require.NoError(t, err)
		httpmw.ExtractAPIKey(db, nil)(successHandler).ServeHTTP(rw, r)
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
		)
		r.AddCookie(&http.Cookie{
			Name:  httpmw.SessionTokenKey,
			Value: fmt.Sprintf("%s-%s", id, secret),
		})

		sentAPIKey, err := db.InsertAPIKey(r.Context(), database.InsertAPIKeyParams{
			ID:           id,
			HashedSecret: hashed[:],
			LoginType:    database.LoginTypeGithub,
			LastUsed:     database.Now(),
			OAuthExpiry:  database.Now().AddDate(0, 0, -1),
		})
		require.NoError(t, err)
		token := &oauth2.Token{
			AccessToken:  "wow",
			RefreshToken: "moo",
			Expiry:       database.Now().AddDate(0, 0, 1),
		}
		httpmw.ExtractAPIKey(db, &httpmw.OAuth2Configs{
			Github: &oauth2Config{
				tokenSource: oauth2TokenSource(func() (*oauth2.Token, error) {
					return token, nil
				}),
			},
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)

		gotAPIKey, err := db.GetAPIKeyByID(r.Context(), id)
		require.NoError(t, err)

		require.Equal(t, sentAPIKey.LastUsed, gotAPIKey.LastUsed)
		require.Equal(t, token.Expiry, gotAPIKey.ExpiresAt)
		require.Equal(t, token.AccessToken, gotAPIKey.OAuthAccessToken)
	})
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

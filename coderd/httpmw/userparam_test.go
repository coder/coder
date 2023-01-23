package httpmw_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/databasefake"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/codersdk"
)

func TestUserParam(t *testing.T) {
	t.Parallel()
	setup := func(t *testing.T) (database.Store, *httptest.ResponseRecorder, *http.Request) {
		var (
			db         = databasefake.New()
			id, secret = randomAPIKeyParts()
			hashed     = sha256.Sum256([]byte(secret))
			r          = httptest.NewRequest("GET", "/", nil)
			rw         = httptest.NewRecorder()
		)
		r.Header.Set(codersdk.SessionCustomHeader, fmt.Sprintf("%s-%s", id, secret))

		user, err := db.InsertUser(r.Context(), database.InsertUserParams{
			ID:        uuid.New(),
			Email:     "admin@email.com",
			Username:  "admin",
			LoginType: database.LoginTypePassword,
		})
		require.NoError(t, err)

		_, err = db.InsertAPIKey(r.Context(), database.InsertAPIKeyParams{
			ID:           id,
			UserID:       user.ID,
			HashedSecret: hashed[:],
			LastUsed:     database.Now(),
			ExpiresAt:    database.Now().Add(time.Minute),
			LoginType:    database.LoginTypePassword,
			Scope:        database.APIKeyScopeAll,
		})
		require.NoError(t, err)

		return db, rw, r
	}

	t.Run("None", func(t *testing.T) {
		t.Parallel()
		db, rw, r := setup(t)

		httpmw.ExtractAPIKey(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: false,
		})(http.HandlerFunc(func(rw http.ResponseWriter, returnedRequest *http.Request) {
			r = returnedRequest
		})).ServeHTTP(rw, r)

		httpmw.ExtractUserParam(db, false)(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			rw.WriteHeader(http.StatusOK)
		})).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusBadRequest, res.StatusCode)
	})

	t.Run("NotMe", func(t *testing.T) {
		t.Parallel()
		db, rw, r := setup(t)

		httpmw.ExtractAPIKey(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: false,
		})(http.HandlerFunc(func(rw http.ResponseWriter, returnedRequest *http.Request) {
			r = returnedRequest
		})).ServeHTTP(rw, r)

		routeContext := chi.NewRouteContext()
		routeContext.URLParams.Add("user", "ben")
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, routeContext))
		httpmw.ExtractUserParam(db, false)(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			rw.WriteHeader(http.StatusOK)
		})).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusBadRequest, res.StatusCode)
	})

	t.Run("me", func(t *testing.T) {
		t.Parallel()
		db, rw, r := setup(t)

		httpmw.ExtractAPIKey(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: false,
		})(http.HandlerFunc(func(rw http.ResponseWriter, returnedRequest *http.Request) {
			r = returnedRequest
		})).ServeHTTP(rw, r)

		routeContext := chi.NewRouteContext()
		routeContext.URLParams.Add("user", "me")
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, routeContext))
		httpmw.ExtractUserParam(db, false)(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			_ = httpmw.UserParam(r)
			rw.WriteHeader(http.StatusOK)
		})).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
	})
}

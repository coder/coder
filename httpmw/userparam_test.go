package httpmw_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/database"
	"github.com/coder/coder/database/databasefake"
	"github.com/coder/coder/httpmw"
)

func TestUserParam(t *testing.T) {
	setup := func(t *testing.T) (database.Store, *httptest.ResponseRecorder, *http.Request) {
		var (
			db         = databasefake.New()
			id, secret = randomAPIKeyParts()
			hashed     = sha256.Sum256([]byte(secret))
			r          = httptest.NewRequest("GET", "/", nil)
			rw         = httptest.NewRecorder()
		)
		r.AddCookie(&http.Cookie{
			Name:  httpmw.AuthCookie,
			Value: fmt.Sprintf("%s-%s", id, secret),
		})

		_, err := db.InsertUser(r.Context(), database.InsertUserParams{
			ID: "bananas",
		})
		require.NoError(t, err)
		_, err = db.InsertAPIKey(r.Context(), database.InsertAPIKeyParams{
			ID:           id,
			UserID:       "bananas",
			HashedSecret: hashed[:],
			LastUsed:     database.Now(),
			ExpiresAt:    database.Now().Add(time.Minute),
		})
		require.NoError(t, err)
		return db, rw, r
	}

	t.Run("None", func(t *testing.T) {
		db, rw, r := setup(t)

		httpmw.ExtractAPIKey(db, nil)(http.HandlerFunc(func(rw http.ResponseWriter, returnedRequest *http.Request) {
			r = returnedRequest
		})).ServeHTTP(rw, r)

		httpmw.ExtractUserParam(db)(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			rw.WriteHeader(http.StatusOK)
		})).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusBadRequest, res.StatusCode)
	})

	t.Run("NotMe", func(t *testing.T) {
		db, rw, r := setup(t)

		httpmw.ExtractAPIKey(db, nil)(http.HandlerFunc(func(rw http.ResponseWriter, returnedRequest *http.Request) {
			r = returnedRequest
		})).ServeHTTP(rw, r)

		routeContext := chi.NewRouteContext()
		routeContext.URLParams.Add("user", "ben")
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, routeContext))
		httpmw.ExtractUserParam(db)(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			rw.WriteHeader(http.StatusOK)
		})).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusBadRequest, res.StatusCode)
	})

	t.Run("Me", func(t *testing.T) {
		db, rw, r := setup(t)

		httpmw.ExtractAPIKey(db, nil)(http.HandlerFunc(func(rw http.ResponseWriter, returnedRequest *http.Request) {
			r = returnedRequest
		})).ServeHTTP(rw, r)

		routeContext := chi.NewRouteContext()
		routeContext.URLParams.Add("user", "me")
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, routeContext))
		httpmw.ExtractUserParam(db)(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			_ = httpmw.UserParam(r)
			rw.WriteHeader(http.StatusOK)
		})).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
	})
}

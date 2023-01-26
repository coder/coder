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
	"github.com/coder/coder/cryptorand"
)

func TestOrganizationParam(t *testing.T) {
	t.Parallel()

	setupAuthentication := func(db database.Store) (*http.Request, database.User) {
		var (
			id, secret = randomAPIKeyParts()
			r          = httptest.NewRequest("GET", "/", nil)
			hashed     = sha256.Sum256([]byte(secret))
		)
		r.Header.Set(codersdk.SessionCustomHeader, fmt.Sprintf("%s-%s", id, secret))

		userID := uuid.New()
		username, err := cryptorand.String(8)
		require.NoError(t, err)

		user, err := db.InsertUser(r.Context(), database.InsertUserParams{
			ID:             userID,
			Email:          "testaccount@coder.com",
			HashedPassword: hashed[:],
			Username:       username,
			CreatedAt:      database.Now(),
			UpdatedAt:      database.Now(),
			LoginType:      database.LoginTypePassword,
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
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, chi.NewRouteContext()))
		return r, user
	}

	t.Run("None", func(t *testing.T) {
		t.Parallel()
		var (
			db   = databasefake.New()
			rw   = httptest.NewRecorder()
			r, _ = setupAuthentication(db)
			rtr  = chi.NewRouter()
		)
		rtr.Use(
			httpmw.ExtractAPIKey(httpmw.ExtractAPIKeyConfig{
				DB:              db,
				RedirectToLogin: false,
			}),
			httpmw.ExtractOrganizationParam(db),
		)
		rtr.Get("/", nil)
		rtr.ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusBadRequest, res.StatusCode)
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		var (
			db   = databasefake.New()
			rw   = httptest.NewRecorder()
			r, _ = setupAuthentication(db)
			rtr  = chi.NewRouter()
		)
		chi.RouteContext(r.Context()).URLParams.Add("organization", uuid.NewString())
		rtr.Use(
			httpmw.ExtractAPIKey(httpmw.ExtractAPIKeyConfig{
				DB:              db,
				RedirectToLogin: false,
			}),
			httpmw.ExtractOrganizationParam(db),
		)
		rtr.Get("/", nil)
		rtr.ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusNotFound, res.StatusCode)
	})

	t.Run("InvalidUUID", func(t *testing.T) {
		t.Parallel()
		var (
			db   = databasefake.New()
			rw   = httptest.NewRecorder()
			r, _ = setupAuthentication(db)
			rtr  = chi.NewRouter()
		)
		chi.RouteContext(r.Context()).URLParams.Add("organization", "not-a-uuid")
		rtr.Use(
			httpmw.ExtractAPIKey(httpmw.ExtractAPIKeyConfig{
				DB:              db,
				RedirectToLogin: false,
			}),
			httpmw.ExtractOrganizationParam(db),
		)
		rtr.Get("/", nil)
		rtr.ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusBadRequest, res.StatusCode)
	})

	t.Run("NotInOrganization", func(t *testing.T) {
		t.Parallel()
		var (
			db   = databasefake.New()
			rw   = httptest.NewRecorder()
			r, u = setupAuthentication(db)
			rtr  = chi.NewRouter()
		)
		organization, err := db.InsertOrganization(r.Context(), database.InsertOrganizationParams{
			ID:        uuid.New(),
			Name:      "test",
			CreatedAt: database.Now(),
			UpdatedAt: database.Now(),
		})
		require.NoError(t, err)
		chi.RouteContext(r.Context()).URLParams.Add("organization", organization.ID.String())
		chi.RouteContext(r.Context()).URLParams.Add("user", u.ID.String())
		rtr.Use(
			httpmw.ExtractAPIKey(httpmw.ExtractAPIKeyConfig{
				DB:              db,
				RedirectToLogin: false,
			}),
			httpmw.ExtractUserParam(db, false),
			httpmw.ExtractOrganizationParam(db),
			httpmw.ExtractOrganizationMemberParam(db),
		)
		rtr.Get("/", nil)
		rtr.ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusNotFound, res.StatusCode)
	})

	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		var (
			db      = databasefake.New()
			rw      = httptest.NewRecorder()
			r, user = setupAuthentication(db)
			rtr     = chi.NewRouter()
		)
		organization, err := db.InsertOrganization(r.Context(), database.InsertOrganizationParams{
			ID:        uuid.New(),
			Name:      "test",
			CreatedAt: database.Now(),
			UpdatedAt: database.Now(),
		})
		require.NoError(t, err)
		_, err = db.InsertOrganizationMember(r.Context(), database.InsertOrganizationMemberParams{
			OrganizationID: organization.ID,
			UserID:         user.ID,
			CreatedAt:      database.Now(),
			UpdatedAt:      database.Now(),
		})
		require.NoError(t, err)
		chi.RouteContext(r.Context()).URLParams.Add("organization", organization.ID.String())
		chi.RouteContext(r.Context()).URLParams.Add("user", user.ID.String())
		rtr.Use(
			httpmw.ExtractAPIKey(httpmw.ExtractAPIKeyConfig{
				DB:              db,
				RedirectToLogin: false,
			}),
			httpmw.ExtractOrganizationParam(db),
			httpmw.ExtractUserParam(db, false),
			httpmw.ExtractOrganizationMemberParam(db),
		)
		rtr.Get("/", func(rw http.ResponseWriter, r *http.Request) {
			_ = httpmw.OrganizationParam(r)
			_ = httpmw.OrganizationMemberParam(r)
			rw.WriteHeader(http.StatusOK)
		})
		rtr.ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
	})
}

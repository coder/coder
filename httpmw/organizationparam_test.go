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
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cryptorand"
	"github.com/coder/coder/database"
	"github.com/coder/coder/database/databasefake"
	"github.com/coder/coder/httpmw"
)

func TestOrganizationParam(t *testing.T) {
	t.Parallel()

	setupAuthentication := func(db database.Store) (*http.Request, database.User) {
		var (
			id, secret = randomAPIKeyParts()
			r          = httptest.NewRequest("GET", "/", nil)
			hashed     = sha256.Sum256([]byte(secret))
		)
		r.AddCookie(&http.Cookie{
			Name:  httpmw.AuthCookie,
			Value: fmt.Sprintf("%s-%s", id, secret),
		})
		userID, err := cryptorand.String(16)
		require.NoError(t, err)
		username, err := cryptorand.String(8)
		require.NoError(t, err)
		user, err := db.InsertUser(r.Context(), database.InsertUserParams{
			ID:             userID,
			Email:          "testaccount@coder.com",
			Name:           "example",
			LoginType:      database.LoginTypeBuiltIn,
			HashedPassword: hashed[:],
			Username:       username,
			CreatedAt:      database.Now(),
			UpdatedAt:      database.Now(),
		})
		require.NoError(t, err)
		_, err = db.InsertAPIKey(r.Context(), database.InsertAPIKeyParams{
			ID:           id,
			UserID:       user.ID,
			HashedSecret: hashed[:],
			LastUsed:     database.Now(),
			ExpiresAt:    database.Now().Add(time.Minute),
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
			httpmw.ExtractAPIKey(db, nil),
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
		chi.RouteContext(r.Context()).URLParams.Add("organization", "nothin")
		rtr.Use(
			httpmw.ExtractAPIKey(db, nil),
			httpmw.ExtractOrganizationParam(db),
		)
		rtr.Get("/", nil)
		rtr.ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusNotFound, res.StatusCode)
	})

	t.Run("NotInOrganization", func(t *testing.T) {
		t.Parallel()
		var (
			db   = databasefake.New()
			rw   = httptest.NewRecorder()
			r, _ = setupAuthentication(db)
			rtr  = chi.NewRouter()
		)
		organization, err := db.InsertOrganization(r.Context(), database.InsertOrganizationParams{
			ID:        uuid.NewString(),
			Name:      "test",
			CreatedAt: database.Now(),
			UpdatedAt: database.Now(),
		})
		require.NoError(t, err)
		chi.RouteContext(r.Context()).URLParams.Add("organization", organization.Name)
		rtr.Use(
			httpmw.ExtractAPIKey(db, nil),
			httpmw.ExtractOrganizationParam(db),
		)
		rtr.Get("/", nil)
		rtr.ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)
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
			ID:        uuid.NewString(),
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
		chi.RouteContext(r.Context()).URLParams.Add("organization", organization.Name)
		rtr.Use(
			httpmw.ExtractAPIKey(db, nil),
			httpmw.ExtractOrganizationParam(db),
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

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
	"github.com/coder/coder/cryptorand"
)

func TestWorkspaceParam(t *testing.T) {
	t.Parallel()

	setup := func(db database.Store) (*http.Request, database.User) {
		var (
			id, secret = randomAPIKeyParts()
			hashed     = sha256.Sum256([]byte(secret))
		)
		r := httptest.NewRequest("GET", "/", nil)
		r.AddCookie(&http.Cookie{
			Name:  httpmw.AuthCookie,
			Value: fmt.Sprintf("%s-%s", id, secret),
		})

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

		ctx := chi.NewRouteContext()
		ctx.URLParams.Add("user", "me")
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))
		return r, user
	}

	t.Run("None", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
		rtr := chi.NewRouter()
		rtr.Use(httpmw.ExtractWorkspaceParam(db))
		rtr.Get("/", nil)
		r, _ := setup(db)
		rw := httptest.NewRecorder()
		rtr.ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusBadRequest, res.StatusCode)
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
		rtr := chi.NewRouter()
		rtr.Use(httpmw.ExtractWorkspaceParam(db))
		rtr.Get("/", nil)
		r, _ := setup(db)
		chi.RouteContext(r.Context()).URLParams.Add("workspace", uuid.NewString())
		rw := httptest.NewRecorder()
		rtr.ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusNotFound, res.StatusCode)
	})

	t.Run("NonPersonal", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
		rtr := chi.NewRouter()
		rtr.Use(
			httpmw.ExtractAPIKey(db, nil),
			httpmw.ExtractWorkspaceParam(db),
		)
		rtr.Get("/", nil)
		r, _ := setup(db)
		workspace, err := db.InsertWorkspace(context.Background(), database.InsertWorkspaceParams{
			ID:      uuid.New(),
			OwnerID: uuid.New(),
			Name:    "hello",
		})
		require.NoError(t, err)
		chi.RouteContext(r.Context()).URLParams.Add("workspace", workspace.ID.String())
		rw := httptest.NewRecorder()
		rtr.ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)
	})

	t.Run("Found", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
		rtr := chi.NewRouter()
		rtr.Use(
			httpmw.ExtractAPIKey(db, nil),
			httpmw.ExtractWorkspaceParam(db),
		)
		rtr.Get("/", func(rw http.ResponseWriter, r *http.Request) {
			_ = httpmw.WorkspaceParam(r)
			rw.WriteHeader(http.StatusOK)
		})
		r, user := setup(db)
		workspace, err := db.InsertWorkspace(context.Background(), database.InsertWorkspaceParams{
			ID:      uuid.New(),
			OwnerID: user.ID,
			Name:    "hello",
		})
		require.NoError(t, err)
		chi.RouteContext(r.Context()).URLParams.Add("workspace", workspace.ID.String())
		rw := httptest.NewRecorder()
		rtr.ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
	})
}

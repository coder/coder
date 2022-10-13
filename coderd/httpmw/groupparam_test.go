package httpmw_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/databasefake"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/testutil"
)

func TestGroupParam(t *testing.T) {
	t.Parallel()

	setup := func(t *testing.T) (database.Store, database.Group) {
		t.Helper()

		ctx, _ := testutil.Context(t)
		db := databasefake.New()

		orgID := uuid.New()
		organization, err := db.InsertOrganization(ctx, database.InsertOrganizationParams{
			ID:          orgID,
			Name:        "banana",
			Description: "wowie",
			CreatedAt:   database.Now(),
			UpdatedAt:   database.Now(),
		})
		require.NoError(t, err)

		group, err := db.InsertGroup(ctx, database.InsertGroupParams{
			ID:             uuid.New(),
			Name:           "yeww",
			OrganizationID: organization.ID,
		})
		require.NoError(t, err)

		return db, group
	}

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		var (
			db, group = setup(t)
			r         = httptest.NewRequest("GET", "/", nil)
			w         = httptest.NewRecorder()
		)

		router := chi.NewRouter()
		router.Use(httpmw.ExtractGroupParam(db))
		router.Get("/", func(w http.ResponseWriter, r *http.Request) {
			g := httpmw.GroupParam(r)
			require.Equal(t, group, g)
			w.WriteHeader(http.StatusOK)
		})

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("group", group.ID.String())
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))

		router.ServeHTTP(w, r)

		res := w.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()

		var (
			db, group = setup(t)
			r         = httptest.NewRequest("GET", "/", nil)
			w         = httptest.NewRecorder()
		)

		router := chi.NewRouter()
		router.Use(httpmw.ExtractGroupParam(db))
		router.Get("/", func(w http.ResponseWriter, r *http.Request) {
			g := httpmw.GroupParam(r)
			require.Equal(t, group, g)
			w.WriteHeader(http.StatusOK)
		})

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("group", uuid.NewString())
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))

		router.ServeHTTP(w, r)

		res := w.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusNotFound, res.StatusCode)
	})
}

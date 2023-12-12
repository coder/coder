package httpmw_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmem"
	"github.com/coder/coder/v2/coderd/httpmw"
)

func TestGroupParam(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		var (
			db    = dbmem.New()
			group = dbgen.Group(t, db, database.Group{})
			r     = httptest.NewRequest("GET", "/", nil)
			w     = httptest.NewRecorder()
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
			db    = dbmem.New()
			group = dbgen.Group(t, db, database.Group{})
			r     = httptest.NewRequest("GET", "/", nil)
			w     = httptest.NewRecorder()
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

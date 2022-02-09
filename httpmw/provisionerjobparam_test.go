package httpmw_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/database"
	"github.com/coder/coder/database/databasefake"
	"github.com/coder/coder/httpmw"
)

func TestProvisionerJobParam(t *testing.T) {
	t.Parallel()

	setup := func(db database.Store) (*http.Request, database.ProvisionerJob) {
		r := httptest.NewRequest("GET", "/", nil)
		provisionerJob, err := db.InsertProvisionerJob(context.Background(), database.InsertProvisionerJobParams{
			ID: uuid.New(),
		})
		require.NoError(t, err)

		ctx := chi.NewRouteContext()
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))
		return r, provisionerJob
	}

	t.Run("None", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
		rtr := chi.NewRouter()
		rtr.Use(
			httpmw.ExtractProvisionerJobParam(db),
		)
		rtr.Get("/", nil)
		r, _ := setup(db)
		rw := httptest.NewRecorder()
		rtr.ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusBadRequest, res.StatusCode)
	})

	t.Run("BadUUID", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
		rtr := chi.NewRouter()
		rtr.Use(
			httpmw.ExtractProvisionerJobParam(db),
		)
		rtr.Get("/", nil)

		r, _ := setup(db)
		chi.RouteContext(r.Context()).URLParams.Add("provisionerjob", "nothin")
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
		rtr.Use(
			httpmw.ExtractProvisionerJobParam(db),
		)
		rtr.Get("/", nil)

		r, _ := setup(db)
		chi.RouteContext(r.Context()).URLParams.Add("provisionerjob", uuid.NewString())
		rw := httptest.NewRecorder()
		rtr.ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusNotFound, res.StatusCode)
	})

	t.Run("ProvisionerJob", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
		rtr := chi.NewRouter()
		rtr.Use(
			httpmw.ExtractProvisionerJobParam(db),
		)
		rtr.Get("/", func(rw http.ResponseWriter, r *http.Request) {
			_ = httpmw.ProvisionerJobParam(r)
			rw.WriteHeader(http.StatusOK)
		})

		r, job := setup(db)
		chi.RouteContext(r.Context()).URLParams.Add("provisionerjob", job.ID.String())
		rw := httptest.NewRecorder()
		rtr.ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
	})
}

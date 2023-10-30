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

func TestWorkspaceResourceParam(t *testing.T) {
	t.Parallel()

	setup := func(t *testing.T, db database.Store, jobType database.ProvisionerJobType) (*http.Request, database.WorkspaceResource) {
		r := httptest.NewRequest("GET", "/", nil)
		job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			Type:          jobType,
			Provisioner:   database.ProvisionerTypeEcho,
			StorageMethod: database.ProvisionerStorageMethodFile,
		})

		build := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
			JobID:      job.ID,
			Transition: database.WorkspaceTransitionStart,
			Reason:     database.BuildReasonInitiator,
		})

		resource := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
			JobID:      job.ID,
			Transition: database.WorkspaceTransitionStart,
		})

		routeCtx := chi.NewRouteContext()
		routeCtx.URLParams.Add("workspacebuild", build.ID.String())
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, routeCtx))
		return r, resource
	}

	t.Run("None", func(t *testing.T) {
		t.Parallel()
		db := dbmem.New()
		rtr := chi.NewRouter()
		rtr.Use(httpmw.ExtractWorkspaceResourceParam(db))
		rtr.Get("/", nil)
		r, _ := setup(t, db, database.ProvisionerJobTypeWorkspaceBuild)
		rw := httptest.NewRecorder()
		rtr.ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusBadRequest, res.StatusCode)
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		db := dbmem.New()
		rtr := chi.NewRouter()
		rtr.Use(
			httpmw.ExtractWorkspaceResourceParam(db),
		)
		rtr.Get("/", nil)

		r, _ := setup(t, db, database.ProvisionerJobTypeWorkspaceBuild)
		chi.RouteContext(r.Context()).URLParams.Add("workspaceresource", uuid.NewString())
		rw := httptest.NewRecorder()
		rtr.ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusNotFound, res.StatusCode)
	})

	t.Run("FoundBadJobType", func(t *testing.T) {
		t.Parallel()
		db := dbmem.New()
		rtr := chi.NewRouter()
		rtr.Use(
			httpmw.ExtractWorkspaceResourceParam(db),
		)
		rtr.Get("/", func(rw http.ResponseWriter, r *http.Request) {
			_ = httpmw.WorkspaceResourceParam(r)
			rw.WriteHeader(http.StatusOK)
		})

		r, job := setup(t, db, database.ProvisionerJobTypeTemplateVersionImport)
		chi.RouteContext(r.Context()).URLParams.Add("workspaceresource", job.ID.String())
		rw := httptest.NewRecorder()
		rtr.ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusBadRequest, res.StatusCode)
	})

	t.Run("Found", func(t *testing.T) {
		t.Parallel()
		db := dbmem.New()
		rtr := chi.NewRouter()
		rtr.Use(
			httpmw.ExtractWorkspaceResourceParam(db),
		)
		rtr.Get("/", func(rw http.ResponseWriter, r *http.Request) {
			_ = httpmw.WorkspaceResourceParam(r)
			rw.WriteHeader(http.StatusOK)
		})

		r, job := setup(t, db, database.ProvisionerJobTypeWorkspaceBuild)
		chi.RouteContext(r.Context()).URLParams.Add("workspaceresource", job.ID.String())
		rw := httptest.NewRecorder()
		rtr.ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
	})
}

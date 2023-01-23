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
)

func TestWorkspaceResourceParam(t *testing.T) {
	t.Parallel()

	setup := func(db database.Store, jobType database.ProvisionerJobType) (*http.Request, database.WorkspaceResource) {
		r := httptest.NewRequest("GET", "/", nil)
		job, err := db.InsertProvisionerJob(context.Background(), database.InsertProvisionerJobParams{
			ID:            uuid.New(),
			Type:          jobType,
			Provisioner:   database.ProvisionerTypeEcho,
			StorageMethod: database.ProvisionerStorageMethodFile,
		})
		require.NoError(t, err)
		workspaceBuild, err := db.InsertWorkspaceBuild(context.Background(), database.InsertWorkspaceBuildParams{
			ID:         uuid.New(),
			JobID:      job.ID,
			Transition: database.WorkspaceTransitionStart,
			Reason:     database.BuildReasonInitiator,
		})
		require.NoError(t, err)
		resource, err := db.InsertWorkspaceResource(context.Background(), database.InsertWorkspaceResourceParams{
			ID:         uuid.New(),
			JobID:      job.ID,
			Transition: database.WorkspaceTransitionStart,
		})
		require.NoError(t, err)

		ctx := chi.NewRouteContext()
		ctx.URLParams.Add("workspacebuild", workspaceBuild.ID.String())
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))
		return r, resource
	}

	t.Run("None", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
		rtr := chi.NewRouter()
		rtr.Use(httpmw.ExtractWorkspaceResourceParam(db))
		rtr.Get("/", nil)
		r, _ := setup(db, database.ProvisionerJobTypeWorkspaceBuild)
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
			httpmw.ExtractWorkspaceResourceParam(db),
		)
		rtr.Get("/", nil)

		r, _ := setup(db, database.ProvisionerJobTypeWorkspaceBuild)
		chi.RouteContext(r.Context()).URLParams.Add("workspaceresource", uuid.NewString())
		rw := httptest.NewRecorder()
		rtr.ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusNotFound, res.StatusCode)
	})

	t.Run("FoundBadJobType", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
		rtr := chi.NewRouter()
		rtr.Use(
			httpmw.ExtractWorkspaceResourceParam(db),
		)
		rtr.Get("/", func(rw http.ResponseWriter, r *http.Request) {
			_ = httpmw.WorkspaceResourceParam(r)
			rw.WriteHeader(http.StatusOK)
		})

		r, job := setup(db, database.ProvisionerJobTypeTemplateVersionImport)
		chi.RouteContext(r.Context()).URLParams.Add("workspaceresource", job.ID.String())
		rw := httptest.NewRecorder()
		rtr.ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusBadRequest, res.StatusCode)
	})

	t.Run("Found", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
		rtr := chi.NewRouter()
		rtr.Use(
			httpmw.ExtractWorkspaceResourceParam(db),
		)
		rtr.Get("/", func(rw http.ResponseWriter, r *http.Request) {
			_ = httpmw.WorkspaceResourceParam(r)
			rw.WriteHeader(http.StatusOK)
		})

		r, job := setup(db, database.ProvisionerJobTypeWorkspaceBuild)
		chi.RouteContext(r.Context()).URLParams.Add("workspaceresource", job.ID.String())
		rw := httptest.NewRecorder()
		rtr.ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
	})
}

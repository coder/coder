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
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
)

func TestWorkspaceAgentParam(t *testing.T) {
	t.Parallel()

	setupAuthentication := func(db database.Store) (*http.Request, database.WorkspaceAgent) {
		var (
			user     = dbgen.User(t, db, database.User{})
			_, token = dbgen.APIKey(t, db, database.APIKey{
				UserID: user.ID,
			})
			workspace = dbgen.Workspace(t, db, database.Workspace{
				OwnerID: user.ID,
			})
			build = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
				WorkspaceID: workspace.ID,
				Transition:  database.WorkspaceTransitionStart,
				Reason:      database.BuildReasonInitiator,
			})
			job = dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
				ID:            build.JobID,
				Type:          database.ProvisionerJobTypeWorkspaceBuild,
				Provisioner:   database.ProvisionerTypeEcho,
				StorageMethod: database.ProvisionerStorageMethodFile,
			})
			resource = dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
				JobID:      job.ID,
				Transition: database.WorkspaceTransitionStart,
			})
			agent = dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
				ResourceID: resource.ID,
			})
		)

		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set(codersdk.SessionTokenHeader, token)

		ctx := chi.NewRouteContext()
		ctx.URLParams.Add("user", user.ID.String())
		ctx.URLParams.Add("workspaceagent", agent.ID.String())
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))
		return r, agent
	}

	t.Run("None", func(t *testing.T) {
		t.Parallel()
		db := dbfake.New()
		rtr := chi.NewRouter()
		rtr.Use(httpmw.ExtractWorkspaceBuildParam(db))
		rtr.Get("/", nil)
		r, _ := setupAuthentication(db)
		rw := httptest.NewRecorder()
		rtr.ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusBadRequest, res.StatusCode)
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		db := dbfake.New()
		rtr := chi.NewRouter()
		rtr.Use(httpmw.ExtractWorkspaceAgentParam(db))
		rtr.Get("/", nil)

		r, _ := setupAuthentication(db)
		chi.RouteContext(r.Context()).URLParams.Add("workspaceagent", uuid.NewString())
		rw := httptest.NewRecorder()
		rtr.ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusNotFound, res.StatusCode)
	})

	t.Run("WorkspaceAgent", func(t *testing.T) {
		t.Parallel()
		db := dbfake.New()
		rtr := chi.NewRouter()
		rtr.Use(
			httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
				DB:              db,
				RedirectToLogin: false,
			}),
			httpmw.ExtractWorkspaceAgentParam(db),
		)
		rtr.Get("/", func(rw http.ResponseWriter, r *http.Request) {
			_ = httpmw.WorkspaceAgentParam(r)
			rw.WriteHeader(http.StatusOK)
		})

		r, _ := setupAuthentication(db)
		rw := httptest.NewRecorder()
		rtr.ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
	})
}

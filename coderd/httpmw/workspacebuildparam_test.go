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
	"github.com/coder/coder/v2/codersdk"
)

func TestWorkspaceBuildParam(t *testing.T) {
	t.Parallel()

	setupAuthentication := func(db database.Store) (*http.Request, database.WorkspaceTable) {
		var (
			user     = dbgen.User(t, db, database.User{})
			_, token = dbgen.APIKey(t, db, database.APIKey{
				UserID: user.ID,
			})
			workspace = dbgen.Workspace(t, db, database.WorkspaceTable{
				OwnerID: user.ID,
			})
		)

		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set(codersdk.SessionTokenHeader, token)

		ctx := chi.NewRouteContext()
		ctx.URLParams.Add("user", user.ID.String())
		ctx.URLParams.Add("workspace", workspace.Name)
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))
		return r, workspace
	}

	t.Run("None", func(t *testing.T) {
		t.Parallel()
		db := dbmem.New()
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
		db := dbmem.New()
		rtr := chi.NewRouter()
		rtr.Use(httpmw.ExtractWorkspaceBuildParam(db))
		rtr.Get("/", nil)

		r, _ := setupAuthentication(db)
		chi.RouteContext(r.Context()).URLParams.Add("workspacebuild", uuid.NewString())
		rw := httptest.NewRecorder()
		rtr.ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusNotFound, res.StatusCode)
	})

	t.Run("WorkspaceBuild", func(t *testing.T) {
		t.Parallel()
		db := dbmem.New()
		rtr := chi.NewRouter()
		rtr.Use(
			httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
				DB:              db,
				RedirectToLogin: false,
			}),
			httpmw.ExtractWorkspaceBuildParam(db),
			httpmw.ExtractWorkspaceParam(db),
		)
		rtr.Get("/", func(rw http.ResponseWriter, r *http.Request) {
			_ = httpmw.WorkspaceBuildParam(r)
			rw.WriteHeader(http.StatusOK)
		})

		r, workspace := setupAuthentication(db)
		workspaceBuild := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
			Transition:  database.WorkspaceTransitionStart,
			Reason:      database.BuildReasonInitiator,
			WorkspaceID: workspace.ID,
		})

		chi.RouteContext(r.Context()).URLParams.Add("workspacebuild", workspaceBuild.ID.String())
		rw := httptest.NewRecorder()
		rtr.ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
	})
}

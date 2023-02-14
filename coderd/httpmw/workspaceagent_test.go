package httpmw_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbfake"
	"github.com/coder/coder/coderd/database/dbgen"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/codersdk"
)

func TestWorkspaceAgent(t *testing.T) {
	t.Parallel()

	setup := func(db database.Store, token uuid.UUID) *http.Request {
		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set(codersdk.SessionTokenHeader, token.String())
		return r
	}

	t.Run("None", func(t *testing.T) {
		t.Parallel()
		db := dbfake.New()
		rtr := chi.NewRouter()
		rtr.Use(
			httpmw.ExtractWorkspaceAgent(db),
		)
		rtr.Get("/", nil)
		r := setup(db, uuid.New())
		rw := httptest.NewRecorder()
		rtr.ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)
	})

	t.Run("Found", func(t *testing.T) {
		t.Parallel()
		db := dbfake.New()
		var (
			user      = dbgen.User(t, db, database.User{})
			workspace = dbgen.Workspace(t, db, database.Workspace{
				OwnerID: user.ID,
			})
			job      = dbgen.ProvisionerJob(t, db, database.ProvisionerJob{})
			resource = dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
				JobID: job.ID,
			})
			_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
				WorkspaceID: workspace.ID,
				JobID:       job.ID,
			})
			agent = dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
				ResourceID: resource.ID,
			})
		)

		rtr := chi.NewRouter()
		rtr.Use(
			httpmw.ExtractWorkspaceAgent(db),
		)
		rtr.Get("/", func(rw http.ResponseWriter, r *http.Request) {
			_ = httpmw.WorkspaceAgent(r)
			rw.WriteHeader(http.StatusOK)
		})
		r := setup(db, agent.AuthToken)
		rw := httptest.NewRecorder()
		rtr.ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
	})
}

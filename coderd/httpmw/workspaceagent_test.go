package httpmw_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
)

func TestWorkspaceAgent(t *testing.T) {
	t.Parallel()

	t.Run("None", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)

		req, rtr := setup(t, db, uuid.New(), httpmw.ExtractWorkspaceAgent(
			httpmw.ExtractWorkspaceAgentConfig{
				DB:       db,
				Optional: false,
			}))

		rw := httptest.NewRecorder()
		req.Header.Set(codersdk.SessionTokenHeader, uuid.New().String())
		rtr.ServeHTTP(rw, req)

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)
	})

	t.Run("Found", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		authToken := uuid.New()
		req, rtr := setup(t, db, authToken, httpmw.ExtractWorkspaceAgent(
			httpmw.ExtractWorkspaceAgentConfig{
				DB:       db,
				Optional: false,
			}))

		rw := httptest.NewRecorder()
		req.Header.Set(codersdk.SessionTokenHeader, authToken.String())
		rtr.ServeHTTP(rw, req)

		//nolint:bodyclose // Closed in `t.Cleanup`
		res := rw.Result()
		t.Cleanup(func() { _ = res.Body.Close() })
		require.Equal(t, http.StatusOK, res.StatusCode)
	})
}

func setup(t testing.TB, db database.Store, authToken uuid.UUID, mw func(http.Handler) http.Handler) (*http.Request, http.Handler) {
	t.Helper()
	org := dbgen.Organization(t, db, database.Organization{})
	user := dbgen.User(t, db, database.User{
		Status: database.UserStatusActive,
	})
	_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         user.ID,
		OrganizationID: org.ID,
	})
	templateVersion := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
	})
	template := dbgen.Template(t, db, database.Template{
		OrganizationID:  org.ID,
		ActiveVersionID: templateVersion.ID,
		CreatedBy:       user.ID,
	})
	workspace := dbgen.Workspace(t, db, database.Workspace{
		OwnerID:        user.ID,
		OrganizationID: org.ID,
		TemplateID:     template.ID,
	})
	job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
		OrganizationID: org.ID,
	})
	resource := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
		JobID: job.ID,
	})
	_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		WorkspaceID:       workspace.ID,
		JobID:             job.ID,
		TemplateVersionID: templateVersion.ID,
	})
	_ = dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
		ResourceID: resource.ID,
		AuthToken:  authToken,
	})

	req := httptest.NewRequest("GET", "/", nil)
	rtr := chi.NewRouter()
	rtr.Use(mw)
	rtr.Get("/", func(rw http.ResponseWriter, r *http.Request) {
		_ = httpmw.WorkspaceAgent(r)
		rw.WriteHeader(http.StatusOK)
	})

	return req, rtr
}

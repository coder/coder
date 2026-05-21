package httpmw_test

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
)

func TestWorkspaceAgent(t *testing.T) {
	t.Parallel()

	t.Run("None", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)

		req, rtr, _, _ := setup(t, db, uuid.New(), httpmw.ExtractWorkspaceAgentAndLatestBuild(
			httpmw.ExtractWorkspaceAgentAndLatestBuildConfig{
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
		req, rtr, _, _ := setup(t, db, authToken, httpmw.ExtractWorkspaceAgentAndLatestBuild(
			httpmw.ExtractWorkspaceAgentAndLatestBuildConfig{
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

	t.Run("Latest", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		authToken := uuid.New()
		req, rtr, ws, tpv := setup(t, db, authToken, httpmw.ExtractWorkspaceAgentAndLatestBuild(
			httpmw.ExtractWorkspaceAgentAndLatestBuildConfig{
				DB:       db,
				Optional: false,
			}),
		)

		// Create a newer build
		job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			OrganizationID: ws.OrganizationID,
		})
		resource := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
			JobID: job.ID,
		})
		_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
			WorkspaceID:       ws.ID,
			JobID:             job.ID,
			TemplateVersionID: tpv.ID,
			BuildNumber:       2,
		})
		_ = dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
			ResourceID: resource.ID,
		})

		rw := httptest.NewRecorder()
		req.Header.Set(codersdk.SessionTokenHeader, authToken.String())
		rtr.ServeHTTP(rw, req)

		//nolint:bodyclose // Closed in `t.Cleanup`
		res := rw.Result()
		t.Cleanup(func() { _ = res.Body.Close() })
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)
	})

	t.Run("DuringShutdown", func(t *testing.T) {
		t.Parallel()
		db, ps := dbtestutil.NewDB(t)
		authToken := uuid.New()
		req, rtr, ws, tpv := setup(t, db, authToken, httpmw.ExtractWorkspaceAgentAndLatestBuild(
			httpmw.ExtractWorkspaceAgentAndLatestBuildConfig{
				DB:       db,
				Optional: false,
			}),
		)

		// Create a STOP build with running job (becomes latest).
		stopJob := dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{
			OrganizationID: ws.OrganizationID,
			JobStatus:      database.ProvisionerJobStatusRunning,
		})
		_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
			WorkspaceID:       ws.ID,
			JobID:             stopJob.ID,
			TemplateVersionID: tpv.ID,
			BuildNumber:       2,
			Transition:        database.WorkspaceTransitionStop,
		})

		// Agent should still authenticate during shutdown.
		rw := httptest.NewRecorder()
		req.Header.Set(codersdk.SessionTokenHeader, authToken.String())
		rtr.ServeHTTP(rw, req)

		//nolint:bodyclose // Closed in `t.Cleanup`
		res := rw.Result()
		t.Cleanup(func() { _ = res.Body.Close() })
		require.Equal(t, http.StatusOK, res.StatusCode, "agent should authenticate during stop build execution")
	})

	t.Run("AfterShutdownCompletes", func(t *testing.T) {
		t.Parallel()
		db, ps := dbtestutil.NewDB(t)
		authToken := uuid.New()
		req, rtr, ws, tpv := setup(t, db, authToken, httpmw.ExtractWorkspaceAgentAndLatestBuild(
			httpmw.ExtractWorkspaceAgentAndLatestBuildConfig{
				DB:       db,
				Optional: false,
			}),
		)

		// Create a STOP build with completed job (becomes latest).
		stopJob := dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{
			OrganizationID: ws.OrganizationID,
			JobStatus:      database.ProvisionerJobStatusSucceeded,
			CompletedAt: sql.NullTime{
				Time:  dbtime.Now(),
				Valid: true,
			},
		})
		_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
			WorkspaceID:       ws.ID,
			JobID:             stopJob.ID,
			TemplateVersionID: tpv.ID,
			BuildNumber:       2,
			Transition:        database.WorkspaceTransitionStop,
		})

		// Agent should NOT authenticate after stop job completes.
		rw := httptest.NewRecorder()
		req.Header.Set(codersdk.SessionTokenHeader, authToken.String())
		rtr.ServeHTTP(rw, req)

		//nolint:bodyclose // Closed in `t.Cleanup`
		res := rw.Result()
		t.Cleanup(func() { _ = res.Body.Close() })
		require.Equal(t, http.StatusUnauthorized, res.StatusCode, "agent should not authenticate after stop job completes")
	})

	t.Run("FailedStartBuild", func(t *testing.T) {
		t.Parallel()
		db, ps := dbtestutil.NewDB(t)
		authToken := uuid.New()

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
		workspace := dbgen.Workspace(t, db, database.WorkspaceTable{
			OwnerID:        user.ID,
			OrganizationID: org.ID,
			TemplateID:     template.ID,
		})

		// Create START build with FAILED job status.
		startJob := dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{
			OrganizationID: org.ID,
			JobStatus:      database.ProvisionerJobStatusFailed,
			StartedAt: sql.NullTime{
				Time:  dbtime.Now().Add(-time.Minute),
				Valid: true,
			},
			CompletedAt: sql.NullTime{
				Time:  dbtime.Now(),
				Valid: true,
			},
			Error: sql.NullString{
				String: "build failed",
				Valid:  true,
			},
			ErrorCode: sql.NullString{
				String: "FAILED",
				Valid:  true,
			},
		})
		resource := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
			JobID: startJob.ID,
		})
		_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
			WorkspaceID:       workspace.ID,
			JobID:             startJob.ID,
			TemplateVersionID: templateVersion.ID,
			BuildNumber:       1,
			Transition:        database.WorkspaceTransitionStart,
		})
		_ = dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
			ResourceID: resource.ID,
			AuthToken:  authToken,
		})

		// Create a STOP build with running job.
		stopJob := dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{
			OrganizationID: org.ID,
			JobStatus:      database.ProvisionerJobStatusRunning,
		})
		_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
			WorkspaceID:       workspace.ID,
			JobID:             stopJob.ID,
			TemplateVersionID: templateVersion.ID,
			BuildNumber:       2,
			Transition:        database.WorkspaceTransitionStop,
		})

		req := httptest.NewRequest("GET", "/", nil)
		rtr := chi.NewRouter()
		rtr.Use(httpmw.ExtractWorkspaceAgentAndLatestBuild(
			httpmw.ExtractWorkspaceAgentAndLatestBuildConfig{
				DB:       db,
				Optional: false,
			}))
		rtr.Get("/", func(rw http.ResponseWriter, r *http.Request) {
			_ = httpmw.WorkspaceAgent(r)
			rw.WriteHeader(http.StatusOK)
		})

		// Agent should NOT authenticate (start build failed).
		rw := httptest.NewRecorder()
		req.Header.Set(codersdk.SessionTokenHeader, authToken.String())
		rtr.ServeHTTP(rw, req)

		//nolint:bodyclose // Closed in `t.Cleanup`
		res := rw.Result()
		t.Cleanup(func() { _ = res.Body.Close() })
		require.Equal(t, http.StatusUnauthorized, res.StatusCode, "agent should not authenticate when start build failed")
	})
}

func setup(t testing.TB, db database.Store, authToken uuid.UUID, mw func(http.Handler) http.Handler) (*http.Request, http.Handler, database.WorkspaceTable, database.TemplateVersion) {
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
	workspace := dbgen.Workspace(t, db, database.WorkspaceTable{
		OwnerID:        user.ID,
		OrganizationID: org.ID,
		TemplateID:     template.ID,
	})
	job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
		OrganizationID: org.ID,
		JobStatus:      database.ProvisionerJobStatusSucceeded,
		StartedAt: sql.NullTime{
			Time:  dbtime.Now().Add(-30 * time.Second),
			Valid: true,
		},
		CompletedAt: sql.NullTime{
			Time:  dbtime.Now(),
			Valid: true,
		},
	})
	resource := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
		JobID: job.ID,
	})
	_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		WorkspaceID:       workspace.ID,
		JobID:             job.ID,
		TemplateVersionID: templateVersion.ID,
		BuildNumber:       1,
		Transition:        database.WorkspaceTransitionStart,
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

	return req, rtr, workspace, templateVersion
}

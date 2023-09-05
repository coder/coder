package telemetry_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/telemetry"
	"github.com/coder/coder/v2/testutil"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestTelemetry(t *testing.T) {
	t.Parallel()
	t.Run("Snapshot", func(t *testing.T) {
		t.Parallel()

		var err error

		db := dbfake.New()

		ctx := testutil.Context(t, testutil.WaitMedium)
		_, _ = dbgen.APIKey(t, db, database.APIKey{})
		_ = dbgen.ProvisionerJob(t, db, database.ProvisionerJob{
			Provisioner:   database.ProvisionerTypeTerraform,
			StorageMethod: database.ProvisionerStorageMethodFile,
			Type:          database.ProvisionerJobTypeTemplateVersionDryRun,
		})
		_ = dbgen.Template(t, db, database.Template{
			Provisioner: database.ProvisionerTypeTerraform,
		})
		_ = dbgen.TemplateVersion(t, db, database.TemplateVersion{})
		_ = dbgen.User(t, db, database.User{})
		_ = dbgen.Workspace(t, db, database.Workspace{})
		_ = dbgen.WorkspaceApp(t, db, database.WorkspaceApp{
			SharingLevel: database.AppSharingLevelOwner,
			Health:       database.WorkspaceAppHealthDisabled,
		})
		wsagent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{})
		// Update the workspace agent to have a valid subsystem.
		err = db.UpdateWorkspaceAgentStartupByID(ctx, database.UpdateWorkspaceAgentStartupByIDParams{
			ID:                wsagent.ID,
			Version:           wsagent.Version,
			ExpandedDirectory: wsagent.ExpandedDirectory,
			Subsystems: []database.WorkspaceAgentSubsystem{
				database.WorkspaceAgentSubsystemEnvbox,
				database.WorkspaceAgentSubsystemExectrace,
			},
		})
		require.NoError(t, err)

		_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
			Transition: database.WorkspaceTransitionStart,
			Reason:     database.BuildReasonAutostart,
		})
		_ = dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
			Transition: database.WorkspaceTransitionStart,
		})
		_ = dbgen.WorkspaceAgentStat(t, db, database.WorkspaceAgentStat{})
		_, err = db.InsertLicense(ctx, database.InsertLicenseParams{
			UploadedAt: dbtime.Now(),
			JWT:        "",
			Exp:        dbtime.Now().Add(time.Hour),
			UUID:       uuid.New(),
		})
		assert.NoError(t, err)
		_, _ = dbgen.WorkspaceProxy(t, db, database.WorkspaceProxy{})

		_, snapshot := collectSnapshot(t, db)
		require.Len(t, snapshot.ProvisionerJobs, 1)
		require.Len(t, snapshot.Licenses, 1)
		require.Len(t, snapshot.Templates, 1)
		require.Len(t, snapshot.TemplateVersions, 1)
		require.Len(t, snapshot.Users, 1)
		require.Len(t, snapshot.Workspaces, 1)
		require.Len(t, snapshot.WorkspaceApps, 1)
		require.Len(t, snapshot.WorkspaceAgents, 1)
		require.Len(t, snapshot.WorkspaceBuilds, 1)
		require.Len(t, snapshot.WorkspaceResources, 1)
		require.Len(t, snapshot.WorkspaceAgentStats, 1)
		require.Len(t, snapshot.WorkspaceProxies, 1)

		wsa := snapshot.WorkspaceAgents[0]
		require.Len(t, wsa.Subsystems, 2)
		require.Equal(t, string(database.WorkspaceAgentSubsystemEnvbox), wsa.Subsystems[0])
		require.Equal(t, string(database.WorkspaceAgentSubsystemExectrace), wsa.Subsystems[1])
	})
	t.Run("HashedEmail", func(t *testing.T) {
		t.Parallel()
		db := dbfake.New()
		_ = dbgen.User(t, db, database.User{
			Email: "kyle@coder.com",
		})
		_, snapshot := collectSnapshot(t, db)
		require.Len(t, snapshot.Users, 1)
		require.Equal(t, snapshot.Users[0].EmailHashed, "bb44bf07cf9a2db0554bba63a03d822c927deae77df101874496df5a6a3e896d@coder.com")
	})
}

// nolint:paralleltest
func TestTelemetryInstallSource(t *testing.T) {
	t.Setenv("CODER_TELEMETRY_INSTALL_SOURCE", "aws_marketplace")
	db := dbfake.New()
	deployment, _ := collectSnapshot(t, db)
	require.Equal(t, "aws_marketplace", deployment.InstallSource)
}

func collectSnapshot(t *testing.T, db database.Store) (*telemetry.Deployment, *telemetry.Snapshot) {
	t.Helper()
	deployment := make(chan *telemetry.Deployment, 64)
	snapshot := make(chan *telemetry.Snapshot, 64)
	r := chi.NewRouter()
	r.Post("/deployment", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, buildinfo.Version(), r.Header.Get(telemetry.VersionHeader))
		w.WriteHeader(http.StatusAccepted)
		dd := &telemetry.Deployment{}
		err := json.NewDecoder(r.Body).Decode(dd)
		require.NoError(t, err)
		deployment <- dd
	})
	r.Post("/snapshot", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, buildinfo.Version(), r.Header.Get(telemetry.VersionHeader))
		w.WriteHeader(http.StatusAccepted)
		ss := &telemetry.Snapshot{}
		err := json.NewDecoder(r.Body).Decode(ss)
		require.NoError(t, err)
		snapshot <- ss
	})
	server := httptest.NewServer(r)
	t.Cleanup(server.Close)
	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)
	reporter, err := telemetry.New(telemetry.Options{
		Database:     db,
		Logger:       slogtest.Make(t, nil).Leveled(slog.LevelDebug),
		URL:          serverURL,
		DeploymentID: uuid.NewString(),
	})
	require.NoError(t, err)
	t.Cleanup(reporter.Close)
	return <-deployment, <-snapshot
}

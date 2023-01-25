package telemetry_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/go-chi/chi"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/buildinfo"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/databasefake"
	"github.com/coder/coder/coderd/telemetry"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestTelemetry(t *testing.T) {
	t.Parallel()
	t.Run("Snapshot", func(t *testing.T) {
		t.Parallel()

		var err error

		db := databasefake.New()

		ctx := context.Background()
		_, err = db.InsertAPIKey(ctx, database.InsertAPIKeyParams{
			ID:        uuid.NewString(),
			LastUsed:  database.Now(),
			Scope:     database.APIKeyScopeAll,
			LoginType: database.LoginTypePassword,
		})
		assert.NoError(t, err)
		_, err = db.InsertParameterSchema(ctx, database.InsertParameterSchemaParams{
			ID:                       uuid.New(),
			CreatedAt:                database.Now(),
			DefaultSourceScheme:      database.ParameterSourceSchemeNone,
			DefaultDestinationScheme: database.ParameterDestinationSchemeNone,
			ValidationTypeSystem:     database.ParameterTypeSystemNone,
		})
		assert.NoError(t, err)
		_, err = db.InsertProvisionerJob(ctx, database.InsertProvisionerJobParams{
			ID:            uuid.New(),
			CreatedAt:     database.Now(),
			Provisioner:   database.ProvisionerTypeTerraform,
			StorageMethod: database.ProvisionerStorageMethodFile,
			Type:          database.ProvisionerJobTypeTemplateVersionDryRun,
		})
		assert.NoError(t, err)
		_, err = db.InsertTemplate(ctx, database.InsertTemplateParams{
			ID:          uuid.New(),
			CreatedAt:   database.Now(),
			Provisioner: database.ProvisionerTypeTerraform,
		})
		assert.NoError(t, err)
		_, err = db.InsertTemplateVersion(ctx, database.InsertTemplateVersionParams{
			ID:        uuid.New(),
			CreatedAt: database.Now(),
		})
		assert.NoError(t, err)
		_, err = db.InsertUser(ctx, database.InsertUserParams{
			ID:        uuid.New(),
			CreatedAt: database.Now(),
			LoginType: database.LoginTypePassword,
		})
		assert.NoError(t, err)
		_, err = db.InsertWorkspace(ctx, database.InsertWorkspaceParams{
			ID:        uuid.New(),
			CreatedAt: database.Now(),
		})
		assert.NoError(t, err)
		_, err = db.InsertWorkspaceApp(ctx, database.InsertWorkspaceAppParams{
			ID:           uuid.New(),
			CreatedAt:    database.Now(),
			SharingLevel: database.AppSharingLevelOwner,
			Health:       database.WorkspaceAppHealthDisabled,
		})
		assert.NoError(t, err)
		_, err = db.InsertWorkspaceAgent(ctx, database.InsertWorkspaceAgentParams{
			ID:        uuid.New(),
			CreatedAt: database.Now(),
		})
		assert.NoError(t, err)
		_, err = db.InsertWorkspaceBuild(ctx, database.InsertWorkspaceBuildParams{
			ID:         uuid.New(),
			CreatedAt:  database.Now(),
			Transition: database.WorkspaceTransitionStart,
			Reason:     database.BuildReasonAutostart,
		})
		assert.NoError(t, err)
		_, err = db.InsertWorkspaceResource(ctx, database.InsertWorkspaceResourceParams{
			ID:         uuid.New(),
			CreatedAt:  database.Now(),
			Transition: database.WorkspaceTransitionStart,
		})
		assert.NoError(t, err)
		_, err = db.InsertLicense(ctx, database.InsertLicenseParams{
			UploadedAt: database.Now(),
			JWT:        "",
			Exp:        database.Now().Add(time.Hour),
			Uuid: uuid.NullUUID{
				UUID:  uuid.New(),
				Valid: true,
			},
		})
		assert.NoError(t, err)
		snapshot := collectSnapshot(t, db)
		require.Len(t, snapshot.ParameterSchemas, 1)
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
	})
	t.Run("HashedEmail", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
		_, err := db.InsertUser(context.Background(), database.InsertUserParams{
			ID:        uuid.New(),
			Email:     "kyle@coder.com",
			CreatedAt: database.Now(),
			LoginType: database.LoginTypePassword,
		})
		require.NoError(t, err)
		snapshot := collectSnapshot(t, db)
		require.Len(t, snapshot.Users, 1)
		require.Equal(t, snapshot.Users[0].EmailHashed, "bb44bf07cf9a2db0554bba63a03d822c927deae77df101874496df5a6a3e896d@coder.com")
	})
}

func collectSnapshot(t *testing.T, db database.Store) *telemetry.Snapshot {
	t.Helper()
	deployment := make(chan struct{}, 64)
	snapshot := make(chan *telemetry.Snapshot, 64)
	r := chi.NewRouter()
	r.Post("/deployment", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, buildinfo.Version(), r.Header.Get(telemetry.VersionHeader))
		w.WriteHeader(http.StatusAccepted)
		deployment <- struct{}{}
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
	<-deployment
	return <-snapshot
}

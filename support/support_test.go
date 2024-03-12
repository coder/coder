package support_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/support"
	"github.com/coder/coder/v2/testutil"
)

func TestRun(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		cfg := coderdtest.DeploymentValues(t)
		cfg.Experiments = []string{"foo"}
		ctx := testutil.Context(t, testutil.WaitShort)
		client, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{
			DeploymentValues: cfg,
			Logger:           ptr.Ref(slog.Make(sloghuman.Sink(io.Discard))),
		})
		admin := coderdtest.CreateFirstUser(t, client)
		ws, agt := setupWorkspaceAndAgent(ctx, t, client, db, admin)

		bun, err := support.Run(ctx, &support.Deps{
			Client:      client,
			Log:         slogtest.Make(t, nil).Named("bundle").Leveled(slog.LevelDebug),
			WorkspaceID: ws.ID,
			AgentID:     agt.ID,
		})
		require.NoError(t, err)
		require.NotEmpty(t, bun)
		require.NotEmpty(t, bun.Deployment.BuildInfo)
		require.NotEmpty(t, bun.Deployment.Config)
		require.NotEmpty(t, bun.Deployment.Config.Options)
		assertSanitizedDeploymentConfig(t, bun.Deployment.Config)
		require.NotEmpty(t, bun.Deployment.HealthReport)
		require.NotEmpty(t, bun.Deployment.Experiments)
		require.NotEmpty(t, bun.Network.CoordinatorDebug)
		require.NotEmpty(t, bun.Network.TailnetDebug)
		require.NotNil(t, bun.Network.NetcheckLocal)
		require.NotNil(t, bun.Workspace.Workspace)
		assertSanitizedWorkspace(t, bun.Workspace.Workspace)
		require.NotEmpty(t, bun.Workspace.BuildLogs)
		require.NotNil(t, bun.Workspace.Agent)
		require.NotEmpty(t, bun.Workspace.AgentStartupLogs)
		require.NotEmpty(t, bun.Workspace.Template)
		require.NotEmpty(t, bun.Workspace.TemplateVersion)
		require.NotEmpty(t, bun.Workspace.TemplateFileBase64)
		require.NotNil(t, bun.Workspace.Parameters)
		require.NotEmpty(t, bun.Logs)
	})

	t.Run("OK_NoAgent", func(t *testing.T) {
		t.Parallel()
		cfg := coderdtest.DeploymentValues(t)
		cfg.Experiments = []string{"foo"}
		ctx := testutil.Context(t, testutil.WaitShort)
		client := coderdtest.New(t, &coderdtest.Options{
			DeploymentValues: cfg,
			Logger:           ptr.Ref(slog.Make(sloghuman.Sink(io.Discard))),
		})
		_ = coderdtest.CreateFirstUser(t, client)
		bun, err := support.Run(ctx, &support.Deps{
			Client: client,
			Log:    slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Named("bundle").Leveled(slog.LevelDebug),
		})
		require.NoError(t, err)
		require.NotEmpty(t, bun)
		require.NotEmpty(t, bun.Deployment.BuildInfo)
		require.NotEmpty(t, bun.Deployment.Config)
		require.NotEmpty(t, bun.Deployment.Config.Options)
		assertSanitizedDeploymentConfig(t, bun.Deployment.Config)
		require.NotEmpty(t, bun.Deployment.HealthReport)
		require.NotEmpty(t, bun.Deployment.Experiments)
		require.NotEmpty(t, bun.Network.CoordinatorDebug)
		require.NotEmpty(t, bun.Network.TailnetDebug)
		require.NotNil(t, bun.Workspace)
		assertSanitizedWorkspace(t, bun.Workspace.Workspace)
		require.NotEmpty(t, bun.Logs)
	})

	t.Run("NoAuth", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		client := coderdtest.New(t, &coderdtest.Options{
			Logger: ptr.Ref(slog.Make(sloghuman.Sink(io.Discard))),
		})
		bun, err := support.Run(ctx, &support.Deps{
			Client: client,
			Log:    slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Named("bundle").Leveled(slog.LevelDebug),
		})
		var sdkErr *codersdk.Error
		require.NotNil(t, bun)
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusUnauthorized, sdkErr.StatusCode())
		require.NotEmpty(t, bun)
		require.NotEmpty(t, bun.Logs)
	})

	t.Run("MissingPrivilege", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		client := coderdtest.New(t, &coderdtest.Options{
			Logger: ptr.Ref(slog.Make(sloghuman.Sink(io.Discard))),
		})
		admin := coderdtest.CreateFirstUser(t, client)
		memberClient, _ := coderdtest.CreateAnotherUser(t, client, admin.OrganizationID)
		bun, err := support.Run(ctx, &support.Deps{
			Client: memberClient,
			Log:    slogtest.Make(t, nil).Named("bundle").Leveled(slog.LevelDebug),
		})
		require.ErrorContains(t, err, "failed authorization check")
		require.NotEmpty(t, bun)
		require.NotEmpty(t, bun.Logs)
	})
}

func assertSanitizedDeploymentConfig(t *testing.T, dc *codersdk.DeploymentConfig) {
	t.Helper()
	for _, opt := range dc.Options {
		if opt.Annotations.IsSet("secret") {
			assert.Empty(t, opt.Value.String())
		}
	}
}

func assertSanitizedWorkspace(t *testing.T, ws codersdk.Workspace) {
	t.Helper()
	for _, res := range ws.LatestBuild.Resources {
		for _, agt := range res.Agents {
			for k, v := range agt.EnvironmentVariables {
				assert.Equal(t, "***REDACTED***", v, "environment variable %q not sanitized", k)
			}
		}
	}
}

func setupWorkspaceAndAgent(ctx context.Context, t *testing.T, client *codersdk.Client, db database.Store, user codersdk.CreateFirstUserResponse) (codersdk.Workspace, codersdk.WorkspaceAgent) {
	// This is a valid zip file
	zipBytes := make([]byte, 22)
	zipBytes[0] = 80
	zipBytes[1] = 75
	zipBytes[2] = 0o5
	zipBytes[3] = 0o6
	uploadRes, err := client.Upload(ctx, codersdk.ContentTypeZip, bytes.NewReader(zipBytes))
	require.NoError(t, err)

	tv := dbfake.TemplateVersion(t, db).
		FileID(uploadRes.ID).
		Seed(database.TemplateVersion{
			OrganizationID: user.OrganizationID,
			CreatedBy:      user.UserID,
		}).
		Do()
	wbr := dbfake.WorkspaceBuild(t, db, database.Workspace{
		OrganizationID: user.OrganizationID,
		OwnerID:        user.UserID,
		TemplateID:     tv.Template.ID,
	}).Resource().WithAgent().Do()
	ws, err := client.Workspace(ctx, wbr.Workspace.ID)
	require.NoError(t, err)
	agt := ws.LatestBuild.Resources[0].Agents[0]

	// Insert a provisioner job log
	_, err = db.InsertProvisionerJobLogs(ctx, database.InsertProvisionerJobLogsParams{
		JobID:     wbr.Build.JobID,
		CreatedAt: []time.Time{dbtime.Now()},
		Source:    []database.LogSource{database.LogSourceProvisionerDaemon},
		Level:     []database.LogLevel{database.LogLevelInfo},
		Stage:     []string{"The World"},
		Output:    []string{"Players"},
	})
	require.NoError(t, err)
	// Insert an agent log
	_, err = db.InsertWorkspaceAgentLogs(ctx, database.InsertWorkspaceAgentLogsParams{
		AgentID:      agt.ID,
		CreatedAt:    dbtime.Now(),
		Output:       []string{"Bond, James Bond"},
		Level:        []database.LogLevel{database.LogLevelInfo},
		LogSourceID:  wbr.Build.JobID,
		OutputLength: 0o7,
	})
	require.NoError(t, err)

	return ws, agt
}

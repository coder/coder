package support_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/support"
	"github.com/coder/coder/v2/testutil"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestRun(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		cfg := coderdtest.DeploymentValues(t)
		cfg.Experiments = []string{"foo"}
		ctx := testutil.Context(t, testutil.WaitLong)
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
		assertNotNilNotEmpty(t, bun, "bundle should be present")
		assertNotNilNotEmpty(t, bun.Deployment.BuildInfo, "deployment build info should be present")
		assertNotNilNotEmpty(t, bun.Deployment.Config, "deployment config should be present")
		assertNotNilNotEmpty(t, bun.Deployment.Config.Options, "deployment config should be present")
		assertSanitizedDeploymentConfig(t, bun.Deployment.Config)
		assertNotNilNotEmpty(t, bun.Deployment.HealthReport, "deployment health report should be present")
		assertNotNilNotEmpty(t, bun.Deployment.Experiments, "deployment experiments should be present")
		assertNotNilNotEmpty(t, bun.Network.CoordinatorDebug, "network coordinator debug should be present")
		assertNotNilNotEmpty(t, bun.Network.TailnetDebug, "network tailnet debug should be present")
		assertNotNilNotEmpty(t, bun.Network.Netcheck, "network netcheck should be present")
		assertNotNilNotEmpty(t, bun.Workspace.Workspace, "workspace should be present")
		assertSanitizedWorkspace(t, bun.Workspace.Workspace)
		assertNotNilNotEmpty(t, bun.Workspace.BuildLogs, "workspace build logs should be present")
		assertNotNilNotEmpty(t, bun.Workspace.Template, "workspace template should be present")
		assertNotNilNotEmpty(t, bun.Workspace.TemplateVersion, "workspace template version should be present")
		assertNotNilNotEmpty(t, bun.Workspace.TemplateFileBase64, "workspace template file should be present")
		require.NotNil(t, bun.Workspace.Parameters, "workspace parameters should be present")
		assertNotNilNotEmpty(t, bun.Agent.Agent, "agent should be present")
		assertSanitizedAgent(t, *bun.Agent.Agent)
		assertNotNilNotEmpty(t, bun.Agent.ListeningPorts, "agent listening ports should be present")
		assertNotNilNotEmpty(t, bun.Agent.Logs, "agent logs should be present")
		assertNotNilNotEmpty(t, bun.Agent.AgentMagicsockHTML, "agent magicsock should be present")
		assertNotNilNotEmpty(t, bun.Agent.ClientMagicsockHTML, "client magicsock should be present")
		assertNotNilNotEmpty(t, bun.Agent.PeerDiagnostics, "agent peer diagnostics should be present")
		assertNotNilNotEmpty(t, bun.Agent.PingResult, "agent ping result should be present")
		assertNotNilNotEmpty(t, bun.Agent.Prometheus, "agent prometheus metrics should be present")
		assertNotNilNotEmpty(t, bun.Agent.StartupLogs, "agent startup logs should be present")
		assertNotNilNotEmpty(t, bun.Logs, "bundle logs should be present")
	})

	t.Run("OK_NoWorkspace", func(t *testing.T) {
		t.Parallel()
		cfg := coderdtest.DeploymentValues(t)
		cfg.Experiments = []string{"foo"}
		ctx := testutil.Context(t, testutil.WaitLong)
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
		assertNotNilNotEmpty(t, bun, "bundle should be present")
		assertNotNilNotEmpty(t, bun.Deployment.BuildInfo, "deployment build info should be present")
		assertNotNilNotEmpty(t, bun.Deployment.Config, "deployment config should be present")
		assertNotNilNotEmpty(t, bun.Deployment.Config.Options, "deployment config should be present")
		assertSanitizedDeploymentConfig(t, bun.Deployment.Config)
		assertNotNilNotEmpty(t, bun.Deployment.HealthReport, "deployment health report should be present")
		assertNotNilNotEmpty(t, bun.Deployment.Experiments, "deployment experiments should be present")
		assertNotNilNotEmpty(t, bun.Network.CoordinatorDebug, "network coordinator debug should be present")
		assertNotNilNotEmpty(t, bun.Network.TailnetDebug, "network tailnet debug should be present")
		assert.Empty(t, bun.Network.Netcheck, "did not expect netcheck to be present")
		assert.Empty(t, bun.Workspace.Workspace, "did not expect workspace to be present")
		assert.Empty(t, bun.Agent, "did not expect agent to be present")
		assertNotNilNotEmpty(t, bun.Logs, "bundle logs should be present")
	})

	t.Run("NoAuth", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
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
		ctx := testutil.Context(t, testutil.WaitLong)
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
			assertSanitizedAgent(t, agt)
		}
	}
}

func assertSanitizedAgent(t *testing.T, agt codersdk.WorkspaceAgent) {
	t.Helper()
	for k, v := range agt.EnvironmentVariables {
		assert.Equal(t, "***REDACTED***", v, "agent %q environment variable %q not sanitized", agt.Name, k)
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

	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "coder-agent.log")
	require.NoError(t, os.WriteFile(logPath, []byte("hello from the agent"), 0o600))
	_ = agenttest.New(t, client.URL, wbr.AgentToken, func(o *agent.Options) {
		o.LogDir = tempDir
	})
	coderdtest.NewWorkspaceAgentWaiter(t, client, wbr.Workspace.ID).Wait()

	return ws, agt
}

func assertNotNilNotEmpty[T any](t *testing.T, v T, msg string) {
	t.Helper()

	if assert.NotNil(t, v, msg+" but was nil") {
		assert.NotEmpty(t, v, msg+" but was empty")
	}
}

package cli_test

import (
	"archive/zip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"tailscale.com/ipn/ipnstate"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/testutil"
)

func TestSupportBundle(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("for some reason, windows fails to remove tempdirs sometimes")
	}

	t.Run("Workspace", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		client, db := coderdtest.NewWithDatabase(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)
		r := dbfake.WorkspaceBuild(t, db, database.Workspace{
			OrganizationID: owner.OrganizationID,
			OwnerID:        owner.UserID,
		}).WithAgent().Do()
		ws, err := client.Workspace(ctx, r.Workspace.ID)
		require.NoError(t, err)
		tempDir := t.TempDir()
		logPath := filepath.Join(tempDir, "coder-agent.log")
		require.NoError(t, os.WriteFile(logPath, []byte("hello from the agent"), 0o600))
		agt := agenttest.New(t, client.URL, r.AgentToken, func(o *agent.Options) {
			o.LogDir = tempDir
		})
		defer agt.Close()
		coderdtest.NewWorkspaceAgentWaiter(t, client, r.Workspace.ID).Wait()

		// Insert a provisioner job log
		_, err = db.InsertProvisionerJobLogs(ctx, database.InsertProvisionerJobLogsParams{
			JobID:     r.Build.JobID,
			CreatedAt: []time.Time{dbtime.Now()},
			Source:    []database.LogSource{database.LogSourceProvisionerDaemon},
			Level:     []database.LogLevel{database.LogLevelInfo},
			Stage:     []string{"provision"},
			Output:    []string{"done"},
		})
		require.NoError(t, err)
		// Insert an agent log
		_, err = db.InsertWorkspaceAgentLogs(ctx, database.InsertWorkspaceAgentLogsParams{
			AgentID:      ws.LatestBuild.Resources[0].Agents[0].ID,
			CreatedAt:    dbtime.Now(),
			Output:       []string{"started up"},
			Level:        []database.LogLevel{database.LogLevelInfo},
			LogSourceID:  r.Build.JobID,
			OutputLength: 10,
		})
		require.NoError(t, err)

		d := t.TempDir()
		path := filepath.Join(d, "bundle.zip")
		inv, root := clitest.New(t, "support", "bundle", r.Workspace.Name, "--output", path)
		//nolint: gocritic // requires owner privilege
		clitest.SetupConfig(t, client, root)
		err = inv.Run()
		require.NoError(t, err)
		assertBundleContents(t, path)
	})

	t.Run("NoWorkspace", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		inv, root := clitest.New(t, "support", "bundle")
		//nolint: gocritic // requires owner privilege
		clitest.SetupConfig(t, client, root)
		err := inv.Run()
		require.ErrorContains(t, err, "must specify workspace name")
	})

	t.Run("NoAgent", func(t *testing.T) {
		t.Parallel()
		client, db := coderdtest.NewWithDatabase(t, nil)
		admin := coderdtest.CreateFirstUser(t, client)
		r := dbfake.WorkspaceBuild(t, db, database.Workspace{
			OrganizationID: admin.OrganizationID,
			OwnerID:        admin.UserID,
		}).Do() // without agent!
		inv, root := clitest.New(t, "support", "bundle", r.Workspace.Name)
		//nolint: gocritic // requires owner privilege
		clitest.SetupConfig(t, client, root)
		err := inv.Run()
		require.ErrorContains(t, err, "could not find agent")
	})

	t.Run("NoPrivilege", func(t *testing.T) {
		t.Parallel()
		client, db := coderdtest.NewWithDatabase(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		memberClient, member := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
		r := dbfake.WorkspaceBuild(t, db, database.Workspace{
			OrganizationID: user.OrganizationID,
			OwnerID:        member.ID,
		}).WithAgent().Do()
		inv, root := clitest.New(t, "support", "bundle", r.Workspace.Name)
		clitest.SetupConfig(t, memberClient, root)
		err := inv.Run()
		require.ErrorContains(t, err, "failed authorization check")
	})
}

func assertBundleContents(t *testing.T, path string) {
	t.Helper()
	r, err := zip.OpenReader(path)
	require.NoError(t, err, "open zip file")
	defer r.Close()
	for _, f := range r.File {
		switch f.Name {
		case "deployment/buildinfo.json":
			var v codersdk.BuildInfoResponse
			decodeJSONFromZip(t, f, &v)
			require.NotEmpty(t, v, "deployment build info should not be empty")
		case "deployment/config.json":
			var v codersdk.DeploymentConfig
			decodeJSONFromZip(t, f, &v)
			require.NotEmpty(t, v, "deployment config should not be empty")
		case "deployment/experiments.json":
			var v codersdk.Experiments
			decodeJSONFromZip(t, f, &v)
			require.NotEmpty(t, f, v, "experiments should not be empty")
		case "deployment/health.json":
			var v codersdk.HealthcheckReport
			decodeJSONFromZip(t, f, &v)
			require.NotEmpty(t, v, "health report should not be empty")
		case "network/coordinator_debug.html":
			bs := readBytesFromZip(t, f)
			require.NotEmpty(t, bs, "coordinator debug should not be empty")
		case "network/tailnet_debug.html":
			bs := readBytesFromZip(t, f)
			require.NotEmpty(t, bs, "tailnet debug should not be empty")
		case "network/netcheck.json":
			var v codersdk.WorkspaceAgentConnectionInfo
			decodeJSONFromZip(t, f, &v)
			require.NotEmpty(t, v, "connection info should not be empty")
		case "workspace/workspace.json":
			var v codersdk.Workspace
			decodeJSONFromZip(t, f, &v)
			require.NotEmpty(t, v, "workspace should not be empty")
		case "workspace/build_logs.txt":
			bs := readBytesFromZip(t, f)
			require.Contains(t, string(bs), "provision done")
		case "agent/agent.json":
			var v codersdk.WorkspaceAgent
			decodeJSONFromZip(t, f, &v)
			require.NotEmpty(t, v, "agent should not be empty")
		case "agent/listening_ports.json":
			var v codersdk.WorkspaceAgentListeningPortsResponse
			decodeJSONFromZip(t, f, &v)
			require.NotEmpty(t, v, "agent listening ports should not be empty")
		case "agent/logs.txt":
			bs := readBytesFromZip(t, f)
			require.NotEmpty(t, bs, "logs should not be empty")
		case "agent/magicsock.html":
			bs := readBytesFromZip(t, f)
			require.NotEmpty(t, bs, "agent magicsock should not be empty")
		case "agent/manifest.json":
			var v agentsdk.Manifest
			decodeJSONFromZip(t, f, &v)
			require.NotEmpty(t, v, "agent manifest should not be empty")
		case "agent/peer_diagnostics.json":
			var v *tailnet.PeerDiagnostics
			decodeJSONFromZip(t, f, &v)
			require.NotEmpty(t, v, "peer diagnostics should not be empty")
		case "agent/ping_result.json":
			var v *ipnstate.PingResult
			decodeJSONFromZip(t, f, &v)
			require.NotEmpty(t, v, "ping result should not be empty")
		case "agent/startup_logs.txt":
			bs := readBytesFromZip(t, f)
			require.Contains(t, string(bs), "started up")
		case "workspace/template.json":
			var v codersdk.Template
			decodeJSONFromZip(t, f, &v)
			require.NotEmpty(t, v, "workspace template should not be empty")
		case "workspace/template_version.json":
			var v codersdk.TemplateVersion
			decodeJSONFromZip(t, f, &v)
			require.NotEmpty(t, v, "workspace template version should not be empty")
		case "workspace/parameters.json":
			var v []codersdk.WorkspaceBuildParameter
			decodeJSONFromZip(t, f, &v)
			require.NotNil(t, v, "workspace parameters should not be nil")
		case "workspace/template_file.zip":
			bs := readBytesFromZip(t, f)
			require.NotNil(t, bs, "template file should not be nil")
		case "logs.txt":
			bs := readBytesFromZip(t, f)
			require.NotEmpty(t, bs, "logs should not be empty")
		default:
			require.Failf(t, "unexpected file in bundle", f.Name)
		}
	}
}

func decodeJSONFromZip(t *testing.T, f *zip.File, dest any) {
	t.Helper()
	rc, err := f.Open()
	require.NoError(t, err, "open file from zip")
	defer rc.Close()
	require.NoError(t, json.NewDecoder(rc).Decode(&dest))
}

func readBytesFromZip(t *testing.T, f *zip.File) []byte {
	t.Helper()
	rc, err := f.Open()
	require.NoError(t, err, "open file from zip")
	bs, err := io.ReadAll(rc)
	require.NoError(t, err, "read bytes from zip")
	return bs
}

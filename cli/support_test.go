package cli_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"tailscale.com/ipn/ipnstate"

	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/healthcheck/derphealth"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/codersdk/healthsdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/testutil"
)

func TestSupportBundle(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("for some reason, windows fails to remove tempdirs sometimes")
	}

	// Support bundle tests can share a single coderdtest instance.
	var dc codersdk.DeploymentConfig
	secretValue := uuid.NewString()
	seedSecretDeploymentOptions(t, &dc, secretValue)
	client, closer, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
		DeploymentValues:   dc.Values,
		HealthcheckTimeout: testutil.WaitSuperLong,
	})

	t.Cleanup(func() { closer.Close() })
	owner := coderdtest.CreateFirstUser(t, client)
	memberClient, member := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

	// Set up test fixtures
	setupCtx := testutil.Context(t, testutil.WaitSuperLong)
	workspaceWithAgent := setupSupportBundleTestFixture(setupCtx, t, api.Database, owner.OrganizationID, owner.UserID, func(agents []*proto.Agent) []*proto.Agent {
		// This should not show up in the bundle output
		agents[0].Env["SECRET_VALUE"] = secretValue
		return agents
	})
	workspaceWithoutAgent := setupSupportBundleTestFixture(setupCtx, t, api.Database, owner.OrganizationID, owner.UserID, nil)
	memberWorkspace := setupSupportBundleTestFixture(setupCtx, t, api.Database, owner.OrganizationID, member.ID, nil)

	// Wait for healthcheck to complete successfully before continuing with sub-tests.
	// The result is cached so subsequent requests will be fast.
	healthcheckDone := make(chan *healthsdk.HealthcheckReport)
	go func() {
		defer close(healthcheckDone)
		hc, err := healthsdk.New(client).DebugHealth(setupCtx)
		if err != nil {
			assert.NoError(t, err, "seed healthcheck cache")
			return
		}
		healthcheckDone <- &hc
	}()
	if _, ok := testutil.AssertReceive(setupCtx, t, healthcheckDone); !ok {
		t.Fatal("healthcheck did not complete in time -- this may be a transient issue")
	}

	t.Run("WorkspaceWithAgent", func(t *testing.T) {
		t.Parallel()

		tempDir := t.TempDir()
		logPath := filepath.Join(tempDir, "coder-agent.log")
		require.NoError(t, os.WriteFile(logPath, []byte("hello from the agent"), 0o600))
		agt := agenttest.New(t, client.URL, workspaceWithAgent.AgentToken, func(o *agent.Options) {
			o.LogDir = tempDir
		})
		defer agt.Close()
		coderdtest.NewWorkspaceAgentWaiter(t, client, workspaceWithAgent.Workspace.ID).Wait()

		d := t.TempDir()
		path := filepath.Join(d, "bundle.zip")
		inv, root := clitest.New(t, "support", "bundle", workspaceWithAgent.Workspace.Name, "--output-file", path, "--yes")
		//nolint: gocritic // requires owner privilege
		clitest.SetupConfig(t, client, root)
		err := inv.Run()
		require.NoError(t, err)
		assertBundleContents(t, path, true, true, []string{secretValue})
	})

	t.Run("NoWorkspace", func(t *testing.T) {
		t.Parallel()

		d := t.TempDir()
		path := filepath.Join(d, "bundle.zip")
		inv, root := clitest.New(t, "support", "bundle", "--output-file", path, "--yes")
		//nolint: gocritic // requires owner privilege
		clitest.SetupConfig(t, client, root)
		err := inv.Run()
		require.NoError(t, err)
		assertBundleContents(t, path, false, false, []string{secretValue})
	})

	t.Run("NoAgent", func(t *testing.T) {
		t.Parallel()
		d := t.TempDir()
		path := filepath.Join(d, "bundle.zip")
		inv, root := clitest.New(t, "support", "bundle", workspaceWithoutAgent.Workspace.Name, "--output-file", path, "--yes")
		//nolint: gocritic // requires owner privilege
		clitest.SetupConfig(t, client, root)
		err := inv.Run()
		require.NoError(t, err)
		assertBundleContents(t, path, true, false, []string{secretValue})
	})

	t.Run("NoPrivilege", func(t *testing.T) {
		t.Parallel()
		inv, root := clitest.New(t, "support", "bundle", memberWorkspace.Workspace.Name, "--yes")
		clitest.SetupConfig(t, memberClient, root)
		err := inv.Run()
		require.ErrorContains(t, err, "failed authorization check")
	})

	// This ensures that the CLI does not panic when trying to generate a support bundle
	// against a fake server that returns an empty response for all requests. This essentially
	// ensures that (almost) all of the support bundle generating code paths get a zero value.
	t.Run("DontPanic", func(t *testing.T) {
		t.Parallel()

		for _, code := range []int{
			http.StatusOK,
			http.StatusUnauthorized,
			http.StatusForbidden,
			http.StatusNotFound,
			http.StatusInternalServerError,
		} {
			t.Run(http.StatusText(code), func(t *testing.T) {
				t.Parallel()
				// Start up a fake server
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					t.Logf("received request: %s %s", r.Method, r.URL)
					switch r.URL.Path {
					case "/api/v2/authcheck":
						// Fake auth check
						resp := codersdk.AuthorizationResponse{
							"Read DeploymentValues": true,
						}
						w.WriteHeader(http.StatusOK)
						assert.NoError(t, json.NewEncoder(w).Encode(resp))
					default:
						// Simply return a blank response for everything else.
						w.WriteHeader(code)
					}
				}))
				defer srv.Close()
				u, err := url.Parse(srv.URL)
				require.NoError(t, err)
				client := codersdk.New(u)

				d := t.TempDir()
				path := filepath.Join(d, "bundle.zip")

				inv, root := clitest.New(t, "support", "bundle", "--url-override", srv.URL, "--output-file", path, "--yes")
				clitest.SetupConfig(t, client, root)
				err = inv.Run()
				require.NoError(t, err)
			})
		}
	})
}

// nolint:revive // It's a control flag, but this is just a test.
func assertBundleContents(t *testing.T, path string, wantWorkspace bool, wantAgent bool, badValues []string) {
	t.Helper()
	r, err := zip.OpenReader(path)
	require.NoError(t, err, "open zip file")
	defer r.Close()
	for _, f := range r.File {
		assertDoesNotContain(t, f, badValues...)
		switch f.Name {
		case "deployment/buildinfo.json":
			var v codersdk.BuildInfoResponse
			decodeJSONFromZip(t, f, &v)
			require.NotEmpty(t, v, "deployment build info should not be empty")
		case "deployment/config.json":
			var v codersdk.DeploymentConfig
			decodeJSONFromZip(t, f, &v)
			require.NotEmpty(t, v, "deployment config should not be empty")
		case "deployment/entitlements.json":
			var v codersdk.Entitlements
			decodeJSONFromZip(t, f, &v)
			require.NotNil(t, v, "entitlements should not be nil")
		case "deployment/experiments.json":
			var v codersdk.Experiments
			decodeJSONFromZip(t, f, &v)
			require.NotEmpty(t, f, v, "experiments should not be empty")
		case "deployment/health.json":
			var v healthsdk.HealthcheckReport
			decodeJSONFromZip(t, f, &v)
			require.NotEmpty(t, v, "health report should not be empty")
		case "deployment/health_settings.json":
			var v healthsdk.HealthSettings
			decodeJSONFromZip(t, f, &v)
			require.NotEmpty(t, v, "health settings should not be empty")
		case "deployment/stats.json":
			var v codersdk.DeploymentStats
			decodeJSONFromZip(t, f, &v)
			require.NotNil(t, v, "deployment stats should not be nil")
		case "deployment/workspaces.json":
			var v codersdk.Workspace
			decodeJSONFromZip(t, f, &v)
			require.NotNil(t, v, "deployment workspaces should not be nil")
		case "deployment/prometheus.txt":
			// Prometheus metrics may or may not be present depending on config.
			_ = readBytesFromZip(t, f)
		case "network/connection_info.json":
			var v workspacesdk.AgentConnectionInfo
			decodeJSONFromZip(t, f, &v)
			require.NotEmpty(t, v, "agent connection info should not be empty")
		case "network/coordinator_debug.html":
			bs := readBytesFromZip(t, f)
			require.NotEmpty(t, bs, "coordinator debug should not be empty")
		case "network/tailnet_debug.html":
			bs := readBytesFromZip(t, f)
			require.NotEmpty(t, bs, "tailnet debug should not be empty")
		case "network/netcheck.json":
			var v derphealth.Report
			decodeJSONFromZip(t, f, &v)
			require.NotEmpty(t, v, "netcheck should not be empty")
		case "network/interfaces.json":
			var v healthsdk.InterfacesReport
			decodeJSONFromZip(t, f, &v)
			require.NotEmpty(t, v, "interfaces should not be empty")
		case "workspace/workspace.json":
			var v codersdk.Workspace
			decodeJSONFromZip(t, f, &v)
			if !wantWorkspace {
				require.Empty(t, v, "expected workspace to be empty")
				continue
			}
			require.NotEmpty(t, v, "workspace should not be empty")
		case "workspace/build_logs.txt":
			bs := readBytesFromZip(t, f)
			if !wantWorkspace {
				require.Empty(t, bs, "expected workspace build logs to be empty")
				continue
			}
			require.Contains(t, string(bs), "provision done")
		case "workspace/template.json":
			var v codersdk.Template
			decodeJSONFromZip(t, f, &v)
			if !wantWorkspace {
				require.Empty(t, v, "expected workspace template to be empty")
				continue
			}
			require.NotEmpty(t, v, "workspace template should not be empty")
		case "workspace/template_version.json":
			var v codersdk.TemplateVersion
			decodeJSONFromZip(t, f, &v)
			if !wantWorkspace {
				require.Empty(t, v, "expected workspace template version to be empty")
				continue
			}
			require.NotEmpty(t, v, "workspace template version should not be empty")
		case "workspace/parameters.json":
			var v []codersdk.WorkspaceBuildParameter
			decodeJSONFromZip(t, f, &v)
			if !wantWorkspace {
				require.Empty(t, v, "expected workspace parameters to be empty")
				continue
			}
			require.NotNil(t, v, "workspace parameters should not be nil")
		case "workspace/template_file.zip":
			bs := readBytesFromZip(t, f)
			if !wantWorkspace {
				require.Empty(t, bs, "expected template file to be empty")
				continue
			}
			require.NotNil(t, bs, "template file should not be nil")
		case "agent/agent.json":
			var v codersdk.WorkspaceAgent
			decodeJSONFromZip(t, f, &v)
			if !wantAgent {
				require.Empty(t, v, "expected agent to be empty")
				continue
			}
			require.NotEmpty(t, v, "agent should not be empty")
		case "agent/listening_ports.json":
			var v codersdk.WorkspaceAgentListeningPortsResponse
			decodeJSONFromZip(t, f, &v)
			if !wantAgent {
				require.Empty(t, v, "expected agent listening ports to be empty")
				continue
			}
			require.NotEmpty(t, v, "agent listening ports should not be empty")
		case "agent/logs.txt":
			bs := readBytesFromZip(t, f)
			if !wantAgent {
				require.Empty(t, bs, "expected agent logs to be empty")
				continue
			}
			require.NotEmpty(t, bs, "logs should not be empty")
		case "agent/agent_magicsock.html":
			bs := readBytesFromZip(t, f)
			if !wantAgent {
				require.Empty(t, bs, "expected agent magicsock to be empty")
				continue
			}
			require.NotEmpty(t, bs, "agent magicsock should not be empty")
		case "agent/client_magicsock.html":
			bs := readBytesFromZip(t, f)
			if !wantAgent {
				require.Empty(t, bs, "expected client magicsock to be empty")
				continue
			}
			require.NotEmpty(t, bs, "client magicsock should not be empty")
		case "agent/manifest.json":
			var v agentsdk.Manifest
			decodeJSONFromZip(t, f, &v)
			if !wantAgent {
				require.Empty(t, v, "expected agent manifest to be empty")
				continue
			}
			require.NotEmpty(t, v, "agent manifest should not be empty")
		case "agent/peer_diagnostics.json":
			var v *tailnet.PeerDiagnostics
			decodeJSONFromZip(t, f, &v)
			if !wantAgent {
				require.Empty(t, v, "expected peer diagnostics to be empty")
				continue
			}
			require.NotEmpty(t, v, "peer diagnostics should not be empty")
		case "agent/ping_result.json":
			var v *ipnstate.PingResult
			decodeJSONFromZip(t, f, &v)
			if !wantAgent {
				require.Empty(t, v, "expected ping result to be empty")
				continue
			}
			require.NotEmpty(t, v, "ping result should not be empty")
		case "agent/prometheus.txt":
			bs := readBytesFromZip(t, f)
			if !wantAgent {
				require.Empty(t, bs, "expected agent prometheus metrics to be empty")
				continue
			}
			require.NotEmpty(t, bs, "agent prometheus metrics should not be empty")
		case "agent/startup_logs.txt":
			bs := readBytesFromZip(t, f)
			if !wantAgent {
				require.Empty(t, bs, "expected agent startup logs to be empty")
				continue
			}
			require.Contains(t, string(bs), "started up")
		case "logs.txt":
			bs := readBytesFromZip(t, f)
			require.NotEmpty(t, bs, "logs should not be empty")
		case "cli_logs.txt":
			bs := readBytesFromZip(t, f)
			require.NotEmpty(t, bs, "CLI logs should not be empty")
		case "license-status.txt":
			bs := readBytesFromZip(t, f)
			require.NotEmpty(t, bs, "license status should not be empty")
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

func assertDoesNotContain(t *testing.T, f *zip.File, vals ...string) {
	t.Helper()
	bs := readBytesFromZip(t, f)
	for _, val := range vals {
		if bytes.Contains(bs, []byte(val)) {
			t.Fatalf("file %q should not contain value %q", f.Name, val)
		}
	}
}

func seedSecretDeploymentOptions(t *testing.T, dc *codersdk.DeploymentConfig, secretValue string) {
	t.Helper()
	if dc == nil {
		dc = &codersdk.DeploymentConfig{}
	}
	for _, opt := range dc.Options {
		if codersdk.IsSecretDeploymentOption(opt) {
			opt.Value.Set(secretValue)
		}
	}
}

func setupSupportBundleTestFixture(
	ctx context.Context,
	t testing.TB,
	db database.Store,
	orgID, ownerID uuid.UUID,
	withAgent func([]*proto.Agent) []*proto.Agent,
) dbfake.WorkspaceResponse {
	t.Helper()
	// nolint: gocritic // Used for seeding test data only.
	ctx = dbauthz.AsSystemRestricted(ctx)
	b := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OrganizationID: orgID,
		OwnerID:        ownerID,
	})
	if withAgent != nil {
		b = b.WithAgent(withAgent)
	}
	r := b.Do()
	_, err := db.InsertProvisionerJobLogs(ctx, database.InsertProvisionerJobLogsParams{
		JobID:     r.Build.JobID,
		CreatedAt: []time.Time{dbtime.Now()},
		Source:    []database.LogSource{database.LogSourceProvisionerDaemon},
		Level:     []database.LogLevel{database.LogLevelInfo},
		Stage:     []string{"provision"},
		Output:    []string{"done"},
	})
	require.NoError(t, err)
	if withAgent != nil {
		res, err := db.GetWorkspaceResourcesByJobID(ctx, r.Build.JobID)
		require.NoError(t, err)
		var resIDs []uuid.UUID
		for _, res := range res {
			resIDs = append(resIDs, res.ID)
		}
		agents, err := db.GetWorkspaceAgentsByResourceIDs(ctx, resIDs)
		require.NoError(t, err)
		for _, agt := range agents {
			_, err = db.InsertWorkspaceAgentLogs(ctx, database.InsertWorkspaceAgentLogsParams{
				AgentID:      agt.ID,
				CreatedAt:    dbtime.Now(),
				Output:       []string{"started up"},
				Level:        []database.LogLevel{database.LogLevelInfo},
				LogSourceID:  r.Build.JobID,
				OutputLength: 10,
			})
			require.NoError(t, err)
		}
	}
	return r
}

package coderd_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
	"tailscale.com/tailcfg"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/coderdtest/oidctest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmem"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/externalauth"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/tailnet/tailnettest"
	"github.com/coder/coder/v2/testutil"
)

func TestWorkspaceAgent(t *testing.T) {
	t.Parallel()
	t.Run("Connect", func(t *testing.T) {
		t.Parallel()
		client, db := coderdtest.NewWithDatabase(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		tmpDir := t.TempDir()
		anotherClient, anotherUser := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)

		r := dbfake.WorkspaceBuild(t, db, database.Workspace{
			OrganizationID: user.OrganizationID,
			OwnerID:        anotherUser.ID,
		}).WithAgent(func(agents []*proto.Agent) []*proto.Agent {
			agents[0].Directory = tmpDir
			return agents
		}).Do()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		workspace, err := anotherClient.Workspace(ctx, r.Workspace.ID)
		require.NoError(t, err)
		require.Equal(t, tmpDir, workspace.LatestBuild.Resources[0].Agents[0].Directory)
		_, err = anotherClient.WorkspaceAgent(ctx, workspace.LatestBuild.Resources[0].Agents[0].ID)
		require.NoError(t, err)
		require.True(t, workspace.LatestBuild.Resources[0].Agents[0].Health.Healthy)
	})
	t.Run("HasFallbackTroubleshootingURL", func(t *testing.T) {
		t.Parallel()
		client, db := coderdtest.NewWithDatabase(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		tmpDir := t.TempDir()
		r := dbfake.WorkspaceBuild(t, db, database.Workspace{
			OrganizationID: user.OrganizationID,
			OwnerID:        user.UserID,
		}).WithAgent(func(agents []*proto.Agent) []*proto.Agent {
			agents[0].Directory = tmpDir
			return agents
		}).Do()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
		defer cancel()

		workspace, err := client.Workspace(ctx, r.Workspace.ID)
		require.NoError(t, err)
		require.NotEmpty(t, workspace.LatestBuild.Resources[0].Agents[0].TroubleshootingURL)
		t.Log(workspace.LatestBuild.Resources[0].Agents[0].TroubleshootingURL)
	})
	t.Run("Timeout", func(t *testing.T) {
		t.Parallel()
		// timeouts can cause error logs to be dropped on shutdown
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
		client, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{
			Logger: &logger,
		})
		user := coderdtest.CreateFirstUser(t, client)
		tmpDir := t.TempDir()

		wantTroubleshootingURL := "https://example.com/troubleshoot"

		r := dbfake.WorkspaceBuild(t, db, database.Workspace{
			OrganizationID: user.OrganizationID,
			OwnerID:        user.UserID,
		}).WithAgent(func(agents []*proto.Agent) []*proto.Agent {
			agents[0].Directory = tmpDir
			agents[0].ConnectionTimeoutSeconds = 1
			agents[0].TroubleshootingUrl = wantTroubleshootingURL
			return agents
		}).Do()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
		defer cancel()

		var err error
		var workspace codersdk.Workspace
		testutil.Eventually(ctx, t, func(ctx context.Context) (done bool) {
			workspace, err = client.Workspace(ctx, r.Workspace.ID)
			if !assert.NoError(t, err) {
				return false
			}
			return workspace.LatestBuild.Resources[0].Agents[0].Status == codersdk.WorkspaceAgentTimeout
		}, testutil.IntervalMedium, "agent status timeout")

		require.Equal(t, wantTroubleshootingURL, workspace.LatestBuild.Resources[0].Agents[0].TroubleshootingURL)
		require.False(t, workspace.LatestBuild.Resources[0].Agents[0].Health.Healthy)
		require.NotEmpty(t, workspace.LatestBuild.Resources[0].Agents[0].Health.Reason)
	})

	t.Run("DisplayApps", func(t *testing.T) {
		t.Parallel()
		client, db := coderdtest.NewWithDatabase(t, nil)
		user := coderdtest.CreateFirstUser(t, client)

		tmpDir := t.TempDir()
		apps := &proto.DisplayApps{
			Vscode:               true,
			VscodeInsiders:       true,
			WebTerminal:          true,
			PortForwardingHelper: true,
			SshHelper:            true,
		}
		r := dbfake.WorkspaceBuild(t, db, database.Workspace{
			OrganizationID: user.OrganizationID,
			OwnerID:        user.UserID,
		}).WithAgent(func(agents []*proto.Agent) []*proto.Agent {
			agents[0].Directory = tmpDir
			agents[0].DisplayApps = apps
			return agents
		}).Do()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		workspace, err := client.Workspace(ctx, r.Workspace.ID)
		require.NoError(t, err)
		agent, err := client.WorkspaceAgent(ctx, workspace.LatestBuild.Resources[0].Agents[0].ID)
		require.NoError(t, err)
		expectedApps := []codersdk.DisplayApp{
			codersdk.DisplayAppPortForward,
			codersdk.DisplayAppSSH,
			codersdk.DisplayAppVSCodeDesktop,
			codersdk.DisplayAppVSCodeInsiders,
			codersdk.DisplayAppWebTerminal,
		}
		require.ElementsMatch(t, expectedApps, agent.DisplayApps)

		// Flips all the apps to false.
		apps.PortForwardingHelper = false
		apps.Vscode = false
		apps.VscodeInsiders = false
		apps.SshHelper = false
		apps.WebTerminal = false

		// Creating another workspace is easier
		r = dbfake.WorkspaceBuild(t, db, database.Workspace{
			OrganizationID: user.OrganizationID,
			OwnerID:        user.UserID,
		}).WithAgent(func(agents []*proto.Agent) []*proto.Agent {
			agents[0].Directory = tmpDir
			agents[0].DisplayApps = apps
			return agents
		}).Do()
		workspace, err = client.Workspace(ctx, r.Workspace.ID)
		require.NoError(t, err)

		agent, err = client.WorkspaceAgent(ctx, workspace.LatestBuild.Resources[0].Agents[0].ID)
		require.NoError(t, err)
		require.Len(t, agent.DisplayApps, 0)
	})
}

func TestWorkspaceAgentLogs(t *testing.T) {
	t.Parallel()
	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		client, db := coderdtest.NewWithDatabase(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		r := dbfake.WorkspaceBuild(t, db, database.Workspace{
			OrganizationID: user.OrganizationID,
			OwnerID:        user.UserID,
		}).WithAgent().Do()

		agentClient := agentsdk.New(client.URL)
		agentClient.SetSessionToken(r.AgentToken)
		err := agentClient.PatchLogs(ctx, agentsdk.PatchLogs{
			Logs: []agentsdk.Log{
				{
					CreatedAt: dbtime.Now(),
					Output:    "testing",
				},
				{
					CreatedAt: dbtime.Now(),
					Output:    "testing2",
				},
			},
		})
		require.NoError(t, err)
		workspace, err := client.Workspace(ctx, r.Workspace.ID)
		require.NoError(t, err)
		logs, closer, err := client.WorkspaceAgentLogsAfter(ctx, workspace.LatestBuild.Resources[0].Agents[0].ID, 0, true)
		require.NoError(t, err)
		defer func() {
			_ = closer.Close()
		}()
		var logChunk []codersdk.WorkspaceAgentLog
		select {
		case <-ctx.Done():
		case logChunk = <-logs:
		}
		require.NoError(t, ctx.Err())
		require.Len(t, logChunk, 2) // No EOF.
		require.Equal(t, "testing", logChunk[0].Output)
		require.Equal(t, "testing2", logChunk[1].Output)
	})
	t.Run("Close logs on outdated build", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		client, db := coderdtest.NewWithDatabase(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		r := dbfake.WorkspaceBuild(t, db, database.Workspace{
			OrganizationID: user.OrganizationID,
			OwnerID:        user.UserID,
		}).WithAgent().Do()
		agentClient := agentsdk.New(client.URL)
		agentClient.SetSessionToken(r.AgentToken)
		err := agentClient.PatchLogs(ctx, agentsdk.PatchLogs{
			Logs: []agentsdk.Log{
				{
					CreatedAt: dbtime.Now(),
					Output:    "testing",
				},
			},
		})
		require.NoError(t, err)
		workspace, err := client.Workspace(ctx, r.Workspace.ID)
		require.NoError(t, err)
		logs, closer, err := client.WorkspaceAgentLogsAfter(ctx, workspace.LatestBuild.Resources[0].Agents[0].ID, 0, true)
		require.NoError(t, err)
		defer func() {
			_ = closer.Close()
		}()

		select {
		case <-ctx.Done():
			require.FailNow(t, "context done while waiting for first log")
		case <-logs:
		}

		_ = coderdtest.CreateWorkspaceBuild(t, client, workspace, database.WorkspaceTransitionStart)

		select {
		case <-ctx.Done():
			require.FailNow(t, "context done while waiting for logs close")
		case <-logs:
		}
	})
	t.Run("PublishesOnOverflow", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		client, db := coderdtest.NewWithDatabase(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		r := dbfake.WorkspaceBuild(t, db, database.Workspace{
			OrganizationID: user.OrganizationID,
			OwnerID:        user.UserID,
		}).WithAgent().Do()
		updates, err := client.WatchWorkspace(ctx, r.Workspace.ID)
		require.NoError(t, err)

		agentClient := agentsdk.New(client.URL)
		agentClient.SetSessionToken(r.AgentToken)
		err = agentClient.PatchLogs(ctx, agentsdk.PatchLogs{
			Logs: []agentsdk.Log{{
				CreatedAt: dbtime.Now(),
				Output:    strings.Repeat("a", (1<<20)+1),
			}},
		})
		var apiError *codersdk.Error
		require.ErrorAs(t, err, &apiError)
		require.Equal(t, http.StatusRequestEntityTooLarge, apiError.StatusCode())

		// It's possible we have multiple updates queued, but that's alright, we just
		// wait for the one where it overflows.
		for {
			var update codersdk.Workspace
			select {
			case <-ctx.Done():
				t.FailNow()
			case update = <-updates:
			}
			if update.LatestBuild.Resources[0].Agents[0].LogsOverflowed {
				break
			}
		}
	})
}

func TestWorkspaceAgentListen(t *testing.T) {
	t.Parallel()

	t.Run("Connect", func(t *testing.T) {
		t.Parallel()

		client, db := coderdtest.NewWithDatabase(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		r := dbfake.WorkspaceBuild(t, db, database.Workspace{
			OrganizationID: user.OrganizationID,
			OwnerID:        user.UserID,
		}).WithAgent().Do()
		_ = agenttest.New(t, client.URL, r.AgentToken)
		resources := coderdtest.AwaitWorkspaceAgents(t, client, r.Workspace.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		conn, err := client.DialWorkspaceAgent(ctx, resources[0].Agents[0].ID, nil)
		require.NoError(t, err)
		defer func() {
			_ = conn.Close()
		}()
		conn.AwaitReachable(ctx)
	})

	t.Run("FailNonLatestBuild", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{
			IncludeProvisionerDaemon: true,
		})

		user := coderdtest.CreateFirstUser(t, client)
		authToken := uuid.NewString()
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionPlan:  echo.PlanComplete,
			ProvisionApply: echo.ProvisionApplyWithAgent(authToken),
		})

		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

		version = coderdtest.UpdateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:         echo.ParseComplete,
			ProvisionPlan: echo.PlanComplete,
			ProvisionApply: []*proto.Response{{
				Type: &proto.Response_Apply{
					Apply: &proto.ApplyComplete{
						Resources: []*proto.Resource{{
							Name: "example",
							Type: "aws_instance",
							Agents: []*proto.Agent{{
								Id: uuid.NewString(),
								Auth: &proto.Agent_Token{
									Token: uuid.NewString(),
								},
							}},
						}},
					},
				},
			}},
		}, template.ID)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		stopBuild, err := client.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
			TemplateVersionID: version.ID,
			Transition:        codersdk.WorkspaceTransitionStop,
		})
		require.NoError(t, err)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, stopBuild.ID)

		agentClient := agentsdk.New(client.URL)
		agentClient.SetSessionToken(authToken)

		_, err = agentClient.Listen(ctx)
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusForbidden, sdkErr.StatusCode())
	})
}

func TestWorkspaceAgentTailnet(t *testing.T) {
	t.Parallel()
	client, db := coderdtest.NewWithDatabase(t, nil)
	user := coderdtest.CreateFirstUser(t, client)

	r := dbfake.WorkspaceBuild(t, db, database.Workspace{
		OrganizationID: user.OrganizationID,
		OwnerID:        user.UserID,
	}).WithAgent().Do()

	_ = agenttest.New(t, client.URL, r.AgentToken)
	resources := coderdtest.AwaitWorkspaceAgents(t, client, r.Workspace.ID)

	conn, err := func() (*codersdk.WorkspaceAgentConn, error) {
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel() // Connection should remain open even if the dial context is canceled.

		return client.DialWorkspaceAgent(ctx, resources[0].Agents[0].ID, &codersdk.DialWorkspaceAgentOptions{
			Logger: slogtest.Make(t, nil).Named("client").Leveled(slog.LevelDebug),
		})
	}()
	require.NoError(t, err)
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	sshClient, err := conn.SSHClient(ctx)
	require.NoError(t, err)
	session, err := sshClient.NewSession()
	require.NoError(t, err)
	output, err := session.CombinedOutput("echo test")
	require.NoError(t, err)
	_ = session.Close()
	_ = sshClient.Close()
	_ = conn.Close()
	require.Equal(t, "test", strings.TrimSpace(string(output)))
}

func TestWorkspaceAgentClientCoordinate_BadVersion(t *testing.T) {
	t.Parallel()
	client, db := coderdtest.NewWithDatabase(t, nil)
	user := coderdtest.CreateFirstUser(t, client)

	r := dbfake.WorkspaceBuild(t, db, database.Workspace{
		OrganizationID: user.OrganizationID,
		OwnerID:        user.UserID,
	}).WithAgent().Do()

	ctx := testutil.Context(t, testutil.WaitShort)
	agentToken, err := uuid.Parse(r.AgentToken)
	require.NoError(t, err)
	//nolint: gocritic // testing
	ao, err := db.GetWorkspaceAgentAndOwnerByAuthToken(dbauthz.AsSystemRestricted(ctx), agentToken)
	require.NoError(t, err)

	//nolint: bodyclose // closed by ReadBodyAsError
	resp, err := client.Request(ctx, http.MethodGet,
		fmt.Sprintf("api/v2/workspaceagents/%s/coordinate", ao.WorkspaceAgent.ID),
		nil,
		codersdk.WithQueryParam("version", "99.99"))
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	err = codersdk.ReadBodyAsError(resp)
	var sdkErr *codersdk.Error
	require.ErrorAs(t, err, &sdkErr)
	require.Equal(t, "Unknown or unsupported API version", sdkErr.Message)
	require.Len(t, sdkErr.Validations, 1)
	require.Equal(t, "version", sdkErr.Validations[0].Field)
}

func TestWorkspaceAgentTailnetDirectDisabled(t *testing.T) {
	t.Parallel()

	dv := coderdtest.DeploymentValues(t)
	err := dv.DERP.Config.BlockDirect.Set("true")
	require.NoError(t, err)
	require.True(t, dv.DERP.Config.BlockDirect.Value())

	client, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{
		DeploymentValues: dv,
	})
	user := coderdtest.CreateFirstUser(t, client)
	r := dbfake.WorkspaceBuild(t, db, database.Workspace{
		OrganizationID: user.OrganizationID,
		OwnerID:        user.UserID,
	}).WithAgent().Do()
	ctx := testutil.Context(t, testutil.WaitLong)

	// Verify that the manifest has DisableDirectConnections set to true.
	agentClient := agentsdk.New(client.URL)
	agentClient.SetSessionToken(r.AgentToken)
	manifest, err := agentClient.Manifest(ctx)
	require.NoError(t, err)
	require.True(t, manifest.DisableDirectConnections)

	_ = agenttest.New(t, client.URL, r.AgentToken)
	resources := coderdtest.AwaitWorkspaceAgents(t, client, r.Workspace.ID)
	agentID := resources[0].Agents[0].ID

	// Verify that the connection data has no STUN ports and
	// DisableDirectConnections set to true.
	res, err := client.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspaceagents/%s/connection", agentID), nil)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)
	var connInfo codersdk.WorkspaceAgentConnectionInfo
	err = json.NewDecoder(res.Body).Decode(&connInfo)
	require.NoError(t, err)
	require.True(t, connInfo.DisableDirectConnections)
	for _, region := range connInfo.DERPMap.Regions {
		t.Logf("region %s (%v)", region.RegionCode, region.EmbeddedRelay)
		for _, node := range region.Nodes {
			t.Logf("  node %s (stun %d)", node.Name, node.STUNPort)
			require.EqualValues(t, -1, node.STUNPort)
			// tailnet.NewDERPMap() will create nodes with "stun" in the name,
			// but not if direct is disabled.
			require.NotContains(t, node.Name, "stun")
			require.False(t, node.STUNOnly)
		}
	}

	conn, err := client.DialWorkspaceAgent(ctx, resources[0].Agents[0].ID, &codersdk.DialWorkspaceAgentOptions{
		Logger: slogtest.Make(t, nil).Named("client").Leveled(slog.LevelDebug),
	})
	require.NoError(t, err)
	defer conn.Close()

	require.True(t, conn.AwaitReachable(ctx))
	_, p2p, _, err := conn.Ping(ctx)
	require.NoError(t, err)
	require.False(t, p2p)
}

func TestWorkspaceAgentListeningPorts(t *testing.T) {
	t.Parallel()

	setup := func(t *testing.T, apps []*proto.App, dv *codersdk.DeploymentValues) (*codersdk.Client, uint16, uuid.UUID) {
		t.Helper()

		client, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{
			DeploymentValues: dv,
		})
		coderdPort, err := strconv.Atoi(client.URL.Port())
		require.NoError(t, err)

		user := coderdtest.CreateFirstUser(t, client)
		r := dbfake.WorkspaceBuild(t, db, database.Workspace{
			OrganizationID: user.OrganizationID,
			OwnerID:        user.UserID,
		}).WithAgent(func(agents []*proto.Agent) []*proto.Agent {
			agents[0].Apps = apps
			return agents
		}).Do()
		_ = agenttest.New(t, client.URL, r.AgentToken, func(o *agent.Options) {
			o.PortCacheDuration = time.Millisecond
		})
		resources := coderdtest.AwaitWorkspaceAgents(t, client, r.Workspace.ID)
		return client, uint16(coderdPort), resources[0].Agents[0].ID
	}

	willFilterPort := func(port int) bool {
		if port < codersdk.WorkspaceAgentMinimumListeningPort || port > 65535 {
			return true
		}
		if _, ok := codersdk.WorkspaceAgentIgnoredListeningPorts[uint16(port)]; ok {
			return true
		}

		return false
	}

	generateUnfilteredPort := func(t *testing.T) (net.Listener, uint16) {
		var (
			l    net.Listener
			port uint16
		)
		require.Eventually(t, func() bool {
			var err error
			l, err = net.Listen("tcp", "localhost:0")
			if err != nil {
				return false
			}
			tcpAddr, _ := l.Addr().(*net.TCPAddr)
			if willFilterPort(tcpAddr.Port) {
				_ = l.Close()
				return false
			}
			t.Cleanup(func() {
				_ = l.Close()
			})

			port = uint16(tcpAddr.Port)
			return true
		}, testutil.WaitShort, testutil.IntervalFast)

		return l, port
	}

	generateFilteredPort := func(t *testing.T) (net.Listener, uint16) {
		var (
			l    net.Listener
			port uint16
		)
		require.Eventually(t, func() bool {
			for ignoredPort := range codersdk.WorkspaceAgentIgnoredListeningPorts {
				if ignoredPort < 1024 || ignoredPort == 5432 {
					continue
				}

				var err error
				l, err = net.Listen("tcp", fmt.Sprintf("localhost:%d", ignoredPort))
				if err != nil {
					continue
				}
				t.Cleanup(func() {
					_ = l.Close()
				})

				port = ignoredPort
				return true
			}

			return false
		}, testutil.WaitShort, testutil.IntervalFast)

		return l, port
	}

	t.Run("LinuxAndWindows", func(t *testing.T) {
		t.Parallel()
		if runtime.GOOS != "linux" && runtime.GOOS != "windows" {
			t.Skip("only runs on linux and windows")
			return
		}

		for _, tc := range []struct {
			name  string
			setDV func(t *testing.T, dv *codersdk.DeploymentValues)
		}{
			{
				name:  "Mainline",
				setDV: func(*testing.T, *codersdk.DeploymentValues) {},
			},
			{
				name: "BlockDirect",
				setDV: func(t *testing.T, dv *codersdk.DeploymentValues) {
					err := dv.DERP.Config.BlockDirect.Set("true")
					require.NoError(t, err)
					require.True(t, dv.DERP.Config.BlockDirect.Value())
				},
			},
		} {
			tc := tc
			t.Run("OK_"+tc.name, func(t *testing.T) {
				t.Parallel()

				dv := coderdtest.DeploymentValues(t)
				tc.setDV(t, dv)
				client, coderdPort, agentID := setup(t, nil, dv)

				ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
				defer cancel()

				// Generate a random unfiltered port.
				l, lPort := generateUnfilteredPort(t)

				// List ports and ensure that the port we expect to see is there.
				res, err := client.WorkspaceAgentListeningPorts(ctx, agentID)
				require.NoError(t, err)

				expected := map[uint16]bool{
					// expect the listener we made
					lPort: false,
					// expect the coderdtest server
					coderdPort: false,
				}
				for _, port := range res.Ports {
					if port.Network == "tcp" {
						if val, ok := expected[port.Port]; ok {
							if val {
								t.Fatalf("expected to find TCP port %d only once in response", port.Port)
							}
						}
						expected[port.Port] = true
					}
				}
				for port, found := range expected {
					if !found {
						t.Fatalf("expected to find TCP port %d in response", port)
					}
				}

				// Close the listener and check that the port is no longer in the response.
				require.NoError(t, l.Close())
				t.Log("checking for ports after listener close:")
				require.Eventually(t, func() bool {
					res, err = client.WorkspaceAgentListeningPorts(ctx, agentID)
					if !assert.NoError(t, err) {
						return false
					}

					for _, port := range res.Ports {
						if port.Network == "tcp" && port.Port == lPort {
							t.Logf("expected to not find TCP port %d in response", lPort)
							return false
						}
					}
					return true
				}, testutil.WaitLong, testutil.IntervalMedium)
			})
		}

		t.Run("Filter", func(t *testing.T) {
			t.Parallel()

			// Generate an unfiltered port that we will create an app for and
			// should not exist in the response.
			_, appLPort := generateUnfilteredPort(t)
			app := &proto.App{
				Slug: "test-app",
				Url:  fmt.Sprintf("http://localhost:%d", appLPort),
			}

			// Generate a filtered port that should not exist in the response.
			_, filteredLPort := generateFilteredPort(t)

			client, coderdPort, agentID := setup(t, []*proto.App{app}, nil)

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			res, err := client.WorkspaceAgentListeningPorts(ctx, agentID)
			require.NoError(t, err)

			sawCoderdPort := false
			for _, port := range res.Ports {
				if port.Network == "tcp" {
					if port.Port == appLPort {
						t.Fatalf("expected to not find TCP port (app port) %d in response", appLPort)
					}
					if port.Port == filteredLPort {
						t.Fatalf("expected to not find TCP port (filtered port) %d in response", filteredLPort)
					}
					if port.Port == coderdPort {
						sawCoderdPort = true
					}
				}
			}
			if !sawCoderdPort {
				t.Fatalf("expected to find TCP port (coderd port) %d in response", coderdPort)
			}
		})
	})

	t.Run("Darwin", func(t *testing.T) {
		t.Parallel()
		if runtime.GOOS != "darwin" {
			t.Skip("only runs on darwin")
			return
		}

		client, _, agentID := setup(t, nil, nil)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		// Create a TCP listener on a random port.
		l, err := net.Listen("tcp", "localhost:0")
		require.NoError(t, err)
		defer l.Close()

		// List ports and ensure that the list is empty because we're on darwin.
		res, err := client.WorkspaceAgentListeningPorts(ctx, agentID)
		require.NoError(t, err)
		require.Len(t, res.Ports, 0)
	})
}

func TestWorkspaceAgentAppHealth(t *testing.T) {
	t.Parallel()
	client, db := coderdtest.NewWithDatabase(t, nil)
	user := coderdtest.CreateFirstUser(t, client)
	apps := []*proto.App{
		{
			Slug:    "code-server",
			Command: "some-command",
			Url:     "http://localhost:3000",
			Icon:    "/code.svg",
		},
		{
			Slug:        "code-server-2",
			DisplayName: "code-server-2",
			Command:     "some-command",
			Url:         "http://localhost:3000",
			Icon:        "/code.svg",
			Healthcheck: &proto.Healthcheck{
				Url:       "http://localhost:3000",
				Interval:  5,
				Threshold: 6,
			},
		},
	}
	r := dbfake.WorkspaceBuild(t, db, database.Workspace{
		OrganizationID: user.OrganizationID,
		OwnerID:        user.UserID,
	}).WithAgent(func(agents []*proto.Agent) []*proto.Agent {
		agents[0].Apps = apps
		return agents
	}).Do()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	agentClient := agentsdk.New(client.URL)
	agentClient.SetSessionToken(r.AgentToken)

	manifest, err := agentClient.Manifest(ctx)
	require.NoError(t, err)
	require.EqualValues(t, codersdk.WorkspaceAppHealthDisabled, manifest.Apps[0].Health)
	require.EqualValues(t, codersdk.WorkspaceAppHealthInitializing, manifest.Apps[1].Health)
	err = agentClient.PostAppHealth(ctx, agentsdk.PostAppHealthsRequest{})
	require.Error(t, err)
	// empty
	err = agentClient.PostAppHealth(ctx, agentsdk.PostAppHealthsRequest{})
	require.Error(t, err)
	// healthcheck disabled
	err = agentClient.PostAppHealth(ctx, agentsdk.PostAppHealthsRequest{
		Healths: map[uuid.UUID]codersdk.WorkspaceAppHealth{
			manifest.Apps[0].ID: codersdk.WorkspaceAppHealthInitializing,
		},
	})
	require.Error(t, err)
	// invalid value
	err = agentClient.PostAppHealth(ctx, agentsdk.PostAppHealthsRequest{
		Healths: map[uuid.UUID]codersdk.WorkspaceAppHealth{
			manifest.Apps[1].ID: codersdk.WorkspaceAppHealth("bad-value"),
		},
	})
	require.Error(t, err)
	// update to healthy
	err = agentClient.PostAppHealth(ctx, agentsdk.PostAppHealthsRequest{
		Healths: map[uuid.UUID]codersdk.WorkspaceAppHealth{
			manifest.Apps[1].ID: codersdk.WorkspaceAppHealthHealthy,
		},
	})
	require.NoError(t, err)
	manifest, err = agentClient.Manifest(ctx)
	require.NoError(t, err)
	require.EqualValues(t, codersdk.WorkspaceAppHealthHealthy, manifest.Apps[1].Health)
	// update to unhealthy
	err = agentClient.PostAppHealth(ctx, agentsdk.PostAppHealthsRequest{
		Healths: map[uuid.UUID]codersdk.WorkspaceAppHealth{
			manifest.Apps[1].ID: codersdk.WorkspaceAppHealthUnhealthy,
		},
	})
	require.NoError(t, err)
	manifest, err = agentClient.Manifest(ctx)
	require.NoError(t, err)
	require.EqualValues(t, codersdk.WorkspaceAppHealthUnhealthy, manifest.Apps[1].Health)
}

func TestWorkspaceAgentReportStats(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		client, db := coderdtest.NewWithDatabase(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		r := dbfake.WorkspaceBuild(t, db, database.Workspace{
			OrganizationID: user.OrganizationID,
			OwnerID:        user.UserID,
		}).WithAgent().Do()

		agentClient := agentsdk.New(client.URL)
		agentClient.SetSessionToken(r.AgentToken)

		_, err := agentClient.PostStats(context.Background(), &agentsdk.Stats{
			ConnectionsByProto: map[string]int64{"TCP": 1},
			// Set connection count to 1 but all session counts to zero to
			// assert we aren't updating last_used_at for a connections that may
			// be spawned passively by the dashboard.
			ConnectionCount:             1,
			RxPackets:                   1,
			RxBytes:                     1,
			TxPackets:                   1,
			TxBytes:                     1,
			SessionCountVSCode:          0,
			SessionCountJetBrains:       0,
			SessionCountReconnectingPTY: 0,
			SessionCountSSH:             0,
			ConnectionMedianLatencyMS:   10,
		})
		require.NoError(t, err)

		newWorkspace, err := client.Workspace(context.Background(), r.Workspace.ID)
		require.NoError(t, err)

		assert.True(t,
			newWorkspace.LastUsedAt.Equal(r.Workspace.LastUsedAt),
			"%s and %s should not differ", newWorkspace.LastUsedAt, r.Workspace.LastUsedAt,
		)

		_, err = agentClient.PostStats(context.Background(), &agentsdk.Stats{
			ConnectionsByProto:          map[string]int64{"TCP": 1},
			ConnectionCount:             1,
			RxPackets:                   1,
			RxBytes:                     1,
			TxPackets:                   1,
			TxBytes:                     1,
			SessionCountVSCode:          1,
			SessionCountJetBrains:       0,
			SessionCountReconnectingPTY: 0,
			SessionCountSSH:             0,
			ConnectionMedianLatencyMS:   10,
		})
		require.NoError(t, err)

		newWorkspace, err = client.Workspace(context.Background(), r.Workspace.ID)
		require.NoError(t, err)

		assert.True(t,
			newWorkspace.LastUsedAt.After(r.Workspace.LastUsedAt),
			"%s is not after %s", newWorkspace.LastUsedAt, r.Workspace.LastUsedAt,
		)
	})

	t.Run("FailDeleted", func(t *testing.T) {
		t.Parallel()

		owner, db := coderdtest.NewWithDatabase(t, nil)
		ownerUser := coderdtest.CreateFirstUser(t, owner)
		client, admin := coderdtest.CreateAnotherUser(t, owner, ownerUser.OrganizationID, rbac.RoleTemplateAdmin(), rbac.RoleUserAdmin())
		r := dbfake.WorkspaceBuild(t, db, database.Workspace{
			OrganizationID: admin.OrganizationIDs[0],
			OwnerID:        admin.ID,
		}).WithAgent().Do()

		agentClient := agentsdk.New(client.URL)
		agentClient.SetSessionToken(r.AgentToken)

		_, err := agentClient.PostStats(context.Background(), &agentsdk.Stats{
			ConnectionsByProto:          map[string]int64{"TCP": 1},
			ConnectionCount:             1,
			RxPackets:                   1,
			RxBytes:                     1,
			TxPackets:                   1,
			TxBytes:                     1,
			SessionCountVSCode:          0,
			SessionCountJetBrains:       0,
			SessionCountReconnectingPTY: 0,
			SessionCountSSH:             0,
			ConnectionMedianLatencyMS:   10,
		})
		require.NoError(t, err)

		newWorkspace, err := client.Workspace(context.Background(), r.Workspace.ID)
		require.NoError(t, err)

		// nolint:gocritic // using db directly over creating a delete job
		err = db.UpdateWorkspaceDeletedByID(dbauthz.As(context.Background(),
			coderdtest.AuthzUserSubject(admin, ownerUser.OrganizationID)),
			database.UpdateWorkspaceDeletedByIDParams{
				ID:      newWorkspace.ID,
				Deleted: true,
			})
		require.NoError(t, err)

		_, err = agentClient.PostStats(context.Background(), &agentsdk.Stats{
			ConnectionsByProto:          map[string]int64{"TCP": 1},
			ConnectionCount:             1,
			RxPackets:                   1,
			RxBytes:                     1,
			TxPackets:                   1,
			TxBytes:                     1,
			SessionCountVSCode:          1,
			SessionCountJetBrains:       0,
			SessionCountReconnectingPTY: 0,
			SessionCountSSH:             0,
			ConnectionMedianLatencyMS:   10,
		})
		require.ErrorContains(t, err, "agent is invalid")
	})
}

func TestWorkspaceAgent_LifecycleState(t *testing.T) {
	t.Parallel()

	t.Run("Set", func(t *testing.T) {
		t.Parallel()

		client, db := coderdtest.NewWithDatabase(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		r := dbfake.WorkspaceBuild(t, db, database.Workspace{
			OrganizationID: user.OrganizationID,
			OwnerID:        user.UserID,
		}).WithAgent().Do()
		workspace, err := client.Workspace(context.Background(), r.Workspace.ID)
		require.NoError(t, err)
		for _, res := range workspace.LatestBuild.Resources {
			for _, a := range res.Agents {
				require.Equal(t, codersdk.WorkspaceAgentLifecycleCreated, a.LifecycleState)
			}
		}

		agentClient := agentsdk.New(client.URL)
		agentClient.SetSessionToken(r.AgentToken)

		tests := []struct {
			state   codersdk.WorkspaceAgentLifecycle
			wantErr bool
		}{
			{codersdk.WorkspaceAgentLifecycleCreated, false},
			{codersdk.WorkspaceAgentLifecycleStarting, false},
			{codersdk.WorkspaceAgentLifecycleStartTimeout, false},
			{codersdk.WorkspaceAgentLifecycleStartError, false},
			{codersdk.WorkspaceAgentLifecycleReady, false},
			{codersdk.WorkspaceAgentLifecycleShuttingDown, false},
			{codersdk.WorkspaceAgentLifecycleShutdownTimeout, false},
			{codersdk.WorkspaceAgentLifecycleShutdownError, false},
			{codersdk.WorkspaceAgentLifecycleOff, false},
			{codersdk.WorkspaceAgentLifecycle("nonexistent_state"), true},
			{codersdk.WorkspaceAgentLifecycle(""), true},
		}
		//nolint:paralleltest // No race between setting the state and getting the workspace.
		for _, tt := range tests {
			tt := tt
			t.Run(string(tt.state), func(t *testing.T) {
				ctx := testutil.Context(t, testutil.WaitLong)

				err := agentClient.PostLifecycle(ctx, agentsdk.PostLifecycleRequest{
					State:     tt.state,
					ChangedAt: time.Now(),
				})
				if tt.wantErr {
					require.Error(t, err)
					return
				}
				require.NoError(t, err, "post lifecycle state %q", tt.state)

				workspace, err = client.Workspace(ctx, workspace.ID)
				require.NoError(t, err, "get workspace")

				for _, res := range workspace.LatestBuild.Resources {
					for _, agent := range res.Agents {
						require.Equal(t, tt.state, agent.LifecycleState)
					}
				}
			})
		}
	})
}

func TestWorkspaceAgent_Metadata(t *testing.T) {
	t.Parallel()

	client, db := coderdtest.NewWithDatabase(t, nil)
	user := coderdtest.CreateFirstUser(t, client)
	r := dbfake.WorkspaceBuild(t, db, database.Workspace{
		OrganizationID: user.OrganizationID,
		OwnerID:        user.UserID,
	}).WithAgent(func(agents []*proto.Agent) []*proto.Agent {
		agents[0].Metadata = []*proto.Agent_Metadata{
			{
				DisplayName: "First Meta",
				Key:         "foo1",
				Script:      "echo hi",
				Interval:    10,
				Timeout:     3,
			},
			{
				DisplayName: "Second Meta",
				Key:         "foo2",
				Script:      "echo howdy",
				Interval:    10,
				Timeout:     3,
			},
			{
				DisplayName: "TooLong",
				Key:         "foo3",
				Script:      "echo howdy",
				Interval:    10,
				Timeout:     3,
			},
		}
		return agents
	}).Do()

	workspace, err := client.Workspace(context.Background(), r.Workspace.ID)
	require.NoError(t, err)
	for _, res := range workspace.LatestBuild.Resources {
		for _, a := range res.Agents {
			require.Equal(t, codersdk.WorkspaceAgentLifecycleCreated, a.LifecycleState)
		}
	}

	agentClient := agentsdk.New(client.URL)
	agentClient.SetSessionToken(r.AgentToken)

	ctx := testutil.Context(t, testutil.WaitMedium)

	manifest, err := agentClient.Manifest(ctx)
	require.NoError(t, err)

	// Verify manifest API response.
	require.Equal(t, workspace.ID, manifest.WorkspaceID)
	require.Equal(t, workspace.OwnerName, manifest.OwnerName)
	require.Equal(t, "First Meta", manifest.Metadata[0].DisplayName)
	require.Equal(t, "foo1", manifest.Metadata[0].Key)
	require.Equal(t, "echo hi", manifest.Metadata[0].Script)
	require.EqualValues(t, 10, manifest.Metadata[0].Interval)
	require.EqualValues(t, 3, manifest.Metadata[0].Timeout)

	post := func(ctx context.Context, key string, mr codersdk.WorkspaceAgentMetadataResult) {
		err := agentClient.PostMetadata(ctx, agentsdk.PostMetadataRequest{
			Metadata: []agentsdk.Metadata{
				{
					Key:                          key,
					WorkspaceAgentMetadataResult: mr,
				},
			},
		})
		require.NoError(t, err, "post metadata: %s, %#v", key, mr)
	}

	workspace, err = client.Workspace(ctx, workspace.ID)
	require.NoError(t, err, "get workspace")

	agentID := workspace.LatestBuild.Resources[0].Agents[0].ID

	var update []codersdk.WorkspaceAgentMetadata

	wantMetadata1 := codersdk.WorkspaceAgentMetadataResult{
		CollectedAt: time.Now(),
		Value:       "bar",
	}

	// Setup is complete, reset the context.
	ctx = testutil.Context(t, testutil.WaitMedium)

	// Initial post must come before the Watch is established.
	post(ctx, "foo1", wantMetadata1)

	updates, errors := client.WatchWorkspaceAgentMetadata(ctx, agentID)

	recvUpdate := func() []codersdk.WorkspaceAgentMetadata {
		select {
		case <-ctx.Done():
			t.Fatalf("context done: %v", ctx.Err())
		case err := <-errors:
			t.Fatalf("error watching metadata: %v", err)
		case update := <-updates:
			return update
		}
		return nil
	}

	check := func(want codersdk.WorkspaceAgentMetadataResult, got codersdk.WorkspaceAgentMetadata, retry bool) {
		// We can't trust the order of the updates due to timers and debounces,
		// so let's check a few times more.
		for i := 0; retry && i < 2 && (want.Value != got.Result.Value || want.Error != got.Result.Error); i++ {
			update = recvUpdate()
			for _, m := range update {
				if m.Description.Key == got.Description.Key {
					got = m
					break
				}
			}
		}
		ok1 := assert.Equal(t, want.Value, got.Result.Value)
		ok2 := assert.Equal(t, want.Error, got.Result.Error)
		if !ok1 || !ok2 {
			require.FailNow(t, "check failed")
		}
	}

	update = recvUpdate()
	require.Len(t, update, 3)
	check(wantMetadata1, update[0], false)
	// The second metadata result is not yet posted.
	require.Zero(t, update[1].Result.CollectedAt)

	wantMetadata2 := wantMetadata1
	post(ctx, "foo2", wantMetadata2)
	update = recvUpdate()
	require.Len(t, update, 3)
	check(wantMetadata1, update[0], true)
	check(wantMetadata2, update[1], true)

	wantMetadata1.Error = "error"
	post(ctx, "foo1", wantMetadata1)
	update = recvUpdate()
	require.Len(t, update, 3)
	check(wantMetadata1, update[0], true)

	const maxValueLen = 2048
	tooLongValueMetadata := wantMetadata1
	tooLongValueMetadata.Value = strings.Repeat("a", maxValueLen*2)
	tooLongValueMetadata.Error = ""
	tooLongValueMetadata.CollectedAt = time.Now()
	post(ctx, "foo3", tooLongValueMetadata)
	got := recvUpdate()[2]
	for i := 0; i < 2 && len(got.Result.Value) != maxValueLen; i++ {
		got = recvUpdate()[2]
	}
	require.Len(t, got.Result.Value, maxValueLen)
	require.NotEmpty(t, got.Result.Error)

	unknownKeyMetadata := wantMetadata1
	post(ctx, "unknown", unknownKeyMetadata)
}

type testWAMErrorStore struct {
	database.Store
	err atomic.Pointer[error]
}

func (s *testWAMErrorStore) GetWorkspaceAgentMetadata(ctx context.Context, arg database.GetWorkspaceAgentMetadataParams) ([]database.WorkspaceAgentMetadatum, error) {
	err := s.err.Load()
	if err != nil {
		return nil, *err
	}
	return s.Store.GetWorkspaceAgentMetadata(ctx, arg)
}

func TestWorkspaceAgent_Metadata_CatchMemoryLeak(t *testing.T) {
	t.Parallel()

	db := &testWAMErrorStore{Store: dbmem.New()}
	psub := pubsub.NewInMemory()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Named("coderd").Leveled(slog.LevelDebug)
	client := coderdtest.New(t, &coderdtest.Options{
		Database:                 db,
		Pubsub:                   psub,
		IncludeProvisionerDaemon: true,
		Logger:                   &logger,
	})
	user := coderdtest.CreateFirstUser(t, client)
	r := dbfake.WorkspaceBuild(t, db, database.Workspace{
		OrganizationID: user.OrganizationID,
		OwnerID:        user.UserID,
	}).WithAgent(func(agents []*proto.Agent) []*proto.Agent {
		agents[0].Metadata = []*proto.Agent_Metadata{
			{
				DisplayName: "First Meta",
				Key:         "foo1",
				Script:      "echo hi",
				Interval:    10,
				Timeout:     3,
			},
			{
				DisplayName: "Second Meta",
				Key:         "foo2",
				Script:      "echo bye",
				Interval:    10,
				Timeout:     3,
			},
		}
		return agents
	}).Do()
	workspace, err := client.Workspace(context.Background(), r.Workspace.ID)
	require.NoError(t, err)
	for _, res := range workspace.LatestBuild.Resources {
		for _, a := range res.Agents {
			require.Equal(t, codersdk.WorkspaceAgentLifecycleCreated, a.LifecycleState)
		}
	}

	agentClient := agentsdk.New(client.URL)
	agentClient.SetSessionToken(r.AgentToken)

	ctx, cancel := context.WithCancel(testutil.Context(t, testutil.WaitSuperLong))

	manifest, err := agentClient.Manifest(ctx)
	require.NoError(t, err)

	post := func(ctx context.Context, key, value string) error {
		return agentClient.PostMetadata(ctx, agentsdk.PostMetadataRequest{
			Metadata: []agentsdk.Metadata{
				{
					Key: key,
					WorkspaceAgentMetadataResult: codersdk.WorkspaceAgentMetadataResult{
						CollectedAt: time.Now(),
						Value:       value,
					},
				},
			},
		})
	}

	workspace, err = client.Workspace(ctx, workspace.ID)
	require.NoError(t, err, "get workspace")

	// Start the SSE connection.
	metadata, errors := client.WatchWorkspaceAgentMetadata(ctx, manifest.AgentID)

	// Discard the output, pretending to be a client consuming it.
	wantErr := xerrors.New("test error")
	metadataDone := testutil.Go(t, func() {
		for {
			select {
			case <-ctx.Done():
				return
			case _, ok := <-metadata:
				if !ok {
					return
				}
			case err := <-errors:
				if err != nil && !strings.Contains(err.Error(), wantErr.Error()) {
					assert.NoError(t, err, "watch metadata")
				}
				return
			}
		}
	})

	postDone := testutil.Go(t, func() {
		for {
			// We need to send two separate metadata updates to trigger the
			// memory leak. foo2 will cause the number of foo1 to be doubled, etc.
			err = post(ctx, "foo1", "hi")
			if err != nil {
				if !xerrors.Is(err, context.Canceled) {
					assert.NoError(t, err, "post metadata foo1")
				}
				return
			}
			err = post(ctx, "foo2", "bye")
			if err != nil {
				if !xerrors.Is(err, context.Canceled) {
					assert.NoError(t, err, "post metadata foo1")
				}
				return
			}
		}
	})

	// In a previously faulty implementation, this database error will trigger
	// a close of the goroutine that consumes metadata updates for refreshing
	// the metadata sent over SSE. As it was, the exit of the consumer was not
	// detected as a trigger to close down the connection.
	//
	// Further, there was a memory leak in the pubsub subscription that cause
	// ballooning of memory (almost double in size every received metadata).
	//
	// This db error should trigger a close of the SSE connection in the fixed
	// implementation. The memory leak should not happen in either case, but
	// testing it is not straightforward.
	db.err.Store(&wantErr)

	select {
	case <-ctx.Done():
		t.Fatal("timeout waiting for SSE to close")
	case <-metadataDone:
	}
	cancel()
	<-postDone
}

func TestWorkspaceAgent_Startup(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		client, db := coderdtest.NewWithDatabase(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		r := dbfake.WorkspaceBuild(t, db, database.Workspace{
			OrganizationID: user.OrganizationID,
			OwnerID:        user.UserID,
		}).WithAgent().Do()
		agentClient := agentsdk.New(client.URL)
		agentClient.SetSessionToken(r.AgentToken)

		ctx := testutil.Context(t, testutil.WaitMedium)

		var (
			expectedVersion    = "v1.2.3"
			expectedDir        = "/home/coder"
			expectedSubsystems = []codersdk.AgentSubsystem{
				codersdk.AgentSubsystemEnvbox,
				codersdk.AgentSubsystemExectrace,
			}
		)

		err := agentClient.PostStartup(ctx, agentsdk.PostStartupRequest{
			Version:           expectedVersion,
			ExpandedDirectory: expectedDir,
			Subsystems: []codersdk.AgentSubsystem{
				// Not sorted.
				expectedSubsystems[1],
				expectedSubsystems[0],
			},
		})
		require.NoError(t, err)

		workspace, err := client.Workspace(ctx, r.Workspace.ID)
		require.NoError(t, err)

		wsagent, err := client.WorkspaceAgent(ctx, workspace.LatestBuild.Resources[0].Agents[0].ID)
		require.NoError(t, err)
		require.Equal(t, expectedVersion, wsagent.Version)
		require.Equal(t, expectedDir, wsagent.ExpandedDirectory)
		// Sorted
		require.Equal(t, expectedSubsystems, wsagent.Subsystems)
		require.Equal(t, coderd.AgentAPIVersionREST, wsagent.APIVersion)
	})

	t.Run("InvalidSemver", func(t *testing.T) {
		t.Parallel()

		client, db := coderdtest.NewWithDatabase(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		r := dbfake.WorkspaceBuild(t, db, database.Workspace{
			OrganizationID: user.OrganizationID,
			OwnerID:        user.UserID,
		}).WithAgent().Do()

		agentClient := agentsdk.New(client.URL)
		agentClient.SetSessionToken(r.AgentToken)

		ctx := testutil.Context(t, testutil.WaitMedium)

		err := agentClient.PostStartup(ctx, agentsdk.PostStartupRequest{
			Version: "1.2.3",
		})
		require.Error(t, err)
		cerr, ok := codersdk.AsError(err)
		require.True(t, ok)
		require.Equal(t, http.StatusBadRequest, cerr.StatusCode())
	})
}

// TestWorkspaceAgent_UpdatedDERP runs a real coderd server, with a real agent
// and a real client, and updates the DERP map live to ensure connections still
// work.
func TestWorkspaceAgent_UpdatedDERP(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)

	dv := coderdtest.DeploymentValues(t)
	err := dv.DERP.Config.BlockDirect.Set("true")
	require.NoError(t, err)

	client, closer, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
		DeploymentValues: dv,
	})
	defer closer.Close()
	user := coderdtest.CreateFirstUser(t, client)

	// Change the DERP mapper to our custom one.
	var currentDerpMap atomic.Pointer[tailcfg.DERPMap]
	originalDerpMap, _ := tailnettest.RunDERPAndSTUN(t)
	currentDerpMap.Store(originalDerpMap)
	derpMapFn := func(_ *tailcfg.DERPMap) *tailcfg.DERPMap {
		return currentDerpMap.Load().Clone()
	}
	api.DERPMapper.Store(&derpMapFn)

	// Start workspace a workspace agent.
	r := dbfake.WorkspaceBuild(t, api.Database, database.Workspace{
		OrganizationID: user.OrganizationID,
		OwnerID:        user.UserID,
	}).WithAgent().Do()

	agentCloser := agenttest.New(t, client.URL, r.AgentToken)
	resources := coderdtest.AwaitWorkspaceAgents(t, client, r.Workspace.ID)
	agentID := resources[0].Agents[0].ID

	// Connect from a client.
	conn1, err := func() (*codersdk.WorkspaceAgentConn, error) {
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel() // Connection should remain open even if the dial context is canceled.

		return client.DialWorkspaceAgent(ctx, agentID, &codersdk.DialWorkspaceAgentOptions{
			Logger: logger.Named("client1"),
		})
	}()
	require.NoError(t, err)
	defer conn1.Close()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	ok := conn1.AwaitReachable(ctx)
	require.True(t, ok)

	// Change the DERP map and change the region ID.
	newDerpMap, _ := tailnettest.RunDERPAndSTUN(t)
	require.NotNil(t, newDerpMap)
	newDerpMap.Regions[2] = newDerpMap.Regions[1]
	delete(newDerpMap.Regions, 1)
	newDerpMap.Regions[2].RegionID = 2
	for _, node := range newDerpMap.Regions[2].Nodes {
		node.RegionID = 2
	}
	currentDerpMap.Store(newDerpMap)

	// Wait for the agent's DERP map to be updated.
	require.Eventually(t, func() bool {
		conn := agentCloser.TailnetConn()
		if conn == nil {
			return false
		}
		regionIDs := conn.DERPMap().RegionIDs()
		return len(regionIDs) == 1 && regionIDs[0] == 2 && conn.Node().PreferredDERP == 2
	}, testutil.WaitLong, testutil.IntervalFast)

	// Wait for the DERP map to be updated on the existing client.
	require.Eventually(t, func() bool {
		regionIDs := conn1.Conn.DERPMap().RegionIDs()
		return len(regionIDs) == 1 && regionIDs[0] == 2
	}, testutil.WaitLong, testutil.IntervalFast)

	// The first client should still be able to reach the agent.
	ok = conn1.AwaitReachable(ctx)
	require.True(t, ok)

	// Connect from a second client.
	conn2, err := client.DialWorkspaceAgent(ctx, agentID, &codersdk.DialWorkspaceAgentOptions{
		Logger: logger.Named("client2"),
	})
	require.NoError(t, err)
	defer conn2.Close()
	ok = conn2.AwaitReachable(ctx)
	require.True(t, ok)
	require.Equal(t, []int{2}, conn2.DERPMap().RegionIDs())
}

func TestWorkspaceAgentExternalAuthListen(t *testing.T) {
	t.Parallel()

	// ValidateURLSpam acts as a workspace calling GIT_ASK_PASS which
	// will wait until the external auth token is valid. The issue is we spam
	// the validate endpoint with requests until the token is valid. We do this
	// even if the token has not changed. We are calling validate with the
	// same inputs expecting a different result (insanity?). To reduce our
	// api rate limit usage, we should do nothing if the inputs have not
	// changed.
	//
	// Note that an expired oauth token is already skipped, so this really
	// only covers the case of a revoked token.
	t.Run("ValidateURLSpam", func(t *testing.T) {
		t.Parallel()

		const providerID = "fake-idp"

		// Count all the times we call validate
		validateCalls := 0
		fake := oidctest.NewFakeIDP(t, oidctest.WithServing(), oidctest.WithMiddlewares(func(handler http.Handler) http.Handler {
			return http.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Count all the validate calls
				if strings.Contains(r.URL.Path, "/external-auth-validate/") {
					validateCalls++
				}
				handler.ServeHTTP(w, r)
			}))
		}))

		ticks := make(chan time.Time)
		// setup
		ownerClient, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{
			NewTicker: func(duration time.Duration) (<-chan time.Time, func()) {
				return ticks, func() {}
			},
			ExternalAuthConfigs: []*externalauth.Config{
				fake.ExternalAuthConfig(t, providerID, nil, func(cfg *externalauth.Config) {
					cfg.Type = codersdk.EnhancedExternalAuthProviderGitHub.String()
				}),
			},
		})
		first := coderdtest.CreateFirstUser(t, ownerClient)
		tmpDir := t.TempDir()
		client, user := coderdtest.CreateAnotherUser(t, ownerClient, first.OrganizationID)

		r := dbfake.WorkspaceBuild(t, db, database.Workspace{
			OrganizationID: first.OrganizationID,
			OwnerID:        user.ID,
		}).WithAgent(func(agents []*proto.Agent) []*proto.Agent {
			agents[0].Directory = tmpDir
			return agents
		}).Do()

		agentClient := agentsdk.New(client.URL)
		agentClient.SetSessionToken(r.AgentToken)

		// We need to include an invalid oauth token that is not expired.
		dbgen.ExternalAuthLink(t, db, database.ExternalAuthLink{
			ProviderID:        providerID,
			UserID:            user.ID,
			CreatedAt:         dbtime.Now(),
			UpdatedAt:         dbtime.Now(),
			OAuthAccessToken:  "invalid",
			OAuthRefreshToken: "bad",
			OAuthExpiry:       dbtime.Now().Add(time.Hour),
		})

		ctx, cancel := context.WithCancel(testutil.Context(t, testutil.WaitShort))
		go func() {
			// The request that will block and fire off validate calls.
			_, err := agentClient.ExternalAuth(ctx, agentsdk.ExternalAuthRequest{
				ID:     providerID,
				Match:  "",
				Listen: true,
			})
			assert.Error(t, err, "this should fail")
		}()

		// Send off 10 ticks to cause 10 validate calls
		for i := 0; i < 10; i++ {
			ticks <- time.Now()
		}
		cancel()
		// We expect only 1
		// In a failed test, you will likely see 9, as the last one
		// gets canceled.
		require.Equal(t, 1, validateCalls, "validate calls duplicated on same token")
	})
}

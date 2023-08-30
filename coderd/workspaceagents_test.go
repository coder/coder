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
	"tailscale.com/tailcfg"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
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
		client := coderdtest.New(t, &coderdtest.Options{
			IncludeProvisionerDaemon: true,
		})
		user := coderdtest.CreateFirstUser(t, client)
		authToken := uuid.NewString()
		tmpDir := t.TempDir()
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:         echo.ParseComplete,
			ProvisionPlan: echo.PlanComplete,
			ProvisionApply: []*proto.Response{{
				Type: &proto.Response_Apply{
					Apply: &proto.ApplyComplete{
						Resources: []*proto.Resource{{
							Name: "example",
							Type: "aws_instance",
							Agents: []*proto.Agent{{
								Id:        uuid.NewString(),
								Directory: tmpDir,
								Auth: &proto.Agent_Token{
									Token: authToken,
								},
							}},
						}},
					},
				},
			}},
		})
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		workspace, err := client.Workspace(ctx, workspace.ID)
		require.NoError(t, err)
		require.Equal(t, tmpDir, workspace.LatestBuild.Resources[0].Agents[0].Directory)
		_, err = client.WorkspaceAgent(ctx, workspace.LatestBuild.Resources[0].Agents[0].ID)
		require.NoError(t, err)
		require.True(t, workspace.LatestBuild.Resources[0].Agents[0].Health.Healthy)
	})
	t.Run("HasFallbackTroubleshootingURL", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{
			IncludeProvisionerDaemon: true,
		})
		user := coderdtest.CreateFirstUser(t, client)
		authToken := uuid.NewString()
		tmpDir := t.TempDir()
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:         echo.ParseComplete,
			ProvisionPlan: echo.PlanComplete,
			ProvisionApply: []*proto.Response{{
				Type: &proto.Response_Apply{
					Apply: &proto.ApplyComplete{
						Resources: []*proto.Resource{{
							Name: "example",
							Type: "aws_instance",
							Agents: []*proto.Agent{{
								Id:        uuid.NewString(),
								Directory: tmpDir,
								Auth: &proto.Agent_Token{
									Token: authToken,
								},
							}},
						}},
					},
				},
			}},
		})
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
		defer cancel()

		workspace, err := client.Workspace(ctx, workspace.ID)
		require.NoError(t, err)
		require.NotEmpty(t, workspace.LatestBuild.Resources[0].Agents[0].TroubleshootingURL)
		t.Log(workspace.LatestBuild.Resources[0].Agents[0].TroubleshootingURL)
	})
	t.Run("Timeout", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{
			IncludeProvisionerDaemon: true,
		})
		user := coderdtest.CreateFirstUser(t, client)
		authToken := uuid.NewString()
		tmpDir := t.TempDir()

		wantTroubleshootingURL := "https://example.com/troubleshoot"

		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:         echo.ParseComplete,
			ProvisionPlan: echo.PlanComplete,
			ProvisionApply: []*proto.Response{{
				Type: &proto.Response_Apply{
					Apply: &proto.ApplyComplete{
						Resources: []*proto.Resource{{
							Name: "example",
							Type: "aws_instance",
							Agents: []*proto.Agent{{
								Id:        uuid.NewString(),
								Directory: tmpDir,
								Auth: &proto.Agent_Token{
									Token: authToken,
								},
								ConnectionTimeoutSeconds: 1,
								TroubleshootingUrl:       wantTroubleshootingURL,
							}},
						}},
					},
				},
			}},
		})
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
		defer cancel()

		var err error
		testutil.Eventually(ctx, t, func(ctx context.Context) (done bool) {
			workspace, err = client.Workspace(ctx, workspace.ID)
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
		client := coderdtest.New(t, &coderdtest.Options{
			IncludeProvisionerDaemon: true,
		})
		user := coderdtest.CreateFirstUser(t, client)
		authToken := uuid.NewString()
		tmpDir := t.TempDir()
		apps := &proto.DisplayApps{
			Vscode:               true,
			VscodeInsiders:       true,
			WebTerminal:          true,
			PortForwardingHelper: true,
			SshHelper:            true,
		}

		echoResp := &echo.Responses{
			Parse:         echo.ParseComplete,
			ProvisionPlan: echo.PlanComplete,
			ProvisionApply: []*proto.Response{{
				Type: &proto.Response_Apply{
					Apply: &proto.ApplyComplete{
						Resources: []*proto.Resource{
							{
								Name: "example",
								Type: "aws_instance",
								Agents: []*proto.Agent{
									{
										Id:        uuid.NewString(),
										Directory: tmpDir,
										Auth: &proto.Agent_Token{
											Token: authToken,
										},
										DisplayApps: apps,
									},
								},
							},
						},
					},
				},
			}},
		}

		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, echoResp)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		workspace, err := client.Workspace(ctx, workspace.ID)
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

		version = coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, echoResp,
			func(req *codersdk.CreateTemplateVersionRequest) {
				req.TemplateID = template.ID
			})

		err = client.UpdateActiveTemplateVersion(ctx, template.ID, codersdk.UpdateActiveTemplateVersion{
			ID: version.ID,
		})
		require.NoError(t, err)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		// Creating another workspace is just easier.
		workspace = coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		build := coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
		require.NoError(t, err)
		agent, err = client.WorkspaceAgent(ctx, build.Resources[0].Agents[0].ID)
		require.NoError(t, err)
		require.Len(t, agent.DisplayApps, 0)
	})
}

func TestWorkspaceAgentStartupLogs(t *testing.T) {
	t.Parallel()
	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		client := coderdtest.New(t, &coderdtest.Options{
			IncludeProvisionerDaemon: true,
		})
		user := coderdtest.CreateFirstUser(t, client)
		authToken := uuid.NewString()
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
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
									Token: authToken,
								},
							}},
						}},
					},
				},
			}},
		})
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		build := coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

		agentClient := agentsdk.New(client.URL)
		agentClient.SetSessionToken(authToken)
		err := agentClient.PatchLogs(ctx, agentsdk.PatchLogs{
			Logs: []agentsdk.Log{
				{
					CreatedAt: database.Now(),
					Output:    "testing",
				},
				{
					CreatedAt: database.Now(),
					Output:    "testing2",
				},
			},
		})
		require.NoError(t, err)

		logs, closer, err := client.WorkspaceAgentLogsAfter(ctx, build.Resources[0].Agents[0].ID, 0, true)
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
		client := coderdtest.New(t, &coderdtest.Options{
			IncludeProvisionerDaemon: true,
		})
		user := coderdtest.CreateFirstUser(t, client)
		authToken := uuid.NewString()
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
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
									Token: authToken,
								},
							}},
						}},
					},
				},
			}},
		})
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		build := coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

		agentClient := agentsdk.New(client.URL)
		agentClient.SetSessionToken(authToken)
		err := agentClient.PatchLogs(ctx, agentsdk.PatchLogs{
			Logs: []agentsdk.Log{
				{
					CreatedAt: database.Now(),
					Output:    "testing",
				},
			},
		})
		require.NoError(t, err)

		logs, closer, err := client.WorkspaceAgentLogsAfter(ctx, build.Resources[0].Agents[0].ID, 0, true)
		require.NoError(t, err)
		defer func() {
			_ = closer.Close()
		}()

		first := make(chan struct{})
		go func() {
			select {
			case <-ctx.Done():
				assert.Fail(t, "context done while waiting in goroutine")
			case <-logs:
				close(first)
			}
		}()
		select {
		case <-ctx.Done():
			require.FailNow(t, "context done while waiting for first log")
		case <-first:
		}

		_ = coderdtest.CreateWorkspaceBuild(t, client, workspace, database.WorkspaceTransitionStart)

		// Send a new log message to trigger a re-check.
		err = agentClient.PatchLogs(ctx, agentsdk.PatchLogs{
			Logs: []agentsdk.Log{
				{
					CreatedAt: database.Now(),
					Output:    "testing2",
				},
			},
		})
		require.NoError(t, err)

		select {
		case <-ctx.Done():
			require.FailNow(t, "context done while waiting for logs close")
		case <-logs:
		}
	})
	t.Run("PublishesOnOverflow", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		client := coderdtest.New(t, &coderdtest.Options{
			IncludeProvisionerDaemon: true,
		})
		user := coderdtest.CreateFirstUser(t, client)
		authToken := uuid.NewString()
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
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
									Token: authToken,
								},
							}},
						}},
					},
				},
			}},
		})
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

		updates, err := client.WatchWorkspace(ctx, workspace.ID)
		require.NoError(t, err)

		agentClient := agentsdk.New(client.URL)
		agentClient.SetSessionToken(authToken)
		err = agentClient.PatchLogs(ctx, agentsdk.PatchLogs{
			Logs: []agentsdk.Log{{
				CreatedAt: database.Now(),
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
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

		agentClient := agentsdk.New(client.URL)
		agentClient.SetSessionToken(authToken)
		agentCloser := agent.New(agent.Options{
			Client: agentClient,
			Logger: slogtest.Make(t, nil).Named("agent").Leveled(slog.LevelDebug),
		})
		defer func() {
			_ = agentCloser.Close()
		}()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		resources := coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)
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
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

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
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		stopBuild, err := client.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
			TemplateVersionID: version.ID,
			Transition:        codersdk.WorkspaceTransitionStop,
		})
		require.NoError(t, err)
		coderdtest.AwaitWorkspaceBuildJob(t, client, stopBuild.ID)

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
	client, daemonCloser := coderdtest.NewWithProvisionerCloser(t, nil)
	user := coderdtest.CreateFirstUser(t, client)
	authToken := uuid.NewString()
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse:          echo.ParseComplete,
		ProvisionPlan:  echo.PlanComplete,
		ProvisionApply: echo.ProvisionApplyWithAgent(authToken),
	})
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
	coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
	daemonCloser.Close()

	agentClient := agentsdk.New(client.URL)
	agentClient.SetSessionToken(authToken)
	agentCloser := agent.New(agent.Options{
		Client: agentClient,
		Logger: slogtest.Make(t, nil).Named("agent").Leveled(slog.LevelDebug),
	})
	defer agentCloser.Close()
	resources := coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)

	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()
	conn, err := client.DialWorkspaceAgent(ctx, resources[0].Agents[0].ID, &codersdk.DialWorkspaceAgentOptions{
		Logger: slogtest.Make(t, nil).Named("client").Leveled(slog.LevelDebug),
	})
	require.NoError(t, err)
	defer conn.Close()
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

func TestWorkspaceAgentTailnetDirectDisabled(t *testing.T) {
	t.Parallel()

	dv := coderdtest.DeploymentValues(t)
	err := dv.DERP.Config.BlockDirect.Set("true")
	require.NoError(t, err)
	require.True(t, dv.DERP.Config.BlockDirect.Value())

	client, daemonCloser := coderdtest.NewWithProvisionerCloser(t, &coderdtest.Options{
		DeploymentValues: dv,
	})
	user := coderdtest.CreateFirstUser(t, client)
	authToken := uuid.NewString()
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse:          echo.ParseComplete,
		ProvisionPlan:  echo.PlanComplete,
		ProvisionApply: echo.ProvisionApplyWithAgent(authToken),
	})
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
	coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
	daemonCloser.Close()

	ctx := testutil.Context(t, testutil.WaitLong)

	// Verify that the manifest has DisableDirectConnections set to true.
	agentClient := agentsdk.New(client.URL)
	agentClient.SetSessionToken(authToken)
	manifest, err := agentClient.Manifest(ctx)
	require.NoError(t, err)
	require.True(t, manifest.DisableDirectConnections)

	agentCloser := agent.New(agent.Options{
		Client: agentClient,
		Logger: slogtest.Make(t, nil).Named("agent").Leveled(slog.LevelDebug),
	})
	defer agentCloser.Close()
	resources := coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)
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
	require.True(t, conn.BlockEndpoints())

	require.True(t, conn.AwaitReachable(ctx))
	_, p2p, _, err := conn.Ping(ctx)
	require.NoError(t, err)
	require.False(t, p2p)
}

func TestWorkspaceAgentListeningPorts(t *testing.T) {
	t.Parallel()

	setup := func(t *testing.T, apps []*proto.App) (*codersdk.Client, uint16, uuid.UUID) {
		t.Helper()

		client := coderdtest.New(t, &coderdtest.Options{
			IncludeProvisionerDaemon: true,
		})
		coderdPort, err := strconv.Atoi(client.URL.Port())
		require.NoError(t, err)

		user := coderdtest.CreateFirstUser(t, client)
		authToken := uuid.NewString()
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
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
									Token: authToken,
								},
								Apps: apps,
							}},
						}},
					},
				},
			}},
		})
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

		agentClient := agentsdk.New(client.URL)
		agentClient.SetSessionToken(authToken)
		agentCloser := agent.New(agent.Options{
			Client: agentClient,
			Logger: slogtest.Make(t, nil).Named("agent").Leveled(slog.LevelDebug),
		})
		t.Cleanup(func() {
			_ = agentCloser.Close()
		})
		resources := coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)

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

		t.Run("OK", func(t *testing.T) {
			t.Parallel()

			client, coderdPort, agentID := setup(t, nil)

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
			time.Sleep(2 * time.Second) // avoid cache
			res, err = client.WorkspaceAgentListeningPorts(ctx, agentID)
			require.NoError(t, err)

			for _, port := range res.Ports {
				if port.Network == "tcp" && port.Port == lPort {
					t.Fatalf("expected to not find TCP port %d in response", lPort)
				}
			}
		})

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

			client, coderdPort, agentID := setup(t, []*proto.App{app})

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

		client, _, agentID := setup(t, nil)

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
	client := coderdtest.New(t, &coderdtest.Options{
		IncludeProvisionerDaemon: true,
	})
	user := coderdtest.CreateFirstUser(t, client)
	authToken := uuid.NewString()
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
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse: echo.ParseComplete,
		ProvisionApply: []*proto.Response{{
			Type: &proto.Response_Apply{
				Apply: &proto.ApplyComplete{
					Resources: []*proto.Resource{{
						Name: "example",
						Type: "aws_instance",
						Agents: []*proto.Agent{{
							Id: uuid.NewString(),
							Auth: &proto.Agent_Token{
								Token: authToken,
							},
							Apps: apps,
						}},
					}},
				},
			},
		}},
	})
	coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
	coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	agentClient := agentsdk.New(client.URL)
	agentClient.SetSessionToken(authToken)

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
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

		agentClient := agentsdk.New(client.URL)
		agentClient.SetSessionToken(authToken)

		_, err := agentClient.PostStats(context.Background(), &agentsdk.Stats{
			ConnectionsByProto:          map[string]int64{"TCP": 1},
			ConnectionCount:             1,
			RxPackets:                   1,
			RxBytes:                     1,
			TxPackets:                   1,
			TxBytes:                     1,
			SessionCountVSCode:          1,
			SessionCountJetBrains:       1,
			SessionCountReconnectingPTY: 1,
			SessionCountSSH:             1,
			ConnectionMedianLatencyMS:   10,
		})
		require.NoError(t, err)

		newWorkspace, err := client.Workspace(context.Background(), workspace.ID)
		require.NoError(t, err)

		assert.True(t,
			newWorkspace.LastUsedAt.After(workspace.LastUsedAt),
			"%s is not after %s", newWorkspace.LastUsedAt, workspace.LastUsedAt,
		)
	})
}

func TestWorkspaceAgent_LifecycleState(t *testing.T) {
	t.Parallel()

	t.Run("Set", func(t *testing.T) {
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
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

		for _, res := range workspace.LatestBuild.Resources {
			for _, a := range res.Agents {
				require.Equal(t, codersdk.WorkspaceAgentLifecycleCreated, a.LifecycleState)
			}
		}

		agentClient := agentsdk.New(client.URL)
		agentClient.SetSessionToken(authToken)

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

	client := coderdtest.New(t, &coderdtest.Options{
		IncludeProvisionerDaemon: true,
	})
	user := coderdtest.CreateFirstUser(t, client)
	authToken := uuid.NewString()
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse:         echo.ParseComplete,
		ProvisionPlan: echo.PlanComplete,
		ProvisionApply: []*proto.Response{{
			Type: &proto.Response_Apply{
				Apply: &proto.ApplyComplete{
					Resources: []*proto.Resource{{
						Name: "example",
						Type: "aws_instance",
						Agents: []*proto.Agent{{
							Metadata: []*proto.Agent_Metadata{
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
							},
							Id: uuid.NewString(),
							Auth: &proto.Agent_Token{
								Token: authToken,
							},
						}},
					}},
				},
			},
		}},
	})
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
	coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

	for _, res := range workspace.LatestBuild.Resources {
		for _, a := range res.Agents {
			require.Equal(t, codersdk.WorkspaceAgentLifecycleCreated, a.LifecycleState)
		}
	}

	agentClient := agentsdk.New(client.URL)
	agentClient.SetSessionToken(authToken)

	ctx := testutil.Context(t, testutil.WaitMedium)

	manifest, err := agentClient.Manifest(ctx)
	require.NoError(t, err)

	// Verify manifest API response.
	require.Equal(t, "First Meta", manifest.Metadata[0].DisplayName)
	require.Equal(t, "foo1", manifest.Metadata[0].Key)
	require.Equal(t, "echo hi", manifest.Metadata[0].Script)
	require.EqualValues(t, 10, manifest.Metadata[0].Interval)
	require.EqualValues(t, 3, manifest.Metadata[0].Timeout)

	post := func(key string, mr codersdk.WorkspaceAgentMetadataResult) {
		err := agentClient.PostMetadata(ctx, key, mr)
		require.NoError(t, err, "post metadata", t)
	}

	workspace, err = client.Workspace(ctx, workspace.ID)
	require.NoError(t, err, "get workspace")

	agentID := workspace.LatestBuild.Resources[0].Agents[0].ID

	var update []codersdk.WorkspaceAgentMetadata

	wantMetadata1 := codersdk.WorkspaceAgentMetadataResult{
		CollectedAt: time.Now(),
		Value:       "bar",
	}

	// Initial post must come before the Watch is established.
	post("foo1", wantMetadata1)

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
	post("foo2", wantMetadata2)
	update = recvUpdate()
	require.Len(t, update, 3)
	check(wantMetadata1, update[0], true)
	check(wantMetadata2, update[1], true)

	wantMetadata1.Error = "error"
	post("foo1", wantMetadata1)
	update = recvUpdate()
	require.Len(t, update, 3)
	check(wantMetadata1, update[0], true)

	const maxValueLen = 2048
	tooLongValueMetadata := wantMetadata1
	tooLongValueMetadata.Value = strings.Repeat("a", maxValueLen*2)
	tooLongValueMetadata.Error = ""
	tooLongValueMetadata.CollectedAt = time.Now()
	post("foo3", tooLongValueMetadata)
	got := recvUpdate()[2]
	for i := 0; i < 2 && len(got.Result.Value) != maxValueLen; i++ {
		got = recvUpdate()[2]
	}
	require.Len(t, got.Result.Value, maxValueLen)
	require.NotEmpty(t, got.Result.Error)

	unknownKeyMetadata := wantMetadata1
	err = agentClient.PostMetadata(ctx, "unknown", unknownKeyMetadata)
	require.NoError(t, err)
}

func TestWorkspaceAgent_Startup(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
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
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

		agentClient := agentsdk.New(client.URL)
		agentClient.SetSessionToken(authToken)

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

		workspace, err = client.Workspace(ctx, workspace.ID)
		require.NoError(t, err)

		wsagent, err := client.WorkspaceAgent(ctx, workspace.LatestBuild.Resources[0].Agents[0].ID)
		require.NoError(t, err)
		require.Equal(t, expectedVersion, wsagent.Version)
		require.Equal(t, expectedDir, wsagent.ExpandedDirectory)
		// Sorted
		require.Equal(t, expectedSubsystems, wsagent.Subsystems)
	})

	t.Run("InvalidSemver", func(t *testing.T) {
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
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

		agentClient := agentsdk.New(client.URL)
		agentClient.SetSessionToken(authToken)

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
		IncludeProvisionerDaemon: true,
		DeploymentValues:         dv,
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
	agentToken := uuid.NewString()
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse:          echo.ParseComplete,
		ProvisionPlan:  echo.PlanComplete,
		ProvisionApply: echo.ProvisionApplyWithAgent(agentToken),
	})
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
	coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
	agentClient := agentsdk.New(client.URL)
	agentClient.SetSessionToken(agentToken)
	agentCloser := agent.New(agent.Options{
		Client: agentClient,
		Logger: logger.Named("agent"),
	})
	defer func() {
		_ = agentCloser.Close()
	}()
	resources := coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)
	agentID := resources[0].Agents[0].ID

	// Connect from a client.
	ctx := testutil.Context(t, testutil.WaitLong)
	conn1, err := client.DialWorkspaceAgent(ctx, agentID, &codersdk.DialWorkspaceAgentOptions{
		Logger: logger.Named("client1"),
	})
	require.NoError(t, err)
	defer conn1.Close()
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

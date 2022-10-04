package coderd_test

import (
	"bufio"
	"context"
	"encoding/json"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/agent"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
	"github.com/coder/coder/testutil"
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
			Parse:           echo.ParseComplete,
			ProvisionDryRun: echo.ProvisionComplete,
			Provision: []*proto.Provision_Response{{
				Type: &proto.Provision_Response_Complete{
					Complete: &proto.Provision_Complete{
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
			Parse:           echo.ParseComplete,
			ProvisionDryRun: echo.ProvisionComplete,
			Provision: []*proto.Provision_Response{{
				Type: &proto.Provision_Response_Complete{
					Complete: &proto.Provision_Complete{
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

		agentClient := codersdk.New(client.URL)
		agentClient.SessionToken = authToken
		agentCloser := agent.New(agent.Options{
			FetchMetadata:     agentClient.WorkspaceAgentMetadata,
			CoordinatorDialer: agentClient.ListenWorkspaceAgentTailnet,
			Logger:            slogtest.Make(t, nil).Named("agent").Leveled(slog.LevelDebug),
		})
		defer func() {
			_ = agentCloser.Close()
		}()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		resources := coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)
		conn, err := client.DialWorkspaceAgentTailnet(ctx, slog.Logger{}, resources[0].Agents[0].ID)
		require.NoError(t, err)
		defer func() {
			_ = conn.Close()
		}()
		require.Eventually(t, func() bool {
			_, err := conn.Ping()
			return err == nil
		}, testutil.WaitLong, testutil.IntervalFast)
	})

	t.Run("FailNonLatestBuild", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{
			IncludeProvisionerDaemon: true,
		})

		user := coderdtest.CreateFirstUser(t, client)
		authToken := uuid.NewString()
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:           echo.ParseComplete,
			ProvisionDryRun: echo.ProvisionComplete,
			Provision: []*proto.Provision_Response{{
				Type: &proto.Provision_Response_Complete{
					Complete: &proto.Provision_Complete{
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

		version = coderdtest.UpdateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:           echo.ParseComplete,
			ProvisionDryRun: echo.ProvisionComplete,
			Provision: []*proto.Provision_Response{{
				Type: &proto.Provision_Response_Complete{
					Complete: &proto.Provision_Complete{
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

		agentClient := codersdk.New(client.URL)
		agentClient.SessionToken = authToken

		_, err = agentClient.ListenWorkspaceAgentTailnet(ctx)
		require.Error(t, err)
		require.ErrorContains(t, err, "build is outdated")
	})
}

func TestWorkspaceAgentTailnet(t *testing.T) {
	t.Parallel()
	client, daemonCloser := coderdtest.NewWithProvisionerCloser(t, nil)
	user := coderdtest.CreateFirstUser(t, client)
	authToken := uuid.NewString()
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse:           echo.ParseComplete,
		ProvisionDryRun: echo.ProvisionComplete,
		Provision: []*proto.Provision_Response{{
			Type: &proto.Provision_Response_Complete{
				Complete: &proto.Provision_Complete{
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
	daemonCloser.Close()

	agentClient := codersdk.New(client.URL)
	agentClient.SessionToken = authToken
	agentCloser := agent.New(agent.Options{
		FetchMetadata:     agentClient.WorkspaceAgentMetadata,
		CoordinatorDialer: agentClient.ListenWorkspaceAgentTailnet,
		Logger:            slogtest.Make(t, nil).Named("agent").Leveled(slog.LevelDebug),
	})
	defer agentCloser.Close()
	resources := coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)

	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()
	conn, err := client.DialWorkspaceAgentTailnet(ctx, slogtest.Make(t, nil).Named("client").Leveled(slog.LevelDebug), resources[0].Agents[0].ID)
	require.NoError(t, err)
	defer conn.Close()
	sshClient, err := conn.SSHClient()
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

func TestWorkspaceAgentPTY(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		// This might be our implementation, or ConPTY itself.
		// It's difficult to find extensive tests for it, so
		// it seems like it could be either.
		t.Skip("ConPTY appears to be inconsistent on Windows.")
	}
	client := coderdtest.New(t, &coderdtest.Options{
		IncludeProvisionerDaemon: true,
	})
	user := coderdtest.CreateFirstUser(t, client)
	authToken := uuid.NewString()
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse:           echo.ParseComplete,
		ProvisionDryRun: echo.ProvisionComplete,
		Provision: []*proto.Provision_Response{{
			Type: &proto.Provision_Response_Complete{
				Complete: &proto.Provision_Complete{
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

	agentClient := codersdk.New(client.URL)
	agentClient.SessionToken = authToken
	agentCloser := agent.New(agent.Options{
		FetchMetadata:     agentClient.WorkspaceAgentMetadata,
		CoordinatorDialer: agentClient.ListenWorkspaceAgentTailnet,
		Logger:            slogtest.Make(t, nil).Named("agent").Leveled(slog.LevelDebug),
	})
	defer func() {
		_ = agentCloser.Close()
	}()
	resources := coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	conn, err := client.WorkspaceAgentReconnectingPTY(ctx, resources[0].Agents[0].ID, uuid.New(), 80, 80, "/bin/bash")
	require.NoError(t, err)
	defer conn.Close()

	// First attempt to resize the TTY.
	// The websocket will close if it fails!
	data, err := json.Marshal(codersdk.ReconnectingPTYRequest{
		Height: 250,
		Width:  250,
	})
	require.NoError(t, err)
	_, err = conn.Write(data)
	require.NoError(t, err)
	bufRead := bufio.NewReader(conn)

	// Brief pause to reduce the likelihood that we send keystrokes while
	// the shell is simultaneously sending a prompt.
	time.Sleep(100 * time.Millisecond)

	data, err = json.Marshal(codersdk.ReconnectingPTYRequest{
		Data: "echo test\r\n",
	})
	require.NoError(t, err)
	_, err = conn.Write(data)
	require.NoError(t, err)

	expectLine := func(matcher func(string) bool) {
		for {
			line, err := bufRead.ReadString('\n')
			require.NoError(t, err)
			if matcher(line) {
				break
			}
		}
	}
	matchEchoCommand := func(line string) bool {
		return strings.Contains(line, "echo test")
	}
	matchEchoOutput := func(line string) bool {
		return strings.Contains(line, "test") && !strings.Contains(line, "echo")
	}

	expectLine(matchEchoCommand)
	expectLine(matchEchoOutput)
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
			Name:    "code-server",
			Command: "some-command",
			Url:     "http://localhost:3000",
			Icon:    "/code.svg",
		},
		{
			Name:    "code-server-2",
			Command: "some-command",
			Url:     "http://localhost:3000",
			Icon:    "/code.svg",
			Healthcheck: &proto.Healthcheck{
				Url:       "http://localhost:3000",
				Interval:  5,
				Threshold: 6,
			},
		},
	}
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse: echo.ParseComplete,
		Provision: []*proto.Provision_Response{{
			Type: &proto.Provision_Response_Complete{
				Complete: &proto.Provision_Complete{
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

	agentClient := codersdk.New(client.URL)
	agentClient.SessionToken = authToken

	apiApps, err := agentClient.WorkspaceAgentApps(ctx)
	require.NoError(t, err)
	require.EqualValues(t, codersdk.WorkspaceAppHealthDisabled, apiApps[0].Health)
	require.EqualValues(t, codersdk.WorkspaceAppHealthInitializing, apiApps[1].Health)
	err = agentClient.PostWorkspaceAgentAppHealth(ctx, codersdk.PostWorkspaceAppHealthsRequest{})
	require.Error(t, err)
	// empty
	err = agentClient.PostWorkspaceAgentAppHealth(ctx, codersdk.PostWorkspaceAppHealthsRequest{})
	require.Error(t, err)
	// invalid name
	err = agentClient.PostWorkspaceAgentAppHealth(ctx, codersdk.PostWorkspaceAppHealthsRequest{
		Healths: map[string]codersdk.WorkspaceAppHealth{
			"bad-name": codersdk.WorkspaceAppHealthDisabled,
		},
	})
	require.Error(t, err)
	// healcheck disabled
	err = agentClient.PostWorkspaceAgentAppHealth(ctx, codersdk.PostWorkspaceAppHealthsRequest{
		Healths: map[string]codersdk.WorkspaceAppHealth{
			"code-server": codersdk.WorkspaceAppHealthInitializing,
		},
	})
	require.Error(t, err)
	// invalid value
	err = agentClient.PostWorkspaceAgentAppHealth(ctx, codersdk.PostWorkspaceAppHealthsRequest{
		Healths: map[string]codersdk.WorkspaceAppHealth{
			"code-server-2": codersdk.WorkspaceAppHealth("bad-value"),
		},
	})
	require.Error(t, err)
	// update to healthy
	err = agentClient.PostWorkspaceAgentAppHealth(ctx, codersdk.PostWorkspaceAppHealthsRequest{
		Healths: map[string]codersdk.WorkspaceAppHealth{
			"code-server-2": codersdk.WorkspaceAppHealthHealthy,
		},
	})
	require.NoError(t, err)
	apiApps, err = agentClient.WorkspaceAgentApps(ctx)
	require.NoError(t, err)
	require.EqualValues(t, codersdk.WorkspaceAppHealthHealthy, apiApps[1].Health)
	// update to unhealthy
	err = agentClient.PostWorkspaceAgentAppHealth(ctx, codersdk.PostWorkspaceAppHealthsRequest{
		Healths: map[string]codersdk.WorkspaceAppHealth{
			"code-server-2": codersdk.WorkspaceAppHealthUnhealthy,
		},
	})
	require.NoError(t, err)
	apiApps, err = agentClient.WorkspaceAgentApps(ctx)
	require.NoError(t, err)
	require.EqualValues(t, codersdk.WorkspaceAppHealthUnhealthy, apiApps[1].Health)
}

package coderd_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pion/webrtc/v3"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/agent"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/peer"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
)

func TestWorkspaceAgent(t *testing.T) {
	t.Parallel()
	t.Run("Connect", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{
			IncludeProvisionerD: true,
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
								Id:        uuid.NewString(),
								Directory: "/tmp",
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

		resources, err := client.WorkspaceResourcesByBuild(context.Background(), workspace.LatestBuild.ID)
		require.NoError(t, err)
		require.Equal(t, "/tmp", resources[0].Agents[0].Directory)
		_, err = client.WorkspaceAgent(context.Background(), resources[0].Agents[0].ID)
		require.NoError(t, err)
	})
}

func TestWorkspaceAgentListen(t *testing.T) {
	t.Parallel()

	t.Run("Connect", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{
			IncludeProvisionerD: true,
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
			FetchMetadata: agentClient.WorkspaceAgentMetadata,
			WebRTCDialer:  agentClient.ListenWorkspaceAgent,
			Logger:        slogtest.Make(t, nil).Named("agent").Leveled(slog.LevelDebug),
		})
		t.Cleanup(func() {
			_ = agentCloser.Close()
		})
		resources := coderdtest.AwaitWorkspaceAgents(t, client, workspace.LatestBuild.ID)
		conn, err := client.DialWorkspaceAgent(context.Background(), resources[0].Agents[0].ID, nil)
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = conn.Close()
		})
		_, err = conn.Ping()
		require.NoError(t, err)
	})

	t.Run("FailNonLatestBuild", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		client := coderdtest.New(t, &coderdtest.Options{
			IncludeProvisionerD: true,
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

		stopBuild, err := client.CreateWorkspaceBuild(context.Background(), workspace.ID, codersdk.CreateWorkspaceBuildRequest{
			TemplateVersionID: version.ID,
			Transition:        codersdk.WorkspaceTransitionStop,
		})
		require.NoError(t, err)
		coderdtest.AwaitWorkspaceBuildJob(t, client, stopBuild.ID)

		agentClient := codersdk.New(client.URL)
		agentClient.SessionToken = authToken

		_, err = agentClient.ListenWorkspaceAgent(ctx, slogtest.Make(t, nil))
		require.Error(t, err)
		require.ErrorContains(t, err, "build is outdated")
	})
}

func TestWorkspaceAgentTURN(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, &coderdtest.Options{
		IncludeProvisionerD: true,
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
		FetchMetadata: agentClient.WorkspaceAgentMetadata,
		WebRTCDialer:  agentClient.ListenWorkspaceAgent,
		Logger:        slogtest.Make(t, nil).Named("agent").Leveled(slog.LevelDebug),
	})
	t.Cleanup(func() {
		_ = agentCloser.Close()
	})
	resources := coderdtest.AwaitWorkspaceAgents(t, client, workspace.LatestBuild.ID)
	opts := &peer.ConnOptions{
		Logger: slogtest.Make(t, nil).Named("client"),
	}
	// Force a TURN connection!
	opts.SettingEngine.SetNetworkTypes([]webrtc.NetworkType{webrtc.NetworkTypeTCP4})
	conn, err := client.DialWorkspaceAgent(context.Background(), resources[0].Agents[0].ID, opts)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = conn.Close()
	})
	_, err = conn.Ping()
	require.NoError(t, err)
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
		FetchMetadata: agentClient.WorkspaceAgentMetadata,
		WebRTCDialer:  agentClient.ListenWorkspaceAgent,
		EnableTailnet: true,
		NodeDialer:    agentClient.WorkspaceAgentNodeBroker,
		Logger:        slogtest.Make(t, nil).Named("agent").Leveled(slog.LevelDebug),
	})
	t.Cleanup(func() {
		_ = agentCloser.Close()
	})
	resources := coderdtest.AwaitWorkspaceAgents(t, client, workspace.LatestBuild.ID)

	time.Sleep(3 * time.Second)

	conn, err := client.DialWorkspaceAgentTailnet(context.Background(), resources[0].Agents[0].ID, slogtest.Make(t, nil).Named("tailnet").Leveled(slog.LevelDebug))
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = conn.Close()
	})
	sshClient, err := conn.SSHClient()
	require.NoError(t, err)
	session, err := sshClient.NewSession()
	require.NoError(t, err)
	output, err := session.CombinedOutput("echo test")
	require.NoError(t, err)
	fmt.Printf("Output: %s\n", output)
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
		IncludeProvisionerD: true,
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
		FetchMetadata: agentClient.WorkspaceAgentMetadata,
		WebRTCDialer:  agentClient.ListenWorkspaceAgent,
		Logger:        slogtest.Make(t, nil).Named("agent").Leveled(slog.LevelDebug),
	})
	t.Cleanup(func() {
		_ = agentCloser.Close()
	})
	resources := coderdtest.AwaitWorkspaceAgents(t, client, workspace.LatestBuild.ID)

	conn, err := client.WorkspaceAgentReconnectingPTY(context.Background(), resources[0].Agents[0].ID, uuid.New(), 80, 80, "/bin/bash")
	require.NoError(t, err)
	defer conn.Close()

	// First attempt to resize the TTY.
	// The websocket will close if it fails!
	data, err := json.Marshal(agent.ReconnectingPTYRequest{
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

	data, err = json.Marshal(agent.ReconnectingPTYRequest{
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

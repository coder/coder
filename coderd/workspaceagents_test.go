package coderd_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

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
	client := coderdtest.New(t, nil)
	user := coderdtest.CreateFirstUser(t, client)
	daemonCloser := coderdtest.NewProvisionerDaemon(t, client)
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

	resources, err := client.WorkspaceResourcesByBuild(context.Background(), workspace.LatestBuild.ID)
	require.NoError(t, err)
	_, err = client.WorkspaceAgent(context.Background(), resources[0].Agents[0].ID)
	require.NoError(t, err)
}

func TestWorkspaceAgentListen(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, nil)
	user := coderdtest.CreateFirstUser(t, client)
	daemonCloser := coderdtest.NewProvisionerDaemon(t, client)
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
	agentCloser := agent.New(agentClient.ListenWorkspaceAgent, &agent.Options{
		Logger: slogtest.Make(t, nil).Named("agent").Leveled(slog.LevelDebug),
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
}

func TestWorkspaceAgentTURN(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, nil)
	user := coderdtest.CreateFirstUser(t, client)
	daemonCloser := coderdtest.NewProvisionerDaemon(t, client)
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
	agentCloser := agent.New(agentClient.ListenWorkspaceAgent, &agent.Options{
		Logger: slogtest.Make(t, nil),
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

func TestWorkspaceAgentPTY(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, nil)
	user := coderdtest.CreateFirstUser(t, client)
	daemonCloser := coderdtest.NewProvisionerDaemon(t, client)
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
	agentCloser := agent.New(agentClient.ListenWorkspaceAgent, &agent.Options{
		Logger: slogtest.Make(t, nil),
	})
	t.Cleanup(func() {
		_ = agentCloser.Close()
	})
	resources := coderdtest.AwaitWorkspaceAgents(t, client, workspace.LatestBuild.ID)

	conn, err := client.WorkspaceAgentReconnectingPTY(context.Background(), resources[0].Agents[0].ID, uuid.New(), 80, 80)
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

	data, err = json.Marshal(agent.ReconnectingPTYRequest{
		Data: "echo test\r\n",
	})
	require.NoError(t, err)
	_, err = conn.Write(data)
	require.NoError(t, err)

	findEcho := func() {
		for {
			read, err := conn.Read(data)
			require.NoError(t, err)
			if strings.Contains(string(data[:read]), "test") {
				return
			}
		}
	}

	findEcho()
	findEcho()
}

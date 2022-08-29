package coderd_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/agent"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/peer"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
	"github.com/coder/coder/testutil"
)

func TestMetrics(t *testing.T) {
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
	agentCloser := agent.New(agentClient.ListenWorkspaceAgent, &agent.Options{
		Logger:        slogtest.Make(t, nil),
		StatsReporter: agentClient.AgentReportStats,
	})
	defer func() {
		_ = agentCloser.Close()
	}()
	resources := coderdtest.AwaitWorkspaceAgents(t, client, workspace.LatestBuild.ID)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	opts := &peer.ConnOptions{
		Logger: slogtest.Make(t, nil).Named("client"),
	}

	daus, err := client.GetDAUsFromAgentStats(context.Background())
	require.NoError(t, err)

	require.Equal(t, &codersdk.DAUsResponse{
		Entries: []codersdk.DAUEntry{},
	}, daus, "no DAUs when stats are empty")

	conn, err := client.DialWorkspaceAgent(ctx, resources[0].Agents[0].ID, opts)
	require.NoError(t, err)
	defer func() {
		_ = conn.Close()
	}()

	sshConn, err := conn.SSHClient()
	require.NoError(t, err)

	session, err := sshConn.NewSession()
	require.NoError(t, err)

	_, err = session.Output("echo hello")
	require.NoError(t, err)

	// Give enough time for stats to hit DB
	// and metrics cache to refresh.
	time.Sleep(time.Second * 5)

	daus, err = client.GetDAUsFromAgentStats(context.Background())
	require.NoError(t, err)

	require.Equal(t, &codersdk.DAUsResponse{
		Entries: []codersdk.DAUEntry{
			{

				Date: time.Now().UTC().Truncate(time.Hour * 24),
				DAUs: 1,
			},
		},
	}, daus)
}

package coderd_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/agent"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/codersdk/agentsdk"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
	"github.com/coder/coder/testutil"
)

func TestDeploymentInsights(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, &coderdtest.Options{
		IncludeProvisionerDaemon:    true,
		AgentStatsRefreshInterval:   time.Millisecond * 100,
		MetricsCacheRefreshInterval: time.Millisecond * 100,
	})

	user := coderdtest.CreateFirstUser(t, client)
	authToken := uuid.NewString()
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse:         echo.ParseComplete,
		ProvisionPlan: echo.ProvisionComplete,
		ProvisionApply: []*proto.Provision_Response{{
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
	require.Empty(t, template.BuildTimeStats[codersdk.WorkspaceTransitionStart])

	coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
	coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

	agentClient := agentsdk.New(client.URL)
	agentClient.SetSessionToken(authToken)
	agentCloser := agent.New(agent.Options{
		Logger: slogtest.Make(t, nil),
		Client: agentClient,
	})
	defer func() {
		_ = agentCloser.Close()
	}()
	resources := coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	daus, err := client.DeploymentDAUs(context.Background())
	require.NoError(t, err)

	require.Equal(t, &codersdk.DeploymentDAUsResponse{
		Entries: []codersdk.DAUEntry{},
	}, daus, "no DAUs when stats are empty")

	res, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{})
	require.NoError(t, err)
	assert.Zero(t, res.Workspaces[0].LastUsedAt)

	conn, err := client.DialWorkspaceAgent(ctx, resources[0].Agents[0].ID, &codersdk.DialWorkspaceAgentOptions{
		Logger: slogtest.Make(t, nil).Named("tailnet"),
	})
	require.NoError(t, err)
	defer func() {
		_ = conn.Close()
	}()

	sshConn, err := conn.SSHClient(ctx)
	require.NoError(t, err)
	_ = sshConn.Close()

	wantDAUs := &codersdk.DeploymentDAUsResponse{
		Entries: []codersdk.DAUEntry{
			{

				Date:   time.Now().UTC().Truncate(time.Hour * 24),
				Amount: 1,
			},
		},
	}
	require.Eventuallyf(t, func() bool {
		daus, err = client.DeploymentDAUs(ctx)
		require.NoError(t, err)
		return len(daus.Entries) > 0
	},
		testutil.WaitShort, testutil.IntervalFast,
		"deployment daus never loaded",
	)
	gotDAUs, err := client.DeploymentDAUs(ctx)
	require.NoError(t, err)
	require.Equal(t, gotDAUs, wantDAUs)

	template, err = client.Template(ctx, template.ID)
	require.NoError(t, err)

	res, err = client.Workspaces(ctx, codersdk.WorkspaceFilter{})
	require.NoError(t, err)
}

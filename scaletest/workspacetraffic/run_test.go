package workspacetraffic_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/coder/coder/agent"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/codersdk/agentsdk"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
	"github.com/coder/coder/scaletest/workspacetraffic"
	"github.com/coder/coder/testutil"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRun(t *testing.T) {
	t.Parallel()

	// We need to stand up an in-memory coderd and run a fake workspace.
	var (
		client    = coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user      = coderdtest.CreateFirstUser(t, client)
		authToken = uuid.NewString()
		agentName = "agent"
		version   = coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:         echo.ParseComplete,
			ProvisionPlan: echo.ProvisionComplete,
			ProvisionApply: []*proto.Provision_Response{{
				Type: &proto.Provision_Response_Complete{
					Complete: &proto.Provision_Complete{
						Resources: []*proto.Resource{{
							Name: "example",
							Type: "aws_instance",
							Agents: []*proto.Agent{{
								// Agent ID gets generated no matter what we say ¯\_(ツ)_/¯
								Name: agentName,
								Auth: &proto.Agent_Token{
									Token: authToken,
								},
								Apps: []*proto.App{},
							}},
						}},
					},
				},
			}},
		})
		template = coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		_        = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		// In order to be picked up as a scaletest workspace, the workspace must be named specifically
		ws = coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID, func(cwr *codersdk.CreateWorkspaceRequest) {
			cwr.Name = "scaletest-test"
		})
		_ = coderdtest.AwaitWorkspaceBuildJob(t, client, ws.LatestBuild.ID)
	)

	// We also need a running agent to run this test.
	agentClient := agentsdk.New(client.URL)
	agentClient.SetSessionToken(authToken)
	agentCloser := agent.New(agent.Options{
		Client: agentClient,
	})
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)

	t.Cleanup(cancel)
	t.Cleanup(func() {
		_ = agentCloser.Close()
	})
	// Make sure the agent is connected before we go any further.
	resources := coderdtest.AwaitWorkspaceAgents(t, client, ws.ID)
	var agentID uuid.UUID
	for _, res := range resources {
		for _, agt := range res.Agents {
			agentID = agt.ID
		}
	}
	require.NotEqual(t, uuid.Nil, agentID, "did not expect agentID to be nil")

	// Now we can start the runner.
	reg := prometheus.NewRegistry()
	metrics := workspacetraffic.NewMetrics(reg, "username", "workspace_name", "agent_name")
	runner := workspacetraffic.NewRunner(client, workspacetraffic.Config{
		AgentID:        agentID,
		AgentName:      agentName,
		WorkspaceName:  ws.Name,
		WorkspaceOwner: ws.OwnerName,
		BytesPerTick:   1024,
		TickInterval:   testutil.IntervalMedium,
		Duration:       testutil.WaitMedium - time.Second,
		Registry:       reg,
	}, metrics)

	var logs strings.Builder
	require.NoError(t, runner.Run(ctx, "", &logs), "unexpected error calling Run()")

	var collected []prometheus.Metric
	collectCh := make(chan prometheus.Metric)
	go func() {
		for metric := range collectCh {
			collected = append(collected, metric)
		}
	}()
	reg.Collect(collectCh)
	assert.NotEmpty(t, collected)
	for _, m := range collected {
		assert.NotZero(t, m.Desc())
	}
}

package coderconnect_test

import (
	"bytes"
	"strconv"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/scaletest/coderconnect"
	"github.com/coder/coder/v2/scaletest/harness"
	"github.com/coder/coder/v2/scaletest/workspacebuild"
	"github.com/coder/coder/v2/testutil"
)

func TestRun(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, &coderdtest.Options{
		IncludeProvisionerDaemon: true,
	})
	user := coderdtest.CreateFirstUser(t, client)

	numUsers := 2
	userWorkspaces := 2
	numWorkspaces := numUsers * userWorkspaces

	authToken := uuid.NewString()
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse:         echo.ParseComplete,
		ProvisionPlan: echo.PlanComplete,
		ProvisionApply: []*proto.Response{
			{
				Type: &proto.Response_Apply{
					Apply: &proto.ApplyComplete{
						Resources: []*proto.Resource{
							{
								Name: "example",
								Type: "aws_instance",
								Agents: []*proto.Agent{
									{
										Id:   uuid.NewString(),
										Name: "agent",
										Auth: &proto.Agent_Token{
											Token: authToken,
										},
										Apps: []*proto.App{},
									},
								},
							},
						},
					},
				},
			},
		},
	})

	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

	barrier := harness.NewBarrier(numUsers)
	metrics := coderconnect.NewMetrics(prometheus.NewRegistry(), "num_workspaces", "username")

	th := harness.NewTestHarness(harness.ConcurrentExecutionStrategy{}, harness.ConcurrentExecutionStrategy{})
	for i := range numUsers {
		cfg := coderconnect.Config{
			User: coderconnect.UserConfig{
				OrganizationID: user.OrganizationID,
			},
			Workspace: workspacebuild.Config{
				OrganizationID: user.OrganizationID,
				Request: codersdk.CreateWorkspaceRequest{
					TemplateID: template.ID,
				},
				NoWaitForAgents: true,
			},
			WorkspaceCount:          int64(userWorkspaces),
			DialTimeout:             testutil.WaitMedium,
			WorkspaceUpdatesTimeout: testutil.WaitLong,
			NoCleanup:               false,
			Metrics:                 metrics,
			MetricLabelValues:       []string{"1", "fake-username"},
			DialBarrier:             barrier,
		}
		err := cfg.Validate()
		require.NoError(t, err)
		th.AddRun("coderconnect", strconv.Itoa(i), coderconnect.NewRunner(client, cfg))
	}

	ctx := testutil.Context(t, testutil.WaitLong)

	// Run the tests
	err := th.Run(ctx)
	require.NoError(t, err)

	users, err := client.Users(ctx, codersdk.UsersRequest{})
	require.NoError(t, err)
	require.Len(t, users.Users, numUsers+1) // owner + created users

	workspaces, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{})
	require.NoError(t, err)
	require.Len(t, workspaces.Workspaces, numWorkspaces)

	// Cleanup the tests
	cleanupLogs := bytes.NewBuffer(nil)
	err = th.Cleanup(ctx)
	require.NoError(t, err)
	t.Log("Cleanup logs:\n\n" + cleanupLogs.String())

	workspaces, err = client.Workspaces(ctx, codersdk.WorkspaceFilter{})
	require.NoError(t, err)
	require.Len(t, workspaces.Workspaces, 0)

	users, err = client.Users(ctx, codersdk.UsersRequest{})
	require.NoError(t, err)
	require.Len(t, users.Users, 1) // owner

	for i := range numUsers {
		id := strconv.Itoa(i)
		require.Contains(t, th.Results().Runs, "coderconnect/"+id)
		metrics := th.Results().Runs["coderconnect/"+id].Metrics
		require.Contains(t, metrics, coderconnect.WorkspaceUpdatesLatencyMetric)
		require.Contains(t, metrics, coderconnect.WorkspaceUpdatesErrorsTotal)
	}
}

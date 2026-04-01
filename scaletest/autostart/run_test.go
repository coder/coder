package autostart_test

import (
	"io"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/scaletest/autostart"
	"github.com/coder/coder/v2/scaletest/createusers"
	"github.com/coder/coder/v2/scaletest/workspacebuild"
	"github.com/coder/coder/v2/testutil"
)

func TestRun(t *testing.T) {
	t.Parallel()
	numUsers := 2
	autoStartDelay := 2 * time.Minute

	// Faking a workspace autostart schedule start time at the coderd level
	// is difficult and error-prone.
	t.Skip("This test takes several minutes to run, and is intended as a manual regression test")

	ctx := testutil.Context(t, time.Minute*3)

	client := coderdtest.New(t, &coderdtest.Options{
		IncludeProvisionerDaemon: true,
		AutobuildTicker:          time.NewTicker(time.Second * 1).C,
	})
	user := coderdtest.CreateFirstUser(t, client)

	authToken := uuid.NewString()
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse:         echo.ParseComplete,
		ProvisionPlan: echo.PlanComplete,
		ProvisionGraph: []*proto.Response{
			{
				Type: &proto.Response_Graph{
					Graph: &proto.GraphComplete{
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

	barrier := new(sync.WaitGroup)
	barrier.Add(numUsers)
	metrics := autostart.NewMetrics(prometheus.NewRegistry())

	eg, runCtx := errgroup.WithContext(ctx)

	runners := make([]*autostart.Runner, 0, numUsers)
	for i := range numUsers {
		cfg := autostart.Config{
			User: createusers.Config{
				OrganizationID: user.OrganizationID,
			},
			Workspace: workspacebuild.Config{
				OrganizationID: user.OrganizationID,
				Request: codersdk.CreateWorkspaceRequest{
					TemplateID: template.ID,
				},
				NoWaitForAgents: true,
			},
			WorkspaceJobTimeout: testutil.WaitMedium,
			AutostartDelay:      autoStartDelay,
			AutostartTimeout:    testutil.WaitShort,
			Metrics:             metrics,
			SetupBarrier:        barrier,
		}
		err := cfg.Validate()
		require.NoError(t, err)

		runner := autostart.NewRunner(client, cfg)
		runners = append(runners, runner)
		eg.Go(func() error {
			return runner.Run(runCtx, strconv.Itoa(i), io.Discard)
		})
	}

	err := eg.Wait()
	require.NoError(t, err)

	users, err := client.Users(ctx, codersdk.UsersRequest{})
	require.NoError(t, err)
	require.Len(t, users.Users, 1+numUsers) // owner + created users

	workspaces, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{})
	require.NoError(t, err)
	require.Len(t, workspaces.Workspaces, numUsers) // one workspace per user

	// Verify that workspaces have autostart schedules set and are running
	for _, workspace := range workspaces.Workspaces {
		require.NotNil(t, workspace.AutostartSchedule)
		require.Equal(t, codersdk.WorkspaceTransitionStart, workspace.LatestBuild.Transition)
		require.Equal(t, codersdk.ProvisionerJobSucceeded, workspace.LatestBuild.Job.Status)
	}

	cleanupEg, cleanupCtx := errgroup.WithContext(ctx)
	for i, runner := range runners {
		cleanupEg.Go(func() error {
			return runner.Cleanup(cleanupCtx, strconv.Itoa(i), io.Discard)
		})
	}
	err = cleanupEg.Wait()
	require.NoError(t, err)

	workspaces, err = client.Workspaces(ctx, codersdk.WorkspaceFilter{})
	require.NoError(t, err)
	require.Len(t, workspaces.Workspaces, 0)

	users, err = client.Users(ctx, codersdk.UsersRequest{})
	require.NoError(t, err)
	require.Len(t, users.Users, 1) // owner

	for _, runner := range runners {
		metrics := runner.GetMetrics()
		require.Contains(t, metrics, autostart.AutostartTotalLatencyMetric)
		latency, ok := metrics[autostart.AutostartTotalLatencyMetric].(float64)
		require.True(t, ok)
		jobCreationLatency, ok := metrics[autostart.AutostartJobCreationLatencyMetric].(float64)
		require.True(t, ok)
		jobAcquiredLatency, ok := metrics[autostart.AutostartJobAcquiredLatencyMetric].(float64)
		require.True(t, ok)
		require.Greater(t, latency, float64(0))
		require.Greater(t, jobCreationLatency, float64(0))
		require.Greater(t, jobAcquiredLatency, float64(0))
	}
}

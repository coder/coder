package coderd_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/testutil"
)

func TestWorkspaceAgentShutdownLookupFromRESTWhileConnected(t *testing.T) {
	t.Parallel()
	setHandler, cancel, serverURL, options := coderdtest.NewOptions(t, nil)
	// Isolate the RPC response for a completed build from connection retirement.
	// This does not test production teardown timing: the next monitor check
	// disconnects the agent after completion, preventing further lookups.
	// TestCheckBuildIsLatestDuringShutdown covers that retirement condition.
	options.AgentConnectionUpdateFrequency = time.Hour
	api := coderd.New(options)
	setHandler(api.RootHandler)
	daemon := coderdtest.NewProvisionerDaemon(t, api)
	client := codersdk.New(serverURL, codersdk.WithHTTPClient(coderdtest.NewIsolatedHTTPClient(serverURL)))
	t.Cleanup(func() {
		cancel()
		_ = daemon.Close()
		_ = api.Close()
		client.HTTPClient.CloseIdleConnections()
	})
	user := coderdtest.CreateFirstUser(t, client)
	for _, transition := range []codersdk.WorkspaceTransition{codersdk.WorkspaceTransitionStop, codersdk.WorkspaceTransitionDelete} {
		t.Run(string(transition), func(t *testing.T) {
			t.Parallel()
			authToken := uuid.NewString()
			version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
				Parse: echo.ParseComplete, ProvisionPlan: echo.PlanComplete,
				ProvisionGraph: echo.ProvisionGraphWithAgent(authToken),
			})
			coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
			template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
			workspace := coderdtest.CreateWorkspace(t, client, template.ID)
			coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)
			ctx := testutil.Context(t, testutil.WaitLong)
			agentClient := agentsdk.New(client.URL, agentsdk.WithFixedToken(authToken))
			rpc, _, err := agentClient.ConnectRPC213WithRole(ctx, "agent")
			require.NoError(t, err)
			defer rpc.DRPCConn().Close()
			initial, err := rpc.GetWorkspaceShutdown(ctx, &emptypb.Empty{})
			require.NoError(t, err)
			require.Empty(t, initial.BuildId)

			build, err := client.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{Transition: transition})
			require.NoError(t, err)
			completed := coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, build.ID)
			require.Equal(t, codersdk.ProvisionerJobSucceeded, completed.Job.Status)
			// A completed build's reason is available on a surviving connection.
			shutdown, err := rpc.GetWorkspaceShutdown(ctx, &emptypb.Empty{})
			require.NoError(t, err)
			require.Equal(t, build.ID.String(), shutdown.BuildId)
			require.Equal(t, string(transition), shutdown.Transition)
			require.Equal(t, string(build.Reason), shutdown.BuildReason)
		})
	}
}

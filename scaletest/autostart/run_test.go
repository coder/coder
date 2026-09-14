package autostart_test

import (
	"context"
	"io"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/scaletest/autostart"
	"github.com/coder/coder/v2/scaletest/createusers"
	"github.com/coder/coder/v2/scaletest/loadtestutil"
	"github.com/coder/coder/v2/scaletest/workspacebuild"
	"github.com/coder/coder/v2/testutil"
)

func TestRun(t *testing.T) {
	t.Parallel()
	numUsers := 2
	autoStartDelay := 2 * time.Minute

	// Faking a workspace autostart schedule start time at the coderd level
	// is difficult and error-prone. This test verifies the setup phase only
	// (creating workspaces, stopping them, and configuring autostart schedules).
	t.Skip("This test takes several minutes to run, and is intended as a manual regression test")

	ctx := testutil.Context(t, time.Minute*3)

	client := coderdtest.New(t, &coderdtest.Options{
		IncludeProvisionerDaemon: true,
		AutobuildTicker:          time.NewTicker(time.Second * 1).C,
		DeploymentValues: coderdtest.DeploymentValues(t, func(dv *codersdk.DeploymentValues) {
			dv.Experiments = []string{string(codersdk.ExperimentWorkspaceBuildUpdates)}
		}),
	})
	user := coderdtest.CreateFirstUser(t, client)

	template := createAutostartTestTemplate(t, client, user.OrganizationID)

	barrier := new(sync.WaitGroup)
	barrier.Add(numUsers)

	// Pre-create channels for each workspace keyed by deterministic name.
	workspaceChannels := make(map[string]chan codersdk.WorkspaceBuildUpdate)
	for i := range numUsers {
		id := strconv.Itoa(i)
		workspaceName := loadtestutil.GenerateDeterministicWorkspaceName(id)
		workspaceChannels[workspaceName] = make(chan codersdk.WorkspaceBuildUpdate, 16)
	}

	// Start watching all workspace builds.
	decoder, err := client.WatchAllWorkspaceBuilds(ctx)
	require.NoError(t, err)
	defer decoder.Close()

	// Start the dispatcher goroutine.
	go func() {
		for update := range decoder.Chan() {
			if ch, ok := workspaceChannels[update.WorkspaceName]; ok {
				select {
				case ch <- update:
				case <-ctx.Done():
					return
				}
			}
		}
		for _, ch := range workspaceChannels {
			close(ch)
		}
	}()

	eg, runCtx := errgroup.WithContext(ctx)

	runners := make([]*autostart.Runner, 0, numUsers)
	for i := range numUsers {
		id := strconv.Itoa(i)
		workspaceName := loadtestutil.GenerateDeterministicWorkspaceName(id)
		cfg := autostart.Config{
			User: createusers.Config{
				OrganizationID: user.OrganizationID,
			},
			Workspace: workspacebuild.Config{
				OrganizationID: user.OrganizationID,
				Request: codersdk.CreateWorkspaceRequest{
					TemplateID: template.ID,
					Name:       workspaceName,
				},
				NoWaitForAgents: true,
			},
			WorkspaceJobTimeout: testutil.WaitMedium,
			AutostartDelay:      autoStartDelay,
			SetupBarrier:        barrier,
			BuildUpdates:        workspaceChannels[workspaceName],
		}
		err := cfg.Validate()
		require.NoError(t, err)

		runner := autostart.NewRunner(client, cfg)
		runners = append(runners, runner)
		eg.Go(func() error {
			return runner.Run(runCtx, strconv.Itoa(i), io.Discard)
		})
	}

	err = eg.Wait()
	require.NoError(t, err)

	users, err := client.Users(ctx, codersdk.UsersRequest{})
	require.NoError(t, err)
	require.Len(t, users.Users, 1+numUsers) // owner + created users

	workspaces, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{})
	require.NoError(t, err)
	require.Len(t, workspaces.Workspaces, numUsers) // one workspace per user

	// Verify that workspaces have autostart schedules set and are stopped
	// (the test exits after configuring autostart, before it triggers).
	for _, workspace := range workspaces.Workspaces {
		require.NotNil(t, workspace.AutostartSchedule)
		require.Equal(t, codersdk.WorkspaceTransitionStop, workspace.LatestBuild.Transition)
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
}

// TestRunReuseUser drives the reuse branch of RunReturningResult: the runner is
// given a pre-created user + token instead of creating one. Driving the whole
// autostart cycle to completion needs the scheduled autostart build to fire (see
// the skipped TestRun above), so this test disables the autobuild ticker and
// runs the runner up to the autostart wait, which is enough to assert the reuse
// guarantees:
//   - the workspace is built as the reused user and no new user is created, and
//   - Cleanup deletes the workspace but not the reused user.
func TestRunReuseUser(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)

	client := coderdtest.New(t, &coderdtest.Options{
		IncludeProvisionerDaemon: true,
		// Set the autostart ticker far in the future so the scheduled autostart
		// build never fires during the test; the runner blocks waiting for it and
		// we cancel instead.
		AutobuildTicker: time.NewTicker(time.Hour).C,
		DeploymentValues: coderdtest.DeploymentValues(t, func(dv *codersdk.DeploymentValues) {
			dv.Experiments = []string{string(codersdk.ExperimentWorkspaceBuildUpdates)}
		}),
	})
	owner := coderdtest.CreateFirstUser(t, client)
	template := createAutostartTestTemplate(t, client, owner.OrganizationID)

	// Pre-create the user the runner will reuse.
	reuseClient, reuseUser := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

	workspaceName := loadtestutil.GenerateDeterministicWorkspaceName("0")
	updates := make(chan codersdk.WorkspaceBuildUpdate, 16)

	decoder, err := client.WatchAllWorkspaceBuilds(ctx)
	require.NoError(t, err)
	defer decoder.Close()
	go func() {
		for update := range decoder.Chan() {
			if update.WorkspaceName != workspaceName {
				continue
			}
			select {
			case updates <- update:
			case <-ctx.Done():
				return
			}
		}
	}()

	barrier := new(sync.WaitGroup)
	barrier.Add(1)

	cfg := autostart.Config{
		// Reuse mode still resolves the workspace organization from User.
		User:           createusers.Config{OrganizationID: owner.OrganizationID},
		SessionToken:   reuseClient.SessionToken(),
		PreCreatedUser: reuseUser,
		Workspace: workspacebuild.Config{
			OrganizationID: owner.OrganizationID,
			Request: codersdk.CreateWorkspaceRequest{
				TemplateID: template.ID,
				Name:       workspaceName,
			},
			NoWaitForAgents: true,
		},
		WorkspaceJobTimeout:   testutil.WaitMedium,
		AutostartDelay:        2 * time.Minute,
		AutostartBuildTimeout: testutil.WaitLong,
		SetupBarrier:          barrier,
		BuildUpdates:          updates,
	}
	require.NoError(t, cfg.Validate())

	runner := autostart.NewRunner(client, cfg)

	runCtx, cancelRun := context.WithCancel(ctx)
	runErr := make(chan error, 1)
	go func() {
		runErr <- runner.Run(runCtx, "0", io.Discard)
	}()

	// Wait until the runner has built the workspace as the reused user and
	// configured autostart; it then blocks waiting for the scheduled build.
	var ws codersdk.Workspace
	require.Eventually(t, func() bool {
		wss, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{Name: workspaceName})
		if err != nil || len(wss.Workspaces) != 1 {
			return false
		}
		ws = wss.Workspaces[0]
		return ws.AutostartSchedule != nil && *ws.AutostartSchedule != "" &&
			ws.LatestBuild.Transition == codersdk.WorkspaceTransitionStop &&
			ws.LatestBuild.Job.Status == codersdk.ProvisionerJobSucceeded
	}, testutil.WaitLong, testutil.IntervalMedium)

	// The workspace was built as the reused user, and no extra user was created.
	require.Equal(t, reuseUser.ID, ws.OwnerID)
	require.Equal(t, reuseUser.Username, ws.OwnerName)
	users, err := client.Users(ctx, codersdk.UsersRequest{})
	require.NoError(t, err)
	require.Len(t, users.Users, 2) // owner + reused user only

	// Unblock the runner, which is waiting for the scheduled autostart build.
	cancelRun()
	require.ErrorIs(t, <-runErr, context.Canceled)

	// Cleanup removes the workspace but must not delete the reused user.
	require.NoError(t, runner.Cleanup(ctx, "0", io.Discard))

	wss, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{})
	require.NoError(t, err)
	require.Len(t, wss.Workspaces, 0)

	_, err = client.User(ctx, reuseUser.ID.String())
	require.NoError(t, err) // reused user still exists
}

func createAutostartTestTemplate(t *testing.T, client *codersdk.Client, orgID uuid.UUID) codersdk.Template {
	t.Helper()

	authToken := uuid.NewString()
	version := coderdtest.CreateTemplateVersion(t, client, orgID, &echo.Responses{
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

	template := coderdtest.CreateTemplate(t, client, orgID, version.ID)
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	return template
}

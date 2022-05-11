package executor_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/lifecycle/schedule"
	"github.com/coder/coder/codersdk"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func Test_Executor_Autostart_OK(t *testing.T) {
	t.Parallel()

	var (
		ctx    = context.Background()
		err    error
		tickCh = make(chan time.Time)
		client = coderdtest.New(t, &coderdtest.Options{
			LifecycleTicker: tickCh,
		})
		// Given: we have a user with a workspace
		workspace = mustProvisionWorkspace(t, client)
	)
	// Given: workspace is stopped
	workspace = mustTransitionWorkspace(t, client, workspace.ID, database.WorkspaceTransitionStart, database.WorkspaceTransitionStop)

	// Given: the workspace initially has autostart disabled
	require.Empty(t, workspace.AutostartSchedule)

	// When: we enable workspace autostart
	sched, err := schedule.Weekly("* * * * *")
	require.NoError(t, err)
	require.NoError(t, client.UpdateWorkspaceAutostart(ctx, workspace.ID, codersdk.UpdateWorkspaceAutostartRequest{
		Schedule: sched.String(),
	}))

	// When: the lifecycle executor ticks
	go func() {
		tickCh <- time.Now().UTC().Add(time.Minute)
		close(tickCh)
	}()

	// Then: the workspace should be started
	require.Eventually(t, func() bool {
		ws := mustWorkspace(t, client, workspace.ID)
		return ws.LatestBuild.Job.Status == codersdk.ProvisionerJobSucceeded &&
			ws.LatestBuild.Transition == database.WorkspaceTransitionStart
	}, 5*time.Second, 250*time.Millisecond)
}

func Test_Executor_Autostart_AlreadyRunning(t *testing.T) {
	t.Parallel()

	var (
		ctx    = context.Background()
		err    error
		tickCh = make(chan time.Time)
		client = coderdtest.New(t, &coderdtest.Options{
			LifecycleTicker: tickCh,
		})
		// Given: we have a user with a workspace
		workspace = mustProvisionWorkspace(t, client)
	)

	// Given: we ensure the workspace is running
	require.Equal(t, database.WorkspaceTransitionStart, workspace.LatestBuild.Transition)

	// Given: the workspace initially has autostart disabled
	require.Empty(t, workspace.AutostartSchedule)

	// When: we enable workspace autostart
	sched, err := schedule.Weekly("* * * * *")
	require.NoError(t, err)
	require.NoError(t, client.UpdateWorkspaceAutostart(ctx, workspace.ID, codersdk.UpdateWorkspaceAutostartRequest{
		Schedule: sched.String(),
	}))

	// When: the lifecycle executor ticks
	go func() {
		tickCh <- time.Now().UTC().Add(time.Minute)
		close(tickCh)
	}()

	// Then: the workspace should not be started.
	require.Never(t, func() bool {
		ws := mustWorkspace(t, client, workspace.ID)
		return ws.LatestBuild.ID != workspace.LatestBuild.ID && ws.LatestBuild.Transition == database.WorkspaceTransitionStart
	}, 5*time.Second, 250*time.Millisecond)
}

func Test_Executor_Autostart_NotEnabled(t *testing.T) {
	t.Parallel()

	var (
		tickCh = make(chan time.Time)
		client = coderdtest.New(t, &coderdtest.Options{
			LifecycleTicker: tickCh,
		})
		// Given: we have a user with a workspace
		workspace = mustProvisionWorkspace(t, client)
	)

	// Given: workspace is stopped
	workspace = mustTransitionWorkspace(t, client, workspace.ID, database.WorkspaceTransitionStart, database.WorkspaceTransitionStop)

	// Given: the workspace has autostart disabled
	require.Empty(t, workspace.AutostartSchedule)

	// When: the lifecycle executor ticks
	go func() {
		tickCh <- time.Now().UTC().Add(time.Minute)
		close(tickCh)
	}()

	// Then: the workspace should not be started.
	require.Never(t, func() bool {
		ws := mustWorkspace(t, client, workspace.ID)
		return ws.LatestBuild.ID != workspace.LatestBuild.ID && ws.LatestBuild.Transition == database.WorkspaceTransitionStart
	}, 5*time.Second, 250*time.Millisecond)
}

func Test_Executor_Autostop_OK(t *testing.T) {
	t.Parallel()

	var (
		ctx    = context.Background()
		err    error
		tickCh = make(chan time.Time)
		client = coderdtest.New(t, &coderdtest.Options{
			LifecycleTicker: tickCh,
		})
		// Given: we have a user with a workspace
		workspace = mustProvisionWorkspace(t, client)
	)
	// Given: workspace is running
	require.Equal(t, database.WorkspaceTransitionStart, workspace.LatestBuild.Transition)

	// Given: the workspace initially has autostop disabled
	require.Empty(t, workspace.AutostopSchedule)

	// When: we enable workspace autostop
	sched, err := schedule.Weekly("* * * * *")
	require.NoError(t, err)
	require.NoError(t, client.UpdateWorkspaceAutostop(ctx, workspace.ID, codersdk.UpdateWorkspaceAutostopRequest{
		Schedule: sched.String(),
	}))

	// When: the lifecycle executor ticks
	go func() {
		tickCh <- time.Now().UTC().Add(time.Minute)
		close(tickCh)
	}()

	// Then: the workspace should be started
	require.Eventually(t, func() bool {
		ws := mustWorkspace(t, client, workspace.ID)
		return ws.LatestBuild.ID != workspace.LatestBuild.ID && ws.LatestBuild.Transition == database.WorkspaceTransitionStop
	}, 5*time.Second, 250*time.Millisecond)
}
func Test_Executor_Autostop_AlreadyStopped(t *testing.T) {
	t.Parallel()

	var (
		ctx    = context.Background()
		err    error
		tickCh = make(chan time.Time)
		client = coderdtest.New(t, &coderdtest.Options{
			LifecycleTicker: tickCh,
		})
		// Given: we have a user with a workspace
		workspace = mustProvisionWorkspace(t, client)
	)

	// Given: workspace is stopped
	workspace = mustTransitionWorkspace(t, client, workspace.ID, database.WorkspaceTransitionStart, database.WorkspaceTransitionStop)

	// Given: the workspace initially has autostop disabled
	require.Empty(t, workspace.AutostopSchedule)

	// When: we enable workspace autostart
	sched, err := schedule.Weekly("* * * * *")
	require.NoError(t, err)
	require.NoError(t, client.UpdateWorkspaceAutostop(ctx, workspace.ID, codersdk.UpdateWorkspaceAutostopRequest{
		Schedule: sched.String(),
	}))

	// When: the lifecycle executor ticks
	go func() {
		tickCh <- time.Now().UTC().Add(time.Minute)
		close(tickCh)
	}()

	// Then: the workspace should not be stopped.
	require.Never(t, func() bool {
		ws := mustWorkspace(t, client, workspace.ID)
		return ws.LatestBuild.ID == workspace.LatestBuild.ID && ws.LatestBuild.Transition == database.WorkspaceTransitionStop
	}, 5*time.Second, 250*time.Millisecond)
}

func Test_Executor_Autostop_NotEnabled(t *testing.T) {
	t.Parallel()

	var (
		tickCh = make(chan time.Time)
		client = coderdtest.New(t, &coderdtest.Options{
			LifecycleTicker: tickCh,
		})
		// Given: we have a user with a workspace
		workspace = mustProvisionWorkspace(t, client)
	)

	// Given: workspace is running
	require.Equal(t, database.WorkspaceTransitionStart, workspace.LatestBuild.Transition)

	// Given: the workspace has autostop disabled
	require.Empty(t, workspace.AutostopSchedule)

	// When: the lifecycle executor ticks
	go func() {
		tickCh <- time.Now().UTC().Add(time.Minute)
		close(tickCh)
	}()

	// Then: the workspace should not be stopped.
	require.Never(t, func() bool {
		ws := mustWorkspace(t, client, workspace.ID)
		return ws.LatestBuild.ID == workspace.LatestBuild.ID && ws.LatestBuild.Transition == database.WorkspaceTransitionStop
	}, 5*time.Second, 250*time.Millisecond)
}

func Test_Executor_Workspace_Deleted(t *testing.T) {
	t.Parallel()

	var (
		ctx    = context.Background()
		err    error
		tickCh = make(chan time.Time)
		client = coderdtest.New(t, &coderdtest.Options{
			LifecycleTicker: tickCh,
		})
		// Given: we have a user with a workspace
		workspace = mustProvisionWorkspace(t, client)
	)

	// Given: the workspace initially has autostart disabled
	require.Empty(t, workspace.AutostopSchedule)

	// When: we enable workspace autostart
	sched, err := schedule.Weekly("* * * * *")
	require.NoError(t, err)
	require.NoError(t, client.UpdateWorkspaceAutostop(ctx, workspace.ID, codersdk.UpdateWorkspaceAutostopRequest{
		Schedule: sched.String(),
	}))

	// Given: workspace is deleted
	workspace = mustTransitionWorkspace(t, client, workspace.ID, database.WorkspaceTransitionStart, database.WorkspaceTransitionDelete)

	// When: the lifecycle executor ticks
	go func() {
		tickCh <- time.Now().UTC().Add(time.Minute)
		close(tickCh)
	}()

	// Then: nothing should happen
	require.Never(t, func() bool {
		ws := mustWorkspace(t, client, workspace.ID)
		return ws.LatestBuild.Transition != database.WorkspaceTransitionDelete
	}, 5*time.Second, 250*time.Millisecond)
}

func Test_Executor_Workspace_TooEarly(t *testing.T) {
	t.Parallel()

	var (
		ctx    = context.Background()
		err    error
		tickCh = make(chan time.Time)
		client = coderdtest.New(t, &coderdtest.Options{
			LifecycleTicker: tickCh,
		})
		// Given: we have a user with a workspace
		workspace = mustProvisionWorkspace(t, client)
	)

	// Given: the workspace initially has autostart disabled
	require.Empty(t, workspace.AutostopSchedule)

	// When: we enable workspace autostart with some time in the future
	futureTime := time.Now().Add(time.Hour)
	futureTimeCron := fmt.Sprintf("%d %d * * *", futureTime.Minute(), futureTime.Hour())
	sched, err := schedule.Weekly(futureTimeCron)
	require.NoError(t, err)
	require.NoError(t, client.UpdateWorkspaceAutostop(ctx, workspace.ID, codersdk.UpdateWorkspaceAutostopRequest{
		Schedule: sched.String(),
	}))

	// When: the lifecycle executor ticks
	go func() {
		tickCh <- time.Now().UTC()
		close(tickCh)
	}()

	// Then: nothing should happen
	require.Never(t, func() bool {
		ws := mustWorkspace(t, client, workspace.ID)
		return ws.LatestBuild.Transition != database.WorkspaceTransitionStart
	}, 5*time.Second, 250*time.Millisecond)
}

func mustProvisionWorkspace(t *testing.T, client *codersdk.Client) codersdk.Workspace {
	t.Helper()
	coderdtest.NewProvisionerDaemon(t, client)
	user := coderdtest.CreateFirstUser(t, client)
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
	ws := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
	coderdtest.AwaitWorkspaceBuildJob(t, client, ws.LatestBuild.ID)
	return mustWorkspace(t, client, ws.ID)
}

func mustTransitionWorkspace(t *testing.T, client *codersdk.Client, workspaceID uuid.UUID, from, to database.WorkspaceTransition) codersdk.Workspace {
	t.Helper()
	ctx := context.Background()
	workspace, err := client.Workspace(ctx, workspaceID)
	require.NoError(t, err, "unexpected error fetching workspace")
	require.Equal(t, workspace.LatestBuild.Transition, from, "expected workspace state: %s got: %s", from, workspace.LatestBuild.Transition)

	template, err := client.Template(ctx, workspace.TemplateID)
	require.NoError(t, err, "fetch workspace template")

	build, err := client.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
		TemplateVersionID: template.ActiveVersionID,
		Transition:        to,
	})
	require.NoError(t, err, "unexpected error transitioning workspace to %s", to)

	_ = coderdtest.AwaitWorkspaceBuildJob(t, client, build.ID)

	updated := mustWorkspace(t, client, workspace.ID)
	require.Equal(t, to, updated.LatestBuild.Transition, "expected workspace to be in state %s but got %s", to, updated.LatestBuild.Transition)
	return updated
}

func mustWorkspace(t *testing.T, client *codersdk.Client, workspaceID uuid.UUID) codersdk.Workspace {
	ctx := context.Background()
	ws, err := client.Workspace(ctx, workspaceID)
	require.NoError(t, err, "no workspace found with id %s", workspaceID)
	return ws
}

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

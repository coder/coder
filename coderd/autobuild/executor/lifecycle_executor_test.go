package executor_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/coder/coder/coderd/autobuild/schedule"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/codersdk"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestExecutorAutostartOK(t *testing.T) {
	t.Parallel()

	var (
		ctx    = context.Background()
		err    error
		tickCh = make(chan time.Time)
		client = coderdtest.New(t, &coderdtest.Options{
			AutobuildTicker: tickCh,
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

	// When: the autobuild executor ticks
	go func() {
		tickCh <- time.Now().UTC().Add(time.Minute)
		close(tickCh)
	}()

	// Then: the workspace should be started
	<-time.After(5 * time.Second)
	ws := mustWorkspace(t, client, workspace.ID)
	require.NotEqual(t, workspace.LatestBuild.ID, ws.LatestBuild.ID, "expected a workspace build to occur")
	require.Equal(t, codersdk.ProvisionerJobSucceeded, ws.LatestBuild.Job.Status, "expected provisioner job to have succeeded")
	require.Equal(t, database.WorkspaceTransitionStart, ws.LatestBuild.Transition, "expected latest transition to be start")
}

func TestExecutorAutostartTemplateUpdated(t *testing.T) {
	t.Parallel()

	var (
		ctx    = context.Background()
		err    error
		tickCh = make(chan time.Time)
		client = coderdtest.New(t, &coderdtest.Options{
			AutobuildTicker: tickCh,
		})
		// Given: we have a user with a workspace
		workspace = mustProvisionWorkspace(t, client)
	)
	// Given: workspace is stopped
	workspace = mustTransitionWorkspace(t, client, workspace.ID, database.WorkspaceTransitionStart, database.WorkspaceTransitionStop)

	// Given: the workspace initially has autostart disabled
	require.Empty(t, workspace.AutostartSchedule)

	// Given: the workspace template has been updated
	orgs, err := client.OrganizationsByUser(ctx, workspace.OwnerID)
	require.NoError(t, err)
	require.Len(t, orgs, 1)

	newVersion := coderdtest.UpdateTemplateVersion(t, client, orgs[0].ID, nil, workspace.TemplateID)
	coderdtest.AwaitTemplateVersionJob(t, client, newVersion.ID)
	require.NoError(t, client.UpdateActiveTemplateVersion(ctx, workspace.TemplateID, codersdk.UpdateActiveTemplateVersion{
		ID: newVersion.ID,
	}))

	// When: we enable workspace autostart
	sched, err := schedule.Weekly("* * * * *")
	require.NoError(t, err)
	require.NoError(t, client.UpdateWorkspaceAutostart(ctx, workspace.ID, codersdk.UpdateWorkspaceAutostartRequest{
		Schedule: sched.String(),
	}))

	// When: the autobuild executor ticks
	go func() {
		tickCh <- time.Now().UTC().Add(time.Minute)
		close(tickCh)
	}()

	// Then: the workspace should be started using the previous template version, and not the updated version.
	<-time.After(5 * time.Second)
	ws := mustWorkspace(t, client, workspace.ID)
	require.NotEqual(t, workspace.LatestBuild.ID, ws.LatestBuild.ID, "expected a workspace build to occur")
	require.Equal(t, codersdk.ProvisionerJobSucceeded, ws.LatestBuild.Job.Status, "expected provisioner job to have succeeded")
	require.Equal(t, database.WorkspaceTransitionStart, ws.LatestBuild.Transition, "expected latest transition to be start")
	require.Equal(t, workspace.LatestBuild.TemplateVersionID, ws.LatestBuild.TemplateVersionID, "expected workspace build to be using the old template version")
}

func TestExecutorAutostartAlreadyRunning(t *testing.T) {
	t.Parallel()

	var (
		ctx    = context.Background()
		err    error
		tickCh = make(chan time.Time)
		client = coderdtest.New(t, &coderdtest.Options{
			AutobuildTicker: tickCh,
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

	// When: the autobuild executor ticks
	go func() {
		tickCh <- time.Now().UTC().Add(time.Minute)
		close(tickCh)
	}()

	// Then: the workspace should not be started.
	<-time.After(5 * time.Second)
	ws := mustWorkspace(t, client, workspace.ID)
	require.Equal(t, workspace.LatestBuild.ID, ws.LatestBuild.ID, "expected no further workspace builds to occur")
	require.Equal(t, database.WorkspaceTransitionStart, ws.LatestBuild.Transition, "expected workspace to be running")
}

func TestExecutorAutostartNotEnabled(t *testing.T) {
	t.Parallel()

	var (
		tickCh = make(chan time.Time)
		client = coderdtest.New(t, &coderdtest.Options{
			AutobuildTicker: tickCh,
		})
		// Given: we have a user with a workspace
		workspace = mustProvisionWorkspace(t, client)
	)

	// Given: workspace is stopped
	workspace = mustTransitionWorkspace(t, client, workspace.ID, database.WorkspaceTransitionStart, database.WorkspaceTransitionStop)

	// Given: the workspace has autostart disabled
	require.Empty(t, workspace.AutostartSchedule)

	// When: the autobuild executor ticks
	go func() {
		tickCh <- time.Now().UTC().Add(time.Minute)
		close(tickCh)
	}()

	// Then: the workspace should not be started.
	<-time.After(5 * time.Second)
	ws := mustWorkspace(t, client, workspace.ID)
	require.Equal(t, workspace.LatestBuild.ID, ws.LatestBuild.ID, "expected no further workspace builds to occur")
	require.NotEqual(t, database.WorkspaceTransitionStart, ws.LatestBuild.Transition, "expected workspace not to be running")
}

func TestExecutorAutostopOK(t *testing.T) {
	t.Parallel()

	var (
		ctx    = context.Background()
		err    error
		tickCh = make(chan time.Time)
		client = coderdtest.New(t, &coderdtest.Options{
			AutobuildTicker: tickCh,
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

	// When: the autobuild executor ticks
	go func() {
		tickCh <- time.Now().UTC().Add(time.Minute)
		close(tickCh)
	}()

	// Then: the workspace should be started
	<-time.After(5 * time.Second)
	ws := mustWorkspace(t, client, workspace.ID)
	require.NotEqual(t, workspace.LatestBuild.ID, ws.LatestBuild.ID, "expected a workspace build to occur")
	require.Equal(t, codersdk.ProvisionerJobSucceeded, ws.LatestBuild.Job.Status, "expected provisioner job to have succeeded")
	require.Equal(t, database.WorkspaceTransitionStop, ws.LatestBuild.Transition, "expected workspace not to be running")
}

func TestExecutorAutostopAlreadyStopped(t *testing.T) {
	t.Parallel()

	var (
		ctx    = context.Background()
		err    error
		tickCh = make(chan time.Time)
		client = coderdtest.New(t, &coderdtest.Options{
			AutobuildTicker: tickCh,
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

	// When: the autobuild executor ticks
	go func() {
		tickCh <- time.Now().UTC().Add(time.Minute)
		close(tickCh)
	}()

	// Then: the workspace should not be stopped.
	<-time.After(5 * time.Second)
	ws := mustWorkspace(t, client, workspace.ID)
	require.Equal(t, workspace.LatestBuild.ID, ws.LatestBuild.ID, "expected no further workspace builds to occur")
	require.Equal(t, database.WorkspaceTransitionStop, ws.LatestBuild.Transition, "expected workspace not to be running")
}

func TestExecutorAutostopNotEnabled(t *testing.T) {
	t.Parallel()

	var (
		tickCh = make(chan time.Time)
		client = coderdtest.New(t, &coderdtest.Options{
			AutobuildTicker: tickCh,
		})
		// Given: we have a user with a workspace
		workspace = mustProvisionWorkspace(t, client)
	)

	// Given: workspace is running
	require.Equal(t, database.WorkspaceTransitionStart, workspace.LatestBuild.Transition)

	// Given: the workspace has autostop disabled
	require.Empty(t, workspace.AutostopSchedule)

	// When: the autobuild executor ticks
	go func() {
		tickCh <- time.Now().UTC().Add(time.Minute)
		close(tickCh)
	}()

	// Then: the workspace should not be stopped.
	<-time.After(5 * time.Second)
	ws := mustWorkspace(t, client, workspace.ID)
	require.Equal(t, workspace.LatestBuild.ID, ws.LatestBuild.ID, "expected no further workspace builds to occur")
	require.Equal(t, database.WorkspaceTransitionStart, ws.LatestBuild.Transition, "expected workspace to be running")
}

func TestExecutorWorkspaceDeleted(t *testing.T) {
	t.Parallel()

	var (
		ctx    = context.Background()
		err    error
		tickCh = make(chan time.Time)
		client = coderdtest.New(t, &coderdtest.Options{
			AutobuildTicker: tickCh,
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

	// When: the autobuild executor ticks
	go func() {
		tickCh <- time.Now().UTC().Add(time.Minute)
		close(tickCh)
	}()

	// Then: nothing should happen
	<-time.After(5 * time.Second)
	ws := mustWorkspace(t, client, workspace.ID)
	require.Equal(t, workspace.LatestBuild.ID, ws.LatestBuild.ID, "expected no further workspace builds to occur")
	require.Equal(t, database.WorkspaceTransitionDelete, ws.LatestBuild.Transition, "expected workspace to be deleted")
}

func TestExecutorWorkspaceTooEarly(t *testing.T) {
	t.Parallel()

	var (
		ctx    = context.Background()
		err    error
		tickCh = make(chan time.Time)
		client = coderdtest.New(t, &coderdtest.Options{
			AutobuildTicker: tickCh,
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

	// When: the autobuild executor ticks
	go func() {
		tickCh <- time.Now().UTC()
		close(tickCh)
	}()

	// Then: nothing should happen
	<-time.After(5 * time.Second)
	ws := mustWorkspace(t, client, workspace.ID)
	require.Equal(t, workspace.LatestBuild.ID, ws.LatestBuild.ID, "expected no further workspace builds to occur")
	require.Equal(t, database.WorkspaceTransitionStart, ws.LatestBuild.Transition, "expected workspace to be running")
}

func TestExecutorAutostartMultipleOK(t *testing.T) {
	if os.Getenv("DB") == "" {
		t.Skip(`This test only really works when using a "real" database, similar to a HA setup`)
	}

	t.Parallel()

	var (
		ctx     = context.Background()
		err     error
		tickCh  = make(chan time.Time)
		tickCh2 = make(chan time.Time)
		client  = coderdtest.New(t, &coderdtest.Options{
			AutobuildTicker: tickCh,
		})
		_ = coderdtest.New(t, &coderdtest.Options{
			AutobuildTicker: tickCh2,
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

	// When: the autobuild executor ticks
	go func() {
		tickCh <- time.Now().UTC().Add(time.Minute)
		tickCh2 <- time.Now().UTC().Add(time.Minute)
		close(tickCh)
		close(tickCh2)
	}()

	// Then: the workspace should be started
	<-time.After(5 * time.Second)
	ws := mustWorkspace(t, client, workspace.ID)
	require.NotEqual(t, workspace.LatestBuild.ID, ws.LatestBuild.ID, "expected a workspace build to occur")
	require.Equal(t, codersdk.ProvisionerJobSucceeded, ws.LatestBuild.Job.Status, "expected provisioner job to have succeeded")
	require.Equal(t, database.WorkspaceTransitionStart, ws.LatestBuild.Transition, "expected latest transition to be start")
	builds, err := client.WorkspaceBuilds(ctx, ws.ID)
	require.NoError(t, err, "fetch list of workspace builds from primary")
	// One build to start, one stop transition, and one autostart. No more.
	require.Len(t, builds, 3, "unexpected number of builds for workspace from primary")
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

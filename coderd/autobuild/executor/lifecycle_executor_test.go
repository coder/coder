package executor_test

import (
	"context"
	"fmt"
	"os"
	"strings"
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
			AutobuildTicker:     tickCh,
			IncludeProvisionerD: true,
		})
		// Given: we have a user with a workspace
		workspace = mustProvisionWorkspace(t, client)
	)
	// Given: workspace is stopped
	workspace = mustTransitionWorkspace(t, client, workspace.ID, database.WorkspaceTransitionStart, database.WorkspaceTransitionStop)

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
	require.Equal(t, codersdk.WorkspaceTransitionStart, ws.LatestBuild.Transition, "expected latest transition to be start")
}

func TestExecutorAutostartTemplateUpdated(t *testing.T) {
	t.Parallel()

	var (
		ctx    = context.Background()
		err    error
		tickCh = make(chan time.Time)
		client = coderdtest.New(t, &coderdtest.Options{
			AutobuildTicker:     tickCh,
			IncludeProvisionerD: true,
		})
		// Given: we have a user with a workspace
		workspace = mustProvisionWorkspace(t, client)
	)
	// Given: workspace is stopped
	workspace = mustTransitionWorkspace(t, client, workspace.ID, database.WorkspaceTransitionStart, database.WorkspaceTransitionStop)

	// Given: the workspace template has been updated
	orgs, err := client.OrganizationsByUser(ctx, workspace.OwnerID.String())
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
	require.Equal(t, codersdk.WorkspaceTransitionStart, ws.LatestBuild.Transition, "expected latest transition to be start")
	require.Equal(t, workspace.LatestBuild.TemplateVersionID, ws.LatestBuild.TemplateVersionID, "expected workspace build to be using the old template version")
}

func TestExecutorAutostartAlreadyRunning(t *testing.T) {
	t.Parallel()

	var (
		ctx    = context.Background()
		err    error
		tickCh = make(chan time.Time)
		client = coderdtest.New(t, &coderdtest.Options{
			AutobuildTicker:     tickCh,
			IncludeProvisionerD: true,
		})
		// Given: we have a user with a workspace
		workspace = mustProvisionWorkspace(t, client)
	)

	// Given: we ensure the workspace is running
	require.Equal(t, codersdk.WorkspaceTransitionStart, workspace.LatestBuild.Transition)

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
	require.Equal(t, codersdk.WorkspaceTransitionStart, ws.LatestBuild.Transition, "expected workspace to be running")
}

func TestExecutorAutostartNotEnabled(t *testing.T) {
	t.Parallel()

	var (
		tickCh = make(chan time.Time)
		client = coderdtest.New(t, &coderdtest.Options{
			AutobuildTicker:     tickCh,
			IncludeProvisionerD: true,
		})
		// Given: we have a user with a workspace
		workspace = mustProvisionWorkspace(t, client, func(cwr *codersdk.CreateWorkspaceRequest) {
			cwr.AutostartSchedule = nil
		})
	)

	// Given: workspace does not have autostart enabled
	require.Empty(t, workspace.AutostartSchedule)

	// Given: workspace is stopped
	workspace = mustTransitionWorkspace(t, client, workspace.ID, database.WorkspaceTransitionStart, database.WorkspaceTransitionStop)

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
		tickCh = make(chan time.Time)
		client = coderdtest.New(t, &coderdtest.Options{
			AutobuildTicker:     tickCh,
			IncludeProvisionerD: true,
		})
		// Given: we have a user with a workspace
		workspace = mustProvisionWorkspace(t, client)
	)
	// Given: workspace is running
	require.Equal(t, codersdk.WorkspaceTransitionStart, workspace.LatestBuild.Transition)
	require.NotZero(t, workspace.LatestBuild.Deadline)

	// When: the autobuild executor ticks *after* the deadline:
	go func() {
		tickCh <- workspace.LatestBuild.Deadline.Add(time.Minute)
		close(tickCh)
	}()

	// Then: the workspace should be stopped
	<-time.After(5 * time.Second)
	ws := mustWorkspace(t, client, workspace.ID)
	require.NotEqual(t, workspace.LatestBuild.ID, ws.LatestBuild.ID, "expected a workspace build to occur")
	require.Equal(t, codersdk.ProvisionerJobSucceeded, ws.LatestBuild.Job.Status, "expected provisioner job to have succeeded")
	require.Equal(t, codersdk.WorkspaceTransitionStop, ws.LatestBuild.Transition, "expected workspace not to be running")
}

func TestExecutorAutostopExtend(t *testing.T) {
	t.Parallel()

	var (
		ctx    = context.Background()
		tickCh = make(chan time.Time)
		client = coderdtest.New(t, &coderdtest.Options{
			AutobuildTicker:     tickCh,
			IncludeProvisionerD: true,
		})
		// Given: we have a user with a workspace
		workspace        = mustProvisionWorkspace(t, client)
		originalDeadline = workspace.LatestBuild.Deadline
	)
	// Given: workspace is running
	require.Equal(t, codersdk.WorkspaceTransitionStart, workspace.LatestBuild.Transition)
	require.NotZero(t, originalDeadline)

	// Given: we extend the workspace deadline
	err := client.PutExtendWorkspace(ctx, workspace.ID, codersdk.PutExtendWorkspaceRequest{
		Deadline: originalDeadline.Add(30 * time.Minute),
	})
	require.NoError(t, err, "extend workspace deadline")

	// When: the autobuild executor ticks *after* the original deadline:
	go func() {
		tickCh <- originalDeadline.Add(time.Minute)
	}()

	// Then: nothing should happen
	<-time.After(5 * time.Second)
	ws := mustWorkspace(t, client, workspace.ID)
	require.Equal(t, workspace.LatestBuild.ID, ws.LatestBuild.ID, "expected no further workspace builds to occur")
	require.Equal(t, codersdk.WorkspaceTransitionStart, ws.LatestBuild.Transition, "expected workspace to be running")

	// When: the autobuild executor ticks after the *new* deadline:
	go func() {
		tickCh <- ws.LatestBuild.Deadline.Add(time.Minute)
		close(tickCh)
	}()

	// Then: the workspace should be stopped
	<-time.After(5 * time.Second)
	ws = mustWorkspace(t, client, workspace.ID)
	require.NotEqual(t, workspace.LatestBuild.ID, ws.LatestBuild.ID, "expected a workspace build to occur")
	require.Equal(t, codersdk.ProvisionerJobSucceeded, ws.LatestBuild.Job.Status, "expected provisioner job to have succeeded")
	require.Equal(t, codersdk.WorkspaceTransitionStop, ws.LatestBuild.Transition, "expected workspace not to be running")
}

func TestExecutorAutostopAlreadyStopped(t *testing.T) {
	t.Parallel()

	var (
		tickCh = make(chan time.Time)
		client = coderdtest.New(t, &coderdtest.Options{
			AutobuildTicker:     tickCh,
			IncludeProvisionerD: true,
		})
		// Given: we have a user with a workspace (disabling autostart)
		workspace = mustProvisionWorkspace(t, client, func(cwr *codersdk.CreateWorkspaceRequest) {
			cwr.AutostartSchedule = nil
		})
	)

	// Given: workspace is stopped
	workspace = mustTransitionWorkspace(t, client, workspace.ID, database.WorkspaceTransitionStart, database.WorkspaceTransitionStop)

	// When: the autobuild executor ticks past the TTL
	go func() {
		tickCh <- workspace.LatestBuild.Deadline.Add(time.Minute)
		close(tickCh)
	}()

	// Then: the workspace should not be stopped.
	<-time.After(5 * time.Second)
	ws := mustWorkspace(t, client, workspace.ID)
	require.Equal(t, codersdk.WorkspaceTransitionStop, ws.LatestBuild.Transition, "expected workspace not to be running")
	require.Equal(t, workspace.LatestBuild.ID, ws.LatestBuild.ID, "expected no further workspace builds to occur")
}

func TestExecutorAutostopNotEnabled(t *testing.T) {
	t.Parallel()

	var (
		tickCh = make(chan time.Time)
		client = coderdtest.New(t, &coderdtest.Options{
			AutobuildTicker:     tickCh,
			IncludeProvisionerD: true,
		})
		// Given: we have a user with a workspace that has no TTL set
		workspace = mustProvisionWorkspace(t, client, func(cwr *codersdk.CreateWorkspaceRequest) {
			cwr.TTL = nil
		})
	)

	// Given: workspace has no TTL set
	require.Nil(t, workspace.TTL)

	// Given: workspace is running
	require.Equal(t, codersdk.WorkspaceTransitionStart, workspace.LatestBuild.Transition)

	// When: the autobuild executor ticks past the TTL
	go func() {
		tickCh <- workspace.LatestBuild.Deadline.Add(time.Minute)
		close(tickCh)
	}()

	// Then: the workspace should not be stopped.
	<-time.After(5 * time.Second)
	ws := mustWorkspace(t, client, workspace.ID)
	require.Equal(t, workspace.LatestBuild.ID, ws.LatestBuild.ID, "expected no further workspace builds to occur")
	require.Equal(t, codersdk.WorkspaceTransitionStart, ws.LatestBuild.Transition, "expected workspace to be running")
}

func TestExecutorWorkspaceDeleted(t *testing.T) {
	t.Parallel()

	var (
		ctx    = context.Background()
		err    error
		tickCh = make(chan time.Time)
		client = coderdtest.New(t, &coderdtest.Options{
			AutobuildTicker:     tickCh,
			IncludeProvisionerD: true,
		})
		// Given: we have a user with a workspace
		workspace = mustProvisionWorkspace(t, client)
	)

	// When: we enable workspace autostart
	sched, err := schedule.Weekly("* * * * *")
	require.NoError(t, err)
	require.NoError(t, client.UpdateWorkspaceAutostart(ctx, workspace.ID, codersdk.UpdateWorkspaceAutostartRequest{
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
	require.Equal(t, codersdk.WorkspaceTransitionDelete, ws.LatestBuild.Transition, "expected workspace to be deleted")
}

func TestExecutorWorkspaceAutostartTooEarly(t *testing.T) {
	t.Parallel()

	var (
		ctx    = context.Background()
		err    error
		tickCh = make(chan time.Time)
		client = coderdtest.New(t, &coderdtest.Options{
			AutobuildTicker:     tickCh,
			IncludeProvisionerD: true,
		})
		futureTime     = time.Now().Add(time.Hour)
		futureTimeCron = fmt.Sprintf("%d %d * * *", futureTime.Minute(), futureTime.Hour())
		// Given: we have a user with a workspace configured to autostart some time in the future
		workspace = mustProvisionWorkspace(t, client, func(cwr *codersdk.CreateWorkspaceRequest) {
			cwr.AutostartSchedule = &futureTimeCron
		})
	)

	// When: we enable workspace autostart with some time in the future
	sched, err := schedule.Weekly(futureTimeCron)
	require.NoError(t, err)
	require.NoError(t, client.UpdateWorkspaceAutostart(ctx, workspace.ID, codersdk.UpdateWorkspaceAutostartRequest{
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
	require.Equal(t, codersdk.WorkspaceTransitionStart, ws.LatestBuild.Transition, "expected workspace to be running")
}

func TestExecutorWorkspaceAutostopBeforeDeadline(t *testing.T) {
	t.Parallel()

	var (
		tickCh = make(chan time.Time)
		client = coderdtest.New(t, &coderdtest.Options{
			AutobuildTicker:     tickCh,
			IncludeProvisionerD: true,
		})
		// Given: we have a user with a workspace
		workspace = mustProvisionWorkspace(t, client)
	)

	// When: the autobuild executor ticks before the TTL
	go func() {
		tickCh <- workspace.LatestBuild.Deadline.Add(-1 * time.Minute)
		close(tickCh)
	}()

	// Then: nothing should happen
	<-time.After(5 * time.Second)
	ws := mustWorkspace(t, client, workspace.ID)
	require.Equal(t, workspace.LatestBuild.ID, ws.LatestBuild.ID, "expected no further workspace builds to occur")
	require.Equal(t, codersdk.WorkspaceTransitionStart, ws.LatestBuild.Transition, "expected workspace to be running")
}

func TestExecutorWorkspaceAutostopNoWaitChangedMyMind(t *testing.T) {
	t.Parallel()

	var (
		ctx    = context.Background()
		tickCh = make(chan time.Time)
		client = coderdtest.New(t, &coderdtest.Options{
			AutobuildTicker:     tickCh,
			IncludeProvisionerD: true,
		})
		// Given: we have a user with a workspace
		workspace = mustProvisionWorkspace(t, client)
	)

	// Given: the user changes their mind and decides their workspace should not auto-stop
	err := client.UpdateWorkspaceTTL(ctx, workspace.ID, codersdk.UpdateWorkspaceTTLRequest{TTL: nil})
	require.NoError(t, err)

	// When: the autobuild executor ticks after the deadline
	go func() {
		tickCh <- workspace.LatestBuild.Deadline.Add(time.Minute)
		close(tickCh)
	}()

	// Then: the workspace should still stop - sorry!
	<-time.After(5 * time.Second)
	ws := mustWorkspace(t, client, workspace.ID)
	require.NotEqual(t, workspace.LatestBuild.ID, ws.LatestBuild.ID, "expected a workspace build to occur")
	require.Equal(t, codersdk.ProvisionerJobSucceeded, ws.LatestBuild.Job.Status, "expected provisioner job to have succeeded")
	require.Equal(t, codersdk.WorkspaceTransitionStop, ws.LatestBuild.Transition, "expected workspace not to be running")
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
			AutobuildTicker:     tickCh,
			IncludeProvisionerD: true,
		})
		_ = coderdtest.New(t, &coderdtest.Options{
			AutobuildTicker:     tickCh2,
			IncludeProvisionerD: true,
		})
		// Given: we have a user with a workspace that has autostart enabled (default)
		workspace = mustProvisionWorkspace(t, client)
	)
	// Given: workspace is stopped
	workspace = mustTransitionWorkspace(t, client, workspace.ID, database.WorkspaceTransitionStart, database.WorkspaceTransitionStop)

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
	require.Equal(t, codersdk.WorkspaceTransitionStart, ws.LatestBuild.Transition, "expected latest transition to be start")
	builds, err := client.WorkspaceBuilds(ctx, codersdk.WorkspaceBuildsRequest{WorkspaceID: ws.ID})
	require.NoError(t, err, "fetch list of workspace builds from primary")
	// One build to start, one stop transition, and one autostart. No more.
	require.Equal(t, codersdk.WorkspaceTransitionStart, builds[0].Transition)
	require.Equal(t, codersdk.WorkspaceTransitionStop, builds[1].Transition)
	require.Equal(t, codersdk.WorkspaceTransitionStart, builds[2].Transition)
	require.Len(t, builds, 3, "unexpected number of builds for workspace from primary")

	// Builds are returned most recent first.
	require.True(t, builds[0].CreatedAt.After(builds[1].CreatedAt))
	require.True(t, builds[1].CreatedAt.After(builds[2].CreatedAt))
}

func mustProvisionWorkspace(t *testing.T, client *codersdk.Client, mut ...func(*codersdk.CreateWorkspaceRequest)) codersdk.Workspace {
	t.Helper()
	user := coderdtest.CreateFirstUser(t, client)
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
	ws := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID, mut...)
	coderdtest.AwaitWorkspaceBuildJob(t, client, ws.LatestBuild.ID)
	return mustWorkspace(t, client, ws.ID)
}

func mustTransitionWorkspace(t *testing.T, client *codersdk.Client, workspaceID uuid.UUID, from, to database.WorkspaceTransition) codersdk.Workspace {
	t.Helper()
	ctx := context.Background()
	workspace, err := client.Workspace(ctx, workspaceID)
	require.NoError(t, err, "unexpected error fetching workspace")
	require.Equal(t, workspace.LatestBuild.Transition, codersdk.WorkspaceTransition(from), "expected workspace state: %s got: %s", from, workspace.LatestBuild.Transition)

	template, err := client.Template(ctx, workspace.TemplateID)
	require.NoError(t, err, "fetch workspace template")

	build, err := client.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
		TemplateVersionID: template.ActiveVersionID,
		Transition:        codersdk.WorkspaceTransition(to),
	})
	require.NoError(t, err, "unexpected error transitioning workspace to %s", to)

	_ = coderdtest.AwaitWorkspaceBuildJob(t, client, build.ID)

	updated := mustWorkspace(t, client, workspace.ID)
	require.Equal(t, codersdk.WorkspaceTransition(to), updated.LatestBuild.Transition, "expected workspace to be in state %s but got %s", to, updated.LatestBuild.Transition)
	return updated
}

func mustWorkspace(t *testing.T, client *codersdk.Client, workspaceID uuid.UUID) codersdk.Workspace {
	ctx := context.Background()
	ws, err := client.Workspace(ctx, workspaceID)
	if err != nil && strings.Contains(err.Error(), "status code 410") {
		ws, err = client.DeletedWorkspace(ctx, workspaceID)
	}
	require.NoError(t, err, "no workspace found with id %s", workspaceID)
	return ws
}

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

package executor_test

import (
	"context"
	"os"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/google/uuid"

	"github.com/coder/coder/coderd/autobuild/executor"
	"github.com/coder/coder/coderd/autobuild/schedule"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/util/ptr"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecutorAutostartOK(t *testing.T) {
	t.Parallel()

	var (
		sched   = mustSchedule(t, "CRON_TZ=UTC 0 * * * *")
		tickCh  = make(chan time.Time)
		statsCh = make(chan executor.Stats)
		client  = coderdtest.New(t, &coderdtest.Options{
			AutobuildTicker:          tickCh,
			IncludeProvisionerDaemon: true,
			AutobuildStats:           statsCh,
		})
		// Given: we have a user with a workspace that has autostart enabled
		workspace = mustProvisionWorkspace(t, client, func(cwr *codersdk.CreateWorkspaceRequest) {
			cwr.AutostartSchedule = ptr.Ref(sched.String())
		})
	)
	// Given: workspace is stopped
	workspace = coderdtest.MustTransitionWorkspace(t, client, workspace.ID, database.WorkspaceTransitionStart, database.WorkspaceTransitionStop)

	// When: the autobuild executor ticks after the scheduled time
	go func() {
		tickCh <- sched.Next(workspace.LatestBuild.CreatedAt)
		close(tickCh)
	}()

	// Then: the workspace should eventually be started
	stats := <-statsCh
	assert.NoError(t, stats.Error)
	assert.Len(t, stats.Transitions, 1)
	assert.Contains(t, stats.Transitions, workspace.ID)
	assert.Equal(t, database.WorkspaceTransitionStart, stats.Transitions[workspace.ID])

	workspace = coderdtest.MustWorkspace(t, client, workspace.ID)
	assert.Equal(t, codersdk.BuildReasonAutostart, workspace.LatestBuild.Reason)
}

func TestExecutorAutostartTemplateUpdated(t *testing.T) {
	t.Parallel()

	var (
		sched   = mustSchedule(t, "CRON_TZ=UTC 0 * * * *")
		ctx     = context.Background()
		err     error
		tickCh  = make(chan time.Time)
		statsCh = make(chan executor.Stats)
		client  = coderdtest.New(t, &coderdtest.Options{
			AutobuildTicker:          tickCh,
			IncludeProvisionerDaemon: true,
			AutobuildStats:           statsCh,
		})
		// Given: we have a user with a workspace that has autostart enabled
		workspace = mustProvisionWorkspace(t, client, func(cwr *codersdk.CreateWorkspaceRequest) {
			cwr.AutostartSchedule = ptr.Ref(sched.String())
		})
	)
	// Given: workspace is stopped
	workspace = coderdtest.MustTransitionWorkspace(t, client, workspace.ID, database.WorkspaceTransitionStart, database.WorkspaceTransitionStop)

	// Given: the workspace template has been updated
	orgs, err := client.OrganizationsByUser(ctx, workspace.OwnerID.String())
	require.NoError(t, err)
	require.Len(t, orgs, 1)

	newVersion := coderdtest.UpdateTemplateVersion(t, client, orgs[0].ID, nil, workspace.TemplateID)
	coderdtest.AwaitTemplateVersionJob(t, client, newVersion.ID)
	require.NoError(t, client.UpdateActiveTemplateVersion(ctx, workspace.TemplateID, codersdk.UpdateActiveTemplateVersion{
		ID: newVersion.ID,
	}))

	// When: the autobuild executor ticks after the scheduled time
	go func() {
		tickCh <- sched.Next(workspace.LatestBuild.CreatedAt)
		close(tickCh)
	}()

	// Then: the workspace is started using the new template version, not the old one.
	stats := <-statsCh
	assert.NoError(t, stats.Error)
	assert.Len(t, stats.Transitions, 1)
	assert.Contains(t, stats.Transitions, workspace.ID)
	assert.Equal(t, database.WorkspaceTransitionStart, stats.Transitions[workspace.ID])
	ws := coderdtest.MustWorkspace(t, client, workspace.ID)
	assert.Equal(t, newVersion.ID, ws.LatestBuild.TemplateVersionID, "expected workspace build to be using the new template version")
}

func TestExecutorAutostartAlreadyRunning(t *testing.T) {
	t.Parallel()

	var (
		sched   = mustSchedule(t, "CRON_TZ=UTC 0 * * * *")
		tickCh  = make(chan time.Time)
		statsCh = make(chan executor.Stats)
		client  = coderdtest.New(t, &coderdtest.Options{
			AutobuildTicker:          tickCh,
			IncludeProvisionerDaemon: true,
			AutobuildStats:           statsCh,
		})
		// Given: we have a user with a workspace that has autostart enabled
		workspace = mustProvisionWorkspace(t, client, func(cwr *codersdk.CreateWorkspaceRequest) {
			cwr.AutostartSchedule = ptr.Ref(sched.String())
		})
	)

	// Given: we ensure the workspace is running
	require.Equal(t, codersdk.WorkspaceTransitionStart, workspace.LatestBuild.Transition)

	// When: the autobuild executor ticks
	go func() {
		tickCh <- sched.Next(workspace.LatestBuild.CreatedAt)
		close(tickCh)
	}()

	// Then: the workspace should not be started.
	stats := <-statsCh
	require.NoError(t, stats.Error)
	require.Len(t, stats.Transitions, 0)
}

func TestExecutorAutostartNotEnabled(t *testing.T) {
	t.Parallel()

	var (
		tickCh  = make(chan time.Time)
		statsCh = make(chan executor.Stats)
		client  = coderdtest.New(t, &coderdtest.Options{
			AutobuildTicker:          tickCh,
			IncludeProvisionerDaemon: true,
			AutobuildStats:           statsCh,
		})
		// Given: we have a user with a workspace that does not have autostart enabled
		workspace = mustProvisionWorkspace(t, client, func(cwr *codersdk.CreateWorkspaceRequest) {
			cwr.AutostartSchedule = nil
		})
	)

	// Given: workspace does not have autostart enabled
	require.Empty(t, workspace.AutostartSchedule)

	// Given: workspace is stopped
	workspace = coderdtest.MustTransitionWorkspace(t, client, workspace.ID, database.WorkspaceTransitionStart, database.WorkspaceTransitionStop)

	// When: the autobuild executor ticks way into the future
	go func() {
		tickCh <- workspace.LatestBuild.CreatedAt.Add(24 * time.Hour)
		close(tickCh)
	}()

	// Then: the workspace should not be started.
	stats := <-statsCh
	require.NoError(t, stats.Error)
	require.Len(t, stats.Transitions, 0)
}

func TestExecutorAutostopOK(t *testing.T) {
	t.Parallel()

	var (
		tickCh  = make(chan time.Time)
		statsCh = make(chan executor.Stats)
		client  = coderdtest.New(t, &coderdtest.Options{
			AutobuildTicker:          tickCh,
			IncludeProvisionerDaemon: true,
			AutobuildStats:           statsCh,
		})
		// Given: we have a user with a workspace
		workspace = mustProvisionWorkspace(t, client)
	)
	// Given: workspace is running
	require.Equal(t, codersdk.WorkspaceTransitionStart, workspace.LatestBuild.Transition)
	require.NotZero(t, workspace.LatestBuild.Deadline)

	// When: the autobuild executor ticks *after* the deadline:
	go func() {
		tickCh <- workspace.LatestBuild.Deadline.Time.Add(time.Minute)
		close(tickCh)
	}()

	// Then: the workspace should be stopped
	stats := <-statsCh
	assert.NoError(t, stats.Error)
	assert.Len(t, stats.Transitions, 1)
	assert.Contains(t, stats.Transitions, workspace.ID)
	assert.Equal(t, database.WorkspaceTransitionStop, stats.Transitions[workspace.ID])

	workspace = coderdtest.MustWorkspace(t, client, workspace.ID)
	assert.Equal(t, codersdk.BuildReasonAutostop, workspace.LatestBuild.Reason)
}

func TestExecutorAutostopExtend(t *testing.T) {
	t.Parallel()

	var (
		ctx     = context.Background()
		tickCh  = make(chan time.Time)
		statsCh = make(chan executor.Stats)
		client  = coderdtest.New(t, &coderdtest.Options{
			AutobuildTicker:          tickCh,
			IncludeProvisionerDaemon: true,
			AutobuildStats:           statsCh,
		})
		// Given: we have a user with a workspace
		workspace        = mustProvisionWorkspace(t, client)
		originalDeadline = workspace.LatestBuild.Deadline
	)
	// Given: workspace is running
	require.Equal(t, codersdk.WorkspaceTransitionStart, workspace.LatestBuild.Transition)
	require.NotZero(t, originalDeadline)

	// Given: we extend the workspace deadline
	newDeadline := originalDeadline.Time.Add(30 * time.Minute)
	err := client.PutExtendWorkspace(ctx, workspace.ID, codersdk.PutExtendWorkspaceRequest{
		Deadline: newDeadline,
	})
	require.NoError(t, err, "extend workspace deadline")

	// When: the autobuild executor ticks *after* the original deadline:
	go func() {
		tickCh <- originalDeadline.Time.Add(time.Minute)
	}()

	// Then: nothing should happen and the workspace should stay running
	stats := <-statsCh
	assert.NoError(t, stats.Error)
	assert.Len(t, stats.Transitions, 0)

	// When: the autobuild executor ticks after the *new* deadline:
	go func() {
		tickCh <- newDeadline.Add(time.Minute)
		close(tickCh)
	}()

	// Then: the workspace should be stopped
	stats = <-statsCh
	assert.NoError(t, stats.Error)
	assert.Len(t, stats.Transitions, 1)
	assert.Contains(t, stats.Transitions, workspace.ID)
	assert.Equal(t, database.WorkspaceTransitionStop, stats.Transitions[workspace.ID])
}

func TestExecutorAutostopAlreadyStopped(t *testing.T) {
	t.Parallel()

	var (
		tickCh  = make(chan time.Time)
		statsCh = make(chan executor.Stats)
		client  = coderdtest.New(t, &coderdtest.Options{
			AutobuildTicker:          tickCh,
			IncludeProvisionerDaemon: true,
			AutobuildStats:           statsCh,
		})
		// Given: we have a user with a workspace (disabling autostart)
		workspace = mustProvisionWorkspace(t, client, func(cwr *codersdk.CreateWorkspaceRequest) {
			cwr.AutostartSchedule = nil
		})
	)

	// Given: workspace is stopped
	workspace = coderdtest.MustTransitionWorkspace(t, client, workspace.ID, database.WorkspaceTransitionStart, database.WorkspaceTransitionStop)

	// When: the autobuild executor ticks past the TTL
	go func() {
		tickCh <- workspace.LatestBuild.Deadline.Time.Add(time.Minute)
		close(tickCh)
	}()

	// Then: the workspace should remain stopped and no build should happen.
	stats := <-statsCh
	assert.NoError(t, stats.Error)
	assert.Len(t, stats.Transitions, 0)
}

func TestExecutorAutostopNotEnabled(t *testing.T) {
	t.Parallel()

	var (
		ctx     = context.Background()
		tickCh  = make(chan time.Time)
		statsCh = make(chan executor.Stats)
		client  = coderdtest.New(t, &coderdtest.Options{
			AutobuildTicker:          tickCh,
			IncludeProvisionerDaemon: true,
			AutobuildStats:           statsCh,
		})
		// Given: we have a user with a workspace
		workspace = mustProvisionWorkspace(t, client)
	)

	// Given: workspace has no TTL set
	err := client.UpdateWorkspaceTTL(ctx, workspace.ID, codersdk.UpdateWorkspaceTTLRequest{TTLMillis: nil})
	require.NoError(t, err)
	workspace, err = client.Workspace(ctx, workspace.ID)
	require.NoError(t, err)
	require.Nil(t, workspace.TTLMillis)

	// TODO(cian): need to stop and start the workspace as we do not update the deadline. See: #2229
	coderdtest.MustTransitionWorkspace(t, client, workspace.ID, database.WorkspaceTransitionStart, database.WorkspaceTransitionStop)
	coderdtest.MustTransitionWorkspace(t, client, workspace.ID, database.WorkspaceTransitionStop, database.WorkspaceTransitionStart)

	// Given: workspace is running
	require.Equal(t, codersdk.WorkspaceTransitionStart, workspace.LatestBuild.Transition)

	// When: the autobuild executor ticks past the TTL
	go func() {
		tickCh <- workspace.LatestBuild.Deadline.Time.Add(time.Minute)
		close(tickCh)
	}()

	// Then: the workspace should not be stopped.
	stats := <-statsCh
	assert.NoError(t, stats.Error)
	assert.Len(t, stats.Transitions, 0)
}

func TestExecutorWorkspaceDeleted(t *testing.T) {
	t.Parallel()

	var (
		sched   = mustSchedule(t, "CRON_TZ=UTC 0 * * * *")
		tickCh  = make(chan time.Time)
		statsCh = make(chan executor.Stats)
		client  = coderdtest.New(t, &coderdtest.Options{
			AutobuildTicker:          tickCh,
			IncludeProvisionerDaemon: true,
			AutobuildStats:           statsCh,
		})
		// Given: we have a user with a workspace that has autostart enabled
		workspace = mustProvisionWorkspace(t, client, func(cwr *codersdk.CreateWorkspaceRequest) {
			cwr.AutostartSchedule = ptr.Ref(sched.String())
		})
	)

	// Given: workspace is deleted
	workspace = coderdtest.MustTransitionWorkspace(t, client, workspace.ID, database.WorkspaceTransitionStart, database.WorkspaceTransitionDelete)

	// When: the autobuild executor ticks
	go func() {
		tickCh <- sched.Next(workspace.LatestBuild.CreatedAt)
		close(tickCh)
	}()

	// Then: nothing should happen
	stats := <-statsCh
	assert.NoError(t, stats.Error)
	assert.Len(t, stats.Transitions, 0)
}

func TestExecutorWorkspaceAutostartTooEarly(t *testing.T) {
	t.Parallel()

	var (
		sched   = mustSchedule(t, "CRON_TZ=UTC 0 * * * *")
		tickCh  = make(chan time.Time)
		statsCh = make(chan executor.Stats)
		client  = coderdtest.New(t, &coderdtest.Options{
			AutobuildTicker:          tickCh,
			IncludeProvisionerDaemon: true,
			AutobuildStats:           statsCh,
		})
		// futureTime     = time.Now().Add(time.Hour)
		// futureTimeCron = fmt.Sprintf("%d %d * * *", futureTime.Minute(), futureTime.Hour())
		// Given: we have a user with a workspace configured to autostart some time in the future
		workspace = mustProvisionWorkspace(t, client, func(cwr *codersdk.CreateWorkspaceRequest) {
			cwr.AutostartSchedule = ptr.Ref(sched.String())
		})
	)

	// When: the autobuild executor ticks before the next scheduled time
	go func() {
		tickCh <- sched.Next(workspace.LatestBuild.CreatedAt).Add(-time.Minute)
		close(tickCh)
	}()

	// Then: nothing should happen
	stats := <-statsCh
	assert.NoError(t, stats.Error)
	assert.Len(t, stats.Transitions, 0)
}

func TestExecutorWorkspaceAutostopBeforeDeadline(t *testing.T) {
	t.Parallel()

	var (
		tickCh  = make(chan time.Time)
		statsCh = make(chan executor.Stats)
		client  = coderdtest.New(t, &coderdtest.Options{
			AutobuildTicker:          tickCh,
			IncludeProvisionerDaemon: true,
			AutobuildStats:           statsCh,
		})
		// Given: we have a user with a workspace
		workspace = mustProvisionWorkspace(t, client)
	)

	// When: the autobuild executor ticks before the TTL
	go func() {
		tickCh <- workspace.LatestBuild.Deadline.Time.Add(-1 * time.Minute)
		close(tickCh)
	}()

	// Then: nothing should happen
	stats := <-statsCh
	assert.NoError(t, stats.Error)
	assert.Len(t, stats.Transitions, 0)
}

func TestExecutorWorkspaceAutostopNoWaitChangedMyMind(t *testing.T) {
	t.Parallel()

	var (
		ctx     = context.Background()
		tickCh  = make(chan time.Time)
		statsCh = make(chan executor.Stats)
		client  = coderdtest.New(t, &coderdtest.Options{
			AutobuildTicker:          tickCh,
			IncludeProvisionerDaemon: true,
			AutobuildStats:           statsCh,
		})
		// Given: we have a user with a workspace
		workspace = mustProvisionWorkspace(t, client)
	)

	// Given: the user changes their mind and decides their workspace should not auto-stop
	err := client.UpdateWorkspaceTTL(ctx, workspace.ID, codersdk.UpdateWorkspaceTTLRequest{TTLMillis: nil})
	require.NoError(t, err)

	// Then: the deadline should still be the original value
	updated := coderdtest.MustWorkspace(t, client, workspace.ID)
	assert.WithinDuration(t, workspace.LatestBuild.Deadline.Time, updated.LatestBuild.Deadline.Time, time.Minute)

	// When: the autobuild executor ticks after the original deadline
	go func() {
		tickCh <- workspace.LatestBuild.Deadline.Time.Add(time.Minute)
	}()

	// Then: the workspace should stop
	stats := <-statsCh
	assert.NoError(t, stats.Error)
	assert.Len(t, stats.Transitions, 1)
	assert.Equal(t, stats.Transitions[workspace.ID], database.WorkspaceTransitionStop)

	// Wait for stop to complete
	updated = coderdtest.MustWorkspace(t, client, workspace.ID)
	_ = coderdtest.AwaitWorkspaceBuildJob(t, client, updated.LatestBuild.ID)

	// Start the workspace again
	workspace = coderdtest.MustTransitionWorkspace(t, client, workspace.ID, database.WorkspaceTransitionStop, database.WorkspaceTransitionStart)

	// Given: the user changes their mind again and wants to enable auto-stop
	newTTL := 8 * time.Hour
	err = client.UpdateWorkspaceTTL(ctx, workspace.ID, codersdk.UpdateWorkspaceTTLRequest{TTLMillis: ptr.Ref(newTTL.Milliseconds())})
	require.NoError(t, err)

	// Then: the deadline should remain at the zero value
	updated = coderdtest.MustWorkspace(t, client, workspace.ID)
	assert.Zero(t, updated.LatestBuild.Deadline)

	// When: the relentless onward march of time continues
	go func() {
		tickCh <- workspace.LatestBuild.Deadline.Time.Add(newTTL + time.Minute)
		close(tickCh)
	}()

	// Then: the workspace should not stop
	stats = <-statsCh
	assert.NoError(t, stats.Error)
	assert.Len(t, stats.Transitions, 0)
}

func TestExecutorAutostartMultipleOK(t *testing.T) {
	if os.Getenv("DB") == "" {
		t.Skip(`This test only really works when using a "real" database, similar to a HA setup`)
	}

	t.Parallel()

	var (
		sched    = mustSchedule(t, "CRON_TZ=UTC 0 * * * *")
		tickCh   = make(chan time.Time)
		tickCh2  = make(chan time.Time)
		statsCh1 = make(chan executor.Stats)
		statsCh2 = make(chan executor.Stats)
		client   = coderdtest.New(t, &coderdtest.Options{
			AutobuildTicker:          tickCh,
			IncludeProvisionerDaemon: true,
			AutobuildStats:           statsCh1,
		})
		_ = coderdtest.New(t, &coderdtest.Options{
			AutobuildTicker:          tickCh2,
			IncludeProvisionerDaemon: true,
			AutobuildStats:           statsCh2,
		})
		// Given: we have a user with a workspace that has autostart enabled (default)
		workspace = mustProvisionWorkspace(t, client, func(cwr *codersdk.CreateWorkspaceRequest) {
			cwr.AutostartSchedule = ptr.Ref(sched.String())
		})
	)
	// Given: workspace is stopped
	workspace = coderdtest.MustTransitionWorkspace(t, client, workspace.ID, database.WorkspaceTransitionStart, database.WorkspaceTransitionStop)

	// When: the autobuild executor ticks past the scheduled time
	go func() {
		tickCh <- sched.Next(workspace.LatestBuild.CreatedAt)
		tickCh2 <- sched.Next(workspace.LatestBuild.CreatedAt)
		close(tickCh)
		close(tickCh2)
	}()

	// Then: the workspace should eventually be started
	stats1 := <-statsCh1
	assert.NoError(t, stats1.Error)
	assert.Len(t, stats1.Transitions, 1)
	assert.Contains(t, stats1.Transitions, workspace.ID)
	assert.Equal(t, database.WorkspaceTransitionStart, stats1.Transitions[workspace.ID])

	// Then: the other executor should not have done anything
	stats2 := <-statsCh2
	assert.NoError(t, stats2.Error)
	assert.Len(t, stats2.Transitions, 0)
}

func TestExecutorAutostartWithParameters(t *testing.T) {
	t.Parallel()

	const (
		stringParameterName  = "string_parameter"
		stringParameterValue = "abc"

		numberParameterName  = "number_parameter"
		numberParameterValue = "7"
	)

	var (
		sched   = mustSchedule(t, "CRON_TZ=UTC 0 * * * *")
		tickCh  = make(chan time.Time)
		statsCh = make(chan executor.Stats)
		client  = coderdtest.New(t, &coderdtest.Options{
			AutobuildTicker:          tickCh,
			IncludeProvisionerDaemon: true,
			AutobuildStats:           statsCh,
		})

		richParameters = []*proto.RichParameter{
			{Name: stringParameterName, Type: "string", Mutable: true},
			{Name: numberParameterName, Type: "number", Mutable: true},
		}

		// Given: we have a user with a workspace that has autostart enabled
		workspace = mustProvisionWorkspaceWithParameters(t, client, richParameters, func(cwr *codersdk.CreateWorkspaceRequest) {
			cwr.AutostartSchedule = ptr.Ref(sched.String())
			cwr.RichParameterValues = []codersdk.WorkspaceBuildParameter{
				{
					Name:  stringParameterName,
					Value: stringParameterValue,
				},
				{
					Name:  numberParameterName,
					Value: numberParameterValue,
				},
			}
		})
	)
	// Given: workspace is stopped
	workspace = coderdtest.MustTransitionWorkspace(t, client, workspace.ID, database.WorkspaceTransitionStart, database.WorkspaceTransitionStop)

	// When: the autobuild executor ticks after the scheduled time
	go func() {
		tickCh <- sched.Next(workspace.LatestBuild.CreatedAt)
		close(tickCh)
	}()

	// Then: the workspace with parameters should eventually be started
	stats := <-statsCh
	assert.NoError(t, stats.Error)
	assert.Len(t, stats.Transitions, 1)
	assert.Contains(t, stats.Transitions, workspace.ID)
	assert.Equal(t, database.WorkspaceTransitionStart, stats.Transitions[workspace.ID])

	workspace = coderdtest.MustWorkspace(t, client, workspace.ID)
	mustWorkspaceParameters(t, client, workspace.LatestBuild.ID)
}

func mustProvisionWorkspace(t *testing.T, client *codersdk.Client, mut ...func(*codersdk.CreateWorkspaceRequest)) codersdk.Workspace {
	t.Helper()
	user := coderdtest.CreateFirstUser(t, client)
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
	ws := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID, mut...)
	coderdtest.AwaitWorkspaceBuildJob(t, client, ws.LatestBuild.ID)
	return coderdtest.MustWorkspace(t, client, ws.ID)
}

func mustProvisionWorkspaceWithParameters(t *testing.T, client *codersdk.Client, richParameters []*proto.RichParameter, mut ...func(*codersdk.CreateWorkspaceRequest)) codersdk.Workspace {
	t.Helper()
	user := coderdtest.CreateFirstUser(t, client)
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse: echo.ParseComplete,
		ProvisionPlan: []*proto.Provision_Response{
			{
				Type: &proto.Provision_Response_Complete{
					Complete: &proto.Provision_Complete{
						Parameters: richParameters,
					},
				},
			},
		},
		ProvisionApply: []*proto.Provision_Response{
			{
				Type: &proto.Provision_Response_Complete{
					Complete: &proto.Provision_Complete{},
				},
			},
		},
	})
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
	ws := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID, mut...)
	coderdtest.AwaitWorkspaceBuildJob(t, client, ws.LatestBuild.ID)
	return coderdtest.MustWorkspace(t, client, ws.ID)
}

func mustSchedule(t *testing.T, s string) *schedule.Schedule {
	t.Helper()
	sched, err := schedule.Weekly(s)
	require.NoError(t, err)
	return sched
}

func mustWorkspaceParameters(t *testing.T, client *codersdk.Client, workspaceID uuid.UUID) {
	ctx := context.Background()
	buildParameters, err := client.WorkspaceBuildParameters(ctx, workspaceID)
	require.NoError(t, err)
	require.NotEmpty(t, buildParameters)
}

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

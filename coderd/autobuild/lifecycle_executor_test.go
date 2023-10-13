package autobuild_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/v2/coderd/autobuild"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/schedule"
	"github.com/coder/coder/v2/coderd/schedule/cron"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk/proto"
)

func TestExecutorAutostartOK(t *testing.T) {
	t.Parallel()

	var (
		sched   = mustSchedule(t, "CRON_TZ=UTC 0 * * * *")
		tickCh  = make(chan time.Time)
		statsCh = make(chan autobuild.Stats)
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

	testCases := []struct {
		name                 string
		automaticUpdates     codersdk.AutomaticUpdates
		compatibleParameters bool
		expectStart          bool
		expectUpdate         bool
	}{
		{
			name:                 "Never",
			automaticUpdates:     codersdk.AutomaticUpdatesNever,
			compatibleParameters: true,
			expectStart:          true,
			expectUpdate:         false,
		},
		{
			name:                 "Always_Compatible",
			automaticUpdates:     codersdk.AutomaticUpdatesAlways,
			compatibleParameters: true,
			expectStart:          true,
			expectUpdate:         true,
		},
		{
			name:                 "Always_Incompatible",
			automaticUpdates:     codersdk.AutomaticUpdatesAlways,
			compatibleParameters: false,
			expectStart:          false,
			expectUpdate:         false,
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var (
				sched   = mustSchedule(t, "CRON_TZ=UTC 0 * * * *")
				ctx     = context.Background()
				err     error
				tickCh  = make(chan time.Time)
				statsCh = make(chan autobuild.Stats)
				logger  = slogtest.Make(t, &slogtest.Options{IgnoreErrors: !tc.expectStart}).Leveled(slog.LevelDebug)
				client  = coderdtest.New(t, &coderdtest.Options{
					AutobuildTicker:          tickCh,
					IncludeProvisionerDaemon: true,
					AutobuildStats:           statsCh,
					Logger:                   &logger,
				})
				// Given: we have a user with a workspace that has autostart enabled
				workspace = mustProvisionWorkspace(t, client, func(cwr *codersdk.CreateWorkspaceRequest) {
					cwr.AutostartSchedule = ptr.Ref(sched.String())
					// Given: automatic updates from the test case
					cwr.AutomaticUpdates = tc.automaticUpdates
				})
			)
			// Given: workspace is stopped
			workspace = coderdtest.MustTransitionWorkspace(
				t, client, workspace.ID, database.WorkspaceTransitionStart, database.WorkspaceTransitionStop)

			orgs, err := client.OrganizationsByUser(ctx, workspace.OwnerID.String())
			require.NoError(t, err)
			require.Len(t, orgs, 1)

			var res *echo.Responses
			if !tc.compatibleParameters {
				// Given, parameters of the new version are not compatible.
				// Since initial version has no parameters, any parameters in the new version will be incompatible
				res = &echo.Responses{
					Parse: echo.ParseComplete,
					ProvisionApply: []*proto.Response{{
						Type: &proto.Response_Apply{
							Apply: &proto.ApplyComplete{
								Parameters: []*proto.RichParameter{
									{
										Name:     "new",
										Mutable:  false,
										Required: true,
									},
								},
							},
						},
					}},
				}
			}

			// Given: the workspace template has been updated
			newVersion := coderdtest.UpdateTemplateVersion(t, client, orgs[0].ID, res, workspace.TemplateID)
			coderdtest.AwaitTemplateVersionJobCompleted(t, client, newVersion.ID)
			require.NoError(t, client.UpdateActiveTemplateVersion(
				ctx, workspace.TemplateID, codersdk.UpdateActiveTemplateVersion{
					ID: newVersion.ID,
				},
			))

			t.Log("sending autobuild tick")
			// When: the autobuild executor ticks after the scheduled time
			go func() {
				tickCh <- sched.Next(workspace.LatestBuild.CreatedAt)
				close(tickCh)
			}()

			stats := <-statsCh
			assert.NoError(t, stats.Error)
			if !tc.expectStart {
				// Then: the workspace should not be started
				assert.Len(t, stats.Transitions, 0)
				return
			}

			// Then: the workspace should be started
			assert.Len(t, stats.Transitions, 1)
			assert.Contains(t, stats.Transitions, workspace.ID)
			assert.Equal(t, database.WorkspaceTransitionStart, stats.Transitions[workspace.ID])
			ws := coderdtest.MustWorkspace(t, client, workspace.ID)
			if tc.expectUpdate {
				// Then: uses the updated version
				assert.Equal(t, newVersion.ID, ws.LatestBuild.TemplateVersionID,
					"expected workspace build to be using the updated template version")
			} else {
				// Then: uses the previous template version
				assert.Equal(t, workspace.LatestBuild.TemplateVersionID, ws.LatestBuild.TemplateVersionID,
					"expected workspace build to be using the old template version")
			}
		})
	}
}

func TestExecutorAutostartAlreadyRunning(t *testing.T) {
	t.Parallel()

	var (
		sched   = mustSchedule(t, "CRON_TZ=UTC 0 * * * *")
		tickCh  = make(chan time.Time)
		statsCh = make(chan autobuild.Stats)
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
		statsCh = make(chan autobuild.Stats)
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
		statsCh = make(chan autobuild.Stats)
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
		statsCh = make(chan autobuild.Stats)
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
		statsCh = make(chan autobuild.Stats)
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
		statsCh = make(chan autobuild.Stats)
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
		statsCh = make(chan autobuild.Stats)
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
		statsCh = make(chan autobuild.Stats)
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
		statsCh = make(chan autobuild.Stats)
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
		statsCh = make(chan autobuild.Stats)
		client  = coderdtest.New(t, &coderdtest.Options{
			AutobuildTicker:          tickCh,
			IncludeProvisionerDaemon: true,
			AutobuildStats:           statsCh,
		})
		// Given: we have a user with a workspace
		workspace = mustProvisionWorkspace(t, client)
	)

	// Given: the user changes their mind and decides their workspace should not autostop
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
	_ = coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, updated.LatestBuild.ID)

	// Start the workspace again
	workspace = coderdtest.MustTransitionWorkspace(t, client, workspace.ID, database.WorkspaceTransitionStop, database.WorkspaceTransitionStart)

	// Given: the user changes their mind again and wants to enable autostop
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
		statsCh1 = make(chan autobuild.Stats)
		statsCh2 = make(chan autobuild.Stats)
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
		statsCh = make(chan autobuild.Stats)
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

func TestExecutorAutostartTemplateDisabled(t *testing.T) {
	t.Parallel()

	var (
		sched   = mustSchedule(t, "CRON_TZ=UTC 0 * * * *")
		tickCh  = make(chan time.Time)
		statsCh = make(chan autobuild.Stats)

		client = coderdtest.New(t, &coderdtest.Options{
			AutobuildTicker:          tickCh,
			IncludeProvisionerDaemon: true,
			AutobuildStats:           statsCh,
			TemplateScheduleStore: schedule.MockTemplateScheduleStore{
				GetFn: func(_ context.Context, _ database.Store, _ uuid.UUID) (schedule.TemplateScheduleOptions, error) {
					return schedule.TemplateScheduleOptions{
						UserAutostartEnabled: false,
						UserAutostopEnabled:  true,
						DefaultTTL:           0,
						AutostopRequirement:  schedule.TemplateAutostopRequirement{},
					}, nil
				},
			},
		})
		// futureTime     = time.Now().Add(time.Hour)
		// futureTimeCron = fmt.Sprintf("%d %d * * *", futureTime.Minute(), futureTime.Hour())
		// Given: we have a user with a workspace configured to autostart some time in the future
		workspace = mustProvisionWorkspace(t, client, func(cwr *codersdk.CreateWorkspaceRequest) {
			cwr.AutostartSchedule = ptr.Ref(sched.String())
		})
	)
	// Given: workspace is stopped
	workspace = coderdtest.MustTransitionWorkspace(t, client, workspace.ID, database.WorkspaceTransitionStart, database.WorkspaceTransitionStop)

	// When: the autobuild executor ticks before the next scheduled time
	go func() {
		tickCh <- sched.Next(workspace.LatestBuild.CreatedAt).Add(time.Minute)
		close(tickCh)
	}()

	// Then: nothing should happen
	stats := <-statsCh
	assert.NoError(t, stats.Error)
	assert.Len(t, stats.Transitions, 0)
}

func TestExecutorAutostopTemplateDisabled(t *testing.T) {
	t.Parallel()

	// Given: we have a workspace built from a template that disallows user autostop
	var (
		sched   = mustSchedule(t, "CRON_TZ=UTC 0 * * * *")
		tickCh  = make(chan time.Time)
		statsCh = make(chan autobuild.Stats)

		client = coderdtest.New(t, &coderdtest.Options{
			AutobuildTicker:          tickCh,
			IncludeProvisionerDaemon: true,
			AutobuildStats:           statsCh,
			// We are using a mock store here as the AGPL store does not implement this.
			TemplateScheduleStore: schedule.MockTemplateScheduleStore{
				GetFn: func(_ context.Context, _ database.Store, _ uuid.UUID) (schedule.TemplateScheduleOptions, error) {
					return schedule.TemplateScheduleOptions{
						UserAutostopEnabled: false,
						DefaultTTL:          time.Hour,
					}, nil
				},
			},
		})
		// Given: we have a user with a workspace configured to autostart some time in the future
		workspace = mustProvisionWorkspace(t, client, func(cwr *codersdk.CreateWorkspaceRequest) {
			cwr.TTLMillis = ptr.Ref(8 * time.Hour.Milliseconds())
		})
	)

	// When: we create the workspace
	// Then: the deadline should be set to the template default TTL
	assert.WithinDuration(t, workspace.LatestBuild.CreatedAt.Add(time.Hour), workspace.LatestBuild.Deadline.Time, time.Minute)

	// When: the autobuild executor ticks before the next scheduled time
	go func() {
		tickCh <- sched.Next(workspace.LatestBuild.CreatedAt).Add(time.Minute)
		close(tickCh)
	}()

	// Then: nothing should happen
	stats := <-statsCh
	assert.NoError(t, stats.Error)
	assert.Len(t, stats.Transitions, 0)
}

// TestExecutorFailedWorkspace test AGPL functionality which mainly
// ensures that autostop actions as a result of a failed workspace
// build do not trigger.
// For enterprise functionality see enterprise/coderd/workspaces_test.go
func TestExecutorFailedWorkspace(t *testing.T) {
	t.Parallel()

	// Test that an AGPL TemplateScheduleStore properly disables
	// functionality.
	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		var (
			ticker = make(chan time.Time)
			statCh = make(chan autobuild.Stats)
			logger = slogtest.Make(t, &slogtest.Options{
				// We ignore errors here since we expect to fail
				// builds.
				IgnoreErrors: true,
			})
			failureTTL = time.Millisecond

			client = coderdtest.New(t, &coderdtest.Options{
				Logger:                   &logger,
				AutobuildTicker:          ticker,
				IncludeProvisionerDaemon: true,
				AutobuildStats:           statCh,
				TemplateScheduleStore:    schedule.NewAGPLTemplateScheduleStore(),
			})
		)
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionPlan:  echo.PlanComplete,
			ProvisionApply: echo.ApplyFailed,
		})
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.FailureTTLMillis = ptr.Ref[int64](failureTTL.Milliseconds())
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		ws := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		build := coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, ws.LatestBuild.ID)
		require.Equal(t, codersdk.WorkspaceStatusFailed, build.Status)
		ticker <- build.Job.CompletedAt.Add(failureTTL * 2)
		stats := <-statCh
		// Expect no transitions since we're using AGPL.
		require.Len(t, stats.Transitions, 0)
	})
}

// TestExecutorInactiveWorkspace test AGPL functionality which mainly
// ensures that autostop actions as a result of an inactive workspace
// do not trigger.
// For enterprise functionality see enterprise/coderd/workspaces_test.go
func TestExecutorInactiveWorkspace(t *testing.T) {
	t.Parallel()

	// Test that an AGPL TemplateScheduleStore properly disables
	// functionality.
	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		var (
			ticker = make(chan time.Time)
			statCh = make(chan autobuild.Stats)
			logger = slogtest.Make(t, &slogtest.Options{
				// We ignore errors here since we expect to fail
				// builds.
				IgnoreErrors: true,
			})
			inactiveTTL = time.Millisecond

			client = coderdtest.New(t, &coderdtest.Options{
				Logger:                   &logger,
				AutobuildTicker:          ticker,
				IncludeProvisionerDaemon: true,
				AutobuildStats:           statCh,
				TemplateScheduleStore:    schedule.NewAGPLTemplateScheduleStore(),
			})
		)
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionPlan:  echo.PlanComplete,
			ProvisionApply: echo.ApplyComplete,
		})
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.TimeTilDormantMillis = ptr.Ref[int64](inactiveTTL.Milliseconds())
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		ws := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		build := coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, ws.LatestBuild.ID)
		require.Equal(t, codersdk.WorkspaceStatusRunning, build.Status)
		ticker <- ws.LastUsedAt.Add(inactiveTTL * 2)
		stats := <-statCh
		// Expect no transitions since we're using AGPL.
		require.Len(t, stats.Transitions, 0)
	})
}

func mustProvisionWorkspace(t *testing.T, client *codersdk.Client, mut ...func(*codersdk.CreateWorkspaceRequest)) codersdk.Workspace {
	t.Helper()
	user := coderdtest.CreateFirstUser(t, client)
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	ws := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID, mut...)
	coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, ws.LatestBuild.ID)
	return coderdtest.MustWorkspace(t, client, ws.ID)
}

func mustProvisionWorkspaceWithParameters(t *testing.T, client *codersdk.Client, richParameters []*proto.RichParameter, mut ...func(*codersdk.CreateWorkspaceRequest)) codersdk.Workspace {
	t.Helper()
	user := coderdtest.CreateFirstUser(t, client)
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse: echo.ParseComplete,
		ProvisionPlan: []*proto.Response{
			{
				Type: &proto.Response_Plan{
					Plan: &proto.PlanComplete{
						Parameters: richParameters,
					},
				},
			},
		},
		ProvisionApply: echo.ApplyComplete,
	})
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	ws := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID, mut...)
	coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, ws.LatestBuild.ID)
	return coderdtest.MustWorkspace(t, client, ws.ID)
}

func mustSchedule(t *testing.T, s string) *cron.Schedule {
	t.Helper()
	sched, err := cron.Weekly(s)
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

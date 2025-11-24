package autobuild_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/provisionerdserver"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/quartz"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/v2/coderd/autobuild"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/coderd/notifications/notificationstest"
	"github.com/coder/coder/v2/coderd/schedule"
	"github.com/coder/coder/v2/coderd/schedule/cron"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m, testutil.GoleakOptions...)
}

func TestExecutorAutostartOK(t *testing.T) {
	t.Parallel()

	var (
		sched      = mustSchedule(t, "CRON_TZ=UTC 0 * * * *")
		tickCh     = make(chan time.Time)
		statsCh    = make(chan autobuild.Stats)
		client, db = coderdtest.NewWithDatabase(t, &coderdtest.Options{
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
	workspace = coderdtest.MustTransitionWorkspace(t, client, workspace.ID, codersdk.WorkspaceTransitionStart, codersdk.WorkspaceTransitionStop)
	p, err := coderdtest.GetProvisionerForTags(db, time.Now(), workspace.OrganizationID, map[string]string{})
	require.NoError(t, err)
	// When: the autobuild executor ticks after the scheduled time
	go func() {
		tickTime := sched.Next(workspace.LatestBuild.CreatedAt)
		coderdtest.UpdateProvisionerLastSeenAt(t, db, p.ID, tickTime)
		tickCh <- tickTime
		close(tickCh)
	}()

	// Then: the workspace should eventually be started
	stats := <-statsCh
	assert.Len(t, stats.Errors, 0)
	assert.Len(t, stats.Transitions, 1)
	assert.Contains(t, stats.Transitions, workspace.ID)
	assert.Equal(t, database.WorkspaceTransitionStart, stats.Transitions[workspace.ID])

	workspace = coderdtest.MustWorkspace(t, client, workspace.ID)
	assert.Equal(t, codersdk.BuildReasonAutostart, workspace.LatestBuild.Reason)
	// Assert some template props. If this is not set correctly, the test
	// will fail.
	ctx := testutil.Context(t, testutil.WaitShort)
	template, err := client.Template(ctx, workspace.TemplateID)
	require.NoError(t, err)
	require.Equal(t, template.AutostartRequirement.DaysOfWeek, []string{"monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday"})
}

func TestMultipleLifecycleExecutors(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)

	var (
		sched = mustSchedule(t, "CRON_TZ=UTC 0 * * * *")
		// Create our first client
		tickCh   = make(chan time.Time, 2)
		statsChA = make(chan autobuild.Stats)
		clientA  = coderdtest.New(t, &coderdtest.Options{
			IncludeProvisionerDaemon: true,
			AutobuildTicker:          tickCh,
			AutobuildStats:           statsChA,
			Database:                 db,
			Pubsub:                   ps,
		})
		// ... And then our second client
		statsChB = make(chan autobuild.Stats)
		_        = coderdtest.New(t, &coderdtest.Options{
			IncludeProvisionerDaemon: true,
			AutobuildTicker:          tickCh,
			AutobuildStats:           statsChB,
			Database:                 db,
			Pubsub:                   ps,
		})
		// Now create a workspace (we can use either client, it doesn't matter)
		workspace = mustProvisionWorkspace(t, clientA, func(cwr *codersdk.CreateWorkspaceRequest) {
			cwr.AutostartSchedule = ptr.Ref(sched.String())
		})
	)

	// Have the workspace stopped so we can perform an autostart
	workspace = coderdtest.MustTransitionWorkspace(t, clientA, workspace.ID, codersdk.WorkspaceTransitionStart, codersdk.WorkspaceTransitionStop)

	p, err := coderdtest.GetProvisionerForTags(db, time.Now(), workspace.OrganizationID, nil)
	require.NoError(t, err)
	// Get both clients to perform a lifecycle execution tick
	next := sched.Next(workspace.LatestBuild.CreatedAt)
	coderdtest.UpdateProvisionerLastSeenAt(t, db, p.ID, next)

	startCh := make(chan struct{})
	go func() {
		<-startCh
		tickCh <- next
	}()
	go func() {
		<-startCh
		tickCh <- next
	}()
	close(startCh)

	// Now we want to check the stats for both clients
	statsA := <-statsChA
	statsB := <-statsChB

	// We expect there to be no errors
	assert.Len(t, statsA.Errors, 0)
	assert.Len(t, statsB.Errors, 0)

	// We also expect there to have been only one transition
	require.Equal(t, 1, len(statsA.Transitions)+len(statsB.Transitions))

	stats := statsA
	if len(statsB.Transitions) == 1 {
		stats = statsB
	}

	// And we expect this transition to have been a start transition
	assert.Contains(t, stats.Transitions, workspace.ID)
	assert.Equal(t, database.WorkspaceTransitionStart, stats.Transitions[workspace.ID])
}

func TestExecutorAutostartTemplateUpdated(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name                 string
		automaticUpdates     codersdk.AutomaticUpdates
		compatibleParameters bool
		expectStart          bool
		expectUpdate         bool
		expectNotification   bool
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
			expectNotification:   true,
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
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var (
				sched      = mustSchedule(t, "CRON_TZ=UTC 0 * * * *")
				ctx        = context.Background()
				err        error
				tickCh     = make(chan time.Time)
				statsCh    = make(chan autobuild.Stats)
				logger     = slogtest.Make(t, &slogtest.Options{IgnoreErrors: !tc.expectStart}).Leveled(slog.LevelDebug)
				enqueuer   = notificationstest.FakeEnqueuer{}
				client, db = coderdtest.NewWithDatabase(t, &coderdtest.Options{
					AutobuildTicker:          tickCh,
					IncludeProvisionerDaemon: true,
					AutobuildStats:           statsCh,
					Logger:                   &logger,
					NotificationsEnqueuer:    &enqueuer,
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
				t, client, workspace.ID, codersdk.WorkspaceTransitionStart, codersdk.WorkspaceTransitionStop)

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

			p, err := coderdtest.GetProvisionerForTags(db, time.Now(), workspace.OrganizationID, nil)
			require.NoError(t, err)

			t.Log("sending autobuild tick")
			// When: the autobuild executor ticks after the scheduled time
			go func() {
				tickTime := sched.Next(workspace.LatestBuild.CreatedAt)
				coderdtest.UpdateProvisionerLastSeenAt(t, db, p.ID, tickTime)
				tickCh <- tickTime
				close(tickCh)
			}()

			stats := <-statsCh
			if !tc.expectStart {
				// Then: the workspace should not be started
				assert.Len(t, stats.Transitions, 0)
				assert.Len(t, stats.Errors, 1)
				return
			}

			assert.Len(t, stats.Errors, 0)
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

			if tc.expectNotification {
				sent := enqueuer.Sent(notificationstest.WithTemplateID(notifications.TemplateWorkspaceAutoUpdated))
				require.Len(t, sent, 1)
				require.Equal(t, sent[0].UserID, workspace.OwnerID)
				require.Contains(t, sent[0].Targets, workspace.TemplateID)
				require.Contains(t, sent[0].Targets, workspace.ID)
				require.Contains(t, sent[0].Targets, workspace.OrganizationID)
				require.Contains(t, sent[0].Targets, workspace.OwnerID)
				require.Equal(t, newVersion.Name, sent[0].Labels["template_version_name"])
				require.Equal(t, "autobuild", sent[0].Labels["initiator"])
				require.Equal(t, "autostart", sent[0].Labels["reason"])
			} else {
				sent := enqueuer.Sent(notificationstest.WithTemplateID(notifications.TemplateWorkspaceAutoUpdated))
				require.Empty(t, sent)
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
	assert.Len(t, stats.Errors, 0)
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
	workspace = coderdtest.MustTransitionWorkspace(t, client, workspace.ID, codersdk.WorkspaceTransitionStart, codersdk.WorkspaceTransitionStop)

	// When: the autobuild executor ticks way into the future
	go func() {
		tickCh <- workspace.LatestBuild.CreatedAt.Add(24 * time.Hour)
		close(tickCh)
	}()

	// Then: the workspace should not be started.
	stats := <-statsCh
	assert.Len(t, stats.Errors, 0)
	require.Len(t, stats.Transitions, 0)
}

func TestExecutorAutostartUserSuspended(t *testing.T) {
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
	)

	admin := coderdtest.CreateFirstUser(t, client)
	version := coderdtest.CreateTemplateVersion(t, client, admin.OrganizationID, nil)
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, admin.OrganizationID, version.ID)
	userClient, user := coderdtest.CreateAnotherUser(t, client, admin.OrganizationID)
	workspace := coderdtest.CreateWorkspace(t, userClient, template.ID, func(cwr *codersdk.CreateWorkspaceRequest) {
		cwr.AutostartSchedule = ptr.Ref(sched.String())
	})
	coderdtest.AwaitWorkspaceBuildJobCompleted(t, userClient, workspace.LatestBuild.ID)
	workspace = coderdtest.MustWorkspace(t, userClient, workspace.ID)

	// Given: workspace is stopped, and the user is suspended.
	workspace = coderdtest.MustTransitionWorkspace(t, userClient, workspace.ID, codersdk.WorkspaceTransitionStart, codersdk.WorkspaceTransitionStop)

	ctx := testutil.Context(t, testutil.WaitShort)

	_, err := client.UpdateUserStatus(ctx, user.ID.String(), codersdk.UserStatusSuspended)
	require.NoError(t, err, "update user status")

	// When: the autobuild executor ticks after the scheduled time
	go func() {
		tickCh <- sched.Next(workspace.LatestBuild.CreatedAt)
		close(tickCh)
	}()

	// Then: nothing should happen
	stats := testutil.TryReceive(ctx, t, statsCh)
	assert.Len(t, stats.Errors, 0)
	assert.Len(t, stats.Transitions, 0)
}

func TestExecutorAutostopOK(t *testing.T) {
	t.Parallel()

	var (
		tickCh     = make(chan time.Time)
		statsCh    = make(chan autobuild.Stats)
		client, db = coderdtest.NewWithDatabase(t, &coderdtest.Options{
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

	p, err := coderdtest.GetProvisionerForTags(db, time.Now(), workspace.OrganizationID, nil)
	require.NoError(t, err)

	// When: the autobuild executor ticks *after* the deadline:
	go func() {
		tickTime := workspace.LatestBuild.Deadline.Time.Add(time.Minute)
		coderdtest.UpdateProvisionerLastSeenAt(t, db, p.ID, tickTime)
		tickCh <- tickTime
		close(tickCh)
	}()

	// Then: the workspace should be stopped
	stats := <-statsCh
	assert.Len(t, stats.Errors, 0)
	assert.Len(t, stats.Transitions, 1)
	assert.Contains(t, stats.Transitions, workspace.ID)
	assert.Equal(t, database.WorkspaceTransitionStop, stats.Transitions[workspace.ID])

	workspace = coderdtest.MustWorkspace(t, client, workspace.ID)
	assert.Equal(t, codersdk.BuildReasonAutostop, workspace.LatestBuild.Reason)
}

func TestExecutorAutostopExtend(t *testing.T) {
	t.Parallel()

	var (
		ctx        = context.Background()
		tickCh     = make(chan time.Time)
		statsCh    = make(chan autobuild.Stats)
		client, db = coderdtest.NewWithDatabase(t, &coderdtest.Options{
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

	p, err := coderdtest.GetProvisionerForTags(db, time.Now(), workspace.OrganizationID, nil)
	require.NoError(t, err)

	// When: the autobuild executor ticks *after* the original deadline:
	go func() {
		tickTime := originalDeadline.Time.Add(time.Minute)
		coderdtest.UpdateProvisionerLastSeenAt(t, db, p.ID, tickTime)
		tickCh <- tickTime
	}()

	// Then: nothing should happen and the workspace should stay running
	stats := <-statsCh
	assert.Len(t, stats.Errors, 0)
	assert.Len(t, stats.Transitions, 0)

	// When: the autobuild executor ticks after the *new* deadline:
	go func() {
		tickTime := newDeadline.Add(time.Minute)
		coderdtest.UpdateProvisionerLastSeenAt(t, db, p.ID, tickTime)
		tickCh <- tickTime
		close(tickCh)
	}()

	// Then: the workspace should be stopped
	stats = <-statsCh
	assert.Len(t, stats.Errors, 0)
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
	workspace = coderdtest.MustTransitionWorkspace(t, client, workspace.ID, codersdk.WorkspaceTransitionStart, codersdk.WorkspaceTransitionStop)

	// When: the autobuild executor ticks past the TTL
	go func() {
		tickCh <- workspace.LatestBuild.Deadline.Time.Add(time.Minute)
		close(tickCh)
	}()

	// Then: the workspace should remain stopped and no build should happen.
	stats := <-statsCh
	assert.Len(t, stats.Errors, 0)
	assert.Len(t, stats.Transitions, 0)
}

func TestExecutorAutostopNotEnabled(t *testing.T) {
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
		workspace = mustProvisionWorkspace(t, client, func(cwr *codersdk.CreateWorkspaceRequest) {
			cwr.TTLMillis = nil
		})
	)

	// Given: workspace has no TTL set
	workspace = coderdtest.MustWorkspace(t, client, workspace.ID)
	require.Nil(t, workspace.TTLMillis)
	require.Zero(t, workspace.LatestBuild.Deadline)
	require.NotZero(t, workspace.LatestBuild.Job.CompletedAt)

	// Given: workspace is running
	require.Equal(t, codersdk.WorkspaceTransitionStart, workspace.LatestBuild.Transition)

	// When: the autobuild executor ticks a year in the future
	go func() {
		tickCh <- workspace.LatestBuild.Job.CompletedAt.AddDate(1, 0, 0)
		close(tickCh)
	}()

	// Then: the workspace should not be stopped.
	stats := <-statsCh
	assert.Len(t, stats.Errors, 0)
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
	workspace = coderdtest.MustTransitionWorkspace(t, client, workspace.ID, codersdk.WorkspaceTransitionStart, codersdk.WorkspaceTransitionDelete)

	// When: the autobuild executor ticks
	go func() {
		tickCh <- sched.Next(workspace.LatestBuild.CreatedAt)
		close(tickCh)
	}()

	// Then: nothing should happen
	stats := <-statsCh
	assert.Len(t, stats.Errors, 0)
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
	assert.Len(t, stats.Errors, 0)
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

	// Given: workspace is running and has a non-zero deadline
	require.Equal(t, codersdk.WorkspaceTransitionStart, workspace.LatestBuild.Transition)
	require.NotZero(t, workspace.LatestBuild.Deadline)

	// When: the autobuild executor ticks before the TTL
	go func() {
		tickCh <- workspace.LatestBuild.Deadline.Time.Add(-1 * time.Minute)
		close(tickCh)
	}()

	// Then: nothing should happen
	stats := <-statsCh
	assert.Len(t, stats.Errors, 0)
	assert.Len(t, stats.Transitions, 0)
}

func TestExecuteAutostopSuspendedUser(t *testing.T) {
	t.Parallel()

	var (
		tickCh     = make(chan time.Time)
		statsCh    = make(chan autobuild.Stats)
		client, db = coderdtest.NewWithDatabase(t, &coderdtest.Options{
			AutobuildTicker:          tickCh,
			IncludeProvisionerDaemon: true,
			AutobuildStats:           statsCh,
		})
	)

	admin := coderdtest.CreateFirstUser(t, client)
	// Wait for provisioner to be available
	coderdtest.MustWaitForAnyProvisioner(t, db)
	version := coderdtest.CreateTemplateVersion(t, client, admin.OrganizationID, nil)
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, admin.OrganizationID, version.ID)
	userClient, user := coderdtest.CreateAnotherUser(t, client, admin.OrganizationID)
	workspace := coderdtest.CreateWorkspace(t, userClient, template.ID)
	coderdtest.AwaitWorkspaceBuildJobCompleted(t, userClient, workspace.LatestBuild.ID)

	// Given: workspace is running, and the user is suspended.
	workspace = coderdtest.MustWorkspace(t, userClient, workspace.ID)
	require.Equal(t, codersdk.WorkspaceStatusRunning, workspace.LatestBuild.Status)

	ctx := testutil.Context(t, testutil.WaitShort)

	_, err := client.UpdateUserStatus(ctx, user.ID.String(), codersdk.UserStatusSuspended)
	require.NoError(t, err, "update user status")

	// When: the autobuild executor ticks after the scheduled time
	go func() {
		tickCh <- time.Unix(0, 0) // the exact time is not important
		close(tickCh)
	}()

	// Then: the workspace should be stopped
	stats := <-statsCh
	assert.Len(t, stats.Errors, 0)
	assert.Len(t, stats.Transitions, 1)
	assert.Equal(t, stats.Transitions[workspace.ID], database.WorkspaceTransitionStop)

	// Wait for stop to complete
	workspace = coderdtest.MustWorkspace(t, client, workspace.ID)
	workspaceBuild := coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)
	assert.Equal(t, codersdk.WorkspaceStatusStopped, workspaceBuild.Status)
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

	// Then: the deadline should be set to zero
	updated := coderdtest.MustWorkspace(t, client, workspace.ID)
	assert.True(t, !updated.LatestBuild.Deadline.Valid)

	// When: the autobuild executor ticks after the original deadline
	go func() {
		tickCh <- workspace.LatestBuild.Deadline.Time.Add(time.Minute)
	}()

	// Then: the workspace should not stop
	stats := <-statsCh
	assert.Len(t, stats.Errors, 0)
	assert.Len(t, stats.Transitions, 0)
}

func TestExecutorAutostartMultipleOK(t *testing.T) {
	t.Parallel()

	var (
		sched      = mustSchedule(t, "CRON_TZ=UTC 0 * * * *")
		tickCh     = make(chan time.Time)
		tickCh2    = make(chan time.Time)
		statsCh1   = make(chan autobuild.Stats)
		statsCh2   = make(chan autobuild.Stats)
		client, db = coderdtest.NewWithDatabase(t, &coderdtest.Options{
			AutobuildTicker:          tickCh,
			IncludeProvisionerDaemon: true,
			AutobuildStats:           statsCh1,
		})
		_, _ = coderdtest.NewWithDatabase(t, &coderdtest.Options{
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
	workspace = coderdtest.MustTransitionWorkspace(t, client, workspace.ID, codersdk.WorkspaceTransitionStart, codersdk.WorkspaceTransitionStop)

	p, err := coderdtest.GetProvisionerForTags(db, time.Now(), workspace.OrganizationID, nil)
	require.NoError(t, err)

	// When: the autobuild executor ticks past the scheduled time
	go func() {
		tickTime := sched.Next(workspace.LatestBuild.CreatedAt)
		coderdtest.UpdateProvisionerLastSeenAt(t, db, p.ID, tickTime)
		tickCh <- tickTime
		tickCh2 <- tickTime
		close(tickCh)
		close(tickCh2)
	}()

	// Then: the workspace should eventually be started
	stats1 := <-statsCh1
	assert.Len(t, stats1.Errors, 0)
	assert.Len(t, stats1.Transitions, 1)
	assert.Contains(t, stats1.Transitions, workspace.ID)
	assert.Equal(t, database.WorkspaceTransitionStart, stats1.Transitions[workspace.ID])

	// Then: the other executor should not have done anything
	stats2 := <-statsCh2
	assert.Len(t, stats2.Errors, 0)
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
		sched      = mustSchedule(t, "CRON_TZ=UTC 0 * * * *")
		tickCh     = make(chan time.Time)
		statsCh    = make(chan autobuild.Stats)
		client, db = coderdtest.NewWithDatabase(t, &coderdtest.Options{
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
	workspace = coderdtest.MustTransitionWorkspace(t, client, workspace.ID, codersdk.WorkspaceTransitionStart, codersdk.WorkspaceTransitionStop)

	p, err := coderdtest.GetProvisionerForTags(db, time.Now(), workspace.OrganizationID, nil)
	require.NoError(t, err)

	// When: the autobuild executor ticks after the scheduled time
	go func() {
		tickTime := sched.Next(workspace.LatestBuild.CreatedAt)
		coderdtest.UpdateProvisionerLastSeenAt(t, db, p.ID, tickTime)
		tickCh <- tickTime
		close(tickCh)
	}()

	// Then: the workspace with parameters should eventually be started
	stats := <-statsCh
	assert.Len(t, stats.Errors, 0)
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
	workspace = coderdtest.MustTransitionWorkspace(t, client, workspace.ID, codersdk.WorkspaceTransitionStart, codersdk.WorkspaceTransitionStop)

	// When: the autobuild executor ticks before the next scheduled time
	go func() {
		tickCh <- sched.Next(workspace.LatestBuild.CreatedAt).Add(time.Minute)
		close(tickCh)
	}()

	// Then: nothing should happen
	stats := <-statsCh
	assert.Len(t, stats.Errors, 0)
	assert.Len(t, stats.Transitions, 0)
}

func TestExecutorAutostopTemplateDisabled(t *testing.T) {
	t.Parallel()

	// Given: we have a workspace built from a template that disallows user autostop
	var (
		tickCh  = make(chan time.Time)
		statsCh = make(chan autobuild.Stats)

		client, db = coderdtest.NewWithDatabase(t, &coderdtest.Options{
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
		// Given: we have a user with a workspace configured to autostop 30 minutes in the future
		workspace = mustProvisionWorkspace(t, client, func(cwr *codersdk.CreateWorkspaceRequest) {
			cwr.TTLMillis = ptr.Ref(30 * time.Minute.Milliseconds())
		})
	)

	// When: we create the workspace
	// Then: the deadline should be set to the template default TTL
	assert.WithinDuration(t, workspace.LatestBuild.CreatedAt.Add(time.Hour), workspace.LatestBuild.Deadline.Time, time.Minute)

	p, err := coderdtest.GetProvisionerForTags(db, time.Now(), workspace.OrganizationID, nil)
	require.NoError(t, err)

	// When: the autobuild executor ticks after the workspace setting, but before the template setting:
	go func() {
		tickTime := workspace.LatestBuild.Job.CompletedAt.Add(45 * time.Minute)
		coderdtest.UpdateProvisionerLastSeenAt(t, db, p.ID, tickTime)
		tickCh <- tickTime
	}()

	// Then: nothing should happen
	stats := <-statsCh
	assert.Len(t, stats.Errors, 0)
	assert.Len(t, stats.Transitions, 0)

	// When: the autobuild executor ticks after the template setting:
	go func() {
		tickTime := workspace.LatestBuild.Job.CompletedAt.Add(61 * time.Minute)
		coderdtest.UpdateProvisionerLastSeenAt(t, db, p.ID, tickTime)
		tickCh <- tickTime
		close(tickCh)
	}()

	// Then: the workspace should be stopped
	stats = <-statsCh
	assert.Len(t, stats.Errors, 0)
	assert.Len(t, stats.Transitions, 1)
	assert.Contains(t, stats.Transitions, workspace.ID)
	assert.Equal(t, database.WorkspaceTransitionStop, stats.Transitions[workspace.ID])
}

// Test that an AGPL AccessControlStore properly disables
// functionality.
func TestExecutorRequireActiveVersion(t *testing.T) {
	t.Parallel()

	var (
		sched  = mustSchedule(t, "CRON_TZ=UTC 0 * * * *")
		ticker = make(chan time.Time)
		statCh = make(chan autobuild.Stats)

		ownerClient, db = coderdtest.NewWithDatabase(t, &coderdtest.Options{
			AutobuildTicker:          ticker,
			IncludeProvisionerDaemon: true,
			AutobuildStats:           statCh,
			TemplateScheduleStore:    schedule.NewAGPLTemplateScheduleStore(),
		})
	)
	// Wait for provisioner to be available
	coderdtest.MustWaitForAnyProvisioner(t, db)

	ctx := testutil.Context(t, testutil.WaitShort)
	owner := coderdtest.CreateFirstUser(t, ownerClient)
	me, err := ownerClient.User(ctx, codersdk.Me)
	require.NoError(t, err)

	// Create an active and inactive template version. We'll
	// build a regular member's workspace using a non-active
	// template version and assert that the field is not abided
	// since there is no enterprise license.
	activeVersion := coderdtest.CreateTemplateVersion(t, ownerClient, owner.OrganizationID, nil)
	coderdtest.AwaitTemplateVersionJobCompleted(t, ownerClient, activeVersion.ID)
	template := coderdtest.CreateTemplate(t, ownerClient, owner.OrganizationID, activeVersion.ID)

	ctx = testutil.Context(t, testutil.WaitShort) // Reset context after setting up the template.

	//nolint We need to set this in the database directly, because the API will return an error
	// letting you know that this feature requires an enterprise license.
	err = db.UpdateTemplateAccessControlByID(dbauthz.As(ctx, coderdtest.AuthzUserSubject(me, owner.OrganizationID)), database.UpdateTemplateAccessControlByIDParams{
		ID:                   template.ID,
		RequireActiveVersion: true,
	})
	require.NoError(t, err)
	inactiveVersion := coderdtest.CreateTemplateVersion(t, ownerClient, owner.OrganizationID, nil, func(ctvr *codersdk.CreateTemplateVersionRequest) {
		ctvr.TemplateID = template.ID
	})
	coderdtest.AwaitTemplateVersionJobCompleted(t, ownerClient, inactiveVersion.ID)
	memberClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)
	ws := coderdtest.CreateWorkspace(t, memberClient, uuid.Nil, func(cwr *codersdk.CreateWorkspaceRequest) {
		cwr.TemplateVersionID = inactiveVersion.ID
		cwr.AutostartSchedule = ptr.Ref(sched.String())
	})
	_ = coderdtest.AwaitWorkspaceBuildJobCompleted(t, ownerClient, ws.LatestBuild.ID)
	ws = coderdtest.MustTransitionWorkspace(t, memberClient, ws.ID, codersdk.WorkspaceTransitionStart, codersdk.WorkspaceTransitionStop, func(req *codersdk.CreateWorkspaceBuildRequest) {
		req.TemplateVersionID = inactiveVersion.ID
	})
	require.Equal(t, inactiveVersion.ID, ws.LatestBuild.TemplateVersionID)

	p, err := coderdtest.GetProvisionerForTags(db, time.Now(), ws.OrganizationID, nil)
	require.NoError(t, err)

	tickTime := sched.Next(ws.LatestBuild.CreatedAt)
	coderdtest.UpdateProvisionerLastSeenAt(t, db, p.ID, tickTime)
	ticker <- tickTime
	stats := <-statCh
	require.Len(t, stats.Transitions, 1)

	ws = coderdtest.MustWorkspace(t, memberClient, ws.ID)
	require.Equal(t, inactiveVersion.ID, ws.LatestBuild.TemplateVersionID)
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
		ws := coderdtest.CreateWorkspace(t, client, template.ID)
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
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.TimeTilDormantMillis = ptr.Ref[int64](inactiveTTL.Milliseconds())
		})
		ws := coderdtest.CreateWorkspace(t, client, template.ID)
		build := coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, ws.LatestBuild.ID)
		require.Equal(t, codersdk.WorkspaceStatusRunning, build.Status)
		ticker <- ws.LastUsedAt.Add(inactiveTTL * 2)
		stats := <-statCh
		// Expect no transitions since we're using AGPL.
		require.Len(t, stats.Transitions, 0)
	})
}

func TestNotifications(t *testing.T) {
	t.Parallel()

	t.Run("Dormancy", func(t *testing.T) {
		t.Parallel()

		// Setup template with dormancy and create a workspace with it
		var (
			ticker         = make(chan time.Time)
			statCh         = make(chan autobuild.Stats)
			notifyEnq      = notificationstest.FakeEnqueuer{}
			timeTilDormant = time.Minute
			client, db     = coderdtest.NewWithDatabase(t, &coderdtest.Options{
				AutobuildTicker:          ticker,
				AutobuildStats:           statCh,
				IncludeProvisionerDaemon: true,
				NotificationsEnqueuer:    &notifyEnq,
				TemplateScheduleStore: schedule.MockTemplateScheduleStore{
					SetFn: func(ctx context.Context, db database.Store, template database.Template, options schedule.TemplateScheduleOptions) (database.Template, error) {
						template.TimeTilDormant = int64(options.TimeTilDormant)
						return schedule.NewAGPLTemplateScheduleStore().Set(ctx, db, template, options)
					},
					GetFn: func(_ context.Context, _ database.Store, _ uuid.UUID) (schedule.TemplateScheduleOptions, error) {
						return schedule.TemplateScheduleOptions{
							UserAutostartEnabled: false,
							UserAutostopEnabled:  true,
							DefaultTTL:           0,
							AutostopRequirement:  schedule.TemplateAutostopRequirement{},
							TimeTilDormant:       timeTilDormant,
						}, nil
					},
				},
			})
			admin   = coderdtest.CreateFirstUser(t, client)
			version = coderdtest.CreateTemplateVersion(t, client, admin.OrganizationID, nil)
		)

		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, admin.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.TimeTilDormantMillis = ptr.Ref(timeTilDormant.Milliseconds())
		})
		userClient, _ := coderdtest.CreateAnotherUser(t, client, admin.OrganizationID)
		workspace := coderdtest.CreateWorkspace(t, userClient, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, userClient, workspace.LatestBuild.ID)

		// Stop workspace
		workspace = coderdtest.MustTransitionWorkspace(t, client, workspace.ID, codersdk.WorkspaceTransitionStart, codersdk.WorkspaceTransitionStop)
		_ = coderdtest.AwaitWorkspaceBuildJobCompleted(t, userClient, workspace.LatestBuild.ID)

		p, err := coderdtest.GetProvisionerForTags(db, time.Now(), workspace.OrganizationID, nil)
		require.NoError(t, err)

		// Wait for workspace to become dormant
		notifyEnq.Clear()
		tickTime := workspace.LastUsedAt.Add(timeTilDormant * 3)
		coderdtest.UpdateProvisionerLastSeenAt(t, db, p.ID, tickTime)
		ticker <- tickTime
		_ = testutil.TryReceive(testutil.Context(t, testutil.WaitShort), t, statCh)

		// Check that the workspace is dormant
		workspace = coderdtest.MustWorkspace(t, client, workspace.ID)
		require.NotNil(t, workspace.DormantAt)

		// Check that a notification was enqueued
		sent := notifyEnq.Sent()
		require.Len(t, sent, 1)
		require.Equal(t, sent[0].UserID, workspace.OwnerID)
		require.Equal(t, sent[0].TemplateID, notifications.TemplateWorkspaceDormant)
		require.Contains(t, sent[0].Targets, template.ID)
		require.Contains(t, sent[0].Targets, workspace.ID)
		require.Contains(t, sent[0].Targets, workspace.OrganizationID)
		require.Contains(t, sent[0].Targets, workspace.OwnerID)
	})
}

// TestExecutorPrebuilds verifies AGPL behavior for prebuilt workspaces.
// It ensures that workspace schedules do not trigger while the workspace
// is still in a prebuilt state. Scheduling behavior only applies after the
// workspace has been claimed and becomes a regular user workspace.
// For enterprise-related functionality, see enterprise/coderd/workspaces_test.go.
func TestExecutorPrebuilds(t *testing.T) {
	t.Parallel()

	// Prebuild workspaces should not be autostopped when the deadline is reached.
	// After being claimed, the workspace should stop at the deadline.
	t.Run("OnlyStopsAfterClaimed", func(t *testing.T) {
		t.Parallel()

		// Setup
		ctx := testutil.Context(t, testutil.WaitShort)
		clock := quartz.NewMock(t)
		db, pb := dbtestutil.NewDB(t, dbtestutil.WithDumpOnFailure())
		var (
			tickCh  = make(chan time.Time)
			statsCh = make(chan autobuild.Stats)
			client  = coderdtest.New(t, &coderdtest.Options{
				Database:                 db,
				Pubsub:                   pb,
				AutobuildTicker:          tickCh,
				IncludeProvisionerDaemon: true,
				AutobuildStats:           statsCh,
			})
		)

		// Setup user, template and template version
		owner := coderdtest.CreateFirstUser(t, client)
		_, user := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleMember())
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		// Database setup of a preset with a prebuild instance
		preset := setupTestDBPreset(t, db, version.ID, int32(1))

		// Given: a running prebuilt workspace with a deadline and ready to be claimed
		dbPrebuild := setupTestDBPrebuiltWorkspace(
			ctx, t, clock, db, pb,
			owner.OrganizationID,
			template.ID,
			version.ID,
			preset.ID,
		)
		prebuild := coderdtest.MustWorkspace(t, client, dbPrebuild.ID)
		require.Equal(t, codersdk.WorkspaceTransitionStart, prebuild.LatestBuild.Transition)
		require.NotZero(t, prebuild.LatestBuild.Deadline)

		p, err := coderdtest.GetProvisionerForTags(db, time.Now(), prebuild.OrganizationID, nil)
		require.NoError(t, err)

		// When: the autobuild executor ticks *after* the deadline:
		go func() {
			tickTime := prebuild.LatestBuild.Deadline.Time.Add(time.Minute)
			coderdtest.UpdateProvisionerLastSeenAt(t, db, p.ID, tickTime)
			tickCh <- tickTime
		}()

		// Then: the prebuilt workspace should remain in a start transition
		prebuildStats := testutil.RequireReceive(ctx, t, statsCh)
		require.Len(t, prebuildStats.Errors, 0)
		require.Len(t, prebuildStats.Transitions, 0)
		require.Equal(t, codersdk.WorkspaceTransitionStart, prebuild.LatestBuild.Transition)
		prebuild = coderdtest.MustWorkspace(t, client, prebuild.ID)
		require.Equal(t, codersdk.BuildReasonInitiator, prebuild.LatestBuild.Reason)

		// Given: a user claims the prebuilt workspace
		dbWorkspace := dbgen.ClaimPrebuild(
			t, db,
			clock.Now(),
			user.ID,
			"claimedWorkspace-autostop",
			preset.ID,
			sql.NullString{},
			sql.NullTime{},
			sql.NullInt64{})
		workspace := coderdtest.MustWorkspace(t, client, dbWorkspace.ID)

		// When: the autobuild executor ticks *after* the deadline:
		go func() {
			tickTime := workspace.LatestBuild.Deadline.Time.Add(time.Minute)
			coderdtest.UpdateProvisionerLastSeenAt(t, db, p.ID, tickTime)
			tickCh <- tickTime
			close(tickCh)
		}()

		// Then: the workspace should be stopped
		workspaceStats := testutil.RequireReceive(ctx, t, statsCh)
		require.Len(t, workspaceStats.Errors, 0)
		require.Len(t, workspaceStats.Transitions, 1)
		require.Contains(t, workspaceStats.Transitions, workspace.ID)
		require.Equal(t, database.WorkspaceTransitionStop, workspaceStats.Transitions[workspace.ID])
		workspace = coderdtest.MustWorkspace(t, client, workspace.ID)
		require.Equal(t, codersdk.BuildReasonAutostop, workspace.LatestBuild.Reason)
	})

	// Prebuild workspaces should not be autostarted when the autostart scheduled is reached.
	// After being claimed, the workspace should autostart at the schedule.
	t.Run("OnlyStartsAfterClaimed", func(t *testing.T) {
		t.Parallel()

		// Setup
		ctx := testutil.Context(t, testutil.WaitShort)
		clock := quartz.NewMock(t)
		db, pb := dbtestutil.NewDB(t, dbtestutil.WithDumpOnFailure())
		var (
			tickCh  = make(chan time.Time)
			statsCh = make(chan autobuild.Stats)
			client  = coderdtest.New(t, &coderdtest.Options{
				Database:                 db,
				Pubsub:                   pb,
				AutobuildTicker:          tickCh,
				IncludeProvisionerDaemon: true,
				AutobuildStats:           statsCh,
			})
		)

		// Setup user, template and template version
		owner := coderdtest.CreateFirstUser(t, client)
		_, user := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleMember())
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		// Database setup of a preset with a prebuild instance
		preset := setupTestDBPreset(t, db, version.ID, int32(1))

		// Given: prebuilt workspace is stopped and set to autostart daily at midnight
		sched := mustSchedule(t, "CRON_TZ=UTC 0 0 * * *")
		autostartSched := sql.NullString{
			String: sched.String(),
			Valid:  true,
		}
		dbPrebuild := setupTestDBPrebuiltWorkspace(
			ctx, t, clock, db, pb,
			owner.OrganizationID,
			template.ID,
			version.ID,
			preset.ID,
			WithAutostartSchedule(autostartSched),
			WithIsStopped(true),
		)
		prebuild := coderdtest.MustWorkspace(t, client, dbPrebuild.ID)
		require.Equal(t, codersdk.WorkspaceTransitionStop, prebuild.LatestBuild.Transition)
		require.NotNil(t, prebuild.AutostartSchedule)

		// Tick at the next scheduled time after the prebuild’s LatestBuild.CreatedAt,
		// since the next allowed autostart is calculated starting from that point.
		// When: the autobuild executor ticks after the scheduled time
		go func() {
			tickCh <- sched.Next(prebuild.LatestBuild.CreatedAt).Add(time.Minute)
		}()

		// Then: the prebuilt workspace should remain in a stop transition
		prebuildStats := testutil.RequireReceive(ctx, t, statsCh)
		require.Len(t, prebuildStats.Errors, 0)
		require.Len(t, prebuildStats.Transitions, 0)
		require.Equal(t, codersdk.WorkspaceTransitionStop, prebuild.LatestBuild.Transition)
		prebuild = coderdtest.MustWorkspace(t, client, prebuild.ID)
		require.Equal(t, codersdk.BuildReasonInitiator, prebuild.LatestBuild.Reason)

		// Given: prebuilt workspace is in a start status
		setupTestDBWorkspaceBuild(
			ctx, t, clock, db, pb,
			owner.OrganizationID,
			prebuild.ID,
			version.ID,
			preset.ID,
			database.WorkspaceTransitionStart)

		// Given: a user claims the prebuilt workspace
		dbWorkspace := dbgen.ClaimPrebuild(
			t, db,
			clock.Now(),
			user.ID,
			"claimedWorkspace-autostart",
			preset.ID,
			autostartSched,
			sql.NullTime{},
			sql.NullInt64{})
		workspace := coderdtest.MustWorkspace(t, client, dbWorkspace.ID)

		// Given: the prebuilt workspace goes to a stop status
		setupTestDBWorkspaceBuild(
			ctx, t, clock, db, pb,
			owner.OrganizationID,
			prebuild.ID,
			version.ID,
			preset.ID,
			database.WorkspaceTransitionStop)

		// Tick at the next scheduled time after the prebuild’s LatestBuild.CreatedAt,
		// since the next allowed autostart is calculated starting from that point.
		// When: the autobuild executor ticks after the scheduled time
		go func() {
			tickCh <- sched.Next(workspace.LatestBuild.CreatedAt).Add(time.Minute)
			close(tickCh)
		}()

		// Then: the workspace should eventually be started
		workspaceStats := testutil.RequireReceive(ctx, t, statsCh)
		require.Len(t, workspaceStats.Errors, 0)
		require.Len(t, workspaceStats.Transitions, 1)
		require.Contains(t, workspaceStats.Transitions, workspace.ID)
		require.Equal(t, database.WorkspaceTransitionStart, workspaceStats.Transitions[workspace.ID])
		workspace = coderdtest.MustWorkspace(t, client, workspace.ID)
		require.Equal(t, codersdk.BuildReasonAutostart, workspace.LatestBuild.Reason)
	})
}

func setupTestDBPreset(
	t *testing.T,
	db database.Store,
	templateVersionID uuid.UUID,
	desiredInstances int32,
) database.TemplateVersionPreset {
	t.Helper()

	preset := dbgen.Preset(t, db, database.InsertPresetParams{
		TemplateVersionID: templateVersionID,
		Name:              "preset-test",
		DesiredInstances: sql.NullInt32{
			Valid: true,
			Int32: desiredInstances,
		},
	})
	dbgen.PresetParameter(t, db, database.InsertPresetParametersParams{
		TemplateVersionPresetID: preset.ID,
		Names:                   []string{"test-name"},
		Values:                  []string{"test-value"},
	})

	return preset
}

type SetupPrebuiltOptions struct {
	AutostartSchedule sql.NullString
	IsStopped         bool
}

func WithAutostartSchedule(sched sql.NullString) func(*SetupPrebuiltOptions) {
	return func(o *SetupPrebuiltOptions) {
		o.AutostartSchedule = sched
	}
}

func WithIsStopped(isStopped bool) func(*SetupPrebuiltOptions) {
	return func(o *SetupPrebuiltOptions) {
		o.IsStopped = isStopped
	}
}

func setupTestDBWorkspaceBuild(
	ctx context.Context,
	t *testing.T,
	clock quartz.Clock,
	db database.Store,
	ps pubsub.Pubsub,
	orgID uuid.UUID,
	workspaceID uuid.UUID,
	templateVersionID uuid.UUID,
	presetID uuid.UUID,
	transition database.WorkspaceTransition,
) (database.ProvisionerJob, database.WorkspaceBuild) {
	t.Helper()

	var buildNumber int32 = 1
	latestWorkspaceBuild, err := db.GetLatestWorkspaceBuildByWorkspaceID(ctx, workspaceID)
	if !errors.Is(err, sql.ErrNoRows) {
		buildNumber = latestWorkspaceBuild.BuildNumber + 1
	}

	job := dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{
		InitiatorID:    database.PrebuildsSystemUserID,
		CreatedAt:      clock.Now().Add(-time.Hour * 2),
		StartedAt:      sql.NullTime{Time: clock.Now().Add(-time.Hour * 2), Valid: true},
		CompletedAt:    sql.NullTime{Time: clock.Now().Add(-time.Hour), Valid: true},
		OrganizationID: orgID,
		JobStatus:      database.ProvisionerJobStatusSucceeded,
	})
	workspaceBuild := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		WorkspaceID:             workspaceID,
		InitiatorID:             database.PrebuildsSystemUserID,
		TemplateVersionID:       templateVersionID,
		BuildNumber:             buildNumber,
		JobID:                   job.ID,
		TemplateVersionPresetID: uuid.NullUUID{UUID: presetID, Valid: true},
		Transition:              transition,
		CreatedAt:               clock.Now(),
	})
	dbgen.WorkspaceBuildParameters(t, db, []database.WorkspaceBuildParameter{
		{
			WorkspaceBuildID: workspaceBuild.ID,
			Name:             "test",
			Value:            "test",
		},
	})

	workspaceResource := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
		JobID:      job.ID,
		Transition: database.WorkspaceTransitionStart,
		Type:       "compute",
		Name:       "main",
	})

	// Workspaces are eligible to be claimed once their agent is marked "ready"
	dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
		Name:            "test",
		ResourceID:      workspaceResource.ID,
		Architecture:    "i386",
		OperatingSystem: "linux",
		LifecycleState:  database.WorkspaceAgentLifecycleStateReady,
		StartedAt:       sql.NullTime{Time: clock.Now().Add(time.Hour), Valid: true},
		ReadyAt:         sql.NullTime{Time: clock.Now().Add(-1 * time.Hour), Valid: true},
		APIKeyScope:     database.AgentKeyScopeEnumAll,
	})

	return job, workspaceBuild
}

func setupTestDBPrebuiltWorkspace(
	ctx context.Context,
	t *testing.T,
	clock quartz.Clock,
	db database.Store,
	ps pubsub.Pubsub,
	orgID uuid.UUID,
	templateID uuid.UUID,
	templateVersionID uuid.UUID,
	presetID uuid.UUID,
	opts ...func(*SetupPrebuiltOptions),
) database.WorkspaceTable {
	t.Helper()

	// Optional parameters
	options := &SetupPrebuiltOptions{}
	for _, opt := range opts {
		opt(options)
	}

	buildTransition := database.WorkspaceTransitionStart
	if options.IsStopped {
		buildTransition = database.WorkspaceTransitionStop
	}

	workspace := dbgen.Workspace(t, db, database.WorkspaceTable{
		TemplateID:        templateID,
		OrganizationID:    orgID,
		OwnerID:           database.PrebuildsSystemUserID,
		Deleted:           false,
		CreatedAt:         clock.Now().Add(-time.Hour * 2),
		AutostartSchedule: options.AutostartSchedule,
		LastUsedAt:        clock.Now(),
	})
	setupTestDBWorkspaceBuild(ctx, t, clock, db, ps, orgID, workspace.ID, templateVersionID, presetID, buildTransition)

	return workspace
}

func mustProvisionWorkspace(t *testing.T, client *codersdk.Client, mut ...func(*codersdk.CreateWorkspaceRequest)) codersdk.Workspace {
	t.Helper()
	user := coderdtest.CreateFirstUser(t, client)
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	ws := coderdtest.CreateWorkspace(t, client, template.ID, mut...)
	coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, ws.LatestBuild.ID)
	return coderdtest.MustWorkspace(t, client, ws.ID)
}

// mustProvisionWorkspaceWithProvisionerTags creates a workspace with a template version that has specific provisioner tags
func mustProvisionWorkspaceWithProvisionerTags(t *testing.T, client *codersdk.Client, provisionerTags map[string]string, mut ...func(*codersdk.CreateWorkspaceRequest)) codersdk.Workspace {
	t.Helper()
	user := coderdtest.CreateFirstUser(t, client)

	// Create template version with specific provisioner tags
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil, func(request *codersdk.CreateTemplateVersionRequest) {
		request.ProvisionerTags = provisionerTags
	})
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	t.Logf("template version %s job has completed with provisioner tags %v", version.ID, provisionerTags)

	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

	ws := coderdtest.CreateWorkspace(t, client, template.ID, mut...)
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
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	ws := coderdtest.CreateWorkspace(t, client, template.ID, mut...)
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
	ctx := testutil.Context(t, testutil.WaitShort)
	buildParameters, err := client.WorkspaceBuildParameters(ctx, workspaceID)
	require.NoError(t, err)
	require.NotEmpty(t, buildParameters)
}

func TestExecutorAutostartSkipsWhenNoProvisionersAvailable(t *testing.T) {
	t.Parallel()

	var (
		sched   = mustSchedule(t, "CRON_TZ=UTC 0 * * * *")
		tickCh  = make(chan time.Time)
		statsCh = make(chan autobuild.Stats)
	)

	// Use provisioner daemon tags so we can test `hasAvailableProvisioner` more thoroughly.
	// We can't overwrite owner or scope as there's a `provisionersdk.MutateTags` function that has restrictions on those.
	provisionerDaemonTags := map[string]string{"test-tag": "asdf"}
	t.Logf("Setting provisioner daemon tags: %v", provisionerDaemonTags)

	db, ps := dbtestutil.NewDB(t)
	client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
		Database:                 db,
		Pubsub:                   ps,
		IncludeProvisionerDaemon: false,
		AutobuildTicker:          tickCh,
		AutobuildStats:           statsCh,
	})

	daemon1Closer := coderdtest.NewTaggedProvisionerDaemon(t, api, "name", provisionerDaemonTags)
	t.Cleanup(func() {
		_ = daemon1Closer.Close()
	})

	// Create workspace with autostart enabled and matching provisioner tags
	workspace := mustProvisionWorkspaceWithProvisionerTags(t, client, provisionerDaemonTags, func(cwr *codersdk.CreateWorkspaceRequest) {
		cwr.AutostartSchedule = ptr.Ref(sched.String())
	})

	// Stop the workspace while provisioner is available
	workspace = coderdtest.MustTransitionWorkspace(t, client, workspace.ID, codersdk.WorkspaceTransitionStart, codersdk.WorkspaceTransitionStop)

	p, err := coderdtest.GetProvisionerForTags(db, time.Now(), workspace.OrganizationID, provisionerDaemonTags)
	require.NoError(t, err, "Error getting provisioner for workspace")

	// We're going to use an artificial next scheduled autostart time, as opposed to calculating it via sched.Next, since
	// we want to assert/require specific behavior here around the provisioner being stale, and therefore we need to be
	// able to give the provisioner(s) specific `LastSeenAt` times while dealing with the contraint that we cannot set
	// that value to some time in the past (relative to it's current value).
	next := p.LastSeenAt.Time.Add(5 * time.Minute)
	staleTime := next.Add(-(provisionerdserver.StaleInterval + time.Second))
	coderdtest.UpdateProvisionerLastSeenAt(t, db, p.ID, staleTime)

	// Require that the provisioners LastSeenAt has been updated to the expected time.
	p, err = coderdtest.GetProvisionerForTags(db, time.Now(), workspace.OrganizationID, provisionerDaemonTags)
	require.NoError(t, err, "Error getting provisioner for workspace")
	// This assertion *may* no longer need to be `Eventually`.
	require.Eventually(t, func() bool { return p.LastSeenAt.Time.UnixNano() == staleTime.UnixNano() },
		testutil.WaitMedium, testutil.IntervalFast, "expected provisioner LastSeenAt to be:%+v, saw :%+v", staleTime.UTC(), p.LastSeenAt.Time.UTC())

	// Ensure the provisioner is gone or stale, relative to the artificial next autostart time, before triggering the autobuild.
	coderdtest.MustWaitForProvisionersUnavailable(t, db, workspace, provisionerDaemonTags, next)

	// Trigger autobuild.
	tickCh <- next
	stats := <-statsCh
	assert.Len(t, stats.Transitions, 0, "should not create builds when no provisioners available")

	daemon2Closer := coderdtest.NewTaggedProvisionerDaemon(t, api, "name", provisionerDaemonTags)
	t.Cleanup(func() {
		_ = daemon2Closer.Close()
	})

	// Ensure the provisioner is  NOT stale, and see if we get a successful state transition.
	p, err = coderdtest.GetProvisionerForTags(db, time.Now(), workspace.OrganizationID, provisionerDaemonTags)
	require.NoError(t, err, "Error getting provisioner for workspace")

	next = sched.Next(workspace.LatestBuild.CreatedAt)
	notStaleTime := next.Add((-1 * provisionerdserver.StaleInterval) + 10*time.Second)
	coderdtest.UpdateProvisionerLastSeenAt(t, db, p.ID, notStaleTime)
	// Require that the provisioner time has actually been updated to the expected value.
	p, err = coderdtest.GetProvisionerForTags(db, time.Now(), workspace.OrganizationID, provisionerDaemonTags)
	require.NoError(t, err, "Error getting provisioner for workspace")
	require.True(t, next.UnixNano() > p.LastSeenAt.Time.UnixNano())

	// Trigger autobuild
	go func() {
		tickCh <- next
		close(tickCh)
	}()
	stats = <-statsCh

	assert.Len(t, stats.Transitions, 1, "should create builds when provisioners are available")
}

func TestExecutorTaskWorkspace(t *testing.T) {
	t.Parallel()

	createTaskTemplate := func(t *testing.T, client *codersdk.Client, orgID uuid.UUID, ctx context.Context, defaultTTL time.Duration) codersdk.Template {
		t.Helper()

		taskAppID := uuid.New()
		version := coderdtest.CreateTemplateVersion(t, client, orgID, &echo.Responses{
			Parse: echo.ParseComplete,
			ProvisionPlan: []*proto.Response{
				{
					Type: &proto.Response_Plan{
						Plan: &proto.PlanComplete{HasAiTasks: true},
					},
				},
			},
			ProvisionApply: []*proto.Response{
				{
					Type: &proto.Response_Apply{
						Apply: &proto.ApplyComplete{
							Resources: []*proto.Resource{
								{
									Agents: []*proto.Agent{
										{
											Id:   uuid.NewString(),
											Name: "dev",
											Auth: &proto.Agent_Token{
												Token: uuid.NewString(),
											},
											Apps: []*proto.App{
												{
													Id:   taskAppID.String(),
													Slug: "task-app",
												},
											},
										},
									},
								},
							},
							AiTasks: []*proto.AITask{
								{
									AppId: taskAppID.String(),
								},
							},
						},
					},
				},
			},
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, orgID, version.ID)

		if defaultTTL > 0 {
			_, err := client.UpdateTemplateMeta(ctx, template.ID, codersdk.UpdateTemplateMeta{
				DefaultTTLMillis: defaultTTL.Milliseconds(),
			})
			require.NoError(t, err)
		}

		return template
	}

	createTaskWorkspace := func(t *testing.T, client *codersdk.Client, template codersdk.Template, ctx context.Context, input string) codersdk.Workspace {
		t.Helper()

		exp := codersdk.NewExperimentalClient(client)
		task, err := exp.CreateTask(ctx, "me", codersdk.CreateTaskRequest{
			TemplateVersionID: template.ActiveVersionID,
			Input:             input,
		})
		require.NoError(t, err)
		require.True(t, task.WorkspaceID.Valid, "task should have a workspace")

		workspace, err := client.Workspace(ctx, task.WorkspaceID.UUID)
		require.NoError(t, err)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

		return workspace
	}

	t.Run("Autostart", func(t *testing.T) {
		t.Parallel()

		var (
			ctx        = testutil.Context(t, testutil.WaitShort)
			sched      = mustSchedule(t, "CRON_TZ=UTC 0 * * * *")
			tickCh     = make(chan time.Time)
			statsCh    = make(chan autobuild.Stats)
			client, db = coderdtest.NewWithDatabase(t, &coderdtest.Options{
				AutobuildTicker:          tickCh,
				IncludeProvisionerDaemon: true,
				AutobuildStats:           statsCh,
			})
			admin = coderdtest.CreateFirstUser(t, client)
		)

		// Given: A task workspace
		template := createTaskTemplate(t, client, admin.OrganizationID, ctx, 0)
		workspace := createTaskWorkspace(t, client, template, ctx, "test task for autostart")

		// Given: The task workspace has an autostart schedule
		err := client.UpdateWorkspaceAutostart(ctx, workspace.ID, codersdk.UpdateWorkspaceAutostartRequest{
			Schedule: ptr.Ref(sched.String()),
		})
		require.NoError(t, err)

		// Given: That the workspace is in a stopped state.
		workspace = coderdtest.MustTransitionWorkspace(t, client, workspace.ID, codersdk.WorkspaceTransitionStart, codersdk.WorkspaceTransitionStop)

		p, err := coderdtest.GetProvisionerForTags(db, time.Now(), workspace.OrganizationID, map[string]string{})
		require.NoError(t, err)

		// When: the autobuild executor ticks after the scheduled time
		go func() {
			tickTime := sched.Next(workspace.LatestBuild.CreatedAt)
			coderdtest.UpdateProvisionerLastSeenAt(t, db, p.ID, tickTime)
			tickCh <- tickTime
			close(tickCh)
		}()

		// Then: We expect to see a start transition
		stats := <-statsCh
		require.Len(t, stats.Transitions, 1, "lifecycle executor should transition the task workspace")
		assert.Contains(t, stats.Transitions, workspace.ID, "task workspace should be in transitions")
		assert.Equal(t, database.WorkspaceTransitionStart, stats.Transitions[workspace.ID], "should autostart the workspace")
		require.Empty(t, stats.Errors, "should have no errors when managing task workspaces")
	})

	t.Run("Autostop", func(t *testing.T) {
		t.Parallel()

		var (
			ctx        = testutil.Context(t, testutil.WaitShort)
			tickCh     = make(chan time.Time)
			statsCh    = make(chan autobuild.Stats)
			client, db = coderdtest.NewWithDatabase(t, &coderdtest.Options{
				AutobuildTicker:          tickCh,
				IncludeProvisionerDaemon: true,
				AutobuildStats:           statsCh,
			})
			admin = coderdtest.CreateFirstUser(t, client)
		)

		// Given: A task workspace with an 8 hour deadline
		template := createTaskTemplate(t, client, admin.OrganizationID, ctx, 8*time.Hour)
		workspace := createTaskWorkspace(t, client, template, ctx, "test task for autostop")

		// Given: The workspace is currently running
		workspace = coderdtest.MustWorkspace(t, client, workspace.ID)
		require.Equal(t, codersdk.WorkspaceTransitionStart, workspace.LatestBuild.Transition)
		require.NotZero(t, workspace.LatestBuild.Deadline, "workspace should have a deadline for autostop")

		p, err := coderdtest.GetProvisionerForTags(db, time.Now(), workspace.OrganizationID, map[string]string{})
		require.NoError(t, err)

		// When: the autobuild executor ticks after the deadline
		go func() {
			tickTime := workspace.LatestBuild.Deadline.Time.Add(time.Minute)
			coderdtest.UpdateProvisionerLastSeenAt(t, db, p.ID, tickTime)
			tickCh <- tickTime
			close(tickCh)
		}()

		// Then: We expect to see a stop transition
		stats := <-statsCh
		require.Len(t, stats.Transitions, 1, "lifecycle executor should transition the task workspace")
		assert.Contains(t, stats.Transitions, workspace.ID, "task workspace should be in transitions")
		assert.Equal(t, database.WorkspaceTransitionStop, stats.Transitions[workspace.ID], "should autostop the workspace")
		require.Empty(t, stats.Errors, "should have no errors when managing task workspaces")
	})
}

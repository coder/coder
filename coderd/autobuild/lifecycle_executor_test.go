package autobuild_test

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/autobuild"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/coderd/notifications/notificationstest"
	"github.com/coder/coder/v2/coderd/provisionerdserver"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/schedule"
	"github.com/coder/coder/v2/coderd/schedule/cron"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
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
	tickTime := coderdtest.NextAutostartTick(t, workspace)
	go func() {
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
	next := coderdtest.NextAutostartTick(t, workspace)
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

// uniqueViolationStore wraps a database.Store and injects a unique violation
// error from InsertWorkspaceBuild after a configurable number of successful
// calls. This simulates a concurrent build race (e.g. an API-driven start
// racing with the lifecycle executor autostart).
type uniqueViolationStore struct {
	database.Store
	insertCount *atomic.Int32 // pointer: shared across InTx copies
	failAfterN  int32
}

func newUniqueViolationStore(db database.Store, failAfterN int32) *uniqueViolationStore {
	return &uniqueViolationStore{
		Store:       db,
		insertCount: &atomic.Int32{},
		failAfterN:  failAfterN,
	}
}

func (s *uniqueViolationStore) InTx(fn func(database.Store) error, opts *database.TxOptions) error {
	return s.Store.InTx(func(tx database.Store) error {
		return fn(&uniqueViolationStore{
			Store:       tx,
			insertCount: s.insertCount, // shared pointer
			failAfterN:  s.failAfterN,
		})
	}, opts)
}

func (s *uniqueViolationStore) InsertWorkspaceBuild(ctx context.Context, arg database.InsertWorkspaceBuildParams) error {
	n := s.insertCount.Add(1)
	if n > s.failAfterN {
		return &pq.Error{
			Code:       pq.ErrorCode("23505"),
			Constraint: string(database.UniqueWorkspaceBuildsWorkspaceIDBuildNumberKey),
			Message:    `duplicate key value violates unique constraint "workspace_builds_workspace_id_build_number_key"`,
		}
	}
	return s.Store.InsertWorkspaceBuild(ctx, arg)
}

func TestExecutorBuildNumberRaceIsHandled(t *testing.T) {
	t.Parallel()

	// The lifecycle executor must handle a unique-violation from
	// InsertWorkspaceBuild gracefully. This error occurs when a concurrent
	// actor (API handler, another executor, prebuilds reconciler) inserts a
	// build with the same number before the executor's INSERT lands.
	//
	// We inject the error via a store wrapper. The first two
	// InsertWorkspaceBuild calls succeed (setup builds), then the third
	// (the lifecycle executor's autostart build) gets a unique violation.

	realDB, ps := dbtestutil.NewDB(t)
	wrappedDB := newUniqueViolationStore(realDB, 2) // Allow builds 1 (start) and 2 (stop); fail build 3 (autostart)

	var (
		sched, _ = cron.Weekly("CRON_TZ=UTC 0 * * * *")
		tickCh   = make(chan time.Time)
		statsCh  = make(chan autobuild.Stats)
		client   = coderdtest.New(t, &coderdtest.Options{
			IncludeProvisionerDaemon: true,
			AutobuildTicker:          tickCh,
			AutobuildStats:           statsCh,
			Database:                 wrappedDB,
			Pubsub:                   ps,
		})
		workspace = mustProvisionWorkspace(t, client, func(cwr *codersdk.CreateWorkspaceRequest) {
			cwr.AutostartSchedule = ptr.Ref(sched.String())
		})
	)

	workspace = coderdtest.MustTransitionWorkspace(t, client, workspace.ID, codersdk.WorkspaceTransitionStart, codersdk.WorkspaceTransitionStop)

	p, err := coderdtest.GetProvisionerForTags(realDB, time.Now(), workspace.OrganizationID, nil)
	require.NoError(t, err)
	next := coderdtest.NextAutostartTick(t, workspace)
	coderdtest.UpdateProvisionerLastSeenAt(t, realDB, p.ID, next)

	tickCh <- next
	stats := <-statsCh

	// The lifecycle executor should treat the unique violation as a benign
	// race, not as a hard error.
	assert.Empty(t, stats.Errors, "lifecycle executor should not report unique-violation as error")
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
					ProvisionGraph: []*proto.Response{{
						Type: &proto.Response_Graph{
							Graph: &proto.GraphComplete{
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
			tickTime := coderdtest.NextAutostartTick(t, workspace)
			go func() {
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

func TestExecutorAutostopAIAgentActivity(t *testing.T) {
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
	)

	// Given: we have a user with a task workspace.
	user := coderdtest.CreateFirstUser(t, client)
	r := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OrganizationID: user.OrganizationID,
		OwnerID:        user.UserID,
	}).WithTask(database.TaskTable{
		Name:   "test-task",
		Prompt: "AI agent activity test task",
	}, &proto.App{Slug: "test-app"}).Do()

	// Given: template has activity bump enabled.
	_, err := client.UpdateTemplateMeta(ctx, r.Template.ID, codersdk.UpdateTemplateMeta{
		DefaultTTLMillis:   ptr.Ref((2 * time.Hour).Milliseconds()),
		ActivityBumpMillis: ptr.Ref(time.Hour.Milliseconds()),
	})
	require.NoError(t, err)

	// Set deadline to past to meet 5% threshold for activity bump.
	now := time.Now()
	pastDeadline := now.Add(-30 * time.Minute)
	err = db.UpdateWorkspaceBuildDeadlineByID(dbauthz.AsSystemRestricted(ctx), database.UpdateWorkspaceBuildDeadlineByIDParams{
		ID:          r.Build.ID,
		UpdatedAt:   now,
		Deadline:    pastDeadline,
		MaxDeadline: time.Time{},
	})
	require.NoError(t, err)

	// Given: agent reports "working" status. ActivityBumpWorkspace uses the
	// database NOW(), so tick times below derive from the bumped deadline to
	// avoid minute-boundary truncation races.
	agentClient := agentsdk.New(client.URL, agentsdk.WithFixedToken(r.AgentToken))
	err = agentClient.PatchAppStatus(ctx, agentsdk.PatchAppStatus{
		AppSlug: "test-app",
		State:   codersdk.WorkspaceAppStatusStateWorking,
		Message: "AI agent is working",
	})
	require.NoError(t, err)

	// Anchor tick times to the database deadline, not the test clock.
	bumpedBuild, err := db.GetWorkspaceBuildByID(dbauthz.AsSystemRestricted(ctx), r.Build.ID)
	require.NoError(t, err)
	require.True(t, bumpedBuild.Deadline.After(now),
		"expected activity bump to push deadline into the future, got %s", bumpedBuild.Deadline)

	p, err := coderdtest.GetProvisionerForTags(db, time.Now(), r.Workspace.OrganizationID, nil)
	require.NoError(t, err)

	// When: the autobuild executor ticks before the bumped deadline.
	go func() {
		tickTime := bumpedBuild.Deadline.Add(-30 * time.Minute)
		coderdtest.UpdateProvisionerLastSeenAt(t, db, p.ID, tickTime)
		tickCh <- tickTime
	}()

	// Then: nothing should happen and the workspace should stay running.
	stats := <-statsCh
	require.Len(t, stats.Errors, 0)
	require.Len(t, stats.Transitions, 0)

	// Given: agent reports "complete" status. This invokes ActivityBumpWorkspace
	// again, but activitybump.sql only updates the deadline once more than 5% of
	// the activity_bump duration has elapsed since the last bump. We just bumped
	// milliseconds ago, so the UPDATE matches zero rows and the deadline is
	// unchanged.
	err = agentClient.PatchAppStatus(ctx, agentsdk.PatchAppStatus{
		AppSlug: "test-app",
		State:   codersdk.WorkspaceAppStatusStateComplete,
		Message: "AI agent completed",
	})
	require.NoError(t, err)

	// When: the autobuild executor ticks after the bumped deadline.
	// Adding a full minute ensures the truncated tick exceeds the deadline.
	go func() {
		tickTime := bumpedBuild.Deadline.Add(time.Minute)
		coderdtest.UpdateProvisionerLastSeenAt(t, db, p.ID, tickTime)
		tickCh <- tickTime
		close(tickCh)
	}()

	// Then: the workspace should be stopped.
	stats = <-statsCh
	require.Len(t, stats.Errors, 0)
	require.Len(t, stats.Transitions, 1)
	require.Contains(t, stats.Transitions, r.Workspace.ID)
	require.Equal(t, database.WorkspaceTransitionStop, stats.Transitions[r.Workspace.ID])
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
	tickTime := coderdtest.NextAutostartTick(t, workspace)
	go func() {
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
	tickTime := coderdtest.NextAutostartTick(t, workspace)
	go func() {
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
	err = db.UpdateTemplateAccessControlByID(dbauthz.As(ctx, coderdtest.AuthzUserSubject(me)), database.UpdateTemplateAccessControlByIDParams{
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
			ProvisionInit:  echo.InitComplete,
			ProvisionPlan:  echo.PlanComplete,
			ProvisionApply: echo.ApplyFailed,
			ProvisionGraph: echo.GraphComplete,
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

		// The template does not configure auto-delete, so the body must not
		// indicate a deletion timeline.
		require.NotContains(t, sent[0].Labels, "timeTilDelete")
		require.Equal(t, workspace.Name, sent[0].Labels["name"])
		require.Equal(t, "inactivity exceeded the dormancy threshold", sent[0].Labels["reason"])
	})

	t.Run("DormancyAutoDelete", func(t *testing.T) {
		t.Parallel()

		// Setup template with dormancy and auto-delete and create a workspace
		// with it. The two durations are intentionally far apart to reliably
		// check what's rendered in the notification.
		var (
			ticker    = make(chan time.Time)
			statCh    = make(chan autobuild.Stats)
			notifyEnq = notificationstest.FakeEnqueuer{}
			// 35 days is inside humanize.Time's "1 month" bucket (between 30 and 60 days).
			timeTilDormant           = time.Minute
			timeTilDormantAutoDelete = 35 * 24 * time.Hour
			client, db               = coderdtest.NewWithDatabase(t, &coderdtest.Options{
				AutobuildTicker:          ticker,
				AutobuildStats:           statCh,
				IncludeProvisionerDaemon: true,
				NotificationsEnqueuer:    &notifyEnq,
				TemplateScheduleStore: schedule.MockTemplateScheduleStore{
					SetFn: func(ctx context.Context, db database.Store, template database.Template, options schedule.TemplateScheduleOptions) (database.Template, error) {
						template.TimeTilDormant = int64(options.TimeTilDormant)
						template.TimeTilDormantAutoDelete = int64(options.TimeTilDormantAutoDelete)
						return schedule.NewAGPLTemplateScheduleStore().Set(ctx, db, template, options)
					},
					GetFn: func(_ context.Context, _ database.Store, _ uuid.UUID) (schedule.TemplateScheduleOptions, error) {
						return schedule.TemplateScheduleOptions{
							UserAutostartEnabled:     false,
							UserAutostopEnabled:      true,
							DefaultTTL:               0,
							AutostopRequirement:      schedule.TemplateAutostopRequirement{},
							TimeTilDormant:           timeTilDormant,
							TimeTilDormantAutoDelete: timeTilDormantAutoDelete,
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
			ctr.TimeTilDormantAutoDeleteMillis = ptr.Ref(timeTilDormantAutoDelete.Milliseconds())
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

		// The notification body should render the deletion countdown using the template's
		// `time_til_dormant_autodelete` value. With auto-delete at 35 days and dormancy
		// at 1 minute, humanize.Time renders the label as "1 month from now".
		sent := notifyEnq.Sent()
		require.Len(t, sent, 1)
		require.Equal(t, sent[0].TemplateID, notifications.TemplateWorkspaceDormant)
		require.Contains(t, sent[0].Labels, "timeTilDelete")
		require.Contains(t, sent[0].Labels["timeTilDelete"], "1 month",
			"timeTilDelete must humanize TimeTilDormantAutoDelete, got %q",
			sent[0].Labels["timeTilDelete"])
		require.NotContains(t, sent[0].Labels["timeTilDelete"], "ago",
			"timeTilDelete must be a future timestamp, got %q",
			sent[0].Labels["timeTilDelete"])
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

// setupAutostopReminderWorkspace provisions a running workspace whose template
// has the given time_til_autostop_notify configured, using the caller-supplied
// notifications enqueuer. It returns the harness channels needed to drive ticks
// and observe notifications.
func setupAutostopReminderWorkspace(t *testing.T, timeTilAutostopNotify time.Duration, enq notifications.Enqueuer) (
	client *codersdk.Client,
	db database.Store,
	tickCh chan time.Time,
	statsCh chan autobuild.Stats,
	workspace codersdk.Workspace,
) {
	t.Helper()

	tickCh = make(chan time.Time)
	statsCh = make(chan autobuild.Stats)
	client, db = coderdtest.NewWithDatabase(t, &coderdtest.Options{
		AutobuildTicker:          tickCh,
		AutobuildStats:           statsCh,
		IncludeProvisionerDaemon: true,
		NotificationsEnqueuer:    enq,
		// The AGPL schedule store persists and returns time_til_autostop_notify.
		TemplateScheduleStore: schedule.NewAGPLTemplateScheduleStore(),
	})

	user := coderdtest.CreateFirstUser(t, client)
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
		if timeTilAutostopNotify > 0 {
			ctr.TimeTilAutostopNotifyMillis = ptr.Ref(timeTilAutostopNotify.Milliseconds())
		}
	})
	ws := coderdtest.CreateWorkspace(t, client, template.ID)
	coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, ws.LatestBuild.ID)
	workspace = coderdtest.MustWorkspace(t, client, ws.ID)

	// The build must have a non-zero deadline for a reminder to ever fire.
	require.Equal(t, codersdk.WorkspaceTransitionStart, workspace.LatestBuild.Transition)
	require.NotZero(t, workspace.LatestBuild.Deadline)

	// Age last_used_at far before every tick so the active-user guard never
	// trips for the default subtests. A freshly created workspace has a recent
	// last_used_at, which would otherwise look "active" and suppress the
	// reminder. Subtests that exercise the active-user guard reset last_used_at
	// to a recent value via db.
	ctx := dbauthz.AsSystemRestricted(context.Background())
	require.NoError(t, db.UpdateWorkspaceLastUsedAt(ctx, database.UpdateWorkspaceLastUsedAtParams{
		ID:         workspace.ID,
		LastUsedAt: workspace.LatestBuild.Deadline.Time.Add(-365 * 24 * time.Hour),
	}))

	return client, db, tickCh, statsCh, workspace
}

// failOnceEnqueuer fails its first Enqueue call and delegates every subsequent
// call to the wrapped enqueuer. It is used by the FailedEnqueueNotRetried
// subtest to verify that a failed reminder enqueue is not retried (the
// at-most-once guarantee); notificationstest.FakeEnqueuer.Enqueue always
// succeeds, so this wrapper is the only way to inject a send failure.
type failOnceEnqueuer struct {
	notifications.Enqueuer
	mu     sync.Mutex
	failed bool
}

func (f *failOnceEnqueuer) Enqueue(ctx context.Context, userID, templateID uuid.UUID, labels map[string]string, createdBy string, targets ...uuid.UUID) ([]uuid.UUID, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.failed {
		f.failed = true
		return nil, xerrors.New("injected enqueue failure")
	}
	return f.Enqueuer.Enqueue(ctx, userID, templateID, labels, createdBy, targets...)
}

func TestExecutorAutostopReminder(t *testing.T) {
	t.Parallel()

	// Sent: a reminder is enqueued when a tick lands inside the lead window
	// [deadline - ttl, deadline).
	t.Run("Sent", func(t *testing.T) {
		t.Parallel()

		timeTilNotify := 30 * time.Minute
		notifyEnq := &notificationstest.FakeEnqueuer{}
		_, _, tickCh, statsCh, workspace := setupAutostopReminderWorkspace(t, timeTilNotify, notifyEnq)
		deadline := workspace.LatestBuild.Deadline.Time

		go func() {
			// Halfway into the lead window.
			tickCh <- deadline.Add(-timeTilNotify / 2)
			close(tickCh)
		}()

		stats := testutil.TryReceive(testutil.Context(t, testutil.WaitShort), t, statsCh)
		require.Len(t, stats.Errors, 0)
		require.Len(t, stats.Transitions, 0)

		sent := notifyEnq.Sent(notificationstest.WithTemplateID(notifications.TemplateWorkspaceAutostopReminder))
		require.Len(t, sent, 1)
		require.Equal(t, workspace.OwnerID, sent[0].UserID)
		require.Equal(t, workspace.Name, sent[0].Labels["workspace"])
		require.NotEmpty(t, sent[0].Labels["timeTilShutdown"])
		require.Contains(t, sent[0].Targets, workspace.ID)
		require.Contains(t, sent[0].Targets, workspace.OwnerID)
		require.Contains(t, sent[0].Targets, workspace.TemplateID)
		require.Contains(t, sent[0].Targets, workspace.OrganizationID)
	})

	// ActiveWorkspaceNotReminded: a workspace used within the 15-minute active
	// threshold keeps getting its deadline bumped, so no reminder is sent even
	// though the tick lands inside the window. This is the active-user guard, the
	// exact complement of the Sent subtest.
	t.Run("ActiveWorkspaceNotReminded", func(t *testing.T) {
		t.Parallel()

		timeTilNotify := 30 * time.Minute
		notifyEnq := &notificationstest.FakeEnqueuer{}
		_, db, tickCh, statsCh, workspace := setupAutostopReminderWorkspace(t, timeTilNotify, notifyEnq)
		deadline := workspace.LatestBuild.Deadline.Time

		// Tick halfway into the lead window, exactly as the Sent subtest does.
		tick := deadline.Add(-timeTilNotify / 2)

		// Mark the workspace as recently used: last_used_at within the 15-minute
		// active threshold of the tick makes currentTick - last_used_at <
		// autostopReminderActiveThreshold, so the active-user guard suppresses the
		// reminder (and the SQL pre-filter drops the row).
		ctx := dbauthz.AsSystemRestricted(context.Background())
		require.NoError(t, db.UpdateWorkspaceLastUsedAt(ctx, database.UpdateWorkspaceLastUsedAtParams{
			ID:         workspace.ID,
			LastUsedAt: tick.Add(-time.Minute),
		}))

		go func() {
			tickCh <- tick
			close(tickCh)
		}()

		stats := testutil.TryReceive(testutil.Context(t, testutil.WaitShort), t, statsCh)
		require.Len(t, stats.Errors, 0)
		require.Empty(t, notifyEnq.Sent(notificationstest.WithTemplateID(notifications.TemplateWorkspaceAutostopReminder)))
	})

	// ActiveWorkspaceAtMaxDeadlineReminded: an active workspace is still
	// reminded when the hard max_deadline ceiling sits inside the lead window.
	// Activity bumps cannot push the stop past max_deadline, so the workspace
	// will stop regardless of activity and the reminder must fire. This is the
	// max_deadline override of the active-user guard.
	t.Run("ActiveWorkspaceAtMaxDeadlineReminded", func(t *testing.T) {
		t.Parallel()

		ctx := dbauthz.AsSystemRestricted(context.Background())
		timeTilNotify := 30 * time.Minute
		notifyEnq := &notificationstest.FakeEnqueuer{}
		_, db, tickCh, statsCh, workspace := setupAutostopReminderWorkspace(t, timeTilNotify, notifyEnq)
		deadline := workspace.LatestBuild.Deadline.Time

		// Tick halfway into the lead window, exactly as the Sent subtest does.
		tick := deadline.Add(-timeTilNotify / 2)

		// Mark the workspace as recently used (active): without the max_deadline
		// ceiling this would suppress the reminder, see ActiveWorkspaceNotReminded.
		require.NoError(t, db.UpdateWorkspaceLastUsedAt(ctx, database.UpdateWorkspaceLastUsedAtParams{
			ID:         workspace.ID,
			LastUsedAt: tick.Add(-time.Minute),
		}))

		// Pin the build's max_deadline inside the lead window (max_deadline <=
		// tick + ttl). A bump cannot move the stop past this ceiling, so the
		// workspace will stop even though the user is active and the reminder
		// must still fire. The deadline itself is left unchanged.
		require.NoError(t, db.UpdateWorkspaceBuildDeadlineByID(ctx, database.UpdateWorkspaceBuildDeadlineByIDParams{
			ID:          workspace.LatestBuild.ID,
			Deadline:    deadline,
			MaxDeadline: deadline,
			UpdatedAt:   tick,
		}))

		go func() {
			tickCh <- tick
			close(tickCh)
		}()

		stats := testutil.TryReceive(testutil.Context(t, testutil.WaitShort), t, statsCh)
		require.Len(t, stats.Errors, 0)

		sent := notifyEnq.Sent(notificationstest.WithTemplateID(notifications.TemplateWorkspaceAutostopReminder))
		require.Len(t, sent, 1)
		require.Equal(t, workspace.OwnerID, sent[0].UserID)
	})

	// NotBeforeWindow: no reminder when the tick precedes the lead window.
	t.Run("NotBeforeWindow", func(t *testing.T) {
		t.Parallel()

		timeTilNotify := 30 * time.Minute
		notifyEnq := &notificationstest.FakeEnqueuer{}
		_, _, tickCh, statsCh, workspace := setupAutostopReminderWorkspace(t, timeTilNotify, notifyEnq)
		deadline := workspace.LatestBuild.Deadline.Time

		go func() {
			// Well before the window opens.
			tickCh <- deadline.Add(-2 * timeTilNotify)
			close(tickCh)
		}()

		stats := testutil.TryReceive(testutil.Context(t, testutil.WaitShort), t, statsCh)
		require.Len(t, stats.Errors, 0)
		require.Empty(t, notifyEnq.Sent(notificationstest.WithTemplateID(notifications.TemplateWorkspaceAutostopReminder)))
	})

	// Disabled: time_til_autostop_notify of 0 (the default) never reminds.
	t.Run("Disabled", func(t *testing.T) {
		t.Parallel()

		notifyEnq := &notificationstest.FakeEnqueuer{}
		_, _, tickCh, statsCh, workspace := setupAutostopReminderWorkspace(t, 0, notifyEnq)
		deadline := workspace.LatestBuild.Deadline.Time

		go func() {
			tickCh <- deadline.Add(-time.Minute)
			close(tickCh)
		}()

		stats := testutil.TryReceive(testutil.Context(t, testutil.WaitShort), t, statsCh)
		require.Len(t, stats.Errors, 0)
		require.Empty(t, notifyEnq.Sent(notificationstest.WithTemplateID(notifications.TemplateWorkspaceAutostopReminder)))
	})

	// NoDuplicate: a second tick still inside the window does not re-notify
	// because the idempotence marker was stamped.
	t.Run("NoDuplicate", func(t *testing.T) {
		t.Parallel()

		timeTilNotify := 30 * time.Minute
		notifyEnq := &notificationstest.FakeEnqueuer{}
		_, _, tickCh, statsCh, workspace := setupAutostopReminderWorkspace(t, timeTilNotify, notifyEnq)
		deadline := workspace.LatestBuild.Deadline.Time

		// First tick: reminder fires. Receiving from statsCh acts as the
		// per-tick barrier guaranteeing the enqueue already happened.
		go func() {
			tickCh <- deadline.Add(-timeTilNotify / 2)
		}()
		testutil.TryReceive(testutil.Context(t, testutil.WaitShort), t, statsCh)
		require.Len(t, notifyEnq.Sent(notificationstest.WithTemplateID(notifications.TemplateWorkspaceAutostopReminder)), 1)

		// Second tick still inside the window: no new reminder. Sent()
		// accumulates across ticks, so a cumulative count still at 1 proves
		// the duplicate was suppressed.
		go func() {
			tickCh <- deadline.Add(-timeTilNotify / 4)
			close(tickCh)
		}()
		testutil.TryReceive(testutil.Context(t, testutil.WaitShort), t, statsCh)
		require.Len(t, notifyEnq.Sent(notificationstest.WithTemplateID(notifications.TemplateWorkspaceAutostopReminder)), 1)
	})

	// DeadlineBumped: extending the deadline re-arms the marker, so a new
	// reminder fires once the new deadline re-enters the window.
	t.Run("DeadlineBumped", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		timeTilNotify := 30 * time.Minute
		notifyEnq := &notificationstest.FakeEnqueuer{}
		client, _, tickCh, statsCh, workspace := setupAutostopReminderWorkspace(t, timeTilNotify, notifyEnq)
		deadline := workspace.LatestBuild.Deadline.Time

		// First tick: reminder fires for the original deadline.
		go func() {
			tickCh <- deadline.Add(-timeTilNotify / 2)
		}()
		testutil.TryReceive(ctx, t, statsCh)
		sent := notifyEnq.Sent(notificationstest.WithTemplateID(notifications.TemplateWorkspaceAutostopReminder))
		require.Len(t, sent, 1)
		require.NotEmpty(t, sent[0].Labels["timeTilShutdown"])

		// Move the deadline well into the future. The marker now differs from
		// the build deadline, re-arming the reminder.
		newDeadline := deadline.Add(2 * time.Hour)
		require.NoError(t, client.PutExtendWorkspace(ctx, workspace.ID, codersdk.PutExtendWorkspaceRequest{
			Deadline: newDeadline,
		}))

		// Second tick inside the new window fires another reminder. Sent()
		// accumulates across ticks, so two total proves the second reminder
		// fired; sent[1] carries the bumped deadline.
		go func() {
			tickCh <- newDeadline.Add(-timeTilNotify / 2)
			close(tickCh)
		}()
		testutil.TryReceive(ctx, t, statsCh)
		sent = notifyEnq.Sent(notificationstest.WithTemplateID(notifications.TemplateWorkspaceAutostopReminder))
		require.Len(t, sent, 2)
		require.NotEmpty(t, sent[1].Labels["timeTilShutdown"])
	})

	// ExceedsLifetime: a time_til_autostop_notify larger than the
	// workspace's remaining lifetime yields exactly one reminder, not one per
	// tick.
	t.Run("ExceedsLifetime", func(t *testing.T) {
		t.Parallel()

		// Far larger than the workspace's 8h TTL, so the lead window already
		// includes "now" at build creation.
		timeTilNotify := 100 * time.Hour
		notifyEnq := &notificationstest.FakeEnqueuer{}
		_, _, tickCh, statsCh, workspace := setupAutostopReminderWorkspace(t, timeTilNotify, notifyEnq)
		deadline := workspace.LatestBuild.Deadline.Time

		// First tick: a single reminder fires.
		go func() {
			tickCh <- deadline.Add(-time.Hour)
		}()
		testutil.TryReceive(testutil.Context(t, testutil.WaitShort), t, statsCh)
		require.Len(t, notifyEnq.Sent(notificationstest.WithTemplateID(notifications.TemplateWorkspaceAutostopReminder)), 1)

		// Second tick still before the deadline: no flood of reminders. Sent()
		// accumulates across ticks, so a cumulative count still at 1 proves no
		// duplicate fired.
		go func() {
			tickCh <- deadline.Add(-30 * time.Minute)
			close(tickCh)
		}()
		testutil.TryReceive(testutil.Context(t, testutil.WaitShort), t, statsCh)
		require.Len(t, notifyEnq.Sent(notificationstest.WithTemplateID(notifications.TemplateWorkspaceAutostopReminder)), 1)
	})

	// FailedEnqueueNotRetried pins the marker-before-enqueue / at-most-once
	// guarantee: the marker is committed inside the transaction before the
	// post-commit enqueue, so a failed enqueue on the first tick is NOT
	// retried on a later tick even though the workspace is still inside the
	// lead window. failOnceEnqueuer injects that single send failure;
	// notificationstest.FakeEnqueuer.Enqueue always succeeds.
	t.Run("FailedEnqueueNotRetried", func(t *testing.T) {
		t.Parallel()

		fake := &notificationstest.FakeEnqueuer{}
		enq := &failOnceEnqueuer{Enqueuer: fake}
		timeTilNotify := 2 * time.Hour
		_, _, tickCh, statsCh, workspace := setupAutostopReminderWorkspace(t, timeTilNotify, enq)
		deadline := workspace.LatestBuild.Deadline.Time

		// Tick 1 inside the window: the enqueue fails. Because the marker is
		// stamped before the enqueue, the failure only logs and nothing is
		// sent.
		go func() {
			tickCh <- deadline.Add(-time.Hour)
		}()
		testutil.TryReceive(testutil.Context(t, testutil.WaitShort), t, statsCh)
		require.Len(t, fake.Sent(notificationstest.WithTemplateID(notifications.TemplateWorkspaceAutostopReminder)), 0)

		// Tick 2 still inside the window: the committed marker suppresses
		// re-selection, so the failed reminder is NOT retried. A cumulative
		// count still at 0 proves the at-most-once guarantee described at the
		// enqueue block in lifecycle_executor.go.
		go func() {
			tickCh <- deadline.Add(-time.Hour + time.Minute)
			close(tickCh)
		}()
		testutil.TryReceive(testutil.Context(t, testutil.WaitShort), t, statsCh)
		require.Len(t, fake.Sent(notificationstest.WithTemplateID(notifications.TemplateWorkspaceAutostopReminder)), 0)
	})
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
		ProvisionGraph: []*proto.Response{
			{
				Type: &proto.Response_Graph{
					Graph: &proto.GraphComplete{
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

	next = coderdtest.NextAutostartTick(t, workspace)
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
			ProvisionGraph: []*proto.Response{
				{
					Type: &proto.Response_Graph{
						Graph: &proto.GraphComplete{
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
							HasAiTasks: true,
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
				DefaultTTLMillis: ptr.Ref(defaultTTL.Milliseconds()),
			})
			require.NoError(t, err)
		}

		return template
	}

	createTaskWorkspace := func(t *testing.T, client *codersdk.Client, template codersdk.Template, ctx context.Context, input string) codersdk.Workspace {
		t.Helper()

		task, err := client.CreateTask(ctx, "me", codersdk.CreateTaskRequest{
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
		tickTime := coderdtest.NextAutostartTick(t, workspace)
		go func() {
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

		// Then: The build reason should be TaskAutoPause (not regular Autostop)
		workspace = coderdtest.MustWorkspace(t, client, workspace.ID)
		_ = coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)
		workspace = coderdtest.MustWorkspace(t, client, workspace.ID)
		assert.Equal(t, codersdk.BuildReasonTaskAutoPause, workspace.LatestBuild.Reason, "task workspace should use TaskAutoPause build reason")
	})

	t.Run("AutostopNotification", func(t *testing.T) {
		t.Parallel()

		var (
			tickCh     = make(chan time.Time)
			statsCh    = make(chan autobuild.Stats)
			notifyEnq  = notificationstest.FakeEnqueuer{}
			client, db = coderdtest.NewWithDatabase(t, &coderdtest.Options{
				AutobuildTicker:          tickCh,
				IncludeProvisionerDaemon: true,
				AutobuildStats:           statsCh,
				NotificationsEnqueuer:    &notifyEnq,
			})
			admin = coderdtest.CreateFirstUser(t, client)
		)

		// Given: A task workspace with an 8 hour deadline
		ctx := testutil.Context(t, testutil.WaitShort)
		template := createTaskTemplate(t, client, admin.OrganizationID, ctx, 8*time.Hour)
		workspace := createTaskWorkspace(t, client, template, ctx, "test task for autostop notification")

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

		// Then: A task paused notification was sent with "idle timeout" reason
		require.True(t, workspace.TaskID.Valid, "workspace should have a task ID")
		task, err := db.GetTaskByID(dbauthz.AsSystemRestricted(ctx), workspace.TaskID.UUID)
		require.NoError(t, err)

		sent := notifyEnq.Sent(notificationstest.WithTemplateID(notifications.TemplateTaskPaused))
		require.Len(t, sent, 1)
		require.Equal(t, workspace.OwnerID, sent[0].UserID)
		require.Equal(t, task.Name, sent[0].Labels["task"])
		require.Equal(t, task.ID.String(), sent[0].Labels["task_id"])
		require.Equal(t, workspace.Name, sent[0].Labels["workspace"])
		require.Equal(t, "idle timeout", sent[0].Labels["pause_reason"])
	})
}

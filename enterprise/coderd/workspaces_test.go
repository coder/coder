package coderd_test

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/v2/coderd/autobuild"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	agplschedule "github.com/coder/coder/v2/coderd/schedule"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/enterprise/coderd/schedule"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/testutil"
)

// agplUserQuietHoursScheduleStore is passed to
// NewEnterpriseTemplateScheduleStore as we don't care about updating the
// schedule and having it recalculate the build deadline in these tests.
func agplUserQuietHoursScheduleStore() *atomic.Pointer[agplschedule.UserQuietHoursScheduleStore] {
	store := agplschedule.NewAGPLUserQuietHoursScheduleStore()
	p := &atomic.Pointer[agplschedule.UserQuietHoursScheduleStore]{}
	p.Store(&store)
	return p
}

func TestCreateWorkspace(t *testing.T) {
	t.Parallel()

	// Test that a user cannot indirectly access
	// a template they do not have access to.
	t.Run("Unauthorized", func(t *testing.T) {
		t.Parallel()

		client, user := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		}})

		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		acl, err := client.TemplateACL(ctx, template.ID)
		require.NoError(t, err)

		require.Len(t, acl.Groups, 1)
		require.Len(t, acl.Users, 0)

		err = client.UpdateTemplateACL(ctx, template.ID, codersdk.UpdateTemplateACL{
			GroupPerms: map[string]codersdk.TemplateRole{
				acl.Groups[0].ID.String(): codersdk.TemplateRoleDeleted,
			},
		})
		require.NoError(t, err)

		client1, user1 := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)

		_, err = client1.Template(ctx, template.ID)
		require.Error(t, err)
		cerr, ok := codersdk.AsError(err)
		require.True(t, ok)
		require.Equal(t, http.StatusNotFound, cerr.StatusCode())

		req := codersdk.CreateWorkspaceRequest{
			TemplateID:        template.ID,
			Name:              "testme",
			AutostartSchedule: ptr.Ref("CRON_TZ=US/Central 30 9 * * 1-5"),
			TTLMillis:         ptr.Ref((8 * time.Hour).Milliseconds()),
		}

		_, err = client1.CreateWorkspace(ctx, user.OrganizationID, user1.ID.String(), req)
		require.Error(t, err)
	})
}

func TestWorkspaceAutobuild(t *testing.T) {
	t.Parallel()

	t.Run("FailureTTLOK", func(t *testing.T) {
		t.Parallel()

		var (
			ticker = make(chan time.Time)
			statCh = make(chan autobuild.Stats)
			logger = slogtest.Make(t, &slogtest.Options{
				// We ignore errors here since we expect to fail
				// builds.
				IgnoreErrors: true,
			})
			failureTTL = time.Minute
		)

		client, user := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				Logger:                   &logger,
				AutobuildTicker:          ticker,
				IncludeProvisionerDaemon: true,
				AutobuildStats:           statCh,
				TemplateScheduleStore:    schedule.NewEnterpriseTemplateScheduleStore(agplUserQuietHoursScheduleStore()),
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{codersdk.FeatureAdvancedTemplateScheduling: 1},
			},
		})

		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionPlan:  echo.ProvisionComplete,
			ProvisionApply: echo.ProvisionFailed,
		})
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.FailureTTLMillis = ptr.Ref[int64](failureTTL.Milliseconds())
		})
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		ws := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		build := coderdtest.AwaitWorkspaceBuildJob(t, client, ws.LatestBuild.ID)
		require.Equal(t, codersdk.WorkspaceStatusFailed, build.Status)
		ticker <- build.Job.CompletedAt.Add(failureTTL * 2)
		stats := <-statCh
		// Expect workspace to transition to stopped state for breaching
		// failure TTL.
		require.Len(t, stats.Transitions, 1)
		require.Equal(t, stats.Transitions[ws.ID], database.WorkspaceTransitionStop)
	})

	t.Run("FailureTTLTooEarly", func(t *testing.T) {
		t.Parallel()

		var (
			ticker = make(chan time.Time)
			statCh = make(chan autobuild.Stats)
			logger = slogtest.Make(t, &slogtest.Options{
				// We ignore errors here since we expect to fail
				// builds.
				IgnoreErrors: true,
			})
			failureTTL = time.Minute
		)

		client, user := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				Logger:                   &logger,
				AutobuildTicker:          ticker,
				IncludeProvisionerDaemon: true,
				AutobuildStats:           statCh,
				TemplateScheduleStore:    schedule.NewEnterpriseTemplateScheduleStore(agplUserQuietHoursScheduleStore()),
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{codersdk.FeatureAdvancedTemplateScheduling: 1},
			},
		})
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionPlan:  echo.ProvisionComplete,
			ProvisionApply: echo.ProvisionFailed,
		})
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.FailureTTLMillis = ptr.Ref[int64](failureTTL.Milliseconds())
		})
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		ws := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		build := coderdtest.AwaitWorkspaceBuildJob(t, client, ws.LatestBuild.ID)
		require.Equal(t, codersdk.WorkspaceStatusFailed, build.Status)
		// Make it impossible to trigger the failure TTL.
		ticker <- build.Job.CompletedAt.Add(-failureTTL * 2)
		stats := <-statCh
		// Expect no transitions since not enough time has elapsed.
		require.Len(t, stats.Transitions, 0)
	})

	// This just provides a baseline that no actions are being taken
	// against a workspace when none of the TTL fields are set.
	t.Run("TemplateTTLsUnset", func(t *testing.T) {
		t.Parallel()

		var (
			ticker = make(chan time.Time)
			statCh = make(chan autobuild.Stats)
			logger = slogtest.Make(t, &slogtest.Options{
				// We ignore errors here since we expect to fail
				// builds.
				IgnoreErrors: true,
			})
		)

		client, user := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				Logger:                   &logger,
				AutobuildTicker:          ticker,
				IncludeProvisionerDaemon: true,
				AutobuildStats:           statCh,
				TemplateScheduleStore:    schedule.NewEnterpriseTemplateScheduleStore(agplUserQuietHoursScheduleStore()),
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{codersdk.FeatureAdvancedTemplateScheduling: 1},
			},
		})
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionPlan:  echo.ProvisionComplete,
			ProvisionApply: echo.ProvisionComplete,
		})
		// Create a template without setting a failure_ttl.
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		require.Zero(t, template.TimeTilDormantMillis)
		require.Zero(t, template.FailureTTLMillis)
		require.Zero(t, template.TimeTilDormantAutoDeleteMillis)

		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		ws := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		build := coderdtest.AwaitWorkspaceBuildJob(t, client, ws.LatestBuild.ID)
		require.Equal(t, codersdk.WorkspaceStatusRunning, build.Status)
		ticker <- time.Now()
		stats := <-statCh
		// Expect no transitions since the fields are unset on the template.
		require.Len(t, stats.Transitions, 0)
	})

	t.Run("InactiveTTLOK", func(t *testing.T) {
		t.Parallel()

		var (
			ctx         = testutil.Context(t, testutil.WaitMedium)
			ticker      = make(chan time.Time)
			statCh      = make(chan autobuild.Stats)
			inactiveTTL = time.Minute
		)

		client, user := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				AutobuildTicker:          ticker,
				IncludeProvisionerDaemon: true,
				AutobuildStats:           statCh,
				TemplateScheduleStore:    schedule.NewEnterpriseTemplateScheduleStore(agplUserQuietHoursScheduleStore()),
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{codersdk.FeatureAdvancedTemplateScheduling: 1},
			},
		})

		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionPlan:  echo.ProvisionComplete,
			ProvisionApply: echo.ProvisionComplete,
		})
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.TimeTilDormantMillis = ptr.Ref[int64](inactiveTTL.Milliseconds())
		})
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)

		ws := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		build := coderdtest.AwaitWorkspaceBuildJob(t, client, ws.LatestBuild.ID)
		require.Equal(t, codersdk.WorkspaceStatusRunning, build.Status)
		// Simulate being inactive.
		ticker <- ws.LastUsedAt.Add(inactiveTTL * 2)
		stats := <-statCh

		// Expect workspace to transition to stopped state for breaching
		// failure TTL.
		require.Len(t, stats.Transitions, 1)
		require.Equal(t, stats.Transitions[ws.ID], database.WorkspaceTransitionStop)

		// The workspace should be dormant.
		ws = coderdtest.MustWorkspace(t, client, ws.ID)
		require.NotNil(t, ws.DormantAt)
		lastUsedAt := ws.LastUsedAt

		err := client.UpdateWorkspaceDormancy(ctx, ws.ID, codersdk.UpdateWorkspaceDormancy{Dormant: false})
		require.NoError(t, err)

		// Assert that we updated our last_used_at so that we don't immediately
		// retrigger another lock action.
		ws = coderdtest.MustWorkspace(t, client, ws.ID)
		require.True(t, ws.LastUsedAt.After(lastUsedAt))
	})

	t.Run("InactiveTTLTooEarly", func(t *testing.T) {
		t.Parallel()

		var (
			ticker      = make(chan time.Time)
			statCh      = make(chan autobuild.Stats)
			inactiveTTL = time.Minute
		)

		client, user := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				AutobuildTicker:          ticker,
				IncludeProvisionerDaemon: true,
				AutobuildStats:           statCh,
				TemplateScheduleStore:    schedule.NewEnterpriseTemplateScheduleStore(agplUserQuietHoursScheduleStore()),
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{codersdk.FeatureAdvancedTemplateScheduling: 1},
			},
		})
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionPlan:  echo.ProvisionComplete,
			ProvisionApply: echo.ProvisionComplete,
		})
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.TimeTilDormantMillis = ptr.Ref[int64](inactiveTTL.Milliseconds())
		})
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		ws := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		build := coderdtest.AwaitWorkspaceBuildJob(t, client, ws.LatestBuild.ID)
		require.Equal(t, codersdk.WorkspaceStatusRunning, build.Status)
		// Make it impossible to trigger the inactive ttl.
		ticker <- ws.LastUsedAt.Add(-inactiveTTL)
		stats := <-statCh
		// Expect no transitions since not enough time has elapsed.
		require.Len(t, stats.Transitions, 0)
	})

	// This is kind of a dumb test but it exists to offer some marginal
	// confidence that a bug in the auto-deletion logic doesn't delete running
	// workspaces.
	t.Run("ActiveWorkspacesNotDeleted", func(t *testing.T) {
		t.Parallel()

		var (
			ticker        = make(chan time.Time)
			statCh        = make(chan autobuild.Stats)
			autoDeleteTTL = time.Minute
		)

		client, user := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				AutobuildTicker:          ticker,
				IncludeProvisionerDaemon: true,
				AutobuildStats:           statCh,
				TemplateScheduleStore:    schedule.NewEnterpriseTemplateScheduleStore(agplUserQuietHoursScheduleStore()),
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{codersdk.FeatureAdvancedTemplateScheduling: 1},
			},
		})
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionPlan:  echo.ProvisionComplete,
			ProvisionApply: echo.ProvisionComplete,
		})
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.TimeTilDormantAutoDeleteMillis = ptr.Ref[int64](autoDeleteTTL.Milliseconds())
		})
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		ws := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		build := coderdtest.AwaitWorkspaceBuildJob(t, client, ws.LatestBuild.ID)
		require.Nil(t, ws.DormantAt)
		require.Equal(t, codersdk.WorkspaceStatusRunning, build.Status)
		ticker <- ws.LastUsedAt.Add(autoDeleteTTL * 2)
		stats := <-statCh
		// Expect no transitions since workspace is active.
		require.Len(t, stats.Transitions, 0)
	})

	// Assert that a stopped workspace that breaches the inactivity threshold
	// does not trigger a build transition but is still placed in the
	// lock state.
	t.Run("InactiveStoppedWorkspaceNoTransition", func(t *testing.T) {
		t.Parallel()

		var (
			ticker      = make(chan time.Time)
			statCh      = make(chan autobuild.Stats)
			inactiveTTL = time.Minute
		)

		client, user := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				AutobuildTicker:          ticker,
				IncludeProvisionerDaemon: true,
				AutobuildStats:           statCh,
				TemplateScheduleStore:    schedule.NewEnterpriseTemplateScheduleStore(agplUserQuietHoursScheduleStore()),
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{codersdk.FeatureAdvancedTemplateScheduling: 1},
			},
		})
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionPlan:  echo.ProvisionComplete,
			ProvisionApply: echo.ProvisionComplete,
		})
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.TimeTilDormantMillis = ptr.Ref[int64](inactiveTTL.Milliseconds())
		})
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)

		ws := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		build := coderdtest.AwaitWorkspaceBuildJob(t, client, ws.LatestBuild.ID)
		require.Equal(t, codersdk.WorkspaceStatusRunning, build.Status)

		// Stop the workspace so we can assert autobuild does nothing
		// if we breach our inactivity threshold.
		ws = coderdtest.MustTransitionWorkspace(t, client, ws.ID, database.WorkspaceTransitionStart, database.WorkspaceTransitionStop)

		// Simulate not having accessed the workspace in a while.
		ticker <- ws.LastUsedAt.Add(2 * inactiveTTL)
		stats := <-statCh
		// Expect no transitions since workspace is stopped.
		require.Len(t, stats.Transitions, 0)
		ws = coderdtest.MustWorkspace(t, client, ws.ID)
		// The workspace should still be dormant even though we didn't
		// transition the workspace.
		require.NotNil(t, ws.DormantAt)
	})

	// Test the flow of a workspace transitioning from
	// inactive -> dormant -> deleted.
	t.Run("WorkspaceInactiveDeleteTransition", func(t *testing.T) {
		t.Parallel()

		var (
			ticker        = make(chan time.Time)
			statCh        = make(chan autobuild.Stats)
			transitionTTL = time.Minute
		)

		client, user := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				AutobuildTicker:          ticker,
				IncludeProvisionerDaemon: true,
				AutobuildStats:           statCh,
				TemplateScheduleStore:    schedule.NewEnterpriseTemplateScheduleStore(agplUserQuietHoursScheduleStore()),
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{codersdk.FeatureAdvancedTemplateScheduling: 1},
			},
		})

		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionPlan:  echo.ProvisionComplete,
			ProvisionApply: echo.ProvisionComplete,
		})
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.TimeTilDormantMillis = ptr.Ref[int64](transitionTTL.Milliseconds())
			ctr.TimeTilDormantAutoDeleteMillis = ptr.Ref[int64](transitionTTL.Milliseconds())
		})
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)

		ws := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		build := coderdtest.AwaitWorkspaceBuildJob(t, client, ws.LatestBuild.ID)
		require.Equal(t, codersdk.WorkspaceStatusRunning, build.Status)

		// Simulate not having accessed the workspace in a while.
		ticker <- ws.LastUsedAt.Add(2 * transitionTTL)
		stats := <-statCh
		// Expect workspace to transition to stopped state for breaching
		// inactive TTL.
		require.Len(t, stats.Transitions, 1)
		require.Equal(t, stats.Transitions[ws.ID], database.WorkspaceTransitionStop)

		ws = coderdtest.MustWorkspace(t, client, ws.ID)
		// The workspace should be dormant.
		require.NotNil(t, ws.DormantAt)

		// Wait for the autobuilder to stop the workspace.
		_ = coderdtest.AwaitWorkspaceBuildJob(t, client, ws.LatestBuild.ID)

		// Simulate the workspace being dormant beyond the threshold.
		ticker <- ws.DormantAt.Add(2 * transitionTTL)
		stats = <-statCh
		require.Len(t, stats.Transitions, 1)
		// The workspace should be scheduled for deletion.
		require.Equal(t, stats.Transitions[ws.ID], database.WorkspaceTransitionDelete)

		// Wait for the workspace to be deleted.
		ws = coderdtest.MustWorkspace(t, client, ws.ID)
		_ = coderdtest.AwaitWorkspaceBuildJob(t, client, ws.LatestBuild.ID)

		// Assert that the workspace is actually deleted.
		_, err := client.Workspace(testutil.Context(t, testutil.WaitShort), ws.ID)
		require.Error(t, err)
		cerr, ok := codersdk.AsError(err)
		require.True(t, ok)
		require.Equal(t, http.StatusGone, cerr.StatusCode())
	})

	t.Run("DormantTTLTooEarly", func(t *testing.T) {
		t.Parallel()

		var (
			ticker     = make(chan time.Time)
			statCh     = make(chan autobuild.Stats)
			dormantTTL = time.Minute
		)

		client, user := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				AutobuildTicker:          ticker,
				IncludeProvisionerDaemon: true,
				AutobuildStats:           statCh,
				TemplateScheduleStore:    schedule.NewEnterpriseTemplateScheduleStore(agplUserQuietHoursScheduleStore()),
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{codersdk.FeatureAdvancedTemplateScheduling: 1},
			},
		})
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionPlan:  echo.ProvisionComplete,
			ProvisionApply: echo.ProvisionComplete,
		})
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.TimeTilDormantAutoDeleteMillis = ptr.Ref[int64](dormantTTL.Milliseconds())
		})
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		ws := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		build := coderdtest.AwaitWorkspaceBuildJob(t, client, ws.LatestBuild.ID)
		require.Equal(t, codersdk.WorkspaceStatusRunning, build.Status)

		ctx := testutil.Context(t, testutil.WaitMedium)
		err := client.UpdateWorkspaceDormancy(ctx, ws.ID, codersdk.UpdateWorkspaceDormancy{
			Dormant: true,
		})
		require.NoError(t, err)

		ws = coderdtest.MustWorkspace(t, client, ws.ID)
		require.NotNil(t, ws.DormantAt)

		// Ensure we haven't breached our threshold.
		ticker <- ws.DormantAt.Add(-dormantTTL * 2)
		stats := <-statCh
		// Expect no transitions since not enough time has elapsed.
		require.Len(t, stats.Transitions, 0)

		_, err = client.UpdateTemplateMeta(ctx, template.ID, codersdk.UpdateTemplateMeta{
			TimeTilDormantAutoDeleteMillis: dormantTTL.Milliseconds(),
		})
		require.NoError(t, err)

		// Simlute the workspace breaching the threshold.
		ticker <- ws.DormantAt.Add(dormantTTL * 2)
		stats = <-statCh
		require.Len(t, stats.Transitions, 1)
		require.Equal(t, database.WorkspaceTransitionDelete, stats.Transitions[ws.ID])
	})

	// Assert that a dormant workspace does not autostart.
	t.Run("DormantNoAutostart", func(t *testing.T) {
		t.Parallel()

		var (
			ctx         = testutil.Context(t, testutil.WaitMedium)
			tickCh      = make(chan time.Time)
			statsCh     = make(chan autobuild.Stats)
			inactiveTTL = time.Minute
		)
		client, user := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				AutobuildTicker:          tickCh,
				IncludeProvisionerDaemon: true,
				AutobuildStats:           statsCh,
				TemplateScheduleStore:    schedule.NewEnterpriseTemplateScheduleStore(agplUserQuietHoursScheduleStore()),
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{codersdk.FeatureAdvancedTemplateScheduling: 1},
			},
		})

		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionPlan:  echo.ProvisionComplete,
			ProvisionApply: echo.ProvisionComplete,
		})
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)

		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		sched, err := agplschedule.Weekly("CRON_TZ=UTC 0 * * * *")
		require.NoError(t, err)

		ws := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID, func(cwr *codersdk.CreateWorkspaceRequest) {
			cwr.AutostartSchedule = ptr.Ref(sched.String())
		})
		coderdtest.AwaitWorkspaceBuildJob(t, client, ws.LatestBuild.ID)
		coderdtest.MustTransitionWorkspace(t, client, ws.ID, database.WorkspaceTransitionStart, database.WorkspaceTransitionStop)

		// Assert that autostart works when the workspace isn't dormant..
		tickCh <- sched.Next(ws.LatestBuild.CreatedAt)
		stats := <-statsCh
		require.NoError(t, stats.Error)
		require.Len(t, stats.Transitions, 1)
		require.Contains(t, stats.Transitions, ws.ID)
		require.Equal(t, database.WorkspaceTransitionStart, stats.Transitions[ws.ID])

		ws = coderdtest.MustWorkspace(t, client, ws.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, client, ws.LatestBuild.ID)

		// Now that we've validated that the workspace is eligible for autostart
		// lets cause it to become dormant.
		_, err = client.UpdateTemplateMeta(ctx, template.ID, codersdk.UpdateTemplateMeta{
			TimeTilDormantMillis: inactiveTTL.Milliseconds(),
		})
		require.NoError(t, err)

		// We should see the workspace get stopped now.
		tickCh <- ws.LastUsedAt.Add(inactiveTTL * 2)
		stats = <-statsCh
		require.NoError(t, stats.Error)
		require.Len(t, stats.Transitions, 1)
		require.Contains(t, stats.Transitions, ws.ID)
		require.Equal(t, database.WorkspaceTransitionStop, stats.Transitions[ws.ID])

		// The workspace should be dormant now.
		ws = coderdtest.MustWorkspace(t, client, ws.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, client, ws.LatestBuild.ID)
		require.NotNil(t, ws.DormantAt)

		// Assert that autostart is no longer triggered since workspace is dormant.
		tickCh <- sched.Next(ws.LatestBuild.CreatedAt)
		stats = <-statsCh
		require.Len(t, stats.Transitions, 0)
	})
}

func TestWorkspacesFiltering(t *testing.T) {
	t.Parallel()

	t.Run("DeletingBy", func(t *testing.T) {
		t.Parallel()

		dormantTTL := 24 * time.Hour

		client, user := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				IncludeProvisionerDaemon: true,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureAdvancedTemplateScheduling: 1,
				},
			},
		})

		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		_ = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		// update template with inactivity ttl
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		template, err := client.UpdateTemplateMeta(ctx, template.ID, codersdk.UpdateTemplateMeta{
			TimeTilDormantAutoDeleteMillis: dormantTTL.Milliseconds(),
		})
		require.NoError(t, err)
		require.Equal(t, dormantTTL.Milliseconds(), template.TimeTilDormantAutoDeleteMillis)

		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		_ = coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

		// stop build so workspace is inactive
		stopBuild := coderdtest.CreateWorkspaceBuild(t, client, workspace, database.WorkspaceTransitionStop)
		coderdtest.AwaitWorkspaceBuildJob(t, client, stopBuild.ID)
		err = client.UpdateWorkspaceDormancy(ctx, workspace.ID, codersdk.UpdateWorkspaceDormancy{
			Dormant: true,
		})
		require.NoError(t, err)
		workspace = coderdtest.MustWorkspace(t, client, workspace.ID)
		require.NotNil(t, workspace.DeletingAt)

		res, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{
			// adding a second to time.Now() to give some buffer in case test runs quickly
			FilterQuery: fmt.Sprintf("deleting_by:%s", time.Now().Add(time.Second).Add(dormantTTL).Format("2006-01-02")),
		})
		require.NoError(t, err)
		require.Len(t, res.Workspaces, 1)
		require.Equal(t, workspace.ID, res.Workspaces[0].ID)
	})
}

// TestWorkspacesWithoutTemplatePerms creates a workspace for a user, then drops
// the user's perms to the underlying template.
func TestWorkspacesWithoutTemplatePerms(t *testing.T) {
	t.Parallel()

	client, first := coderdenttest.New(t, &coderdenttest.Options{
		Options: &coderdtest.Options{
			IncludeProvisionerDaemon: true,
		},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		},
	})

	version := coderdtest.CreateTemplateVersion(t, client, first.OrganizationID, nil)
	coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, first.OrganizationID, version.ID)

	user, _ := coderdtest.CreateAnotherUser(t, client, first.OrganizationID)
	workspace := coderdtest.CreateWorkspace(t, user, first.OrganizationID, template.ID)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	// Remove everyone access
	err := client.UpdateTemplateACL(ctx, template.ID, codersdk.UpdateTemplateACL{
		GroupPerms: map[string]codersdk.TemplateRole{
			first.OrganizationID.String(): codersdk.TemplateRoleDeleted,
		},
	})
	require.NoError(t, err, "remove everyone access")

	// This should fail as the user cannot read the template
	_, err = user.Workspace(ctx, workspace.ID)
	require.Error(t, err, "fetch workspace")
	var sdkError *codersdk.Error
	require.ErrorAs(t, err, &sdkError)
	require.Equal(t, http.StatusForbidden, sdkError.StatusCode())

	_, err = user.Workspaces(ctx, codersdk.WorkspaceFilter{})
	require.NoError(t, err, "fetch workspaces should not fail")

	// Now create another workspace the user can read.
	version2 := coderdtest.CreateTemplateVersion(t, client, first.OrganizationID, nil)
	coderdtest.AwaitTemplateVersionJob(t, client, version2.ID)
	template2 := coderdtest.CreateTemplate(t, client, first.OrganizationID, version2.ID)
	_ = coderdtest.CreateWorkspace(t, user, first.OrganizationID, template2.ID)

	workspaces, err := user.Workspaces(ctx, codersdk.WorkspaceFilter{})
	require.NoError(t, err, "fetch workspaces should not fail")
	require.Len(t, workspaces.Workspaces, 1)
}

func TestWorkspaceLock(t *testing.T) {
	t.Parallel()

	t.Run("TemplateTimeTilDormantAutoDelete", func(t *testing.T) {
		t.Parallel()
		var (
			client, user = coderdenttest.New(t, &coderdenttest.Options{
				Options: &coderdtest.Options{
					IncludeProvisionerDaemon: true,
					TemplateScheduleStore:    &schedule.EnterpriseTemplateScheduleStore{},
				},
				LicenseOptions: &coderdenttest.LicenseOptions{
					Features: license.Features{
						codersdk.FeatureAdvancedTemplateScheduling: 1,
					},
				},
			})

			version    = coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
			_          = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
			dormantTTL = time.Minute
		)

		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.TimeTilDormantAutoDeleteMillis = ptr.Ref[int64](dormantTTL.Milliseconds())
		})

		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		_ = coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		lastUsedAt := workspace.LastUsedAt
		err := client.UpdateWorkspaceDormancy(ctx, workspace.ID, codersdk.UpdateWorkspaceDormancy{
			Dormant: true,
		})
		require.NoError(t, err)

		workspace = coderdtest.MustWorkspace(t, client, workspace.ID)
		require.NoError(t, err, "fetch provisioned workspace")
		require.NotNil(t, workspace.DeletingAt)
		require.NotNil(t, workspace.DormantAt)
		require.Equal(t, workspace.DormantAt.Add(dormantTTL), *workspace.DeletingAt)
		require.WithinRange(t, *workspace.DormantAt, time.Now().Add(-time.Second*10), time.Now())
		// Locking a workspace shouldn't update the last_used_at.
		require.Equal(t, lastUsedAt, workspace.LastUsedAt)

		workspace = coderdtest.MustWorkspace(t, client, workspace.ID)
		lastUsedAt = workspace.LastUsedAt
		err = client.UpdateWorkspaceDormancy(ctx, workspace.ID, codersdk.UpdateWorkspaceDormancy{
			Dormant: false,
		})
		require.NoError(t, err)

		workspace, err = client.Workspace(ctx, workspace.ID)
		require.NoError(t, err, "fetch provisioned workspace")
		require.Nil(t, workspace.DormantAt)
		// Unlocking a workspace should cause the deleting_at to be unset.
		require.Nil(t, workspace.DeletingAt)
		// The last_used_at should get updated when we unlock the workspace.
		require.True(t, workspace.LastUsedAt.After(lastUsedAt))
	})
}

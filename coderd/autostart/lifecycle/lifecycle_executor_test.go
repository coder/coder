package lifecycle_test

import (
	"context"
	"testing"
	"time"

	"github.com/coder/coder/coderd/autostart/schedule"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/codersdk"

	"github.com/stretchr/testify/require"
)

func Test_Executor_Run(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		var (
			ctx    = context.Background()
			err    error
			tickCh = make(chan time.Time)
			client = coderdtest.New(t, &coderdtest.Options{
				LifecycleTicker: tickCh,
			})
			// Given: we have a user with a workspace
			_         = coderdtest.NewProvisionerDaemon(t, client)
			user      = coderdtest.CreateFirstUser(t, client)
			version   = coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
			template  = coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
			_         = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
			workspace = coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
			_         = coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
		)
		// Given: workspace is stopped
		build, err := client.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
			TemplateVersionID: template.ActiveVersionID,
			Transition:        database.WorkspaceTransitionStop,
		})
		require.NoError(t, err, "stop workspace")
		// Given: we wait for the stop to complete
		_ = coderdtest.AwaitWorkspaceBuildJob(t, client, build.ID)

		// Given: we update the workspace with its new state
		workspace = coderdtest.MustWorkspace(t, client, workspace.ID)
		// Given: we ensure the workspace is now in a stopped state
		require.Equal(t, database.WorkspaceTransitionStop, workspace.LatestBuild.Transition)

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
		}()

		// Then: the workspace should be started
		require.Eventually(t, func() bool {
			ws := coderdtest.MustWorkspace(t, client, workspace.ID)
			return ws.LatestBuild.Job.Status == codersdk.ProvisionerJobSucceeded &&
				ws.LatestBuild.Transition == database.WorkspaceTransitionStart
		}, 5*time.Second, 250*time.Millisecond)
	})

	t.Run("AlreadyRunning", func(t *testing.T) {
		t.Parallel()

		var (
			ctx    = context.Background()
			err    error
			tickCh = make(chan time.Time)
			client = coderdtest.New(t, &coderdtest.Options{
				LifecycleTicker: tickCh,
			})
			// Given: we have a user with a workspace
			_         = coderdtest.NewProvisionerDaemon(t, client)
			user      = coderdtest.CreateFirstUser(t, client)
			version   = coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
			template  = coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
			_         = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
			workspace = coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
			_         = coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
		)

		// Given: we ensure the workspace is now in a stopped state
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
		}()

		// Then: the workspace should not be started.
		require.Never(t, func() bool {
			ws := coderdtest.MustWorkspace(t, client, workspace.ID)
			return ws.LatestBuild.ID != workspace.LatestBuild.ID
		}, 5*time.Second, 250*time.Millisecond)
	})

	t.Run("NotEnabled", func(t *testing.T) {
		t.Parallel()

		var (
			tickCh = make(chan time.Time)
			client = coderdtest.New(t, &coderdtest.Options{
				LifecycleTicker: tickCh,
			})
			// Given: we have a user with a workspace
			_         = coderdtest.NewProvisionerDaemon(t, client)
			user      = coderdtest.CreateFirstUser(t, client)
			version   = coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
			template  = coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
			_         = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
			workspace = coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
			_         = coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
		)

		// Given: we ensure the workspace is now in a stopped state
		require.Equal(t, database.WorkspaceTransitionStart, workspace.LatestBuild.Transition)

		// Given: the workspace has autostart disabled
		require.Empty(t, workspace.AutostartSchedule)

		// When: the lifecycle executor ticks
		go func() {
			tickCh <- time.Now().UTC().Add(time.Minute)
		}()

		// Then: the workspace should not be started.
		require.Never(t, func() bool {
			ws := coderdtest.MustWorkspace(t, client, workspace.ID)
			return ws.LatestBuild.ID != workspace.LatestBuild.ID
		}, 5*time.Second, 250*time.Millisecond)
	})
}

package cli_test

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
)

func TestScheduleShow(t *testing.T) {
	t.Parallel()
	t.Run("Enabled", func(t *testing.T) {
		t.Parallel()

		var (
			tz        = "Europe/Dublin"
			sched     = "30 7 * * 1-5"
			schedCron = fmt.Sprintf("CRON_TZ=%s %s", tz, sched)
			ttl       = 8 * time.Hour
			client    = coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			user      = coderdtest.CreateFirstUser(t, client)
			version   = coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
			_         = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
			project   = coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
			workspace = coderdtest.CreateWorkspace(t, client, user.OrganizationID, project.ID, func(cwr *codersdk.CreateWorkspaceRequest) {
				cwr.AutostartSchedule = ptr.Ref(schedCron)
				cwr.TTLMillis = ptr.Ref(ttl.Milliseconds())
			})
			_         = coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
			cmdArgs   = []string{"schedule", "show", workspace.Name}
			stdoutBuf = &bytes.Buffer{}
		)

		inv, root := clitest.New(t, cmdArgs...)
		clitest.SetupConfig(t, client, root)
		inv.Stdout = stdoutBuf

		err := inv.Run()
		require.NoError(t, err, "unexpected error")
		lines := strings.Split(strings.TrimSpace(stdoutBuf.String()), "\n")
		if assert.Len(t, lines, 4) {
			assert.Contains(t, lines[0], "Starts at    7:30AM Mon-Fri (Europe/Dublin)")
			assert.Contains(t, lines[1], "Starts next  7:30AM")
			// it should have either IST or GMT
			if !strings.Contains(lines[1], "IST") && !strings.Contains(lines[1], "GMT") {
				t.Error("expected either IST or GMT")
			}
			assert.Contains(t, lines[2], "Stops at     8h after start")
			assert.NotContains(t, lines[3], "Stops next   -")
		}
	})

	t.Run("Manual", func(t *testing.T) {
		t.Parallel()

		var (
			client    = coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			user      = coderdtest.CreateFirstUser(t, client)
			version   = coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
			_         = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
			project   = coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
			workspace = coderdtest.CreateWorkspace(t, client, user.OrganizationID, project.ID, func(cwr *codersdk.CreateWorkspaceRequest) {
				cwr.AutostartSchedule = nil
				cwr.TTLMillis = nil
			})
			_         = coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
			cmdArgs   = []string{"schedule", "show", workspace.Name}
			stdoutBuf = &bytes.Buffer{}
		)

		inv, root := clitest.New(t, cmdArgs...)
		clitest.SetupConfig(t, client, root)
		inv.Stdout = stdoutBuf

		err := inv.Run()
		require.NoError(t, err, "unexpected error")
		lines := strings.Split(strings.TrimSpace(stdoutBuf.String()), "\n")
		if assert.Len(t, lines, 4) {
			assert.Contains(t, lines[0], "Starts at    manual")
			assert.Contains(t, lines[1], "Starts next  -")
			assert.Contains(t, lines[2], "Stops at     manual")
			assert.Contains(t, lines[3], "Stops next   -")
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()

		var (
			client  = coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			user    = coderdtest.CreateFirstUser(t, client)
			version = coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
			_       = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		)

		inv, root := clitest.New(t, "schedule", "show", "doesnotexist")
		clitest.SetupConfig(t, client, root)

		err := inv.Run()
		require.ErrorContains(t, err, "status code 404", "unexpected error")
	})
}

func TestScheduleStart(t *testing.T) {
	t.Parallel()

	var (
		ctx       = context.Background()
		client    = coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user      = coderdtest.CreateFirstUser(t, client)
		version   = coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		_         = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		project   = coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		workspace = coderdtest.CreateWorkspace(t, client, user.OrganizationID, project.ID, func(cwr *codersdk.CreateWorkspaceRequest) {
			cwr.AutostartSchedule = nil
		})
		_         = coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
		tz        = "Europe/Dublin"
		sched     = "CRON_TZ=Europe/Dublin 30 9 * * Mon-Fri"
		stdoutBuf = &bytes.Buffer{}
	)

	// Set a well-specified autostart schedule
	inv, root := clitest.New(t, "schedule", "start", workspace.Name, "9:30AM", "Mon-Fri", tz)
	clitest.SetupConfig(t, client, root)
	inv.Stdout = stdoutBuf

	err := inv.Run()
	assert.NoError(t, err, "unexpected error")
	lines := strings.Split(strings.TrimSpace(stdoutBuf.String()), "\n")
	if assert.Len(t, lines, 4) {
		assert.Contains(t, lines[0], "Starts at    9:30AM Mon-Fri (Europe/Dublin)")
		assert.Contains(t, lines[1], "Starts next  9:30AM")
		// it should have either IST or GMT
		if !strings.Contains(lines[1], "IST") && !strings.Contains(lines[1], "GMT") {
			t.Error("expected either IST or GMT")
		}
	}

	// Ensure autostart schedule updated
	updated, err := client.Workspace(ctx, workspace.ID)
	require.NoError(t, err, "fetch updated workspace")
	require.Equal(t, sched, *updated.AutostartSchedule, "expected autostart schedule to be set")

	// Reset stdout
	stdoutBuf = &bytes.Buffer{}

	// unset schedule
	inv, root = clitest.New(t, "schedule", "start", workspace.Name, "manual")
	clitest.SetupConfig(t, client, root)
	inv.Stdout = stdoutBuf

	err = inv.Run()
	assert.NoError(t, err, "unexpected error")
	lines = strings.Split(strings.TrimSpace(stdoutBuf.String()), "\n")
	if assert.Len(t, lines, 4) {
		assert.Contains(t, lines[0], "Starts at    manual")
		assert.Contains(t, lines[1], "Starts next  -")
	}
}

func TestScheduleStop(t *testing.T) {
	t.Parallel()

	var (
		client    = coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user      = coderdtest.CreateFirstUser(t, client)
		version   = coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		_         = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		project   = coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		ttl       = 8*time.Hour + 30*time.Minute
		workspace = coderdtest.CreateWorkspace(t, client, user.OrganizationID, project.ID)
		_         = coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
		stdoutBuf = &bytes.Buffer{}
	)

	// Set the workspace TTL
	inv, root := clitest.New(t, "schedule", "stop", workspace.Name, ttl.String())
	clitest.SetupConfig(t, client, root)
	inv.Stdout = stdoutBuf

	err := inv.Run()
	assert.NoError(t, err, "unexpected error")
	lines := strings.Split(strings.TrimSpace(stdoutBuf.String()), "\n")
	if assert.Len(t, lines, 4) {
		assert.Contains(t, lines[2], "Stops at     8h30m after start")
		// Should not be manual
		assert.NotContains(t, lines[3], "Stops next   -")
	}

	// Reset stdout
	stdoutBuf = &bytes.Buffer{}

	// Unset the workspace TTL
	inv, root = clitest.New(t, "schedule", "stop", workspace.Name, "manual")
	clitest.SetupConfig(t, client, root)
	inv.Stdout = stdoutBuf

	err = inv.Run()
	assert.NoError(t, err, "unexpected error")
	lines = strings.Split(strings.TrimSpace(stdoutBuf.String()), "\n")
	if assert.Len(t, lines, 4) {
		assert.Contains(t, lines[2], "Stops at     manual")
		// Deadline of a running workspace is not updated.
		assert.NotContains(t, lines[3], "Stops next   -")
	}
}

func TestScheduleOverride(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		// Given: we have a workspace
		var (
			err       error
			ctx       = context.Background()
			client    = coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			user      = coderdtest.CreateFirstUser(t, client)
			version   = coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
			_         = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
			project   = coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
			workspace = coderdtest.CreateWorkspace(t, client, user.OrganizationID, project.ID)
			cmdArgs   = []string{"schedule", "override-stop", workspace.Name, "10h"}
			stdoutBuf = &bytes.Buffer{}
		)

		// Given: we wait for the workspace to be built
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
		workspace, err = client.Workspace(ctx, workspace.ID)
		require.NoError(t, err)
		expectedDeadline := time.Now().Add(10 * time.Hour)

		// Assert test invariant: workspace build has a deadline set equal to now plus ttl
		initDeadline := time.Now().Add(time.Duration(*workspace.TTLMillis) * time.Millisecond)
		require.WithinDuration(t, initDeadline, workspace.LatestBuild.Deadline.Time, time.Minute)

		inv, root := clitest.New(t, cmdArgs...)
		clitest.SetupConfig(t, client, root)
		inv.Stdout = stdoutBuf

		// When: we execute `coder schedule override workspace <number without units>`
		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)

		// Then: the deadline of the latest build is updated assuming the units are minutes
		updated, err := client.Workspace(ctx, workspace.ID)
		require.NoError(t, err)
		require.WithinDuration(t, expectedDeadline, updated.LatestBuild.Deadline.Time, time.Minute)
	})

	t.Run("InvalidDuration", func(t *testing.T) {
		t.Parallel()

		// Given: we have a workspace
		var (
			err       error
			ctx       = context.Background()
			client    = coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			user      = coderdtest.CreateFirstUser(t, client)
			version   = coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
			_         = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
			project   = coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
			workspace = coderdtest.CreateWorkspace(t, client, user.OrganizationID, project.ID)
			cmdArgs   = []string{"schedule", "override-stop", workspace.Name, "kwyjibo"}
			stdoutBuf = &bytes.Buffer{}
		)

		// Given: we wait for the workspace to be built
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
		workspace, err = client.Workspace(ctx, workspace.ID)
		require.NoError(t, err)

		// Assert test invariant: workspace build has a deadline set equal to now plus ttl
		initDeadline := time.Now().Add(time.Duration(*workspace.TTLMillis) * time.Millisecond)
		require.WithinDuration(t, initDeadline, workspace.LatestBuild.Deadline.Time, time.Minute)

		inv, root := clitest.New(t, cmdArgs...)
		clitest.SetupConfig(t, client, root)
		inv.Stdout = stdoutBuf

		// When: we execute `coder bump workspace <not a number>`
		err = inv.WithContext(ctx).Run()
		// Then: the command fails
		require.ErrorContains(t, err, "invalid duration")
	})

	t.Run("NoDeadline", func(t *testing.T) {
		t.Parallel()

		// Given: we have a workspace with no deadline set
		var (
			err       error
			ctx       = context.Background()
			client    = coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			user      = coderdtest.CreateFirstUser(t, client)
			version   = coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
			_         = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
			template  = coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
			workspace = coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID, func(cwr *codersdk.CreateWorkspaceRequest) {
				cwr.TTLMillis = nil
			})
			cmdArgs   = []string{"schedule", "override-stop", workspace.Name, "1h"}
			stdoutBuf = &bytes.Buffer{}
		)
		require.Zero(t, template.DefaultTTLMillis)
		require.Empty(t, template.AutostopRequirement.DaysOfWeek)
		require.EqualValues(t, 1, template.AutostopRequirement.Weeks)

		// Unset the workspace TTL
		err = client.UpdateWorkspaceTTL(ctx, workspace.ID, codersdk.UpdateWorkspaceTTLRequest{TTLMillis: nil})
		require.NoError(t, err)
		workspace, err = client.Workspace(ctx, workspace.ID)
		require.NoError(t, err)
		require.Nil(t, workspace.TTLMillis)

		// Given: we wait for the workspace to build
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
		workspace, err = client.Workspace(ctx, workspace.ID)
		require.NoError(t, err)

		// NOTE(cian): need to stop and start the workspace as we do not update the deadline
		//             see: https://github.com/coder/coder/issues/2224
		coderdtest.MustTransitionWorkspace(t, client, workspace.ID, database.WorkspaceTransitionStart, database.WorkspaceTransitionStop)
		coderdtest.MustTransitionWorkspace(t, client, workspace.ID, database.WorkspaceTransitionStop, database.WorkspaceTransitionStart)

		// Assert test invariant: workspace has no TTL set
		require.Zero(t, workspace.LatestBuild.Deadline)
		require.NoError(t, err)

		inv, root := clitest.New(t, cmdArgs...)
		clitest.SetupConfig(t, client, root)
		inv.Stdout = stdoutBuf

		// When: we execute `coder bump workspace``
		err = inv.WithContext(ctx).Run()
		require.Error(t, err)

		// Then: nothing happens and the deadline remains unset
		updated, err := client.Workspace(ctx, workspace.ID)
		require.NoError(t, err)
		require.Zero(t, updated.LatestBuild.Deadline)
	})
}

//nolint:paralleltest // t.Setenv
func TestScheduleStartDefaults(t *testing.T) {
	t.Setenv("TZ", "Pacific/Tongatapu")
	var (
		client    = coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user      = coderdtest.CreateFirstUser(t, client)
		version   = coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		_         = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		project   = coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		workspace = coderdtest.CreateWorkspace(t, client, user.OrganizationID, project.ID, func(cwr *codersdk.CreateWorkspaceRequest) {
			cwr.AutostartSchedule = nil
		})
		stdoutBuf = &bytes.Buffer{}
	)

	// Set an underspecified schedule
	inv, root := clitest.New(t, "schedule", "start", workspace.Name, "9:30AM")
	clitest.SetupConfig(t, client, root)
	inv.Stdout = stdoutBuf
	err := inv.Run()
	require.NoError(t, err, "unexpected error")
	lines := strings.Split(strings.TrimSpace(stdoutBuf.String()), "\n")
	if assert.Len(t, lines, 4) {
		assert.Contains(t, lines[0], "Starts at    9:30AM daily (Pacific/Tongatapu)")
		assert.Contains(t, lines[1], "Starts next  9:30AM +13 on")
		assert.Contains(t, lines[2], "Stops at     8h after start")
	}
}

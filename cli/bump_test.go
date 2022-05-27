package cli_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
)

func TestBump(t *testing.T) {
	t.Parallel()

	t.Run("BumpOKDefault", func(t *testing.T) {
		t.Parallel()

		// Given: we have a workspace
		var (
			err       error
			ctx       = context.Background()
			client    = coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
			user      = coderdtest.CreateFirstUser(t, client)
			version   = coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
			_         = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
			project   = coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
			workspace = coderdtest.CreateWorkspace(t, client, user.OrganizationID, project.ID)
			cmdArgs   = []string{"bump", workspace.Name}
			stdoutBuf = &bytes.Buffer{}
		)

		// Given: we wait for the workspace to be built
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
		workspace, err = client.Workspace(ctx, workspace.ID)
		require.NoError(t, err)
		expectedDeadline := workspace.LatestBuild.Deadline.Add(90 * time.Minute)

		// Assert test invariant: workspace build has a deadline set equal to now plus ttl
		require.WithinDuration(t, workspace.LatestBuild.Deadline, time.Now().Add(*workspace.TTL), time.Minute)
		require.NoError(t, err)

		cmd, root := clitest.New(t, cmdArgs...)
		clitest.SetupConfig(t, client, root)
		cmd.SetOut(stdoutBuf)

		// When: we execute `coder bump <workspace>`
		err = cmd.ExecuteContext(ctx)
		require.NoError(t, err, "unexpected error")

		// Then: the deadline of the latest build is updated
		updated, err := client.Workspace(ctx, workspace.ID)
		require.NoError(t, err)
		require.WithinDuration(t, expectedDeadline, updated.LatestBuild.Deadline, time.Minute)
	})

	t.Run("BumpSpecificDuration", func(t *testing.T) {
		t.Parallel()

		// Given: we have a workspace
		var (
			err       error
			ctx       = context.Background()
			client    = coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
			user      = coderdtest.CreateFirstUser(t, client)
			version   = coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
			_         = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
			project   = coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
			workspace = coderdtest.CreateWorkspace(t, client, user.OrganizationID, project.ID)
			cmdArgs   = []string{"bump", workspace.Name, "30"}
			stdoutBuf = &bytes.Buffer{}
		)

		// Given: we wait for the workspace to be built
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
		workspace, err = client.Workspace(ctx, workspace.ID)
		require.NoError(t, err)
		expectedDeadline := workspace.LatestBuild.Deadline.Add(30 * time.Minute)

		// Assert test invariant: workspace build has a deadline set equal to now plus ttl
		require.WithinDuration(t, workspace.LatestBuild.Deadline, time.Now().Add(*workspace.TTL), time.Minute)
		require.NoError(t, err)

		cmd, root := clitest.New(t, cmdArgs...)
		clitest.SetupConfig(t, client, root)
		cmd.SetOut(stdoutBuf)

		// When: we execute `coder bump workspace <number without units>`
		err = cmd.ExecuteContext(ctx)
		require.NoError(t, err)

		// Then: the deadline of the latest build is updated assuming the units are minutes
		updated, err := client.Workspace(ctx, workspace.ID)
		require.NoError(t, err)
		require.WithinDuration(t, expectedDeadline, updated.LatestBuild.Deadline, time.Minute)
	})

	t.Run("BumpInvalidDuration", func(t *testing.T) {
		t.Parallel()

		// Given: we have a workspace
		var (
			err       error
			ctx       = context.Background()
			client    = coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
			user      = coderdtest.CreateFirstUser(t, client)
			version   = coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
			_         = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
			project   = coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
			workspace = coderdtest.CreateWorkspace(t, client, user.OrganizationID, project.ID)
			cmdArgs   = []string{"bump", workspace.Name, "kwyjibo"}
			stdoutBuf = &bytes.Buffer{}
		)

		// Given: we wait for the workspace to be built
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
		workspace, err = client.Workspace(ctx, workspace.ID)
		require.NoError(t, err)

		// Assert test invariant: workspace build has a deadline set equal to now plus ttl
		require.WithinDuration(t, workspace.LatestBuild.Deadline, time.Now().Add(*workspace.TTL), time.Minute)
		require.NoError(t, err)

		cmd, root := clitest.New(t, cmdArgs...)
		clitest.SetupConfig(t, client, root)
		cmd.SetOut(stdoutBuf)

		// When: we execute `coder bump workspace <not a number>`
		err = cmd.ExecuteContext(ctx)
		// Then: the command fails
		require.ErrorContains(t, err, "invalid duration")
	})

	t.Run("BumpNoDeadline", func(t *testing.T) {
		t.Parallel()

		// Given: we have a workspace with no deadline set
		var (
			err       error
			ctx       = context.Background()
			client    = coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
			user      = coderdtest.CreateFirstUser(t, client)
			version   = coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
			_         = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
			project   = coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
			workspace = coderdtest.CreateWorkspace(t, client, user.OrganizationID, project.ID, func(cwr *codersdk.CreateWorkspaceRequest) {
				cwr.TTL = nil
			})
			cmdArgs   = []string{"bump", workspace.Name}
			stdoutBuf = &bytes.Buffer{}
		)

		// Given: we wait for the workspace to build
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
		workspace, err = client.Workspace(ctx, workspace.ID)
		require.NoError(t, err)

		// Assert test invariant: workspace has no TTL set
		require.Zero(t, workspace.LatestBuild.Deadline)
		require.NoError(t, err)

		cmd, root := clitest.New(t, cmdArgs...)
		clitest.SetupConfig(t, client, root)
		cmd.SetOut(stdoutBuf)

		// When: we execute `coder bump workspace``
		err = cmd.ExecuteContext(ctx)

		// Then: nothing happens and the deadline remains unset
		require.NoError(t, err)
		updated, err := client.Workspace(ctx, workspace.ID)
		require.NoError(t, err)
		require.Zero(t, updated.LatestBuild.Deadline)
	})

	t.Run("BumpMinimumDuration", func(t *testing.T) {
		t.Parallel()

		// Given: we have a workspace with no deadline set
		var (
			err       error
			ctx       = context.Background()
			client    = coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
			user      = coderdtest.CreateFirstUser(t, client)
			version   = coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
			_         = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
			project   = coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
			workspace = coderdtest.CreateWorkspace(t, client, user.OrganizationID, project.ID)
			cmdArgs   = []string{"bump", workspace.Name, "59s"}
			stdoutBuf = &bytes.Buffer{}
		)

		// Given: we wait for the workspace to build
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
		workspace, err = client.Workspace(ctx, workspace.ID)
		require.NoError(t, err)

		// Assert test invariant: workspace build has a deadline set equal to now plus ttl
		require.WithinDuration(t, workspace.LatestBuild.Deadline, time.Now().Add(*workspace.TTL), time.Minute)
		require.NoError(t, err)

		cmd, root := clitest.New(t, cmdArgs...)
		clitest.SetupConfig(t, client, root)
		cmd.SetOut(stdoutBuf)

		// When: we execute `coder bump workspace 59s`
		err = cmd.ExecuteContext(ctx)

		// Then: an error is reported and the deadline remains as before
		require.ErrorContains(t, err, "minimum bump duration is 1 minute")
		updated, err := client.Workspace(ctx, workspace.ID)
		require.NoError(t, err)
		require.WithinDuration(t, workspace.LatestBuild.Deadline, updated.LatestBuild.Deadline, time.Minute)
	})
}

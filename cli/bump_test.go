package cli_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/codersdk"
)

func TestBump(t *testing.T) {
	t.Parallel()

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
			cmdArgs   = []string{"bump", workspace.Name, "10h"}
			stdoutBuf = &bytes.Buffer{}
		)

		// Given: we wait for the workspace to be built
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
		workspace, err = client.Workspace(ctx, workspace.ID)
		require.NoError(t, err)
		expectedDeadline := time.Now().Add(10 * time.Hour)

		// Assert test invariant: workspace build has a deadline set equal to now plus ttl
		initDeadline := time.Now().Add(time.Duration(*workspace.TTLMillis) * time.Millisecond)
		require.WithinDuration(t, initDeadline, workspace.LatestBuild.Deadline, time.Minute)

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
		initDeadline := time.Now().Add(time.Duration(*workspace.TTLMillis) * time.Millisecond)
		require.WithinDuration(t, initDeadline, workspace.LatestBuild.Deadline, time.Minute)

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
				cwr.TTLMillis = nil
			})
			cmdArgs   = []string{"bump", workspace.Name, "1h"}
			stdoutBuf = &bytes.Buffer{}
		)
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

		// TODO(cian): need to stop and start the workspace as we do not update the deadline yet
		//             see: https://github.com/coder/coder/issues/1783
		coderdtest.MustTransitionWorkspace(t, client, workspace.ID, database.WorkspaceTransitionStart, database.WorkspaceTransitionStop)
		coderdtest.MustTransitionWorkspace(t, client, workspace.ID, database.WorkspaceTransitionStop, database.WorkspaceTransitionStart)

		// Assert test invariant: workspace has no TTL set
		require.Zero(t, workspace.LatestBuild.Deadline)
		require.NoError(t, err)

		cmd, root := clitest.New(t, cmdArgs...)
		clitest.SetupConfig(t, client, root)
		cmd.SetOut(stdoutBuf)

		// When: we execute `coder bump workspace``
		err = cmd.ExecuteContext(ctx)
		require.Error(t, err)

		// Then: nothing happens and the deadline remains unset
		updated, err := client.Workspace(ctx, workspace.ID)
		require.NoError(t, err)
		require.Zero(t, updated.LatestBuild.Deadline)
	})
}

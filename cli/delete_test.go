package cli_test

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/pty/ptytest"
)

func TestDelete(t *testing.T) {
	t.Parallel()
	t.Run("WithParameter", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
		cmd, root := clitest.New(t, "delete", workspace.Name, "-y")
		clitest.SetupConfig(t, client, root)
		doneChan := make(chan struct{})
		pty := ptytest.New(t)
		cmd.SetIn(pty.Input())
		cmd.SetOut(pty.Output())
		go func() {
			defer close(doneChan)
			err := cmd.Execute()
			// When running with the race detector on, we sometimes get an EOF.
			if err != nil {
				assert.ErrorIs(t, err, io.EOF)
			}
		}()
		pty.ExpectMatch("workspace has been deleted")
		<-doneChan
	})

	t.Run("Orphan", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
		cmd, root := clitest.New(t, "delete", workspace.Name, "-y", "--orphan")

		clitest.SetupConfig(t, client, root)
		doneChan := make(chan struct{})
		pty := ptytest.New(t)
		cmd.SetIn(pty.Input())
		cmd.SetOut(pty.Output())
		cmd.SetErr(pty.Output())
		go func() {
			defer close(doneChan)
			err := cmd.Execute()
			// When running with the race detector on, we sometimes get an EOF.
			if err != nil {
				assert.ErrorIs(t, err, io.EOF)
			}
		}()
		pty.ExpectMatch("workspace has been deleted")
		<-doneChan
	})

	t.Run("DifferentUser", func(t *testing.T) {
		t.Parallel()
		adminClient := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		adminUser := coderdtest.CreateFirstUser(t, adminClient)
		orgID := adminUser.OrganizationID
		client, _ := coderdtest.CreateAnotherUser(t, adminClient, orgID)
		user, err := client.User(context.Background(), codersdk.Me)
		require.NoError(t, err)

		version := coderdtest.CreateTemplateVersion(t, adminClient, orgID, nil)
		coderdtest.AwaitTemplateVersionJob(t, adminClient, version.ID)
		template := coderdtest.CreateTemplate(t, adminClient, orgID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, orgID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

		cmd, root := clitest.New(t, "delete", user.Username+"/"+workspace.Name, "-y")
		clitest.SetupConfig(t, adminClient, root)
		doneChan := make(chan struct{})
		pty := ptytest.New(t)
		cmd.SetIn(pty.Input())
		cmd.SetOut(pty.Output())
		go func() {
			defer close(doneChan)
			err := cmd.Execute()
			// When running with the race detector on, we sometimes get an EOF.
			if err != nil {
				assert.ErrorIs(t, err, io.EOF)
			}
		}()

		pty.ExpectMatch("workspace has been deleted")
		<-doneChan

		workspace, err = client.Workspace(context.Background(), workspace.ID)
		require.ErrorContains(t, err, "was deleted")
	})

	t.Run("InvalidWorkspaceIdentifier", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		cmd, root := clitest.New(t, "delete", "a/b/c", "-y")
		clitest.SetupConfig(t, client, root)
		doneChan := make(chan struct{})
		go func() {
			defer close(doneChan)
			err := cmd.Execute()
			assert.ErrorContains(t, err, "invalid workspace name: \"a/b/c\"")
		}()
		<-doneChan
	})
}

package cli_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/pty/ptytest"
)

func TestProjectList(t *testing.T) {
	t.Parallel()
	t.Run("None", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		coderdtest.CreateInitialUser(t, client)
		cmd, root := clitest.New(t, "projects", "list")
		clitest.SetupConfig(t, client, root)
		pty := ptytest.New(t)
		cmd.SetIn(pty.Input())
		cmd.SetOut(pty.Output())
		closeChan := make(chan struct{})
		go func() {
			err := cmd.Execute()
			require.NoError(t, err)
			close(closeChan)
		}()
		pty.ExpectMatch("No projects found")
		<-closeChan
	})
	t.Run("List", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		daemon := coderdtest.NewProvisionerDaemon(t, client)
		job := coderdtest.CreateProjectImportJob(t, client, user.Organization, nil)
		coderdtest.AwaitProjectImportJob(t, client, user.Organization, job.ID)
		_ = daemon.Close()
		project := coderdtest.CreateProject(t, client, user.Organization, job.ID)
		cmd, root := clitest.New(t, "projects", "list")
		clitest.SetupConfig(t, client, root)
		pty := ptytest.New(t)
		cmd.SetIn(pty.Input())
		cmd.SetOut(pty.Output())
		closeChan := make(chan struct{})
		go func() {
			err := cmd.Execute()
			require.NoError(t, err)
			close(closeChan)
		}()
		pty.ExpectMatch(project.Name)
		<-closeChan
	})
}

package cli_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/pty/ptytest"
)

func TestTemplateCreate(t *testing.T) {
	t.Parallel()
	t.Run("Create", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		coderdtest.CreateFirstUser(t, client)
		source := clitest.CreateTemplateVersionSource(t, &echo.Responses{
			Parse:     echo.ParseComplete,
			Provision: echo.ProvisionComplete,
		})
		cmd, root := clitest.New(t, "templates", "create", "my-template", "--directory", source, "--test.provisioner", string(database.ProvisionerTypeEcho))
		clitest.SetupConfig(t, client, root)
		_ = coderdtest.NewProvisionerDaemon(t, client)
		pty := ptytest.New(t)
		cmd.SetIn(pty.Input())
		cmd.SetOut(pty.Output())

		execDone := make(chan error)
		go func() {
			execDone <- cmd.Execute()
		}()

		matches := []struct {
			match string
			write string
		}{
			{match: "Create and upload", write: "yes"},
			{match: "Confirm create?", write: "yes"},
		}
		for _, m := range matches {
			pty.ExpectMatch(m.match)
			pty.WriteLine(m.write)
		}

		require.NoError(t, <-execDone)
	})
}

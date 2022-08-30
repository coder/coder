package cli_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/pty/ptytest"
)

func TestProvisionerRun(t *testing.T) {
	t.Parallel()
	t.Run("Provisioner", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		provisionerResponse, err := client.CreateProvisionerDaemon(context.Background(),
			codersdk.CreateProvisionerDaemonRequest{
				Name: "foobar",
			},
		)
		require.NoError(t, err)
		token := provisionerResponse.AuthToken
		require.NotNil(t, token)

		doneCh := make(chan error)
		defer func() {
			err := <-doneCh
			require.ErrorIs(t, err, context.Canceled, "provisioner command terminated with error")
		}()

		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()

		cmd, root := clitest.New(t, "provisioners", "run",
			"--token", token.String(),
			"--verbose", // to test debug-level logs
			"--test.use-echo-provisioner",
		)
		pty := ptytest.New(t)
		defer pty.Close()
		cmd.SetErr(pty.Output())
		// command should only have access to provisioner auth token, not user credentials
		err = root.URL().Write(client.URL.String())
		require.NoError(t, err)

		go func() {
			defer close(doneCh)
			doneCh <- cmd.ExecuteContext(ctx)
		}()

		pty.ExpectMatch("\tprovisioner client connected")

		source := clitest.CreateTemplateVersionSource(t, &echo.Responses{
			Parse:     echo.ParseComplete,
			Provision: provisionCompleteWithAgent,
		})
		args := []string{
			"templates",
			"create",
			"my-template",
			"--directory", source,
			"--test.provisioner", string(database.ProvisionerTypeEcho),
			"--max-ttl", "24h",
			"--min-autostart-interval", "2h",
		}
		createCmd, root := clitest.New(t, args...)
		clitest.SetupConfig(t, client, root)
		pty = ptytest.New(t)
		defer pty.Close()
		createCmd.SetIn(pty.Input())
		createCmd.SetOut(pty.Output())

		execDone := make(chan error)
		go func() {
			execDone <- createCmd.Execute()
		}()

		matches := []struct {
			match string
			write string
		}{
			{match: "Create and upload", write: "yes"},
			{match: "compute.main"},
			{match: "smith (linux, i386)"},
			{match: "Confirm create?", write: "yes"},
		}
		for _, m := range matches {
			pty.ExpectMatch(m.match)
			if len(m.write) > 0 {
				pty.WriteLine(m.write)
			}
		}

		require.NoError(t, <-execDone)
	})
}

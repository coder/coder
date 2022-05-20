package cli_test

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/pty/ptytest"
)

func TestRequestLogging(t *testing.T) {
	t.Parallel()
	t.Run("List", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		_ = coderdtest.CreateFirstUser(t, client)
		cmd, root := clitest.New(t, "ls", "--log-requests")
		clitest.SetupConfig(t, client, root)
		doneChan := make(chan struct{})
		pty := ptytest.New(t)
		cmd.SetIn(pty.Input())
		cmd.SetOut(pty.Output())
		go func() {
			defer close(doneChan)
			err := cmd.Execute()
			require.NoError(t, err)
		}()
		pty.ExpectMatch("GET " + client.URL.String() + "/api/v2/workspaces 200")
		<-doneChan
	})

	t.Run("NoAuthentication", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		cmd, root := clitest.New(t, "ls", "--log-requests")
		clitest.SetupConfig(t, client, root)
		doneChan := make(chan struct{})
		pty := ptytest.New(t)
		cmd.SetIn(pty.Input())
		cmd.SetOut(pty.Output())
		go func() {
			defer close(doneChan)
			err := cmd.Execute()
			require.Error(t, err)
		}()
		pty.ExpectMatch("GET " + client.URL.String() + "/api/v2/workspaces 401")
		<-doneChan
	})

	t.Run("InvalidUrl", func(t *testing.T) {
		t.Parallel()
		parsedURL, err := url.Parse("invalidprotocol://foobar")
		require.NoError(t, err)
		client := codersdk.New(parsedURL)
		cmd, root := clitest.New(t, "ls", "--log-requests")
		clitest.SetupConfig(t, client, root)
		doneChan := make(chan struct{})
		pty := ptytest.New(t)
		cmd.SetIn(pty.Input())
		cmd.SetOut(pty.Output())
		go func() {
			defer close(doneChan)
			err := cmd.Execute()
			require.Error(t, err)
		}()
		pty.ExpectMatch("GET " + client.URL.String() + "/api/v2/workspaces (err)")
		<-doneChan
	})
}

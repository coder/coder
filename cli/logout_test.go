package cli_test

import (
	"testing"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/pty/ptytest"
	"github.com/stretchr/testify/require"
)

func TestLogout(t *testing.T) {
	t.Parallel()

	// login
	client := coderdtest.New(t, nil)
	coderdtest.CreateFirstUser(t, client)

	doneChan := make(chan struct{})
	root, config := clitest.New(t, "login", "--force-tty", client.URL.String(), "--no-open")
	pty := ptytest.New(t)
	root.SetIn(pty.Input())
	root.SetOut(pty.Output())
	go func() {
		defer close(doneChan)
		err := root.Execute()
		require.NoError(t, err)
	}()

	pty.ExpectMatch("Paste your token here:")
	pty.WriteLine(client.SessionToken)
	pty.ExpectMatch("Welcome to Coder")
	<-doneChan

	// ensure session files exist
	require.FileExists(t, string(config.URL()))
	require.FileExists(t, string(config.Session()))

	logout, _ := clitest.New(t, "logout", "--global-config", string(config))
	err := logout.Execute()
	require.NoError(t, err)
	require.NoFileExists(t, string(config.URL()))
	require.NoFileExists(t, string(config.Session()))
}

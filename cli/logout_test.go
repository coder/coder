package cli_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/cli/config"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/pty/ptytest"
)

func TestLogout(t *testing.T) {
	t.Parallel()
	t.Run("Logout", func(t *testing.T) {
		t.Parallel()

		config := login(t)

		// ensure session files exist
		assert.FileExists(t, string(config.URL()))
		assert.FileExists(t, string(config.Session()))

		logout, _ := clitest.New(t, "logout", "--global-config", string(config))
		err := logout.Execute()
		assert.NoError(t, err)
		assert.NoFileExists(t, string(config.URL()))
		assert.NoFileExists(t, string(config.Session()))
	})
	t.Run("NoURLFile", func(t *testing.T) {
		t.Parallel()

		logout, _ := clitest.New(t, "logout")

		err := logout.Execute()
		assert.EqualError(t, err, "You are not logged in. Try logging in using 'coder login <url>'.")
	})
	t.Run("NoSessionFile", func(t *testing.T) {
		t.Parallel()

		config := login(t)

		// ensure session files exist
		assert.FileExists(t, string(config.URL()))
		assert.FileExists(t, string(config.Session()))

		os.RemoveAll(string(config.Session()))

		logout, _ := clitest.New(t, "logout", "--global-config", string(config))

		err := logout.Execute()
		assert.EqualError(t, err, "You are not logged in. Try logging in using 'coder login <url>'.")
	})
}

func login(t *testing.T) config.Root {
	t.Helper()

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
		assert.NoError(t, err)
	}()

	pty.ExpectMatch("Paste your token here:")
	pty.WriteLine(client.SessionToken)
	pty.ExpectMatch("Welcome to Coder")
	<-doneChan

	return config
}

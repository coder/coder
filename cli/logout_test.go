package cli_test

import (
	"os"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/cli/config"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/pty/ptytest"
)

func TestLogout(t *testing.T) {
	t.Parallel()
	t.Run("Logout", func(t *testing.T) {
		t.Parallel()

		pty := ptytest.New(t)
		config := login(t, pty)

		// Ensure session files exist.
		require.FileExists(t, string(config.URL()))
		require.FileExists(t, string(config.Session()))

		logoutChan := make(chan struct{})
		logout, _ := clitest.New(t, "logout", "--global-config", string(config))
		logout.SetIn(pty.Input())
		logout.SetOut(pty.Output())

		go func() {
			defer close(logoutChan)
			err := logout.Execute()
			assert.NoError(t, err)
			assert.NoFileExists(t, string(config.URL()))
			assert.NoFileExists(t, string(config.Session()))
		}()

		pty.ExpectMatch("Are you sure you want to logout?")
		pty.WriteLine("yes")
		pty.ExpectMatch("You are no longer logged in. Try logging in using 'coder login <url>'.")
		<-logoutChan
	})
	t.Run("SkipPrompt", func(t *testing.T) {
		t.Parallel()

		pty := ptytest.New(t)
		config := login(t, pty)

		// Ensure session files exist.
		require.FileExists(t, string(config.URL()))
		require.FileExists(t, string(config.Session()))

		logoutChan := make(chan struct{})
		logout, _ := clitest.New(t, "logout", "--global-config", string(config), "-y")
		logout.SetIn(pty.Input())
		logout.SetOut(pty.Output())

		go func() {
			defer close(logoutChan)
			err := logout.Execute()
			assert.NoError(t, err)
			assert.NoFileExists(t, string(config.URL()))
			assert.NoFileExists(t, string(config.Session()))
		}()

		pty.ExpectMatch("You are no longer logged in. Try logging in using 'coder login <url>'.")
		<-logoutChan
	})
	t.Run("NoURLFile", func(t *testing.T) {
		t.Parallel()

		pty := ptytest.New(t)
		config := login(t, pty)

		// Ensure session files exist.
		require.FileExists(t, string(config.URL()))
		require.FileExists(t, string(config.Session()))

		err := os.Remove(string(config.URL()))
		require.NoError(t, err)

		logoutChan := make(chan struct{})
		logout, _ := clitest.New(t, "logout", "--global-config", string(config))

		logout.SetIn(pty.Input())
		logout.SetOut(pty.Output())

		go func() {
			defer close(logoutChan)
			err := logout.Execute()
			assert.EqualError(t, err, "You are not logged in. Try logging in using 'coder login <url>'.")
		}()

		<-logoutChan
	})
	t.Run("NoSessionFile", func(t *testing.T) {
		t.Parallel()

		pty := ptytest.New(t)
		config := login(t, pty)

		// Ensure session files exist.
		require.FileExists(t, string(config.URL()))
		require.FileExists(t, string(config.Session()))

		err := os.Remove(string(config.Session()))
		require.NoError(t, err)

		logoutChan := make(chan struct{})
		logout, _ := clitest.New(t, "logout", "--global-config", string(config))

		logout.SetIn(pty.Input())
		logout.SetOut(pty.Output())

		go func() {
			defer close(logoutChan)
			err = logout.Execute()
			assert.EqualError(t, err, "You are not logged in. Try logging in using 'coder login <url>'.")
		}()

		<-logoutChan
	})
	t.Run("ConfigDirectoryPermissionDenied", func(t *testing.T) {
		t.Parallel()

		pty := ptytest.New(t)
		config := login(t, pty)

		// Ensure session files exist.
		require.FileExists(t, string(config.URL()))
		require.FileExists(t, string(config.Session()))

		// Changing the permissions to throw error during deletion.
		err := os.Chmod(string(config), 0500)
		require.NoError(t, err)

		logoutChan := make(chan struct{})
		logout, _ := clitest.New(t, "logout", "--global-config", string(config))

		logout.SetIn(pty.Input())
		logout.SetOut(pty.Output())

		go func() {
			defer close(logoutChan)
			err := logout.Execute()
			errRegex := regexp.MustCompile("Failed to log out.\n\tremove URL file: .+: permission denied\n\tremove session file: .+: permission denied")
			assert.Regexp(t, errRegex, err.Error())
		}()

		pty.ExpectMatch("Are you sure you want to logout?")
		pty.WriteLine("yes")
		<-logoutChan

		// Setting the permissions back for cleanup.
		err = os.Chmod(string(config), 0700)
		require.NoError(t, err)
	})
}

func login(t *testing.T, pty *ptytest.PTY) config.Root {
	t.Helper()

	client := coderdtest.New(t, nil)
	coderdtest.CreateFirstUser(t, client)

	doneChan := make(chan struct{})
	root, cfg := clitest.New(t, "login", "--force-tty", client.URL.String(), "--no-open")
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

	return cfg
}

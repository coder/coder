package cli_test

import (
	"fmt"
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/cli/config"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/pty/ptytest"
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
		logout.Stdin = pty.Input()
		logout.Stdout = pty.Output()

		go func() {
			defer close(logoutChan)
			err := logout.Run()
			assert.NoError(t, err)
			assert.NoFileExists(t, string(config.URL()))
			assert.NoFileExists(t, string(config.Session()))
		}()

		pty.ExpectMatch("Are you sure you want to log out?")
		pty.WriteLine("yes")
		pty.ExpectMatch("You are no longer logged in. You can log in using 'coder login <url>'.")
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
		logout.Stdin = pty.Input()
		logout.Stdout = pty.Output()

		go func() {
			defer close(logoutChan)
			err := logout.Run()
			assert.NoError(t, err)
			assert.NoFileExists(t, string(config.URL()))
			assert.NoFileExists(t, string(config.Session()))
		}()

		pty.ExpectMatch("You are no longer logged in. You can log in using 'coder login <url>'.")
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

		logout.Stdin = pty.Input()
		logout.Stdout = pty.Output()

		executable, err := os.Executable()
		require.NoError(t, err)
		require.NotEqual(t, "", executable)

		go func() {
			defer close(logoutChan)
			err = logout.Run()
			require.Contains(t, err.Error(), fmt.Sprintf("Try logging in using '%s login <url>'.", executable))
		}()

		<-logoutChan
	})
	t.Run("CannotDeleteFiles", func(t *testing.T) {
		t.Parallel()

		pty := ptytest.New(t)
		config := login(t, pty)

		// Ensure session files exist.
		require.FileExists(t, string(config.URL()))
		require.FileExists(t, string(config.Session()))

		var (
			err         error
			urlFile     *os.File
			sessionFile *os.File
		)
		if runtime.GOOS == "windows" {
			// Opening the files so Windows does not allow deleting them.
			urlFile, err = os.Open(string(config.URL()))
			require.NoError(t, err)
			sessionFile, err = os.Open(string(config.Session()))
			require.NoError(t, err)
		} else {
			// Changing the permissions to throw error during deletion.
			err = os.Chmod(string(config), 0o500)
			require.NoError(t, err)
		}
		defer func() {
			if runtime.GOOS == "windows" {
				// Closing the opened files for cleanup.
				err = urlFile.Close()
				assert.NoError(t, err)
				err = sessionFile.Close()
				assert.NoError(t, err)
			} else {
				// Setting the permissions back for cleanup.
				err = os.Chmod(string(config), 0o700)
				assert.NoError(t, err)
			}
		}()

		logout, _ := clitest.New(t, "logout", "--global-config", string(config))

		logout.Stdin = pty.Input()
		logout.Stdout = pty.Output()

		go func() {
			pty.ExpectMatch("Are you sure you want to log out?")
			pty.WriteLine("yes")
		}()
		err = logout.Run()
		require.Error(t, err)

		t.Logf("err: %v", err)

		var wantError string
		if runtime.GOOS == "windows" {
			wantError = "The process cannot access the file because it is being used by another process."
		} else {
			wantError = "permission denied"
		}
		require.ErrorContains(t, err, wantError)
	})
}

func login(t *testing.T, pty *ptytest.PTY) config.Root {
	t.Helper()

	client := coderdtest.New(t, nil)
	coderdtest.CreateFirstUser(t, client)

	doneChan := make(chan struct{})
	root, cfg := clitest.New(t, "login", "--force-tty", client.URL.String(), "--no-open")
	root.Stdin = pty.Input()
	root.Stdout = pty.Output()
	go func() {
		defer close(doneChan)
		err := root.Run()
		assert.NoError(t, err)
	}()

	pty.ExpectMatch("Paste your token here:")
	pty.WriteLine(client.SessionToken())
	pty.ExpectMatch("Welcome to Coder")
	<-doneChan

	return cfg
}

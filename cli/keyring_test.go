package cli_test

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"os"
	"path"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli"
	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/cli/sessionstore"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/pty/ptytest"
)

// keyringTestServiceName generates a unique test service name for use with the OS keyring.
// It uses a combination of the test name, a timestamp, and a random number to prevent
// collisions between parallel tests.
func keyringTestServiceName(t *testing.T) string {
	t.Helper()
	var n uint32
	err := binary.Read(rand.Reader, binary.BigEndian, &n)
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%s_%d_%d", t.Name(), time.Now().UnixNano(), n)
}

func TestUseKeyring(t *testing.T) {
	t.Parallel()

	// Only run on platforms where keyring is supported
	if runtime.GOOS != "windows" && runtime.GOOS != "darwin" {
		t.Skip("keyring storage only supported on Windows and macOS")
	}

	t.Run("Login", func(t *testing.T) {
		t.Parallel()

		// Create a test server
		client := coderdtest.New(t, nil)
		coderdtest.CreateFirstUser(t, client)

		// Create a pty for interactive prompts
		pty := ptytest.New(t)

		// Create a unique keyring service name to avoid collisions with parallel tests
		serviceName := keyringTestServiceName(t)
		keyringBackend := sessionstore.NewKeyringWithService(serviceName)

		// Create CLI invocation with unique keyring backend
		var root cli.RootCmd
		cmd, err := root.Command(root.AGPL())
		require.NoError(t, err)
		root.WithSessionStorageBackend(keyringBackend)

		inv, cfg := clitest.NewWithCommand(t, cmd,
			"login",
			"--force-tty",
			"--no-open",
			client.URL.String(),
		)
		inv.Stdin = pty.Input()
		inv.Stdout = pty.Output()

		// Run login in background
		doneChan := make(chan struct{})
		go func() {
			defer close(doneChan)
			err := inv.Run()
			assert.NoError(t, err)
		}()

		// Provide the token when prompted
		pty.ExpectMatch("Paste your token here:")
		pty.WriteLine(client.SessionToken())
		pty.ExpectMatch("Welcome to Coder")
		<-doneChan

		// Verify that session file was NOT created (using keyring instead)
		sessionFile := path.Join(string(cfg), "session")
		_, err = os.Stat(sessionFile)
		require.True(t, os.IsNotExist(err), "session file should not exist when using keyring")

		// Clean up: remove from keyring
		t.Cleanup(func() {
			_ = keyringBackend.Delete(client.URL) // Best effort cleanup
		})
	})

	t.Run("Logout", func(t *testing.T) {
		t.Parallel()

		// Create a test server
		client := coderdtest.New(t, nil)
		coderdtest.CreateFirstUser(t, client)

		// Create a pty for interactive prompts
		pty := ptytest.New(t)

		// Create a unique keyring service name to avoid collisions with parallel tests
		serviceName := keyringTestServiceName(t)
		keyringBackend := sessionstore.NewKeyringWithService(serviceName)

		// First, login using keyring with unique service name
		var loginRoot cli.RootCmd
		loginCmd, err := loginRoot.Command(loginRoot.AGPL())
		require.NoError(t, err)
		loginRoot.WithSessionStorageBackend(keyringBackend)

		loginInv, cfg := clitest.NewWithCommand(t, loginCmd,
			"login",
			"--force-tty",
			"--no-open",
			client.URL.String(),
		)
		loginInv.Stdin = pty.Input()
		loginInv.Stdout = pty.Output()

		doneChan := make(chan struct{})
		go func() {
			defer close(doneChan)
			err := loginInv.Run()
			assert.NoError(t, err)
		}()

		pty.ExpectMatch("Paste your token here:")
		pty.WriteLine(client.SessionToken())
		pty.ExpectMatch("Welcome to Coder")
		<-doneChan

		// Now run logout using the same keyring backend
		var logoutRoot cli.RootCmd
		logoutCmd, err := logoutRoot.Command(logoutRoot.AGPL())
		require.NoError(t, err)
		logoutRoot.WithSessionStorageBackend(keyringBackend)

		logoutInv, _ := clitest.NewWithCommand(t, logoutCmd,
			"logout",
			"--yes",
			"--global-config", string(cfg),
		)

		var logoutOut bytes.Buffer
		logoutInv.Stdout = &logoutOut

		err = logoutInv.Run()
		require.NoError(t, err, "logout should succeed")

		// Verify the session file still doesn't exist after logout
		sessionFile := path.Join(string(cfg), "session")
		_, err = os.Stat(sessionFile)
		require.True(t, os.IsNotExist(err), "session file should not exist after keyring logout")

		// Verify the credential was actually deleted from keyring
		_, err = keyringBackend.Read(client.URL)
		require.ErrorIs(t, err, os.ErrNotExist, "credential should be deleted from keyring")
	})
}

func TestUseKeyringUnsupportedOS(t *testing.T) {
	t.Parallel()

	// Only run this on an unsupported OS.
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		t.Skipf("Skipping unsupported OS test on %s where keyring is supported", runtime.GOOS)
	}

	t.Run("LoginWithDeprecatedFlag", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		coderdtest.CreateFirstUser(t, client)
		pty := ptytest.New(t)

		// Login with deprecated --use-keyring=true flag (should be ignored)
		// Use NewWithCommand to exercise the real code path that will
		// automatically use file-based storage on Linux
		var root cli.RootCmd
		cmd, err := root.Command(root.AGPL())
		require.NoError(t, err)

		inv, cfg := clitest.NewWithCommand(t, cmd,
			"login",
			"--force-tty",
			"--use-keyring=true",
			"--no-open",
			client.URL.String(),
		)
		inv.Stdin = pty.Input()
		inv.Stdout = pty.Output()

		doneChan := make(chan struct{})
		go func() {
			defer close(doneChan)
			err := inv.Run()
			assert.NoError(t, err)
		}()

		pty.ExpectMatch("Paste your token here:")
		pty.WriteLine(client.SessionToken())
		pty.ExpectMatch("Welcome to Coder")
		<-doneChan

		// Verify file storage was used (flag was ignored and Linux defaults to file)
		sessionFile := path.Join(string(cfg), "session")
		_, err = os.Stat(sessionFile)
		require.NoError(t, err, "session file should exist - flag is deprecated and Linux uses file storage")

		// Read and verify the token from file
		content, err := os.ReadFile(sessionFile)
		require.NoError(t, err, "should be able to read session file")
		require.Equal(t, client.SessionToken(), string(content), "file should contain the session token")
	})
}

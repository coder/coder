package cli_test

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"net/url"
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
	"github.com/coder/serpent"
)

// keyringTestServiceName generates a unique service name for keyring tests
// using the test name and a nanosecond timestamp to prevent collisions.
func keyringTestServiceName(t *testing.T) string {
	t.Helper()
	var n uint32
	err := binary.Read(rand.Reader, binary.BigEndian, &n)
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%s_%v_%d", t.Name(), time.Now().UnixNano(), n)
}

// instrumentKeyring sets up the CLI invocation to use the actual OS keyring
// with a unique test service name to allow test parallelization. It returns
// the backend and URL for verification of keyring contents in tests.
func instrumentKeyring(t *testing.T, inv *serpent.Invocation, serverURL string) (sessionstore.Backend, *url.URL) {
	t.Helper()

	serviceName := keyringTestServiceName(t)
	backend := sessionstore.NewKeyringWithService(serviceName)

	srvURL, err := url.Parse(serverURL)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = backend.Delete(srvURL)
	})

	var root cli.RootCmd
	cmd, err := root.Command(root.AGPL())
	require.NoError(t, err)
	root.WithSessionStorageBackend(backend)
	inv.Command = cmd

	return backend, srvURL
}

func TestUseKeyring(t *testing.T) {
	// Verify that the --use-keyring flag default opts into using a keyring backend
	// for storing session tokens instead of plain text files.
	t.Parallel()

	t.Run("Login", func(t *testing.T) {
		t.Parallel()

		if runtime.GOOS != "windows" && runtime.GOOS != "darwin" {
			t.Skip("keyring is not supported on this OS")
		}

		// Create a test server
		client := coderdtest.New(t, nil)
		coderdtest.CreateFirstUser(t, client)

		// Create a pty for interactive prompts
		pty := ptytest.New(t)

		// Create CLI invocation which defaults to using the keyring
		inv, cfg := clitest.New(t,
			"login",
			"--force-tty",
			"--no-open",
			client.URL.String(),
		)
		inv.Stdin = pty.Input()
		inv.Stdout = pty.Output()

		backend, srvURL := instrumentKeyring(t, inv, client.URL.String())

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
		_, err := os.Stat(sessionFile)
		require.True(t, os.IsNotExist(err), "session file should not exist when using keyring")

		// Verify that the credential IS stored in OS keyring
		cred, err := backend.Read(srvURL)
		require.NoError(t, err, "credential should be stored in OS keyring")
		require.Equal(t, client.SessionToken(), cred, "stored token should match login token")
	})

	t.Run("Logout", func(t *testing.T) {
		t.Parallel()

		if runtime.GOOS != "windows" && runtime.GOOS != "darwin" {
			t.Skip("keyring is not supported on this OS")
		}

		// Create a test server
		client := coderdtest.New(t, nil)
		coderdtest.CreateFirstUser(t, client)

		// Create a pty for interactive prompts
		pty := ptytest.New(t)

		// First, login with the keyring (default)
		loginInv, cfg := clitest.New(t,
			"login",
			"--force-tty",
			"--no-open",
			client.URL.String(),
		)
		loginInv.Stdin = pty.Input()
		loginInv.Stdout = pty.Output()

		backend, srvURL := instrumentKeyring(t, loginInv, client.URL.String())

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

		// Verify credential exists in OS keyring
		cred, err := backend.Read(srvURL)
		require.NoError(t, err, "read credential should succeed before logout")
		require.NotEmpty(t, cred, "credential should exist before logout")

		// Now logout
		logoutInv, _ := clitest.New(t,
			"logout",
			"--yes",
			"--global-config", string(cfg),
		)

		// Instrument logout with the same backend
		var logoutRoot cli.RootCmd
		logoutCmd, err := logoutRoot.Command(logoutRoot.AGPL())
		require.NoError(t, err)
		logoutRoot.WithSessionStorageBackend(backend)
		logoutInv.Command = logoutCmd

		var logoutOut bytes.Buffer
		logoutInv.Stdout = &logoutOut

		err = logoutInv.Run()
		require.NoError(t, err, "logout should succeed")

		// Verify the credential was deleted from OS keyring
		_, err = backend.Read(srvURL)
		require.ErrorIs(t, err, os.ErrNotExist, "credential should be deleted from keyring after logout")
	})

	t.Run("DefaultFileStorage", func(t *testing.T) {
		t.Parallel()

		if runtime.GOOS != "linux" {
			t.Skip("file storage is the default for Linux")
		}

		// Create a test server
		client := coderdtest.New(t, nil)
		coderdtest.CreateFirstUser(t, client)

		// Create a pty for interactive prompts
		pty := ptytest.New(t)

		inv, cfg := clitest.New(t,
			"login",
			"--force-tty",
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

		// Verify that session file WAS created (not using keyring)
		sessionFile := path.Join(string(cfg), "session")
		_, err := os.Stat(sessionFile)
		require.NoError(t, err, "session file should exist when NOT using --use-keyring on Linux")

		// Read and verify the token from file
		content, err := os.ReadFile(sessionFile)
		require.NoError(t, err, "should be able to read session file")
		require.Equal(t, client.SessionToken(), string(content), "file should contain the session token")
	})

	t.Run("EnvironmentVariable", func(t *testing.T) {
		t.Parallel()

		// Create a test server
		client := coderdtest.New(t, nil)
		coderdtest.CreateFirstUser(t, client)

		// Create a pty for interactive prompts
		pty := ptytest.New(t)

		// Login using CODER_USE_KEYRING environment variable set to disable keyring usage,
		// which should have the same behavior on all platforms.
		inv, cfg := clitest.New(t,
			"login",
			"--force-tty",
			"--no-open",
			client.URL.String(),
		)
		inv.Stdin = pty.Input()
		inv.Stdout = pty.Output()
		inv.Environ.Set("CODER_USE_KEYRING", "false")

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

		// Verify that session file WAS created (not using keyring)
		sessionFile := path.Join(string(cfg), "session")
		_, err := os.Stat(sessionFile)
		require.NoError(t, err, "session file should exist when CODER_USE_KEYRING set to false")

		// Read and verify the token from file
		content, err := os.ReadFile(sessionFile)
		require.NoError(t, err, "should be able to read session file")
		require.Equal(t, client.SessionToken(), string(content), "file should contain the session token")
	})

	t.Run("DisableKeyringWithFlag", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		coderdtest.CreateFirstUser(t, client)
		pty := ptytest.New(t)

		// Login with --use-keyring=false to explicitly disable keyring usage, which
		// should have the same behavior on all platforms.
		inv, cfg := clitest.New(t,
			"login",
			"--use-keyring=false",
			"--force-tty",
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

		// Verify that session file WAS created (not using keyring)
		sessionFile := path.Join(string(cfg), "session")
		_, err := os.Stat(sessionFile)
		require.NoError(t, err, "session file should exist when --use-keyring=false is specified")

		// Read and verify the token from file
		content, err := os.ReadFile(sessionFile)
		require.NoError(t, err, "should be able to read session file")
		require.Equal(t, client.SessionToken(), string(content), "file should contain the session token")
	})
}

func TestUseKeyringUnsupportedOS(t *testing.T) {
	// Verify that trying to use --use-keyring on an unsupported operating system produces
	// a helpful error message.
	t.Parallel()

	// Only run this on an unsupported OS.
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		t.Skipf("Skipping unsupported OS test on %s where keyring is supported", runtime.GOOS)
	}

	const expMessage = "keyring storage is not supported on this operating system; omit --use-keyring to use file-based storage"

	t.Run("LoginWithUnsupportedKeyring", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		coderdtest.CreateFirstUser(t, client)

		// Try to login with --use-keyring on an unsupported OS
		inv, _ := clitest.New(t,
			"login",
			"--use-keyring",
			client.URL.String(),
		)

		// The error should occur immediately, before any prompts
		loginErr := inv.Run()

		// Verify we got an error about unsupported OS
		require.Error(t, loginErr)
		require.Contains(t, loginErr.Error(), expMessage)
	})

	t.Run("LogoutWithUnsupportedKeyring", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		coderdtest.CreateFirstUser(t, client)
		pty := ptytest.New(t)

		// First login without keyring to create a session (default behavior)
		loginInv, cfg := clitest.New(t,
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

		// Now try to logout with --use-keyring on an unsupported OS
		logoutInv, _ := clitest.New(t,
			"logout",
			"--use-keyring",
			"--yes",
			"--global-config", string(cfg),
		)

		err := logoutInv.Run()
		// Verify we got an error about unsupported OS
		require.Error(t, err)
		require.Contains(t, err.Error(), expMessage)
	})
}

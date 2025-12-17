package cli_test

import (
	"bytes"
	"net/url"
	"os"
	"path"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli"
	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/cli/config"
	"github.com/coder/coder/v2/cli/sessionstore"
	"github.com/coder/coder/v2/cli/sessionstore/testhelpers"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/serpent"
)

type keyringTestEnv struct {
	serviceName string
	keyring     sessionstore.Keyring
	inv         *serpent.Invocation
	cfg         config.Root
	clientURL   *url.URL
}

func setupKeyringTestEnv(t *testing.T, clientURL string, args ...string) keyringTestEnv {
	t.Helper()

	var root cli.RootCmd

	cmd, err := root.Command(root.AGPL())
	require.NoError(t, err)

	serviceName := testhelpers.KeyringServiceName(t)
	root.WithKeyringServiceName(serviceName)
	root.UseKeyringWithGlobalConfig()

	inv, cfg := clitest.NewWithDefaultKeyringCommand(t, cmd, args...)

	parsedURL, err := url.Parse(clientURL)
	require.NoError(t, err)

	backend := sessionstore.NewKeyringWithService(serviceName)
	t.Cleanup(func() {
		_ = backend.Delete(parsedURL)
	})

	return keyringTestEnv{serviceName, backend, inv, cfg, parsedURL}
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
		env := setupKeyringTestEnv(t, client.URL.String(),
			"login",
			"--force-tty",
			"--no-open",
			client.URL.String())
		inv := env.inv
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
		sessionFile := path.Join(string(env.cfg), "session")
		_, err := os.Stat(sessionFile)
		require.True(t, os.IsNotExist(err), "session file should not exist when using keyring")

		// Verify that the credential IS stored in OS keyring
		cred, err := env.keyring.Read(env.clientURL)
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
		env := setupKeyringTestEnv(t, client.URL.String(),
			"login",
			"--force-tty",
			"--no-open",
			client.URL.String(),
		)
		loginInv := env.inv
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

		// Verify credential exists in OS keyring
		cred, err := env.keyring.Read(env.clientURL)
		require.NoError(t, err, "read credential should succeed before logout")
		require.NotEmpty(t, cred, "credential should exist before logout")

		// Now logout using the same keyring service name
		var logoutRoot cli.RootCmd
		logoutCmd, err := logoutRoot.Command(logoutRoot.AGPL())
		require.NoError(t, err)
		logoutRoot.WithKeyringServiceName(env.serviceName)
		logoutRoot.UseKeyringWithGlobalConfig()

		logoutInv, _ := clitest.NewWithDefaultKeyringCommand(t, logoutCmd,
			"logout",
			"--yes",
			"--global-config", string(env.cfg),
		)

		var logoutOut bytes.Buffer
		logoutInv.Stdout = &logoutOut

		err = logoutInv.Run()
		require.NoError(t, err, "logout should succeed")

		// Verify the credential was deleted from OS keyring
		_, err = env.keyring.Read(env.clientURL)
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

		env := setupKeyringTestEnv(t, client.URL.String(),
			"login",
			"--force-tty",
			"--no-open",
			client.URL.String(),
		)
		inv := env.inv
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
		sessionFile := path.Join(string(env.cfg), "session")
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
		env := setupKeyringTestEnv(t, client.URL.String(),
			"login",
			"--force-tty",
			"--no-open",
			client.URL.String(),
		)
		inv := env.inv
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
		sessionFile := path.Join(string(env.cfg), "session")
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
		env := setupKeyringTestEnv(t, client.URL.String(),
			"login",
			"--use-keyring=false",
			"--force-tty",
			"--no-open",
			client.URL.String(),
		)
		inv := env.inv
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
		sessionFile := path.Join(string(env.cfg), "session")
		_, err := os.Stat(sessionFile)
		require.NoError(t, err, "session file should exist when --use-keyring=false is specified")

		// Read and verify the token from file
		content, err := os.ReadFile(sessionFile)
		require.NoError(t, err, "should be able to read session file")
		require.Equal(t, client.SessionToken(), string(content), "file should contain the session token")
	})
}

func TestUseKeyringUnsupportedOS(t *testing.T) {
	// Verify that on unsupported operating systems, file-based storage is used
	// automatically even when --use-keyring is set to true (the default).
	t.Parallel()

	// Only run this on an unsupported OS.
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		t.Skipf("Skipping unsupported OS test on %s where keyring is supported", runtime.GOOS)
	}

	t.Run("LoginWithDefaultKeyring", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		coderdtest.CreateFirstUser(t, client)
		pty := ptytest.New(t)

		env := setupKeyringTestEnv(t, client.URL.String(),
			"login",
			"--force-tty",
			"--no-open",
			client.URL.String(),
		)
		inv := env.inv
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

		// Verify that session file WAS created (automatic fallback to file storage)
		sessionFile := path.Join(string(env.cfg), "session")
		_, err := os.Stat(sessionFile)
		require.NoError(t, err, "session file should exist due to automatic fallback to file storage")

		content, err := os.ReadFile(sessionFile)
		require.NoError(t, err, "should be able to read session file")
		require.Equal(t, client.SessionToken(), string(content), "file should contain the session token")
	})

	t.Run("LogoutWithDefaultKeyring", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		coderdtest.CreateFirstUser(t, client)
		pty := ptytest.New(t)

		// First login to create a session (will use file storage due to automatic fallback)
		env := setupKeyringTestEnv(t, client.URL.String(),
			"login",
			"--force-tty",
			"--no-open",
			client.URL.String(),
		)
		loginInv := env.inv
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

		// Verify session file exists
		sessionFile := path.Join(string(env.cfg), "session")
		_, err := os.Stat(sessionFile)
		require.NoError(t, err, "session file should exist before logout")

		// Now logout - should succeed and delete the file
		logoutEnv := setupKeyringTestEnv(t, client.URL.String(),
			"logout",
			"--yes",
			"--global-config", string(env.cfg),
		)

		err = logoutEnv.inv.Run()
		require.NoError(t, err, "logout should succeed with automatic file storage fallback")

		_, err = os.Stat(sessionFile)
		require.True(t, os.IsNotExist(err), "session file should be deleted after logout")
	})
}

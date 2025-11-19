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
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/pty/ptytest"
)

// mockKeyring is a mock sessionstore.Backend implementation.
type mockKeyring struct {
	credentials map[string]string // service name -> credential
}

const mockServiceName = "mock-service-name"

func newMockKeyring() *mockKeyring {
	return &mockKeyring{credentials: make(map[string]string)}
}

func (m *mockKeyring) Read(_ *url.URL) (string, error) {
	cred, ok := m.credentials[mockServiceName]
	if !ok {
		return "", os.ErrNotExist
	}
	return cred, nil
}

func (m *mockKeyring) Write(_ *url.URL, token string) error {
	m.credentials[mockServiceName] = token
	return nil
}

func (m *mockKeyring) Delete(_ *url.URL) error {
	_, ok := m.credentials[mockServiceName]
	if !ok {
		return os.ErrNotExist
	}
	delete(m.credentials, mockServiceName)
	return nil
}

func TestUseKeyring(t *testing.T) {
	// Verify that keyring storage works correctly:
	// - On Windows/macOS: keyring is mandatory (mocked via backend injection in tests)
	// - On Linux: file storage is used by default
	// - The --use-keyring flag is deprecated and has no effect
	t.Parallel()

	t.Run("Login", func(t *testing.T) {
		t.Parallel()

		// Create a test server
		client := coderdtest.New(t, nil)
		coderdtest.CreateFirstUser(t, client)

		// Create a pty for interactive prompts
		pty := ptytest.New(t)

		// Create CLI invocation (uses mock keyring backend via injection)
		inv, cfg := clitest.New(t,
			"login",
			"--force-tty",
			"--use-keyring", // Deprecated flag (ignored)
			"--no-open",
			client.URL.String(),
		)
		inv.Stdin = pty.Input()
		inv.Stdout = pty.Output()

		// Inject the mock backend before running the command
		var root cli.RootCmd
		cmd, err := root.Command(root.AGPL())
		require.NoError(t, err)
		mockBackend := newMockKeyring()
		root.WithSessionStorageBackend(mockBackend)
		inv.Command = cmd

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

		// Verify that the credential IS stored in mock keyring
		cred, err := mockBackend.Read(nil)
		require.NoError(t, err, "credential should be stored in mock keyring")
		require.Equal(t, client.SessionToken(), cred, "stored token should match login token")
	})

	t.Run("Logout", func(t *testing.T) {
		t.Parallel()

		// Create a test server
		client := coderdtest.New(t, nil)
		coderdtest.CreateFirstUser(t, client)

		// Create a pty for interactive prompts
		pty := ptytest.New(t)

		// First, login (uses mock keyring backend via injection)
		loginInv, cfg := clitest.New(t,
			"login",
			"--force-tty",
			"--use-keyring", // Deprecated flag (ignored)
			"--no-open",
			client.URL.String(),
		)
		loginInv.Stdin = pty.Input()
		loginInv.Stdout = pty.Output()

		// Inject the mock backend
		var loginRoot cli.RootCmd
		loginCmd, err := loginRoot.Command(loginRoot.AGPL())
		require.NoError(t, err)
		mockBackend := newMockKeyring()
		loginRoot.WithSessionStorageBackend(mockBackend)
		loginInv.Command = loginCmd

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

		// Verify credential exists in mock keyring
		cred, err := mockBackend.Read(nil)
		require.NoError(t, err, "read credential should succeed before logout")
		require.NotEmpty(t, cred, "credential should exist after logout")

		// Now run logout (uses same mock keyring backend)
		logoutInv, _ := clitest.New(t,
			"logout",
			"--use-keyring", // Deprecated flag (ignored)
			"--yes",
			"--global-config", string(cfg),
		)

		// Inject the same mock backend
		var logoutRoot cli.RootCmd
		logoutCmd, err := logoutRoot.Command(logoutRoot.AGPL())
		require.NoError(t, err)
		logoutRoot.WithSessionStorageBackend(mockBackend)
		logoutInv.Command = logoutCmd

		var logoutOut bytes.Buffer
		logoutInv.Stdout = &logoutOut

		err = logoutInv.Run()
		require.NoError(t, err, "logout should succeed")

		// Verify the credential was deleted from mock keyring
		_, err = mockBackend.Read(nil)
		require.ErrorIs(t, err, os.ErrNotExist, "credential should be deleted from keyring after logout")
	})

	t.Run("MockBackend", func(t *testing.T) {
		t.Parallel()

		// clitest.New() injects a file-based backend for all platforms. This
		// test verifies that session tokens are stored correctly in tests.

		// Create a test server
		client := coderdtest.New(t, nil)
		coderdtest.CreateFirstUser(t, client)

		// Create a pty for interactive prompts
		pty := ptytest.New(t)

		// clitest.New injects file backend for us
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

		// Verify the session file was created by the injected file backend
		sessionFile := path.Join(string(cfg), "session")
		_, err := os.Stat(sessionFile)
		require.NoError(t, err, "session file should exist from injected file backend")

		// Read and verify the token
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

		// Login using CODER_USE_KEYRING environment variable instead of flag
		inv, cfg := clitest.New(t,
			"login",
			"--force-tty",
			"--no-open",
			client.URL.String(),
		)
		inv.Stdin = pty.Input()
		inv.Stdout = pty.Output()
		inv.Environ.Set("CODER_USE_KEYRING", "true")

		// Inject the mock backend
		var root cli.RootCmd
		cmd, err := root.Command(root.AGPL())
		require.NoError(t, err)
		mockBackend := newMockKeyring()
		root.WithSessionStorageBackend(mockBackend)
		inv.Command = cmd

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

		// Verify that session file was NOT created (using keyring via env var)
		sessionFile := path.Join(string(cfg), "session")
		_, err = os.Stat(sessionFile)
		require.True(t, os.IsNotExist(err), "session file should not exist when using keyring via env var")

		// Verify credential is in mock keyring
		cred, err := mockBackend.Read(nil)
		require.NoError(t, err, "credential should be stored in keyring when CODER_USE_KEYRING=true")
		require.NotEmpty(t, cred)
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
		inv, cfg := clitest.New(t,
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

		// Verify file storage was used (flag was ignored)
		sessionFile := path.Join(string(cfg), "session")
		_, err := os.Stat(sessionFile)
		require.NoError(t, err, "session file should exist - flag is deprecated and ignored")

		// Read and verify the token from file
		content, err := os.ReadFile(sessionFile)
		require.NoError(t, err, "should be able to read session file")
		require.Equal(t, client.SessionToken(), string(content), "file should contain the session token")
	})
}

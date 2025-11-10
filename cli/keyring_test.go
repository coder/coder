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
	// Verify that the --use-keyring flag opts into using a keyring backend for
	// storing session tokens instead of plain text files.
	t.Parallel()

	t.Run("Login", func(t *testing.T) {
		t.Parallel()

		// Create a test server
		client := coderdtest.New(t, nil)
		coderdtest.CreateFirstUser(t, client)

		// Create a pty for interactive prompts
		pty := ptytest.New(t)

		// Create CLI invocation with --use-keyring flag
		inv, cfg := clitest.New(t,
			"login",
			"--force-tty",
			"--use-keyring",
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

		// First, login with --use-keyring
		loginInv, cfg := clitest.New(t,
			"login",
			"--force-tty",
			"--use-keyring",
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

		// Now run logout with --use-keyring
		logoutInv, _ := clitest.New(t,
			"logout",
			"--use-keyring",
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

	t.Run("OmitFlag", func(t *testing.T) {
		t.Parallel()

		// Create a test server
		client := coderdtest.New(t, nil)
		coderdtest.CreateFirstUser(t, client)

		// Create a pty for interactive prompts
		pty := ptytest.New(t)

		// --use-keyring flag omitted (should use file-based storage)
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
		require.NoError(t, err, "session file should exist when NOT using --use-keyring")

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
	// Verify that trying to use --use-keyring on an unsupported operating system produces
	// a helpful error message.
	t.Parallel()

	// Skip on Windows since the keyring is actually supported.
	if runtime.GOOS == "windows" {
		t.Skip("Skipping unsupported OS test on Windows where keyring is supported")
	}

	const expMessage = "keyring storage is not supported on this operating system; remove the --use-keyring flag"

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

		// First login without keyring to create a session
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

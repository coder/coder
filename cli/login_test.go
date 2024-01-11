package cli_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/pretty"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/pty/ptytest"
)

func TestLogin(t *testing.T) {
	t.Parallel()
	t.Run("InitialUserNoTTY", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		root, _ := clitest.New(t, "login", client.URL.String())
		err := root.Run()
		require.Error(t, err)
	})

	t.Run("InitialUserBadLoginURL", func(t *testing.T) {
		t.Parallel()
		badLoginURL := "https://fcca2077f06e68aaf9"
		root, _ := clitest.New(t, "login", badLoginURL)
		err := root.Run()
		errMsg := fmt.Sprintf("Failed to check server %q for first user, is the URL correct and is coder accessible from your browser?", badLoginURL)
		require.ErrorContains(t, err, errMsg)
	})

	t.Run("InitialUserNonCoderURLFail", func(t *testing.T) {
		t.Parallel()

		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("Not Found"))
		}))
		defer ts.Close()

		badLoginURL := ts.URL
		root, _ := clitest.New(t, "login", badLoginURL)
		err := root.Run()
		errMsg := fmt.Sprintf("Failed to check server %q for first user, is the URL correct and is coder accessible from your browser?", badLoginURL)
		require.ErrorContains(t, err, errMsg)
	})

	t.Run("InitialUserNonCoderURLSuccess", func(t *testing.T) {
		t.Parallel()

		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set(codersdk.BuildVersionHeader, "something")
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("Not Found"))
		}))
		defer ts.Close()

		badLoginURL := ts.URL
		root, _ := clitest.New(t, "login", badLoginURL)
		err := root.Run()
		// this means we passed the check for a valid coder server
		require.ErrorContains(t, err, "the initial user cannot be created in non-interactive mode")
	})

	t.Run("InitialUserTTY", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		// The --force-tty flag is required on Windows, because the `isatty` library does not
		// accurately detect Windows ptys when they are not attached to a process:
		// https://github.com/mattn/go-isatty/issues/59
		doneChan := make(chan struct{})
		root, _ := clitest.New(t, "login", "--force-tty", client.URL.String())
		pty := ptytest.New(t).Attach(root)
		go func() {
			defer close(doneChan)
			err := root.Run()
			assert.NoError(t, err)
		}()

		matches := []string{
			"first user?", "yes",
			"username", "testuser",
			"email", "user@coder.com",
			"password", "SomeSecurePassword!",
			"password", "SomeSecurePassword!", // Confirm.
			"trial", "yes",
		}
		for i := 0; i < len(matches); i += 2 {
			match := matches[i]
			value := matches[i+1]
			pty.ExpectMatch(match)
			pty.WriteLine(value)
		}
		pty.ExpectMatch("Welcome to Coder")
		<-doneChan
	})

	t.Run("InitialUserTTYFlag", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		// The --force-tty flag is required on Windows, because the `isatty` library does not
		// accurately detect Windows ptys when they are not attached to a process:
		// https://github.com/mattn/go-isatty/issues/59
		inv, _ := clitest.New(t, "--url", client.URL.String(), "login", "--force-tty")
		pty := ptytest.New(t).Attach(inv)

		clitest.Start(t, inv)

		matches := []string{
			"first user?", "yes",
			"username", "testuser",
			"email", "user@coder.com",
			"password", "SomeSecurePassword!",
			"password", "SomeSecurePassword!", // Confirm.
			"trial", "yes",
		}
		for i := 0; i < len(matches); i += 2 {
			match := matches[i]
			value := matches[i+1]
			pty.ExpectMatch(match)
			pty.WriteLine(value)
		}
		pty.ExpectMatch("Welcome to Coder")
	})

	t.Run("InitialUserFlags", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		inv, _ := clitest.New(
			t, "login", client.URL.String(),
			"--first-user-username", "testuser", "--first-user-email", "user@coder.com",
			"--first-user-password", "SomeSecurePassword!", "--first-user-trial",
		)
		pty := ptytest.New(t).Attach(inv)
		w := clitest.StartWithWaiter(t, inv)
		pty.ExpectMatch("Welcome to Coder")
		w.RequireSuccess()
	})

	t.Run("InitialUserTTYConfirmPasswordFailAndReprompt", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		client := coderdtest.New(t, nil)
		// The --force-tty flag is required on Windows, because the `isatty` library does not
		// accurately detect Windows ptys when they are not attached to a process:
		// https://github.com/mattn/go-isatty/issues/59
		doneChan := make(chan struct{})
		root, _ := clitest.New(t, "login", "--force-tty", client.URL.String())
		pty := ptytest.New(t).Attach(root)
		go func() {
			defer close(doneChan)
			err := root.WithContext(ctx).Run()
			assert.NoError(t, err)
		}()

		matches := []string{
			"first user?", "yes",
			"username", "testuser",
			"email", "user@coder.com",
			"password", "MyFirstSecurePassword!",
			"password", "MyNonMatchingSecurePassword!", // Confirm.
		}
		for i := 0; i < len(matches); i += 2 {
			match := matches[i]
			value := matches[i+1]
			pty.ExpectMatch(match)
			pty.WriteLine(value)
		}

		// Validate that we reprompt for matching passwords.
		pty.ExpectMatch("Passwords do not match")
		pty.ExpectMatch("Enter a " + pretty.Sprint(cliui.DefaultStyles.Field, "password"))

		pty.WriteLine("SomeSecurePassword!")
		pty.ExpectMatch("Confirm")
		pty.WriteLine("SomeSecurePassword!")
		pty.ExpectMatch("trial")
		pty.WriteLine("yes")
		pty.ExpectMatch("Welcome to Coder")
		<-doneChan
	})

	t.Run("ExistingUserValidTokenTTY", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		coderdtest.CreateFirstUser(t, client)

		doneChan := make(chan struct{})
		root, _ := clitest.New(t, "login", "--force-tty", client.URL.String(), "--no-open")
		pty := ptytest.New(t).Attach(root)
		go func() {
			defer close(doneChan)
			err := root.Run()
			assert.NoError(t, err)
		}()

		pty.ExpectMatch("Paste your token here:")
		pty.WriteLine(client.SessionToken())
		if runtime.GOOS != "windows" {
			// For some reason, the match does not show up on Windows.
			pty.ExpectMatch(client.SessionToken())
		}
		pty.ExpectMatch("Welcome to Coder")
		<-doneChan
	})

	t.Run("ExistingUserInvalidTokenTTY", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		coderdtest.CreateFirstUser(t, client)

		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()
		doneChan := make(chan struct{})
		root, _ := clitest.New(t, "login", client.URL.String(), "--no-open")
		pty := ptytest.New(t).Attach(root)
		go func() {
			defer close(doneChan)
			err := root.WithContext(ctx).Run()
			// An error is expected in this case, since the login wasn't successful:
			assert.Error(t, err)
		}()

		pty.ExpectMatch("Paste your token here:")
		pty.WriteLine("an-invalid-token")
		if runtime.GOOS != "windows" {
			// For some reason, the match does not show up on Windows.
			pty.ExpectMatch("an-invalid-token")
		}
		pty.ExpectMatch("That's not a valid token!")
		cancelFunc()
		<-doneChan
	})

	// TokenFlag should generate a new session token and store it in the session file.
	t.Run("TokenFlag", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		coderdtest.CreateFirstUser(t, client)
		root, cfg := clitest.New(t, "login", client.URL.String(), "--token", client.SessionToken())
		err := root.Run()
		require.NoError(t, err)
		sessionFile, err := cfg.Session().Read()
		require.NoError(t, err)
		// This **should not be equal** to the token we passed in.
		require.NotEqual(t, client.SessionToken(), sessionFile)
	})
}

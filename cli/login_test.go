package cli_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/pretty"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
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
			"username", coderdtest.FirstUserParams.Username,
			"name", coderdtest.FirstUserParams.Name,
			"email", coderdtest.FirstUserParams.Email,
			"password", coderdtest.FirstUserParams.Password,
			"password", coderdtest.FirstUserParams.Password, // confirm
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
		ctx := testutil.Context(t, testutil.WaitShort)
		resp, err := client.LoginWithPassword(ctx, codersdk.LoginWithPasswordRequest{
			Email:    coderdtest.FirstUserParams.Email,
			Password: coderdtest.FirstUserParams.Password,
		})
		require.NoError(t, err)
		client.SetSessionToken(resp.SessionToken)
		me, err := client.User(ctx, codersdk.Me)
		require.NoError(t, err)
		assert.Equal(t, coderdtest.FirstUserParams.Username, me.Username)
		assert.Equal(t, coderdtest.FirstUserParams.Name, me.Name)
		assert.Equal(t, coderdtest.FirstUserParams.Email, me.Email)
	})

	t.Run("InitialUserTTYNameOptional", func(t *testing.T) {
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
			"username", coderdtest.FirstUserParams.Username,
			"name", "",
			"email", coderdtest.FirstUserParams.Email,
			"password", coderdtest.FirstUserParams.Password,
			"password", coderdtest.FirstUserParams.Password, // confirm
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
		ctx := testutil.Context(t, testutil.WaitShort)
		resp, err := client.LoginWithPassword(ctx, codersdk.LoginWithPasswordRequest{
			Email:    coderdtest.FirstUserParams.Email,
			Password: coderdtest.FirstUserParams.Password,
		})
		require.NoError(t, err)
		client.SetSessionToken(resp.SessionToken)
		me, err := client.User(ctx, codersdk.Me)
		require.NoError(t, err)
		assert.Equal(t, coderdtest.FirstUserParams.Username, me.Username)
		assert.Equal(t, coderdtest.FirstUserParams.Email, me.Email)
		assert.Empty(t, me.Name)
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

		pty.ExpectMatch(fmt.Sprintf("Attempting to authenticate with flag URL: '%s'", client.URL.String()))
		matches := []string{
			"first user?", "yes",
			"username", coderdtest.FirstUserParams.Username,
			"name", coderdtest.FirstUserParams.Name,
			"email", coderdtest.FirstUserParams.Email,
			"password", coderdtest.FirstUserParams.Password,
			"password", coderdtest.FirstUserParams.Password, // confirm
			"trial", "yes",
		}
		for i := 0; i < len(matches); i += 2 {
			match := matches[i]
			value := matches[i+1]
			pty.ExpectMatch(match)
			pty.WriteLine(value)
		}
		pty.ExpectMatch("Welcome to Coder")
		ctx := testutil.Context(t, testutil.WaitShort)
		resp, err := client.LoginWithPassword(ctx, codersdk.LoginWithPasswordRequest{
			Email:    coderdtest.FirstUserParams.Email,
			Password: coderdtest.FirstUserParams.Password,
		})
		require.NoError(t, err)
		client.SetSessionToken(resp.SessionToken)
		me, err := client.User(ctx, codersdk.Me)
		require.NoError(t, err)
		assert.Equal(t, coderdtest.FirstUserParams.Username, me.Username)
		assert.Equal(t, coderdtest.FirstUserParams.Name, me.Name)
		assert.Equal(t, coderdtest.FirstUserParams.Email, me.Email)
	})

	t.Run("InitialUserFlags", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		inv, _ := clitest.New(
			t, "login", client.URL.String(),
			"--first-user-username", coderdtest.FirstUserParams.Username,
			"--first-user-full-name", coderdtest.FirstUserParams.Name,
			"--first-user-email", coderdtest.FirstUserParams.Email,
			"--first-user-password", coderdtest.FirstUserParams.Password,
			"--first-user-trial",
		)
		pty := ptytest.New(t).Attach(inv)
		w := clitest.StartWithWaiter(t, inv)
		pty.ExpectMatch("Welcome to Coder")
		w.RequireSuccess()
		ctx := testutil.Context(t, testutil.WaitShort)
		resp, err := client.LoginWithPassword(ctx, codersdk.LoginWithPasswordRequest{
			Email:    coderdtest.FirstUserParams.Email,
			Password: coderdtest.FirstUserParams.Password,
		})
		require.NoError(t, err)
		client.SetSessionToken(resp.SessionToken)
		me, err := client.User(ctx, codersdk.Me)
		require.NoError(t, err)
		assert.Equal(t, coderdtest.FirstUserParams.Username, me.Username)
		assert.Equal(t, coderdtest.FirstUserParams.Name, me.Name)
		assert.Equal(t, coderdtest.FirstUserParams.Email, me.Email)
	})

	t.Run("InitialUserFlagsNameOptional", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		inv, _ := clitest.New(
			t, "login", client.URL.String(),
			"--first-user-username", coderdtest.FirstUserParams.Username,
			"--first-user-email", coderdtest.FirstUserParams.Email,
			"--first-user-password", coderdtest.FirstUserParams.Password,
			"--first-user-trial",
		)
		pty := ptytest.New(t).Attach(inv)
		w := clitest.StartWithWaiter(t, inv)
		pty.ExpectMatch("Welcome to Coder")
		w.RequireSuccess()
		ctx := testutil.Context(t, testutil.WaitShort)
		resp, err := client.LoginWithPassword(ctx, codersdk.LoginWithPasswordRequest{
			Email:    coderdtest.FirstUserParams.Email,
			Password: coderdtest.FirstUserParams.Password,
		})
		require.NoError(t, err)
		client.SetSessionToken(resp.SessionToken)
		me, err := client.User(ctx, codersdk.Me)
		require.NoError(t, err)
		assert.Equal(t, coderdtest.FirstUserParams.Username, me.Username)
		assert.Equal(t, coderdtest.FirstUserParams.Email, me.Email)
		assert.Empty(t, me.Name)
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
			"username", coderdtest.FirstUserParams.Username,
			"name", coderdtest.FirstUserParams.Name,
			"email", coderdtest.FirstUserParams.Email,
			"password", coderdtest.FirstUserParams.Password,
			"password", "something completely different",
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

		pty.WriteLine(coderdtest.FirstUserParams.Password)
		pty.ExpectMatch("Confirm")
		pty.WriteLine(coderdtest.FirstUserParams.Password)
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

		pty.ExpectMatch(fmt.Sprintf("Attempting to authenticate with argument URL: '%s'", client.URL.String()))
		pty.ExpectMatch("Paste your token here:")
		pty.WriteLine(client.SessionToken())
		if runtime.GOOS != "windows" {
			// For some reason, the match does not show up on Windows.
			pty.ExpectMatch(client.SessionToken())
		}
		pty.ExpectMatch("Welcome to Coder")
		<-doneChan
	})

	t.Run("ExistingUserURLSavedInConfig", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		url := client.URL.String()
		coderdtest.CreateFirstUser(t, client)

		inv, root := clitest.New(t, "login", "--no-open")
		clitest.SetupConfig(t, client, root)

		doneChan := make(chan struct{})
		pty := ptytest.New(t).Attach(inv)
		go func() {
			defer close(doneChan)
			err := inv.Run()
			assert.NoError(t, err)
		}()

		pty.ExpectMatch(fmt.Sprintf("Attempting to authenticate with config URL: '%s'", url))
		pty.ExpectMatch("Paste your token here:")
		pty.WriteLine(client.SessionToken())
		<-doneChan
	})

	t.Run("ExistingUserURLSavedInEnv", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		url := client.URL.String()
		coderdtest.CreateFirstUser(t, client)

		inv, _ := clitest.New(t, "login", "--no-open")
		inv.Environ.Set("CODER_URL", url)

		doneChan := make(chan struct{})
		pty := ptytest.New(t).Attach(inv)
		go func() {
			defer close(doneChan)
			err := inv.Run()
			assert.NoError(t, err)
		}()

		pty.ExpectMatch(fmt.Sprintf("Attempting to authenticate with environment URL: '%s'", url))
		pty.ExpectMatch("Paste your token here:")
		pty.WriteLine(client.SessionToken())
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

	// Login should reset the configured organization if the user is not a member
	t.Run("ResetOrganization", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		coderdtest.CreateFirstUser(t, client)
		root, cfg := clitest.New(t, "login", client.URL.String(), "--token", client.SessionToken())

		notRealOrg := uuid.NewString()
		err := cfg.Organization().Write(notRealOrg)
		require.NoError(t, err, "write bad org to config")

		err = root.Run()
		require.NoError(t, err)
		sessionFile, err := cfg.Session().Read()
		require.NoError(t, err)
		require.NotEqual(t, client.SessionToken(), sessionFile)

		// Organization config should be deleted since the org does not exist
		selected, err := cfg.Organization().Read()
		require.ErrorIs(t, err, os.ErrNotExist)
		require.NotEqual(t, selected, notRealOrg)
	})

	t.Run("KeepOrganizationContext", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		first := coderdtest.CreateFirstUser(t, client)
		root, cfg := clitest.New(t, "login", client.URL.String(), "--token", client.SessionToken())

		err := cfg.Organization().Write(first.OrganizationID.String())
		require.NoError(t, err, "write bad org to config")

		err = root.Run()
		require.NoError(t, err)
		sessionFile, err := cfg.Session().Read()
		require.NoError(t, err)
		require.NotEqual(t, client.SessionToken(), sessionFile)

		// Organization config should be deleted since the org does not exist
		selected, err := cfg.Organization().Read()
		require.NoError(t, err)
		require.Equal(t, selected, first.OrganizationID.String())
	})
}

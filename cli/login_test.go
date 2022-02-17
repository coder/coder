package cli_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/console"
)

func TestLogin(t *testing.T) {
	t.Parallel()
	t.Run("InitialUserNoTTY", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		root, _ := clitest.New(t, "login", client.URL.String())
		err := root.Execute()
		require.Error(t, err)
	})

	t.Run("InitialUserTTY", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		// The --force-tty flag is required on Windows, because the `isatty` library does not
		// accurately detect Windows ptys when they are not attached to a process:
		// https://github.com/mattn/go-isatty/issues/59
		root, _ := clitest.New(t, "login", client.URL.String(), "--force-tty")
		cons := console.New(t, root)
		go func() {
			err := root.Execute()
			require.NoError(t, err)
		}()

		matches := []string{
			"first user?", "y",
			"username", "testuser",
			"organization", "testorg",
			"email", "user@coder.com",
			"password", "password",
		}
		for i := 0; i < len(matches); i += 2 {
			match := matches[i]
			value := matches[i+1]
			_, err := cons.ExpectString(match)
			require.NoError(t, err)
			_, err = cons.SendLine(value)
			require.NoError(t, err)
		}
		_, err := cons.ExpectString("Welcome to Coder")
		require.NoError(t, err)
	})

	t.Run("ExistingUserValidTokenTTY", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		_, err := client.CreateInitialUser(context.Background(), coderd.CreateInitialUserRequest{
			Username:     "test-user",
			Email:        "test-user@coder.com",
			Organization: "acme-corp",
			Password:     "password",
		})
		require.NoError(t, err)
		token, err := client.LoginWithPassword(context.Background(), coderd.LoginWithPasswordRequest{
			Email:    "test-user@coder.com",
			Password: "password",
		})
		require.NoError(t, err)

		root, _ := clitest.New(t, "login", client.URL.String(), "--force-tty")
		cons := console.New(t, root)
		go func() {
			err := root.Execute()
			require.NoError(t, err)
		}()

		_, err = cons.ExpectString("Paste your token here:")
		require.NoError(t, err)
		_, err = cons.SendLine(token.SessionToken)
		require.NoError(t, err)
		_, err = cons.ExpectString("Welcome to Coder")
		require.NoError(t, err)
	})

	t.Run("ExistingUserInvalidTokenTTY", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		_, err := client.CreateInitialUser(context.Background(), coderd.CreateInitialUserRequest{
			Username:     "test-user",
			Email:        "test-user@coder.com",
			Organization: "acme-corp",
			Password:     "password",
		})
		require.NoError(t, err)

		root, _ := clitest.New(t, "login", client.URL.String(), "--force-tty")
		cons := console.New(t, root)
		go func() {
			err := root.Execute()
			require.Error(t, err)
		}()

		_, err = cons.ExpectString("Paste your token here:")
		require.NoError(t, err)
		_, err = cons.SendLine("an-invalid-token")
		require.NoError(t, err)
		_, err = cons.ExpectString("That's not a valid token!")
		require.NoError(t, err)
	})
}

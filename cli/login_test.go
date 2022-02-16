package cli_test

import (
	"testing"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/console"
	"github.com/stretchr/testify/require"
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
}

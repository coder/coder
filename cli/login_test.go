package cli_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
<<<<<<< HEAD

	"github.com/coder/coder/expect"
||||||| df13fef
	"github.com/stretchr/testify/require"

	"github.com/Netflix/go-expect"
=======
	"github.com/stretchr/testify/require"
>>>>>>> main
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
<<<<<<< HEAD
		// The --force-tty flag is required on Windows, because the `isatty` library does not
		// accurately detect Windows ptys when they are not attached to a process:
		// https://github.com/mattn/go-isatty/issues/59
		root, _ := clitest.New(t, "login", client.URL.String(), "--force-tty")
		root.SetIn(console.InTty())
		root.SetOut(console.OutTty())
||||||| df13fef
		root, _ := clitest.New(t, "login", client.URL.String())
		root.SetIn(console.Tty())
		root.SetOut(console.Tty())
=======
		root, _ := clitest.New(t, "login", client.URL.String())
		console := clitest.NewConsole(t, root)
>>>>>>> main
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
			_, err := console.ExpectString(match)
			require.NoError(t, err)
			_, err = console.SendLine(value)
			require.NoError(t, err)
		}
		_, err := console.ExpectString("Welcome to Coder")
		require.NoError(t, err)
	})
}

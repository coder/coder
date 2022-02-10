//go:build !windows

package cli_test

import (
	"testing"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/stretchr/testify/require"

	"github.com/Netflix/go-expect"
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
		console, err := expect.NewConsole(expect.WithStdout(clitest.StdoutLogs(t)))
		require.NoError(t, err)
		client := coderdtest.New(t)
		root, _ := clitest.New(t, "login", client.URL.String())
		root.SetIn(console.Tty())
		root.SetOut(console.Tty())
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
			_, err = console.ExpectString(match)
			require.NoError(t, err)
			_, err = console.SendLine(value)
			require.NoError(t, err)
		}
		_, err = console.ExpectString("Welcome to Coder")
		require.NoError(t, err)
	})
}

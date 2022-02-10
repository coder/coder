package cli_test

import (
	"testing"

	"github.com/Netflix/go-expect"
	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/stretchr/testify/require"
)

func TestProjectCreate(t *testing.T) {
	t.Parallel()
	t.Run("InitialUserTTY", func(t *testing.T) {
		t.Parallel()
		console, err := expect.NewConsole(expect.WithStdout(clitest.StdoutLogs(t)))
		require.NoError(t, err)
		client := coderdtest.New(t)
		directory := t.TempDir()
		cmd, root := clitest.New(t, "projects", "create", "--directory", directory)
		_ = clitest.CreateInitialUser(t, client, root)
		cmd.SetIn(console.Tty())
		cmd.SetOut(console.Tty())
		go func() {
			err := cmd.Execute()
			require.NoError(t, err)
		}()

		matches := []string{
			"organization?", "y",
			"name?", "",
		}
		for i := 0; i < len(matches); i += 2 {
			match := matches[i]
			value := matches[i+1]
			_, err = console.ExpectString(match)
			require.NoError(t, err)
			_, err = console.SendLine(value)
			require.NoError(t, err)
		}
	})
}

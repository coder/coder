package cli_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/pty/ptytest"
)

func TestProjectInit(t *testing.T) {
	t.Parallel()
	t.Run("Extract", func(t *testing.T) {
		t.Parallel()
		tempDir := t.TempDir()
		cmd, _ := clitest.New(t, "projects", "init", tempDir)
		doneChan := make(chan struct{})
		pty := ptytest.New(t)
		cmd.SetIn(pty.Input())
		cmd.SetOut(pty.Output())
		go func() {
			defer close(doneChan)
			err := cmd.Execute()
			require.NoError(t, err)
		}()
		<-doneChan
		files, err := os.ReadDir(tempDir)
		require.NoError(t, err)
		require.Greater(t, len(files), 0)
	})
}

package cli_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/pty/ptytest"
)

func TestTemplateInit(t *testing.T) {
	t.Parallel()
	t.Run("Extract", func(t *testing.T) {
		t.Parallel()
		tempDir := t.TempDir()
		inv, _ := clitest.New(t, "templates", "init", tempDir)
		ptytest.New(t).Attach(inv)
		clitest.Run(t, inv)
		files, err := os.ReadDir(tempDir)
		require.NoError(t, err)
		require.Greater(t, len(files), 0)
	})

	t.Run("ExtractSpecific", func(t *testing.T) {
		t.Parallel()
		tempDir := t.TempDir()
		inv, _ := clitest.New(t, "templates", "init", "--id", "docker", tempDir)
		ptytest.New(t).Attach(inv)
		clitest.Run(t, inv)
		files, err := os.ReadDir(tempDir)
		require.NoError(t, err)
		require.Greater(t, len(files), 0)
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		tempDir := t.TempDir()
		inv, _ := clitest.New(t, "templates", "init", "--id", "thistemplatedoesnotexist", tempDir)
		ptytest.New(t).Attach(inv)
		err := inv.Run()
		require.ErrorContains(t, err, "invalid choice: thistemplatedoesnotexist, should be one of")
		files, err := os.ReadDir(tempDir)
		require.NoError(t, err)
		require.Empty(t, files)
	})
}

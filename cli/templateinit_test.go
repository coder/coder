package cli_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
)

func TestTemplateInit(t *testing.T) {
	t.Parallel()
	t.Run("Extract", func(t *testing.T) {
		t.Parallel()
		tempDir := t.TempDir()
		inv, _ := clitest.New(t, "templates", "init", tempDir)
		clitest.Run(t, inv)
		files, err := os.ReadDir(tempDir)
		require.NoError(t, err)
		require.Greater(t, len(files), 0)
	})

	t.Run("ExtractSpecific", func(t *testing.T) {
		t.Parallel()
		tempDir := t.TempDir()
		inv, _ := clitest.New(t, "templates", "init", "--id", "docker", tempDir)
		clitest.Run(t, inv)
		files, err := os.ReadDir(tempDir)
		require.NoError(t, err)
		require.Greater(t, len(files), 0)
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		tempDir := t.TempDir()
		inv, _ := clitest.New(t, "templates", "init", "--id", "thistemplatedoesnotexist", tempDir)
		err := inv.Run()
		require.ErrorContains(t, err, "invalid choice: thistemplatedoesnotexist, should be one of")
		files, err := os.ReadDir(tempDir)
		require.NoError(t, err)
		require.Empty(t, files)
	})
}

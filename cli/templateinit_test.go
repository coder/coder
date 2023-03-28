package cli_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/pty/ptytest"
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
}

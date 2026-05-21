package cli_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
)

func TestExpChatContextAdd(t *testing.T) {
	t.Parallel()

	t.Run("RequiresWorkspaceOrDir", func(t *testing.T) {
		t.Parallel()

		inv, _ := clitest.New(t, "exp", "chat", "context", "add")

		err := inv.Run()
		require.Error(t, err)
		require.Contains(t, err.Error(), "this command must be run inside a Coder workspace")
	})

	t.Run("AllowsExplicitDir", func(t *testing.T) {
		t.Parallel()

		inv, _ := clitest.New(t, "exp", "chat", "context", "add", "--dir", t.TempDir())

		err := inv.Run()
		if err != nil {
			require.NotContains(t, err.Error(), "this command must be run inside a Coder workspace")
		}
	})

	t.Run("AllowsWorkspaceEnv", func(t *testing.T) {
		t.Parallel()

		inv, _ := clitest.New(t, "exp", "chat", "context", "add")
		inv.Environ.Set("CODER", "true")

		err := inv.Run()
		if err != nil {
			require.NotContains(t, err.Error(), "this command must be run inside a Coder workspace")
		}
	})
}

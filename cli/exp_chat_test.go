package cli_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
)

func TestExpChatContextAdd(t *testing.T) {
	t.Parallel()

	t.Run("RequiresPathArgument", func(t *testing.T) {
		t.Parallel()

		// `add` registers a context source identified by <path>, so the path
		// argument is required and a bare invocation is a usage error.
		inv, _ := clitest.New(t, "exp", "chat", "context", "add")

		err := inv.Run()
		require.Error(t, err)
		require.Contains(t, err.Error(), "wanted 1 args but got 0")
	})

	t.Run("RequiresWorkspaceSocket", func(t *testing.T) {
		t.Parallel()

		// Source registration talks to the agent over its local socket, so
		// outside a workspace it fails to connect rather than silently doing
		// nothing. Point at a socket path that does not exist so the dial
		// fails deterministically (and never touches a real agent socket).
		missingSocket := filepath.Join(t.TempDir(), "agent.sock")
		inv, _ := clitest.New(t, "exp", "chat", "context", "add", t.TempDir(),
			"--socket-path", missingSocket)

		err := inv.Run()
		require.Error(t, err)
		require.Contains(t, err.Error(), "inside the workspace")
	})
}

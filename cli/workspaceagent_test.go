package cli_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clitest"
)

func TestWorkspaceAgent(t *testing.T) {
	t.Parallel()
	cmd, _ := clitest.New(t, "workspaces", "agent")
	require.NoError(t, cmd.Execute())
}

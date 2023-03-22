package cli_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/cli/config"
	"github.com/coder/coder/enterprise/cli"
)

func newCLI(t *testing.T, args ...string) (*clibase.Invocation, config.Root) {
	var root cli.RootCmd
	cmd, err := root.Command(root.EnterpriseSubcommands())
	require.NoError(t, err)
	return clitest.NewWithCommand(t, cmd, args...)
}

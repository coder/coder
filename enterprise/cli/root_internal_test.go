package cli

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli"
	"github.com/coder/coder/v2/cli/clibase"
	"github.com/coder/coder/v2/cli/clitest"
)

//nolint:tparallel,paralleltest
func TestEnterpriseCommandHelp(t *testing.T) {
	// Only test the enterprise commands
	getCmds := func(t *testing.T) *clibase.Cmd {
		// Must return a fresh instance of cmds each time.
		t.Helper()
		var root cli.RootCmd
		rootCmd, err := root.Command((&RootCmd{}).enterpriseOnly())
		require.NoError(t, err)

		return rootCmd
	}
	clitest.TestCommandHelp(t, getCmds, clitest.DefaultCases())
}

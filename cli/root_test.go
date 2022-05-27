package cli_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli"
	"github.com/coder/coder/cli/clitest"
)

func TestRoot(t *testing.T) {
	t.Run("FormatCobraError", func(t *testing.T) {
		t.Parallel()

		cmd, _ := clitest.New(t, "delete")

		cmd, err := cmd.ExecuteC()
		errStr := cli.FormatCobraError(err, cmd)
		require.Contains(t, errStr, "Run 'coder delete --help' for usage.")
	})
}

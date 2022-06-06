package cli_test

import (
	"bytes"
	"testing"

	"github.com/coder/coder/buildinfo"

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

	t.Run("TestVersion", func(t *testing.T) {
		t.Parallel()

		buf := new(bytes.Buffer)
		cmd, _ := clitest.New(t, "version")
		cmd.SetOut(buf)
		err := cmd.Execute()
		require.NoError(t, err)

		output := buf.String()
		require.Contains(t, output, buildinfo.Version(), "has version")
		require.Contains(t, output, buildinfo.ExternalURL(), "has url")
	})
}

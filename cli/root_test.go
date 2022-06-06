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

		err := cmd.Execute()
		errStr := cli.FormatCobraError(err, cmd)
		require.Contains(t, errStr, "Run 'coder delete --help' for usage.")
	})

	// The "-v" short flag should return the usage output, not a version output
	t.Run("TestVShortFlag", func(t *testing.T) {
		t.Parallel()

		buf := new(bytes.Buffer)
		cmd, _ := clitest.New(t, "-v")
		cmd.SetOut(buf)

		err := cmd.Execute()
		require.NoError(t, err)
		// Check the usage string is the output when using "-v"
		require.Contains(t, buf.String(), cmd.UsageString())
		require.Contains(t, buf.String(), cmd.Root().Long)
	})

	t.Run("TestVersion", func(t *testing.T) {
		t.Parallel()

		bufFlag := new(bytes.Buffer)
		cmd, _ := clitest.New(t, "--version")
		cmd.SetOut(bufFlag)
		err := cmd.Execute()
		require.NoError(t, err)

		bufCmd := new(bytes.Buffer)
		cmd, _ = clitest.New(t, "version")
		cmd.SetOut(bufCmd)
		err = cmd.Execute()
		require.NoError(t, err)

		require.Equal(t, bufFlag.String(), bufCmd.String(), "cmd and flag identical output")
		require.Contains(t, bufFlag.String(), buildinfo.Version(), "has version")
		require.Contains(t, bufFlag.String(), buildinfo.ExternalURL(), "has url")
	})
}

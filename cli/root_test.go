package cli_test

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/buildinfo"
	"github.com/coder/coder/cli"
	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/codersdk"
)

func TestRoot(t *testing.T) {
	t.Run("FormatCobraError", func(t *testing.T) {
		t.Parallel()

		t.Run("OK", func(t *testing.T) {
			t.Parallel()

			cmd, _ := clitest.New(t, "delete")

			cmd, err := cmd.ExecuteC()
			errStr := cli.FormatCobraError(err, cmd)
			require.Contains(t, errStr, "Run 'coder delete --help' for usage.")
		})

		t.Run("Verbose", func(t *testing.T) {
			t.Parallel()

			// Test that the verbose error is masked without verbose flag.
			t.Run("NoVerboseAPIError", func(t *testing.T) {
				t.Parallel()

				cmd, _ := clitest.New(t)

				cmd.RunE = func(cmd *cobra.Command, args []string) error {
					var err error = &codersdk.Error{
						Response: codersdk.Response{
							Message: "This is a message.",
						},
						Helper: "Try this instead.",
					}

					err = xerrors.Errorf("wrap me: %w", err)

					return err
				}

				cmd, err := cmd.ExecuteC()
				errStr := cli.FormatCobraError(err, cmd)
				require.Contains(t, errStr, "This is a message. Try this instead.")
				require.NotContains(t, errStr, err.Error())
			})

			// Assert that a regular error is not masked when verbose is not
			// specified.
			t.Run("NoVerboseRegularError", func(t *testing.T) {
				t.Parallel()

				cmd, _ := clitest.New(t)

				cmd.RunE = func(cmd *cobra.Command, args []string) error {
					return xerrors.Errorf("this is a non-codersdk error: %w", xerrors.Errorf("a wrapped error"))
				}

				cmd, err := cmd.ExecuteC()
				errStr := cli.FormatCobraError(err, cmd)
				require.Contains(t, errStr, err.Error())
			})

			// Test that both the friendly error and the verbose error are
			// displayed when verbose is passed.
			t.Run("APIError", func(t *testing.T) {
				t.Parallel()

				cmd, _ := clitest.New(t, "--verbose")

				cmd.RunE = func(cmd *cobra.Command, args []string) error {
					var err error = &codersdk.Error{
						Response: codersdk.Response{
							Message: "This is a message.",
						},
						Helper: "Try this instead.",
					}

					err = xerrors.Errorf("wrap me: %w", err)

					return err
				}

				cmd, err := cmd.ExecuteC()
				errStr := cli.FormatCobraError(err, cmd)
				require.Contains(t, errStr, "This is a message. Try this instead.")
				require.Contains(t, errStr, err.Error())
			})

			// Assert that a regular error is not masked when verbose specified.
			t.Run("RegularError", func(t *testing.T) {
				t.Parallel()

				cmd, _ := clitest.New(t, "--verbose")

				cmd.RunE = func(cmd *cobra.Command, args []string) error {
					return xerrors.Errorf("this is a non-codersdk error: %w", xerrors.Errorf("a wrapped error"))
				}

				cmd, err := cmd.ExecuteC()
				errStr := cli.FormatCobraError(err, cmd)
				require.Contains(t, errStr, err.Error())
			})
		})
	})

	t.Run("Version", func(t *testing.T) {
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

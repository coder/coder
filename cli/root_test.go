package cli_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/buildinfo"
	"github.com/coder/coder/cli"
	"github.com/coder/coder/cli/cliflag"
	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/codersdk"
)

func TestRoot(t *testing.T) {
	t.Parallel()
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

	t.Run("Header", func(t *testing.T) {
		t.Parallel()

		done := make(chan struct{})
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "wow", r.Header.Get("X-Testing"))
			w.WriteHeader(http.StatusGone)
			select {
			case <-done:
				close(done)
			default:
			}
		}))
		defer srv.Close()
		buf := new(bytes.Buffer)
		cmd, _ := clitest.New(t, "--header", "X-Testing=wow", "login", srv.URL)
		cmd.SetOut(buf)
		// This won't succeed, because we're using the login cmd to assert requests.
		_ = cmd.Execute()
	})

	t.Run("Experimental", func(t *testing.T) {
		t.Parallel()

		cmd, _ := clitest.New(t, "--experimental")
		err := cmd.Execute()
		require.NoError(t, err)
		require.True(t, cli.ExperimentalEnabled(cmd))

		cmd, _ = clitest.New(t, "help", "--verbose")
		_ = cmd.Execute()
		_, set := cliflag.IsSet(cmd, "verbose")
		require.True(t, set)
		require.ErrorContains(t, cli.EnsureExperimental(cmd, "verbose"), "--experimental")
	})
}

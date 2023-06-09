package cli_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/coder/coder/cli/clibase"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/buildinfo"
	"github.com/coder/coder/cli"
	"github.com/coder/coder/cli/clitest"
)

//nolint:tparallel,paralleltest
func TestCommandHelp(t *testing.T) {
	// Test with AGPL commands
	getCmds := func(t *testing.T) *clibase.Cmd {
		// Must return a fresh instance of cmds each time.

		t.Helper()
		var root cli.RootCmd
		rootCmd, err := root.Command(root.AGPL())
		require.NoError(t, err)

		return rootCmd
	}
	clitest.TestCommandHelp(t, getCmds, append(clitest.DefaultCases(),
		clitest.CommandHelpCase{
			Name: "coder agent --help",
			Cmd:  []string{"agent", "--help"},
		},
		clitest.CommandHelpCase{
			Name: "coder list --output json",
			Cmd:  []string{"list", "--output", "json"},
		},
		clitest.CommandHelpCase{
			Name: "coder users list --output json",
			Cmd:  []string{"users", "list", "--output", "json"},
		},
	))
}

func TestRoot(t *testing.T) {
	t.Parallel()
	t.Run("Version", func(t *testing.T) {
		t.Parallel()

		buf := new(bytes.Buffer)
		inv, _ := clitest.New(t, "version")
		inv.Stdout = buf
		err := inv.Run()
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
		inv, _ := clitest.New(t, "--header", "X-Testing=wow", "login", srv.URL)
		inv.Stdout = buf
		// This won't succeed, because we're using the login cmd to assert requests.
		_ = inv.Run()
	})
}

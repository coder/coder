package cli_test

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/coder/coder/v2/cli/clibase"
	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/cli"
	"github.com/coder/coder/v2/cli/clitest"
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

		var url string
		var called int64
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt64(&called, 1)
			assert.Equal(t, "wow", r.Header.Get("X-Testing"))
			assert.Equal(t, "Dean was Here!", r.Header.Get("Cool-Header"))
			assert.Equal(t, "very-wow-"+url, r.Header.Get("X-Process-Testing"))
			assert.Equal(t, "more-wow", r.Header.Get("X-Process-Testing2"))
			w.WriteHeader(http.StatusGone)
		}))
		defer srv.Close()
		url = srv.URL
		buf := new(bytes.Buffer)
		coderURLEnv := "$CODER_URL"
		if runtime.GOOS == "windows" {
			coderURLEnv = "%CODER_URL%"
		}
		inv, _ := clitest.New(t,
			"--no-feature-warning",
			"--no-version-warning",
			"--header", "X-Testing=wow",
			"--header", "Cool-Header=Dean was Here!",
			"--header-command", "printf X-Process-Testing=very-wow-"+coderURLEnv+"'\\r\\n'X-Process-Testing2=more-wow",
			"login", srv.URL,
		)
		inv.Stdout = buf

		err := inv.Run()
		require.Error(t, err)
		require.ErrorContains(t, err, "unexpected status code 410")
		require.EqualValues(t, 1, atomic.LoadInt64(&called), "called exactly once")
	})
}

// TestDERPHeaders ensures that the client sends the global `--header`s and
// `--header-command` to the DERP server when connecting.
func TestDERPHeaders(t *testing.T) {
	t.Parallel()

	// Create a coderd API instance the hard way since we need to change the
	// handler to inject our custom /derp handler.
	dv := coderdtest.DeploymentValues(t)
	dv.DERP.Config.BlockDirect = true
	setHandler, cancelFunc, serverURL, newOptions := coderdtest.NewOptions(t, &coderdtest.Options{
		DeploymentValues: dv,
	})

	// We set the handler after server creation for the access URL.
	coderAPI := coderd.New(newOptions)
	setHandler(coderAPI.RootHandler)
	provisionerCloser := coderdtest.NewProvisionerDaemon(t, coderAPI)
	t.Cleanup(func() {
		_ = provisionerCloser.Close()
	})
	client := codersdk.New(serverURL)
	t.Cleanup(func() {
		cancelFunc()
		_ = provisionerCloser.Close()
		_ = coderAPI.Close()
		client.HTTPClient.CloseIdleConnections()
	})

	var (
		admin     = coderdtest.CreateFirstUser(t, client)
		member, _ = coderdtest.CreateAnotherUser(t, client, admin.OrganizationID)
		workspace = runAgent(t, client, member)
	)

	// Inject custom /derp handler so we can inspect the headers.
	var (
		expectedHeaders = map[string]string{
			"X-Test-Header":     "test-value",
			"Cool-Header":       "Dean was Here!",
			"X-Process-Testing": "very-wow",
		}
		derpCalled int64
	)
	setHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/derp") {
			ok := true
			for k, v := range expectedHeaders {
				if r.Header.Get(k) != v {
					ok = false
					break
				}
			}
			if ok {
				// Only increment if all the headers are set, because the agent
				// calls derp also.
				atomic.AddInt64(&derpCalled, 1)
			}
		}

		coderAPI.RootHandler.ServeHTTP(w, r)
	}))

	// Connect with the headers set as args.
	args := []string{
		"-v",
		"--no-feature-warning",
		"--no-version-warning",
		"ping", workspace.Name,
		"-n", "1",
		"--header-command", "printf X-Process-Testing=very-wow",
	}
	for k, v := range expectedHeaders {
		if k != "X-Process-Testing" {
			args = append(args, "--header", fmt.Sprintf("%s=%s", k, v))
		}
	}
	inv, root := clitest.New(t, args...)
	clitest.SetupConfig(t, member, root)
	pty := ptytest.New(t)
	inv.Stdin = pty.Input()
	inv.Stderr = pty.Output()
	inv.Stdout = pty.Output()

	ctx := testutil.Context(t, testutil.WaitLong)
	cmdDone := tGo(t, func() {
		err := inv.WithContext(ctx).Run()
		assert.NoError(t, err)
	})

	pty.ExpectMatch("pong from " + workspace.Name)
	<-cmdDone

	require.Greater(t, atomic.LoadInt64(&derpCalled), int64(0), "expected /derp to be called at least once")
}

func TestHandlersOK(t *testing.T) {
	t.Parallel()

	var root cli.RootCmd
	cmd, err := root.Command(root.Core())
	require.NoError(t, err)

	clitest.HandlersOK(t, cmd)
}

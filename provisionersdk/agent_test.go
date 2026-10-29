//go:build !windows
// +build !windows

// There isn't a portable Windows binary equivalent to "echo".
// This can be tested, but it's a bit harder.

package provisionersdk_test

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/provisionersdk"
	"github.com/coder/coder/v2/testutil"
)

// mimicking the --version output which we use to test the binary (see provisionersdk/scripts/bootstrap_*).
const versionOutput = `Coder v2.11.0+8979bfe Tue May  7 17:30:19 UTC 2024`

// bashEcho is a script that calls the local `echo` with the arguments.  This is preferable to
// sending the real `echo` binary since macOS 14.4+ immediately sigkills `echo` if it is copied to
// another directory and run locally.
const bashEcho = `#!/usr/bin/env bash
echo "` + versionOutput + `"`

const unexpectedEcho = `#!/usr/bin/env bash
echo "this is not the agent you are looking for"`

func TestAgentScript(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		script := serveScript(t, bashEcho)

		output := testutil.NewWaitBuffer()
		// This is intentionally ran in single quotes to mimic how a customer may
		// embed our script. Our scripts should not include any single quotes.
		//nolint:gosec
		cmd := exec.CommandContext(ctx, "sh", "-c", "sh -c '"+script+"'")
		cmd.Stdout = output
		cmd.Stderr = output
		require.NoError(t, cmd.Start())

		err := cmd.Wait()
		if err != nil {
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				require.Equal(t, 0, exitErr.ExitCode())
			} else {
				t.Fatalf("unexpected err: %s", err)
			}
		}

		t.Log(output.String())
		require.NoError(t, err)
		// Ignore debug output from `set -x`, we're only interested in the last line.
		lines := strings.Split(strings.TrimSpace(output.String()), "\n")
		lastLine := lines[len(lines)-1]
		// When we use the "bashEcho" binary, we should expect the arguments provided
		// as the response to executing our script.
		require.Equal(t, versionOutput, lastLine)
	})

	t.Run("Invalid", func(t *testing.T) {
		t.Parallel()

		script := serveScript(t, unexpectedEcho)
		output := runScriptUntil(t, script, "ERROR: Downloaded agent binary returned unexpected version output")
		output.RequireWaitFor(testutil.Context(t, testutil.WaitShort), t, "See "+troubleshootingURL)
	})

	t.Run("HTTPError", func(t *testing.T) {
		t.Parallel()
		requireCurl(t)

		srv := httptest.NewServer(http.NotFoundHandler())
		t.Cleanup(srv.Close)

		output := runScriptUntil(t, agentScript(t, srv.URL+"/"), "hint: The Coder server returned an HTTP error")
		output.RequireWaitFor(testutil.Context(t, testutil.WaitShort), t, "See "+troubleshootingURL)
	})

	t.Run("ConnectionRefused", func(t *testing.T) {
		t.Parallel()
		requireCurl(t)

		// Reserve a port and close it so that nothing is listening.
		l, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		accessURL := "http://" + l.Addr().String() + "/"
		require.NoError(t, l.Close())

		runScriptUntil(t, agentScript(t, accessURL), "hint: Could not connect to "+accessURL)
	})

	t.Run("ArchitectureMismatch", func(t *testing.T) {
		t.Parallel()
		if runtime.GOOS != "linux" {
			t.Skip("the architecture hint is only printed by the Linux script")
		}
		other := "arm64"
		if runtime.GOARCH == "arm64" {
			other = "amd64"
		}

		srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
			_, _ = rw.Write([]byte(unexpectedEcho))
		}))
		t.Cleanup(srv.Close)

		script := provisionersdk.AgentScriptEnv()["CODER_AGENT_SCRIPT_linux_"+other]
		script = strings.ReplaceAll(script, "${ACCESS_URL}", srv.URL+"/")
		script = strings.ReplaceAll(script, "${AUTH_TYPE}", "token")

		runScriptUntil(t, script, "but the template requested the linux-"+other+" agent")
	})
}

const troubleshootingURL = "https://coder.com/docs/admin/templates/troubleshooting#agent-connection-issues"

// requireCurl skips the test when curl is unavailable, because the download
// hints depend on the exit codes of the download tool that the script picks.
func requireCurl(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("curl"); err != nil {
		t.Skip("curl is required to test download hints")
	}
}

// runScriptUntil runs the bootstrap script until its output contains want, then
// kills it. Failed scripts retry or sleep instead of exiting, so the test
// cannot wait for the script to finish.
func runScriptUntil(t *testing.T, script, want string) *testutil.WaitBuffer {
	t.Helper()

	ctx := testutil.Context(t, testutil.WaitShort)
	output := testutil.NewWaitBuffer()
	// This is intentionally ran in single quotes to mimic how a customer may
	// embed our script. Our scripts should not include any single quotes.
	//nolint:gosec
	cmd := exec.CommandContext(ctx, "sh", "-c", "sh -c '"+script+"'")
	cmd.WaitDelay = time.Second
	cmd.Stdout = output
	cmd.Stderr = output
	require.NoError(t, cmd.Start())

	var wg sync.WaitGroup
	wg.Go(func() {
		_ = cmd.Wait()
	})
	t.Cleanup(func() {
		err := cmd.Cancel()
		if err != nil && !errors.Is(err, os.ErrProcessDone) {
			t.Errorf("unable to cancel the command: %v", err)
		}
		wg.Wait()
		t.Log(output.String())
	})

	output.RequireWaitFor(ctx, t, want)
	return output
}

// serveScript creates a fake HTTP server which serves a requested "agent binary" (which is actually the given input string)
// which will be attempted to run to verify that it is correct.
func serveScript(t *testing.T, in string) string {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(http.StatusOK)
		_, _ = rw.Write([]byte(in))
	}))
	t.Cleanup(srv.Close)
	srvURL, err := url.Parse(srv.URL)
	require.NoError(t, err)

	return agentScript(t, srvURL.String()+"/")
}

// agentScript returns the bootstrap script for the current platform with the
// given access URL.
func agentScript(t *testing.T, accessURL string) string {
	t.Helper()

	script, exists := provisionersdk.AgentScriptEnv()[fmt.Sprintf("CODER_AGENT_SCRIPT_%s_%s", runtime.GOOS, runtime.GOARCH)]
	if !exists {
		t.Skip("Agent not supported...")
		return ""
	}
	script = strings.ReplaceAll(script, "${ACCESS_URL}", accessURL)
	script = strings.ReplaceAll(script, "${AUTH_TYPE}", "token")
	return script
}

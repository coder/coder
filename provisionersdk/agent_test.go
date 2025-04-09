//go:build !windows
// +build !windows

// There isn't a portable Windows binary equivalent to "echo".
// This can be tested, but it's a bit harder.

package provisionersdk_test

import (
	"bytes"
	"errors"
	"fmt"
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

	"github.com/coder/coder/v2/testutil"

	"github.com/coder/coder/v2/provisionersdk"
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

		var output safeBuffer
		// This is intentionally ran in single quotes to mimic how a customer may
		// embed our script. Our scripts should not include any single quotes.
		// nolint:gosec
		cmd := exec.CommandContext(ctx, "sh", "-c", "sh -c '"+script+"'")
		cmd.Stdout = &output
		cmd.Stderr = &output
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

		ctx := testutil.Context(t, testutil.WaitShort)
		script := serveScript(t, unexpectedEcho)

		var output safeBuffer
		// This is intentionally ran in single quotes to mimic how a customer may
		// embed our script. Our scripts should not include any single quotes.
		// nolint:gosec
		cmd := exec.CommandContext(ctx, "sh", "-c", "sh -c '"+script+"'")
		cmd.WaitDelay = time.Second
		cmd.Stdout = &output
		cmd.Stderr = &output
		require.NoError(t, cmd.Start())

		done := make(chan error, 1)
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()

			// The bootstrap scripts trap exit codes to allow operators to view the script logs and debug the process
			// while it is still running. We do not expect Wait() to complete.
			err := cmd.Wait()
			done <- err
		}()

		select {
		case <-ctx.Done():
			// Timeout.
			break
		case err := <-done:
			// If done signals before context times out, script behaved in an unexpected way.
			if err != nil {
				t.Fatalf("unexpected err: %s", err)
			}
		}

		// Kill the command, wait for the command to yield.
		err := cmd.Cancel()
		if errors.Is(err, os.ErrProcessDone) {
			t.Log("script has already finished execution")
		} else if err != nil {
			t.Fatalf("unable to cancel the command: %v, see logs:\n%s", err, output.String())
		}
		wg.Wait()

		t.Log(output.String())

		require.Eventually(t, func() bool {
			return bytes.Contains(output.Bytes(), []byte("ERROR: Downloaded agent binary returned unexpected version output"))
		}, testutil.WaitShort, testutil.IntervalSlow)
	})
}

// serveScript creates a fake HTTP server which serves a requested "agent binary" (which is actually the given input string)
// which will be attempted to run to verify that it is correct.
func serveScript(t *testing.T, in string) string {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(http.StatusOK)
		rw.Write([]byte(in))
	}))
	t.Cleanup(srv.Close)
	srvURL, err := url.Parse(srv.URL)
	require.NoError(t, err)

	script, exists := provisionersdk.AgentScriptEnv()[fmt.Sprintf("CODER_AGENT_SCRIPT_%s_%s", runtime.GOOS, runtime.GOARCH)]
	if !exists {
		t.Skip("Agent not supported...")
		return ""
	}
	script = strings.ReplaceAll(script, "${ACCESS_URL}", srvURL.String()+"/")
	script = strings.ReplaceAll(script, "${AUTH_TYPE}", "token")
	return script
}

// safeBuffer is a concurrency-safe bytes.Buffer
type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (sb *safeBuffer) Write(p []byte) (n int, err error) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.Write(p)
}

func (sb *safeBuffer) Read(p []byte) (n int, err error) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.Read(p)
}

func (sb *safeBuffer) Bytes() []byte {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.Bytes()
}

func (sb *safeBuffer) String() string {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.String()
}

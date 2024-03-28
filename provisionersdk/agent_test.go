//go:build !windows
// +build !windows

// There isn't a portable Windows binary equivalent to "echo".
// This can be tested, but it's a bit harder.

package provisionersdk_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"testing"

	"github.com/go-chi/render"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/provisionersdk"
)

// bashEcho is a script that calls the local `echo` with the arguments.  This is preferable to
// sending the real `echo` binary since macOS 14.4+ immediately sigkills `echo` if it is copied to
// another directory and run locally.
const bashEcho = `#!/usr/bin/env bash
echo $@`

func TestAgentScript(t *testing.T) {
	t.Parallel()
	t.Run("Run", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			render.Status(r, http.StatusOK)
			render.Data(rw, r, []byte(bashEcho))
		}))
		defer srv.Close()
		srvURL, err := url.Parse(srv.URL)
		require.NoError(t, err)

		script, exists := provisionersdk.AgentScriptEnv()[fmt.Sprintf("CODER_AGENT_SCRIPT_%s_%s", runtime.GOOS, runtime.GOARCH)]
		if !exists {
			t.Skip("Agent not supported...")
			return
		}
		script = strings.ReplaceAll(script, "${ACCESS_URL}", srvURL.String()+"/")
		script = strings.ReplaceAll(script, "${AUTH_TYPE}", "token")
		// This is intentionally ran in single quotes to mimic how a customer may
		// embed our script. Our scripts should not include any single quotes.
		// nolint:gosec
		output, err := exec.Command("sh", "-c", "sh -c '"+script+"'").CombinedOutput()
		t.Log(string(output))
		require.NoError(t, err)
		// Ignore debug output from `set -x`, we're only interested in the last line.
		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		lastLine := lines[len(lines)-1]
		// Because we use the "echo" binary, we should expect the arguments provided
		// as the response to executing our script.
		require.Equal(t, "agent", lastLine)
	})
}

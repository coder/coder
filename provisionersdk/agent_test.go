package provisionersdk_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"

	"github.com/coder/coder/provisionersdk"
	"github.com/go-chi/render"
	"github.com/stretchr/testify/require"
)

func TestAgentScript(t *testing.T) {
	t.Parallel()
	t.Run("Run", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			content, err := os.ReadFile("/usr/bin/echo")
			require.NoError(t, err)
			render.Status(r, http.StatusOK)
			render.Data(rw, r, content)
		}))
		t.Cleanup(srv.Close)

		script, err := provisionersdk.AgentScript(runtime.GOOS, runtime.GOARCH, srv.URL)
		require.NoError(t, err)

		output, err := exec.Command("sh", "-c", script).CombinedOutput()
		t.Log(string(output))
		require.NoError(t, err)
		// Because we use the "echo" binary, we should expect the arguments provided
		// as the response to executing our script.
		require.Equal(t, "agent", strings.TrimSpace(string(output)))
	})

	t.Run("UnsupportedOS", func(t *testing.T) {
		t.Parallel()
		_, err := provisionersdk.AgentScript("unsupported", "", nil)
		require.Error(t, err)
	})

	t.Run("UnsupportedArch", func(t *testing.T) {
		t.Parallel()
		_, err := provisionersdk.AgentScript(runtime.GOOS, "unsupported", nil)
		require.Error(t, err)
	})
}

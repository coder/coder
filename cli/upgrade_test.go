package cli_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestUpgrade(t *testing.T) {
	t.Parallel()

	var (
		tmpDir  = t.TempDir()
		binName = "coder"
		binPath = filepath.Join(tmpDir, binName)
	)

	// Build a binary. We have to do this because 'coder upgrade'
	// replaces the running binary with the one pulled from the server.
	buildCoder(t, binPath)

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		var (
			dir       = t.TempDir()
			coderPath = filepath.Join(dir, binName)
			server    = newUpgradeServer(t, binPath)
		)

		copyFile(t, binPath, coderPath)

		stat, err := os.Stat(coderPath)
		require.NoError(t, err)

		cmd := exec.Command(coderPath, "--url", server.URL, "upgrade")
		t.Log("Running command " + strings.Join(cmd.Args, " "))

		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "output: %s", out)

		newStat, err := os.Stat(coderPath)
		require.NoError(t, err)

		require.True(t, newStat.ModTime().After(stat.ModTime()), "mod time should update")

		// Validate that the new binary is still executable.
		out, err = exec.Command(binPath, "version").CombinedOutput()
		require.NoError(t, err, "output: %s", out)
	})
}

// buildCoder build coder/cmd/coder.
func buildCoder(t *testing.T, dest string) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").CombinedOutput()
	require.NoError(t, err)

	t.Logf("top level: %s", out)

	cliDir := filepath.Join(strings.TrimSpace(string(out)), "cmd/coder")

	t.Logf("cliDir: %s", cliDir)

	now := time.Now()
	out, err = exec.Command("go", "build", "-o", dest, cliDir).CombinedOutput()
	require.NoError(t, err, "failed to compile (%s): %v", out, err)
	t.Logf("compilation took %v", time.Since(now))
}

func copyFile(t *testing.T, from, to string) {
	b, err := os.ReadFile(from)
	require.NoError(t, err)

	err = os.WriteFile(to, b, 0755)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = os.Remove(to)
	})
}

// newUpgradeServer starts a server with just the routes necessary
// to complete the 'coder upgrade' path.
func newUpgradeServer(t *testing.T, binPath string) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc(
		fmt.Sprintf("/bin/coder-%s-%s", runtime.GOOS, runtime.GOARCH),
		func(w http.ResponseWriter, r *http.Request) {
			http.ServeFile(w, r, binPath)
		})

	server := httptest.NewServer(mux)

	t.Cleanup(func() {
		server.Close()
	})

	return server
}

package cli_test

import (
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestUpgrade(t *testing.T) {
	t.Parallel()

	var (
		tmpDir  = t.TempDir()
		binPath = filepath.Join(tmpDir, "coder")
	)

	out, err := exec.Command("git", "rev-parse", "--show-toplevel").CombinedOutput()
	require.NoError(t, err)

	cliDir := filepath.Join(string(out), "cmd/coder")

	now := time.Now()
	out, err = exec.Command("go", "build", "-o", binPath, cliDir).CombinedOutput()
	require.NoError(t, err, "failed to compile (%s): %v", out, err)
	t.Logf("compilation took %v", time.Since(now))

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

	})

}

//go:build !windows

package dbtestutil

import (
	"os/exec"
	"path/filepath"
	"time"
)

const timeFormat = "2006-01-02 15:04:05.000"

// cleanerCmd builds the cleaner binary in a temporary directory and returns a command to execute it. We can do this on
// POSIX because it's OK to delete the temporary directory after the test: the binary can still run. This is not
// possible on Windows because cleaning the temporary directory will fail if the binary is still running.
// c.f. cleaner_windows.go.
func cleanerCmd(t TBSubset) *exec.Cmd {
	start := time.Now()
	t.Logf("[%s] starting cleaner binary build", start.Format(timeFormat))
	tempDir := t.TempDir()
	cleanerBinary := filepath.Join(tempDir, "cleaner")

	buildCmd := exec.Command("go", "build", "-o", cleanerBinary, "github.com/coder/coder/v2/coderd/database/dbtestutil/cleanercmd")
	output, err := buildCmd.CombinedOutput()
	if err != nil {
		t.Logf("failed to build cleaner binary: %v", err)
		t.Logf("output: %s", string(output))
		// Fall back to go run if build fails
		return exec.Command("go", "run", "github.com/coder/coder/v2/coderd/database/dbtestutil/cleanercmd")
	}
	t.Logf("[%s] cleaner binary %s built in %s", time.Now().Format(timeFormat), cleanerBinary, time.Since(start))

	return exec.Command(cleanerBinary)
}

//go:build windows

package dbtestutil

import "os/exec"

// cleanerCmd returns a command to execute the cleaner binary. We do this with go run on Windows because we can't
// delete the temporary directory after the test: the binary will still be running. c.f. cleaner_posix.go.
func cleanerCmd(_ TBSubset) *exec.Cmd {
	return exec.Command("go", "run", "github.com/coder/coder/v2/coderd/database/dbtestutil/cleanercmd")
}

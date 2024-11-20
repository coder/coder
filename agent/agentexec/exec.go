package agentexec

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"golang.org/x/xerrors"
)

const (
	// EnvProcPrioMgmt is the environment variable that determines whether
	// we attempt to manage process CPU and OOM Killer priority.
	EnvProcPrioMgmt = "CODER_PROC_PRIO_MGMT"
)

// CommandContext returns an exec.Cmd that calls "coder agent-exec" prior to exec'ing
// the provided command if CODER_PROC_PRIO_MGMT is set, otherwise a normal exec.Cmd
// is returned. All instances of exec.Cmd should flow through this function to ensure
// proper resource constraints are applied to the child process.
func CommandContext(ctx context.Context, cmd string, args ...string) (*exec.Cmd, error) {
	environ := os.Environ()
	_, enabled := envVal(environ, EnvProcPrioMgmt)
	if runtime.GOOS != "linux" || !enabled {
		return exec.CommandContext(ctx, cmd, args...), nil
	}

	executable, err := os.Executable()
	if err != nil {
		return nil, xerrors.Errorf("get executable: %w", err)
	}

	bin, err := filepath.EvalSymlinks(executable)
	if err != nil {
		return nil, xerrors.Errorf("eval symlinks: %w", err)
	}

	args = append([]string{"agent-exec", cmd}, args...)
	return exec.CommandContext(ctx, bin, args...), nil
}

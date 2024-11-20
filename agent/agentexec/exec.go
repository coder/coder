package agentexec

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"golang.org/x/xerrors"
)

const (
	// EnvProcPrioMgmt is the environment variable that determines whether
	// we attempt to manage process CPU and OOM Killer priority.
	EnvProcPrioMgmt  = "CODER_PROC_PRIO_MGMT"
	EnvProcOOMScore  = "CODER_PROC_OOM_SCORE"
	EnvProcNiceScore = "CODER_PROC_NICE_SCORE"
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

// envValInt searches for a key in a list of environment variables and parses it to an int.
// If the key is not found or cannot be parsed, returns 0 and false.
func envValInt(env []string, key string) (int, bool) {
	val, ok := envVal(env, key)
	if !ok {
		return 0, false
	}

	i, err := strconv.Atoi(val)
	if err != nil {
		return 0, false
	}
	return i, true
}

// envVal searches for a key in a list of environment variables and returns its value.
// If the key is not found, returns empty string and false.
func envVal(env []string, key string) (string, bool) {
	prefix := key + "="
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			return strings.TrimPrefix(e, prefix), true
		}
	}
	return "", false
}

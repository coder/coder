package agentexec

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"

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
	_, enabled := os.LookupEnv(EnvProcPrioMgmt)
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

	execArgs := []string{"agent-exec"}
	if score, ok := envValInt(EnvProcOOMScore); ok {
		execArgs = append(execArgs, oomScoreArg(score))
	}

	if score, ok := envValInt(EnvProcNiceScore); ok {
		execArgs = append(execArgs, niceScoreArg(score))
	}
	execArgs = append(execArgs, "--", cmd)
	execArgs = append(execArgs, args...)

	return exec.CommandContext(ctx, bin, execArgs...), nil
}

// envValInt searches for a key in a list of environment variables and parses it to an int.
// If the key is not found or cannot be parsed, returns 0 and false.
func envValInt(key string) (int, bool) {
	val, ok := os.LookupEnv(key)
	if !ok {
		return 0, false
	}

	i, err := strconv.Atoi(val)
	if err != nil {
		return 0, false
	}
	return i, true
}

// The following are flags used by the agent-exec command. We use flags instead of
// environment variables to avoid having to deal with a caller overriding the
// environment variables.
const (
	niceFlag = "coder-nice"
	oomFlag  = "coder-oom"
)

func niceScoreArg(score int) string {
	return fmt.Sprintf("--%s=%d", niceFlag, score)
}

func oomScoreArg(score int) string {
	return fmt.Sprintf("--%s=%d", oomFlag, score)
}

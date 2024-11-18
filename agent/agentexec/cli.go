package agentexec

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/xerrors"
)

const (
	EnvProcOOMScore  = "CODER_PROC_OOM_SCORE"
	EnvProcNiceScore = "CODER_PROC_NICE_SCORE"
)

// CLI runs the agent-exec command. It should only be called by the cli package.
func CLI(args []string, environ []string) error {
	if runtime.GOOS != "linux" {
		return xerrors.Errorf("agent-exec is only supported on Linux")
	}

	if len(args) < 2 {
		return xerrors.Errorf("malformed command %q", args)
	}

	args = args[2:]

	pid := os.Getpid()

	oomScore, ok := envVal(environ, EnvProcOOMScore)
	if !ok {
		return xerrors.Errorf("missing %q", EnvProcOOMScore)
	}

	niceScore, ok := envVal(environ, EnvProcNiceScore)
	if !ok {
		return xerrors.Errorf("missing %q", EnvProcNiceScore)
	}

	score, err := strconv.Atoi(niceScore)
	if err != nil {
		return xerrors.Errorf("invalid nice score: %w", err)
	}

	err = syscall.Setpriority(syscall.PRIO_PROCESS, pid, score)
	if err != nil {
		return xerrors.Errorf("set nice score: %w", err)
	}

	oomPath := fmt.Sprintf("/proc/%d/oom_score_adj", pid)
	err = os.WriteFile(oomPath, []byte(oomScore), 0o600)
	if err != nil {
		return xerrors.Errorf("set oom score: %w", err)
	}

	path, err := exec.LookPath(args[0])
	if err != nil {
		return xerrors.Errorf("look path: %w", err)
	}

	env := slices.DeleteFunc(environ, func(env string) bool {
		return strings.HasPrefix(env, EnvProcOOMScore) || strings.HasPrefix(env, EnvProcNiceScore)
	})

	return syscall.Exec(path, args, env)
}

func envVal(environ []string, key string) (string, bool) {
	for _, env := range environ {
		if strings.HasPrefix(env, key+"=") {
			return strings.TrimPrefix(env, key+"="), true
		}
	}
	return "", false
}

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

	"github.com/coder/coder/v2/pty"
)

const (
	// EnvProcPrioMgmt is the environment variable that determines whether
	// we attempt to manage process CPU and OOM Killer priority.
	EnvProcPrioMgmt  = "CODER_PROC_PRIO_MGMT"
	EnvProcOOMScore  = "CODER_PROC_OOM_SCORE"
	EnvProcNiceScore = "CODER_PROC_NICE_SCORE"

	// unset is set to an invalid value for nice and oom scores.
	unset = -2000
)

var DefaultExecer Execer = execer{}

// Execer defines an abstraction for creating exec.Cmd variants. It's unfortunately
// necessary because we need to be able to wrap child processes with "coder agent-exec"
// for templates that expect the agent to manage process priority.
type Execer interface {
	// CommandContext returns an exec.Cmd that calls "coder agent-exec" prior to exec'ing
	// the provided command if CODER_PROC_PRIO_MGMT is set, otherwise a normal exec.Cmd
	// is returned. All instances of exec.Cmd should flow through this function to ensure
	// proper resource constraints are applied to the child process.
	CommandContext(ctx context.Context, cmd string, args ...string) *exec.Cmd
	// PTYCommandContext returns an pty.Cmd that calls "coder agent-exec" prior to exec'ing
	// the provided command if CODER_PROC_PRIO_MGMT is set, otherwise a normal pty.Cmd
	// is returned. All instances of pty.Cmd should flow through this function to ensure
	// proper resource constraints are applied to the child process.
	PTYCommandContext(ctx context.Context, cmd string, args ...string) *pty.Cmd
}

func NewExecer() (Execer, error) {
	_, enabled := os.LookupEnv(EnvProcPrioMgmt)
	if runtime.GOOS != "linux" || !enabled {
		return DefaultExecer, nil
	}

	executable, err := os.Executable()
	if err != nil {
		return nil, xerrors.Errorf("get executable: %w", err)
	}

	bin, err := filepath.EvalSymlinks(executable)
	if err != nil {
		return nil, xerrors.Errorf("eval symlinks: %w", err)
	}

	oomScore, ok := envValInt(EnvProcOOMScore)
	if !ok {
		oomScore = unset
	}

	niceScore, ok := envValInt(EnvProcNiceScore)
	if !ok {
		niceScore = unset
	}

	return priorityExecer{
		binPath:   bin,
		oomScore:  oomScore,
		niceScore: niceScore,
	}, nil
}

type execer struct{}

func (execer) CommandContext(ctx context.Context, cmd string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, cmd, args...)
}

func (execer) PTYCommandContext(ctx context.Context, cmd string, args ...string) *pty.Cmd {
	return pty.CommandContext(ctx, cmd, args...)
}

type priorityExecer struct {
	binPath   string
	oomScore  int
	niceScore int
}

func (e priorityExecer) CommandContext(ctx context.Context, cmd string, args ...string) *exec.Cmd {
	cmd, args = e.agentExecCmd(cmd, args...)
	return exec.CommandContext(ctx, cmd, args...)
}

func (e priorityExecer) PTYCommandContext(ctx context.Context, cmd string, args ...string) *pty.Cmd {
	cmd, args = e.agentExecCmd(cmd, args...)
	return pty.CommandContext(ctx, cmd, args...)
}

func (e priorityExecer) agentExecCmd(cmd string, args ...string) (string, []string) {
	execArgs := []string{"agent-exec"}
	if e.oomScore != unset {
		execArgs = append(execArgs, oomScoreArg(e.oomScore))
	}

	if e.niceScore != unset {
		execArgs = append(execArgs, niceScoreArg(e.niceScore))
	}
	execArgs = append(execArgs, "--", cmd)
	execArgs = append(execArgs, args...)

	return e.binPath, execArgs
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

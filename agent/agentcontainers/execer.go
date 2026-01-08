package agentcontainers

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/agent/usershell"
	"github.com/coder/coder/v2/pty"
)

// CommandEnv is a function that returns the shell, working directory,
// and environment variables to use when executing a command. It takes
// an EnvInfoer and a pre-existing environment slice as arguments.
// This signature matches agentssh.Server.CommandEnv.
type CommandEnv func(ei usershell.EnvInfoer, addEnv []string) (shell, dir string, env []string, err error)

// commandEnvExecer is an agentexec.Execer that uses a CommandEnv to
// determine the shell, working directory, and environment variables
// for commands. It wraps another agentexec.Execer to provide the
// necessary context.
type commandEnvExecer struct {
	logger     slog.Logger
	commandEnv CommandEnv
	execer     agentexec.Execer
}

func newCommandEnvExecer(
	logger slog.Logger,
	commandEnv CommandEnv,
	execer agentexec.Execer,
) *commandEnvExecer {
	return &commandEnvExecer{
		logger:     logger,
		commandEnv: commandEnv,
		execer:     execer,
	}
}

// Ensure commandEnvExecer implements agentexec.Execer.
var _ agentexec.Execer = (*commandEnvExecer)(nil)

func (e *commandEnvExecer) prepare(ctx context.Context, inName string, inArgs ...string) (name string, args []string, dir string, env []string) {
	shell, dir, env, err := e.commandEnv(nil, nil)
	if err != nil {
		e.logger.Error(ctx, "get command environment failed", slog.Error(err))
		return inName, inArgs, "", nil
	}

	caller := "-c"
	if runtime.GOOS == "windows" {
		caller = "/c"
	}
	name = shell
	for _, arg := range append([]string{inName}, inArgs...) {
		args = append(args, fmt.Sprintf("%q", arg))
	}
	args = []string{caller, strings.Join(args, " ")}
	return name, args, dir, env
}

func (e *commandEnvExecer) CommandContext(ctx context.Context, cmd string, args ...string) *exec.Cmd {
	name, args, dir, env := e.prepare(ctx, cmd, args...)
	c := e.execer.CommandContext(ctx, name, args...)
	c.Dir = dir
	c.Env = env
	return c
}

func (e *commandEnvExecer) PTYCommandContext(ctx context.Context, cmd string, args ...string) *pty.Cmd {
	name, args, dir, env := e.prepare(ctx, cmd, args...)
	c := e.execer.PTYCommandContext(ctx, name, args...)
	c.Dir = dir
	c.Env = env
	return c
}

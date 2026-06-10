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

	name = shell
	cmdArgs := append([]string{inName}, inArgs...)
	if runtime.GOOS == "windows" {
		// Preserve the prior quoting behavior on Windows. This path was
		// already non-functional: the selected shell is usually PowerShell
		// (see usershell.Get), which rejects the hardcoded cmd.exe "/c"
		// switch. A correct Windows implementation is deferred.
		var quoted []string
		for _, arg := range cmdArgs {
			quoted = append(quoted, fmt.Sprintf("%q", arg))
		}
		args = []string{"/c", strings.Join(quoted, " ")}
		return name, args, dir, env
	}
	// Pass the command through the shell as positional parameters and run
	// "$@" so the shell re-emits argv verbatim without re-parsing it. This
	// prevents arguments containing shell metacharacters such as $, `, and
	// quotes from being interpreted (e.g. command substitution). The first
	// argument after the script ($0) is a placeholder for the shell name.
	args = append([]string{"-c", `"$@"`, shell}, cmdArgs...)
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

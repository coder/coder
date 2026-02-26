package chattool

import (
	"context"
	"time"

	"charm.land/fantasy"
	"golang.org/x/crypto/ssh"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

const (
	defaultExecuteTimeout    = 60 * time.Second
	chatAgentEnvVar          = "CODER_CHAT_AGENT"
	gitAuthRequiredPrefix    = "CODER_GITAUTH_REQUIRED:"
	authRequiredResultReason = "authentication_required"
)

type ExecuteOptions struct {
	GetWorkspaceConn func(context.Context) (workspacesdk.AgentConn, error)
	DefaultTimeout   time.Duration
}

type ExecuteArgs struct {
	Command        string `json:"command"`
	TimeoutSeconds *int   `json:"timeout_seconds,omitempty"`
}

func Execute(options ExecuteOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"execute",
		"Execute a shell command in the workspace.",
		func(ctx context.Context, args ExecuteArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if options.GetWorkspaceConn == nil {
				return fantasy.NewTextErrorResponse("workspace connection resolver is not configured"), nil
			}
			conn, err := options.GetWorkspaceConn(ctx)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			return executeTool(ctx, conn, args, options.DefaultTimeout), nil
		},
	)
}

func executeTool(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	args ExecuteArgs,
	defaultTimeout time.Duration,
) fantasy.ToolResponse {
	if args.Command == "" {
		return fantasy.NewTextErrorResponse("command is required")
	}

	timeout := defaultTimeout
	if timeout <= 0 {
		timeout = defaultExecuteTimeout
	}
	if args.TimeoutSeconds != nil {
		timeout = time.Duration(*args.TimeoutSeconds) * time.Second
	}
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	output, exitCode, err := runCommand(cmdCtx, conn, args.Command)
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error())
	}
	return toolResponse(map[string]any{
		"output":    output,
		"exit_code": exitCode,
	})
}

func runCommand(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	command string,
) (string, int, error) {
	sshClient, err := conn.SSHClient(ctx)
	if err != nil {
		return "", 0, err
	}
	defer sshClient.Close()

	session, err := sshClient.NewSession()
	if err != nil {
		return "", 0, err
	}
	defer session.Close()
	if err := session.Setenv(chatAgentEnvVar, "true"); err != nil {
		return "", 0, xerrors.Errorf("set %s: %w", chatAgentEnvVar, err)
	}

	resultCh := make(chan struct {
		output   string
		exitCode int
		err      error
	}, 1)

	go func() {
		output, err := session.CombinedOutput(command)
		exitCode := 0
		if err != nil {
			var exitErr *ssh.ExitError
			if xerrors.As(err, &exitErr) {
				exitCode = exitErr.ExitStatus()
			} else {
				exitCode = 1
			}
		}
		resultCh <- struct {
			output   string
			exitCode int
			err      error
		}{
			output:   string(output),
			exitCode: exitCode,
			err:      err,
		}
	}()

	select {
	case <-ctx.Done():
		_ = session.Close()
		return "", 0, ctx.Err()
	case result := <-resultCh:
		return result.output, result.exitCode, result.err
	}
}

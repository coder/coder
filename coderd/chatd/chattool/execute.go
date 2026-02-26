package chattool

import (
	"context"
	"encoding/json"
	"strings"
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
	authRequired, cleanedOutput := extractGitAuthRequiredMarker(output)
	resultPayload := map[string]any{
		"output":    cleanedOutput,
		"exit_code": exitCode,
	}
	if authRequired != nil {
		resultPayload["auth_required"] = true
		resultPayload["authenticate_url"] = authRequired.AuthenticateURL
		resultPayload["reason"] = authRequiredResultReason
		if strings.TrimSpace(authRequired.ProviderID) != "" {
			resultPayload["provider_id"] = authRequired.ProviderID
		}
		if strings.TrimSpace(authRequired.ProviderType) != "" {
			resultPayload["provider_type"] = authRequired.ProviderType
		}
		if strings.TrimSpace(authRequired.ProviderDisplayName) != "" {
			resultPayload["provider_display_name"] = authRequired.ProviderDisplayName
		}
		if err != nil {
			resultPayload["error"] = err.Error()
		}
		return toolResponse(resultPayload)
	}
	if err != nil {
		resultPayload["error"] = err.Error()
		data, _ := json.Marshal(resultPayload)
		return fantasy.ToolResponse{
			Type:    "text",
			Content: string(data),
			IsError: true,
		}
	}
	return toolResponse(resultPayload)
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

type gitAuthRequiredMarker struct {
	ProviderID          string `json:"provider_id"`
	ProviderType        string `json:"provider_type,omitempty"`
	ProviderDisplayName string `json:"provider_display_name,omitempty"`
	AuthenticateURL     string `json:"authenticate_url"`
}

func extractGitAuthRequiredMarker(output string) (*gitAuthRequiredMarker, string) {
	if output == "" {
		return nil, output
	}

	var marker *gitAuthRequiredMarker
	lines := strings.Split(output, "\n")
	filteredLines := make([]string, 0, len(lines))
	for _, line := range lines {
		idx := strings.Index(line, gitAuthRequiredPrefix)
		if idx == -1 {
			filteredLines = append(filteredLines, line)
			continue
		}

		rawPayload := strings.TrimSpace(line[idx+len(gitAuthRequiredPrefix):])
		candidate := gitAuthRequiredMarker{}
		if rawPayload == "" || json.Unmarshal([]byte(rawPayload), &candidate) != nil || strings.TrimSpace(candidate.AuthenticateURL) == "" {
			filteredLines = append(filteredLines, line)
			continue
		}
		if marker == nil {
			marker = &candidate
		}

		prefix := strings.TrimSpace(line[:idx])
		if prefix != "" {
			filteredLines = append(filteredLines, prefix)
		}
	}
	return marker, strings.Join(filteredLines, "\n")
}

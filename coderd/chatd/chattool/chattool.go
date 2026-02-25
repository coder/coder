package chattool

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"time"

	"charm.land/fantasy"
	"github.com/coder/coder/v2/coderd/chatd/chatprompt"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"golang.org/x/crypto/ssh"
	"golang.org/x/xerrors"
)

const (
	defaultExecuteTimeout    = 60 * time.Second
	chatAgentEnvVar          = "CODER_CHAT_AGENT"
	gitAuthRequiredPrefix    = "CODER_GITAUTH_REQUIRED:"
	authRequiredResultReason = "authentication_required"
)

type ReadFileOptions struct {
	GetWorkspaceConn func(context.Context) (workspacesdk.AgentConn, error)
}

type ReadFileArgs struct {
	Path   string `json:"path"`
	Offset *int64 `json:"offset,omitempty"`
	Limit  *int64 `json:"limit,omitempty"`
}

func ReadFile(options ReadFileOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"read_file",
		"Read a file from the workspace.",
		func(ctx context.Context, args ReadFileArgs, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			result := chatprompt.ToolResultBlock{
				ToolCallID: call.ID,
				ToolName:   call.Name,
			}
			if options.GetWorkspaceConn == nil {
				return toolResultBlockToAgentResponse(
					toolError(result, xerrors.New("workspace connection resolver is not configured")),
				), nil
			}
			conn, err := options.GetWorkspaceConn(ctx)
			if err != nil {
				return toolResultBlockToAgentResponse(toolError(result, err)), nil
			}
			return toolResultBlockToAgentResponse(executeReadFileTool(ctx, conn, result, args)), nil
		},
	)
}

type WriteFileOptions struct {
	GetWorkspaceConn func(context.Context) (workspacesdk.AgentConn, error)
}

type WriteFileArgs struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func WriteFile(options WriteFileOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"write_file",
		"Write a file to the workspace.",
		func(ctx context.Context, args WriteFileArgs, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			result := chatprompt.ToolResultBlock{
				ToolCallID: call.ID,
				ToolName:   call.Name,
			}
			if options.GetWorkspaceConn == nil {
				return toolResultBlockToAgentResponse(
					toolError(result, xerrors.New("workspace connection resolver is not configured")),
				), nil
			}
			conn, err := options.GetWorkspaceConn(ctx)
			if err != nil {
				return toolResultBlockToAgentResponse(toolError(result, err)), nil
			}
			return toolResultBlockToAgentResponse(executeWriteFileTool(ctx, conn, result, args)), nil
		},
	)
}

type EditFilesOptions struct {
	GetWorkspaceConn func(context.Context) (workspacesdk.AgentConn, error)
}

type EditFilesArgs struct {
	Files []workspacesdk.FileEdits `json:"files"`
}

func EditFiles(options EditFilesOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"edit_files",
		"Perform search-and-replace edits on one or more files in the workspace."+
			" Each file can have multiple edits applied atomically.",
		func(ctx context.Context, args EditFilesArgs, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			result := chatprompt.ToolResultBlock{
				ToolCallID: call.ID,
				ToolName:   call.Name,
			}
			if options.GetWorkspaceConn == nil {
				return toolResultBlockToAgentResponse(
					toolError(result, xerrors.New("workspace connection resolver is not configured")),
				), nil
			}
			conn, err := options.GetWorkspaceConn(ctx)
			if err != nil {
				return toolResultBlockToAgentResponse(toolError(result, err)), nil
			}
			return toolResultBlockToAgentResponse(executeEditFilesTool(ctx, conn, result, args)), nil
		},
	)
}

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
		func(ctx context.Context, args ExecuteArgs, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			result := chatprompt.ToolResultBlock{
				ToolCallID: call.ID,
				ToolName:   call.Name,
			}
			if options.GetWorkspaceConn == nil {
				return toolResultBlockToAgentResponse(
					toolError(result, xerrors.New("workspace connection resolver is not configured")),
				), nil
			}
			conn, err := options.GetWorkspaceConn(ctx)
			if err != nil {
				return toolResultBlockToAgentResponse(toolError(result, err)), nil
			}
			return toolResultBlockToAgentResponse(executeTool(
				ctx,
				conn,
				result,
				args,
				options.DefaultTimeout,
			)), nil
		},
	)
}

func executeReadFileTool(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	result chatprompt.ToolResultBlock,
	args ReadFileArgs,
) chatprompt.ToolResultBlock {
	if args.Path == "" {
		return toolError(result, xerrors.New("path is required"))
	}

	offset := int64(0)
	limit := int64(0)
	if args.Offset != nil {
		offset = *args.Offset
	}
	if args.Limit != nil {
		limit = *args.Limit
	}

	reader, mimeType, err := conn.ReadFile(ctx, args.Path, offset, limit)
	if err != nil {
		return toolError(result, err)
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return toolError(result, err)
	}

	result.Result = map[string]any{
		"content":   string(data),
		"mime_type": mimeType,
	}
	return result
}

func executeWriteFileTool(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	result chatprompt.ToolResultBlock,
	args WriteFileArgs,
) chatprompt.ToolResultBlock {
	if args.Path == "" {
		return toolError(result, xerrors.New("path is required"))
	}

	if err := conn.WriteFile(ctx, args.Path, strings.NewReader(args.Content)); err != nil {
		return toolError(result, err)
	}
	result.Result = map[string]any{"ok": true}
	return result
}

func executeEditFilesTool(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	result chatprompt.ToolResultBlock,
	args EditFilesArgs,
) chatprompt.ToolResultBlock {
	if len(args.Files) == 0 {
		return toolError(result, xerrors.New("files is required"))
	}

	if err := conn.EditFiles(ctx, workspacesdk.FileEditRequest{Files: args.Files}); err != nil {
		return toolError(result, err)
	}
	result.Result = map[string]any{"ok": true}
	return result
}

func executeTool(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	result chatprompt.ToolResultBlock,
	args ExecuteArgs,
	defaultTimeout time.Duration,
) chatprompt.ToolResultBlock {
	if args.Command == "" {
		return toolError(result, xerrors.New("command is required"))
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
		result.Result = resultPayload
		return result
	}
	if err != nil {
		resultPayload["error"] = err.Error()
		result.IsError = true
	}
	result.Result = resultPayload
	return result
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

func toolError(result chatprompt.ToolResultBlock, err error) chatprompt.ToolResultBlock {
	result.IsError = true
	result.Result = map[string]any{"error": err.Error()}
	return result
}

func toolResultBlockToAgentResponse(result chatprompt.ToolResultBlock) fantasy.ToolResponse {
	content := ""
	if result.IsError {
		if fields, ok := result.Result.(map[string]any); ok {
			if extracted, ok := fields["error"].(string); ok && strings.TrimSpace(extracted) != "" {
				content = extracted
			}
		}
		if content == "" {
			if raw, err := json.Marshal(result.Result); err == nil {
				content = strings.TrimSpace(string(raw))
			}
		}
	} else if payload, err := json.Marshal(result.Result); err == nil {
		content = string(payload)
	}

	metadata := ""
	if raw, err := json.Marshal(result); err == nil {
		metadata = string(raw)
	}

	return fantasy.ToolResponse{
		Content:  content,
		IsError:  result.IsError,
		Metadata: metadata,
	}
}

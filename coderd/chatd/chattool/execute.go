package chattool

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"charm.land/fantasy"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

const (
	// defaultTimeout is the default timeout for command
	// execution.
	defaultTimeout = 10 * time.Second

	// maxOutputToModel is the maximum output sent to the LLM.
	maxOutputToModel = 32 << 10 // 32KB

	// pollInterval is how often we check for process completion
	// in foreground mode.
	pollInterval = 200 * time.Millisecond
)

// nonInteractiveEnvVars are set on every process to prevent
// interactive prompts that would hang a headless execution.
var nonInteractiveEnvVars = map[string]string{
	"GIT_EDITOR":          "true",
	"GIT_SEQUENCE_EDITOR": "true",
	"EDITOR":              "true",
	"VISUAL":              "true",
	"GIT_TERMINAL_PROMPT": "0",
	"NO_COLOR":            "1",
	"TERM":                "dumb",
	"PAGER":               "cat",
	"GIT_PAGER":           "cat",
}

// fileDumpPatterns detects commands that dump entire files.
// When matched, a note is added suggesting read_file instead.
var fileDumpPatterns = []*regexp.Regexp{
	regexp.MustCompile(`^cat\s+`),
	regexp.MustCompile(`^(rg|grep)\s+.*--include-all`),
	regexp.MustCompile(`^(rg|grep)\s+-l\s+`),
}

// ExecuteResult is the structured response from the execute
// tool.
type ExecuteResult struct {
	Success             bool                            `json:"success"`
	Output              string                          `json:"output,omitempty"`
	ExitCode            int                             `json:"exit_code"`
	WallDurationMs      int64                           `json:"wall_duration_ms"`
	Error               string                          `json:"error,omitempty"`
	Truncated           *workspacesdk.ProcessTruncation `json:"truncated,omitempty"`
	Note                string                          `json:"note,omitempty"`
	BackgroundProcessID string                          `json:"background_process_id,omitempty"`
}

// ExecuteOptions configures the execute tool.
type ExecuteOptions struct {
	GetWorkspaceConn func(context.Context) (workspacesdk.AgentConn, error)
	DefaultTimeout   time.Duration
}

// ProcessToolOptions configures a process management tool
// (process_output, process_list, or process_signal). Each of
// these tools only needs a workspace connection resolver.
type ProcessToolOptions struct {
	GetWorkspaceConn func(context.Context) (workspacesdk.AgentConn, error)
}

// ExecuteArgs are the parameters accepted by the execute tool.
type ExecuteArgs struct {
	Command         string  `json:"command"`
	Timeout         *string `json:"timeout,omitempty"`
	WorkDir         *string `json:"workdir,omitempty"`
	RunInBackground *bool   `json:"run_in_background,omitempty"`
}

// Execute returns an AgentTool that runs a shell command in the
// workspace via the agent HTTP API.
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
	optTimeout time.Duration,
) fantasy.ToolResponse {
	if args.Command == "" {
		return fantasy.NewTextErrorResponse("command is required")
	}

	// Build the environment map for the process request.
	env := make(map[string]string, len(nonInteractiveEnvVars)+1)
	env["CODER_CHAT_AGENT"] = "true"
	for k, v := range nonInteractiveEnvVars {
		env[k] = v
	}

	background := args.RunInBackground != nil && *args.RunInBackground

	var workDir string
	if args.WorkDir != nil {
		workDir = *args.WorkDir
	}

	if background {
		return executeBackground(ctx, conn, args.Command, workDir, env)
	}
	return executeForeground(ctx, conn, args, optTimeout, workDir, env)
}

// executeBackground starts a process in the background and
// returns immediately with the process ID.
func executeBackground(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	command string,
	workDir string,
	env map[string]string,
) fantasy.ToolResponse {
	resp, err := conn.StartProcess(ctx, workspacesdk.StartProcessRequest{
		Command:    command,
		WorkDir:    workDir,
		Env:        env,
		Background: true,
	})
	if err != nil {
		return errorResult(fmt.Sprintf("start background process: %v", err))
	}

	result := ExecuteResult{
		Success:             true,
		BackgroundProcessID: resp.ID,
	}
	data, err := json.Marshal(result)
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error())
	}
	return fantasy.NewTextResponse(string(data))
}

// executeForeground starts a process and polls for its
// completion, enforcing the configured timeout.
func executeForeground(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	args ExecuteArgs,
	optTimeout time.Duration,
	workDir string,
	env map[string]string,
) fantasy.ToolResponse {
	timeout := optTimeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	if args.Timeout != nil {
		parsed, err := time.ParseDuration(*args.Timeout)
		if err != nil {
			return fantasy.NewTextErrorResponse(
				fmt.Sprintf("invalid timeout %q: %v", *args.Timeout, err),
			)
		}
		timeout = parsed
	}

	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()

	resp, err := conn.StartProcess(cmdCtx, workspacesdk.StartProcessRequest{
		Command:    args.Command,
		WorkDir:    workDir,
		Env:        env,
		Background: false,
	})
	if err != nil {
		return errorResult(fmt.Sprintf("start process: %v", err))
	}

	result := pollProcess(cmdCtx, conn, resp.ID, timeout)
	result.WallDurationMs = time.Since(start).Milliseconds()

	// Add an advisory note for file-dump commands.
	if note := detectFileDump(args.Command); note != "" {
		result.Note = note
	}

	data, err := json.Marshal(result)
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error())
	}
	return fantasy.NewTextResponse(string(data))
}

// truncateOutput safely truncates output to maxOutputToModel,
// ensuring the result is valid UTF-8 even if the cut falls in
// the middle of a multi-byte character.
func truncateOutput(output string) string {
	if len(output) > maxOutputToModel {
		output = strings.ToValidUTF8(output[:maxOutputToModel], "")
	}
	return output
}

// pollProcess polls for process output until the process exits
// or the context times out.
func pollProcess(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	processID string,
	timeout time.Duration,
) ExecuteResult {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Timeout — get whatever output we have. Use a
			// fresh context since cmdCtx is already canceled.
			bgCtx, bgCancel := context.WithTimeout(
				context.Background(),
				5*time.Second,
			)
			outputResp, _ := conn.ProcessOutput(bgCtx, processID)
			bgCancel()
			output := truncateOutput(outputResp.Output)
			return ExecuteResult{
				Success:   false,
				Output:    output,
				ExitCode:  -1,
				Error:     fmt.Sprintf("command timed out after %s", timeout),
				Truncated: outputResp.Truncated,
			}
		case <-ticker.C:
			outputResp, err := conn.ProcessOutput(ctx, processID)
			if err != nil {
				return ExecuteResult{
					Success: false,
					Error:   fmt.Sprintf("get process output: %v", err),
				}
			}
			if !outputResp.Running {
				exitCode := 0
				if outputResp.ExitCode != nil {
					exitCode = *outputResp.ExitCode
				}
				output := truncateOutput(outputResp.Output)
				return ExecuteResult{
					Success:   exitCode == 0,
					Output:    output,
					ExitCode:  exitCode,
					Truncated: outputResp.Truncated,
				}
			}
		}
	}
}

// errorResult builds a ToolResponse from an ExecuteResult with
// an error message.
func errorResult(msg string) fantasy.ToolResponse {
	data, err := json.Marshal(ExecuteResult{
		Success: false,
		Error:   msg,
	})
	if err != nil {
		return fantasy.NewTextErrorResponse(msg)
	}
	return fantasy.NewTextResponse(string(data))
}

// detectFileDump checks whether the command matches a file-dump
// pattern and returns an advisory note, or empty string if no
// match.
func detectFileDump(command string) string {
	for _, pat := range fileDumpPatterns {
		if pat.MatchString(command) {
			return "Consider using read_file instead of " +
				"dumping file contents with shell commands."
		}
	}
	return ""
}

// ProcessOutputArgs are the parameters accepted by the
// process_output tool.
type ProcessOutputArgs struct {
	ProcessID string `json:"process_id"`
}

// ProcessOutput returns an AgentTool that retrieves the output
// of a background process by its ID.
func ProcessOutput(options ProcessToolOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"process_output",
		"Retrieve output from a background process. "+
			"Use the process_id returned by execute with "+
			"run_in_background=true. Returns the current output, "+
			"whether the process is still running, and the exit "+
			"code if it has finished.",
		func(ctx context.Context, args ProcessOutputArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if options.GetWorkspaceConn == nil {
				return fantasy.NewTextErrorResponse("workspace connection resolver is not configured"), nil
			}
			if args.ProcessID == "" {
				return fantasy.NewTextErrorResponse("process_id is required"), nil
			}
			conn, err := options.GetWorkspaceConn(ctx)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			resp, err := conn.ProcessOutput(ctx, args.ProcessID)
			if err != nil {
				return errorResult(fmt.Sprintf("get process output: %v", err)), nil
			}
			output := truncateOutput(resp.Output)
			exitCode := 0
			if resp.ExitCode != nil {
				exitCode = *resp.ExitCode
			}
			result := ExecuteResult{
				Success:   !resp.Running && exitCode == 0,
				Output:    output,
				ExitCode:  exitCode,
				Truncated: resp.Truncated,
			}
			if resp.Running {
				// Process is still running — success is not
				// yet determined.
				result.Success = true
				result.Note = "process is still running"
			}
			data, err := json.Marshal(result)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			return fantasy.NewTextResponse(string(data)), nil
		},
	)
}

// ProcessList returns an AgentTool that lists all tracked
// processes on the workspace agent.
func ProcessList(options ProcessToolOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"process_list",
		"List all tracked processes in the workspace. "+
			"Returns process IDs, commands, status (running or "+
			"exited), and exit codes. Use this to discover "+
			"background processes or check which processes are "+
			"still running.",
		func(ctx context.Context, _ struct{}, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if options.GetWorkspaceConn == nil {
				return fantasy.NewTextErrorResponse("workspace connection resolver is not configured"), nil
			}
			conn, err := options.GetWorkspaceConn(ctx)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			resp, err := conn.ListProcesses(ctx)
			if err != nil {
				return errorResult(fmt.Sprintf("list processes: %v", err)), nil
			}
			data, err := json.Marshal(resp)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			return fantasy.NewTextResponse(string(data)), nil
		},
	)
}

// ProcessSignalArgs are the parameters accepted by the
// process_signal tool.
type ProcessSignalArgs struct {
	ProcessID string `json:"process_id"`
	Signal    string `json:"signal"`
}

// ProcessSignal returns an AgentTool that sends a signal to a
// tracked process on the workspace agent.
func ProcessSignal(options ProcessToolOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"process_signal",
		"Send a signal to a background process. "+
			"Use \"terminate\" (SIGTERM) for graceful shutdown "+
			"or \"kill\" (SIGKILL) to force stop. Use the "+
			"process_id returned by execute with "+
			"run_in_background=true or from process_list.",
		func(ctx context.Context, args ProcessSignalArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if options.GetWorkspaceConn == nil {
				return fantasy.NewTextErrorResponse("workspace connection resolver is not configured"), nil
			}
			if args.ProcessID == "" {
				return fantasy.NewTextErrorResponse("process_id is required"), nil
			}
			if args.Signal != "terminate" && args.Signal != "kill" {
				return fantasy.NewTextErrorResponse(
					"signal must be \"terminate\" or \"kill\"",
				), nil
			}
			conn, err := options.GetWorkspaceConn(ctx)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			if err := conn.SignalProcess(ctx, args.ProcessID, args.Signal); err != nil {
				return errorResult(fmt.Sprintf("signal process: %v", err)), nil
			}
			data, err := json.Marshal(map[string]any{
				"success": true,
				"message": fmt.Sprintf(
					"signal %q sent to process %s",
					args.Signal, args.ProcessID,
				),
			})
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			return fantasy.NewTextResponse(string(data)), nil
		},
	)
}

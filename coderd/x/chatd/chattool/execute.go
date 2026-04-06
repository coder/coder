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

	// snapshotTimeout is how long a non-blocking fallback
	// request is allowed to take when retrieving a process
	// output snapshot after a blocking wait times out.
	snapshotTimeout = 30 * time.Second
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
	Command         string  `json:"command" description:"The shell command to execute."`
	Timeout         *string `json:"timeout,omitempty" description:"How long to wait for completion (e.g. '30s', '5m'). Default is 10s. The process keeps running if this expires and you get a background_process_id to re-attach. Only applies to foreground commands."`
	WorkDir         *string `json:"workdir,omitempty" description:"Working directory for the command."`
	RunInBackground *bool   `json:"run_in_background,omitempty" description:"Run without blocking. Use for persistent processes (dev servers, file watchers) or when you want to continue working while a command runs and check the result later with process_output. For commands whose result you need before continuing, prefer foreground with a longer timeout. Do NOT use shell & to background processes — it will not work correctly. Always use this parameter instead."`
}

// Execute returns an AgentTool that runs a shell command in the
// workspace via the agent HTTP API.
func Execute(options ExecuteOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"execute",
		"Execute a shell command in the workspace. Runs the command and waits for completion up to the timeout (default 10s, override with the timeout parameter e.g. '30s', '5m'). If the command exceeds the timeout, the response includes a background_process_id; use process_output with that ID to re-attach and wait for the result. Use run_in_background=true for persistent processes (dev servers, file watchers) or when you want to continue other work while the command runs. Never use shell '&' for backgrounding.",
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

	// Detect shell-style backgrounding (trailing &) and promote to
	// background mode. Models sometimes use "cmd &" instead of the
	// run_in_background parameter, which causes the shell to fork
	// and exit immediately, leaving an untracked orphan process.
	trimmed := strings.TrimSpace(args.Command)
	if !background && strings.HasSuffix(trimmed, "&") && !strings.HasSuffix(trimmed, "&&") && !strings.HasSuffix(trimmed, "|&") {
		background = true
		args.Command = strings.TrimSpace(strings.TrimSuffix(trimmed, "&"))
	}

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

// executeForeground starts a process and waits for its
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

	result := waitForProcess(cmdCtx, ctx, conn, resp.ID, timeout)
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

// waitForProcess waits for process completion using the
// blocking process output API instead of polling.
// waitForProcess blocks until the process exits or the context
// expires. On any error (timeout or transport), it tries a
// non-blocking snapshot to recover. Total wall time may exceed
// timeout by up to snapshotTimeout if recovery is needed.
func waitForProcess(
	ctx context.Context,
	parentCtx context.Context,
	conn workspacesdk.AgentConn,
	processID string,
	timeout time.Duration,
) ExecuteResult {
	// Block until the process exits or the context is
	// canceled.
	resp, err := conn.ProcessOutput(ctx, processID, &workspacesdk.ProcessOutputOptions{
		Wait: true,
	})
	if err != nil {
		origErr := err
		timedOut := ctx.Err() != nil

		// Fetch a snapshot with a fresh context. The blocking
		// request may have failed due to a context timeout or
		// a transport error (e.g. the server's WriteTimeout
		// killed the connection). Either way, the process may
		// still have output available.
		bgCtx, bgCancel := context.WithTimeout(
			parentCtx,
			snapshotTimeout,
		)
		defer bgCancel()
		resp, err = conn.ProcessOutput(bgCtx, processID, nil)
		if err != nil {
			errMsg := fmt.Sprintf("get process output: %v; use process_output with ID %s to retry", origErr, processID)
			if timedOut {
				errMsg = fmt.Sprintf("command timed out after %s; failed to get output: %v", timeout, err)
			}
			return ExecuteResult{
				Success:             false,
				ExitCode:            -1,
				Error:               errMsg,
				BackgroundProcessID: processID,
			}
		}

		// Snapshot succeeded. If the process finished, return
		// its real result (transparent recovery).
		if !resp.Running {
			exitCode := 0
			if resp.ExitCode != nil {
				exitCode = *resp.ExitCode
			}
			output := truncateOutput(resp.Output)
			return ExecuteResult{
				Success:   exitCode == 0,
				Output:    output,
				ExitCode:  exitCode,
				Truncated: resp.Truncated,
			}
		}

		// Process still running, return partial output.
		output := truncateOutput(resp.Output)
		errMsg := fmt.Sprintf("command timed out after %s", timeout)
		if !timedOut {
			errMsg = fmt.Sprintf("get process output: %v (process still running, use process_output to check later)", origErr)
		}
		return ExecuteResult{
			Success:             false,
			Output:              output,
			ExitCode:            -1,
			Error:               errMsg,
			Truncated:           resp.Truncated,
			BackgroundProcessID: processID,
		}
	}

	// The server-side wait may return before the
	// process exits if maxWaitDuration is shorter than
	// the client's timeout. Retry if our context still
	// has time left.
	if resp.Running {
		if ctx.Err() == nil {
			// Still within the caller's timeout, retry.
			return waitForProcess(ctx, parentCtx, conn, processID, timeout)
		}
		output := truncateOutput(resp.Output)
		return ExecuteResult{
			Success:             false,
			Output:              output,
			ExitCode:            -1,
			Error:               fmt.Sprintf("command timed out after %s", timeout),
			Truncated:           resp.Truncated,
			BackgroundProcessID: processID,
		}
	}

	exitCode := 0
	if resp.ExitCode != nil {
		exitCode = *resp.ExitCode
	}
	output := truncateOutput(resp.Output)
	return ExecuteResult{
		Success:   exitCode == 0,
		Output:    output,
		ExitCode:  exitCode,
		Truncated: resp.Truncated,
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

const (
	// defaultProcessOutputTimeout is the default time the
	// process_output tool blocks waiting for new output or
	// process exit before returning. This avoids polling
	// loops that waste tokens and HTTP round-trips.
	defaultProcessOutputTimeout = 10 * time.Second
)

// ProcessOutputArgs are the parameters accepted by the
// process_output tool.
type ProcessOutputArgs struct {
	ProcessID   string  `json:"process_id"`
	WaitTimeout *string `json:"wait_timeout,omitempty" description:"Override the default 10s block duration. The call blocks until the process exits or this timeout is reached. Set to '0s' for an immediate snapshot without waiting."`
}

// ProcessOutput returns an AgentTool that retrieves the output
// of a tracked process by its ID.
func ProcessOutput(options ProcessToolOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"process_output",
		"Retrieve output from a tracked process by ID. "+
			"Use the process_id returned by execute with "+
			"run_in_background=true or from a timed-out "+
			"execute's background_process_id. Blocks up to "+
			"10s for the process to exit, then returns the "+
			"output and exit_code. If still running after "+
			"the timeout, returns the output so far. Use "+
			"wait_timeout to override the default 10s wait "+
			"(e.g. '30s', or '0s' for an immediate snapshot "+
			"without waiting).",
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

			timeout := defaultProcessOutputTimeout
			if args.WaitTimeout != nil {
				parsed, err := time.ParseDuration(*args.WaitTimeout)
				if err != nil {
					return fantasy.NewTextErrorResponse(
						fmt.Sprintf("invalid wait_timeout %q: %v", *args.WaitTimeout, err),
					), nil
				}
				timeout = parsed
			}
			var opts *workspacesdk.ProcessOutputOptions
			// Save parent context before applying timeout.
			parentCtx := ctx
			if timeout > 0 {
				opts = &workspacesdk.ProcessOutputOptions{
					Wait: true,
				}
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, timeout)
				defer cancel()
			}
			resp, err := conn.ProcessOutput(ctx, args.ProcessID, opts)
			if err != nil {
				// The blocking request may have failed due to a
				// context timeout or a transport error (e.g.
				// server WriteTimeout). Try a non-blocking
				// snapshot if the parent context is still alive.
				if parentCtx.Err() != nil {
					return errorResult(fmt.Sprintf("get process output: %v", err)), nil
				}
				bgCtx, bgCancel := context.WithTimeout(parentCtx, snapshotTimeout)
				defer bgCancel()
				resp, err = conn.ProcessOutput(bgCtx, args.ProcessID, nil)
				if err != nil {
					return errorResult(fmt.Sprintf("get process output: %v", err)), nil
				}
				// Fall through to normal response handling below.
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
				// Process is still running, success is not
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
			"processes or check which are still running.",
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
// tracked process on the workspace agent by its ID.
func ProcessSignal(options ProcessToolOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"process_signal",
		"Send a signal to a tracked process. "+
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

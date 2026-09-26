package chattool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"charm.land/fantasy"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

const (
	// ExecuteDefaultTimeout is how long a foreground execute call waits
	// for its command when neither the timeout argument nor
	// ExecuteOptions.DefaultTimeout is set. The tool's description and
	// timeout argument state it as 10s.
	ExecuteDefaultTimeout = 10 * time.Second

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

const (
	// shNotFoundFragment omits the trailing path variable
	// (%PATH% vs $PATH) for OS portability. Only transport
	// errors from StartProcess contain it, never command output.
	shNotFoundFragment = `exec: "sh": executable file not found`

	// shNotFoundGuidance is model-facing remediation text, relayed
	// to the user. Keep the docs anchor in sync with
	// docs/ai-coder/agents/architecture.md.
	shNotFoundGuidance = "The workspace has no POSIX shell (sh) on its PATH. " +
		"Coder Agents run commands with \"sh -c\". On Windows, install sh " +
		"via Git Bash, MSYS2, or WSL, then restart the workspace to pick " +
		"up the updated PATH. See " +
		"https://coder.com/docs/ai-coder/agents/architecture#windows-workspace-shell-requirement"
)

// enrichStartError appends actionable guidance when a StartProcess
// error indicates the workspace has no sh binary.
func enrichStartError(msg string) string {
	if strings.Contains(msg, shNotFoundFragment) {
		return msg + "\n\n" + shNotFoundGuidance
	}
	return msg
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
	Command             string                          `json:"command,omitempty"`
	Running             bool                            `json:"running,omitempty"`
	Backgrounded        bool                            `json:"backgrounded,omitempty"`
}

// ExecuteOptions configures the execute tool.
type ExecuteOptions struct {
	GetWorkspaceConn func(context.Context) (workspacesdk.AgentConn, error)
	DefaultTimeout   time.Duration
	// AgentBrowserSession, when non-empty, is exported as
	// AGENT_BROWSER_SESSION so agent-browser CLI invocations land in a
	// browser session scoped to this chat instead of a shared default.
	AgentBrowserSession string
}

// ProcessToolOptions configures a process management tool
// (process_output, process_list, or process_signal). Each of
// these tools only needs a workspace connection resolver.
type ProcessToolOptions struct {
	GetWorkspaceConn func(context.Context) (workspacesdk.AgentConn, error)
}

// ExecuteArgs are the parameters accepted by the execute tool.
type ExecuteArgs struct {
	Command         string  `json:"command" description:"The shell command to execute. Runs under \"sh -c\" (POSIX)."`
	ModelIntent     *string `json:"model_intent,omitempty" description:"A short, natural-language, present-participle phrase describing what you are doing. This is shown to the user alongside the command, with backgrounded commands framed as \"<intent> in the background using <command>\", so do not include the word \"background\" or restate the command or a duration. Use plain English with no underscores or technical jargon. Keep it under 100 characters. Good examples: \"Running the unit tests\", \"Checking repository state\", \"Inspecting build output\"."`
	Timeout         *string `json:"timeout,omitempty" description:"How long to wait for completion (e.g. '30s', '5m'). Default is 10s. The process keeps running if this expires and you get a background_process_id to re-attach. Only applies to foreground commands."`
	WorkDir         *string `json:"workdir,omitempty" description:"Working directory for the command."`
	RunInBackground *bool   `json:"run_in_background,omitempty" description:"Run without blocking. Use for persistent processes (dev servers, file watchers) or when you want to continue working while a command runs and check the result later with process_output. For commands whose result you need before continuing, prefer foreground with a longer timeout. Do NOT use shell & to background processes. It will not work correctly. Always use this parameter instead."`
}

// ExecuteToolName is the registered name of the execute tool.
const ExecuteToolName = "execute"

// RunsInBackground reports whether the execute tool runs the command in
// the background: run_in_background is set, or the command ends with a
// shell-style "&" that is not "&&" or "|&".
func (a ExecuteArgs) RunsInBackground() bool {
	if a.RunInBackground != nil && *a.RunInBackground {
		return true
	}
	_, ok := trailingAmpersandCommand(a.Command)
	return ok
}

// trailingAmpersandCommand returns command without a shell-style
// trailing "&", and whether it had one. Models sometimes use "cmd &"
// instead of run_in_background, which makes the shell fork and exit
// immediately, leaving an untracked orphan process.
func trailingAmpersandCommand(command string) (string, bool) {
	trimmed := strings.TrimSpace(command)
	if !strings.HasSuffix(trimmed, "&") || strings.HasSuffix(trimmed, "&&") || strings.HasSuffix(trimmed, "|&") {
		return command, false
	}
	return strings.TrimSpace(strings.TrimSuffix(trimmed, "&")), true
}

// EffectiveTimeout returns how long a foreground execute call waits for
// its command: the timeout argument, or optionTimeout
// (ExecuteOptions.DefaultTimeout) when the argument is unset, or 10s
// when neither is set.
func (a ExecuteArgs) EffectiveTimeout(optionTimeout time.Duration) (time.Duration, error) {
	timeout := optionTimeout
	if timeout <= 0 {
		timeout = ExecuteDefaultTimeout
	}
	if a.Timeout != nil {
		parsed, err := time.ParseDuration(*a.Timeout)
		if err != nil {
			return 0, xerrors.Errorf("invalid timeout %q: %w", *a.Timeout, err)
		}
		timeout = parsed
	}
	return timeout, nil
}

// Execute returns an AgentTool that runs a shell command in the
// workspace via the agent HTTP API.
func Execute(options ExecuteOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		ExecuteToolName,
		"Execute a shell command in the workspace. Runs under \"sh -c\" (POSIX). Waits for completion up to the timeout (default 10s, override with the timeout parameter e.g. '30s', '5m'). If the command exceeds the timeout, the response includes a background_process_id; use process_output with that ID to re-attach and wait for the result. Use run_in_background=true for persistent processes (dev servers, file watchers) or when you want to continue other work while the command runs. Never use shell '&' for backgrounding.",
		func(ctx context.Context, args ExecuteArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if options.GetWorkspaceConn == nil {
				return fantasy.NewTextErrorResponse("workspace connection resolver is not configured"), nil
			}
			conn, err := options.GetWorkspaceConn(ctx)
			if err != nil {
				// An earlier attempt of this tool call may have started
				// the command, unless no workspace agent exists to run it.
				if id, ok := ToolCallIdentityFromContext(ctx); ok && ctx.Err() == nil && !HasNoWorkspaceAgent(err) {
					return fantasy.NewTextErrorResponse(UnknownOutcome(AgentUnreachableReason(err),
						"an earlier attempt may have started the command", checkProcessText(id))), nil
				}
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			return executeTool(ctx, conn, args, options), nil
		},
	)
}

func executeTool(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	args ExecuteArgs,
	options ExecuteOptions,
) fantasy.ToolResponse {
	if args.Command == "" {
		return fantasy.NewTextErrorResponse("command is required")
	}

	// Build the environment map for the process request.
	env := make(map[string]string, len(nonInteractiveEnvVars)+2)
	env["CODER_CHAT_AGENT"] = "true"
	if options.AgentBrowserSession != "" {
		env["AGENT_BROWSER_SESSION"] = options.AgentBrowserSession
	}
	for k, v := range nonInteractiveEnvVars {
		env[k] = v
	}

	background := args.RunsInBackground()
	// A trailing "&" is promoted to background mode and stripped.
	if args.RunInBackground == nil || !*args.RunInBackground {
		args.Command, _ = trailingAmpersandCommand(args.Command)
	}

	var workDir string
	if args.WorkDir != nil {
		workDir = *args.WorkDir
	}

	if background {
		return executeBackground(ctx, conn, args.Command, workDir, env)
	}
	return executeForeground(ctx, conn, args, options.DefaultTimeout, workDir, env)
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
	startCtx := ctx
	if id, ok := ToolCallIdentityFromContext(ctx); ok {
		startCtx = workspacesdk.WithToolCall(ctx, id.AgentToolCall())
	}
	resp, err := conn.StartProcess(startCtx, workspacesdk.StartProcessRequest{
		Command:    command,
		WorkDir:    workDir,
		Env:        env,
		Background: true,
	})
	if err != nil {
		return startErrorResult(ctx, "start background process", err)
	}

	data, err := json.Marshal(backgroundStartedResult(resp.ID))
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error())
	}
	return fantasy.NewTextResponse(string(data))
}

// backgroundStartedResult is the result of a background execute call
// whose process started with processID.
func backgroundStartedResult(processID string) ExecuteResult {
	return ExecuteResult{
		Success:             true,
		BackgroundProcessID: processID,
		Backgrounded:        true,
	}
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
	timeout, err := args.EffectiveTimeout(optTimeout)
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error())
	}

	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()

	id, hasID := ToolCallIdentityFromContext(ctx)
	startCtx := cmdCtx
	if hasID {
		startCtx = workspacesdk.WithToolCall(cmdCtx, id.AgentToolCall())
	}
	resp, err := conn.StartProcess(startCtx, workspacesdk.StartProcessRequest{
		Command:    args.Command,
		WorkDir:    workDir,
		Env:        env,
		Background: false,
	})
	if err != nil {
		return startErrorResult(ctx, "start process", err)
	}

	var result ExecuteResult
	if hasID && resp.ID == id.UUID() {
		result = waitForToolCallProcess(ctx, conn, resp, timeout)
	} else {
		result = waitForProcess(cmdCtx, ctx, conn, resp.ID, timeout)
		result.WallDurationMs = time.Since(start).Milliseconds()
	}

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

// HasNoWorkspaceAgent reports whether a workspace connection error means
// no workspace agent exists: the chat has no workspace, the workspace was
// deleted, or it has no running agent. Processes do not outlive the
// agent, so no earlier attempt's command can still be running.
func HasNoWorkspaceAgent(err error) bool {
	return errors.Is(err, ErrChatHasNoWorkspace) ||
		errors.Is(err, ErrWorkspaceDeleted) ||
		errors.Is(err, ErrWorkspaceHasNoAgent)
}

// startErrorResult converts a StartProcess error into a result. ctx is
// the tool call's context. A *workspacesdk.ToolCallError is the agent's
// answer for the tool call in ctx.
func startErrorResult(ctx context.Context, action string, err error) fantasy.ToolResponse {
	id, hasID := ToolCallIdentityFromContext(ctx)
	kind, code := ClassifyAgentError(err)
	switch {
	case kind == AgentErrorRefused:
		switch code {
		case workspacesdk.ToolCallErrorAgentStartedAfterToolCall:
			return errorResult(UnknownOutcome(AgentRestartedReason,
				"the command may have run before the restart", "Check the workspace state before running it again."))
		case workspacesdk.ToolCallErrorInputMismatch:
			return errorResult(fmt.Sprintf("%s: this request changed nothing because a process for this tool call "+
				"already exists with a different input (process ID %s): %v", action, id.UUID(), err))
		default:
			// stale_tool_call and tool_call_canceled reach only a stale
			// attempt, whose commit fails the history version fence.
			return errorResult(fmt.Sprintf("%s: %v", action, err))
		}
	// A canceled ctx means the result will not be committed.
	case !hasID || kind == AgentErrorResponse || ctx.Err() != nil:
		return errorResult(enrichStartError(fmt.Sprintf("%s: %v", action, err)))
	case kind == AgentErrorUnreachable:
		return errorResult(UnknownOutcome(AgentUnreachableReason(err), "the command may have started", checkProcessText(id)))
	default:
		return errorResult(UnknownOutcome(AgentUnreadableReason(err), "the command may have started", checkProcessText(id)))
	}
}

// InterruptExecute ends a foreground execute call the user interrupted
// and returns its result. timeout is the call's effective timeout. A
// process past its execute deadline keeps running, as the timed out
// result with background_process_id promises, and so does one whose
// state cannot be read; anything else is canceled. ok is false when the
// agent's answer does not describe the tool call: an error response
// other than agent_started_after_tool_call, including the 404 of an
// agent without the cancel route.
func InterruptExecute(ctx context.Context, conn workspacesdk.AgentConn, id ToolCallIdentity, timeout time.Duration) (result ExecuteResult, ok bool) {
	processID := id.UUID()
	// The execute deadline is process start plus timeout, and the process
	// starts after the tool call is committed, so a younger tool call is
	// within it. Both ages are measured when the request is sent, after
	// the dial, so a deadline that passes during the dial keeps the process
	// running.
	if id.Age.Now() >= timeout {
		out, err := conn.ProcessOutput(ctx, processID, nil)
		if err == nil && out.Running && time.Duration(out.AgeMs)*time.Millisecond >= timeout {
			result := timedOutRunningResult(out, timeout, processID)
			result.WallDurationMs = out.AgeMs
			return result, true
		}
		if err != nil {
			// Without the agent's answer the process may be past its
			// deadline, and canceling it would break the timed out
			// result's promise.
			switch kind, _ := ClassifyAgentError(err); kind {
			case AgentErrorUnreachable:
				return notCanceledResult(id, AgentUnreachableReason(err)), true
			case AgentErrorUnreadable:
				return notCanceledResult(id, AgentUnreadableReason(err)), true
			}
		}
	}
	resp, err := conn.CancelProcess(workspacesdk.WithToolCall(ctx, id.AgentToolCall()), processID)
	return canceledExecuteResult(id, resp, err)
}

// notCanceledResult is the result of a foreground execute call whose
// process was left running because its state could not be read.
func notCanceledResult(id ToolCallIdentity, reason string) ExecuteResult {
	return ExecuteResult{Error: UnknownOutcome(reason,
		"the command, which may be past its timeout, was not canceled and may still be running", checkOrSignalProcessText(id))}
}

// InterruptBackgroundExecute returns the result of a background execute
// call the user interrupted, without ending its process. ok is false when
// the agent answers that it has no process for the tool call, including
// an agent without tool call support, which picks another process ID.
func InterruptBackgroundExecute(ctx context.Context, conn workspacesdk.AgentConn, id ToolCallIdentity) (result ExecuteResult, ok bool) {
	processID := id.UUID()
	_, err := conn.ProcessOutput(ctx, processID, nil)
	if err == nil {
		return backgroundStartedResult(processID), true
	}
	var reason string
	switch kind, _ := ClassifyAgentError(err); kind {
	case AgentErrorUnreachable:
		return AgentUnreachableBackgroundExecuteResult(id, err), true
	case AgentErrorUnreadable:
		reason = AgentUnreadableReason(err)
	default:
		return ExecuteResult{}, false
	}
	return ExecuteResult{Error: UnknownOutcome(reason, backgroundRunningEffect, checkOrSignalProcessText(id))}, true
}

// backgroundRunningEffect is what may have happened to an interrupted
// background execute call whose process the agent did not report.
const backgroundRunningEffect = "the command may be running in the background"

// canceledExecuteResult returns the result of a foreground execute call
// from the workspace agent's answer to CancelProcess for its process.
func canceledExecuteResult(id ToolCallIdentity, resp workspacesdk.CancelProcessResponse, err error) (result ExecuteResult, ok bool) {
	if err != nil {
		switch kind, code := ClassifyAgentError(err); {
		case kind == AgentErrorRefused && code == workspacesdk.ToolCallErrorAgentStartedAfterToolCall:
			return ExecuteResult{Error: UnknownOutcome(AgentRestartedReason,
				"the command may have run before the restart", "Check the workspace state before running it again.")}, true
		case kind == AgentErrorRefused, kind == AgentErrorResponse:
			return ExecuteResult{}, false
		case kind == AgentErrorUnreachable:
			return AgentUnreachableExecuteResult(id, err), true
		default:
			return ExecuteResult{Error: UnknownOutcome(AgentUnreadableReason(err),
				"the command may still be running", checkOrSignalProcessText(id))}, true
		}
	}
	switch {
	case !resp.Started:
		// The agent answers the same for a cancel that arrived before the
		// start request and for a recorded start failure.
		return ExecuteResult{Error: "not run: the command was canceled before the workspace agent received it, " +
			"or the agent failed to start it."}, true
	case resp.Canceled:
		exitCode := -1
		if resp.ExitCode != nil {
			exitCode = *resp.ExitCode
		}
		return ExecuteResult{
			Output:         truncateOutput(resp.Output),
			ExitCode:       exitCode,
			WallDurationMs: resp.AgeMs,
			Error:          fmt.Sprintf("canceled by the user after %s.", time.Duration(resp.AgeMs)*time.Millisecond),
			Truncated:      resp.Truncated,
		}, true
	default:
		result := completedResult(workspacesdk.ProcessOutputResponse{
			Output:    resp.Output,
			ExitCode:  resp.ExitCode,
			Truncated: resp.Truncated,
		})
		result.WallDurationMs = resp.AgeMs
		return result, true
	}
}

// AgentUnreachableExecuteResult returns the result of a foreground
// execute call the user interrupted when the workspace agent could not
// be reached.
func AgentUnreachableExecuteResult(id ToolCallIdentity, err error) ExecuteResult {
	return ExecuteResult{Error: UnknownOutcome(AgentUnreachableReason(err),
		"the command may still be running", checkOrSignalProcessText(id))}
}

// AgentUnreachableBackgroundExecuteResult returns the result of a
// background execute call the user interrupted when the workspace agent
// could not be reached.
func AgentUnreachableBackgroundExecuteResult(id ToolCallIdentity, err error) ExecuteResult {
	return ExecuteResult{Error: UnknownOutcome(AgentUnreachableReason(err), backgroundRunningEffect, checkOrSignalProcessText(id))}
}

// checkOrSignalProcessText tells the model how to find or stop the tool
// call's process.
func checkOrSignalProcessText(id ToolCallIdentity) string {
	return fmt.Sprintf("Check or signal it with process_output or process_signal using process ID %s.", id.UUID())
}

// checkProcessText tells the model how to find the tool call's process.
func checkProcessText(id ToolCallIdentity) string {
	return fmt.Sprintf("Check it with process_output using process ID %s.", id.UUID())
}

// waitForToolCallProcess waits for the tool call's process until it
// exits or the execute deadline, timeout after the process started, so
// a later attempt continues the same timeout instead of restarting it.
func waitForToolCallProcess(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	resp workspacesdk.StartProcessResponse,
	timeout time.Duration,
) ExecuteResult {
	waitStart := time.Now()
	ageMs := max(resp.AgeMs, 0)
	var result ExecuteResult
	if remaining := timeout - time.Duration(ageMs)*time.Millisecond; remaining > 0 {
		waitCtx, cancel := context.WithTimeout(ctx, remaining)
		defer cancel()
		result = waitForProcess(waitCtx, ctx, conn, resp.ID, timeout)
	} else {
		result = processSnapshotResult(ctx, conn, resp.ID, timeout)
	}
	// Time since the process started, as reported by the agent, plus this attempt's wait.
	result.WallDurationMs = ageMs + time.Since(waitStart).Milliseconds()
	return result
}

// processSnapshotResult reads a process past its execute deadline once
// without blocking.
func processSnapshotResult(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	processID string,
	timeout time.Duration,
) ExecuteResult {
	snapshotCtx, cancel := context.WithTimeout(ctx, snapshotTimeout)
	defer cancel()
	resp, err := conn.ProcessOutput(snapshotCtx, processID, nil)
	if err != nil {
		return ExecuteResult{
			Success:             false,
			ExitCode:            -1,
			Error:               fmt.Sprintf("command timed out after %s; failed to get output: %v", timeout, err),
			BackgroundProcessID: processID,
		}
	}
	if resp.Running {
		return timedOutRunningResult(resp, timeout, processID)
	}
	return completedResult(resp)
}

// timedOutRunningResult reports partial output plus the process ID, so
// the model can re-attach or poll.
func timedOutRunningResult(resp workspacesdk.ProcessOutputResponse, timeout time.Duration, processID string) ExecuteResult {
	return ExecuteResult{
		Success:             false,
		Output:              truncateOutput(resp.Output),
		ExitCode:            -1,
		Error:               fmt.Sprintf("command timed out after %s", timeout),
		Truncated:           resp.Truncated,
		BackgroundProcessID: processID,
	}
}

// completedResult builds the result for an exited process, treating a
// missing exit code as success.
func completedResult(resp workspacesdk.ProcessOutputResponse) ExecuteResult {
	exitCode := 0
	if resp.ExitCode != nil {
		exitCode = *resp.ExitCode
	}
	return ExecuteResult{
		Success:   exitCode == 0,
		Output:    truncateOutput(resp.Output),
		ExitCode:  exitCode,
		Truncated: resp.Truncated,
	}
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
			return completedResult(resp)
		}

		// Process still running, return partial output.
		if timedOut {
			return timedOutRunningResult(resp, timeout, processID)
		}
		return ExecuteResult{
			Success:             false,
			Output:              truncateOutput(resp.Output),
			ExitCode:            -1,
			Error:               fmt.Sprintf("get process output: %v (process still running, use process_output to check later)", origErr),
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
		return timedOutRunningResult(resp, timeout, processID)
	}

	return completedResult(resp)
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
	ModelIntent *string `json:"model_intent,omitempty" description:"A short, natural-language, present-participle phrase describing why you are checking this process. This is shown as the user's primary label for the action, so make it self-sufficient: the command itself is not displayed alongside it. Use plain English with no underscores or technical jargon. Do not restate the command or include a duration. Keep it under 100 characters. Good examples: \"Waiting for the dev server to be ready\", \"Confirming the tests still pass\"."`
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
				Command:   resp.Command,
			}
			if resp.Running {
				// Process is still running, success is not
				// yet determined.
				result.Success = true
				result.Running = true
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

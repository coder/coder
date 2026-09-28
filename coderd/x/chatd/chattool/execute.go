package chattool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"charm.land/fantasy"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/quartz"
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

	// agentOutputWaitCap is the longest the agent holds a blocking
	// output request before it answers with the process still running
	// (maxWaitDuration in agent/agentproc).
	agentOutputWaitCap = 5 * time.Minute
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
	// Clock times the retries of requests the agent does not answer
	// (AgentAnswerTimeout). Nil means a real clock.
	Clock quartz.Clock
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
	RunInBackground *bool   `json:"run_in_background,omitempty" description:"Run without blocking. Use for persistent processes (dev servers, file watchers) or when you want to continue working while a command runs and check the result later with process_output. For commands whose result you need before continuing, prefer foreground with a longer timeout. Use this parameter instead of shell '&', which leaves an untracked process that process_output cannot read."`
}

// ExecuteToolName is the registered name of the execute tool.
const ExecuteToolName = "execute"

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
				if id, ok := ToolCallIdentityFromContext(ctx); ok {
					if text, ok := ConnErrorText(err, executeWords(id)); ok {
						return errorResult(text), nil
					}
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

	if id, ok := ToolCallIdentityFromContext(ctx); ok {
		if id.Cause == ToolCallCauseInterrupt {
			// After the cancel the agent never starts the command, so
			// the start below replays the recorded response or gets
			// tool_call_canceled.
			if err := cancelToolCall(ctx, conn, options.Clock, id); err != nil {
				return cancelErrorResult(id, err)
			}
		}
		if background {
			return executeBackgroundToolCall(ctx, conn, options.Clock, id, args.Command, workDir, env)
		}
		return executeForegroundToolCall(ctx, conn, options.Clock, id, args, options.DefaultTimeout, workDir, env)
	}
	if background {
		return executeBackground(ctx, conn, args.Command, workDir, env)
	}
	return executeForeground(ctx, conn, args, options.DefaultTimeout, workDir, env)
}

// foregroundTimeout returns the execute timeout for a foreground
// command: the timeout argument, else optTimeout, else defaultTimeout.
func foregroundTimeout(args ExecuteArgs, optTimeout time.Duration) (time.Duration, error) {
	timeout := optTimeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	if args.Timeout != nil {
		parsed, err := time.ParseDuration(*args.Timeout)
		if err != nil {
			return 0, xerrors.Errorf("invalid timeout %q: %v", *args.Timeout, err)
		}
		timeout = parsed
	}
	return timeout, nil
}

// executeWords are the words execute uses in tool call results.
func executeWords(id ToolCallIdentity) ToolCallWords {
	return ToolCallWords{
		NotRun:       "command not run",
		Effect:       "the command may have run",
		Check:        fmt.Sprintf("Check it with process_output using process ID %s.", id.UUID()),
		UnknownCheck: "Check the workspace state before running it again.",
	}
}

// cancelToolCall cancels the tool call id on the agent until the agent
// answers or AgentAnswerTimeout ends. A nil error means the tool call's
// outcome is final: any process it started has stopped.
func cancelToolCall(ctx context.Context, conn workspacesdk.AgentConn, clock quartz.Clock, id ToolCallIdentity) error {
	ctx = workspacesdk.WithToolCall(ctx, id.AgentToolCall())
	return RequestUntilAnswered(ctx, clock, func(ctx context.Context) error {
		return conn.CancelToolCall(ctx, id.UUID())
	})
}

// cancelErrorResult converts the error of a failed cancel into a
// result. The start is never sent after a failed cancel, because it
// could start the command the user interrupted.
func cancelErrorResult(id ToolCallIdentity, err error) fantasy.ToolResponse {
	var sdkErr *codersdk.Error
	if errors.As(err, &sdkErr) && sdkErr.StatusCode() == http.StatusNotFound {
		// The agent has no cancel route, so it is older than its
		// api_version said and the call keeps the generic result.
		return fantasy.NewTextErrorResponse(InterruptedToolResultMessage)
	}
	words := executeWords(id)
	if text, ok := ToolCallErrorText(err, words); ok {
		return errorResult(text)
	}
	return errorResult(UnknownOutcome(fmt.Sprintf("the workspace agent refused to cancel it (%v)", err), words.Effect, words.Check))
}

// startToolCallProcess sends req with the tool call headers of id until
// the agent answers or AgentAnswerTimeout ends.
func startToolCallProcess(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	clock quartz.Clock,
	id ToolCallIdentity,
	req workspacesdk.StartProcessRequest,
) (resp workspacesdk.StartProcessResponse, err error) {
	ctx = workspacesdk.WithToolCall(ctx, id.AgentToolCall())
	err = RequestUntilAnswered(ctx, clock, func(ctx context.Context) error {
		resp, err = conn.StartProcess(ctx, req)
		return err
	})
	return resp, err
}

// toolCallStartErrorResult converts the error of a start request with
// tool call headers into a result.
func toolCallStartErrorResult(id ToolCallIdentity, action string, err error) fantasy.ToolResponse {
	if text, ok := ToolCallErrorText(err, executeWords(id)); ok {
		return errorResult(text)
	}
	return errorResult(enrichStartError(fmt.Sprintf("%s: %v", action, err)))
}

// executeBackgroundToolCall starts a background process for the tool
// call id and returns its process ID.
func executeBackgroundToolCall(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	clock quartz.Clock,
	id ToolCallIdentity,
	command string,
	workDir string,
	env map[string]string,
) fantasy.ToolResponse {
	resp, err := startToolCallProcess(ctx, conn, clock, id, workspacesdk.StartProcessRequest{
		Command:    command,
		WorkDir:    workDir,
		Env:        env,
		Background: true,
	})
	if err != nil {
		return toolCallStartErrorResult(id, "start background process", err)
	}
	if id.Cause == ToolCallCauseInterrupt {
		return marshalResult(readCanceledBackgroundProcess(ctx, conn, clock, resp.ID))
	}
	return marshalResult(ExecuteResult{
		Success:             true,
		BackgroundProcessID: resp.ID,
		Backgrounded:        true,
	})
}

// executeForegroundToolCall starts a foreground process for the tool
// call id with the execute timeout, and waits until the agent reports
// that it exited or passed its execute deadline. The agent keeps the
// deadline of the first start, so a task retry or ownership change
// continues the same wait.
func executeForegroundToolCall(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	clock quartz.Clock,
	id ToolCallIdentity,
	args ExecuteArgs,
	optTimeout time.Duration,
	workDir string,
	env map[string]string,
) fantasy.ToolResponse {
	timeout, err := foregroundTimeout(args, optTimeout)
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error())
	}

	resp, err := startToolCallProcess(ctx, conn, clock, id, workspacesdk.StartProcessRequest{
		Command: args.Command,
		WorkDir: workDir,
		Env:     env,
		// Zero means no deadline to the agent, so a zero or negative
		// timeout is sent as the shortest one.
		TimeoutMs: max(timeout.Milliseconds(), 1),
	})
	if err != nil {
		return toolCallStartErrorResult(id, "start process", err)
	}

	// After an interrupt's cancel the process has stopped, so the
	// agent answers without waiting.
	waitBound := agentOutputWaitCap + AgentAnswerTimeout
	if id.Cause == ToolCallCauseInterrupt {
		waitBound = AgentAnswerTimeout
	}
	result := waitForToolCallProcess(ctx, conn, clock, resp.ID, timeout, waitBound)
	if note := detectFileDump(args.Command); note != "" {
		result.Note = note
	}
	return marshalResult(result)
}

// waitForToolCallProcess waits on the output of a process started with
// an execute deadline until the agent reports that it exited or timed
// out. A read the agent does not answer is sent again until waitBound
// ends: the agent's wait cap plus AgentAnswerTimeout in generation.
func waitForToolCallProcess(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	clock quartz.Clock,
	processID string,
	timeout time.Duration,
	waitBound time.Duration,
) ExecuteResult {
	for {
		var resp workspacesdk.ProcessOutputResponse
		err := requestUntilAnsweredWithin(ctx, clock, waitBound, func(ctx context.Context) error {
			var err error
			resp, err = conn.ProcessOutput(ctx, processID, &workspacesdk.ProcessOutputOptions{Wait: true})
			return err
		})
		if err != nil {
			return unreadOutputResult(processID, err)
		}
		var result ExecuteResult
		switch {
		case !resp.Running && resp.Canceled:
			result = canceledResult(resp)
		case !resp.Running:
			result = completedResult(resp)
		case resp.TimedOut:
			result = timedOutRunningResult(resp, timeout, processID)
		default:
			// The agent's wait cap ended before the deadline.
			continue
		}
		result.WallDurationMs = resp.DurationMs
		return result
	}
}

// unreadOutputResult is the result when the output of processID could
// not be read. The command may have exited or passed its deadline, so
// neither is claimed.
func unreadOutputResult(processID string, err error) ExecuteResult {
	return ExecuteResult{
		Success:  false,
		ExitCode: -1,
		Error: UnknownOutcome(fmt.Sprintf("the command's result could not be read (%v)", err),
			"the command may still be running or may have finished",
			fmt.Sprintf("Check it with process_output using process ID %s.", processID)),
		BackgroundProcessID: processID,
	}
}

// readCanceledBackgroundProcess reads the output of the background
// process processID once after an interrupt canceled its tool call,
// until the agent answers or AgentAnswerTimeout ends. A process that is
// still running, because it was not stopped, keeps the background start
// result.
func readCanceledBackgroundProcess(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	clock quartz.Clock,
	processID string,
) ExecuteResult {
	var resp workspacesdk.ProcessOutputResponse
	err := RequestUntilAnswered(ctx, clock, func(ctx context.Context) error {
		var err error
		resp, err = conn.ProcessOutput(ctx, processID, nil)
		return err
	})
	if err != nil {
		return unreadOutputResult(processID, err)
	}
	var result ExecuteResult
	switch {
	case resp.Running:
		return ExecuteResult{
			Success:             true,
			BackgroundProcessID: processID,
			Backgrounded:        true,
		}
	case resp.Canceled:
		result = canceledResult(resp)
	default:
		result = completedResult(resp)
	}
	result.WallDurationMs = resp.DurationMs
	return result
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

// canceledResult builds the result for a process a cancel killed, with
// its partial output.
func canceledResult(resp workspacesdk.ProcessOutputResponse) ExecuteResult {
	exitCode := -1
	if resp.ExitCode != nil {
		exitCode = *resp.ExitCode
	}
	return ExecuteResult{
		Success:   false,
		Output:    truncateOutput(resp.Output),
		ExitCode:  exitCode,
		Error:     fmt.Sprintf("command canceled by the user after %s", time.Duration(resp.DurationMs)*time.Millisecond),
		Truncated: resp.Truncated,
	}
}

// timedOutRunningResult reports partial output plus the process ID of a
// process still running past its execute deadline, so the model can
// re-attach or poll.
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

// marshalResult returns result as a JSON text response.
func marshalResult(result ExecuteResult) fantasy.ToolResponse {
	data, err := json.Marshal(result)
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error())
	}
	return fantasy.NewTextResponse(string(data))
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
		return errorResult(enrichStartError(fmt.Sprintf("start background process: %v", err)))
	}

	result := ExecuteResult{
		Success:             true,
		BackgroundProcessID: resp.ID,
		Backgrounded:        true,
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
	timeout, err := foregroundTimeout(args, optTimeout)
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error())
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
		return errorResult(enrichStartError(fmt.Sprintf("start process: %v", err)))
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

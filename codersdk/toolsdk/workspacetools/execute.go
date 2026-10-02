// Package workspacetools implements the workspace tool behavior shared by
// Coder Agents (coderd/x/chatd/chattool) and the Coder MCP server
// (codersdk/toolsdk). It talks to the workspace agent through
// workspacesdk.AgentConn and has no dependency on either tool framework,
// so both surfaces run the same commands and return the same results.
package workspacetools

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

const (
	// DefaultExecuteTimeout is how long a foreground command runs before
	// the caller gets a background process ID instead of the final result.
	DefaultExecuteTimeout = 10 * time.Second

	// DefaultProcessOutputWait is how long a process output request
	// blocks for the process to exit before returning output so far.
	DefaultProcessOutputWait = 10 * time.Second

	// MaxOutputBytes is the maximum command output returned to a model.
	MaxOutputBytes = 32 << 10 // 32KB

	// snapshotTimeout is how long a non-blocking fallback request is
	// allowed to take when retrieving a process output snapshot after a
	// blocking wait fails.
	snapshotTimeout = 30 * time.Second
)

// NonInteractiveEnv is set on every process to prevent interactive
// prompts that would hang a headless execution.
var NonInteractiveEnv = map[string]string{
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
var fileDumpPatterns = []*regexp.Regexp{
	regexp.MustCompile(`^cat\s+`),
	regexp.MustCompile(`^(rg|grep)\s+.*--include-all`),
	regexp.MustCompile(`^(rg|grep)\s+-l\s+`),
}

const (
	// shNotFoundFragment omits the trailing path variable (%PATH% vs
	// $PATH) for OS portability. Only transport errors from
	// StartProcess contain it, never command output.
	shNotFoundFragment = `exec: "sh": executable file not found`

	// shNotFoundGuidance is model-facing remediation text, relayed to
	// the user. Keep the docs anchor in sync with
	// docs/ai-coder/agents/architecture.md.
	shNotFoundGuidance = "The workspace has no POSIX shell (sh) on its PATH. " +
		"Coder Agents run commands with \"sh -c\". On Windows, install sh " +
		"via Git Bash, MSYS2, or WSL, then restart the workspace to pick " +
		"up the updated PATH. See " +
		"https://coder.com/docs/ai-coder/agents/architecture#windows-workspace-shell-requirement"
)

// ToolNames are the names under which a tool surface registers the
// workspace tools. Descriptions and follow-up hints reference them so a
// model is always pointed at a tool that exists on its surface.
type ToolNames struct {
	Execute       string
	ProcessOutput string
	ProcessList   string
	ReadFile      string
	EditFiles     string
}

// ExecuteResult is the structured response from the execute and
// process output tools.
type ExecuteResult struct {
	Success             bool                            `json:"success"`
	Output              string                          `json:"output,omitempty"`
	ExitCode            int                             `json:"exit_code"`
	WallDurationMs      int64                           `json:"wall_duration_ms,omitempty"`
	Error               string                          `json:"error,omitempty"`
	Truncated           *workspacesdk.ProcessTruncation `json:"truncated,omitempty"`
	Note                string                          `json:"note,omitempty"`
	BackgroundProcessID string                          `json:"background_process_id,omitempty"`
	Command             string                          `json:"command,omitempty"`
	Running             bool                            `json:"running,omitempty"`
	Backgrounded        bool                            `json:"backgrounded,omitempty"`
	Canceled            bool                            `json:"canceled,omitempty"` // an interrupt canceled the command while it ran
}

// ExecuteRequest describes a command to run in the workspace.
type ExecuteRequest struct {
	Command string
	// Timeout is the caller-supplied wait duration, such as "30s".
	// Nil uses DefaultTimeout.
	Timeout         *string
	WorkDir         string
	RunInBackground bool
	// DefaultTimeout overrides DefaultExecuteTimeout when positive.
	DefaultTimeout time.Duration
	// Env is merged over NonInteractiveEnv.
	Env map[string]string
	// Names supplies the tool names used in follow-up hints.
	Names ToolNames
}

// Execute runs a command in the workspace through the agent's process
// API. A returned error means the request itself is invalid and nothing
// ran; every other outcome, including start failures and timeouts, is
// reported in the ExecuteResult.
func Execute(ctx context.Context, conn workspacesdk.AgentConn, req ExecuteRequest) (ExecuteResult, error) {
	if req.Command == "" {
		return ExecuteResult{}, xerrors.New("command is required")
	}

	env := make(map[string]string, len(NonInteractiveEnv)+len(req.Env))
	for k, v := range req.Env {
		env[k] = v
	}
	for k, v := range NonInteractiveEnv {
		env[k] = v
	}

	command := req.Command
	background := req.RunInBackground
	// Detect shell-style backgrounding (trailing &) and promote to
	// background mode. Models sometimes use "cmd &" instead of the
	// run_in_background parameter, which causes the shell to fork and
	// exit immediately, leaving an untracked orphan process.
	trimmed := strings.TrimSpace(command)
	if !background && strings.HasSuffix(trimmed, "&") && !strings.HasSuffix(trimmed, "&&") && !strings.HasSuffix(trimmed, "|&") {
		background = true
		command = strings.TrimSpace(strings.TrimSuffix(trimmed, "&"))
	}

	if background {
		resp, err := conn.StartProcess(ctx, workspacesdk.StartProcessRequest{
			Command:    command,
			WorkDir:    req.WorkDir,
			Env:        env,
			Background: true,
		})
		if err != nil {
			return StartErrorResult("start background process", err), nil
		}
		return ExecuteResult{
			Success:             true,
			BackgroundProcessID: resp.ID,
			Backgrounded:        true,
		}, nil
	}

	timeout := req.DefaultTimeout
	if timeout <= 0 {
		timeout = DefaultExecuteTimeout
	}
	if req.Timeout != nil {
		parsed, err := time.ParseDuration(*req.Timeout)
		if err != nil {
			return ExecuteResult{}, xerrors.Errorf("invalid timeout %q: %v", *req.Timeout, err)
		}
		timeout = parsed
	}

	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	resp, err := conn.StartProcess(cmdCtx, workspacesdk.StartProcessRequest{
		Command:    command,
		WorkDir:    req.WorkDir,
		Env:        env,
		Background: false,
		TimeoutMs:  timeout.Milliseconds(),
	})
	if err != nil {
		return StartErrorResult("start process", err), nil
	}

	result := waitForProcess(cmdCtx, ctx, conn, resp.ID, timeout, req.Names)
	result.WallDurationMs = time.Since(start).Milliseconds()
	if note := detectFileDump(command, req.Names); note != "" {
		result.Note = note
	}
	return result, nil
}

// StartErrorResult reports a failed process start, adding remediation
// guidance when the workspace has no POSIX shell.
func StartErrorResult(action string, err error) ExecuteResult {
	msg := fmt.Sprintf("%s: %v", action, err)
	if strings.Contains(msg, shNotFoundFragment) {
		msg += "\n\n" + shNotFoundGuidance
	}
	return ExecuteResult{Success: false, Error: msg}
}

// ProcessOutput retrieves a tracked process's output. A nil waitTimeout
// blocks for up to DefaultProcessOutputWait; "0s" returns a snapshot
// without waiting. A returned error means the request is invalid.
func ProcessOutput(ctx context.Context, conn workspacesdk.AgentConn, processID string, waitTimeout *string) (ExecuteResult, error) {
	if processID == "" {
		return ExecuteResult{}, xerrors.New("process_id is required")
	}
	timeout := DefaultProcessOutputWait
	if waitTimeout != nil {
		parsed, err := time.ParseDuration(*waitTimeout)
		if err != nil {
			return ExecuteResult{}, xerrors.Errorf("invalid wait_timeout %q: %v", *waitTimeout, err)
		}
		timeout = parsed
	}

	var opts *workspacesdk.ProcessOutputOptions
	parentCtx := ctx
	if timeout > 0 {
		opts = &workspacesdk.ProcessOutputOptions{Wait: true}
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	resp, err := conn.ProcessOutput(ctx, processID, opts)
	if err != nil {
		// The blocking request may have failed due to a context timeout
		// or a transport error (e.g. server WriteTimeout). Try a
		// non-blocking snapshot if the parent context is still alive.
		if parentCtx.Err() != nil {
			return ExecuteResult{Error: fmt.Sprintf("get process output: %v", err)}, nil
		}
		bgCtx, bgCancel := context.WithTimeout(parentCtx, snapshotTimeout)
		defer bgCancel()
		resp, err = conn.ProcessOutput(bgCtx, processID, nil)
		if err != nil {
			return ExecuteResult{Error: fmt.Sprintf("get process output: %v", err)}, nil
		}
	}

	exitCode := 0
	if resp.ExitCode != nil {
		exitCode = *resp.ExitCode
	}
	result := ExecuteResult{
		Success:   !resp.Running && exitCode == 0,
		Output:    TruncateOutput(resp.Output),
		ExitCode:  exitCode,
		Truncated: resp.Truncated,
		Command:   resp.Command,
	}
	if resp.Running {
		// Success is not yet determined while the process runs.
		result.Success = true
		result.Running = true
		result.Note = "process is still running"
	}
	return result, nil
}

// ValidateSignal reports whether signal is one the process tools accept.
func ValidateSignal(signal string) error {
	if signal != "terminate" && signal != "kill" {
		return xerrors.New(`signal must be "terminate" or "kill"`)
	}
	return nil
}

// SignalResult is the structured success response from the process
// signal tool.
type SignalResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// SignalSent reports that signal was delivered to processID.
func SignalSent(processID, signal string) SignalResult {
	return SignalResult{
		Success: true,
		Message: fmt.Sprintf("signal %q sent to process %s", signal, processID),
	}
}

// TruncateOutput truncates output to MaxOutputBytes, keeping the result
// valid UTF-8 even if the cut falls inside a multi-byte character.
func TruncateOutput(output string) string {
	if len(output) > MaxOutputBytes {
		output = strings.ToValidUTF8(output[:MaxOutputBytes], "")
	}
	return output
}

// ExitedResult converts the output of a finished process to a result.
func ExitedResult(resp workspacesdk.ProcessOutputResponse) ExecuteResult {
	exitCode := 0
	if resp.ExitCode != nil {
		exitCode = *resp.ExitCode
	}
	return ExecuteResult{
		Success:   exitCode == 0,
		Output:    TruncateOutput(resp.Output),
		ExitCode:  exitCode,
		Truncated: resp.Truncated,
	}
}

// waitForProcess blocks until the process exits or ctx expires. On any
// error (timeout or transport), it tries a non-blocking snapshot to
// recover. Total wall time may exceed timeout by up to snapshotTimeout
// if recovery is needed.
func waitForProcess(
	ctx context.Context,
	parentCtx context.Context,
	conn workspacesdk.AgentConn,
	processID string,
	timeout time.Duration,
	names ToolNames,
) ExecuteResult {
	resp, err := conn.ProcessOutput(ctx, processID, &workspacesdk.ProcessOutputOptions{
		Wait:                    true,
		TimeoutFromStartProcess: true,
	})
	if err != nil {
		origErr := err
		timedOut := ctx.Err() != nil

		// The blocking request may have failed due to a context timeout
		// or a transport error (e.g. the server's WriteTimeout killed the
		// connection). Either way, the process may still have output.
		bgCtx, bgCancel := context.WithTimeout(parentCtx, snapshotTimeout)
		defer bgCancel()
		resp, err = conn.ProcessOutput(bgCtx, processID, nil)
		if err != nil {
			errMsg := fmt.Sprintf("get process output: %v; use %s with ID %s to retry", origErr, names.ProcessOutput, processID)
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

		if !resp.Running {
			return ExitedResult(resp)
		}

		errMsg := fmt.Sprintf("command timed out after %s", timeout)
		if !timedOut {
			errMsg = fmt.Sprintf("get process output: %v (process still running, use %s to check later)", origErr, names.ProcessOutput)
		}
		return ExecuteResult{
			Success:             false,
			Output:              TruncateOutput(resp.Output),
			ExitCode:            -1,
			Error:               errMsg,
			Truncated:           resp.Truncated,
			BackgroundProcessID: processID,
		}
	}

	// The server-side wait may return before the process exits if its
	// maximum wait is shorter than the client's timeout. Retry while
	// ctx has time left and the execute timeout, counted from the first
	// start, has not passed.
	if resp.Running {
		if ctx.Err() == nil && !resp.TimedOut {
			return waitForProcess(ctx, parentCtx, conn, processID, timeout, names)
		}
		return ExecuteResult{
			Success:             false,
			Output:              TruncateOutput(resp.Output),
			ExitCode:            -1,
			Error:               fmt.Sprintf("command timed out after %s", timeout),
			Truncated:           resp.Truncated,
			BackgroundProcessID: processID,
		}
	}

	return ExitedResult(resp)
}

// detectFileDump returns an advisory note when the command dumps file
// contents that the read file tool would return more efficiently.
func detectFileDump(command string, names ToolNames) string {
	for _, pat := range fileDumpPatterns {
		if pat.MatchString(command) {
			return "Consider using " + names.ReadFile + " instead of " +
				"dumping file contents with shell commands."
		}
	}
	return ""
}

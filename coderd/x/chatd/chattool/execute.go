package chattool

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/codersdk"
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

	// maxExecuteTimeout is the upper bound for the execute
	// tool's timeout and the process_output tool's
	// wait_timeout. Longer requests are clamped, not
	// rejected.
	maxExecuteTimeout = 4 * time.Hour

	// recordWriteTimeout bounds the uncanceled ledger writes for
	// process starts and terminal observations.
	recordWriteTimeout = 15 * time.Second

	// claimStaleAfter is how long a starting claim stays fresh.
	// A claimer that has not recorded a process handle within
	// this window is presumed dead and the process state is
	// declared unknown; it may have dispatched a process whose
	// handle was lost.
	claimStaleAfter = 60 * time.Second

	// claimPollInterval is how often a fresh starting claim is
	// re-read while waiting for its owner to record the process
	// handle.
	claimPollInterval = 2 * time.Second
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
}

// ExecutionStatus mirrors the chat_tool_call_executions status enum.
// Lifecycle statuses are orthogonal to chat-result commit: a row
// keeps its lifecycle truth (for example unknown or detached) after
// its tool result has been committed.
type ExecutionStatus string

const (
	// ExecutionStatusReserved means the intent was persisted with
	// the assistant message and nothing was dispatched.
	ExecutionStatusReserved ExecutionStatus = "reserved"
	// ExecutionStatusStarting means a runner claimed the execution
	// and dispatch may be in flight.
	ExecutionStatusStarting ExecutionStatus = "starting"
	// ExecutionStatusRunning means the process identity was
	// recorded.
	ExecutionStatusRunning ExecutionStatus = "running"
	// ExecutionStatusExited means the tool observed the process
	// exit.
	ExecutionStatusExited ExecutionStatus = "exited"
	// ExecutionStatusDetached means the chat moved on and the
	// process was deliberately left alive (background start,
	// timed-out foreground, or interrupted background).
	ExecutionStatusDetached ExecutionStatus = "detached"
	// ExecutionStatusCancelRequested means an interrupt resolved
	// the call in chat but process termination is unconfirmed.
	ExecutionStatusCancelRequested ExecutionStatus = "cancel_requested"
	// ExecutionStatusCanceled means termination was confirmed or
	// nothing was ever dispatched.
	ExecutionStatusCanceled ExecutionStatus = "canceled"
	// ExecutionStatusUnknown means the command may have run but
	// its outcome is unobservable. Terminal for auto-restart.
	ExecutionStatusUnknown ExecutionStatus = "unknown"
	// ExecutionStatusNoEffect means validation failed before any
	// external dispatch.
	ExecutionStatusNoEffect ExecutionStatus = "no_effect"
)

// AbortToolExecutionError marks a tool failure that must abort the
// whole local tool batch instead of committing an error tool
// result. Used for execution ledger infrastructure failures before
// any process is dispatched: committing a bogus result would end
// the call permanently, while aborting lets the task retry re-claim
// the intent (dispatched siblings re-attach through the ledger).
type AbortToolExecutionError struct {
	Err error
}

func (e *AbortToolExecutionError) Error() string {
	return e.Err.Error()
}

func (e *AbortToolExecutionError) Unwrap() error {
	return e.Err
}

// ErrExecutionRecordNotFound reports that no ledger row exists for
// the tool call, distinguishing a call that predates the ledger
// from an unreadable ledger.
var ErrExecutionRecordNotFound = xerrors.New("execution record not found")

// ErrExecutionInputMismatch reports that the ledger row targeted by
// a claim was created for different tool input, meaning the caller
// is executing against stale lineage.
var ErrExecutionInputMismatch = xerrors.New("execution ledger input hash mismatch")

// HashToolInput returns the hex SHA-256 of a tool call's raw
// persisted input. The intent writer and the claiming tool hash the
// same persisted bytes, so a mismatch proves stale lineage.
func HashToolInput(input string) string {
	sum := sha256.Sum256([]byte(input))
	return hex.EncodeToString(sum[:])
}

// ExecutionRecord is one row of the durable execution ledger for
// execute tool calls. It lets a retried task attempt re-attach to a
// process started by a previous attempt instead of spawning a
// duplicate.
type ExecutionRecord struct {
	// ID is the stable execution identity, generated at intent
	// creation.
	ID     string
	Status ExecutionStatus
	// ProcessID is empty until the process start is recorded.
	ProcessID  string
	Command    string
	Background bool
	Timeout    time.Duration
	// ClaimEpoch guards process-identity writes; it advances on
	// every claim takeover.
	ClaimEpoch int64
	ClaimedAt  time.Time
	StartedAt  time.Time
	// WorkspaceAgentID is the agent that owns the recorded
	// process. A 404 from a different agent proves nothing about
	// the process, so re-attach only treats it as loss when the
	// probed agent matches.
	WorkspaceAgentID uuid.UUID
}

// ExecutionRecorder is the tool's view of the execution ledger.
// Rows are keyed by tool call ID within the generation step's
// assistant message, which is durable in chat history before
// execution begins.
type ExecutionRecorder interface {
	// Claim attempts to take ownership of dispatch for the tool
	// call, creating the row when the assistant message predates
	// the ledger. claimed reports whether this caller owns the
	// fresh dispatch; when false, rec is the current row and the
	// caller must resume from its status instead of dispatching.
	// Returns an error wrapping ErrExecutionInputMismatch when the
	// row was created for different input.
	Claim(ctx context.Context, toolCallID string, inputSHA256 string, command string, background bool, timeout time.Duration) (rec ExecutionRecord, claimed bool, err error)
	// Get reads the current row without claiming it. Returns an
	// error wrapping ErrExecutionRecordNotFound when no row
	// exists for the tool call.
	Get(ctx context.Context, toolCallID string) (ExecutionRecord, error)
	// MarkStaleClaimUnknown resolves a stale starting claim to
	// unknown. Unlike MarkTerminal it matches only starting rows:
	// the claim owner can record its process concurrently with
	// the staleness verdict, and that write must win. applied is
	// false when the row advanced first; latest is then the fresh
	// row for the caller to resume from.
	MarkStaleClaimUnknown(ctx context.Context, toolCallID string) (latest ExecutionRecord, applied bool, err error)
	// RecordStart stores the process identity on the claim that
	// dispatched it and moves the row to running. claimEpoch must
	// be the epoch returned by the Claim that owns the dispatch.
	RecordStart(ctx context.Context, toolCallID string, claimEpoch int64, processID string, agentID uuid.UUID) error
	// MarkTerminal applies a lifecycle observation (exited,
	// detached, unknown, or no_effect). Observations never
	// overwrite states written by the interrupt reconciler.
	MarkTerminal(ctx context.Context, toolCallID string, status ExecutionStatus) error
}

// ExecuteOptions configures the execute tool.
type ExecuteOptions struct {
	// GetWorkspaceConn returns the workspace connection together
	// with the ID of the agent behind it, captured atomically so
	// ledger writes attribute the process to the agent that
	// actually served the dispatch.
	GetWorkspaceConn func(context.Context) (workspacesdk.AgentConn, uuid.UUID, error)
	DefaultTimeout   time.Duration
	// Logger records ledger observability events. The zero Logger
	// is a valid no-op.
	Logger slog.Logger
	// Recorder persists per-tool-call execution records so a
	// retried attempt re-attaches instead of starting a
	// duplicate process. A nil Recorder disables idempotent
	// starts.
	Recorder ExecutionRecorder
	// connAgentID is the agent behind the conn returned by
	// GetWorkspaceConn, captured with it and carried to ledger
	// writes on this options copy.
	connAgentID uuid.UUID
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
	ModelIntent     *string `json:"model_intent,omitempty" description:"A short, natural-language, present-participle phrase describing what you are doing. This is shown to the user alongside the command. Use plain English with no underscores or technical jargon. The UI appends \"using <command>\" and \"for <duration>\" automatically, so do not repeat the command or include a duration. Keep it under 100 characters. Good examples: \"Running the unit tests\", \"Checking repository state\", \"Inspecting build output\"."`
	Timeout         *string `json:"timeout,omitempty" description:"How long to wait for completion (e.g. '30s', '5m'). Default is 10s, maximum is 4h (longer values are clamped). The process keeps running if this expires and you get a background_process_id to re-attach. Only applies to foreground commands."`
	WorkDir         *string `json:"workdir,omitempty" description:"Working directory for the command."`
	RunInBackground *bool   `json:"run_in_background,omitempty" description:"Run without blocking. Use for persistent processes (dev servers, file watchers) or when you want to continue working while a command runs and check the result later with process_output. For commands whose result you need before continuing, prefer foreground with a longer timeout. Do NOT use shell & to background processes. It will not work correctly. Always use this parameter instead."`
}

// ExecuteToolName is the registered name of the execute tool.
const ExecuteToolName = "execute"

// Execute returns an AgentTool that runs a shell command in the
// workspace via the agent HTTP API.
func Execute(options ExecuteOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		ExecuteToolName,
		"Execute a shell command in the workspace. Runs under \"sh -c\" (POSIX). Waits for completion up to the timeout (default 10s, override with the timeout parameter e.g. '30s', '5m'). If the command exceeds the timeout, the response includes a background_process_id; use process_output with that ID to re-attach and wait for the result. Use run_in_background=true for persistent processes (dev servers, file watchers) or when you want to continue other work while the command runs. Never use shell '&' for backgrounding.",
		func(ctx context.Context, args ExecuteArgs, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if options.GetWorkspaceConn == nil {
				return fantasy.NewTextErrorResponse("workspace connection resolver is not configured"), nil
			}
			conn, agentID, err := options.GetWorkspaceConn(ctx)
			if err != nil {
				return resolveConnFailure(ctx, options, call.ID, err)
			}
			// Concurrent execute calls in one step share this
			// closure; stamp the agent on a per-call copy so one
			// call cannot overwrite another's attribution.
			callOptions := options
			callOptions.connAgentID = agentID
			return executeTool(ctx, conn, args, callOptions, call)
		},
	)
}

// unknownOutcomeMessage is the stable result for executions whose
// process state cannot be observed. It is identical across retries
// so a re-executed call converges instead of flapping.
const unknownOutcomeMessage = "a previous attempt may have started this command, but its process handle was lost and the process state is unknown. Re-run the command only if it is safe to run twice."

func executeTool(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	args ExecuteArgs,
	options ExecuteOptions,
	call fantasy.ToolCall,
) (fantasy.ToolResponse, error) {
	toolCallID := call.ID
	if args.Command == "" {
		markTerminal(ctx, options, toolCallID, ExecutionStatusNoEffect)
		return fantasy.NewTextErrorResponse("command is required"), nil
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

	timeout := options.DefaultTimeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	// The timeout argument only applies to foreground commands,
	// so backgrounded calls ignore it entirely instead of failing
	// validation.
	if args.Timeout != nil && !background {
		parsed, err := time.ParseDuration(*args.Timeout)
		if err != nil {
			markTerminal(ctx, options, toolCallID, ExecutionStatusNoEffect)
			return fantasy.NewTextErrorResponse(
				fmt.Sprintf("invalid timeout %q: %v", *args.Timeout, err),
			), nil
		}
		if parsed <= 0 {
			markTerminal(ctx, options, toolCallID, ExecutionStatusNoEffect)
			return fantasy.NewTextErrorResponse(
				fmt.Sprintf("timeout must be positive, got %q", *args.Timeout),
			), nil
		}
		timeout = parsed
	}
	timeout = min(timeout, maxExecuteTimeout)

	// Build the environment map for the process request.
	env := make(map[string]string, len(nonInteractiveEnvVars)+1)
	env["CODER_CHAT_AGENT"] = "true"
	for k, v := range nonInteractiveEnvVars {
		env[k] = v
	}

	var workDir string
	if args.WorkDir != nil {
		workDir = *args.WorkDir
	}

	var rec ExecutionRecord
	if options.Recorder != nil {
		var claimed bool
		var err error
		rec, claimed, err = options.Recorder.Claim(ctx, toolCallID, HashToolInput(call.Input), args.Command, background, timeout)
		if err != nil {
			if xerrors.Is(err, ErrExecutionInputMismatch) {
				options.Logger.Warn(ctx, "execute claim targeted stale lineage",
					slog.F("tool_call_id", toolCallID),
					slog.Error(err),
				)
				return fantasy.NewTextErrorResponse(
					"the recorded execution for this tool call was created for different input; refusing to run against stale lineage. Retry the request.",
				), nil
			}
			// An infrastructure failure before dispatch must not
			// commit a bogus result: abort the batch so the task
			// retry re-claims the intent.
			return fantasy.ToolResponse{}, &AbortToolExecutionError{Err: xerrors.Errorf("claim execution record: %w", err)}
		}
		if !claimed {
			return resumeExecution(ctx, conn, options, toolCallID, rec)
		}
	}

	if background {
		return executeBackground(ctx, conn, options, toolCallID, rec, args.Command, workDir, env), nil
	}
	return executeForeground(ctx, conn, options, toolCallID, rec, args.Command, timeout, workDir, env), nil
}

// resumeExecution handles a tool call whose ledger row is owned by
// another claim or already carries a lifecycle outcome. It never
// dispatches a fresh process.
func resumeExecution(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	options ExecuteOptions,
	toolCallID string,
	rec ExecutionRecord,
) (fantasy.ToolResponse, error) {
	switch rec.Status {
	case ExecutionStatusStarting:
		// Another claim owns dispatch. Give it until the claim
		// goes stale to record the process handle, then declare
		// the process state unknown. Dispatching here could run
		// the command twice.
		latest, err := awaitRecordedProcess(ctx, options, toolCallID, rec)
		if err != nil {
			// Cancellation is an expected exit for this attempt,
			// not evidence about the claim owner: marking unknown
			// here could race ahead of the owner's RecordStart and
			// orphan a real process. Only a claim that actually
			// went stale is unobservable.
			if ctx.Err() != nil {
				return errorResult(fmt.Sprintf("wait for execution claim owner: %v", ctx.Err())), nil
			}
			fresh, applied, markErr := options.Recorder.MarkStaleClaimUnknown(ctx, toolCallID)
			if markErr != nil {
				// Committing unknown on an unverified row could end
				// recovery of a real process; abort and retry.
				return fantasy.ToolResponse{}, &AbortToolExecutionError{Err: xerrors.Errorf("mark stale execution claim unknown: %w", markErr)}
			}
			if !applied {
				// The owner recorded its process after the last
				// poll; resume from the fresh row instead of
				// downgrading it.
				return resumeExecution(ctx, conn, options, toolCallID, fresh)
			}
			return fantasy.NewTextErrorResponse(unknownOutcomeMessage), nil
		}
		return resumeExecution(ctx, conn, options, toolCallID, latest)
	case ExecutionStatusRunning, ExecutionStatusExited, ExecutionStatusDetached:
		return reattachProcess(ctx, conn, options, toolCallID, rec)
	default:
		resp, ok := terminalRowResponse(ctx, options, toolCallID, rec)
		if !ok {
			// Reserved rows are always claimable, so resume
			// should never see one.
			return errorResult(fmt.Sprintf("execution row in unexpected state %s; retry the command", rec.Status)), nil
		}
		return resp, nil
	}
}

// terminalRowResponse returns the stable response for a ledger row
// whose status forbids both dispatch and re-attach. ok is false for
// statuses that need a workspace connection to make progress.
func terminalRowResponse(ctx context.Context, options ExecuteOptions, toolCallID string, rec ExecutionRecord) (fantasy.ToolResponse, bool) {
	switch rec.Status {
	case ExecutionStatusUnknown:
		return fantasy.NewTextErrorResponse(unknownOutcomeMessage), true
	case ExecutionStatusCanceled, ExecutionStatusCancelRequested, ExecutionStatusNoEffect:
		// The ledger resolved this call but history has no result
		// for it, so the chat is re-executing a call the ledger
		// considers finished. Never dispatch on top of a resolved
		// row.
		options.Logger.Warn(ctx, "execution ledger row already resolved for unresolved tool call",
			slog.F("tool_call_id", toolCallID),
			slog.F("status", string(rec.Status)),
		)
		return fantasy.NewTextErrorResponse(fmt.Sprintf(
			"the recorded execution for this command was already resolved (%s) and will not be restarted. Re-run the command only if it is safe to run twice.",
			rec.Status,
		)), true
	default:
		return fantasy.ToolResponse{}, false
	}
}

// resolveConnFailure resolves a tool call when the workspace
// connection is unavailable. Rows the ledger already resolved and
// background rows whose handle is the entire result return their
// stable response. Rows that may carry a dispatched process abort
// the batch: committing a result would set result_committed_at and
// permanently end re-attachment. Only rows proving nothing was
// dispatched surface the dial error as a normal tool result.
func resolveConnFailure(ctx context.Context, options ExecuteOptions, toolCallID string, dialErr error) (fantasy.ToolResponse, error) {
	if options.Recorder == nil {
		return fantasy.NewTextErrorResponse(dialErr.Error()), nil
	}
	rec, err := options.Recorder.Get(ctx, toolCallID)
	if err != nil {
		if xerrors.Is(err, ErrExecutionRecordNotFound) {
			// The call predates the ledger; nothing the ledger
			// dispatched needs this connection.
			return fantasy.NewTextErrorResponse(dialErr.Error()), nil
		}
		return fantasy.ToolResponse{}, &AbortToolExecutionError{Err: xerrors.Errorf("read execution record with workspace connection unavailable: %w", err)}
	}
	if rec.Background && rec.ProcessID != "" {
		switch rec.Status {
		case ExecutionStatusRunning, ExecutionStatusExited, ExecutionStatusDetached:
			return resolveBackgroundRow(ctx, options, toolCallID, rec), nil
		}
	}
	if resp, ok := terminalRowResponse(ctx, options, toolCallID, rec); ok {
		return resp, nil
	}
	if rec.Status == ExecutionStatusStarting && staleStartingClaim(rec, claimStaleAfter) {
		// Stale-claim resolution needs no agent access: the
		// connected path would also resolve this row to unknown.
		// Aborting instead would wedge the chat for as long as
		// the agent stays unreachable.
		fresh, applied, markErr := options.Recorder.MarkStaleClaimUnknown(ctx, toolCallID)
		if markErr != nil {
			return fantasy.ToolResponse{}, &AbortToolExecutionError{Err: xerrors.Errorf("mark stale execution claim unknown: %w", markErr)}
		}
		if applied {
			return fantasy.NewTextErrorResponse(unknownOutcomeMessage), nil
		}
		if fresh.Background && fresh.ProcessID != "" {
			return resolveBackgroundRow(ctx, options, toolCallID, fresh), nil
		}
		if resp, ok := terminalRowResponse(ctx, options, toolCallID, fresh); ok {
			return resp, nil
		}
		// The owner recorded a foreground process concurrently;
		// resuming it needs the agent connection.
	}
	switch rec.Status {
	case ExecutionStatusStarting, ExecutionStatusRunning, ExecutionStatusExited, ExecutionStatusDetached:
		return fantasy.ToolResponse{}, &AbortToolExecutionError{Err: xerrors.Errorf("workspace connection unavailable while execution needs re-attach (status %s): %w", rec.Status, dialErr)}
	}
	// A reserved row proves nothing was dispatched.
	return fantasy.NewTextErrorResponse(dialErr.Error()), nil
}

// staleStartingClaim reports whether a starting claim's owner has
// had at least `after` since the claim to record a process handle.
func staleStartingClaim(rec ExecutionRecord, after time.Duration) bool {
	return !rec.ClaimedAt.IsZero() && !time.Now().Before(rec.ClaimedAt.Add(after))
}

// resolveBackgroundRow returns the durable result for a background
// execute whose process was already dispatched: the handle itself.
// Output retrieval stays with process_output, so no agent access is
// needed.
func resolveBackgroundRow(ctx context.Context, options ExecuteOptions, toolCallID string, rec ExecutionRecord) fantasy.ToolResponse {
	if rec.Status != ExecutionStatusDetached {
		markTerminal(ctx, options, toolCallID, ExecutionStatusDetached)
	}
	return marshalResult(ExecuteResult{
		Success:             true,
		BackgroundProcessID: rec.ProcessID,
	})
}

// awaitRecordedProcess polls a starting claim until its owner
// records a process handle or otherwise advances the row, the claim
// goes stale, or the context is canceled. It returns an error when
// the row was still starting at the staleness deadline.
func awaitRecordedProcess(
	ctx context.Context,
	options ExecuteOptions,
	toolCallID string,
	rec ExecutionRecord,
) (ExecutionRecord, error) {
	deadline := rec.ClaimedAt.Add(claimStaleAfter)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ExecutionRecord{}, ctx.Err()
		case <-time.After(claimPollInterval):
		}
		latest, err := options.Recorder.Get(ctx, toolCallID)
		if err != nil {
			continue
		}
		if latest.Status != ExecutionStatusStarting {
			return latest, nil
		}
	}
	return ExecutionRecord{}, xerrors.New("claim went stale without a recorded process")
}

// reattachProcess resumes an execute tool call whose process was
// started by a previous attempt, without starting a second
// process.
func reattachProcess(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	options ExecuteOptions,
	toolCallID string,
	rec ExecutionRecord,
) (fantasy.ToolResponse, error) {
	if rec.Background {
		return resolveBackgroundRow(ctx, options, toolCallID, rec), nil
	}

	snapCtx, cancel := context.WithTimeout(ctx, snapshotTimeout)
	resp, err := conn.ProcessOutput(snapCtx, rec.ProcessID, nil)
	cancel()
	if err != nil {
		// Only a definite 404 (the agent was reached and does not
		// know the process) means the result is gone, and only
		// when the connection targets the agent that owns the
		// process. Transport errors, cancellations, and server
		// errors leave the process potentially retrievable.
		var sdkErr *codersdk.Error
		if xerrors.As(err, &sdkErr) && sdkErr.StatusCode() == http.StatusNotFound {
			if rec.WorkspaceAgentID == uuid.Nil || rec.WorkspaceAgentID == options.connAgentID {
				markTerminal(ctx, options, toolCallID, ExecutionStatusUnknown)
				return processLostResponse(rec.ProcessID), nil
			}
			// A chat rebound to a different agent asked the wrong
			// agent, which knows nothing about the process. Abort
			// instead of committing a result that would end
			// re-attachment without ever asking the owning agent.
			return fantasy.ToolResponse{}, &AbortToolExecutionError{Err: xerrors.Errorf(
				"process %s belongs to agent %s but the workspace connection targets agent %s: %w",
				rec.ProcessID, rec.WorkspaceAgentID, options.connAgentID, err,
			)}
		}
		return errorResultWithProcess(
			fmt.Sprintf("re-attach to process: %v; use process_output with ID %s to retry", err, rec.ProcessID),
			rec.ProcessID,
		), nil
	}

	if !resp.Running {
		// The process finished while no attempt was watching.
		// Return the real result even if the deadline passed.
		markTerminal(ctx, options, toolCallID, ExecutionStatusExited)
		exitCode := 0
		if resp.ExitCode != nil {
			exitCode = *resp.ExitCode
		}
		result := ExecuteResult{
			Success:   exitCode == 0,
			Output:    truncateOutput(resp.Output),
			ExitCode:  exitCode,
			Truncated: resp.Truncated,
		}
		if !rec.StartedAt.IsZero() {
			result.WallDurationMs = time.Since(rec.StartedAt).Milliseconds()
		}
		return marshalResult(result), nil
	}

	deadline := rec.StartedAt.Add(rec.Timeout)
	if !time.Now().Before(deadline) {
		markTerminal(ctx, options, toolCallID, ExecutionStatusDetached)
		return marshalResult(ExecuteResult{
			Success:             false,
			Output:              truncateOutput(resp.Output),
			ExitCode:            -1,
			Error:               fmt.Sprintf("command timed out after %s", rec.Timeout),
			Truncated:           resp.Truncated,
			BackgroundProcessID: rec.ProcessID,
		}), nil
	}

	cmdCtx, cancelWait := context.WithDeadline(ctx, deadline)
	defer cancelWait()
	result, lost := waitForProcess(cmdCtx, ctx, conn, rec.ProcessID, rec.Timeout)
	if lost {
		markTerminal(ctx, options, toolCallID, ExecutionStatusUnknown)
		return processLostResponse(rec.ProcessID), nil
	}
	result.WallDurationMs = time.Since(rec.StartedAt).Milliseconds()
	markWaitOutcome(ctx, options, toolCallID, result)
	return marshalResult(result), nil
}

// processLostResponse is the stable result for a process the agent
// was reached about but no longer knows: the command may have run,
// but its outcome is unobservable.
func processLostResponse(processID string) fantasy.ToolResponse {
	return fantasy.NewTextErrorResponse(fmt.Sprintf(
		"process %s is no longer known to the workspace agent; the command may have run, but its result was lost and the outcome is unknown. Re-run the command only if it is safe to run twice.",
		processID,
	))
}

// executeBackground starts a process in the background and
// returns immediately with the process ID.
func executeBackground(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	options ExecuteOptions,
	toolCallID string,
	rec ExecutionRecord,
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
		// The request may or may not have reached the agent, so
		// the honest lifecycle outcome is unobservable.
		markTerminal(ctx, options, toolCallID, ExecutionStatusUnknown)
		return errorResult(enrichStartError(fmt.Sprintf("start background process: %v", err)))
	}
	// Mark detached only when the handle write landed: a detached
	// row with no recorded process would make a retry return
	// success without a usable handle instead of re-resolving the
	// dispatch through the still-starting row.
	if recordProcessStart(ctx, options, toolCallID, rec.ClaimEpoch, resp.ID) {
		markTerminal(ctx, options, toolCallID, ExecutionStatusDetached)
	}

	return marshalResult(ExecuteResult{
		Success:             true,
		BackgroundProcessID: resp.ID,
	})
}

// recordProcessStart persists a freshly started process identity on
// an uncanceled, bounded context. The generation context can be
// canceled by an interrupt right after StartProcess returns, and
// the interrupt path needs the recorded handle to kill the process.
// Failures are logged, never fatal: the process is already running
// and attached, so discarding the wait would throw away real work.
func recordProcessStart(ctx context.Context, options ExecuteOptions, toolCallID string, claimEpoch int64, processID string) bool {
	if options.Recorder == nil {
		return false
	}
	recordCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), recordWriteTimeout)
	defer cancel()
	if err := options.Recorder.RecordStart(recordCtx, toolCallID, claimEpoch, processID, options.connAgentID); err != nil {
		options.Logger.Warn(ctx, "failed to record execute process start",
			slog.F("tool_call_id", toolCallID),
			slog.F("process_id", processID),
			slog.Error(err),
		)
		return false
	}
	return true
}

// markTerminal records a lifecycle observation on an uncanceled,
// bounded context so observations survive an interrupt canceling
// the generation context. Best-effort: a failed write only costs
// diagnostic fidelity, never correctness of the returned result.
func markTerminal(ctx context.Context, options ExecuteOptions, toolCallID string, status ExecutionStatus) {
	if options.Recorder == nil {
		return
	}
	recordCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), recordWriteTimeout)
	defer cancel()
	if err := options.Recorder.MarkTerminal(recordCtx, toolCallID, status); err != nil {
		options.Logger.Warn(ctx, "failed to mark execution lifecycle status",
			slog.F("tool_call_id", toolCallID),
			slog.F("status", string(status)),
			slog.Error(err),
		)
	}
}

// markWaitOutcome records the lifecycle observation matching a
// finished foreground wait: results that hand the process handle
// back to the model leave the process alive (detached), everything
// else observed a real exit. A wait cut short by the generation
// context being canceled (an interrupt) records nothing: the row's
// current state is still the truth, and the interrupt commit owns
// the transition to cancel_requested.
func markWaitOutcome(ctx context.Context, options ExecuteOptions, toolCallID string, result ExecuteResult) {
	if ctx.Err() != nil {
		return
	}
	if result.BackgroundProcessID != "" {
		markTerminal(ctx, options, toolCallID, ExecutionStatusDetached)
		return
	}
	markTerminal(ctx, options, toolCallID, ExecutionStatusExited)
}

// executeForeground starts a process and waits for its
// completion, enforcing the configured timeout.
func executeForeground(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	options ExecuteOptions,
	toolCallID string,
	rec ExecutionRecord,
	command string,
	timeout time.Duration,
	workDir string,
	env map[string]string,
) fantasy.ToolResponse {
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()

	resp, err := conn.StartProcess(cmdCtx, workspacesdk.StartProcessRequest{
		Command:    command,
		WorkDir:    workDir,
		Env:        env,
		Background: false,
	})
	if err != nil {
		// The request may or may not have reached the agent, so
		// the honest lifecycle outcome is unobservable.
		markTerminal(ctx, options, toolCallID, ExecutionStatusUnknown)
		return errorResult(enrichStartError(fmt.Sprintf("start process: %v", err)))
	}
	recorded := recordProcessStart(ctx, options, toolCallID, rec.ClaimEpoch, resp.ID)

	result, lost := waitForProcess(cmdCtx, ctx, conn, resp.ID, timeout)
	if lost {
		markTerminal(ctx, options, toolCallID, ExecutionStatusUnknown)
		return processLostResponse(resp.ID)
	}
	result.WallDurationMs = time.Since(start).Milliseconds()
	// Record the wait outcome only when the handle write landed:
	// an exited or detached row without a recorded process would
	// strand a retry with nothing to re-attach to, while a
	// still-starting row resolves through claim recovery.
	if recorded {
		markWaitOutcome(ctx, options, toolCallID, result)
	}

	// Add an advisory note for file-dump commands.
	if note := detectFileDump(command); note != "" {
		result.Note = note
	}

	return marshalResult(result)
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
// timeout by up to snapshotTimeout if recovery is needed. lost
// reports that the agent was reached and definitively does not
// know the process, so the result is gone and the returned
// ExecuteResult carries no re-attachable handle.
func waitForProcess(
	ctx context.Context,
	parentCtx context.Context,
	conn workspacesdk.AgentConn,
	processID string,
	timeout time.Duration,
) (result ExecuteResult, lost bool) {
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
			var sdkErr *codersdk.Error
			if xerrors.As(err, &sdkErr) && sdkErr.StatusCode() == http.StatusNotFound {
				return ExecuteResult{Success: false, ExitCode: -1}, true
			}
			errMsg := fmt.Sprintf("get process output: %v; use process_output with ID %s to retry", origErr, processID)
			if timedOut {
				errMsg = fmt.Sprintf("command timed out after %s; failed to get output: %v", timeout, err)
			}
			return ExecuteResult{
				Success:             false,
				ExitCode:            -1,
				Error:               errMsg,
				BackgroundProcessID: processID,
			}, false
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
			}, false
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
		}, false
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
		}, false
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
	}, false
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

// errorResultWithProcess is errorResult with a process handle the
// model can use to re-attach via process_output.
func errorResultWithProcess(msg string, processID string) fantasy.ToolResponse {
	data, err := json.Marshal(ExecuteResult{
		Success:             false,
		Error:               msg,
		BackgroundProcessID: processID,
	})
	if err != nil {
		return fantasy.NewTextErrorResponse(msg)
	}
	return fantasy.NewTextResponse(string(data))
}

func marshalResult(result ExecuteResult) fantasy.ToolResponse {
	data, err := json.Marshal(result)
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error())
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
	WaitTimeout *string `json:"wait_timeout,omitempty" description:"Override the default 10s block duration, up to 4h (longer values are clamped). The call blocks until the process exits or this timeout is reached. Set to '0s' for an immediate snapshot without waiting."`
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
			timeout = min(timeout, maxExecuteTimeout)
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

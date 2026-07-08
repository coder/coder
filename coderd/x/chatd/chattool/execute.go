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
	// this window is presumed dead; recovery asks the agent
	// whether the dead claimer's dispatch reached it.
	claimStaleAfter = 60 * time.Second

	// claimPollInterval is how often a fresh starting claim is
	// re-read while waiting for its owner to record the process
	// handle.
	claimPollInterval = 2 * time.Second

	// processWaitRetryDelay is the pause before re-issuing a
	// blocking process wait whose server-side bound returned
	// early, so a short agent-side wait cannot degenerate into a
	// zero-delay request loop.
	processWaitRetryDelay = time.Second

	// TokenTrustWindow bounds how long after a claim a "token not
	// found" probe answer proves the dispatch never reached the
	// agent. Agents reap token entries together with exited
	// processes after 60 minutes (exitedProcessReapAge), so a
	// late answer proves nothing. Kept well under the reap age.
	TokenTrustWindow = 30 * time.Minute
)

// TrustAbsentToken reports whether a Found=false, Pending=false
// token probe answer proves the dispatch never reached the agent.
// It requires the claim to be within TokenTrustWindow (the token
// was not reaped) and the agent's in-memory token index to predate
// the claim: a restarted agent answers with an empty index, so a
// young index proves nothing about what an earlier agent process
// may have started. Both sides of the index comparison are
// durations, so agent and coderd wall clocks never mix. A negative
// claim age means the claim was written by a replica whose clock
// is ahead of this one; the index comparison is then meaningless,
// so the answer is not trusted.
func TrustAbsentToken(resp workspacesdk.ProcessByTokenResponse, claimedAt time.Time) bool {
	claimAge := time.Since(claimedAt)
	if claimAge < 0 || claimAge >= TokenTrustWindow {
		return false
	}
	return time.Duration(resp.TokenIndexAgeMS)*time.Millisecond >= claimAge
}

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

// startFailureResponse resolves a StartProcess failure after the
// ledger claim. A structured agent response other than 409 means
// the spawn failed and the agent released the token reservation,
// so nothing is running: the row resolves no_effect and the error
// commits as a normal tool result the model can act on. Transport
// errors leave the dispatch unobservable and a 409 means a process
// may exist under this token (a mismatched reuse, or an aborted
// wait on a reservation whose owner may still publish), so the row
// stays starting and the attempt aborts uncommitted: the token is
// durable, and the retried call resolves the dispatch through the
// agent's token index instead of committing a result that would
// end same-token recovery.
func startFailureResponse(ctx context.Context, options ExecuteOptions, toolCallID string, action string, err error) (fantasy.ToolResponse, error) {
	var sdkErr *codersdk.Error
	structured := xerrors.As(err, &sdkErr) && sdkErr.StatusCode() != http.StatusConflict
	// Without a ledger there is no token to recover through, so
	// aborting buys nothing over the model-facing error.
	if structured || options.Recorder == nil {
		markTerminal(ctx, options, toolCallID, ExecutionStatusNoEffect)
		return errorResult(enrichStartError(fmt.Sprintf("%s: %v", action, err))), nil
	}
	return fantasy.ToolResponse{}, &AbortToolExecutionError{Err: xerrors.Errorf("%s: %w", action, err)}
}

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

// SparedBackgroundCancellationResult is the tool result committed
// for a background execute whose process a cancellation transition
// (interrupt, message edit, new message) deliberately leaves
// running. The result is committed permanently, so it must carry the
// process handle: a generic cancellation would strand the live
// process without an addressable ID.
func SparedBackgroundCancellationResult(processID string) (json.RawMessage, error) {
	payload, err := json.Marshal(ExecuteResult{
		Success:             true,
		BackgroundProcessID: processID,
		Note:                "the run was canceled; the background process was left alone. Use process_output with this ID to check on it.",
	})
	if err != nil {
		return nil, xerrors.Errorf("marshal spared background result: %w", err)
	}
	return payload, nil
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
// same persisted bytes, so a mismatch proves stale lineage. Callers
// must pass the persisted bytes verbatim: any re-encoding (JSON
// re-marshaling, key reordering, whitespace changes) would fail
// every replayed claim closed with ErrExecutionInputMismatch.
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
	Background bool
	Timeout    time.Duration
	// ClaimEpoch guards process-identity writes; it advances on
	// every claim takeover.
	ClaimEpoch int64
	ClaimedAt  time.Time
	StartedAt  time.Time
	// WorkspaceAgentID is the dispatch target recorded at claim
	// time, before the dispatch happens. A chat rebound to a
	// different agent cannot observe what the original target did
	// with the dispatch, so recovery only trusts the token index
	// when it probes this exact agent, and re-attach treats a 404
	// as loss only from this agent.
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
	// A starting row claimed before staleBefore may be taken over
	// (advancing the claim epoch); the zero time only accepts
	// reserved or missing rows. agentID records the dispatch
	// target on the claim so recovery can match probes against
	// it. Returns an error wrapping ErrExecutionInputMismatch
	// when the row was created for different input.
	Claim(ctx context.Context, toolCallID string, inputSHA256 string, command string, background bool, timeout time.Duration, agentID uuid.UUID, staleBefore time.Time) (rec ExecutionRecord, claimed bool, err error)
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
	// startedAt anchors re-attach deadlines; adoption of an
	// already-running process passes the claim time as a lower
	// bound of the true start instead of the adoption time.
	RecordStart(ctx context.Context, toolCallID string, claimEpoch int64, processID string, agentID uuid.UUID, startedAt time.Time) error
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
	// DialAgent dials a specific workspace agent so re-attach can
	// reach a recorded process whose agent is no longer the one
	// behind GetWorkspaceConn. When nil, such re-attaches abort
	// the batch instead of probing the wrong agent.
	DialAgent AgentConnFunc
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
		"Execute a shell command in the workspace. Runs under \"sh -c\" (POSIX). Waits for completion up to the timeout (default 10s, override with the timeout parameter e.g. '30s', '5m'; maximum 4h, longer values are clamped). If the command exceeds the timeout, the response includes a background_process_id; use process_output with that ID to re-attach and wait for the result. Use run_in_background=true for persistent processes (dev servers, file watchers) or when you want to continue other work while the command runs. Never use shell '&' for backgrounding.",
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
	if options.Recorder != nil && toolCallID == "" {
		// The ledger keys rows by tool call ID; an ID-less call
		// cannot be tracked and must not share the empty-ID row
		// with another such call.
		return fantasy.NewTextErrorResponse("tool call has no ID; refusing to execute untracked command"), nil
	}
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

	dispatch := dispatchInputs{
		inputSHA256: HashToolInput(call.Input),
		command:     args.Command,
		background:  background,
		timeout:     timeout,
		workDir:     workDir,
		env:         env,
	}

	var rec ExecutionRecord
	if options.Recorder != nil {
		// Background executions have no completion deadline, so
		// their rows record a zero timeout instead of a value
		// re-attach would never read.
		claimTimeout := timeout
		if background {
			claimTimeout = 0
		}
		var claimed bool
		var err error
		rec, claimed, err = options.Recorder.Claim(ctx, toolCallID, dispatch.inputSHA256, args.Command, background, claimTimeout, options.connAgentID, time.Time{})
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
			return resumeExecution(ctx, conn, options, toolCallID, rec, dispatch)
		}
	}

	if background {
		return executeBackground(ctx, conn, options, toolCallID, rec, args.Command, workDir, env)
	}
	return executeForeground(ctx, conn, options, toolCallID, rec, args.Command, timeout, workDir, env)
}

// dispatchInputs carries the validated execute arguments so
// recovery paths can re-dispatch the same request under the same
// execution token.
type dispatchInputs struct {
	inputSHA256 string
	command     string
	background  bool
	timeout     time.Duration
	workDir     string
	env         map[string]string
}

// logStartIdempotency records whether the agent honored the
// idempotency token (the execution ID) sent with a StartProcess
// request. A missing echo means the agent predates idempotent
// starts, so only the durable execution ledger protects against
// duplicate processes.
func logStartIdempotency(ctx context.Context, logger slog.Logger, resp workspacesdk.StartProcessResponse, toolCallID string) {
	if resp.ClientToken == "" {
		logger.Warn(ctx, "workspace agent does not support idempotent process starts",
			slog.F("tool_call_id", toolCallID),
			slog.F("process_id", resp.ID),
		)
		return
	}
	if resp.Attached {
		logger.Info(ctx, "execute_agent_deduped",
			slog.F("tool_call_id", toolCallID),
			slog.F("process_id", resp.ID),
		)
	}
}

// resumeExecution handles a tool call whose ledger row is owned by
// another claim or already carries a lifecycle outcome. It only
// dispatches a process through the stale-claim recovery, which
// proves via the agent's token index that the original dispatch
// never happened.
func resumeExecution(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	options ExecuteOptions,
	toolCallID string,
	rec ExecutionRecord,
	dispatch dispatchInputs,
) (fantasy.ToolResponse, error) {
	switch rec.Status {
	case ExecutionStatusStarting:
		// Another claim owns dispatch. Give it until the claim
		// goes stale to record the process handle, then ask the
		// agent what became of the dispatch. Dispatching here
		// without that proof could run the command twice.
		latest, err := awaitRecordedProcess(ctx, options, toolCallID, rec)
		if err != nil {
			// Cancellation is an expected exit for this attempt,
			// not evidence about the claim owner: acting on it
			// could race ahead of the owner's RecordStart.
			if ctx.Err() != nil {
				return errorResult(fmt.Sprintf("wait for execution claim owner: %v", ctx.Err())), nil
			}
			return recoverStaleClaim(ctx, conn, options, toolCallID, rec, dispatch)
		}
		return resumeExecution(ctx, conn, options, toolCallID, latest, dispatch)
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
	if rec.Status == ExecutionStatusStarting && staleStartingClaim(rec, TokenTrustWindow) {
		// Within TokenTrustWindow the abort below is preferred:
		// once the agent is reachable again, the token probe can
		// still adopt a live process or prove nothing was
		// dispatched. Past the window absence answers prove
		// nothing anyway, so the guarded unknown write (which
		// needs no agent access) resolves the row instead of
		// wedging the chat for as long as the agent stays
		// unreachable.
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
			// A failed poll is not evidence about the claim
			// owner; keep polling, but leave a trace so a DB
			// outage here is distinguishable from a claimer
			// that really never recorded its process.
			options.Logger.Warn(ctx, "failed to poll execution claim during grace window",
				slog.F("tool_call_id", toolCallID),
				slog.Error(err),
			)
			continue
		}
		if latest.Status != ExecutionStatusStarting {
			return latest, nil
		}
	}
	return ExecutionRecord{}, xerrors.New("claim went stale without a recorded process")
}

// resolveStaleClaimUnknown resolves a stale starting claim to
// unknown through the guarded transition: the claim owner's
// RecordStart can land concurrently with the staleness verdict, and
// that write wins, with the attempt resuming the recorded process
// instead of downgrading it. A write failure aborts uncommitted,
// because committing unknown on an unverified row could end
// recovery of a real process.
func resolveStaleClaimUnknown(ctx context.Context, conn workspacesdk.AgentConn, options ExecuteOptions, toolCallID string, dispatch dispatchInputs) (fantasy.ToolResponse, error) {
	fresh, applied, err := options.Recorder.MarkStaleClaimUnknown(ctx, toolCallID)
	if err != nil {
		return fantasy.ToolResponse{}, &AbortToolExecutionError{Err: xerrors.Errorf("mark stale execution claim unknown: %w", err)}
	}
	if !applied {
		return resumeExecution(ctx, conn, options, toolCallID, fresh, dispatch)
	}
	return fantasy.NewTextErrorResponse(unknownOutcomeMessage), nil
}

// recoverStaleClaim resolves a starting claim whose owner never
// recorded a process handle. The execution ID doubles as the
// agent-side idempotency token, so the agent's token index reveals
// what became of the dead claimer's dispatch. Recovery never mints
// a new token for the tool call: a found process is adopted, and
// re-dispatch reuses the same token so the agent dedups any race
// with the original dispatch.
func recoverStaleClaim(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	options ExecuteOptions,
	toolCallID string,
	rec ExecutionRecord,
	dispatch dispatchInputs,
) (fantasy.ToolResponse, error) {
	if rec.WorkspaceAgentID == uuid.Nil || rec.WorkspaceAgentID != options.connAgentID {
		// The stale claim dispatched to a different agent (the
		// workspace was rebuilt or the chat rebound). This
		// connection's token index cannot observe what that agent
		// did with the dispatch, so neither adoption nor
		// re-dispatch is safe.
		return resolveStaleClaimUnknown(ctx, conn, options, toolCallID, dispatch)
	}
	probeCtx, cancel := context.WithTimeout(ctx, snapshotTimeout)
	resp, err := workspacesdk.ProbeProcessToken(probeCtx, conn, rec.ID)
	cancel()
	if err != nil {
		if isNotFoundError(err) || xerrors.Is(err, workspacesdk.ErrProcessTokenProbeUnsupported) {
			// The agent predates the token probe endpoint or the
			// connection cannot probe, so the dead claimer's
			// dispatch cannot be verified.
			return resolveStaleClaimUnknown(ctx, conn, options, toolCallID, dispatch)
		}
		// Transport errors prove nothing. Abort uncommitted so
		// the retried call probes the same token again; a
		// committed result would end same-token recovery.
		return fantasy.ToolResponse{}, &AbortToolExecutionError{Err: xerrors.Errorf("probe execution token: %w", err)}
	}

	if resp.Found {
		// Adopt the process on the stale claim's epoch, then
		// resume from the updated row. The claim time is the
		// lower bound of the true start, so an adopted process
		// never wins a fresh full timeout.
		recorded := recordProcessStart(ctx, options, toolCallID, rec.ClaimEpoch, resp.ProcessID, rec.ClaimedAt)
		latest, err := options.Recorder.Get(ctx, toolCallID)
		if err == nil && latest.Status != ExecutionStatusStarting {
			return resumeExecution(ctx, conn, options, toolCallID, latest, dispatch)
		}
		if recorded {
			// The write landed but the read-back failed or
			// lagged. Re-attach with the known handle, anchoring
			// the deadline at the claim time as a lower bound of
			// the true start.
			rec.ProcessID = resp.ProcessID
			rec.StartedAt = rec.ClaimedAt
			return reattachProcess(ctx, conn, options, toolCallID, rec)
		}
		// The adoption write did not land and the row is still
		// starting: re-attaching now could terminalize a row
		// that carries no process handle, stranding a later
		// retry. Abort uncommitted so the retried call probes
		// the durable token index and adopts again.
		return fantasy.ToolResponse{}, &AbortToolExecutionError{Err: xerrors.Errorf(
			"found process %s for a previous attempt but failed to record it", resp.ProcessID,
		)}
	}

	if !resp.Pending && !TrustAbsentToken(resp, rec.ClaimedAt) {
		// The agent may have reaped the token with its exited
		// process, or restarted with an empty token index, so
		// absence proves nothing.
		return resolveStaleClaimUnknown(ctx, conn, options, toolCallID, dispatch)
	}

	// A pending reservation or a trustworthy absent token is
	// safe to re-dispatch under the same token: the agent
	// attaches to an in-flight start and dedups any race with
	// the original dispatch. Take over the stale claim first.
	reclaimed, claimed, err := options.Recorder.Claim(ctx, toolCallID, dispatch.inputSHA256, dispatch.command, dispatch.background, dispatch.timeout, options.connAgentID, time.Now().Add(-claimStaleAfter))
	if err != nil {
		// A transient reclaim failure must not commit a result:
		// that would stamp result_committed_at and stop later
		// retries from recovering this token even though the
		// original dispatch may still publish a process. Abort
		// uncommitted like the initial claim path.
		return fantasy.ToolResponse{}, &AbortToolExecutionError{Err: xerrors.Errorf("reclaim stale execution: %w", err)}
	}
	if !claimed {
		return resumeExecution(ctx, conn, options, toolCallID, reclaimed, dispatch)
	}
	// The takeover stamped a fresh claimed_at. Keep the original
	// claim time on the dispatched record: if the agent attaches
	// to the process the stale dispatch started, that time is the
	// lower bound of the true start, and anchoring the deadline
	// there stops the attached process from getting a fresh
	// timeout.
	reclaimed.ClaimedAt = rec.ClaimedAt
	options.Logger.Info(ctx, "re-dispatching execution after stale claim takeover",
		slog.F("tool_call_id", toolCallID),
		slog.F("execution_id", reclaimed.ID),
		slog.F("claim_epoch", reclaimed.ClaimEpoch),
	)
	if dispatch.background {
		return executeBackground(ctx, conn, options, toolCallID, reclaimed, dispatch.command, dispatch.workDir, dispatch.env)
	}
	return executeForeground(ctx, conn, options, toolCallID, reclaimed, dispatch.command, dispatch.timeout, dispatch.workDir, dispatch.env)
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

	// The recorded process only exists on the agent that started
	// it. When the turn's connection targets a different agent,
	// dial the owner directly instead of probing an agent that
	// knows nothing about the process.
	connTargetsOwner := rec.WorkspaceAgentID == uuid.Nil || rec.WorkspaceAgentID == options.connAgentID
	if !connTargetsOwner && options.DialAgent != nil {
		ownerConn, release, dialErr := options.DialAgent(ctx, rec.WorkspaceAgentID)
		if dialErr != nil {
			// Committing a result here would end re-attachment
			// without ever asking the owning agent.
			return fantasy.ToolResponse{}, &AbortToolExecutionError{Err: xerrors.Errorf(
				"dial agent %s that owns process %s: %w",
				rec.WorkspaceAgentID, rec.ProcessID, dialErr,
			)}
		}
		if release != nil {
			defer release()
		}
		conn = ownerConn
		connTargetsOwner = true
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
		if isNotFoundError(err) {
			if connTargetsOwner {
				options.Logger.Warn(ctx, "recorded execute process is no longer known to its agent; resolving execution unknown",
					slog.F("tool_call_id", toolCallID),
					slog.F("process_id", rec.ProcessID),
				)
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
		options.Logger.Warn(ctx, "failed to re-attach to recorded execute process",
			slog.F("tool_call_id", toolCallID),
			slog.F("process_id", rec.ProcessID),
			slog.Error(err),
		)
		return errorResultWithProcess(
			fmt.Sprintf("re-attach to process: %v; use process_output with ID %s to retry", err, rec.ProcessID),
			rec.ProcessID,
		), nil
	}

	if !resp.Running {
		// The process finished while no attempt was watching.
		// Return the real result even if the deadline passed.
		markTerminal(ctx, options, toolCallID, ExecutionStatusExited)
		result := completedResult(resp)
		if !rec.StartedAt.IsZero() {
			result.WallDurationMs = time.Since(rec.StartedAt).Milliseconds()
		}
		return marshalResult(result), nil
	}

	deadline := rec.StartedAt.Add(rec.Timeout)
	if !time.Now().Before(deadline) {
		markTerminal(ctx, options, toolCallID, ExecutionStatusDetached)
		return marshalResult(timedOutRunningResult(resp, rec.Timeout, rec.ProcessID)), nil
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

// isNotFoundError reports a definite HTTP 404 from the agent: the
// agent was reached and does not know the requested resource (or,
// for a route-level 404, predates the endpoint).
func isNotFoundError(err error) bool {
	var sdkErr *codersdk.Error
	return xerrors.As(err, &sdkErr) && sdkErr.StatusCode() == http.StatusNotFound
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
) (fantasy.ToolResponse, error) {
	resp, err := conn.StartProcess(ctx, workspacesdk.StartProcessRequest{
		Command:     command,
		WorkDir:     workDir,
		Env:         env,
		Background:  true,
		ClientToken: rec.ID,
	})
	if err != nil {
		return startFailureResponse(ctx, options, toolCallID, "start background process", err)
	}
	KickAttemptKeepalive(ctx)
	logStartIdempotency(ctx, options.Logger, resp, toolCallID)
	startedAt := time.Now()
	if resp.Attached && !rec.ClaimedAt.IsZero() {
		// An attached process has been running since the earlier
		// dispatch; its claim time is the lower bound of the
		// true start.
		startedAt = rec.ClaimedAt
	}
	// Mark detached only when the handle write landed: a detached
	// row with no recorded process would make a retry return
	// success without a usable handle instead of re-resolving the
	// dispatch through the still-starting row.
	if recordProcessStart(ctx, options, toolCallID, rec.ClaimEpoch, resp.ID, startedAt) {
		markTerminal(ctx, options, toolCallID, ExecutionStatusDetached)
	}

	return marshalResult(ExecuteResult{
		Success:             true,
		BackgroundProcessID: resp.ID,
	}), nil
}

// recordProcessStart persists a freshly started process identity on
// an uncanceled, bounded context. The generation context can be
// canceled by an interrupt right after StartProcess returns, and
// the interrupt path needs the recorded handle to kill the process.
// Failures are logged, never fatal: the process is already running
// and attached, so discarding the wait would throw away real work.
func recordProcessStart(ctx context.Context, options ExecuteOptions, toolCallID string, claimEpoch int64, processID string, startedAt time.Time) bool {
	if options.Recorder == nil {
		return false
	}
	recordCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), recordWriteTimeout)
	defer cancel()
	if err := options.Recorder.RecordStart(recordCtx, toolCallID, claimEpoch, processID, options.connAgentID, startedAt); err != nil {
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
) (fantasy.ToolResponse, error) {
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()

	resp, err := conn.StartProcess(cmdCtx, workspacesdk.StartProcessRequest{
		Command:     command,
		WorkDir:     workDir,
		Env:         env,
		Background:  false,
		ClientToken: rec.ID,
	})
	if err != nil {
		return startFailureResponse(ctx, options, toolCallID, "start process", err)
	}
	KickAttemptKeepalive(ctx)
	logStartIdempotency(ctx, options.Logger, resp, toolCallID)
	startedAt := time.Now()
	if resp.Attached && !rec.ClaimedAt.IsZero() {
		// The agent deduped this start against a process an
		// earlier dispatch of this token created. The claim time
		// is the lower bound of the true start; anchoring the
		// deadline and wall duration there stops the attached
		// process from getting a fresh timeout on every retry.
		startedAt = rec.ClaimedAt
	}
	recorded := recordProcessStart(ctx, options, toolCallID, rec.ClaimEpoch, resp.ID, startedAt)
	if resp.Attached && recorded {
		// Resume through the re-attach flow so an attached
		// process past its original deadline detaches with the
		// graceful timed-out result instead of waiting a full
		// timeout again.
		rec.ProcessID = resp.ID
		rec.StartedAt = startedAt
		rec.Timeout = timeout
		return reattachProcess(ctx, conn, options, toolCallID, rec)
	}

	result, lost := waitForProcess(cmdCtx, ctx, conn, resp.ID, timeout)
	if lost {
		markTerminal(ctx, options, toolCallID, ExecutionStatusUnknown)
		return processLostResponse(resp.ID), nil
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

	return marshalResult(result), nil
}

// timedOutRunningResult builds the ExecuteResult for a process that
// is still running when the caller's timeout expires: partial output
// plus the process handle so the model can re-attach or poll.
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

// completedResult builds the ExecuteResult for a process output
// snapshot, treating a missing exit code as success. Callers
// override the running-process fields when resp.Running is true.
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
	for {
		// Block until the process exits or the context is
		// canceled.
		resp, err := conn.ProcessOutput(ctx, processID, &workspacesdk.ProcessOutputOptions{
			Wait: true,
		})
		if err == nil {
			KickAttemptKeepalive(parentCtx)
		}
		if err == nil && resp.Running && ctx.Err() == nil {
			// The server-side wait can return before the process
			// exits when its maximum wait is shorter than the
			// caller's timeout. Pause briefly, then re-issue the
			// wait with the remaining budget.
			select {
			case <-ctx.Done():
			case <-time.After(processWaitRetryDelay):
			}
			if ctx.Err() == nil {
				continue
			}
		}
		return resolveProcessWait(ctx, parentCtx, conn, processID, timeout, resp, err)
	}
}

// resolveProcessWait turns the final blocking-wait response (or
// error) into the ExecuteResult returned to the model.
func resolveProcessWait(
	ctx context.Context,
	parentCtx context.Context,
	conn workspacesdk.AgentConn,
	processID string,
	timeout time.Duration,
	resp workspacesdk.ProcessOutputResponse,
	err error,
) (result ExecuteResult, lost bool) {
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
			if isNotFoundError(err) {
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

		KickAttemptKeepalive(parentCtx)

		// Snapshot succeeded. If the process finished, return
		// its real result (transparent recovery).
		if !resp.Running {
			return completedResult(resp), false
		}

		// Process still running, return partial output.
		if timedOut {
			return timedOutRunningResult(resp, timeout, processID), false
		}
		return ExecuteResult{
			Success:             false,
			Output:              truncateOutput(resp.Output),
			ExitCode:            -1,
			Error:               fmt.Sprintf("get process output: %v (process still running, use process_output to check later)", origErr),
			Truncated:           resp.Truncated,
			BackgroundProcessID: processID,
		}, false
	}

	if resp.Running {
		// Only reachable once the caller's timeout expired while
		// the process was still running; waitForProcess retries
		// early server-side wait returns itself.
		return timedOutRunningResult(resp, timeout, processID), false
	}

	return completedResult(resp), false
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
			"without waiting; maximum 4h, longer values are "+
			"clamped).",
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
				if parsed < 0 {
					return fantasy.NewTextErrorResponse(
						fmt.Sprintf("wait_timeout must not be negative, got %q; use '0s' for an immediate snapshot", *args.WaitTimeout),
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
			KickAttemptKeepalive(parentCtx)
			result := completedResult(resp)
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

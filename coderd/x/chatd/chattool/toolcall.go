package chattool

import (
	"context"
	"errors"
	"fmt"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/apiversion"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/quartz"
)

// Workspace connection errors that mean no workspace agent exists to
// run a tool call. Their messages are shown to the model.
var (
	ErrChatHasNoWorkspace  = xerrors.New("this tool requires a workspace and this chat does not have one. Use the create_workspace tool to create one")
	ErrWorkspaceHasNoAgent = xerrors.New("workspace has no running agent: the workspace is likely stopped. Use the start_workspace tool to start it")
	ErrWorkspaceDeleted    = xerrors.New("the chat's workspace was deleted (for example by dormancy cleanup) and cannot execute tools. Use the create_workspace tool to create a new one")
)

// capableAgentMajor and capableAgentMinor are the first agent API version
// whose agent records tool calls by their tool call headers.
const (
	capableAgentMajor = 2
	capableAgentMinor = 13
)

// IsCapableAgent reports whether agent handles tool call headers: its
// api_version is at least 2.13. The agent writes api_version on every
// connection before it serves tool requests, and a coderd older than
// 2.13 rejects a 2.13 dial, so a capable agent's cutoff message ID came
// from a coderd that serves it. An unset or malformed version is not
// capable.
func IsCapableAgent(agent database.WorkspaceAgent) bool {
	major, minor, err := apiversion.Parse(agent.APIVersion)
	if err != nil {
		return false
	}
	return major > capableAgentMajor || (major == capableAgentMajor && minor >= capableAgentMinor)
}

// SendsToolCallIdentity reports whether the tool named name sends tool
// call headers to a capable agent, so its dispatch needs a
// ToolCallIdentity.
func SendsToolCallIdentity(name string) bool {
	return name == ExecuteToolName || name == EditFilesToolName || name == WriteFileToolName
}

// ToolCallCause says why a tool call runs outside its first dispatch.
type ToolCallCause string

const (
	// ToolCallCauseInterrupt marks the interrupt run, which cancels the
	// tool call on the agent before sending its request again.
	ToolCallCauseInterrupt ToolCallCause = "interrupt"
)

// ToolCallIdentity identifies the committed tool call a tool runs for.
// The dispatcher attaches it only for a capable agent, so a tool with an
// identity sends tool call headers and one without behaves as it does
// for an older agent.
type ToolCallIdentity struct {
	ChatID uuid.UUID
	// MessageID is the ID of the assistant message that contains the
	// tool call.
	MessageID int64
	// ToolCallID is the provider tool call ID.
	ToolCallID string
	// ToolName is the tool name committed with the tool call.
	ToolName string
	// Cause is empty in generation.
	Cause ToolCallCause
}

// AgentToolCall returns the tool call to attach to workspace agent
// requests with workspacesdk.WithToolCall.
func (id ToolCallIdentity) AgentToolCall() workspacesdk.ToolCall {
	return workspacesdk.ToolCall{
		MessageID: id.MessageID,
		ID:        id.ToolCallID,
		Name:      id.ToolName,
	}
}

// UUID returns the tool call UUID: the ID of a process the tool call
// starts and the ID in the agent's cancel route.
func (id ToolCallIdentity) UUID() string {
	return workspacesdk.ToolCallUUID(id.ChatID, id.MessageID, id.ToolName, id.ToolCallID).String()
}

type toolCallIdentityKey struct{}

// WithToolCallIdentity returns a context carrying id for the tool call
// run with it.
func WithToolCallIdentity(ctx context.Context, id ToolCallIdentity) context.Context {
	return context.WithValue(ctx, toolCallIdentityKey{}, id)
}

// ToolCallIdentityFromContext returns the identity set by
// WithToolCallIdentity. ok is false when the dispatcher attached none.
func ToolCallIdentityFromContext(ctx context.Context) (ToolCallIdentity, bool) {
	id, ok := ctx.Value(toolCallIdentityKey{}).(ToolCallIdentity)
	return id, ok
}

// AgentAnswerTimeout is how long chatd keeps sending one request for a
// tool call to the workspace agent while the agent gives no answer. It
// bounds the wait for an answer, never the work.
const AgentAnswerTimeout = time.Minute

// toolCallRetryInterval is the pause between two sends of a request that
// got no answer.
const toolCallRetryInterval = time.Second

// ErrAgentNoAnswer is wrapped by the error RequestUntilAnswered returns
// when the agent did not answer before its timeout ended.
var ErrAgentNoAnswer = xerrors.New("the workspace agent did not answer")

// noAnswerError is the error of a request the agent did not answer
// within timeout. last is the error of the last send.
type noAnswerError struct {
	timeout time.Duration
	last    error
}

func (e *noAnswerError) Error() string {
	return fmt.Sprintf("%s within %s: %v", ErrAgentNoAnswer, e.timeout, e.last)
}

func (e *noAnswerError) Unwrap() []error {
	return []error{ErrAgentNoAnswer, e.last}
}

// IsAgentAnswer reports whether err, returned by a workspace agent
// request, carries the agent's answer: nil, a coded refusal
// (*workspacesdk.ToolCallError), or another HTTP error answer
// (*codersdk.Error). Any other error is a transport failure or an
// unreadable answer, so the agent may or may not have acted.
func IsAgentAnswer(err error) bool {
	if err == nil {
		return true
	}
	var tcErr *workspacesdk.ToolCallError
	if errors.As(err, &tcErr) {
		return true
	}
	var sdkErr *codersdk.Error
	return errors.As(err, &sdkErr)
}

// RequestUntilAnswered calls send until it returns an agent answer (see
// IsAgentAnswer) and returns that answer. When AgentAnswerTimeout on
// clock ends first, it returns an error wrapping ErrAgentNoAnswer and the
// last error. When ctx ends first, it returns ctx's error. Every call
// must send the same request, so the agent replays one tool call. A nil
// clock means a real clock.
func RequestUntilAnswered(ctx context.Context, clock quartz.Clock, send func(ctx context.Context) error) error {
	return requestUntilAnsweredWithin(ctx, clock, AgentAnswerTimeout, send)
}

func requestUntilAnsweredWithin(ctx context.Context, clock quartz.Clock, timeout time.Duration, send func(ctx context.Context) error) error {
	if clock == nil {
		clock = quartz.NewReal()
	}
	reqCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	timer := clock.AfterFunc(timeout, func() { cancel(ErrAgentNoAnswer) }, "chattool", "agent-answer")
	defer timer.Stop()

	for {
		err := send(reqCtx)
		if IsAgentAnswer(err) {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(context.Cause(reqCtx), ErrAgentNoAnswer) {
			return &noAnswerError{timeout: timeout, last: err}
		}
		retry := clock.NewTimer(toolCallRetryInterval, "chattool", "agent-retry")
		select {
		case <-reqCtx.Done():
			retry.Stop()
		case <-retry.C:
		}
	}
}

// ToolCallWords are the words one tool uses in the results that
// ToolCallErrorText builds.
type ToolCallWords struct {
	// NotRun says the tool call had no effect, as in "command not run".
	NotRun string
	// Effect says what may have happened when the outcome is unknown,
	// as in "the command may have run".
	Effect string
	// Check tells the model how to find out while the agent may still
	// have the tool call's record, as in "Check it with process_output
	// using process ID <UUID>."
	Check string
	// UnknownCheck replaces Check after a tool_call_unknown refusal,
	// when the agent has no record of the tool call.
	UnknownCheck string
}

// UnknownOutcome words a result whose side effect may or may not have
// happened: reason says why chatd cannot tell, effect what may have
// happened, and check how the model can find out.
func UnknownOutcome(reason, effect, check string) string {
	return fmt.Sprintf("outcome unknown: %s, so %s. %s", reason, effect, check)
}

// ToolCallErrorText returns the result text for err, returned by a
// request for a tool call that carried tool call headers. ok is false
// when the tool reports err with its usual text: an HTTP error answer is
// the recorded response of the tool call's one run.
func ToolCallErrorText(err error, words ToolCallWords) (text string, ok bool) {
	var tcErr *workspacesdk.ToolCallError
	switch {
	case errors.As(err, &tcErr) && tcErr.Code == workspacesdk.ToolCallErrorCanceled:
		return fmt.Sprintf("%s: the tool call was canceled before the workspace agent received it.", words.NotRun), true
	case errors.As(err, &tcErr) && tcErr.Code == workspacesdk.ToolCallErrorUnknown:
		return UnknownOutcome("the workspace agent cannot tell whether this tool call ran, because it restarted or discarded its records after the tool call",
			words.Effect, words.UnknownCheck), true
	case IsAgentAnswer(err):
		return "", false
	default:
		return UnknownOutcome(err.Error(), words.Effect, words.Check), true
	}
}

// ConnErrorText returns the result text for err, returned by the
// workspace connection resolver for a tool call with an identity. ok is
// false when no workspace agent exists, so no earlier attempt can have
// acted and the tool reports err as it does without an identity.
func ConnErrorText(err error, words ToolCallWords) (text string, ok bool) {
	if hasNoWorkspaceAgent(err) {
		return "", false
	}
	return UnknownOutcome(fmt.Sprintf("the workspace agent could not be reached (%v)", err), words.Effect, words.Check), true
}

// hasNoWorkspaceAgent reports whether a workspace connection error means
// no workspace agent exists: the chat has no workspace, the workspace was
// deleted, or it has no running agent.
func hasNoWorkspaceAgent(err error) bool {
	return errors.Is(err, ErrChatHasNoWorkspace) ||
		errors.Is(err, ErrWorkspaceDeleted) ||
		errors.Is(err, ErrWorkspaceHasNoAgent)
}

// ToolCallToolsOptions configures NewToolCallTools.
type ToolCallToolsOptions struct {
	GetWorkspaceConn func(context.Context) (workspacesdk.AgentConn, error)
	// ResolvePlanPath and IsPlanTurn configure edit_files and
	// write_file.
	ResolvePlanPath func(context.Context) (chatPath string, home string, err error)
	IsPlanTurn      bool
	// AgentBrowserSession configures execute, see ExecuteOptions.
	AgentBrowserSession string
	// Clock times the retries of requests the agent does not answer.
	// Nil means a real clock.
	Clock quartz.Clock
}

// ToolCallTools are the tools whose workspace side effects the agent
// records per tool call.
type ToolCallTools struct {
	Execute   fantasy.AgentTool
	EditFiles fantasy.AgentTool
	WriteFile fantasy.AgentTool
}

// NewToolCallTools builds the execute, edit_files, and write_file tools.
// Generation and the interrupt run both build them here, so every
// attempt of a tool call sends the agent the same request.
func NewToolCallTools(opts ToolCallToolsOptions) ToolCallTools {
	return ToolCallTools{
		Execute: Execute(ExecuteOptions{
			GetWorkspaceConn:    opts.GetWorkspaceConn,
			AgentBrowserSession: opts.AgentBrowserSession,
			Clock:               opts.Clock,
		}),
		EditFiles: EditFiles(EditFilesOptions{
			GetWorkspaceConn: opts.GetWorkspaceConn,
			ResolvePlanPath:  opts.ResolvePlanPath,
			IsPlanTurn:       opts.IsPlanTurn,
			Clock:            opts.Clock,
		}),
		WriteFile: WriteFile(WriteFileOptions{
			GetWorkspaceConn: opts.GetWorkspaceConn,
			ResolvePlanPath:  opts.ResolvePlanPath,
			IsPlanTurn:       opts.IsPlanTurn,
			Clock:            opts.Clock,
		}),
	}
}

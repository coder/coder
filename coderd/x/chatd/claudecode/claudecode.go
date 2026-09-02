// Package claudecode runs chat generation turns on a Claude Code agent
// over the Agent Client Protocol (ACP). The adapter process runs inside
// the chat's workspace; chatd is the ACP client. One adapter process
// serves one generation turn: conversation continuity between turns
// comes from Claude Code's on-disk session storage in the workspace
// (session/resume, then session/load), with a lossy transcript reseed
// as the fallback when the session is gone (e.g. after a rebuild).
package claudecode

import (
	"bytes"
	"context"
	"encoding/json"
	stdslog "log/slog"
	"strings"
	"sync"
	"time"

	"charm.land/fantasy"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	acp "github.com/coder/acp-go-sdk"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/codersdk"
)

const (
	// cancelResolveTimeout bounds how long a turn waits for the agent
	// to resolve the in-flight prompt with stopReason=canceled after
	// session/cancel is sent.
	cancelResolveTimeout = 10 * time.Second
	// permissionDeniedNote is streamed when the adapter asks for a
	// permission the auto-policy rejects.
	permissionDeniedNote = "Claude Code requested a permission this chat's policy does not grant; the action was declined automatically."
)

// TurnInput configures one generation turn.
type TurnInput struct {
	// SessionID resumes a previous ACP session when non-empty.
	SessionID string
	// SessionCwd is the working directory the resumed session was
	// created with. Resume and load requests use it (falling back to
	// Cwd) because Claude Code keys its session storage by directory.
	SessionCwd string
	// Cwd is the absolute working directory for new sessions.
	Cwd string
	// PromptText is the user message for this turn.
	PromptText string
	// ReseedContext, when non-empty and a brand-new session must be
	// created for a chat that has history, is prepended to the prompt
	// so Claude regains conversation context. It is lossy.
	ReseedContext string
	// PermissionMode selects the adapter session mode (e.g.
	// acceptEdits). Empty keeps the adapter default.
	PermissionMode string
	// Publish streams preview parts into the chat's message part
	// buffer.
	Publish func(codersdk.ChatMessageRole, codersdk.ChatMessagePart)
	Logger  slog.Logger
}

// TurnOutcome is the durable result of one generation turn.
type TurnOutcome struct {
	// SessionID identifies the ACP session that served the turn, for
	// persistence in chats.runtime_state.
	SessionID string
	// Resumed reports whether the previous session was continued
	// (session/resume or session/load) rather than started fresh.
	Resumed bool
	// StopReason is the ACP turn stop reason.
	StopReason acp.StopReason
	// Content is the collected assistant output in the same shape the
	// built-in pipeline persists.
	Content []fantasy.Content
	// Usage is per-turn token usage when the adapter reports it.
	Usage *acp.Usage
}

// RunTurn executes a full prompt turn against a fresh adapter process.
func RunTurn(ctx context.Context, transport Transport, input TurnInput) (TurnOutcome, error) {
	process, err := transport.Start(ctx)
	if err != nil {
		return TurnOutcome{}, xerrors.Errorf("start adapter: %w", err)
	}
	defer func() {
		_ = process.Close()
	}()

	collector := &turnCollector{
		publish: input.Publish,
		logger:  input.Logger,
	}
	conn := acp.NewClientSideConnection(collector, process.Stdin(), process.Stdout())
	// Without an explicit logger the SDK logs through slog.Default(),
	// whose process-wide handler is not guaranteed to be safe for
	// concurrent use (trivy installs a racy deferred handler).
	conn.SetLogger(stdslog.New(stdslog.DiscardHandler))

	// Setup RPCs stay cancelable so a hung adapter cannot outlive an
	// interrupted chat or a shutting-down worker: a canceled ctx
	// aborts them, RunTurn returns, and the deferred Close reaps the
	// process. Only the prompt and its cancel handshake below use an
	// uncancelable context, so an interrupt can still resolve the
	// in-flight prompt with stopReason=canceled per spec.
	initResp, err := conn.Initialize(ctx, acp.InitializeRequest{
		ProtocolVersion:    acp.ProtocolVersionNumber,
		ClientCapabilities: acp.ClientCapabilities{
			// Claude Code executes tools inside the workspace itself;
			// chatd offers no client-side fs or terminal surface.
		},
		ClientInfo: &acp.Implementation{
			Name:    "coder-chatd",
			Version: "1",
		},
	})
	if err != nil {
		return TurnOutcome{}, xerrors.Errorf("initialize: %w", err)
	}

	session, resumed, err := establishSession(ctx, conn, collector, initResp.AgentCapabilities, input)
	if err != nil {
		return TurnOutcome{}, err
	}

	if input.PermissionMode != "" {
		if _, err := conn.SetSessionMode(ctx, acp.SetSessionModeRequest{
			SessionId: session,
			ModeId:    acp.SessionModeId(input.PermissionMode),
		}); err != nil {
			if ctx.Err() != nil {
				return TurnOutcome{}, xerrors.Errorf("set session mode: %w", err)
			}
			// Mode support varies by adapter version; a turn without
			// the requested mode still runs under the auto-deny
			// permission policy.
			input.Logger.Warn(ctx, "set claude code session mode failed",
				slog.F("mode", input.PermissionMode), slog.Error(err))
		}
	}

	promptText := input.PromptText
	if !resumed && input.ReseedContext != "" {
		promptText = input.ReseedContext + "\n\n" + promptText
	}

	// The connection dies with the process; a canceled parent must
	// still let the cancel handshake below run, so the prompt RPCs
	// use an uncancelable context and cancellation is handled
	// explicitly.
	rpcCtx := context.WithoutCancel(ctx)

	type promptResult struct {
		resp acp.PromptResponse
		err  error
	}
	resultCh := make(chan promptResult, 1)
	go func() {
		resp, err := conn.Prompt(rpcCtx, acp.PromptRequest{
			SessionId: session,
			Prompt:    []acp.ContentBlock{acp.TextBlock(promptText)},
		})
		resultCh <- promptResult{resp: resp, err: err}
	}()

	var result promptResult
	select {
	case result = <-resultCh:
	case <-ctx.Done():
		// Interrupt: ask the agent to stop, then wait briefly for the
		// prompt to resolve with stopReason=canceled per spec. The
		// cancel notification goes out on its own bounded context in a
		// goroutine so a wedged connection (blocked JSON-RPC write)
		// cannot keep RunTurn from reaching the timeout below.
		go func() {
			cancelCtx, done := context.WithTimeout(rpcCtx, cancelResolveTimeout)
			defer done()
			_ = conn.Cancel(cancelCtx, acp.CancelNotification{SessionId: session})
		}()
		select {
		case result = <-resultCh:
		case <-time.After(cancelResolveTimeout):
			return TurnOutcome{}, xerrors.Errorf("claude code turn: %w", ctx.Err())
		}
	}
	if result.err != nil {
		return TurnOutcome{}, xerrors.Errorf("prompt: %w", result.err)
	}

	return TurnOutcome{
		SessionID:  string(session),
		Resumed:    resumed,
		StopReason: result.resp.StopReason,
		Content:    collector.finalize(),
		Usage:      result.resp.Usage,
	}, nil
}

// establishSession resumes the previous ACP session when possible and
// starts a new one otherwise. It returns whether history was preserved.
func establishSession(
	ctx context.Context,
	conn *acp.ClientSideConnection,
	collector *turnCollector,
	caps acp.AgentCapabilities,
	input TurnInput,
) (acp.SessionId, bool, error) {
	if input.SessionID != "" {
		prior := acp.SessionId(input.SessionID)
		resumeCwd := input.SessionCwd
		if resumeCwd == "" {
			resumeCwd = input.Cwd
		}
		if caps.SessionCapabilities.Resume != nil {
			_, err := conn.ResumeSession(ctx, acp.ResumeSessionRequest{
				SessionId: prior,
				Cwd:       resumeCwd,
			})
			if err == nil {
				return prior, true, nil
			}
			// Resume failures normally fall back, but a canceled turn
			// must abort the chain, not race a fallback session.
			if ctx.Err() != nil {
				return "", false, xerrors.Errorf("resume session: %w", err)
			}
			input.Logger.Warn(ctx, "claude code session resume failed, falling back",
				slog.F("session_id", input.SessionID), slog.Error(err))
		}
		if caps.LoadSession {
			// session/load replays the whole conversation as
			// session/update notifications; the chat already has that
			// history persisted, so suppress collection during replay.
			collector.setSuppressed(true)
			_, err := conn.LoadSession(ctx, acp.LoadSessionRequest{
				SessionId:  prior,
				Cwd:        resumeCwd,
				McpServers: []acp.McpServer{},
			})
			collector.setSuppressed(false)
			if err == nil {
				return prior, true, nil
			}
			if ctx.Err() != nil {
				return "", false, xerrors.Errorf("load session: %w", err)
			}
			input.Logger.Warn(ctx, "claude code session load failed, starting new session",
				slog.F("session_id", input.SessionID), slog.Error(err))
		}
	}
	resp, err := conn.NewSession(ctx, acp.NewSessionRequest{
		Cwd: input.Cwd,
		// MCP passthrough is not supported yet; the field is required
		// by the schema.
		McpServers: []acp.McpServer{},
	})
	if err != nil {
		return "", false, xerrors.Errorf("new session: %w", err)
	}
	return resp.SessionId, false, nil
}

// turnCollector implements acp.Client. It streams preview parts and
// accumulates durable content in arrival order.
type turnCollector struct {
	mu            sync.Mutex
	suppressed    bool
	content       []fantasy.Content
	text          strings.Builder
	reasoning     strings.Builder
	openToolCalls map[string]*openToolCall

	publish func(codersdk.ChatMessageRole, codersdk.ChatMessagePart)
	logger  slog.Logger
}

type openToolCall struct {
	name         string
	rawInput     json.RawMessage
	output       strings.Builder
	contentIndex int
}

var _ acp.Client = (*turnCollector)(nil)

func (c *turnCollector) setSuppressed(v bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.suppressed = v
}

func (c *turnCollector) emit(role codersdk.ChatMessageRole, part codersdk.ChatMessagePart) {
	if c.publish != nil {
		c.publish(role, part)
	}
}

// flushTextLocked moves accumulated text/reasoning into durable content
// preserving arrival order relative to tool calls.
func (c *turnCollector) flushTextLocked() {
	if c.reasoning.Len() > 0 {
		c.content = append(c.content, fantasy.ReasoningContent{Text: c.reasoning.String()})
		c.reasoning.Reset()
	}
	if c.text.Len() > 0 {
		c.content = append(c.content, fantasy.TextContent{Text: c.text.String()})
		c.text.Reset()
	}
}

func (c *turnCollector) finalize() []fantasy.Content {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.flushTextLocked()
	return c.content
}

func (c *turnCollector) SessionUpdate(_ context.Context, params acp.SessionNotification) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.suppressed {
		return nil
	}
	update := params.Update
	switch {
	case update.AgentMessageChunk != nil:
		if text := contentBlockText(update.AgentMessageChunk.Content); text != "" {
			_, _ = c.text.WriteString(text)
			c.emit(codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageText(text))
		}
	case update.AgentThoughtChunk != nil:
		if text := contentBlockText(update.AgentThoughtChunk.Content); text != "" {
			_, _ = c.reasoning.WriteString(text)
			c.emit(codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageReasoning(text))
		}
	case update.ToolCall != nil:
		c.handleToolCallLocked(update.ToolCall)
	case update.ToolCallUpdate != nil:
		c.handleToolCallUpdateLocked(update.ToolCallUpdate)
	default:
		// Plan, mode, command, config, session info, and usage updates
		// have no chat part mapping yet.
	}
	return nil
}

func (c *turnCollector) handleToolCallLocked(call *acp.SessionUpdateToolCall) {
	c.flushTextLocked()
	name := toolDisplayName(string(call.Kind), call.Title)
	input := marshalRawJSON(call.RawInput)
	if c.openToolCalls == nil {
		c.openToolCalls = map[string]*openToolCall{}
	}
	content := fantasy.ToolCallContent{
		ToolCallID: string(call.ToolCallId),
		ToolName:   name,
		Input:      string(input),
	}
	c.openToolCalls[string(call.ToolCallId)] = &openToolCall{
		name:         name,
		rawInput:     input,
		contentIndex: len(c.content),
	}
	c.content = append(c.content, content)
	c.emit(codersdk.ChatMessageRoleAssistant,
		chatprompt.PartFromContentWithLogger(context.Background(), c.logger, content))
	c.collectToolContentLocked(string(call.ToolCallId), call.Content)
	if isTerminalToolStatus(call.Status) {
		c.completeToolCallLocked(string(call.ToolCallId), call.Status, call.RawOutput)
	}
}

func (c *turnCollector) handleToolCallUpdateLocked(update *acp.SessionToolCallUpdate) {
	id := string(update.ToolCallId)
	open, tracked := c.openToolCalls[id]
	if !tracked {
		// Update for a call this turn never opened (or one already
		// completed); nothing durable to attach it to.
		return
	}
	if update.RawInput != nil {
		// Adapters may open a tool call with empty or partial input
		// and deliver the final arguments in a later update. Patch
		// the already-appended durable content and re-emit the part
		// (the preview merges parts by tool call id) so history and
		// preview keep the final input.
		if input := marshalRawJSON(update.RawInput); !bytes.Equal(input, open.rawInput) {
			open.rawInput = input
			content := fantasy.ToolCallContent{
				ToolCallID: id,
				ToolName:   open.name,
				Input:      string(input),
			}
			c.content[open.contentIndex] = content
			c.emit(codersdk.ChatMessageRoleAssistant,
				chatprompt.PartFromContentWithLogger(context.Background(), c.logger, content))
		}
	}
	c.collectToolContentLocked(id, update.Content)
	var status acp.ToolCallStatus
	if update.Status != nil {
		status = *update.Status
	}
	if isTerminalToolStatus(status) {
		c.completeToolCallLocked(id, status, update.RawOutput)
	}
}

func (c *turnCollector) collectToolContentLocked(id string, blocks []acp.ToolCallContent) {
	open := c.openToolCalls[id]
	if open == nil {
		return
	}
	for _, block := range blocks {
		if block.Content == nil {
			continue
		}
		if text := contentBlockText(block.Content.Content); text != "" {
			_, _ = open.output.WriteString(text)
		}
	}
}

func (c *turnCollector) completeToolCallLocked(id string, status acp.ToolCallStatus, rawOutput any) {
	open := c.openToolCalls[id]
	if open == nil {
		return
	}
	delete(c.openToolCalls, id)
	outputText := open.output.String()
	if outputText == "" && rawOutput != nil {
		outputText = string(marshalRawJSON(rawOutput))
	}
	var result fantasy.ToolResultOutputContent
	if status == acp.ToolCallStatusFailed {
		result = fantasy.ToolResultOutputContentError{Error: xerrors.New(outputText)}
	} else {
		result = fantasy.ToolResultOutputContentText{Text: outputText}
	}
	content := fantasy.ToolResultContent{
		ToolCallID: id,
		ToolName:   open.name,
		Result:     result,
	}
	c.content = append(c.content, content)
	c.emit(codersdk.ChatMessageRoleTool,
		chatprompt.PartFromContentWithLogger(context.Background(), c.logger, content))
}

// RequestPermission auto-denies: v1 runs the adapter in a permission
// mode that avoids prompts (e.g. acceptEdits); anything that still
// prompts is declined with a visible note.
func (c *turnCollector) RequestPermission(_ context.Context, params acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
	c.mu.Lock()
	c.emit(codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageText("\n\n"+permissionDeniedNote+"\n\n"))
	_, _ = c.text.WriteString("\n\n" + permissionDeniedNote + "\n\n")
	c.mu.Unlock()
	for _, option := range params.Options {
		if option.Kind == acp.PermissionOptionKindRejectOnce {
			return acp.RequestPermissionResponse{
				Outcome: acp.NewRequestPermissionOutcomeSelected(option.OptionId),
			}, nil
		}
	}
	return acp.RequestPermissionResponse{
		Outcome: acp.NewRequestPermissionOutcomeCancelled(),
	}, nil
}

// The fs and terminal client capabilities are not advertised, so these
// must never be called; failing loudly beats corrupting a turn.
func (*turnCollector) ReadTextFile(context.Context, acp.ReadTextFileRequest) (acp.ReadTextFileResponse, error) {
	return acp.ReadTextFileResponse{}, xerrors.New("fs capability not supported")
}

func (*turnCollector) WriteTextFile(context.Context, acp.WriteTextFileRequest) (acp.WriteTextFileResponse, error) {
	return acp.WriteTextFileResponse{}, xerrors.New("fs capability not supported")
}

func (*turnCollector) CreateTerminal(context.Context, acp.CreateTerminalRequest) (acp.CreateTerminalResponse, error) {
	return acp.CreateTerminalResponse{}, xerrors.New("terminal capability not supported")
}

func (*turnCollector) KillTerminal(context.Context, acp.KillTerminalRequest) (acp.KillTerminalResponse, error) {
	return acp.KillTerminalResponse{}, xerrors.New("terminal capability not supported")
}

func (*turnCollector) TerminalOutput(context.Context, acp.TerminalOutputRequest) (acp.TerminalOutputResponse, error) {
	return acp.TerminalOutputResponse{}, xerrors.New("terminal capability not supported")
}

func (*turnCollector) ReleaseTerminal(context.Context, acp.ReleaseTerminalRequest) (acp.ReleaseTerminalResponse, error) {
	return acp.ReleaseTerminalResponse{}, xerrors.New("terminal capability not supported")
}

func (*turnCollector) WaitForTerminalExit(context.Context, acp.WaitForTerminalExitRequest) (acp.WaitForTerminalExitResponse, error) {
	return acp.WaitForTerminalExitResponse{}, xerrors.New("terminal capability not supported")
}

func isTerminalToolStatus(status acp.ToolCallStatus) bool {
	return status == acp.ToolCallStatusCompleted || status == acp.ToolCallStatusFailed
}

func contentBlockText(block acp.ContentBlock) string {
	if block.Text != nil {
		return block.Text.Text
	}
	return ""
}

func toolDisplayName(kind, title string) string {
	if strings.TrimSpace(title) != "" {
		return title
	}
	if strings.TrimSpace(kind) != "" {
		return kind
	}
	return "tool"
}

func marshalRawJSON(v any) json.RawMessage {
	if v == nil {
		return nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return raw
}

// BuildReseedContext renders prior conversation turns into a plain-text
// context block for seeding a fresh Claude session when the previous
// ACP session is gone. It is lossy by design and bounded in size.
func BuildReseedContext(turns []ReseedTurn) string {
	if len(turns) == 0 {
		return ""
	}
	const maxBytes = 32 * 1024
	var sb strings.Builder
	_, _ = sb.WriteString("<conversation-context>\n")
	_, _ = sb.WriteString("This chat has prior history, but your session state was lost (for example the workspace was rebuilt). The transcript below restores context. Do not mention this mechanism; just continue the conversation.\n\n")
	rendered := make([]string, 0, len(turns))
	total := 0
	for i := len(turns) - 1; i >= 0; i-- {
		entry := turns[i].Role + ": " + turns[i].Text + "\n"
		if total+len(entry) > maxBytes {
			break
		}
		total += len(entry)
		rendered = append(rendered, entry)
	}
	for i := len(rendered) - 1; i >= 0; i-- {
		_, _ = sb.WriteString(rendered[i])
	}
	_, _ = sb.WriteString("</conversation-context>")
	return sb.String()
}

// ReseedTurn is one prior conversation entry for BuildReseedContext.
type ReseedTurn struct {
	Role string
	Text string
}

// RuntimeState is the payload persisted in chats.runtime_state for
// claude_code chats. It carries what the next turn needs to resume the
// workspace-side conversation.
type RuntimeState struct {
	// SessionID is the ACP session that served the last turn.
	SessionID string `json:"session_id,omitempty"`
	// Cwd is the working directory the session was created with.
	Cwd string `json:"cwd,omitempty"`
	// Usage holds the session-cumulative token counts after the last
	// turn; the next resumed turn subtracts them to derive per-turn
	// usage.
	Usage *UsageTotals `json:"usage,omitempty"`
	// UpdatedAt records when the state was last written.
	UpdatedAt time.Time `json:"updated_at"`
}

// UsageTotals mirrors the ACP usage counters, which report
// session-cumulative counts rather than per-turn ones.
type UsageTotals struct {
	InputTokens         int64 `json:"input_tokens,omitempty"`
	OutputTokens        int64 `json:"output_tokens,omitempty"`
	TotalTokens         int64 `json:"total_tokens,omitempty"`
	ReasoningTokens     int64 `json:"reasoning_tokens,omitempty"`
	CacheCreationTokens int64 `json:"cache_creation_tokens,omitempty"`
	CacheReadTokens     int64 `json:"cache_read_tokens,omitempty"`
}

// ParseRuntimeState decodes persisted runtime state, tolerating absent
// or malformed payloads so broken state degrades to a fresh session
// instead of a wedged chat.
func ParseRuntimeState(raw []byte) RuntimeState {
	if len(raw) == 0 {
		return RuntimeState{}
	}
	var state RuntimeState
	if err := json.Unmarshal(raw, &state); err != nil {
		return RuntimeState{}
	}
	return state
}

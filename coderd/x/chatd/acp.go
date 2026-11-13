package chatd

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/quartz"
)

// Leave room beyond the agent's 90-second session request budget for transit.
const acpRequestTimeout = 2 * time.Minute

type acpSpawnArgs struct {
	Prompt           string `json:"prompt"`
	WorkingDirectory string `json:"working_directory"`
	Agent            struct {
		HarnessID string            `json:"harness_id"`
		Config    map[string]string `json:"config,omitempty"`
	} `json:"agent"`
}
type acpSessionArgs struct {
	HarnessID        string `json:"harness_id" description:"Harness ID returned by acp_spawn_agent."`
	SessionID        string `json:"session_id" description:"Native ACP session ID returned by acp_spawn_agent."`
	WorkingDirectory string `json:"working_directory" description:"Existing absolute working directory of the subagent."`
}
type acpWaitArgs struct {
	HarnessID        string `json:"harness_id" description:"Harness ID returned by acp_spawn_agent."`
	SessionID        string `json:"session_id" description:"Native ACP session ID returned by acp_spawn_agent."`
	WorkingDirectory string `json:"working_directory" description:"Existing absolute working directory of the subagent."`
	TimeoutSeconds   *int   `json:"timeout_seconds,omitempty" description:"How long to wait in seconds. Defaults to 300 seconds. Zero or negative values time out immediately. A timeout leaves the subagent running."`
}
type acpMessageArgs struct {
	HarnessID        string `json:"harness_id" description:"Harness ID returned by acp_spawn_agent."`
	SessionID        string `json:"session_id" description:"Native ACP session ID returned by acp_spawn_agent."`
	WorkingDirectory string `json:"working_directory" description:"Existing absolute working directory of the subagent."`
	Message          string `json:"message" description:"Starts a new turn in an idle session."`
}

type acpToolset struct {
	harnesses []workspacesdk.ACPHarness
	resolve   func(context.Context) (workspacesdk.AgentConn, error)
	clock     quartz.Clock
	logger    slog.Logger
}

func (p *Server) acpTools(ctx context.Context, opts rootChatToolsOptions) []fantasy.AgentTool {
	if opts.isPlanModeTurn || opts.chat.ParentChatID.Valid || !p.experiments.Enabled(codersdk.ExperimentChatACPSubagents) {
		return nil
	}
	resources, err := p.db.ListChatContextResourcesByChatID(ctx, opts.chat.ID)
	if err != nil {
		p.logger.Warn(ctx, "read ACP harness context", slog.Error(err))
		return nil
	}
	harnesses := acpHarnessesFromResources(resources)
	if len(harnesses) == 0 {
		return nil
	}
	return newACPTools(acpToolset{
		harnesses: harnesses,
		clock:     p.clock,
		logger:    p.logger,
		resolve:   opts.workspaceCtx.getWorkspaceConn,
	})
}

func newACPTools(toolset acpToolset) []fantasy.AgentTool {
	if len(toolset.harnesses) == 0 {
		return nil
	}
	return []fantasy.AgentTool{
		acpSpawnTool{
			AgentTool:  fantasy.NewParallelAgentTool("acp_spawn_agent", "Start a workspace-local ACP subagent using a configured external harness. The harness uses its own credentials and does not inherit this chat's MCP servers. Use acp_wait_agent to collect its response.", toolset.spawn),
			parameters: acpSpawnProperties(toolset.harnesses),
		},
		fantasy.NewParallelAgentTool("acp_wait_agent", "Wait for an ACP session to stop running and collect its assistant response. Streams the transcript to the user. Timeout and cancellation leave the session running.", toolset.wait),
		fantasy.NewParallelAgentTool("acp_message_agent", "Send a message to an idle ACP session. Busy sessions reject messages. Use acp_wait_agent to wait for completion, or acp_interrupt_agent followed by acp_wait_agent, before sending another message. Use acp_wait_agent to collect its response.", toolset.message),
		fantasy.NewParallelAgentTool("acp_interrupt_agent", "Request cancellation of the ACP session's running turn. Use acp_wait_agent to observe when it stops.", toolset.interrupt),
	}
}

func acpResponse(result any) fantasy.ToolResponse {
	data, err := json.Marshal(result)
	if err != nil {
		return fantasy.NewTextErrorResponse("encode ACP result: " + err.Error())
	}
	return fantasy.NewTextResponse(string(data))
}

type acpSessionResult struct {
	SessionID          string
	HarnessID          string
	WorkingDirectory   string
	HarnessDisplayName *string
}

func (t acpToolset) sessionResult(id workspacesdk.ACPSessionID) acpSessionResult {
	result := acpSessionResult{
		SessionID:        id.SessionID,
		HarnessID:        id.HarnessSlug,
		WorkingDirectory: id.WorkingDirectory,
	}
	for _, harness := range t.harnesses {
		if harness.Slug == id.HarnessSlug {
			result.HarnessDisplayName = new(harness.DisplayName)
			break
		}
	}
	return result
}

func (t acpToolset) errorResponse(err error, id *workspacesdk.ACPSessionID) fantasy.ToolResponse {
	result := map[string]any{"error": err.Error()}
	if id != nil {
		session := t.sessionResult(*id)
		result["session_id"] = session.SessionID
		result["harness_id"] = session.HarnessID
		result["working_directory"] = session.WorkingDirectory
		if session.HarnessDisplayName != nil {
			result["harness_display_name"] = *session.HarnessDisplayName
		}
	}
	response := acpResponse(result)
	response.IsError = true
	return response
}

func (args acpSessionArgs) sessionRef() (workspacesdk.ACPSessionID, error) {
	if strings.TrimSpace(args.HarnessID) == "" || strings.TrimSpace(args.SessionID) == "" || strings.TrimSpace(args.WorkingDirectory) == "" {
		return workspacesdk.ACPSessionID{}, xerrors.New("harness_id, session_id, and working_directory are required")
	}
	return workspacesdk.ACPSessionID{HarnessSlug: args.HarnessID, WorkingDirectory: args.WorkingDirectory, SessionID: args.SessionID}, nil
}

func acpToolRequest[T any](ctx context.Context, clock quartz.Clock, operation string, request func(context.Context) (T, error)) (T, error) {
	ctx, cancel := context.WithCancelCause(ctx)
	timer := clock.AfterFunc(acpRequestTimeout, func() { cancel(context.DeadlineExceeded) }, "chatd", "acp-request")
	defer func() {
		timer.Stop()
		cancel(nil)
	}()
	result, err := request(ctx)
	if err != nil {
		if ctx.Err() != nil {
			err = context.Cause(ctx)
		}
		return result, xerrors.Errorf("%s: %w", operation, err)
	}
	return result, nil
}

func (t acpToolset) spawn(ctx context.Context, args acpSpawnArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
	if strings.TrimSpace(args.Prompt) == "" || strings.TrimSpace(args.WorkingDirectory) == "" {
		return t.errorResponse(xerrors.New("prompt and working_directory are required"), nil), nil
	}
	var harness *workspacesdk.ACPHarness
	for i := range t.harnesses {
		if t.harnesses[i].Slug == args.Agent.HarnessID {
			harness = &t.harnesses[i]
			break
		}
	}
	if harness == nil {
		return t.errorResponse(xerrors.New("ACP harness is not available in this chat's pinned context"), nil), nil
	}
	if err := validateACPConfig(*harness, args.Agent.Config); err != nil {
		return t.errorResponse(err, nil), nil
	}
	id, ok := workspacesdk.ToolCallIDFromContext(ctx)
	if !ok {
		return t.errorResponse(xerrors.New("ACP spawn requires a deterministic tool-call ID"), nil), nil
	}
	conn, err := t.resolve(ctx)
	if err != nil {
		return t.errorResponse(err, nil), nil
	}
	session, err := acpToolRequest(ctx, t.clock, "create ACP session", func(ctx context.Context) (workspacesdk.ACPSession, error) {
		return conn.CreateACPSession(ctx, workspacesdk.ACPCreateSessionRequest{HarnessSlug: harness.Slug, WorkingDirectory: args.WorkingDirectory, Config: args.Agent.Config})
	})
	if err != nil {
		return t.errorResponse(err, nil), nil
	}
	// The agent deduplicates by chat and tool-call ID across endpoints. Use a
	// separate ID derived from `id` and a constant string so the prompt does
	// not replay session creation and retries do not send it twice.
	promptID := uuid.NewSHA1(id, []byte("initial-prompt"))
	_, err = acpToolRequest(workspacesdk.WithToolCallID(ctx, promptID), t.clock, "submit ACP prompt", func(ctx context.Context) (workspacesdk.ACPMessageResponse, error) {
		return conn.SendACPMessage(ctx, session.ID, workspacesdk.ACPMessageRequest{Text: args.Prompt})
	})
	if err != nil {
		return t.errorResponse(err, &session.ID), nil
	}
	return acpResponse(map[string]any{
		"session_id":           session.ID.SessionID,
		"harness_id":           session.ID.HarnessSlug,
		"harness_display_name": session.HarnessDisplayName,
		"status":               workspacesdk.ACPSessionStatusRunning,
	}), nil
}

func (t acpToolset) message(ctx context.Context, args acpMessageArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
	if strings.TrimSpace(args.Message) == "" {
		return t.errorResponse(xerrors.New("message is required"), nil), nil
	}
	session, err := (acpSessionArgs{HarnessID: args.HarnessID, SessionID: args.SessionID, WorkingDirectory: args.WorkingDirectory}).sessionRef()
	if err != nil {
		return t.errorResponse(err, nil), nil
	}
	conn, err := t.resolve(ctx)
	if err != nil {
		return t.errorResponse(err, &session), nil
	}
	if _, ok := workspacesdk.ToolCallIDFromContext(ctx); !ok {
		return t.errorResponse(xerrors.New("ACP message requires a deterministic tool-call ID"), &session), nil
	}
	_, err = acpToolRequest(ctx, t.clock, "submit ACP prompt", func(ctx context.Context) (workspacesdk.ACPMessageResponse, error) {
		return conn.SendACPMessage(ctx, session, workspacesdk.ACPMessageRequest{Text: args.Message})
	})
	if err != nil {
		return t.errorResponse(err, &session), nil
	}
	info := t.sessionResult(session)
	result := map[string]any{
		"session_id":        info.SessionID,
		"harness_id":        info.HarnessID,
		"working_directory": info.WorkingDirectory,
		"status":            workspacesdk.ACPSessionStatusRunning,
	}
	if info.HarnessDisplayName != nil {
		result["harness_display_name"] = *info.HarnessDisplayName
	}
	return acpResponse(result), nil
}

func (t acpToolset) interrupt(ctx context.Context, args acpSessionArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
	session, err := args.sessionRef()
	if err != nil {
		return t.errorResponse(err, nil), nil
	}
	conn, err := t.resolve(ctx)
	if err != nil {
		return t.errorResponse(err, &session), nil
	}
	info, err := acpToolRequest(ctx, t.clock, "interrupt ACP session", func(ctx context.Context) (workspacesdk.ACPSession, error) {
		return conn.InterruptACPSession(ctx, session)
	})
	if err != nil {
		return t.errorResponse(err, &session), nil
	}
	sessionInfo := t.sessionResult(session)
	if info.HarnessDisplayName != "" {
		sessionInfo.HarnessDisplayName = new(info.HarnessDisplayName)
	}
	result := map[string]any{
		"session_id":        sessionInfo.SessionID,
		"harness_id":        sessionInfo.HarnessID,
		"working_directory": sessionInfo.WorkingDirectory,
		"status":            info.Status,
		"interrupted":       info.Status == workspacesdk.ACPSessionStatusInterrupting,
	}
	if sessionInfo.HarnessDisplayName != nil {
		result["harness_display_name"] = *sessionInfo.HarnessDisplayName
	}
	return acpResponse(result), nil
}

func (t acpToolset) timeout(ctx context.Context, duration time.Duration, tag string) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)
	timer := t.clock.AfterFunc(duration, cancel, tag)
	return ctx, func() { timer.Stop(); cancel() }
}

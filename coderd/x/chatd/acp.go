package chatd

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/quartz"
)

type acpSpawnArgs struct {
	Prompt           string `json:"prompt"`
	WorkingDirectory string `json:"working_directory"`
	Agent            struct {
		HarnessID string            `json:"harness_id"`
		Config    map[string]string `json:"config,omitempty"`
	} `json:"agent"`
}
type acpSessionArgs struct {
	HarnessID        string `json:"harness_id"`
	SessionID        string `json:"session_id"`
	WorkingDirectory string `json:"working_directory"`
}
type acpWaitArgs struct {
	acpSessionArgs
	TimeoutSeconds *int `json:"timeout_seconds,omitempty"`
}
type acpMessageArgs struct {
	acpSessionArgs
	Message string `json:"message"`
}

type acpToolOptions struct {
	harnesses []workspacesdk.ACPHarness
	resolve   func(context.Context) (workspacesdk.AgentConn, database.Chat, uuid.UUID, error)
	clock     quartz.Clock
	logger    slog.Logger
}

var errACPWorkspaceChanged = xerrors.New("ACP chat workspace changed")

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
	return newACPTools(acpToolOptions{
		harnesses: harnesses, clock: p.clock, logger: p.logger,
		resolve: func(ctx context.Context) (workspacesdk.AgentConn, database.Chat, uuid.UUID, error) {
			// Keep the connection and workspace snapshot consistent with workspace tools.
			if opts.workspaceMu != nil {
				opts.workspaceMu.Lock()
				defer opts.workspaceMu.Unlock()
			}
			chat, err := p.db.GetChatByID(ctx, opts.chat.ID)
			if err != nil {
				return nil, database.Chat{}, uuid.Nil, err
			}
			current := opts.workspaceCtx.currentChatSnapshot()
			if !nullUUIDEqual(chat.WorkspaceID, current.WorkspaceID) || !nullUUIDEqual(chat.AgentID, current.AgentID) {
				opts.workspaceCtx.selectWorkspace(chat)
			}
			conn, err := opts.workspaceCtx.getWorkspaceConn(ctx)
			if err != nil {
				return nil, database.Chat{}, uuid.Nil, err
			}
			opts.workspaceCtx.mu.Lock()
			agentID := opts.workspaceCtx.agent.ID
			opts.workspaceCtx.mu.Unlock()
			return conn, opts.workspaceCtx.currentChatSnapshot(), agentID, nil
		},
	})
}

func newACPTools(opts acpToolOptions) []fantasy.AgentTool {
	if len(opts.harnesses) == 0 {
		return nil
	}
	sessionProperties := func() map[string]any {
		return map[string]any{
			"harness_id":        map[string]any{"type": "string", "description": "Harness ID returned by acp_spawn_agent."},
			"session_id":        map[string]any{"type": "string", "description": "Native ACP session ID returned by acp_spawn_agent."},
			"working_directory": map[string]any{"type": "string", "description": "Existing absolute working directory of the subagent."},
		}
	}
	waitProperties := sessionProperties()
	waitProperties["timeout_seconds"] = map[string]any{"type": "integer", "minimum": 1, "maximum": 300, "description": "How long to wait. Defaults to 300 seconds. A timeout leaves the subagent running."}
	messageProperties := sessionProperties()
	messageProperties["message"] = map[string]any{"type": "string", "description": "Starts a new turn in an idle session."}
	tools := []fantasy.AgentTool{
		newACPTool("acp_wait_agent", "Wait for an ACP session to stop running and collect its assistant response. Streams the transcript to the user. Timeout and cancellation leave the session running.", waitProperties, []string{"harness_id", "session_id", "working_directory"}, opts.wait),
		newACPTool("acp_message_agent", "Send a message to an idle ACP session. Busy sessions reject messages. Use acp_wait_agent to wait for completion, or acp_interrupt_agent followed by acp_wait_agent, before sending another message. Use acp_wait_agent to collect its response.", messageProperties, []string{"harness_id", "session_id", "working_directory", "message"}, opts.message),
		newACPTool("acp_interrupt_agent", "Request cancellation of the ACP session's running turn. Use acp_wait_agent to observe when it stops.", sessionProperties(), []string{"harness_id", "session_id", "working_directory"}, opts.interrupt),
	}
	return append([]fantasy.AgentTool{newACPTool("acp_spawn_agent", "Start a workspace-local ACP subagent using a configured external harness. The harness uses its own credentials and does not inherit this chat's MCP servers. Use acp_wait_agent to collect its response.", acpSpawnProperties(opts.harnesses), []string{"prompt", "working_directory", "agent"}, opts.spawn)}, tools...)
}

func acpResponse(result any) fantasy.ToolResponse {
	data, err := json.Marshal(result)
	if err != nil {
		return fantasy.NewTextErrorResponse("encode ACP result: " + err.Error())
	}
	return fantasy.NewTextResponse(string(data))
}

func (o acpToolOptions) sessionResult(id workspacesdk.ACPSessionID) map[string]any {
	result := map[string]any{"session_id": id.SessionID, "harness_id": id.HarnessSlug, "working_directory": id.WorkingDirectory}
	for _, harness := range o.harnesses {
		if harness.Slug == id.HarnessSlug {
			result["harness_display_name"] = harness.DisplayName
			break
		}
	}
	return result
}

func (o acpToolOptions) errorResponse(err error, id *workspacesdk.ACPSessionID) fantasy.ToolResponse {
	result := map[string]any{}
	if id != nil {
		result = o.sessionResult(*id)
	}
	result["error"] = err.Error()
	response := acpResponse(result)
	response.IsError = true
	return response
}

func (args acpSessionArgs) nativeID() (workspacesdk.ACPSessionID, error) {
	if strings.TrimSpace(args.HarnessID) == "" || strings.TrimSpace(args.SessionID) == "" || !filepath.IsAbs(args.WorkingDirectory) {
		return workspacesdk.ACPSessionID{}, xerrors.New("harness_id, session_id, and an absolute working_directory are required")
	}
	return workspacesdk.ACPSessionID{HarnessSlug: args.HarnessID, WorkingDirectory: args.WorkingDirectory, SessionID: args.SessionID}, nil
}

func (o acpToolOptions) connection(ctx context.Context, workspaceID uuid.UUID) (workspacesdk.AgentConn, database.Chat, uuid.UUID, error) {
	conn, chat, agentID, err := o.resolve(ctx)
	if err != nil {
		return nil, database.Chat{}, uuid.Nil, err
	}
	if !chat.WorkspaceID.Valid || (workspaceID != uuid.Nil && chat.WorkspaceID.UUID != workspaceID) {
		return nil, database.Chat{}, uuid.Nil, errACPWorkspaceChanged
	}
	return conn, chat, agentID, nil
}

func (o acpToolOptions) spawn(ctx context.Context, args acpSpawnArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
	if strings.TrimSpace(args.Prompt) == "" || !filepath.IsAbs(args.WorkingDirectory) {
		return o.errorResponse(xerrors.New("prompt and an absolute working_directory are required"), nil), nil
	}
	var harness *workspacesdk.ACPHarness
	for i := range o.harnesses {
		if o.harnesses[i].Slug == args.Agent.HarnessID {
			harness = &o.harnesses[i]
			break
		}
	}
	if harness == nil {
		return o.errorResponse(xerrors.New("ACP harness is not available in this chat's pinned context"), nil), nil
	}
	if err := validateACPConfig(*harness, args.Agent.Config); err != nil {
		return o.errorResponse(err, nil), nil
	}
	id, ok := workspacesdk.ToolCallIDFromContext(ctx)
	if !ok {
		return o.errorResponse(xerrors.New("ACP spawn requires a deterministic tool-call ID"), nil), nil
	}
	conn, _, _, err := o.connection(ctx, uuid.Nil)
	if err != nil {
		return o.errorResponse(err, nil), nil
	}
	session, err := conn.CreateACPSession(ctx, workspacesdk.ACPCreateSessionRequest{HarnessSlug: harness.Slug, WorkingDirectory: args.WorkingDirectory, Config: args.Agent.Config})
	if err != nil {
		return o.errorResponse(err, nil), nil
	}
	promptID := uuid.NewSHA1(id, []byte("initial-prompt"))
	_, err = conn.SendACPMessage(workspacesdk.WithToolCallID(ctx, promptID), session.ID, workspacesdk.ACPMessageRequest{ID: promptID, Text: args.Prompt})
	if err != nil {
		return o.errorResponse(err, &session.ID), nil
	}
	result := o.sessionResult(session.ID)
	delete(result, "working_directory")
	result["harness_display_name"] = session.HarnessDisplayName
	result["status"] = workspacesdk.ACPSessionStatusRunning
	return acpResponse(result), nil
}

func (o acpToolOptions) message(ctx context.Context, args acpMessageArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
	if strings.TrimSpace(args.Message) == "" {
		return o.errorResponse(xerrors.New("message is required"), nil), nil
	}
	session, err := args.nativeID()
	if err != nil {
		return o.errorResponse(err, nil), nil
	}
	conn, _, _, err := o.connection(ctx, uuid.Nil)
	if err != nil {
		return o.errorResponse(err, &session), nil
	}
	id, ok := workspacesdk.ToolCallIDFromContext(ctx)
	if !ok {
		return o.errorResponse(xerrors.New("ACP message requires a deterministic tool-call ID"), &session), nil
	}
	_, err = conn.SendACPMessage(ctx, session, workspacesdk.ACPMessageRequest{ID: id, Text: args.Message})
	if err != nil {
		return o.errorResponse(err, &session), nil
	}
	result := o.sessionResult(session)
	result["status"] = workspacesdk.ACPSessionStatusRunning
	return acpResponse(result), nil
}

func (o acpToolOptions) interrupt(ctx context.Context, args acpSessionArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
	session, err := args.nativeID()
	if err != nil {
		return o.errorResponse(err, nil), nil
	}
	conn, _, _, err := o.connection(ctx, uuid.Nil)
	if err != nil {
		return o.errorResponse(err, &session), nil
	}
	info, err := conn.InterruptACPSession(ctx, session)
	if err != nil {
		return o.errorResponse(err, &session), nil
	}
	result := o.sessionResult(session)
	if info.HarnessDisplayName != "" {
		result["harness_display_name"] = info.HarnessDisplayName
	}
	result["status"], result["interrupted"] = info.Status, info.Status == workspacesdk.ACPSessionStatusInterrupting
	return acpResponse(result), nil
}

func (o acpToolOptions) timeout(ctx context.Context, duration time.Duration, tag string) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)
	timer := o.clock.AfterFunc(duration, cancel, tag)
	return ctx, func() { timer.Stop(); cancel() }
}

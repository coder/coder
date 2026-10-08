package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
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
		Harness string            `json:"harness"`
		Config  map[string]string `json:"config,omitempty"`
	} `json:"agent"`
}
type acpSessionArgs struct {
	SessionID string `json:"session_id"`
}
type acpWaitArgs struct {
	SessionID      string `json:"session_id"`
	TimeoutSeconds *int   `json:"timeout_seconds,omitempty"`
}
type acpMessageArgs struct {
	SessionID string `json:"session_id"`
	Message   string `json:"message"`
}
type acpListArgs struct {
	Limit  *int32 `json:"limit,omitempty"`
	Offset int32  `json:"offset,omitempty"`
}

type acpToolOptions struct {
	db        database.Store
	chatID    uuid.UUID
	harnesses []workspacesdk.ACPHarness
	resolve   func(context.Context) (workspacesdk.AgentConn, database.Chat, uuid.UUID, error)
	clock     quartz.Clock
	logger    slog.Logger
}

var errACPWorkspaceChanged = xerrors.New("ACP session belongs to a different workspace")

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
		count, err := p.db.CountAgentsACPSessionsByChatID(ctx, opts.chat.ID)
		if err != nil || count == 0 {
			return nil
		}
	}
	return newACPTools(acpToolOptions{
		db: p.db, chatID: opts.chat.ID, harnesses: harnesses, clock: p.clock, logger: p.logger,
		resolve: func(ctx context.Context) (workspacesdk.AgentConn, database.Chat, uuid.UUID, error) {
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
	sessionProperties := func() map[string]any {
		return map[string]any{"session_id": map[string]any{"type": "string", "description": "Session ID returned by acp_spawn_agent or acp_list_agents."}}
	}
	waitProperties := sessionProperties()
	waitProperties["timeout_seconds"] = map[string]any{"type": "integer", "minimum": 1, "maximum": 300, "description": "How long to wait. Defaults to 300 seconds. A timeout leaves the subagent running."}
	messageProperties := sessionProperties()
	messageProperties["message"] = map[string]any{"type": "string", "description": "Starts a turn when idle, or steers a running turn if the harness supports steering."}
	tools := []fantasy.AgentTool{
		newACPTool("acp_list_agents", "List this chat's ACP sessions, most recently updated first.", map[string]any{
			"limit":  map[string]any{"type": "integer", "minimum": 1, "maximum": 50, "description": "Maximum sessions to return. Defaults to 10."},
			"offset": map[string]any{"type": "integer", "minimum": 0, "description": "Number of sessions to skip. Defaults to 0."},
		}, []string{}, opts.list),
		newACPTool("acp_wait_agent", "Wait for an ACP session to stop running and collect its assistant response. Streams the transcript to the user. Timeout and cancellation leave the session running.", waitProperties, []string{"session_id"}, opts.wait),
		newACPTool("acp_message_agent", "Send a message to an ACP session. Running sessions require harness steering support; messages are not queued. Use acp_wait_agent to collect its response.", messageProperties, []string{"session_id", "message"}, opts.message),
		newACPTool("acp_interrupt_agent", "Request cancellation of the ACP session's running turn. Use acp_wait_agent to observe when it stops.", sessionProperties(), []string{"session_id"}, opts.interrupt),
	}
	if len(opts.harnesses) > 0 {
		tools = append([]fantasy.AgentTool{newACPTool("acp_spawn_agent", "Start a workspace-local ACP subagent using a configured external harness. The harness uses its own credentials and does not inherit this chat's MCP servers. Use acp_wait_agent to collect its response.", acpSpawnProperties(opts.harnesses), []string{"prompt", "working_directory", "agent"}, opts.spawn)}, tools...)
	}
	return tools
}

func acpNativeID(session database.AgentsAcpSession) workspacesdk.ACPSessionID {
	return workspacesdk.ACPSessionID{HarnessSlug: session.HarnessSlug, WorkingDirectory: session.WorkingDirectory, SessionID: session.SessionID}
}

func acpResponse(result any) fantasy.ToolResponse {
	data, err := json.Marshal(result)
	if err != nil {
		return fantasy.NewTextErrorResponse("encode ACP result: " + err.Error())
	}
	return fantasy.NewTextResponse(string(data))
}

func acpError(err error, session *database.AgentsAcpSession) fantasy.ToolResponse {
	result := map[string]any{"error": err.Error()}
	if session != nil {
		result["session_id"], result["harness"] = session.ID.String(), session.HarnessSlug
		result["harness_display_name"] = session.HarnessDisplayName
	}
	response := acpResponse(result)
	response.IsError = true
	return response
}

func (o acpToolOptions) session(ctx context.Context, rawID string) (database.AgentsAcpSession, error) {
	id, err := uuid.Parse(rawID)
	if err != nil || id == uuid.Nil {
		return database.AgentsAcpSession{}, xerrors.New("invalid ACP session ID")
	}
	return o.db.GetAgentsACPSessionByIDAndChatID(ctx, database.GetAgentsACPSessionByIDAndChatIDParams{ID: id, ChatID: o.chatID})
}

func (o acpToolOptions) connection(ctx context.Context, session database.AgentsAcpSession) (workspacesdk.AgentConn, uuid.UUID, error) {
	conn, chat, agentID, err := o.resolve(ctx)
	if err != nil {
		return nil, uuid.Nil, err
	}
	if chat.ID != session.ChatID || chat.OrganizationID != session.OrganizationID || !chat.WorkspaceID.Valid || chat.WorkspaceID.UUID != session.WorkspaceID {
		return nil, uuid.Nil, errACPWorkspaceChanged
	}
	return conn, agentID, nil
}

func (o acpToolOptions) touchSession(ctx context.Context, session database.AgentsAcpSession) (workspacesdk.AgentConn, uuid.UUID, error) {
	conn, agentID, err := o.connection(ctx, session)
	if err != nil {
		return nil, uuid.Nil, err
	}
	// Guard the update against a concurrent chat workspace change.
	_, err = o.db.UpdateAgentsACPSessionUpdatedAt(ctx, database.UpdateAgentsACPSessionUpdatedAtParams{ID: session.ID, ChatID: o.chatID, AgentID: agentID})
	if err != nil {
		return nil, uuid.Nil, xerrors.Errorf("update ACP session timestamp: %w", err)
	}
	return conn, agentID, nil
}

func (o acpToolOptions) spawn(ctx context.Context, args acpSpawnArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
	if strings.TrimSpace(args.Prompt) == "" || !filepath.IsAbs(args.WorkingDirectory) {
		return acpError(xerrors.New("prompt and an absolute working_directory are required"), nil), nil
	}
	var harness *workspacesdk.ACPHarness
	for i := range o.harnesses {
		if o.harnesses[i].Slug == args.Agent.Harness {
			harness = &o.harnesses[i]
			break
		}
	}
	if harness == nil {
		return acpError(xerrors.New("ACP harness is not available in this chat's pinned context"), nil), nil
	}
	if err := validateACPConfig(*harness, args.Agent.Config); err != nil {
		return acpError(err, nil), nil
	}
	id, ok := workspacesdk.ToolCallIDFromContext(ctx)
	if !ok {
		return acpError(xerrors.New("ACP spawn requires a deterministic tool-call ID"), nil), nil
	}
	session, err := o.db.GetAgentsACPSessionByIDAndChatID(ctx, database.GetAgentsACPSessionByIDAndChatIDParams{ID: id, ChatID: o.chatID})
	var conn workspacesdk.AgentConn
	if errors.Is(err, sql.ErrNoRows) {
		var chat database.Chat
		var agentID uuid.UUID
		conn, chat, agentID, err = o.resolve(ctx)
		if err != nil {
			return acpError(err, nil), nil
		}
		created, err := conn.CreateACPSession(ctx, workspacesdk.ACPCreateSessionRequest{RequestID: id, HarnessSlug: harness.Slug, WorkingDirectory: args.WorkingDirectory, Config: args.Agent.Config})
		if err != nil {
			return acpError(err, nil), nil
		}
		session, err = o.db.InsertAgentsACPSession(ctx, database.InsertAgentsACPSessionParams{ID: id, ChatID: o.chatID, OrganizationID: chat.OrganizationID, WorkspaceID: chat.WorkspaceID.UUID, AgentID: agentID, WorkingDirectory: created.ID.WorkingDirectory, HarnessSlug: created.ID.HarnessSlug, HarnessDisplayName: created.HarnessDisplayName, SessionID: created.ID.SessionID})
		if err != nil {
			return acpError(xerrors.Errorf("save ACP session: %w", err), nil), nil
		}
	} else if err != nil {
		return acpError(err, nil), nil
	}
	if session.HarnessSlug != args.Agent.Harness || session.WorkingDirectory != args.WorkingDirectory {
		return acpError(xerrors.New("ACP spawn retry changed the session identity"), &session), nil
	}
	conn, _, err = o.touchSession(ctx, session)
	if err != nil {
		return acpError(err, &session), nil
	}
	_, err = conn.SendACPMessage(ctx, acpNativeID(session), workspacesdk.ACPMessageRequest{ID: uuid.NewSHA1(id, []byte("initial-prompt")), Text: args.Prompt})
	if err != nil {
		return acpError(err, &session), nil
	}
	return acpResponse(map[string]any{"session_id": session.ID.String(), "harness": session.HarnessSlug, "harness_display_name": session.HarnessDisplayName, "status": workspacesdk.ACPSessionStatusRunning}), nil
}

func (o acpToolOptions) message(ctx context.Context, args acpMessageArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
	if strings.TrimSpace(args.Message) == "" {
		return acpError(xerrors.New("message is required"), nil), nil
	}
	session, err := o.session(ctx, args.SessionID)
	if err != nil {
		return acpError(err, nil), nil
	}
	conn, _, err := o.touchSession(ctx, session)
	if err != nil {
		return acpError(err, &session), nil
	}
	id, ok := workspacesdk.ToolCallIDFromContext(ctx)
	if !ok {
		return acpError(xerrors.New("ACP message requires a deterministic tool-call ID"), &session), nil
	}
	_, err = conn.SendACPMessage(ctx, acpNativeID(session), workspacesdk.ACPMessageRequest{ID: id, Text: args.Message})
	if err != nil {
		return acpError(err, &session), nil
	}
	return acpResponse(map[string]any{"session_id": session.ID.String(), "harness": session.HarnessSlug, "harness_display_name": session.HarnessDisplayName, "status": workspacesdk.ACPSessionStatusRunning}), nil
}

func (o acpToolOptions) interrupt(ctx context.Context, args acpSessionArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
	session, err := o.session(ctx, args.SessionID)
	if err != nil {
		return acpError(err, nil), nil
	}
	conn, _, err := o.touchSession(ctx, session)
	if err != nil {
		return acpError(err, &session), nil
	}
	info, err := conn.InterruptACPSession(ctx, acpNativeID(session))
	if err != nil {
		return acpError(err, &session), nil
	}
	return acpResponse(map[string]any{"session_id": session.ID.String(), "harness": session.HarnessSlug, "harness_display_name": session.HarnessDisplayName, "status": info.Status, "interrupted": info.Status == workspacesdk.ACPSessionStatusRunning}), nil
}

func (o acpToolOptions) list(ctx context.Context, args acpListArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
	limit := int32(10)
	if args.Limit != nil {
		limit = *args.Limit
	}
	if limit < 1 || limit > 50 || args.Offset < 0 {
		return acpError(xerrors.New("limit must be 1 through 50 and offset must be a nonnegative 32-bit integer"), nil), nil
	}
	rows, err := o.db.ListAgentsACPSessionsByChatID(ctx, database.ListAgentsACPSessionsByChatIDParams{ChatID: o.chatID, LimitValue: limit, OffsetValue: args.Offset})
	if err != nil {
		return acpError(err, nil), nil
	}
	total, err := o.db.CountAgentsACPSessionsByChatID(ctx, o.chatID)
	if err != nil {
		return acpError(err, nil), nil
	}
	inventory := make(map[workspacesdk.ACPSessionID]workspacesdk.ACPSession)
	var chat database.Chat
	if len(rows) > 0 {
		var conn workspacesdk.AgentConn
		conn, chat, _, err = o.resolve(ctx)
		if err != nil {
			return acpError(err, nil), nil
		}
		nativeSessions, err := conn.ListACPSessions(ctx)
		if err != nil {
			return acpError(err, nil), nil
		}
		for _, native := range nativeSessions {
			inventory[native.ID] = native
		}
	}
	agents := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		entry := map[string]any{"session_id": row.ID.String(), "harness": row.HarnessSlug, "harness_display_name": row.HarnessDisplayName, "created_at": row.CreatedAt, "updated_at": row.UpdatedAt, "status": "unknown"}
		if chat.ID != row.ChatID || chat.OrganizationID != row.OrganizationID || !chat.WorkspaceID.Valid || chat.WorkspaceID.UUID != row.WorkspaceID {
			entry["status"], entry["error"] = workspacesdk.ACPSessionStatusError, errACPWorkspaceChanged.Error()
		} else if info, exists := inventory[acpNativeID(row)]; exists {
			entry["status"] = info.Status
			if info.Error != "" {
				entry["error"] = info.Error
			}
		}
		agents = append(agents, entry)
	}
	return acpResponse(map[string]any{"agents": agents, "total": total, "returned": len(agents), "offset": args.Offset, "has_more": int64(args.Offset)+int64(len(agents)) < total}), nil
}

func (o acpToolOptions) timeout(ctx context.Context, duration time.Duration, tag string) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)
	timer := o.clock.AfterFunc(duration, cancel, tag)
	return ctx, func() { timer.Stop(); cancel() }
}

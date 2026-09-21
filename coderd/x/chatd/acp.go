package chatd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

type acpTargetArgs struct {
	SessionID        string `json:"session_id"`
	WorkspaceAgentID string `json:"workspace_agent_id" description:"Workspace agent ID returned by acp_spawn_agent."`
}
type acpWaitArgs struct {
	SessionID        string `json:"session_id"`
	WorkspaceAgentID string `json:"workspace_agent_id"`
	TimeoutSeconds   int    `json:"timeout_seconds,omitempty"`
}
type acpMessageArgs struct {
	SessionID        string `json:"session_id"`
	WorkspaceAgentID string `json:"workspace_agent_id"`
	Message          string `json:"message"`
	Interrupt        bool   `json:"interrupt,omitempty"`
}

func (*Server) acpTools(opts rootChatToolsOptions) []fantasy.AgentTool {
	request := func(ctx context.Context, target acpTargetArgs, method, action string, body any) (json.RawMessage, uuid.UUID, error) {
		conn, err := opts.workspaceCtx.getWorkspaceConn(ctx)
		if err != nil {
			return nil, uuid.Nil, err
		}
		opts.workspaceCtx.mu.Lock()
		agentID := opts.workspaceCtx.agent.ID
		parent := opts.workspaceCtx.currentChatSnapshot()
		opts.workspaceCtx.mu.Unlock()
		if target.WorkspaceAgentID != "" {
			requested, err := uuid.Parse(target.WorkspaceAgentID)
			if err != nil {
				return nil, uuid.Nil, err
			}
			if requested != agentID {
				return nil, uuid.Nil, xerrors.New("ACP session belongs to a different workspace agent; it may have expired")
			}
		}
		path := workspacesdk.ACPPath(parent.OrganizationID, parent.ID)
		if target.SessionID != "" {
			id, err := uuid.Parse(target.SessionID)
			if err != nil {
				return nil, uuid.Nil, err
			}
			path += id.String() + "/"
		}
		response, err := workspacesdk.ACPRequest(ctx, conn, method, path+action, body)
		if err != nil {
			return nil, uuid.Nil, err
		}
		defer response.Body.Close()
		if response.StatusCode >= 400 {
			return nil, uuid.Nil, codersdk.ReadBodyAsError(response)
		}
		var result json.RawMessage
		err = json.NewDecoder(response.Body).Decode(&result)
		return result, agentID, err
	}

	return []fantasy.AgentTool{
		fantasy.NewAgentTool("acp_spawn_agent", "Spawn an ephemeral workspace agent. agent is exactly claude_code (bash: exec npx -y @agentclientprotocol/claude-agent-acp) or codex (bash: exec npx -y @agentclientprotocol/codex-acp). Requires a running workspace with credentials. Adapters automatically approve permissions. Sessions live until the workspace agent exits. Use acp_wait_agent for results and acp_message_agent for follow-ups.", func(ctx context.Context, args codersdk.ACPSpawnRequest, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			return acpToolResponse(request(ctx, acpTargetArgs{}, http.MethodPost, "", args)), nil
		}),
		fantasy.NewAgentTool("acp_message_agent", "Send a follow-up to an ACP session. Busy sessions queue messages FIFO. interrupt cancels current work before processing queued messages.", func(ctx context.Context, args acpMessageArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if args.SessionID == "" {
				return fantasy.NewTextErrorResponse("session_id is required"), nil
			}
			return acpToolResponse(request(ctx, acpTargetArgs{SessionID: args.SessionID, WorkspaceAgentID: args.WorkspaceAgentID}, http.MethodPost, "messages", codersdk.ACPMessageRequest{Message: args.Message, Interrupt: args.Interrupt})), nil
		}),
		fantasy.NewAgentTool("acp_interrupt_agent", "Interrupt current ACP work without closing the session. Queued messages remain.", func(ctx context.Context, args acpTargetArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if args.SessionID == "" {
				return fantasy.NewTextErrorResponse("session_id is required"), nil
			}
			return acpToolResponse(request(ctx, args, http.MethodPost, "interrupt", nil)), nil
		}),
		fantasy.NewAgentTool("acp_wait_agent", "Wait for an ACP session to become idle or fail. Defaults to 300 seconds; timeout does not stop the agent. Returns its latest response and current status.", func(ctx context.Context, args acpWaitArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if args.SessionID == "" {
				return fantasy.NewTextErrorResponse("session_id is required"), nil
			}
			return acpToolResponse(request(ctx, acpTargetArgs{SessionID: args.SessionID, WorkspaceAgentID: args.WorkspaceAgentID}, http.MethodGet, "wait?timeout_seconds="+strconv.Itoa(args.TimeoutSeconds), nil)), nil
		}),
		fantasy.NewAgentTool("acp_list_agents", "List this chat's in-memory ACP sessions, most recently active first. Defaults to 10 results, maximum 50.", func(ctx context.Context, args listAgentsArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			q := url.Values{}
			if args.Limit != nil {
				q.Set("limit", strconv.Itoa(*args.Limit))
			}
			if args.Offset != nil {
				q.Set("offset", strconv.Itoa(*args.Offset))
			}
			raw, id, err := request(ctx, acpTargetArgs{}, http.MethodGet, "?"+q.Encode(), nil)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			var list codersdk.ACPListResponse
			if err = json.Unmarshal(raw, &list); err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			for i := range list.Agents {
				list.Agents[i].WorkspaceAgentID = id
			}
			return toolJSONResponse(map[string]any{"agents": list.Agents, "total": list.Total, "has_more": list.HasMore}), nil
		}),
	}
}

func acpToolResponse(raw json.RawMessage, agentID uuid.UUID, err error) fantasy.ToolResponse {
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error())
	}
	var s codersdk.ACPSession
	if err = json.Unmarshal(raw, &s); err != nil {
		return fantasy.NewTextErrorResponse(err.Error())
	}
	response := map[string]any{"session_id": s.SessionID, "workspace_agent_id": agentID, "parent_chat_id": s.ParentChatID, "agent": s.Agent, "title": s.Title, "status": s.Status, "error": s.Error}
	var report string
	var entries []codersdk.ACPEntry
	for _, entry := range s.Entries {
		if entry.Role == "user" {
			report = ""
			entries = nil
		}
		if entry.Role == "assistant" {
			entries = append(entries, entry)
		}
		if entry.Role == "assistant" && entry.Kind == "text" {
			report += entry.Text
		}
	}
	response["report"] = report
	response["entries"] = entries
	return toolJSONResponse(response)
}

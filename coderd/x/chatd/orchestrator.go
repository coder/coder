package chatd

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/codersdk"
)

const (
	spawnChatToolName = "spawn_chat"
	listChatsToolName = "list_chats"
	readChatToolName  = "read_chat"

	defaultListChatsLimit = 20
	maxListChatsLimit     = 100
	maxSearchChatsRunes   = 200
)

// OrchestratorSystemPrompt replaces the deployment default for orchestrator
// chats, which have no workspace and coordinate the user's other chats.
const OrchestratorSystemPrompt = `You are the Coder orchestrator: a persistent chat that coordinates the user's other Coder agent chats.
You have no workspace and no workspace tools. Do not attempt to read, write, or execute anything directly.

<behavior>
Use your tools to answer questions about the user's chats and to start new work on their behalf.
Use ` + listChatsToolName + ` to see the user's chats, including their status, summaries, and workspaces. Use ` + readChatToolName + ` to read a specific chat's latest response before summarizing or reporting on it.
Use ` + spawnChatToolName + ` to start a new, independent chat when the user asks for work that needs a workspace or a focused agent. Write a complete, self-contained prompt: the new chat does not see this conversation. It can create or attach its own workspace when needed.
Spawned chats run asynchronously. After spawning, report the chat ID and title, and use ` + readChatToolName + ` or ` + listChatsToolName + ` to check progress when the user asks.
Prefer reusing an existing chat over spawning a duplicate when one already covers the same work.
Use configured external MCP tools when they help.
Be concise and direct. Do not ask clarifying questions when the answer is available from the user's chats.
</behavior>`

// orchestratorAwareness replaces the workspace awareness system message on
// orchestrator chats.
const orchestratorAwareness = "This is the orchestrator chat. It never attaches a workspace. Coordinate work through " +
	spawnChatToolName + ", " + listChatsToolName + ", and " + readChatToolName + " instead of workspace tools."

// resolveOrchestratorSystemPrompt combines the orchestrator prompt with the
// admin-configured custom prompt. The deployment default is omitted because
// it describes workspace tools the orchestrator does not have.
func (p *Server) resolveOrchestratorSystemPrompt(ctx context.Context) string {
	config, err := p.db.GetChatSystemPromptConfig(ctx)
	if err != nil {
		p.logger.Error(ctx, "failed to fetch chat system prompt configuration for orchestrator, using default", slog.Error(err))
		return OrchestratorSystemPrompt
	}
	parts := []string{OrchestratorSystemPrompt}
	if custom := codersdk.SanitizePromptText(config.ChatSystemPrompt); custom != "" {
		parts = append(parts, custom)
	}
	return strings.Join(parts, "\n\n")
}

type spawnChatArgs struct {
	Prompt string `json:"prompt" description:"Complete, self-contained instructions for the new chat. It does not see the orchestrator conversation."`
	Title  string `json:"title,omitempty" description:"Optional short title. Derived from the prompt when omitted."`
}

type listChatsArgs struct {
	Search          string `json:"search,omitempty" description:"Optional free-text search over chat titles and pull request metadata."`
	IncludeArchived bool   `json:"include_archived,omitempty" description:"Include archived chats. Defaults to false."`
	Limit           *int   `json:"limit,omitempty" description:"Maximum chats to return (default 20, max 100)."`
	Offset          *int   `json:"offset,omitempty" description:"Number of chats to skip for paging."`
}

type readChatArgs struct {
	ChatID string `json:"chat_id" description:"UUID of one of the user's chats."`
}

// orchestratorTools are the chat coordination tools available only on
// orchestrator chats. They operate on the owner's root chats rather than
// on parent/child subagent relationships.
func (p *Server) orchestratorTools(currentChat func() database.Chat) []fantasy.AgentTool {
	return []fantasy.AgentTool{
		fantasy.NewAgentTool(
			spawnChatToolName,
			"Start a new independent chat owned by the user. The chat appears "+
				"in their sidebar, runs asynchronously, and can create or attach "+
				"its own workspace. Returns chat_id, title, and status. Use "+
				readChatToolName+" to check on it later.",
			func(ctx context.Context, args spawnChatArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
				orchestrator := currentChat()
				prompt := strings.TrimSpace(args.Prompt)
				if prompt == "" {
					return fantasy.NewTextErrorResponse("prompt is required"), nil
				}
				title := strings.TrimSpace(args.Title)
				if title == "" {
					title = chatprompt.FallbackTitle(prompt)
				}
				chat, err := p.CreateChat(ctx, CreateOptions{
					OrganizationID:          orchestrator.OrganizationID,
					OwnerID:                 orchestrator.OwnerID,
					Title:                   title,
					TitleDerivedFromContent: args.Title == "",
					ModelConfigID:           orchestrator.LastModelConfigID,
					ClientType:              orchestrator.ClientType,
					InitialUserContent:      []codersdk.ChatMessagePart{codersdk.ChatMessageText(prompt)},
					MCPServerIDs:            orchestrator.MCPServerIDs,
				})
				if err != nil {
					p.logger.Warn(ctx, "orchestrator failed to spawn chat",
						slog.F("orchestrator_chat_id", orchestrator.ID),
						slog.Error(err),
					)
					return fantasy.NewTextErrorResponse(xerrors.Errorf("spawn chat: %w", err).Error()), nil
				}
				return toolJSONResponse(orchestratorChatSummary(chat)), nil
			},
		),
		fantasy.NewAgentTool(
			listChatsToolName,
			"List the user's chats, most recently active first. Each entry has "+
				"chat_id, title, status, workspace_id, summary, last_turn_summary, "+
				"created_at, and updated_at. Status: running = working, waiting = "+
				"idle, error = stopped on error. Returns `returned`, `offset`, and "+
				"`has_more`; use `offset` to page.",
			func(ctx context.Context, args listChatsArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
				orchestrator := currentChat()
				limit := defaultListChatsLimit
				if args.Limit != nil {
					limit = min(max(*args.Limit, 1), maxListChatsLimit)
				}
				offset := 0
				if args.Offset != nil && *args.Offset > 0 {
					offset = *args.Offset
				}
				search := strings.TrimSpace(args.Search)
				if len([]rune(search)) > maxSearchChatsRunes {
					return fantasy.NewTextErrorResponse("search is too long"), nil
				}
				ownerCtx, err := p.chatOwnerContext(ctx, orchestrator.OwnerID)
				if err != nil {
					return fantasy.NewTextErrorResponse(err.Error()), nil
				}
				archived := sql.NullBool{Bool: false, Valid: true}
				if args.IncludeArchived {
					archived = sql.NullBool{}
				}
				// Fetch one extra row to report has_more without a count query.
				rows, err := p.db.GetChats(ownerCtx, database.GetChatsParams{
					OwnedOnly: true,
					ViewerID:  orchestrator.OwnerID,
					Archived:  archived,
					Search:    search,
					// #nosec G115 - bounded by maxListChatsLimit and small offsets.
					OffsetOpt: int32(offset),
					// #nosec G115 - bounded by maxListChatsLimit.
					LimitOpt: int32(limit + 1),
				})
				if err != nil {
					p.logger.Warn(ctx, "orchestrator failed to list chats",
						slog.F("orchestrator_chat_id", orchestrator.ID),
						slog.Error(err),
					)
					return fantasy.NewTextErrorResponse("internal error listing chats"), nil
				}
				hasMore := len(rows) > limit
				if hasMore {
					rows = rows[:limit]
				}
				chats := make([]map[string]any, 0, len(rows))
				for _, row := range rows {
					chats = append(chats, orchestratorChatSummary(row.Chat))
				}
				return toolJSONResponse(map[string]any{
					"chats":    chats,
					"returned": len(chats),
					"offset":   offset,
					"has_more": hasMore,
				}), nil
			},
		),
		fantasy.NewAgentTool(
			readChatToolName,
			"Read one of the user's chats: its summary fields plus the latest "+
				"assistant response text. Only chats owned by the user are readable.",
			func(ctx context.Context, args readChatArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
				orchestrator := currentChat()
				chatID, err := parseSubagentToolChatID(args.ChatID)
				if err != nil {
					return fantasy.NewTextErrorResponse(err.Error()), nil
				}
				chat, err := p.db.GetChatByID(ctx, chatID)
				if err != nil {
					if xerrors.Is(err, sql.ErrNoRows) {
						return fantasy.NewTextErrorResponse("chat not found"), nil
					}
					p.logger.Warn(ctx, "orchestrator failed to read chat",
						slog.F("orchestrator_chat_id", orchestrator.ID),
						slog.F("chat_id", chatID),
						slog.Error(err),
					)
					return fantasy.NewTextErrorResponse("internal error reading chat"), nil
				}
				// Ownership is the boundary: shared chats stay out of reach so
				// the orchestrator cannot read another user's work.
				if chat.OwnerID != orchestrator.OwnerID {
					return fantasy.NewTextErrorResponse("chat not found"), nil
				}
				latest, err := latestSubagentAssistantMessage(ctx, p.db, chat.ID)
				if err != nil {
					p.logger.Warn(ctx, "orchestrator failed to read latest chat response",
						slog.F("chat_id", chatID),
						slog.Error(err),
					)
					return fantasy.NewTextErrorResponse("internal error reading chat"), nil
				}
				result := orchestratorChatSummary(chat)
				result["latest_response"] = latest
				if _, lastError := subagentLastError(chat.LastError); lastError != "" {
					result["last_error"] = lastError
				}
				return toolJSONResponse(result), nil
			},
		),
	}
}

// chatOwnerContext returns a context acting as the chat owner so
// authorization-filtered queries such as GetChats see the owner's chats.
func (p *Server) chatOwnerContext(ctx context.Context, ownerID uuid.UUID) (context.Context, error) {
	owner, _, err := httpmw.UserRBACSubject(ctx, p.db, ownerID, rbac.ScopeAll)
	if err != nil {
		return nil, xerrors.Errorf("load chat owner authorization: %w", err)
	}
	return dbauthz.As(ctx, owner), nil
}

func orchestratorChatSummary(chat database.Chat) map[string]any {
	summary := map[string]any{
		"chat_id":    chat.ID.String(),
		"title":      chat.Title,
		"status":     string(chat.Status),
		"archived":   chat.Archived,
		"created_at": chat.CreatedAt.Format(time.RFC3339),
		"updated_at": chat.UpdatedAt.Format(time.RFC3339),
	}
	if chat.WorkspaceID.Valid {
		summary["workspace_id"] = chat.WorkspaceID.UUID.String()
	}
	if chat.Summary.Valid && chat.Summary.String != "" {
		summary["summary"] = chat.Summary.String
	}
	if chat.LastTurnSummary.Valid && chat.LastTurnSummary.String != "" {
		summary["last_turn_summary"] = chat.LastTurnSummary.String
	}
	return summary
}

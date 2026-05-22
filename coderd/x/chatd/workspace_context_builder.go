package chatd

import (
	"context"
	"database/sql"
	"sync"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// errWorkspaceContextUnavailable is returned by buildWorkspaceContext
// when there is nothing safe to persist for the current committed
// metadata, e.g. the chat has no bound workspace agent or the agent is
// no longer resolvable. Callers treat it as an expected exit.
var errWorkspaceContextUnavailable = xerrors.New("workspace context unavailable")

// buildWorkspaceContext fetches workspace context for the chat's
// bound workspace agent and returns durable chatstate.Message values
// for the generation action to commit. It returns
// errWorkspaceContextUnavailable when there is nothing safe to
// persist for the current committed metadata.
func (server *Server) buildWorkspaceContext(
	ctx context.Context,
	input workspaceContextBuildInput,
) (workspaceContextBuildResult, error) {
	chat := input.Chat
	if !chat.WorkspaceID.Valid || !chat.AgentID.Valid {
		return workspaceContextBuildResult{}, errWorkspaceContextUnavailable
	}
	logger := server.logger.With(
		slog.F("chat_id", chat.ID),
		slog.F("owner_id", chat.OwnerID),
	)

	// Build a per-call workspace context with the latest committed
	// chat snapshot so getWorkspaceAgent and getWorkspaceConn dial
	// the agent we actually want to fetch context from.
	currentChat := chat
	var chatStateMu sync.Mutex
	wsCtx := turnWorkspaceContext{
		server:           server,
		chatStateMu:      &chatStateMu,
		currentChat:      &currentChat,
		loadChatSnapshot: server.db.GetChatByID,
	}
	defer wsCtx.close()

	parts, expectedAgentID := server.fetchContextForBuild(ctx, chat, &wsCtx, logger)
	// If the workspace or agent is gone, fall back to no-op so the
	// generation action exits without committing stale context.
	if expectedAgentID == uuid.Nil {
		return workspaceContextBuildResult{}, errWorkspaceContextUnavailable
	}

	hasContent := false
	hasContextFilePart := false
	for _, part := range parts {
		if part.Type == codersdk.ChatMessagePartTypeContextFile {
			hasContextFilePart = true
			if part.ContextFileContent != "" {
				hasContent = true
			}
		}
	}

	agentID := uuid.NullUUID{UUID: expectedAgentID, Valid: true}

	// If we have no content but the agent is known, commit a blank
	// context-file marker (sentinel) so subsequent turns skip the
	// workspace-agent dial and the decision helper observes the
	// attempt in committed history. This applies whether the
	// workspace connection succeeded but returned no AGENTS.md, or
	// the agent's context config fetch failed: in both cases we
	// have a known agent and committing a sentinel breaks the
	// otherwise-infinite decision loop.
	if !hasContent {
		if !hasContextFilePart {
			parts = append([]codersdk.ChatMessagePart{{
				Type:               codersdk.ChatMessagePartTypeContextFile,
				ContextFileAgentID: agentID,
			}}, parts...)
		}
	}

	content, err := chatprompt.MarshalParts(parts)
	if err != nil {
		return workspaceContextBuildResult{}, xerrors.Errorf("marshal workspace context parts: %w", err)
	}

	modelConfigID := chat.LastModelConfigID
	msg := chatstate.Message{
		Role:           database.ChatMessageRoleUser,
		Content:        content,
		Visibility:     database.ChatMessageVisibilityBoth,
		ModelConfigID:  uuid.NullUUID{UUID: modelConfigID, Valid: modelConfigID != uuid.Nil},
		ContentVersion: chatprompt.CurrentContentVersion,
		APIKeyID:       sql.NullString{String: input.ActiveAPIKeyID, Valid: input.ActiveAPIKeyID != ""},
	}

	// Update the cache column so subsequent turns can read the last
	// injected context without scanning messages. This is a
	// best-effort write that does not mutate chat history; the
	// generation action separately commits the durable message
	// below.
	stripped := make([]codersdk.ChatMessagePart, len(parts))
	copy(stripped, parts)
	for i := range stripped {
		stripped[i].StripInternal()
	}
	server.updateLastInjectedContext(ctx, chat.ID, stripped)

	return workspaceContextBuildResult{Messages: []chatstate.Message{msg}}, nil
}

// fetchContextForBuild fetches workspace context parts from the
// agent, returning the parts to persist. expectedAgentID is the agent
// ID the fetch was bound to, or uuid.Nil if the agent could not be
// resolved.
func (server *Server) fetchContextForBuild(
	ctx context.Context,
	chat database.Chat,
	wsCtx *turnWorkspaceContext,
	logger slog.Logger,
) (parts []codersdk.ChatMessagePart, expectedAgentID uuid.UUID) {
	agent, agentParts, _, _ := server.fetchWorkspaceContext(
		ctx, chat, wsCtx.getWorkspaceAgent,
		func(instructionCtx context.Context) (workspacesdk.AgentConn, error) {
			if _, _, err := wsCtx.workspaceAgentIDForConn(instructionCtx); err != nil {
				return nil, err
			}
			return wsCtx.getWorkspaceConn(instructionCtx)
		},
	)
	if agent == nil {
		// fetchWorkspaceContext returns nil for the agent when the
		// chat has no valid workspace or the agent lookup fails.
		logger.Debug(ctx, "workspace context build: workspace agent not resolvable")
		return nil, uuid.Nil
	}
	return agentParts, agent.ID
}

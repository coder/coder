package chatd

import (
	"context"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
)

// memoryScopeStatus reports whether a chat has a memory scope.
type memoryScopeStatus int

const (
	// memoryScopeUnavailable covers chats outside a project, subagents, the
	// disabled experiment, and transient lookup failures.
	memoryScopeUnavailable memoryScopeStatus = iota
	memoryScopeAvailable
)

// resolveMemoryScope returns the durable-memory store available to a chat.
// Only root chats inside a project have memory.
func (p *Server) resolveMemoryScope(ctx context.Context, chat database.Chat) (chattool.MemoryStore, chattool.MemoryScope, memoryScopeStatus) {
	if !p.experiments.Enabled(codersdk.ExperimentChatProjects) {
		return nil, chattool.MemoryScope{}, memoryScopeUnavailable
	}
	if chat.ParentChatID.Valid || !chat.ProjectID.Valid {
		return nil, chattool.MemoryScope{}, memoryScopeUnavailable
	}
	project, err := p.db.GetChatProjectByID(ctx, chat.ProjectID.UUID)
	if err != nil {
		p.logger.Debug(ctx, "failed to load chat project for memory scope", slog.F("chat_id", chat.ID), slog.Error(err))
		return nil, chattool.MemoryScope{}, memoryScopeUnavailable
	}
	scope := chattool.MemoryScope{Label: project.Name}
	return chattool.NewProjectMemoryStore(p.db, chat.ProjectID.UUID, chat.OrganizationID, chat.ID, chat.OwnerID), scope, memoryScopeAvailable
}

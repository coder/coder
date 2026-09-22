package chatd

import (
	"context"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
)

// resolveProjectMemory returns the durable-memory store and project name for
// a chat. Only root chats inside a project have memory; chats outside a
// project, subagents, the disabled experiment, and transient lookup failures
// all report ok=false.
func (p *Server) resolveProjectMemory(ctx context.Context, chat database.Chat) (store chattool.MemoryStore, projectName string, ok bool) {
	if !p.experiments.Enabled(codersdk.ExperimentChatProjects) {
		return nil, "", false
	}
	if chat.ParentChatID.Valid || !chat.ProjectID.Valid {
		return nil, "", false
	}
	project, err := p.db.GetChatProjectByID(ctx, chat.ProjectID.UUID)
	if err != nil {
		p.logger.Debug(ctx, "failed to load chat project for memory", slog.F("chat_id", chat.ID), slog.Error(err))
		return nil, "", false
	}
	return chattool.NewProjectMemoryStore(p.db, chat.ProjectID.UUID, chat.OrganizationID, chat.ID, chat.OwnerID), project.Name, true
}

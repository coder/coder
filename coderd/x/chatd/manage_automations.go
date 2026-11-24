package chatd

import (
	"context"

	"charm.land/fantasy"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatautomation"
)

// manageAutomationsAvailable reports whether a root chat's turn offers the
// manage_automations tool: in a non-archived chat that no automation created,
// outside plan mode, while the chat-automations experiment is on for the
// owner. Explore mode exists only for sub-agent chats, which never get root
// chat tools.
func (p *Server) manageAutomationsAvailable(ctx context.Context, chat database.Chat, isPlanModeTurn bool) bool {
	return !chat.Archived && !chat.AutomationID.Valid && !isPlanModeTurn &&
		p.automationsEnabled != nil && p.automationsEnabled(ctx, chat.OwnerID)
}

func (p *Server) manageAutomationsTool(chat database.Chat) fantasy.AgentTool {
	return chatautomation.Tool(chatautomation.ToolOptions{
		DB:     p.db,
		Chat:   chat,
		Audit:  toolAuditor[database.ChatAutomation](p, chat),
		Logger: p.logger,
		Clock:  p.clock,
	})
}

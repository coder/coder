package chatd

import (
	"context"

	"charm.land/fantasy"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/x/chatd/chatautomation"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
)

// manageAutomationsAvailable reports whether a root chat's turn offers the
// manage_automations tool: in a non-archived chat that no automation created,
// while the chat-automations experiment is on for the owner. Plan mode turns
// drop it because builtinPlanToolAllowed does not list it, and explore mode
// exists only for sub-agent chats, which get no root chat tools.
func (p *Server) manageAutomationsAvailable(ctx context.Context, chat database.Chat) bool {
	return !chat.Archived && !chat.AutomationID.Valid &&
		p.automationsEnabled != nil && p.automationsEnabled(ctx, chat.OwnerID)
}

func (p *Server) manageAutomationsTool(chat database.Chat) fantasy.AgentTool {
	return chatautomation.Tool(chatautomation.ToolOptions{
		DB:     p.db,
		Chat:   chat,
		Audit:  toolAuditor[database.ChatAutomation](p, chat),
		Events: chatEventSource{server: p, chat: chat},
		Logger: p.logger,
		Clock:  p.clock,
	})
}

// chatEventSource gives the tool the MCP servers that its chat's turns use,
// with the owner's credentials.
type chatEventSource struct {
	server *Server
	chat   database.Chat
}

func (s chatEventSource) Servers(ctx context.Context) ([]database.MCPServerConfig, error) {
	//nolint:gocritic // A turn reads the chat's MCP servers as chatd too.
	return s.server.effectiveMCPServerConfigs(dbauthz.AsChatd(ctx), s.chat)
}

func (s chatEventSource) Open(ctx context.Context, cfg database.MCPServerConfig) (*mcpclient.EventsSession, error) {
	//nolint:gocritic // Opening a session reads and refreshes the owner's token as chatd.
	conn, err := s.server.openEvents(dbauthz.AsChatd(ctx), cfg, s.chat.OwnerID)
	if err != nil {
		return nil, err
	}
	return conn.session, nil
}

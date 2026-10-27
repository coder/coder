package chatd

import (
	"context"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
)

// ErrMCPServerNotAvailable means the chat owner cannot use the MCP server
// in the chat's organization: it does not exist, it is disabled, or the
// owner cannot read it.
var ErrMCPServerNotAvailable = xerrors.New("MCP server is not available for this chat")

// MCPAppSession connects to one org MCP server for the browser requests
// of MCP Apps (mcp-apps experiment), as the chat owner. ctx must carry
// the owner's authorization: the server must be enabled, in the chat's
// organization and readable by the owner. That is the check of a new
// turn's MCP selection, so a server the user just turned on in the
// composer works before the next message. The caller must close the
// session.
func (p *Server) MCPAppSession(
	ctx context.Context,
	chat database.Chat,
	configID uuid.UUID,
) (*mcpclient.AppSession, database.MCPServerConfig, error) {
	configs, err := p.db.GetEnabledMCPServerConfigsByOrganizationAndIDs(ctx,
		database.GetEnabledMCPServerConfigsByOrganizationAndIDsParams{
			OrganizationID: chat.OrganizationID,
			IDs:            []uuid.UUID{configID},
		})
	if err != nil {
		return nil, database.MCPServerConfig{}, xerrors.Errorf("get MCP server config: %w", err)
	}
	if len(configs) != 1 {
		return nil, database.MCPServerConfig{}, ErrMCPServerNotAvailable
	}
	cfg := configs[0]

	// Tokens are read as chatd, as a turn reads them. The caller's
	// context already passed the owner's config read above.
	//nolint:gocritic // chatd reads the chat owner's MCP tokens for a turn too.
	sysCtx := dbauthz.AsChatd(ctx)
	tokens, err := p.db.GetMCPServerUserTokensByUserID(sysCtx, chat.OwnerID)
	if err != nil {
		return nil, cfg, xerrors.Errorf("get MCP server user tokens: %w", err)
	}
	tokens, err = p.refreshExpiredMCPTokens(sysCtx, p.logger, configs, tokens)
	if err != nil {
		return nil, cfg, err
	}
	srv, err := mcpclient.ServerFromConfig(cfg)
	if err != nil {
		return nil, cfg, xerrors.Errorf("convert MCP server config: %w", err)
	}
	session, err := mcpclient.ConnectAppSession(
		sysCtx, p.logger.Named("mcp_apps"), srv, tokens, chat.OwnerID,
		p.oidcTokenSource, chatprovider.CoderHeaders(chat), p.mcpHTTPClient,
	)
	if err != nil {
		return nil, cfg, xerrors.Errorf("connect to MCP server %q: %w", cfg.Slug, err)
	}
	return session, cfg, nil
}

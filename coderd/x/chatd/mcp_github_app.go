package chatd

import (
	"context"
	"database/sql"
	"errors"
	"slices"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
	"github.com/coder/coder/v2/coderd/x/githubapp"
	"github.com/coder/coder/v2/codersdk"
)

// agentAttachedMCPServers returns the MCP servers currently attached to the
// agent whose user is ownerID. It returns nil when ownerID is not an
// agent's user. An agent's stored credentials are only lent to these
// servers, so a chat that lists another server, or a server the owner
// detached, never receives them.
func (p *Server) agentAttachedMCPServers(ctx context.Context, ownerID uuid.UUID) (map[uuid.UUID]struct{}, error) {
	//nolint:gocritic // chatd reads the chat owner's agent only to limit which servers get its credentials.
	project, err := p.db.GetChatProjectByAgentUserID(dbauthz.AsChatd(ctx), ownerID)
	if errors.Is(err, sql.ErrNoRows) {
		//nolint:nilnil // A nil set means the owner is a person, which callers treat as no limit.
		return nil, nil
	}
	if err != nil {
		return nil, xerrors.Errorf("get agent for chat owner: %w", err)
	}
	attached := make(map[uuid.UUID]struct{}, len(project.MCPServerIDs))
	for _, id := range project.MCPServerIDs {
		attached[id] = struct{}{}
	}
	return attached, nil
}

// onlyAttachedMCPTokens drops tokens for servers outside attached. A nil
// attached set, for a person's chat, keeps every token.
func onlyAttachedMCPTokens(attached map[uuid.UUID]struct{}, tokens []database.MCPServerUserToken) []database.MCPServerUserToken {
	if attached == nil {
		return tokens
	}
	return slices.DeleteFunc(tokens, func(tok database.MCPServerUserToken) bool {
		_, ok := attached[tok.MCPServerConfigID]
		return !ok
	})
}

// githubAppMCPTokens replaces the owner's tokens for user_api_key servers
// where the owner, an agent's user, authenticates as a GitHub App. Each such
// server attached to the agent gets a freshly minted installation token.
// Servers whose token could not be minted are returned as failures, so the
// caller skips them instead of reporting a missing token. attached comes
// from agentAttachedMCPServers; nil means the owner is not an agent's user.
func (p *Server) githubAppMCPTokens(
	ctx context.Context,
	logger slog.Logger,
	ownerID uuid.UUID,
	attached map[uuid.UUID]struct{},
	configs []database.MCPServerConfig,
	tokens []database.MCPServerUserToken,
) ([]database.MCPServerUserToken, map[uuid.UUID]mcpclient.ConnectSummary) {
	if p.githubAppTokens == nil || attached == nil {
		return tokens, nil
	}
	userAPIKeyConfigs := make(map[uuid.UUID]database.MCPServerConfig)
	for _, cfg := range configs {
		if _, ok := attached[cfg.ID]; !ok {
			continue
		}
		if cfg.Enabled && cfg.AuthType == codersdk.MCPServerAuthTypeUserAPIKey {
			userAPIKeyConfigs[cfg.ID] = cfg
		}
	}
	if len(userAPIKeyConfigs) == 0 {
		return tokens, nil
	}
	creds, err := p.db.GetMCPServerGitHubAppCredentialsByUserID(ctx, ownerID)
	if err != nil {
		logger.Warn(ctx, "failed to load GitHub App credentials for MCP servers", slog.Error(err))
		return tokens, nil
	}
	failures := map[uuid.UUID]mcpclient.ConnectSummary{}
	for _, stored := range creds {
		cfg, ok := userAPIKeyConfigs[stored.MCPServerConfigID]
		if !ok {
			continue
		}
		// Drop any pasted token so only the app's identity is used.
		tokens = slices.DeleteFunc(tokens, func(tok database.MCPServerUserToken) bool {
			return tok.MCPServerConfigID == cfg.ID
		})
		minted, err := p.mintGitHubAppMCPToken(ctx, cfg, stored)
		if err != nil {
			logger.Warn(ctx, "failed to mint GitHub App token for MCP server",
				slog.F("server_slug", cfg.Slug),
				slog.F("github_app_id", stored.AppID),
				slog.F("github_installation_id", stored.InstallationID),
				slog.Error(err),
			)
			failures[cfg.ID] = mcpclient.ConnectSummary{
				ConfigID: cfg.ID,
				Slug:     cfg.Slug,
				Outcome:  mcpclient.ConnectOutcomeError,
				Error:    "could not get a GitHub App installation token: " + githubAppErrorText(err),
			}
			continue
		}
		tokens = append(tokens, database.MCPServerUserToken{
			MCPServerConfigID: cfg.ID,
			UserID:            ownerID,
			AccessToken:       minted.Token,
			TokenType:         "Bearer",
			Expiry:            sql.NullTime{Time: minted.ExpiresAt, Valid: true},
		})
	}
	return tokens, failures
}

func (p *Server) mintGitHubAppMCPToken(ctx context.Context, cfg database.MCPServerConfig, stored database.MCPServerGitHubAppCredential) (githubapp.Token, error) {
	cred, err := githubapp.CredentialFor(cfg, stored)
	if err != nil {
		return githubapp.Token{}, err
	}
	return p.githubAppTokens.Token(ctx, cred)
}

// githubAppErrorText is GitHub's own explanation when there is one.
func githubAppErrorText(err error) string {
	var ghErr *githubapp.Error
	if errors.As(err, &ghErr) {
		return ghErr.Error()
	}
	return err.Error()
}

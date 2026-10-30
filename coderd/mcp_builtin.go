package coderd

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
	"github.com/coder/coder/v2/coderd/x/mcpbuiltin"
	"github.com/coder/coder/v2/codersdk"
)

// newBuiltinMCPHandler builds Coder's built-in MCP servers. The endpoints
// take the upstream API token as their bearer token and need no Coder
// session; signed Coder identity headers identify the calling chat.
func (api *API) newBuiltinMCPHandler() *mcpbuiltin.Handler {
	return mcpbuiltin.New(mcpbuiltin.Options{
		Logger:          api.Logger,
		AccessURL:       api.AccessURL,
		HTTPClient:      api.HTTPClient,
		AllowedPrefixes: api.MCPAllowedPrivateCIDRs,
		Identity:        mcpbuiltin.HMACVerifier{Secrets: api.builtinMCPSigningSecrets},
		Conversations:   builtinMCPConversations{db: api.Database},
	})
}

// builtinMCPExperimentGate hides the built-in MCP servers unless the
// chat projects experiment is on.
func (api *API) builtinMCPExperimentGate() func(http.Handler) http.Handler {
	return httpmw.RequireExperimentWithDevBypass(api.Experiments, codersdk.ExperimentChatProjects)
}

// chatMCPHTTPClient is the MCP client chatd uses. Requests for the
// built-in servers at the deployment's access URL are served in-process,
// so they work even when the access URL resolves to an address the SSRF
// guard blocks. All other requests dial out through the guard.
func (api *API) chatMCPHTTPClient() *http.Client {
	if api.AccessURL == nil || api.builtinMCP == nil {
		return api.mcpHTTPClient
	}
	return mcpclient.WithInProcessRoute(api.mcpHTTPClient, api.builtinMCP.InProcessRoute(api.AccessURL, api.builtinMCPExperimentGate()))
}

// builtinMCPSigningSecrets returns the signing secrets of the MCP servers
// in the chat's organization that point at the named built-in server on
// this deployment's access URL.
func (api *API) builtinMCPSigningSecrets(ctx context.Context, server string, chatID uuid.UUID) ([]string, error) {
	if api.AccessURL == nil {
		return nil, nil
	}
	// The built-in endpoints have no Coder session. The HMAC signature
	// being verified is the authentication, so the lookup runs as chatd,
	// which may read chats and MCP server configs.
	chatdCtx := dbauthz.AsChatd(ctx) //nolint:gocritic // The request signature, not a session, authenticates this lookup.
	chat, err := api.Database.GetChatByID(chatdCtx, chatID)
	if err != nil {
		if httpapi.Is404Error(err) {
			return nil, nil
		}
		return nil, xerrors.Errorf("get chat: %w", err)
	}
	configs, err := api.Database.GetMCPServerConfigsByOrganization(chatdCtx, chat.OrganizationID)
	if err != nil {
		return nil, xerrors.Errorf("get MCP server configs: %w", err)
	}
	endpoint := mcpbuiltin.EndpointPath(server)
	var secrets []string
	for _, cfg := range configs {
		if !cfg.ForwardCoderHeaders || cfg.SigningSecret == "" {
			continue
		}
		u, err := url.Parse(cfg.Url)
		if err != nil || !sameOrigin(u, api.AccessURL) || strings.TrimSuffix(u.Path, "/") != endpoint {
			continue
		}
		secrets = append(secrets, cfg.SigningSecret)
	}
	return secrets, nil
}

// builtinMCPConversations resolves a verified calling chat to the trigger
// conversation that started it.
type builtinMCPConversations struct {
	db database.Store
}

var _ mcpbuiltin.ConversationResolver = builtinMCPConversations{}

// SlackThread implements mcpbuiltin.ConversationResolver. A subchat's own
// conversation wins; otherwise the signed chat and then its root chat are
// tried, so a delegated chat replies in the thread that started the tree.
func (c builtinMCPConversations) SlackThread(ctx context.Context, identity mcpbuiltin.Identity) (mcpbuiltin.SlackThread, bool, error) {
	// The caller's identity is already proven by the request signature,
	// so the lookup runs as chatd, which may read any chat.
	ctx = dbauthz.AsChatd(ctx) //nolint:gocritic // The verified request signature identifies the chat.

	var candidates []uuid.UUID
	if identity.SubchatID.Valid {
		sub, err := c.db.GetChatByID(ctx, identity.SubchatID.UUID)
		switch {
		case err == nil:
			// The signature covers both headers, but only trust a
			// subchat that really belongs to the signed chat.
			if sub.ParentChatID.UUID == identity.ChatID || sub.RootChatID.UUID == identity.ChatID {
				candidates = append(candidates, sub.ID)
			}
		case !httpapi.Is404Error(err):
			return mcpbuiltin.SlackThread{}, false, xerrors.Errorf("get subchat: %w", err)
		}
	}
	candidates = append(candidates, identity.ChatID)
	chat, err := c.db.GetChatByID(ctx, identity.ChatID)
	switch {
	case err == nil:
		if chat.RootChatID.Valid && chat.RootChatID.UUID != chat.ID {
			candidates = append(candidates, chat.RootChatID.UUID)
		}
	case httpapi.Is404Error(err):
		return mcpbuiltin.SlackThread{}, false, nil
	default:
		return mcpbuiltin.SlackThread{}, false, xerrors.Errorf("get chat: %w", err)
	}

	for _, chatID := range candidates {
		conversation, err := c.db.GetChatProjectTriggerConversationByChatID(ctx, chatID)
		if err != nil {
			if httpapi.Is404Error(err) {
				continue
			}
			return mcpbuiltin.SlackThread{}, false, xerrors.Errorf("get chat conversation: %w", err)
		}
		if conversation.Source != string(codersdk.ChatProjectTriggerSourceSlack) {
			// The newest conversation decides; a chat continued from
			// another source has no Slack thread to default to.
			return mcpbuiltin.SlackThread{}, false, nil
		}
		thread, err := mcpbuiltin.ParseSlackConversationKey(conversation.ConversationKey)
		if err != nil {
			return mcpbuiltin.SlackThread{}, false, xerrors.Errorf("parse chat conversation: %w", err)
		}
		return thread, true, nil
	}
	return mcpbuiltin.SlackThread{}, false, nil
}

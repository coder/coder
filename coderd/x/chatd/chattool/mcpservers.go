package chattool

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/slack-go/slack"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
)

// MCPProposalCancelActionID is the Slack block-actions action id of the
// Cancel button on MCP server proposal cards. slackd handles clicks on
// it over Socket Mode.
const MCPProposalCancelActionID = "mcp_proposal_cancel"

// MCPProposalPendingColor is the attachment bar color of a pending MCP
// server proposal card (Slack aubergine).
const MCPProposalPendingColor = "#4A154B"

// MCPServerProposalRequest is the proposed MCP server config persisted
// on an mcp_server_proposals row. It is written by the
// propose_mcp_server tool and consumed by the slackd accept handler
// when the proposal is accepted.
type MCPServerProposalRequest struct {
	DisplayName  string `json:"display_name"`
	Slug         string `json:"slug"`
	Description  string `json:"description,omitempty"`
	Instructions string `json:"instructions,omitempty"`
	IconURL      string `json:"icon_url,omitempty"`
	URL          string `json:"url"`
	Transport    string `json:"transport"`
	AuthType     string `json:"auth_type"`

	OAuth2ClientID     string `json:"oauth2_client_id,omitempty"`
	OAuth2ClientSecret string `json:"oauth2_client_secret,omitempty"`
	// OAuth2ClientSecretPlaceholder is a short form-field placeholder for a
	// secret the requester must supply on the proposal review page.
	OAuth2ClientSecretPlaceholder string `json:"oauth2_client_secret_placeholder,omitempty"`
	OAuth2AuthURL                 string `json:"oauth2_auth_url,omitempty"`
	OAuth2TokenURL                string `json:"oauth2_token_url,omitempty"`
	OAuth2Scopes                  string `json:"oauth2_scopes,omitempty"`

	APIKeyHeader             string            `json:"api_key_header,omitempty"`
	APIKeyValue              string            `json:"api_key_value,omitempty"`
	APIKeyValuePlaceholder   string            `json:"api_key_value_placeholder,omitempty"`
	CustomHeaders            map[string]string `json:"custom_headers,omitempty"`
	CustomHeaderPlaceholders map[string]string `json:"custom_header_placeholders,omitempty"`

	ToolAllowList []string `json:"tool_allow_list,omitempty"`
	ToolDenyList  []string `json:"tool_deny_list,omitempty"`

	Disabled bool `json:"disabled,omitempty"`
}

// MCPServerProposalSecret is a secret-valued propose_mcp_server argument.
// Exactly one of Value or Placeholder must be set when the argument is used.
type MCPServerProposalSecret struct {
	Value       string `json:"value,omitempty" description:"Concrete secret value, only when it is already available to the agent"`
	Placeholder string `json:"placeholder,omitempty" description:"Short form-field placeholder showing the expected credential format. Shown inside the review-page input, so keep it brief (e.g. \"Bearer gh_xxx\"). Do not put setup instructions here; put those in instructions."`
}

// MCPServerToolsOptions configures the MCP server management tools for
// a slackd-bound chat.
type MCPServerToolsOptions struct {
	DB     database.Store
	Logger slog.Logger

	ChatID      uuid.UUID
	ChatOwnerID uuid.UUID

	// AccessURL is the deployment access URL, used to build OAuth2
	// connect URLs and proposal review URLs.
	AccessURL *url.URL
	// ValidateOAuth2Discovery verifies that automatic OAuth2
	// configuration is supported before a proposal is posted. It must not
	// perform Dynamic Client Registration or other persistent mutations.
	ValidateOAuth2Discovery func(ctx context.Context, serverURL string) error

	// ChatMCPServerIDs returns the chat's currently enabled MCP server
	// IDs, fresh from the database.
	ChatMCPServerIDs func(ctx context.Context) ([]uuid.UUID, error)
	// EnableMCPServer appends a server to the chat's MCP server IDs
	// through chatd's validated update path. Enabling an
	// already-enabled server is a no-op.
	EnableMCPServer func(ctx context.Context, serverID uuid.UUID) error

	// Slack thread the chat is bound to; propose_mcp_server posts the
	// confirmation card there.
	SlackAPI SlackAPI
	Channel  string
	ThreadTS string
	// SlackSenderID is the Slack user id of the sender of the message
	// that started the current turn. ResolveSlackUser maps it to the
	// Coder requester stored on a proposal. Neither is LLM-supplied.
	SlackSenderID    string
	ResolveSlackUser func(ctx context.Context, slackUserID string) (uuid.UUID, error)

	// SharedMode marks chats owned by the fallback chat owner because
	// the Slack sender is not linked to a Coder user. propose_mcp_server
	// then returns an error directing the agent to ask the user to
	// connect their Coder account to Slack.
	SharedMode bool
}

// MCPServerTools returns all MCP server management tools, including the
// mutating ones.
func MCPServerTools(opts MCPServerToolsOptions) []fantasy.AgentTool {
	return append([]fantasy.AgentTool{
		enableMCPServer(opts),
		proposeMCPServer(opts),
	}, MCPServerReadOnlyTools(opts)...)
}

// MCPServerReadOnlyTools returns only the MCP server tools without side
// effects. Used on plan-mode turns.
func MCPServerReadOnlyTools(opts MCPServerToolsOptions) []fantasy.AgentTool {
	return []fantasy.AgentTool{listMCPServers(opts)}
}

// mcpServerView is the trimmed MCP server config view returned to the
// model. It omits admin and secret metadata.
type mcpServerView struct {
	ID                 string   `json:"id"`
	DisplayName        string   `json:"display_name"`
	Slug               string   `json:"slug"`
	Description        string   `json:"description,omitempty"`
	URL                string   `json:"url"`
	Transport          string   `json:"transport"`
	AuthType           string   `json:"auth_type"`
	Availability       string   `json:"availability"`
	Enabled            bool     `json:"enabled"`
	EnabledForThisChat bool     `json:"enabled_for_this_chat"`
	Authenticated      bool     `json:"authenticated"`
	ToolAllowList      []string `json:"tool_allow_list,omitempty"`
	ToolDenyList       []string `json:"tool_deny_list,omitempty"`
}

// MCPOAuth2ConnectURL builds the browser URL that starts the OAuth2
// connect flow for an MCP server config. Duplicated from the coderd
// route layout on purpose; do not refactor coderd/mcp.go to share it.
func MCPOAuth2ConnectURL(accessURL *url.URL, configID uuid.UUID) string {
	if accessURL == nil {
		return ""
	}
	return strings.TrimSuffix(accessURL.String(), "/") +
		"/api/experimental/mcp/servers/" + configID.String() + "/oauth2/connect"
}

// MCPProposalReviewURL builds the dashboard URL where the chat owner
// reviews an MCP server proposal.
func MCPProposalReviewURL(accessURL *url.URL, proposalID uuid.UUID) string {
	if accessURL == nil {
		return ""
	}
	return strings.TrimSuffix(accessURL.String(), "/") +
		"/agents/settings/mcp-proposals/" + proposalID.String()
}

// MCPAuthConnected reports whether auth for the config is usable by the
// user owning tokens. Non-oauth2 auth types carry static credentials
// (or none), so they always count as connected. For oauth2 the user's
// token must exist and either be refreshable or not yet expired. The
// logic mirrors coderd's AuthConnected resolution without the active
// token refresh.
func MCPAuthConnected(config database.MCPServerConfig, token *database.MCPServerUserToken, now time.Time) bool {
	if config.AuthType != "oauth2" {
		return true
	}
	if token == nil {
		return false
	}
	if token.RefreshToken != "" {
		return true
	}
	return !token.Expiry.Valid || token.Expiry.Time.After(now)
}

// visibleMCPServerConfigs returns the enabled global configs plus the
// chat owner's personal configs, matching the list API's visibility.
func visibleMCPServerConfigs(ctx context.Context, db database.Store, ownerID uuid.UUID) ([]database.MCPServerConfig, error) {
	//nolint:gocritic // Chat tools need daemon-scoped deployment-config reads; visibility is scoped to the chat owner by the query.
	return db.GetEnabledMCPServerConfigs(dbauthz.AsChatd(ctx), ownerID)
}

// ownerMCPTokens returns the chat owner's MCP OAuth2 tokens keyed by
// config id.
func ownerMCPTokens(ctx context.Context, db database.Store, ownerID uuid.UUID) (map[uuid.UUID]database.MCPServerUserToken, error) {
	//nolint:gocritic // Token existence for the chat owner requires daemon-scoped reads.
	tokens, err := db.GetMCPServerUserTokensByUserID(dbauthz.AsChatd(ctx), ownerID)
	if err != nil {
		return nil, err
	}
	byConfig := make(map[uuid.UUID]database.MCPServerUserToken, len(tokens))
	for _, tok := range tokens {
		byConfig[tok.MCPServerConfigID] = tok
	}
	return byConfig, nil
}

// buildMCPServerView converts a config into the trimmed model view.
func buildMCPServerView(
	config database.MCPServerConfig,
	enabledIDs map[uuid.UUID]struct{},
	tokens map[uuid.UUID]database.MCPServerUserToken,
) mcpServerView {
	_, enabledForChat := enabledIDs[config.ID]
	if config.Availability == "force_on" && config.Enabled {
		enabledForChat = true
	}
	var token *database.MCPServerUserToken
	if tok, ok := tokens[config.ID]; ok {
		token = &tok
	}
	return mcpServerView{
		ID:                 config.ID.String(),
		DisplayName:        config.DisplayName,
		Slug:               config.Slug,
		Description:        config.Description,
		URL:                config.Url,
		Transport:          config.Transport,
		AuthType:           config.AuthType,
		Availability:       config.Availability,
		Enabled:            config.Enabled,
		EnabledForThisChat: enabledForChat,
		Authenticated:      MCPAuthConnected(config, token, time.Now()),
		ToolAllowList:      config.ToolAllowList,
		ToolDenyList:       config.ToolDenyList,
	}
}

type listMCPServersArgs struct{}

func listMCPServers(opts MCPServerToolsOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"list_mcp_servers",
		`List the MCP server configs available to this chat's owner, with per-chat enablement (enabled_for_this_chat) and authentication status (authenticated). Use it before enable_mcp_server to find a server, or to check whether a server the user asked about already exists.`,
		func(ctx context.Context, _ listMCPServersArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			configs, err := visibleMCPServerConfigs(ctx, opts.DB, opts.ChatOwnerID)
			if err != nil {
				return toolResponse(map[string]any{"error": "list mcp servers: " + err.Error()}), nil
			}
			enabledIDs, err := opts.chatEnabledIDSet(ctx)
			if err != nil {
				return toolResponse(map[string]any{"error": "load chat mcp servers: " + err.Error()}), nil
			}
			tokens, err := ownerMCPTokens(ctx, opts.DB, opts.ChatOwnerID)
			if err != nil {
				return toolResponse(map[string]any{"error": "load mcp auth status: " + err.Error()}), nil
			}
			views := make([]mcpServerView, 0, len(configs))
			for _, config := range configs {
				views = append(views, buildMCPServerView(config, enabledIDs, tokens))
			}
			return marshalToolResponse(map[string]any{"mcp_servers": views}), nil
		})
}

func (opts MCPServerToolsOptions) chatEnabledIDSet(ctx context.Context) (map[uuid.UUID]struct{}, error) {
	ids, err := opts.ChatMCPServerIDs(ctx)
	if err != nil {
		return nil, err
	}
	set := make(map[uuid.UUID]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	return set, nil
}

type enableMCPServerArgs struct {
	Server string `json:"server" description:"MCP server ID or slug (slug match is case-insensitive)"`
}

func enableMCPServer(opts MCPServerToolsOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"enable_mcp_server",
		`Enable an existing MCP server for the current chat. Its tools become available from the next generation step (your next turn). If the server requires authentication that is not connected yet, the result carries authenticated: false and, for oauth2 servers, a connect_url; share that URL with the user so they can authenticate.`,
		func(ctx context.Context, args enableMCPServerArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			ref := strings.TrimSpace(args.Server)
			if ref == "" {
				return toolResponse(map[string]any{"error": "server is required; call list_mcp_servers to see the available servers"}), nil
			}
			configs, err := visibleMCPServerConfigs(ctx, opts.DB, opts.ChatOwnerID)
			if err != nil {
				return toolResponse(map[string]any{"error": "list mcp servers: " + err.Error()}), nil
			}
			var config *database.MCPServerConfig
			for i, c := range configs {
				if strings.EqualFold(c.Slug, ref) || strings.EqualFold(c.ID.String(), ref) {
					config = &configs[i]
					break
				}
			}
			if config == nil {
				return toolResponse(map[string]any{"error": fmt.Sprintf("no MCP server matches %q; call list_mcp_servers to see the available servers", ref)}), nil
			}
			if !config.Enabled {
				return toolResponse(map[string]any{"error": fmt.Sprintf("MCP server %q is disabled on this deployment and cannot be enabled per chat", config.Slug)}), nil
			}
			if err := opts.EnableMCPServer(ctx, config.ID); err != nil {
				return toolResponse(map[string]any{"error": "enable mcp server: " + err.Error()}), nil
			}
			enabledIDs, err := opts.chatEnabledIDSet(ctx)
			if err != nil {
				// The server was enabled; only the fresh view failed.
				enabledIDs = map[uuid.UUID]struct{}{config.ID: {}}
			}
			tokens, err := ownerMCPTokens(ctx, opts.DB, opts.ChatOwnerID)
			if err != nil {
				return toolResponse(map[string]any{"error": "load mcp auth status: " + err.Error()}), nil
			}
			view := buildMCPServerView(*config, enabledIDs, tokens)
			result := map[string]any{
				"mcp_server": view,
				"note":       "The server's tools are available from the next generation step.",
			}
			if !view.Authenticated && config.AuthType == "oauth2" {
				result["connect_url"] = MCPOAuth2ConnectURL(opts.AccessURL, config.ID)
				result["note"] = "The server is enabled for this chat, but the user has not authenticated with it yet. Share connect_url with the user so they can complete the OAuth2 flow; its tools become usable afterwards."
			}
			return marshalToolResponse(result), nil
		})
}

type proposeMCPServerArgs struct {
	DisplayName  string `json:"display_name" description:"Human-readable server name shown in the confirmation card"`
	Slug         string `json:"slug" description:"URL-safe identifier, e.g. linear or github"`
	URL          string `json:"url" description:"The MCP server endpoint URL"`
	Description  string `json:"description,omitempty" description:"Optional short description"`
	Instructions string `json:"instructions,omitempty" description:"Concise step-by-step Markdown guide shown on the proposal review page; required when any credential uses a placeholder. Be exhaustive about what to do, but brief: no filler. Prefer direct clickable links over steps for navigating a web UI."`
	IconURL      string `json:"icon_url,omitempty" description:"Optional icon URL"`
	Transport    string `json:"transport,omitempty" description:"streamable_http (default) or sse"`
	AuthType     string `json:"auth_type,omitempty" description:"none (default), oauth2, api_key, or custom_headers"`

	OAuth2ClientID     string                  `json:"oauth2_client_id,omitempty" description:"Optional; omit for automatic discovery + Dynamic Client Registration"`
	OAuth2ClientSecret MCPServerProposalSecret `json:"oauth2_client_secret,omitempty" description:"Optional secret object containing exactly one of value or placeholder; only valid with complete manual OAuth2 metadata"`
	OAuth2AuthURL      string                  `json:"oauth2_auth_url,omitempty" description:"Optional; omit for automatic discovery"`
	OAuth2TokenURL     string                  `json:"oauth2_token_url,omitempty" description:"Optional; omit for automatic discovery"`
	OAuth2Scopes       string                  `json:"oauth2_scopes,omitempty" description:"Optional space-separated OAuth2 scopes"`

	APIKeyHeader  string                             `json:"api_key_header,omitempty" description:"Header name for api_key auth, e.g. Authorization"`
	APIKeyValue   MCPServerProposalSecret            `json:"api_key_value,omitempty" description:"Secret object containing exactly one of value or placeholder"`
	CustomHeaders map[string]MCPServerProposalSecret `json:"custom_headers,omitempty" description:"Header names mapped to secret objects containing exactly one of value or placeholder"`

	ToolAllowList []string `json:"tool_allow_list,omitempty" description:"Optional allow list of tool names"`
	ToolDenyList  []string `json:"tool_deny_list,omitempty" description:"Optional deny list of tool names"`

	Disabled bool `json:"disabled,omitempty" description:"Create the server disabled; defaults to enabled"`
}

func proposeMCPServer(opts MCPServerToolsOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"propose_mcp_server",
		`Propose a new MCP server for the user requesting it. This tool posts a confirmation card to the Slack thread and returns immediately; NOTHING is created until the requesting user reviews and accepts the proposal on a Coder page. Include a concise description for the Slack card and review page. Include any setup instructions in this tool call so they appear on the review page. The Slack card does NOT show setup instructions; instructions are shown only on the review page.

Accepting creates the server, enables it for this chat, and takes the user straight into authentication when needed. A follow-up [system] message reports the outcome, so end your turn and wait for it instead of assuming the result.

The server is created PERSONAL-SCOPED: it will only be available to the requesting user, not the whole deployment.

For oauth2 servers a server URL alone is usually enough (automatic discovery + Dynamic Client Registration). This tool validates automatic discovery before posting the proposal. If validation fails, use complete manual OAuth2 metadata. When manual OAuth2 metadata is required, use a placeholder for the client secret. For api_key and custom_headers authentication, use placeholders whenever the user must provide credentials. Never ask the user to paste credentials into Slack.

Credential placeholders are shown inside the review-page input fields, so keep them short and format-focused (good: "Bearer gh_xxx"; bad: "Enter your api key that you generated in GitHub settings. The required format is 'gh_xxx'."). Whenever a credential uses a placeholder, instructions must contain a concise step-by-step Markdown guide that is still exhaustive about how to obtain it. Prefer a direct clickable link when one exists; do not narrate how to click through a web UI. Never send those instructions as a separate Slack message; keep the proposal as the single source of truth.`,
		func(ctx context.Context, args proposeMCPServerArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if opts.SharedMode {
				msg := "this chat is in shared mode because the Slack user is not linked to a Coder account. Ask them to connect their Coder account to Slack, then ping you again so you can propose the MCP server on their behalf."
				if opts.AccessURL != nil {
					connectURL := strings.TrimRight(opts.AccessURL.String(), "/") + "/settings/external-auth"
					msg += " They can connect at " + connectURL + "."
				}
				return toolResponse(map[string]any{"error": msg}), nil
			}

			req, errMsg := buildMCPProposalRequest(args)
			if errMsg != "" {
				return toolResponse(map[string]any{"error": errMsg}), nil
			}
			if req.AuthType == "oauth2" && req.OAuth2ClientID == "" {
				const manualOAuth2Guidance = " Call propose_mcp_server again with oauth2_client_id, oauth2_auth_url, and oauth2_token_url for a manual OAuth2 configuration."
				if opts.ValidateOAuth2Discovery == nil {
					return toolResponse(map[string]any{
						"error": "oauth2 auto-discovery validation is unavailable." + manualOAuth2Guidance,
					}), nil
				}
				if err := opts.ValidateOAuth2Discovery(ctx, req.URL); err != nil {
					return toolResponse(map[string]any{
						"error": "oauth2 auto-discovery preflight failed: " + err.Error() + "." + manualOAuth2Guidance,
					}), nil
				}
			}

			requestJSON, err := json.Marshal(req)
			if err != nil {
				return toolResponse(map[string]any{"error": "encode proposal: " + err.Error()}), nil
			}
			if opts.SlackSenderID == "" || opts.ResolveSlackUser == nil {
				return toolResponse(map[string]any{"error": "the requesting Slack user could not be resolved"}), nil
			}
			requesterID, err := opts.ResolveSlackUser(ctx, opts.SlackSenderID)
			if err != nil {
				return toolResponse(map[string]any{"error": "resolve requesting Slack user: " + err.Error()}), nil
			}
			// Every Slack message is routed to the resolved sender's own
			// chat. Fail closed if that invariant is violated rather than
			// storing a requester who does not own the proposing chat.
			if requesterID != opts.ChatOwnerID {
				return toolResponse(map[string]any{"error": "the requesting Slack user does not own this chat"}), nil
			}

			proposalID := uuid.New()
			dialogTS, err := postMCPProposalCard(ctx, opts, proposalID, req)
			if err != nil {
				return slackErrorResponse(err), nil
			}

			//nolint:gocritic // Proposal rows belong to the chat; the chatd actor carries chat update access.
			_, err = opts.DB.InsertMCPServerProposal(dbauthz.AsChatd(ctx), database.InsertMCPServerProposalParams{
				ID:          proposalID,
				ChatID:      opts.ChatID,
				RequesterID: requesterID,
				Channel:     opts.Channel,
				ThreadTs:    opts.ThreadTS,
				MessageTs:   dialogTS,
				Request:     requestJSON,
			})
			if err != nil {
				// Best-effort: retract the card so a dead dialog does
				// not linger in the thread.
				if _, _, _, updateErr := opts.SlackAPI.UpdateMessageContext(ctx, opts.Channel, dialogTS,
					slack.MsgOptionText("This MCP server proposal could not be saved. Please try again.", false),
					slack.MsgOptionAttachments(),
					slack.MsgOptionBlocks(),
				); updateErr != nil {
					opts.Logger.Warn(ctx, "retract mcp proposal card after insert failure",
						slog.F("proposal_id", proposalID), slog.Error(updateErr))
				}
				return toolResponse(map[string]any{"error": "persist proposal: " + err.Error()}), nil
			}

			return marshalToolResponse(map[string]any{
				"ok":          true,
				"proposal_id": proposalID.String(),
				"dialog_ts":   dialogTS,
				"note":        "The proposal card was posted without setup instructions. Instructions are shown only on the review page. The MCP server is NOT created yet: end your turn and wait for the [system] message reporting whether the user accepted or rejected it.",
			}), nil
		})
}

// buildMCPProposalRequest validates and normalizes the tool arguments
// into the persisted request. It returns a non-empty error message on
// invalid input.
func buildMCPProposalRequest(args proposeMCPServerArgs) (MCPServerProposalRequest, string) {
	oauth2ClientSecret, oauth2ClientSecretPlaceholder, errMsg := normalizeMCPProposalSecret("oauth2_client_secret", args.OAuth2ClientSecret, true)
	if errMsg != "" {
		return MCPServerProposalRequest{}, errMsg
	}
	apiKeyValue, apiKeyValuePlaceholder, errMsg := normalizeMCPProposalSecret("api_key_value", args.APIKeyValue, true)
	if errMsg != "" {
		return MCPServerProposalRequest{}, errMsg
	}
	customHeaders := make(map[string]string)
	customHeaderPlaceholders := make(map[string]string)
	for header, secret := range args.CustomHeaders {
		value, placeholder, secretErr := normalizeMCPProposalSecret("custom_headers."+header, secret, false)
		if secretErr != "" {
			return MCPServerProposalRequest{}, secretErr
		}
		if value != "" {
			customHeaders[header] = value
		} else {
			customHeaderPlaceholders[header] = placeholder
		}
	}
	if len(customHeaders) == 0 {
		customHeaders = nil
	}
	if len(customHeaderPlaceholders) == 0 {
		customHeaderPlaceholders = nil
	}

	req := MCPServerProposalRequest{
		DisplayName:  strings.TrimSpace(args.DisplayName),
		Slug:         strings.TrimSpace(args.Slug),
		Description:  strings.TrimSpace(args.Description),
		Instructions: strings.TrimSpace(args.Instructions),
		IconURL:      strings.TrimSpace(args.IconURL),
		URL:          strings.TrimSpace(args.URL),
		Transport:    strings.TrimSpace(args.Transport),
		AuthType:     strings.TrimSpace(args.AuthType),

		OAuth2ClientID:                strings.TrimSpace(args.OAuth2ClientID),
		OAuth2ClientSecret:            oauth2ClientSecret,
		OAuth2ClientSecretPlaceholder: oauth2ClientSecretPlaceholder,
		OAuth2AuthURL:                 strings.TrimSpace(args.OAuth2AuthURL),
		OAuth2TokenURL:                strings.TrimSpace(args.OAuth2TokenURL),
		OAuth2Scopes:                  strings.TrimSpace(args.OAuth2Scopes),

		APIKeyHeader:             strings.TrimSpace(args.APIKeyHeader),
		APIKeyValue:              apiKeyValue,
		APIKeyValuePlaceholder:   apiKeyValuePlaceholder,
		CustomHeaders:            customHeaders,
		CustomHeaderPlaceholders: customHeaderPlaceholders,

		ToolAllowList: trimStringSlice(args.ToolAllowList),
		ToolDenyList:  trimStringSlice(args.ToolDenyList),

		Disabled: args.Disabled,
	}
	if req.DisplayName == "" {
		return req, "display_name is required"
	}
	if req.Slug == "" {
		return req, "slug is required"
	}
	if req.URL == "" {
		return req, "url is required"
	}
	if req.Transport == "" {
		req.Transport = "streamable_http"
	}
	switch req.Transport {
	case "streamable_http", "sse":
	default:
		return req, fmt.Sprintf("invalid transport %q: must be streamable_http or sse", req.Transport)
	}
	if req.AuthType == "" {
		req.AuthType = "none"
	}
	switch req.AuthType {
	case "none", "oauth2":
	case "api_key":
		if req.APIKeyHeader == "" || (req.APIKeyValue == "" && req.APIKeyValuePlaceholder == "") {
			return req, "api_key auth requires api_key_header and api_key_value with either value or placeholder"
		}
	case "custom_headers":
		if len(req.CustomHeaders)+len(req.CustomHeaderPlaceholders) == 0 {
			return req, "custom_headers auth requires at least one custom header"
		}
	default:
		return req, fmt.Sprintf("invalid auth_type %q: must be none, oauth2, api_key, or custom_headers", req.AuthType)
	}
	if req.AuthType == "oauth2" {
		provided := req.OAuth2ClientID != "" || req.OAuth2AuthURL != "" || req.OAuth2TokenURL != ""
		complete := req.OAuth2ClientID != "" && req.OAuth2AuthURL != "" && req.OAuth2TokenURL != ""
		if provided && !complete {
			return req, "oauth2 requires either all of oauth2_client_id, oauth2_auth_url, and oauth2_token_url, or none of them (automatic discovery)"
		}
		if !provided && (req.OAuth2ClientSecret != "" || req.OAuth2ClientSecretPlaceholder != "") {
			return req, "oauth2_client_secret is only valid with complete manual OAuth2 metadata"
		}
	}
	credentialsRequired := req.OAuth2ClientSecretPlaceholder != "" ||
		req.APIKeyValuePlaceholder != "" || len(req.CustomHeaderPlaceholders) > 0
	if credentialsRequired && req.Instructions == "" {
		return req, "instructions is required when the user must provide credentials"
	}
	return req, ""
}

func normalizeMCPProposalSecret(name string, secret MCPServerProposalSecret, optional bool) (normalizedValue, normalizedPlaceholder, errMsg string) {
	value := strings.TrimSpace(secret.Value)
	placeholder := strings.TrimSpace(secret.Placeholder)
	switch {
	case value != "" && placeholder != "":
		return "", "", name + " must contain exactly one of value or placeholder"
	case value != "":
		return secret.Value, "", ""
	case placeholder != "":
		return "", placeholder, ""
	case optional:
		return "", "", ""
	default:
		return "", "", name + " must contain exactly one of value or placeholder"
	}
}

// postMCPProposalCard posts the pending confirmation card to the Slack
// thread and returns the posted message timestamp.
func postMCPProposalCard(ctx context.Context, opts MCPServerToolsOptions, proposalID uuid.UUID, req MCPServerProposalRequest) (string, error) {
	reviewURL := MCPProposalReviewURL(opts.AccessURL, proposalID)

	reviewBtn := slack.NewButtonBlockElement("mcp_proposal_review", proposalID.String(),
		slack.NewTextBlockObject(slack.PlainTextType, "Review", false, false))
	reviewBtn.URL = reviewURL
	reviewBtn.Style = slack.StylePrimary
	cancelBtn := slack.NewButtonBlockElement(MCPProposalCancelActionID, proposalID.String(),
		slack.NewTextBlockObject(slack.PlainTextType, "Cancel", false, false))

	summary := fmt.Sprintf("*%s* (`%s`)\n%s", req.DisplayName, req.Slug, req.URL)
	blocks := []slack.Block{
		slack.NewHeaderBlock(slack.NewTextBlockObject(slack.PlainTextType,
			fmt.Sprintf("Connect %s?", req.DisplayName), false, false)),
		slack.NewSectionBlock(slack.NewTextBlockObject(slack.MarkdownType, summary, false, false), nil, nil),
	}
	if req.Description != "" {
		blocks = append(blocks, slack.NewSectionBlock(slack.NewTextBlockObject(slack.MarkdownType,
			"*Description*\n"+req.Description, false, false), nil, nil))
	}
	blocks = append(blocks,
		slack.NewSectionBlock(slack.NewTextBlockObject(slack.MarkdownType,
			"This MCP server will only be available to you.", false, false), nil, nil),
		slack.NewActionBlock("mcp_proposal_actions_"+proposalID.String(), reviewBtn, cancelBtn),
	)

	attachment := slack.Attachment{
		Color:  MCPProposalPendingColor,
		Blocks: slack.Blocks{BlockSet: blocks},
	}

	_, ts, err := opts.SlackAPI.PostMessageContext(ctx, opts.Channel,
		slack.MsgOptionText(fmt.Sprintf("MCP server proposal: %s", req.DisplayName), false),
		slack.MsgOptionAttachments(attachment),
		slack.MsgOptionTS(opts.ThreadTS),
	)
	if err != nil {
		return "", xerrors.Errorf("post proposal card: %w", err)
	}
	return ts, nil
}

// trimStringSlice trims whitespace from each element and drops empty
// strings. Duplicated from coderd/mcp.go on purpose.
func trimStringSlice(ss []string) []string {
	if ss == nil {
		return nil
	}
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		if trimmed := strings.TrimSpace(s); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

package chattool

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/url"
	"slices"
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

	OAuth2ClientID     MCPServerProposalField `json:"oauth2_client_id,omitzero"`
	OAuth2ClientSecret MCPServerProposalField `json:"oauth2_client_secret,omitzero"`
	OAuth2AuthURL      MCPServerProposalField `json:"oauth2_auth_url,omitzero"`
	OAuth2TokenURL     MCPServerProposalField `json:"oauth2_token_url,omitzero"`
	OAuth2Scopes       MCPServerProposalField `json:"oauth2_scopes,omitzero"`

	APIKeyHeader  MCPServerProposalField            `json:"api_key_header,omitzero"`
	APIKeyValue   MCPServerProposalField            `json:"api_key_value,omitzero"`
	CustomHeaders map[string]MCPServerProposalField `json:"custom_headers,omitempty"`

	ToolAllowList []string `json:"tool_allow_list,omitempty"`
	ToolDenyList  []string `json:"tool_deny_list,omitempty"`

	Disabled bool `json:"disabled,omitempty"`

	// ReservedConfigID is the MCP server config id that will be used when
	// this proposal is accepted. Set server-side for manual OAuth2 proposals
	// so the review page can show a stable OAuth2 redirect URI before accept.
	// Agents must not supply this field.
	ReservedConfigID uuid.UUID `json:"reserved_config_id,omitzero"`
}

// MCPServerProposalField is an auth-valued propose_mcp_server argument.
// Exactly one of Value or UserInput must be set when the argument is used.
type MCPServerProposalField struct {
	Value     string                      `json:"value,omitempty" description:"Concrete value, only when it is already available to the agent"`
	UserInput *MCPServerProposalUserInput `json:"user_input,omitempty" description:"Request this value from the user on the proposal review page"`

	present      bool
	valueSet     bool
	userInputSet bool
}

// UnmarshalJSON records which wrapper properties were explicitly supplied so
// validation can distinguish an omitted field from an empty wrapper.
func (f *MCPServerProposalField) UnmarshalJSON(data []byte) error {
	type fieldJSON struct {
		Value     string                      `json:"value"`
		UserInput *MCPServerProposalUserInput `json:"user_input"`
	}
	var decoded fieldJSON
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var properties map[string]json.RawMessage
	if err := json.Unmarshal(data, &properties); err != nil {
		return err
	}
	*f = MCPServerProposalField{
		Value:        decoded.Value,
		UserInput:    decoded.UserInput,
		present:      true,
		valueSet:     properties["value"] != nil,
		userInputSet: properties["user_input"] != nil,
	}
	return nil
}

// IsZero reports whether the field has neither a concrete value nor a user
// input declaration.
func (f MCPServerProposalField) IsZero() bool {
	return f.Value == "" && f.UserInput == nil
}

// MCPServerProposalUserInput describes a value the requester must enter on
// the proposal review page.
type MCPServerProposalUserInput struct {
	Placeholder string `json:"placeholder" description:"Short form-field placeholder showing the expected format. Keep it brief and put setup instructions in instructions."`
	Sensitive   bool   `json:"sensitive" description:"Whether the review page must mask this value. Set true for secrets and false for public values."`

	sensitiveSet bool
}

// UnmarshalJSON records whether sensitivity was explicitly declared. False is
// a meaningful declaration and cannot be inferred from the zero value.
func (i *MCPServerProposalUserInput) UnmarshalJSON(data []byte) error {
	type userInputJSON struct {
		Placeholder string `json:"placeholder"`
		Sensitive   *bool  `json:"sensitive"`
	}
	var decoded userInputJSON
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*i = MCPServerProposalUserInput{Placeholder: decoded.Placeholder}
	if decoded.Sensitive != nil {
		i.Sensitive = *decoded.Sensitive
		i.sensitiveSet = true
	}
	return nil
}

// MCPServerProposalRequiredInput is a user input declared by a proposal.
// Field is an opaque identifier that must be returned with the value when the
// proposal is accepted.
type MCPServerProposalRequiredInput struct {
	Field       string
	Label       string
	Placeholder string
	Sensitive   bool
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
	Instructions string `json:"instructions,omitempty" description:"Concise step-by-step Markdown guide shown on the proposal review page; required when any auth field uses user_input. Be exhaustive about what to do, but brief: no filler. Prefer direct clickable links over steps for navigating a web UI. Do not assume the user knows how the external service works or is configured: never give vague directives like \"ensure the Foo API is enabled\"; instead name the exact page and action, e.g. \"Ensure the Foo API is enabled: visit [this page](https://...) and confirm Foo is turned on.\" For manual OAuth2 (not dynamic client registration), the review page shows the OAuth2 redirect URI; tell the user to copy it from there when registering the OAuth application."`
	IconURL      string `json:"icon_url,omitempty" description:"Optional icon URL"`
	Transport    string `json:"transport,omitempty" description:"streamable_http (default) or sse"`
	AuthType     string `json:"auth_type,omitempty" description:"none (default), oauth2, api_key, or custom_headers"`

	OAuth2ClientID     MCPServerProposalField `json:"oauth2_client_id,omitempty" description:"Optional value or user_input wrapper; omit for automatic discovery + Dynamic Client Registration"`
	OAuth2ClientSecret MCPServerProposalField `json:"oauth2_client_secret,omitempty" description:"Optional value or user_input wrapper; only valid with complete manual OAuth2 metadata"`
	OAuth2AuthURL      MCPServerProposalField `json:"oauth2_auth_url,omitempty" description:"Optional value or user_input wrapper; omit for automatic discovery"`
	OAuth2TokenURL     MCPServerProposalField `json:"oauth2_token_url,omitempty" description:"Optional value or user_input wrapper; omit for automatic discovery"`
	OAuth2Scopes       MCPServerProposalField `json:"oauth2_scopes,omitempty" description:"Optional value or user_input wrapper containing space-separated OAuth2 scopes"`

	APIKeyHeader  MCPServerProposalField            `json:"api_key_header,omitempty" description:"Value or user_input wrapper for the API key header name, e.g. Authorization"`
	APIKeyValue   MCPServerProposalField            `json:"api_key_value,omitempty" description:"Value or user_input wrapper for the API key value"`
	CustomHeaders map[string]MCPServerProposalField `json:"custom_headers,omitempty" description:"Header names mapped to value or user_input wrappers"`

	ToolAllowList []string `json:"tool_allow_list,omitempty" description:"Optional allow list of tool names"`
	ToolDenyList  []string `json:"tool_deny_list,omitempty" description:"Optional deny list of tool names"`

	Disabled bool `json:"disabled,omitempty" description:"Create the server disabled; defaults to enabled"`
}

func proposeMCPServer(opts MCPServerToolsOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"propose_mcp_server",
		`Propose a new MCP server for the user requesting it. This tool posts a confirmation card to the Slack thread and returns immediately; NOTHING is created until the requesting user reviews and accepts the proposal on a Coder page. Include a concise description for the Slack card and review page. Include any setup instructions in this tool call so they appear on the review page. The Slack card does NOT show setup instructions; instructions are shown only on the review page.

Be proactive: if the user expressed interest in an external service and you do not already have an MCP server for it, call this tool immediately. Do not ask whether they would like you to propose a server, whether they want help setting it up, or for permission to proceed. Just propose it; they can reject the card if they do not want it.

Accepting creates the server, enables it for this chat, and takes the user straight into authentication when needed. A follow-up [system] message reports the outcome, so end your turn and wait for it instead of assuming the result.

The server is created PERSONAL-SCOPED: it will only be available to the requesting user, not the whole deployment.

For oauth2 servers a server URL alone is usually enough (automatic discovery + Dynamic Client Registration). This tool validates automatic discovery before posting the proposal. If validation fails, use complete manual OAuth2 metadata. Every auth field uses exactly one of {"value":"known value"} or {"user_input":{"placeholder":"Expected format","sensitive":true|false}}. Use user_input whenever the user must provide a value. Set sensitive true for secrets and false for public values such as client IDs. Never ask the user to paste credentials into Slack.

User-input placeholders are shown inside the review-page input fields, so keep them short and format-focused (good: "Bearer gh_xxx"; bad: "Enter your api key that you generated in GitHub settings. The required format is 'gh_xxx'."). Whenever a field uses user_input, instructions must contain a concise step-by-step Markdown guide that is still exhaustive about how to obtain each value and how to configure credentials. Do not assume the user is familiar with how the external service works or is configured: never write vague directives like "ensure the Foo API is enabled"; instead name the exact page and action, e.g. "Ensure the Foo API is enabled: visit [this page](https://...) and confirm Foo is turned on." For manual OAuth2 (when not using dynamic client registration), the review page shows the OAuth2 redirect URI; tell the user to copy it from the review page when registering the OAuth application. Prefer a direct clickable link when one exists; do not narrate how to click through a web UI. Never send those instructions as a separate Slack message; keep the proposal as the single source of truth.`,
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
			// Manual OAuth2 needs a stable config id before accept so the
			// review page can show the redirect URI users register with the
			// provider.
			if req.AuthType == "oauth2" && req.OAuth2ClientID.configured() {
				req.ReservedConfigID = uuid.New()
			}
			if req.AuthType == "oauth2" && !req.OAuth2ClientID.configured() {
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
	req := MCPServerProposalRequest{
		DisplayName:  strings.TrimSpace(args.DisplayName),
		Slug:         strings.TrimSpace(args.Slug),
		Description:  strings.TrimSpace(args.Description),
		Instructions: strings.TrimSpace(args.Instructions),
		IconURL:      strings.TrimSpace(args.IconURL),
		URL:          strings.TrimSpace(args.URL),
		Transport:    strings.TrimSpace(args.Transport),
		AuthType:     strings.TrimSpace(args.AuthType),

		OAuth2ClientID:     args.OAuth2ClientID,
		OAuth2ClientSecret: args.OAuth2ClientSecret,
		OAuth2AuthURL:      args.OAuth2AuthURL,
		OAuth2TokenURL:     args.OAuth2TokenURL,
		OAuth2Scopes:       args.OAuth2Scopes,

		APIKeyHeader:  args.APIKeyHeader,
		APIKeyValue:   args.APIKeyValue,
		CustomHeaders: args.CustomHeaders,

		ToolAllowList: trimStringSlice(args.ToolAllowList),
		ToolDenyList:  trimStringSlice(args.ToolDenyList),

		Disabled: args.Disabled,
	}
	if req.Transport == "" {
		req.Transport = "streamable_http"
	}
	if req.AuthType == "" {
		req.AuthType = "none"
	}
	for _, field := range mcpProposalAuthFields(&req) {
		if !field.value.present && !field.isCustomHeader {
			continue
		}
		normalized, errMsg := normalizeMCPProposalField(
			field.fieldID,
			*field.value,
			field.normalizeValue,
		)
		if errMsg != "" {
			return MCPServerProposalRequest{}, errMsg
		}
		field.set(&req, normalized)
	}
	if len(req.CustomHeaders) == 0 {
		req.CustomHeaders = nil
	}
	if errMsg := validateMCPProposalRequest(req); errMsg != "" {
		return req, errMsg
	}
	return req, ""
}

func validateMCPProposalRequest(req MCPServerProposalRequest) string {
	if req.DisplayName == "" {
		return "display_name is required"
	}
	if req.Slug == "" {
		return "slug is required"
	}
	if req.URL == "" {
		return "url is required"
	}
	switch req.Transport {
	case "streamable_http", "sse":
	default:
		return fmt.Sprintf("invalid transport %q: must be streamable_http or sse", req.Transport)
	}
	switch req.AuthType {
	case "none", "oauth2":
	case "api_key":
		if !req.APIKeyHeader.configured() || !req.APIKeyValue.configured() {
			return "api_key auth requires api_key_header and api_key_value with either value or user_input"
		}
	case "custom_headers":
		if len(req.CustomHeaders) == 0 {
			return "custom_headers auth requires at least one custom header"
		}
	default:
		return fmt.Sprintf("invalid auth_type %q: must be none, oauth2, api_key, or custom_headers", req.AuthType)
	}
	if req.AuthType == "oauth2" {
		provided := req.OAuth2ClientID.configured() || req.OAuth2AuthURL.configured() || req.OAuth2TokenURL.configured()
		complete := req.OAuth2ClientID.configured() && req.OAuth2AuthURL.configured() && req.OAuth2TokenURL.configured()
		if provided && !complete {
			return "oauth2 requires either all of oauth2_client_id, oauth2_auth_url, and oauth2_token_url, or none of them (automatic discovery)"
		}
		if !provided && req.OAuth2ClientSecret.configured() {
			return "oauth2_client_secret is only valid with complete manual OAuth2 metadata"
		}
	}
	if len(RequiredMCPServerProposalInputs(req)) > 0 && req.Instructions == "" {
		return "instructions is required when the user must provide values"
	}
	return ""
}

func (f MCPServerProposalField) configured() bool {
	return strings.TrimSpace(f.Value) != "" || f.UserInput != nil
}

func normalizeMCPProposalField(name string, field MCPServerProposalField, normalizeValue func(string) string) (MCPServerProposalField, string) {
	if !field.present {
		return MCPServerProposalField{}, name + " must contain exactly one of value or user_input"
	}
	if field.valueSet == field.userInputSet {
		return MCPServerProposalField{}, name + " must contain exactly one of value or user_input"
	}

	value := strings.TrimSpace(field.Value)
	if field.valueSet {
		if value == "" {
			return MCPServerProposalField{}, name + ".value is required"
		}
		if normalizeValue != nil {
			field.Value = normalizeValue(field.Value)
		}
		field.UserInput = nil
		field.present = true
		field.valueSet = true
		field.userInputSet = false
		return field, ""
	}
	if field.UserInput == nil {
		return MCPServerProposalField{}, name + ".user_input is required"
	}
	placeholder := strings.TrimSpace(field.UserInput.Placeholder)
	if placeholder == "" {
		return MCPServerProposalField{}, name + ".user_input.placeholder is required"
	}
	if !field.UserInput.sensitiveSet {
		return MCPServerProposalField{}, name + ".user_input.sensitive is required"
	}
	return MCPServerProposalField{
		UserInput: &MCPServerProposalUserInput{
			Placeholder:  placeholder,
			Sensitive:    field.UserInput.Sensitive,
			sensitiveSet: true,
		},
		present:      true,
		userInputSet: true,
	}, ""
}

type mcpProposalAuthField struct {
	fieldID        string
	label          string
	value          *MCPServerProposalField
	normalizeValue func(string) string
	customHeader   string
	isCustomHeader bool
}

func (f mcpProposalAuthField) set(req *MCPServerProposalRequest, value MCPServerProposalField) {
	if f.isCustomHeader {
		req.CustomHeaders[f.customHeader] = value
		return
	}
	*f.value = value
}

func mcpProposalAuthFields(req *MCPServerProposalRequest) []mcpProposalAuthField {
	fields := []mcpProposalAuthField{
		{fieldID: "oauth2_client_id", label: "OAuth2 client ID", value: &req.OAuth2ClientID, normalizeValue: strings.TrimSpace},
		{fieldID: "oauth2_client_secret", label: "OAuth2 client secret", value: &req.OAuth2ClientSecret},
		{fieldID: "oauth2_auth_url", label: "OAuth2 authorization URL", value: &req.OAuth2AuthURL, normalizeValue: strings.TrimSpace},
		{fieldID: "oauth2_token_url", label: "OAuth2 token URL", value: &req.OAuth2TokenURL, normalizeValue: strings.TrimSpace},
		{fieldID: "oauth2_scopes", label: "OAuth2 scopes", value: &req.OAuth2Scopes, normalizeValue: strings.TrimSpace},
		{fieldID: "api_key_header", label: "API key header", value: &req.APIKeyHeader, normalizeValue: strings.TrimSpace},
		{fieldID: "api_key_value", label: "API key", value: &req.APIKeyValue},
	}
	for _, header := range slices.Sorted(maps.Keys(req.CustomHeaders)) {
		value := req.CustomHeaders[header]
		fields = append(fields, mcpProposalAuthField{
			fieldID:        "custom_headers." + header,
			label:          header,
			value:          &value,
			customHeader:   header,
			isCustomHeader: true,
		})
	}
	return fields
}

// RequiredMCPServerProposalInputs returns the user inputs declared by req in a
// deterministic order.
func RequiredMCPServerProposalInputs(req MCPServerProposalRequest) []MCPServerProposalRequiredInput {
	var inputs []MCPServerProposalRequiredInput
	for _, field := range mcpProposalAuthFields(&req) {
		if field.value.UserInput == nil {
			continue
		}
		inputs = append(inputs, MCPServerProposalRequiredInput{
			Field:       field.fieldID,
			Label:       field.label,
			Placeholder: field.value.UserInput.Placeholder,
			Sensitive:   field.value.UserInput.Sensitive,
		})
	}
	return inputs
}

// ResolveMCPServerProposalInputs applies the values supplied during proposal
// acceptance. Values are resolved in memory and are never written back to the
// proposal row.
func ResolveMCPServerProposalInputs(req MCPServerProposalRequest, values map[string]string) (MCPServerProposalRequest, error) {
	fields := mcpProposalAuthFields(&req)
	requested := make(map[string]mcpProposalAuthField)
	for _, field := range fields {
		if field.value.UserInput != nil {
			requested[field.fieldID] = field
		}
	}
	for fieldID := range values {
		if _, ok := requested[fieldID]; !ok {
			return req, xerrors.Errorf("%s was not requested", fieldID)
		}
	}
	for _, field := range fields {
		if field.value.UserInput == nil {
			continue
		}
		value, ok := values[field.fieldID]
		if !ok || strings.TrimSpace(value) == "" {
			return req, xerrors.Errorf("%s is required", field.fieldID)
		}
		if field.normalizeValue != nil {
			value = field.normalizeValue(value)
		}
		resolved := MCPServerProposalField{Value: value}
		field.set(&req, resolved)
	}
	return req, nil
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

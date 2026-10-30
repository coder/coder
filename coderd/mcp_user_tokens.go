package coderd

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/net/http/httpguts"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
)

// user_api_key MCP servers have no shared credential. Every user stores
// their own token, and chats send only their owner's token. An agent's owner
// stores the token its agent user sends.

const (
	defaultMCPUserAPIKeyHeader = "Authorization"
	defaultMCPUserAPIKeyPrefix = "Bearer "
	maxMCPUserTokenBytes       = 16 << 10
)

// mcpUserAPIKeyAuthError is a user_api_key config value that cannot be sent
// as an HTTP header.
type mcpUserAPIKeyAuthError string

func (e mcpUserAPIKeyAuthError) Error() string { return string(e) }

// defaultMCPUserAPIKeyAuth fills in the header and prefix a user_api_key
// server uses when the request leaves them out.
func defaultMCPUserAPIKeyAuth(header string, prefix *string) (resolvedHeader string, resolvedPrefix string) {
	header = strings.TrimSpace(header)
	if header == "" {
		header = defaultMCPUserAPIKeyHeader
	}
	if prefix == nil {
		return header, defaultMCPUserAPIKeyPrefix
	}
	return header, *prefix
}

// validateMCPUserAPIKeyAuth returns why a user_api_key header or prefix is
// invalid, or "" when both are valid.
func validateMCPUserAPIKeyAuth(header, prefix string) string {
	if header == "" || !httpguts.ValidHeaderFieldName(header) {
		return "User API key auth requires api_key_header to be a valid HTTP header name."
	}
	if !httpguts.ValidHeaderFieldValue(prefix) {
		return "api_key_prefix must not contain line breaks or control characters."
	}
	return ""
}

// normalizeMCPUserToken trims a pasted token and returns why it cannot be
// stored, or "" when it can.
func normalizeMCPUserToken(raw string) (token string, problem string) {
	token = strings.TrimSpace(raw)
	switch {
	case token == "":
		return "", "Token is required."
	case len(token) > maxMCPUserTokenBytes:
		return "", "Token is too long."
	case !httpguts.ValidHeaderFieldValue(token):
		return "", "Token must not contain line breaks or control characters."
	}
	return token, ""
}

// storeMCPServerUserToken saves userID's token for a user_api_key server.
func storeMCPServerUserToken(ctx context.Context, store database.Store, configID, userID uuid.UUID, token string) (database.MCPServerUserToken, error) {
	return store.UpsertMCPServerUserToken(ctx, database.UpsertMCPServerUserTokenParams{
		MCPServerConfigID: configID,
		UserID:            userID,
		AccessToken:       token,
		AccessTokenKeyID:  sql.NullString{},
		RefreshToken:      "",
		RefreshTokenKeyID: sql.NullString{},
		TokenType:         "",
		Expiry:            sql.NullTime{},
	})
}

func mcpServerUserTokenStatus(token database.MCPServerUserToken) codersdk.MCPServerUserTokenStatus {
	updatedAt := token.UpdatedAt
	status := codersdk.MCPServerUserTokenStatus{HasToken: token.AccessToken != "", UpdatedAt: &updatedAt}
	if status.HasToken {
		status.CredentialKind = codersdk.MCPServerCredentialKindToken
	}
	return status
}

// mcpServerCredentialKind resolves the kind a token request asks for.
func mcpServerCredentialKind(req codersdk.UpdateMCPServerUserTokenRequest) codersdk.MCPServerCredentialKind {
	if req.CredentialKind != "" {
		return req.CredentialKind
	}
	if req.GitHubApp != nil {
		return codersdk.MCPServerCredentialKindGitHubApp
	}
	return codersdk.MCPServerCredentialKindToken
}

// @Summary Set MCP server user token
// @Description Stores the caller's own token for a user_api_key MCP server.
// @Description The token is never returned.
// @ID set-mcp-server-user-token
// @Security CoderSessionToken
// @Tags MCP
// @Accept json
// @Produce json
// @Param mcpServer path string true "MCP server config ID" format(uuid)
// @Param request body codersdk.UpdateMCPServerUserTokenRequest true "Token"
// @Success 200 {object} codersdk.MCPServerUserTokenStatus
// @Router /api/v2/mcp/servers/{mcpServer}/user-token [put]
// @x-apidocgen {"skip": true}
func (api *API) putMCPServerUserToken(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)
	configID, ok := httpmw.ParseUUIDParam(rw, r, "mcpServer")
	if !ok {
		return
	}
	if !api.Authorize(r, policy.ActionUpdatePersonal, rbac.ResourceUserObject(apiKey.UserID)) {
		httpapi.Forbidden(rw)
		return
	}
	var req codersdk.UpdateMCPServerUserTokenRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	if mcpServerCredentialKind(req) != codersdk.MCPServerCredentialKindToken {
		writeChatProjectFieldError(ctx, rw, "credential_kind", "Only agents can use a GitHub App credential. Set it on the agent instead.")
		return
	}
	token, msg := normalizeMCPUserToken(req.Token)
	if msg != "" {
		writeChatProjectFieldError(ctx, rw, "token", msg)
		return
	}
	// Reading the config as the caller limits tokens to servers they can
	// use.
	config, err := api.Database.GetMCPServerConfigByID(ctx, configID)
	if err != nil {
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.InternalServerError(rw, err)
		return
	}
	if config.AuthType != codersdk.MCPServerAuthTypeUserAPIKey {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "This MCP server does not use personal tokens.",
		})
		return
	}
	stored, err := storeMCPServerUserToken(ctx, api.Database, config.ID, apiKey.UserID, token)
	if err != nil {
		if dbauthz.IsNotAuthorizedError(err) {
			httpapi.Forbidden(rw)
			return
		}
		httpapi.InternalServerError(rw, err)
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, mcpServerUserTokenStatus(stored))
}

// @Summary Delete MCP server user token
// @ID delete-mcp-server-user-token
// @Security CoderSessionToken
// @Tags MCP
// @Param mcpServer path string true "MCP server config ID" format(uuid)
// @Success 204
// @Router /api/v2/mcp/servers/{mcpServer}/user-token [delete]
// @x-apidocgen {"skip": true}
func (api *API) deleteMCPServerUserToken(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)
	configID, ok := httpmw.ParseUUIDParam(rw, r, "mcpServer")
	if !ok {
		return
	}
	if !api.Authorize(r, policy.ActionUpdatePersonal, rbac.ResourceUserObject(apiKey.UserID)) {
		httpapi.Forbidden(rw)
		return
	}
	// Deleting needs no config access, so people who lost access to the
	// server can still remove their token.
	err := api.Database.DeleteMCPServerUserToken(ctx, database.DeleteMCPServerUserTokenParams{
		MCPServerConfigID: configID,
		UserID:            apiKey.UserID,
	})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		if dbauthz.IsNotAuthorizedError(err) {
			httpapi.Forbidden(rw)
			return
		}
		httpapi.InternalServerError(rw, err)
		return
	}
	rw.WriteHeader(http.StatusNoContent)
}

// chatProjectMCPServerParam loads the route's project with its agent user
// and the attached MCP server named by the route. Changing the agent's
// token lets chats act with it, so like triggers it is limited to the
// agent's owner.
func (api *API) chatProjectMCPServerParam(rw http.ResponseWriter, r *http.Request) (database.ChatProject, database.MCPServerConfig, bool) {
	ctx := r.Context()
	project, ok := api.chatProjectForExternalAuth(rw, r, policy.ActionUpdate)
	if !ok {
		return database.ChatProject{}, database.MCPServerConfig{}, false
	}
	if project.OwnerID != httpmw.APIKey(r).UserID {
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: "Only the agent owner can change the agent's MCP server tokens.",
		})
		return database.ChatProject{}, database.MCPServerConfig{}, false
	}
	configID, ok := httpmw.ParseUUIDParam(rw, r, "mcpServer")
	if !ok {
		return database.ChatProject{}, database.MCPServerConfig{}, false
	}
	if !slices.Contains(project.MCPServerIDs, configID) {
		httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
			Message: "The MCP server is not attached to this agent.",
		})
		return database.ChatProject{}, database.MCPServerConfig{}, false
	}
	//nolint:gocritic // The server is attached to an agent the caller owns.
	config, err := api.Database.GetMCPServerConfigByID(dbauthz.AsSystemRestricted(ctx), configID)
	if err != nil {
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return database.ChatProject{}, database.MCPServerConfig{}, false
		}
		httpapi.InternalServerError(rw, err)
		return database.ChatProject{}, database.MCPServerConfig{}, false
	}
	if config.OrganizationID != project.OrganizationID {
		httpapi.ResourceNotFound(rw)
		return database.ChatProject{}, database.MCPServerConfig{}, false
	}
	return project, config, true
}

// @Summary List chat project MCP server tokens
// @Description Lists the agent's MCP servers, which of them need a personal
// @Description token, and whether the agent's user has one. Tokens are never
// @Description returned.
// @ID list-chat-project-mcp-server-tokens
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param project path string true "Chat project ID" format(uuid)
// @Success 200 {array} codersdk.ChatProjectMCPServerToken
// @Router /api/experimental/organizations/{organization}/chats/projects/{project}/mcp-servers [get]
// @x-apidocgen {"skip": true}
func (api *API) listChatProjectMCPServerTokens(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project, ok := api.chatProjectForExternalAuth(rw, r, policy.ActionRead)
	if !ok {
		return
	}
	//nolint:gocritic // Reading the agent's attached servers and its user's token state is authorized by the project read above.
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	tokens, err := api.Database.GetMCPServerUserTokensByUserID(sysCtx, project.AgentUserID.UUID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		httpapi.InternalServerError(rw, xerrors.Errorf("get agent MCP server tokens: %w", err))
		return
	}
	tokensByConfig := make(map[uuid.UUID]database.MCPServerUserToken, len(tokens))
	for _, token := range tokens {
		tokensByConfig[token.MCPServerConfigID] = token
	}
	apps, err := api.Database.GetMCPServerGitHubAppCredentialsByUserID(sysCtx, project.AgentUserID.UUID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		httpapi.InternalServerError(rw, xerrors.Errorf("get agent GitHub App credentials: %w", err))
		return
	}
	appsByConfig := make(map[uuid.UUID]database.MCPServerGitHubAppCredential, len(apps))
	for _, app := range apps {
		appsByConfig[app.MCPServerConfigID] = app
	}
	resp := make([]codersdk.ChatProjectMCPServerToken, 0, len(project.MCPServerIDs))
	// The app's IDs and repositories are setup details for the owner, who
	// manages the credential. Others only see that one is set.
	isOwner := project.OwnerID == httpmw.APIKey(r).UserID
	for _, id := range project.MCPServerIDs {
		config, err := api.Database.GetMCPServerConfigByID(sysCtx, id)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			httpapi.InternalServerError(rw, xerrors.Errorf("get MCP server config: %w", err))
			return
		}
		if config.OrganizationID != project.OrganizationID {
			continue
		}
		entry := codersdk.ChatProjectMCPServerToken{
			MCPServerConfigID: config.ID,
			DisplayName:       config.DisplayName,
			Slug:              config.Slug,
			IconURL:           config.IconURL,
			AuthType:          config.AuthType,
			TokenRequired:     config.AuthType == codersdk.MCPServerAuthTypeUserAPIKey,
		}
		if app, ok := appsByConfig[config.ID]; ok && entry.TokenRequired {
			status := mcpServerGitHubAppStatus(app)
			entry.HasToken = true
			entry.TokenUpdatedAt = status.UpdatedAt
			entry.CredentialKind = status.CredentialKind
			if isOwner {
				entry.GitHubApp = status.GitHubApp
			}
		} else if token, ok := tokensByConfig[config.ID]; ok && entry.TokenRequired && token.AccessToken != "" {
			updatedAt := token.UpdatedAt
			entry.HasToken = true
			entry.TokenUpdatedAt = &updatedAt
			entry.CredentialKind = codersdk.MCPServerCredentialKindToken
		}
		resp = append(resp, entry)
	}
	httpapi.Write(ctx, rw, http.StatusOK, resp)
}

// @Summary Set chat project MCP server token
// @Description Stores the token the agent's user sends to a user_api_key MCP
// @Description server. Only the agent owner may set it, and it is never
// @Description returned.
// @ID set-chat-project-mcp-server-token
// @Security CoderSessionToken
// @Tags Chats
// @Accept json
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param project path string true "Chat project ID" format(uuid)
// @Param mcpServer path string true "MCP server config ID" format(uuid)
// @Param request body codersdk.UpdateMCPServerUserTokenRequest true "Token"
// @Success 200 {object} codersdk.MCPServerUserTokenStatus
// @Router /api/experimental/organizations/{organization}/chats/projects/{project}/mcp-servers/{mcpServer}/token [put]
// @x-apidocgen {"skip": true}
func (api *API) putChatProjectMCPServerToken(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project, config, ok := api.chatProjectMCPServerParam(rw, r)
	if !ok {
		return
	}
	var req codersdk.UpdateMCPServerUserTokenRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	if config.AuthType != codersdk.MCPServerAuthTypeUserAPIKey {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "This MCP server does not use personal tokens.",
		})
		return
	}
	agentUserID := project.AgentUserID.UUID
	//nolint:gocritic // The owner check above authorizes writing the agent user's credential.
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	switch mcpServerCredentialKind(req) {
	case codersdk.MCPServerCredentialKindToken:
		token, msg := normalizeMCPUserToken(req.Token)
		if msg != "" {
			writeChatProjectFieldError(ctx, rw, "token", msg)
			return
		}
		var stored database.MCPServerUserToken
		err := api.Database.InTx(func(tx database.Store) error {
			var err error
			stored, err = storeMCPServerUserToken(sysCtx, tx, config.ID, agentUserID, token)
			if err != nil {
				return err
			}
			// One credential per server: the token replaces a GitHub App.
			return ignoreNoRows(tx.DeleteMCPServerGitHubAppCredential(sysCtx, database.DeleteMCPServerGitHubAppCredentialParams{
				MCPServerConfigID: config.ID,
				UserID:            agentUserID,
			}))
		}, nil)
		if err != nil {
			httpapi.InternalServerError(rw, err)
			return
		}
		httpapi.Write(ctx, rw, http.StatusOK, mcpServerUserTokenStatus(stored))
	case codersdk.MCPServerCredentialKindGitHubApp:
		status, ok := api.storeAgentGitHubAppCredential(sysCtx, rw, config, agentUserID, req.GitHubApp)
		if !ok {
			return
		}
		httpapi.Write(ctx, rw, http.StatusOK, status)
	default:
		writeChatProjectFieldError(ctx, rw, "credential_kind", "Credential kind must be token or github_app.")
	}
}

// @Summary Delete chat project MCP server token
// @ID delete-chat-project-mcp-server-token
// @Security CoderSessionToken
// @Tags Chats
// @Param organization path string true "Organization ID" format(uuid)
// @Param project path string true "Chat project ID" format(uuid)
// @Param mcpServer path string true "MCP server config ID" format(uuid)
// @Success 204
// @Router /api/experimental/organizations/{organization}/chats/projects/{project}/mcp-servers/{mcpServer}/token [delete]
// @x-apidocgen {"skip": true}
func (api *API) deleteChatProjectMCPServerToken(rw http.ResponseWriter, r *http.Request) {
	project, config, ok := api.chatProjectMCPServerParam(rw, r)
	if !ok {
		return
	}
	//nolint:gocritic // The owner check above authorizes removing the agent user's credentials.
	sysCtx := dbauthz.AsSystemRestricted(r.Context())
	err := api.Database.InTx(func(tx database.Store) error {
		err := tx.DeleteMCPServerUserToken(sysCtx, database.DeleteMCPServerUserTokenParams{
			MCPServerConfigID: config.ID,
			UserID:            project.AgentUserID.UUID,
		})
		if err := ignoreNoRows(err); err != nil {
			return err
		}
		return ignoreNoRows(tx.DeleteMCPServerGitHubAppCredential(sysCtx, database.DeleteMCPServerGitHubAppCredentialParams{
			MCPServerConfigID: config.ID,
			UserID:            project.AgentUserID.UUID,
		}))
	}, nil)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	rw.WriteHeader(http.StatusNoContent)
}

// ignoreNoRows treats deleting a credential that does not exist as done.
func ignoreNoRows(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	return err
}

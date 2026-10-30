package coderd

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/externalauth"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/x/githubapp"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
)

// An agent's user can authenticate to a user_api_key MCP server as a GitHub
// App instead of with a pasted token. Coder stores the app's private key and
// mints a short-lived installation token whenever one is needed, so the
// agent acts as the app's bot account. When the owner opts in, the same
// identity serves the agent's git operations in workspaces through the
// external auth endpoint. Tokens cover the listed repositories, or every
// repository the installation can access when none are listed.

func mcpServerGitHubAppStatus(app database.MCPServerGitHubAppCredential) codersdk.MCPServerUserTokenStatus {
	updatedAt := app.UpdatedAt
	repos := app.Repositories
	if repos == nil {
		repos = []string{}
	}
	return codersdk.MCPServerUserTokenStatus{
		HasToken:       true,
		UpdatedAt:      &updatedAt,
		CredentialKind: codersdk.MCPServerCredentialKindGitHubApp,
		GitHubApp: &codersdk.MCPServerGitHubApp{
			AppID:          app.AppID,
			InstallationID: app.InstallationID,
			Repositories:   repos,
			UseForGit:      app.UseForGit,
		},
	}
}

// storeAgentGitHubAppCredential checks a GitHub App credential by minting a
// token with it, then stores it as the agent user's only credential for the
// server. It writes the error response and returns false when the
// credential is invalid.
func (api *API) storeAgentGitHubAppCredential(
	ctx context.Context,
	rw http.ResponseWriter,
	config database.MCPServerConfig,
	agentUserID uuid.UUID,
	req *codersdk.MCPServerGitHubAppCredentialRequest,
) (codersdk.MCPServerUserTokenStatus, bool) {
	if req == nil {
		writeChatProjectFieldError(ctx, rw, "github_app", "GitHub App details are required.")
		return codersdk.MCPServerUserTokenStatus{}, false
	}
	if req.AppID <= 0 {
		writeChatProjectFieldError(ctx, rw, "github_app.app_id", "App ID must be the app's numeric ID.")
		return codersdk.MCPServerUserTokenStatus{}, false
	}
	if req.InstallationID <= 0 {
		writeChatProjectFieldError(ctx, rw, "github_app.installation_id", "Installation ID must be the installation's numeric ID.")
		return codersdk.MCPServerUserTokenStatus{}, false
	}
	repos, err := githubapp.NormalizeRepositories(req.Repositories)
	if err != nil {
		writeChatProjectFieldError(ctx, rw, "github_app.repositories", capitalize(err.Error())+".")
		return codersdk.MCPServerUserTokenStatus{}, false
	}
	privateKey := strings.TrimSpace(req.PrivateKey)
	if privateKey == "" {
		// The key is write-only, so keep the stored one when the same app
		// is saved again, for example to change its repositories.
		existing, err := api.Database.GetMCPServerGitHubAppCredential(ctx, database.GetMCPServerGitHubAppCredentialParams{
			MCPServerConfigID: config.ID,
			UserID:            agentUserID,
		})
		switch {
		case err == nil && existing.AppID == req.AppID:
			privateKey = existing.PrivateKey
		case err != nil && !errors.Is(err, sql.ErrNoRows):
			httpapi.InternalServerError(rw, xerrors.Errorf("get GitHub App credential: %w", err))
			return codersdk.MCPServerUserTokenStatus{}, false
		default:
			writeChatProjectFieldError(ctx, rw, "github_app.private_key", "Private key is required.")
			return codersdk.MCPServerUserTokenStatus{}, false
		}
	}
	if _, err := githubapp.ParsePrivateKey(privateKey); err != nil {
		writeChatProjectFieldError(ctx, rw, "github_app.private_key", capitalize(err.Error())+".")
		return codersdk.MCPServerUserTokenStatus{}, false
	}
	apiBase, err := githubapp.APIBaseURLFromServerURL(config.Url)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "The MCP server's URL does not select a valid GitHub API.",
			Detail:  err.Error(),
		})
		return codersdk.MCPServerUserTokenStatus{}, false
	}
	cred := githubapp.Credential{
		APIBaseURL:     apiBase,
		AppID:          req.AppID,
		InstallationID: req.InstallationID,
		PrivateKeyPEM:  privateKey,
		Repositories:   repos,
	}
	// Saving checks the app with a fresh token, so a save right after
	// changing the app's permissions picks them up.
	api.githubAppTokens.Forget(cred)
	if _, err := api.githubAppTokens.Token(ctx, cred); err != nil {
		writeGitHubAppMintError(ctx, rw, apiBase, err)
		return codersdk.MCPServerUserTokenStatus{}, false
	}

	var stored database.MCPServerGitHubAppCredential
	err = api.Database.InTx(func(tx database.Store) error {
		var err error
		stored, err = tx.UpsertMCPServerGitHubAppCredential(ctx, database.UpsertMCPServerGitHubAppCredentialParams{
			MCPServerConfigID: config.ID,
			UserID:            agentUserID,
			AppID:             req.AppID,
			InstallationID:    req.InstallationID,
			PrivateKey:        privateKey,
			// dbcrypt sets the key ID when it encrypts the key.
			PrivateKeyKeyID: sql.NullString{},
			Repositories:    repos,
			UseForGit:       req.UseForGit,
		})
		if err != nil {
			return xerrors.Errorf("store GitHub App credential: %w", err)
		}
		// One credential per server: the app replaces a pasted token.
		return ignoreNoRows(tx.DeleteMCPServerUserToken(ctx, database.DeleteMCPServerUserTokenParams{
			MCPServerConfigID: config.ID,
			UserID:            agentUserID,
		}))
	}, nil)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return codersdk.MCPServerUserTokenStatus{}, false
	}
	return mcpServerGitHubAppStatus(stored), true
}

// writeGitHubAppMintError explains why GitHub refused to mint a token,
// including GitHub's own message.
func writeGitHubAppMintError(ctx context.Context, rw http.ResponseWriter, apiBase string, err error) {
	var ghErr *githubapp.Error
	if !errors.As(err, &ghErr) {
		httpapi.Write(ctx, rw, http.StatusBadGateway, codersdk.Response{
			Message: fmt.Sprintf("Coder could not reach GitHub at %s to check the GitHub App credential.", apiBase),
			Detail:  err.Error(),
		})
		return
	}
	var hint string
	switch ghErr.StatusCode {
	case http.StatusUnauthorized:
		hint = "Check the app ID and the private key."
	case http.StatusNotFound:
		hint = "Check the installation ID, and that the app is installed on the account."
	case http.StatusUnprocessableEntity:
		hint = "Check that the app is installed on every listed repository."
	case http.StatusForbidden:
		hint = "Check the app's permissions and the installation's status."
	}
	detail := "GitHub said: " + cmp.Or(ghErr.Message, http.StatusText(ghErr.StatusCode))
	if hint != "" {
		detail = hint + " " + detail
	}
	httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
		Message: "GitHub rejected the GitHub App credential.",
		Detail:  detail,
	})
}

// githubAppExternalAuthResponse returns git credentials minted from the
// GitHub App the workspace owner, an agent's user, uses on one of its MCP
// servers, when that app is on the same GitHub as the external auth
// provider. ok is false when no such app applies and the usual external
// auth link should be used.
func (api *API) githubAppExternalAuthResponse(ctx context.Context, provider *externalauth.Config, ownerID uuid.UUID) (resp agentsdk.ExternalAuthResponse, ok bool, err error) {
	if api.githubAppTokens == nil || !strings.EqualFold(provider.Type, string(codersdk.EnhancedExternalAuthProviderGitHub)) {
		return agentsdk.ExternalAuthResponse{}, false, nil
	}
	providerHost := githubProviderAPIHost(provider)
	if providerHost == "" {
		return agentsdk.ExternalAuthResponse{}, false, nil
	}
	//nolint:gocritic // The workspace agent asks for its own owner's git credentials; ownership was checked by the caller.
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	owner, err := api.Database.GetUserByID(sysCtx, ownerID)
	if err != nil {
		return agentsdk.ExternalAuthResponse{}, false, xerrors.Errorf("get workspace owner: %w", err)
	}
	if !owner.IsAgent {
		return agentsdk.ExternalAuthResponse{}, false, nil
	}
	project, err := api.Database.GetChatProjectByAgentUserID(sysCtx, ownerID)
	if errors.Is(err, sql.ErrNoRows) {
		return agentsdk.ExternalAuthResponse{}, false, nil
	}
	if err != nil {
		return agentsdk.ExternalAuthResponse{}, false, xerrors.Errorf("get agent: %w", err)
	}
	creds, err := api.Database.GetMCPServerGitHubAppCredentialsByUserID(sysCtx, ownerID)
	if err != nil {
		return agentsdk.ExternalAuthResponse{}, false, xerrors.Errorf("get GitHub App credentials: %w", err)
	}
	for _, stored := range creds {
		// Only apps the owner opted in to git, on servers still attached
		// to the agent, lend it their identity.
		if !stored.UseForGit {
			continue
		}
		if !slices.Contains(project.MCPServerIDs, stored.MCPServerConfigID) {
			continue
		}
		config, err := api.Database.GetMCPServerConfigByID(sysCtx, stored.MCPServerConfigID)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return agentsdk.ExternalAuthResponse{}, false, xerrors.Errorf("get MCP server config: %w", err)
		}
		if !config.Enabled || config.AuthType != codersdk.MCPServerAuthTypeUserAPIKey {
			continue
		}
		cred, err := githubapp.CredentialFor(config, stored)
		if err != nil {
			api.Logger.Warn(ctx, "skipping GitHub App credential with an invalid MCP server URL",
				slog.F("mcp_server_config_id", config.ID), slog.Error(err))
			continue
		}
		if githubapp.APIHost(cred.APIBaseURL) != providerHost {
			continue
		}
		tok, err := api.githubAppTokens.Token(ctx, cred)
		if err != nil {
			return agentsdk.ExternalAuthResponse{}, false, xerrors.Errorf("mint GitHub App installation token for MCP server %q: %w", config.Slug, err)
		}
		return agentsdk.ExternalAuthResponse{
			// GitHub's documented git username for installation tokens.
			Username:    "x-access-token",
			Password:    tok.Token,
			AccessToken: tok.Token,
			Type:        provider.Type,
			ExpiresAt:   tok.ExpiresAt.UTC(),
		}, true, nil
	}
	return agentsdk.ExternalAuthResponse{}, false, nil
}

// githubProviderAPIHost is the API host of a GitHub external auth provider.
// The validate URL is preferred because GitHub Enterprise Server providers
// must set it, while the API base URL defaults to github.com.
func githubProviderAPIHost(provider *externalauth.Config) string {
	for _, raw := range []string{provider.ValidateURL, provider.APIBaseURL} {
		if raw == "" {
			continue
		}
		u, err := url.Parse(raw)
		if err == nil && u.Hostname() != "" {
			return strings.ToLower(u.Hostname())
		}
	}
	return ""
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

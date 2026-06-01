package coderd

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"golang.org/x/oauth2"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/promoauth"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
	"github.com/coder/coder/v2/codersdk"
)

// oidcMCPTokenSource implements mcpclient.UserOIDCTokenSource using
// the same refresh strategy as provisionerdserver.ObtainOIDCAccessToken.
// The logic is duplicated to avoid importing provisionerdserver from
// coderd; keep the two in sync.
type oidcMCPTokenSource struct {
	db     database.Store
	config promoauth.OAuth2Config
	logger slog.Logger
}

// newOIDCMCPTokenSource returns nil when no OIDC provider is
// configured. mcpclient treats a nil source the same as "no token
// available" and omits the Authorization header.
func newOIDCMCPTokenSource(db database.Store, config promoauth.OAuth2Config, logger slog.Logger) mcpclient.UserOIDCTokenSource {
	if config == nil {
		return nil
	}
	return &oidcMCPTokenSource{
		db:     db,
		config: config,
		logger: logger,
	}
}

// OIDCAccessToken implements mcpclient.UserOIDCTokenSource. It
// refreshes expired tokens and persists the refreshed token back
// to user_links. The chatd dbauthz subject does not grant
// ResourceSystem.Read or ResourceUser.UpdatePersonal, so DB calls
// elevate to AsSystemRestricted; the per-user authorization is
// already enforced by the API handler that owns ctx.
func (s *oidcMCPTokenSource) OIDCAccessToken(ctx context.Context, userID uuid.UUID) (string, error) {
	//nolint:gocritic // user_links read needs system access; the
	// caller's user identity is supplied via the userID parameter.
	dbCtx := dbauthz.AsSystemRestricted(ctx)
	link, err := s.db.GetUserLinkByUserIDLoginType(dbCtx, database.GetUserLinkByUserIDLoginTypeParams{
		UserID:    userID,
		LoginType: database.LoginTypeOIDC,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", xerrors.Errorf("get oidc user link: %w", err)
	}

	if shouldRefresh, expiresAt := shouldRefreshOIDCToken(link); shouldRefresh {
		token, err := s.config.TokenSource(ctx, &oauth2.Token{
			AccessToken:  link.OAuthAccessToken,
			RefreshToken: link.OAuthRefreshToken,
			// Use the expiresAt returned by shouldRefreshOIDCToken.
			// It will force a refresh with an expired time.
			Expiry: expiresAt,
		}).Token()
		if err != nil {
			// Don't fail the request; the upstream MCP server will see no
			// Authorization header and can return a 401 if it requires one.
			s.logger.Warn(ctx, "failed to refresh OIDC token for MCP request",
				slog.F("user_id", userID),
				slog.Error(err),
			)
			return "", nil
		}
		link.OAuthAccessToken = token.AccessToken
		link.OAuthRefreshToken = token.RefreshToken
		link.OAuthExpiry = token.Expiry

		// Persist on a detached context so a canceled chat request
		// cannot drop a refresh-token rotation, see PR #24332.
		persistCtx, persistCancel := context.WithTimeout(
			context.WithoutCancel(dbCtx), 10*time.Second,
		)
		link, err = s.db.UpdateUserLink(persistCtx, database.UpdateUserLinkParams{
			UserID:                 userID,
			LoginType:              database.LoginTypeOIDC,
			OAuthAccessToken:       link.OAuthAccessToken,
			OAuthAccessTokenKeyID:  sql.NullString{}, // set by dbcrypt if required
			OAuthRefreshToken:      link.OAuthRefreshToken,
			OAuthRefreshTokenKeyID: sql.NullString{}, // set by dbcrypt if required
			OAuthExpiry:            link.OAuthExpiry,
			Claims:                 link.Claims,
		})
		persistCancel()
		if err != nil {
			return "", xerrors.Errorf("update user link after oidc refresh: %w", err)
		}
		s.logger.Info(ctx, "refreshed expired OIDC token for MCP request",
			slog.F("user_id", userID),
		)
	}

	return link.OAuthAccessToken, nil
}

// shouldRefreshOIDCToken mirrors provisionerdserver.shouldRefreshOIDCToken.
// See that function for the rationale behind the 10-minute pre-expiry
// buffer.
func shouldRefreshOIDCToken(link database.UserLink) (bool, time.Time) {
	if link.OAuthRefreshToken == "" {
		return false, link.OAuthExpiry
	}
	if link.OAuthExpiry.IsZero() {
		// A zero expiry means the token never expires.
		return false, link.OAuthExpiry
	}
	expiresAt := link.OAuthExpiry.Add(-time.Minute * 10)
	return expiresAt.Before(dbtime.Now()), expiresAt
}

// @Summary List MCP server configs
// @x-apidocgen {"skip": true}
// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
//nolint:revive // HTTP handler writes to ResponseWriter.
func (api *API) listMCPServerConfigs(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)

	// Admin users can see all MCP server configs (including disabled
	// ones) for management purposes. Non-admin users see only enabled
	// configs, which is sufficient for using the chat feature.
	isAdmin := api.Authorize(r, policy.ActionRead, rbac.ResourceDeploymentConfig)

	var configs []database.MCPServerConfig
	var err error
	if isAdmin {
		configs, err = api.Database.GetMCPServerConfigs(ctx)
	} else {
		//nolint:gocritic // All authenticated users need to read enabled MCP server configs to use the chat feature.
		configs, err = api.Database.GetEnabledMCPServerConfigs(dbauthz.AsSystemRestricted(ctx))
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to list MCP server configs.",
			Detail:  err.Error(),
		})
		return
	}

	// Look up the calling user's OAuth2 tokens so we can populate
	// auth_connected per server. Attempt to refresh expired tokens
	// so the status is accurate and the token is ready for use.
	//nolint:gocritic // Need to check user tokens across all servers.
	userTokens, err := api.Database.GetMCPServerUserTokensByUserID(dbauthz.AsSystemRestricted(ctx), apiKey.UserID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get user tokens.",
			Detail:  err.Error(),
		})
		return
	}

	// Look up the calling user's custom_headers user-set values so
	// auth_connected can reflect whether the user has supplied every
	// required header.
	userHeaderValues, err := api.Database.GetMCPServerUserHeaderValuesByUserID(ctx, apiKey.UserID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get user header values.",
			Detail:  err.Error(),
		})
		return
	}
	headerValuesByConfigID, err := decodeMCPUserHeaderValues(userHeaderValues)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to decode user header values.",
			Detail:  err.Error(),
		})
		return
	}

	// Build a config lookup for the refresh helper.
	configByID := make(map[uuid.UUID]database.MCPServerConfig, len(configs))
	for _, c := range configs {
		configByID[c.ID] = c
	}

	tokenMap := make(map[uuid.UUID]bool, len(userTokens))
	for _, tok := range userTokens {
		cfg, ok := configByID[tok.MCPServerConfigID]
		if !ok {
			continue
		}
		tokenMap[tok.MCPServerConfigID] = api.refreshMCPUserToken(ctx, cfg, tok, apiKey.UserID)
	}

	resp := make([]codersdk.MCPServerConfig, 0, len(configs))
	for _, config := range configs {
		var sdkConfig codersdk.MCPServerConfig
		if isAdmin {
			sdkConfig = convertMCPServerConfig(ctx, api.Logger, config)
		} else {
			sdkConfig = convertMCPServerConfigRedacted(ctx, api.Logger, config)
		}
		switch config.AuthType {
		case "oauth2":
			sdkConfig.AuthConnected = tokenMap[config.ID]
		case "custom_headers":
			sdkConfig.AuthConnected = mcpCustomHeadersConnected(
				headerValuesByConfigID[config.ID],
				config.CustomHeadersUserKeys,
			)
		default:
			sdkConfig.AuthConnected = true
		}
		resp = append(resp, sdkConfig)
	}

	httpapi.Write(ctx, rw, http.StatusOK, resp)
}

// @Summary Create MCP server config
// @x-apidocgen {"skip": true}
// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
//nolint:revive // HTTP handler writes to ResponseWriter.
func (api *API) createMCPServerConfig(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)
	if !api.Authorize(r, policy.ActionUpdate, rbac.ResourceDeploymentConfig) {
		httpapi.Forbidden(rw)
		return
	}

	var req codersdk.CreateMCPServerConfigRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	// Validate auth-type-dependent fields.
	// Reject custom_headers_user_keys for auth types that do not use
	// custom headers, and validate the user-key set against the
	// admin-set headers.
	if len(req.CustomHeadersUserKeys) > 0 && req.AuthType != "custom_headers" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "custom_headers_user_keys is only valid when auth_type is custom_headers.",
		})
		return
	}
	if len(req.CustomHeadersUserKeyDescriptions) > 0 && req.AuthType != "custom_headers" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "custom_headers_user_key_descriptions is only valid when auth_type is custom_headers.",
		})
		return
	}
	customHeadersUserKeys, customHeadersUserKeyDescriptions, err := validateCustomHeaderUserKeys(req.CustomHeadersUserKeys, req.CustomHeaders, req.CustomHeadersUserKeyDescriptions)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid custom_headers_user_keys.",
			Detail:  err.Error(),
		})
		return
	}
	customHeadersUserKeyDescriptionsJSON, err := marshalCustomHeaderUserKeyDescriptions(customHeadersUserKeyDescriptions)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid custom_headers_user_key_descriptions.",
			Detail:  err.Error(),
		})
		return
	}

	switch req.AuthType {
	case "oauth2":
		// When the admin does not provide OAuth2 credentials, attempt
		// automatic discovery and Dynamic Client Registration (RFC 7591)
		// using the MCP server URL.  This follows the MCP authorization
		// spec: discover the authorization server via Protected Resource
		// Metadata (RFC 9728) and Authorization Server Metadata
		// (RFC 8414), then register a client dynamically.
		if req.OAuth2ClientID == "" && req.OAuth2AuthURL == "" && req.OAuth2TokenURL == "" {
			// Auto-discovery flow: we need the config ID first to
			// build the correct callback URL.  Insert the record
			// with empty OAuth2 fields, perform discovery, then
			// update.
			customHeadersJSON, err := marshalCustomHeaders(req.CustomHeaders)
			if err != nil {
				httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
					Message: "Invalid custom headers.",
					Detail:  err.Error(),
				})
				return
			}

			inserted, err := api.Database.InsertMCPServerConfig(ctx, database.InsertMCPServerConfigParams{
				DisplayName:                      strings.TrimSpace(req.DisplayName),
				Slug:                             strings.TrimSpace(req.Slug),
				Description:                      strings.TrimSpace(req.Description),
				IconURL:                          strings.TrimSpace(req.IconURL),
				Transport:                        strings.TrimSpace(req.Transport),
				Url:                              strings.TrimSpace(req.URL),
				AuthType:                         strings.TrimSpace(req.AuthType),
				OAuth2ClientID:                   "",
				OAuth2ClientSecret:               "",
				OAuth2ClientSecretKeyID:          sql.NullString{},
				OAuth2AuthURL:                    "",
				OAuth2TokenURL:                   "",
				OAuth2Scopes:                     "",
				APIKeyHeader:                     strings.TrimSpace(req.APIKeyHeader),
				APIKeyValue:                      strings.TrimSpace(req.APIKeyValue),
				APIKeyValueKeyID:                 sql.NullString{},
				CustomHeaders:                    customHeadersJSON,
				CustomHeadersKeyID:               sql.NullString{},
				CustomHeadersUserKeys:            customHeadersUserKeys,
				CustomHeadersUserKeyDescriptions: customHeadersUserKeyDescriptionsJSON,
				ToolAllowList:                    coalesceStringSlice(trimStringSlice(req.ToolAllowList)),
				ToolDenyList:                     coalesceStringSlice(trimStringSlice(req.ToolDenyList)),
				Availability:                     strings.TrimSpace(req.Availability),
				Enabled:                          req.Enabled,
				ModelIntent:                      req.ModelIntent,
				AllowInPlanMode:                  req.AllowInPlanMode,
				ForwardCoderHeaders:              req.ForwardCoderHeaders,
				CreatedBy:                        apiKey.UserID,
				UpdatedBy:                        apiKey.UserID,
			})
			if err != nil {
				switch {
				case database.IsUniqueViolation(err):
					httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
						Message: "MCP server config already exists.",
						Detail:  err.Error(),
					})
					return
				case database.IsCheckViolation(err):
					httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
						Message: "Invalid MCP server config.",
						Detail:  err.Error(),
					})
					return
				default:
					httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
						Message: "Failed to create MCP server config.",
						Detail:  err.Error(),
					})
					return
				}
			}

			// Now build the callback URL with the actual ID.
			callbackURL := fmt.Sprintf("%s/api/experimental/mcp/servers/%s/oauth2/callback", api.AccessURL.String(), inserted.ID)
			httpClient := api.HTTPClient
			if httpClient == nil {
				httpClient = &http.Client{Timeout: 30 * time.Second}
			}
			result, err := discoverAndRegisterMCPOAuth2(ctx, httpClient, strings.TrimSpace(req.URL), callbackURL)
			if err != nil {
				// Clean up: delete the partially created config.
				deleteErr := api.Database.DeleteMCPServerConfigByID(ctx, inserted.ID)
				if deleteErr != nil {
					api.Logger.Warn(ctx, "failed to clean up MCP server config after OAuth2 discovery failure",
						slog.F("config_id", inserted.ID),
						slog.Error(deleteErr),
					)
				}

				api.Logger.Warn(ctx, "mcp oauth2 auto-discovery failed",
					slog.F("url", req.URL),
					slog.Error(err),
				)
				httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
					Message: "OAuth2 auto-discovery failed. Provide oauth2_client_id, oauth2_auth_url, and oauth2_token_url manually, or ensure the MCP server supports RFC 9728 (Protected Resource Metadata) and RFC 7591 (Dynamic Client Registration).",
					Detail:  err.Error(),
				})
				return
			}

			// Determine scopes: use the request value if provided,
			// otherwise fall back to the discovered value.
			oauth2Scopes := strings.TrimSpace(req.OAuth2Scopes)
			if oauth2Scopes == "" {
				oauth2Scopes = result.scopes
			}

			// Update the record with discovered OAuth2 credentials.
			updated, err := api.Database.UpdateMCPServerConfig(ctx, database.UpdateMCPServerConfigParams{
				ID:                               inserted.ID,
				DisplayName:                      inserted.DisplayName,
				Slug:                             inserted.Slug,
				Description:                      inserted.Description,
				IconURL:                          inserted.IconURL,
				Transport:                        inserted.Transport,
				Url:                              inserted.Url,
				AuthType:                         inserted.AuthType,
				OAuth2ClientID:                   result.clientID,
				OAuth2ClientSecret:               result.clientSecret,
				OAuth2ClientSecretKeyID:          sql.NullString{},
				OAuth2AuthURL:                    result.authURL,
				OAuth2TokenURL:                   result.tokenURL,
				OAuth2Scopes:                     oauth2Scopes,
				APIKeyHeader:                     inserted.APIKeyHeader,
				APIKeyValue:                      inserted.APIKeyValue,
				APIKeyValueKeyID:                 inserted.APIKeyValueKeyID,
				CustomHeaders:                    inserted.CustomHeaders,
				CustomHeadersKeyID:               inserted.CustomHeadersKeyID,
				CustomHeadersUserKeys:            inserted.CustomHeadersUserKeys,
				CustomHeadersUserKeyDescriptions: inserted.CustomHeadersUserKeyDescriptions,
				ToolAllowList:                    inserted.ToolAllowList,
				ToolDenyList:                     inserted.ToolDenyList,
				Availability:                     inserted.Availability,
				Enabled:                          inserted.Enabled,
				ModelIntent:                      inserted.ModelIntent,
				AllowInPlanMode:                  inserted.AllowInPlanMode,
				ForwardCoderHeaders:              inserted.ForwardCoderHeaders,
				UpdatedBy:                        apiKey.UserID,
			})
			if err != nil {
				httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
					Message: "Failed to update MCP server config with OAuth2 credentials.",
					Detail:  err.Error(),
				})
				return
			}

			httpapi.Write(ctx, rw, http.StatusCreated, convertMCPServerConfig(ctx, api.Logger, updated))
			return
		} else if req.OAuth2ClientID == "" || req.OAuth2AuthURL == "" || req.OAuth2TokenURL == "" {
			// Partial manual config: all three fields are required together.
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "OAuth2 auth type requires either all of oauth2_client_id, oauth2_auth_url, and oauth2_token_url (manual configuration), or none of them (automatic discovery via RFC 7591).",
			})
			return
		}
	case "api_key":
		if req.APIKeyHeader == "" || req.APIKeyValue == "" {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "API key auth type requires api_key_header and api_key_value.",
			})
			return
		}
	case "custom_headers":
		if len(req.CustomHeaders)+len(req.CustomHeadersUserKeys) == 0 {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Custom headers auth type requires at least one custom header or custom_headers_user_keys entry.",
			})
			return
		}
	}

	customHeadersJSON, err := marshalCustomHeaders(req.CustomHeaders)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid custom headers.",
			Detail:  err.Error(),
		})
		return
	}

	inserted, err := api.Database.InsertMCPServerConfig(ctx, database.InsertMCPServerConfigParams{
		DisplayName:                      strings.TrimSpace(req.DisplayName),
		Slug:                             strings.TrimSpace(req.Slug),
		Description:                      strings.TrimSpace(req.Description),
		IconURL:                          strings.TrimSpace(req.IconURL),
		Transport:                        strings.TrimSpace(req.Transport),
		Url:                              strings.TrimSpace(req.URL),
		AuthType:                         strings.TrimSpace(req.AuthType),
		OAuth2ClientID:                   strings.TrimSpace(req.OAuth2ClientID),
		OAuth2ClientSecret:               strings.TrimSpace(req.OAuth2ClientSecret),
		OAuth2ClientSecretKeyID:          sql.NullString{},
		OAuth2AuthURL:                    strings.TrimSpace(req.OAuth2AuthURL),
		OAuth2TokenURL:                   strings.TrimSpace(req.OAuth2TokenURL),
		OAuth2Scopes:                     strings.TrimSpace(req.OAuth2Scopes),
		APIKeyHeader:                     strings.TrimSpace(req.APIKeyHeader),
		APIKeyValue:                      strings.TrimSpace(req.APIKeyValue),
		APIKeyValueKeyID:                 sql.NullString{},
		CustomHeaders:                    customHeadersJSON,
		CustomHeadersKeyID:               sql.NullString{},
		CustomHeadersUserKeys:            customHeadersUserKeys,
		CustomHeadersUserKeyDescriptions: customHeadersUserKeyDescriptionsJSON,
		ToolAllowList:                    coalesceStringSlice(trimStringSlice(req.ToolAllowList)),
		ToolDenyList:                     coalesceStringSlice(trimStringSlice(req.ToolDenyList)),
		Availability:                     strings.TrimSpace(req.Availability),
		Enabled:                          req.Enabled,
		ModelIntent:                      req.ModelIntent,
		AllowInPlanMode:                  req.AllowInPlanMode,
		ForwardCoderHeaders:              req.ForwardCoderHeaders,
		CreatedBy:                        apiKey.UserID,
		UpdatedBy:                        apiKey.UserID,
	})
	if err != nil {
		switch {
		case database.IsUniqueViolation(err):
			httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
				Message: "MCP server config already exists.",
				Detail:  err.Error(),
			})
			return
		case database.IsCheckViolation(err):
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Invalid MCP server config.",
				Detail:  err.Error(),
			})
			return
		default:
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to create MCP server config.",
				Detail:  err.Error(),
			})
			return
		}
	}

	httpapi.Write(ctx, rw, http.StatusCreated, convertMCPServerConfig(ctx, api.Logger, inserted))
}

// @Summary Get MCP server config
// @x-apidocgen {"skip": true}
// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
//nolint:revive // HTTP handler writes to ResponseWriter.
func (api *API) getMCPServerConfig(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)

	mcpServerID, ok := parseMCPServerConfigID(rw, r)
	if !ok {
		return
	}

	isAdmin := api.Authorize(r, policy.ActionRead, rbac.ResourceDeploymentConfig)

	var config database.MCPServerConfig
	var err error
	if isAdmin {
		config, err = api.Database.GetMCPServerConfigByID(ctx, mcpServerID)
	} else {
		//nolint:gocritic // All authenticated users can view enabled MCP server configs.
		config, err = api.Database.GetMCPServerConfigByID(dbauthz.AsSystemRestricted(ctx), mcpServerID)
		if err == nil && !config.Enabled {
			httpapi.ResourceNotFound(rw)
			return
		}
	}
	if err != nil {
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get MCP server config.",
			Detail:  err.Error(),
		})
		return
	}

	var sdkConfig codersdk.MCPServerConfig
	if isAdmin {
		sdkConfig = convertMCPServerConfig(ctx, api.Logger, config)
	} else {
		sdkConfig = convertMCPServerConfigRedacted(ctx, api.Logger, config)
	}

	// Populate AuthConnected for the calling user. Attempt to
	// refresh the token so the status is accurate.
	switch config.AuthType {
	case "oauth2":
		//nolint:gocritic // Need to check user token for this server.
		userTokens, err := api.Database.GetMCPServerUserTokensByUserID(dbauthz.AsSystemRestricted(ctx), apiKey.UserID)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to get user tokens.",
				Detail:  err.Error(),
			})
			return
		}
		for _, tok := range userTokens {
			if tok.MCPServerConfigID == config.ID {
				sdkConfig.AuthConnected = api.refreshMCPUserToken(ctx, config, tok, apiKey.UserID)
				break
			}
		}
	case "custom_headers":
		stored := map[string]string{}
		if len(config.CustomHeadersUserKeys) > 0 {
			row, hvErr := api.Database.GetMCPServerUserHeaderValues(ctx, database.GetMCPServerUserHeaderValuesParams{
				MCPServerConfigID: config.ID,
				UserID:            apiKey.UserID,
			})
			if hvErr != nil && !errors.Is(hvErr, sql.ErrNoRows) {
				httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
					Message: "Failed to get user header values.",
					Detail:  hvErr.Error(),
				})
				return
			}
			if hvErr == nil {
				decoded, decErr := decodeHeaderValuesJSON(row.HeaderValues)
				if decErr != nil {
					httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
						Message: "Failed to decode stored user header values.",
						Detail:  decErr.Error(),
					})
					return
				}
				stored = decoded
			}
		}
		sdkConfig.AuthConnected = mcpCustomHeadersConnected(stored, config.CustomHeadersUserKeys)
	default:
		sdkConfig.AuthConnected = true
	}

	httpapi.Write(ctx, rw, http.StatusOK, sdkConfig)
}

// @Summary Update MCP server config
// @x-apidocgen {"skip": true}
// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
//nolint:revive // HTTP handler writes to ResponseWriter.
func (api *API) updateMCPServerConfig(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)
	if !api.Authorize(r, policy.ActionUpdate, rbac.ResourceDeploymentConfig) {
		httpapi.Forbidden(rw)
		return
	}

	mcpServerID, ok := parseMCPServerConfigID(rw, r)
	if !ok {
		return
	}

	var req codersdk.UpdateMCPServerConfigRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	// Pre-validate custom headers before entering the transaction.
	var customHeadersJSON string
	if req.CustomHeaders != nil {
		var chErr error
		customHeadersJSON, chErr = marshalCustomHeaders(*req.CustomHeaders)
		if chErr != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Invalid custom headers.",
				Detail:  chErr.Error(),
			})
			return
		}
	}

	var updated database.MCPServerConfig
	err := api.Database.InTx(func(tx database.Store) error {
		existing, err := tx.GetMCPServerConfigByID(ctx, mcpServerID)
		if err != nil {
			return err
		}

		displayName := existing.DisplayName
		if req.DisplayName != nil {
			displayName = strings.TrimSpace(*req.DisplayName)
		}

		slug := existing.Slug
		if req.Slug != nil {
			slug = strings.TrimSpace(*req.Slug)
		}

		description := existing.Description
		if req.Description != nil {
			description = strings.TrimSpace(*req.Description)
		}

		iconURL := existing.IconURL
		if req.IconURL != nil {
			iconURL = strings.TrimSpace(*req.IconURL)
		}

		transport := existing.Transport
		if req.Transport != nil {
			transport = strings.TrimSpace(*req.Transport)
		}

		serverURL := existing.Url
		if req.URL != nil {
			serverURL = strings.TrimSpace(*req.URL)
		}

		authType := existing.AuthType
		if req.AuthType != nil {
			authType = strings.TrimSpace(*req.AuthType)
		}

		oauth2ClientID := existing.OAuth2ClientID
		if req.OAuth2ClientID != nil {
			oauth2ClientID = strings.TrimSpace(*req.OAuth2ClientID)
		}

		oauth2ClientSecret := existing.OAuth2ClientSecret
		oauth2ClientSecretKeyID := existing.OAuth2ClientSecretKeyID
		if req.OAuth2ClientSecret != nil {
			oauth2ClientSecret = strings.TrimSpace(*req.OAuth2ClientSecret)
			// Clear the key ID when the secret is explicitly updated.
			oauth2ClientSecretKeyID = sql.NullString{}
		}

		oauth2AuthURL := existing.OAuth2AuthURL
		if req.OAuth2AuthURL != nil {
			oauth2AuthURL = strings.TrimSpace(*req.OAuth2AuthURL)
		}

		oauth2TokenURL := existing.OAuth2TokenURL
		if req.OAuth2TokenURL != nil {
			oauth2TokenURL = strings.TrimSpace(*req.OAuth2TokenURL)
		}

		oauth2Scopes := existing.OAuth2Scopes
		if req.OAuth2Scopes != nil {
			oauth2Scopes = strings.TrimSpace(*req.OAuth2Scopes)
		}

		apiKeyHeader := existing.APIKeyHeader
		if req.APIKeyHeader != nil {
			apiKeyHeader = strings.TrimSpace(*req.APIKeyHeader)
		}

		apiKeyValue := existing.APIKeyValue
		apiKeyValueKeyID := existing.APIKeyValueKeyID
		if req.APIKeyValue != nil {
			apiKeyValue = strings.TrimSpace(*req.APIKeyValue)
			// Clear the key ID when the value is explicitly updated.
			apiKeyValueKeyID = sql.NullString{}
		}

		customHeaders := existing.CustomHeaders
		customHeadersKeyID := existing.CustomHeadersKeyID
		if req.CustomHeaders != nil {
			customHeaders = customHeadersJSON
			// Clear the key ID when headers are explicitly updated.
			customHeadersKeyID = sql.NullString{}
		}

		// Compute the final admin headers map for disjointness
		// validation against custom_headers_user_keys.
		var finalAdminHeaders map[string]string
		if req.CustomHeaders != nil {
			finalAdminHeaders = *req.CustomHeaders
		} else {
			decoded, decErr := decodeCustomHeaders(existing.CustomHeaders)
			if decErr != nil {
				return decErr
			}
			finalAdminHeaders = decoded
		}

		customHeadersUserKeys := existing.CustomHeadersUserKeys
		existingDescriptions, descErr := decodeCustomHeaderUserKeyDescriptions(existing.CustomHeadersUserKeyDescriptions)
		if descErr != nil {
			return descErr
		}
		customHeadersUserKeyDescriptions := existingDescriptions
		switch {
		case req.CustomHeadersUserKeys != nil:
			if authType != "custom_headers" && len(*req.CustomHeadersUserKeys) > 0 {
				return &mcpValidationError{msg: "custom_headers_user_keys is only valid when auth_type is custom_headers."}
			}
			// When the caller didn't send descriptions, carry over
			// the existing map but silently drop entries whose key
			// is no longer in the new key set; the validator would
			// otherwise reject this routine refresh.
			var descriptionsInput map[string]string
			if req.CustomHeadersUserKeyDescriptions != nil {
				if authType != "custom_headers" && len(*req.CustomHeadersUserKeyDescriptions) > 0 {
					return &mcpValidationError{msg: "custom_headers_user_key_descriptions is only valid when auth_type is custom_headers."}
				}
				descriptionsInput = *req.CustomHeadersUserKeyDescriptions
			} else {
				descriptionsInput = filterDescriptionsToKeys(existingDescriptions, *req.CustomHeadersUserKeys)
			}
			cleanedKeys, cleanedDescriptions, vErr := validateCustomHeaderUserKeys(*req.CustomHeadersUserKeys, finalAdminHeaders, descriptionsInput)
			if vErr != nil {
				return &mcpValidationError{msg: vErr.Error()}
			}
			customHeadersUserKeys = cleanedKeys
			customHeadersUserKeyDescriptions = cleanedDescriptions
		case req.CustomHeadersUserKeyDescriptions != nil:
			// Keys unchanged; descriptions are being replaced.
			if authType != "custom_headers" && len(*req.CustomHeadersUserKeyDescriptions) > 0 {
				return &mcpValidationError{msg: "custom_headers_user_key_descriptions is only valid when auth_type is custom_headers."}
			}
			_, cleanedDescriptions, vErr := validateCustomHeaderUserKeys(existing.CustomHeadersUserKeys, finalAdminHeaders, *req.CustomHeadersUserKeyDescriptions)
			if vErr != nil {
				return &mcpValidationError{msg: vErr.Error()}
			}
			customHeadersUserKeyDescriptions = cleanedDescriptions
		case req.CustomHeaders != nil && len(existing.CustomHeadersUserKeys) > 0:
			// Admin headers changed but user keys did not; re-validate
			// the unchanged user keys against the new admin map.
			if _, _, vErr := validateCustomHeaderUserKeys(existing.CustomHeadersUserKeys, finalAdminHeaders, existingDescriptions); vErr != nil {
				return &mcpValidationError{msg: vErr.Error()}
			}
		}

		toolAllowList := existing.ToolAllowList
		if req.ToolAllowList != nil {
			toolAllowList = coalesceStringSlice(trimStringSlice(*req.ToolAllowList))
		}

		toolDenyList := existing.ToolDenyList
		if req.ToolDenyList != nil {
			toolDenyList = coalesceStringSlice(trimStringSlice(*req.ToolDenyList))
		}

		availability := existing.Availability
		if req.Availability != nil {
			availability = strings.TrimSpace(*req.Availability)
		}

		enabled := existing.Enabled
		if req.Enabled != nil {
			enabled = *req.Enabled
		}

		modelIntent := existing.ModelIntent
		if req.ModelIntent != nil {
			modelIntent = *req.ModelIntent
		}

		allowInPlanMode := existing.AllowInPlanMode
		if req.AllowInPlanMode != nil {
			allowInPlanMode = *req.AllowInPlanMode
		}

		forwardCoderHeaders := existing.ForwardCoderHeaders
		if req.ForwardCoderHeaders != nil {
			forwardCoderHeaders = *req.ForwardCoderHeaders
		}

		// When auth_type changes, clear fields belonging to the
		// previous auth type so stale secrets don't persist.
		if authType != existing.AuthType {
			switch authType {
			case "none":
				oauth2ClientID = ""
				oauth2ClientSecret = ""
				oauth2ClientSecretKeyID = sql.NullString{}
				oauth2AuthURL = ""
				oauth2TokenURL = ""
				oauth2Scopes = ""
				apiKeyHeader = ""
				apiKeyValue = ""
				apiKeyValueKeyID = sql.NullString{}
				customHeaders = "{}"
				customHeadersKeyID = sql.NullString{}
				customHeadersUserKeys = nil
				customHeadersUserKeyDescriptions = nil
			case "oauth2":
				apiKeyHeader = ""
				apiKeyValue = ""
				apiKeyValueKeyID = sql.NullString{}
				customHeaders = "{}"
				customHeadersKeyID = sql.NullString{}
				customHeadersUserKeys = nil
				customHeadersUserKeyDescriptions = nil
			case "api_key":
				oauth2ClientID = ""
				oauth2ClientSecret = ""
				oauth2ClientSecretKeyID = sql.NullString{}
				oauth2AuthURL = ""
				oauth2TokenURL = ""
				oauth2Scopes = ""
				customHeaders = "{}"
				customHeadersKeyID = sql.NullString{}
				customHeadersUserKeys = nil
				customHeadersUserKeyDescriptions = nil
			case "custom_headers":
				oauth2ClientID = ""
				oauth2ClientSecret = ""
				oauth2ClientSecretKeyID = sql.NullString{}
				oauth2AuthURL = ""
				oauth2TokenURL = ""
				oauth2Scopes = ""
				apiKeyHeader = ""
				apiKeyValue = ""
				apiKeyValueKeyID = sql.NullString{}
			case "user_oidc":
				// user_oidc forwards the calling user's OIDC access token
				// from user_links at request time, so no admin-configured
				// secrets are stored on the row.
				oauth2ClientID = ""
				oauth2ClientSecret = ""
				oauth2ClientSecretKeyID = sql.NullString{}
				oauth2AuthURL = ""
				oauth2TokenURL = ""
				oauth2Scopes = ""
				apiKeyHeader = ""
				apiKeyValue = ""
				apiKeyValueKeyID = sql.NullString{}
				customHeaders = "{}"
				customHeadersKeyID = sql.NullString{}
				customHeadersUserKeys = nil
				customHeadersUserKeyDescriptions = nil
			}
		}

		// Post-merge validation: when staying on or moving to
		// custom_headers, at least one admin header or one
		// user-set key is required. Mirrors the create handler.
		if authType == "custom_headers" && len(finalAdminHeaders)+len(customHeadersUserKeys) == 0 {
			return &mcpValidationError{msg: "Custom headers auth type requires at least one custom header or custom_headers_user_keys entry."}
		}

		// When auth_type changes away from custom_headers or the
		// admin alters the user-set key list, clear every user's
		// stored header values for this config so stale
		// credentials do not silently reactivate if the previous
		// key set is later restored. Equal slices (order-insensitive,
		// case-sensitive) skip the delete so a no-op update keeps
		// each user's values intact.
		if !mcpUserKeySetsEqual(existing.CustomHeadersUserKeys, customHeadersUserKeys) {
			if dErr := tx.DeleteMCPServerUserHeaderValuesByConfigID(ctx, existing.ID); dErr != nil {
				return xerrors.Errorf("clear orphaned user header values: %w", dErr)
			}
		}

		customHeadersUserKeyDescriptionsJSON, mErr := marshalCustomHeaderUserKeyDescriptions(customHeadersUserKeyDescriptions)
		if mErr != nil {
			return mErr
		}

		updated, err = tx.UpdateMCPServerConfig(ctx, database.UpdateMCPServerConfigParams{
			DisplayName:                      displayName,
			Slug:                             slug,
			Description:                      description,
			IconURL:                          iconURL,
			Transport:                        transport,
			Url:                              serverURL,
			AuthType:                         authType,
			OAuth2ClientID:                   oauth2ClientID,
			OAuth2ClientSecret:               oauth2ClientSecret,
			OAuth2ClientSecretKeyID:          oauth2ClientSecretKeyID,
			OAuth2AuthURL:                    oauth2AuthURL,
			OAuth2TokenURL:                   oauth2TokenURL,
			OAuth2Scopes:                     oauth2Scopes,
			APIKeyHeader:                     apiKeyHeader,
			APIKeyValue:                      apiKeyValue,
			APIKeyValueKeyID:                 apiKeyValueKeyID,
			CustomHeaders:                    customHeaders,
			CustomHeadersKeyID:               customHeadersKeyID,
			CustomHeadersUserKeys:            coalesceStringSlice(customHeadersUserKeys),
			CustomHeadersUserKeyDescriptions: customHeadersUserKeyDescriptionsJSON,
			ToolAllowList:                    toolAllowList,
			ToolDenyList:                     toolDenyList,
			Availability:                     availability,
			Enabled:                          enabled,
			ModelIntent:                      modelIntent,
			AllowInPlanMode:                  allowInPlanMode,
			ForwardCoderHeaders:              forwardCoderHeaders,
			UpdatedBy:                        apiKey.UserID,
			ID:                               existing.ID,
		})
		return err
	}, nil)
	if err != nil {
		var vErr *mcpValidationError
		if errors.As(err, &vErr) {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Invalid MCP server config update.",
				Detail:  vErr.Error(),
			})
			return
		}
		switch {
		case httpapi.Is404Error(err):
			httpapi.ResourceNotFound(rw)
			return
		case database.IsUniqueViolation(err):
			httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
				Message: "MCP server config slug already exists.",
				Detail:  err.Error(),
			})
			return
		case database.IsCheckViolation(err):
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Invalid MCP server config.",
				Detail:  err.Error(),
			})
			return
		default:
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to update MCP server config.",
				Detail:  err.Error(),
			})
			return
		}
	}

	httpapi.Write(ctx, rw, http.StatusOK, convertMCPServerConfig(ctx, api.Logger, updated))
}

// @Summary Delete MCP server config
// @x-apidocgen {"skip": true}
// EXPERIMENTAL: this endpoint is experimental and is subject to change.
func (api *API) deleteMCPServerConfig(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !api.Authorize(r, policy.ActionUpdate, rbac.ResourceDeploymentConfig) {
		httpapi.Forbidden(rw)
		return
	}

	mcpServerID, ok := parseMCPServerConfigID(rw, r)
	if !ok {
		return
	}

	if _, err := api.Database.GetMCPServerConfigByID(ctx, mcpServerID); err != nil {
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get MCP server config.",
			Detail:  err.Error(),
		})
		return
	}

	if err := api.Database.DeleteMCPServerConfigByID(ctx, mcpServerID); err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to delete MCP server config.",
			Detail:  err.Error(),
		})
		return
	}

	rw.WriteHeader(http.StatusNoContent)
}

// @Summary Initiate MCP server OAuth2 connect
// @x-apidocgen {"skip": true}
// EXPERIMENTAL: this endpoint is experimental and is subject to change.
// Redirects the user to the MCP server's OAuth2 authorization URL.
//
//nolint:revive // HTTP handler writes to ResponseWriter.
func (api *API) mcpServerOAuth2Connect(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	mcpServerID, ok := parseMCPServerConfigID(rw, r)
	if !ok {
		return
	}

	//nolint:gocritic // Any authenticated user can initiate OAuth2 for an enabled MCP server.
	config, err := api.Database.GetMCPServerConfigByID(dbauthz.AsSystemRestricted(ctx), mcpServerID)
	if err != nil {
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get MCP server config.",
			Detail:  err.Error(),
		})
		return
	}

	if !config.Enabled {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "MCP server is not enabled.",
		})
		return
	}

	if config.AuthType != "oauth2" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "MCP server does not use OAuth2 authentication.",
		})
		return
	}

	if config.OAuth2AuthURL == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "MCP server OAuth2 authorization URL is not configured.",
		})
		return
	}

	// Build the authorization URL. The frontend opens this in a popup.
	// The callback URL is on our server; after the exchange we store
	// the token and close the popup.
	state := uuid.New().String()
	callbackPath := fmt.Sprintf("/api/experimental/mcp/servers/%s/oauth2/callback", config.ID)
	http.SetCookie(rw, api.DeploymentValues.HTTPCookies.Apply(&http.Cookie{
		Name:     "mcp_oauth2_state_" + config.ID.String(),
		Value:    state,
		Path:     callbackPath,
		MaxAge:   600, // 10 minutes
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}))

	// PKCE (RFC 7636) is required by many OAuth2 providers (e.g.
	// Linear). We always send it because it is harmless when the
	// server ignores it and essential when it does not.
	verifier := oauth2.GenerateVerifier()
	http.SetCookie(rw, api.DeploymentValues.HTTPCookies.Apply(&http.Cookie{
		Name:     "mcp_oauth2_verifier_" + config.ID.String(),
		Value:    verifier,
		Path:     callbackPath,
		MaxAge:   600, // 10 minutes
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}))

	oauth2Config := &oauth2.Config{
		ClientID:     config.OAuth2ClientID,
		ClientSecret: config.OAuth2ClientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:  config.OAuth2AuthURL,
			TokenURL: config.OAuth2TokenURL,
		},
		RedirectURL: fmt.Sprintf("%s%s", api.AccessURL.String(), callbackPath),
	}
	var scopes []string
	if config.OAuth2Scopes != "" {
		scopes = strings.Split(config.OAuth2Scopes, " ")
	}
	oauth2Config.Scopes = scopes
	authURL := oauth2Config.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier))
	http.Redirect(rw, r, authURL, http.StatusTemporaryRedirect)
}

// @Summary Handle MCP server OAuth2 callback
// @x-apidocgen {"skip": true}
// EXPERIMENTAL: this endpoint is experimental and is subject to change.
// Exchanges the authorization code for tokens and stores them.
//
//nolint:revive // HTTP handler writes to ResponseWriter.
func (api *API) mcpServerOAuth2Callback(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)

	mcpServerID, ok := parseMCPServerConfigID(rw, r)
	if !ok {
		return
	}

	//nolint:gocritic // Any authenticated user can complete OAuth2 for an enabled MCP server.
	config, err := api.Database.GetMCPServerConfigByID(dbauthz.AsSystemRestricted(ctx), mcpServerID)
	if err != nil {
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get MCP server config.",
			Detail:  err.Error(),
		})
		return
	}

	if !config.Enabled {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "MCP server is not enabled.",
		})
		return
	}

	if config.AuthType != "oauth2" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "MCP server does not use OAuth2 authentication.",
		})
		return
	}

	// Check if the OAuth2 provider returned an error (e.g., user
	// denied consent).
	if oauthError := r.URL.Query().Get("error"); oauthError != "" {
		desc := r.URL.Query().Get("error_description")
		if desc == "" {
			desc = oauthError
		}
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "OAuth2 provider returned an error.",
			Detail:  desc,
		})
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Missing authorization code.",
		})
		return
	}

	// Validate the state parameter for CSRF protection.
	expectedState := ""
	if cookie, err := r.Cookie("mcp_oauth2_state_" + config.ID.String()); err == nil {
		expectedState = cookie.Value
	}
	actualState := r.URL.Query().Get("state")
	if expectedState == "" || actualState != expectedState {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid or missing OAuth2 state parameter.",
		})
		return
	}
	// Clear the state cookie.
	callbackPath := fmt.Sprintf("/api/experimental/mcp/servers/%s/oauth2/callback", config.ID)
	http.SetCookie(rw, api.DeploymentValues.HTTPCookies.Apply(&http.Cookie{
		Name:     "mcp_oauth2_state_" + config.ID.String(),
		Value:    "",
		Path:     callbackPath,
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}))

	// Recover the PKCE code_verifier set during the connect step.
	var exchangeOpts []oauth2.AuthCodeOption
	if verifierCookie, err := r.Cookie("mcp_oauth2_verifier_" + config.ID.String()); err == nil {
		exchangeOpts = append(exchangeOpts, oauth2.VerifierOption(verifierCookie.Value))
	}
	// Clear the verifier cookie regardless of whether it was present.
	http.SetCookie(rw, api.DeploymentValues.HTTPCookies.Apply(&http.Cookie{
		Name:     "mcp_oauth2_verifier_" + config.ID.String(),
		Value:    "",
		Path:     callbackPath,
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}))

	// Exchange the authorization code for tokens.
	oauth2Config := &oauth2.Config{
		ClientID:     config.OAuth2ClientID,
		ClientSecret: config.OAuth2ClientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:  config.OAuth2AuthURL,
			TokenURL: config.OAuth2TokenURL,
		},
		RedirectURL: fmt.Sprintf("%s%s", api.AccessURL.String(), callbackPath),
	}
	var scopes []string
	if config.OAuth2Scopes != "" {
		scopes = strings.Split(config.OAuth2Scopes, " ")
	}
	oauth2Config.Scopes = scopes

	// Use the deployment's HTTP client for the token exchange to
	// respect proxy settings and avoid using http.DefaultClient.
	// Guard against nil so the oauth2 library falls back to the
	// default client instead of panicking.
	exchangeCtx := ctx
	if api.HTTPClient != nil {
		exchangeCtx = context.WithValue(ctx, oauth2.HTTPClient, api.HTTPClient)
	}
	token, err := oauth2Config.Exchange(exchangeCtx, code, exchangeOpts...)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadGateway, codersdk.Response{
			Message: "Failed to exchange authorization code for token.",
			Detail:  "The OAuth2 token exchange with the upstream provider failed.",
		})
		return
	}

	// Store the token for the user.
	refreshToken := ""
	if token.RefreshToken != "" {
		refreshToken = token.RefreshToken
	}

	var expiry sql.NullTime
	if !token.Expiry.IsZero() {
		expiry = sql.NullTime{Time: token.Expiry, Valid: true}
	}

	//nolint:gocritic // Users store their own tokens.
	_, err = api.Database.UpsertMCPServerUserToken(dbauthz.AsSystemRestricted(ctx), database.UpsertMCPServerUserTokenParams{
		MCPServerConfigID: mcpServerID,
		UserID:            apiKey.UserID,
		AccessToken:       token.AccessToken,
		AccessTokenKeyID:  sql.NullString{},
		RefreshToken:      refreshToken,
		RefreshTokenKeyID: sql.NullString{},
		TokenType:         token.TokenType,
		Expiry:            expiry,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to store OAuth2 token.",
			Detail:  err.Error(),
		})
		return
	}

	// Respond with a simple HTML page that closes the popup window.
	rw.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'unsafe-inline'")
	rw.Header().Set("Content-Type", "text/html; charset=utf-8")
	rw.WriteHeader(http.StatusOK)
	_, _ = rw.Write([]byte(`<!DOCTYPE html><html><body><script>
		if (window.opener) {
			window.opener.postMessage({type: "mcp-oauth2-complete", serverID: "` + config.ID.String() + `"}, "` + api.AccessURL.String() + `");
			window.close();
		} else {
			document.body.innerText = "Authentication successful. You may close this window.";
		}
	</script></body></html>`))
}

// @Summary Disconnect MCP server OAuth2 token
// @x-apidocgen {"skip": true}
// EXPERIMENTAL: this endpoint is experimental and is subject to change.
// Removes the user's stored OAuth2 token for an MCP server.
func (api *API) mcpServerOAuth2Disconnect(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)

	mcpServerID, ok := parseMCPServerConfigID(rw, r)
	if !ok {
		return
	}

	//nolint:gocritic // Users manage their own tokens.
	err := api.Database.DeleteMCPServerUserToken(dbauthz.AsSystemRestricted(ctx), database.DeleteMCPServerUserTokenParams{
		MCPServerConfigID: mcpServerID,
		UserID:            apiKey.UserID,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to disconnect OAuth2 token.",
			Detail:  err.Error(),
		})
		return
	}

	rw.WriteHeader(http.StatusNoContent)
}

// @Summary Get MCP user-set custom header values
// @x-apidocgen {"skip": true}
// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
//nolint:revive // HTTP handler writes to ResponseWriter.
func (api *API) getMCPServerUserHeaderValues(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)

	mcpServerID, ok := parseMCPServerConfigID(rw, r)
	if !ok {
		return
	}

	// Load the config to know which keys the admin has marked as
	// user-set. We use system context because the user can't
	// authorize a direct ResourceDeploymentConfig read.
	//nolint:gocritic // Users read their own header values; need config metadata to bound the response.
	cfg, err := api.Database.GetMCPServerConfigByID(dbauthz.AsSystemRestricted(ctx), mcpServerID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get MCP server config.",
			Detail:  err.Error(),
		})
		return
	}
	if !cfg.Enabled {
		httpapi.ResourceNotFound(rw)
		return
	}
	if cfg.AuthType != "custom_headers" || len(cfg.CustomHeadersUserKeys) == 0 {
		// No user-set keys; respond with an empty has_values map.
		httpapi.Write(ctx, rw, http.StatusOK, codersdk.MCPServerUserHeaderValues{
			MCPServerConfigID: cfg.ID,
			HasValues:         map[string]bool{},
		})
		return
	}

	row, err := api.Database.GetMCPServerUserHeaderValues(ctx, database.GetMCPServerUserHeaderValuesParams{
		MCPServerConfigID: mcpServerID,
		UserID:            apiKey.UserID,
	})
	stored := map[string]string{}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get user header values.",
			Detail:  err.Error(),
		})
		return
	}
	if err == nil {
		decoded, decErr := decodeHeaderValuesJSON(row.HeaderValues)
		if decErr != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to decode stored user header values.",
				Detail:  decErr.Error(),
			})
			return
		}
		stored = decoded
	}

	hasValues := make(map[string]bool, len(cfg.CustomHeadersUserKeys))
	for _, key := range cfg.CustomHeadersUserKeys {
		v, _ := headerValueForKey(stored, key)
		hasValues[key] = v != ""
	}
	httpapi.Write(ctx, rw, http.StatusOK, codersdk.MCPServerUserHeaderValues{
		MCPServerConfigID: cfg.ID,
		HasValues:         hasValues,
	})
}

// @Summary Update MCP user-set custom header values
// @x-apidocgen {"skip": true}
// EXPERIMENTAL: this endpoint is experimental and is subject to change.
func (api *API) updateMCPServerUserHeaderValues(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)

	mcpServerID, ok := parseMCPServerConfigID(rw, r)
	if !ok {
		return
	}

	var req codersdk.UpdateMCPServerUserHeaderValuesRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	//nolint:gocritic // Users update their own header values; need config metadata to validate the request.
	cfg, err := api.Database.GetMCPServerConfigByID(dbauthz.AsSystemRestricted(ctx), mcpServerID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get MCP server config.",
			Detail:  err.Error(),
		})
		return
	}
	if !cfg.Enabled {
		httpapi.ResourceNotFound(rw)
		return
	}
	if cfg.AuthType != "custom_headers" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "This MCP server does not support user-set headers. Contact your Coder administrator if you believe this is unexpected.",
		})
		return
	}
	if len(cfg.CustomHeadersUserKeys) == 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "This MCP server has no user-set headers configured. Contact your Coder administrator to add one.",
		})
		return
	}

	// Build a case-insensitive lookup of allowed user keys, preserving
	// the admin's casing for storage.
	allowed := make(map[string]string, len(cfg.CustomHeadersUserKeys))
	for _, k := range cfg.CustomHeadersUserKeys {
		allowed[strings.ToLower(k)] = k
	}

	// Validate every key in the request matches an allowed user key.
	normalized := make(map[string]string, len(req.Values))
	for reqKey, reqVal := range req.Values {
		canonical, ok := allowed[strings.ToLower(strings.TrimSpace(reqKey))]
		if !ok {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: fmt.Sprintf("Header %q is not in the MCP server's user-set custom header keys.", reqKey),
			})
			return
		}
		// Reject control characters that would enable CRLF/null injection
		// into the outgoing MCP request headers.
		if strings.ContainsAny(reqVal, "\r\n\x00") {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: fmt.Sprintf("Header %q value contains disallowed control characters (CR, LF, or NUL).", reqKey),
			})
			return
		}
		if strings.TrimSpace(reqVal) != "" {
			normalized[canonical] = reqVal
		}
	}

	// Merge with any existing stored values so a partial update only
	// overwrites the keys it touches. A user can clear a single value
	// by sending an empty string for that key.
	merged := map[string]string{}
	existing, err := api.Database.GetMCPServerUserHeaderValues(ctx, database.GetMCPServerUserHeaderValuesParams{
		MCPServerConfigID: mcpServerID,
		UserID:            apiKey.UserID,
	})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get existing user header values.",
			Detail:  err.Error(),
		})
		return
	}
	if err == nil {
		decoded, decErr := decodeHeaderValuesJSON(existing.HeaderValues)
		if decErr != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to decode existing user header values.",
				Detail:  decErr.Error(),
			})
			return
		}
		merged = decoded
	}
	for _, k := range cfg.CustomHeadersUserKeys {
		if _, sent := req.Values[k]; !sent {
			// Case-insensitive check for the canonical key.
			alreadyInRequest := false
			for reqKey := range req.Values {
				if strings.EqualFold(strings.TrimSpace(reqKey), k) {
					alreadyInRequest = true
					break
				}
			}
			if alreadyInRequest {
				continue
			}
			// Preserve existing stored value if any (case-insensitive lookup
			// so a case-only admin rename does not silently drop the value).
			if v, has := headerValueForKey(merged, k); has && v != "" {
				normalized[k] = v
			}
		}
	}

	encoded, err := json.Marshal(normalized)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to encode user header values.",
			Detail:  err.Error(),
		})
		return
	}

	if _, err := api.Database.UpsertMCPServerUserHeaderValues(ctx, database.UpsertMCPServerUserHeaderValuesParams{
		MCPServerConfigID: mcpServerID,
		UserID:            apiKey.UserID,
		HeaderValues:      string(encoded),
		HeaderValuesKeyID: sql.NullString{},
	}); err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to save user header values.",
			Detail:  err.Error(),
		})
		return
	}

	hasValues := make(map[string]bool, len(cfg.CustomHeadersUserKeys))
	for _, k := range cfg.CustomHeadersUserKeys {
		hasValues[k] = normalized[k] != ""
	}
	httpapi.Write(ctx, rw, http.StatusOK, codersdk.MCPServerUserHeaderValues{
		MCPServerConfigID: cfg.ID,
		HasValues:         hasValues,
	})
}

// @Summary Delete MCP user-set custom header values
// @x-apidocgen {"skip": true}
// EXPERIMENTAL: this endpoint is experimental and is subject to change.
func (api *API) deleteMCPServerUserHeaderValues(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)

	mcpServerID, ok := parseMCPServerConfigID(rw, r)
	if !ok {
		return
	}

	err := api.Database.DeleteMCPServerUserHeaderValues(ctx, database.DeleteMCPServerUserHeaderValuesParams{
		MCPServerConfigID: mcpServerID,
		UserID:            apiKey.UserID,
	})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to delete user header values.",
			Detail:  err.Error(),
		})
		return
	}
	rw.WriteHeader(http.StatusNoContent)
}

// refreshMCPUserToken attempts to refresh an expired OAuth2 token
// for the given MCP server config. Returns true when the token is
// valid (either still fresh or successfully refreshed), false when
// the token is expired and cannot be refreshed.
func (api *API) refreshMCPUserToken(
	ctx context.Context,
	cfg database.MCPServerConfig,
	tok database.MCPServerUserToken,
	userID uuid.UUID,
) bool {
	if cfg.AuthType != "oauth2" {
		return true
	}
	if tok.RefreshToken == "" {
		// No refresh token — consider connected only if not
		// expired (or no expiry set).
		return !tok.Expiry.Valid || tok.Expiry.Time.After(time.Now())
	}

	result, err := mcpclient.RefreshOAuth2Token(ctx, cfg, tok)
	if err != nil {
		api.Logger.Warn(ctx, "failed to refresh MCP oauth2 token",
			slog.F("server_slug", cfg.Slug),
			slog.Error(err),
		)
		// Refresh failed — token is dead.
		return false
	}

	if result.Refreshed {
		var expiry sql.NullTime
		if !result.Expiry.IsZero() {
			expiry = sql.NullTime{Time: result.Expiry, Valid: true}
		}

		//nolint:gocritic // Need system-level write access to
		// persist the refreshed OAuth2 token.
		_, err = api.Database.UpsertMCPServerUserToken(
			dbauthz.AsSystemRestricted(ctx),
			database.UpsertMCPServerUserTokenParams{
				MCPServerConfigID: tok.MCPServerConfigID,
				UserID:            userID,
				AccessToken:       result.AccessToken,
				AccessTokenKeyID:  sql.NullString{},
				RefreshToken:      result.RefreshToken,
				RefreshTokenKeyID: sql.NullString{},
				TokenType:         result.TokenType,
				Expiry:            expiry,
			},
		)
		if err != nil {
			api.Logger.Warn(ctx, "failed to persist refreshed MCP oauth2 token",
				slog.F("server_slug", cfg.Slug),
				slog.Error(err),
			)
		}
	}

	return true
}

// parseMCPServerConfigID extracts the MCP server config UUID from the
// "mcpServer" path parameter.
func parseMCPServerConfigID(rw http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	mcpServerID, err := uuid.Parse(chi.URLParam(r, "mcpServer"))
	if err != nil {
		httpapi.Write(r.Context(), rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid MCP server config ID.",
			Detail:  err.Error(),
		})
		return uuid.Nil, false
	}
	return mcpServerID, true
}

// convertMCPServerConfig converts a database MCP server config to the
// SDK type. Secrets are never returned; only has_* booleans are set.
// Admin-only fields (OAuth2 client ID, auth URLs, etc.) are included.
// A malformed custom_headers_user_key_descriptions payload is logged
// and defaulted to an empty map so a single corrupt row does not
// break the entire list endpoint.
func convertMCPServerConfig(ctx context.Context, logger slog.Logger, config database.MCPServerConfig) codersdk.MCPServerConfig {
	descriptions, err := decodeCustomHeaderUserKeyDescriptions(config.CustomHeadersUserKeyDescriptions)
	if err != nil {
		logger.Warn(ctx,
			"failed to decode mcp_server_configs.custom_headers_user_key_descriptions; defaulting to empty map",
			slog.F("mcp_server_config_id", config.ID),
			slog.Error(err),
		)
		descriptions = map[string]string{}
	}
	return codersdk.MCPServerConfig{
		ID:          config.ID,
		DisplayName: config.DisplayName,
		Slug:        config.Slug,
		Description: config.Description,
		IconURL:     config.IconURL,

		Transport: config.Transport,
		URL:       config.Url,

		AuthType:        config.AuthType,
		OAuth2ClientID:  config.OAuth2ClientID,
		HasOAuth2Secret: config.OAuth2ClientSecret != "",
		OAuth2AuthURL:   config.OAuth2AuthURL,
		OAuth2TokenURL:  config.OAuth2TokenURL,
		OAuth2Scopes:    config.OAuth2Scopes,

		APIKeyHeader: config.APIKeyHeader,
		HasAPIKey:    config.APIKeyValue != "",

		HasCustomHeaders: len(config.CustomHeaders) > 0 && config.CustomHeaders != "{}",

		CustomHeadersUserKeys:            coalesceStringSlice(config.CustomHeadersUserKeys),
		CustomHeadersUserKeyDescriptions: descriptions,

		ToolAllowList: coalesceStringSlice(config.ToolAllowList),
		ToolDenyList:  coalesceStringSlice(config.ToolDenyList),

		Availability: config.Availability,

		Enabled:             config.Enabled,
		ModelIntent:         config.ModelIntent,
		AllowInPlanMode:     config.AllowInPlanMode,
		ForwardCoderHeaders: config.ForwardCoderHeaders,
		CreatedAt:           config.CreatedAt,
		UpdatedAt:           config.UpdatedAt,
	}
}

// convertMCPServerConfigRedacted returns the same SDK config as
// convertMCPServerConfig but strips admin-only fields (URL, transport,
// OAuth2 client/auth/token URLs, scopes, API key header) so the
// payload is safe to expose to non-admin callers. Non-secret
// metadata such as auth_type, has_oauth2_secret, has_api_key,
// has_custom_headers, and the user-set custom header key list is
// retained because end users need it to wire up their own values.
func convertMCPServerConfigRedacted(ctx context.Context, logger slog.Logger, config database.MCPServerConfig) codersdk.MCPServerConfig {
	c := convertMCPServerConfig(ctx, logger, config)
	c.URL = ""
	c.Transport = ""
	c.OAuth2ClientID = ""
	c.OAuth2AuthURL = ""
	c.OAuth2TokenURL = ""
	c.OAuth2Scopes = ""
	c.APIKeyHeader = ""
	return c
}

// marshalCustomHeaders encodes a map of custom headers to JSON for
// database storage. A nil map produces an empty JSON object.
func marshalCustomHeaders(headers map[string]string) (string, error) {
	if headers == nil {
		return "{}", nil
	}
	encoded, err := json.Marshal(headers)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// marshalCustomHeaderUserKeyDescriptions encodes the per-key
// description map for storage in the JSONB column. A nil or empty
// map produces an empty JSON object so the NOT NULL column never
// receives SQL NULL.
func marshalCustomHeaderUserKeyDescriptions(descriptions map[string]string) (json.RawMessage, error) {
	if len(descriptions) == 0 {
		return json.RawMessage("{}"), nil
	}
	encoded, err := json.Marshal(descriptions)
	if err != nil {
		return nil, err
	}
	return encoded, nil
}

// decodeCustomHeaderUserKeyDescriptions decodes the JSONB column
// into a Go map. Empty or null payloads decode to an empty map.
func decodeCustomHeaderUserKeyDescriptions(raw json.RawMessage) (map[string]string, error) {
	if len(raw) == 0 || string(raw) == "{}" || string(raw) == "null" {
		return map[string]string{}, nil
	}
	var out map[string]string
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, xerrors.Errorf("decode custom_headers_user_key_descriptions: %w", err)
	}
	if out == nil {
		return map[string]string{}, nil
	}
	return out, nil
}

// mcpValidationError signals that the InTx update closure failed due
// to a validation rule the caller should fix; the post-tx error
// handler maps it to HTTP 400.
type mcpValidationError struct{ msg string }

func (e *mcpValidationError) Error() string { return e.msg }

// decodeCustomHeaders decodes the database custom_headers JSON column
// to a map. Returns an empty map when the column is empty or "{}".
func decodeCustomHeaders(headers string) (map[string]string, error) {
	if headers == "" || headers == "{}" {
		return map[string]string{}, nil
	}
	var out map[string]string
	if err := json.Unmarshal([]byte(headers), &out); err != nil {
		return nil, xerrors.Errorf("decode custom_headers: %w", err)
	}
	return out, nil
}

// decodeMCPUserHeaderValues decodes each row's header_values JSON
// into a per-config map for the calling user.
func decodeMCPUserHeaderValues(rows []database.McpServerUserHeaderValue) (map[uuid.UUID]map[string]string, error) {
	out := make(map[uuid.UUID]map[string]string, len(rows))
	for _, row := range rows {
		values, err := decodeHeaderValuesJSON(row.HeaderValues)
		if err != nil {
			return nil, xerrors.Errorf("decode mcp_server_user_header_values for config %s: %w", row.MCPServerConfigID, err)
		}
		out[row.MCPServerConfigID] = values
	}
	return out, nil
}

// decodeHeaderValuesJSON decodes the header_values text column from
// mcp_server_user_header_values into a map. An empty or whitespace-only
// payload decodes to an empty map; malformed JSON returns an error.
func decodeHeaderValuesJSON(raw string) (map[string]string, error) {
	if strings.TrimSpace(raw) == "" {
		return map[string]string{}, nil
	}
	var out map[string]string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, xerrors.Errorf("decode header_values: %w", err)
	}
	if out == nil {
		return map[string]string{}, nil
	}
	return out, nil
}

// headerValueForKey returns the stored value for key using a
// case-insensitive match. Admin-defined keys preserve their original
// casing in storage, so a later case-only rename of a user-set key
// would otherwise orphan the stored value until the user re-saves.
func headerValueForKey(stored map[string]string, key string) (string, bool) {
	if v, ok := stored[key]; ok {
		return v, true
	}
	for k, v := range stored {
		if strings.EqualFold(k, key) {
			return v, true
		}
	}
	return "", false
}

// mcpUserKeySetsEqual returns true when a and b contain the same
// keys, ignoring order. Comparison is case-sensitive, so a case-only
// admin rename of a user-set key is treated as a change and triggers
// orphaned-value cleanup.
func mcpUserKeySetsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[string]struct{}, len(a))
	for _, s := range a {
		seen[s] = struct{}{}
	}
	for _, s := range b {
		if _, ok := seen[s]; !ok {
			return false
		}
	}
	return true
}

// mcpCustomHeadersConnected returns true when every key in
// requiredKeys has a non-empty stored value. When requiredKeys is
// empty the connection is considered fully configured (admin headers
// alone are sufficient). The stored lookup is case-insensitive so a
// case-only admin rename does not flip a configured server back to
// disconnected until the user re-saves.
func mcpCustomHeadersConnected(stored map[string]string, requiredKeys []string) bool {
	for _, k := range requiredKeys {
		v, _ := headerValueForKey(stored, k)
		if strings.TrimSpace(v) == "" {
			return false
		}
	}
	return true
}

// filterDescriptionsToKeys returns a copy of descriptions that only
// contains entries whose key matches (case-insensitively) an entry
// in keys. This is used when an admin updates the user-key list
// without explicitly providing descriptions, so orphaned
// descriptions for removed keys are silently dropped.
func filterDescriptionsToKeys(descriptions map[string]string, keys []string) map[string]string {
	if len(descriptions) == 0 || len(keys) == 0 {
		return map[string]string{}
	}
	allowed := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		allowed[strings.ToLower(strings.TrimSpace(k))] = struct{}{}
	}
	filtered := make(map[string]string, len(descriptions))
	for k, v := range descriptions {
		if _, ok := allowed[strings.ToLower(strings.TrimSpace(k))]; ok {
			filtered[k] = v
		}
	}
	return filtered
}

// validateCustomHeaderUserKeys returns the cleaned (trimmed, deduped)
// list of user-set custom header names and the cleaned description
// map. Header names are compared case-insensitively per RFC 7230, but
// the original casing is preserved for storage.
//
// It rejects: empty entries (after trim), case-insensitive duplicates,
// any name that collides (case-insensitively) with a key in
// adminHeaders, and any description whose key does not match (case-
// insensitively) one of the user keys.
//
// Empty-string description values are dropped. Description keys are
// rewritten to use the canonical casing from cleaned user keys so
// callers can index by exact match.
//
// An empty userKeys input returns an empty slice, an empty map, and
// no error; the caller is responsible for any auth-type-specific
// "at least one header" check.
func validateCustomHeaderUserKeys(userKeys []string, adminHeaders map[string]string, descriptions map[string]string) ([]string, map[string]string, error) {
	if len(userKeys) == 0 {
		if len(descriptions) > 0 {
			return nil, nil, xerrors.New("custom_headers_user_key_descriptions requires at least one entry in custom_headers_user_keys")
		}
		return []string{}, map[string]string{}, nil
	}
	seen := make(map[string]string, len(userKeys))
	cleaned := make([]string, 0, len(userKeys))
	for _, raw := range userKeys {
		k := strings.TrimSpace(raw)
		if k == "" {
			return nil, nil, xerrors.New("custom_headers_user_keys entries must not be empty")
		}
		lk := strings.ToLower(k)
		if _, dup := seen[lk]; dup {
			return nil, nil, xerrors.Errorf("duplicate custom_headers_user_keys entry %q", k)
		}
		seen[lk] = k
		cleaned = append(cleaned, k)
	}
	for adminKey := range adminHeaders {
		if _, conflict := seen[strings.ToLower(strings.TrimSpace(adminKey))]; conflict {
			return nil, nil, xerrors.Errorf("custom_headers_user_keys must be disjoint from custom_headers; %q is set by both", adminKey)
		}
	}
	cleanedDescriptions := make(map[string]string, len(descriptions))
	for rawKey, rawValue := range descriptions {
		lk := strings.ToLower(strings.TrimSpace(rawKey))
		canonical, ok := seen[lk]
		if !ok {
			return nil, nil, xerrors.Errorf("custom_headers_user_key_descriptions key %q is not in custom_headers_user_keys", rawKey)
		}
		if _, dup := cleanedDescriptions[canonical]; dup {
			return nil, nil, xerrors.Errorf("duplicate custom_headers_user_key_descriptions entry %q", rawKey)
		}
		value := strings.TrimSpace(rawValue)
		if value == "" {
			continue
		}
		cleanedDescriptions[canonical] = value
	}
	return cleaned, cleanedDescriptions, nil
}

// trimStringSlice trims whitespace from each element and drops empty
// strings.
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

// coalesceStringSlice returns ss if non-nil, otherwise an empty
// non-nil slice. This prevents pq.Array from sending NULL for
// NOT NULL text[] columns.
func coalesceStringSlice(ss []string) []string {
	if ss == nil {
		return []string{}
	}
	return ss
}

// mcpOAuth2Discovery holds the result of MCP OAuth2 auto-discovery
// and Dynamic Client Registration.
type mcpOAuth2Discovery struct {
	clientID     string
	clientSecret string
	authURL      string
	tokenURL     string
	scopes       string // space-separated
}

// protectedResourceMetadata represents the response from a
// Protected Resource Metadata endpoint per RFC 9728 §2.
type protectedResourceMetadata struct {
	Resource             string   `json:"resource"`
	AuthorizationServers []string `json:"authorization_servers"`
	ScopesSupported      []string `json:"scopes_supported,omitempty"`
}

// authServerMetadata represents the response from an Authorization
// Server Metadata endpoint per RFC 8414 §2.
type authServerMetadata struct {
	Issuer                string   `json:"issuer"`
	AuthorizationEndpoint string   `json:"authorization_endpoint"`
	TokenEndpoint         string   `json:"token_endpoint"`
	RegistrationEndpoint  string   `json:"registration_endpoint,omitempty"`
	ScopesSupported       []string `json:"scopes_supported,omitempty"`
}

// fetchJSON performs a GET request to the given URL with the
// standard MCP OAuth2 discovery headers and decodes the JSON
// response into dest. It returns nil on success or an error
// if the request fails or the server returns a non-200 status.
func fetchJSON(ctx context.Context, httpClient *http.Client, rawURL string, dest any) error {
	req, err := http.NewRequestWithContext(
		ctx, http.MethodGet, rawURL, nil,
	)
	if err != nil {
		return xerrors.Errorf("create request for %s: %w", rawURL, err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("MCP-Protocol-Version", mcp.LATEST_PROTOCOL_VERSION)

	resp, err := httpClient.Do(req)
	if err != nil {
		return xerrors.Errorf("GET %s: %w", rawURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return xerrors.Errorf(
			"GET %s returned HTTP %d", rawURL, resp.StatusCode,
		)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return xerrors.Errorf(
			"read response from %s: %w", rawURL, err,
		)
	}

	if err := json.Unmarshal(body, dest); err != nil {
		return xerrors.Errorf(
			"decode JSON from %s: %w", rawURL, err,
		)
	}

	return nil
}

// discoverProtectedResource discovers the Protected Resource
// Metadata for the given MCP server per RFC 9728 §3.1. It
// tries the path-aware well-known URL first, then falls back
// to the root-level URL.
//
// Path-aware: GET {origin}/.well-known/oauth-protected-resource{path}
// Root:       GET {origin}/.well-known/oauth-protected-resource
func discoverProtectedResource(
	ctx context.Context, httpClient *http.Client, origin, path string,
) (*protectedResourceMetadata, error) {
	var urls []string

	// Per RFC 9728 §3.1, when the resource URL contains a
	// path component, the well-known URI is constructed by
	// inserting the well-known prefix before the path.
	if path != "" && path != "/" {
		urls = append(
			urls,
			origin+"/.well-known/oauth-protected-resource"+path,
		)
	}
	// Always try the root-level URL as a fallback.
	urls = append(
		urls, origin+"/.well-known/oauth-protected-resource",
	)

	var lastErr error
	for _, u := range urls {
		var meta protectedResourceMetadata
		if err := fetchJSON(ctx, httpClient, u, &meta); err != nil {
			lastErr = err
			continue
		}
		if len(meta.AuthorizationServers) == 0 {
			lastErr = xerrors.Errorf(
				"protected resource metadata at %s "+
					"has no authorization_servers", u,
			)
			continue
		}
		return &meta, nil
	}

	return nil, xerrors.Errorf(
		"discover protected resource metadata: %w", lastErr,
	)
}

// discoverAuthServerMetadata discovers the Authorization Server
// Metadata per RFC 8414 §3.1. When the authorization server
// issuer URL has a path component, the metadata URL is
// path-aware. Falls back to root-level and OpenID Connect
// discovery as a last resort.
//
// Path-aware: {origin}/.well-known/oauth-authorization-server{path}
// Root:       {origin}/.well-known/oauth-authorization-server
// OpenID:     {issuer}/.well-known/openid-configuration
func discoverAuthServerMetadata(
	ctx context.Context, httpClient *http.Client, authServerURL string,
) (*authServerMetadata, error) {
	parsed, err := url.Parse(authServerURL)
	if err != nil {
		return nil, xerrors.Errorf(
			"parse auth server URL: %w", err,
		)
	}
	asOrigin := fmt.Sprintf(
		"%s://%s", parsed.Scheme, parsed.Host,
	)
	asPath := parsed.Path

	var urls []string

	// Per RFC 8414 §3.1, if the issuer URL has a path,
	// insert the well-known prefix before the path.
	if asPath != "" && asPath != "/" {
		urls = append(
			urls,
			asOrigin+"/.well-known/oauth-authorization-server"+asPath,
		)
	}
	// Root-level fallback.
	urls = append(
		urls,
		asOrigin+"/.well-known/oauth-authorization-server",
	)
	// OpenID Connect discovery as a last resort. Note: this is
	// tried after RFC 8414 (unlike the previous mcp-go code that
	// tried OIDC first) because RFC 8414 is the MCP spec's
	// recommended discovery mechanism.
	// Per OpenID Connect Discovery 1.0 §4, the well-known URL
	// is formed by appending to the full issuer (including
	// path), not just the origin.
	urls = append(
		urls,
		strings.TrimRight(authServerURL, "/")+
			"/.well-known/openid-configuration",
	)

	var lastErr error
	for _, u := range urls {
		var meta authServerMetadata
		if err := fetchJSON(ctx, httpClient, u, &meta); err != nil {
			lastErr = err
			continue
		}
		if meta.AuthorizationEndpoint == "" || meta.TokenEndpoint == "" {
			lastErr = xerrors.Errorf(
				"auth server metadata at %s missing required "+
					"endpoints", u,
			)
			continue
		}
		return &meta, nil
	}

	return nil, xerrors.Errorf(
		"discover auth server metadata: %w", lastErr,
	)
}

// registerOAuth2Client performs Dynamic Client Registration per
// RFC 7591 by POSTing client metadata to the registration
// endpoint and returning the assigned client_id and optional
// client_secret.
func registerOAuth2Client(
	ctx context.Context, httpClient *http.Client,
	registrationEndpoint, callbackURL, clientName string,
) (clientID string, clientSecret string, err error) {
	payload := map[string]any{
		"client_name":                clientName,
		"redirect_uris":              []string{callbackURL},
		"token_endpoint_auth_method": "none",
		"grant_types":                []string{"authorization_code", "refresh_token"},
		"response_types":             []string{"code"},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", "", xerrors.Errorf(
			"marshal registration request: %w", err,
		)
	}

	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost,
		registrationEndpoint, bytes.NewReader(body),
	)
	if err != nil {
		return "", "", xerrors.Errorf(
			"create registration request: %w", err,
		)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", "", xerrors.Errorf(
			"POST %s: %w", registrationEndpoint, err,
		)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", "", xerrors.Errorf(
			"read registration response: %w", err,
		)
	}

	if resp.StatusCode != http.StatusOK &&
		resp.StatusCode != http.StatusCreated {
		// Truncate to avoid leaking verbose upstream errors
		// through the API.
		const maxErrBody = 512
		errMsg := string(respBody)
		if len(errMsg) > maxErrBody {
			errMsg = errMsg[:maxErrBody] + "..."
		}
		return "", "", xerrors.Errorf(
			"registration endpoint returned HTTP %d: %s",
			resp.StatusCode, errMsg,
		)
	}

	var result struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", "", xerrors.Errorf(
			"decode registration response: %w", err,
		)
	}
	if result.ClientID == "" {
		return "", "", xerrors.New(
			"registration response missing client_id",
		)
	}

	return result.ClientID, result.ClientSecret, nil
}

// discoverAndRegisterMCPOAuth2 performs the full MCP OAuth2
// discovery and Dynamic Client Registration flow:
//
//  1. Discover the authorization server via Protected Resource
//     Metadata (RFC 9728).
//  2. Fetch Authorization Server Metadata (RFC 8414).
//  3. Register a client via Dynamic Client Registration
//     (RFC 7591).
//  4. Return the discovered endpoints and credentials.
//
// Unlike a root-only approach, this implementation follows the
// path-aware well-known URI construction rules from RFC 9728
// §3.1 and RFC 8414 §3.1, which is required for servers that
// serve metadata at path-specific URLs (e.g.
// https://api.githubcopilot.com/mcp/).
func discoverAndRegisterMCPOAuth2(ctx context.Context, httpClient *http.Client, mcpServerURL, callbackURL string) (*mcpOAuth2Discovery, error) {
	// Parse the MCP server URL into origin and path.
	parsed, err := url.Parse(mcpServerURL)
	if err != nil {
		return nil, xerrors.Errorf(
			"parse MCP server URL: %w", err,
		)
	}
	origin := fmt.Sprintf("%s://%s", parsed.Scheme, parsed.Host)
	path := parsed.Path

	// Step 1: Discover the Protected Resource Metadata
	// (RFC 9728) to find the authorization server.
	prm, err := discoverProtectedResource(ctx, httpClient, origin, path)
	if err != nil {
		return nil, xerrors.Errorf(
			"protected resource discovery: %w", err,
		)
	}

	// Step 2: Fetch Authorization Server Metadata (RFC 8414)
	// from the first advertised authorization server.
	asMeta, err := discoverAuthServerMetadata(
		ctx, httpClient, prm.AuthorizationServers[0],
	)
	if err != nil {
		return nil, xerrors.Errorf(
			"auth server metadata discovery: %w", err,
		)
	}

	// Only RegistrationEndpoint needs checking here;
	// discoverAuthServerMetadata already validates that
	// AuthorizationEndpoint and TokenEndpoint are present.
	if asMeta.RegistrationEndpoint == "" {
		return nil, xerrors.New(
			"authorization server does not advertise a " +
				"registration_endpoint (dynamic client " +
				"registration may not be supported)",
		)
	}

	// Step 3: Register via Dynamic Client Registration
	// (RFC 7591).
	clientID, clientSecret, err := registerOAuth2Client(
		ctx, httpClient, asMeta.RegistrationEndpoint, callbackURL, "Coder",
	)
	if err != nil {
		return nil, xerrors.Errorf(
			"dynamic client registration: %w", err,
		)
	}

	scopes := strings.Join(asMeta.ScopesSupported, " ")

	return &mcpOAuth2Discovery{
		clientID:     clientID,
		clientSecret: clientSecret,
		authURL:      asMeta.AuthorizationEndpoint,
		tokenURL:     asMeta.TokenEndpoint,
		scopes:       scopes,
	}, nil
}

package coderd

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/client/transport"
	"golang.org/x/oauth2"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
)

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
	// auth_connected per server.
	//nolint:gocritic // Need to check user tokens across all servers.
	userTokens, err := api.Database.GetMCPServerUserTokensByUserID(dbauthz.AsSystemRestricted(ctx), apiKey.UserID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get user tokens.",
			Detail:  err.Error(),
		})
		return
	}
	tokenMap := make(map[uuid.UUID]bool, len(userTokens))
	for _, t := range userTokens {
		tokenMap[t.MCPServerConfigID] = true
	}

	resp := make([]codersdk.MCPServerConfig, 0, len(configs))
	for _, config := range configs {
		var sdkConfig codersdk.MCPServerConfig
		if isAdmin {
			sdkConfig = convertMCPServerConfig(config)
		} else {
			sdkConfig = convertMCPServerConfigRedacted(config)
		}
		if config.AuthType == "oauth2" {
			sdkConfig.AuthConnected = tokenMap[config.ID]
		} else {
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
				DisplayName:             strings.TrimSpace(req.DisplayName),
				Slug:                    strings.TrimSpace(req.Slug),
				Description:             strings.TrimSpace(req.Description),
				IconURL:                 strings.TrimSpace(req.IconURL),
				Transport:               strings.TrimSpace(req.Transport),
				Url:                     strings.TrimSpace(req.URL),
				AuthType:                strings.TrimSpace(req.AuthType),
				OAuth2ClientID:          "",
				OAuth2ClientSecret:      "",
				OAuth2ClientSecretKeyID: sql.NullString{},
				OAuth2AuthURL:           "",
				OAuth2TokenURL:          "",
				OAuth2Scopes:            "",
				APIKeyHeader:            strings.TrimSpace(req.APIKeyHeader),
				APIKeyValue:             strings.TrimSpace(req.APIKeyValue),
				APIKeyValueKeyID:        sql.NullString{},
				CustomHeaders:           customHeadersJSON,
				CustomHeadersKeyID:      sql.NullString{},
				ToolAllowList:           coalesceStringSlice(trimStringSlice(req.ToolAllowList)),
				ToolDenyList:            coalesceStringSlice(trimStringSlice(req.ToolDenyList)),
				Availability:            strings.TrimSpace(req.Availability),
				Enabled:                 req.Enabled,
				CreatedBy:               apiKey.UserID,
				UpdatedBy:               apiKey.UserID,
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
			result, err := discoverAndRegisterMCPOAuth2(ctx, strings.TrimSpace(req.URL), callbackURL)
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
					Message: "This MCP server requires manual OAuth2 configuration.",
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
				ID:                      inserted.ID,
				DisplayName:             inserted.DisplayName,
				Slug:                    inserted.Slug,
				Description:             inserted.Description,
				IconURL:                 inserted.IconURL,
				Transport:               inserted.Transport,
				Url:                     inserted.Url,
				AuthType:                inserted.AuthType,
				OAuth2ClientID:          result.clientID,
				OAuth2ClientSecret:      result.clientSecret,
				OAuth2ClientSecretKeyID: sql.NullString{},
				OAuth2AuthURL:           result.authURL,
				OAuth2TokenURL:          result.tokenURL,
				OAuth2Scopes:            oauth2Scopes,
				APIKeyHeader:            inserted.APIKeyHeader,
				APIKeyValue:             inserted.APIKeyValue,
				APIKeyValueKeyID:        inserted.APIKeyValueKeyID,
				CustomHeaders:           inserted.CustomHeaders,
				CustomHeadersKeyID:      inserted.CustomHeadersKeyID,
				ToolAllowList:           inserted.ToolAllowList,
				ToolDenyList:            inserted.ToolDenyList,
				Availability:            inserted.Availability,
				Enabled:                 inserted.Enabled,
				UpdatedBy:               apiKey.UserID,
			})
			if err != nil {
				httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
					Message: "Failed to update MCP server config with OAuth2 credentials.",
					Detail:  err.Error(),
				})
				return
			}

			httpapi.Write(ctx, rw, http.StatusCreated, convertMCPServerConfig(updated))
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
		if len(req.CustomHeaders) == 0 {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Custom headers auth type requires at least one custom header.",
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
		DisplayName:             strings.TrimSpace(req.DisplayName),
		Slug:                    strings.TrimSpace(req.Slug),
		Description:             strings.TrimSpace(req.Description),
		IconURL:                 strings.TrimSpace(req.IconURL),
		Transport:               strings.TrimSpace(req.Transport),
		Url:                     strings.TrimSpace(req.URL),
		AuthType:                strings.TrimSpace(req.AuthType),
		OAuth2ClientID:          strings.TrimSpace(req.OAuth2ClientID),
		OAuth2ClientSecret:      strings.TrimSpace(req.OAuth2ClientSecret),
		OAuth2ClientSecretKeyID: sql.NullString{},
		OAuth2AuthURL:           strings.TrimSpace(req.OAuth2AuthURL),
		OAuth2TokenURL:          strings.TrimSpace(req.OAuth2TokenURL),
		OAuth2Scopes:            strings.TrimSpace(req.OAuth2Scopes),
		APIKeyHeader:            strings.TrimSpace(req.APIKeyHeader),
		APIKeyValue:             strings.TrimSpace(req.APIKeyValue),
		APIKeyValueKeyID:        sql.NullString{},
		CustomHeaders:           customHeadersJSON,
		CustomHeadersKeyID:      sql.NullString{},
		ToolAllowList:           coalesceStringSlice(trimStringSlice(req.ToolAllowList)),
		ToolDenyList:            coalesceStringSlice(trimStringSlice(req.ToolDenyList)),
		Availability:            strings.TrimSpace(req.Availability),
		Enabled:                 req.Enabled,
		CreatedBy:               apiKey.UserID,
		UpdatedBy:               apiKey.UserID,
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

	httpapi.Write(ctx, rw, http.StatusCreated, convertMCPServerConfig(inserted))
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
		sdkConfig = convertMCPServerConfig(config)
	} else {
		sdkConfig = convertMCPServerConfigRedacted(config)
	}

	// Populate AuthConnected for the calling user.
	if config.AuthType == "oauth2" {
		//nolint:gocritic // Need to check user token for this server.
		userTokens, err := api.Database.GetMCPServerUserTokensByUserID(dbauthz.AsSystemRestricted(ctx), apiKey.UserID)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to get user tokens.",
				Detail:  err.Error(),
			})
			return
		}
		for _, t := range userTokens {
			if t.MCPServerConfigID == config.ID {
				sdkConfig.AuthConnected = true
				break
			}
		}
	} else {
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
			case "oauth2":
				apiKeyHeader = ""
				apiKeyValue = ""
				apiKeyValueKeyID = sql.NullString{}
				customHeaders = "{}"
				customHeadersKeyID = sql.NullString{}
			case "api_key":
				oauth2ClientID = ""
				oauth2ClientSecret = ""
				oauth2ClientSecretKeyID = sql.NullString{}
				oauth2AuthURL = ""
				oauth2TokenURL = ""
				oauth2Scopes = ""
				customHeaders = "{}"
				customHeadersKeyID = sql.NullString{}
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
			}
		}

		updated, err = tx.UpdateMCPServerConfig(ctx, database.UpdateMCPServerConfigParams{
			DisplayName:             displayName,
			Slug:                    slug,
			Description:             description,
			IconURL:                 iconURL,
			Transport:               transport,
			Url:                     serverURL,
			AuthType:                authType,
			OAuth2ClientID:          oauth2ClientID,
			OAuth2ClientSecret:      oauth2ClientSecret,
			OAuth2ClientSecretKeyID: oauth2ClientSecretKeyID,
			OAuth2AuthURL:           oauth2AuthURL,
			OAuth2TokenURL:          oauth2TokenURL,
			OAuth2Scopes:            oauth2Scopes,
			APIKeyHeader:            apiKeyHeader,
			APIKeyValue:             apiKeyValue,
			APIKeyValueKeyID:        apiKeyValueKeyID,
			CustomHeaders:           customHeaders,
			CustomHeadersKeyID:      customHeadersKeyID,
			ToolAllowList:           toolAllowList,
			ToolDenyList:            toolDenyList,
			Availability:            availability,
			Enabled:                 enabled,
			UpdatedBy:               apiKey.UserID,
			ID:                      existing.ID,
		})
		return err
	}, nil)
	if err != nil {
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

	httpapi.Write(ctx, rw, http.StatusOK, convertMCPServerConfig(updated))
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
	http.SetCookie(rw, api.DeploymentValues.HTTPCookies.Apply(&http.Cookie{
		Name:     "mcp_oauth2_state_" + config.ID.String(),
		Value:    state,
		Path:     fmt.Sprintf("/api/experimental/mcp/servers/%s/oauth2/callback", config.ID),
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
		RedirectURL: fmt.Sprintf("%s/api/experimental/mcp/servers/%s/oauth2/callback", api.AccessURL.String(), config.ID),
	}
	var scopes []string
	if config.OAuth2Scopes != "" {
		scopes = strings.Split(config.OAuth2Scopes, " ")
	}
	oauth2Config.Scopes = scopes
	authURL := oauth2Config.AuthCodeURL(state)
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
	http.SetCookie(rw, api.DeploymentValues.HTTPCookies.Apply(&http.Cookie{
		Name:     "mcp_oauth2_state_" + config.ID.String(),
		Value:    "",
		Path:     fmt.Sprintf("/api/experimental/mcp/servers/%s/oauth2/callback", config.ID),
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
		RedirectURL: fmt.Sprintf("%s/api/experimental/mcp/servers/%s/oauth2/callback", api.AccessURL.String(), config.ID),
	}
	var scopes []string
	if config.OAuth2Scopes != "" {
		scopes = strings.Split(config.OAuth2Scopes, " ")
	}
	oauth2Config.Scopes = scopes

	// Use the deployment's HTTP client for the token exchange to
	// respect proxy settings and avoid using http.DefaultClient.
	exchangeCtx := context.WithValue(ctx, oauth2.HTTPClient, api.HTTPClient)
	token, err := oauth2Config.Exchange(exchangeCtx, code)
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
func convertMCPServerConfig(config database.MCPServerConfig) codersdk.MCPServerConfig {
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

		ToolAllowList: coalesceStringSlice(config.ToolAllowList),
		ToolDenyList:  coalesceStringSlice(config.ToolDenyList),

		Availability: config.Availability,

		Enabled:   config.Enabled,
		CreatedAt: config.CreatedAt,
		UpdatedAt: config.UpdatedAt,
	}
}

// convertMCPServerConfigRedacted is the same as convertMCPServerConfig
// but strips admin-only fields (OAuth2 details, API key header) for
// non-admin callers.
func convertMCPServerConfigRedacted(config database.MCPServerConfig) codersdk.MCPServerConfig {
	c := convertMCPServerConfig(config)
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

// discoverAndRegisterMCPOAuth2 discovers the authorization server via
// Protected Resource Metadata (RFC 9728) and Authorization Server
// Metadata (RFC 8414), then registers a client via Dynamic Client
// Registration (RFC 7591).
func discoverAndRegisterMCPOAuth2(ctx context.Context, mcpServerURL, callbackURL string) (*mcpOAuth2Discovery, error) {
	// Per the MCP spec, the authorization base URL is the MCP server
	// URL with the path component discarded (scheme + host only).
	parsed, err := url.Parse(mcpServerURL)
	if err != nil {
		return nil, xerrors.Errorf("parse MCP server URL: %w", err)
	}
	origin := fmt.Sprintf("%s://%s", parsed.Scheme, parsed.Host)

	oauthHandler := transport.NewOAuthHandler(transport.OAuthConfig{
		RedirectURI: callbackURL,
		TokenStore:  transport.NewMemoryTokenStore(),
	})
	oauthHandler.SetBaseURL(origin)

	// Step 1: Discover authorization server metadata (RFC 9728 + RFC 8414).
	metadata, err := oauthHandler.GetServerMetadata(ctx)
	if err != nil {
		return nil, xerrors.Errorf("discover authorization server: %w", err)
	}
	if metadata.AuthorizationEndpoint == "" {
		return nil, xerrors.New("authorization server metadata missing authorization_endpoint")
	}
	if metadata.TokenEndpoint == "" {
		return nil, xerrors.New("authorization server metadata missing token_endpoint")
	}
	if metadata.RegistrationEndpoint == "" {
		return nil, xerrors.New("authorization server does not advertise a registration_endpoint (dynamic client registration may not be supported)")
	}

	// Step 2: Register a client via Dynamic Client Registration
	// (RFC 7591).  We perform this ourselves rather than using
	// mcp-go's RegisterClient because it always registers as a
	// public client (token_endpoint_auth_method="none").  Many
	// authorization servers restrict public clients to loopback
	// redirect URIs per RFC 8252, which rejects the non-loopback
	// callback URL that Coder uses.  Registering as a confidential
	// client lets the server generate a secret and typically
	// relaxes redirect-URI restrictions.
	authMethod := pickTokenEndpointAuthMethod(metadata.TokenEndpointAuthMethodsSupported)

	// If the server only supports public clients and Coder's
	// callback URL is not a loopback address, DCR will fail.
	// Most OAuth2 providers that only support public clients
	// restrict redirect URIs to loopback addresses (RFC 8252
	// section 7.3), which only works for desktop MCP clients
	// like Claude and VS Code that run on the user's machine.
	// Server-side clients like Coder need a non-loopback
	// callback, which requires creating an OAuth2 app in the
	// provider's dashboard and supplying the credentials
	// manually.
	if authMethod == "none" && !callbackURLIsLoopback(callbackURL) {
		return nil, xerrors.Errorf(
			"this MCP server's authorization provider only supports "+
				"automatic setup for desktop MCP clients (like Claude "+
				"or VS Code). To connect from Coder, create an OAuth2 "+
				"application in the provider's dashboard with %s as "+
				"the redirect URI, then add this MCP server with the "+
				"oauth2_client_id, oauth2_auth_url, and oauth2_token_url "+
				"fields set manually (discovered auth_url: %s, "+
				"token_url: %s)",
			callbackURL,
			metadata.AuthorizationEndpoint,
			metadata.TokenEndpoint,
		)
	}
	result, err := registerDynamicOAuth2Client(ctx, metadata.RegistrationEndpoint, callbackURL, authMethod)
	if err != nil {
		return nil, xerrors.Errorf("dynamic client registration: %w", err)
	}

	scopes := strings.Join(metadata.ScopesSupported, " ")

	return &mcpOAuth2Discovery{
		clientID:     result.clientID,
		clientSecret: result.clientSecret,
		authURL:      metadata.AuthorizationEndpoint,
		tokenURL:     metadata.TokenEndpoint,
		scopes:       scopes,
	}, nil
}

// callbackURLIsLoopback reports whether the callback URL points
// at a loopback address (localhost, 127.0.0.1, or [::1]).
func callbackURLIsLoopback(callbackURL string) bool {
	parsed, err := url.Parse(callbackURL)
	if err != nil {
		return false
	}
	h := parsed.Hostname()
	return h == "localhost" || h == "127.0.0.1" || h == "::1"
}

// pickTokenEndpointAuthMethod selects the best auth method for
// Dynamic Client Registration.  It prefers confidential-client
// methods because many authorization servers restrict public
// clients ("none") to loopback-only redirect URIs per RFC 8252.
// If the server does not advertise supported methods, it defaults
// to client_secret_post (a safe confidential-client method).
func pickTokenEndpointAuthMethod(supported []string) string {
	// Prefer confidential methods in order of preference.
	for _, method := range []string{"client_secret_post", "client_secret_basic"} {
		if len(supported) == 0 || slices.Contains(supported, method) {
			return method
		}
	}
	// Fall back to "none" (public client) if the server only
	// supports that.
	if slices.Contains(supported, "none") {
		return "none"
	}
	// Default to client_secret_post when we cannot determine the
	// server's capabilities.
	return "client_secret_post"
}

// dcrClientResponse is the relevant subset of an RFC 7591 Dynamic
// Client Registration response.
type dcrClientResponse struct {
	clientID     string
	clientSecret string
}

// registerDynamicOAuth2Client sends an RFC 7591 Dynamic Client
// Registration request and returns the generated credentials.
func registerDynamicOAuth2Client(ctx context.Context, registrationEndpoint, callbackURL, authMethod string) (*dcrClientResponse, error) {
	regRequest := map[string]any{
		"client_name":                "Coder",
		"redirect_uris":              []string{callbackURL},
		"token_endpoint_auth_method": authMethod,
		"grant_types":                []string{"authorization_code", "refresh_token"},
		"response_types":             []string{"code"},
	}

	reqBody, err := json.Marshal(regRequest)
	if err != nil {
		return nil, xerrors.Errorf("marshal registration request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, registrationEndpoint, bytes.NewReader(reqBody))
	if err != nil {
		return nil, xerrors.Errorf("create registration request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, xerrors.Errorf("send registration request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, xerrors.Errorf("read registration response: %w", err)
	}

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		// Try to parse as an RFC 7591 error.
		var oauthErr struct {
			Error            string `json:"error"`
			ErrorDescription string `json:"error_description"`
		}
		if jsonErr := json.Unmarshal(body, &oauthErr); jsonErr == nil && oauthErr.Error != "" {
			if oauthErr.ErrorDescription != "" {
				return nil, xerrors.Errorf("registration request failed: OAuth error: %s - %s", oauthErr.Error, oauthErr.ErrorDescription)
			}
			return nil, xerrors.Errorf("registration request failed: OAuth error: %s", oauthErr.Error)
		}
		return nil, xerrors.Errorf("registration request failed with status %d: %s", resp.StatusCode, body)
	}

	var regResponse struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret,omitempty"`
	}
	if err := json.Unmarshal(body, &regResponse); err != nil {
		return nil, xerrors.Errorf("decode registration response: %w", err)
	}
	if regResponse.ClientID == "" {
		return nil, xerrors.New("registration response missing client_id")
	}

	return &dcrClientResponse{
		clientID:     regResponse.ClientID,
		clientSecret: regResponse.ClientSecret,
	}, nil
}

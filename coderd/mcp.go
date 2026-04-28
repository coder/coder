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
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
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
				ModelIntent:             req.ModelIntent,
				AllowInPlanMode:         req.AllowInPlanMode,
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
				ModelIntent:             inserted.ModelIntent,
				AllowInPlanMode:         inserted.AllowInPlanMode,
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
		ModelIntent:             req.ModelIntent,
		AllowInPlanMode:         req.AllowInPlanMode,
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

	// Populate AuthConnected for the calling user. Attempt to
	// refresh the token so the status is accurate.
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
		for _, tok := range userTokens {
			if tok.MCPServerConfigID == config.ID {
				sdkConfig.AuthConnected = api.refreshMCPUserToken(ctx, config, tok, apiKey.UserID)
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

		modelIntent := existing.ModelIntent
		if req.ModelIntent != nil {
			modelIntent = *req.ModelIntent
		}

		allowInPlanMode := existing.AllowInPlanMode
		if req.AllowInPlanMode != nil {
			allowInPlanMode = *req.AllowInPlanMode
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
			ModelIntent:             modelIntent,
			AllowInPlanMode:         allowInPlanMode,
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

// parseMCPServerConfigID extracts the MCP server config UUID from the
// "mcpServer" path parameter.
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

		Enabled:         config.Enabled,
		ModelIntent:     config.ModelIntent,
		AllowInPlanMode: config.AllowInPlanMode,
		CreatedAt:       config.CreatedAt,
		UpdatedAt:       config.UpdatedAt,
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

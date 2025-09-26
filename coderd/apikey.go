package coderd

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/moby/moby/pkg/namesgenerator"
	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/coderd/apikey"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/telemetry"
	"github.com/coder/coder/v2/codersdk"
)

// Creates a new token API key with the given scope and lifetime.
//
// @Summary Create token API key
// @ID create-token-api-key
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Users
// @Param user path string true "User ID, name, or me"
// @Param request body codersdk.CreateTokenRequest true "Create token request"
// @Success 201 {object} codersdk.GenerateAPIKeyResponse
// @Router /users/{user}/keys/tokens [post]
func (api *API) postToken(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		user              = httpmw.UserParam(r)
		auditor           = api.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.APIKey](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionCreate,
		})
	)
	aReq.Old = database.APIKey{}
	defer commitAudit()

	var createToken codersdk.CreateTokenRequest
	if !httpapi.Read(ctx, rw, r, &createToken) {
		return
	}

	// TODO(Cian): System users technically just have the 'member' role
	// and we don't want to disallow all members from creating API keys.
	if user.IsSystem {
		api.Logger.Warn(ctx, "disallowed creating api key for system user", slog.F("user_id", user.ID))
		httpapi.Forbidden(rw)
		return
	}

	scopes, err := normalizeTokenScopes(createToken.Scope, createToken.Scopes)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to create API key.",
			Detail:  err.Error(),
		})
		return
	}

	tokenName := namesgenerator.GetRandomName(1)

	if len(createToken.TokenName) != 0 {
		tokenName = createToken.TokenName
	}

	params := apikey.CreateParams{
		UserID:          user.ID,
		LoginType:       database.LoginTypeToken,
		DefaultLifetime: api.DeploymentValues.Sessions.DefaultTokenDuration.Value(),
		Scopes:          scopes,
		TokenName:       tokenName,
	}

	if allowList, provided, err := normalizeTokenAllowList(createToken.AllowList); err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to create API key.",
			Detail:  err.Error(),
		})
		return
	} else if provided {
		params.AllowList = allowList
	}

	if createToken.Lifetime != 0 {
		err := api.validateAPIKeyLifetime(ctx, user.ID, createToken.Lifetime)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Failed to validate create API key request.",
				Detail:  err.Error(),
			})
			return
		}
		params.ExpiresAt = dbtime.Now().Add(createToken.Lifetime)
		params.LifetimeSeconds = int64(createToken.Lifetime.Seconds())
	}

	cookie, key, err := api.createAPIKey(ctx, params)
	if err != nil {
		if database.IsUniqueViolation(err, database.UniqueIndexAPIKeyName) {
			httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
				Message: fmt.Sprintf("A token with name %q already exists.", tokenName),
				Validations: []codersdk.ValidationError{{
					Field:  "name",
					Detail: "This value is already in use and should be unique.",
				}},
			})
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to create API key.",
			Detail:  err.Error(),
		})
		return
	}
	aReq.New = *key
	aReq.SetAdditionalFields(audit.WrapAPIKeyFields(audit.APIKeyFields(ctx, api.Logger, *key)))
	httpapi.Write(ctx, rw, http.StatusCreated, codersdk.GenerateAPIKeyResponse{Key: cookie.Value})
}

// Creates a new session key, used for logging in via the CLI.
//
// @Summary Create new session key
// @ID create-new-session-key
// @Security CoderSessionToken
// @Produce json
// @Tags Users
// @Param user path string true "User ID, name, or me"
// @Success 201 {object} codersdk.GenerateAPIKeyResponse
// @Router /users/{user}/keys [post]
func (api *API) postAPIKey(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		user              = httpmw.UserParam(r)
		auditor           = api.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.APIKey](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionCreate,
		})
	)
	aReq.Old = database.APIKey{}
	defer commitAudit()

	// TODO(Cian): System users technically just have the 'member' role
	// and we don't want to disallow all members from creating API keys.
	if user.IsSystem {
		api.Logger.Warn(ctx, "disallowed creating api key for system user", slog.F("user_id", user.ID))
		httpapi.Forbidden(rw)
		return
	}

	cookie, key, err := api.createAPIKey(ctx, apikey.CreateParams{
		UserID:          user.ID,
		DefaultLifetime: api.DeploymentValues.Sessions.DefaultTokenDuration.Value(),
		LoginType:       database.LoginTypePassword,
		RemoteAddr:      r.RemoteAddr,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to create API key.",
			Detail:  err.Error(),
		})
		return
	}

	aReq.New = *key
	aReq.SetAdditionalFields(audit.WrapAPIKeyFields(audit.APIKeyFields(ctx, api.Logger, *key)))
	// We intentionally do not set the cookie on the response here.
	// Setting the cookie will couple the browser session to the API
	// key we return here, meaning logging out of the website would
	// invalid your CLI key.
	httpapi.Write(ctx, rw, http.StatusCreated, codersdk.GenerateAPIKeyResponse{Key: cookie.Value})
}

// @Summary Get API key by ID
// @ID get-api-key-by-id
// @Security CoderSessionToken
// @Produce json
// @Tags Users
// @Param user path string true "User ID, name, or me"
// @Param keyid path string true "Key ID" format(string)
// @Success 200 {object} codersdk.APIKey
// @Router /users/{user}/keys/{keyid} [get]
func (api *API) apiKeyByID(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	keyID := chi.URLParam(r, "keyid")
	key, err := api.Database.GetAPIKeyByID(ctx, keyID)
	if httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching API key.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, convertAPIKey(key))
}

// @Summary Get API key by token name
// @ID get-api-key-by-token-name
// @Security CoderSessionToken
// @Produce json
// @Tags Users
// @Param user path string true "User ID, name, or me"
// @Param keyname path string true "Key Name" format(string)
// @Success 200 {object} codersdk.APIKey
// @Router /users/{user}/keys/tokens/{keyname} [get]
func (api *API) apiKeyByName(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx       = r.Context()
		user      = httpmw.UserParam(r)
		tokenName = chi.URLParam(r, "keyname")
	)

	token, err := api.Database.GetAPIKeyByName(ctx, database.GetAPIKeyByNameParams{
		TokenName: tokenName,
		UserID:    user.ID,
	})
	if httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching API key.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, convertAPIKey(token))
}

// @Summary Update token API key
// @ID update-token-api-key
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Users
// @Param user path string true "User ID, name, or me"
// @Param keyname path string true "Key Name" format(string)
// @Param request body codersdk.UpdateTokenRequest true "Update token request"
// @Success 200 {object} codersdk.APIKey
// @Router /users/{user}/keys/tokens/{keyname} [patch]
func (api *API) patchToken(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx       = r.Context()
		user      = httpmw.UserParam(r)
		tokenName = chi.URLParam(r, "keyname")
		auditor   = api.Auditor.Load()
	)

	var updateReq codersdk.UpdateTokenRequest
	if !httpapi.Read(ctx, rw, r, &updateReq) {
		return
	}

	if updateReq.Scope != nil && updateReq.Scopes != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to update API key.",
			Detail:  "provide either scope or scopes, not both",
		})
		return
	}

	token, err := api.Database.GetAPIKeyByName(ctx, database.GetAPIKeyByNameParams{
		TokenName: tokenName,
		UserID:    user.ID,
	})
	if httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching API key.",
			Detail:  err.Error(),
		})
		return
	}

	aReq, commitAudit := audit.InitRequest[database.APIKey](rw, &audit.RequestParams{
		Audit:   *auditor,
		Log:     api.Logger,
		Request: r,
		Action:  database.AuditActionWrite,
	})
	aReq.Old = token
	defer commitAudit()

	if !api.Authorize(r, policy.ActionUpdate, token) {
		httpapi.Forbidden(rw)
		return
	}

	updatedScopes := token.Scopes
	if updateReq.Scopes != nil {
		normalized, err := normalizeTokenScopes(codersdk.APIKeyScope(""), *updateReq.Scopes)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Failed to update API key.",
				Detail:  err.Error(),
			})
			return
		}
		updatedScopes = normalized
	} else if updateReq.Scope != nil {
		normalized, err := normalizeTokenScopes(*updateReq.Scope, nil)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Failed to update API key.",
				Detail:  err.Error(),
			})
			return
		}
		updatedScopes = normalized
	}

	updatedAllowList := token.AllowList
	if updateReq.AllowList != nil {
		if len(*updateReq.AllowList) == 0 {
			updatedAllowList = database.AllowList{rbac.AllowListAll()}
		} else {
			normalized, _, err := normalizeTokenAllowList(*updateReq.AllowList)
			if err != nil {
				httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
					Message: "Failed to update API key.",
					Detail:  err.Error(),
				})
				return
			}
			updatedAllowList = normalized
		}
	}

	expiresAt := token.ExpiresAt
	lifetimeSeconds := token.LifetimeSeconds
	if updateReq.Lifetime != nil {
		if err := api.validateAPIKeyLifetime(ctx, user.ID, *updateReq.Lifetime); err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Failed to update API key.",
				Detail:  err.Error(),
			})
			return
		}
		expiresAt = dbtime.Now().Add(*updateReq.Lifetime)
		lifetimeSeconds = int64(updateReq.Lifetime.Seconds())
	}

	updatedToken, err := api.Database.UpdateAPIKeySettings(ctx, database.UpdateAPIKeySettingsParams{
		ID:              token.ID,
		Scopes:          updatedScopes,
		AllowList:       updatedAllowList,
		LifetimeSeconds: lifetimeSeconds,
		ExpiresAt:       expiresAt,
		UpdatedAt:       dbtime.Now(),
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to update API key.",
			Detail:  err.Error(),
		})
		return
	}

	aReq.New = updatedToken
	aReq.SetAdditionalFields(audit.WrapAPIKeyFields(audit.APIKeyFields(ctx, api.Logger, updatedToken)))
	httpapi.Write(ctx, rw, http.StatusOK, convertAPIKey(updatedToken))
}

// @Summary Get user tokens
// @ID get-user-tokens
// @Security CoderSessionToken
// @Produce json
// @Tags Users
// @Param user path string true "User ID, name, or me"
// @Success 200 {array} codersdk.APIKey
// @Router /users/{user}/keys/tokens [get]
func (api *API) tokens(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx           = r.Context()
		user          = httpmw.UserParam(r)
		keys          []database.APIKey
		err           error
		queryStr      = r.URL.Query().Get("include_all")
		includeAll, _ = strconv.ParseBool(queryStr)
	)

	if includeAll {
		// get tokens for all users
		keys, err = api.Database.GetAPIKeysByLoginType(ctx, database.LoginTypeToken)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error fetching API keys.",
				Detail:  err.Error(),
			})
			return
		}
	} else {
		// get user's tokens only
		keys, err = api.Database.GetAPIKeysByUserID(ctx, database.GetAPIKeysByUserIDParams{LoginType: database.LoginTypeToken, UserID: user.ID})
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error fetching API keys.",
				Detail:  err.Error(),
			})
			return
		}
	}

	keys, err = AuthorizeFilter(api.HTTPAuth, r, policy.ActionRead, keys)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching keys.",
			Detail:  err.Error(),
		})
		return
	}

	var userIDs []uuid.UUID
	for _, key := range keys {
		userIDs = append(userIDs, key.UserID)
	}

	users, _ := api.Database.GetUsersByIDs(ctx, userIDs)
	usersByID := map[uuid.UUID]database.User{}
	for _, user := range users {
		usersByID[user.ID] = user
	}

	var apiKeys []codersdk.APIKeyWithOwner
	for _, key := range keys {
		if user, exists := usersByID[key.UserID]; exists {
			apiKeys = append(apiKeys, codersdk.APIKeyWithOwner{
				APIKey:   convertAPIKey(key),
				Username: user.Username,
			})
		} else {
			apiKeys = append(apiKeys, codersdk.APIKeyWithOwner{
				APIKey:   convertAPIKey(key),
				Username: "",
			})
		}
	}

	httpapi.Write(ctx, rw, http.StatusOK, apiKeys)
}

// @Summary Delete API key
// @ID delete-api-key
// @Security CoderSessionToken
// @Tags Users
// @Param user path string true "User ID, name, or me"
// @Param keyid path string true "Key ID" format(string)
// @Success 204
// @Router /users/{user}/keys/{keyid} [delete]
func (api *API) deleteAPIKey(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		keyID             = chi.URLParam(r, "keyid")
		auditor           = api.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.APIKey](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionDelete,
		})
		key, err = api.Database.GetAPIKeyByID(ctx, keyID)
	)
	if err != nil {
		api.Logger.Warn(ctx, "get API Key for audit log")
	}
	aReq.Old = key
	if err == nil {
		aReq.SetAdditionalFields(audit.WrapAPIKeyFields(audit.APIKeyFields(ctx, api.Logger, key)))
	}
	defer commitAudit()

	err = api.Database.DeleteAPIKeyByID(ctx, keyID)
	if httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error deleting API key.",
			Detail:  err.Error(),
		})
		return
	}

	rw.WriteHeader(http.StatusNoContent)
}

// @Summary Get token config
// @ID get-token-config
// @Security CoderSessionToken
// @Produce json
// @Tags General
// @Param user path string true "User ID, name, or me"
// @Success 200 {object} codersdk.TokenConfig
// @Router /users/{user}/keys/tokens/tokenconfig [get]
func (api *API) tokenConfig(rw http.ResponseWriter, r *http.Request) {
	user := httpmw.UserParam(r)
	maxLifetime, err := api.getMaxTokenLifetime(r.Context(), user.ID)
	if err != nil {
		httpapi.Write(r.Context(), rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get token configuration.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(
		r.Context(), rw, http.StatusOK,
		codersdk.TokenConfig{
			MaxTokenLifetime: maxLifetime,
		},
	)
}

func (api *API) validateAPIKeyLifetime(ctx context.Context, userID uuid.UUID, lifetime time.Duration) error {
	if lifetime <= 0 {
		return xerrors.New("lifetime must be positive number greater than 0")
	}

	maxLifetime, err := api.getMaxTokenLifetime(ctx, userID)
	if err != nil {
		return xerrors.Errorf("failed to get max token lifetime: %w", err)
	}

	if lifetime > maxLifetime {
		return xerrors.Errorf(
			"lifetime must be less than %v",
			maxLifetime,
		)
	}

	return nil
}

// getMaxTokenLifetime returns the maximum allowed token lifetime for a user.
// It distinguishes between regular users and owners.
func (api *API) getMaxTokenLifetime(ctx context.Context, userID uuid.UUID) (time.Duration, error) {
	subject, _, err := httpmw.UserRBACSubject(ctx, api.Database, userID, rbac.ScopeAll)
	if err != nil {
		return 0, xerrors.Errorf("failed to get user rbac subject: %w", err)
	}

	roles, err := subject.Roles.Expand()
	if err != nil {
		return 0, xerrors.Errorf("failed to expand user roles: %w", err)
	}

	maxLifetime := api.DeploymentValues.Sessions.MaximumTokenDuration.Value()
	for _, role := range roles {
		if role.Identifier.Name == codersdk.RoleOwner {
			// Owners have a different max lifetime.
			maxLifetime = api.DeploymentValues.Sessions.MaximumAdminTokenDuration.Value()
			break
		}
	}

	return maxLifetime, nil
}

func (api *API) createAPIKey(ctx context.Context, params apikey.CreateParams) (*http.Cookie, *database.APIKey, error) {
	key, sessionToken, err := apikey.Generate(params)
	if err != nil {
		return nil, nil, xerrors.Errorf("generate API key: %w", err)
	}

	newkey, err := api.Database.InsertAPIKey(ctx, key)
	if err != nil {
		return nil, nil, xerrors.Errorf("insert API key: %w", err)
	}

	api.Telemetry.Report(&telemetry.Snapshot{
		APIKeys: []telemetry.APIKey{telemetry.ConvertAPIKey(newkey)},
	})

	return api.DeploymentValues.HTTPCookies.Apply(&http.Cookie{
		Name:     codersdk.SessionTokenCookie,
		Value:    sessionToken,
		Path:     "/",
		HttpOnly: true,
	}), &newkey, nil
}

func normalizeTokenScopes(scope codersdk.APIKeyScope, scopes []codersdk.APIKeyScope) (database.APIKeyScopes, error) {
	// Default to coder:all for backward compatibility when nothing is provided.
	if len(scopes) == 0 && string(scope) == "" {
		return database.APIKeyScopes{database.ApiKeyScopeCoderAll}, nil
	}

	if len(scopes) == 0 {
		name := string(scope)
		if name == "" {
			return database.APIKeyScopes{database.ApiKeyScopeCoderAll}, nil
		}
		if !rbac.IsExternalScope(rbac.ScopeName(name)) {
			return nil, xerrors.Errorf("invalid or unsupported API key scope: %q", name)
		}
		switch name {
		case "all":
			return database.APIKeyScopes{database.ApiKeyScopeCoderAll}, nil
		case "application_connect":
			return database.APIKeyScopes{database.ApiKeyScopeCoderApplicationConnect}, nil
		default:
			return database.APIKeyScopes{database.APIKeyScope(name)}, nil
		}
	}

	out := make(database.APIKeyScopes, 0, len(scopes))
	for _, raw := range scopes {
		name := string(raw)
		if !rbac.IsExternalScope(rbac.ScopeName(name)) {
			return nil, xerrors.Errorf("invalid or unsupported API key scope: %q", name)
		}
		out = append(out, database.APIKeyScope(name))
	}
	return out, nil
}

func normalizeTokenAllowList(entries []codersdk.APIAllowListTarget) (database.AllowList, bool, error) {
	if len(entries) == 0 {
		return nil, false, nil
	}

	rbacAllowList := make([]rbac.AllowListElement, 0, len(entries))
	for _, entry := range entries {
		re, err := rbac.NewAllowListElement(string(entry.Type), entry.ID)
		if err != nil {
			return nil, false, err
		}
		rbacAllowList = append(rbacAllowList, re)
	}

	normalized, err := rbac.NormalizeAllowList(rbacAllowList)
	if err != nil {
		return nil, false, err
	}

	return database.AllowList(normalized), true, nil
}

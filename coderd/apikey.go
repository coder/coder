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

	"github.com/coder/coder/v2/coderd/apikey"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/telemetry"
	"github.com/coder/coder/v2/codersdk"
)

// Creates a new token API key that effectively doesn't expire.
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

	scope := database.APIKeyScopeAll
	if scope != "" {
		scope = database.APIKeyScope(createToken.Scope)
	}

	// default lifetime is 30 days
	lifeTime := 30 * 24 * time.Hour
	if createToken.Lifetime != 0 {
		lifeTime = createToken.Lifetime
	}

	tokenName := namesgenerator.GetRandomName(1)

	if len(createToken.TokenName) != 0 {
		tokenName = createToken.TokenName
	}

	err := api.validateAPIKeyLifetime(lifeTime)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to validate create API key request.",
			Detail:  err.Error(),
		})
		return
	}

	cookie, key, err := api.createAPIKey(ctx, apikey.CreateParams{
		UserID:          user.ID,
		LoginType:       database.LoginTypeToken,
		DefaultLifetime: api.DeploymentValues.SessionDuration.Value(),
		ExpiresAt:       dbtime.Now().Add(lifeTime),
		Scope:           scope,
		LifetimeSeconds: int64(lifeTime.Seconds()),
		TokenName:       tokenName,
	})
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
	ctx := r.Context()
	user := httpmw.UserParam(r)

	lifeTime := time.Hour * 24 * 7
	cookie, _, err := api.createAPIKey(ctx, apikey.CreateParams{
		UserID:          user.ID,
		DefaultLifetime: api.DeploymentValues.SessionDuration.Value(),
		LoginType:       database.LoginTypePassword,
		RemoteAddr:      r.RemoteAddr,
		// All api generated keys will last 1 week. Browser login tokens have
		// a shorter life.
		ExpiresAt:       dbtime.Now().Add(lifeTime),
		LifetimeSeconds: int64(lifeTime.Seconds()),
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to create API key.",
			Detail:  err.Error(),
		})
		return
	}

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
// @Param keyid path string true "Key ID" format(uuid)
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

	keys, err = AuthorizeFilter(api.HTTPAuth, r, rbac.ActionRead, keys)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching keys.",
			Detail:  err.Error(),
		})
		return
	}

	var userIds []uuid.UUID
	for _, key := range keys {
		userIds = append(userIds, key.UserID)
	}

	users, _ := api.Database.GetUsersByIDs(ctx, userIds)
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
// @Param keyid path string true "Key ID" format(uuid)
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

	httpapi.Write(ctx, rw, http.StatusNoContent, nil)
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
	values, err := api.DeploymentValues.WithoutSecrets()
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(
		r.Context(), rw, http.StatusOK,
		codersdk.TokenConfig{
			MaxTokenLifetime: values.MaxTokenLifetime.Value(),
		},
	)
}

func (api *API) validateAPIKeyLifetime(lifetime time.Duration) error {
	if lifetime <= 0 {
		return xerrors.New("lifetime must be positive number greater than 0")
	}

	if lifetime > api.DeploymentValues.MaxTokenLifetime.Value() {
		return xerrors.Errorf(
			"lifetime must be less than %v",
			api.DeploymentValues.MaxTokenLifetime,
		)
	}

	return nil
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

	return &http.Cookie{
		Name:     codersdk.SessionTokenCookie,
		Value:    sessionToken,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   api.SecureAuthCookie,
	}, &newkey, nil
}

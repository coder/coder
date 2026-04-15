package coderd

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary Create a new user secret
// @ID create-a-new-user-secret
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Secrets
// @Param user path string true "User ID, username, or me"
// @Param request body codersdk.CreateUserSecretRequest true "Create secret request"
// @Success 201 {object} codersdk.UserSecret
// @Router /users/{user}/secrets [post]
func (api *API) postUserSecret(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := httpmw.UserParam(r)

	var req codersdk.CreateUserSecretRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	if req.Name == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Name is required.",
		})
		return
	}
	if req.Value == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Value is required.",
		})
		return
	}
	if err := codersdk.UserSecretValueValid(req.Value); err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid secret value.",
			Detail:  err.Error(),
		})
		return
	}
	if err := codersdk.UserSecretEnvNameValid(req.EnvName); err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid environment variable name.",
			Detail:  err.Error(),
		})
		return
	}
	if err := codersdk.UserSecretFilePathValid(req.FilePath); err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid file path.",
			Detail:  err.Error(),
		})
		return
	}

	secret, err := api.Database.CreateUserSecret(ctx, database.CreateUserSecretParams{
		ID:          uuid.New(),
		UserID:      user.ID,
		Name:        req.Name,
		Description: req.Description,
		Value:       req.Value,
		ValueKeyID:  sql.NullString{},
		EnvName:     req.EnvName,
		FilePath:    req.FilePath,
	})
	if err != nil {
		if database.IsUniqueViolation(err) {
			httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
				Message: "A secret with that name, environment variable, or file path already exists.",
				Detail:  err.Error(),
			})
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error creating secret.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusCreated, db2sdk.UserSecretFromFull(secret))
}

// @Summary List user secrets
// @ID list-user-secrets
// @Security CoderSessionToken
// @Produce json
// @Tags Secrets
// @Param user path string true "User ID, username, or me"
// @Success 200 {array} codersdk.UserSecret
// @Router /users/{user}/secrets [get]
func (api *API) getUserSecrets(rw http.ResponseWriter, r *http.Request) { //nolint:revive // Method name matches route.
	ctx := r.Context()
	user := httpmw.UserParam(r)

	secrets, err := api.Database.ListUserSecrets(ctx, user.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error listing secrets.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.UserSecrets(secrets))
}

// @Summary Get a user secret by name
// @ID get-a-user-secret-by-name
// @Security CoderSessionToken
// @Produce json
// @Tags Secrets
// @Param user path string true "User ID, username, or me"
// @Param name path string true "Secret name"
// @Success 200 {object} codersdk.UserSecret
// @Router /users/{user}/secrets/{name} [get]
func (api *API) getUserSecret(rw http.ResponseWriter, r *http.Request) { //nolint:revive // Method name matches route.
	ctx := r.Context()
	user := httpmw.UserParam(r)
	name := chi.URLParam(r, "name")

	secret, err := api.Database.GetUserSecretByUserIDAndName(ctx, database.GetUserSecretByUserIDAndNameParams{
		UserID: user.ID,
		Name:   name,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching secret.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.UserSecretFromFull(secret))
}

// @Summary Update a user secret
// @ID update-a-user-secret
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Secrets
// @Param user path string true "User ID, username, or me"
// @Param name path string true "Secret name"
// @Param request body codersdk.UpdateUserSecretRequest true "Update secret request"
// @Success 200 {object} codersdk.UserSecret
// @Router /users/{user}/secrets/{name} [patch]
func (api *API) patchUserSecret(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := httpmw.UserParam(r)
	name := chi.URLParam(r, "name")

	var req codersdk.UpdateUserSecretRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	if req.Value == nil && req.Description == nil && req.EnvName == nil && req.FilePath == nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "At least one field must be provided.",
		})
		return
	}
	if req.EnvName != nil {
		if err := codersdk.UserSecretEnvNameValid(*req.EnvName); err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Invalid environment variable name.",
				Detail:  err.Error(),
			})
			return
		}
	}
	if req.FilePath != nil {
		if err := codersdk.UserSecretFilePathValid(*req.FilePath); err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Invalid file path.",
				Detail:  err.Error(),
			})
			return
		}
	}

	params := database.UpdateUserSecretByUserIDAndNameParams{
		UserID:            user.ID,
		Name:              name,
		UpdateValue:       req.Value != nil,
		Value:             "",
		ValueKeyID:        sql.NullString{},
		UpdateDescription: req.Description != nil,
		Description:       "",
		UpdateEnvName:     req.EnvName != nil,
		EnvName:           "",
		UpdateFilePath:    req.FilePath != nil,
		FilePath:          "",
	}
	if req.Value != nil {
		if err := codersdk.UserSecretValueValid(*req.Value); err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Invalid secret value.",
				Detail:  err.Error(),
			})
			return
		}
		params.Value = *req.Value
	}
	if req.Description != nil {
		params.Description = *req.Description
	}
	if req.EnvName != nil {
		params.EnvName = *req.EnvName
	}
	if req.FilePath != nil {
		params.FilePath = *req.FilePath
	}

	secret, err := api.Database.UpdateUserSecretByUserIDAndName(ctx, params)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.ResourceNotFound(rw)
			return
		}
		if database.IsUniqueViolation(err) {
			httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
				Message: "Update would conflict with an existing secret.",
				Detail:  err.Error(),
			})
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error updating secret.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.UserSecretFromFull(secret))
}

// @Summary Delete a user secret
// @ID delete-a-user-secret
// @Security CoderSessionToken
// @Tags Secrets
// @Param user path string true "User ID, username, or me"
// @Param name path string true "Secret name"
// @Success 204
// @Router /users/{user}/secrets/{name} [delete]
func (api *API) deleteUserSecret(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := httpmw.UserParam(r)
	name := chi.URLParam(r, "name")

	rowsAffected, err := api.Database.DeleteUserSecretByUserIDAndName(ctx, database.DeleteUserSecretByUserIDAndNameParams{
		UserID: user.ID,
		Name:   name,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error deleting secret.",
			Detail:  err.Error(),
		})
		return
	}
	if rowsAffected == 0 {
		httpapi.ResourceNotFound(rw)
		return
	}

	rw.WriteHeader(http.StatusNoContent)
}

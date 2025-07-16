package coderd

import (
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"net/http"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

// Creates a new user secret.
// Returns a newly created user secret.
//
// @Summary Create user secret
// @ID create-user-secret
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags User-Secrets
// @Param request body codersdk.CreateUserSecretRequest true "Request body"
// @Success 200 {object} codersdk.UserSecret
// @Router /users/secrets [post]
func (api *API) createUserSecret(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)

	var req codersdk.CreateUserSecretRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	secret, err := api.Database.InsertUserSecret(ctx, database.InsertUserSecretParams{
		ID:          uuid.New(),
		UserID:      apiKey.UserID,
		Name:        req.Name,
		Description: req.Description,
		Value:       req.Value,
	})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusCreated, db2sdk.UserSecret(secret))
}

// Returns a list of user secrets.
//
// @Summary Returns a list of user secrets.
// @ID list-user-secrets
// @Security CoderSessionToken
// @Produce json
// @Tags User-Secrets
// @Success 200 {object} codersdk.ListUserSecretsResponse
// @Router /users/secrets [get]
func (api *API) listUserSecrets(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)

	secrets, err := api.Database.ListUserSecrets(ctx, apiKey.UserID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	response := codersdk.ListUserSecretsResponse{
		Secrets: make([]codersdk.UserSecret, len(secrets)),
	}
	for i, secret := range secrets {
		response.Secrets[i] = db2sdk.UserSecret(secret)
	}

	httpapi.Write(ctx, rw, http.StatusOK, response)
}

// Returns a user secret.
//
// @Summary Returns a user secret.
// @ID get-user-secret
// @Security CoderSessionToken
// @Produce json
// @Tags User-Secrets
// @Success 200 {object} codersdk.UserSecret
// @Router /users/secrets/{name} [get]
func (api *API) getUserSecret(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)
	secretName := chi.URLParam(r, "name")

	userSecret, err := api.Database.GetUserSecret(ctx, database.GetUserSecretParams{
		UserID: apiKey.UserID,
		Name:   secretName,
	})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	response := db2sdk.UserSecret(userSecret)

	httpapi.Write(ctx, rw, http.StatusOK, response)
}

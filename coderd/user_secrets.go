package coderd

import (
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/httpmw"
	"net/http"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

func (api *API) createUserSecret(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)

	var req codersdk.CreateUserSecretRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	secret, err := api.Database.InsertUserSecret(ctx, database.InsertUserSecretParams{
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

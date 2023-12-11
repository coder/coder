package coderd

import (
	"crypto/sha256"
	"net/http"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/cryptorand"
)

func convertApp(app database.OAuth2App) codersdk.OAuth2App {
	return codersdk.OAuth2App{
		ID:          app.ID,
		Name:        app.Name,
		CallbackURL: app.CallbackURL,
		Icon:        app.Icon,
	}
}

// @Summary Get OAuth2 applications.
// @ID get-oauth2-applications
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Success 200 {array} codersdk.OAuth2App
// @Router /oauth2/apps [get]
func (api *API) oAuth2Apps(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	dbApps, err := api.Database.GetOAuth2Apps(ctx)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	apps := []codersdk.OAuth2App{}
	for _, app := range dbApps {
		apps = append(apps, convertApp(app))
	}
	httpapi.Write(ctx, rw, http.StatusOK, apps)
}

// @Summary Get OAuth2 application.
// @ID get-oauth2-application
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param app path string true "App ID"
// @Success 200 {object} codersdk.OAuth2App
// @Router /oauth2/apps/{app} [get]
func (*API) oAuth2App(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	app := httpmw.OAuth2App(r)
	httpapi.Write(ctx, rw, http.StatusOK, convertApp(app))
}

// @Summary Create OAuth2 application.
// @ID create-oauth2-application
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Enterprise
// @Param request body codersdk.PostOAuth2AppRequest true "The OAuth2 application to create."
// @Success 200 {object} codersdk.OAuth2App
// @Router /oauth2/apps [post]
func (api *API) postOAuth2App(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req codersdk.PostOAuth2AppRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	app, err := api.Database.InsertOAuth2App(ctx, database.InsertOAuth2AppParams{
		ID:          uuid.New(),
		CreatedAt:   dbtime.Now(),
		UpdatedAt:   dbtime.Now(),
		Name:        req.Name,
		Icon:        req.Icon,
		CallbackURL: req.CallbackURL,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error creating OAuth2 application.",
			Detail:  err.Error(),
		})
		return
	}
	httpapi.Write(ctx, rw, http.StatusCreated, convertApp(app))
}

// @Summary Update OAuth2 application.
// @ID update-oauth2-application
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Enterprise
// @Param app path string true "App ID"
// @Param request body codersdk.PutOAuth2AppRequest true "Update an OAuth2 application."
// @Success 200 {object} codersdk.OAuth2App
// @Router /oauth2/apps/{app} [put]
func (api *API) putOAuth2App(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	app := httpmw.OAuth2App(r)
	var req codersdk.PutOAuth2AppRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	app, err := api.Database.UpdateOAuth2AppByID(ctx, database.UpdateOAuth2AppByIDParams{
		ID:          app.ID,
		UpdatedAt:   dbtime.Now(),
		Name:        req.Name,
		Icon:        req.Icon,
		CallbackURL: req.CallbackURL,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error creating OAuth2 application.",
			Detail:  err.Error(),
		})
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, convertApp(app))
}

// @Summary Delete OAuth2 application.
// @ID delete-oauth2-application
// @Security CoderSessionToken
// @Tags Enterprise
// @Param app path string true "App ID"
// @Success 204
// @Router /oauth2/apps/{app} [delete]
func (api *API) deleteOAuth2App(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	app := httpmw.OAuth2App(r)
	err := api.Database.DeleteOAuth2AppByID(ctx, app.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error deleting OAuth2 application.",
			Detail:  err.Error(),
		})
		return
	}
	httpapi.Write(ctx, rw, http.StatusNoContent, nil)
}

// @Summary Get OAuth2 application secrets.
// @ID get-oauth2-application-secrets
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param app path string true "App ID"
// @Success 200 {array} codersdk.OAuth2AppSecret
// @Router /oauth2/apps/{app}/secrets [get]
func (api *API) oAuth2AppSecrets(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	app := httpmw.OAuth2App(r)
	dbSecrets, err := api.Database.GetOAuth2AppSecretsByAppID(ctx, app.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error getting OAuth2 client secrets.",
			Detail:  err.Error(),
		})
		return
	}
	secrets := []codersdk.OAuth2AppSecret{}
	for _, secret := range dbSecrets {
		secrets = append(secrets, codersdk.OAuth2AppSecret{
			ID:                    secret.ID,
			LastUsedAt:            codersdk.NullTime{NullTime: secret.LastUsedAt},
			ClientSecretTruncated: secret.DisplaySecret,
		})
	}
	httpapi.Write(ctx, rw, http.StatusOK, secrets)
}

// @Summary Create OAuth2 application secret.
// @ID create-oauth2-application-secret
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param app path string true "App ID"
// @Success 200 {array} codersdk.OAuth2AppSecretFull
// @Router /oauth2/apps/{app}/secrets [post]
func (api *API) postOAuth2AppSecret(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	app := httpmw.OAuth2App(r)
	rawSecret, err := cryptorand.String(40)
	if err != nil {
		httpapi.Write(r.Context(), rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to generate OAuth2 client secret.",
		})
		return
	}
	hashed := sha256.Sum256([]byte(rawSecret))
	secret, err := api.Database.InsertOAuth2AppSecret(ctx, database.InsertOAuth2AppSecretParams{
		ID:            uuid.New(),
		CreatedAt:     dbtime.Now(),
		HashedSecret:  hashed[:],
		DisplaySecret: rawSecret[len(rawSecret)-6:],
		AppID:         app.ID,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error creating OAuth2 client secret.",
			Detail:  err.Error(),
		})
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, codersdk.OAuth2AppSecretFull{
		ID:               secret.ID,
		ClientSecretFull: rawSecret,
	})
}

// @Summary Delete OAuth2 application secret.
// @ID delete-oauth2-application-secret
// @Security CoderSessionToken
// @Tags Enterprise
// @Param app path string true "App ID"
// @Param secret path string true "Secret ID"
// @Success 204
// @Router /oauth2/apps/{app}/secrets/{secret} [delete]
func (api *API) deleteOAuth2AppSecret(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	secret := httpmw.OAuth2AppSecret(r)
	err := api.Database.DeleteOAuth2AppSecretByID(ctx, secret.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error deleting OAuth2 client secret.",
			Detail:  err.Error(),
		})
		return
	}
	httpapi.Write(ctx, rw, http.StatusNoContent, nil)
}

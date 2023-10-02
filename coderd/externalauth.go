package coderd

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"golang.org/x/sync/errgroup"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/externalauth"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary Get external auth by ID
// @ID get-external-auth-by-id
// @Security CoderSessionToken
// @Produce json
// @Tags Git
// @Param externalauth path string true "Git Provider ID" format(string)
// @Success 200 {object} codersdk.ExternalAuth
// @Router /externalauth/{externalauth} [get]
func (api *API) externalAuthByID(w http.ResponseWriter, r *http.Request) {
	config := httpmw.ExternalAuthParam(r)
	apiKey := httpmw.APIKey(r)
	ctx := r.Context()

	res := codersdk.ExternalAuth{
		Authenticated:    false,
		Device:           config.DeviceAuth != nil,
		AppInstallURL:    config.AppInstallURL,
		Type:             config.Type.Pretty(),
		AppInstallations: []codersdk.ExternalAuthAppInstallation{},
	}

	link, err := api.Database.GetExternalAuthLink(ctx, database.GetExternalAuthLinkParams{
		ProviderID: config.ID,
		UserID:     apiKey.UserID,
	})
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			httpapi.Write(ctx, w, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to get external auth link.",
				Detail:  err.Error(),
			})
			return
		}

		httpapi.Write(ctx, w, http.StatusOK, res)
		return
	}
	var eg errgroup.Group
	eg.Go(func() (err error) {
		res.Authenticated, res.User, err = config.ValidateToken(ctx, link.OAuthAccessToken)
		return err
	})
	eg.Go(func() (err error) {
		res.AppInstallations, res.AppInstallable, err = config.AppInstallations(ctx, link.OAuthAccessToken)
		return err
	})
	err = eg.Wait()
	if err != nil {
		httpapi.Write(ctx, w, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to validate token.",
			Detail:  err.Error(),
		})
		return
	}
	if res.AppInstallations == nil {
		res.AppInstallations = []codersdk.ExternalAuthAppInstallation{}
	}
	httpapi.Write(ctx, w, http.StatusOK, res)
}

// @Summary Post external auth device by ID
// @ID post-external-auth-device-by-id
// @Security CoderSessionToken
// @Tags Git
// @Param externalauth path string true "External Provider ID" format(string)
// @Success 204
// @Router /externalauth/{externalauth}/device [post]
func (api *API) postExternalAuthDeviceByID(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)
	config := httpmw.ExternalAuthParam(r)

	var req codersdk.ExternalAuthDeviceExchange
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	if config.DeviceAuth == nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Git auth provider does not support device flow.",
		})
		return
	}

	token, err := config.DeviceAuth.ExchangeDeviceCode(ctx, req.DeviceCode)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to exchange device code.",
			Detail:  err.Error(),
		})
		return
	}

	_, err = api.Database.GetExternalAuthLink(ctx, database.GetExternalAuthLinkParams{
		ProviderID: config.ID,
		UserID:     apiKey.UserID,
	})
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Failed to get external auth link.",
				Detail:  err.Error(),
			})
			return
		}

		_, err = api.Database.InsertExternalAuthLink(ctx, database.InsertExternalAuthLinkParams{
			ProviderID:             config.ID,
			UserID:                 apiKey.UserID,
			CreatedAt:              dbtime.Now(),
			UpdatedAt:              dbtime.Now(),
			OAuthAccessToken:       token.AccessToken,
			OAuthAccessTokenKeyID:  sql.NullString{}, // dbcrypt will set as required
			OAuthRefreshToken:      token.RefreshToken,
			OAuthRefreshTokenKeyID: sql.NullString{}, // dbcrypt will set as required
			OAuthExpiry:            token.Expiry,
		})
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Failed to insert external auth link.",
				Detail:  err.Error(),
			})
			return
		}
	} else {
		_, err = api.Database.UpdateExternalAuthLink(ctx, database.UpdateExternalAuthLinkParams{
			ProviderID:             config.ID,
			UserID:                 apiKey.UserID,
			UpdatedAt:              dbtime.Now(),
			OAuthAccessToken:       token.AccessToken,
			OAuthAccessTokenKeyID:  sql.NullString{}, // dbcrypt will update as required
			OAuthRefreshToken:      token.RefreshToken,
			OAuthRefreshTokenKeyID: sql.NullString{}, // dbcrypt will update as required
			OAuthExpiry:            token.Expiry,
		})
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Failed to update external auth link.",
				Detail:  err.Error(),
			})
			return
		}
	}
	httpapi.Write(ctx, rw, http.StatusNoContent, nil)
}

// @Summary Get external auth device by ID.
// @ID get-external-auth-device-by-id
// @Security CoderSessionToken
// @Produce json
// @Tags Git
// @Param externalauth path string true "Git Provider ID" format(string)
// @Success 200 {object} codersdk.ExternalAuthDevice
// @Router /externalauth/{externalauth}/device [get]
func (*API) externalAuthDeviceByID(rw http.ResponseWriter, r *http.Request) {
	config := httpmw.ExternalAuthParam(r)
	ctx := r.Context()

	if config.DeviceAuth == nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Git auth device flow not supported.",
		})
		return
	}

	deviceAuth, err := config.DeviceAuth.AuthorizeDevice(r.Context())
	if err != nil {
		httpapi.Write(r.Context(), rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to authorize device.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, deviceAuth)
}

func (api *API) externalAuthCallback(externalAuthConfig *externalauth.Config) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		var (
			ctx    = r.Context()
			state  = httpmw.OAuth2(r)
			apiKey = httpmw.APIKey(r)
		)

		_, err := api.Database.GetExternalAuthLink(ctx, database.GetExternalAuthLinkParams{
			ProviderID: externalAuthConfig.ID,
			UserID:     apiKey.UserID,
		})
		if err != nil {
			if !errors.Is(err, sql.ErrNoRows) {
				httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
					Message: "Failed to get external auth link.",
					Detail:  err.Error(),
				})
				return
			}

			_, err = api.Database.InsertExternalAuthLink(ctx, database.InsertExternalAuthLinkParams{
				ProviderID:             externalAuthConfig.ID,
				UserID:                 apiKey.UserID,
				CreatedAt:              dbtime.Now(),
				UpdatedAt:              dbtime.Now(),
				OAuthAccessToken:       state.Token.AccessToken,
				OAuthAccessTokenKeyID:  sql.NullString{}, // dbcrypt will set as required
				OAuthRefreshToken:      state.Token.RefreshToken,
				OAuthRefreshTokenKeyID: sql.NullString{}, // dbcrypt will set as required
				OAuthExpiry:            state.Token.Expiry,
			})
			if err != nil {
				httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
					Message: "Failed to insert external auth link.",
					Detail:  err.Error(),
				})
				return
			}
		} else {
			_, err = api.Database.UpdateExternalAuthLink(ctx, database.UpdateExternalAuthLinkParams{
				ProviderID:             externalAuthConfig.ID,
				UserID:                 apiKey.UserID,
				UpdatedAt:              dbtime.Now(),
				OAuthAccessToken:       state.Token.AccessToken,
				OAuthAccessTokenKeyID:  sql.NullString{}, // dbcrypt will update as required
				OAuthRefreshToken:      state.Token.RefreshToken,
				OAuthRefreshTokenKeyID: sql.NullString{}, // dbcrypt will update as required
				OAuthExpiry:            state.Token.Expiry,
			})
			if err != nil {
				httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
					Message: "Failed to update external auth link.",
					Detail:  err.Error(),
				})
				return
			}
		}

		redirect := state.Redirect
		if redirect == "" {
			// This is a nicely rendered screen on the frontend
			redirect = fmt.Sprintf("/externalauth/%s", externalAuthConfig.ID)
		}
		http.Redirect(rw, r, redirect, http.StatusTemporaryRedirect)
	}
}

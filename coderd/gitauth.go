package coderd

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"golang.org/x/sync/errgroup"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/gitauth"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary Get git auth by ID
// @ID get-git-auth-by-id
// @Security CoderSessionToken
// @Produce json
// @Tags Git
// @Param gitauth path string true "Git Provider ID" format(string)
// @Success 200 {object} codersdk.GitAuth
// @Router /gitauth/{gitauth} [get]
func (api *API) gitAuthByID(w http.ResponseWriter, r *http.Request) {
	config := httpmw.GitAuthParam(r)
	apiKey := httpmw.APIKey(r)
	ctx := r.Context()

	res := codersdk.GitAuth{
		Authenticated:    false,
		Device:           config.DeviceAuth != nil,
		AppInstallURL:    config.AppInstallURL,
		Type:             config.Type.Pretty(),
		AppInstallations: []codersdk.GitAuthAppInstallation{},
	}

	link, err := api.Database.GetExternalAuthLink(ctx, database.GetExternalAuthLinkParams{
		ProviderID: config.ID,
		UserID:     apiKey.UserID,
	})
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			httpapi.Write(ctx, w, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to get git auth link.",
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
		res.AppInstallations = []codersdk.GitAuthAppInstallation{}
	}
	httpapi.Write(ctx, w, http.StatusOK, res)
}

// @Summary Post git auth device by ID
// @ID post-git-auth-device-by-id
// @Security CoderSessionToken
// @Tags Git
// @Param gitauth path string true "Git Provider ID" format(string)
// @Success 204
// @Router /gitauth/{gitauth}/device [post]
func (api *API) postGitAuthDeviceByID(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)
	config := httpmw.GitAuthParam(r)

	var req codersdk.GitAuthDeviceExchange
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
				Message: "Failed to get git auth link.",
				Detail:  err.Error(),
			})
			return
		}

		_, err = api.Database.InsertExternalAuthLink(ctx, database.InsertExternalAuthLinkParams{
			ProviderID:        config.ID,
			UserID:            apiKey.UserID,
			CreatedAt:         dbtime.Now(),
			UpdatedAt:         dbtime.Now(),
			OAuthAccessToken:  token.AccessToken,
			OAuthRefreshToken: token.RefreshToken,
			OAuthExpiry:       token.Expiry,
		})
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Failed to insert git auth link.",
				Detail:  err.Error(),
			})
			return
		}
	} else {
		_, err = api.Database.UpdateExternalAuthLink(ctx, database.UpdateExternalAuthLinkParams{
			ProviderID:        config.ID,
			UserID:            apiKey.UserID,
			UpdatedAt:         dbtime.Now(),
			OAuthAccessToken:  token.AccessToken,
			OAuthRefreshToken: token.RefreshToken,
			OAuthExpiry:       token.Expiry,
		})
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Failed to update git auth link.",
				Detail:  err.Error(),
			})
			return
		}
	}
	httpapi.Write(ctx, rw, http.StatusNoContent, nil)
}

// @Summary Get git auth device by ID.
// @ID get-git-auth-device-by-id
// @Security CoderSessionToken
// @Produce json
// @Tags Git
// @Param gitauth path string true "Git Provider ID" format(string)
// @Success 200 {object} codersdk.GitAuthDevice
// @Router /gitauth/{gitauth}/device [get]
func (*API) gitAuthDeviceByID(rw http.ResponseWriter, r *http.Request) {
	config := httpmw.GitAuthParam(r)
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

func (api *API) gitAuthCallback(gitAuthConfig *gitauth.Config) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		var (
			ctx    = r.Context()
			state  = httpmw.OAuth2(r)
			apiKey = httpmw.APIKey(r)
		)

		_, err := api.Database.GetExternalAuthLink(ctx, database.GetExternalAuthLinkParams{
			ProviderID: gitAuthConfig.ID,
			UserID:     apiKey.UserID,
		})
		if err != nil {
			if !errors.Is(err, sql.ErrNoRows) {
				httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
					Message: "Failed to get git auth link.",
					Detail:  err.Error(),
				})
				return
			}

			_, err = api.Database.InsertExternalAuthLink(ctx, database.InsertExternalAuthLinkParams{
				ProviderID:        gitAuthConfig.ID,
				UserID:            apiKey.UserID,
				CreatedAt:         dbtime.Now(),
				UpdatedAt:         dbtime.Now(),
				OAuthAccessToken:  state.Token.AccessToken,
				OAuthRefreshToken: state.Token.RefreshToken,
				OAuthExpiry:       state.Token.Expiry,
			})
			if err != nil {
				httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
					Message: "Failed to insert git auth link.",
					Detail:  err.Error(),
				})
				return
			}
		} else {
			_, err = api.Database.UpdateExternalAuthLink(ctx, database.UpdateExternalAuthLinkParams{
				ProviderID:        gitAuthConfig.ID,
				UserID:            apiKey.UserID,
				UpdatedAt:         dbtime.Now(),
				OAuthAccessToken:  state.Token.AccessToken,
				OAuthRefreshToken: state.Token.RefreshToken,
				OAuthExpiry:       state.Token.Expiry,
			})
			if err != nil {
				httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
					Message: "Failed to update git auth link.",
					Detail:  err.Error(),
				})
				return
			}
		}

		redirect := state.Redirect
		if redirect == "" {
			// This is a nicely rendered screen on the frontend
			redirect = fmt.Sprintf("/gitauth/%s", gitAuthConfig.ID)
		}
		http.Redirect(rw, r, redirect, http.StatusTemporaryRedirect)
	}
}

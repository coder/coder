package coderd

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"golang.org/x/sync/errgroup"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/gitauth"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/codersdk"
)

// gitAuthByID returns the git auth status for the given git auth config ID.
func (api *API) gitAuthByID(w http.ResponseWriter, r *http.Request) {
	config := httpmw.GitAuthParam(r)
	apiKey := httpmw.APIKey(r)
	ctx := r.Context()

	res := codersdk.GitAuth{
		Authenticated: false,
		Device:        config.DeviceAuth != nil,
		AppInstallURL: config.AppInstallURL,
		Type:          config.Type.Pretty(),
	}

	link, err := api.Database.GetGitAuthLink(ctx, database.GetGitAuthLinkParams{
		ProviderID: config.ID,
		UserID:     apiKey.UserID,
	})
	if err == nil {
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
	}

	httpapi.Write(ctx, w, http.StatusOK, res)
}

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

	_, err = api.Database.GetGitAuthLink(ctx, database.GetGitAuthLinkParams{
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

		_, err = api.Database.InsertGitAuthLink(ctx, database.InsertGitAuthLinkParams{
			ProviderID:        config.ID,
			UserID:            apiKey.UserID,
			CreatedAt:         database.Now(),
			UpdatedAt:         database.Now(),
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
		_, err = api.Database.UpdateGitAuthLink(ctx, database.UpdateGitAuthLinkParams{
			ProviderID:        config.ID,
			UserID:            apiKey.UserID,
			UpdatedAt:         database.Now(),
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

// gitAuthDeviceByID issues a new device auth code for the given git auth config ID.
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

		_, err := api.Database.GetGitAuthLink(ctx, database.GetGitAuthLinkParams{
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

			_, err = api.Database.InsertGitAuthLink(ctx, database.InsertGitAuthLinkParams{
				ProviderID:        gitAuthConfig.ID,
				UserID:            apiKey.UserID,
				CreatedAt:         database.Now(),
				UpdatedAt:         database.Now(),
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
			_, err = api.Database.UpdateGitAuthLink(ctx, database.UpdateGitAuthLinkParams{
				ProviderID:        gitAuthConfig.ID,
				UserID:            apiKey.UserID,
				UpdatedAt:         database.Now(),
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

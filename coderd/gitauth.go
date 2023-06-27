package coderd

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/gitauth"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/codersdk"
)

func (*API) gitAuthDeviceRedirect(gitAuthConfig *gitauth.Config) func(http.Handler) http.Handler {
	route := fmt.Sprintf("/gitauth/%s/device", gitAuthConfig.ID)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			deviceCode := r.URL.Query().Get("device_code")
			if r.Method != http.MethodGet || deviceCode != "" {
				next.ServeHTTP(w, r)
				return
			}
			// If no device code is provided, redirect to the dashboard with query params!
			deviceAuth, err := gitAuthConfig.DeviceAuth.AuthorizeDevice(r.Context())
			if err != nil {
				httpapi.Write(r.Context(), w, http.StatusInternalServerError, codersdk.Response{
					Message: "Failed to authorize device.",
					Detail:  err.Error(),
				})
				return
			}
			v := url.Values{
				"device_code":      {deviceAuth.DeviceCode},
				"user_code":        {deviceAuth.UserCode},
				"expires_in":       {fmt.Sprintf("%d", deviceAuth.ExpiresIn)},
				"interval":         {fmt.Sprintf("%d", deviceAuth.Interval)},
				"verification_uri": {deviceAuth.VerificationURI},
			}
			http.Redirect(w, r, fmt.Sprintf("%s?%s", route, v.Encode()), http.StatusTemporaryRedirect)
		})
	}
}

func (api *API) postGitAuthExchange(gitAuthConfig *gitauth.Config) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		apiKey := httpmw.APIKey(r)

		var req codersdk.ExchangeGitAuthRequest
		if !httpapi.Read(ctx, rw, r, &req) {
			return
		}

		if gitAuthConfig.DeviceAuth == nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Git auth provider does not support device flow.",
			})
			return
		}

		token, err := gitAuthConfig.DeviceAuth.ExchangeDeviceCode(ctx, req.DeviceCode)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Failed to exchange device code.",
				Detail:  err.Error(),
			})
			return
		}

		_, err = api.Database.GetGitAuthLink(ctx, database.GetGitAuthLinkParams{
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
				ProviderID:        gitAuthConfig.ID,
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
}

// device get
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
			redirect = "/gitauth"
		}
		http.Redirect(rw, r, redirect, http.StatusTemporaryRedirect)
	}
}

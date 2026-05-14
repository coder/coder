package coderd

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/sqlc-dev/pqtype"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/externalauth"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary Get external auth by ID
// @ID get-external-auth-by-id
// @Security CoderSessionToken
// @Tags Git
// @Produce json
// @Param externalauth path string true "Git Provider ID" format(string)
// @Success 200 {object} codersdk.ExternalAuth
// @Router /api/v2/external-auth/{externalauth} [get]
func (api *API) externalAuthByID(w http.ResponseWriter, r *http.Request) {
	config := httpmw.ExternalAuthParam(r)
	apiKey := httpmw.APIKey(r)
	ctx := r.Context()

	res := codersdk.ExternalAuth{
		Authenticated:    false,
		Device:           config.DeviceAuth != nil,
		AppInstallURL:    config.AppInstallURL,
		DisplayName:      config.DisplayName,
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
		res.Authenticated, res.User, err = config.ValidateToken(ctx, link.OAuthToken())
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
	if res.Authenticated {
		var identity *codersdk.ExternalAuthIdentity
		link, identity, err = api.refreshExternalAuthIdentity(ctx, config, link)
		if err != nil {
			status := http.StatusBadGateway
			message := "Failed to fetch external auth identity."
			if errors.Is(err, errExternalAuthIdentityChanged) {
				status = http.StatusBadRequest
				message = "External auth identity changed. Unlink and reconnect the provider."
			} else if errors.Is(err, errExternalAuthIdentityUpdate) {
				status = http.StatusInternalServerError
				message = "Failed to update external auth identity."
			}
			httpapi.Write(ctx, w, status, codersdk.Response{
				Message: message,
				Detail:  err.Error(),
			})
			return
		}
		res.Identity = identity
	} else {
		res.Identity = db2sdk.ExternalAuthIdentity(link)
	}
	if res.AppInstallations == nil {
		res.AppInstallations = []codersdk.ExternalAuthAppInstallation{}
	}
	httpapi.Write(ctx, w, http.StatusOK, res)
}

// deleteExternalAuthByID only deletes the link on the Coder side, does not revoke the token on the provider side.
//
// @Summary Delete external auth user link by ID
// @ID delete-external-auth-user-link-by-id
// @Security CoderSessionToken
// @Tags Git
// @Produce json
// @Param externalauth path string true "Git Provider ID" format(string)
// @Success 200 {object} codersdk.DeleteExternalAuthByIDResponse
// @Router /api/v2/external-auth/{externalauth} [delete]
func (api *API) deleteExternalAuthByID(w http.ResponseWriter, r *http.Request) {
	config := httpmw.ExternalAuthParam(r)
	apiKey := httpmw.APIKey(r)
	ctx := r.Context()

	link, err := api.Database.GetExternalAuthLink(ctx, database.GetExternalAuthLinkParams{
		ProviderID: config.ID,
		UserID:     apiKey.UserID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.ResourceNotFound(w)
			return
		}
		httpapi.Write(ctx, w, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get external auth link during deletion.",
			Detail:  err.Error(),
		})
		return
	}

	err = api.Database.DeleteExternalAuthLink(ctx, database.DeleteExternalAuthLinkParams{
		ProviderID: config.ID,
		UserID:     apiKey.UserID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.ResourceNotFound(w)
			return
		}
		httpapi.Write(ctx, w, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to delete external auth link.",
			Detail:  err.Error(),
		})
		return
	}

	ok, err := config.RevokeToken(ctx, link)
	resp := codersdk.DeleteExternalAuthByIDResponse{TokenRevoked: ok}

	if err != nil {
		resp.TokenRevocationError = err.Error()
	}
	httpapi.Write(ctx, w, http.StatusOK, resp)
}

// @Summary Post external auth device by ID
// @ID post-external-auth-device-by-id
// @Security CoderSessionToken
// @Tags Git
// @Param externalauth path string true "External Provider ID" format(string)
// @Success 204
// @Router /api/v2/external-auth/{externalauth}/device [post]
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

	existingLink, err := api.Database.GetExternalAuthLink(ctx, database.GetExternalAuthLinkParams{
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
			// No extra data from device auth!
			OAuthExtra:            pqtype.NullRawMessage{},
			ExternalUserID:        "",
			ExternalUserLogin:     "",
			ExternalUserName:      "",
			ExternalUserEmail:     "",
			ExternalUserAvatarUrl: "",
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
			OAuthExtra:             pqtype.NullRawMessage{},
			ExternalUserID:         existingLink.ExternalUserID,
			ExternalUserLogin:      existingLink.ExternalUserLogin,
			ExternalUserName:       existingLink.ExternalUserName,
			ExternalUserEmail:      existingLink.ExternalUserEmail,
			ExternalUserAvatarUrl:  existingLink.ExternalUserAvatarUrl,
		})
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Failed to update external auth link.",
				Detail:  err.Error(),
			})
			return
		}
	}
	rw.WriteHeader(http.StatusNoContent)
}

var (
	errExternalAuthIdentityChanged = xerrors.New("external auth identity changed")
	errExternalAuthIdentityUpdate  = xerrors.New("update external auth identity")
)

// @Summary Get external auth device by ID.
// @ID get-external-auth-device-by-id
// @Security CoderSessionToken
// @Produce json
// @Tags Git
// @Param externalauth path string true "Git Provider ID" format(string)
// @Success 200 {object} codersdk.ExternalAuthDevice
// @Router /api/v2/external-auth/{externalauth}/device [get]
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

func externalAuthIdentityFields(identity *codersdk.ExternalAuthIdentity) (id, login, name, email, avatarURL string) {
	if identity == nil {
		return "", "", "", "", ""
	}
	return identity.ID, identity.Login, identity.Name, identity.Email, identity.AvatarURL
}

func (api *API) refreshExternalAuthIdentity(ctx context.Context, config *externalauth.Config, link database.ExternalAuthLink) (database.ExternalAuthLink, *codersdk.ExternalAuthIdentity, error) {
	identity, err := config.ExternalAuthIdentity(ctx, link.OAuthAccessToken)
	if err != nil {
		return link, nil, xerrors.Errorf("fetch external auth identity: %w", err)
	}
	if identity == nil {
		return link, db2sdk.ExternalAuthIdentity(link), nil
	}
	if link.ExternalUserID != "" && link.ExternalUserID != identity.ID {
		return link, nil, xerrors.Errorf("%w for provider %q", errExternalAuthIdentityChanged, config.ID)
	}
	if link.ExternalUserID == identity.ID &&
		link.ExternalUserLogin == identity.Login &&
		link.ExternalUserName == identity.Name &&
		link.ExternalUserEmail == identity.Email &&
		link.ExternalUserAvatarUrl == identity.AvatarURL {
		return link, identity, nil
	}
	updated, err := api.Database.UpdateExternalAuthLinkIdentity(ctx, database.UpdateExternalAuthLinkIdentityParams{
		ProviderID:            config.ID,
		UserID:                link.UserID,
		UpdatedAt:             dbtime.Now(),
		ExternalUserID:        identity.ID,
		ExternalUserLogin:     identity.Login,
		ExternalUserName:      identity.Name,
		ExternalUserEmail:     identity.Email,
		ExternalUserAvatarUrl: identity.AvatarURL,
	})
	if err != nil {
		return link, nil, xerrors.Errorf("%w: %w", errExternalAuthIdentityUpdate, err)
	}
	return updated, identity, nil
}

func (api *API) externalAuthCallback(externalAuthConfig *externalauth.Config) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		var (
			ctx    = r.Context()
			state  = httpmw.OAuth2(r)
			apiKey = httpmw.APIKey(r)
		)

		extra, err := externalAuthConfig.GenerateTokenExtra(state.Token)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to generate token extra.",
				Detail:  err.Error(),
			})
			return
		}
		identity, err := externalAuthConfig.ExternalAuthIdentity(ctx, state.Token.AccessToken)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to fetch external auth identity.",
				Detail:  err.Error(),
			})
			return
		}
		externalUserID, externalUserLogin, externalUserName, externalUserEmail, externalUserAvatarURL := externalAuthIdentityFields(identity)
		existingLink, err := api.Database.GetExternalAuthLink(ctx, database.GetExternalAuthLinkParams{
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
				OAuthExtra:             extra,
				ExternalUserID:         externalUserID,
				ExternalUserLogin:      externalUserLogin,
				ExternalUserName:       externalUserName,
				ExternalUserEmail:      externalUserEmail,
				ExternalUserAvatarUrl:  externalUserAvatarURL,
			})
			if err != nil {
				httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
					Message: "Failed to insert external auth link.",
					Detail:  err.Error(),
				})
				return
			}
		} else {
			if existingLink.ExternalUserID != "" && existingLink.ExternalUserID != externalUserID {
				httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
					Message: "External auth identity changed. Unlink and reconnect the provider.",
				})
				return
			}
			_, err = api.Database.UpdateExternalAuthLink(ctx, database.UpdateExternalAuthLinkParams{
				ProviderID:             externalAuthConfig.ID,
				UserID:                 apiKey.UserID,
				UpdatedAt:              dbtime.Now(),
				OAuthAccessToken:       state.Token.AccessToken,
				OAuthAccessTokenKeyID:  sql.NullString{}, // dbcrypt will update as required
				OAuthRefreshToken:      state.Token.RefreshToken,
				OAuthRefreshTokenKeyID: sql.NullString{}, // dbcrypt will update as required
				OAuthExpiry:            state.Token.Expiry,
				OAuthExtra:             extra,
				ExternalUserID:         externalUserID,
				ExternalUserLogin:      externalUserLogin,
				ExternalUserName:       externalUserName,
				ExternalUserEmail:      externalUserEmail,
				ExternalUserAvatarUrl:  externalUserAvatarURL,
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
			// This is a nicely rendered screen on the frontend. Passing the query param lets the
			// FE know not to enter the authentication loop again, and instead display an error.
			redirect = fmt.Sprintf("/external-auth/%s?redirected=true", externalAuthConfig.ID)
		}
		redirect = uriFromURL(redirect)
		http.Redirect(rw, r, redirect, http.StatusTemporaryRedirect)
	}
}

// listUserExternalAuths lists all external auths available to a user and
// their auth links if they exist.
//
// @Summary Get user external auths
// @ID get-user-external-auths
// @Security CoderSessionToken
// @Produce json
// @Tags Git
// @Success 200 {object} codersdk.ExternalAuthLink
// @Router /api/v2/external-auth [get]
func (api *API) listUserExternalAuths(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	key := httpmw.APIKey(r)

	links, err := api.Database.GetExternalAuthLinksByUserID(ctx, key.UserID)
	if err != nil {
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching user's external auths.",
			Detail:  err.Error(),
		})
		return
	}

	// This process of authenticating each external link increases the
	// response time. However, it is necessary to more correctly debug
	// authentication issues.
	// We can do this in parallel if we want to speed it up.
	configs := make(map[string]*externalauth.Config)
	for _, cfg := range api.ExternalAuthConfigs {
		configs[cfg.ID] = cfg
	}
	// Check if the links are authenticated.
	linkMeta := make(map[string]db2sdk.ExternalAuthMeta)
	for i, link := range links {
		if link.OAuthAccessToken != "" {
			cfg, ok := configs[link.ProviderID]
			if ok {
				newLink, err := cfg.RefreshToken(ctx, api.Database, link)
				meta := db2sdk.ExternalAuthMeta{
					Authenticated: err == nil,
				}
				if err != nil {
					meta.ValidateError = err.Error()
				}
				linkMeta[link.ProviderID] = meta

				// Update the link if it was potentially refreshed.
				if err == nil {
					links[i] = newLink
				}
			}
		}
	}

	// Note: It would be really nice if we could cfg.Validate() the links and
	// return their authenticated status. To do this, we would also have to
	// refresh expired tokens too. For now, I do not want to cause the excess
	// traffic on this request, so the user will have to do this with a separate
	// call.
	httpapi.Write(ctx, rw, http.StatusOK, codersdk.ListUserExternalAuthResponse{
		Providers: ExternalAuthConfigs(api.ExternalAuthConfigs),
		Links:     db2sdk.ExternalAuths(links, linkMeta),
	})
}

func ExternalAuthConfigs(auths []*externalauth.Config) []codersdk.ExternalAuthLinkProvider {
	out := make([]codersdk.ExternalAuthLinkProvider, 0, len(auths))
	for _, auth := range auths {
		if auth == nil {
			continue
		}
		out = append(out, ExternalAuthConfig(auth))
	}
	return out
}

func ExternalAuthConfig(cfg *externalauth.Config) codersdk.ExternalAuthLinkProvider {
	return codersdk.ExternalAuthLinkProvider{
		ID:                            cfg.ID,
		Type:                          cfg.Type,
		Device:                        cfg.DeviceAuth != nil,
		DisplayName:                   cfg.DisplayName,
		DisplayIcon:                   cfg.DisplayIcon,
		AllowRefresh:                  !cfg.NoRefresh,
		AllowValidate:                 cfg.SupportsValidate(),
		SupportsRevocation:            cfg.RevokeURL != "",
		CodeChallengeMethodsSupported: slice.ToStrings(cfg.CodeChallengeMethodsSupported),
	}
}

func uriFromURL(u string) string {
	uri, err := url.Parse(u)
	if err != nil {
		return "/"
	}

	return uri.RequestURI()
}

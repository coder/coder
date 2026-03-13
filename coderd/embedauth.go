package coderd

import (
	"net/http"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary Bootstrap embed session
// @ID post-chat-embed-session
// @Tags Chats
// @Accept json
// @Produce json
// @Param request body codersdk.EmbedSessionTokenRequest true "Embed session request"
// @Success 204
// @Router /chats/embed-session [post]
// @x-apidocgen {"skip": true}
func (api *API) postChatEmbedSession(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req codersdk.EmbedSessionTokenRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	if req.Token == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Token is required.",
		})
		return
	}

	result, valErr := httpmw.ValidateAPIKey(ctx, httpmw.ValidateAPIKeyConfig{
		DB: api.Database,
		OAuth2Configs: &httpmw.OAuth2Configs{
			Github: api.GithubOAuth2Config,
			OIDC:   api.OIDCConfig,
		},
		DisableSessionExpiryRefresh: api.DeploymentValues.Sessions.DisableExpiryRefresh.Value(),
		SessionTokenFunc: func(*http.Request) string {
			return req.Token
		},
		Logger: api.Logger,
	}, r)
	if valErr != nil {
		httpapi.Write(ctx, rw, valErr.Code, valErr.Response)
		return
	}

	if result.UserStatus != database.UserStatusActive {
		httpapi.Write(ctx, rw, http.StatusUnauthorized, codersdk.Response{
			Message: "User is not active (status = \"" + string(result.UserStatus) + "\"). Contact an admin to reactivate your account.",
		})
		return
	}

	cookie := api.DeploymentValues.HTTPCookies.Apply(&http.Cookie{
		Name:     codersdk.SessionTokenCookie,
		Value:    req.Token,
		Path:     "/",
		HttpOnly: true,
	})
	cookie.HttpOnly = true
	cookie.Path = "/"
	if api.AccessURL.Scheme == "https" {
		// Production HTTPS needs iframe-compatible cookie attributes.
		cookie.Secure = true
		cookie.SameSite = http.SameSiteNoneMode
	} else {
		// Dev HTTP cannot set Secure cookies because browsers reject them over
		// plain HTTP, regardless of deployment-level defaults.
		cookie.Secure = false
		cookie.SameSite = http.SameSiteLaxMode
	}

	http.SetCookie(rw, cookie)
	rw.WriteHeader(http.StatusNoContent)
}

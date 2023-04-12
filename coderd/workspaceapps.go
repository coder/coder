package coderd

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/coderd/workspaceapps"
	"github.com/coder/coder/codersdk"
)

// @Summary Get applications host
// @ID get-applications-host
// @Security CoderSessionToken
// @Produce json
// @Tags Applications
// @Success 200 {object} codersdk.AppHostResponse
// @Router /applications/host [get]
func (api *API) appHost(rw http.ResponseWriter, r *http.Request) {
	host := api.AppHostname
	if host != "" && api.AccessURL.Port() != "" {
		host += fmt.Sprintf(":%s", api.AccessURL.Port())
	}

	httpapi.Write(r.Context(), rw, http.StatusOK, codersdk.AppHostResponse{
		Host: host,
	})
}

// workspaceApplicationAuth is an endpoint on the main router that handles
// redirects from the subdomain handler.
//
// This endpoint is under /api so we don't return the friendly error page here.
// Any errors on this endpoint should be errors that are unlikely to happen
// in production unless the user messes with the URL.
//
// @Summary Redirect to URI with encrypted API key
// @ID redirect-to-uri-with-encrypted-api-key
// @Security CoderSessionToken
// @Tags Applications
// @Param redirect_uri query string false "Redirect destination"
// @Success 307
// @Router /applications/auth-redirect [get]
func (api *API) workspaceApplicationAuth(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)
	if !api.Authorize(r, rbac.ActionCreate, apiKey) {
		httpapi.ResourceNotFound(rw)
		return
	}

	// Get the redirect URI from the query parameters and parse it.
	redirectURI := r.URL.Query().Get(workspaceapps.RedirectURIQueryParam)
	if redirectURI == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Missing redirect_uri query parameter.",
		})
		return
	}
	u, err := url.Parse(redirectURI)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid redirect_uri query parameter.",
			Detail:  err.Error(),
		})
		return
	}
	// Force the redirect URI to use the same scheme as the access URL for
	// security purposes.
	u.Scheme = api.AccessURL.Scheme

	ok := false
	if api.AppHostnameRegex != nil {
		_, ok = httpapi.ExecuteHostnamePattern(api.AppHostnameRegex, u.Host)
	}

	// Ensure that the redirect URI is a subdomain of api.Hostname and is a
	// valid app subdomain.
	if !ok {
		proxy, err := api.Database.GetWorkspaceProxyByHostname(ctx, u.Hostname())
		if xerrors.Is(err, sql.ErrNoRows) {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "The redirect_uri query parameter must be the primary wildcard app hostname, a workspace proxy access URL or a workspace proxy wildcard app hostname.",
			})
			return
		}
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to get workspace proxy by redirect_uri.",
				Detail:  err.Error(),
			})
			return
		}

		proxyURL, err := url.Parse(proxy.Url)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to parse workspace proxy URL.",
				Detail:  xerrors.Errorf("parse proxy URL %q: %w", proxy.Url, err).Error(),
			})
			return
		}

		// Force the redirect URI to use the same scheme as the proxy access URL
		// for security purposes.
		u.Scheme = proxyURL.Scheme
	}

	// Create the application_connect-scoped API key with the same lifetime as
	// the current session.
	exp := apiKey.ExpiresAt
	lifetimeSeconds := apiKey.LifetimeSeconds
	if exp.IsZero() || time.Until(exp) > api.DeploymentValues.SessionDuration.Value() {
		exp = database.Now().Add(api.DeploymentValues.SessionDuration.Value())
		lifetimeSeconds = int64(api.DeploymentValues.SessionDuration.Value().Seconds())
	}
	cookie, _, err := api.createAPIKey(ctx, createAPIKeyParams{
		UserID:          apiKey.UserID,
		LoginType:       database.LoginTypePassword,
		ExpiresAt:       exp,
		LifetimeSeconds: lifetimeSeconds,
		Scope:           database.APIKeyScopeApplicationConnect,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to create API key.",
			Detail:  err.Error(),
		})
		return
	}

	// Encrypt the API key.
	encryptedAPIKey, err := api.AppSecurityKey.EncryptAPIKey(workspaceapps.EncryptedAPIKeyPayload{
		APIKey: cookie.Value,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to encrypt API key.",
			Detail:  err.Error(),
		})
		return
	}

	// Redirect to the redirect URI with the encrypted API key in the query
	// parameters.
	q := u.Query()
	q.Set(workspaceapps.SubdomainProxyAPIKeyParam, encryptedAPIKey)
	u.RawQuery = q.Encode()
	http.Redirect(rw, r, u.String(), http.StatusSeeOther)
}

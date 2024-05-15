package coderd

import (
	"context"
	"database/sql"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/apikey"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/workspaceapps"
	"github.com/coder/coder/v2/coderd/workspaceapps/appurl"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary Get applications host
// @ID get-applications-host
// @Security CoderSessionToken
// @Produce json
// @Tags Applications
// @Success 200 {object} codersdk.AppHostResponse
// @Router /applications/host [get]
// @Deprecated use api/v2/regions and see the primary proxy.
func (api *API) appHost(rw http.ResponseWriter, r *http.Request) {
	httpapi.Write(r.Context(), rw, http.StatusOK, codersdk.AppHostResponse{
		Host: appurl.SubdomainAppHost(api.AppHostname, api.AccessURL),
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
	if !api.Authorize(r, policy.ActionCreate, apiKey) {
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

	u.Scheme, err = api.ValidWorkspaceAppHostname(ctx, u.Host, ValidWorkspaceAppHostnameOpts{
		// Allow all hosts except primary access URL since we don't need app
		// tokens on the primary dashboard URL.
		AllowPrimaryAccessURL: false,
		AllowPrimaryWildcard:  true,
		AllowProxyAccessURL:   true,
		AllowProxyWildcard:    true,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to verify redirect_uri query parameter.",
			Detail:  err.Error(),
		})
		return
	}
	if u.Scheme == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid redirect_uri.",
			Detail:  "The redirect_uri query parameter must be the primary wildcard app hostname, a workspace proxy access URL or a workspace proxy wildcard app hostname.",
		})
		return
	}

	// Create the application_connect-scoped API key with the same lifetime as
	// the current session.
	exp := apiKey.ExpiresAt
	lifetimeSeconds := apiKey.LifetimeSeconds
	if exp.IsZero() || time.Until(exp) > api.DeploymentValues.Sessions.DefaultDuration.Value() {
		exp = dbtime.Now().Add(api.DeploymentValues.Sessions.DefaultDuration.Value())
		lifetimeSeconds = int64(api.DeploymentValues.Sessions.DefaultDuration.Value().Seconds())
	}
	cookie, _, err := api.createAPIKey(ctx, apikey.CreateParams{
		UserID:          apiKey.UserID,
		LoginType:       database.LoginTypePassword,
		DefaultLifetime: api.DeploymentValues.Sessions.DefaultDuration.Value(),
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

type ValidWorkspaceAppHostnameOpts struct {
	AllowPrimaryAccessURL bool
	AllowPrimaryWildcard  bool
	AllowProxyAccessURL   bool
	AllowProxyWildcard    bool
}

// ValidWorkspaceAppHostname checks if the given host is a valid workspace app
// hostname based on the provided options. It returns a scheme to force on
// success. If the hostname is not valid or doesn't match, an empty string is
// returned. Any error returned is a 500 error.
//
// For hosts that match a wildcard app hostname, the scheme is forced to be the
// corresponding access URL scheme.
func (api *API) ValidWorkspaceAppHostname(ctx context.Context, host string, opts ValidWorkspaceAppHostnameOpts) (string, error) {
	if opts.AllowPrimaryAccessURL && (host == api.AccessURL.Hostname() || host == api.AccessURL.Host) {
		// Force the redirect URI to have the same scheme as the access URL for
		// security purposes.
		return api.AccessURL.Scheme, nil
	}

	if opts.AllowPrimaryWildcard && api.AppHostnameRegex != nil {
		_, ok := appurl.ExecuteHostnamePattern(api.AppHostnameRegex, host)
		if ok {
			// Force the redirect URI to have the same scheme as the access URL
			// for security purposes.
			return api.AccessURL.Scheme, nil
		}
	}

	// Ensure that the redirect URI is a subdomain of api.Hostname and is a
	// valid app subdomain.
	if opts.AllowProxyAccessURL || opts.AllowProxyWildcard {
		// Strip the port for the database query.
		host = strings.Split(host, ":")[0]

		// nolint:gocritic // system query
		systemCtx := dbauthz.AsSystemRestricted(ctx)
		proxy, err := api.Database.GetWorkspaceProxyByHostname(systemCtx, database.GetWorkspaceProxyByHostnameParams{
			Hostname:              host,
			AllowAccessUrl:        opts.AllowProxyAccessURL,
			AllowWildcardHostname: opts.AllowProxyWildcard,
		})
		if xerrors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		if err != nil {
			return "", xerrors.Errorf("get workspace proxy by hostname %q: %w", host, err)
		}

		proxyURL, err := url.Parse(proxy.Url)
		if err != nil {
			return "", xerrors.Errorf("parse proxy URL %q: %w", proxy.Url, err)
		}

		// Force the redirect URI to use the same scheme as the proxy access URL
		// for security purposes.
		return proxyURL.Scheme, nil
	}

	return "", nil
}

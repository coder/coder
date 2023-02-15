package coderd

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"
	jose "gopkg.in/square/go-jose.v2"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbauthz"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/coderd/tracing"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/site"
)

const (
	// This needs to be a super unique query parameter because we don't want to
	// conflict with query parameters that users may use.
	//nolint:gosec
	subdomainProxyAPIKeyParam = "coder_application_connect_api_key_35e783"
	// redirectURIQueryParam is the query param for the app URL to be passed
	// back to the API auth endpoint on the main access URL.
	redirectURIQueryParam = "redirect_uri"
	// appLogoutHostname is the hostname to use for the logout redirect. When
	// the dashboard logs out, it will redirect to this subdomain of the app
	// hostname, and the server will remove the cookie and redirect to the main
	// login page.
	// It is important that this URL can never match a valid app hostname.
	appLogoutHostname = "coder-logout"
)

// nonCanonicalHeaders is a map from "canonical" headers to the actual header we
// should send to the app in the workspace. Some headers (such as the websocket
// upgrade headers from RFC 6455) are not canonical according to the HTTP/1
// spec. Golang has said that they will not add custom cases for these headers,
// so we need to do it ourselves.
//
// Some apps our customers use are sensitive to the case of these headers.
//
// https://github.com/golang/go/issues/18495
var nonCanonicalHeaders = map[string]string{
	"Sec-Websocket-Accept":     "Sec-WebSocket-Accept",
	"Sec-Websocket-Extensions": "Sec-WebSocket-Extensions",
	"Sec-Websocket-Key":        "Sec-WebSocket-Key",
	"Sec-Websocket-Protocol":   "Sec-WebSocket-Protocol",
	"Sec-Websocket-Version":    "Sec-WebSocket-Version",
}

type workspaceAppAccessMethod string

const (
	workspaceAppAccessMethodPath      workspaceAppAccessMethod = "path"
	workspaceAppAccessMethodSubdomain workspaceAppAccessMethod = "subdomain"
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

// workspaceAppsProxyPath proxies requests to a workspace application
// through a relative URL path.
func (api *API) workspaceAppsProxyPath(rw http.ResponseWriter, r *http.Request) {
	workspace := httpmw.WorkspaceParam(r)
	agent := httpmw.WorkspaceAgentParam(r)

	if api.DeploymentConfig.DisablePathApps.Value {
		site.RenderStaticErrorPage(rw, r, site.ErrorPageData{
			Status:       http.StatusUnauthorized,
			Title:        "Unauthorized",
			Description:  "Path-based applications are disabled on this Coder deployment by the administrator.",
			RetryEnabled: false,
			DashboardURL: api.AccessURL.String(),
		})
		return
	}

	// We do not support port proxying on paths, so lookup the app by slug.
	appSlug := chi.URLParam(r, "workspaceapp")
	app, ok := api.lookupWorkspaceApp(rw, r, agent.ID, appSlug)
	if !ok {
		return
	}

	appSharingLevel := database.AppSharingLevelOwner
	if app.SharingLevel != "" {
		appSharingLevel = app.SharingLevel
	}
	authed, ok := api.fetchWorkspaceApplicationAuth(rw, r, workspaceAppAccessMethodPath, workspace, appSharingLevel)
	if !ok {
		return
	}
	if !authed {
		_, hasAPIKey := httpmw.APIKeyOptional(r)
		if hasAPIKey {
			// The request has a valid API key but insufficient permissions.
			renderApplicationNotFound(rw, r, api.AccessURL)
			return
		}

		// Redirect to login as they don't have permission to access the app and
		// they aren't signed in.
		httpmw.RedirectToLogin(rw, r, httpmw.SignedOutErrorMessage)
		return
	}

	// Determine the real path that was hit. The * URL parameter in Chi will not
	// include the leading slash if it was present, so we need to add it back.
	chiPath := chi.URLParam(r, "*")
	basePath := strings.TrimSuffix(r.URL.Path, chiPath)
	if strings.HasSuffix(basePath, "/") {
		chiPath = "/" + chiPath
	}

	api.proxyWorkspaceApplication(proxyApplication{
		AccessMethod: workspaceAppAccessMethodPath,
		Workspace:    workspace,
		Agent:        agent,
		App:          &app,
		Port:         0,
		Path:         chiPath,
	}, rw, r)
}

// handleSubdomainApplications handles subdomain-based application proxy
// requests (aka. DevURLs in Coder V1).
//
// There are a lot of paths here:
//  1. If api.AppHostname is not set then we pass on.
//  2. If we can't read the request hostname then we return a 400.
//  3. If the request hostname matches api.AccessURL then we pass on.
//  5. We split the subdomain into the subdomain and the "rest". If there are no
//     periods in the hostname then we pass on.
//  5. We parse the subdomain into a httpapi.ApplicationURL struct. If we
//     encounter an error:
//     a. If the "rest" does not match api.AppHostname then we pass on;
//     b. Otherwise, we return a 400.
//  6. Finally, we verify that the "rest" matches api.AppHostname, else we
//     return a 404.
//
// Rationales for each of the above steps:
//  1. We pass on if api.AppHostname is not set to avoid returning any errors if
//     `--app-hostname` is not configured.
//  2. Every request should have a valid Host header anyways.
//  3. We pass on if the request hostname matches api.AccessURL so we can
//     support having the access URL be at the same level as the application
//     base hostname.
//  4. We pass on if there are no periods in the hostname as application URLs
//     must be a subdomain of a hostname, which implies there must be at least
//     one period.
//  5. a. If the request subdomain is not a valid application URL, and the
//     "rest" does not match api.AppHostname, then it is very unlikely that
//     the request was intended for this handler. We pass on.
//     b. If the request subdomain is not a valid application URL, but the
//     "rest" matches api.AppHostname, then we return a 400 because the
//     request is probably a typo or something.
//  6. We finally verify that the "rest" matches api.AppHostname for security
//     purposes regarding re-authentication and application proxy session
//     tokens.
func (api *API) handleSubdomainApplications(middlewares ...func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			// Step 1: Pass on if subdomain-based application proxying is not
			// configured.
			if api.AppHostname == "" || api.AppHostnameRegex == nil {
				next.ServeHTTP(rw, r)
				return
			}

			// Step 2: Get the request Host.
			host := httpapi.RequestHost(r)
			if host == "" {
				if r.URL.Path == "/derp" {
					// The /derp endpoint is used by wireguard clients to tunnel
					// through coderd. For some reason these requests don't set
					// a Host header properly sometimes in tests (no idea how),
					// which causes this path to get hit.
					next.ServeHTTP(rw, r)
					return
				}

				httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
					Message: "Could not determine request Host.",
				})
				return
			}

			// Steps 3-6: Parse application from subdomain.
			app, ok := api.parseWorkspaceApplicationHostname(rw, r, next, host)
			if !ok {
				return
			}

			workspaceAgentKey := fmt.Sprintf("%s.%s", app.WorkspaceName, app.AgentName)
			chiCtx := chi.RouteContext(ctx)
			chiCtx.URLParams.Add("workspace_and_agent", workspaceAgentKey)
			chiCtx.URLParams.Add("user", app.Username)

			// Use the passed in app middlewares before passing to the proxy app.
			mws := chi.Middlewares(middlewares)
			mws.Handler(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
				workspace := httpmw.WorkspaceParam(r)
				agent := httpmw.WorkspaceAgentParam(r)

				var workspaceAppPtr *database.WorkspaceApp
				if app.AppSlug != "" {
					workspaceApp, ok := api.lookupWorkspaceApp(rw, r, agent.ID, app.AppSlug)
					if !ok {
						return
					}

					workspaceAppPtr = &workspaceApp
				}

				// Verify application auth. This function will redirect or
				// return an error page if the user doesn't have permission.
				sharingLevel := database.AppSharingLevelOwner
				if workspaceAppPtr != nil && workspaceAppPtr.SharingLevel != "" {
					sharingLevel = workspaceAppPtr.SharingLevel
				}
				if !api.verifyWorkspaceApplicationSubdomainAuth(rw, r, host, workspace, sharingLevel) {
					return
				}

				api.proxyWorkspaceApplication(proxyApplication{
					AccessMethod: workspaceAppAccessMethodSubdomain,
					Workspace:    workspace,
					Agent:        agent,
					App:          workspaceAppPtr,
					Port:         app.Port,
					Path:         r.URL.Path,
				}, rw, r)
			})).ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}

func (api *API) parseWorkspaceApplicationHostname(rw http.ResponseWriter, r *http.Request, next http.Handler, host string) (httpapi.ApplicationURL, bool) {
	// Check if the hostname matches the access URL. If it does, the user was
	// definitely trying to connect to the dashboard/API.
	if httpapi.HostnamesMatch(api.AccessURL.Hostname(), host) {
		next.ServeHTTP(rw, r)
		return httpapi.ApplicationURL{}, false
	}

	// If there are no periods in the hostname, then it can't be a valid
	// application URL.
	if !strings.Contains(host, ".") {
		next.ServeHTTP(rw, r)
		return httpapi.ApplicationURL{}, false
	}

	// Split the subdomain so we can parse the application details and verify it
	// matches the configured app hostname later.
	subdomain, ok := httpapi.ExecuteHostnamePattern(api.AppHostnameRegex, host)
	if !ok {
		// Doesn't match the regex, so it's not a valid application URL.
		next.ServeHTTP(rw, r)
		return httpapi.ApplicationURL{}, false
	}

	// Check if the request is part of a logout flow.
	if subdomain == appLogoutHostname {
		api.handleWorkspaceAppLogout(rw, r)
		return httpapi.ApplicationURL{}, false
	}

	// Parse the application URL from the subdomain.
	app, err := httpapi.ParseSubdomainAppURL(subdomain)
	if err != nil {
		site.RenderStaticErrorPage(rw, r, site.ErrorPageData{
			Status:       http.StatusBadRequest,
			Title:        "Invalid application URL",
			Description:  fmt.Sprintf("Could not parse subdomain application URL %q: %s", subdomain, err.Error()),
			RetryEnabled: false,
			DashboardURL: api.AccessURL.String(),
		})
		return httpapi.ApplicationURL{}, false
	}

	return app, true
}

func (api *API) handleWorkspaceAppLogout(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Delete the API key and cookie first before attempting to parse/validate
	// the redirect URI.
	cookie, err := r.Cookie(httpmw.DevURLSessionTokenCookie)
	if err == nil && cookie.Value != "" {
		id, secret, err := httpmw.SplitAPIToken(cookie.Value)
		// If it's not a valid token then we don't need to delete it from the
		// database, but we'll still delete the cookie.
		if err == nil {
			// To avoid a situation where someone overloads the API with
			// different auth formats, and tricks this endpoint into deleting an
			// unchecked API key, we validate that the secret matches the secret
			// we store in the database.
			//nolint:gocritic // needed for workspace app logout
			apiKey, err := api.Database.GetAPIKeyByID(dbauthz.AsSystemRestricted(ctx), id)
			if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
				httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
					Message: "Failed to lookup API key.",
					Detail:  err.Error(),
				})
				return
			}
			// This is wrapped in `err == nil` because if the API key doesn't
			// exist, we still want to delete the cookie.
			if err == nil {
				hashedSecret := sha256.Sum256([]byte(secret))
				if subtle.ConstantTimeCompare(apiKey.HashedSecret, hashedSecret[:]) != 1 {
					httpapi.Write(ctx, rw, http.StatusUnauthorized, codersdk.Response{
						Message: httpmw.SignedOutErrorMessage,
						Detail:  "API key secret is invalid.",
					})
					return
				}
				//nolint:gocritic // needed for workspace app logout
				err = api.Database.DeleteAPIKeyByID(dbauthz.AsSystemRestricted(ctx), id)
				if err != nil {
					httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
						Message: "Failed to delete API key.",
						Detail:  err.Error(),
					})
					return
				}
			}
		}
	}
	if !api.setWorkspaceAppCookie(rw, r, "") {
		return
	}

	// Read the redirect URI from the query string.
	redirectURI := r.URL.Query().Get(redirectURIQueryParam)
	if redirectURI == "" {
		redirectURI = api.AccessURL.String()
	} else {
		// Validate that the redirect URI is a valid URL and exists on the same
		// host as the access URL or an app URL.
		parsedRedirectURI, err := url.Parse(redirectURI)
		if err != nil {
			site.RenderStaticErrorPage(rw, r, site.ErrorPageData{
				Status:       http.StatusBadRequest,
				Title:        "Invalid redirect URI",
				Description:  fmt.Sprintf("Could not parse redirect URI %q: %s", redirectURI, err.Error()),
				RetryEnabled: false,
				DashboardURL: api.AccessURL.String(),
			})
			return
		}

		// Check if the redirect URI is on the same host as the access URL or an
		// app URL.
		ok := httpapi.HostnamesMatch(api.AccessURL.Hostname(), parsedRedirectURI.Hostname())
		if !ok && api.AppHostnameRegex != nil {
			// We could also check that it's a valid application URL for
			// completeness, but this check should be good enough.
			_, ok = httpapi.ExecuteHostnamePattern(api.AppHostnameRegex, parsedRedirectURI.Hostname())
		}
		if !ok {
			// The redirect URI they provided is not allowed, but we don't want
			// to return an error page because it'll interrupt the logout flow,
			// so we just use the default access URL.
			parsedRedirectURI = api.AccessURL
		}

		redirectURI = parsedRedirectURI.String()
	}

	http.Redirect(rw, r, redirectURI, http.StatusTemporaryRedirect)
}

// lookupWorkspaceApp looks up the workspace application by slug in the given
// agent and returns it. If the application is not found or there was a server
// error while looking it up, an HTML error page is returned and false is
// returned so the caller can return early.
func (api *API) lookupWorkspaceApp(rw http.ResponseWriter, r *http.Request, agentID uuid.UUID, appSlug string) (database.WorkspaceApp, bool) {
	// dbauthz.AsSystemRestricted is allowed here as the app authz is checked later.
	// The app authz is determined by the sharing level.
	//nolint:gocritic
	app, err := api.Database.GetWorkspaceAppByAgentIDAndSlug(dbauthz.AsSystemRestricted(r.Context()), database.GetWorkspaceAppByAgentIDAndSlugParams{
		AgentID: agentID,
		Slug:    appSlug,
	})
	if xerrors.Is(err, sql.ErrNoRows) {
		renderApplicationNotFound(rw, r, api.AccessURL)
		return database.WorkspaceApp{}, false
	}
	if err != nil {
		site.RenderStaticErrorPage(rw, r, site.ErrorPageData{
			Status:       http.StatusInternalServerError,
			Title:        "Internal Server Error",
			Description:  "Could not fetch workspace application: " + err.Error(),
			RetryEnabled: true,
			DashboardURL: api.AccessURL.String(),
		})
		return database.WorkspaceApp{}, false
	}

	return app, true
}

//nolint:revive
func (api *API) authorizeWorkspaceApp(r *http.Request, accessMethod workspaceAppAccessMethod, sharingLevel database.AppSharingLevel, workspace database.Workspace) (bool, error) {
	ctx := r.Context()

	if accessMethod == "" {
		accessMethod = workspaceAppAccessMethodPath
	}
	isPathApp := accessMethod == workspaceAppAccessMethodPath

	// If path-based app sharing is disabled (which is the default), we can
	// force the sharing level to be "owner" so that the user can only access
	// their own apps.
	//
	// Site owners are blocked from accessing path-based apps unless the
	// Dangerous.AllowPathAppSiteOwnerAccess flag is enabled in the check below.
	if isPathApp && !api.DeploymentConfig.Dangerous.AllowPathAppSharing.Value {
		sharingLevel = database.AppSharingLevelOwner
	}

	// Short circuit if not authenticated.
	roles, ok := httpmw.UserAuthorizationOptional(r)
	if !ok {
		// The user is not authenticated, so they can only access the app if it
		// is public.
		return sharingLevel == database.AppSharingLevelPublic, nil
	}

	// Block anyone from accessing workspaces they don't own in path-based apps
	// unless the admin disables this security feature. This blocks site-owners
	// from accessing any apps from any user's workspaces.
	//
	// When the Dangerous.AllowPathAppSharing flag is not enabled, the sharing
	// level will be forced to "owner", so this check will always be true for
	// workspaces owned by different users.
	if isPathApp &&
		sharingLevel == database.AppSharingLevelOwner &&
		workspace.OwnerID.String() != roles.Actor.ID &&
		!api.DeploymentConfig.Dangerous.AllowPathAppSiteOwnerAccess.Value {

		return false, nil
	}

	// Do a standard RBAC check. This accounts for share level "owner" and any
	// other RBAC rules that may be in place.
	//
	// Regardless of share level or whether it's enabled or not, the owner of
	// the workspace can always access applications (as long as their API key's
	// scope allows it).
	err := api.Authorizer.Authorize(ctx, roles.Actor, rbac.ActionCreate, workspace.ApplicationConnectRBAC())
	if err == nil {
		return true, nil
	}

	switch sharingLevel {
	case database.AppSharingLevelOwner:
		// We essentially already did this above with the regular RBAC check.
		// Owners can always access their own apps according to RBAC rules, so
		// they have already been returned from this function.
	case database.AppSharingLevelAuthenticated:
		// The user is authenticated at this point, but we need to make sure
		// that they have ApplicationConnect permissions to their own
		// workspaces. This ensures that the key's scope has permission to
		// connect to workspace apps.
		object := rbac.ResourceWorkspaceApplicationConnect.WithOwner(roles.Actor.ID)
		err := api.Authorizer.Authorize(ctx, roles.Actor, rbac.ActionCreate, object)
		if err == nil {
			return true, nil
		}
	case database.AppSharingLevelPublic:
		// We don't really care about scopes and stuff if it's public anyways.
		// Someone with a restricted-scope API key could just not submit the
		// API key cookie in the request and access the page.
		return true, nil
	}

	// No checks were successful.
	return false, nil
}

// fetchWorkspaceApplicationAuth authorizes the user using api.AppAuthorizer
// for a given app share level in the given workspace. The user's authorization
// status is returned. If a server error occurs, a HTML error page is rendered
// and false is returned so the caller can return early.
func (api *API) fetchWorkspaceApplicationAuth(rw http.ResponseWriter, r *http.Request, accessMethod workspaceAppAccessMethod, workspace database.Workspace, appSharingLevel database.AppSharingLevel) (authed bool, ok bool) {
	ok, err := api.authorizeWorkspaceApp(r, accessMethod, appSharingLevel, workspace)
	if err != nil {
		api.Logger.Error(r.Context(), "authorize workspace app", slog.Error(err))
		site.RenderStaticErrorPage(rw, r, site.ErrorPageData{
			Status:       http.StatusInternalServerError,
			Title:        "Internal Server Error",
			Description:  "Could not verify authorization. Please try again or contact an administrator.",
			RetryEnabled: true,
			DashboardURL: api.AccessURL.String(),
		})
		return false, false
	}

	return ok, true
}

// checkWorkspaceApplicationAuth authorizes the user using api.AppAuthorizer
// for a given app share level in the given workspace. If the user is not
// authorized or a server error occurs, a discrete HTML error page is rendered
// and false is returned so the caller can return early.
func (api *API) checkWorkspaceApplicationAuth(rw http.ResponseWriter, r *http.Request, accessMethod workspaceAppAccessMethod, workspace database.Workspace, appSharingLevel database.AppSharingLevel) bool {
	authed, ok := api.fetchWorkspaceApplicationAuth(rw, r, accessMethod, workspace, appSharingLevel)
	if !ok {
		return false
	}
	if !authed {
		renderApplicationNotFound(rw, r, api.AccessURL)
		return false
	}

	return true
}

// verifyWorkspaceApplicationSubdomainAuth checks that the request is authorized
// to access the given application. If the user does not have a app session key,
// they will be redirected to the route below. If the user does have a session
// key but insufficient permissions a static error page will be rendered.
func (api *API) verifyWorkspaceApplicationSubdomainAuth(rw http.ResponseWriter, r *http.Request, host string, workspace database.Workspace, appSharingLevel database.AppSharingLevel) bool {
	authed, ok := api.fetchWorkspaceApplicationAuth(rw, r, workspaceAppAccessMethodSubdomain, workspace, appSharingLevel)
	if !ok {
		return false
	}
	if authed {
		return true
	}

	_, hasAPIKey := httpmw.APIKeyOptional(r)
	if hasAPIKey {
		// The request has a valid API key but insufficient permissions.
		renderApplicationNotFound(rw, r, api.AccessURL)
		return false
	}

	// If the request has the special query param then we need to set a cookie
	// and strip that query parameter.
	if encryptedAPIKey := r.URL.Query().Get(subdomainProxyAPIKeyParam); encryptedAPIKey != "" {
		// Exchange the encoded API key for a real one.
		_, token, err := decryptAPIKey(r.Context(), api.Database, encryptedAPIKey)
		if err != nil {
			site.RenderStaticErrorPage(rw, r, site.ErrorPageData{
				Status:      http.StatusBadRequest,
				Title:       "Bad Request",
				Description: "Could not decrypt API key. Please remove the query parameter and try again.",
				// Retry is disabled because the user needs to remove the query
				// parameter before they try again.
				RetryEnabled: false,
				DashboardURL: api.AccessURL.String(),
			})
			return false
		}

		api.setWorkspaceAppCookie(rw, r, token)

		// Strip the query parameter.
		path := r.URL.Path
		if path == "" {
			path = "/"
		}
		q := r.URL.Query()
		q.Del(subdomainProxyAPIKeyParam)
		rawQuery := q.Encode()
		if rawQuery != "" {
			path += "?" + q.Encode()
		}

		http.Redirect(rw, r, path, http.StatusTemporaryRedirect)
		return false
	}

	// If the user doesn't have a session key, redirect them to the API endpoint
	// for application auth.
	redirectURI := *r.URL
	redirectURI.Scheme = api.AccessURL.Scheme
	redirectURI.Host = host

	u := *api.AccessURL
	u.Path = "/api/v2/applications/auth-redirect"
	q := u.Query()
	q.Add(redirectURIQueryParam, redirectURI.String())
	u.RawQuery = q.Encode()

	http.Redirect(rw, r, u.String(), http.StatusTemporaryRedirect)
	return false
}

// setWorkspaceAppCookie sets a cookie on the workspace app domain. If the app
// hostname cannot be parsed properly, a static error page is rendered and false
// is returned.
//
// If an empty token is supplied, it will clear the cookie.
func (api *API) setWorkspaceAppCookie(rw http.ResponseWriter, r *http.Request, token string) bool {
	hostSplit := strings.SplitN(api.AppHostname, ".", 2)
	if len(hostSplit) != 2 {
		// This should be impossible as we verify the app hostname on
		// startup, but we'll check anyways.
		api.Logger.Error(r.Context(), "could not split invalid app hostname", slog.F("hostname", api.AppHostname))
		site.RenderStaticErrorPage(rw, r, site.ErrorPageData{
			Status:       http.StatusInternalServerError,
			Title:        "Internal Server Error",
			Description:  "The app is configured with an invalid app wildcard hostname. Please contact an administrator.",
			RetryEnabled: false,
			DashboardURL: api.AccessURL.String(),
		})
		return false
	}

	// Set the app cookie for all subdomains of api.AppHostname. This cookie is
	// handled properly by the ExtractAPIKey middleware.
	//
	// We don't set an expiration because the key in the database already has an
	// expiration.
	maxAge := 0
	if token == "" {
		maxAge = -1
	}
	cookieHost := "." + hostSplit[1]
	http.SetCookie(rw, &http.Cookie{
		Name:     httpmw.DevURLSessionTokenCookie,
		Value:    token,
		Domain:   cookieHost,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   api.SecureAuthCookie,
	})

	return true
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
	if api.AppHostname == "" {
		httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
			Message: "The server does not accept subdomain-based application requests.",
		})
		return
	}

	apiKey := httpmw.APIKey(r)
	if !api.Authorize(r, rbac.ActionCreate, apiKey) {
		httpapi.ResourceNotFound(rw)
		return
	}

	// Get the redirect URI from the query parameters and parse it.
	redirectURI := r.URL.Query().Get(redirectURIQueryParam)
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

	// Ensure that the redirect URI is a subdomain of api.AppHostname and is a
	// valid app subdomain.
	subdomain, ok := httpapi.ExecuteHostnamePattern(api.AppHostnameRegex, u.Host)
	if !ok {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "The redirect_uri query parameter must be a valid app subdomain.",
		})
		return
	}
	_, err = httpapi.ParseSubdomainAppURL(subdomain)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "The redirect_uri query parameter must be a valid app subdomain.",
			Detail:  err.Error(),
		})
		return
	}

	// Create the application_connect-scoped API key with the same lifetime as
	// the current session.
	exp := apiKey.ExpiresAt
	lifetimeSeconds := apiKey.LifetimeSeconds
	if exp.IsZero() || time.Until(exp) > api.DeploymentConfig.SessionDuration.Value {
		exp = database.Now().Add(api.DeploymentConfig.SessionDuration.Value)
		lifetimeSeconds = int64(api.DeploymentConfig.SessionDuration.Value.Seconds())
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
	encryptedAPIKey, err := encryptAPIKey(encryptedAPIKeyPayload{
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
	q.Set(subdomainProxyAPIKeyParam, encryptedAPIKey)
	u.RawQuery = q.Encode()
	http.Redirect(rw, r, u.String(), http.StatusTemporaryRedirect)
}

// proxyApplication are the required fields to proxy a workspace application.
type proxyApplication struct {
	AccessMethod workspaceAppAccessMethod
	Workspace    database.Workspace
	Agent        database.WorkspaceAgent

	// Either App or Port must be set, but not both.
	App  *database.WorkspaceApp
	Port uint16

	// SharingLevel MUST be set to database.AppSharingLevelOwner by default for
	// ports.
	SharingLevel database.AppSharingLevel
	// Path must either be empty or have a leading slash.
	Path string
}

func (api *API) proxyWorkspaceApplication(proxyApp proxyApplication, rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	sharingLevel := database.AppSharingLevelOwner
	if proxyApp.App != nil && proxyApp.App.SharingLevel != "" {
		sharingLevel = proxyApp.App.SharingLevel
	}
	if !api.checkWorkspaceApplicationAuth(rw, r, proxyApp.AccessMethod, proxyApp.Workspace, sharingLevel) {
		return
	}

	// Filter IP headers from untrusted origins!
	httpmw.FilterUntrustedOriginHeaders(api.RealIPConfig, r)
	// Ensure proper IP headers get sent to the forwarded application.
	err := httpmw.EnsureXForwardedForHeader(r)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	// If the app does not exist, but the app slug is a port number, then route
	// to the port as an "anonymous app". We only support HTTP for port-based
	// URLs.
	//
	// This is only supported for subdomain-based applications.
	internalURL := fmt.Sprintf("http://127.0.0.1:%d", proxyApp.Port)
	if proxyApp.App != nil {
		if !proxyApp.App.Url.Valid {
			site.RenderStaticErrorPage(rw, r, site.ErrorPageData{
				Status:       http.StatusBadRequest,
				Title:        "Bad Request",
				Description:  fmt.Sprintf("Application %q does not have a URL set.", proxyApp.App.Slug),
				RetryEnabled: true,
				DashboardURL: api.AccessURL.String(),
			})
			return
		}
		internalURL = proxyApp.App.Url.String
	}

	appURL, err := url.Parse(internalURL)
	if err != nil {
		site.RenderStaticErrorPage(rw, r, site.ErrorPageData{
			Status:       http.StatusBadRequest,
			Title:        "Bad Request",
			Description:  fmt.Sprintf("Application has an invalid URL %q: %s", internalURL, err.Error()),
			RetryEnabled: true,
			DashboardURL: api.AccessURL.String(),
		})
		return
	}

	// Verify that the port is allowed. See the docs above
	// `codersdk.MinimumListeningPort` for more details.
	port := appURL.Port()
	if port != "" {
		portInt, err := strconv.Atoi(port)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: fmt.Sprintf("App URL %q has an invalid port %q.", internalURL, port),
				Detail:  err.Error(),
			})
			return
		}

		if portInt < codersdk.WorkspaceAgentMinimumListeningPort {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: fmt.Sprintf("Application port %d is not permitted. Coder reserves ports less than %d for internal use.", portInt, codersdk.WorkspaceAgentMinimumListeningPort),
			})
			return
		}
	}

	// Ensure path and query parameter correctness.
	if proxyApp.Path == "" {
		// Web applications typically request paths relative to the
		// root URL. This allows for routing behind a proxy or subpath.
		// See https://github.com/coder/code-server/issues/241 for examples.
		http.Redirect(rw, r, r.URL.Path+"/", http.StatusTemporaryRedirect)
		return
	}
	if proxyApp.Path == "/" && r.URL.RawQuery == "" && appURL.RawQuery != "" {
		// If the application defines a default set of query parameters,
		// we should always respect them. The reverse proxy will merge
		// query parameters for server-side requests, but sometimes
		// client-side applications require the query parameters to render
		// properly. With code-server, this is the "folder" param.
		r.URL.RawQuery = appURL.RawQuery
		http.Redirect(rw, r, r.URL.String(), http.StatusTemporaryRedirect)
		return
	}

	r.URL.Path = proxyApp.Path
	appURL.RawQuery = ""

	proxy := httputil.NewSingleHostReverseProxy(appURL)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		site.RenderStaticErrorPage(rw, r, site.ErrorPageData{
			Status:       http.StatusBadGateway,
			Title:        "Bad Gateway",
			Description:  "Failed to proxy request to application: " + err.Error(),
			RetryEnabled: true,
			DashboardURL: api.AccessURL.String(),
		})
	}

	conn, release, err := api.workspaceAgentCache.Acquire(r, proxyApp.Agent.ID)
	if err != nil {
		site.RenderStaticErrorPage(rw, r, site.ErrorPageData{
			Status:       http.StatusBadGateway,
			Title:        "Bad Gateway",
			Description:  "Could not connect to workspace agent: " + err.Error(),
			RetryEnabled: true,
			DashboardURL: api.AccessURL.String(),
		})
		return
	}
	defer release()
	proxy.Transport = conn.HTTPTransport()

	// This strips the session token from a workspace app request.
	cookieHeaders := r.Header.Values("Cookie")[:]
	r.Header.Del("Cookie")
	for _, cookieHeader := range cookieHeaders {
		r.Header.Add("Cookie", httpapi.StripCoderCookies(cookieHeader))
	}

	// Convert canonicalized headers to their non-canonicalized counterparts.
	// See the comment on `nonCanonicalHeaders` for more information on why this
	// is necessary.
	for k, v := range r.Header {
		if n, ok := nonCanonicalHeaders[k]; ok {
			r.Header.Del(k)
			r.Header[n] = v
		}
	}

	// end span so we don't get long lived trace data
	tracing.EndHTTPSpan(r, http.StatusOK, trace.SpanFromContext(ctx))

	proxy.ServeHTTP(rw, r)
}

type encryptedAPIKeyPayload struct {
	APIKey    string    `json:"api_key"`
	ExpiresAt time.Time `json:"expires_at"`
}

// encryptAPIKey encrypts an API key with it's own hashed secret. This is used
// for smuggling (application_connect scoped) API keys securely to app
// hostnames.
//
// We encrypt API keys when smuggling them in query parameters to avoid them
// getting accidentally logged in access logs or stored in browser history.
func encryptAPIKey(data encryptedAPIKeyPayload) (string, error) {
	if data.APIKey == "" {
		return "", xerrors.New("API key is empty")
	}
	if data.ExpiresAt.IsZero() {
		// Very short expiry as these keys are only used once as part of an
		// automatic redirection flow.
		data.ExpiresAt = database.Now().Add(time.Minute)
	}

	payload, err := json.Marshal(data)
	if err != nil {
		return "", xerrors.Errorf("marshal payload: %w", err)
	}

	// We use the hashed key secret as the encryption key. The hashed secret is
	// stored in the API keys table. The HashedSecret is NEVER returned from the
	// API.
	//
	// We chose to use the key secret as the private key for encryption instead
	// of a shared key for a few reasons:
	//   1. A single private key used to encrypt every API key would also be
	//      stored in the database, which means that the risk factor is similar.
	//   2. The secret essentially rotates for each key (for free!), since each
	//      key has a different secret. This means that if someone acquires an
	//      old database dump they can't decrypt new API keys.
	//   3. These tokens are scoped only for application_connect access.
	keyID, keySecret, err := httpmw.SplitAPIToken(data.APIKey)
	if err != nil {
		return "", xerrors.Errorf("split API key: %w", err)
	}
	// SHA256 the key secret so it matches the hashed secret in the database.
	// The key length doesn't matter to the jose.Encrypter.
	privateKey := sha256.Sum256([]byte(keySecret))

	// JWEs seem to apply a nonce themselves.
	encrypter, err := jose.NewEncrypter(
		jose.A256GCM,
		jose.Recipient{
			Algorithm: jose.A256GCMKW,
			KeyID:     keyID,
			Key:       privateKey[:],
		},
		&jose.EncrypterOptions{
			Compression: jose.DEFLATE,
		},
	)
	if err != nil {
		return "", xerrors.Errorf("initializer jose encrypter: %w", err)
	}
	encryptedObject, err := encrypter.Encrypt(payload)
	if err != nil {
		return "", xerrors.Errorf("encrypt jwe: %w", err)
	}

	encrypted := encryptedObject.FullSerialize()
	return base64.RawURLEncoding.EncodeToString([]byte(encrypted)), nil
}

// decryptAPIKey undoes encryptAPIKey and is used in the subdomain app handler.
func decryptAPIKey(ctx context.Context, db database.Store, encryptedAPIKey string) (database.APIKey, string, error) {
	encrypted, err := base64.RawURLEncoding.DecodeString(encryptedAPIKey)
	if err != nil {
		return database.APIKey{}, "", xerrors.Errorf("base64 decode encrypted API key: %w", err)
	}

	object, err := jose.ParseEncrypted(string(encrypted))
	if err != nil {
		return database.APIKey{}, "", xerrors.Errorf("parse encrypted API key: %w", err)
	}

	// Lookup the API key so we can decrypt it.
	keyID := object.Header.KeyID
	//nolint:gocritic // needed to check API key
	key, err := db.GetAPIKeyByID(dbauthz.AsSystemRestricted(ctx), keyID)
	if err != nil {
		return database.APIKey{}, "", xerrors.Errorf("get API key by key ID: %w", err)
	}

	// Decrypt using the hashed secret.
	decrypted, err := object.Decrypt(key.HashedSecret)
	if err != nil {
		return database.APIKey{}, "", xerrors.Errorf("decrypt API key: %w", err)
	}

	// Unmarshal the payload.
	var payload encryptedAPIKeyPayload
	if err := json.Unmarshal(decrypted, &payload); err != nil {
		return database.APIKey{}, "", xerrors.Errorf("unmarshal decrypted payload: %w", err)
	}

	// Validate expiry.
	if payload.ExpiresAt.Before(database.Now()) {
		return database.APIKey{}, "", xerrors.New("encrypted API key expired")
	}

	// Validate that the key matches the one we got from the DB.
	gotKeyID, gotKeySecret, err := httpmw.SplitAPIToken(payload.APIKey)
	if err != nil {
		return database.APIKey{}, "", xerrors.Errorf("split API key: %w", err)
	}
	gotHashedSecret := sha256.Sum256([]byte(gotKeySecret))
	if gotKeyID != key.ID || !bytes.Equal(key.HashedSecret, gotHashedSecret[:]) {
		return database.APIKey{}, "", xerrors.New("encrypted API key does not match key in database")
	}

	return key, payload.APIKey, nil
}

// renderApplicationNotFound should always be used when the app is not found or
// the current user doesn't have permission to access it.
func renderApplicationNotFound(rw http.ResponseWriter, r *http.Request, accessURL *url.URL) {
	site.RenderStaticErrorPage(rw, r, site.ErrorPageData{
		Status:       http.StatusNotFound,
		Title:        "Application Not Found",
		Description:  "The application or workspace you are trying to access does not exist or you do not have permission to access it.",
		RetryEnabled: false,
		DashboardURL: accessURL.String(),
	})
}

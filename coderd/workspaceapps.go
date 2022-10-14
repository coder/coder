package coderd

import (
	"bytes"
	"context"
	"crypto/sha256"
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
	redirectURIQueryParam     = "redirect_uri"
)

func (api *API) appHost(rw http.ResponseWriter, r *http.Request) {
	httpapi.Write(r.Context(), rw, http.StatusOK, codersdk.GetAppHostResponse{
		Host: api.AppHostname,
	})
}

// workspaceAppsProxyPath proxies requests to a workspace application
// through a relative URL path.
func (api *API) workspaceAppsProxyPath(rw http.ResponseWriter, r *http.Request) {
	workspace := httpmw.WorkspaceParam(r)
	agent := httpmw.WorkspaceAgentParam(r)

	// We do not support port proxying on paths, so lookup the app by name.
	appName := chi.URLParam(r, "workspaceapp")
	app, ok := api.lookupWorkspaceApp(rw, r, agent.ID, appName)
	if !ok {
		return
	}

	appSharingLevel := database.AppSharingLevelOwner
	if app.SharingLevel != "" {
		appSharingLevel = app.SharingLevel
	}
	authed, ok := api.fetchWorkspaceApplicationAuth(rw, r, workspace, appSharingLevel)
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
		Workspace: workspace,
		Agent:     agent,
		App:       &app,
		Port:      0,
		Path:      chiPath,
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
			if api.AppHostname == "" {
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
				if app.AppName != "" {
					workspaceApp, ok := api.lookupWorkspaceApp(rw, r, agent.ID, app.AppName)
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
					Workspace: workspace,
					Agent:     agent,
					App:       workspaceAppPtr,
					Port:      app.Port,
					Path:      r.URL.Path,
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

	// Split the subdomain so we can parse the application details and verify it
	// matches the configured app hostname later.
	subdomain, rest := httpapi.SplitSubdomain(host)
	if rest == "" {
		// If there are no periods in the hostname, then it can't be a valid
		// application URL.
		next.ServeHTTP(rw, r)
		return httpapi.ApplicationURL{}, false
	}
	matchingBaseHostname := httpapi.HostnamesMatch(api.AppHostname, rest)

	// Parse the application URL from the subdomain.
	app, err := httpapi.ParseSubdomainAppURL(subdomain)
	if err != nil {
		// If it isn't a valid app URL and the base domain doesn't match the
		// configured app hostname, this request was probably destined for the
		// dashboard/API router.
		if !matchingBaseHostname {
			next.ServeHTTP(rw, r)
			return httpapi.ApplicationURL{}, false
		}

		site.RenderStaticErrorPage(rw, r, site.ErrorPageData{
			Status:       http.StatusBadRequest,
			Title:        "Invalid application URL",
			Description:  fmt.Sprintf("Could not parse subdomain application URL %q: %s", subdomain, err.Error()),
			RetryEnabled: false,
			DashboardURL: api.AccessURL.String(),
		})
		return httpapi.ApplicationURL{}, false
	}

	// At this point we've verified that the subdomain looks like a valid
	// application URL, so the base hostname should match the configured app
	// hostname.
	if !matchingBaseHostname {
		site.RenderStaticErrorPage(rw, r, site.ErrorPageData{
			Status:       http.StatusNotFound,
			Title:        "Not Found",
			Description:  "The server does not accept application requests on this hostname.",
			RetryEnabled: false,
			DashboardURL: api.AccessURL.String(),
		})
		return httpapi.ApplicationURL{}, false
	}

	return app, true
}

// lookupWorkspaceApp looks up the workspace application by name in the given
// agent and returns it. If the application is not found or there was a server
// error while looking it up, an HTML error page is returned and false is
// returned so the caller can return early.
func (api *API) lookupWorkspaceApp(rw http.ResponseWriter, r *http.Request, agentID uuid.UUID, appName string) (database.WorkspaceApp, bool) {
	app, err := api.Database.GetWorkspaceAppByAgentIDAndName(r.Context(), database.GetWorkspaceAppByAgentIDAndNameParams{
		AgentID: agentID,
		Name:    appName,
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

func (api *API) authorizeWorkspaceApp(r *http.Request, sharingLevel database.AppSharingLevel, workspace database.Workspace) (bool, error) {
	ctx := r.Context()

	// Short circuit if not authenticated.
	roles, ok := httpmw.UserAuthorizationOptional(r)
	if !ok {
		// The user is not authenticated, so they can only access the app if it
		// is public.
		return sharingLevel == database.AppSharingLevelPublic, nil
	}

	// Do a standard RBAC check. This accounts for share level "owner" and any
	// other RBAC rules that may be in place.
	//
	// Regardless of share level or whether it's enabled or not, the owner of
	// the workspace can always access applications (as long as their API key's
	// scope allows it).
	err := api.Authorizer.ByRoleName(ctx, roles.ID.String(), roles.Roles, roles.Scope.ToRBAC(), []string{}, rbac.ActionCreate, workspace.ApplicationConnectRBAC())
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
		object := rbac.ResourceWorkspaceApplicationConnect.WithOwner(roles.ID.String())
		err := api.Authorizer.ByRoleName(ctx, roles.ID.String(), roles.Roles, roles.Scope.ToRBAC(), []string{}, rbac.ActionCreate, object)
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
func (api *API) fetchWorkspaceApplicationAuth(rw http.ResponseWriter, r *http.Request, workspace database.Workspace, appSharingLevel database.AppSharingLevel) (authed bool, ok bool) {
	ok, err := api.authorizeWorkspaceApp(r, appSharingLevel, workspace)
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
func (api *API) checkWorkspaceApplicationAuth(rw http.ResponseWriter, r *http.Request, workspace database.Workspace, appSharingLevel database.AppSharingLevel) bool {
	authed, ok := api.fetchWorkspaceApplicationAuth(rw, r, workspace, appSharingLevel)
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
	authed, ok := api.fetchWorkspaceApplicationAuth(rw, r, workspace, appSharingLevel)
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
		_, apiKey, err := decryptAPIKey(r.Context(), api.Database, encryptedAPIKey)
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

		// Set the app cookie for all subdomains of api.AppHostname. This cookie
		// is handled properly by the ExtractAPIKey middleware.
		http.SetCookie(rw, &http.Cookie{
			Name:     httpmw.DevURLSessionTokenCookie,
			Value:    apiKey,
			Domain:   "." + api.AppHostname,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			Secure:   api.SecureAuthCookie,
		})

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

// workspaceApplicationAuth is an endpoint on the main router that handles
// redirects from the subdomain handler.
//
// This endpoint is under /api so we don't return the friendly error page here.
// Any errors on this endpoint should be errors that are unlikely to happen
// in production unless the user messes with the URL.
func (api *API) workspaceApplicationAuth(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if api.AppHostname == "" {
		httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
			Message: "The server does not accept subdomain-based application requests.",
		})
		return
	}

	apiKey := httpmw.APIKey(r)
	if !api.Authorize(r, rbac.ActionCreate, rbac.ResourceAPIKey.WithOwner(apiKey.UserID.String())) {
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
	subdomain, rest := httpapi.SplitSubdomain(u.Hostname())
	if !httpapi.HostnamesMatch(api.AppHostname, rest) {
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
	// the current session (defaulting to 1 day, capped to 1 week).
	exp := apiKey.ExpiresAt
	if exp.IsZero() {
		exp = database.Now().Add(time.Hour * 24)
	}
	if time.Until(exp) > time.Hour*24*7 {
		exp = database.Now().Add(time.Hour * 24 * 7)
	}
	lifetime := apiKey.LifetimeSeconds
	if lifetime > int64((time.Hour * 24 * 7).Seconds()) {
		lifetime = int64((time.Hour * 24 * 7).Seconds())
	}
	cookie, err := api.createAPIKey(ctx, createAPIKeyParams{
		UserID:          apiKey.UserID,
		LoginType:       database.LoginTypePassword,
		ExpiresAt:       exp,
		LifetimeSeconds: lifetime,
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
	Workspace database.Workspace
	Agent     database.WorkspaceAgent

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
	if !api.checkWorkspaceApplicationAuth(rw, r, proxyApp.Workspace, sharingLevel) {
		return
	}

	// If the app does not exist, but the app name is a port number, then
	// route to the port as an "anonymous app". We only support HTTP for
	// port-based URLs.
	//
	// This is only supported for subdomain-based applications.
	internalURL := fmt.Sprintf("http://127.0.0.1:%d", proxyApp.Port)

	// If the app name was used instead, fetch the app from the database so we
	// can get the internal URL.
	if proxyApp.App != nil {
		if !proxyApp.App.Url.Valid {
			site.RenderStaticErrorPage(rw, r, site.ErrorPageData{
				Status:       http.StatusBadRequest,
				Title:        "Bad Request",
				Description:  fmt.Sprintf("Application %q does not have a URL set.", proxyApp.App.Name),
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

		if portInt < codersdk.MinimumListeningPort {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: fmt.Sprintf("Application port %d is not permitted. Coder reserves ports less than %d for internal use.", portInt, codersdk.MinimumListeningPort),
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

	// This strips the session token from a workspace app request.
	cookieHeaders := r.Header.Values("Cookie")[:]
	r.Header.Del("Cookie")
	for _, cookieHeader := range cookieHeaders {
		r.Header.Add("Cookie", httpapi.StripCoderCookies(cookieHeader))
	}
	proxy.Transport = conn.HTTPTransport()

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
	key, err := db.GetAPIKeyByID(ctx, keyID)
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

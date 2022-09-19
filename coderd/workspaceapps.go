package coderd

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel/trace"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/coderd/tracing"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/site"
)

// workspaceAppsProxyPath proxies requests to a workspace application
// through a relative URL path.
func (api *API) workspaceAppsProxyPath(rw http.ResponseWriter, r *http.Request) {
	workspace := httpmw.WorkspaceParam(r)
	agent := httpmw.WorkspaceAgentParam(r)

	if !api.Authorize(r, rbac.ActionCreate, workspace.ApplicationConnectRBAC()) {
		httpapi.ResourceNotFound(rw)
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
		// We do not support port proxying for paths.
		AppName:          chi.URLParam(r, "workspaceapp"),
		Port:             0,
		Path:             chiPath,
		DashboardOnError: true,
	}, rw, r)
}

func (api *API) handleSubdomainApplications(middlewares ...func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			host := httpapi.RequestHost(r)
			if host == "" {
				if r.URL.Path == "/derp" {
					// The /derp endpoint is used by wireguard clients to tunnel
					// through coderd. For some reason these requests don't set
					// a Host header properly sometimes (no idea how), which
					// causes this path to get hit.
					next.ServeHTTP(rw, r)
					return
				}

				httpapi.Write(rw, http.StatusBadRequest, codersdk.Response{
					Message: "Could not determine request Host.",
				})
				return
			}

			app, err := httpapi.ParseSubdomainAppURL(host)
			if err != nil {
				// Subdomain is not a valid application url. Pass through to the
				// rest of the app.
				// TODO: @emyrk we should probably catch invalid subdomains. Meaning
				// 	an invalid application should not route to the coderd.
				//	To do this we would need to know the list of valid access urls
				//	though?
				next.ServeHTTP(rw, r)
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

				api.proxyWorkspaceApplication(proxyApplication{
					Workspace:        workspace,
					Agent:            agent,
					AppName:          app.AppName,
					Port:             app.Port,
					Path:             r.URL.Path,
					DashboardOnError: false,
				}, rw, r)
			})).ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}

// proxyApplication are the required fields to proxy a workspace application.
type proxyApplication struct {
	Workspace database.Workspace
	Agent     database.WorkspaceAgent

	// Either AppName or Port must be set, but not both.
	AppName string
	Port    uint16
	// Path must either be empty or have a leading slash.
	Path string

	// DashboardOnError determines whether or not the dashboard should be
	// rendered on error. This should be set for proxy path URLs but not
	// hostname based URLs.
	DashboardOnError bool
}

func (api *API) proxyWorkspaceApplication(proxyApp proxyApplication, rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !api.Authorize(r, rbac.ActionCreate, proxyApp.Workspace.ApplicationConnectRBAC()) {
		httpapi.ResourceNotFound(rw)
		return
	}

	// If the app does not exist, but the app name is a port number, then
	// route to the port as an "anonymous app". We only support HTTP for
	// port-based URLs.
	internalURL := fmt.Sprintf("http://127.0.0.1:%d", proxyApp.Port)

	// If the app name was used instead, fetch the app from the database so we
	// can get the internal URL.
	if proxyApp.AppName != "" {
		app, err := api.Database.GetWorkspaceAppByAgentIDAndName(ctx, database.GetWorkspaceAppByAgentIDAndNameParams{
			AgentID: proxyApp.Agent.ID,
			Name:    proxyApp.AppName,
		})
		if err != nil {
			httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error fetching workspace application.",
				Detail:  err.Error(),
			})
			return
		}

		if !app.Url.Valid {
			httpapi.Write(rw, http.StatusBadRequest, codersdk.Response{
				Message: fmt.Sprintf("Application %s does not have a url.", app.Name),
			})
			return
		}
		internalURL = app.Url.String
	}

	appURL, err := url.Parse(internalURL)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: fmt.Sprintf("App URL %q is invalid.", internalURL),
			Detail:  err.Error(),
		})
		return
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
		if proxyApp.DashboardOnError {
			// To pass friendly errors to the frontend, special meta tags are
			// overridden in the index.html with the content passed here.
			r = r.WithContext(site.WithAPIResponse(ctx, site.APIResponse{
				StatusCode: http.StatusBadGateway,
				Message:    err.Error(),
			}))
			api.siteHandler.ServeHTTP(w, r)
			return
		}

		httpapi.Write(w, http.StatusBadGateway, codersdk.Response{
			Message: "Failed to proxy request to application.",
			Detail:  err.Error(),
		})
	}

	conn, release, err := api.workspaceAgentCache.Acquire(r, proxyApp.Agent.ID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to dial workspace agent.",
			Detail:  err.Error(),
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

// applicationCookie is a helper function to copy the auth cookie to also
// support subdomains. Until we support creating authentication cookies that can
// only do application authentication, we will just reuse the original token.
// This code should be temporary and be replaced with something that creates
// a unique session_token.
//
// Returns nil if the access URL isn't a hostname.
func (api *API) applicationCookie(authCookie *http.Cookie) *http.Cookie {
	if net.ParseIP(api.AccessURL.Hostname()) != nil {
		return nil
	}

	appCookie := *authCookie
	// We only support setting this cookie on the access URL subdomains. This is
	// to ensure we don't accidentally leak the auth cookie to subdomains on
	// another hostname.
	appCookie.Domain = "." + api.AccessURL.Hostname()
	return &appCookie
}

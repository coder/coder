package coderd

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/coderd/tracing"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/site"
	"github.com/go-chi/chi/v5"
)

// workspaceAppsProxyPath proxies requests to a workspace application
// through a relative URL path.
func (api *API) workspaceAppsProxyPath(rw http.ResponseWriter, r *http.Request) {
	workspace := httpmw.WorkspaceParam(r)
	agent := httpmw.WorkspaceAgentParam(r)

	if !api.Authorize(r, rbac.ActionCreate, workspace.ExecutionRBAC()) {
		httpapi.ResourceNotFound(rw)
		return
	}

	api.proxyWorkspaceApplication(proxyApplication{
		Workspace: workspace,
		Agent:     agent,
		AppName:   chi.URLParam(r, "workspaceapp"),
	}, rw, r)
}

// proxyApplication are the required fields to proxy a workspace application.
type proxyApplication struct {
	Workspace database.Workspace
	Agent     database.WorkspaceAgent

	AppName string
}

func (api *API) proxyWorkspaceApplication(proxyApp proxyApplication, rw http.ResponseWriter, r *http.Request) {
	if !api.Authorize(r, rbac.ActionCreate, proxyApp.Workspace.ExecutionRBAC()) {
		httpapi.ResourceNotFound(rw)
		return
	}

	var internalURL string

	num, err := strconv.Atoi(proxyApp.AppName)
	if err == nil && num <= 65535 {
		// TODO: @emyrk we should probably allow changing the schema?
		internalURL = "http://localhost:" + proxyApp.AppName
	} else {
		app, err := api.Database.GetWorkspaceAppByAgentIDAndName(r.Context(), database.GetWorkspaceAppByAgentIDAndNameParams{
			AgentID: proxyApp.Agent.ID,
			Name:    proxyApp.AppName,
		})
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.Write(rw, http.StatusNotFound, codersdk.Response{
				Message: "Application not found.",
			})
			return
		}
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
			Message: fmt.Sprintf("App url %q must be a valid url.", internalURL),
			Detail:  err.Error(),
		})
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(appURL)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		// This is a browser-facing route so JSON responses are not viable here.
		// To pass friendly errors to the frontend, special meta tags are overridden
		// in the index.html with the content passed here.
		r = r.WithContext(site.WithAPIResponse(r.Context(), site.APIResponse{
			StatusCode: http.StatusBadGateway,
			Message:    err.Error(),
		}))
		api.siteHandler.ServeHTTP(w, r)
	}
	path := chi.URLParam(r, "*")
	if !strings.HasSuffix(r.URL.Path, "/") && path == "" {
		// Web applications typically request paths relative to the
		// root URL. This allows for routing behind a proxy or subpath.
		// See https://github.com/coder/code-server/issues/241 for examples.
		r.URL.Path += "/"
		http.Redirect(rw, r, r.URL.String(), http.StatusTemporaryRedirect)
		return
	}
	if r.URL.RawQuery == "" && appURL.RawQuery != "" {
		// If the application defines a default set of query parameters,
		// we should always respect them. The reverse proxy will merge
		// query parameters for server-side requests, but sometimes
		// client-side applications require the query parameters to render
		// properly. With code-server, this is the "folder" param.
		r.URL.RawQuery = appURL.RawQuery
		http.Redirect(rw, r, r.URL.String(), http.StatusTemporaryRedirect)
		return
	}
	r.URL.Path = path

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
	tracing.EndHTTPSpan(r, 200)

	proxy.ServeHTTP(rw, r)
}

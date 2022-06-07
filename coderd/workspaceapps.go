package coderd

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/site"
)

// workspaceAppsProxyPath proxies requests to a workspace application
// through a relative URL path.
func (api *API) workspaceAppsProxyPath(rw http.ResponseWriter, r *http.Request) {
	user := httpmw.UserParam(r)
	// This can be in the form of: "<workspace-name>.[workspace-agent]" or "<workspace-name>"
	workspaceWithAgent := chi.URLParam(r, "workspacename")
	workspaceParts := strings.Split(workspaceWithAgent, ".")

	workspace, err := api.Database.GetWorkspaceByOwnerIDAndName(r.Context(), database.GetWorkspaceByOwnerIDAndNameParams{
		OwnerID: user.ID,
		Name:    workspaceParts[0],
	})
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusNotFound, httpapi.Response{
			Message: "Workspace not found.",
		})
		return
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: "Internal error fetching workspace.",
			Detail:  err.Error(),
		})
		return
	}
	if !api.Authorize(rw, r, rbac.ActionRead, workspace) {
		return
	}

	build, err := api.Database.GetLatestWorkspaceBuildByWorkspaceID(r.Context(), workspace.ID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: "Internal error fetching workspace build.",
			Detail:  err.Error(),
		})
		return
	}

	resources, err := api.Database.GetWorkspaceResourcesByJobID(r.Context(), build.JobID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: "Internal error fetching workspace resources.",
			Detail:  err.Error(),
		})
		return
	}
	resourceIDs := make([]uuid.UUID, 0)
	for _, resource := range resources {
		resourceIDs = append(resourceIDs, resource.ID)
	}
	agents, err := api.Database.GetWorkspaceAgentsByResourceIDs(r.Context(), resourceIDs)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: "Internal error fetching workspace agents.",
			Detail:  err.Error(),
		})
		return
	}
	if len(agents) == 0 {
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message: "No agents exist.",
		})
	}

	agent := agents[0]
	if len(workspaceParts) > 1 {
		for _, otherAgent := range agents {
			if otherAgent.Name == workspaceParts[1] {
				agent = otherAgent
				break
			}
		}
	}

	app, err := api.Database.GetWorkspaceAppByAgentIDAndName(r.Context(), database.GetWorkspaceAppByAgentIDAndNameParams{
		AgentID: agent.ID,
		Name:    chi.URLParam(r, "workspaceapp"),
	})
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusNotFound, httpapi.Response{
			Message: "Application not found.",
		})
		return
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: "Internal error fetching workspace application.",
			Detail:  err.Error(),
		})
		return
	}
	if !app.Url.Valid {
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message: fmt.Sprintf("Application %s does not have a url.", app.Name),
		})
		return
	}

	appURL, err := url.Parse(app.Url.String)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("App url %q must be a valid url.", app.Url.String),
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

	conn, release, err := api.workspaceAgentCache.Acquire(r, agent.ID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: "Failed to dial workspace agent.",
			Detail:  err.Error(),
		})
		return
	}
	defer release()

	proxy.Transport = conn.HTTPTransport()
	proxy.ServeHTTP(rw, r)
}

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
	"github.com/coder/coder/site"
)

// workspaceAppsProxyPath proxies requests to a workspace application
// through a relative URL path.
func (api *API) workspaceAppsProxyPath(rw http.ResponseWriter, r *http.Request) {
	user := httpmw.UserParam(r)
	// This can be in the form of: "<workspace-name>.[workspace-agent]" or "<workspace-name>"
	workspaceWithAgent := chi.URLParam(r, "workspaceagent")
	workspaceParts := strings.Split(workspaceWithAgent, ".")

	workspace, err := api.Database.GetWorkspaceByOwnerIDAndName(r.Context(), database.GetWorkspaceByOwnerIDAndNameParams{
		OwnerID: user.ID,
		Name:    workspaceParts[0],
	})
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusNotFound, httpapi.Response{
			Message: "workspace not found",
		})
		return
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get workspace: %s", err),
		})
		return
	}

	build, err := api.Database.GetLatestWorkspaceBuildByWorkspaceID(r.Context(), workspace.ID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get workspace build: %s", err),
		})
		return
	}

	resources, err := api.Database.GetWorkspaceResourcesByJobID(r.Context(), build.JobID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get workspace resources: %s", err),
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
			Message: fmt.Sprintf("get workspace agents: %s", err),
		})
		return
	}
	if len(agents) == 0 {
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message: "no agents exist",
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
		Name:    chi.URLParam(r, "application"),
	})
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusNotFound, httpapi.Response{
			Message: "application not found",
		})
		return
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get workspace app: %s", err),
		})
		return
	}
	if !app.Url.Valid {
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message: fmt.Sprintf("application does not have a url: %s", err),
		})
		return
	}

	appURL, err := url.Parse(app.Url.String)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("parse app url: %s", err),
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
		w.WriteHeader(http.StatusBadGateway)
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
			Message: fmt.Sprintf("dial workspace agent: %s", err),
		})
		return
	}
	defer release()

	proxy.Transport = conn.HTTPTransport()
	proxy.ServeHTTP(rw, r)
}

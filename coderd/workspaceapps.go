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
)

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

	conn, err := api.dialWorkspaceAgent(r, agent.ID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("dial workspace agent: %s", err),
		})
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(appURL)
	defaultTransport, valid := http.DefaultTransport.(*http.Transport)
	if !valid {
		panic("dev error: default transport isn't a transport")
	}

	transport := defaultTransport.Clone()
	transport.DialContext = conn.DialContext
	proxy.Transport = transport
	r.URL.Path = chi.URLParam(r, "*")
	proxy.ServeHTTP(rw, r)
}

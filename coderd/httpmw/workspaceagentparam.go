package httpmw

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
)

type workspaceAgentParamContextKey struct{}

// WorkspaceAgentParam returns the workspace agent from the ExtractWorkspaceAgentParam handler.
func WorkspaceAgentParam(r *http.Request) database.WorkspaceAgent {
	user, ok := r.Context().Value(workspaceAgentParamContextKey{}).(database.WorkspaceAgent)
	if !ok {
		panic("developer error: agent middleware not provided")
	}
	return user
}

// ExtractWorkspaceAgentParam grabs a workspace agent from the "workspaceagent" URL parameter.
func ExtractWorkspaceAgentParam(db database.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			agentUUID, parsed := parseUUID(rw, r, "workspaceagent")
			if !parsed {
				return
			}

			agent, err := db.GetWorkspaceAgentByID(r.Context(), agentUUID)
			if errors.Is(err, sql.ErrNoRows) {
				httpapi.Write(rw, http.StatusNotFound, httpapi.Response{
					Message: "Agent doesn't exist with that id.",
				})
				return
			}
			if err != nil {
				httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
					Message: "Internal error fetching workspace agent.",
					Detail:  err.Error(),
				})
				return
			}

			resource, err := db.GetWorkspaceResourceByID(r.Context(), agent.ResourceID)
			if err != nil {
				httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
					Message: "Internal error fetching workspace resource.",
					Detail:  err.Error(),
				})
				return
			}

			job, err := db.GetProvisionerJobByID(r.Context(), resource.JobID)
			if err != nil {
				httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
					Message: "Internal error fetching provisioner job.",
					Detail:  err.Error(),
				})
				return
			}
			if job.Type != database.ProvisionerJobTypeWorkspaceBuild {
				httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
					Message: "Workspace agents can only be fetched for builds.",
				})
				return
			}
			build, err := db.GetWorkspaceBuildByJobID(r.Context(), job.ID)
			if err != nil {
				httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
					Message: "Internal error fetching workspace build.",
					Detail:  err.Error(),
				})
				return
			}

			ctx := context.WithValue(r.Context(), workspaceAgentParamContextKey{}, agent)
			chi.RouteContext(ctx).URLParams.Add("workspace", build.WorkspaceID.String())
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}

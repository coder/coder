package httpmw

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
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
			ctx := r.Context()
			agentUUID, parsed := ParseUUIDParam(rw, r, "workspaceagent")
			if !parsed {
				return
			}

			agent, err := db.GetWorkspaceAgentByID(ctx, agentUUID)
			if httpapi.Is404Error(err) {
				httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
					Message: "Agent doesn't exist with that id, or you do not have access to it.",
				})
				return
			}
			if err != nil {
				httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
					Message: "Internal error fetching workspace agent.",
					Detail:  err.Error(),
				})
				return
			}

			resource, err := db.GetWorkspaceResourceByID(ctx, agent.ResourceID)
			if err != nil {
				httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
					Message: "Internal error fetching workspace resource.",
					Detail:  err.Error(),
				})
				return
			}

			job, err := db.GetProvisionerJobByID(ctx, resource.JobID)
			if err != nil {
				httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
					Message: "Internal error fetching provisioner job.",
					Detail:  err.Error(),
				})
				return
			}
			if job.Type != database.ProvisionerJobTypeWorkspaceBuild {
				httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
					Message: "Workspace agents can only be fetched for builds.",
				})
				return
			}
			build, err := db.GetWorkspaceBuildByJobID(ctx, job.ID)
			if err != nil {
				httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
					Message: "Internal error fetching workspace build.",
					Detail:  err.Error(),
				})
				return
			}

			ctx = context.WithValue(ctx, workspaceAgentParamContextKey{}, agent)
			chi.RouteContext(ctx).URLParams.Add("workspace", build.WorkspaceID.String())
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}

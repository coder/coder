package httpmw

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw/loggermw"
	"github.com/coder/coder/v2/codersdk"
)

type workspaceParamContextKey struct{}

// WorkspaceParam returns the workspace from the ExtractWorkspaceParam handler.
func WorkspaceParam(r *http.Request) database.Workspace {
	workspace, ok := r.Context().Value(workspaceParamContextKey{}).(database.Workspace)
	if !ok {
		panic("developer error: workspace param middleware not provided")
	}
	return workspace
}

// ExtractWorkspaceParam grabs a workspace from the "workspace" URL parameter.
func ExtractWorkspaceParam(db database.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			workspaceID, parsed := ParseUUIDParam(rw, r, "workspace")
			if !parsed {
				return
			}
			workspace, err := db.GetWorkspaceByID(ctx, workspaceID)
			if httpapi.Is404Error(err) {
				httpapi.ResourceNotFound(rw)
				return
			}
			if err != nil {
				httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
					Message: "Internal error fetching workspace.",
					Detail:  err.Error(),
				})
				return
			}

			ctx = context.WithValue(ctx, workspaceParamContextKey{}, workspace)

			if rlogger := loggermw.RequestLoggerFromContext(ctx); rlogger != nil {
				rlogger.WithFields(slog.F("workspace_name", workspace.Name))
			}

			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}

// ExtractWorkspaceAndAgentParam grabs a workspace and an agent from the
// "workspace_and_agent" URL parameter. `ExtractUserParam` must be called
// before this.
// This can be in the form of:
//   - "<workspace-name>.[workspace-agent]"	: If multiple agents exist
//   - "<workspace-name>"					: If one agent exists
func ExtractWorkspaceAndAgentParam(db database.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			user := UserParam(r)
			workspaceWithAgent := chi.URLParam(r, "workspace_and_agent")
			workspaceParts := strings.Split(workspaceWithAgent, ".")

			workspace, err := db.GetWorkspaceByOwnerIDAndName(ctx, database.GetWorkspaceByOwnerIDAndNameParams{
				OwnerID: user.ID,
				Name:    workspaceParts[0],
			})
			if err != nil {
				if httpapi.Is404Error(err) {
					httpapi.ResourceNotFound(rw)
					return
				}
				httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
					Message: "Internal error fetching workspace.",
					Detail:  err.Error(),
				})
				return
			}

			build, err := db.GetLatestWorkspaceBuildByWorkspaceID(ctx, workspace.ID)
			if err != nil {
				httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
					Message: "Internal error fetching workspace build.",
					Detail:  err.Error(),
				})
				return
			}

			resources, err := db.GetWorkspaceResourcesByJobID(ctx, build.JobID)
			if err != nil {
				httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
					Message: "Internal error fetching workspace resources.",
					Detail:  err.Error(),
				})
				return
			}
			resourceIDs := make([]uuid.UUID, 0)
			for _, resource := range resources {
				resourceIDs = append(resourceIDs, resource.ID)
			}

			agents, err := db.GetWorkspaceAgentsByResourceIDs(ctx, resourceIDs)
			if err != nil {
				httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
					Message: "Internal error fetching workspace agents.",
					Detail:  err.Error(),
				})
				return
			}

			if len(agents) == 0 {
				httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
					Message: "No agents exist for this workspace",
				})
				return
			}

			// If we have more than 1 workspace agent, we need to specify which one to use.
			if len(agents) > 1 && len(workspaceParts) <= 1 {
				httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
					Message: "More than one agent exists, but no agent specified.",
				})
				return
			}

			var agent database.WorkspaceAgent
			var found bool
			// If we have more than 1 workspace agent, we need to specify which one to use.
			// If the user specified an agent, we need to make sure that agent
			// actually exists.
			if len(workspaceParts) > 1 || len(agents) > 1 {
				for _, otherAgent := range agents {
					if otherAgent.Name == workspaceParts[1] {
						agent = otherAgent
						found = true
						break
					}
				}
				if !found {
					httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
						Message: fmt.Sprintf("No agent exists with the name %q", workspaceParts[1]),
					})
					return
				}
			} else {
				agent = agents[0]
			}

			ctx = context.WithValue(ctx, workspaceParamContextKey{}, workspace)
			ctx = context.WithValue(ctx, workspaceAgentParamContextKey{}, agent)

			if rlogger := loggermw.RequestLoggerFromContext(ctx); rlogger != nil {
				rlogger.WithFields(
					slog.F("workspace_name", workspace.Name),
					slog.F("agent_name", agent.Name),
				)
			}
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}

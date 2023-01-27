package httpmw

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"github.com/coder/coder/coderd/authzquery"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/codersdk"
)

type workspaceAgentContextKey struct{}

// WorkspaceAgent returns the workspace agent from the ExtractAgent handler.
func WorkspaceAgent(r *http.Request) database.WorkspaceAgent {
	user, ok := r.Context().Value(workspaceAgentContextKey{}).(database.WorkspaceAgent)
	if !ok {
		panic("developer error: agent middleware not provided")
	}
	return user
}

// ExtractWorkspaceAgent requires authentication using a valid agent token.
func ExtractWorkspaceAgent(db database.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			systemCtx := authzquery.WithAuthorizeSystemContext(ctx, rbac.RolesAdminSystem())
			tokenValue := apiTokenFromRequest(r)
			if tokenValue == "" {
				httpapi.Write(ctx, rw, http.StatusUnauthorized, codersdk.Response{
					Message: fmt.Sprintf("Cookie %q must be provided.", codersdk.SessionTokenKey),
				})
				return
			}
			token, err := uuid.Parse(tokenValue)
			if err != nil {
				httpapi.Write(ctx, rw, http.StatusUnauthorized, codersdk.Response{
					Message: "Workspace agent token invalid.",
					Detail:  fmt.Sprintf("An agent token must be a valid UUIDv4. (len %d)", len(tokenValue)),
				})
				return
			}
			agent, err := db.GetWorkspaceAgentByAuthToken(systemCtx, token)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					httpapi.Write(ctx, rw, http.StatusUnauthorized, codersdk.Response{
						Message: "Workspace agent not authorized.",
						Detail:  "The agent cannot authenticate until the workspace provision job has been completed. If the job is no longer running, this agent is invalid.",
					})
					return
				}

				httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
					Message: "Internal error fetching workspace agent.",
					Detail:  err.Error(),
				})
				return
			}

			workspace, err := db.GetWorkspaceByAgentID(systemCtx, agent.ID)
			if err != nil {
				// TODO: details
				httpapi.Write(ctx, rw, http.StatusUnauthorized, codersdk.Response{
					Message: "Workspace agent not authorized.",
				})
				return
			}

			user, err := db.GetUserByID(systemCtx, workspace.OwnerID)
			if err != nil {
				// TODO: details
				httpapi.Write(ctx, rw, http.StatusUnauthorized, codersdk.Response{
					Message: "Workspace agent not authorized.",
				})
				return
			}

			roles, err := db.GetAuthorizationUserRoles(systemCtx, user.ID)
			if err != nil {
				// TODO: details
				httpapi.Write(ctx, rw, http.StatusUnauthorized, codersdk.Response{
					Message: "Workspace agent not authorized.",
				})
				return
			}

			subject := rbac.Subject{
				ID:     user.ID.String(),
				Roles:  rbac.RoleNames(roles.Roles),
				Groups: roles.Groups,
				Scope:  rbac.ScopeAll, // TODO: ScopeWorkspaceAgent
			}

			ctx = context.WithValue(ctx, workspaceAgentContextKey{}, agent)
			ctx = authzquery.WithAuthorizeContext(ctx, subject)
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}

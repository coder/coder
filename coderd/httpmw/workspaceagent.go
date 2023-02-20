package httpmw

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbauthz"
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
			tokenValue := apiTokenFromRequest(r)
			if tokenValue == "" {
				httpapi.Write(ctx, rw, http.StatusUnauthorized, codersdk.Response{
					Message: fmt.Sprintf("Cookie %q must be provided.", codersdk.SessionTokenCookie),
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
			//nolint:gocritic // System needs to be able to get workspace agents.
			agent, err := db.GetWorkspaceAgentByAuthToken(dbauthz.AsSystemRestricted(ctx), token)
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

			//nolint:gocritic // System needs to be able to get workspace agents.
			subject, err := getAgentSubject(dbauthz.AsSystemRestricted(ctx), db, agent)
			if err != nil {
				httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
					Message: "Internal error fetching workspace agent.",
					Detail:  err.Error(),
				})
				return
			}

			ctx = context.WithValue(ctx, workspaceAgentContextKey{}, agent)
			// Also set the dbauthz actor for the request.
			ctx = dbauthz.As(ctx, subject)
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}

func getAgentSubject(ctx context.Context, db database.Store, agent database.WorkspaceAgent) (rbac.Subject, error) {
	// TODO: make a different query that gets the workspace owner and roles along with the agent.
	workspace, err := db.GetWorkspaceByAgentID(ctx, agent.ID)
	if err != nil {
		return rbac.Subject{}, err
	}

	user, err := db.GetUserByID(ctx, workspace.OwnerID)
	if err != nil {
		return rbac.Subject{}, err
	}

	roles, err := db.GetAuthorizationUserRoles(ctx, user.ID)
	if err != nil {
		return rbac.Subject{}, err
	}

	// A user that creates a workspace can use this agent auth token and
	// impersonate the workspace. So to prevent privilege escalation, the
	// subject inherits the roles of the user that owns the workspace.
	// We then add a workspace-agent scope to limit the permissions
	// to only what the workspace agent needs.
	return rbac.Subject{
		ID:     user.ID.String(),
		Roles:  rbac.RoleNames(roles.Roles),
		Groups: roles.Groups,
		Scope:  rbac.WorkspaceAgentScope(workspace.ID, user.ID),
	}, nil
}

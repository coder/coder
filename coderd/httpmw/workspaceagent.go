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

func WorkspaceAgentOptional(r *http.Request) (database.WorkspaceAgent, bool) {
	user, ok := r.Context().Value(workspaceAgentContextKey{}).(database.WorkspaceAgent)
	return user, ok
}

// WorkspaceAgent returns the workspace agent from the ExtractAgent handler.
func WorkspaceAgent(r *http.Request) database.WorkspaceAgent {
	user, ok := WorkspaceAgentOptional(r)
	if !ok {
		panic("developer error: agent middleware not provided or was made optional")
	}
	return user
}

type ExtractWorkspaceAgentConfig struct {
	DB database.Store
	// Optional indicates whether the middleware should be optional.  If true, any
	// requests without the a token or with an invalid token will be allowed to
	// continue and no workspace agent will be set on the request context.
	Optional bool
}

// ExtractWorkspaceAgent requires authentication using a valid agent token.
func ExtractWorkspaceAgent(opts ExtractWorkspaceAgentConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			// optionalWrite wraps httpapi.Write but runs the next handler if the
			// token is optional.
			//
			// It should be used when the token is not provided or is invalid, but not
			// when there are other errors.
			optionalWrite := func(code int, response codersdk.Response) {
				if opts.Optional {
					next.ServeHTTP(rw, r)
					return
				}
				httpapi.Write(ctx, rw, code, response)
			}

			tokenValue := APITokenFromRequest(r)
			if tokenValue == "" {
				optionalWrite(http.StatusUnauthorized, codersdk.Response{
					Message: fmt.Sprintf("Cookie %q must be provided.", codersdk.SessionTokenCookie),
				})
				return
			}
			token, err := uuid.Parse(tokenValue)
			if err != nil {
				optionalWrite(http.StatusUnauthorized, codersdk.Response{
					Message: "Workspace agent token invalid.",
					Detail:  fmt.Sprintf("An agent token must be a valid UUIDv4. (len %d)", len(tokenValue)),
				})
				return
			}

			//nolint:gocritic // System needs to be able to get workspace agents.
			row, err := opts.DB.GetWorkspaceAgentAndOwnerByAuthToken(dbauthz.AsSystemRestricted(ctx), token)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					optionalWrite(http.StatusUnauthorized, codersdk.Response{
						Message: "Workspace agent not authorized.",
						Detail:  "The agent cannot authenticate until the workspace provision job has been completed. If the job is no longer running, this agent is invalid.",
					})
					return
				}

				httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
					Message: "Internal error authorizing workspace agent.",
					Detail:  err.Error(),
				})
				return
			}

			subject := rbac.Subject{
				ID:     row.OwnerID.String(),
				Roles:  rbac.RoleNames(row.OwnerRoles),
				Groups: row.OwnerGroups,
				// Note: this is generated as a NullUUID even though it shouldn't be nullable based on the query.
				Scope: rbac.WorkspaceAgentScope(row.WorkspaceID, row.OwnerID),
			}.WithCachedASTValue()

			ctx = context.WithValue(ctx, workspaceAgentContextKey{}, row.WorkspaceAgent)
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
	}.WithCachedASTValue(), nil
}

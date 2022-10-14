package httpmw

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
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
			cookieValue := apiTokenFromRequest(r)
			if cookieValue == "" {
				httpapi.Write(ctx, rw, http.StatusUnauthorized, codersdk.Response{
					Message: fmt.Sprintf("Cookie %q must be provided.", codersdk.SessionTokenKey),
				})
				return
			}
			token, err := uuid.Parse(cookieValue)
			if err != nil {
				httpapi.Write(ctx, rw, http.StatusUnauthorized, codersdk.Response{
					Message: "Agent token is invalid.",
				})
				return
			}
			agent, err := db.GetWorkspaceAgentByAuthToken(ctx, token)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					httpapi.Write(ctx, rw, http.StatusUnauthorized, codersdk.Response{
						Message: "Agent token is invalid.",
					})
					return
				}

				httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
					Message: "Internal error fetching workspace agent.",
					Detail:  err.Error(),
				})
				return
			}

			ctx = context.WithValue(ctx, workspaceAgentContextKey{}, agent)
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}

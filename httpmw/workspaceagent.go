package httpmw

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"github.com/coder/coder/database"
	"github.com/coder/coder/httpapi"
)

type workspaceAgentContextKey struct{}

// WorkspaceAgent returns the workspace agent from the ExtractWorkspaceAgent handler.
func WorkspaceAgent(r *http.Request) database.WorkspaceAgent {
	user, ok := r.Context().Value(workspaceAgentContextKey{}).(database.WorkspaceAgent)
	if !ok {
		panic("developer error: workspace agent middleware not provided")
	}
	return user
}

// ExtractWorkspaceAgent requires authentication using a valid agent token.
func ExtractWorkspaceAgent(db database.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(AuthCookie)
			if err != nil {
				httpapi.Write(rw, http.StatusUnauthorized, httpapi.Response{
					Message: fmt.Sprintf("%q cookie must be provided", AuthCookie),
				})
				return
			}
			workspaceAgent, err := db.GetWorkspaceAgentByToken(r.Context(), cookie.Value)
			if errors.Is(err, sql.ErrNoRows) {
				if errors.Is(err, sql.ErrNoRows) {
					httpapi.Write(rw, http.StatusUnauthorized, httpapi.Response{
						Message: "agent token is invalid",
					})
					return
				}
			}
			if err != nil {
				httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
					Message: fmt.Sprintf("get workspace agent: %s", err),
				})
				return
			}

			ctx := context.WithValue(r.Context(), workspaceAgentContextKey{}, workspaceAgent)
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}

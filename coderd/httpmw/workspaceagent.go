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
			cookie, err := r.Cookie(SessionTokenKey)
			if err != nil {
				httpapi.Write(rw, http.StatusUnauthorized, httpapi.Response{
					Message: fmt.Sprintf("%q cookie must be provided", SessionTokenKey),
				})
				return
			}
			token, err := uuid.Parse(cookie.Value)
			if err != nil {
				httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
					Message: fmt.Sprintf("parse token %q: %s", cookie.Value, err),
				})
				return
			}
			agent, err := db.GetWorkspaceAgentByAuthToken(r.Context(), token)
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

			ctx := context.WithValue(r.Context(), workspaceAgentContextKey{}, agent)
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}

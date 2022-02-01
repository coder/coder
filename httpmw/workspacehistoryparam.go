package httpmw

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/coder/coder/database"
	"github.com/coder/coder/httpapi"
)

type workspaceHistoryParamContextKey struct{}

// WorkspaceHistoryParam returns the workspace history from the ExtractWorkspaceHistoryParam handler.
func WorkspaceHistoryParam(r *http.Request) database.WorkspaceHistory {
	workspaceHistory, ok := r.Context().Value(workspaceHistoryParamContextKey{}).(database.WorkspaceHistory)
	if !ok {
		panic("developer error: workspace history param middleware not provided")
	}
	return workspaceHistory
}

// ExtractWorkspaceHistoryParam grabs workspace history from the "workspacehistory" URL parameter.
func ExtractWorkspaceHistoryParam(db database.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			workspace := WorkspaceParam(r)
			workspaceHistoryName := chi.URLParam(r, "workspacehistory")
			if workspaceHistoryName == "" {
				httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
					Message: "workspace history name must be provided",
				})
				return
			}
			workspaceHistory, err := db.GetWorkspaceHistoryByWorkspaceIDAndName(r.Context(), database.GetWorkspaceHistoryByWorkspaceIDAndNameParams{
				WorkspaceID: workspace.ID,
				Name:        workspaceHistoryName,
			})
			if errors.Is(err, sql.ErrNoRows) {
				httpapi.Write(rw, http.StatusNotFound, httpapi.Response{
					Message: fmt.Sprintf("workspace history %q does not exist", workspaceHistoryName),
				})
				return
			}
			if err != nil {
				httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
					Message: fmt.Sprintf("get workspace history: %s", err.Error()),
				})
				return
			}

			ctx := context.WithValue(r.Context(), workspaceHistoryParamContextKey{}, workspaceHistory)
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}

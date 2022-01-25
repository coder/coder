package httpmw

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi"

	"github.com/coder/coder/database"
	"github.com/coder/coder/httpapi"
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
			user := UserParam(r)
			workspaceName := chi.URLParam(r, "workspace")
			if workspaceName == "" {
				httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
					Message: "workspace id must be provided",
				})
				return
			}
			workspace, err := db.GetWorkspaceByUserIDAndName(r.Context(), database.GetWorkspaceByUserIDAndNameParams{
				OwnerID: user.ID,
				Name:    workspaceName,
			})
			if errors.Is(err, sql.ErrNoRows) {
				httpapi.Write(rw, http.StatusNotFound, httpapi.Response{
					Message: fmt.Sprintf("workspace %q does not exist", workspace),
				})
				return
			}
			if err != nil {
				httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
					Message: fmt.Sprintf("get workspace: %s", err.Error()),
				})
				return
			}

			ctx := context.WithValue(r.Context(), workspaceParamContextKey{}, workspace)
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}

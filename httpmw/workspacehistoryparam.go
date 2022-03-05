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

type workspaceBuildParamContextKey struct{}

// WorkspaceBuildParam returns the workspace build from the ExtractWorkspaceBuildParam handler.
func WorkspaceBuildParam(r *http.Request) database.WorkspaceBuild {
	workspaceBuild, ok := r.Context().Value(workspaceBuildParamContextKey{}).(database.WorkspaceBuild)
	if !ok {
		panic("developer error: workspace build param middleware not provided")
	}
	return workspaceBuild
}

// ExtractWorkspaceBuildParam grabs workspace build from the "workspacebuild" URL parameter.
func ExtractWorkspaceBuildParam(db database.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			workspace := WorkspaceParam(r)
			workspaceBuildName := chi.URLParam(r, "workspacebuild")
			if workspaceBuildName == "" {
				httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
					Message: "workspace build name must be provided",
				})
				return
			}
			var workspaceBuild database.WorkspaceBuild
			var err error
			if workspaceBuildName == "latest" {
				workspaceBuild, err = db.GetWorkspaceBuildByWorkspaceIDWithoutAfter(r.Context(), workspace.ID)
				if errors.Is(err, sql.ErrNoRows) {
					httpapi.Write(rw, http.StatusNotFound, httpapi.Response{
						Message: "there is no workspace build",
					})
					return
				}
			} else {
				workspaceBuild, err = db.GetWorkspaceBuildByWorkspaceIDAndName(r.Context(), database.GetWorkspaceBuildByWorkspaceIDAndNameParams{
					WorkspaceID: workspace.ID,
					Name:        workspaceBuildName,
				})
				if errors.Is(err, sql.ErrNoRows) {
					httpapi.Write(rw, http.StatusNotFound, httpapi.Response{
						Message: fmt.Sprintf("workspace build %q does not exist", workspaceBuildName),
					})
					return
				}
			}
			if err != nil {
				httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
					Message: fmt.Sprintf("get workspace build: %s", err.Error()),
				})
				return
			}

			ctx := context.WithValue(r.Context(), workspaceBuildParamContextKey{}, workspaceBuild)
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}

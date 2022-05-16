package httpmw

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
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
			workspaceID, parsed := parseUUID(rw, r, "workspace")
			if !parsed {
				return
			}
			workspace, err := db.GetWorkspaceByID(r.Context(), workspaceID)
			if errors.Is(err, sql.ErrNoRows) {
				httpapi.Write(rw, http.StatusNotFound, httpapi.Response{
					Message: fmt.Sprintf("workspace %q does not exist", workspaceID),
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

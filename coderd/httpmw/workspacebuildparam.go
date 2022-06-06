package httpmw

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
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
			workspaceBuildID, parsed := parseUUID(rw, r, "workspacebuild")
			if !parsed {
				return
			}
			workspaceBuild, err := db.GetWorkspaceBuildByID(r.Context(), workspaceBuildID)
			if errors.Is(err, sql.ErrNoRows) {
				httpapi.Write(rw, http.StatusNotFound, httpapi.Response{
					Message: fmt.Sprintf("Workspace build %q does not exist", workspaceBuildID),
				})
				return
			}
			if err != nil {
				httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
					Message: "Internal error fetching workspace build",
					Detail:  err.Error(),
				})
				return
			}

			ctx := context.WithValue(r.Context(), workspaceBuildParamContextKey{}, workspaceBuild)
			// This injects the "workspace" parameter, because it's expected the consumer
			// will want to use the Workspace middleware to ensure the caller owns the workspace.
			chi.RouteContext(ctx).URLParams.Add("workspace", workspaceBuild.WorkspaceID.String())
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}

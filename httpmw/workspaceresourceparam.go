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

type workspaceResourceParamContextKey struct{}

// ProvisionerJobParam returns the project from the ExtractProjectParam handler.
func WorkspaceResourceParam(r *http.Request) database.ProvisionerJobResource {
	resource, ok := r.Context().Value(workspaceResourceParamContextKey{}).(database.ProvisionerJobResource)
	if !ok {
		panic("developer error: workspace resource param middleware not provided")
	}
	return resource
}

// ExtractWorkspaceResourceParam grabs a workspace resource from the "provisionerjob" URL parameter.
func ExtractWorkspaceResourceParam(db database.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			resourceUUID, parsed := parseUUID(rw, r, "workspaceresource")
			if !parsed {
				return
			}
			resource, err := db.GetProvisionerJobResourceByID(r.Context(), resourceUUID)
			if errors.Is(err, sql.ErrNoRows) {
				httpapi.Write(rw, http.StatusNotFound, httpapi.Response{
					Message: "resource doesn't exist with that id",
				})
				return
			}
			if err != nil {
				httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
					Message: fmt.Sprintf("get provisioner resource: %s", err),
				})
				return
			}
			workspaceBuild := WorkspaceBuildParam(r)
			if workspaceBuild.ProvisionJobID.String() != resource.JobID.String() {
				httpapi.Write(rw, http.StatusUnauthorized, httpapi.Response{
					Message: "you don't own this resource",
				})
				return
			}

			ctx := context.WithValue(r.Context(), workspaceResourceParamContextKey{}, resource)
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}

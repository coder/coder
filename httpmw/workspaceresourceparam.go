package httpmw

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

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
			resourceID := chi.URLParam(r, "workspaceresource")
			if resourceID == "" {
				httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
					Message: "workspace resource must be provided",
				})
				return
			}
			resourceUUID, err := uuid.Parse(resourceID)
			if err != nil {
				httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
					Message: "resource id must be a uuid",
				})
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

			ctx := context.WithValue(r.Context(), workspaceResourceParamContextKey{}, resource)
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}

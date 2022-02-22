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

type workspaceResourceContextKey struct{}

// WorkspaceResource returns the workspace resource from the ExtractWorkspaceResource handler.
func WorkspaceResource(r *http.Request) database.WorkspaceResource {
	resource, ok := r.Context().Value(workspaceResourceContextKey{}).(database.WorkspaceResource)
	if !ok {
		panic("developer error: workspace resource middleware not provided")
	}
	return resource
}

// ExtractWorkspaceResource returns a workspace resource from the parameter provided.
func ExtractWorkspaceResource(db database.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			workspaceResourceID := chi.URLParam(r, "workspaceresource")
			if workspaceResourceID == "" {
				httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
					Message: "workspace resource id must be provided",
				})
				return
			}
			workspaceResourceUUID, err := uuid.Parse(workspaceResourceID)
			if err != nil {
				httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
					Message: "workspace resource must be a uuid",
				})
				return
			}
			workspaceResource, err := db.GetWorkspaceResourceByID(r.Context(), workspaceResourceUUID)
			if errors.Is(err, sql.ErrNoRows) {
				httpapi.Write(rw, http.StatusNotFound, httpapi.Response{
					Message: "no workspace resource exists with the id provided",
				})
				return
			}
			if err != nil {
				httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
					Message: fmt.Sprintf("get workspace resource: %s", err.Error()),
				})
				return
			}

			ctx := context.WithValue(r.Context(), workspaceResourceContextKey{}, workspaceResource)
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}

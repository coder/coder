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

type projectParamContextKey struct{}

// ProjectParam returns the project from the ExtractProjectParam handler.
func ProjectParam(r *http.Request) database.Project {
	project, ok := r.Context().Value(projectParamContextKey{}).(database.Project)
	if !ok {
		panic("developer error: project param middleware not provided")
	}
	return project
}

// ExtractProjectParam grabs a project from the "project" URL parameter.
func ExtractProjectParam(db database.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			projectID, parsed := parseUUID(rw, r, "project")
			if !parsed {
				return
			}
			project, err := db.GetProjectByID(r.Context(), projectID)
			if errors.Is(err, sql.ErrNoRows) {
				httpapi.Write(rw, http.StatusNotFound, httpapi.Response{
					Message: fmt.Sprintf("project %q does not exist", projectID),
				})
			}
			if err != nil {
				httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
					Message: fmt.Sprintf("get project: %s", err),
				})
				return
			}

			ctx := context.WithValue(r.Context(), projectParamContextKey{}, project)
			chi.RouteContext(ctx).URLParams.Add("organization", project.OrganizationID)
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}

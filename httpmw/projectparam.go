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

type projectParamContextKey struct{}

// ProjectParam returns the project from the ExtractProjectParameter handler.
func ProjectParam(r *http.Request) database.Project {
	project, ok := r.Context().Value(projectParamContextKey{}).(database.Project)
	if !ok {
		panic("developer error: project param middleware not provided")
	}
	return project
}

// ExtractProjectParameter grabs a project from the "project" URL parameter.
func ExtractProjectParameter(db database.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			organization := OrganizationParam(r)
			projectName := chi.URLParam(r, "project")
			if projectName == "" {
				httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
					Message: "project name must be provided",
				})
				return
			}
			project, err := db.GetProjectByOrganizationAndName(r.Context(), database.GetProjectByOrganizationAndNameParams{
				OrganizationID: organization.ID,
				Name:           projectName,
			})
			if errors.Is(err, sql.ErrNoRows) {
				httpapi.Write(rw, http.StatusNotFound, httpapi.Response{
					Message: fmt.Sprintf("project %q does not exist", projectName),
				})
				return
			}
			if err != nil {
				httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
					Message: fmt.Sprintf("get project: %s", err.Error()),
				})
				return
			}

			ctx := context.WithValue(r.Context(), projectParamContextKey{}, project)
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}

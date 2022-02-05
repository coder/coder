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

type projectVersionParamContextKey struct{}

// ProjectVersionParam returns the project version from the ExtractProjectVersionParam handler.
func ProjectVersionParam(r *http.Request) database.ProjectVersion {
	projectVersion, ok := r.Context().Value(projectVersionParamContextKey{}).(database.ProjectVersion)
	if !ok {
		panic("developer error: project version param middleware not provided")
	}
	return projectVersion
}

// ExtractProjectVersionParam grabs project version from the "projectversion" URL parameter.
func ExtractProjectVersionParam(db database.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			project := ProjectParam(r)
			projectVersionName := chi.URLParam(r, "projectversion")
			if projectVersionName == "" {
				httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
					Message: "project version name must be provided",
				})
				return
			}
			projectVersion, err := db.GetProjectVersionByProjectIDAndName(r.Context(), database.GetProjectVersionByProjectIDAndNameParams{
				ProjectID: project.ID,
				Name:      projectVersionName,
			})
			if errors.Is(err, sql.ErrNoRows) {
				httpapi.Write(rw, http.StatusNotFound, httpapi.Response{
					Message: fmt.Sprintf("project version %q does not exist", projectVersionName),
				})
				return
			}
			if err != nil {
				httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
					Message: fmt.Sprintf("get project version: %s", err.Error()),
				})
				return
			}

			ctx := context.WithValue(r.Context(), projectVersionParamContextKey{}, projectVersion)
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}

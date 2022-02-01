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

type projectHistoryParamContextKey struct{}

// ProjectHistoryParam returns the project history from the ExtractProjectHistoryParam handler.
func ProjectHistoryParam(r *http.Request) database.ProjectHistory {
	projectHistory, ok := r.Context().Value(projectHistoryParamContextKey{}).(database.ProjectHistory)
	if !ok {
		panic("developer error: project history param middleware not provided")
	}
	return projectHistory
}

// ExtractProjectHistoryParam grabs project history from the "projecthistory" URL parameter.
func ExtractProjectHistoryParam(db database.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			project := ProjectParam(r)
			projectHistoryName := chi.URLParam(r, "projecthistory")
			if projectHistoryName == "" {
				httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
					Message: "project history name must be provided",
				})
				return
			}
			projectHistory, err := db.GetProjectHistoryByProjectIDAndName(r.Context(), database.GetProjectHistoryByProjectIDAndNameParams{
				ProjectID: project.ID,
				Name:      projectHistoryName,
			})
			if errors.Is(err, sql.ErrNoRows) {
				httpapi.Write(rw, http.StatusNotFound, httpapi.Response{
					Message: fmt.Sprintf("project history %q does not exist", projectHistoryName),
				})
				return
			}
			if err != nil {
				httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
					Message: fmt.Sprintf("get project history: %s", err.Error()),
				})
				return
			}

			ctx := context.WithValue(r.Context(), projectHistoryParamContextKey{}, projectHistory)
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}

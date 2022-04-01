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
			projectVersionID, parsed := parseUUID(rw, r, "projectversion")
			if !parsed {
				return
			}
			projectVersion, err := db.GetProjectVersionByID(r.Context(), projectVersionID)
			if errors.Is(err, sql.ErrNoRows) {
				httpapi.Write(rw, http.StatusNotFound, httpapi.Response{
					Message: fmt.Sprintf("project version %q does not exist", projectVersionID),
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
			chi.RouteContext(ctx).URLParams.Add("organization", projectVersion.OrganizationID.String())
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}

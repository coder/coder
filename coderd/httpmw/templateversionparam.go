package httpmw

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/codersdk"
)

type templateVersionParamContextKey struct{}

// TemplateVersionParam returns the template version from the ExtractTemplateVersionParam handler.
func TemplateVersionParam(r *http.Request) database.TemplateVersion {
	templateVersion, ok := r.Context().Value(templateVersionParamContextKey{}).(database.TemplateVersion)
	if !ok {
		panic("developer error: template version param middleware not provided")
	}
	return templateVersion
}

// ExtractTemplateVersionParam grabs template version from the "templateversion" URL parameter.
func ExtractTemplateVersionParam(db database.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			templateVersionID, parsed := parseUUID(rw, r, "templateversion")
			if !parsed {
				return
			}
			templateVersion, err := db.GetTemplateVersionByID(r.Context(), templateVersionID)
			if errors.Is(err, sql.ErrNoRows) {
				httpapi.ResourceNotFound(rw)
				return
			}
			if err != nil {
				httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
					Message: "Internal error fetching template version.",
					Detail:  err.Error(),
				})
				return
			}

			ctx := context.WithValue(r.Context(), templateVersionParamContextKey{}, templateVersion)
			chi.RouteContext(ctx).URLParams.Add("organization", templateVersion.OrganizationID.String())
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}

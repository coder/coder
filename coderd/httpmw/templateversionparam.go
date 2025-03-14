package httpmw

import (
	"errors"
	"context"
	"database/sql"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"

	"github.com/coder/coder/v2/codersdk"
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
			ctx := r.Context()

			templateVersionID, parsed := ParseUUIDParam(rw, r, "templateversion")
			if !parsed {
				return
			}
			templateVersion, err := db.GetTemplateVersionByID(ctx, templateVersionID)
			if httpapi.Is404Error(err) {
				httpapi.ResourceNotFound(rw)
				return
			}
			if err != nil {
				httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
					Message: "Internal error fetching template version.",
					Detail:  err.Error(),
				})
				return
			}
			template, err := db.GetTemplateByID(r.Context(), templateVersion.TemplateID.UUID)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
					Message: "Internal error fetching template.",
					Detail:  err.Error(),
				})

				return
			}
			ctx = context.WithValue(ctx, templateVersionParamContextKey{}, templateVersion)
			chi.RouteContext(ctx).URLParams.Add("organization", templateVersion.OrganizationID.String())
			ctx = context.WithValue(ctx, templateParamContextKey{}, template)
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}

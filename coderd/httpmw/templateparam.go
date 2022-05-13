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

type templateParamContextKey struct{}

// TemplateParam returns the template from the ExtractTemplateParam handler.
func TemplateParam(r *http.Request) database.Template {
	template, ok := r.Context().Value(templateParamContextKey{}).(database.Template)
	if !ok {
		panic("developer error: template param middleware not provided")
	}
	return template
}

// ExtractTemplateParam grabs a template from the "template" URL parameter.
func ExtractTemplateParam(db database.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			templateID, parsed := parseUUID(rw, r, "template")
			if !parsed {
				return
			}
			template, err := db.GetTemplateByID(r.Context(), templateID)
			if errors.Is(err, sql.ErrNoRows) {
				httpapi.Write(rw, http.StatusNotFound, httpapi.Response{
					Message: fmt.Sprintf("template %q does not exist", templateID),
				})
			}
			if err != nil {
				httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
					Message: fmt.Sprintf("get template: %s", err),
				})
				return
			}

			if template.Deleted {
				httpapi.Write(rw, http.StatusNotFound, httpapi.Response{
					Message: fmt.Sprintf("template %q does not exist", templateID),
				})
				return
			}

			ctx := context.WithValue(r.Context(), templateParamContextKey{}, template)
			chi.RouteContext(ctx).URLParams.Add("organization", template.OrganizationID.String())
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}

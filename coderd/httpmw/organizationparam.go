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

type organizationParamContextKey struct{}
type organizationMemberParamContextKey struct{}

// OrganizationParam returns the organization from the ExtractOrganizationParam handler.
func OrganizationParam(r *http.Request) database.Organization {
	organization, ok := r.Context().Value(organizationParamContextKey{}).(database.Organization)
	if !ok {
		panic("developer error: organization param middleware not provided")
	}
	return organization
}

// OrganizationMemberParam returns the organization membership that allowed the query
// from the ExtractOrganizationParam handler.
func OrganizationMemberParam(r *http.Request) database.OrganizationMember {
	organizationMember, ok := r.Context().Value(organizationMemberParamContextKey{}).(database.OrganizationMember)
	if !ok {
		panic("developer error: organization param middleware not provided")
	}
	return organizationMember
}

// ExtractOrganizationParam grabs an organization and user membership from the "organization" URL parameter.
// This middleware requires the API key middleware higher in the call stack for authentication.
func ExtractOrganizationParam(db database.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			organizationID := chi.URLParam(r, "organization")
			if organizationID == "" {
				httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
					Message: "organization must be provided",
				})
				return
			}
			organization, err := db.GetOrganizationByID(r.Context(), organizationID)
			if errors.Is(err, sql.ErrNoRows) {
				httpapi.Write(rw, http.StatusNotFound, httpapi.Response{
					Message: fmt.Sprintf("organization %q does not exist", organizationID),
				})
				return
			}
			if err != nil {
				httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
					Message: fmt.Sprintf("get organization: %s", err.Error()),
				})
				return
			}
			apiKey := APIKey(r)
			organizationMember, err := db.GetOrganizationMemberByUserID(r.Context(), database.GetOrganizationMemberByUserIDParams{
				OrganizationID: organization.ID,
				UserID:         apiKey.UserID,
			})
			if errors.Is(err, sql.ErrNoRows) {
				httpapi.Write(rw, http.StatusUnauthorized, httpapi.Response{
					Message: "not a member of the organization",
				})
				return
			}
			if err != nil {
				httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
					Message: fmt.Sprintf("get organization member: %s", err.Error()),
				})
				return
			}

			ctx := context.WithValue(r.Context(), organizationParamContextKey{}, organization)
			ctx = context.WithValue(ctx, organizationMemberParamContextKey{}, organizationMember)
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}

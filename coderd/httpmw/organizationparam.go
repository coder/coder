package httpmw

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/codersdk"
)

type (
	organizationParamContextKey       struct{}
	organizationMemberParamContextKey struct{}
)

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
		panic("developer error: organization member param middleware not provided")
	}
	return organizationMember
}

// ExtractOrganizationParam grabs an organization from the "organization" URL parameter.
// This middleware requires the API key middleware higher in the call stack for authentication.
func ExtractOrganizationParam(db database.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			orgID, ok := parseUUID(rw, r, "organization")
			if !ok {
				return
			}

			organization, err := db.GetOrganizationByID(ctx, orgID)
			if errors.Is(err, sql.ErrNoRows) {
				httpapi.ResourceNotFound(rw)
				return
			}
			if err != nil {
				httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
					Message: "Internal error fetching organization.",
					Detail:  err.Error(),
				})
				return
			}
			ctx = context.WithValue(ctx, organizationParamContextKey{}, organization)
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}

// ExtractOrganizationMemberParam grabs a user membership from the "organization" and "user" URL parameter.
// This middleware requires the ExtractUser and ExtractOrganization middleware higher in the stack
func ExtractOrganizationMemberParam(db database.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			organization := OrganizationParam(r)
			user := UserParam(r)

			organizationMember, err := db.GetOrganizationMemberByUserID(ctx, database.GetOrganizationMemberByUserIDParams{
				OrganizationID: organization.ID,
				UserID:         user.ID,
			})
			if errors.Is(err, sql.ErrNoRows) {
				httpapi.ResourceNotFound(rw)
				return
			}
			if err != nil {
				httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
					Message: "Internal error fetching organization member.",
					Detail:  err.Error(),
				})
				return
			}

			ctx = context.WithValue(ctx, organizationMemberParamContextKey{}, organizationMember)
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}

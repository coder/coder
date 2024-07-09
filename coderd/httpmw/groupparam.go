package httpmw

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

type groupParamContextKey struct{}

// GroupParam returns the group extracted via the ExtraGroupParam middleware.
func GroupParam(r *http.Request) database.Group {
	group, ok := r.Context().Value(groupParamContextKey{}).(database.Group)
	if !ok {
		panic("developer error: group param middleware not provided")
	}
	return group
}

func ExtractGroupByNameParam(db database.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			var (
				ctx = r.Context()
				org = OrganizationParam(r)
			)

			name := r.PathValue("groupName")
			if name == "" {
				httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
					Message: "Missing group name in URL",
				})
				return
			}

			group, err := db.GetGroupByOrgAndName(ctx, database.GetGroupByOrgAndNameParams{
				OrganizationID: org.ID,
				Name:           name,
			})
			if httpapi.Is404Error(err) {
				httpapi.ResourceNotFound(rw)
				return
			}
			if err != nil {
				httpapi.InternalServerError(rw, err)
				return
			}

			ctx = context.WithValue(ctx, groupParamContextKey{}, group)
			chi.RouteContext(ctx).URLParams.Add("organization", group.OrganizationID.String())
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}

// ExtraGroupParam grabs a group from the "group" URL parameter.
func ExtractGroupParam(db database.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			groupID, parsed := ParseUUIDParam(rw, r, "group")
			if !parsed {
				return
			}

			group, err := db.GetGroupByID(r.Context(), groupID)
			if httpapi.Is404Error(err) {
				httpapi.ResourceNotFound(rw)
				return
			}
			if err != nil {
				httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
					Message: "Internal error fetching group.",
					Detail:  err.Error(),
				})
				return
			}

			ctx = context.WithValue(ctx, groupParamContextKey{}, group)
			chi.RouteContext(ctx).URLParams.Add("organization", group.OrganizationID.String())
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}

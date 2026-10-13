package httpmw

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
)

type organizationSkillParamContextKey struct{}

// OrganizationSkillParam returns the organization skill from the
// ExtractOrganizationSkillParam handler.
func OrganizationSkillParam(r *http.Request) database.Skill {
	skill, ok := r.Context().Value(organizationSkillParamContextKey{}).(database.Skill)
	if !ok {
		panic("developer error: organization skill param middleware not provided")
	}
	return skill
}

// ExtractOrganizationSkillParam reads the "skillName" URL parameter within the
// organization from ExtractOrganizationParam. Callers with none of the
// admitted actions are concealed as not found, so denied and missing rows both
// return 404.
func ExtractOrganizationSkillParam(
	db database.Store,
	auth func(r *http.Request, action policy.Action, object rbac.Objecter) bool,
	actions ...policy.Action,
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			// Authorization follows the raw lookup because mutation-only callers
			// may lack the read access enforced by the database wrapper.
			//nolint:gocritic // The explicit action checks below own authorization.
			skill, err := db.GetOrganizationSkillByOrganizationIDAndName(dbauthz.AsSystemRestricted(ctx), database.GetOrganizationSkillByOrganizationIDAndNameParams{
				OrganizationID: OrganizationParam(r).ID,
				Name:           chi.URLParam(r, "skillName"),
			})
			if httpapi.Is404Error(err) {
				httpapi.ResourceNotFound(rw)
				return
			}
			if err != nil {
				httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
					Message: "Internal error fetching organization skill.",
					Detail:  err.Error(),
				})
				return
			}
			admitted := false
			for _, action := range actions {
				if auth(r, action, skill) {
					admitted = true
					break
				}
			}
			if !admitted {
				httpapi.ResourceNotFound(rw)
				return
			}

			ctx = context.WithValue(ctx, organizationSkillParamContextKey{}, skill)
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}

package coderd

import (
	"net/http"

	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/codersdk"

	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/rbac"
)

// assignableSiteRoles returns all site wide roles that can be assigned.
func (*api) assignableSiteRoles(rw http.ResponseWriter, _ *http.Request) {
	// TODO: @emyrk in the future, allow granular subsets of roles to be returned based on the
	// 	role of the user.
	roles := rbac.SiteRoles()
	httpapi.Write(rw, http.StatusOK, codersdk.ConvertRoles(roles))
}

// assignableSiteRoles returns all site wide roles that can be assigned.
func (*api) assignableOrgRoles(rw http.ResponseWriter, r *http.Request) {
	// TODO: @emyrk in the future, allow granular subsets of roles to be returned based on the
	// 	role of the user.
	organization := httpmw.OrganizationParam(r)
	roles := rbac.OrganizationRoles(organization.ID)
	httpapi.Write(rw, http.StatusOK, codersdk.ConvertRoles(roles))
}

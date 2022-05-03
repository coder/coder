package coderd

import (
	"net/http"

	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/rbac"
)

// assignableSiteRoles returns all site wide roles that can be assigned.
func (api *api) assignableSiteRoles(rw http.ResponseWriter, r *http.Request) {
	// TODO: @emyrk in the future, allow granular subsets of roles to be returned based on the
	// 	role of the user.
	roles := rbac.ListSiteRoles()
	httpapi.Write(rw, http.StatusOK, roles)
}

// assignableSiteRoles returns all site wide roles that can be assigned.
func (api *api) assignableOrgRoles(rw http.ResponseWriter, r *http.Request) {
	// TODO: @emyrk in the future, allow granular subsets of roles to be returned based on the
	// 	role of the user.
	roles := rbac.ListSiteRoles()
	httpapi.Write(rw, http.StatusOK, roles)
}

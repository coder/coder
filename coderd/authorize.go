package coderd

import (
	"net/http"

	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/rbac"
)

func (api *api) Authorize(rw http.ResponseWriter, r *http.Request, action rbac.Action, object rbac.Object) bool {
	roles := httpmw.UserRoles(r)
	err := api.Authorizer.ByRoleName(r.Context(), roles.ID.String(), roles.Roles, action, object)
	if err != nil {
		httpapi.Write(rw, http.StatusUnauthorized, httpapi.Response{
			Message: err.Error(),
		})
		return false
	}
	return true
}

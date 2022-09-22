package coderd

import (
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/codersdk"
)

func (api *API) postGroupByOrganization(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx = r.Context()
		org = httpmw.OrganizationParam(r)
	)

	// if api.Authorize(r, rbac.ActionCreate, rbac.ResourceGroup.InOrg(org.ID)) {
	// 	http.NotFound(rw, r)
	// 	return
	// }

	var req codersdk.CreateGroupRequest
	if !httpapi.Read(rw, r, &req) {
		return
	}

	group, err := api.Database.InsertGroup(ctx, database.InsertGroupParams{
		ID:             uuid.New(),
		Name:           req.Name,
		OrganizationID: org.ID,
	})
	if database.IsUniqueViolation(err) {
		httpapi.Write(rw, http.StatusConflict, codersdk.Response{
			Message: fmt.Sprintf("Group with name %q already exists.", req.Name),
		})
		return
	}
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(rw, http.StatusCreated, group)
}

func (api *API) patchGroup(rw http.ResponseWriter, r *http.Request) {

}

func (api *API) deleteGroup(rw http.ResponseWriter, r *http.Request) {

}

func (api *API) group(rw http.ResponseWriter, r *http.Request) {

}

func (api *API) groups(rw http.ResponseWriter, r *http.Request) {

}

func convertGroup(g database.Group) codersdk.Group {
	return codersdk.Group{
		ID:             g.ID,
		Name:           g.Name,
		OrganizationID: g.OrganizationID,
	}
}

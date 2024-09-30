package coderd

import (
	"net/http"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary Get organizations
// @ID get-organizations
// @Security CoderSessionToken
// @Produce json
// @Tags Organizations
// @Success 200 {object} []codersdk.Organization
// @Router /organizations [get]
func (api *API) organizations(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	organizations, err := api.Database.GetOrganizations(ctx, database.GetOrganizationsParams{})
	if httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching organizations.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.List(organizations, db2sdk.Organization))
}

// @Summary Get organization by ID
// @ID get-organization-by-id
// @Security CoderSessionToken
// @Produce json
// @Tags Organizations
// @Param organization path string true "Organization ID" format(uuid)
// @Success 200 {object} codersdk.Organization
// @Router /organizations/{organization} [get]
func (*API) organization(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	organization := httpmw.OrganizationParam(r)

	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.Organization(organization))
}

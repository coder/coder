package coderd

import (
	"net/http"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/codersdk"
)

func (api *API) organization(rw http.ResponseWriter, r *http.Request) {
	organization := httpmw.OrganizationParam(r)

	if !api.Authorize(rw, r, rbac.ActionRead, rbac.ResourceOrganization.
		InOrg(organization.ID).
		WithID(organization.ID.String())) {
		return
	}

	httpapi.Write(rw, http.StatusOK, convertOrganization(organization))
}

// convertOrganization consumes the database representation and outputs an API friendly representation.
func convertOrganization(organization database.Organization) codersdk.Organization {
	return codersdk.Organization{
		ID:        organization.ID,
		Name:      organization.Name,
		CreatedAt: organization.CreatedAt,
		UpdatedAt: organization.UpdatedAt,
	}
}

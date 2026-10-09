package coderd

import (
	"net/http"

	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
)

// @Summary List organization skills
// @ID list-organization-skills
// @Security CoderSessionToken
// @Produce json
// @Tags Organizations
// @Param organization path string true "Organization ID or name"
// @Success 200 {array} codersdk.SkillMetadata
// @Router /api/experimental/organizations/{organization}/skills [get]
// @x-apidocgen {"skip": true}
func (api *API) getOrganizationSkills(rw http.ResponseWriter, r *http.Request) { //nolint:revive // Method name matches route.
	ctx := r.Context()
	organization := httpmw.OrganizationParam(r)

	rows, err := api.Database.ListOrganizationSkillMetadataByOrganizationID(ctx, organization.ID)
	if err != nil {
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.OrganizationSkillMetadataList(rows))
}

// @Summary Get an organization skill by name
// @ID get-an-organization-skill-by-name
// @Security CoderSessionToken
// @Produce json
// @Tags Organizations
// @Param organization path string true "Organization ID or name"
// @Param skillName path string true "Skill name"
// @Success 200 {object} codersdk.Skill
// @Router /api/experimental/organizations/{organization}/skills/{skillName} [get]
// @x-apidocgen {"skip": true}
func (api *API) getOrganizationSkill(rw http.ResponseWriter, r *http.Request) { //nolint:revive // Method name matches route.
	httpapi.Write(r.Context(), rw, http.StatusOK, db2sdk.Skill(httpmw.OrganizationSkillParam(r)))
}

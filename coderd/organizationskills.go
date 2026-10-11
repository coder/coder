package coderd

import (
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/x/skills"
	"github.com/coder/coder/v2/codersdk"
)

const (
	// skillsPerOrganizationLimitConstraint is raised by the skill cap trigger
	// with USING CONSTRAINT, so dbgen does not emit it.
	skillsPerOrganizationLimitConstraint database.CheckConstraint = "skills_per_organization_limit"
	// maxSkillsPerOrganization matches the cap trigger, which applies one
	// limit to every owner kind.
	maxSkillsPerOrganization = skills.MaxPersonalSkillsPerUser
)

// @Summary Create an organization skill
// @ID create-an-organization-skill
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Organizations
// @Param organization path string true "Organization ID or name"
// @Param request body codersdk.CreateSkillRequest true "Create organization skill request"
// @Success 201 {object} codersdk.Skill
// @Router /api/experimental/organizations/{organization}/skills [post]
// @x-apidocgen {"skip": true}
func (api *API) postOrganizationSkill(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		organization      = httpmw.OrganizationParam(r)
		auditor           = api.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.AuditableOrganizationSkill](rw, &audit.RequestParams{
			Audit:          *auditor,
			Log:            api.Logger,
			Request:        r,
			Action:         database.AuditActionCreate,
			OrganizationID: organization.ID,
		})
	)
	defer commitAudit()
	if !api.Authorize(r, policy.ActionCreate, rbac.ResourceOrganizationSkill.InOrg(organization.ID)) {
		httpapi.Write(ctx, rw, http.StatusForbidden, organizationSkillCreateErrors.forbidden)
		return
	}

	skill, ok := createSkill(ctx, rw, r, organizationSkillCreateErrors, func(parsed skills.ParsedSkill, content string) (database.Skill, error) {
		return api.Database.InsertOrganizationSkill(ctx, database.InsertOrganizationSkillParams{
			ID:             uuid.New(),
			OrganizationID: organization.ID,
			Name:           parsed.Name,
			Description:    parsed.Description,
			Content:        content,
			// The Everyone group shares the organization's ID, so new skills
			// reach every member until an admin narrows the ACL.
			GroupACL: database.ChatACL{
				organization.ID.String(): {Permissions: []policy.Action{policy.ActionRead}},
			},
			UserACL: database.ChatACL{},
		})
	})
	if !ok {
		return
	}
	aReq.New = database.AuditableOrganizationSkill{Skill: skill}
}

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

// @Summary Update an organization skill
// @ID update-an-organization-skill
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Organizations
// @Param organization path string true "Organization ID or name"
// @Param skillName path string true "Skill name"
// @Param request body codersdk.UpdateSkillRequest true "Update organization skill request"
// @Success 200 {object} codersdk.Skill
// @Router /api/experimental/organizations/{organization}/skills/{skillName} [patch]
// @x-apidocgen {"skip": true}
func (api *API) patchOrganizationSkill(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		oldSkill          = httpmw.OrganizationSkillParam(r)
		auditor           = api.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.AuditableOrganizationSkill](rw, &audit.RequestParams{
			Audit:          *auditor,
			Log:            api.Logger,
			Request:        r,
			Action:         database.AuditActionWrite,
			OrganizationID: oldSkill.OrganizationID.UUID,
		})
	)
	defer commitAudit()
	aReq.Old = database.AuditableOrganizationSkill{Skill: oldSkill}

	current, skill, ok := api.updateSkill(ctx, rw, r, oldSkill.Name, organizationSkillUpdateErrors, func(tx database.Store, update skillUpdate) (database.Skill, database.Skill, error) {
		//nolint:gocritic // The update below reauthorizes the locked row for update.
		locked, err := tx.GetOrganizationSkillByIDForUpdate(dbauthz.AsSystemRestricted(ctx), oldSkill.ID)
		if err != nil {
			return database.Skill{}, database.Skill{}, xerrors.Errorf("lock organization skill: %w", err)
		}
		updated, err := tx.UpdateOrganizationSkillByOrganizationIDAndName(ctx, database.UpdateOrganizationSkillByOrganizationIDAndNameParams{
			Description:    update.Description,
			Content:        update.Content,
			Enabled:        update.Enabled,
			OrganizationID: locked.OrganizationID.UUID,
			Name:           locked.Name,
		})
		if err != nil {
			return database.Skill{}, database.Skill{}, xerrors.Errorf("update organization skill: %w", err)
		}
		return locked, updated, nil
	})
	if !ok {
		return
	}
	// The audit diff starts from the locked row, not the middleware's read.
	aReq.Old = database.AuditableOrganizationSkill{Skill: current}
	aReq.New = database.AuditableOrganizationSkill{Skill: skill}
}

// @Summary Delete an organization skill
// @ID delete-an-organization-skill
// @Security CoderSessionToken
// @Tags Organizations
// @Param organization path string true "Organization ID or name"
// @Param skillName path string true "Skill name"
// @Success 204
// @Router /api/experimental/organizations/{organization}/skills/{skillName} [delete]
// @x-apidocgen {"skip": true}
func (api *API) deleteOrganizationSkill(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		skill             = httpmw.OrganizationSkillParam(r)
		auditor           = api.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.AuditableOrganizationSkill](rw, &audit.RequestParams{
			Audit:          *auditor,
			Log:            api.Logger,
			Request:        r,
			Action:         database.AuditActionDelete,
			OrganizationID: skill.OrganizationID.UUID,
		})
	)
	defer commitAudit()
	aReq.Old = database.AuditableOrganizationSkill{Skill: skill}

	deleted, err := api.Database.DeleteOrganizationSkillByOrganizationIDAndName(ctx, database.DeleteOrganizationSkillByOrganizationIDAndNameParams{
		OrganizationID: skill.OrganizationID.UUID,
		Name:           skill.Name,
	})
	if err != nil {
		switch {
		case httpapi.IsUnauthorizedError(err):
			httpapi.Write(ctx, rw, http.StatusForbidden, organizationSkillForbidden("delete this organization skill"))
		case httpapi.Is404Error(err):
			httpapi.ResourceNotFound(rw)
		default:
			httpapi.InternalServerError(rw, err)
		}
		return
	}
	aReq.Old = database.AuditableOrganizationSkill{Skill: deleted}

	rw.WriteHeader(http.StatusNoContent)
}

var (
	// These replace httpapi.Forbidden, whose detail says the caller cannot
	// view content these callers may be able to read.
	organizationSkillCreateErrors = skillWriteErrors{
		forbidden: organizationSkillForbidden("create organization skills"),
		limit:     skillsPerOrganizationLimitConstraint,
		limitReached: codersdk.Response{
			Message: "Organization skill limit reached.",
			Detail:  fmt.Sprintf("Each organization can have at most %d skills.", maxSkillsPerOrganization),
		},
		nameIndex: database.UniqueSkillsOrganizationIDNameIndex,
	}
	organizationSkillUpdateErrors = skillWriteErrors{
		forbidden: organizationSkillForbidden("update this organization skill"),
	}
)

func organizationSkillForbidden(action string) codersdk.Response {
	return codersdk.Response{Message: fmt.Sprintf("You don't have permission to %s.", action)}
}

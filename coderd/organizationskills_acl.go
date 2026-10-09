package coderd

import (
	"maps"
	"net/http"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac/acl"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary Get an organization skill ACL
// @ID get-an-organization-skill-acl
// @Security CoderSessionToken
// @Produce json
// @Tags Organizations
// @Param organization path string true "Organization ID or name"
// @Param skillName path string true "Skill name"
// @Success 200 {object} codersdk.OrganizationSkillACL
// @Router /api/experimental/organizations/{organization}/skills/{skillName}/acl [get]
// @x-apidocgen {"skip": true}
func (api *API) getOrganizationSkillACL(rw http.ResponseWriter, r *http.Request) { //nolint:revive // Method name matches route.
	ctx := r.Context()
	skill := httpmw.OrganizationSkillParam(r)

	users, err := resolveACLUsers(ctx, api.Database, skill.UserACL, func(user codersdk.MinimalUser) codersdk.OrganizationSkillUser {
		return codersdk.OrganizationSkillUser{MinimalUser: user, Role: codersdk.OrganizationSkillRoleRead}
	})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	groups, err := resolveACLGroups(ctx, api.Database, skill.GroupACL, func(group codersdk.Group) codersdk.OrganizationSkillGroup {
		return codersdk.OrganizationSkillGroup{Group: group, Role: codersdk.OrganizationSkillRoleRead}
	})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, codersdk.OrganizationSkillACL{
		Users:  users,
		Groups: groups,
	})
}

// @Summary Get available organization skill ACL users and groups
// @ID get-available-organization-skill-acl-users-and-groups
// @Security CoderSessionToken
// @Produce json
// @Tags Organizations
// @Param organization path string true "Organization ID or name"
// @Param skillName path string true "Skill name"
// @Param q query string false "User search query; free-text search also applies to groups"
// @Param after_id query string false "User after ID" format(uuid)
// @Param limit query int false "Page limit for users and groups, if 0 returns all candidates"
// @Param offset query int false "User page offset"
// @Success 200 {object} codersdk.ACLAvailable
// @Router /api/experimental/organizations/{organization}/skills/{skillName}/acl/available [get]
// @x-apidocgen {"skip": true}
func (api *API) getOrganizationSkillACLAvailable(rw http.ResponseWriter, r *http.Request) { //nolint:revive // Method name matches route.
	api.writeOrganizationACLAvailable(rw, r, httpmw.OrganizationSkillParam(r).OrganizationID.UUID)
}

// @Summary Update an organization skill ACL
// @ID update-an-organization-skill-acl
// @Security CoderSessionToken
// @Accept json
// @Tags Organizations
// @Param organization path string true "Organization ID or name"
// @Param skillName path string true "Skill name"
// @Param request body codersdk.UpdateOrganizationSkillACLRequest true "Update organization skill ACL request"
// @Success 204
// @Router /api/experimental/organizations/{organization}/skills/{skillName}/acl [patch]
// @x-apidocgen {"skip": true}
func (api *API) patchOrganizationSkillACL(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		skill             = httpmw.OrganizationSkillParam(r)
		organizationID    = skill.OrganizationID.UUID
		auditor           = api.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.AuditableOrganizationSkill](rw, &audit.RequestParams{
			Audit:          *auditor,
			Log:            api.Logger,
			Request:        r,
			Action:         database.AuditActionWrite,
			OrganizationID: organizationID,
		})
	)
	defer commitAudit()
	aReq.Old = database.AuditableOrganizationSkill{Skill: skill}

	var req codersdk.UpdateOrganizationSkillACLRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	userRoles, groupRoles, validations := validateOrganizationACLUpdate(ctx, api.Database, organizationID, organizationSkillACLUpdateValidator(req))
	if len(validations) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid request to update organization skill ACL.",
			Validations: validations,
		})
		return
	}

	var updated database.Skill
	err := api.Database.InTx(func(tx database.Store) error {
		//nolint:gocritic // The ACL write below reauthorizes the locked row for share.
		current, err := tx.GetOrganizationSkillByIDForUpdate(dbauthz.AsSystemRestricted(ctx), skill.ID)
		if err != nil {
			return xerrors.Errorf("get organization skill for update: %w", err)
		}
		aReq.Old = database.AuditableOrganizationSkill{Skill: current}
		userACL := maps.Clone(current.UserACL)
		groupACL := maps.Clone(current.GroupACL)
		applyACLReadRoles(userACL, userRoles)
		applyACLReadRoles(groupACL, groupRoles)
		updated, err = tx.UpdateOrganizationSkillACLByID(ctx, database.UpdateOrganizationSkillACLByIDParams{
			ID:       skill.ID,
			UserACL:  userACL,
			GroupACL: groupACL,
		})
		if err != nil {
			return xerrors.Errorf("update organization skill ACL: %w", err)
		}
		return nil
	}, nil)
	if err != nil {
		// A delete between the middleware read and the row lock stays
		// concealed as 404, matching the update and delete handlers.
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.InternalServerError(rw, err)
		return
	}
	aReq.New = database.AuditableOrganizationSkill{Skill: updated}
	rw.WriteHeader(http.StatusNoContent)
}

type organizationSkillACLUpdateValidator codersdk.UpdateOrganizationSkillACLRequest

var _ acl.UpdateValidator[codersdk.OrganizationSkillRole] = organizationSkillACLUpdateValidator{}

func (v organizationSkillACLUpdateValidator) Users() (map[string]codersdk.OrganizationSkillRole, string) {
	return v.UserRoles, "user_roles"
}

func (v organizationSkillACLUpdateValidator) Groups() (map[string]codersdk.OrganizationSkillRole, string) {
	return v.GroupRoles, "group_roles"
}

func (organizationSkillACLUpdateValidator) ValidateRole(role codersdk.OrganizationSkillRole) error {
	if role == codersdk.OrganizationSkillRoleDeleted || role == codersdk.OrganizationSkillRoleRead {
		return nil
	}
	return xerrors.Errorf("role %q is not a valid organization skill role", role)
}

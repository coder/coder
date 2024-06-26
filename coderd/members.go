package coderd

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary Add organization member
// @ID add-organization-member
// @Security CoderSessionToken
// @Produce json
// @Tags Members
// @Param organization path string true "Organization ID"
// @Param user path string true "User ID, name, or me"
// @Success 200 {object} codersdk.OrganizationMember
// @Router /organizations/{organization}/members/{user} [post]
func (api *API) postOrganizationMember(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		organization      = httpmw.OrganizationParam(r)
		user              = httpmw.UserParam(r)
		auditor           = api.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.AuditableOrganizationMember](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionCreate,
		})
	)
	aReq.Old = database.AuditableOrganizationMember{}
	defer commitAudit()

	member, err := api.Database.InsertOrganizationMember(ctx, database.InsertOrganizationMemberParams{
		OrganizationID: organization.ID,
		UserID:         user.ID,
		CreatedAt:      dbtime.Now(),
		UpdatedAt:      dbtime.Now(),
		Roles:          []string{},
	})
	if httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if database.IsUniqueViolation(err, database.UniqueOrganizationMembersPkey) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Organization member already exists in this organization",
		})
		return
	}
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	aReq.New = member.Auditable(user.Username)
	resp, err := convertOrganizationMembers(ctx, api.Database, []database.OrganizationMember{member})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	if len(resp) == 0 {
		httpapi.InternalServerError(rw, xerrors.Errorf("marshal member"))
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, resp[0])
}

// @Summary Remove organization member
// @ID remove-organization-member
// @Security CoderSessionToken
// @Produce json
// @Tags Members
// @Param organization path string true "Organization ID"
// @Param user path string true "User ID, name, or me"
// @Success 200 {object} codersdk.OrganizationMember
// @Router /organizations/{organization}/members/{user} [delete]
func (api *API) deleteOrganizationMember(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		organization      = httpmw.OrganizationParam(r)
		member            = httpmw.OrganizationMemberParam(r)
		auditor           = api.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.AuditableOrganizationMember](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionDelete,
		})
	)
	aReq.Old = member.OrganizationMember.Auditable(member.Username)
	defer commitAudit()

	err := api.Database.DeleteOrganizationMember(ctx, database.DeleteOrganizationMemberParams{
		OrganizationID: organization.ID,
		UserID:         member.UserID,
	})
	if httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	aReq.New = database.AuditableOrganizationMember{}
	httpapi.Write(ctx, rw, http.StatusOK, "organization member removed")
}

// @Summary List organization members
// @ID list-organization-members
// @Security CoderSessionToken
// @Produce json
// @Tags Members
// @Param organization path string true "Organization ID"
// @Success 200 {object} []codersdk.OrganizationMemberWithName
// @Router /organizations/{organization}/members [get]
func (api *API) listMembers(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx          = r.Context()
		organization = httpmw.OrganizationParam(r)
	)

	members, err := api.Database.OrganizationMembers(ctx, database.OrganizationMembersParams{
		OrganizationID: organization.ID,
		UserID:         uuid.Nil,
	})
	if httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	resp, err := convertOrganizationMemberRows(ctx, api.Database, members)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, resp)
}

// @Summary Assign role to organization member
// @ID assign-role-to-organization-member
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Members
// @Param organization path string true "Organization ID"
// @Param user path string true "User ID, name, or me"
// @Param request body codersdk.UpdateRoles true "Update roles request"
// @Success 200 {object} codersdk.OrganizationMember
// @Router /organizations/{organization}/members/{user}/roles [put]
func (api *API) putMemberRoles(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		organization      = httpmw.OrganizationParam(r)
		member            = httpmw.OrganizationMemberParam(r)
		apiKey            = httpmw.APIKey(r)
		auditor           = api.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.AuditableOrganizationMember](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionWrite,
		})
	)
	aReq.Old = member.OrganizationMember.Auditable(member.Username)
	defer commitAudit()

	if apiKey.UserID == member.OrganizationMember.UserID {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "You cannot change your own organization roles.",
		})
		return
	}

	var params codersdk.UpdateRoles
	if !httpapi.Read(ctx, rw, r, &params) {
		return
	}

	updatedUser, err := api.Database.UpdateMemberRoles(ctx, database.UpdateMemberRolesParams{
		GrantedRoles: params.Roles,
		UserID:       member.UserID,
		OrgID:        organization.ID,
	})
	if httpapi.Is404Error(err) {
		httpapi.Forbidden(rw)
		return
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: err.Error(),
		})
		return
	}
	aReq.New = database.AuditableOrganizationMember{
		OrganizationMember: updatedUser,
		Username:           member.Username,
	}

	resp, err := convertOrganizationMembers(ctx, api.Database, []database.OrganizationMember{updatedUser})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	if len(resp) != 1 {
		httpapi.InternalServerError(rw, xerrors.Errorf("failed to serialize member to response, update still succeeded"))
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, resp[0])
}

// convertOrganizationMembers batches the role lookup to make only 1 sql call
// We
func convertOrganizationMembers(ctx context.Context, db database.Store, mems []database.OrganizationMember) ([]codersdk.OrganizationMember, error) {
	converted := make([]codersdk.OrganizationMember, 0, len(mems))
	roleLookup := make([]database.NameOrganizationPair, 0)

	for _, m := range mems {
		converted = append(converted, codersdk.OrganizationMember{
			UserID:         m.UserID,
			OrganizationID: m.OrganizationID,
			CreatedAt:      m.CreatedAt,
			UpdatedAt:      m.UpdatedAt,
			Roles: db2sdk.List(m.Roles, func(r string) codersdk.SlimRole {
				// If it is a built-in role, no lookups are needed.
				rbacRole, err := rbac.RoleByName(rbac.RoleIdentifier{Name: r, OrganizationID: m.OrganizationID})
				if err == nil {
					return db2sdk.SlimRole(rbacRole)
				}

				// We know the role name and the organization ID. We are missing the
				// display name. Append the lookup parameter, so we can get the display name
				roleLookup = append(roleLookup, database.NameOrganizationPair{
					Name:           r,
					OrganizationID: m.OrganizationID,
				})
				return codersdk.SlimRole{
					Name:           r,
					DisplayName:    "",
					OrganizationID: m.OrganizationID.String(),
				}
			}),
		})
	}

	customRoles, err := db.CustomRoles(ctx, database.CustomRolesParams{
		LookupRoles:     roleLookup,
		ExcludeOrgRoles: false,
		OrganizationID:  uuid.UUID{},
	})
	if err != nil {
		// We are missing the display names, but that is not absolutely required. So just
		// return the converted and the names will be used instead of the display names.
		return converted, xerrors.Errorf("lookup custom roles: %w", err)
	}

	// Now map the customRoles back to the slimRoles for their display name.
	customRolesMap := make(map[string]database.CustomRole)
	for _, role := range customRoles {
		customRolesMap[role.RoleIdentifier().UniqueName()] = role
	}

	for i := range converted {
		for j, role := range converted[i].Roles {
			if cr, ok := customRolesMap[role.UniqueName()]; ok {
				converted[i].Roles[j].DisplayName = cr.DisplayName
			}
		}
	}

	return converted, nil
}

func convertOrganizationMemberRows(ctx context.Context, db database.Store, rows []database.OrganizationMembersRow) ([]codersdk.OrganizationMemberWithName, error) {
	members := make([]database.OrganizationMember, 0)
	for _, row := range rows {
		members = append(members, row.OrganizationMember)
	}

	convertedMembers, err := convertOrganizationMembers(ctx, db, members)
	if err != nil {
		return nil, err
	}
	if len(convertedMembers) != len(rows) {
		return nil, xerrors.Errorf("conversion failed, mismatch slice lengths")
	}

	converted := make([]codersdk.OrganizationMemberWithName, 0)
	for i := range convertedMembers {
		converted = append(converted, codersdk.OrganizationMemberWithName{
			Username:           rows[i].Username,
			OrganizationMember: convertedMembers[i],
		})
	}

	return converted, nil
}

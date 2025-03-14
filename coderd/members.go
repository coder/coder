package coderd

import (
	"errors"
	"context"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"

	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
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
			OrganizationID: organization.ID,
			Audit:          *auditor,
			Log:            api.Logger,
			Request:        r,
			Action:         database.AuditActionCreate,
		})
	)
	aReq.Old = database.AuditableOrganizationMember{}
	defer commitAudit()
	if !api.manualOrganizationMembership(ctx, rw, user) {
		return
	}
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
		httpapi.InternalServerError(rw, fmt.Errorf("marshal member"))
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, resp[0])
}
// @Summary Remove organization member

// @ID remove-organization-member
// @Security CoderSessionToken
// @Tags Members
// @Param organization path string true "Organization ID"
// @Param user path string true "User ID, name, or me"

// @Success 204
// @Router /organizations/{organization}/members/{user} [delete]
func (api *API) deleteOrganizationMember(rw http.ResponseWriter, r *http.Request) {

	var (
		ctx               = r.Context()
		apiKey            = httpmw.APIKey(r)
		organization      = httpmw.OrganizationParam(r)
		member            = httpmw.OrganizationMemberParam(r)
		auditor           = api.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.AuditableOrganizationMember](rw, &audit.RequestParams{
			OrganizationID: organization.ID,
			Audit:          *auditor,
			Log:            api.Logger,
			Request:        r,
			Action:         database.AuditActionDelete,
		})
	)
	aReq.Old = member.OrganizationMember.Auditable(member.Username)
	defer commitAudit()
	// Note: we disallow adding OIDC users if organization sync is enabled.
	// For removing members, do not have this same enforcement. As long as a user
	// does not re-login, they will not be immediately removed from the organization.
	// There might be an urgent need to revoke access.
	// A user can re-login if they are removed in error.
	// If we add a feature to force logout a user, then we can prevent manual
	// member removal when organization sync is enabled, and use force logout instead.
	if member.UserID == apiKey.UserID {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{Message: "cannot remove self from an organization"})
		return

	}
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
	rw.WriteHeader(http.StatusNoContent)
}
// @Deprecated use /organizations/{organization}/paginated-members [get]
// @Summary List organization members
// @ID list-organization-members
// @Security CoderSessionToken
// @Produce json
// @Tags Members
// @Param organization path string true "Organization ID"
// @Success 200 {object} []codersdk.OrganizationMemberWithUserData
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
	resp, err := convertOrganizationMembersWithUserData(ctx, api.Database, members)
	if err != nil {
		httpapi.InternalServerError(rw, err)

		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, resp)
}
// @Summary Paginated organization members
// @ID paginated-organization-members
// @Security CoderSessionToken
// @Produce json
// @Tags Members
// @Param organization path string true "Organization ID"
// @Param limit query int false "Page limit, if 0 returns all members"
// @Param offset query int false "Page offset"
// @Success 200 {object} []codersdk.PaginatedMembersResponse

// @Router /organizations/{organization}/paginated-members [get]
func (api *API) paginatedMembers(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx                  = r.Context()
		organization         = httpmw.OrganizationParam(r)
		paginationParams, ok = parsePagination(rw, r)

	)
	if !ok {
		return

	}
	paginatedMemberRows, err := api.Database.PaginatedOrganizationMembers(ctx, database.PaginatedOrganizationMembersParams{
		OrganizationID: organization.ID,
		LimitOpt:       int32(paginationParams.Limit),
		OffsetOpt:      int32(paginationParams.Offset),
	})
	if httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	memberRows := make([]database.OrganizationMembersRow, 0)
	for _, pRow := range paginatedMemberRows {
		row := database.OrganizationMembersRow{
			OrganizationMember: pRow.OrganizationMember,
			Username:           pRow.Username,
			AvatarURL:          pRow.AvatarURL,

			Name:               pRow.Name,
			Email:              pRow.Email,
			GlobalRoles:        pRow.GlobalRoles,
		}
		memberRows = append(memberRows, row)
	}
	members, err := convertOrganizationMembersWithUserData(ctx, api.Database, memberRows)
	if err != nil {
		httpapi.InternalServerError(rw, err)
	}
	resp := codersdk.PaginatedMembersResponse{
		Members: members,
		Count:   int(paginatedMemberRows[0].Count),
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
			OrganizationID: organization.ID,
			Audit:          *auditor,
			Log:            api.Logger,
			Request:        r,
			Action:         database.AuditActionWrite,

		})
	)
	aReq.Old = member.OrganizationMember.Auditable(member.Username)
	defer commitAudit()
	// Check if changing roles is allowed
	if !api.allowChangingMemberRoles(ctx, rw, member, organization) {
		return
	}
	if apiKey.UserID == member.OrganizationMember.UserID {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "You cannot change your own organization roles.",
			Detail:  "Another user with the appropriate permissions must change your roles.",
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
		httpapi.InternalServerError(rw, fmt.Errorf("failed to serialize member to response, update still succeeded"))
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, resp[0])
}
func (api *API) allowChangingMemberRoles(ctx context.Context, rw http.ResponseWriter, member httpmw.OrganizationMember, organization database.Organization) bool {
	// nolint:gocritic // The caller could be an org admin without this perm.
	// We need to disable manual role assignment if role sync is enabled for
	// the given organization.
	user, err := api.Database.GetUserByID(dbauthz.AsSystemRestricted(ctx), member.UserID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return false
	}
	if user.LoginType == database.LoginTypeOIDC {
		// nolint:gocritic // fetching settings
		orgSync, err := api.IDPSync.OrganizationRoleSyncEnabled(dbauthz.AsSystemRestricted(ctx), api.Database, organization.ID)
		if err != nil {
			httpapi.InternalServerError(rw, err)
			return false
		}
		if orgSync {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{

				Message: "Cannot modify roles for OIDC users when role sync is enabled. This organization member's roles are managed by the identity provider. Have the user re-login to refresh their roles.",
				Detail:  "'User Role Field' is set in the organization settings. Ask an administrator to adjust or disable these settings.",
			})
			return false
		}
	}
	return true
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
		OrganizationID:  uuid.Nil,
	})
	if err != nil {
		// We are missing the display names, but that is not absolutely required. So just
		// return the converted and the names will be used instead of the display names.

		return converted, fmt.Errorf("lookup custom roles: %w", err)
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
func convertOrganizationMembersWithUserData(ctx context.Context, db database.Store, rows []database.OrganizationMembersRow) ([]codersdk.OrganizationMemberWithUserData, error) {
	members := make([]database.OrganizationMember, 0)
	for _, row := range rows {
		members = append(members, row.OrganizationMember)
	}
	convertedMembers, err := convertOrganizationMembers(ctx, db, members)
	if err != nil {
		return nil, err
	}
	if len(convertedMembers) != len(rows) {
		return nil, fmt.Errorf("conversion failed, mismatch slice lengths")
	}

	converted := make([]codersdk.OrganizationMemberWithUserData, 0)
	for i := range convertedMembers {
		converted = append(converted, codersdk.OrganizationMemberWithUserData{
			Username:           rows[i].Username,
			AvatarURL:          rows[i].AvatarURL,
			Name:               rows[i].Name,
			Email:              rows[i].Email,
			GlobalRoles:        db2sdk.SlimRolesFromNames(rows[i].GlobalRoles),
			OrganizationMember: convertedMembers[i],
		})
	}

	return converted, nil
}
// manualOrganizationMembership checks if the user is an OIDC user and if organization sync is enabled.
// If organization sync is enabled, manual organization assignment is not allowed,
// since all organization membership is controlled by the external IDP.
func (api *API) manualOrganizationMembership(ctx context.Context, rw http.ResponseWriter, user database.User) bool {

	if user.LoginType == database.LoginTypeOIDC && api.IDPSync.OrganizationSyncEnabled(ctx, api.Database) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Organization sync is enabled for OIDC users, meaning manual organization assignment is not allowed for this user. Have the user re-login to refresh their organizations.",
			Detail:  fmt.Sprintf("User %s is an OIDC user and organization sync is enabled. Ask an administrator to resolve the membership in your external IDP.", user.Username),
		})
		return false
	}
	return true

}

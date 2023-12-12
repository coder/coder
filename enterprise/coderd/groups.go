package coderd

import (
	"database/sql"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary Create group for organization
// @ID create-group-for-organization
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Enterprise
// @Param request body codersdk.CreateGroupRequest true "Create group request"
// @Param organization path string true "Organization ID"
// @Success 201 {object} codersdk.Group
// @Router /organizations/{organization}/groups [post]
func (api *API) postGroupByOrganization(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		org               = httpmw.OrganizationParam(r)
		auditor           = api.AGPL.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.AuditableGroup](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionCreate,
		})
	)
	defer commitAudit()

	var req codersdk.CreateGroupRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	if req.Name == database.EveryoneGroup {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("%q is a reserved keyword and cannot be used for a group name.", database.EveryoneGroup),
		})
		return
	}

	group, err := api.Database.InsertGroup(ctx, database.InsertGroupParams{
		ID:             uuid.New(),
		Name:           req.Name,
		DisplayName:    req.DisplayName,
		OrganizationID: org.ID,
		AvatarURL:      req.AvatarURL,
		QuotaAllowance: int32(req.QuotaAllowance),
	})
	if database.IsUniqueViolation(err) {
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
			Message: fmt.Sprintf("Group with name %q already exists.", req.Name),
		})
		return
	}
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	var emptyUsers []database.User
	aReq.New = group.Auditable(emptyUsers)

	httpapi.Write(ctx, rw, http.StatusCreated, convertGroup(group, nil))
}

// @Summary Update group by name
// @ID update-group-by-name
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Enterprise
// @Param group path string true "Group name"
// @Param request body codersdk.PatchGroupRequest true "Patch group request"
// @Success 200 {object} codersdk.Group
// @Router /groups/{group} [patch]
func (api *API) patchGroup(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		group             = httpmw.GroupParam(r)
		auditor           = api.AGPL.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.AuditableGroup](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionWrite,
		})
	)
	defer commitAudit()

	var req codersdk.PatchGroupRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	// If the name matches the existing group name pretend we aren't
	// updating the name at all.
	if req.Name == group.Name {
		req.Name = ""
	}

	if group.IsEveryone() && req.Name != "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("Cannot rename the %q group!", database.EveryoneGroup),
		})
		return
	}

	if group.IsEveryone() && (req.DisplayName != nil && *req.DisplayName != "") {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("Cannot update the Display Name for the %q group!", database.EveryoneGroup),
		})
		return
	}

	if req.Name == database.EveryoneGroup {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("%q is a reserved group name!", database.EveryoneGroup),
		})
		return
	}

	users := make([]string, 0, len(req.AddUsers)+len(req.RemoveUsers))
	users = append(users, req.AddUsers...)
	users = append(users, req.RemoveUsers...)

	if len(users) > 0 && group.Name == database.EveryoneGroup {
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: fmt.Sprintf("Cannot add or remove users from the %q group!", database.EveryoneGroup),
		})
		return
	}

	currentMembers, err := api.Database.GetGroupMembers(ctx, group.ID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	aReq.Old = group.Auditable(currentMembers)

	for _, id := range users {
		if _, err := uuid.Parse(id); err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: fmt.Sprintf("ID %q must be a valid user UUID.", id),
			})
			return
		}
		// TODO: It would be nice to enforce this at the schema level
		// but unfortunately our org_members table does not have an ID.
		_, err := api.Database.GetOrganizationMemberByUserID(ctx, database.GetOrganizationMemberByUserIDParams{
			OrganizationID: group.OrganizationID,
			UserID:         uuid.MustParse(id),
		})
		if xerrors.Is(err, sql.ErrNoRows) {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: fmt.Sprintf("User %q must be a member of organization %q", id, group.ID),
			})
			return
		}
		if err != nil {
			httpapi.InternalServerError(rw, err)
			return
		}
	}

	if req.Name != "" && req.Name != group.Name {
		_, err := api.Database.GetGroupByOrgAndName(ctx, database.GetGroupByOrgAndNameParams{
			OrganizationID: group.OrganizationID,
			Name:           req.Name,
		})
		if err == nil {
			httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
				Message: fmt.Sprintf("A group with name %q already exists.", req.Name),
			})
			return
		}
	}

	err = database.ReadModifyUpdate(api.Database, func(tx database.Store) error {
		group, err = tx.GetGroupByID(ctx, group.ID)
		if err != nil {
			return xerrors.Errorf("get group by ID: %w", err)
		}

		updateGroupParams := database.UpdateGroupByIDParams{
			ID:             group.ID,
			AvatarURL:      group.AvatarURL,
			Name:           group.Name,
			DisplayName:    group.DisplayName,
			QuotaAllowance: group.QuotaAllowance,
		}

		// TODO: Do we care about validating this?
		if req.AvatarURL != nil {
			updateGroupParams.AvatarURL = *req.AvatarURL
		}
		if req.Name != "" {
			updateGroupParams.Name = req.Name
		}
		if req.QuotaAllowance != nil {
			updateGroupParams.QuotaAllowance = int32(*req.QuotaAllowance)
		}
		if req.DisplayName != nil {
			updateGroupParams.DisplayName = *req.DisplayName
		}

		group, err = tx.UpdateGroupByID(ctx, updateGroupParams)
		if err != nil {
			return xerrors.Errorf("update group by ID: %w", err)
		}

		for _, id := range req.AddUsers {
			userID, err := uuid.Parse(id)
			if err != nil {
				return xerrors.Errorf("parse user ID %q: %w", id, err)
			}
			err = tx.InsertGroupMember(ctx, database.InsertGroupMemberParams{
				GroupID: group.ID,
				UserID:  userID,
			})
			if err != nil {
				return xerrors.Errorf("insert group member %q: %w", id, err)
			}
		}
		for _, id := range req.RemoveUsers {
			userID, err := uuid.Parse(id)
			if err != nil {
				return xerrors.Errorf("parse user ID %q: %w", id, err)
			}
			err = tx.DeleteGroupMemberFromGroup(ctx, database.DeleteGroupMemberFromGroupParams{
				UserID:  userID,
				GroupID: group.ID,
			})
			if err != nil {
				return xerrors.Errorf("insert group member %q: %w", id, err)
			}
		}
		return nil
	})

	if database.IsUniqueViolation(err) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Cannot add the same user to a group twice!",
			Detail:  err.Error(),
		})
		return
	}
	if httpapi.Is404Error(err) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to add or remove non-existent group member",
			Detail:  err.Error(),
		})
		return
	}
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	patchedMembers, err := api.Database.GetGroupMembers(ctx, group.ID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	aReq.New = group.Auditable(patchedMembers)

	httpapi.Write(ctx, rw, http.StatusOK, convertGroup(group, patchedMembers))
}

// @Summary Delete group by name
// @ID delete-group-by-name
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param group path string true "Group name"
// @Success 200 {object} codersdk.Group
// @Router /groups/{group} [delete]
func (api *API) deleteGroup(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		group             = httpmw.GroupParam(r)
		auditor           = api.AGPL.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.AuditableGroup](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionDelete,
		})
	)
	defer commitAudit()

	if group.Name == database.EveryoneGroup {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("%q is a reserved group and cannot be deleted!", database.EveryoneGroup),
		})
		return
	}

	groupMembers, getMembersErr := api.Database.GetGroupMembers(ctx, group.ID)
	if getMembersErr != nil {
		httpapi.InternalServerError(rw, getMembersErr)
		return
	}

	aReq.Old = group.Auditable(groupMembers)

	err := api.Database.DeleteGroupByID(ctx, group.ID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.Response{
		Message: "Successfully deleted group!",
	})
}

// @Summary Get group by organization and group name
// @ID get-group-by-organization-and-group-name
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param organization path string true "Organization ID" format(uuid)
// @Param groupName path string true "Group name"
// @Success 200 {object} codersdk.Group
// @Router /organizations/{organization}/groups/{groupName} [get]
func (api *API) groupByOrganization(rw http.ResponseWriter, r *http.Request) {
	api.group(rw, r)
}

// @Summary Get group by ID
// @ID get-group-by-id
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param group path string true "Group id"
// @Success 200 {object} codersdk.Group
// @Router /groups/{group} [get]
func (api *API) group(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx   = r.Context()
		group = httpmw.GroupParam(r)
	)

	users, err := api.Database.GetGroupMembers(ctx, group.ID)
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, convertGroup(group, users))
}

// @Summary Get groups by organization
// @ID get-groups-by-organization
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param organization path string true "Organization ID" format(uuid)
// @Success 200 {array} codersdk.Group
// @Router /organizations/{organization}/groups [get]
func (api *API) groupsByOrganization(rw http.ResponseWriter, r *http.Request) {
	api.groups(rw, r)
}

func (api *API) groups(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx = r.Context()
		org = httpmw.OrganizationParam(r)
	)

	groups, err := api.Database.GetGroupsByOrganizationID(ctx, org.ID)
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		httpapi.InternalServerError(rw, err)
		return
	}

	// Filter groups based on rbac permissions
	groups, err = coderd.AuthorizeFilter(api.AGPL.HTTPAuth, r, rbac.ActionRead, groups)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching groups.",
			Detail:  err.Error(),
		})
		return
	}

	resp := make([]codersdk.Group, 0, len(groups))
	for _, group := range groups {
		members, err := api.Database.GetGroupMembers(ctx, group.ID)
		if err != nil {
			httpapi.InternalServerError(rw, err)
			return
		}

		resp = append(resp, convertGroup(group, members))
	}

	httpapi.Write(ctx, rw, http.StatusOK, resp)
}

func convertGroup(g database.Group, users []database.User) codersdk.Group {
	// It's ridiculous to query all the orgs of a user here
	// especially since as of the writing of this comment there
	// is only one org. So we pretend everyone is only part of
	// the group's organization.
	orgs := make(map[uuid.UUID][]uuid.UUID)
	for _, user := range users {
		orgs[user.ID] = []uuid.UUID{g.OrganizationID}
	}

	return codersdk.Group{
		ID:             g.ID,
		Name:           g.Name,
		DisplayName:    g.DisplayName,
		OrganizationID: g.OrganizationID,
		AvatarURL:      g.AvatarURL,
		QuotaAllowance: int(g.QuotaAllowance),
		Members:        convertUsers(users, orgs),
		Source:         codersdk.GroupSource(g.Source),
	}
}

func convertUser(user database.User, organizationIDs []uuid.UUID) codersdk.User {
	convertedUser := codersdk.User{
		ID:              user.ID,
		Email:           user.Email,
		CreatedAt:       user.CreatedAt,
		LastSeenAt:      user.LastSeenAt,
		Username:        user.Username,
		Status:          codersdk.UserStatus(user.Status),
		OrganizationIDs: organizationIDs,
		Roles:           make([]codersdk.Role, 0, len(user.RBACRoles)),
		AvatarURL:       user.AvatarURL,
		LoginType:       codersdk.LoginType(user.LoginType),
	}

	for _, roleName := range user.RBACRoles {
		rbacRole, _ := rbac.RoleByName(roleName)
		convertedUser.Roles = append(convertedUser.Roles, convertRole(rbacRole))
	}

	return convertedUser
}

func convertUsers(users []database.User, organizationIDsByUserID map[uuid.UUID][]uuid.UUID) []codersdk.User {
	converted := make([]codersdk.User, 0, len(users))
	for _, u := range users {
		userOrganizationIDs := organizationIDsByUserID[u.ID]
		converted = append(converted, convertUser(u, userOrganizationIDs))
	}
	return converted
}

func convertRole(role rbac.Role) codersdk.Role {
	return codersdk.Role{
		DisplayName: role.DisplayName,
		Name:        role.Name,
	}
}

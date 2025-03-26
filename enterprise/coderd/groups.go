package coderd

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
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
			Audit:          *auditor,
			Log:            api.Logger,
			Request:        r,
			Action:         database.AuditActionCreate,
			OrganizationID: org.ID,
		})
	)
	defer commitAudit()

	var req codersdk.CreateGroupRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	if req.Name == database.EveryoneGroup {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid group name.",
			Validations: []codersdk.ValidationError{{Field: "name", Detail: fmt.Sprintf("%q is a reserved group name", req.Name)}},
		})
		return
	}

	group, err := api.Database.InsertGroup(ctx, database.InsertGroupParams{
		ID:             uuid.New(),
		Name:           req.Name,
		DisplayName:    req.DisplayName,
		OrganizationID: org.ID,
		AvatarURL:      req.AvatarURL,
		// #nosec G115 - Quota allowance is small and fits in int32
		QuotaAllowance: int32(req.QuotaAllowance),
	})
	if database.IsUniqueViolation(err) {
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
			Message:     fmt.Sprintf("A group named %q already exists.", req.Name),
			Validations: []codersdk.ValidationError{{Field: "name", Detail: "Group names must be unique"}},
		})
		return
	}
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	var emptyMembers []database.GroupMember
	aReq.New = group.Auditable(emptyMembers)

	httpapi.Write(ctx, rw, http.StatusCreated, db2sdk.Group(database.GetGroupsRow{
		Group:                   group,
		OrganizationName:        org.Name,
		OrganizationDisplayName: org.DisplayName,
	}, nil, 0))
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
			Audit:          *auditor,
			Log:            api.Logger,
			Request:        r,
			Action:         database.AuditActionWrite,
			OrganizationID: group.OrganizationID,
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

	currentMembers, err := api.Database.GetGroupMembersByGroupID(ctx, group.ID)
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
		_, err := database.ExpectOne(api.Database.OrganizationMembers(ctx, database.OrganizationMembersParams{
			OrganizationID: group.OrganizationID,
			UserID:         uuid.MustParse(id),
		}))
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: fmt.Sprintf("User must be a member of organization %q", group.Name),
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
			// #nosec G115 - Quota allowance is small and fits in int32
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

	org, err := api.Database.GetOrganizationByID(ctx, group.OrganizationID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
	}

	patchedMembers, err := api.Database.GetGroupMembersByGroupID(ctx, group.ID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	aReq.New = group.Auditable(patchedMembers)

	memberCount, err := api.Database.GetGroupMembersCountByGroupID(ctx, group.ID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.Group(database.GetGroupsRow{
		Group:                   group,
		OrganizationName:        org.Name,
		OrganizationDisplayName: org.DisplayName,
	}, patchedMembers, int(memberCount)))
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
			Audit:          *auditor,
			Log:            api.Logger,
			Request:        r,
			Action:         database.AuditActionDelete,
			OrganizationID: group.OrganizationID,
		})
	)
	defer commitAudit()

	if group.Name == database.EveryoneGroup {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("%q is a reserved group and cannot be deleted!", database.EveryoneGroup),
		})
		return
	}

	groupMembers, getMembersErr := api.Database.GetGroupMembersByGroupID(ctx, group.ID)
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

	org, err := api.Database.GetOrganizationByID(ctx, group.OrganizationID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
	}

	users, err := api.Database.GetGroupMembersByGroupID(ctx, group.ID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		httpapi.InternalServerError(rw, err)
		return
	}

	memberCount, err := api.Database.GetGroupMembersCountByGroupID(ctx, group.ID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.Group(database.GetGroupsRow{
		Group:                   group,
		OrganizationName:        org.Name,
		OrganizationDisplayName: org.DisplayName,
	}, users, int(memberCount)))
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
	org := httpmw.OrganizationParam(r)

	values := r.URL.Query()
	values.Set("organization", org.ID.String())
	r.URL.RawQuery = values.Encode()

	api.groups(rw, r)
}

// @Summary Get groups
// @ID get-groups
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param organization query string true "Organization ID or name"
// @Param has_member query string true "User ID or name"
// @Param group_ids query string true "Comma separated list of group IDs"
// @Success 200 {array} codersdk.Group
// @Router /groups [get]
func (api *API) groups(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var filter database.GetGroupsParams
	parser := httpapi.NewQueryParamParser()
	// Organization selector can be an org ID or name
	filter.OrganizationID = parser.UUIDorName(r.URL.Query(), uuid.Nil, "organization", func(orgName string) (uuid.UUID, error) {
		org, err := api.Database.GetOrganizationByName(ctx, database.GetOrganizationByNameParams{
			Name:    orgName,
			Deleted: false,
		})
		if err != nil {
			return uuid.Nil, xerrors.Errorf("organization %q not found", orgName)
		}
		return org.ID, nil
	})

	// has_member selector can be a user ID or username
	filter.HasMemberID = parser.UUIDorName(r.URL.Query(), uuid.Nil, "has_member", func(username string) (uuid.UUID, error) {
		user, err := api.Database.GetUserByEmailOrUsername(ctx, database.GetUserByEmailOrUsernameParams{
			Username: username,
			Email:    "",
		})
		if err != nil {
			return uuid.Nil, xerrors.Errorf("user %q not found", username)
		}
		return user.ID, nil
	})

	filter.GroupIds = parser.UUIDs(r.URL.Query(), []uuid.UUID{}, "group_ids")

	parser.ErrorExcessParams(r.URL.Query())
	if len(parser.Errors) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Query parameters have invalid values.",
			Validations: parser.Errors,
		})
		return
	}

	groups, err := api.Database.GetGroups(ctx, filter)
	if httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	resp := make([]codersdk.Group, 0, len(groups))
	for _, group := range groups {
		members, err := api.Database.GetGroupMembersByGroupID(ctx, group.Group.ID)
		if err != nil {
			httpapi.InternalServerError(rw, err)
			return
		}
		memberCount, err := api.Database.GetGroupMembersCountByGroupID(ctx, group.Group.ID)
		if err != nil {
			httpapi.InternalServerError(rw, err)
			return
		}

		resp = append(resp, db2sdk.Group(group, members, int(memberCount)))
	}

	httpapi.Write(ctx, rw, http.StatusOK, resp)
}

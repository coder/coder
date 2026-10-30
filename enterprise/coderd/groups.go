package coderd

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	agpl "github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/searchquery"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary Create group for organization
// @Description Creates a group in an organization.
// @Description Returns `409` if the organization already has a group with that name.
// @Description Requires permission to create groups in the organization.
// @Description Requires a Premium license.
// @ID create-group-for-organization
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Enterprise
// @Param request body codersdk.CreateGroupRequest true "Group to create. `name` is required, must be 255 characters or fewer, and can't be `Everyone`, `new`, or `create`. `quota_allowance` is the workspace quota credits the group adds to the budget of each member."
// @Param organization path string true "Organization ID, name, or `default` for the default organization."
// @Success 201 {object} codersdk.Group
// @Router /api/v2/organizations/{organization}/groups [post]
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
// @Description Updates a group's name, display name, avatar URL, or quota allowance, and adds or removes members by user ID with `add_users` and `remove_users`.
// @Description You can't rename the `Everyone` group, change its display name, or add or remove its members, and the users in `add_users` and `remove_users` must be members of the group's organization.
// @Description Requires permission to update the group.
// @Description Requires a Premium license.
// @ID update-group-by-name
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Enterprise
// @Param group path string true "Group ID."
// @Param request body codersdk.PatchGroupRequest true "Changes to apply to the group. Omitted fields stay as they are. `add_users` and `remove_users` are lists of user IDs, and `name` can't be `Everyone`."
// @Success 200 {object} codersdk.Group
// @Router /api/v2/groups/{group} [patch]
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

	currentMembers, err := api.Database.GetGroupMembersByGroupID(ctx, database.GetGroupMembersByGroupIDParams{
		GroupID:       group.ID,
		IncludeSystem: false,
	})
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
		// Skip membership checks for the prebuilds user. There is a valid use case
		// for adding the prebuilds user to a single group: in order to set a quota
		// allowance specifically for prebuilds.
		if id == database.PrebuildsSystemUserID.String() {
			continue
		}
		_, err := database.ExpectOne(api.Database.OrganizationMembers(ctx, database.OrganizationMembersParams{
			OrganizationID: group.OrganizationID,
			UserID:         uuid.MustParse(id),
			IncludeSystem:  false,
			GithubUserID:   0,
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
		existing, err := api.Database.GetGroupByOrgAndName(ctx, database.GetGroupByOrgAndNameParams{
			OrganizationID: group.OrganizationID,
			Name:           req.Name,
		})
		// GetGroupByOrgAndName matches names case-insensitively, so exclude the
		// group being renamed. This allows changing only the casing of a name
		// while still rejecting a name already taken by a different group.
		if err == nil && existing.ID != group.ID {
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
	// Unauthorized errors return a 404 to not leak information about the
	// existence of resources.
	if httpapi.IsUnauthorizedError(err) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if httpapi.Is404Error(err) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to update group.",
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

	patchedMembers, err := api.Database.GetGroupMembersByGroupID(ctx, database.GetGroupMembersByGroupIDParams{
		GroupID:       group.ID,
		IncludeSystem: false,
	})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	aReq.New = group.Auditable(patchedMembers)

	memberCount, err := api.Database.GetGroupMembersCountByGroupID(ctx, database.GetGroupMembersCountByGroupIDParams{
		GroupID:       group.ID,
		IncludeSystem: false,
	})
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
// @Description Deletes a group by its ID.
// @Description You can't delete the `Everyone` group.
// @Description Requires permission to delete the group.
// @Description Requires a Premium license.
// @ID delete-group-by-name
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param group path string true "Group ID."
// @Success 200 {object} codersdk.Group
// @Router /api/v2/groups/{group} [delete]
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

	groupMembers, getMembersErr := api.Database.GetGroupMembersByGroupID(ctx, database.GetGroupMembersByGroupIDParams{
		GroupID:       group.ID,
		IncludeSystem: false,
	})
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
// @Description Returns a group in an organization by its name, with the members you can read.
// @Description Requires permission to read the group.
// @Description Requires a Premium license.
// @ID get-group-by-organization-and-group-name
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param organization path string true "Organization ID, name, or `default` for the default organization."
// @Param groupName path string true "Group name in the organization. Matching is case-insensitive."
// @Success 200 {object} codersdk.Group
// @Router /api/v2/organizations/{organization}/groups/{groupName} [get]
func (api *API) groupByOrganization(rw http.ResponseWriter, r *http.Request) {
	api.group(rw, r)
}

// @Summary Get group by ID
// @Description Returns a group by its ID, with the members you can read.
// @Description Requires permission to read the group.
// @Description Requires a Premium license.
// @ID get-group-by-id
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param group path string true "Group ID."
// @Param exclude_members query bool false "If `true`, returns the group without its `members`. `total_member_count` is still returned."
// @Success 200 {object} codersdk.Group
// @Router /api/v2/groups/{group} [get]
func (api *API) group(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx   = r.Context()
		group = httpmw.GroupParam(r)
	)

	excludeMembers, _ := strconv.ParseBool(r.URL.Query().Get("exclude_members"))

	org, err := api.Database.GetOrganizationByID(ctx, group.OrganizationID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
	}

	users := []database.GroupMember{}
	if !excludeMembers {
		users, err = api.Database.GetGroupMembersByGroupID(ctx, database.GetGroupMembersByGroupIDParams{
			GroupID:       group.ID,
			IncludeSystem: false,
		})
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			httpapi.InternalServerError(rw, err)
			return
		}
	}

	memberCount, err := api.Database.GetGroupMembersCountByGroupID(ctx, database.GetGroupMembersCountByGroupIDParams{
		GroupID:       group.ID,
		IncludeSystem: false,
	})
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

// @Summary Get group members by organization and group name
// @Description Returns a page of the members of a group in an organization, filtered by the `q` search query.
// @Description Returns only the members you can read.
// @Description Requires a Premium license.
// @ID get-group-members-by-organization-and-group-name
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param organization path string true "Organization ID, name, or `default` for the default organization."
// @Param groupName path string true "Group name in the organization. Matching is case-insensitive."
// @Param q query string false "Search query in the format `key:value`. Available keys are: `name`, `username`, `email`, `status`, `service_account`, `role`, `last_seen_after`, `last_seen_before`, `created_after`, `created_before`, `github_com_user_id`, and `login_type`. Text without a key matches the username, name, or email."
// @Param after_id query string false "ID of the last result on the previous page. Returns the results after it, which pages faster than `offset`." format(uuid)
// @Param limit query int false "Maximum number of results to return. The default, `0`, returns all results."
// @Param offset query int false "Number of results to skip, for paging through results with `limit`."
// @Success 200 {object} codersdk.GroupMembersResponse
// @Router /api/v2/organizations/{organization}/groups/{groupName}/members [get]
func (api *API) groupMembersByOrganization(rw http.ResponseWriter, r *http.Request) {
	api.groupMembers(rw, r)
}

// @Summary Get group members by group ID
// @Description Returns a page of the members of a group, filtered by the `q` search query.
// @Description Returns only the members you can read.
// @Description Requires a Premium license.
// @ID get-group-members-by-group-id
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param group path string true "Group ID."
// @Param q query string false "Search query in the format `key:value`. Available keys are: `name`, `username`, `email`, `status`, `service_account`, `role`, `last_seen_after`, `last_seen_before`, `created_after`, `created_before`, `github_com_user_id`, and `login_type`. Text without a key matches the username, name, or email."
// @Param after_id query string false "ID of the last result on the previous page. Returns the results after it, which pages faster than `offset`." format(uuid)
// @Param limit query int false "Maximum number of results to return. The default, `0`, returns all results."
// @Param offset query int false "Number of results to skip, for paging through results with `limit`."
// @Success 200 {object} codersdk.GroupMembersResponse
// @Router /api/v2/groups/{group}/members [get]
func (api *API) groupMembers(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx   = r.Context()
		group = httpmw.GroupParam(r)
	)

	filterQuery := r.URL.Query().Get("q")
	userFilterParams, filterErrs := searchquery.Users(filterQuery)
	if len(filterErrs) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid member search query.",
			Validations: filterErrs,
		})
		return
	}

	paginationParams, ok := agpl.ParsePagination(rw, r)
	if !ok {
		return
	}

	members, err := api.Database.GetGroupMembersByGroupIDPaginated(ctx, database.GetGroupMembersByGroupIDPaginatedParams{
		AfterID:          paginationParams.AfterID,
		GroupID:          group.ID,
		IncludeSystem:    false,
		Search:           userFilterParams.Search,
		Name:             userFilterParams.Name,
		ExactUsername:    userFilterParams.ExactUsername,
		ExactEmail:       userFilterParams.ExactEmail,
		Status:           userFilterParams.Status,
		IsServiceAccount: userFilterParams.IsServiceAccount,
		RbacRole:         userFilterParams.RbacRole,
		LastSeenBefore:   userFilterParams.LastSeenBefore,
		LastSeenAfter:    userFilterParams.LastSeenAfter,
		CreatedAfter:     userFilterParams.CreatedAfter,
		CreatedBefore:    userFilterParams.CreatedBefore,
		GithubComUserID:  userFilterParams.GithubComUserID,
		LoginType:        userFilterParams.LoginType,
		// #nosec G115 - Pagination offsets are small and fit in int32
		OffsetOpt: int32(paginationParams.Offset),
		// #nosec G115 - Pagination limits are small and fit in int32
		LimitOpt: int32(paginationParams.Limit),
	})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		httpapi.InternalServerError(rw, err)
		return
	}

	if len(members) == 0 {
		httpapi.Write(ctx, rw, http.StatusOK, codersdk.GroupMembersResponse{
			Users: nil,
			Count: 0,
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.GroupMembersResponse{
		Users: db2sdk.ReducedUsersFromGroupMemberRows(members),
		Count: int(members[0].Count),
	})
}

// @Summary Get groups by organization
// @Description Returns the groups in an organization that you can read, each with the members you can read.
// @Description To page through groups and search them by name, use `GET /api/v2/organizations/{organization}/paginated-groups`, which returns each group's member count instead of its members and requires permission to read every group in the organization.
// @Description Requires a Premium license.
// @ID get-groups-by-organization
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param organization path string true "Organization ID, name, or `default` for the default organization."
// @Success 200 {array} codersdk.Group
// @Router /api/v2/organizations/{organization}/groups [get]
func (api *API) groupsByOrganization(rw http.ResponseWriter, r *http.Request) {
	org := httpmw.OrganizationParam(r)

	values := r.URL.Query()
	values.Set("organization", org.ID.String())
	r.URL.RawQuery = values.Encode()

	api.groups(rw, r)
}

// @Summary Get groups by organization (paginated)
// @ID get-groups-by-organization-paginated
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param organization path string true "Organization ID, name, or `default` for the default organization."
// @Param q query string false "Search term that matches group names and display names, either bare or as `search:<term>`. Other keys return `400`."
// @Param limit query int false "Maximum number of results to return. The default, `0`, returns all results."
// @Param offset query int false "Number of results to skip, for paging through results with `limit`."
// @Param after_id query string false "ID of the last result on the previous page. Returns the results after it, which pages faster than `offset`." format(uuid)
// @Success 200 {object} codersdk.PaginatedGroupsResponse
// @Description Returns the groups in an organization, ordered by name and paged with `limit`, with each group's `total_member_count` instead of its members.
// @Description To list a group's members, use `GET /api/v2/groups/{group}/members`.
// @Description Quote a `q` value that contains a colon, as in `search:"team: frontend"`.
// @Description Requires permission to read every group in the organization, and returns an error instead of a partial list without it.
// @Description Requires a Premium license.
// @Router /api/v2/organizations/{organization}/paginated-groups [get]
func (api *API) paginatedGroups(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	org := httpmw.OrganizationParam(r)

	filterQuery := r.URL.Query().Get("q")
	search, filterErrs := searchquery.Groups(filterQuery)
	if len(filterErrs) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid group search query.",
			Validations: filterErrs,
		})
		return
	}

	paginationParams, ok := agpl.ParsePagination(rw, r)
	if !ok {
		return
	}

	groups, err := api.Database.GetGroupsByOrganizationIDPaginated(ctx, database.GetGroupsByOrganizationIDPaginatedParams{
		OrganizationID: org.ID,
		Search:         search,
		AfterID:        paginationParams.AfterID,
		// #nosec G115 - Pagination offsets are small and fit in int32
		OffsetOpt: int32(paginationParams.Offset),
		// #nosec G115 - Pagination limits are small and fit in int32
		LimitOpt: int32(paginationParams.Limit),
	})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	if len(groups) == 0 {
		httpapi.Write(ctx, rw, http.StatusOK, codersdk.PaginatedGroupsResponse{
			Groups: []codersdk.PaginatedGroup{},
			Count:  0,
		})
		return
	}

	resp := codersdk.PaginatedGroupsResponse{
		Groups: make([]codersdk.PaginatedGroup, 0, len(groups)),
		Count:  int(groups[0].Count),
	}

	// Fetch member counts for every group on the page in a single query to
	// avoid an N+1 lookup. We intentionally do not hydrate the per-group member
	// rosters here: they can be large, contain member PII, and a caller
	// authorized to read a group is not necessarily authorized to read its
	// membership. Callers that need the roster page it via the group members
	// endpoint. Only the total member count is returned.
	groupIDs := make([]uuid.UUID, len(groups))
	for i, group := range groups {
		groupIDs[i] = group.Group.ID
	}
	// nolint:gocritic // Member counts are returned even without member read
	// access, matching GetGroupMembersCountByGroupID. The endpoint already
	// authorized org-wide group read.
	countRows, err := api.Database.GetGroupMembersCountByGroupIDs(dbauthz.AsSystemRestricted(ctx), database.GetGroupMembersCountByGroupIDsParams{
		GroupIds:      groupIDs,
		IncludeSystem: false,
	})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	countByGroup := make(map[uuid.UUID]int64, len(countRows))
	for _, row := range countRows {
		countByGroup[row.GroupID] = row.MemberCount
	}

	for _, group := range groups {
		resp.Groups = append(resp.Groups, db2sdk.PaginatedGroup(database.GetGroupsRow{
			Group:                   group.Group,
			OrganizationName:        group.OrganizationName,
			OrganizationDisplayName: group.OrganizationDisplayName,
		}, int(countByGroup[group.Group.ID])))
	}

	httpapi.Write(ctx, rw, http.StatusOK, resp)
}

// @Summary Get groups
// @Description Returns the groups you can read, filtered by `organization`, `has_member`, and `group_ids`, each with the members you can read.
// @Description Use `has_member` to find the groups a user belongs to.
// @Description Requires a Premium license.
// @ID get-groups
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param organization query string false "Only return groups in this organization. Accepts an organization ID or name."
// @Param has_member query string false "Only return groups that this user belongs to. Accepts a user ID or username."
// @Param group_ids query string false "Only return these groups, as a comma-separated list of group IDs."
// @Success 200 {array} codersdk.Group
// @Router /api/v2/groups [get]
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
		members, err := api.Database.GetGroupMembersByGroupID(ctx, database.GetGroupMembersByGroupIDParams{
			GroupID:       group.Group.ID,
			IncludeSystem: false,
		})
		if err != nil {
			httpapi.InternalServerError(rw, err)
			return
		}
		memberCount, err := api.Database.GetGroupMembersCountByGroupID(ctx, database.GetGroupMembersCountByGroupIDParams{
			GroupID:       group.Group.ID,
			IncludeSystem: false,
		})
		if err != nil {
			httpapi.InternalServerError(rw, err)
			return
		}

		resp = append(resp, db2sdk.Group(group, members, int(memberCount)))
	}

	httpapi.Write(ctx, rw, http.StatusOK, resp)
}

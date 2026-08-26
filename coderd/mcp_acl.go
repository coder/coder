package coderd

import (
	"context"
	"database/sql"
	"fmt"
	"maps"
	"net/http"
	"slices"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac/acl"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/searchquery"
	"github.com/coder/coder/v2/codersdk"
)

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
// @Summary Get MCP server config ACL
// @ID get-mcp-server-config-acl
// @Security CoderSessionToken
// @Tags MCP
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param mcpserverconfig path string true "MCP server config ID" format(uuid)
// @Success 200 {object} codersdk.MCPServerConfigACL
// @Router /api/experimental/organizations/{organization}/mcp-servers/{mcpserverconfig}/acl [get]
// @x-apidocgen {"skip": true}
func (api *API) mcpServerConfigACL(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	config := httpmw.MCPServerConfigParam(r)

	// The read gate admits every ACL-granted member, so gate ACL
	// enumeration on the same share permission that gates updates.
	if !api.Authorize(r, policy.ActionShare, config.RBACObject()) {
		httpapi.Forbidden(rw)
		return
	}

	users, ok := api.mcpServerConfigACLUsers(ctx, rw, config.UserACL)
	if !ok {
		return
	}
	groups, ok := api.mcpServerConfigACLGroups(ctx, rw, config.GroupACL)
	if !ok {
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, codersdk.MCPServerConfigACL{
		Users:  users,
		Groups: groups,
	})
}

// @Summary Get available MCP server config ACL users and groups
// @ID get-available-mcp-server-config-acl-users-and-groups
// @Security CoderSessionToken
// @Tags MCP
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param mcpserverconfig path string true "MCP server config ID" format(uuid)
// @Param q query string false "User search query; free-text search also applies to groups"
// @Param after_id query string false "User after ID" format(uuid)
// @Param limit query int false "Page limit for users and groups, if 0 returns all candidates"
// @Param offset query int false "User page offset"
// @Success 200 {object} codersdk.ACLAvailable
// @Router /api/v2/organizations/{organization}/mcp-servers/{mcpserverconfig}/acl/available [get]
// @x-apidocgen {"skip": true}
func (api *API) mcpServerConfigACLAvailable(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	config := httpmw.MCPServerConfigParam(r)
	if !api.Authorize(r, policy.ActionShare, config.RBACObject()) {
		httpapi.ResourceNotFound(rw)
		return
	}

	userFilter, validations := searchquery.Users(r.URL.Query().Get("q"))
	if len(validations) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid user search query.",
			Validations: validations,
		})
		return
	}
	pagination, ok := ParsePagination(rw, r)
	if !ok {
		return
	}

	//nolint:gocritic // The MCP server config share permission authorizes this
	// bounded organization-scoped lookup even when the caller cannot browse the
	// ordinary directories.
	restrictedCtx := dbauthz.AsSystemRestricted(ctx)
	members, err := api.Database.PaginatedOrganizationMembers(restrictedCtx, database.PaginatedOrganizationMembersParams{
		AfterID:          pagination.AfterID,
		OrganizationID:   config.OrganizationID,
		Search:           userFilter.Search,
		Name:             userFilter.Name,
		ExactUsername:    userFilter.ExactUsername,
		ExactEmail:       userFilter.ExactEmail,
		Status:           userFilter.Status,
		IsServiceAccount: userFilter.IsServiceAccount,
		RbacRole:         userFilter.RbacRole,
		LastSeenBefore:   userFilter.LastSeenBefore,
		LastSeenAfter:    userFilter.LastSeenAfter,
		CreatedAfter:     userFilter.CreatedAfter,
		CreatedBefore:    userFilter.CreatedBefore,
		GithubComUserID:  userFilter.GithubComUserID,
		LoginType:        userFilter.LoginType,
		IncludeSystem:    false,
		// #nosec G115 - Pagination offsets are small and fit in int32.
		OffsetOpt: int32(pagination.Offset),
		// #nosec G115 - Pagination limits are small and fit in int32.
		LimitOpt: int32(pagination.Limit),
	})
	if err != nil {
		httpapi.InternalServerError(rw, xerrors.Errorf("list MCP server config ACL users: %w", err))
		return
	}

	groups, err := api.Database.GetGroups(restrictedCtx, database.GetGroupsParams{
		OrganizationID: config.OrganizationID,
		Search:         userFilter.Search,
		// #nosec G115 - Pagination limits are small and fit in int32.
		LimitOpt: int32(pagination.Limit),
	})
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		httpapi.InternalServerError(rw, xerrors.Errorf("list MCP server config ACL groups: %w", err))
		return
	}

	groupIDs := make([]uuid.UUID, len(groups))
	for i, group := range groups {
		groupIDs[i] = group.Group.ID
	}
	countByGroup, ok := api.mcpServerConfigACLGroupMemberCounts(restrictedCtx, rw, groupIDs)
	if !ok {
		return
	}

	sdkUsers := make([]codersdk.ReducedUser, 0, len(members))
	for _, member := range members {
		sdkUsers = append(sdkUsers, mcpServerConfigACLReducedUser(member))
	}
	sdkGroups := make([]codersdk.Group, 0, len(groups))
	for _, group := range groups {
		sdkGroups = append(sdkGroups, db2sdk.Group(group, nil, int(countByGroup[group.Group.ID])))
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.ACLAvailable{
		Users:  sdkUsers,
		Groups: sdkGroups,
	})
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
// @Summary Update MCP server config ACL
// @ID update-mcp-server-config-acl
// @Security CoderSessionToken
// @Tags MCP
// @Accept json
// @Param organization path string true "Organization ID" format(uuid)
// @Param mcpserverconfig path string true "MCP server config ID" format(uuid)
// @Param request body codersdk.UpdateMCPServerConfigACLRequest true "Update MCP server config ACL request"
// @Success 204
// @Router /api/experimental/organizations/{organization}/mcp-servers/{mcpserverconfig}/acl [patch]
// @x-apidocgen {"skip": true}
func (api *API) patchMCPServerConfigACL(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	config := httpmw.MCPServerConfigParam(r)
	apiKey := httpmw.APIKey(r)
	auditor := api.Auditor.Load()
	aReq, commitAudit := audit.InitRequest[database.MCPServerConfig](rw, &audit.RequestParams{
		Audit:          *auditor,
		Log:            api.Logger,
		Request:        r,
		Action:         database.AuditActionWrite,
		OrganizationID: config.OrganizationID,
	})
	defer commitAudit()
	aReq.Old = config

	if !api.Authorize(r, policy.ActionShare, config.RBACObject()) {
		httpapi.Forbidden(rw)
		return
	}

	var req codersdk.UpdateMCPServerConfigACLRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	validations := acl.Validate(ctx, api.Database, MCPServerConfigACLUpdateValidator(req))
	validations = append(validations, api.validateMCPServerConfigACLOrganization(ctx, config.OrganizationID, req)...)
	userRoles, dupErrs := canonicalMCPServerConfigACLRoles("user_roles", req.UserRoles)
	validations = append(validations, dupErrs...)
	groupRoles, dupErrs := canonicalMCPServerConfigACLRoles("group_roles", req.GroupRoles)
	validations = append(validations, dupErrs...)
	if len(validations) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid request to update MCP server config ACL.",
			Validations: validations,
		})
		return
	}

	var updated database.MCPServerConfig
	err := api.Database.InTx(func(tx database.Store) error {
		//nolint:gocritic // The ACL write below reauthorizes the locked row for share.
		current, err := tx.GetMCPServerConfigByIDForUpdate(dbauthz.AsSystemRestricted(ctx), config.ID)
		if err != nil {
			return xerrors.Errorf("get MCP server config for update: %w", err)
		}
		aReq.Old = current
		userACL := maps.Clone(current.UserACL)
		groupACL := maps.Clone(current.GroupACL)
		applyMCPServerConfigACLRoles(userACL, userRoles)
		applyMCPServerConfigACLRoles(groupACL, groupRoles)
		if err := tx.UpdateMCPServerConfigACLByID(ctx, database.UpdateMCPServerConfigACLByIDParams{
			ID:        config.ID,
			UserACL:   userACL,
			GroupACL:  groupACL,
			UpdatedBy: apiKey.UserID,
		}); err != nil {
			return xerrors.Errorf("update MCP server config ACL: %w", err)
		}
		updated = current
		updated.UserACL = userACL
		updated.GroupACL = groupACL
		updated.UpdatedBy = uuid.NullUUID{UUID: apiKey.UserID, Valid: true}
		return nil
	}, nil)
	if err != nil {
		// A concurrent delete between the middleware fetch and the
		// locked re-fetch stays concealed as 404, matching the update
		// and delete handlers.
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.InternalServerError(rw, err)
		return
	}
	aReq.New = updated
	rw.WriteHeader(http.StatusNoContent)
}

func (api *API) mcpServerConfigACLUsers(ctx context.Context, rw http.ResponseWriter, entries database.ChatACL) ([]codersdk.MCPServerConfigUser, bool) {
	ids := parseMCPServerConfigACLIDs(entries)
	//nolint:gocritic // ACL managers may resolve principals after the share gate passes.
	users, err := api.Database.GetUsersByIDs(dbauthz.AsSystemRestricted(ctx), ids)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return nil, false
	}
	result := make([]codersdk.MCPServerConfigUser, 0, len(users))
	for _, user := range users {
		result = append(result, codersdk.MCPServerConfigUser{
			MinimalUser: db2sdk.MinimalUser(user),
			Role:        codersdk.MCPServerConfigRoleRead,
		})
	}
	return result, true
}

func (api *API) mcpServerConfigACLGroups(ctx context.Context, rw http.ResponseWriter, entries database.ChatACL) ([]codersdk.MCPServerConfigGroup, bool) {
	ids := parseMCPServerConfigACLIDs(entries)
	var groups []database.GetGroupsRow
	if len(ids) > 0 {
		var err error
		//nolint:gocritic // ACL managers may resolve principals after the share gate passes.
		groups, err = api.Database.GetGroups(dbauthz.AsSystemRestricted(ctx), database.GetGroupsParams{GroupIds: ids})
		if err != nil {
			httpapi.InternalServerError(rw, err)
			return nil, false
		}
	}
	groupIDs := make([]uuid.UUID, 0, len(groups))
	for _, group := range groups {
		groupIDs = append(groupIDs, group.Group.ID)
	}
	//nolint:gocritic // ACL managers may resolve group sizes after the share gate passes.
	countByGroup, ok := api.mcpServerConfigACLGroupMemberCounts(dbauthz.AsSystemRestricted(ctx), rw, groupIDs)
	if !ok {
		return nil, false
	}
	result := make([]codersdk.MCPServerConfigGroup, 0, len(groups))
	for _, group := range groups {
		result = append(result, codersdk.MCPServerConfigGroup{
			Group: db2sdk.Group(group, nil, int(countByGroup[group.Group.ID])),
			Role:  codersdk.MCPServerConfigRoleRead,
		})
	}
	return result, true
}

func (api *API) mcpServerConfigACLGroupMemberCounts(ctx context.Context, rw http.ResponseWriter, groupIDs []uuid.UUID) (map[uuid.UUID]int64, bool) {
	countByGroup := make(map[uuid.UUID]int64, len(groupIDs))
	if len(groupIDs) == 0 {
		return countByGroup, true
	}

	countRows, err := api.Database.GetGroupMembersCountByGroupIDs(ctx, database.GetGroupMembersCountByGroupIDsParams{
		GroupIds:      groupIDs,
		IncludeSystem: false,
	})
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		httpapi.InternalServerError(rw, xerrors.Errorf("count MCP server config ACL group members: %w", err))
		return nil, false
	}
	for _, row := range countRows {
		countByGroup[row.GroupID] = row.MemberCount
	}
	return countByGroup, true
}

func mcpServerConfigACLReducedUser(member database.PaginatedOrganizationMembersRow) codersdk.ReducedUser {
	return codersdk.ReducedUser{
		MinimalUser: codersdk.MinimalUser{
			ID:        member.OrganizationMember.UserID,
			Username:  member.Username,
			Name:      member.Name,
			AvatarURL: member.AvatarURL,
		},
		Email:            member.Email,
		CreatedAt:        member.UserCreatedAt,
		UpdatedAt:        member.UserUpdatedAt,
		LastSeenAt:       member.LastSeenAt,
		Status:           codersdk.UserStatus(member.Status),
		LoginType:        codersdk.LoginType(member.LoginType),
		IsServiceAccount: member.IsServiceAccount,
	}
}

// canonicalMCPServerConfigACLRoles rekeys the request map by canonical
// uuid.String() values so noncanonical spellings hit the same keys RBAC
// reads, and rejects requests where two spellings collapse to one
// principal because map order would decide which role wins. Unparsable
// keys are skipped; acl.Validate already reports them.
func canonicalMCPServerConfigACLRoles(field string, roles map[string]codersdk.MCPServerConfigRole) (map[string]codersdk.MCPServerConfigRole, []codersdk.ValidationError) {
	canonical := make(map[string]codersdk.MCPServerConfigRole, len(roles))
	var validErrs []codersdk.ValidationError
	for rawID, role := range roles {
		parsed, err := uuid.Parse(rawID)
		if err != nil {
			continue
		}
		id := parsed.String()
		if _, ok := canonical[id]; ok {
			validErrs = append(validErrs, codersdk.ValidationError{
				Field:  field,
				Detail: fmt.Sprintf("duplicate entries for ID %s", id),
			})
			continue
		}
		canonical[id] = role
	}
	return canonical, validErrs
}

func applyMCPServerConfigACLRoles(entries database.ChatACL, roles map[string]codersdk.MCPServerConfigRole) {
	for id, role := range roles {
		if role == codersdk.MCPServerConfigRoleDeleted {
			delete(entries, id)
			continue
		}
		entries[id] = database.ChatACLEntry{Permissions: []policy.Action{policy.ActionRead}}
	}
}

func parseMCPServerConfigACLIDs(entries database.ChatACL) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(entries))
	for rawID := range entries {
		if id, err := uuid.Parse(rawID); err == nil {
			ids = append(ids, id)
		}
	}
	return ids
}

func (api *API) validateMCPServerConfigACLOrganization(ctx context.Context, organizationID uuid.UUID, req codersdk.UpdateMCPServerConfigACLRequest) []codersdk.ValidationError {
	var validations []codersdk.ValidationError
	userIDs := activeMCPServerConfigACLIDs(req.UserRoles)
	if len(userIDs) > 0 {
		//nolint:gocritic // Principal validation requires organization membership visibility.
		memberships, err := api.Database.GetOrganizationIDsByMemberIDs(dbauthz.AsSystemRestricted(ctx), userIDs)
		if err != nil {
			return append(validations, codersdk.ValidationError{Field: "user_roles", Detail: err.Error()})
		}
		byUser := make(map[uuid.UUID][]uuid.UUID, len(memberships))
		for _, membership := range memberships {
			byUser[membership.UserID] = membership.OrganizationIDs
		}
		for _, id := range userIDs {
			if !slices.Contains(byUser[id], organizationID) {
				validations = append(validations, codersdk.ValidationError{
					Field:  "user_roles",
					Detail: "user " + id.String() + " does not belong to organization " + organizationID.String(),
				})
			}
		}
	}

	groupIDs := activeMCPServerConfigACLIDs(req.GroupRoles)
	if len(groupIDs) > 0 {
		//nolint:gocritic // Principal validation requires group organization visibility.
		groups, err := api.Database.GetGroups(dbauthz.AsSystemRestricted(ctx), database.GetGroupsParams{GroupIds: groupIDs})
		if err != nil {
			return append(validations, codersdk.ValidationError{Field: "group_roles", Detail: err.Error()})
		}
		for _, group := range groups {
			if group.Group.OrganizationID != organizationID {
				validations = append(validations, codersdk.ValidationError{
					Field:  "group_roles",
					Detail: "group " + group.Group.ID.String() + " does not belong to organization " + organizationID.String(),
				})
			}
		}
	}
	return validations
}

func activeMCPServerConfigACLIDs(roles map[string]codersdk.MCPServerConfigRole) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(roles))
	for rawID, role := range roles {
		if role == codersdk.MCPServerConfigRoleDeleted {
			continue
		}
		if id, err := uuid.Parse(rawID); err == nil {
			ids = append(ids, id)
		}
	}
	return ids
}

type MCPServerConfigACLUpdateValidator codersdk.UpdateMCPServerConfigACLRequest

var _ acl.UpdateValidator[codersdk.MCPServerConfigRole] = MCPServerConfigACLUpdateValidator{}

func (m MCPServerConfigACLUpdateValidator) Users() (map[string]codersdk.MCPServerConfigRole, string) {
	return m.UserRoles, "user_roles"
}

func (m MCPServerConfigACLUpdateValidator) Groups() (map[string]codersdk.MCPServerConfigRole, string) {
	return m.GroupRoles, "group_roles"
}

func (MCPServerConfigACLUpdateValidator) ValidateRole(role codersdk.MCPServerConfigRole) error {
	if role == codersdk.MCPServerConfigRoleDeleted || role == codersdk.MCPServerConfigRoleRead {
		return nil
	}
	return xerrors.Errorf("role %q is not a valid MCP server config role", role)
}

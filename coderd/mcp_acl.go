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

// @Summary Get MCP server config ACL
// @ID get-mcp-server-config-acl
// @Security CoderSessionToken
// @Tags MCP
// @Produce json
// @Param organization path string true "Organization name or ID"
// @Param mcpserverconfig path string true "MCP server config ID" format(uuid)
// @Success 200 {object} codersdk.MCPServerConfigACL
// @Router /api/v2/organizations/{organization}/mcp-servers/{mcpserverconfig}/acl [get]
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

	users, err := resolveACLUsers(ctx, api.Database, config.UserACL, func(user codersdk.MinimalUser) codersdk.MCPServerConfigUser {
		return codersdk.MCPServerConfigUser{MinimalUser: user, Role: codersdk.MCPServerConfigRoleRead}
	})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	groups, err := resolveACLGroups(ctx, api.Database, config.GroupACL, func(group codersdk.Group) codersdk.MCPServerConfigGroup {
		return codersdk.MCPServerConfigGroup{Group: group, Role: codersdk.MCPServerConfigRoleRead}
	})
	if err != nil {
		httpapi.InternalServerError(rw, err)
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
// @Param organization path string true "Organization name or ID"
// @Param mcpserverconfig path string true "MCP server config ID" format(uuid)
// @Param q query string false "User search query; free-text search also applies to groups"
// @Param after_id query string false "User after ID" format(uuid)
// @Param limit query int false "Page limit for users and groups, if 0 returns all candidates"
// @Param offset query int false "User page offset"
// @Success 200 {object} codersdk.ACLAvailable
// @Router /api/v2/organizations/{organization}/mcp-servers/{mcpserverconfig}/acl/available [get]
// @x-apidocgen {"skip": true}
func (api *API) mcpServerConfigACLAvailable(rw http.ResponseWriter, r *http.Request) {
	config := httpmw.MCPServerConfigParam(r)
	if !api.Authorize(r, policy.ActionShare, config.RBACObject()) {
		httpapi.ResourceNotFound(rw)
		return
	}

	api.writeOrganizationACLAvailable(rw, r, config.OrganizationID)
}

// writeOrganizationACLAvailable writes the organization members and groups that
// can be granted access to a resource in organizationID. Callers must have
// authorized share on that resource.
func (api *API) writeOrganizationACLAvailable(rw http.ResponseWriter, r *http.Request, organizationID uuid.UUID) {
	ctx := r.Context()
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

	//nolint:gocritic // The resource share permission checked by the caller
	// authorizes this bounded organization-scoped lookup even when the caller
	// cannot browse the ordinary directories.
	restrictedCtx := dbauthz.AsSystemRestricted(ctx)
	members, err := api.Database.PaginatedOrganizationMembers(restrictedCtx, database.PaginatedOrganizationMembersParams{
		AfterID:          pagination.AfterID,
		OrganizationID:   organizationID,
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
		httpapi.InternalServerError(rw, xerrors.Errorf("list ACL candidate users: %w", err))
		return
	}

	groups, err := api.Database.GetGroups(restrictedCtx, database.GetGroupsParams{
		OrganizationID: organizationID,
		Search:         userFilter.Search,
		// #nosec G115 - Pagination limits are small and fit in int32.
		LimitOpt: int32(pagination.Limit),
	})
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		httpapi.InternalServerError(rw, xerrors.Errorf("list ACL candidate groups: %w", err))
		return
	}

	groupIDs := make([]uuid.UUID, len(groups))
	for i, group := range groups {
		groupIDs[i] = group.Group.ID
	}
	countByGroup, err := aclGroupMemberCounts(restrictedCtx, api.Database, groupIDs)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	sdkUsers := make([]codersdk.ReducedUser, 0, len(members))
	for _, member := range members {
		sdkUsers = append(sdkUsers, reducedUserFromPaginatedOrganizationMember(member))
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

// @Summary Update MCP server config ACL
// @ID update-mcp-server-config-acl
// @Security CoderSessionToken
// @Tags MCP
// @Accept json
// @Param organization path string true "Organization name or ID"
// @Param mcpserverconfig path string true "MCP server config ID" format(uuid)
// @Param request body codersdk.UpdateMCPServerConfigACLRequest true "Update MCP server config ACL request"
// @Success 204
// @Router /api/v2/organizations/{organization}/mcp-servers/{mcpserverconfig}/acl [patch]
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
	userRoles, groupRoles, validations := validateOrganizationACLUpdate(ctx, api.Database, config.OrganizationID, mcpServerConfigACLUpdateValidator(req))
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
		applyACLReadRoles(userACL, userRoles)
		applyACLReadRoles(groupACL, groupRoles)
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

// resolveACLUsers resolves user ACL entries for callers that already passed
// the resource's share check.
func resolveACLUsers[T any](ctx context.Context, db database.Store, entries database.ChatACL, entry func(codersdk.MinimalUser) T) ([]T, error) {
	ids := parseACLIDs(entries)
	//nolint:gocritic // ACL managers may resolve principals after the share gate passes.
	users, err := db.GetUsersByIDs(dbauthz.AsSystemRestricted(ctx), ids)
	if err != nil {
		return nil, xerrors.Errorf("get ACL users: %w", err)
	}
	result := make([]T, 0, len(users))
	for _, user := range users {
		result = append(result, entry(db2sdk.MinimalUser(user)))
	}
	return result, nil
}

// resolveACLGroups resolves group ACL entries, including member counts, for
// callers that already passed the resource's share check.
func resolveACLGroups[T any](ctx context.Context, db database.Store, entries database.ChatACL, entry func(codersdk.Group) T) ([]T, error) {
	ids := parseACLIDs(entries)
	var groups []database.GetGroupsRow
	if len(ids) > 0 {
		var err error
		//nolint:gocritic // ACL managers may resolve principals after the share gate passes.
		groups, err = db.GetGroups(dbauthz.AsSystemRestricted(ctx), database.GetGroupsParams{GroupIds: ids})
		if err != nil {
			return nil, xerrors.Errorf("get ACL groups: %w", err)
		}
	}
	groupIDs := make([]uuid.UUID, 0, len(groups))
	for _, group := range groups {
		groupIDs = append(groupIDs, group.Group.ID)
	}
	//nolint:gocritic // ACL managers may resolve group sizes after the share gate passes.
	countByGroup, err := aclGroupMemberCounts(dbauthz.AsSystemRestricted(ctx), db, groupIDs)
	if err != nil {
		return nil, err
	}
	result := make([]T, 0, len(groups))
	for _, group := range groups {
		result = append(result, entry(db2sdk.Group(group, nil, int(countByGroup[group.Group.ID]))))
	}
	return result, nil
}

func aclGroupMemberCounts(ctx context.Context, db database.Store, groupIDs []uuid.UUID) (map[uuid.UUID]int64, error) {
	countByGroup := make(map[uuid.UUID]int64, len(groupIDs))
	if len(groupIDs) == 0 {
		return countByGroup, nil
	}

	countRows, err := db.GetGroupMembersCountByGroupIDs(ctx, database.GetGroupMembersCountByGroupIDsParams{
		GroupIds:      groupIDs,
		IncludeSystem: false,
	})
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		return nil, xerrors.Errorf("count ACL group members: %w", err)
	}
	for _, row := range countRows {
		countByGroup[row.GroupID] = row.MemberCount
	}
	return countByGroup, nil
}

// validateOrganizationACLUpdate validates a sparse ACL update for a resource
// in organizationID and returns its roles rekeyed by canonical UUID.
func validateOrganizationACLUpdate[R acl.Role](ctx context.Context, db database.Store, organizationID uuid.UUID, v acl.UpdateValidator[R]) (userRoles, groupRoles map[string]R, validations []codersdk.ValidationError) {
	users, usersField := v.Users()
	groups, groupsField := v.Groups()
	validations = acl.Validate(ctx, db, v)
	validations = append(validations, validateACLOrganization(ctx, db, organizationID, usersField, users, groupsField, groups)...)
	userRoles, dupErrs := canonicalACLRoles(usersField, users)
	validations = append(validations, dupErrs...)
	groupRoles, dupErrs = canonicalACLRoles(groupsField, groups)
	validations = append(validations, dupErrs...)
	return userRoles, groupRoles, validations
}

// canonicalACLRoles rekeys the request map by canonical uuid.String() values
// so noncanonical spellings hit the same keys RBAC reads, and rejects requests
// where two spellings collapse to one principal because map order would
// decide which role wins. Unparsable keys are skipped; acl.Validate already
// reports them.
func canonicalACLRoles[R acl.Role](field string, roles map[string]R) (map[string]R, []codersdk.ValidationError) {
	canonical := make(map[string]R, len(roles))
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

// applyACLReadRoles applies sparse roles to entries: the empty role removes an
// entry and every other validated role grants read.
func applyACLReadRoles[R acl.Role](entries database.ChatACL, roles map[string]R) {
	for id, role := range roles {
		if string(role) == "" {
			delete(entries, id)
			continue
		}
		entries[id] = database.ChatACLEntry{Permissions: []policy.Action{policy.ActionRead}}
	}
}

func parseACLIDs(entries database.ChatACL) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(entries))
	for rawID := range entries {
		if id, err := uuid.Parse(rawID); err == nil {
			ids = append(ids, id)
		}
	}
	return ids
}

func validateACLOrganization[R acl.Role](ctx context.Context, db database.Store, organizationID uuid.UUID, usersField string, userRoles map[string]R, groupsField string, groupRoles map[string]R) []codersdk.ValidationError {
	var validations []codersdk.ValidationError
	userIDs := activeACLIDs(userRoles)
	if len(userIDs) > 0 {
		//nolint:gocritic // Principal validation requires organization membership visibility.
		memberships, err := db.GetOrganizationIDsByMemberIDs(dbauthz.AsSystemRestricted(ctx), userIDs)
		if err != nil {
			return append(validations, codersdk.ValidationError{Field: usersField, Detail: err.Error()})
		}
		byUser := make(map[uuid.UUID][]uuid.UUID, len(memberships))
		for _, membership := range memberships {
			byUser[membership.UserID] = membership.OrganizationIDs
		}
		for _, id := range userIDs {
			if !slices.Contains(byUser[id], organizationID) {
				validations = append(validations, codersdk.ValidationError{
					Field:  usersField,
					Detail: "user " + id.String() + " does not belong to organization " + organizationID.String(),
				})
			}
		}
	}

	groupIDs := activeACLIDs(groupRoles)
	if len(groupIDs) > 0 {
		//nolint:gocritic // Principal validation requires group organization visibility.
		groups, err := db.GetGroups(dbauthz.AsSystemRestricted(ctx), database.GetGroupsParams{GroupIds: groupIDs})
		if err != nil {
			return append(validations, codersdk.ValidationError{Field: groupsField, Detail: err.Error()})
		}
		for _, group := range groups {
			if group.Group.OrganizationID != organizationID {
				validations = append(validations, codersdk.ValidationError{
					Field:  groupsField,
					Detail: "group " + group.Group.ID.String() + " does not belong to organization " + organizationID.String(),
				})
			}
		}
	}
	return validations
}

func activeACLIDs[R acl.Role](roles map[string]R) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(roles))
	for rawID, role := range roles {
		if string(role) == "" {
			continue
		}
		if id, err := uuid.Parse(rawID); err == nil {
			ids = append(ids, id)
		}
	}
	return ids
}

type mcpServerConfigACLUpdateValidator codersdk.UpdateMCPServerConfigACLRequest

var _ acl.UpdateValidator[codersdk.MCPServerConfigRole] = mcpServerConfigACLUpdateValidator{}

func (m mcpServerConfigACLUpdateValidator) Users() (map[string]codersdk.MCPServerConfigRole, string) {
	return m.UserRoles, "user_roles"
}

func (m mcpServerConfigACLUpdateValidator) Groups() (map[string]codersdk.MCPServerConfigRole, string) {
	return m.GroupRoles, "group_roles"
}

func (mcpServerConfigACLUpdateValidator) ValidateRole(role codersdk.MCPServerConfigRole) error {
	if role == codersdk.MCPServerConfigRoleDeleted || role == codersdk.MCPServerConfigRoleRead {
		return nil
	}
	return xerrors.Errorf("role %q is not a valid MCP server config role", role)
}

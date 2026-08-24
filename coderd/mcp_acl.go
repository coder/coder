package coderd

import (
	"context"
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
	"github.com/coder/coder/v2/codersdk"
)

// @Summary Get MCP server config ACL
// @ID get-mcp-server-config-acl
// @Security CoderSessionToken
// @Tags MCP
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param mcpserverconfig path string true "MCP server config ID" format(uuid)
// @Success 200 {object} codersdk.MCPServerConfigACL
// @Router /api/v2/organizations/{organization}/mcp-servers/{mcpserverconfig}/acl [get]
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

// @Summary Update MCP server config ACL
// @ID update-mcp-server-config-acl
// @Security CoderSessionToken
// @Tags MCP
// @Accept json
// @Param organization path string true "Organization ID" format(uuid)
// @Param mcpserverconfig path string true "MCP server config ID" format(uuid)
// @Param request body codersdk.UpdateMCPServerConfigACLRequest true "Update MCP server config ACL request"
// @Success 204
// @Router /api/v2/organizations/{organization}/mcp-servers/{mcpserverconfig}/acl [patch]
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
	countByGroup := make(map[uuid.UUID]int64, len(groups))
	if len(groups) > 0 {
		groupIDs := make([]uuid.UUID, 0, len(groups))
		for _, group := range groups {
			groupIDs = append(groupIDs, group.Group.ID)
		}
		//nolint:gocritic // ACL managers may resolve group sizes after the share gate passes.
		countRows, err := api.Database.GetGroupMembersCountByGroupIDs(dbauthz.AsSystemRestricted(ctx), database.GetGroupMembersCountByGroupIDsParams{
			GroupIds:      groupIDs,
			IncludeSystem: false,
		})
		if err != nil {
			httpapi.InternalServerError(rw, err)
			return nil, false
		}
		for _, row := range countRows {
			countByGroup[row.GroupID] = row.MemberCount
		}
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

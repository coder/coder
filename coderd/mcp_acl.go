package coderd

import (
	"context"
	"database/sql"
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

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
// @Summary Get MCP server config ACL
// @ID get-mcp-server-config-acl
// @Security CoderSessionToken
// @Tags MCP
// @Produce json
// @Param mcpserverconfig path string true "MCP server config ID" format(uuid)
// @Success 200 {object} codersdk.MCPServerConfigACL
// @Router /api/experimental/mcp-servers/{mcpserverconfig}/acl [get]
// @x-apidocgen {"skip": true}
func (api *API) mcpServerConfigACL(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	config := httpmw.MCPServerConfigParam(r)

	users, ok := api.mcpServerConfigACLUsers(ctx, rw, config.UserACL)
	if !ok {
		return
	}
	groups, ok := api.mcpServerConfigACLGroups(ctx, rw, config.GroupACL)
	if !ok {
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, codersdk.MCPServerConfigACL{Users: users, Groups: groups})
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
// @Summary Update MCP server config ACL
// @ID update-mcp-server-config-acl
// @Security CoderSessionToken
// @Tags MCP
// @Accept json
// @Param mcpserverconfig path string true "MCP server config ID" format(uuid)
// @Param request body codersdk.UpdateMCPServerConfigACLRequest true "Update MCP server config ACL request"
// @Success 204
// @Router /api/experimental/mcp-servers/{mcpserverconfig}/acl [patch]
// @x-apidocgen {"skip": true}
func (api *API) patchMCPServerConfigACL(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	config := httpmw.MCPServerConfigParam(r)
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
	if len(validations) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid request to update MCP server config ACL.",
			Validations: validations,
		})
		return
	}

	userACL := maps.Clone(config.UserACL)
	if userACL == nil {
		userACL = database.ChatACL{}
	}
	groupACL := maps.Clone(config.GroupACL)
	if groupACL == nil {
		groupACL = database.ChatACL{}
	}
	for id, role := range req.UserRoles {
		if role == codersdk.MCPServerConfigRoleDeleted {
			delete(userACL, id)
			continue
		}
		userACL[id] = database.ChatACLEntry{Permissions: []policy.Action{policy.ActionRead}}
	}
	for id, role := range req.GroupRoles {
		if role == codersdk.MCPServerConfigRoleDeleted {
			delete(groupACL, id)
			continue
		}
		groupACL[id] = database.ChatACLEntry{Permissions: []policy.Action{policy.ActionRead}}
	}

	if err := api.Database.UpdateMCPServerConfigACLByID(ctx, database.UpdateMCPServerConfigACLByIDParams{
		ID: config.ID, UserACL: userACL, GroupACL: groupACL,
	}); err != nil {
		if dbauthz.IsNotAuthorizedError(err) {
			httpapi.Forbidden(rw)
			return
		}
		httpapi.InternalServerError(rw, err)
		return
	}
	updated, err := api.Database.GetMCPServerConfigByID(ctx, config.ID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	aReq.New = updated
	rw.WriteHeader(http.StatusNoContent)
}

func (api *API) mcpServerConfigACLUsers(ctx context.Context, rw http.ResponseWriter, entries database.ChatACL) ([]codersdk.MCPServerConfigUser, bool) {
	ids := parseMCPServerConfigACLIDs(entries)
	//nolint:gocritic // ACL readers may resolve principals after the config read gate passes.
	users, err := api.Database.GetUsersByIDs(dbauthz.AsSystemRestricted(ctx), ids)
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		httpapi.InternalServerError(rw, err)
		return nil, false
	}
	result := make([]codersdk.MCPServerConfigUser, 0, len(users))
	for _, user := range users {
		result = append(result, codersdk.MCPServerConfigUser{MinimalUser: db2sdk.MinimalUser(user), Role: codersdk.MCPServerConfigRoleRead})
	}
	return result, true
}

func (api *API) mcpServerConfigACLGroups(ctx context.Context, rw http.ResponseWriter, entries database.ChatACL) ([]codersdk.MCPServerConfigGroup, bool) {
	ids := parseMCPServerConfigACLIDs(entries)
	var groups []database.GetGroupsRow
	if len(ids) > 0 {
		var err error
		//nolint:gocritic // ACL readers may resolve principals after the config read gate passes.
		groups, err = api.Database.GetGroups(dbauthz.AsSystemRestricted(ctx), database.GetGroupsParams{GroupIds: ids})
		if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
			httpapi.InternalServerError(rw, err)
			return nil, false
		}
	}
	result := make([]codersdk.MCPServerConfigGroup, 0, len(groups))
	for _, group := range groups {
		result = append(result, codersdk.MCPServerConfigGroup{Group: db2sdk.Group(group, nil, 0), Role: codersdk.MCPServerConfigRoleRead})
	}
	return result, true
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
		if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
			return append(validations, codersdk.ValidationError{Field: "user_roles", Detail: err.Error()})
		}
		byUser := make(map[uuid.UUID][]uuid.UUID, len(memberships))
		for _, membership := range memberships {
			byUser[membership.UserID] = membership.OrganizationIDs
		}
		for _, id := range userIDs {
			if !slices.Contains(byUser[id], organizationID) {
				validations = append(validations, codersdk.ValidationError{Field: "user_roles", Detail: "user " + id.String() + " does not belong to organization " + organizationID.String()})
			}
		}
	}

	groupIDs := activeMCPServerConfigACLIDs(req.GroupRoles)
	if len(groupIDs) > 0 {
		//nolint:gocritic // Principal validation requires group organization visibility.
		groups, err := api.Database.GetGroups(dbauthz.AsSystemRestricted(ctx), database.GetGroupsParams{GroupIds: groupIDs})
		if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
			return append(validations, codersdk.ValidationError{Field: "group_roles", Detail: err.Error()})
		}
		for _, group := range groups {
			if group.Group.OrganizationID != organizationID {
				validations = append(validations, codersdk.ValidationError{Field: "group_roles", Detail: "group " + group.Group.ID.String() + " does not belong to organization " + organizationID.String()})
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

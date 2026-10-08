package coderd

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"slices"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac/acl"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary Get chat project ACL
// @ID get-chat-project-acl
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param project path string true "Chat project ID" format(uuid)
// @Success 200 {object} codersdk.ChatProjectACL
// @Router /api/experimental/organizations/{organization}/chats/projects/{project}/acl [get]
// @x-apidocgen {"skip": true}
//
//nolint:revive // HTTP handler writes to ResponseWriter.
func (api *API) getChatProjectACL(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project := httpmw.ChatProjectParam(r)

	// The read check runs before allowChatSharing so the disabled response
	// does not reveal that a project exists.
	if !api.Authorize(r, policy.ActionRead, project.RBACObject()) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if !api.allowChatSharing(ctx, rw) {
		return
	}

	users, err := api.chatProjectACLUsers(ctx, project)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	groups, err := api.chatProjectACLGroups(ctx, project)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, codersdk.ChatProjectACL{
		Users:  users,
		Groups: groups,
	})
}

// @Summary Update chat project ACL
// @ID update-chat-project-acl
// @Security CoderSessionToken
// @Tags Chats
// @Accept json
// @Param organization path string true "Organization ID" format(uuid)
// @Param project path string true "Chat project ID" format(uuid)
// @Param request body codersdk.UpdateChatProjectACL true "Update chat project ACL request"
// @Success 204
// @Router /api/experimental/organizations/{organization}/chats/projects/{project}/acl [patch]
// @x-apidocgen {"skip": true}
func (api *API) patchChatProjectACL(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project := httpmw.ChatProjectParam(r)
	apiKey := httpmw.APIKey(r)

	aReq, commitAudit := audit.InitRequest[database.ChatProject](rw, &audit.RequestParams{
		Audit:          *api.Auditor.Load(),
		Log:            api.Logger,
		Request:        r,
		Action:         database.AuditActionWrite,
		OrganizationID: project.OrganizationID,
	})
	defer commitAudit()
	aReq.Old = project

	if !api.Authorize(r, policy.ActionRead, project.RBACObject()) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if !api.allowChatSharing(ctx, rw) {
		return
	}
	if !api.Authorize(r, policy.ActionShare, project.RBACObject()) {
		httpapi.Forbidden(rw)
		return
	}

	var req codersdk.UpdateChatProjectACL
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	validErrs := acl.Validate(ctx, api.Database, ChatProjectACLUpdateValidator(req))
	if len(validErrs) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid request to update chat project ACL.",
			Validations: validErrs,
		})
		return
	}
	// ACL keys must be canonical UUIDs: RBAC matches them as strings, so
	// any other spelling would grant nothing and could not be removed.
	userRoles, userKeyErrs := canonicalChatProjectRoles(req.UserRoles, "user_roles")
	groupRoles, groupKeyErrs := canonicalChatProjectRoles(req.GroupRoles, "group_roles")
	if len(userKeyErrs)+len(groupKeyErrs) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid request to update chat project ACL.",
			Validations: slices.Concat(userKeyErrs, groupKeyErrs),
		})
		return
	}

	if _, ok := userRoles[project.OwnerID]; ok {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "The project owner cannot be added to the project's sharing list.",
		})
		return
	}
	if _, ok := userRoles[apiKey.UserID]; ok {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Cannot change your own project sharing role.",
		})
		return
	}

	validErrs, err := api.validateChatProjectACLOrganization(ctx, project, userRoles, groupRoles)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	if len(validErrs) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid request to update chat project ACL.",
			Validations: validErrs,
		})
		return
	}

	err = api.Database.InTx(func(tx database.Store) error {
		current, err := tx.GetChatProjectByIDForUpdate(ctx, project.ID)
		if err != nil {
			return xerrors.Errorf("get chat project for update: %w", err)
		}
		userACL := applyChatProjectRoles(current.UserACL, userRoles)
		groupACL := applyChatProjectRoles(current.GroupACL, groupRoles)
		if err := tx.UpdateChatProjectACLByID(ctx, database.UpdateChatProjectACLByIDParams{
			ID:       project.ID,
			UserACL:  userACL,
			GroupACL: groupACL,
		}); err != nil {
			return xerrors.Errorf("update chat project ACL: %w", err)
		}
		updated, err := tx.GetChatProjectByID(ctx, project.ID)
		if err != nil {
			return xerrors.Errorf("get updated chat project: %w", err)
		}
		aReq.New = updated
		return nil
	}, nil)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.ResourceNotFound(rw)
			return
		}
		if dbauthz.IsNotAuthorizedError(err) {
			httpapi.Forbidden(rw)
			return
		}
		httpapi.InternalServerError(rw, err)
		return
	}

	rw.WriteHeader(http.StatusNoContent)
}

// canonicalChatProjectRoles parses ACL keys into UUIDs. A key that is not a
// UUID, or two spellings of one UUID, is a validation error; with two
// spellings, which role wins would depend on map order.
func canonicalChatProjectRoles(roles map[string]codersdk.ChatProjectRole, field string) (map[uuid.UUID]codersdk.ChatProjectRole, []codersdk.ValidationError) {
	canonical := make(map[uuid.UUID]codersdk.ChatProjectRole, len(roles))
	var validErrs []codersdk.ValidationError
	for rawID, role := range roles {
		id, err := uuid.Parse(rawID)
		if err != nil {
			validErrs = append(validErrs, codersdk.ValidationError{
				Field:  field,
				Detail: fmt.Sprintf("%q is not a valid UUID", rawID),
			})
			continue
		}
		if _, ok := canonical[id]; ok {
			validErrs = append(validErrs, codersdk.ValidationError{
				Field:  field,
				Detail: fmt.Sprintf("ID %v is listed more than once under different spellings", id),
			})
			continue
		}
		canonical[id] = role
	}
	return canonical, validErrs
}

func applyChatProjectRoles(current database.ChatACL, roles map[uuid.UUID]codersdk.ChatProjectRole) database.ChatACL {
	next := make(database.ChatACL, len(current)+len(roles))
	for id, entry := range current {
		next[id] = entry
	}
	for id, role := range roles {
		if role == codersdk.ChatProjectRoleDeleted {
			delete(next, id.String())
			continue
		}
		next[id.String()] = database.ChatACLEntry{Permissions: db2sdk.ChatProjectRoleActions(role)}
	}
	return next
}

// validateChatProjectACLOrganization rejects users and groups outside the
// project's organization. RBAC applies grants only to members of the
// project's organization, so a foreign user would be listed but get no
// access, and a foreign group would grant access to those of its members
// who also belong to the project's organization.
func (api *API) validateChatProjectACLOrganization(
	ctx context.Context,
	project database.ChatProject,
	userRoles, groupRoles map[uuid.UUID]codersdk.ChatProjectRole,
) ([]codersdk.ValidationError, error) {
	//nolint:gocritic // Validation needs every requested user and group, even ones the caller cannot read.
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	var validErrs []codersdk.ValidationError

	userIDs := grantedChatProjectIDs(userRoles)
	if len(userIDs) > 0 {
		memberships, err := api.Database.GetOrganizationIDsByMemberIDs(sysCtx, userIDs)
		if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
			return nil, xerrors.Errorf("get organization memberships: %w", err)
		}
		inOrg := make(map[uuid.UUID]bool, len(memberships))
		for _, membership := range memberships {
			inOrg[membership.UserID] = slices.Contains(membership.OrganizationIDs, project.OrganizationID)
		}
		for _, id := range userIDs {
			if !inOrg[id] {
				validErrs = append(validErrs, codersdk.ValidationError{
					Field:  "user_roles",
					Detail: fmt.Sprintf("user with ID %v is not a member of the project's organization", id),
				})
			}
		}
	}

	groupIDs := grantedChatProjectIDs(groupRoles)
	if len(groupIDs) > 0 {
		groups, err := api.Database.GetGroups(sysCtx, database.GetGroupsParams{GroupIds: groupIDs})
		if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
			return nil, xerrors.Errorf("get groups: %w", err)
		}
		for _, group := range groups {
			if group.Group.OrganizationID != project.OrganizationID {
				validErrs = append(validErrs, codersdk.ValidationError{
					Field:  "group_roles",
					Detail: fmt.Sprintf("group with ID %v is not in the project's organization", group.Group.ID),
				})
			}
		}
	}
	return validErrs, nil
}

// Removals are excluded so entries for deleted principals can still be
// removed.
func grantedChatProjectIDs(roles map[uuid.UUID]codersdk.ChatProjectRole) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(roles))
	for id, role := range roles {
		if role != codersdk.ChatProjectRoleDeleted {
			ids = append(ids, id)
		}
	}
	return ids
}

func (api *API) chatProjectACLUsers(ctx context.Context, project database.ChatProject) ([]codersdk.ChatProjectUser, error) {
	userIDs := make([]uuid.UUID, 0, len(project.UserACL))
	for rawID := range project.UserACL {
		id, err := uuid.Parse(rawID)
		if err != nil {
			api.Logger.Warn(ctx, "found invalid user uuid in chat project acl", slog.Error(err), slog.F("project_id", project.ID))
			continue
		}
		userIDs = append(userIDs, id)
	}
	if len(userIDs) == 0 {
		return []codersdk.ChatProjectUser{}, nil
	}

	//nolint:gocritic // Users who can read the project should see who it is shared with even without user read permission.
	dbUsers, err := api.Database.GetUsersByIDs(dbauthz.AsSystemRestricted(ctx), userIDs)
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		return nil, xerrors.Errorf("get users: %w", err)
	}
	users := make([]codersdk.ChatProjectUser, 0, len(dbUsers))
	for _, user := range dbUsers {
		users = append(users, codersdk.ChatProjectUser{
			MinimalUser: db2sdk.MinimalUser(user),
			Role:        convertToChatProjectRole(project.UserACL[user.ID.String()].Permissions),
		})
	}
	return users, nil
}

func (api *API) chatProjectACLGroups(ctx context.Context, project database.ChatProject) ([]codersdk.ChatProjectGroup, error) {
	groupIDs := make([]uuid.UUID, 0, len(project.GroupACL))
	for rawID := range project.GroupACL {
		id, err := uuid.Parse(rawID)
		if err != nil {
			api.Logger.Warn(ctx, "found invalid group uuid in chat project acl", slog.Error(err), slog.F("project_id", project.ID))
			continue
		}
		groupIDs = append(groupIDs, id)
	}
	if len(groupIDs) == 0 {
		return []codersdk.ChatProjectGroup{}, nil
	}

	//nolint:gocritic // Users who can read the project should see the groups it is shared with even without group read permission.
	dbGroups, err := api.Database.GetGroups(dbauthz.AsSystemRestricted(ctx), database.GetGroupsParams{GroupIds: groupIDs})
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		return nil, xerrors.Errorf("get groups: %w", err)
	}
	groups := make([]codersdk.ChatProjectGroup, 0, len(dbGroups))
	for _, group := range dbGroups {
		//nolint:gocritic // Users who can read the project should see shared group sizes even without group read permission.
		memberCount, err := api.Database.GetGroupMembersCountByGroupID(dbauthz.AsSystemRestricted(ctx), database.GetGroupMembersCountByGroupIDParams{
			GroupID:       group.Group.ID,
			IncludeSystem: false,
		})
		if err != nil {
			return nil, xerrors.Errorf("count group members: %w", err)
		}
		groups = append(groups, codersdk.ChatProjectGroup{
			Group: db2sdk.Group(group, nil, int(memberCount)),
			Role:  convertToChatProjectRole(project.GroupACL[group.Group.ID.String()].Permissions),
		})
	}
	return groups, nil
}

type ChatProjectACLUpdateValidator codersdk.UpdateChatProjectACL

var _ acl.UpdateValidator[codersdk.ChatProjectRole] = ChatProjectACLUpdateValidator{}

func (v ChatProjectACLUpdateValidator) Users() (map[string]codersdk.ChatProjectRole, string) {
	return v.UserRoles, "user_roles"
}

func (v ChatProjectACLUpdateValidator) Groups() (map[string]codersdk.ChatProjectRole, string) {
	return v.GroupRoles, "group_roles"
}

func (ChatProjectACLUpdateValidator) ValidateRole(role codersdk.ChatProjectRole) error {
	switch role {
	case codersdk.ChatProjectRoleDeleted, codersdk.ChatProjectRoleUse, codersdk.ChatProjectRoleAdmin:
		return nil
	}
	return xerrors.Errorf("role %q is not a valid chat project role", role)
}

func convertToChatProjectRole(actions []policy.Action) codersdk.ChatProjectRole {
	for _, role := range []codersdk.ChatProjectRole{codersdk.ChatProjectRoleAdmin, codersdk.ChatProjectRoleUse} {
		if slice.SameElements(actions, db2sdk.ChatProjectRoleActions(role)) {
			return role
		}
	}
	return codersdk.ChatProjectRoleDeleted
}

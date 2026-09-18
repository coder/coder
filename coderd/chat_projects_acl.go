package coderd

import (
	"context"
	"database/sql"
	"errors"
	"maps"
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
	"github.com/coder/coder/v2/coderd/notifications"
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
// @Param project path string true "Chat project ID" format(uuid)
// @Success 200 {object} codersdk.ChatProjectACL
// @Router /api/experimental/chats/projects/{project}/acl [get]
// @x-apidocgen {"skip": true}
//
//nolint:revive // get-return: revive assumes get* must be a getter, but this is an HTTP handler.
func (api *API) getChatProjectACL(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project := httpmw.ChatProjectParam(r)

	if !api.allowChatSharing(ctx, rw) {
		return
	}

	projectACL, err := api.Database.GetChatProjectACLByID(ctx, project.ID)
	if err != nil {
		if dbauthz.IsNotAuthorizedError(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.InternalServerError(rw, err)
		return
	}

	users, ok := api.chatProjectACLUsers(ctx, rw, project, projectACL.Users)
	if !ok {
		return
	}
	groups, ok := api.chatProjectACLGroups(ctx, rw, project, projectACL.Groups)
	if !ok {
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, codersdk.ChatProjectACL{Users: users, Groups: groups})
}

// @Summary Update chat project ACL
// @ID update-chat-project-acl
// @Security CoderSessionToken
// @Tags Chats
// @Accept json
// @Param project path string true "Chat project ID" format(uuid)
// @Param request body codersdk.UpdateChatProjectACL true "Update chat project ACL request"
// @Success 204
// @Router /api/experimental/chats/projects/{project}/acl [patch]
// @x-apidocgen {"skip": true}
func (api *API) patchChatProjectACL(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project := httpmw.ChatProjectParam(r)
	auditor := api.Auditor.Load()
	aReq, commitAudit := audit.InitRequest[database.ChatProject](rw, &audit.RequestParams{
		Audit:          *auditor,
		Log:            api.Logger,
		Request:        r,
		Action:         database.AuditActionWrite,
		OrganizationID: project.OrganizationID,
	})
	defer commitAudit()
	aReq.Old = project

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

	apiKey := httpmw.APIKey(r)
	for userID := range req.UserRoles {
		parsed, err := uuid.Parse(userID)
		if err == nil && parsed == apiKey.UserID {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Cannot change your own project sharing role.",
			})
			return
		}
	}

	validErrs := acl.Validate(ctx, api.Database, ChatProjectACLUpdateValidator(req))
	if len(validErrs) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid request to update chat project ACL.",
			Validations: validErrs,
		})
		return
	}

	var oldProject database.ChatProject
	err := api.Database.InTx(func(tx database.Store) error {
		current, err := tx.GetChatProjectByIDForUpdate(ctx, project.ID)
		if err != nil {
			return xerrors.Errorf("get chat project for update: %w", err)
		}
		if current.UserACL == nil {
			current.UserACL = database.ChatACL{}
		}
		if current.GroupACL == nil {
			current.GroupACL = database.ChatACL{}
		}
		oldProject = current
		oldProject.UserACL = maps.Clone(current.UserACL)

		for id, role := range req.UserRoles {
			if role == codersdk.ChatProjectRoleDeleted {
				delete(current.UserACL, id)
				continue
			}
			current.UserACL[id] = database.ChatACLEntry{Permissions: db2sdk.ChatProjectRoleActions(role)}
		}
		for id, role := range req.GroupRoles {
			if role == codersdk.ChatProjectRoleDeleted {
				delete(current.GroupACL, id)
				continue
			}
			current.GroupACL[id] = database.ChatACLEntry{Permissions: db2sdk.ChatProjectRoleActions(role)}
		}

		if err := tx.UpdateChatProjectACLByID(ctx, database.UpdateChatProjectACLByIDParams{
			ID:       project.ID,
			UserACL:  current.UserACL,
			GroupACL: current.GroupACL,
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
		if dbauthz.IsNotAuthorizedError(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.InternalServerError(rw, err)
		return
	}

	initiator, err := api.Database.GetUserByID(ctx, apiKey.UserID)
	if err != nil {
		api.Logger.Warn(ctx, "failed to load chat project share initiator", slog.Error(err), slog.F("project_id", project.ID))
	} else {
		newProject := aReq.New
		go func() {
			if count, err := api.notifyChatProjectShared(oldProject, newProject, initiator); err != nil {
				api.Logger.Warn(api.ctx, "failed to enqueue one or more chat project shared notifications", slog.Error(err), slog.F("project_id", newProject.ID), slog.F("attempted_recipients", count))
			}
		}()
	}

	rw.WriteHeader(http.StatusNoContent)
}

func (api *API) notifyChatProjectShared(oldProject, newProject database.ChatProject, initiator database.User) (int, error) {
	added, _ := slice.SymmetricDifference(directChatProjectReaders(oldProject), directChatProjectReaders(newProject))
	recipientIDs := make([]uuid.UUID, 0, len(added))
	for _, userID := range added {
		if userID != initiator.ID {
			recipientIDs = append(recipientIDs, userID)
		}
	}
	if len(recipientIDs) == 0 {
		return 0, nil
	}

	labels := map[string]string{
		"project_id":   newProject.ID.String(),
		"project_name": newProject.Name,
		"initiator":    initiator.Username,
	}
	//nolint:gocritic // Notifier actor is required to enqueue notifications.
	notifierCtx := dbauthz.AsNotifier(api.ctx)
	var errs []error
	for _, userID := range recipientIDs {
		if _, err := api.NotificationsEnqueuer.Enqueue(notifierCtx, userID, notifications.TemplateChatProjectShared, labels, initiator.ID.String(), newProject.ID); err != nil {
			errs = append(errs, xerrors.Errorf("enqueue chat project shared notification: %w", err))
		}
	}
	return len(recipientIDs), errors.Join(errs...)
}

// directChatProjectReaders lists users granted read directly, which is who a
// share notification goes to. Group members are not resolved.
func directChatProjectReaders(project database.ChatProject) []uuid.UUID {
	readers := []uuid.UUID{project.CreatedBy}
	for rawUserID, entry := range project.UserACL {
		if !slices.Contains(entry.Permissions, policy.ActionRead) {
			continue
		}
		if userID, err := uuid.Parse(rawUserID); err == nil {
			readers = append(readers, userID)
		}
	}
	return slice.Unique(readers)
}

func (api *API) chatProjectACLUsers(ctx context.Context, rw http.ResponseWriter, project database.ChatProject, entries database.ChatACL) ([]codersdk.ChatProjectUser, bool) {
	userIDs := make([]uuid.UUID, 0, len(entries))
	for userID := range entries {
		id, err := uuid.Parse(userID)
		if err != nil {
			api.Logger.Warn(ctx, "found invalid user uuid in chat project acl", slog.Error(err), slog.F("project_id", project.ID))
			continue
		}
		userIDs = append(userIDs, id)
	}

	//nolint:gocritic // Users who can read the project ACL should see shared users even without user read permission.
	dbUsers, err := api.Database.GetUsersByIDs(dbauthz.AsSystemRestricted(ctx), userIDs)
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		httpapi.InternalServerError(rw, err)
		return nil, false
	}

	users := make([]codersdk.ChatProjectUser, 0, len(dbUsers))
	for _, user := range dbUsers {
		users = append(users, codersdk.ChatProjectUser{
			MinimalUser: db2sdk.MinimalUser(user),
			Role:        convertToChatProjectRole(entries[user.ID.String()].Permissions),
		})
	}
	return users, true
}

func (api *API) chatProjectACLGroups(ctx context.Context, rw http.ResponseWriter, project database.ChatProject, entries database.ChatACL) ([]codersdk.ChatProjectGroup, bool) {
	groupIDs := make([]uuid.UUID, 0, len(entries))
	for groupID := range entries {
		id, err := uuid.Parse(groupID)
		if err != nil {
			api.Logger.Warn(ctx, "found invalid group uuid in chat project acl", slog.Error(err), slog.F("project_id", project.ID))
			continue
		}
		groupIDs = append(groupIDs, id)
	}

	dbGroups := make([]database.GetGroupsRow, 0)
	if len(groupIDs) > 0 {
		var err error
		//nolint:gocritic // Users who can read the project ACL should see shared groups even without group read permission.
		dbGroups, err = api.Database.GetGroups(dbauthz.AsSystemRestricted(ctx), database.GetGroupsParams{GroupIds: groupIDs})
		if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
			httpapi.InternalServerError(rw, err)
			return nil, false
		}
	}

	groups := make([]codersdk.ChatProjectGroup, 0, len(dbGroups))
	for _, group := range dbGroups {
		//nolint:gocritic // Users who can read the project ACL should see shared group sizes even without group read permission.
		memberCount, err := api.Database.GetGroupMembersCountByGroupID(dbauthz.AsSystemRestricted(ctx), database.GetGroupMembersCountByGroupIDParams{
			GroupID:       group.Group.ID,
			IncludeSystem: false,
		})
		if err != nil {
			httpapi.InternalServerError(rw, err)
			return nil, false
		}
		groups = append(groups, codersdk.ChatProjectGroup{
			Group: db2sdk.Group(group, nil, int(memberCount)),
			Role:  convertToChatProjectRole(entries[group.Group.ID.String()].Permissions),
		})
	}
	return groups, true
}

type ChatProjectACLUpdateValidator codersdk.UpdateChatProjectACL

var _ acl.UpdateValidator[codersdk.ChatProjectRole] = ChatProjectACLUpdateValidator{}

func (c ChatProjectACLUpdateValidator) Users() (map[string]codersdk.ChatProjectRole, string) {
	return c.UserRoles, "user_roles"
}

func (c ChatProjectACLUpdateValidator) Groups() (map[string]codersdk.ChatProjectRole, string) {
	return c.GroupRoles, "group_roles"
}

func (ChatProjectACLUpdateValidator) ValidateRole(role codersdk.ChatProjectRole) error {
	if role == codersdk.ChatProjectRoleDeleted || role == codersdk.ChatProjectRoleRead {
		return nil
	}
	return xerrors.Errorf("role %q is not a valid chat project role", role)
}

func convertToChatProjectRole(actions []policy.Action) codersdk.ChatProjectRole {
	if slice.SameElements(actions, db2sdk.ChatProjectRoleActions(codersdk.ChatProjectRoleRead)) {
		return codersdk.ChatProjectRoleRead
	}
	return codersdk.ChatProjectRoleDeleted
}

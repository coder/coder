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

	slog "cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/acl"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"
)

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
// @Summary Get chat ACLs
// @ID get-chat-acls
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Param chat path string true "Chat ID" format(uuid)
// @Success 200 {object} codersdk.ChatACL
// @Router /api/experimental/chats/{chat}/acl [get]
// @x-apidocgen {"skip": true}
// @Description Experimental: this endpoint is subject to change.
//
//nolint:revive // get-return: revive assumes get* must be a getter, but this is an HTTP handler.
func (api *API) getChatACL(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	chat := httpmw.ChatParam(r)

	if !api.allowChatSharing(ctx, rw) {
		return
	}
	if chat.IsSubChat() {
		resp := codersdk.Response{Message: "Chat ACLs can only be set on root chats."}
		if chat.RootChatID.Valid {
			resp.Detail = "Target the root chat (id: " + chat.RootChatID.UUID.String() + ") instead."
		}
		httpapi.Write(ctx, rw, http.StatusBadRequest, resp)
		return
	}

	chatACL, err := api.Database.GetChatACLByID(ctx, chat.ID)
	if err != nil {
		if dbauthz.IsNotAuthorizedError(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.InternalServerError(rw, err)
		return
	}

	users, ok := api.chatACLUsers(ctx, rw, chat, chatACL.Users)
	if !ok {
		return
	}
	groups, ok := api.chatACLGroups(ctx, rw, chat, chatACL.Groups)
	if !ok {
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.ChatACL{
		Users:  users,
		Groups: groups,
	})
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
// @Summary Update chat ACL
// @ID update-chat-acl
// @Security CoderSessionToken
// @Tags Chats
// @Accept json
// @Param chat path string true "Chat ID" format(uuid)
// @Param request body codersdk.UpdateChatACL true "Update chat ACL request"
// @Success 204
// @Router /api/experimental/chats/{chat}/acl [patch]
// @x-apidocgen {"skip": true}
// @Description Experimental: this endpoint is subject to change.
func (api *API) patchChatACL(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	chat := httpmw.ChatParam(r)
	auditor := api.Auditor.Load()
	aReq, commitAudit := audit.InitRequest[database.Chat](rw, &audit.RequestParams{
		Audit:          *auditor,
		Log:            api.Logger,
		Request:        r,
		Action:         database.AuditActionWrite,
		OrganizationID: chat.OrganizationID,
	})
	defer commitAudit()
	aReq.Old = chat

	if !api.allowChatSharing(ctx, rw) {
		return
	}
	if chat.IsSubChat() {
		resp := codersdk.Response{Message: "Chat ACLs can only be set on root chats."}
		if chat.RootChatID.Valid {
			resp.Detail = "Target the root chat (id: " + chat.RootChatID.UUID.String() + ") instead."
		}
		httpapi.Write(ctx, rw, http.StatusBadRequest, resp)
		return
	}
	if !api.Authorize(r, policy.ActionShare, chat.RBACObject()) {
		httpapi.Forbidden(rw)
		return
	}

	var req codersdk.UpdateChatACL
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	apiKey := httpmw.APIKey(r)
	for userID := range req.UserRoles {
		parsed, err := uuid.Parse(userID)
		if err == nil && parsed == apiKey.UserID {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Cannot change your own chat sharing role.",
			})
			return
		}
	}

	validErrs := acl.Validate(ctx, api.Database, ChatACLUpdateValidator(req))
	if len(validErrs) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid request to update chat ACL.",
			Validations: validErrs,
		})
		return
	}

	var oldChat database.Chat
	err := api.Database.InTx(func(tx database.Store) error {
		current, err := tx.GetChatByIDForUpdate(ctx, chat.ID)
		if err != nil {
			return xerrors.Errorf("get chat by ID: %w", err)
		}
		if current.UserACL == nil {
			current.UserACL = database.ChatACL{}
		}
		if current.GroupACL == nil {
			current.GroupACL = database.ChatACL{}
		}
		oldChat = current
		oldChat.UserACL = maps.Clone(current.UserACL)

		for id, role := range req.UserRoles {
			if role == codersdk.ChatRoleDeleted {
				delete(current.UserACL, id)
				continue
			}
			current.UserACL[id] = database.ChatACLEntry{
				Permissions: db2sdk.ChatRoleActions(role),
			}
		}
		for id, role := range req.GroupRoles {
			if role == codersdk.ChatRoleDeleted {
				delete(current.GroupACL, id)
				continue
			}
			current.GroupACL[id] = database.ChatACLEntry{
				Permissions: db2sdk.ChatRoleActions(role),
			}
		}

		if err := tx.UpdateChatACLByID(ctx, database.UpdateChatACLByIDParams{
			ID:       chat.ID,
			UserACL:  current.UserACL,
			GroupACL: current.GroupACL,
		}); err != nil {
			return xerrors.Errorf("update chat ACL: %w", err)
		}
		updatedChat, err := tx.GetChatByID(ctx, chat.ID)
		if err != nil {
			return xerrors.Errorf("get updated chat by ID: %w", err)
		}
		aReq.New = updatedChat
		return nil
	}, nil)
	if err != nil {
		if dbauthz.IsNotAuthorizedError(err) {
			httpapi.Forbidden(rw)
			return
		}
		httpapi.InternalServerError(rw, err)
		return
	}

	initiator, err := api.Database.GetUserByID(ctx, apiKey.UserID)
	if err != nil {
		api.Logger.Warn(ctx, "failed to load chat share initiator", slog.Error(err), slog.F("chat_id", chat.ID))
	} else {
		newChat := aReq.New
		go func() {
			if count, err := api.notifyChatShared(oldChat, newChat, initiator); err != nil {
				api.Logger.Warn(api.ctx, "failed to enqueue one or more chat shared notifications", slog.Error(err), slog.F("chat_id", newChat.ID), slog.F("attempted_recipients", count))
			}
		}()
	}

	rw.WriteHeader(http.StatusNoContent)
}

func (api *API) notifyChatShared(oldChat database.Chat, newChat database.Chat, initiator database.User) (int, error) {
	oldReaders := api.directChatReaders(oldChat)
	newReaders := api.directChatReaders(newChat)

	added, _ := slice.SymmetricDifference(oldReaders, newReaders)
	recipientIDs := make([]uuid.UUID, 0, len(added))
	for _, userID := range added {
		if userID == initiator.ID {
			continue
		}
		recipientIDs = append(recipientIDs, userID)
	}
	if len(recipientIDs) == 0 {
		return 0, nil
	}

	labels := map[string]string{
		"chat_id":    newChat.ID.String(),
		"chat_title": newChat.Title,
		"initiator":  initiator.Username,
	}

	//nolint:gocritic // Notifier actor is required to enqueue notifications.
	notifierCtx := dbauthz.AsNotifier(api.ctx)
	var errs []error
	for _, userID := range recipientIDs {
		if _, err := api.NotificationsEnqueuer.Enqueue(notifierCtx, userID, notifications.TemplateChatShared, labels, initiator.ID.String(), newChat.ID); err != nil {
			errs = append(errs, xerrors.Errorf("enqueue chat shared notification: %w", err))
		}
	}
	return len(recipientIDs), errors.Join(errs...)
}

func (api *API) directChatReaders(chat database.Chat) []uuid.UUID {
	readers := []uuid.UUID{chat.OwnerID}
	for rawUserID, entry := range chat.UserACL {
		if !slices.Contains(entry.Permissions, policy.ActionRead) {
			continue
		}
		userID, err := uuid.Parse(rawUserID)
		if err != nil {
			api.Logger.Warn(api.ctx, "skip chat ACL entry with invalid user UUID", slog.F("chat_id", chat.ID), slog.F("user_id", rawUserID), slog.Error(err))
			continue
		}
		readers = append(readers, userID)
	}
	return slice.Unique(readers)
}

func (api *API) chatACLUsers(ctx context.Context, rw http.ResponseWriter, chat database.Chat, entries database.ChatACL) ([]codersdk.ChatUser, bool) {
	userIDs := make([]uuid.UUID, 0, len(entries))
	for userID := range entries {
		id, err := uuid.Parse(userID)
		if err != nil {
			api.Logger.Warn(ctx, "found invalid user uuid in chat acl", slog.Error(err), slog.F("chat_id", chat.ID))
			continue
		}
		userIDs = append(userIDs, id)
	}

	//nolint:gocritic // Users who can read the chat ACL should see shared users even without user read permission.
	dbUsers, err := api.Database.GetUsersByIDs(dbauthz.AsSystemRestricted(ctx), userIDs)
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		httpapi.InternalServerError(rw, err)
		return nil, false
	}

	users := make([]codersdk.ChatUser, 0, len(dbUsers))
	for _, user := range dbUsers {
		entry := entries[user.ID.String()]
		users = append(users, codersdk.ChatUser{
			MinimalUser: db2sdk.MinimalUser(user),
			Role:        convertToChatRole(entry.Permissions),
		})
	}
	return users, true
}

func (api *API) chatACLGroups(ctx context.Context, rw http.ResponseWriter, chat database.Chat, entries database.ChatACL) ([]codersdk.ChatGroup, bool) {
	groupIDs := make([]uuid.UUID, 0, len(entries))
	for groupID := range entries {
		id, err := uuid.Parse(groupID)
		if err != nil {
			api.Logger.Warn(ctx, "found invalid group uuid in chat acl", slog.Error(err), slog.F("chat_id", chat.ID))
			continue
		}
		groupIDs = append(groupIDs, id)
	}

	dbGroups := make([]database.GetGroupsRow, 0)
	if len(groupIDs) > 0 {
		var err error
		//nolint:gocritic // Users who can read the chat ACL should see shared groups even without group read permission.
		dbGroups, err = api.Database.GetGroups(dbauthz.AsSystemRestricted(ctx), database.GetGroupsParams{GroupIds: groupIDs})
		if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
			httpapi.InternalServerError(rw, err)
			return nil, false
		}
	}

	groups := make([]codersdk.ChatGroup, 0, len(dbGroups))
	for _, group := range dbGroups {
		//nolint:gocritic // Users who can read the chat ACL should see shared group sizes even without group read permission.
		memberCount, err := api.Database.GetGroupMembersCountByGroupID(dbauthz.AsSystemRestricted(ctx), database.GetGroupMembersCountByGroupIDParams{
			GroupID:       group.Group.ID,
			IncludeSystem: false,
		})
		if err != nil {
			httpapi.InternalServerError(rw, err)
			return nil, false
		}
		entry := entries[group.Group.ID.String()]
		groups = append(groups, codersdk.ChatGroup{
			Group: db2sdk.Group(group, nil, int(memberCount)),
			Role:  convertToChatRole(entry.Permissions),
		})
	}
	return groups, true
}

func (api *API) allowChatSharing(ctx context.Context, rw http.ResponseWriter) bool {
	if !api.chatSharingDisabled() {
		return true
	}
	httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
		Message: "Chat sharing is disabled for this deployment.",
	})
	return false
}

func (api *API) chatSharingDisabled() bool {
	return rbac.ChatACLDisabled() || (api.DeploymentValues != nil && bool(api.DeploymentValues.DisableChatSharing))
}

type ChatACLUpdateValidator codersdk.UpdateChatACL

var _ acl.UpdateValidator[codersdk.ChatRole] = ChatACLUpdateValidator{}

func (c ChatACLUpdateValidator) Users() (map[string]codersdk.ChatRole, string) {
	return c.UserRoles, "user_roles"
}

func (c ChatACLUpdateValidator) Groups() (map[string]codersdk.ChatRole, string) {
	return c.GroupRoles, "group_roles"
}

func (ChatACLUpdateValidator) ValidateRole(role codersdk.ChatRole) error {
	if role == codersdk.ChatRoleDeleted || role == codersdk.ChatRoleRead {
		return nil
	}
	return xerrors.Errorf("role %q is not a valid chat role", role)
}

func convertToChatRole(actions []policy.Action) codersdk.ChatRole {
	if slice.SameElements(actions, db2sdk.ChatRoleActions(codersdk.ChatRoleRead)) {
		return codersdk.ChatRoleRead
	}

	return codersdk.ChatRoleDeleted
}

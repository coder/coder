package coderd

import (
	"context"
	"database/sql"
	"net/http"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	slog "cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
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
func (api *API) getChatACL(rw http.ResponseWriter, r *http.Request) { //nolint:revive // HTTP handler writes to ResponseWriter.
	ctx := r.Context()
	chat := httpmw.ChatParam(r)

	if !api.allowChatSharing(ctx, rw) {
		return
	}
	if chat.IsSubChat() {
		writeChatACLSubChatError(ctx, rw, chat)
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
		writeChatACLSubChatError(ctx, rw, chat)
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

	var validErrs []codersdk.ValidationError
	req.UserRoles, validErrs = canonicalChatACLKeys(req.UserRoles, "user_roles")
	var canonicalErrs []codersdk.ValidationError
	req.GroupRoles, canonicalErrs = canonicalChatACLKeys(req.GroupRoles, "group_roles")
	validErrs = append(validErrs, canonicalErrs...)
	if len(validErrs) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid request to update chat ACL.",
			Validations: validErrs,
		})
		return
	}

	apiKey := httpmw.APIKey(r)
	if _, ok := req.UserRoles[apiKey.UserID.String()]; ok {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Cannot change your own chat sharing role.",
		})
		return
	}

	validErrs = acl.Validate(ctx, api.Database, ChatACLUpdateValidator(req))
	if len(validErrs) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid request to update chat ACL.",
			Validations: validErrs,
		})
		return
	}

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

	rw.WriteHeader(http.StatusNoContent)
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

	// ACL reads are already authorized against the chat, and referenced users
	// may not otherwise be readable by the caller.
	//nolint:gocritic
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
		// ACL reads are already authorized against the chat, and referenced groups
		// may not otherwise be readable by the caller.
		var err error
		//nolint:gocritic
		dbGroups, err = api.Database.GetGroups(dbauthz.AsSystemRestricted(ctx), database.GetGroupsParams{GroupIds: groupIDs})
		if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
			httpapi.InternalServerError(rw, err)
			return nil, false
		}
	}

	groups := make([]codersdk.ChatGroup, 0, len(dbGroups))
	for _, group := range dbGroups {
		// Group member totals stay visible even when the caller cannot read
		// the group's membership directly.
		//nolint:gocritic
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

func writeChatACLSubChatError(ctx context.Context, rw http.ResponseWriter, chat database.Chat) {
	if !chat.IsSubChat() {
		panic("developer error: writeChatACLSubChatError called on non-sub-chat")
	}
	resp := codersdk.Response{
		Message: "Chat ACLs can only be set on root chats.",
	}
	if chat.RootChatID.Valid {
		resp.Detail = "Target the root chat (id: " + chat.RootChatID.UUID.String() + ") instead."
	}
	httpapi.Write(ctx, rw, http.StatusBadRequest, resp)
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

// UUIDs can arrive with non-canonical casing from clients. Normalizing
// keys keeps audit comparisons stable, but collided inputs are ambiguous
// and must fail validation.
func canonicalChatACLKeys(entries map[string]codersdk.ChatRole, field string) (map[string]codersdk.ChatRole, []codersdk.ValidationError) {
	if len(entries) == 0 {
		return entries, nil
	}
	out := make(map[string]codersdk.ChatRole, len(entries))
	duplicateKeys := make(map[string]struct{})
	validErrs := make([]codersdk.ValidationError, 0)
	for id, role := range entries {
		key := id
		if parsed, err := uuid.Parse(id); err == nil {
			key = parsed.String()
			if _, ok := out[key]; ok {
				if _, reported := duplicateKeys[key]; !reported {
					validErrs = append(validErrs, codersdk.ValidationError{
						Field:  field,
						Detail: "duplicate UUID " + key + " after canonicalization",
					})
					duplicateKeys[key] = struct{}{}
				}
				continue
			}
		}
		out[key] = role
	}
	return out, validErrs
}

package coderd

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

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

func (api *API) chatACL(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx  = r.Context()
		chat = httpmw.ChatParam(r)
	)

	if chat.IsSubChat() {
		writeChatACLSubChatError(ctx, rw, chat)
		return
	}

	chatACL, err := api.Database.GetChatACLByID(ctx, chat.ID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	userIDs := make([]uuid.UUID, 0, len(chatACL.Users))
	for userID := range chatACL.Users {
		id, err := uuid.Parse(userID)
		if err != nil {
			api.Logger.Warn(ctx, "found invalid user uuid in chat acl", slog.Error(err), slog.F("chat_id", chat.ID))
			continue
		}
		userIDs = append(userIDs, id)
	}
	//nolint:gocritic
	dbUsers, err := api.Database.GetUsersByIDs(dbauthz.AsSystemRestricted(ctx), userIDs)
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		httpapi.InternalServerError(rw, err)
		return
	}

	users := make([]codersdk.ChatUser, 0, len(dbUsers))
	for _, it := range dbUsers {
		entry := chatACL.Users[it.ID.String()]
		users = append(users, codersdk.ChatUser{
			MinimalUser:      db2sdk.MinimalUser(it),
			Role:             convertToChatRole(entry.Permissions),
			ShareToolCalls:   entry.ShareToolCalls,
			ShareAttachments: entry.ShareAttachments,
		})
	}

	groupIDs := make([]uuid.UUID, 0, len(chatACL.Groups))
	for groupID := range chatACL.Groups {
		id, err := uuid.Parse(groupID)
		if err != nil {
			api.Logger.Warn(ctx, "found invalid group uuid in chat acl", slog.Error(err), slog.F("chat_id", chat.ID))
			continue
		}
		groupIDs = append(groupIDs, id)
	}

	dbGroups := make([]database.GetGroupsRow, 0)
	if len(groupIDs) > 0 {
		//nolint:gocritic
		dbGroups, err = api.Database.GetGroups(dbauthz.AsSystemRestricted(ctx), database.GetGroupsParams{GroupIds: groupIDs})
		if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
			httpapi.InternalServerError(rw, err)
			return
		}
	}

	groups := make([]codersdk.ChatGroup, 0, len(dbGroups))
	for _, it := range dbGroups {
		var members []database.GroupMember
		//nolint:gocritic
		members, err = api.Database.GetGroupMembersByGroupID(dbauthz.AsSystemRestricted(ctx), database.GetGroupMembersByGroupIDParams{
			GroupID:       it.Group.ID,
			IncludeSystem: false,
		})
		if err != nil {
			httpapi.InternalServerError(rw, err)
			return
		}
		entry := chatACL.Groups[it.Group.ID.String()]
		groups = append(groups, codersdk.ChatGroup{
			Group: db2sdk.Group(database.GetGroupsRow{
				Group:                   it.Group,
				OrganizationName:        it.OrganizationName,
				OrganizationDisplayName: it.OrganizationDisplayName,
			}, members, len(members)),
			Role:             convertToChatRole(entry.Permissions),
			ShareToolCalls:   entry.ShareToolCalls,
			ShareAttachments: entry.ShareAttachments,
		})
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.ChatACL{
		Users:  users,
		Groups: groups,
	})
}

func (api *API) patchChatACL(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx     = r.Context()
		chat    = httpmw.ChatParam(r)
		auditor = *api.Auditor.Load()
	)
	aReq, commitAudit := audit.InitRequest[database.Chat](rw, &audit.RequestParams{
		Audit:   auditor,
		Log:     api.Logger,
		Request: r,
		Action:  database.AuditActionWrite,
	})
	defer commitAudit()
	aReq.Old = chat
	aReq.UpdateOrganizationID(chat.OrganizationID)

	if chat.IsSubChat() {
		writeChatACLSubChatError(ctx, rw, chat)
		return
	}

	var req codersdk.UpdateChatACL
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	apiKey := httpmw.APIKey(r)
	if _, ok := req.UserRoles[apiKey.UserID.String()]; ok {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "You cannot change your own chat sharing role.",
		})
		return
	}

	validErrs := acl.Validate(ctx, api.Database, ChatACLUpdateValidator(req))
	if len(validErrs) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid request to update chat ACL.",
			Validations: validErrs,
		})
		return
	}
	err := api.Database.InTx(func(tx database.Store) error {
		current, err := tx.GetChatByID(ctx, chat.ID)
		if err != nil {
			return xerrors.Errorf("get chat by ID: %w", err)
		}

		for id, entry := range req.UserRoles {
			if entry.Role == codersdk.ChatRoleDeleted {
				delete(current.UserACL, id)
				continue
			}
			current.UserACL[id] = database.ChatACLEntry{
				Permissions:      db2sdk.ChatRoleActions(entry.Role),
				ShareToolCalls:   entry.ShareToolCalls,
				ShareAttachments: entry.ShareAttachments,
			}
		}
		for id, entry := range req.GroupRoles {
			if entry.Role == codersdk.ChatRoleDeleted {
				delete(current.GroupACL, id)
				continue
			}
			current.GroupACL[id] = database.ChatACLEntry{
				Permissions:      db2sdk.ChatRoleActions(entry.Role),
				ShareToolCalls:   entry.ShareToolCalls,
				ShareAttachments: entry.ShareAttachments,
			}
		}

		err = tx.UpdateChatACLByID(ctx, database.UpdateChatACLByIDParams{
			ID:       chat.ID,
			UserACL:  current.UserACL,
			GroupACL: current.GroupACL,
		})
		if err != nil {
			return err
		}

		updatedChat, err := tx.GetChatByID(ctx, chat.ID)
		if err != nil {
			return xerrors.Errorf("get updated chat by ID: %w", err)
		}
		aReq.New = updatedChat
		return nil
	}, nil)
	if err != nil {
		if errors.Is(err, dbauthz.ErrChatACLSubChat) {
			writeChatACLSubChatError(ctx, rw, chat)
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

func (api *API) deleteChatACL(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx     = r.Context()
		chat    = httpmw.ChatParam(r)
		auditor = *api.Auditor.Load()
	)
	aReq, commitAudit := audit.InitRequest[database.Chat](rw, &audit.RequestParams{
		Audit:   auditor,
		Log:     api.Logger,
		Request: r,
		Action:  database.AuditActionWrite,
	})
	defer commitAudit()
	aReq.Old = chat
	aReq.UpdateOrganizationID(chat.OrganizationID)

	if chat.IsSubChat() {
		writeChatACLSubChatError(ctx, rw, chat)
		return
	}

	err := api.Database.InTx(func(tx database.Store) error {
		err := tx.DeleteChatACLByID(ctx, chat.ID)
		if err != nil {
			return err
		}

		updatedChat, err := tx.GetChatByID(ctx, chat.ID)
		if err != nil {
			return xerrors.Errorf("get updated chat by ID: %w", err)
		}
		aReq.New = updatedChat
		return nil
	}, nil)
	if err != nil {
		if errors.Is(err, dbauthz.ErrChatACLSubChat) {
			writeChatACLSubChatError(ctx, rw, chat)
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

func writeChatACLSubChatError(ctx context.Context, rw http.ResponseWriter, chat database.Chat) {
	if !chat.IsSubChat() {
		panic("developer error: writeChatACLSubChatError called on non-sub-chat")
	}
	var rootID uuid.UUID
	switch {
	case chat.RootChatID.Valid:
		rootID = chat.RootChatID.UUID
	case chat.ParentChatID.Valid:
		rootID = chat.ParentChatID.UUID
	}
	httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
		Message: "Chat ACLs can only be set on root chats.",
		Detail:  "Target the root chat (id: " + rootID.String() + ") instead.",
	})
}

type ChatACLUpdateValidator codersdk.UpdateChatACL

var (
	chatACLUpdateUsersFieldName  = "user_roles"
	chatACLUpdateGroupsFieldName = "group_roles"
)

var _ acl.UpdateValidator[codersdk.ChatRole] = ChatACLUpdateValidator{}

func (c ChatACLUpdateValidator) Users() (map[string]codersdk.ChatRole, string) {
	return projectChatShareEntryRoles(c.UserRoles), chatACLUpdateUsersFieldName
}

func (c ChatACLUpdateValidator) Groups() (map[string]codersdk.ChatRole, string) {
	return projectChatShareEntryRoles(c.GroupRoles), chatACLUpdateGroupsFieldName
}

func (ChatACLUpdateValidator) ValidateRole(role codersdk.ChatRole) error {
	if role == codersdk.ChatRoleDeleted {
		return nil
	}
	if role == codersdk.ChatRoleRead {
		return nil
	}
	return xerrors.Errorf("role %q is not a valid chat role", role)
}

func projectChatShareEntryRoles(entries map[string]codersdk.ChatShareEntry) map[string]codersdk.ChatRole {
	roles := make(map[string]codersdk.ChatRole, len(entries))
	for id, entry := range entries {
		roles[id] = entry.Role
	}
	return roles
}

func convertToChatRole(actions []policy.Action) codersdk.ChatRole {
	if slice.SameElements(actions, db2sdk.ChatRoleActions(codersdk.ChatRoleRead)) {
		return codersdk.ChatRoleRead
	}
	return codersdk.ChatRoleDeleted
}

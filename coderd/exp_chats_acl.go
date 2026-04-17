package coderd

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
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

	if chat.RootChatID.Valid || chat.ParentChatID.Valid {
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
	// nolint:gocritic // Display info must be returned regardless of the caller's org:read perms.
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
		// nolint:gocritic // Display info must be returned regardless of the caller's group:read perms.
		dbGroups, err = api.Database.GetGroups(dbauthz.AsSystemRestricted(ctx), database.GetGroupsParams{GroupIds: groupIDs})
		if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
			httpapi.InternalServerError(rw, err)
			return
		}
	}

	groups := make([]codersdk.ChatGroup, 0, len(dbGroups))
	for _, it := range dbGroups {
		var members []database.GroupMember
		// nolint:gocritic // Display info must be returned regardless of the caller's group:read perms.
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
		ctx  = r.Context()
		chat = httpmw.ChatParam(r)
	)

	if chat.RootChatID.Valid || chat.ParentChatID.Valid {
		writeChatACLSubChatError(ctx, rw, chat)
		return
	}

	if !api.allowChatSharing(ctx, rw, chat) {
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

		return tx.UpdateChatACLByID(ctx, database.UpdateChatACLByIDParams{
			ID:       chat.ID,
			UserACL:  current.UserACL,
			GroupACL: current.GroupACL,
		})
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
		ctx  = r.Context()
		chat = httpmw.ChatParam(r)
	)

	if chat.RootChatID.Valid || chat.ParentChatID.Valid {
		writeChatACLSubChatError(ctx, rw, chat)
		return
	}

	if !api.allowChatSharing(ctx, rw, chat) {
		return
	}

	if err := api.Database.DeleteChatACLByID(ctx, chat.ID); err != nil {
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

// effectiveShareFlagsForViewer returns the OR of the viewer's direct
// user entry and every group entry for groups they belong to. The
// chat owner always sees everything.
func (api *API) effectiveShareFlagsForViewer(
	ctx context.Context,
	chat database.Chat,
	viewerID uuid.UUID,
) (flags codersdk.ViewerShareFlags, err error) {
	if viewerID == chat.OwnerID {
		return codersdk.ViewerShareFlags{ShareToolCalls: true, ShareAttachments: true}, nil
	}

	if entry, ok := chat.UserACL[viewerID.String()]; ok {
		flags.ShareToolCalls = flags.ShareToolCalls || entry.ShareToolCalls
		flags.ShareAttachments = flags.ShareAttachments || entry.ShareAttachments
	}

	if len(chat.GroupACL) == 0 {
		return flags, nil
	}

	// nolint:gocritic // Group membership lookup must ignore the viewer's group:read perms.
	groups, gErr := api.Database.GetGroups(dbauthz.AsSystemRestricted(ctx), database.GetGroupsParams{
		HasMemberID: viewerID,
	})
	if gErr != nil && !errors.Is(gErr, sql.ErrNoRows) {
		return codersdk.ViewerShareFlags{}, xerrors.Errorf("list viewer groups: %w", gErr)
	}
	for _, g := range groups {
		entry, ok := chat.GroupACL[g.Group.ID.String()]
		if !ok {
			continue
		}
		flags.ShareToolCalls = flags.ShareToolCalls || entry.ShareToolCalls
		flags.ShareAttachments = flags.ShareAttachments || entry.ShareAttachments
		if flags.ShareToolCalls && flags.ShareAttachments {
			break
		}
	}
	return flags, nil
}

func writeChatACLSubChatError(ctx context.Context, rw http.ResponseWriter, chat database.Chat) {
	var rootID uuid.UUID
	switch {
	case chat.RootChatID.Valid:
		rootID = chat.RootChatID.UUID
	case chat.ParentChatID.Valid:
		// root_chat_id is NULL on sub-chats inserted before denormalization; parent is the next-best hop.
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

// Unknown permissions map to ChatRoleDeleted so stale or corrupt entries
// are not misread as read access.
func convertToChatRole(actions []policy.Action) codersdk.ChatRole {
	if slice.SameElements(actions, db2sdk.ChatRoleActions(codersdk.ChatRoleRead)) {
		return codersdk.ChatRoleRead
	}
	return codersdk.ChatRoleDeleted
}

func (api *API) allowChatSharing(ctx context.Context, rw http.ResponseWriter, chat database.Chat) bool {
	// nolint:gocritic // This gate must ignore the caller's org:read perms.
	org, err := api.Database.GetOrganizationByID(dbauthz.AsSystemRestricted(ctx), chat.OrganizationID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return false
	}
	switch org.ShareableChatOwners {
	case database.ShareableChatOwnersNone:
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: "Chat sharing is disabled for this organization.",
		})
		return false
	case database.ShareableChatOwnersServiceAccounts:
		// nolint:gocritic // Owner lookup must ignore the caller's user:read perms.
		owner, err := api.Database.GetUserByID(dbauthz.AsSystemRestricted(ctx), chat.OwnerID)
		if err != nil {
			httpapi.InternalServerError(rw, err)
			return false
		}
		if !owner.IsServiceAccount {
			httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
				Message: "Chat sharing is restricted to service-account chats in this organization.",
			})
			return false
		}
	}
	return true
}

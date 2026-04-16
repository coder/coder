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

// chatACL returns the ACL on a chat, hydrated with display info for
// each user and group. Authorization is ActionRead on the chat; shared
// viewers can inspect the ACL they were added to.
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

	// Hydrate users. We use AsSystemRestricted so that display info
	// (username, avatar) is returned even for users the caller cannot
	// normally read. This follows the workspace ACL handler precedent.
	userIDs := make([]uuid.UUID, 0, len(chatACL.Users))
	for userID := range chatACL.Users {
		id, err := uuid.Parse(userID)
		if err != nil {
			api.Logger.Warn(ctx, "found invalid user uuid in chat acl", slog.Error(err), slog.F("chat_id", chat.ID))
			continue
		}
		userIDs = append(userIDs, id)
	}
	// nolint:gocritic // See coderd/workspaces.go:workspaceACL for context.
	dbUsers, err := api.Database.GetUsersByIDs(dbauthz.AsSystemRestricted(ctx), userIDs)
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		httpapi.InternalServerError(rw, err)
		return
	}

	users := make([]codersdk.ChatUser, 0, len(dbUsers))
	for _, it := range dbUsers {
		users = append(users, codersdk.ChatUser{
			MinimalUser: db2sdk.MinimalUser(it),
			Role:        convertToChatRole(chatACL.Users[it.ID.String()].Permissions),
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
		// nolint:gocritic // Same rationale as the users hydration above.
		dbGroups, err = api.Database.GetGroups(dbauthz.AsSystemRestricted(ctx), database.GetGroupsParams{GroupIds: groupIDs})
		if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
			httpapi.InternalServerError(rw, err)
			return
		}
	}

	groups := make([]codersdk.ChatGroup, 0, len(dbGroups))
	for _, it := range dbGroups {
		var members []database.GroupMember
		// nolint:gocritic // See above.
		members, err = api.Database.GetGroupMembersByGroupID(dbauthz.AsSystemRestricted(ctx), database.GetGroupMembersByGroupIDParams{
			GroupID:       it.Group.ID,
			IncludeSystem: false,
		})
		if err != nil {
			httpapi.InternalServerError(rw, err)
			return
		}
		groups = append(groups, codersdk.ChatGroup{
			Group: db2sdk.Group(database.GetGroupsRow{
				Group:                   it.Group,
				OrganizationName:        it.OrganizationName,
				OrganizationDisplayName: it.OrganizationDisplayName,
			}, members, len(members)),
			Role: convertToChatRole(chatACL.Groups[it.Group.ID.String()].Permissions),
		})
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.ChatACL{
		Users:  users,
		Groups: groups,
	})
}

// patchChatACL updates the ACL on a chat. Owners cannot change their
// own role (matches patchWorkspaceACL). Roles other than ChatRoleRead
// and ChatRoleDeleted are rejected with 400.
//
// Before persisting, the handler computes whether the chat contains
// tool calls or attachments that the owner must explicitly acknowledge
// shared viewers will see, and returns 400 if any required confirmation
// flag is missing. See codersdk.UpdateChatACL.
func (api *API) patchChatACL(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx  = r.Context()
		chat = httpmw.ChatParam(r)
	)

	if chat.RootChatID.Valid || chat.ParentChatID.Valid {
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

		for id, role := range req.UserRoles {
			if role == codersdk.ChatRoleDeleted {
				delete(current.UserACL, id)
				continue
			}
			current.UserACL[id] = database.WorkspaceACLEntry{
				Permissions: db2sdk.ChatRoleActions(role),
			}
		}
		for id, role := range req.GroupRoles {
			if role == codersdk.ChatRoleDeleted {
				delete(current.GroupACL, id)
				continue
			}
			current.GroupACL[id] = database.WorkspaceACLEntry{
				Permissions: db2sdk.ChatRoleActions(role),
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

// deleteChatACL clears both ACLs on the chat.
func (api *API) deleteChatACL(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx  = r.Context()
		chat = httpmw.ChatParam(r)
	)

	if chat.RootChatID.Valid || chat.ParentChatID.Valid {
		writeChatACLSubChatError(ctx, rw, chat)
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

// writeChatACLSubChatError is the canonical 400 response for any ACL
// endpoint invoked on a sub-chat. It includes the root chat id so the
// frontend can redirect the user to the place where ACL writes
// actually work.
func writeChatACLSubChatError(ctx context.Context, rw http.ResponseWriter, chat database.Chat) {
	var rootID uuid.UUID
	switch {
	case chat.RootChatID.Valid:
		rootID = chat.RootChatID.UUID
	case chat.ParentChatID.Valid:
		// Sub-chat inserted before root_chat_id was denormalized.
		// Fall back to the parent id; the frontend can follow the
		// parent chain itself if necessary.
		rootID = chat.ParentChatID.UUID
	}
	httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
		Message: "Chat ACLs can only be set on root chats.",
		Detail:  "Target the root chat (id: " + rootID.String() + ") instead.",
	})
}

// ChatACLUpdateValidator implements acl.UpdateValidator[codersdk.ChatRole]
// for the chat share endpoint. Only ChatRoleRead (or the delete
// sentinel) is accepted today; any other value fails validation.
type ChatACLUpdateValidator codersdk.UpdateChatACL

var (
	chatACLUpdateUsersFieldName  = "user_roles"
	chatACLUpdateGroupsFieldName = "group_roles"
)

var _ acl.UpdateValidator[codersdk.ChatRole] = ChatACLUpdateValidator{}

func (c ChatACLUpdateValidator) Users() (map[string]codersdk.ChatRole, string) {
	return c.UserRoles, chatACLUpdateUsersFieldName
}

func (c ChatACLUpdateValidator) Groups() (map[string]codersdk.ChatRole, string) {
	return c.GroupRoles, chatACLUpdateGroupsFieldName
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

// convertToChatRole is the inverse of db2sdk.ChatRoleActions. If the
// stored permissions do not match a known role we return the empty
// string so clients surface "unknown" rather than misreporting. Writes
// must not round-trip the empty role back to the server except when
// deliberately removing an entry.
func convertToChatRole(actions []policy.Action) codersdk.ChatRole {
	if slice.SameElements(actions, db2sdk.ChatRoleActions(codersdk.ChatRoleRead)) {
		return codersdk.ChatRoleRead
	}
	return codersdk.ChatRoleDeleted
}

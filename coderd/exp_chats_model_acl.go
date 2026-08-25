package coderd

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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
	"github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/coderd/rbac/acl"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/searchquery"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary Get an AI model ACL
// @ID get-an-ai-model-acl
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Param organization path string true "Organization name or ID"
// @Param model path string true "Model ID" format(uuid)
// @Success 200 {object} codersdk.ChatModelACL
// @Router /api/v2/organizations/{organization}/chats/models/{model}/acl [get]
// @x-apidocgen {"skip": true}
func (api *API) chatModelConfigACLHandler(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	config := httpmw.ChatModelConfigParam(r)
	if !api.Authorize(r, policy.ActionShare, chatModelConfigRBACObject(config)) {
		httpapi.ResourceNotFound(rw)
		return
	}

	users, ok := api.chatModelACLUsers(ctx, rw, config, config.UserACL)
	if !ok {
		return
	}
	groups, ok := api.chatModelACLGroups(ctx, rw, config, config.GroupACL)
	if !ok {
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.ChatModelACL{
		Users:  users,
		Groups: groups,
	})
}

// @Summary Get available AI model ACL users and groups
// @ID get-available-ai-model-acl-users-groups
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Param organization path string true "Organization name or ID"
// @Param model path string true "Model ID" format(uuid)
// @Param q query string false "User search query; free-text search also applies to groups"
// @Param after_id query string false "User after ID" format(uuid)
// @Param limit query int false "Page limit for users and groups, if 0 returns all candidates"
// @Param offset query int false "User page offset"
// @Success 200 {object} codersdk.ACLAvailable
// @Router /api/v2/organizations/{organization}/chats/models/{model}/acl/available [get]
// @x-apidocgen {"skip": true}
func (api *API) chatModelConfigACLAvailable(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	config := httpmw.ChatModelConfigParam(r)
	if !api.Authorize(r, policy.ActionShare, chatModelConfigRBACObject(config)) {
		httpapi.ResourceNotFound(rw)
		return
	}

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

	//nolint:gocritic // The model share permission authorizes this bounded
	// organization-scoped lookup even when the caller cannot browse the
	// ordinary directories.
	restrictedCtx := dbauthz.AsSystemRestricted(ctx)
	memberRows, err := api.Database.PaginatedOrganizationMembers(restrictedCtx, database.PaginatedOrganizationMembersParams{
		AfterID:          pagination.AfterID,
		OrganizationID:   config.OrganizationID,
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
		httpapi.InternalServerError(rw, xerrors.Errorf("list chat model ACL users: %w", err))
		return
	}

	groups, err := api.Database.GetGroups(restrictedCtx, database.GetGroupsParams{
		OrganizationID: config.OrganizationID,
		Search:         userFilter.Search,
		// #nosec G115 - Pagination limits are small and fit in int32.
		LimitOpt: int32(pagination.Limit),
	})
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		httpapi.InternalServerError(rw, xerrors.Errorf("list chat model ACL groups: %w", err))
		return
	}

	groupIDs := make([]uuid.UUID, len(groups))
	for i, group := range groups {
		groupIDs[i] = group.Group.ID
	}
	countByGroup, ok := api.chatModelACLGroupMemberCounts(restrictedCtx, rw, groupIDs)
	if !ok {
		return
	}

	sdkUsers := make([]codersdk.ReducedUser, 0, len(memberRows))
	for _, member := range memberRows {
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

type chatModelACLValidationError struct {
	Validations []codersdk.ValidationError
}

func (*chatModelACLValidationError) Error() string {
	return "invalid chat model ACL"
}

// @Summary Update an AI model ACL
// @ID update-an-ai-model-acl
// @Security CoderSessionToken
// @Tags Chats
// @Accept json
// @Param organization path string true "Organization name or ID"
// @Param model path string true "Model ID" format(uuid)
// @Param request body codersdk.UpdateChatModelACLRequest true "Sparse model ACL update"
// @Success 204
// @Router /api/v2/organizations/{organization}/chats/models/{model}/acl [patch]
// @x-apidocgen {"skip": true}
func (api *API) updateChatModelConfigACL(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)
	config := httpmw.ChatModelConfigParam(r)
	if !api.Authorize(r, policy.ActionShare, chatModelConfigRBACObject(config)) {
		httpapi.ResourceNotFound(rw)
		return
	}

	auditor := api.Auditor.Load()
	aReq, commitAudit := audit.InitRequest[database.ChatModelConfig](rw, &audit.RequestParams{
		Audit:          *auditor,
		Log:            api.Logger,
		Request:        r,
		Action:         database.AuditActionWrite,
		OrganizationID: config.OrganizationID,
	})
	defer commitAudit()
	aReq.Old = config

	var req codersdk.UpdateChatModelACLRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	userRoles, validations := canonicalChatModelACLRoles("user_roles", req.UserRoles)
	groupRoles, duplicateErrors := canonicalChatModelACLRoles("group_roles", req.GroupRoles)
	validations = append(validations, duplicateErrors...)
	if len(validations) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid request to update chat model ACL.",
			Validations: validations,
		})
		return
	}

	var updated database.ChatModelConfig
	err := api.inChatModelConfigWriteTx(ctx, config.OrganizationID, func(tx database.Store) error {
		//nolint:gocritic // The ACL write below reauthorizes the locked row for share.
		current, err := tx.GetChatModelConfigByID(dbauthz.AsSystemRestricted(ctx), config.ID)
		if err != nil {
			return xerrors.Errorf("get chat model config for ACL update: %w", err)
		}
		aReq.Old = current

		validations := acl.Validate(ctx, tx, ChatModelACLUpdateValidator(req))
		validations = append(validations, validateChatModelACLOrganization(
			ctx,
			tx,
			config.OrganizationID,
			userRoles,
			groupRoles,
		)...)
		if len(validations) > 0 {
			return &chatModelACLValidationError{Validations: validations}
		}

		userACL := maps.Clone(current.UserACL)
		if userACL == nil {
			userACL = database.ChatACL{}
		}
		groupACL := maps.Clone(current.GroupACL)
		if groupACL == nil {
			groupACL = database.ChatACL{}
		}
		applyChatModelACLRoles(userACL, userRoles)
		applyChatModelACLRoles(groupACL, groupRoles)

		updated, err = tx.UpdateChatModelConfigACLByID(ctx, database.UpdateChatModelConfigACLByIDParams{
			ID:        config.ID,
			UserACL:   userACL,
			GroupACL:  groupACL,
			UpdatedBy: uuid.NullUUID{UUID: apiKey.UserID, Valid: apiKey.UserID != uuid.Nil},
		})
		return err
	})
	if err != nil {
		var validationErr *chatModelACLValidationError
		switch {
		case errors.As(err, &validationErr):
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message:     "Invalid request to update chat model ACL.",
				Validations: validationErr.Validations,
			})
		case dbauthz.IsNotAuthorizedError(err):
			httpapi.Forbidden(rw)
		case xerrors.Is(err, sql.ErrNoRows), xerrors.Is(err, errChatModelConfigNotFound), httpapi.Is404Error(err):
			httpapi.ResourceNotFound(rw)
		default:
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to update chat model ACL.",
				Detail:  err.Error(),
			})
		}
		return
	}

	aReq.New = updated
	publishChatConfigEvent(api.Logger, api.Pubsub, pubsub.ChatConfigEventModelConfig, updated.ID)
	rw.WriteHeader(http.StatusNoContent)
}

func (api *API) chatModelACLUsers(ctx context.Context, rw http.ResponseWriter, config database.ChatModelConfig, entries database.ChatACL) ([]codersdk.ChatUser, bool) {
	userIDs := make([]uuid.UUID, 0, len(entries))
	entriesByID := make(map[uuid.UUID]database.ChatACLEntry, len(entries))
	for rawUserID, entry := range entries {
		userID, err := uuid.Parse(rawUserID)
		if err != nil {
			api.Logger.Warn(ctx, "found invalid user uuid in chat model acl", slog.Error(err), slog.F("model_id", config.ID), slog.F("user_id", rawUserID))
			continue
		}
		userIDs = append(userIDs, userID)
		entriesByID[userID] = entry
	}
	if len(userIDs) == 0 {
		return []codersdk.ChatUser{}, true
	}

	//nolint:gocritic // The model share permission authorizes identity hydration
	// for ACL entries even when the caller cannot browse the ordinary user
	// directory.
	dbUsers, err := api.Database.GetUsersByIDs(dbauthz.AsSystemRestricted(ctx), userIDs)
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		httpapi.InternalServerError(rw, xerrors.Errorf("hydrate chat model ACL users: %w", err))
		return nil, false
	}

	users := make([]codersdk.ChatUser, 0, len(dbUsers))
	for _, user := range dbUsers {
		users = append(users, codersdk.ChatUser{
			MinimalUser: db2sdk.MinimalUser(user),
			Role:        convertToChatRole(entriesByID[user.ID].Permissions),
		})
	}
	return users, true
}

func (api *API) chatModelACLGroups(ctx context.Context, rw http.ResponseWriter, config database.ChatModelConfig, entries database.ChatACL) ([]codersdk.ChatGroup, bool) {
	groupIDs := make([]uuid.UUID, 0, len(entries))
	entriesByID := make(map[uuid.UUID]database.ChatACLEntry, len(entries))
	for rawGroupID, entry := range entries {
		groupID, err := uuid.Parse(rawGroupID)
		if err != nil {
			api.Logger.Warn(ctx, "found invalid group uuid in chat model acl", slog.Error(err), slog.F("model_id", config.ID), slog.F("group_id", rawGroupID))
			continue
		}
		groupIDs = append(groupIDs, groupID)
		entriesByID[groupID] = entry
	}
	if len(groupIDs) == 0 {
		return []codersdk.ChatGroup{}, true
	}

	//nolint:gocritic // The model share permission authorizes identity hydration
	// for ACL entries even when the caller cannot browse the ordinary group
	// directory.
	restrictedCtx := dbauthz.AsSystemRestricted(ctx)
	dbGroups, err := api.Database.GetGroups(restrictedCtx, database.GetGroupsParams{
		OrganizationID: config.OrganizationID,
		GroupIds:       groupIDs,
	})
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		httpapi.InternalServerError(rw, xerrors.Errorf("hydrate chat model ACL groups: %w", err))
		return nil, false
	}
	hydratedGroupIDs := make([]uuid.UUID, len(dbGroups))
	for i, group := range dbGroups {
		hydratedGroupIDs[i] = group.Group.ID
	}
	countByGroup, ok := api.chatModelACLGroupMemberCounts(restrictedCtx, rw, hydratedGroupIDs)
	if !ok {
		return nil, false
	}

	groups := make([]codersdk.ChatGroup, 0, len(dbGroups))
	for _, group := range dbGroups {
		groups = append(groups, codersdk.ChatGroup{
			Group: db2sdk.Group(group, nil, int(countByGroup[group.Group.ID])),
			Role:  convertToChatRole(entriesByID[group.Group.ID].Permissions),
		})
	}
	return groups, true
}

func (api *API) chatModelACLGroupMemberCounts(ctx context.Context, rw http.ResponseWriter, groupIDs []uuid.UUID) (map[uuid.UUID]int64, bool) {
	countByGroup := make(map[uuid.UUID]int64, len(groupIDs))
	if len(groupIDs) == 0 {
		return countByGroup, true
	}

	countRows, err := api.Database.GetGroupMembersCountByGroupIDs(ctx, database.GetGroupMembersCountByGroupIDsParams{
		GroupIds:      groupIDs,
		IncludeSystem: false,
	})
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		httpapi.InternalServerError(rw, xerrors.Errorf("count chat model ACL group members: %w", err))
		return nil, false
	}
	for _, row := range countRows {
		countByGroup[row.GroupID] = row.MemberCount
	}
	return countByGroup, true
}

func reducedUserFromPaginatedOrganizationMember(member database.PaginatedOrganizationMembersRow) codersdk.ReducedUser {
	return codersdk.ReducedUser{
		MinimalUser: codersdk.MinimalUser{
			ID:        member.OrganizationMember.UserID,
			Username:  member.Username,
			Name:      member.Name,
			AvatarURL: member.AvatarURL,
		},
		Email:            member.Email,
		CreatedAt:        member.UserCreatedAt,
		UpdatedAt:        member.UserUpdatedAt,
		LastSeenAt:       member.LastSeenAt,
		Status:           codersdk.UserStatus(member.Status),
		LoginType:        codersdk.LoginType(member.LoginType),
		IsServiceAccount: member.IsServiceAccount,
	}
}

func applyChatModelACLRoles(entries database.ChatACL, roles map[string]codersdk.ChatRole) {
	for id, role := range roles {
		if role == codersdk.ChatRoleDeleted {
			delete(entries, id)
			continue
		}
		entries[id] = database.ChatACLEntry{Permissions: db2sdk.ChatRoleActions(role)}
	}
}

func canonicalChatModelACLRoles(field string, roles map[string]codersdk.ChatRole) (map[string]codersdk.ChatRole, []codersdk.ValidationError) {
	canonical := make(map[string]codersdk.ChatRole, len(roles))
	var validations []codersdk.ValidationError
	for rawID, role := range roles {
		parsed, err := uuid.Parse(rawID)
		if err != nil {
			continue
		}
		id := parsed.String()
		if _, ok := canonical[id]; ok {
			validations = append(validations, codersdk.ValidationError{
				Field:  field,
				Detail: fmt.Sprintf("duplicate entries for ID %s", id),
			})
			continue
		}
		canonical[id] = role
	}
	return canonical, validations
}

func validateChatModelACLOrganization(
	ctx context.Context,
	db database.Store,
	organizationID uuid.UUID,
	userRoles map[string]codersdk.ChatRole,
	groupRoles map[string]codersdk.ChatRole,
) []codersdk.ValidationError {
	var validations []codersdk.ValidationError
	userIDs := chatModelACLRoleIDs(userRoles)
	if len(userIDs) > 0 {
		//nolint:gocritic // Principal validation requires organization membership visibility.
		memberships, err := db.GetOrganizationIDsByMemberIDs(dbauthz.AsSystemRestricted(ctx), userIDs)
		if err != nil {
			validations = append(validations, codersdk.ValidationError{Field: "user_roles", Detail: err.Error()})
		} else {
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
	}

	groupIDs := chatModelACLRoleIDs(groupRoles)
	if len(groupIDs) > 0 {
		//nolint:gocritic // Principal validation requires group organization visibility.
		groups, err := db.GetGroups(dbauthz.AsSystemRestricted(ctx), database.GetGroupsParams{GroupIds: groupIDs})
		if err != nil {
			validations = append(validations, codersdk.ValidationError{Field: "group_roles", Detail: err.Error()})
		} else {
			for _, group := range groups {
				if group.Group.OrganizationID != organizationID {
					validations = append(validations, codersdk.ValidationError{
						Field:  "group_roles",
						Detail: "group " + group.Group.ID.String() + " does not belong to organization " + organizationID.String(),
					})
				}
			}
		}
	}
	return validations
}

func chatModelACLRoleIDs(roles map[string]codersdk.ChatRole) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(roles))
	for rawID, role := range roles {
		if role == codersdk.ChatRoleDeleted {
			continue
		}
		if id, err := uuid.Parse(rawID); err == nil {
			ids = append(ids, id)
		}
	}
	return ids
}

type ChatModelACLUpdateValidator codersdk.UpdateChatModelACLRequest

var _ acl.UpdateValidator[codersdk.ChatRole] = ChatModelACLUpdateValidator{}

func (c ChatModelACLUpdateValidator) Users() (map[string]codersdk.ChatRole, string) {
	return c.UserRoles, "user_roles"
}

func (c ChatModelACLUpdateValidator) Groups() (map[string]codersdk.ChatRole, string) {
	return c.GroupRoles, "group_roles"
}

func (ChatModelACLUpdateValidator) ValidateRole(role codersdk.ChatRole) error {
	if role == codersdk.ChatRoleRead || role == codersdk.ChatRoleDeleted {
		return nil
	}
	return xerrors.Errorf("role %q is not a valid chat model role", role)
}

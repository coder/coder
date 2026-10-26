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
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/acl"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary Get chat artifact ACL
// @ID get-chat-artifact-acl
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Param artifact path string true "Chat artifact ID" format(uuid)
// @Success 200 {object} codersdk.ChatACL
// @Router /api/experimental/chats/artifacts/{artifact}/acl [get]
// @x-apidocgen {"skip": true}
//
//nolint:revive // HTTP handler writes to ResponseWriter.
func (api *API) getChatArtifactACL(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	// The param middleware already returned 404 to callers who cannot read
	// the artifact, so the disabled response reveals nothing.
	artifact := httpmw.ChatArtifactParam(r).ChatArtifact
	if !api.allowChatSharing(ctx, rw) {
		return
	}

	users, err := api.chatArtifactACLUsers(ctx, artifact)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	groups, err := api.chatArtifactACLGroups(ctx, artifact)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, codersdk.ChatACL{
		Users:  users,
		Groups: groups,
	})
}

// @Summary Update chat artifact ACL
// @ID update-chat-artifact-acl
// @Security CoderSessionToken
// @Tags Chats
// @Accept json
// @Param artifact path string true "Chat artifact ID" format(uuid)
// @Param request body codersdk.UpdateChatACL true "Update chat artifact ACL request"
// @Success 204
// @Router /api/experimental/chats/artifacts/{artifact}/acl [patch]
// @x-apidocgen {"skip": true}
func (api *API) patchChatArtifactACL(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	row := httpmw.ChatArtifactParam(r)
	artifact := row.ChatArtifact
	apiKey := httpmw.APIKey(r)

	aReq, commitAudit := audit.InitRequest[database.ChatArtifact](rw, &audit.RequestParams{
		Audit:          *api.Auditor.Load(),
		Log:            api.Logger,
		Request:        r,
		Action:         database.AuditActionWrite,
		OrganizationID: artifact.OrganizationID,
	})
	defer commitAudit()
	aReq.Old = artifact

	if !api.allowChatSharing(ctx, rw) {
		return
	}
	obj, err := api.chatArtifactRBACObject(ctx, artifact)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	if !api.Authorize(r, policy.ActionShare, obj) {
		httpapi.Forbidden(rw)
		return
	}

	var req codersdk.UpdateChatACL
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	validErrs := acl.Validate(ctx, api.Database, ChatACLUpdateValidator(req))
	if len(validErrs) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid request to update chat artifact ACL.",
			Validations: validErrs,
		})
		return
	}
	// RBAC matches ACL keys as strings, so only canonical UUIDs work.
	userRoles, userKeyErrs := canonicalChatArtifactRoles(req.UserRoles, "user_roles")
	groupRoles, groupKeyErrs := canonicalChatArtifactRoles(req.GroupRoles, "group_roles")
	if len(userKeyErrs)+len(groupKeyErrs) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid request to update chat artifact ACL.",
			Validations: slices.Concat(userKeyErrs, groupKeyErrs),
		})
		return
	}
	if _, ok := userRoles[artifact.OwnerID]; ok {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "The artifact owner cannot be added to the artifact's sharing list.",
		})
		return
	}
	if _, ok := userRoles[apiKey.UserID]; ok {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Cannot change your own artifact sharing role.",
		})
		return
	}

	err = api.Database.InTx(func(tx database.Store) error {
		// Lock granted users' memberships before the artifact row. A
		// membership delete locks its row and then, through the 000635
		// trigger, the artifact row, so the reverse order could deadlock.
		// Holding the membership lock also keeps a delete from missing the
		// grant written below.
		if userIDs := grantedChatArtifactIDs(userRoles); len(userIDs) > 0 {
			//nolint:gocritic // Validation below reports users that are not members.
			_, err := tx.LockOrganizationMembersByUserIDsForShare(dbauthz.AsSystemRestricted(ctx), database.LockOrganizationMembersByUserIDsForShareParams{
				OrganizationID: artifact.OrganizationID,
				UserIds:        userIDs,
			})
			if err != nil {
				return xerrors.Errorf("lock organization members: %w", err)
			}
		}
		current, err := tx.GetChatArtifactByIDForUpdate(ctx, artifact.ID)
		if err != nil {
			return xerrors.Errorf("get chat artifact for update: %w", err)
		}
		aReq.Old = current
		validErrs, err := validateChatACLOrganization(ctx, tx, current.OrganizationID, "artifact", userRoles, groupRoles)
		if err != nil {
			return err
		}
		if len(validErrs) > 0 {
			return &chatProjectACLValidationError{validations: validErrs}
		}
		if err := tx.UpdateChatArtifactACLByID(ctx, database.UpdateChatArtifactACLByIDParams{
			ID:       current.ID,
			UserACL:  applyChatArtifactRoles(current.UserACL, userRoles),
			GroupACL: applyChatArtifactRoles(current.GroupACL, groupRoles),
		}); err != nil {
			return xerrors.Errorf("update chat artifact ACL: %w", err)
		}
		updated, err := tx.GetChatArtifactByID(ctx, current.ID)
		if err != nil {
			return xerrors.Errorf("get updated chat artifact: %w", err)
		}
		aReq.New = updated.ChatArtifact
		return nil
	}, nil)
	if err != nil {
		if validationErr, ok := errors.AsType[*chatProjectACLValidationError](err); ok {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message:     "Invalid request to update chat artifact ACL.",
				Validations: validationErr.validations,
			})
			return
		}
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

// chatArtifactRBACObject builds the artifact's object with its project's
// ACL, so that the share check sees every grant.
func (api *API) chatArtifactRBACObject(ctx context.Context, artifact database.ChatArtifact) (rbac.Object, error) {
	if !artifact.ProjectID.Valid {
		return artifact.RBACObject(nil), nil
	}
	//nolint:gocritic // The project only adds grants to the artifact object; the caller is authorized against it.
	project, err := api.Database.GetChatProjectByID(dbauthz.AsSystemRestricted(ctx), artifact.ProjectID.UUID)
	if errors.Is(err, sql.ErrNoRows) {
		return artifact.RBACObject(nil), nil
	}
	if err != nil {
		return rbac.Object{}, xerrors.Errorf("get chat artifact project: %w", err)
	}
	return artifact.RBACObject(&project), nil
}

func canonicalChatArtifactRoles(roles map[string]codersdk.ChatRole, field string) (map[uuid.UUID]codersdk.ChatRole, []codersdk.ValidationError) {
	canonical := make(map[uuid.UUID]codersdk.ChatRole, len(roles))
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

func applyChatArtifactRoles(current database.ChatACL, roles map[uuid.UUID]codersdk.ChatRole) database.ChatACL {
	next := make(database.ChatACL, len(current)+len(roles))
	for id, entry := range current {
		next[id] = entry
	}
	for id, role := range roles {
		if role == codersdk.ChatRoleDeleted {
			delete(next, id.String())
			continue
		}
		next[id.String()] = database.ChatACLEntry{Permissions: db2sdk.ChatRoleActions(role)}
	}
	return next
}

func grantedChatArtifactIDs(roles map[uuid.UUID]codersdk.ChatRole) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(roles))
	for id, role := range roles {
		if role != codersdk.ChatRoleDeleted {
			ids = append(ids, id)
		}
	}
	return ids
}

// validateChatACLOrganization reports granted users who are not members of
// the organization and granted groups from other organizations.
func validateChatACLOrganization(
	ctx context.Context,
	db database.Store,
	organizationID uuid.UUID,
	noun string,
	userRoles, groupRoles map[uuid.UUID]codersdk.ChatRole,
) ([]codersdk.ValidationError, error) {
	//nolint:gocritic // Validation needs every requested user and group, even ones the caller cannot read.
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	var validErrs []codersdk.ValidationError

	if userIDs := grantedChatArtifactIDs(userRoles); len(userIDs) > 0 {
		memberships, err := db.GetOrganizationIDsByMemberIDs(sysCtx, userIDs)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, xerrors.Errorf("get organization memberships: %w", err)
		}
		inOrg := make(map[uuid.UUID]bool, len(memberships))
		for _, membership := range memberships {
			inOrg[membership.UserID] = slices.Contains(membership.OrganizationIDs, organizationID)
		}
		for _, id := range userIDs {
			if !inOrg[id] {
				validErrs = append(validErrs, codersdk.ValidationError{
					Field:  "user_roles",
					Detail: fmt.Sprintf("user with ID %v is not a member of the %s's organization", id, noun),
				})
			}
		}
	}

	if groupIDs := grantedChatArtifactIDs(groupRoles); len(groupIDs) > 0 {
		groups, err := db.GetGroups(sysCtx, database.GetGroupsParams{GroupIds: groupIDs})
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, xerrors.Errorf("get groups: %w", err)
		}
		for _, group := range groups {
			if group.Group.OrganizationID != organizationID {
				validErrs = append(validErrs, codersdk.ValidationError{
					Field:  "group_roles",
					Detail: fmt.Sprintf("group with ID %v is not in the %s's organization", group.Group.ID, noun),
				})
			}
		}
	}
	return validErrs, nil
}

func (api *API) chatArtifactACLUsers(ctx context.Context, artifact database.ChatArtifact) ([]codersdk.ChatUser, error) {
	userIDs := make([]uuid.UUID, 0, len(artifact.UserACL))
	for rawID := range artifact.UserACL {
		id, err := uuid.Parse(rawID)
		if err != nil {
			api.Logger.Warn(ctx, "found invalid user uuid in chat artifact acl", slog.Error(err), slog.F("artifact_id", artifact.ID))
			continue
		}
		userIDs = append(userIDs, id)
	}
	if len(userIDs) == 0 {
		return []codersdk.ChatUser{}, nil
	}

	//nolint:gocritic // Artifact readers see who it is shared with without user read permission.
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	// Entries for users outside the organization grant nothing, because
	// access requires membership, so hide them.
	memberships, err := api.Database.GetOrganizationIDsByMemberIDs(sysCtx, userIDs)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, xerrors.Errorf("get organization memberships: %w", err)
	}
	userIDs = userIDs[:0]
	for _, membership := range memberships {
		if slices.Contains(membership.OrganizationIDs, artifact.OrganizationID) {
			userIDs = append(userIDs, membership.UserID)
		}
	}
	if len(userIDs) == 0 {
		return []codersdk.ChatUser{}, nil
	}
	dbUsers, err := api.Database.GetUsersByIDs(sysCtx, userIDs)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, xerrors.Errorf("get users: %w", err)
	}
	users := make([]codersdk.ChatUser, 0, len(dbUsers))
	for _, user := range dbUsers {
		users = append(users, codersdk.ChatUser{
			MinimalUser: db2sdk.MinimalUser(user),
			Role:        convertToChatRole(artifact.UserACL[user.ID.String()].Permissions),
		})
	}
	return users, nil
}

func (api *API) chatArtifactACLGroups(ctx context.Context, artifact database.ChatArtifact) ([]codersdk.ChatGroup, error) {
	groupIDs := make([]uuid.UUID, 0, len(artifact.GroupACL))
	for rawID := range artifact.GroupACL {
		id, err := uuid.Parse(rawID)
		if err != nil {
			api.Logger.Warn(ctx, "found invalid group uuid in chat artifact acl", slog.Error(err), slog.F("artifact_id", artifact.ID))
			continue
		}
		groupIDs = append(groupIDs, id)
	}
	if len(groupIDs) == 0 {
		return []codersdk.ChatGroup{}, nil
	}

	//nolint:gocritic // Artifact readers see shared groups without group read permission.
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	dbGroups, err := api.Database.GetGroups(sysCtx, database.GetGroupsParams{GroupIds: groupIDs})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, xerrors.Errorf("get groups: %w", err)
	}
	groups := make([]codersdk.ChatGroup, 0, len(dbGroups))
	for _, group := range dbGroups {
		memberCount, err := api.Database.GetGroupMembersCountByGroupID(sysCtx, database.GetGroupMembersCountByGroupIDParams{
			GroupID:       group.Group.ID,
			IncludeSystem: false,
		})
		if err != nil {
			return nil, xerrors.Errorf("count group members: %w", err)
		}
		groups = append(groups, codersdk.ChatGroup{
			Group: db2sdk.Group(group, nil, int(memberCount)),
			Role:  convertToChatRole(artifact.GroupACL[group.Group.ID.String()].Permissions),
		})
	}
	return groups, nil
}

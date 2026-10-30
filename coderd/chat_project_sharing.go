package coderd

import (
	"context"
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
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/acl"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
)

// chatProject converts a project and adds what the requester may do with it.
func (api *API) chatProject(r *http.Request, project database.ChatProject) codersdk.ChatProject {
	return api.chatProjects(r, []database.ChatProject{project})[0]
}

// chatProjects converts projects for the requester, including each agent's
// user. Anyone who can read an agent may see which user it acts as.
func (api *API) chatProjects(r *http.Request, projects []database.ChatProject) []codersdk.ChatProject {
	ctx := r.Context()
	userIDs := make([]uuid.UUID, 0, len(projects))
	for _, project := range projects {
		if project.AgentUserID.Valid {
			userIDs = append(userIDs, project.AgentUserID.UUID)
		}
	}
	agentUsers := map[uuid.UUID]codersdk.MinimalUser{}
	if len(userIDs) > 0 {
		//nolint:gocritic // Agent identities are visible to anyone who can read the agent.
		users, err := api.Database.GetUsersByIDs(dbauthz.AsSystemRestricted(ctx), userIDs)
		if err != nil {
			api.Logger.Warn(ctx, "load agent users", slog.Error(err))
		}
		for _, user := range users {
			agentUsers[user.ID] = db2sdk.MinimalUser(user)
		}
	}
	out := make([]codersdk.ChatProject, 0, len(projects))
	for _, project := range projects {
		sdkProject := api.chatProjectWithPermissions(r, project)
		if user, ok := agentUsers[project.AgentUserID.UUID]; ok && project.AgentUserID.Valid {
			sdkProject.AgentUser = &user
		}
		sdkProject.ModelStatus, sdkProject.ModelStatusReason, sdkProject.ModelDisplayName = api.chatProjectModelStatus(ctx, project)
		out = append(out, sdkProject)
	}
	return out
}

// chatProjectModelStatus reports whether chats in the agent can use the
// agent's model. They run as the agent's user, so availability is checked
// for that user.
func (api *API) chatProjectModelStatus(ctx context.Context, project database.ChatProject) (status codersdk.ChatProjectModelStatus, reason string, displayName string) {
	if !project.ModelConfigID.Valid {
		return codersdk.ChatProjectModelStatusDefault, "", ""
	}
	//nolint:gocritic // Anyone who can read the agent may see its model's name and state.
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	config, err := api.Database.GetChatModelConfigByIDIncludingDeleted(sysCtx, project.ModelConfigID.UUID)
	if err != nil {
		if !httpapi.Is404Error(err) {
			api.Logger.Warn(ctx, "load agent model", slog.F("project_id", project.ID), slog.Error(err))
		}
		return codersdk.ChatProjectModelStatusUnavailable, "The model was not found.", ""
	}
	displayName = config.DisplayName
	if displayName == "" {
		displayName = config.Model
	}
	if config.Deleted {
		return codersdk.ChatProjectModelStatusUnavailable, "The model was deleted.", displayName
	}
	if !config.Enabled {
		return codersdk.ChatProjectModelStatusUnavailable, "The model is disabled.", displayName
	}
	userID := project.OwnerID
	if project.AgentUserID.Valid {
		userID = project.AgentUserID.UUID
	}
	subject, _, err := httpmw.UserRBACSubject(ctx, api.Database, userID, rbac.ScopeAll)
	if err != nil {
		api.Logger.Warn(ctx, "load agent user authorization", slog.F("project_id", project.ID), slog.Error(err))
		return codersdk.ChatProjectModelStatusUnavailable, "The agent's access to the model could not be checked.", displayName
	}
	_, availability, err := api.userCanUseChatModelConfig(dbauthz.As(ctx, subject), userID, project.OrganizationID, config.ID)
	if err != nil {
		api.Logger.Warn(ctx, "check agent model availability", slog.F("project_id", project.ID), slog.Error(err))
		return codersdk.ChatProjectModelStatusUnavailable, "The agent's access to the model could not be checked.", displayName
	}
	if availability != chatModelConfigAvailable {
		if availability == chatModelConfigUnavailableModelNotFoundOrDisabled {
			return codersdk.ChatProjectModelStatusUnavailable, "The agent is not allowed to use the model.", displayName
		}
		return codersdk.ChatProjectModelStatusUnavailable, chatProjectModelUnavailableMessage(availability), displayName
	}
	return codersdk.ChatProjectModelStatusAvailable, "", displayName
}

func (api *API) chatProjectWithPermissions(r *http.Request, project database.ChatProject) codersdk.ChatProject {
	sdkProject := db2sdk.ChatProject(project)
	obj := project.RBACObject()
	sdkProject.Permissions = codersdk.ChatProjectPermissions{
		Edit:   api.Authorize(r, policy.ActionUpdate, obj),
		Share:  !api.chatSharingDisabled() && api.Authorize(r, policy.ActionShare, obj),
		Delete: api.Authorize(r, policy.ActionDelete, obj),
		// Triggered chats run as the owner, so nobody else may manage them.
		ManageTriggers: project.OwnerID == httpmw.APIKey(r).UserID,
	}
	return sdkProject
}

// chatProjectRoleActions maps a sharing role to the actions it grants.
func chatProjectRoleActions(role codersdk.ChatProjectRole) []policy.Action {
	switch role {
	case codersdk.ChatProjectRoleEdit:
		return []policy.Action{policy.ActionRead, policy.ActionUpdate}
	case codersdk.ChatProjectRoleUse:
		return []policy.Action{policy.ActionRead}
	default:
		return []policy.Action{}
	}
}

// chatProjectRoleFromActions is the inverse of chatProjectRoleActions. Entries
// that grant update are reported as edit.
func chatProjectRoleFromActions(actions []policy.Action) codersdk.ChatProjectRole {
	if slices.Contains(actions, policy.ActionUpdate) || slices.Contains(actions, policy.WildcardSymbol) {
		return codersdk.ChatProjectRoleEdit
	}
	return codersdk.ChatProjectRoleUse
}

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
func (api *API) chatProjectACL(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project := httpmw.ChatProjectParam(r)
	// Everyone the project is shared with can read it, so listing who else
	// has access requires the share permission.
	if !api.Authorize(r, policy.ActionShare, project.RBACObject()) {
		httpapi.Forbidden(rw)
		return
	}

	users, err := resolveACLUsers(ctx, api.Database, project.UserACL, func(user codersdk.MinimalUser) codersdk.ChatProjectACLUser {
		return codersdk.ChatProjectACLUser{
			MinimalUser: user,
			Role:        chatProjectRoleFromActions(project.UserACL[user.ID.String()].Permissions),
		}
	})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	groups, err := resolveACLGroups(ctx, api.Database, project.GroupACL, func(group codersdk.Group) codersdk.ChatProjectACLGroup {
		return codersdk.ChatProjectACLGroup{
			Group: group,
			Role:  chatProjectRoleFromActions(project.GroupACL[group.ID.String()].Permissions),
		}
	})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, codersdk.ChatProjectACL{
		Users:  users,
		Groups: groups,
	})
}

// @Summary Get available chat project ACL users and groups
// @ID get-available-chat-project-acl-users-and-groups
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param project path string true "Chat project ID" format(uuid)
// @Param q query string false "User search query; free-text search also applies to groups"
// @Param after_id query string false "User after ID" format(uuid)
// @Param limit query int false "Page limit for users and groups, if 0 returns all candidates"
// @Param offset query int false "User page offset"
// @Success 200 {object} codersdk.ACLAvailable
// @Router /api/experimental/organizations/{organization}/chats/projects/{project}/acl/available [get]
// @x-apidocgen {"skip": true}
func (api *API) chatProjectACLAvailable(rw http.ResponseWriter, r *http.Request) {
	project := httpmw.ChatProjectParam(r)
	if !api.Authorize(r, policy.ActionShare, project.RBACObject()) {
		httpapi.Forbidden(rw)
		return
	}
	api.writeOrganizationACLAvailable(rw, r, project.OrganizationID)
}

// @Summary Update chat project ACL
// @ID update-chat-project-acl
// @Security CoderSessionToken
// @Tags Chats
// @Accept json
// @Param organization path string true "Organization ID" format(uuid)
// @Param project path string true "Chat project ID" format(uuid)
// @Param request body codersdk.UpdateChatProjectACLRequest true "Sparse chat project ACL update"
// @Success 204
// @Router /api/experimental/organizations/{organization}/chats/projects/{project}/acl [patch]
// @x-apidocgen {"skip": true}
func (api *API) patchChatProjectACL(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project := httpmw.ChatProjectParam(r)
	if !api.allowChatSharing(ctx, rw) {
		return
	}
	if !api.Authorize(r, policy.ActionShare, project.RBACObject()) {
		httpapi.Forbidden(rw)
		return
	}

	aReq, commitAudit := audit.InitRequest[database.ChatProject](rw, &audit.RequestParams{
		Audit:          *api.Auditor.Load(),
		Log:            api.Logger,
		Request:        r,
		Action:         database.AuditActionWrite,
		OrganizationID: project.OrganizationID,
	})
	defer commitAudit()
	aReq.Old = project

	var req codersdk.UpdateChatProjectACLRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	userRoles, groupRoles, validations := validateOrganizationACLUpdate(ctx, api.Database, project.OrganizationID, chatProjectACLUpdateValidator(req))
	if _, ok := userRoles[project.OwnerID.String()]; ok {
		validations = append(validations, codersdk.ValidationError{
			Field:  "user_roles",
			Detail: "The owner always has full access and cannot be shared with.",
		})
	}
	if len(validations) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid request to update agent sharing.",
			Validations: validations,
		})
		return
	}

	var updated database.ChatProject
	err := api.Database.InTx(func(tx database.Store) error {
		//nolint:gocritic // The ACL write below reauthorizes the locked row for share.
		current, err := tx.GetChatProjectByIDForUpdate(dbauthz.AsSystemRestricted(ctx), project.ID)
		if err != nil {
			return xerrors.Errorf("get chat project for ACL update: %w", err)
		}
		aReq.Old = current
		userACL := maps.Clone(current.UserACL)
		if userACL == nil {
			userACL = database.ChatACL{}
		}
		groupACL := maps.Clone(current.GroupACL)
		if groupACL == nil {
			groupACL = database.ChatACL{}
		}
		applyChatProjectACLRoles(userACL, userRoles)
		applyChatProjectACLRoles(groupACL, groupRoles)
		if err := tx.UpdateChatProjectACLByID(ctx, database.UpdateChatProjectACLByIDParams{
			ID:       project.ID,
			UserACL:  userACL,
			GroupACL: groupACL,
		}); err != nil {
			return xerrors.Errorf("update chat project ACL: %w", err)
		}
		updated = current
		updated.UserACL = userACL
		updated.GroupACL = groupACL
		return nil
	}, nil)
	if err != nil {
		switch {
		case dbauthz.IsNotAuthorizedError(err):
			httpapi.Forbidden(rw)
		case httpapi.Is404Error(err):
			httpapi.ResourceNotFound(rw)
		default:
			httpapi.InternalServerError(rw, err)
		}
		return
	}
	aReq.New = updated
	rw.WriteHeader(http.StatusNoContent)
}

func applyChatProjectACLRoles(entries database.ChatACL, roles map[string]codersdk.ChatProjectRole) {
	for id, role := range roles {
		if role == codersdk.ChatProjectRoleDeleted {
			delete(entries, id)
			continue
		}
		entries[id] = database.ChatACLEntry{Permissions: chatProjectRoleActions(role)}
	}
}

type chatProjectACLUpdateValidator codersdk.UpdateChatProjectACLRequest

var _ acl.UpdateValidator[codersdk.ChatProjectRole] = chatProjectACLUpdateValidator{}

func (v chatProjectACLUpdateValidator) Users() (map[string]codersdk.ChatProjectRole, string) {
	return v.UserRoles, "user_roles"
}

func (v chatProjectACLUpdateValidator) Groups() (map[string]codersdk.ChatProjectRole, string) {
	return v.GroupRoles, "group_roles"
}

func (chatProjectACLUpdateValidator) ValidateRole(role codersdk.ChatProjectRole) error {
	switch role {
	case codersdk.ChatProjectRoleUse, codersdk.ChatProjectRoleEdit, codersdk.ChatProjectRoleDeleted:
		return nil
	default:
		return xerrors.Errorf("role %q is not a valid agent role", role)
	}
}

// chatProjectAccessibleByUser reports whether userID may start chats in the
// project: the owner always can, others need the project shared with them.
func chatProjectAccessibleByUser(ctx context.Context, db database.Store, project database.ChatProject, userID uuid.UUID) (bool, error) {
	if project.OwnerID == userID {
		return true, nil
	}
	//nolint:gocritic // Checking access for a specific user requires reading every project.
	return db.IsChatProjectAccessibleByUserID(dbauthz.AsSystemRestricted(ctx), database.IsChatProjectAccessibleByUserIDParams{
		ProjectID: project.ID,
		UserID:    userID,
	})
}

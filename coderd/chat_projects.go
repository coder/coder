package coderd

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary List chat projects
// @ID list-chat-projects
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Success 200 {array} codersdk.ChatProject
// @Router /api/experimental/chats/projects [get]
// @x-apidocgen {"skip": true}
func (api *API) listChatProjects(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)

	projects, err := api.Database.GetChatProjectsOwnedOrSharedWithUserID(ctx, apiKey.UserID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to list chat projects.",
			Detail:  err.Error(),
		})
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, slice.List(projects, func(project database.ChatProject) codersdk.ChatProject {
		return api.convertChatProject(r, project)
	}))
}

// @Summary Create chat project
// @ID create-chat-project
// @Security CoderSessionToken
// @Tags Chats
// @Accept json
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param request body codersdk.CreateChatProjectRequest true "Create chat project request"
// @Success 201 {object} codersdk.ChatProject
// @Router /api/experimental/organizations/{organization}/chats/projects [post]
// @x-apidocgen {"skip": true}
func (api *API) postChatProject(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)
	organization := httpmw.OrganizationParam(r)

	var req codersdk.CreateChatProjectRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Icon = strings.TrimSpace(req.Icon)
	if req.Name == "" {
		writeChatProjectFieldError(ctx, rw, "name", "Name is required.")
		return
	}
	if !validateChatProjectFields(ctx, rw, req.Name, req.Description, req.Icon) {
		return
	}
	if !api.Authorize(r, policy.ActionCreate, database.ChatProject{
		OrganizationID: organization.ID,
		OwnerID:        apiKey.UserID,
	}.RBACObject()) {
		httpapi.Forbidden(rw)
		return
	}

	aReq, commitAudit := audit.InitRequest[database.ChatProject](rw, &audit.RequestParams{
		Audit:          *api.Auditor.Load(),
		Log:            api.Logger,
		Request:        r,
		Action:         database.AuditActionCreate,
		OrganizationID: organization.ID,
	})
	defer commitAudit()

	var project database.ChatProject
	err := api.Database.InTx(func(tx database.Store) error {
		// The lock serializes a user's creates so concurrent requests cannot
		// both observe room under the cap.
		if err := tx.AcquireLock(ctx, database.GenLockID("chat-projects-owner:"+apiKey.UserID.String())); err != nil {
			return xerrors.Errorf("lock chat projects: %w", err)
		}
		// The count spans the user's projects in every organization, and the
		// caller was already authorized to create one.
		//nolint:gocritic // See above.
		count, err := tx.CountChatProjectsByOwnerID(dbauthz.AsSystemRestricted(ctx), apiKey.UserID)
		if err != nil {
			return xerrors.Errorf("count chat projects: %w", err)
		}
		if count >= maxChatProjectsPerOwner {
			return errChatProjectLimit
		}
		project, err = tx.InsertChatProject(ctx, database.InsertChatProjectParams{
			ID:             uuid.NullUUID{},
			OrganizationID: organization.ID,
			OwnerID:        apiKey.UserID,
			Name:           req.Name,
			Description:    req.Description,
			Icon:           req.Icon,
		})
		return err
	}, nil)
	if errors.Is(err, errChatProjectLimit) {
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
			Message: fmt.Sprintf("You can have at most %d chat projects. Delete a project to create another.", maxChatProjectsPerOwner),
		})
		return
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to create chat project.",
			Detail:  err.Error(),
		})
		return
	}
	aReq.New = project
	httpapi.Write(ctx, rw, http.StatusCreated, api.convertChatProject(r, project))
}

// @Summary Get chat project
// @ID get-chat-project
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param project path string true "Chat project ID" format(uuid)
// @Success 200 {object} codersdk.ChatProject
// @Router /api/experimental/organizations/{organization}/chats/projects/{project} [get]
// @x-apidocgen {"skip": true}
//
//nolint:revive // HTTP handler writes to ResponseWriter.
func (api *API) getChatProject(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project := httpmw.ChatProjectParam(r)
	if !api.Authorize(r, policy.ActionRead, project.RBACObject()) {
		httpapi.ResourceNotFound(rw)
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, api.convertChatProject(r, project))
}

// @Summary Update chat project
// @ID update-chat-project
// @Security CoderSessionToken
// @Tags Chats
// @Accept json
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param project path string true "Chat project ID" format(uuid)
// @Param request body codersdk.UpdateChatProjectRequest true "Update chat project request"
// @Success 200 {object} codersdk.ChatProject
// @Router /api/experimental/organizations/{organization}/chats/projects/{project} [patch]
// @x-apidocgen {"skip": true}
func (api *API) patchChatProject(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project := httpmw.ChatProjectParam(r)
	if !api.authorizeChatProjectChange(rw, r, policy.ActionUpdate, project.RBACObject()) {
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

	var req codersdk.UpdateChatProjectRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	if req.Name != nil {
		*req.Name = strings.TrimSpace(*req.Name)
	}
	if req.Icon != nil {
		*req.Icon = strings.TrimSpace(*req.Icon)
	}
	if req.Name != nil && *req.Name == "" {
		writeChatProjectFieldError(ctx, rw, "name", "Name must not be blank.")
		return
	}
	name := ptr.NilToDefault(req.Name, project.Name)
	description := ptr.NilToDefault(req.Description, project.Description)
	icon := ptr.NilToDefault(req.Icon, project.Icon)
	if !validateChatProjectFields(ctx, rw, name, description, icon) {
		return
	}

	updated, err := api.Database.UpdateChatProjectByID(ctx, database.UpdateChatProjectByIDParams{
		ID:          project.ID,
		Name:        name,
		Description: description,
		Icon:        icon,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to update chat project.",
			Detail:  err.Error(),
		})
		return
	}
	aReq.New = updated
	httpapi.Write(ctx, rw, http.StatusOK, api.convertChatProject(r, updated))
}

// @Summary Delete chat project
// @ID delete-chat-project
// @Security CoderSessionToken
// @Tags Chats
// @Param organization path string true "Organization ID" format(uuid)
// @Param project path string true "Chat project ID" format(uuid)
// @Success 204
// @Router /api/experimental/organizations/{organization}/chats/projects/{project} [delete]
// @x-apidocgen {"skip": true}
func (api *API) deleteChatProject(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project := httpmw.ChatProjectParam(r)
	if !api.authorizeChatProjectChange(rw, r, policy.ActionDelete, project.RBACObject()) {
		return
	}

	aReq, commitAudit := audit.InitRequest[database.ChatProject](rw, &audit.RequestParams{
		Audit:          *api.Auditor.Load(),
		Log:            api.Logger,
		Request:        r,
		Action:         database.AuditActionDelete,
		OrganizationID: project.OrganizationID,
	})
	defer commitAudit()
	aReq.Old = project

	var deleted []database.Chat
	var err error
	if api.chatDaemon != nil {
		deleted, err = api.chatDaemon.DeleteChatProject(ctx, project.ID)
	} else {
		// Without the AI Gateway no worker runs chats and no sidebar needs
		// watch events, so the deletion runs directly.
		deleted, err = chatd.DeleteChatProjectWithoutEvents(ctx, api.Database, project.ID, chatd.DefaultInFlightChatStaleAfter)
	}
	api.auditChatProjectChatDeletes(ctx, r, project, deleted)
	if errors.Is(err, chatd.ErrChatProjectHasRunningChats) {
		message := "A chat in this project is running, possibly one started by a user the project is shared with. Try again after it finishes."
		if len(deleted) > 0 {
			message = "Some of the project's chats were already deleted. A chat in this project is running, possibly one started by a user the project is shared with. Try again after it finishes to delete the rest."
		}
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{Message: message})
		return
	}
	if errors.Is(err, sql.ErrNoRows) || httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to delete chat project.",
			Detail:  err.Error(),
		})
		return
	}
	rw.WriteHeader(http.StatusNoContent)
}

// auditChatProjectChatDeletes records each root chat deleted with its
// project. Sub-chats go with their root and are not audited separately,
// matching archive.
func (api *API) auditChatProjectChatDeletes(ctx context.Context, r *http.Request, project database.ChatProject, chats []database.Chat) {
	apiKey := httpmw.APIKey(r)
	auditor := api.Auditor.Load()
	auditCtx := context.WithoutCancel(ctx)
	for _, chat := range chats {
		if chat.IsSubChat() {
			continue
		}
		audit.BackgroundAudit(auditCtx, &audit.BackgroundAuditParams[database.Chat]{
			Audit:          *auditor,
			Log:            api.Logger,
			UserID:         apiKey.UserID,
			RequestID:      httpmw.RequestID(r),
			Status:         http.StatusNoContent,
			Action:         database.AuditActionDelete,
			OrganizationID: project.OrganizationID,
			IP:             r.RemoteAddr,
			UserAgent:      r.UserAgent(),
			Old:            chat,
		})
	}
}

// authorizeChatProjectChange authorizes action on a project or its
// memories. Callers who cannot read the object get 404 so the response does
// not reveal that the project exists.
func (api *API) authorizeChatProjectChange(rw http.ResponseWriter, r *http.Request, action policy.Action, object rbac.Objecter) bool {
	if !api.Authorize(r, policy.ActionRead, object) {
		httpapi.ResourceNotFound(rw)
		return false
	}
	if !api.Authorize(r, action, object) {
		httpapi.Forbidden(rw)
		return false
	}
	return true
}

// convertChatProject includes the caller's permissions so clients show
// only the actions that will succeed.
func (api *API) convertChatProject(r *http.Request, project database.ChatProject) codersdk.ChatProject {
	obj := project.RBACObject()
	return db2sdk.ChatProject(project, codersdk.ChatProjectPermissions{
		Update: api.Authorize(r, policy.ActionUpdate, obj),
		Delete: api.Authorize(r, policy.ActionDelete, obj),
		Share:  api.Authorize(r, policy.ActionShare, obj),
	})
}

// maxChatProjectsPerOwner caps how many projects one user owns across all
// organizations. It does not bound projects shared with the user, so the
// project list can return more.
const maxChatProjectsPerOwner = 100

var errChatProjectLimit = xerrors.New("chat project limit reached")

const (
	// The project name is embedded in every generation's system prompt and
	// memory tool descriptions, so it is kept short.
	chatProjectNameMaxChars        = 64
	chatProjectDescriptionMaxChars = 1024
	chatProjectIconMaxChars        = 256
)

// validateChatProjectFields writes a 400 response and returns false when a
// sanitized field exceeds its limit.
func validateChatProjectFields(ctx context.Context, rw http.ResponseWriter, name, description, icon string) bool {
	if utf8.RuneCountInString(name) > chatProjectNameMaxChars {
		writeChatProjectFieldError(ctx, rw, "name", fmt.Sprintf("Name must be at most %d characters.", chatProjectNameMaxChars))
		return false
	}
	if utf8.RuneCountInString(description) > chatProjectDescriptionMaxChars {
		writeChatProjectFieldError(ctx, rw, "description", fmt.Sprintf("Description must be at most %d characters.", chatProjectDescriptionMaxChars))
		return false
	}
	if utf8.RuneCountInString(icon) > chatProjectIconMaxChars {
		writeChatProjectFieldError(ctx, rw, "icon", fmt.Sprintf("Icon must be at most %d characters.", chatProjectIconMaxChars))
		return false
	}
	return true
}

func writeChatProjectFieldError(ctx context.Context, rw http.ResponseWriter, field, detail string) {
	httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
		Message:     detail,
		Validations: []codersdk.ValidationError{{Field: field, Detail: detail}},
	})
}

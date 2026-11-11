package coderd

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
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
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary List chat projects
// @ID list-chat-projects
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Param archived query bool false "List archived projects instead of active ones"
// @Success 200 {array} codersdk.ChatProject
// @Router /api/experimental/chats/projects [get]
// @x-apidocgen {"skip": true}
func (api *API) listChatProjects(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)

	archived := false
	if raw := r.URL.Query().Get("archived"); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message:     "Invalid archived query parameter.",
				Validations: []codersdk.ValidationError{{Field: "archived", Detail: "Must be true or false."}},
			})
			return
		}
		archived = parsed
	}

	projects, err := api.Database.GetChatProjectsOwnedOrSharedWithUserID(ctx, database.GetChatProjectsOwnedOrSharedWithUserIDParams{
		UserID:   apiKey.UserID,
		Archived: sql.NullBool{Bool: archived, Valid: true},
	})
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
	if rejectArchivedChatProject(ctx, rw, project) {
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

	var updated database.ChatProject
	err := api.Database.InTx(func(tx database.Store) error {
		if err := lockUnarchivedChatProject(ctx, tx.GetChatProjectByIDForUpdate, project.ID); err != nil {
			return err
		}
		var err error
		updated, err = tx.UpdateChatProjectByID(ctx, database.UpdateChatProjectByIDParams{
			ID:          project.ID,
			Name:        name,
			Description: description,
			Icon:        icon,
		})
		return err
	}, nil)
	if writeChatProjectArchivedError(ctx, rw, err) {
		return
	}
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

	err := api.Database.DeleteChatProjectByID(ctx, project.ID)
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

// @Summary Archive chat project
// @ID archive-chat-project
// @Security CoderSessionToken
// @Tags Chats
// @Accept json
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param project path string true "Chat project ID" format(uuid)
// @Param request body codersdk.ArchiveChatProjectRequest false "Archive chat project request"
// @Success 200 {object} codersdk.ChatProject
// @Router /api/experimental/organizations/{organization}/chats/projects/{project}/archive [post]
// @x-apidocgen {"skip": true}
func (api *API) archiveChatProject(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project := httpmw.ChatProjectParam(r)
	// Archiving changes every user's chats in the project, so it needs the
	// same access as deleting the project.
	if !api.authorizeChatProjectChange(rw, r, policy.ActionDelete, project.RBACObject()) {
		return
	}

	// The body is optional; an empty body archives like {}.
	var req codersdk.ArchiveChatProjectRequest
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<16))
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to read request body.",
			Detail:  err.Error(),
		})
		return
	}
	if len(bytes.TrimSpace(body)) > 0 {
		r.Body = io.NopCloser(bytes.NewReader(body))
		if !httpapi.Read(ctx, rw, r, &req) {
			return
		}
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

	archived, err := api.chatDaemon.ArchiveChatProject(ctx, project.ID, req.StopActiveChats)
	var activeErr *chatstate.ChatProjectActiveChatsError
	switch {
	case errors.As(err, &activeErr):
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
			Message: chatProjectActiveChatsMessage(activeErr.ActiveChatCount),
		})
		return
	case errors.Is(err, chatstate.ErrChatProjectAlreadyArchived):
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
			Message: "Chat project is already archived.",
		})
		return
	case errors.Is(err, chatstate.ErrChatProjectNotFound) || httpapi.Is404Error(err):
		httpapi.ResourceNotFound(rw)
		return
	case err != nil:
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to archive chat project.",
			Detail:  err.Error(),
		})
		return
	}
	aReq.New = archived
	httpapi.Write(ctx, rw, http.StatusOK, api.convertChatProject(r, archived))
}

// chatProjectActiveChatsMessage explains why an archive without
// stop_active_chats was refused.
func chatProjectActiveChatsMessage(count int64) string {
	if count == 1 {
		return "Chat project has 1 active chat. Set stop_active_chats to stop and archive it."
	}
	return fmt.Sprintf("Chat project has %d active chats. Set stop_active_chats to stop and archive them.", count)
}

// @Summary Unarchive chat project
// @ID unarchive-chat-project
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param project path string true "Chat project ID" format(uuid)
// @Success 200 {object} codersdk.ChatProject
// @Router /api/experimental/organizations/{organization}/chats/projects/{project}/unarchive [post]
// @x-apidocgen {"skip": true}
func (api *API) unarchiveChatProject(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project := httpmw.ChatProjectParam(r)
	if !api.authorizeChatProjectChange(rw, r, policy.ActionDelete, project.RBACObject()) {
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

	unarchived, err := api.chatDaemon.UnarchiveChatProject(ctx, project.ID)
	switch {
	case errors.Is(err, chatstate.ErrChatProjectNotArchived):
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
			Message: "Chat project is not archived.",
		})
		return
	case errors.Is(err, chatstate.ErrChatProjectNotFound) || httpapi.Is404Error(err):
		httpapi.ResourceNotFound(rw)
		return
	case err != nil:
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to unarchive chat project.",
			Detail:  err.Error(),
		})
		return
	}
	aReq.New = unarchived
	httpapi.Write(ctx, rw, http.StatusOK, api.convertChatProject(r, unarchived))
}

// @Summary Preview chat project archive
// @ID preview-chat-project-archive
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param project path string true "Chat project ID" format(uuid)
// @Success 200 {object} codersdk.ChatProjectArchivePreview
// @Router /api/experimental/organizations/{organization}/chats/projects/{project}/archive-preview [get]
// @x-apidocgen {"skip": true}
func (api *API) chatProjectArchivePreview(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project := httpmw.ChatProjectParam(r)
	if !api.authorizeChatProjectChange(rw, r, policy.ActionDelete, project.RBACObject()) {
		return
	}
	// The counts span every user's chats in the project, which the caller,
	// authorized to archive it, may not be able to read.
	//nolint:gocritic // See above.
	preview, err := api.Database.CountChatProjectArchivePreview(dbauthz.AsSystemRestricted(ctx), project.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to preview chat project archive.",
			Detail:  err.Error(),
		})
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, codersdk.ChatProjectArchivePreview{
		ChatCount:       preview.ChatCount,
		ActiveChatCount: preview.ActiveChatCount,
	})
}

// rejectArchivedChatProject writes a 409 response and returns true when the
// project is archived. Archived projects are read-only until unarchived.
func rejectArchivedChatProject(ctx context.Context, rw http.ResponseWriter, project database.ChatProject) bool {
	if !project.ArchivedAt.Valid {
		return false
	}
	httpapi.Write(ctx, rw, http.StatusConflict, chatProjectArchivedResponse)
	return true
}

var chatProjectArchivedResponse = codersdk.Response{Message: "Chat project is archived."}

// errChatProjectArchived reports that a project write found the project
// archived under the project's row lock.
var errChatProjectArchived = xerrors.New("chat project is archived")

// lockUnarchivedChatProject locks the project row with lock, a
// GetChatProjectByIDFor* query of the write's transaction, and returns
// errChatProjectArchived if the project is archived. Archiving locks the row
// FOR UPDATE, so a write that holds the lock until commit either commits
// before the archive or sees it. Use FOR SHARE unless the transaction later
// takes a stronger lock on the row, such as an update of the row or a skill
// insert, whose trigger locks it FOR NO KEY UPDATE: two writers upgrading
// from FOR SHARE would deadlock, so those use FOR UPDATE.
func lockUnarchivedChatProject(ctx context.Context, lock func(context.Context, uuid.UUID) (database.ChatProject, error), projectID uuid.UUID) error {
	// The caller authorized the write, and roles that grant update without
	// read can write.
	//nolint:gocritic // See above.
	project, err := lock(dbauthz.AsSystemRestricted(ctx), projectID)
	if err != nil {
		return xerrors.Errorf("lock chat project: %w", err)
	}
	if project.ArchivedAt.Valid {
		return errChatProjectArchived
	}
	return nil
}

// writeChatProjectArchivedError writes a 409 response and returns true when
// err reports an archived project.
func writeChatProjectArchivedError(ctx context.Context, rw http.ResponseWriter, err error) bool {
	if !errors.Is(err, errChatProjectArchived) {
		return false
	}
	httpapi.Write(ctx, rw, http.StatusConflict, chatProjectArchivedResponse)
	return true
}

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

func (api *API) convertChatProject(r *http.Request, project database.ChatProject) codersdk.ChatProject {
	ctx := r.Context()
	subject := httpmw.UserAuthorization(ctx)
	obj := project.RBACObject()
	// Denials are expected here, so this skips api.Authorize, which logs
	// each one as a warning.
	can := func(action policy.Action) bool {
		return api.HTTPAuth.Authorizer.Authorize(ctx, subject, action, obj) == nil
	}
	return db2sdk.ChatProject(project, codersdk.ChatProjectPermissions{
		Update: can(policy.ActionUpdate),
		Delete: can(policy.ActionDelete),
		Share:  can(policy.ActionShare),
		// Archiving is gated like deleting; see archiveChatProject.
		Archive: can(policy.ActionDelete),
	})
}

// maxChatProjectsPerOwner caps how many projects one user owns across all
// organizations.
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

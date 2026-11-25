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
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
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
	converted, err := api.convertChatProjects(r, projects)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to list chat projects.",
			Detail:  err.Error(),
		})
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, converted)
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
	req.TemplateIDs = slice.Unique(req.TemplateIDs)
	templates, err := api.attachableChatProjectTemplates(ctx, organization.ID, req.TemplateIDs)
	if err != nil {
		api.writeChatProjectTemplateError(ctx, rw, err, "Failed to validate chat project templates.")
		return
	}
	defaultTemplate, err := validateChatProjectDefaultTemplate(ctx, api.Database, templates, req.DefaultTemplate)
	if err != nil {
		api.writeChatProjectTemplateError(ctx, rw, err, "Failed to validate the default template.")
		return
	}

	// The project's own audit diff does not hold its templates, so the ones
	// attached here are audited like later attachments. Deferred first, so
	// it is recorded after the project's creation.
	templatesAudit, commitTemplatesAudit := api.initChatProjectTemplatesAudit(rw, r, organization.ID)
	defer commitTemplatesAudit()
	aReq, commitAudit := audit.InitRequest[database.ChatProject](rw, &audit.RequestParams{
		Audit:          *api.Auditor.Load(),
		Log:            api.Logger,
		Request:        r,
		Action:         database.AuditActionCreate,
		OrganizationID: organization.ID,
	})
	defer commitAudit()

	var project database.ChatProject
	err = api.Database.InTx(func(tx database.Store) error {
		// The lock serializes a user's creates so concurrent requests cannot
		// both observe room under the cap, and orders the creator's pin with
		// their other pin writes.
		if err := tx.AcquireLock(ctx, chatProjectsOwnerLockID(apiKey.UserID)); err != nil {
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

			LimitTemplates:                 req.LimitTemplates,
			DefaultTemplateID:              defaultTemplate.templateID,
			DefaultTemplateVersionPresetID: defaultTemplate.presetID,
			DefaultParameters:              defaultTemplate.parameters,
		})
		if err != nil {
			return err
		}
		if len(req.TemplateIDs) > 0 {
			_, err = tx.InsertChatProjectTemplates(ctx, database.InsertChatProjectTemplatesParams{
				ProjectID:   project.ID,
				TemplateIDs: req.TemplateIDs,
				CreatedBy:   apiKey.UserID,
			})
			if err != nil {
				return err
			}
		}
		// New projects start pinned for their creator.
		return tx.PinChatProject(ctx, database.PinChatProjectParams{
			UserID:    apiKey.UserID,
			ProjectID: project.ID,
			Source:    database.ChatProjectPinSourceDefault,
		})
	}, nil)
	if errors.Is(err, errChatProjectLimit) {
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
			Message: fmt.Sprintf("You can have at most %d chat projects. Delete a project to create another.", maxChatProjectsPerOwner),
		})
		return
	}
	if err != nil {
		api.writeChatProjectTemplateError(ctx, rw, err, "Failed to create chat project.")
		return
	}
	aReq.New = project
	if len(req.TemplateIDs) > 0 {
		templatesAudit.Old = database.AuditableChatProjectTemplates{
			ProjectID:   project.ID,
			ProjectName: project.Name,
			TemplateIDs: []uuid.UUID{},
		}
		templatesAudit.New = templatesAudit.Old
		templatesAudit.New.TemplateIDs = req.TemplateIDs
	}
	api.writeChatProject(rw, r, http.StatusCreated, project)
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
	project := httpmw.ChatProjectParam(r)
	if !api.Authorize(r, policy.ActionRead, project.RBACObject()) {
		httpapi.ResourceNotFound(rw)
		return
	}
	api.writeChatProject(rw, r, http.StatusOK, project)
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
	// Update covers the template limit and default template, so anyone
	// allowed to edit the project, including through its ACL, can change
	// them.
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

	// Omitted template settings are left to the query, which keeps the
	// current columns instead of writing back this request's snapshot.
	var limitTemplates sql.NullBool
	if req.LimitTemplates != nil {
		limitTemplates = sql.NullBool{Bool: *req.LimitTemplates, Valid: true}
	}
	var updated database.ChatProject
	err := api.Database.InTx(func(tx database.Store) error {
		var defaultTemplate chatProjectDefaultTemplate
		if req.DefaultTemplate != nil {
			// Detaching a template takes the same lock, so the default
			// cannot be pointed at a template that is being detached.
			if _, err := tx.GetChatProjectByIDForUpdate(ctx, project.ID); err != nil {
				return err
			}
			templates, err := chattool.LoadProjectTemplates(ctx, tx, project)
			if err != nil {
				return err
			}
			defaultTemplate, err = validateChatProjectDefaultTemplate(ctx, tx, templates.Templates, req.DefaultTemplate)
			if err != nil {
				return err
			}
		}
		var err error
		updated, err = tx.UpdateChatProjectByID(ctx, database.UpdateChatProjectByIDParams{
			ID:          project.ID,
			Name:        name,
			Description: description,
			Icon:        icon,

			LimitTemplates:                 limitTemplates,
			UpdateDefaultTemplate:          req.DefaultTemplate != nil,
			DefaultTemplateID:              defaultTemplate.templateID,
			DefaultTemplateVersionPresetID: defaultTemplate.presetID,
			DefaultParameters:              defaultTemplate.parameters,
		})
		return err
	}, nil)
	if err != nil {
		api.writeChatProjectTemplateError(ctx, rw, err, "Failed to update chat project.")
		return
	}
	aReq.New = updated
	api.writeChatProject(rw, r, http.StatusOK, updated)
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

// writeChatProject writes the project as the caller sees it.
func (api *API) writeChatProject(rw http.ResponseWriter, r *http.Request, status int, project database.ChatProject) {
	ctx := r.Context()
	converted, err := api.convertChatProjects(r, []database.ChatProject{project})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to load chat project templates.",
			Detail:  err.Error(),
		})
		return
	}
	httpapi.Write(ctx, rw, status, converted[0])
}

// convertChatProjects converts projects with the caller's permissions on
// them and the attached templates the caller can access.
func (api *API) convertChatProjects(r *http.Request, projects []database.ChatProject) ([]codersdk.ChatProject, error) {
	ctx := r.Context()
	templates, err := chattool.LoadProjectsTemplates(ctx, api.Database, projects)
	if err != nil {
		return nil, err
	}
	subject := httpmw.UserAuthorization(ctx)
	return slice.List(projects, func(project database.ChatProject) codersdk.ChatProject {
		obj := project.RBACObject()
		// Denials are expected here, so this skips api.Authorize, which
		// logs each one as a warning.
		can := func(action policy.Action) bool {
			return api.HTTPAuth.Authorizer.Authorize(ctx, subject, action, obj) == nil
		}
		return db2sdk.ChatProject(project, codersdk.ChatProjectPermissions{
			Update: can(policy.ActionUpdate),
			Delete: can(policy.ActionDelete),
			Share:  can(policy.ActionShare),
		}, templates[project.ID])
	}), nil
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

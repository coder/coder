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

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary List chat projects in an organization
// @ID list-organization-chat-projects
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Success 200 {array} codersdk.ChatProject
// @Router /api/experimental/organizations/{organization}/chats/projects [get]
// @x-apidocgen {"skip": true}
func (api *API) listOrganizationChatProjects(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	organization := httpmw.OrganizationParam(r)

	projects, err := api.Database.GetChatProjectsByOrganizationID(ctx, organization.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to list chat projects.",
			Detail:  err.Error(),
		})
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, slice.List(projects, db2sdk.ChatProject))
}

// @Summary List the authenticated user's chat projects
// @ID list-user-chat-projects
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Success 200 {array} codersdk.ChatProject
// @Router /api/experimental/chats/projects [get]
// @x-apidocgen {"skip": true}
func (api *API) listUserChatProjects(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)

	projects, err := api.Database.GetChatProjectsByOwnerID(ctx, apiKey.UserID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to list chat projects.",
			Detail:  err.Error(),
		})
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, slice.List(projects, db2sdk.ChatProject))
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

	project, err := api.Database.InsertChatProject(ctx, database.InsertChatProjectParams{
		ID:             uuid.NullUUID{},
		OrganizationID: organization.ID,
		OwnerID:        apiKey.UserID,
		Name:           req.Name,
		Description:    req.Description,
		Icon:           req.Icon,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to create chat project.",
			Detail:  err.Error(),
		})
		return
	}
	aReq.New = project
	httpapi.Write(ctx, rw, http.StatusCreated, db2sdk.ChatProject(project))
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
	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.ChatProject(project))
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
	if !api.Authorize(r, policy.ActionUpdate, project.RBACObject()) {
		httpapi.ResourceNotFound(rw)
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
	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.ChatProject(updated))
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
	if !api.Authorize(r, policy.ActionDelete, project.RBACObject()) {
		httpapi.ResourceNotFound(rw)
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

package coderd

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary List chat projects
// @ID list-chat-projects
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Param organization query string true "Organization ID" format(uuid)
// @Success 200 {array} codersdk.ChatProject
// @Router /api/experimental/chats/projects [get]
// @x-apidocgen {"skip": true}
func (api *API) listChatProjects(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	organizationID, err := uuid.Parse(r.URL.Query().Get("organization"))
	if err != nil || organizationID == uuid.Nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{Message: "organization query parameter is required."})
		return
	}

	projects, err := api.Database.GetChatProjectsByOrganizationID(ctx, organizationID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to list chat projects.",
			Detail:  err.Error(),
		})
		return
	}

	response := make([]codersdk.ChatProject, len(projects))
	for i, project := range projects {
		response[i] = db2sdk.ChatProjectRow(project)
	}
	httpapi.Write(ctx, rw, http.StatusOK, response)
}

// @Summary Create chat project
// @ID create-chat-project
// @Security CoderSessionToken
// @Tags Chats
// @Accept json
// @Produce json
// @Param request body codersdk.CreateChatProjectRequest true "Create chat project request"
// @Success 201 {object} codersdk.ChatProject
// @Router /api/experimental/chats/projects [post]
// @x-apidocgen {"skip": true}
func (api *API) postChatProject(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)

	var req codersdk.CreateChatProjectRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	if req.OrganizationID == uuid.Nil || strings.TrimSpace(req.Name) == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{Message: "organization_id and name are required."})
		return
	}
	if !api.Authorize(r, policy.ActionCreate, database.ChatProject{
		OrganizationID: req.OrganizationID,
		CreatedBy:      apiKey.UserID,
	}.RBACObject()) {
		httpapi.Forbidden(rw)
		return
	}

	aReq, commitAudit := audit.InitRequest[database.ChatProject](rw, &audit.RequestParams{
		Audit:          *api.Auditor.Load(),
		Log:            api.Logger,
		Request:        r,
		Action:         database.AuditActionCreate,
		OrganizationID: req.OrganizationID,
	})
	defer commitAudit()

	project, err := api.Database.InsertChatProject(ctx, database.InsertChatProjectParams{
		ID:             uuid.NullUUID{},
		OrganizationID: req.OrganizationID,
		CreatedBy:      apiKey.UserID,
		Name:           strings.TrimSpace(req.Name),
		Description:    req.Description,
	})
	if database.IsUniqueViolation(err) {
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{Message: "A chat project with this name already exists in the organization."})
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
	httpapi.Write(ctx, rw, http.StatusCreated, db2sdk.ChatProject(project))
}

// @Summary Get chat project
// @ID get-chat-project
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Param project path string true "Chat project ID" format(uuid)
// @Success 200 {object} codersdk.ChatProject
// @Router /api/experimental/chats/projects/{project} [get]
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
// @Param project path string true "Chat project ID" format(uuid)
// @Param request body codersdk.UpdateChatProjectRequest true "Update chat project request"
// @Success 200 {object} codersdk.ChatProject
// @Router /api/experimental/chats/projects/{project} [patch]
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
	name := project.Name
	if req.Name != nil {
		name = strings.TrimSpace(*req.Name)
	}
	if name == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{Message: "name must not be blank."})
		return
	}
	description := project.Description
	if req.Description != nil {
		description = *req.Description
	}

	updated, err := api.Database.UpdateChatProjectByID(ctx, database.UpdateChatProjectByIDParams{
		ID:          project.ID,
		Name:        name,
		Description: description,
	})
	if database.IsUniqueViolation(err) {
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{Message: "A chat project with this name already exists in the organization."})
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
	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.ChatProject(updated))
}

// @Summary Delete chat project
// @ID delete-chat-project
// @Security CoderSessionToken
// @Tags Chats
// @Param project path string true "Chat project ID" format(uuid)
// @Success 204
// @Router /api/experimental/chats/projects/{project} [delete]
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

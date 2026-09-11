package coderd

import (
	"database/sql"
	"errors"
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
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary List chat project memories
// @ID list-chat-project-memories
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Param project path string true "Chat project ID" format(uuid)
// @Success 200 {array} codersdk.ChatProjectMemory
// @Router /api/experimental/chats/projects/{project}/memories [get]
// @x-apidocgen {"skip": true}
func (api *API) listChatProjectMemories(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project := httpmw.ChatProjectParam(r)
	if !api.Authorize(r, policy.ActionRead, project.RBACObject()) {
		httpapi.ResourceNotFound(rw)
		return
	}
	memories, err := api.Database.GetChatProjectMemoriesByProjectID(ctx, project.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{Message: "Failed to list chat project memories.", Detail: err.Error()})
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.ChatProjectMemoryRows(memories))
}

// @Summary Create chat project memory
// @ID create-chat-project-memory
// @Security CoderSessionToken
// @Tags Chats
// @Accept json
// @Produce json
// @Param project path string true "Chat project ID" format(uuid)
// @Param request body codersdk.CreateChatProjectMemoryRequest true "Create memory request"
// @Success 201 {object} codersdk.ChatProjectMemory
// @Router /api/experimental/chats/projects/{project}/memories [post]
// @x-apidocgen {"skip": true}
func (api *API) postChatProjectMemory(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project := httpmw.ChatProjectParam(r)
	apiKey := httpmw.APIKey(r)
	if !api.Authorize(r, policy.ActionCreate, rbacMemoryObject(project.OrganizationID)) {
		httpapi.ResourceNotFound(rw)
		return
	}
	var req codersdk.CreateChatProjectMemoryRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	normalized, resp := validateChatProjectMemory(req.Name, req.Description, req.Body)
	if resp != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, *resp)
		return
	}
	count, err := api.Database.CountChatProjectMemoriesByProjectID(ctx, project.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{Message: "Failed to count chat project memories.", Detail: err.Error()})
		return
	}
	if count >= chattool.MaxProjectMemories {
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{Message: "Chat project memory limit reached."})
		return
	}
	aReq, commit := audit.InitRequest[database.ChatProjectMemory](rw, &audit.RequestParams{Audit: *api.Auditor.Load(), Log: api.Logger, Request: r, Action: database.AuditActionCreate, OrganizationID: project.OrganizationID})
	defer commit()
	memory, err := api.Database.InsertChatProjectMemory(ctx, database.InsertChatProjectMemoryParams{ID: uuid.NullUUID{}, ProjectID: project.ID, OrganizationID: project.OrganizationID, Name: normalized.Name, Description: normalized.Description, Body: normalized.Body, SourceChatID: uuid.NullUUID{}, CreatedBy: apiKey.UserID})
	if database.IsUniqueViolation(err) {
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{Message: "A chat project memory with this name already exists."})
		return
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{Message: "Failed to create chat project memory.", Detail: err.Error()})
		return
	}
	aReq.New = memory
	row, err := api.Database.GetChatProjectMemoryByID(ctx, memory.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{Message: "Failed to read chat project memory.", Detail: err.Error()})
		return
	}
	httpapi.Write(ctx, rw, http.StatusCreated, db2sdk.ChatProjectMemory(row))
}

// @Summary Get chat project memory
// @ID get-chat-project-memory
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Param project path string true "Chat project ID" format(uuid)
// @Param memory path string true "Chat project memory ID" format(uuid)
// @Success 200 {object} codersdk.ChatProjectMemory
// @Router /api/experimental/chats/projects/{project}/memories/{memory} [get]
// @x-apidocgen {"skip": true}
//
//nolint:revive // HTTP handler writes to ResponseWriter.
func (api *API) getChatProjectMemory(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project := httpmw.ChatProjectParam(r)
	memory := httpmw.ChatProjectMemoryParam(r)
	if memory.ChatProjectMemory.ProjectID != project.ID || !api.Authorize(r, policy.ActionRead, memory.RBACObject()) {
		httpapi.ResourceNotFound(rw)
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.ChatProjectMemory(memory))
}

// @Summary Update chat project memory
// @ID update-chat-project-memory
// @Security CoderSessionToken
// @Tags Chats
// @Accept json
// @Produce json
// @Param project path string true "Chat project ID" format(uuid)
// @Param memory path string true "Chat project memory ID" format(uuid)
// @Param request body codersdk.UpdateChatProjectMemoryRequest true "Update memory request"
// @Success 200 {object} codersdk.ChatProjectMemory
// @Router /api/experimental/chats/projects/{project}/memories/{memory} [patch]
// @x-apidocgen {"skip": true}
func (api *API) patchChatProjectMemory(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project := httpmw.ChatProjectParam(r)
	memoryRow := httpmw.ChatProjectMemoryParam(r)
	memory := memoryRow.ChatProjectMemory
	if memory.ProjectID != project.ID || !api.Authorize(r, policy.ActionUpdate, memory.RBACObject()) {
		httpapi.ResourceNotFound(rw)
		return
	}
	var req codersdk.UpdateChatProjectMemoryRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	name := memory.Name
	if req.Name != nil {
		name = *req.Name
	}
	description := memory.Description
	if req.Description != nil {
		description = *req.Description
	}
	body := memory.Body
	if req.Body != nil {
		body = *req.Body
	}
	normalized, resp := validateChatProjectMemory(name, description, body)
	if resp != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, *resp)
		return
	}
	aReq, commit := audit.InitRequest[database.ChatProjectMemory](rw, &audit.RequestParams{Audit: *api.Auditor.Load(), Log: api.Logger, Request: r, Action: database.AuditActionWrite, OrganizationID: project.OrganizationID})
	defer commit()
	aReq.Old = memory
	updated, err := api.Database.UpdateChatProjectMemoryByID(ctx, database.UpdateChatProjectMemoryByIDParams{ID: memory.ID, Name: normalized.Name, Description: normalized.Description, Body: normalized.Body})
	if database.IsUniqueViolation(err) {
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{Message: "A chat project memory with this name already exists."})
		return
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{Message: "Failed to update chat project memory.", Detail: err.Error()})
		return
	}
	aReq.New = updated
	row, err := api.Database.GetChatProjectMemoryByID(ctx, updated.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{Message: "Failed to read chat project memory.", Detail: err.Error()})
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.ChatProjectMemory(row))
}

// @Summary Delete chat project memory
// @ID delete-chat-project-memory
// @Security CoderSessionToken
// @Tags Chats
// @Param project path string true "Chat project ID" format(uuid)
// @Param memory path string true "Chat project memory ID" format(uuid)
// @Success 204
// @Router /api/experimental/chats/projects/{project}/memories/{memory} [delete]
// @x-apidocgen {"skip": true}
func (api *API) deleteChatProjectMemory(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project := httpmw.ChatProjectParam(r)
	memory := httpmw.ChatProjectMemoryParam(r)
	if memory.ChatProjectMemory.ProjectID != project.ID || !api.Authorize(r, policy.ActionDelete, memory.RBACObject()) {
		httpapi.ResourceNotFound(rw)
		return
	}
	aReq, commit := audit.InitRequest[database.ChatProjectMemory](rw, &audit.RequestParams{Audit: *api.Auditor.Load(), Log: api.Logger, Request: r, Action: database.AuditActionDelete, OrganizationID: project.OrganizationID})
	defer commit()
	aReq.Old = memory.ChatProjectMemory
	if err := api.Database.DeleteChatProjectMemoryByID(ctx, memory.ChatProjectMemory.ID); errors.Is(err, sql.ErrNoRows) || httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return
	} else if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{Message: "Failed to delete chat project memory.", Detail: err.Error()})
		return
	}
	rw.WriteHeader(http.StatusNoContent)
}

type normalizedChatProjectMemory struct {
	Name        string
	Description string
	Body        string
}

func validateChatProjectMemory(name, description, body string) (normalizedChatProjectMemory, *codersdk.Response) {
	name = strings.ToLower(strings.TrimSpace(name))
	description = chattool.NormalizeProjectMemoryText(description)
	body = chattool.NormalizeProjectMemoryText(body)
	if err := chattool.ValidateProjectMemoryName(name); err != nil {
		return normalizedChatProjectMemory{}, &codersdk.Response{Message: err.Error()}
	}
	if description == "" || utf8.RuneCountInString(description) > chattool.MaxProjectMemoryDescriptionChars {
		return normalizedChatProjectMemory{}, &codersdk.Response{Message: "description must be at most 150 characters."}
	}
	if body == "" || len(body) > chattool.MaxProjectMemoryBodyBytes {
		return normalizedChatProjectMemory{}, &codersdk.Response{Message: "body must be at most 8192 bytes."}
	}
	return normalizedChatProjectMemory{Name: name, Description: description, Body: body}, nil
}

func rbacMemoryObject(organizationID uuid.UUID) database.ChatProjectMemory {
	return database.ChatProjectMemory{OrganizationID: organizationID}
}

package coderd

import (
	"database/sql"
	"errors"
	"net/http"

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
// @Param organization path string true "Organization ID" format(uuid)
// @Param project path string true "Chat project ID" format(uuid)
// @Success 200 {array} codersdk.ChatProjectMemory
// @Router /api/experimental/organizations/{organization}/chats/projects/{project}/memories [get]
// @x-apidocgen {"skip": true}
func (api *API) listChatProjectMemories(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project := httpmw.ChatProjectParam(r)
	if !api.Authorize(r, policy.ActionRead, database.ChatProjectMemoryRBACObject(project)) {
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
// @Param organization path string true "Organization ID" format(uuid)
// @Param project path string true "Chat project ID" format(uuid)
// @Param request body codersdk.CreateChatProjectMemoryRequest true "Create memory request"
// @Success 201 {object} codersdk.ChatProjectMemory
// @Router /api/experimental/organizations/{organization}/chats/projects/{project}/memories [post]
// @x-apidocgen {"skip": true}
func (api *API) postChatProjectMemory(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project := httpmw.ChatProjectParam(r)
	apiKey := httpmw.APIKey(r)
	if !api.Authorize(r, policy.ActionCreate, database.ChatProjectMemoryRBACObject(project)) {
		httpapi.ResourceNotFound(rw)
		return
	}
	var req codersdk.CreateChatProjectMemoryRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	normalized, err := chattool.NormalizeMemoryInput(req.Name, req.Description, req.Body)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{Message: "Invalid chat project memory.", Detail: err.Error()})
		return
	}
	aReq, commit := audit.InitRequest[database.ChatProjectMemory](rw, &audit.RequestParams{Audit: *api.Auditor.Load(), Log: api.Logger, Request: r, Action: database.AuditActionCreate, OrganizationID: project.OrganizationID})
	defer commit()
	memory, _, err := chattool.InsertProjectMemory(ctx, api.Database, database.InsertChatProjectMemoryParams{ID: uuid.NullUUID{}, ProjectID: project.ID, OrganizationID: project.OrganizationID, Name: normalized.Name, Description: normalized.Description, Body: normalized.Body, CreatedBy: apiKey.UserID})
	if errors.Is(err, chattool.ErrMemoryLimit) {
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{Message: "Chat project memory limit reached."})
		return
	}
	if errors.Is(err, chattool.ErrMemoryExists) {
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{Message: "A chat project memory with this name already exists."})
		return
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{Message: "Failed to create chat project memory.", Detail: err.Error()})
		return
	}
	aReq.New = memory
	httpapi.Write(ctx, rw, http.StatusCreated, db2sdk.ChatProjectMemory(database.GetChatProjectMemoryByIDRow{
		ChatProjectMemory: memory,
		CreatedByUsername: httpmw.UserAuthorization(ctx).FriendlyName,
	}))
}

// @Summary Get chat project memory
// @ID get-chat-project-memory
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param project path string true "Chat project ID" format(uuid)
// @Param memory path string true "Chat project memory ID" format(uuid)
// @Success 200 {object} codersdk.ChatProjectMemory
// @Router /api/experimental/organizations/{organization}/chats/projects/{project}/memories/{memory} [get]
// @x-apidocgen {"skip": true}
//
//nolint:revive // HTTP handler writes to ResponseWriter.
func (api *API) getChatProjectMemory(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project := httpmw.ChatProjectParam(r)
	memory := httpmw.ChatProjectMemoryParam(r)
	if memory.ChatProjectMemory.ProjectID != project.ID || !api.Authorize(r, policy.ActionRead, memory.ChatProjectMemory.RBACObject(project)) {
		httpapi.ResourceNotFound(rw)
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.ChatProjectMemory(memory))
}

// @Summary Delete chat project memory
// @ID delete-chat-project-memory
// @Security CoderSessionToken
// @Tags Chats
// @Param organization path string true "Organization ID" format(uuid)
// @Param project path string true "Chat project ID" format(uuid)
// @Param memory path string true "Chat project memory ID" format(uuid)
// @Success 204
// @Router /api/experimental/organizations/{organization}/chats/projects/{project}/memories/{memory} [delete]
// @x-apidocgen {"skip": true}
func (api *API) deleteChatProjectMemory(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project := httpmw.ChatProjectParam(r)
	memory := httpmw.ChatProjectMemoryParam(r)
	if memory.ChatProjectMemory.ProjectID != project.ID || !api.Authorize(r, policy.ActionDelete, memory.ChatProjectMemory.RBACObject(project)) {
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

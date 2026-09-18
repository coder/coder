package coderd

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary List chat user memories
// @ID list-chat-user-memories
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Param organization query string true "Organization ID" format(uuid)
// @Success 200 {array} codersdk.ChatUserMemory
// @Router /api/experimental/chats/memories [get]
// @x-apidocgen {"skip": true}
func (api *API) listChatUserMemories(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)
	organizationID, err := uuid.Parse(r.URL.Query().Get("organization"))
	if err != nil || organizationID == uuid.Nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{Message: "organization query parameter is required."})
		return
	}

	memories, err := api.Database.GetChatUserMemoriesByUserAndOrganization(ctx, database.GetChatUserMemoriesByUserAndOrganizationParams{
		UserID:         apiKey.UserID,
		OrganizationID: organizationID,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{Message: "Failed to list chat user memories.", Detail: err.Error()})
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.ChatUserMemoryRows(memories))
}

// @Summary Create chat user memory
// @ID create-chat-user-memory
// @Security CoderSessionToken
// @Tags Chats
// @Accept json
// @Produce json
// @Param request body codersdk.CreateChatUserMemoryRequest true "Create memory request"
// @Success 201 {object} codersdk.ChatUserMemory
// @Router /api/experimental/chats/memories [post]
// @x-apidocgen {"skip": true}
func (api *API) postChatUserMemory(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)
	var req codersdk.CreateChatUserMemoryRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	if req.OrganizationID == uuid.Nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{Message: "organization_id is required."})
		return
	}
	normalized, resp := validateChatUserMemory(req.Name, req.Description, req.Body)
	if resp != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, *resp)
		return
	}
	memoryObject := database.ChatUserMemory{OrganizationID: req.OrganizationID, UserID: apiKey.UserID}.RBACObject()
	if !api.Authorize(r, policy.ActionCreate, memoryObject) {
		httpapi.ResourceNotFound(rw)
		return
	}
	aReq, commit := audit.InitRequest[database.ChatUserMemory](rw, &audit.RequestParams{Audit: *api.Auditor.Load(), Log: api.Logger, Request: r, Action: database.AuditActionCreate, OrganizationID: req.OrganizationID})
	defer commit()
	memory, err := chattool.InsertUserMemory(ctx, api.Database, database.InsertChatUserMemoryParams{
		ID:             uuid.NullUUID{},
		OrganizationID: req.OrganizationID,
		UserID:         apiKey.UserID,
		Name:           normalized.Name,
		Description:    normalized.Description,
		Body:           normalized.Body,
		SourceChatID:   uuid.NullUUID{},
	})
	if errors.Is(err, chattool.ErrMemoryLimit) {
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{Message: "Chat user memory limit reached."})
		return
	}
	if database.IsUniqueViolation(err) {
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{Message: "A chat user memory with this name already exists."})
		return
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{Message: "Failed to create chat user memory.", Detail: err.Error()})
		return
	}
	aReq.New = memory
	// Built from the insert result: a create-only token has no read scope
	// for the lookup that would otherwise follow.
	httpapi.Write(ctx, rw, http.StatusCreated, db2sdk.ChatUserMemory(database.GetChatUserMemoryByIDRow{
		ChatUserMemory:    memory,
		CreatedByUsername: httpmw.UserAuthorization(ctx).FriendlyName,
	}))
}

// @Summary Get chat user memory
// @ID get-chat-user-memory
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Param memory path string true "Chat user memory ID" format(uuid)
// @Success 200 {object} codersdk.ChatUserMemory
// @Router /api/experimental/chats/memories/{memory} [get]
// @x-apidocgen {"skip": true}
//
//nolint:revive // HTTP handler writes to ResponseWriter.
func (api *API) getChatUserMemory(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	memory := httpmw.ChatUserMemoryParam(r)
	if !api.Authorize(r, policy.ActionRead, memory.RBACObject()) {
		httpapi.ResourceNotFound(rw)
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.ChatUserMemory(memory))
}

// @Summary Update chat user memory
// @ID update-chat-user-memory
// @Security CoderSessionToken
// @Tags Chats
// @Accept json
// @Produce json
// @Param memory path string true "Chat user memory ID" format(uuid)
// @Param request body codersdk.UpdateChatUserMemoryRequest true "Update memory request"
// @Success 200 {object} codersdk.ChatUserMemory
// @Router /api/experimental/chats/memories/{memory} [patch]
// @x-apidocgen {"skip": true}
func (api *API) patchChatUserMemory(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	memoryRow := httpmw.ChatUserMemoryParam(r)
	memory := memoryRow.ChatUserMemory
	if !api.Authorize(r, policy.ActionUpdate, memory.RBACObject()) {
		httpapi.ResourceNotFound(rw)
		return
	}
	var req codersdk.UpdateChatUserMemoryRequest
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
	normalized, resp := validateChatUserMemory(name, description, body)
	if resp != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, *resp)
		return
	}
	aReq, commit := audit.InitRequest[database.ChatUserMemory](rw, &audit.RequestParams{Audit: *api.Auditor.Load(), Log: api.Logger, Request: r, Action: database.AuditActionWrite, OrganizationID: memory.OrganizationID})
	defer commit()
	aReq.Old = memory
	updated, err := api.Database.UpdateChatUserMemoryByID(ctx, database.UpdateChatUserMemoryByIDParams{ID: memory.ID, Name: normalized.Name, Description: normalized.Description, Body: normalized.Body})
	if database.IsUniqueViolation(err) {
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{Message: "A chat user memory with this name already exists."})
		return
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{Message: "Failed to update chat user memory.", Detail: err.Error()})
		return
	}
	aReq.New = updated
	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.ChatUserMemory(database.GetChatUserMemoryByIDRow{
		ChatUserMemory:    updated,
		CreatedByUsername: memoryRow.CreatedByUsername,
	}))
}

// @Summary Delete chat user memory
// @ID delete-chat-user-memory
// @Security CoderSessionToken
// @Tags Chats
// @Param memory path string true "Chat user memory ID" format(uuid)
// @Success 204
// @Router /api/experimental/chats/memories/{memory} [delete]
// @x-apidocgen {"skip": true}
func (api *API) deleteChatUserMemory(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	memory := httpmw.ChatUserMemoryParam(r)
	if !api.Authorize(r, policy.ActionDelete, memory.RBACObject()) {
		httpapi.ResourceNotFound(rw)
		return
	}
	aReq, commit := audit.InitRequest[database.ChatUserMemory](rw, &audit.RequestParams{Audit: *api.Auditor.Load(), Log: api.Logger, Request: r, Action: database.AuditActionDelete, OrganizationID: memory.ChatUserMemory.OrganizationID})
	defer commit()
	aReq.Old = memory.ChatUserMemory
	if err := api.Database.DeleteChatUserMemoryByID(ctx, memory.ChatUserMemory.ID); errors.Is(err, sql.ErrNoRows) || httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return
	} else if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{Message: "Failed to delete chat user memory.", Detail: err.Error()})
		return
	}
	rw.WriteHeader(http.StatusNoContent)
}

func validateChatUserMemory(name, description, body string) (normalizedChatProjectMemory, *codersdk.Response) {
	return validateChatProjectMemory(name, description, body)
}

// @Summary Get user chat personal memory settings
// @ID get-user-chat-personal-memory-settings
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Success 200 {object} codersdk.ChatPersonalMemorySettings
// @Router /api/v2/chats/config/user-memory [get]
//
//nolint:revive // get-return: revive assumes get* must be a getter, but this is an HTTP handler.
func (api *API) getUserChatPersonalMemorySettings(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)
	enabled, err := api.Database.GetUserChatPersonalMemoryEnabled(ctx, apiKey.UserID)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{Message: "Error reading user chat personal memory settings.", Detail: err.Error()})
			return
		}
		enabled = strconv.FormatBool(true)
	}
	httpapi.Write(ctx, rw, http.StatusOK, codersdk.ChatPersonalMemorySettings{Enabled: enabled == "true"})
}

// @Summary Update user chat personal memory settings
// @ID update-user-chat-personal-memory-settings
// @Security CoderSessionToken
// @Tags Chats
// @Accept json
// @Produce json
// @Param request body codersdk.UpdateChatPersonalMemorySettingsRequest true "Request body"
// @Success 200 {object} codersdk.ChatPersonalMemorySettings
// @Router /api/v2/chats/config/user-memory [put]
func (api *API) putUserChatPersonalMemorySettings(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)
	var req codersdk.UpdateChatPersonalMemorySettingsRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	if _, err := api.Database.UpsertUserChatPersonalMemoryEnabled(ctx, database.UpsertUserChatPersonalMemoryEnabledParams{
		UserID: apiKey.UserID,
		Value:  strconv.FormatBool(req.Enabled),
	}); err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{Message: "Error updating user chat personal memory settings.", Detail: err.Error()})
		return
	}
	publishChatConfigEvent(api.Logger, api.Pubsub, pubsub.ChatConfigEventUserPersonalMemory, apiKey.UserID)
	httpapi.Write(ctx, rw, http.StatusOK, codersdk.ChatPersonalMemorySettings(req))
}

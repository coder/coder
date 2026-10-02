package coderd

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/codersdk"
)

// chatAutomationNextRunCount is the number of upcoming schedule runs
// returned with each automation.
const chatAutomationNextRunCount = 5

// chatAutomationResponse converts row to its SDK form with its upcoming
// schedule runs after now.
func chatAutomationResponse(row database.ChatAutomation, now time.Time) codersdk.ChatAutomation {
	return db2sdk.ChatAutomation(row, chatd.AutomationNextRuns(row, now, chatAutomationNextRunCount))
}

// requireChatAutomations returns 404 unless the chat-automations
// experiment is on for the caller. The experiment is user-scoped, so it
// is decided per request from the runtime rules.
func (api *API) requireChatAutomations(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		apiKey := httpmw.APIKey(r)
		if api.chatDaemon == nil || !chatd.AutomationsEnabled(r.Context(), api.ExperimentEvaluator, apiKey.UserID) {
			httpapi.ResourceNotFound(rw)
			return
		}
		next.ServeHTTP(rw, r)
	})
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
// @Summary List chat automations
// @ID list-chat-automations
// @Security CoderSessionToken
// @Produce json
// @Tags Chats
// @Param organization path string true "Organization ID"
// @Success 200 {array} codersdk.ChatAutomation
// @Router /api/experimental/organizations/{organization}/chat-automations [get]
// @x-apidocgen {"skip": true}
func (api *API) listChatAutomations(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	organization := httpmw.OrganizationParam(r)

	rows, err := api.Database.GetChatAutomationsByOrganizationID(ctx, organization.ID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	now := api.Clock.Now()
	automations := make([]codersdk.ChatAutomation, 0, len(rows))
	for _, row := range rows {
		automations = append(automations, chatAutomationResponse(row, now))
	}
	httpapi.Write(ctx, rw, http.StatusOK, automations)
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
// @Summary Create chat automation
// @ID create-chat-automation
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Chats
// @Param organization path string true "Organization ID"
// @Param request body codersdk.CreateChatAutomationRequest true "Chat automation"
// @Success 201 {object} codersdk.CreateChatAutomationResponse
// @Router /api/experimental/organizations/{organization}/chat-automations [post]
// @x-apidocgen {"skip": true}
func (api *API) postChatAutomation(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)
	organization := httpmw.OrganizationParam(r)

	auditor := api.Auditor.Load()
	aReq, commitAudit := audit.InitRequest[database.ChatAutomation](rw, &audit.RequestParams{
		Audit:          *auditor,
		Log:            api.Logger,
		Request:        r,
		Action:         database.AuditActionCreate,
		OrganizationID: organization.ID,
	})
	defer commitAudit()

	var req codersdk.CreateChatAutomationRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	automation, secret, err := api.chatDaemon.CreateAutomation(ctx, chatd.CreateAutomationParams{
		OrganizationID: organization.ID,
		OwnerID:        apiKey.UserID,
		Request:        req,
	})
	if err != nil {
		api.writeChatAutomationError(ctx, rw, err)
		return
	}
	aReq.New = automation

	httpapi.Write(ctx, rw, http.StatusCreated, codersdk.CreateChatAutomationResponse{
		Automation:    chatAutomationResponse(automation, api.Clock.Now()),
		WebhookSecret: secret,
	})
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
// @Summary Get chat automation
// @ID get-chat-automation
// @Security CoderSessionToken
// @Produce json
// @Tags Chats
// @Param organization path string true "Organization ID"
// @Param automation path string true "Automation ID" format(uuid)
// @Success 200 {object} codersdk.ChatAutomation
// @Router /api/experimental/organizations/{organization}/chat-automations/{automation} [get]
// @x-apidocgen {"skip": true}
func (api *API) chatAutomation(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	automation, ok := api.chatAutomationParam(rw, r)
	if !ok {
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, chatAutomationResponse(automation, api.Clock.Now()))
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
// @Summary Update chat automation
// @Description Only the owner of an automation can update it. The kind and target mode of an automation cannot change.
// @ID update-chat-automation
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Chats
// @Param organization path string true "Organization ID"
// @Param automation path string true "Automation ID" format(uuid)
// @Param request body codersdk.UpdateChatAutomationRequest true "Chat automation changes"
// @Success 200 {object} codersdk.ChatAutomation
// @Router /api/experimental/organizations/{organization}/chat-automations/{automation} [patch]
// @x-apidocgen {"skip": true}
func (api *API) patchChatAutomation(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)
	automation, ok := api.chatAutomationParam(rw, r)
	if !ok {
		return
	}

	auditor := api.Auditor.Load()
	aReq, commitAudit := audit.InitRequest[database.ChatAutomation](rw, &audit.RequestParams{
		Audit:          *auditor,
		Log:            api.Logger,
		Request:        r,
		Action:         database.AuditActionWrite,
		OrganizationID: automation.OrganizationID,
	})
	aReq.Old = automation
	defer commitAudit()

	var req struct {
		codersdk.UpdateChatAutomationRequest
		// Kind and TargetMode are decoded only to reject them: they are
		// fixed at create time.
		Kind       json.RawMessage `json:"kind"`
		TargetMode json.RawMessage `json:"target_mode"`
	}
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	for _, fixed := range []struct {
		field string
		value json.RawMessage
	}{{"kind", req.Kind}, {"target_mode", req.TargetMode}} {
		if fixed.value != nil {
			api.writeChatAutomationError(ctx, rw, &chatd.AutomationValidationError{Field: fixed.field, Detail: "cannot be changed after the automation is created"})
			return
		}
	}

	updated, err := api.chatDaemon.UpdateAutomation(ctx, apiKey.UserID, automation.ID, req.UpdateChatAutomationRequest)
	if err != nil {
		api.writeChatAutomationError(ctx, rw, err)
		return
	}
	aReq.New = updated

	httpapi.Write(ctx, rw, http.StatusOK, chatAutomationResponse(updated, api.Clock.Now()))
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
// @Summary Delete chat automation
// @ID delete-chat-automation
// @Security CoderSessionToken
// @Tags Chats
// @Param organization path string true "Organization ID"
// @Param automation path string true "Automation ID" format(uuid)
// @Success 204
// @Router /api/experimental/organizations/{organization}/chat-automations/{automation} [delete]
// @x-apidocgen {"skip": true}
func (api *API) deleteChatAutomation(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	automation, ok := api.chatAutomationParam(rw, r)
	if !ok {
		return
	}

	auditor := api.Auditor.Load()
	aReq, commitAudit := audit.InitRequest[database.ChatAutomation](rw, &audit.RequestParams{
		Audit:          *auditor,
		Log:            api.Logger,
		Request:        r,
		Action:         database.AuditActionDelete,
		OrganizationID: automation.OrganizationID,
	})
	aReq.Old = automation
	defer commitAudit()

	if err := api.chatDaemon.DeleteAutomation(ctx, automation.ID); err != nil {
		api.writeChatAutomationError(ctx, rw, err)
		return
	}
	rw.WriteHeader(http.StatusNoContent)
}

// chatAutomationParam loads the {automation} path parameter as the caller.
// Automations the caller cannot read, or that belong to another
// organization than the path, are not found.
func (api *API) chatAutomationParam(rw http.ResponseWriter, r *http.Request) (database.ChatAutomation, bool) {
	ctx := r.Context()
	organization := httpmw.OrganizationParam(r)
	id, ok := httpmw.ParseUUIDParam(rw, r, "automation")
	if !ok {
		return database.ChatAutomation{}, false
	}
	automation, err := api.Database.GetChatAutomationByID(ctx, id)
	if httpapi.Is404Error(err) || (err == nil && automation.OrganizationID != organization.ID) {
		httpapi.ResourceNotFound(rw)
		return database.ChatAutomation{}, false
	}
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return database.ChatAutomation{}, false
	}
	return automation, true
}

// writeChatAutomationError maps chatd automation errors to responses.
func (api *API) writeChatAutomationError(ctx context.Context, rw http.ResponseWriter, err error) {
	var validationErr *chatd.AutomationValidationError
	switch {
	case errors.As(err, &validationErr):
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid chat automation.",
			Validations: []codersdk.ValidationError{{Field: validationErr.Field, Detail: validationErr.Detail}},
		})
	case errors.Is(err, chatd.ErrAutomationOwnerOnly):
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: "Only the owner of a chat automation can change it.",
		})
	case errors.Is(err, chatd.ErrAutomationLimitReached):
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
			Message: "Chat automation limit reached.",
			Detail:  fmt.Sprintf("A user can own at most %d chat automations across all organizations.", api.chatLimits.MaxAutomationsPerOwner),
		})
	case errors.Is(err, chatd.ErrAutomationNotFound), errors.Is(err, sql.ErrNoRows):
		httpapi.ResourceNotFound(rw)
	case dbauthz.IsNotAuthorizedError(err):
		httpapi.Forbidden(rw)
	default:
		httpapi.InternalServerError(rw, err)
	}
}

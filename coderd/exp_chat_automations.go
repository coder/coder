package coderd

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
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
// @Description Only the owner of an automation can update it, except that anyone allowed to update it can send a request that only sets enabled to false. Disabling removes the messages the automation queued that have not started. Re-enabling a schedule resumes at its next future occurrence. The kind and target mode of an automation cannot change.
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

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
// @Summary Rotate chat automation webhook secret
// @Description Only the owner of a webhook automation can rotate its secret. The previous secret stops working, and the new secret is returned only in this response.
// @ID rotate-chat-automation-secret
// @Security CoderSessionToken
// @Produce json
// @Tags Chats
// @Param organization path string true "Organization ID"
// @Param automation path string true "Automation ID" format(uuid)
// @Success 200 {object} codersdk.RotateChatAutomationSecretResponse
// @Router /api/experimental/organizations/{organization}/chat-automations/{automation}/secret/rotate [post]
// @x-apidocgen {"skip": true}
func (api *API) postChatAutomationSecretRotate(rw http.ResponseWriter, r *http.Request) {
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

	rotated, secret, err := api.chatDaemon.RotateAutomationSecret(ctx, apiKey.UserID, automation.ID)
	if err != nil {
		api.writeChatAutomationError(ctx, rw, err)
		return
	}
	aReq.New = rotated

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.RotateChatAutomationSecretResponse{
		WebhookSecret:        secret,
		WebhookSecretVersion: rotated.WebhookSecretVersion,
	})
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
// @Summary Preview chat automation schedule
// @Description Validates a schedule like chat automation create does and returns its next run times. Nothing is stored.
// @ID preview-chat-automation-schedule
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Chats
// @Param organization path string true "Organization ID"
// @Param request body codersdk.ChatAutomationSchedulePreviewRequest true "Schedule"
// @Success 200 {object} codersdk.ChatAutomationSchedulePreviewResponse
// @Router /api/experimental/organizations/{organization}/chat-automations/schedule-preview [post]
// @x-apidocgen {"skip": true}
func (api *API) postChatAutomationSchedulePreview(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req codersdk.ChatAutomationSchedulePreviewRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	runs, err := chatd.PreviewAutomationSchedule(req.ScheduleCron, req.ScheduleTimeZone, api.Clock.Now(), chatAutomationNextRunCount)
	if err != nil {
		api.writeChatAutomationError(ctx, rw, err)
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, codersdk.ChatAutomationSchedulePreviewResponse{NextRunTimes: runs})
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

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
// @Summary Deliver chat automation webhook event
// @Description Delivers an event to a webhook automation. The caller authenticates with the automation's webhook secret as a bearer token, not with a Coder session. The body can be any JSON value up to 256 KiB. The automation owner's saved prompt and the event data are sent to the target chat as the owner; a running turn is never interrupted.
// @ID deliver-chat-automation-event
// @Accept json
// @Produce json
// @Tags Chats
// @Param automation path string true "Automation ID" format(uuid)
// @Param Authorization header string true "Bearer followed by the webhook secret"
// @Param request body object true "Event data"
// @Success 202 {object} codersdk.ChatAutomationEventResponse
// @Failure 400 {object} codersdk.Response
// @Failure 401 {object} codersdk.Response
// @Failure 403 {object} codersdk.Response
// @Failure 404 {object} codersdk.Response
// @Failure 409 {object} codersdk.Response
// @Failure 413 {object} codersdk.Response
// @Failure 429 {object} codersdk.Response
// @Failure 501 {object} codersdk.Response
// @Failure 502 {object} codersdk.Response
// @Router /api/experimental/chat-automations/{automation}/events [post]
// @x-apidocgen {"skip": true}
func (api *API) postChatAutomationEvent(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	automationID, ok := httpmw.ParseUUIDParam(rw, r, "automation")
	if !ok {
		return
	}
	if api.chatDaemon == nil {
		httpapi.ResourceNotFound(rw)
		return
	}
	secret, ok := chatAutomationBearerSecret(r)
	if !ok {
		writeChatAutomationUnauthorized(ctx, rw)
		return
	}
	// The caller has no Coder identity; the secret is checked against the
	// stored hash before the automation is used in any other way.
	//nolint:gocritic // Webhook callers authenticate only with the automation secret.
	automation, err := api.Database.GetChatAutomationByID(dbauthz.AsChatd(ctx), automationID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		httpapi.InternalServerError(rw, err)
		return
	}
	hash := sha256.Sum256([]byte(secret))
	// Unknown ids, other kinds, and wrong secrets get the same response,
	// so callers cannot probe for automation ids.
	if err != nil || automation.Kind != database.ChatAutomationKindWebhook ||
		subtle.ConstantTimeCompare(hash[:], automation.WebhookSecretHash) != 1 {
		writeChatAutomationUnauthorized(ctx, rw)
		return
	}
	if !chatd.AutomationsEnabled(ctx, api.ExperimentEvaluator, automation.OwnerID) {
		httpapi.ResourceNotFound(rw)
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(rw, r.Body, maxChatRequestBodyBytes))
	if err != nil {
		if mbe, ok := errors.AsType[*http.MaxBytesError](err); ok {
			httpapi.RecordRequestBodyLimit(ctx, mbe.Limit)
			httpapi.Write(ctx, rw, http.StatusRequestEntityTooLarge, codersdk.Response{
				Message: "Request body too large.",
				Detail:  fmt.Sprintf("Maximum request body size is %d bytes.", mbe.Limit),
			})
			return
		}
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to read request body.",
			Detail:  err.Error(),
		})
		return
	}
	if !json.Valid(body) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Request body must be valid JSON.",
		})
		return
	}

	result, err := api.chatDaemon.PublishAutomationWebhook(ctx, chatd.PublishAutomationWebhookParams{
		AutomationID:  automation.ID,
		SecretVersion: automation.WebhookSecretVersion,
		Body:          body,
	})
	if err != nil {
		writeChatAutomationEventError(ctx, rw, err)
		return
	}
	httpapi.Write(ctx, rw, http.StatusAccepted, codersdk.ChatAutomationEventResponse{
		InputID: result.InputID,
		ChatID:  result.ChatID,
	})
}

// chatAutomationBearerSecret returns the bearer token of the request.
func chatAutomationBearerSecret(r *http.Request) (string, bool) {
	scheme, secret, ok := strings.Cut(r.Header.Get("Authorization"), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}
	secret = strings.TrimSpace(secret)
	return secret, secret != ""
}

func writeChatAutomationUnauthorized(ctx context.Context, rw http.ResponseWriter) {
	httpapi.Write(ctx, rw, http.StatusUnauthorized, codersdk.Response{
		Message: "Invalid chat automation webhook secret.",
		Detail:  "Send the webhook secret of the automation as a bearer token in the Authorization header.",
	})
}

// writeChatAutomationEventError maps chatd publish errors to responses.
func writeChatAutomationEventError(ctx context.Context, rw http.ResponseWriter, err error) {
	if writeChatHookErr(ctx, rw, err, "Chat automation event denied by lifecycle hook.") {
		return
	}
	switch {
	case errors.Is(err, chatd.ErrAutomationNotFound), errors.Is(err, chatd.ErrAutomationSecretChanged):
		writeChatAutomationUnauthorized(ctx, rw)
	case errors.Is(err, chatd.ErrAutomationDisabled):
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{Message: "Chat automation is disabled."})
	case errors.Is(err, chatd.ErrAutomationOwnerInactive):
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{Message: "The owner of the chat automation is not active."})
	case errors.Is(err, chatd.ErrAutomationForbidden), dbauthz.IsNotAuthorizedError(err):
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{Message: "The owner of the chat automation cannot send messages to the target chat."})
	case errors.Is(err, chatd.ErrAutomationWebhookConsumed):
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{Message: "This single-use webhook was already used."})
	case errors.Is(err, chatd.ErrAutomationTargetUnavailable):
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{Message: "The target chat of the chat automation is unavailable."})
	case errors.Is(err, chatd.ErrAutomationChatBusy):
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
			Message: "The target chat is busy.",
			Detail:  "The automation skips events while the chat is busy.",
		})
	case errors.Is(err, chatd.ErrAutomationQueueShareFull):
		httpapi.Write(ctx, rw, http.StatusTooManyRequests, codersdk.Response{
			Message: "Too many automation messages are queued in the target chat.",
			Detail:  err.Error(),
		})
	case errors.Is(err, chatstate.ErrMessageQueueFull):
		httpapi.Write(ctx, rw, http.StatusTooManyRequests, codersdk.Response{Message: "Message queue is full."})
	case errors.Is(err, chatd.ErrAutomationTargetNotSupported):
		// new_chat targets are delivered by a follow-up change.
		httpapi.Write(ctx, rw, http.StatusNotImplemented, codersdk.Response{
			Message: "Chat automations that start a new chat cannot receive events yet.",
		})
	default:
		httpapi.InternalServerError(rw, err)
	}
}

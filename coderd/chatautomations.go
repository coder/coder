package coderd

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/coder/coder/v2/coderd/chatd/autochat"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
)

// convertChatAutomation converts a database ChatAutomation to the
// SDK representation. When maskSecret is true the webhook secret is
// omitted from the response.
func convertChatAutomation(
	automation database.ChatAutomation,
	accessURL string,
	maskSecret bool,
) codersdk.ChatAutomation {
	result := codersdk.ChatAutomation{
		ID:                automation.ID,
		OwnerID:           automation.OwnerID,
		Name:              automation.Name,
		Description:       automation.Description,
		Icon:              automation.Icon,
		TriggerType:       codersdk.ChatAutomationTriggerType(automation.TriggerType),
		ModelConfigID:     automation.ModelConfigID,
		SystemPrompt:      automation.SystemPrompt,
		PromptTemplate:    automation.PromptTemplate,
		Enabled:           automation.Enabled,
		MaxConcurrentRuns: automation.MaxConcurrentRuns,
		CreatedAt:         automation.CreatedAt,
		UpdatedAt:         automation.UpdatedAt,
	}
	if automation.CronSchedule.Valid {
		result.CronSchedule = &automation.CronSchedule.String
	}
	if automation.TriggerType == string(codersdk.ChatAutomationTriggerWebhook) {
		result.WebhookURL = fmt.Sprintf(
			"%s/api/v2/chats/automations/%s/webhook",
			accessURL, automation.ID,
		)
		if !maskSecret && automation.WebhookSecret.Valid {
			result.WebhookSecret = &automation.WebhookSecret.String
		}
	}
	return result
}

// convertChatAutomationRun converts a database ChatAutomationRun
// to the SDK representation.
func convertChatAutomationRun(
	run database.ChatAutomationRun,
) codersdk.ChatAutomationRun {
	result := codersdk.ChatAutomationRun{
		ID:             run.ID,
		AutomationID:   run.AutomationID,
		TriggerPayload: run.TriggerPayload,
		RenderedPrompt: run.RenderedPrompt,
		Status:         run.Status,
		CreatedAt:      run.CreatedAt,
	}
	if run.ChatID.Valid {
		result.ChatID = &run.ChatID.UUID
	}
	if run.Error.Valid {
		result.Error = &run.Error.String
	}
	if run.StartedAt.Valid {
		result.StartedAt = &run.StartedAt.Time
	}
	if run.CompletedAt.Valid {
		result.CompletedAt = &run.CompletedAt.Time
	}
	return result
}

// @Summary List chat automations
// @ID list-chat-automations
// @Security CoderSessionToken
// @Produce json
// @Tags Chat
// @Success 200 {array} codersdk.ChatAutomation
// @Router /chats/automations [get]
// @x-apidocgen {"skip": true}
func (api *API) listChatAutomations(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)

	automations, err := api.Database.GetChatAutomationsByOwnerID(ctx, apiKey.UserID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to list chat automations.",
			Detail:  err.Error(),
		})
		return
	}

	result := make([]codersdk.ChatAutomation, len(automations))
	for i, a := range automations {
		result[i] = convertChatAutomation(a, api.AccessURL.String(), true)
	}

	httpapi.Write(ctx, rw, http.StatusOK, result)
}

// @Summary Create a chat automation
// @ID create-chat-automation
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Chat
// @Param request body codersdk.CreateChatAutomationRequest true "Create chat automation request"
// @Success 201 {object} codersdk.ChatAutomation
// @Router /chats/automations [post]
// @x-apidocgen {"skip": true}
func (api *API) createChatAutomation(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)

	var req codersdk.CreateChatAutomationRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	if req.Name == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Name is required.",
		})
		return
	}

	triggerType := string(req.TriggerType)
	if triggerType != string(codersdk.ChatAutomationTriggerWebhook) &&
		triggerType != string(codersdk.ChatAutomationTriggerCron) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid trigger type.",
			Detail:  "Must be \"webhook\" or \"cron\".",
		})
		return
	}

	var webhookSecret sql.NullString
	if triggerType == string(codersdk.ChatAutomationTriggerWebhook) {
		secret, err := autochat.GenerateWebhookSecret()
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to generate webhook secret.",
				Detail:  err.Error(),
			})
			return
		}
		webhookSecret = sql.NullString{String: secret, Valid: true}
	}

	var cronSchedule sql.NullString
	if req.CronSchedule != nil {
		cronSchedule = sql.NullString{String: *req.CronSchedule, Valid: true}
	}

	maxConcurrent := req.MaxConcurrentRuns
	if maxConcurrent <= 0 {
		maxConcurrent = 1
	}

	automation, err := api.Database.InsertChatAutomation(ctx, database.InsertChatAutomationParams{
		OwnerID:           apiKey.UserID,
		Name:              req.Name,
		Description:       req.Description,
		Icon:              req.Icon,
		TriggerType:       triggerType,
		WebhookSecret:     webhookSecret,
		CronSchedule:      cronSchedule,
		ModelConfigID:     req.ModelConfigID,
		SystemPrompt:      req.SystemPrompt,
		PromptTemplate:    req.PromptTemplate,
		Enabled:           true,
		MaxConcurrentRuns: maxConcurrent,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to create chat automation.",
			Detail:  err.Error(),
		})
		return
	}

	// Return the secret unmasked on creation — this is the only
	// time the caller can see it.
	httpapi.Write(ctx, rw, http.StatusCreated,
		convertChatAutomation(automation, api.AccessURL.String(), false))
}

// @Summary Get a chat automation
// @ID get-chat-automation
// @Security CoderSessionToken
// @Produce json
// @Tags Chat
// @Param chatAutomation path string true "Chat automation ID" format(uuid)
// @Success 200 {object} codersdk.ChatAutomation
// @Router /chats/automations/{chatAutomation} [get]
// @x-apidocgen {"skip": true}
func (api *API) getChatAutomation(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	automation := httpmw.ChatAutomationParam(r)
	httpapi.Write(ctx, rw, http.StatusOK,
		convertChatAutomation(automation, api.AccessURL.String(), true))
}

// @Summary Update a chat automation
// @ID update-chat-automation
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Chat
// @Param chatAutomation path string true "Chat automation ID" format(uuid)
// @Param request body codersdk.UpdateChatAutomationRequest true "Update chat automation request"
// @Success 200 {object} codersdk.ChatAutomation
// @Router /chats/automations/{chatAutomation} [patch]
// @x-apidocgen {"skip": true}
func (api *API) updateChatAutomation(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	automation := httpmw.ChatAutomationParam(r)

	var req codersdk.UpdateChatAutomationRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	var cronSchedule sql.NullString
	if req.CronSchedule != nil {
		cronSchedule = sql.NullString{String: *req.CronSchedule, Valid: true}
	}

	maxConcurrent := req.MaxConcurrentRuns
	if maxConcurrent <= 0 {
		maxConcurrent = 1
	}

	updated, err := api.Database.UpdateChatAutomation(ctx, database.UpdateChatAutomationParams{
		ID:                automation.ID,
		Name:              req.Name,
		Description:       req.Description,
		Icon:              req.Icon,
		CronSchedule:      cronSchedule,
		ModelConfigID:     req.ModelConfigID,
		SystemPrompt:      req.SystemPrompt,
		PromptTemplate:    req.PromptTemplate,
		Enabled:           req.Enabled,
		MaxConcurrentRuns: maxConcurrent,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to update chat automation.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK,
		convertChatAutomation(updated, api.AccessURL.String(), true))
}

// @Summary Delete a chat automation
// @ID delete-chat-automation
// @Security CoderSessionToken
// @Tags Chat
// @Param chatAutomation path string true "Chat automation ID" format(uuid)
// @Success 204
// @Router /chats/automations/{chatAutomation} [delete]
// @x-apidocgen {"skip": true}
func (api *API) deleteChatAutomation(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	automation := httpmw.ChatAutomationParam(r)

	err := api.Database.DeleteChatAutomation(ctx, automation.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to delete chat automation.",
			Detail:  err.Error(),
		})
		return
	}

	rw.WriteHeader(http.StatusNoContent)
}

// @Summary Trigger a chat automation
// @ID trigger-chat-automation
// @Security CoderSessionToken
// @Produce json
// @Tags Chat
// @Param chatAutomation path string true "Chat automation ID" format(uuid)
// @Success 201 {object} codersdk.ChatAutomationRun
// @Router /chats/automations/{chatAutomation}/trigger [post]
// @x-apidocgen {"skip": true}
func (api *API) triggerChatAutomation(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	automation := httpmw.ChatAutomationParam(r)

	if !automation.Enabled {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Automation is disabled.",
		})
		return
	}

	if api.autochatExecutor == nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Automation executor is not configured.",
		})
		return
	}

	templateData := map[string]any{
		"Source": "manual",
	}
	run, err := api.autochatExecutor.Fire(ctx, automation, json.RawMessage(`{"source":"manual"}`), templateData)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to trigger automation.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusCreated, convertChatAutomationRun(run))
}

// @Summary Rotate a chat automation webhook secret
// @ID rotate-chat-automation-secret
// @Security CoderSessionToken
// @Produce json
// @Tags Chat
// @Param chatAutomation path string true "Chat automation ID" format(uuid)
// @Success 200 {object} codersdk.ChatAutomation
// @Router /chats/automations/{chatAutomation}/rotate-secret [post]
// @x-apidocgen {"skip": true}
func (api *API) rotateChatAutomationSecret(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	automation := httpmw.ChatAutomationParam(r)

	if automation.TriggerType != string(codersdk.ChatAutomationTriggerWebhook) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Secret rotation is only supported for webhook automations.",
		})
		return
	}

	secret, err := autochat.GenerateWebhookSecret()
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to generate webhook secret.",
			Detail:  err.Error(),
		})
		return
	}

	updated, err := api.Database.UpdateChatAutomationWebhookSecret(ctx, database.UpdateChatAutomationWebhookSecretParams{
		ID:            automation.ID,
		WebhookSecret: sql.NullString{String: secret, Valid: true},
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to rotate webhook secret.",
			Detail:  err.Error(),
		})
		return
	}

	// Return the new secret unmasked so the caller can store it.
	httpapi.Write(ctx, rw, http.StatusOK,
		convertChatAutomation(updated, api.AccessURL.String(), false))
}

// @Summary List chat automation runs
// @ID list-chat-automation-runs
// @Security CoderSessionToken
// @Produce json
// @Tags Chat
// @Param chatAutomation path string true "Chat automation ID" format(uuid)
// @Success 200 {array} codersdk.ChatAutomationRun
// @Router /chats/automations/{chatAutomation}/runs [get]
// @x-apidocgen {"skip": true}
func (api *API) listChatAutomationRuns(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	automation := httpmw.ChatAutomationParam(r)

	runs, err := api.Database.GetChatAutomationRunsByAutomationID(ctx, database.GetChatAutomationRunsByAutomationIDParams{
		AutomationID: automation.ID,
		LimitOpt:     50,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to list automation runs.",
			Detail:  err.Error(),
		})
		return
	}

	result := make([]codersdk.ChatAutomationRun, len(runs))
	for i, run := range runs {
		result[i] = convertChatAutomationRun(run)
	}

	httpapi.Write(ctx, rw, http.StatusOK, result)
}

// @Summary Webhook ingress for a chat automation
// @ID chat-automation-webhook
// @Accept json
// @Produce json
// @Tags Chat
// @Param chatAutomation path string true "Chat automation ID" format(uuid)
// @Success 201 {object} codersdk.ChatAutomationRun
// @Router /chats/automations/{chatAutomation}/webhook [post]
// @x-apidocgen {"skip": true}
func (api *API) chatAutomationWebhook(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Look up the automation directly — this endpoint is
	// unauthenticated so we cannot rely on the middleware that
	// requires a session.
	automationID, parsed := httpmw.ParseUUIDParam(rw, r, "chatAutomation")
	if !parsed {
		return
	}

	automation, err := api.Database.GetChatAutomationByID(ctx, automationID)
	if httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching chat automation.",
			Detail:  err.Error(),
		})
		return
	}

	if automation.TriggerType != string(codersdk.ChatAutomationTriggerWebhook) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Automation is not webhook-triggered.",
		})
		return
	}

	if !automation.Enabled {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Automation is disabled.",
		})
		return
	}

	if !automation.WebhookSecret.Valid {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Automation has no webhook secret configured.",
		})
		return
	}

	// Read the raw body BEFORE any JSON parsing. Parsing changes
	// whitespace and would break the HMAC signature verification.
	// Cap at 1 MB to prevent abuse.
	rawBody, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to read request body.",
			Detail:  err.Error(),
		})
		return
	}

	if err := autochat.VerifyWebhookSignature(rawBody, r.Header, automation.WebhookSecret.String); err != nil {
		httpapi.Write(ctx, rw, http.StatusUnauthorized, codersdk.Response{
			Message: "Webhook signature verification failed.",
			Detail:  err.Error(),
		})
		return
	}

	// Validate that the body is well-formed JSON.
	var payload json.RawMessage
	if len(rawBody) == 0 {
		payload = json.RawMessage(`{}`)
	} else {
		if !json.Valid(rawBody) {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Request body is not valid JSON.",
			})
			return
		}
		payload = rawBody
	}

	if api.autochatExecutor == nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Automation executor is not configured.",
		})
		return
	}

	// Parse the body into a map for template rendering.
	var bodyMap map[string]any
	if err := json.Unmarshal(payload, &bodyMap); err != nil {
		bodyMap = map[string]any{"raw": string(payload)}
	}

	// Flatten headers into a simple map for template access.
	headerMap := make(map[string]string, len(r.Header))
	for k, v := range r.Header {
		headerMap[k] = strings.Join(v, ", ")
	}

	templateData := map[string]any{
		"Body":    bodyMap,
		"Headers": headerMap,
	}

	run, err := api.autochatExecutor.Fire(ctx, automation, payload, templateData)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to fire automation.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusAccepted, convertChatAutomationRun(run))
}


package coderd

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/automations"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
)

func generateWebhookSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", xerrors.Errorf("generate webhook secret: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func convertAutomation(a database.Automation, accessURL string) codersdk.Automation {
	result := codersdk.Automation{
		ID:                    a.ID,
		OwnerID:               a.OwnerID,
		OrganizationID:        a.OrganizationID,
		Name:                  a.Name,
		Description:           a.Description,
		WebhookURL:            accessURL + "/api/v2/automations/" + a.ID.String() + "/webhook",
		Filter:                a.Filter.RawMessage,
		SessionLabels:         a.SessionLabels.RawMessage,
		SystemPrompt:          a.SystemPrompt,
		MCPServerIDs:          a.MCPServerIDs,
		AllowedTools:          a.AllowedTools,
		Status:                codersdk.AutomationStatus(a.Status),
		MaxChatCreatesPerHour: a.MaxChatCreatesPerHour,
		MaxMessagesPerHour:    a.MaxMessagesPerHour,
		CreatedAt:             a.CreatedAt,
		UpdatedAt:             a.UpdatedAt,
	}
	if a.ModelConfigID.Valid {
		result.ModelConfigID = &a.ModelConfigID.UUID
	}
	if a.WorkspaceID.Valid {
		result.WorkspaceID = &a.WorkspaceID.UUID
	}
	if a.MCPServerIDs == nil {
		result.MCPServerIDs = []uuid.UUID{}
	}
	if a.AllowedTools == nil {
		result.AllowedTools = []string{}
	}
	return result
}

func convertWebhookEvent(e database.AutomationWebhookEvent) codersdk.AutomationWebhookEvent {
	result := codersdk.AutomationWebhookEvent{
		ID:             e.ID,
		AutomationID:   e.AutomationID,
		ReceivedAt:     e.ReceivedAt,
		Payload:        e.Payload,
		FilterMatched:  e.FilterMatched,
		ResolvedLabels: e.ResolvedLabels.RawMessage,
		Status:         codersdk.AutomationWebhookEventStatus(e.Status),
	}
	if e.MatchedChatID.Valid {
		result.MatchedChatID = &e.MatchedChatID.UUID
	}
	if e.CreatedChatID.Valid {
		result.CreatedChatID = &e.CreatedChatID.UUID
	}
	if e.Error.Valid {
		result.Error = &e.Error.String
	}
	return result
}

// postAutomation creates a new automation.
func (api *API) postAutomation(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)

	var req codersdk.CreateAutomationRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	if req.Name == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Name is required.",
		})
		return
	}

	secret, err := generateWebhookSecret()
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to generate webhook secret.",
			Detail:  err.Error(),
		})
		return
	}

	// Use the first organization the user belongs to.
	orgs, err := api.Database.GetOrganizationsByUserID(ctx, database.GetOrganizationsByUserIDParams{
		UserID: apiKey.UserID,
	})
	if err != nil || len(orgs) == 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Could not determine organization.",
		})
		return
	}
	orgID := orgs[0].ID

	maxCreates := int32(10)
	if req.MaxChatCreatesPerHour != nil {
		maxCreates = *req.MaxChatCreatesPerHour
	}
	maxMessages := int32(60)
	if req.MaxMessagesPerHour != nil {
		maxMessages = *req.MaxMessagesPerHour
	}

	arg := database.InsertAutomationParams{
		OwnerID:               apiKey.UserID,
		OrganizationID:        orgID,
		Name:                  req.Name,
		Description:           req.Description,
		WebhookSecret:         secret,
		SystemPrompt:          req.SystemPrompt,
		MCPServerIDs:          req.MCPServerIDs,
		AllowedTools:          req.AllowedTools,
		Status:                "disabled",
		MaxChatCreatesPerHour: maxCreates,
		MaxMessagesPerHour:    maxMessages,
	}
	if len(req.Filter) > 0 {
		arg.Filter = pqtype.NullRawMessage{RawMessage: req.Filter, Valid: true}
	}
	if len(req.SessionLabels) > 0 {
		arg.SessionLabels = pqtype.NullRawMessage{RawMessage: req.SessionLabels, Valid: true}
	}
	if req.ModelConfigID != nil {
		arg.ModelConfigID = uuid.NullUUID{UUID: *req.ModelConfigID, Valid: true}
	}
	if req.WorkspaceID != nil {
		arg.WorkspaceID = uuid.NullUUID{UUID: *req.WorkspaceID, Valid: true}
	}

	automation, err := api.Database.InsertAutomation(ctx, arg)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to create automation.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusCreated, convertAutomation(automation, api.AccessURL.String()))
}

// listAutomations returns all automations visible to the user.
func (api *API) listAutomations(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)

	dbAutomations, err := api.Database.GetAutomations(ctx, database.GetAutomationsParams{
		OwnerID: apiKey.UserID,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to list automations.",
			Detail:  err.Error(),
		})
		return
	}

	result := make([]codersdk.Automation, 0, len(dbAutomations))
	for _, a := range dbAutomations {
		result = append(result, convertAutomation(a, api.AccessURL.String()))
	}
	httpapi.Write(ctx, rw, http.StatusOK, result)
}

// getAutomation returns a single automation.
func (api *API) getAutomation(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	automation := httpmw.AutomationParam(r)
	httpapi.Write(ctx, rw, http.StatusOK, convertAutomation(automation, api.AccessURL.String()))
}

// patchAutomation updates an automation's configuration.
func (api *API) patchAutomation(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	automation := httpmw.AutomationParam(r)

	var req codersdk.UpdateAutomationRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	// Merge: start from current values, apply updates.
	arg := database.UpdateAutomationParams{
		ID:                    automation.ID,
		Name:                  automation.Name,
		Description:           automation.Description,
		Filter:                automation.Filter,
		SessionLabels:         automation.SessionLabels,
		SystemPrompt:          automation.SystemPrompt,
		ModelConfigID:         automation.ModelConfigID,
		WorkspaceID:           automation.WorkspaceID,
		MCPServerIDs:          automation.MCPServerIDs,
		AllowedTools:          automation.AllowedTools,
		Status:                automation.Status,
		MaxChatCreatesPerHour: automation.MaxChatCreatesPerHour,
		MaxMessagesPerHour:    automation.MaxMessagesPerHour,
	}
	if req.Name != nil {
		arg.Name = *req.Name
	}
	if req.Description != nil {
		arg.Description = *req.Description
	}
	if req.Filter != nil {
		arg.Filter = pqtype.NullRawMessage{RawMessage: req.Filter, Valid: true}
	}
	if req.SessionLabels != nil {
		arg.SessionLabels = pqtype.NullRawMessage{RawMessage: req.SessionLabels, Valid: true}
	}
	if req.SystemPrompt != nil {
		arg.SystemPrompt = *req.SystemPrompt
	}
	if req.ModelConfigID != nil {
		arg.ModelConfigID = uuid.NullUUID{UUID: *req.ModelConfigID, Valid: true}
	}
	if req.WorkspaceID != nil {
		arg.WorkspaceID = uuid.NullUUID{UUID: *req.WorkspaceID, Valid: true}
	}
	if req.MCPServerIDs != nil {
		arg.MCPServerIDs = *req.MCPServerIDs
	}
	if req.AllowedTools != nil {
		arg.AllowedTools = *req.AllowedTools
	}
	if req.Status != nil {
		arg.Status = string(*req.Status)
	}
	if req.MaxChatCreatesPerHour != nil {
		arg.MaxChatCreatesPerHour = *req.MaxChatCreatesPerHour
	}
	if req.MaxMessagesPerHour != nil {
		arg.MaxMessagesPerHour = *req.MaxMessagesPerHour
	}

	updated, err := api.Database.UpdateAutomation(ctx, arg)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to update automation.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, convertAutomation(updated, api.AccessURL.String()))
}

// deleteAutomation deletes an automation.
func (api *API) deleteAutomation(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	automation := httpmw.AutomationParam(r)

	err := api.Database.DeleteAutomationByID(ctx, automation.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to delete automation.",
			Detail:  err.Error(),
		})
		return
	}

	rw.WriteHeader(http.StatusNoContent)
}

// regenerateAutomationSecret generates a new webhook secret for an
// automation.
func (api *API) regenerateAutomationSecret(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	automation := httpmw.AutomationParam(r)

	secret, err := generateWebhookSecret()
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to generate webhook secret.",
			Detail:  err.Error(),
		})
		return
	}

	updated, err := api.Database.UpdateAutomationWebhookSecret(ctx, database.UpdateAutomationWebhookSecretParams{
		ID:            automation.ID,
		WebhookSecret: secret,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to update webhook secret.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, convertAutomation(updated, api.AccessURL.String()))
}

// listAutomationEvents returns recent webhook events for an
// automation.
func (api *API) listAutomationEvents(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	automation := httpmw.AutomationParam(r)

	events, err := api.Database.GetAutomationWebhookEvents(ctx, database.GetAutomationWebhookEventsParams{
		AutomationID: automation.ID,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to list webhook events.",
			Detail:  err.Error(),
		})
		return
	}

	result := make([]codersdk.AutomationWebhookEvent, 0, len(events))
	for _, e := range events {
		result = append(result, convertWebhookEvent(e))
	}
	httpapi.Write(ctx, rw, http.StatusOK, result)
}

// testAutomation performs a dry-run of the filter and session
// resolution logic against a sample payload.
func (api *API) testAutomation(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	automation := httpmw.AutomationParam(r)

	var payload json.RawMessage
	if !httpapi.Read(ctx, rw, r, &payload) {
		return
	}

	payloadStr := string(payload)
	matched := automations.MatchFilter(payloadStr, automation.Filter.RawMessage)

	result := codersdk.AutomationTestResult{
		FilterMatched: matched,
	}

	// If filter matched and session_labels are configured, resolve
	// them.
	if matched && automation.SessionLabels.Valid {
		var labelPaths map[string]string
		if err := json.Unmarshal(automation.SessionLabels.RawMessage, &labelPaths); err == nil {
			resolvedLabels := automations.ResolveLabels(payloadStr, labelPaths)
			if labelsJSON, err := json.Marshal(resolvedLabels); err == nil {
				result.ResolvedLabels = labelsJSON
			}

			// Look for existing chat with these labels.
			if len(resolvedLabels) > 0 {
				labelsJSON, _ := json.Marshal(resolvedLabels)
				chats, err := api.Database.GetChats(ctx, database.GetChatsParams{
					OwnerID: automation.OwnerID,
					LabelFilter: pqtype.NullRawMessage{
						RawMessage: labelsJSON,
						Valid:      true,
					},
					LimitOpt: 1,
				})
				if err == nil && len(chats) > 0 {
					result.ExistingChatID = &chats[0].ID
				}
			}
		}
	}

	result.WouldCreateChat = matched && result.ExistingChatID == nil

	httpapi.Write(ctx, rw, http.StatusOK, result)
}

// postAutomationWebhook is the unauthenticated stable v2 endpoint
// that receives webhook deliveries from external systems.
func (api *API) postAutomationWebhook(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	automationID, parsed := httpmw.ParseUUIDParam(rw, r, "automation_id")
	if !parsed {
		return
	}

	// Always return 200 to prevent source-system retries.
	//nolint:gocritic // Webhook handler must bypass auth to look up automation.
	automation, err := api.Database.GetAutomationByID(dbauthz.AsSystemRestricted(ctx), automationID)
	if err != nil {
		// Still return 200 even if automation not found.
		rw.WriteHeader(http.StatusOK)
		return
	}

	if automation.Status == "disabled" {
		rw.WriteHeader(http.StatusOK)
		return
	}

	// Read body with size limit.
	r.Body = http.MaxBytesReader(rw, r.Body, 256*1024) // 256 KB
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		rw.WriteHeader(http.StatusOK)
		return
	}

	// Verify HMAC signature.
	sig := r.Header.Get("X-Hub-Signature-256")
	if !automations.VerifySignature(payload, automation.WebhookSecret, sig) {
		// Log event as error but still return 200.
		//nolint:gocritic // System context for event logging.
		_, _ = api.Database.InsertAutomationWebhookEvent(dbauthz.AsSystemRestricted(ctx), database.InsertAutomationWebhookEventParams{
			AutomationID:  automation.ID,
			Payload:       truncatePayload(payload),
			FilterMatched: false,
			Status:        "error",
			Error:         sql.NullString{String: "signature verification failed", Valid: true},
		})
		rw.WriteHeader(http.StatusOK)
		return
	}

	payloadStr := string(payload)
	matched := automations.MatchFilter(payloadStr, automation.Filter.RawMessage)

	if !matched {
		//nolint:gocritic // System context for event logging.
		_, _ = api.Database.InsertAutomationWebhookEvent(dbauthz.AsSystemRestricted(ctx), database.InsertAutomationWebhookEventParams{
			AutomationID:  automation.ID,
			Payload:       truncatePayload(payload),
			FilterMatched: false,
			Status:        "filtered",
		})
		rw.WriteHeader(http.StatusOK)
		return
	}

	// Resolve session labels.
	var resolvedLabels map[string]string
	var resolvedLabelsJSON pqtype.NullRawMessage
	if automation.SessionLabels.Valid {
		var labelPaths map[string]string
		if err := json.Unmarshal(automation.SessionLabels.RawMessage, &labelPaths); err == nil {
			resolvedLabels = automations.ResolveLabels(payloadStr, labelPaths)
			if j, err := json.Marshal(resolvedLabels); err == nil {
				resolvedLabelsJSON = pqtype.NullRawMessage{RawMessage: j, Valid: true}
			}
		}
	}

	// Preview mode: log but don't act.
	if automation.Status == "preview" {
		eventArg := database.InsertAutomationWebhookEventParams{
			AutomationID:   automation.ID,
			Payload:        truncatePayload(payload),
			FilterMatched:  true,
			ResolvedLabels: resolvedLabelsJSON,
			Status:         "preview",
		}

		// Still resolve the chat for the preview log.
		if len(resolvedLabels) > 0 {
			labelsJSON, _ := json.Marshal(resolvedLabels)
			//nolint:gocritic // System context for chat lookup.
			chats, chatErr := api.Database.GetChats(dbauthz.AsSystemRestricted(ctx), database.GetChatsParams{
				OwnerID: automation.OwnerID,
				LabelFilter: pqtype.NullRawMessage{
					RawMessage: labelsJSON,
					Valid:      true,
				},
				LimitOpt: 1,
			})
			if chatErr == nil && len(chats) > 0 {
				eventArg.MatchedChatID = uuid.NullUUID{UUID: chats[0].ID, Valid: true}
			}
		}

		//nolint:gocritic // System context for event logging.
		_, _ = api.Database.InsertAutomationWebhookEvent(dbauthz.AsSystemRestricted(ctx), eventArg)
		rw.WriteHeader(http.StatusOK)
		return
	}

	// Active mode: TODO — implement rate limiting and chat
	// creation/continuation. For now, log the event.
	//nolint:gocritic // System context for event logging.
	_, _ = api.Database.InsertAutomationWebhookEvent(dbauthz.AsSystemRestricted(ctx), database.InsertAutomationWebhookEventParams{
		AutomationID:   automation.ID,
		Payload:        truncatePayload(payload),
		FilterMatched:  true,
		ResolvedLabels: resolvedLabelsJSON,
		Status:         "created",
	})
	rw.WriteHeader(http.StatusOK)
}

// truncatePayload limits the stored payload to 64 KB.
func truncatePayload(payload []byte) json.RawMessage {
	const maxPayloadSize = 64 * 1024
	if len(payload) > maxPayloadSize {
		payload = payload[:maxPayloadSize]
	}
	return json.RawMessage(payload)
}

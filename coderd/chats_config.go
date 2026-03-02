package coderd

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary Get chat config settings
// @ID get-chat-config-settings
// @Security CoderSessionToken
// @Produce json
// @Tags Chat
// @Success 200 {object} codersdk.ChatConfigSettings
// @Router /chats/config [get]
func (api *API) chatConfigSettings(rw http.ResponseWriter, r *http.Request) {
	settingsJSON, err := api.Database.GetChatConfigSettings(r.Context())
	if err != nil {
		httpapi.Write(r.Context(), rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to fetch chat config settings.",
			Detail:  err.Error(),
		})
		return
	}

	var settings codersdk.ChatConfigSettings
	if len(settingsJSON) > 0 {
		if err := json.Unmarshal([]byte(settingsJSON), &settings); err != nil {
			httpapi.Write(r.Context(), rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to unmarshal chat config settings.",
				Detail:  err.Error(),
			})
			return
		}
	}

	httpapi.Write(r.Context(), rw, http.StatusOK, settings)
}

// @Summary Update chat config settings
// @ID update-chat-config-settings
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Chat
// @Param request body codersdk.ChatConfigSettings true "Chat config settings request"
// @Success 200 {object} codersdk.ChatConfigSettings
// @Success 304
// @Router /chats/config [put]
func (api *API) putChatConfigSettings(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var settings codersdk.ChatConfigSettings
	if !httpapi.Read(ctx, rw, r, &settings) {
		return
	}

	settingsJSON, err := json.Marshal(&settings)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to marshal chat config settings.",
			Detail:  err.Error(),
		})
		return
	}

	currentSettingsJSON, err := api.Database.GetChatConfigSettings(ctx)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to fetch current chat config settings.",
			Detail:  err.Error(),
		})
		return
	}

	if bytes.Equal(settingsJSON, []byte(currentSettingsJSON)) {
		// See: https://www.rfc-editor.org/rfc/rfc7232#section-4.1
		httpapi.Write(ctx, rw, http.StatusNotModified, nil)
		return
	}

	auditor := api.Auditor.Load()
	aReq, commitAudit := audit.InitRequest[database.ChatConfigSettings](rw, &audit.RequestParams{
		Audit:   *auditor,
		Log:     api.Logger,
		Request: r,
		Action:  database.AuditActionWrite,
	})
	defer commitAudit()

	aReq.New = database.ChatConfigSettings{
		ID:           uuid.New(),
		SystemPrompt: settings.SystemPrompt,
	}

	err = api.Database.UpsertChatConfigSettings(ctx, string(settingsJSON))
	if err != nil {
		if rbac.IsUnauthorizedError(err) {
			httpapi.Forbidden(rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to update chat config settings.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, settings)
}

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
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary Get notifications settings
// @ID get-notifications-settings
// @Security CoderSessionToken
// @Produce json
// @Tags General
// @Success 200 {object} codersdk.NotificationsSettings
// @Router /notifications/settings [get]
func (api *API) notificationsSettings(rw http.ResponseWriter, r *http.Request) {
	settingsJSON, err := api.Database.GetNotificationsSettings(r.Context())
	if err != nil {
		httpapi.Write(r.Context(), rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to fetch current notifications settings.",
			Detail:  err.Error(),
		})
		return
	}

	var settings codersdk.NotificationsSettings
	if len(settingsJSON) > 0 {
		err = json.Unmarshal([]byte(settingsJSON), &settings)
		if err != nil {
			httpapi.Write(r.Context(), rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to unmarshal notifications settings.",
				Detail:  err.Error(),
			})
			return
		}
	}
	httpapi.Write(r.Context(), rw, http.StatusOK, settings)
}

// @Summary Update notifications settings
// @ID update-notifications-settings
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags General
// @Param request body codersdk.NotificationsSettings true "Notifications settings request"
// @Success 200 {object} codersdk.NotificationsSettings
// @Router /notifications/settings [put]
func (api *API) putNotificationsSettings(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if !api.Authorize(r, policy.ActionUpdate, rbac.ResourceDeploymentConfig) {
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: "Insufficient permissions to update notifications settings.",
		})
		return
	}

	var settings codersdk.NotificationsSettings
	if !httpapi.Read(ctx, rw, r, &settings) {
		return
	}

	settingsJSON, err := json.Marshal(&settings)
	if err != nil {
		httpapi.Write(r.Context(), rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to marshal notifications settings.",
			Detail:  err.Error(),
		})
		return
	}

	currentSettingsJSON, err := api.Database.GetNotificationsSettings(r.Context())
	if err != nil {
		httpapi.Write(r.Context(), rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to fetch current notifications settings.",
			Detail:  err.Error(),
		})
		return
	}

	if bytes.Equal(settingsJSON, []byte(currentSettingsJSON)) {
		// See: https://www.rfc-editor.org/rfc/rfc7232#section-4.1
		httpapi.Write(r.Context(), rw, http.StatusNotModified, nil)
		return
	}

	auditor := api.Auditor.Load()
	aReq, commitAudit := audit.InitRequest[database.NotificationsSettings](rw, &audit.RequestParams{
		Audit:   *auditor,
		Log:     api.Logger,
		Request: r,
		Action:  database.AuditActionWrite,
	})
	defer commitAudit()

	aReq.New = database.NotificationsSettings{
		ID:             uuid.New(),
		NotifierPaused: settings.NotifierPaused,
	}

	err = api.Database.UpsertNotificationsSettings(ctx, string(settingsJSON))
	if err != nil {
		httpapi.Write(r.Context(), rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to update notifications settings.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(r.Context(), rw, http.StatusOK, settings)
}

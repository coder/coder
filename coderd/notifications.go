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

// @Summary Get notifications settings
// @ID get-notifications-settings
// @Security CoderSessionToken
// @Produce json
// @Tags Notifications
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
// @Tags Notifications
// @Param request body codersdk.NotificationsSettings true "Notifications settings request"
// @Success 200 {object} codersdk.NotificationsSettings
// @Success 304
// @Router /notifications/settings [put]
func (api *API) putNotificationsSettings(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

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

	currentSettingsJSON, err := api.Database.GetNotificationsSettings(ctx)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to fetch current notifications settings.",
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
		if rbac.IsUnauthorizedError(err) {
			httpapi.Forbidden(rw)
		} else {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to update notifications settings.",
				Detail:  err.Error(),
			})
		}

		return
	}

	httpapi.Write(r.Context(), rw, http.StatusOK, settings)
}

// @Summary Get system notification templates
// @ID get-system-notification-templates
// @Security CoderSessionToken
// @Produce json
// @Tags Notifications
// @Success 200 {array} codersdk.NotificationTemplate
// @Router /notifications/templates/system [get]
func (api *API) systemNotificationTemplates(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	templates, err := api.Database.GetNotificationTemplatesByKind(ctx, database.NotificationTemplateKindSystem)
	if err != nil {
		httpapi.Write(r.Context(), rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to retrieve system notifications templates.",
			Detail:  err.Error(),
		})
		return
	}

	out := convertNotificationTemplates(templates)
	httpapi.Write(r.Context(), rw, http.StatusOK, out)
}

// @Summary TODO Get notification templates pertaining to system events
// @ID TODO system-notification-templates
// @Security TODO CoderSessionToken
// @Produce TODO json
// @Tags TODO Notifications
// @Success TODO 200 {array} codersdk.NotificationTemplate
// @Router TODO /notifications/templates/system [get]
func (api *API) userNotificationPreferences(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	key := httpmw.APIKey(r)

	if !api.Authorize(r, policy.ActionReadPersonal, rbac.ResourceNotificationPreference) {
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: "Insufficient permissions to update notification preferences.",
		})
		return
	}

	prefs, err := api.Database.GetUserNotificationPreferences(ctx, key.UserID)
	if err != nil {
		httpapi.Write(r.Context(), rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to retrieve user notification preferences.",
			Detail:  err.Error(),
		})
		return
	}

	out := convertNotificationPreferences(prefs)
	httpapi.Write(r.Context(), rw, http.StatusOK, out)
}

// @Summary TODO Get notification templates pertaining to system events
// @ID TODO system-notification-templates
// @Security TODO CoderSessionToken
// @Produce TODO json
// @Tags TODO Notifications
// @Success TODO 200 {array} codersdk.NotificationTemplate
// @Router TODO /notifications/templates/system [put]
func (api *API) putUserNotificationPreferences(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if !api.Authorize(r, policy.ActionUpdatePersonal, rbac.ResourceNotificationPreference) {
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: "Insufficient permissions to update notification preferences.",
		})
		return
	}

	var prefs codersdk.UpdateUserNotificationPreferences
	if !httpapi.Read(ctx, rw, r, &prefs) {
		return
	}

	input := database.UpdateUserNotificationPreferencesParams{
		NotificationTemplateIds: make([]uuid.UUID, 0, len(prefs.TemplateDisabledMap)),
		Disableds:               make([]bool, 0, len(prefs.TemplateDisabledMap)),
	}
	for tmplID, disabled := range prefs.TemplateDisabledMap {
		id, err := uuid.Parse(tmplID)
		if err != nil {
			httpapi.Write(r.Context(), rw, http.StatusBadRequest, codersdk.Response{
				Message: "Unable to parse notification template UUID.",
				Detail:  err.Error(),
			})
			return
		}

		input.NotificationTemplateIds = append(input.NotificationTemplateIds, id)
		input.Disableds = append(input.Disableds, disabled)
	}

	updated, err := api.Database.UpdateUserNotificationPreferences(ctx, input)
	if err != nil {
		httpapi.Write(r.Context(), rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to update notifications preferences.",
			Detail:  err.Error(),
		})
		return
	}

	out := convertNotificationPreferences(updated)
	httpapi.Write(r.Context(), rw, http.StatusOK, out)
}

func convertNotificationTemplates(in []database.NotificationTemplate) (out []codersdk.NotificationTemplate) {
	for _, tmpl := range in {
		out = append(out, codersdk.NotificationTemplate{
			ID:            tmpl.ID,
			Name:          tmpl.Name,
			TitleTemplate: tmpl.TitleTemplate,
			BodyTemplate:  tmpl.BodyTemplate,
			Actions:       string(tmpl.Actions),
			Group:         tmpl.Group.String,
			Method:        string(tmpl.Method.NotificationMethod),
			Kind:          string(tmpl.Kind),
		})
	}

	return out
}

func convertNotificationPreferences(in []database.NotificationPreference) (out []codersdk.NotificationPreference) {
	for _, pref := range in {
		out = append(out, codersdk.NotificationPreference{
			NotificationTemplateID: pref.NotificationTemplateID,
			Disabled:               pref.Disabled,
			UpdatedAt:              pref.UpdatedAt,
		})
	}

	return out
}

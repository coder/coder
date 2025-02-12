package coderd

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
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
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to update notifications settings.",
			Detail:  err.Error(),
		})

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

// @Summary Get notification dispatch methods
// @ID get-notification-dispatch-methods
// @Security CoderSessionToken
// @Produce json
// @Tags Notifications
// @Success 200 {array} codersdk.NotificationMethodsResponse
// @Router /notifications/dispatch-methods [get]
func (api *API) notificationDispatchMethods(rw http.ResponseWriter, r *http.Request) {
	var methods []string
	for _, nm := range database.AllNotificationMethodValues() {
		methods = append(methods, string(nm))
	}

	httpapi.Write(r.Context(), rw, http.StatusOK, codersdk.NotificationMethodsResponse{
		AvailableNotificationMethods: methods,
		DefaultNotificationMethod:    api.DeploymentValues.Notifications.Method.Value(),
	})
}

// @Summary Get user notification preferences
// @ID get-user-notification-preferences
// @Security CoderSessionToken
// @Produce json
// @Tags Notifications
// @Param user path string true "User ID, name, or me"
// @Success 200 {array} codersdk.NotificationPreference
// @Router /users/{user}/notifications/preferences [get]
func (api *API) userNotificationPreferences(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx    = r.Context()
		user   = httpmw.UserParam(r)
		logger = api.Logger.Named("notifications.preferences").With(slog.F("user_id", user.ID))
	)

	prefs, err := api.Database.GetUserNotificationPreferences(ctx, user.ID)
	if err != nil {
		logger.Error(ctx, "failed to retrieve preferences", slog.Error(err))

		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to retrieve user notification preferences.",
			Detail:  err.Error(),
		})
		return
	}

	out := convertNotificationPreferences(prefs)
	httpapi.Write(ctx, rw, http.StatusOK, out)
}

// @Summary Update user notification preferences
// @ID update-user-notification-preferences
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Notifications
// @Param request body codersdk.UpdateUserNotificationPreferences true "Preferences"
// @Param user path string true "User ID, name, or me"
// @Success 200 {array} codersdk.NotificationPreference
// @Router /users/{user}/notifications/preferences [put]
func (api *API) putUserNotificationPreferences(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx    = r.Context()
		user   = httpmw.UserParam(r)
		logger = api.Logger.Named("notifications.preferences").With(slog.F("user_id", user.ID))
	)

	// Parse request.
	var prefs codersdk.UpdateUserNotificationPreferences
	if !httpapi.Read(ctx, rw, r, &prefs) {
		return
	}

	// Build query params.
	input := database.UpdateUserNotificationPreferencesParams{
		UserID:                  user.ID,
		NotificationTemplateIds: make([]uuid.UUID, 0, len(prefs.TemplateDisabledMap)),
		Disableds:               make([]bool, 0, len(prefs.TemplateDisabledMap)),
	}
	for tmplID, disabled := range prefs.TemplateDisabledMap {
		id, err := uuid.Parse(tmplID)
		if err != nil {
			logger.Warn(ctx, "failed to parse notification template UUID", slog.F("input", tmplID), slog.Error(err))

			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Unable to parse notification template UUID.",
				Detail:  err.Error(),
			})
			return
		}

		input.NotificationTemplateIds = append(input.NotificationTemplateIds, id)
		input.Disableds = append(input.Disableds, disabled)
	}

	// Update preferences with params.
	updated, err := api.Database.UpdateUserNotificationPreferences(ctx, input)
	if err != nil {
		logger.Error(ctx, "failed to update preferences", slog.Error(err))

		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to update user notifications preferences.",
			Detail:  err.Error(),
		})
		return
	}

	// Preferences updated, now fetch all preferences belonging to this user.
	logger.Info(ctx, "updated preferences", slog.F("count", updated))

	userPrefs, err := api.Database.GetUserNotificationPreferences(ctx, user.ID)
	if err != nil {
		logger.Error(ctx, "failed to retrieve preferences", slog.Error(err))

		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to retrieve user notifications preferences.",
			Detail:  err.Error(),
		})
		return
	}

	out := convertNotificationPreferences(userPrefs)
	httpapi.Write(ctx, rw, http.StatusOK, out)
}

func convertNotificationTemplates(in []database.NotificationTemplate) (out []codersdk.NotificationTemplate) {
	for _, tmpl := range in {
		out = append(out, codersdk.NotificationTemplate{
			ID:               tmpl.ID,
			Name:             tmpl.Name,
			TitleTemplate:    tmpl.TitleTemplate,
			BodyTemplate:     tmpl.BodyTemplate,
			Actions:          string(tmpl.Actions),
			Group:            tmpl.Group.String,
			Method:           string(tmpl.Method.NotificationMethod),
			Kind:             string(tmpl.Kind),
			EnabledByDefault: tmpl.EnabledByDefault,
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

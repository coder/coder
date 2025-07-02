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
	"github.com/coder/coder/v2/enterprise/coderd/prebuilds"
)

// @Summary Get prebuilds settings
// @ID get-prebuilds-settings
// @Security CoderSessionToken
// @Produce json
// @Tags Prebuilds
// @Success 200 {object} codersdk.PrebuildsSettings
// @Router /prebuilds/settings [get]
func (api *API) prebuildsSettings(rw http.ResponseWriter, r *http.Request) {
	settingsJSON, err := api.Database.GetPrebuildsSettings(r.Context())
	if err != nil {
		httpapi.Write(r.Context(), rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to fetch current prebuilds settings.",
			Detail:  err.Error(),
		})
		return
	}

	var settings codersdk.PrebuildsSettings
	if len(settingsJSON) > 0 {
		if err := json.Unmarshal([]byte(settingsJSON), &settings); err != nil {
			httpapi.Write(r.Context(), rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to unmarshal prebuilds settings.",
				Detail:  err.Error(),
			})
			return
		}
	}

	httpapi.Write(r.Context(), rw, http.StatusOK, settings)
}

// @Summary Update prebuilds settings
// @ID update-prebuilds-settings
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Prebuilds
// @Param request body codersdk.PrebuildsSettings true "Prebuilds settings request"
// @Success 200 {object} codersdk.PrebuildsSettings
// @Success 304
// @Router /prebuilds/settings [put]
func (api *API) putPrebuildsSettings(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var settings codersdk.PrebuildsSettings
	if !httpapi.Read(ctx, rw, r, &settings) {
		return
	}

	settingsJSON, err := json.Marshal(&settings)
	if err != nil {
		httpapi.Write(r.Context(), rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to marshal prebuilds settings.",
			Detail:  err.Error(),
		})
		return
	}

	currentSettingsJSON, err := api.Database.GetPrebuildsSettings(ctx)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to fetch current prebuilds settings.",
			Detail:  err.Error(),
		})
		return
	}

	if bytes.Equal(settingsJSON, []byte(currentSettingsJSON)) {
		// See: https://www.rfc-editor.org/rfc/rfc7232#section-4.1
		httpapi.Write(ctx, rw, http.StatusNotModified, nil)
		return
	}

	auditor := api.AGPL.Auditor.Load()
	aReq, commitAudit := audit.InitRequest[database.PrebuildsSettings](rw, &audit.RequestParams{
		Audit:   *auditor,
		Log:     api.Logger,
		Request: r,
		Action:  database.AuditActionWrite,
	})
	defer commitAudit()

	aReq.New = database.PrebuildsSettings{
		ID:                   uuid.New(),
		ReconciliationPaused: settings.ReconciliationPaused,
	}

	err = prebuilds.SetPrebuildsReconciliationPaused(ctx, api.Database, settings.ReconciliationPaused)
	if err != nil {
		if rbac.IsUnauthorizedError(err) {
			httpapi.Forbidden(rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to update prebuilds settings.",
			Detail:  err.Error(),
		})

		return
	}

	httpapi.Write(r.Context(), rw, http.StatusOK, settings)
}

package coderd
import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
)
// @Summary Update notification template dispatch method
// @ID update-notification-template-dispatch-method
// @Security CoderSessionToken
// @Produce json
// @Param notification_template path string true "Notification template UUID"
// @Tags Enterprise
// @Success 200 "Success"
// @Success 304 "Not modified"
// @Router /notifications/templates/{notification_template}/method [put]
func (api *API) updateNotificationTemplateMethod(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		template          = httpmw.NotificationTemplateParam(r)
		auditor           = api.AGPL.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.NotificationTemplate](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionWrite,
		})
	)
	var req codersdk.UpdateNotificationTemplateMethod
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	var nm database.NullNotificationMethod
	if err := nm.Scan(req.Method); err != nil || !nm.Valid || !nm.NotificationMethod.Valid() {
		vals := database.AllNotificationMethodValues()
		acceptable := make([]string, len(vals))
		for i, v := range vals {
			acceptable[i] = string(v)
		}
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid request to update notification template method",
			Validations: []codersdk.ValidationError{
				{
					Field: "method",
					Detail: fmt.Sprintf("%q is not a valid method; %s are the available options",
						req.Method, strings.Join(acceptable, ", "),
					),
				},
			},
		})
		return
	}
	if template.Method == nm {
		httpapi.Write(ctx, rw, http.StatusNotModified, codersdk.Response{
			Message: "Notification template method unchanged.",
		})
		return
	}
	defer commitAudit()
	aReq.Old = template
	err := api.Database.InTx(func(tx database.Store) error {
		var err error
		template, err = api.Database.UpdateNotificationTemplateMethodByID(r.Context(), database.UpdateNotificationTemplateMethodByIDParams{
			ID:     template.ID,
			Method: nm,
		})
		if err != nil {
			return fmt.Errorf("failed to update notification template ID: %w", err)
		}
		return err
	}, nil)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	aReq.New = template
	httpapi.Write(ctx, rw, http.StatusOK, codersdk.Response{
		Message: "Successfully updated notification template method.",
	})
}

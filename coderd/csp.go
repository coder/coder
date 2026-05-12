package coderd

import (
	"encoding/json"
	"net/http"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

type cspViolation struct {
	Report map[string]interface{} `json:"csp-report"`
}

// logReportCSPViolations will log all reported csp violations.
//
// @Summary Report CSP violations
// @ID report-csp-violations
// @Security CoderSessionToken
// @Accept json
// @Tags General
// @Param request body cspViolation true "Violation report"
// @Success 200
// @Router /api/v2/csp/reports [post]
func (api *API) logReportCSPViolations(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var v cspViolation

	dec := json.NewDecoder(r.Body)
	err := dec.Decode(&v)
	if err != nil {
		api.Logger.Warn(ctx, "CSP violation reported", slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to read body, invalid json.",
			Detail:  err.Error(),
		})
		return
	}

	api.Logger.Debug(ctx, "CSP violation reported", slog.F("report", v.Report))

	httpapi.Write(ctx, rw, http.StatusOK, "ok")
}

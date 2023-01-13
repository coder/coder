package coderd

import (
	"encoding/json"
	"net/http"

	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/codersdk"

	"cdr.dev/slog"
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
// @Router /csp/reports [post]
func (api *API) logReportCSPViolations(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var v cspViolation

	dec := json.NewDecoder(r.Body)
	err := dec.Decode(&v)
	if err != nil {
		api.Logger.Warn(ctx, "csp violation", slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to read body, invalid json.",
			Detail:  err.Error(),
		})
		return
	}

	fields := make([]slog.Field, 0, len(v.Report))
	for k, v := range v.Report {
		fields = append(fields, slog.F(k, v))
	}
	api.Logger.Debug(ctx, "csp violation", fields...)

	httpapi.Write(ctx, rw, http.StatusOK, "ok")
}

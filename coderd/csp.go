package coderd

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

// cspReportMaxBytes bounds the size of a single CSP violation report. This
// endpoint is unauthenticated and CSRF-exempt (it's the browser's
// `report-uri` target), so it must not allow unbounded body sizes to reach
// json.Decode. Real CSP reports are small JSON objects; 64KB is generous.
const cspReportMaxBytes = 64 * 1024

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
// @Failure 413 {object} codersdk.Response
// @Router /api/v2/csp/reports [post]
func (api *API) logReportCSPViolations(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var v cspViolation

	r.Body = http.MaxBytesReader(rw, r.Body, cspReportMaxBytes)
	dec := json.NewDecoder(r.Body)
	err := dec.Decode(&v)
	if err != nil {
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			httpapi.Write(ctx, rw, http.StatusRequestEntityTooLarge, codersdk.Response{
				Message: "Request body too large.",
				Detail:  fmt.Sprintf("Maximum CSP report size is %d bytes.", cspReportMaxBytes),
			})
			return
		}
		api.Logger.Warn(ctx, "CSP violation reported", slog.Error(err))
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
	api.Logger.Debug(ctx, "CSP violation reported", fields...)

	httpapi.Write(ctx, rw, http.StatusOK, "ok")
}

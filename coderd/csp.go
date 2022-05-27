package coderd

import (
	"encoding/json"
	"net/http"

	"github.com/coder/coder/coderd/httpapi"

	"cdr.dev/slog"
)

type cspViolation struct {
	Report map[string]interface{} `json:"csp-report"`
}

// logReportCSPViolations will log all reported csp violations.
func (api *API) logReportCSPViolations(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var v cspViolation

	dec := json.NewDecoder(r.Body)
	err := dec.Decode(&v)
	if err != nil {
		api.Logger.Warn(ctx, "csp violation", slog.Error(err))
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message: "failed to read body",
		})
		return
	}

	fields := make([]slog.Field, 0, len(v.Report))
	for k, v := range v.Report {
		fields = append(fields, slog.F(k, v))
	}
	api.Logger.Warn(ctx, "csp violation", fields...)

	httpapi.Write(rw, http.StatusOK, "ok")
}

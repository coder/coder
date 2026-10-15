package coderd

import (
	"fmt"
	"net/http"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/mcp"
	"github.com/coder/coder/v2/coderd/telemetry"
	"github.com/coder/coder/v2/codersdk"
)

const (
	// maxMCPToolUsageReportEntries bounds a single report. A report has at
	// most one entry per tool and normalized client, which is well below
	// this limit.
	maxMCPToolUsageReportEntries = 256
	// maxMCPToolUsageReportCalls bounds the calls in one entry so a single
	// report cannot inflate the aggregates arbitrarily.
	maxMCPToolUsageReportCalls = 100_000
)

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
// @Summary Report MCP tool usage
// @ID report-mcp-tool-usage
// @Security CoderSessionToken
// @Accept json
// @Tags General
// @Param request body codersdk.MCPToolUsageReport true "Aggregated tool call counts"
// @Success 204
// @Router /api/experimental/mcp/tool-usage [post]
// @x-apidocgen {"skip": true}
func (api *API) postMCPToolUsage(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req codersdk.MCPToolUsageReport
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	if len(req.Entries) > maxMCPToolUsageReportEntries {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("A report can contain at most %d entries.", maxMCPToolUsageReportEntries),
		})
		return
	}
	var validations []codersdk.ValidationError
	for i, entry := range req.Entries {
		errs := entry.ToolErrors + entry.ValidationErrors + entry.AuthzErrors
		if entry.Calls <= 0 || entry.Calls > maxMCPToolUsageReportCalls ||
			entry.ToolErrors < 0 || entry.ValidationErrors < 0 || entry.AuthzErrors < 0 ||
			errs > entry.Calls {
			validations = append(validations, codersdk.ValidationError{
				Field:  fmt.Sprintf("entries[%d]", i),
				Detail: fmt.Sprintf("calls must be between 1 and %d, and error counts must be non-negative and not exceed calls", maxMCPToolUsageReportCalls),
			})
		}
	}
	if len(validations) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid MCP tool usage report.",
			Validations: validations,
		})
		return
	}

	// Counts are only kept to be reported, so drop them when the
	// deployment does not send telemetry.
	if api.Telemetry.Enabled() {
		for _, entry := range req.Entries {
			api.MCPToolUsage.Add(telemetry.MCPToolUsageKey{
				Transport: telemetry.MCPTransportStdio,
				Tool:      mcp.NormalizeToolName(entry.Tool),
				Client:    mcp.NormalizeClientName(entry.Client),
			}, telemetry.MCPToolUsageCounts{
				Calls:            entry.Calls,
				ToolErrors:       entry.ToolErrors,
				ValidationErrors: entry.ValidationErrors,
				AuthzErrors:      entry.AuthzErrors,
			})
		}
	}
	rw.WriteHeader(http.StatusNoContent)
}

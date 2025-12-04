package coderd

import (
	"net/http"

	agpl "github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/searchquery"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary Get boundary network audit logs
// @ID get-boundary-network-audit-logs
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param q query string false "Search query"
// @Param limit query int true "Page limit"
// @Param offset query int false "Page offset"
// @Success 200 {object} codersdk.BoundaryNetworkAuditLogResponse
// @Router /boundary-network-audit-logs [get]
func (api *API) boundaryNetworkAuditLogs(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)

	page, ok := agpl.ParsePagination(rw, r)
	if !ok {
		return
	}

	queryStr := r.URL.Query().Get("q")
	filter, countFilter, errs := searchquery.BoundaryNetworkAuditLogs(ctx, api.Database, queryStr, apiKey)
	if len(errs) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid boundary network audit log search query.",
			Validations: errs,
		})
		return
	}
	// #nosec G115 - Safe conversion as pagination offset is expected to be within int32 range
	filter.OffsetOpt = int32(page.Offset)
	// #nosec G115 - Safe conversion as pagination limit is expected to be within int32 range
	filter.LimitOpt = int32(page.Limit)

	count, err := api.Database.CountBoundaryNetworkAuditLogs(ctx, countFilter)
	if dbauthz.IsNotAuthorizedError(err) {
		httpapi.Forbidden(rw)
		return
	}
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	if count == 0 {
		httpapi.Write(ctx, rw, http.StatusOK, codersdk.BoundaryNetworkAuditLogResponse{
			Logs:  []codersdk.BoundaryNetworkAuditLog{},
			Count: 0,
		})
		return
	}

	dblogs, err := api.Database.GetBoundaryNetworkAuditLogs(ctx, filter)
	if dbauthz.IsNotAuthorizedError(err) {
		httpapi.Forbidden(rw)
		return
	}
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.BoundaryNetworkAuditLogResponse{
		Logs:  convertBoundaryNetworkAuditLogs(dblogs),
		Count: count,
	})
}

func convertBoundaryNetworkAuditLogs(dblogs []database.GetBoundaryNetworkAuditLogsRow) []codersdk.BoundaryNetworkAuditLog {
	logs := make([]codersdk.BoundaryNetworkAuditLog, 0, len(dblogs))
	for _, dblog := range dblogs {
		logs = append(logs, convertBoundaryNetworkAuditLog(dblog))
	}
	return logs
}

func convertBoundaryNetworkAuditLog(dblog database.GetBoundaryNetworkAuditLogsRow) codersdk.BoundaryNetworkAuditLog {
	return codersdk.BoundaryNetworkAuditLog{
		ID:   dblog.ID,
		Time: dblog.Time,
		Organization: codersdk.MinimalOrganization{
			ID:          dblog.OrganizationID,
			Name:        dblog.OrganizationName,
			DisplayName: dblog.OrganizationDisplayName,
			Icon:        dblog.OrganizationIcon,
		},
		WorkspaceID:            dblog.WorkspaceID,
		WorkspaceOwnerID:       dblog.WorkspaceOwnerID,
		WorkspaceOwnerUsername: dblog.WorkspaceOwnerUsername,
		WorkspaceName:          dblog.WorkspaceName,
		AgentID:                dblog.AgentID,
		AgentName:              dblog.AgentName,
		Domain:                 dblog.Domain,
		Action:                 codersdk.BoundaryNetworkAction(dblog.Action),
	}
}

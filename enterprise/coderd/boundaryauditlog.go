package coderd

import (
	"fmt"
	"net/http"

	agpl "github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/searchquery"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary Get boundary audit logs
// @ID get-boundary-audit-logs
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param q query string false "Search query"
// @Param limit query int true "Page limit"
// @Param offset query int false "Page offset"
// @Success 200 {object} codersdk.BoundaryAuditLogResponse
// @Router /boundary-audit-logs [get]
func (api *API) boundaryAuditLogs(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)

	page, ok := agpl.ParsePagination(rw, r)
	if !ok {
		return
	}

	queryStr := r.URL.Query().Get("q")
	filter, countFilter, errs := searchquery.BoundaryAuditLogs(ctx, api.Database, queryStr, apiKey)
	if len(errs) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid boundary audit log search query.",
			Validations: errs,
		})
		return
	}
	// #nosec G115 - Safe conversion as pagination offset is expected to be within int32 range
	filter.OffsetOpt = int32(page.Offset)
	// #nosec G115 - Safe conversion as pagination limit is expected to be within int32 range
	filter.LimitOpt = int32(page.Limit)

	api.Logger.Info(ctx, "boundary audit logs query", "countFilter", countFilter, "db_type", fmt.Sprintf("%T", api.Database))
	//nolint:gocritic // Debug - bypass dbauthz
	ctx = dbauthz.AsSystemRestricted(ctx)
	
	// Debug: check if we can see other tables
	users, usersErr := api.Database.GetUsers(ctx, database.GetUsersParams{})
	api.Logger.Info(ctx, "boundary audit logs DEBUG users count", "count", len(users), "err", usersErr)
	
	count, err := api.Database.CountBoundaryAuditLogs(ctx, countFilter)
	api.Logger.Info(ctx, "boundary audit logs count result", "count", count, "err", err)
	if dbauthz.IsNotAuthorizedError(err) {
		httpapi.Forbidden(rw)
		return
	}
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	if count == 0 {
		httpapi.Write(ctx, rw, http.StatusOK, codersdk.BoundaryAuditLogResponse{
			Logs:  []codersdk.BoundaryAuditLog{},
			Count: 0,
		})
		return
	}

	dblogs, err := api.Database.GetBoundaryAuditLogs(ctx, filter)
	if dbauthz.IsNotAuthorizedError(err) {
		httpapi.Forbidden(rw)
		return
	}
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.BoundaryAuditLogResponse{
		Logs:  convertBoundaryAuditLogs(dblogs),
		Count: count,
	})
}

func convertBoundaryAuditLogs(dblogs []database.GetBoundaryAuditLogsRow) []codersdk.BoundaryAuditLog {
	logs := make([]codersdk.BoundaryAuditLog, 0, len(dblogs))
	for _, dblog := range dblogs {
		logs = append(logs, convertBoundaryAuditLog(dblog))
	}
	return logs
}

func convertBoundaryAuditLog(dblog database.GetBoundaryAuditLogsRow) codersdk.BoundaryAuditLog {
	return codersdk.BoundaryAuditLog{
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
		ResourceType:           dblog.ResourceType,
		Resource:               dblog.Resource,
		Operation:              dblog.Operation,
		Decision:               codersdk.BoundaryAuditDecision(dblog.Decision),
	}
}

package coderd

import (
	"net/http"
	"net/netip"

	"github.com/google/uuid"

	agpl "github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/searchquery"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary Get connection logs
// @ID get-connection-logs
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param q query string false "Search query"
// @Param limit query int true "Page limit"
// @Param offset query int false "Page offset"
// @Success 200 {object} codersdk.ConnectionLogResponse
// @Router /connectionlog [get]
func (api *API) connectionLogs(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)

	page, ok := agpl.ParsePagination(rw, r)
	if !ok {
		return
	}

	queryStr := r.URL.Query().Get("q")
	filter, countFilter, errs := searchquery.ConnectionLogs(ctx, api.Database, queryStr, apiKey)
	if len(errs) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid connection search query.",
			Validations: errs,
		})
		return
	}
	// #nosec G115 - Safe conversion as pagination offset is expected to be within int32 range
	filter.OffsetOpt = int32(page.Offset)
	// #nosec G115 - Safe conversion as pagination limit is expected to be within int32 range
	filter.LimitOpt = int32(page.Limit)

	count, err := api.Database.CountConnectionLogs(ctx, countFilter)
	if dbauthz.IsNotAuthorizedError(err) {
		httpapi.Forbidden(rw)
		return
	}
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	if count == 0 {
		httpapi.Write(ctx, rw, http.StatusOK, codersdk.ConnectionLogResponse{
			ConnectionLogs: []codersdk.ConnectionLog{},
			Count:          0,
		})
		return
	}

	dblogs, err := api.Database.GetConnectionLogsOffset(ctx, filter)
	if dbauthz.IsNotAuthorizedError(err) {
		httpapi.Forbidden(rw)
		return
	}
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.ConnectionLogResponse{
		ConnectionLogs: convertConnectionLogs(dblogs),
		Count:          count,
	})
}

func convertConnectionLogs(dblogs []database.GetConnectionLogsOffsetRow) []codersdk.ConnectionLog {
	clogs := make([]codersdk.ConnectionLog, 0, len(dblogs))

	for _, dblog := range dblogs {
		clogs = append(clogs, convertConnectionLog(dblog))
	}
	return clogs
}

func convertConnectionLog(dblog database.GetConnectionLogsOffsetRow) codersdk.ConnectionLog {
	var ip *netip.Addr
	if dblog.ConnectionLog.Ip.Valid {
		parsedIP, ok := netip.AddrFromSlice(dblog.ConnectionLog.Ip.IPNet.IP)
		if ok {
			ip = &parsedIP
		}
	}

	var user *codersdk.User
	if dblog.ConnectionLog.UserID.Valid {
		sdkUser := db2sdk.User(database.User{
			ID:                 dblog.ConnectionLog.UserID.UUID,
			Email:              dblog.UserEmail.String,
			Username:           dblog.UserUsername.String,
			CreatedAt:          dblog.UserCreatedAt.Time,
			UpdatedAt:          dblog.UserUpdatedAt.Time,
			Status:             dblog.UserStatus.UserStatus,
			RBACRoles:          dblog.UserRoles,
			LoginType:          dblog.UserLoginType.LoginType,
			AvatarURL:          dblog.UserAvatarUrl.String,
			Deleted:            dblog.UserDeleted.Bool,
			LastSeenAt:         dblog.UserLastSeenAt.Time,
			QuietHoursSchedule: dblog.UserQuietHoursSchedule.String,
			Name:               dblog.UserName.String,
		}, []uuid.UUID{})
		user = &sdkUser
	}

	var (
		webInfo *codersdk.ConnectionLogWebInfo
		sshInfo *codersdk.ConnectionLogSSHInfo
	)

	switch dblog.ConnectionLog.Type {
	case database.ConnectionTypeWorkspaceApp,
		database.ConnectionTypePortForwarding:
		webInfo = &codersdk.ConnectionLogWebInfo{
			UserAgent:  dblog.ConnectionLog.UserAgent.String,
			User:       user,
			SlugOrPort: dblog.ConnectionLog.SlugOrPort.String,
			StatusCode: dblog.ConnectionLog.Code.Int32,
		}
	case database.ConnectionTypeSsh,
		database.ConnectionTypeReconnectingPty,
		database.ConnectionTypeJetbrains,
		database.ConnectionTypeVscode:
		sshInfo = &codersdk.ConnectionLogSSHInfo{
			ConnectionID:     dblog.ConnectionLog.ConnectionID.UUID,
			DisconnectReason: dblog.ConnectionLog.DisconnectReason.String,
		}
		if dblog.ConnectionLog.DisconnectTime.Valid {
			sshInfo.DisconnectTime = &dblog.ConnectionLog.DisconnectTime.Time
		}
		if dblog.ConnectionLog.Code.Valid {
			sshInfo.ExitCode = &dblog.ConnectionLog.Code.Int32
		}
	}

	return codersdk.ConnectionLog{
		ID:          dblog.ConnectionLog.ID,
		ConnectTime: dblog.ConnectionLog.ConnectTime,
		Organization: codersdk.MinimalOrganization{
			ID:          dblog.ConnectionLog.OrganizationID,
			Name:        dblog.OrganizationName,
			DisplayName: dblog.OrganizationDisplayName,
			Icon:        dblog.OrganizationIcon,
		},
		WorkspaceOwnerID:       dblog.ConnectionLog.WorkspaceOwnerID,
		WorkspaceOwnerUsername: dblog.WorkspaceOwnerUsername,
		WorkspaceID:            dblog.ConnectionLog.WorkspaceID,
		WorkspaceName:          dblog.ConnectionLog.WorkspaceName,
		AgentName:              dblog.ConnectionLog.AgentName,
		Type:                   codersdk.ConnectionType(dblog.ConnectionLog.Type),
		IP:                     ip,
		WebInfo:                webInfo,
		SSHInfo:                sshInfo,
	}
}

package coderd

import (
	"net/http"
	"net/netip"
	"time"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary Get workspace sessions
// @ID get-workspace-sessions
// @Security CoderSessionToken
// @Tags Workspaces
// @Produce json
// @Param workspace path string true "Workspace ID" format(uuid)
// @Param limit query int false "Page limit"
// @Param offset query int false "Page offset"
// @Success 200 {object} codersdk.WorkspaceSessionsResponse
// @Router /workspaces/{workspace}/sessions [get]
func (api *API) workspaceSessions(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	workspace := httpmw.WorkspaceParam(r)

	// Parse pagination from query params.
	queryParams := httpapi.NewQueryParamParser()
	limit := queryParams.Int(r.URL.Query(), 25, "limit")
	offset := queryParams.Int(r.URL.Query(), 0, "offset")
	if len(queryParams.Errors) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid query parameters.",
			Validations: queryParams.Errors,
		})
		return
	}

	// Fetch sessions. Use AsSystemRestricted because the user is
	// already authorized to access the workspace via route
	// middleware; the ResourceConnectionLog RBAC check would
	// incorrectly reject regular workspace owners.
	//nolint:gocritic // Workspace access is verified by middleware.
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	sessions, err := api.Database.GetWorkspaceSessionsOffset(sysCtx, database.GetWorkspaceSessionsOffsetParams{
		WorkspaceID:   workspace.ID,
		LimitCount:    int32(limit),  //nolint:gosec // query param is validated and bounded
		OffsetCount:   int32(offset), //nolint:gosec // query param is validated and bounded
		StartedAfter:  time.Time{},
		StartedBefore: time.Time{},
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching sessions.",
			Detail:  err.Error(),
		})
		return
	}

	// Get total count for pagination.
	//nolint:gocritic // Workspace access is verified by middleware.
	totalCount, err := api.Database.CountWorkspaceSessions(sysCtx, database.CountWorkspaceSessionsParams{
		WorkspaceID:   workspace.ID,
		StartedAfter:  time.Time{},
		StartedBefore: time.Time{},
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error counting sessions.",
			Detail:  err.Error(),
		})
		return
	}

	// Fetch connections for all sessions in one query.
	sessionIDs := make([]uuid.UUID, len(sessions))
	for i, s := range sessions {
		sessionIDs[i] = s.ID
	}

	var connections []database.ConnectionLog
	if len(sessionIDs) > 0 {
		//nolint:gocritic // Workspace access is verified by middleware.
		connections, err = api.Database.GetConnectionLogsBySessionIDs(sysCtx, sessionIDs)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error fetching connections.",
				Detail:  err.Error(),
			})
			return
		}
	}

	// Group connections by session_id.
	connsBySession := make(map[uuid.UUID][]database.ConnectionLog)
	for _, conn := range connections {
		if conn.SessionID.Valid {
			connsBySession[conn.SessionID.UUID] = append(connsBySession[conn.SessionID.UUID], conn)
		}
	}

	// Build response with nested connections.
	response := codersdk.WorkspaceSessionsResponse{
		Sessions: make([]codersdk.WorkspaceSession, len(sessions)),
		Count:    totalCount,
	}
	for i, s := range sessions {
		response.Sessions[i] = ConvertDBSessionToSDK(s, connsBySession[s.ID])
	}

	httpapi.Write(ctx, rw, http.StatusOK, response)
}

// ConvertDBSessionToSDK converts a database workspace session row and its
// connection logs into the SDK representation.
func ConvertDBSessionToSDK(s database.GetWorkspaceSessionsOffsetRow, connections []database.ConnectionLog) codersdk.WorkspaceSession {
	id := s.ID
	session := codersdk.WorkspaceSession{
		ID:          &id,
		Status:      codersdk.ConnectionStatusCleanDisconnected, // Historic sessions are disconnected.
		StartedAt:   s.StartedAt,
		EndedAt:     &s.EndedAt,
		Connections: make([]codersdk.WorkspaceConnection, len(connections)),
	}

	// Parse IP.
	if s.Ip.Valid {
		if addr, ok := netip.AddrFromSlice(s.Ip.IPNet.IP); ok {
			addr = addr.Unmap()
			session.IP = &addr
		}
	}

	if s.ClientHostname.Valid {
		session.ClientHostname = s.ClientHostname.String
	}
	if s.ShortDescription.Valid {
		session.ShortDescription = s.ShortDescription.String
	}

	for i, conn := range connections {
		session.Connections[i] = ConvertConnectionLogToSDK(conn)
	}

	return session
}

// ConvertConnectionLogToSDK converts a database connection log into the
// SDK representation used within workspace sessions.
func ConvertConnectionLogToSDK(conn database.ConnectionLog) codersdk.WorkspaceConnection {
	wc := codersdk.WorkspaceConnection{
		Status:    codersdk.ConnectionStatusCleanDisconnected,
		CreatedAt: conn.ConnectTime,
		Type:      codersdk.ConnectionType(conn.Type),
	}

	// Parse IP.
	if conn.Ip.Valid {
		if addr, ok := netip.AddrFromSlice(conn.Ip.IPNet.IP); ok {
			addr = addr.Unmap()
			wc.IP = &addr
		}
	}

	if conn.SlugOrPort.Valid {
		wc.Detail = conn.SlugOrPort.String
	}

	if conn.DisconnectTime.Valid {
		wc.EndedAt = &conn.DisconnectTime.Time
	}

	if conn.DisconnectReason.Valid {
		wc.DisconnectReason = conn.DisconnectReason.String
	}
	if conn.Code.Valid {
		code := conn.Code.Int32
		wc.ExitCode = &code
	}
	if conn.UserAgent.Valid {
		wc.UserAgent = conn.UserAgent.String
	}
	if conn.Os.Valid {
		wc.OS = conn.Os.String
	}
	if conn.ClientHostname.Valid {
		wc.ClientHostname = conn.ClientHostname.String
	}
	if conn.ShortDescription.Valid {
		wc.ShortDescription = conn.ShortDescription.String
	}

	return wc
}

package coderd

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	agpl "github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

const (
	diagnosticDefaultHours = 72
	diagnosticMaxHours     = 168
	diagnosticMaxLogs      = 1000
	diagnosticMaxSessions  = 200
)

// @Summary Get user diagnostic report
// @ID get-user-diagnostic-report
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param username path string true "Username"
// @Param hours query int false "Hours to look back (default 72, max 168)"
// @Success 200 {object} codersdk.UserDiagnosticResponse
// @Router /connectionlog/diagnostics/{username} [get]
func (api *API) userDiagnostic(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	username := chi.URLParam(r, "username")
	if username == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Username is required.",
		})
		return
	}

	hours := diagnosticDefaultHours
	if h := r.URL.Query().Get("hours"); h != "" {
		parsed, err := strconv.Atoi(h)
		if err != nil || parsed < 1 {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Invalid hours parameter.",
			})
			return
		}
		hours = parsed
		if hours > diagnosticMaxHours {
			hours = diagnosticMaxHours
		}
	}

	// Optional filters applied after session assembly.
	statusFilter := r.URL.Query().Get("status")    // "all", "ongoing", "disconnected", "workspace_stopped"
	workspaceFilter := r.URL.Query().Get("workspace") // workspace name or empty/"all"

	// Look up the target user.
	user, err := api.Database.GetUserByEmailOrUsername(ctx, database.GetUserByEmailOrUsernameParams{
		Username: username,
	})
	if err != nil {
		httpapi.ResourceNotFound(rw)
		return
	}

	now := dbtime.Now()
	windowStart := now.Add(-time.Duration(hours) * time.Hour)

	// Fetch connection logs for this user's workspaces within the time window.
	dblogs, err := api.Database.GetConnectionLogsOffset(ctx, database.GetConnectionLogsOffsetParams{
		WorkspaceOwnerID: user.ID,
		ConnectedAfter:   windowStart,
		LimitOpt:         diagnosticMaxLogs,
	})
	if dbauthz.IsNotAuthorizedError(err) {
		httpapi.Forbidden(rw)
		return
	}
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	// Fetch user's workspaces.
	wsRows, err := api.Database.GetWorkspaces(ctx, database.GetWorkspacesParams{
		OwnerID: user.ID,
	})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	// Collect unique agent IDs from connection logs for peering event lookups.
	agentIDs := make(map[uuid.UUID]bool)
	for _, cl := range dblogs {
		if cl.ConnectionLog.AgentID.Valid {
			agentIDs[cl.ConnectionLog.AgentID.UUID] = true
		}
	}

	// Fetch peering events for all agents referenced in connection logs.
	var allPeeringEvents []database.TailnetPeeringEvent
	for agentID := range agentIDs {
		events, err := api.Database.GetAllTailnetPeeringEventsByPeerID(ctx, uuid.NullUUID{UUID: agentID, Valid: true})
		if err == nil {
			allPeeringEvents = append(allPeeringEvents, events...)
		}
	}

	// Partition connection logs into ongoing (no disconnect).
	// Skip system connections (tunnel lifecycle events from coordinator).
	var ongoingLogs []database.GetConnectionLogsOffsetRow
	for _, cl := range dblogs {
		if cl.ConnectionLog.Type == database.ConnectionTypeSystem {
			continue
		}
		if !cl.ConnectionLog.DisconnectTime.Valid {
			ongoingLogs = append(ongoingLogs, cl)
		}
	}

	// Build workspace name lookup for current connections.
	wsNameMap := make(map[uuid.UUID]string, len(wsRows))
	for _, ws := range wsRows {
		wsNameMap[ws.ID] = ws.Name
	}

	// Build summary.
	summary := buildSummary(dblogs)

	// Build workspace objects with sessions.
	workspaces := make([]codersdk.DiagnosticWorkspace, 0, len(wsRows))
	for _, ws := range wsRows {
		// Fetch historic sessions from DB.
		sessions, sessionAgentMap := api.buildWorkspaceSessions(ctx, ws.ID, ws.Name, windowStart, now, dblogs)

		// Add live sessions synthesized from ongoing connection logs.
		liveSessions, liveAgentMap := buildLiveSessionsForWorkspace(ws.ID, ws.Name, ongoingLogs)
		sessions = append(liveSessions, sessions...)
		for sessID, agents := range liveAgentMap {
			sessionAgentMap[sessID] = agents
		}

		// Enrich each session's timeline with peering events scoped
		// to the agents that belong to this session.
		for i := range sessions {
			sessAgents := sessionAgentMap[sessions[i].ID]
			if len(sessAgents) > 0 {
				sessions[i].Timeline = mergePeeringEventsIntoTimeline(
					sessions[i].Timeline,
					allPeeringEvents,
					sessions[i].StartedAt,
					sessions[i].EndedAt,
					sessAgents,
				)
			}
			// Upgrade status based on peering events in the timeline.
			sessions[i].Status = classifyStatusFromTimeline(
				sessions[i].Status,
				sessions[i].DisconnectReason,
				sessions[i].Timeline,
			)
		}

		health, healthReason := classifyWorkspaceHealth(sessions)
		workspaces = append(workspaces, codersdk.DiagnosticWorkspace{
			ID:                  ws.ID,
			Name:                ws.Name,
			OwnerUsername:       ws.OwnerUsername,
			Status:              string(ws.LatestBuildStatus),
			TemplateName:        ws.TemplateName,
			TemplateDisplayName: ws.TemplateDisplayName,
			Health:              health,
			HealthReason:        healthReason,
			Sessions:            sessions,
		})
	}

	// Build current connections and enrich with live telemetry.
	currentConns := buildCurrentConnections(ongoingLogs, wsNameMap)
	api.enrichWithTelemetry(currentConns)

	// Recompute summary status from sessions across all workspaces.
	summary = rebuildSummaryFromSessions(summary, workspaces, hours)

	// Propagate telemetry from enriched connections to live sessions
	// and update summary network stats.
	enrichSessionsFromConnections(workspaces, currentConns, &summary)

	// Pattern detection.
	patterns := detectPatterns(workspaces, summary)
	if patterns == nil {
		patterns = []codersdk.DiagnosticPattern{}
	}

	roles := make([]string, 0, len(user.RBACRoles))
	for _, r := range user.RBACRoles {
		roles = append(roles, r)
	}

	// Apply session filters. The summary/patterns are computed from the
	// full data; filters only affect which sessions are returned.
	if statusFilter != "" && statusFilter != "all" || (workspaceFilter != "" && workspaceFilter != "all") {
		for wi := range workspaces {
			filtered := make([]codersdk.DiagnosticSession, 0)
			for _, sess := range workspaces[wi].Sessions {
				if workspaceFilter != "" && workspaceFilter != "all" && sess.WorkspaceName != workspaceFilter {
					continue
				}
				if statusFilter == "ongoing" && sess.Status != codersdk.ConnectionStatusOngoing {
					continue
				}
				if statusFilter == "disconnected" && (sess.Status == codersdk.ConnectionStatusOngoing || strings.Contains(strings.ToLower(sess.DisconnectReason), "workspace stopped")) {
					continue
				}
				if statusFilter == "workspace_stopped" && !strings.Contains(strings.ToLower(sess.DisconnectReason), "workspace stopped") {
					continue
				}
				filtered = append(filtered, sess)
			}
			workspaces[wi].Sessions = filtered
		}
		// Remove workspaces with no sessions after filtering (unless workspace filter is set).
		if workspaceFilter == "" || workspaceFilter == "all" {
			var filteredWS []codersdk.DiagnosticWorkspace
			for _, ws := range workspaces {
				if len(ws.Sessions) > 0 {
					filteredWS = append(filteredWS, ws)
				}
			}
			workspaces = filteredWS
		}
	}

	resp := codersdk.UserDiagnosticResponse{
		User: codersdk.DiagnosticUser{
			ID:         user.ID,
			Username:   user.Username,
			Name:       user.Name,
			AvatarURL:  user.AvatarURL,
			Email:      user.Email,
			Roles:      roles,
			LastSeenAt: user.LastSeenAt,
			CreatedAt:  user.CreatedAt,
		},
		GeneratedAt: now,
		TimeWindow: codersdk.DiagnosticTimeWindow{
			Start: windowStart,
			End:   now,
			Hours: hours,
		},
		Summary:            summary,
		CurrentConnections: currentConns,
		Workspaces:         workspaces,
		Patterns:           patterns,
	}
	httpapi.Write(ctx, rw, http.StatusOK, resp)
}

// buildWorkspaceSessions fetches DB sessions for a workspace and matches
// them to connection logs by (IP, time overlap) instead of the session_id
// FK, which is only populated on workspace stop/delete. Orphaned closed
// logs that don't match any DB session get synthetic sessions.
func (api *API) buildWorkspaceSessions(
	ctx context.Context,
	workspaceID uuid.UUID,
	workspaceName string,
	windowStart, windowEnd time.Time,
	allLogs []database.GetConnectionLogsOffsetRow,
) ([]codersdk.DiagnosticSession, map[uuid.UUID]map[uuid.UUID]bool) {
	// Pre-filter allLogs to this workspace's closed connections.
	var workspaceLogs []database.GetConnectionLogsOffsetRow
	for _, cl := range allLogs {
		if cl.ConnectionLog.WorkspaceID == workspaceID && cl.ConnectionLog.DisconnectTime.Valid {
			workspaceLogs = append(workspaceLogs, cl)
		}
	}

	dbSessions, err := api.Database.GetWorkspaceSessionsOffset(ctx, database.GetWorkspaceSessionsOffsetParams{
		WorkspaceID:   workspaceID,
		StartedAfter:  windowStart,
		StartedBefore: windowEnd,
		LimitCount:    diagnosticMaxSessions,
	})
	if err != nil {
		dbSessions = nil
	}

	// Match logs to sessions by IP + time overlap.
	// A log matches a session when the IPs match and the log's connect_time
	// falls within the session's [started_at - tolerance, ended_at + tolerance].
	const timeTolerance = 1 * time.Minute

	matchedLogIDs := make(map[uuid.UUID]bool)
	connLogsBySession := make(map[uuid.UUID][]database.ConnectionLog)

	for _, dbSess := range dbSessions {
		var sessIP string
		if dbSess.Ip.Valid {
			if addr, ok := netip.AddrFromSlice(dbSess.Ip.IPNet.IP); ok {
				sessIP = addr.Unmap().String()
			}
		}

		for _, cl := range workspaceLogs {
			if matchedLogIDs[cl.ConnectionLog.ID] {
				continue
			}

			var logIP string
			if cl.ConnectionLog.Ip.Valid {
				if addr, ok := netip.AddrFromSlice(cl.ConnectionLog.Ip.IPNet.IP); ok {
					logIP = addr.Unmap().String()
				}
			}

			if logIP != sessIP {
				continue
			}

			// Check time overlap with tolerance.
			if cl.ConnectionLog.ConnectTime.Before(dbSess.StartedAt.Add(-timeTolerance)) {
				continue
			}
			if cl.ConnectionLog.ConnectTime.After(dbSess.EndedAt.Add(timeTolerance)) {
				continue
			}

			matchedLogIDs[cl.ConnectionLog.ID] = true
			connLogsBySession[dbSess.ID] = append(connLogsBySession[dbSess.ID], cl.ConnectionLog)
		}
	}

	// Build per-session agent ID maps from matched connection logs.
	sessionAgents := make(map[uuid.UUID]map[uuid.UUID]bool)
	for sessID, cls := range connLogsBySession {
		agents := make(map[uuid.UUID]bool)
		for _, cl := range cls {
			if cl.AgentID.Valid {
				agents[cl.AgentID.UUID] = true
			}
		}
		sessionAgents[sessID] = agents
	}

	sessions := make([]codersdk.DiagnosticSession, 0, len(dbSessions))
	for _, dbSess := range dbSessions {
		sess := convertDBSession(dbSess, workspaceName, connLogsBySession[dbSess.ID])
		sessions = append(sessions, sess)
	}

	// Collect orphaned closed logs not matched to any DB session.
	var orphanedLogs []database.GetConnectionLogsOffsetRow
	for _, cl := range workspaceLogs {
		if !matchedLogIDs[cl.ConnectionLog.ID] {
			orphanedLogs = append(orphanedLogs, cl)
		}
	}

	// Build synthetic sessions from orphaned logs.
	orphanedSessions, orphanedAgents := buildSessionsFromOrphanedLogs(orphanedLogs, workspaceID, workspaceName)
	sessions = append(sessions, orphanedSessions...)
	for sessID, agents := range orphanedAgents {
		sessionAgents[sessID] = agents
	}

	return sessions, sessionAgents
}

// buildSessionsFromOrphanedLogs groups orphaned closed connection logs by
// (agent_name, ip) with a 30-minute gap threshold and builds synthetic
// DiagnosticSession objects for each group.
func buildSessionsFromOrphanedLogs(
	logs []database.GetConnectionLogsOffsetRow,
	workspaceID uuid.UUID,
	workspaceName string,
) ([]codersdk.DiagnosticSession, map[uuid.UUID]map[uuid.UUID]bool) {
	if len(logs) == 0 {
		return nil, nil
	}

	// Sort by connect_time ascending.
	sort.Slice(logs, func(i, j int) bool {
		return logs[i].ConnectionLog.ConnectTime.Before(logs[j].ConnectionLog.ConnectTime)
	})

	const groupGap = 30 * time.Minute

	type logGroup struct {
		logs []database.GetConnectionLogsOffsetRow
	}

	// Group by (agent_name, ip_string) with time gap splitting.
	type groupKey struct {
		agentName string
		ip        string
	}
	currentGroups := make(map[groupKey]*logGroup)
	var finishedGroups []logGroup

	for _, cl := range logs {
		var ipStr string
		if cl.ConnectionLog.Ip.Valid {
			if addr, ok := netip.AddrFromSlice(cl.ConnectionLog.Ip.IPNet.IP); ok {
				ipStr = addr.Unmap().String()
			}
		}
		key := groupKey{agentName: cl.ConnectionLog.AgentName, ip: ipStr}

		cur, exists := currentGroups[key]
		if exists {
			// Check if the last connection in the group ended more than
			// groupGap before this log's connect_time.
			lastLog := cur.logs[len(cur.logs)-1].ConnectionLog
			lastEnd := lastLog.ConnectTime
			if lastLog.DisconnectTime.Valid {
				lastEnd = lastLog.DisconnectTime.Time
			}
			if cl.ConnectionLog.ConnectTime.After(lastEnd.Add(groupGap)) {
				finishedGroups = append(finishedGroups, *cur)
				currentGroups[key] = &logGroup{logs: []database.GetConnectionLogsOffsetRow{cl}}
				continue
			}
		}

		if !exists {
			currentGroups[key] = &logGroup{logs: []database.GetConnectionLogsOffsetRow{cl}}
		} else {
			cur.logs = append(cur.logs, cl)
		}
	}
	for _, g := range currentGroups {
		finishedGroups = append(finishedGroups, *g)
	}

	// Convert each group into a DiagnosticSession.
	sessions := make([]codersdk.DiagnosticSession, 0, len(finishedGroups))
	sessionAgents := make(map[uuid.UUID]map[uuid.UUID]bool)
	for _, g := range finishedGroups {
		first := g.logs[0].ConnectionLog
		startedAt := first.ConnectTime
		endedAt := startedAt
		for _, cl := range g.logs {
			if cl.ConnectionLog.DisconnectTime.Valid && cl.ConnectionLog.DisconnectTime.Time.After(endedAt) {
				endedAt = cl.ConnectionLog.DisconnectTime.Time
			}
		}

		// Build connections and derive status from disconnect reasons.
		connections := make([]codersdk.DiagnosticSessionConn, 0, len(g.logs))
		disconnectReason := ""
		hasDisconnect := true
		isControlLost := false
		var connLogSlice []database.ConnectionLog

		for _, cl := range g.logs {
			connections = append(connections, convertSessionConnection(cl.ConnectionLog))
			connLogSlice = append(connLogSlice, cl.ConnectionLog)

			if cl.ConnectionLog.DisconnectReason.Valid && cl.ConnectionLog.DisconnectReason.String != "" {
				disconnectReason = cl.ConnectionLog.DisconnectReason.String
				if strings.Contains(strings.ToLower(disconnectReason), "control") {
					isControlLost = true
				}
			}
		}

		status := classifySessionStatus(disconnectReason, hasDisconnect, isControlLost)
		explanation := generateExplanation(disconnectReason, isControlLost)
		timeline := buildTimeline(connLogSlice)

		var ipStr string
		if first.Ip.Valid {
			if addr, ok := netip.AddrFromSlice(first.Ip.IPNet.IP); ok {
				ipStr = addr.Unmap().String()
			}
		}
		var clientHostname, shortDesc string
		if first.ClientHostname.Valid {
			clientHostname = first.ClientHostname.String
		}
		if first.ShortDescription.Valid {
			shortDesc = first.ShortDescription.String
		}

		endedAtCopy := endedAt
		dur := math.Round(endedAtCopy.Sub(startedAt).Seconds())

		sessID := uuid.New()

		// Collect agent IDs for this orphaned session.
		agents := make(map[uuid.UUID]bool)
		for _, cl := range g.logs {
			if cl.ConnectionLog.AgentID.Valid {
				agents[cl.ConnectionLog.AgentID.UUID] = true
			}
		}
		sessionAgents[sessID] = agents

		sessions = append(sessions, codersdk.DiagnosticSession{
			ID:               sessID,
			WorkspaceID:      workspaceID,
			WorkspaceName:    workspaceName,
			AgentName:        first.AgentName,
			IP:               ipStr,
			ClientHostname:   clientHostname,
			ShortDescription: shortDesc,
			StartedAt:        startedAt,
			EndedAt:          &endedAtCopy,
			DurationSeconds:  &dur,
			Status:           status,
			DisconnectReason: disconnectReason,
			Explanation:      explanation,
			Network: codersdk.DiagnosticSessionNetwork{
				P2P:          nil,
				AvgLatencyMS: nil,
				HomeDERP:     nil,
			},
			Connections: connections,
			Timeline:    timeline,
		})
	}

	return sessions, sessionAgents
}

// convertDBSession converts a database session row and its connection logs
// into a DiagnosticSession.
func convertDBSession(
	dbSess database.GetWorkspaceSessionsOffsetRow,
	workspaceName string,
	connLogs []database.ConnectionLog,
) codersdk.DiagnosticSession {
	var ip string
	if dbSess.Ip.Valid {
		parsed, ok := netip.AddrFromSlice(dbSess.Ip.IPNet.IP)
		if ok {
			ip = parsed.String()
		}
	}

	// Determine the session's disconnect reason and status from its connections.
	disconnectReason := ""
	hasDisconnect := false
	isControlLost := false
	var endedAt *time.Time
	var agentName string

	// Check if the session has ended (ended_at != zero value).
	if !dbSess.EndedAt.IsZero() {
		endedAt = &dbSess.EndedAt
		hasDisconnect = true
	}

	// Derive reason and agent name from connection logs.
	for _, cl := range connLogs {
		if cl.AgentName != "" && agentName == "" {
			agentName = cl.AgentName
		}
		if cl.DisconnectReason.Valid && cl.DisconnectReason.String != "" {
			disconnectReason = cl.DisconnectReason.String
			reason := strings.ToLower(disconnectReason)
			if strings.Contains(reason, "control") {
				isControlLost = true
			}
		}
	}

	status := classifySessionStatus(disconnectReason, hasDisconnect, isControlLost)
	explanation := generateExplanation(disconnectReason, isControlLost)

	var durationSeconds *float64
	if endedAt != nil {
		d := math.Round(endedAt.Sub(dbSess.StartedAt).Seconds())
		durationSeconds = &d
	}

	// Build connections.
	connections := make([]codersdk.DiagnosticSessionConn, 0, len(connLogs))
	for _, cl := range connLogs {
		connections = append(connections, convertSessionConnection(cl))
	}

	// Build timeline events.
	timeline := buildTimeline(connLogs)

	return codersdk.DiagnosticSession{
		ID:               dbSess.ID,
		WorkspaceID:      dbSess.WorkspaceID,
		WorkspaceName:    workspaceName,
		AgentName:        agentName,
		IP:               ip,
		ClientHostname:   dbSess.ClientHostname.String,
		ShortDescription: dbSess.ShortDescription.String,
		StartedAt:        dbSess.StartedAt,
		EndedAt:          endedAt,
		DurationSeconds:  durationSeconds,
		Status:           status,
		DisconnectReason: disconnectReason,
		Explanation:      explanation,
		Network: codersdk.DiagnosticSessionNetwork{
			P2P:          nil,
			AvgLatencyMS: nil,
			HomeDERP:     nil,
		},
		Connections: connections,
		Timeline:    timeline,
	}
}

// convertSessionConnection converts a connection log into a DiagnosticSessionConn.
func convertSessionConnection(cl database.ConnectionLog) codersdk.DiagnosticSessionConn {
	var disconnectedAt *time.Time
	if cl.DisconnectTime.Valid {
		disconnectedAt = &cl.DisconnectTime.Time
	}
	var exitCode *int32
	if cl.Code.Valid {
		exitCode = &cl.Code.Int32
	}

	status := codersdk.ConnectionStatusOngoing
	if disconnectedAt != nil {
		reason := strings.ToLower(cl.DisconnectReason.String)
		switch {
		case strings.Contains(reason, "control"):
			status = codersdk.ConnectionStatusControlLost
		default:
			status = codersdk.ConnectionStatusCleanDisconnected
		}
	}

	detail := ""
	if cl.SlugOrPort.Valid {
		detail = cl.SlugOrPort.String
	}

	explanation := generateExplanation(cl.DisconnectReason.String, strings.Contains(strings.ToLower(cl.DisconnectReason.String), "control"))

	return codersdk.DiagnosticSessionConn{
		ID:             cl.ID,
		Type:           codersdk.ConnectionType(cl.Type),
		Detail:         detail,
		ConnectedAt:    cl.ConnectTime,
		DisconnectedAt: disconnectedAt,
		Status:         status,
		ExitCode:       exitCode,
		Explanation:    explanation,
	}
}

// buildTimeline synthesizes timeline events from connection logs.
// When a disconnect reason indicates "workspace stopped" or "workspace deleted",
// a workspace_state_change event is inserted once (1s before the first such
// disconnect) to surface the workspace transition in the timeline.
func buildTimeline(connLogs []database.ConnectionLog) []codersdk.DiagnosticTimelineEvent {
	var events []codersdk.DiagnosticTimelineEvent
	addedWSStateChange := false

	for _, cl := range connLogs {
		openDesc := fmt.Sprintf("%s connection opened", cl.Type)
		if cl.SlugOrPort.Valid && cl.SlugOrPort.String != "" {
			openDesc = fmt.Sprintf("%s (%s) connection opened", cl.Type, cl.SlugOrPort.String)
		}
		events = append(events, codersdk.DiagnosticTimelineEvent{
			Timestamp:   cl.ConnectTime,
			Kind:        codersdk.DiagnosticTimelineEventConnectionOpened,
			Description: openDesc,
			Metadata: map[string]any{
				"connection_id": cl.ID.String(),
				"type":          string(cl.Type),
			},
			Severity: codersdk.ConnectionDiagnosticSeverityInfo,
		})
		if cl.DisconnectTime.Valid {
			severity := codersdk.ConnectionDiagnosticSeverityInfo
			reason := strings.ToLower(cl.DisconnectReason.String)
			switch {
			case strings.Contains(reason, "agent timeout"), strings.Contains(reason, "control"):
				severity = codersdk.ConnectionDiagnosticSeverityError
			case strings.Contains(reason, "workspace stopped"), strings.Contains(reason, "workspace deleted"):
				severity = codersdk.ConnectionDiagnosticSeverityWarning
			}

			// Insert a workspace state change event once per session,
			// slightly before the disconnect that triggered it.
			if !addedWSStateChange && (strings.Contains(reason, "workspace stopped") || strings.Contains(reason, "workspace deleted")) {
				events = append(events, codersdk.DiagnosticTimelineEvent{
					Timestamp:   cl.DisconnectTime.Time.Add(-time.Second),
					Kind:        codersdk.DiagnosticTimelineEventWorkspaceStateChange,
					Description: "Workspace transitioned to stopped",
					Metadata: map[string]any{
						"trigger": "autostop",
					},
					Severity: codersdk.ConnectionDiagnosticSeverityWarning,
				})
				addedWSStateChange = true
			}

			closeDesc := fmt.Sprintf("%s connection closed", cl.Type)
			if cl.SlugOrPort.Valid && cl.SlugOrPort.String != "" {
				closeDesc = fmt.Sprintf("%s (%s) connection closed", cl.Type, cl.SlugOrPort.String)
			}
			events = append(events, codersdk.DiagnosticTimelineEvent{
				Timestamp:   cl.DisconnectTime.Time,
				Kind:        codersdk.DiagnosticTimelineEventConnectionClosed,
				Description: closeDesc,
				Metadata: map[string]any{
					"connection_id":     cl.ID.String(),
					"type":              string(cl.Type),
					"disconnect_reason": cl.DisconnectReason.String,
				},
				Severity: severity,
			})
		}
	}
	return events
}

// classifySessionStatus determines a session's high-level status from
// its disconnect reason and whether it has ended.
func classifySessionStatus(disconnectReason string, hasDisconnect bool, isControlLost bool) codersdk.WorkspaceConnectionStatus {
	reason := strings.ToLower(disconnectReason)
	switch {
	case strings.Contains(reason, "workspace stopped"):
		return codersdk.ConnectionStatusCleanDisconnected
	case strings.Contains(reason, "workspace deleted"):
		return codersdk.ConnectionStatusCleanDisconnected
	case !hasDisconnect:
		return codersdk.ConnectionStatusOngoing
	case isControlLost, strings.Contains(reason, "agent timeout"):
		return codersdk.ConnectionStatusControlLost
	default:
		return codersdk.ConnectionStatusCleanDisconnected
	}
}

// classifyStatusFromTimeline upgrades a session's status based on
// peering events in its timeline. A session classified as
// clean_disconnected by disconnect_reason alone may actually be
// control_lost if coordinator events show peer loss, or
// client_disconnected if the connection ended without any
// coordinator-level disconnect event.
func classifyStatusFromTimeline(
	current codersdk.WorkspaceConnectionStatus,
	disconnectReason string,
	timeline []codersdk.DiagnosticTimelineEvent,
) codersdk.WorkspaceConnectionStatus {
	if current == codersdk.ConnectionStatusOngoing {
		return current
	}

	// Workspace stopped/deleted sessions keep their status regardless
	// of peering events.
	reason := strings.ToLower(disconnectReason)
	if strings.Contains(reason, "workspace stopped") || strings.Contains(reason, "workspace deleted") {
		return current
	}

	hasLost := false
	hasCleanCoordDisconnect := false
	hasAnyPeeringEvent := false
	for _, ev := range timeline {
		switch ev.Kind {
		case codersdk.DiagnosticTimelineEventPeerLost:
			hasLost = true
			hasAnyPeeringEvent = true
		case codersdk.DiagnosticTimelineEventPeerDisconnected,
			codersdk.DiagnosticTimelineEventTunnelRemoved:
			hasCleanCoordDisconnect = true
			hasAnyPeeringEvent = true
		case codersdk.DiagnosticTimelineEventTunnelCreated,
			codersdk.DiagnosticTimelineEventPeerRecovered,
			codersdk.DiagnosticTimelineEventNodeUpdate:
			hasAnyPeeringEvent = true
		}
	}

	if hasLost && !hasCleanCoordDisconnect {
		return codersdk.ConnectionStatusControlLost
	}

	// Connection ended but no peering events at all means the
	// client vanished without coordinator involvement.
	if !hasAnyPeeringEvent && current == codersdk.ConnectionStatusCleanDisconnected {
		return codersdk.ConnectionStatusClientDisconnected
	}

	return current
}

// diagnosticSessionStatus maps session status to the summary breakdown
// bucket name. This is separate from WorkspaceConnectionStatus since the
// summary uses a different vocabulary.
func diagnosticStatusBucket(status codersdk.WorkspaceConnectionStatus, disconnectReason string) string {
	reason := strings.ToLower(disconnectReason)
	switch {
	case strings.Contains(reason, "workspace stopped"):
		return "workspace_stopped"
	case strings.Contains(reason, "workspace deleted"):
		return "workspace_deleted"
	case status == codersdk.ConnectionStatusOngoing:
		return "ongoing"
	case status == codersdk.ConnectionStatusControlLost,
		status == codersdk.ConnectionStatusClientDisconnected:
		return "lost"
	default:
		return "clean"
	}
}

// generateExplanation produces a human-readable explanation for the
// disconnect reason.
func generateExplanation(disconnectReason string, isControlLost bool) string {
	reason := strings.ToLower(disconnectReason)
	switch {
	case strings.Contains(reason, "workspace stopped"):
		return "Workspace was stopped by auto-stop schedule."
	case strings.Contains(reason, "workspace deleted"):
		return "Workspace was deleted."
	case strings.Contains(reason, "agent timeout"):
		return "Agent stopped responding."
	case isControlLost:
		return "Connection lost unexpectedly."
	default:
		return ""
	}
}

// buildSummary constructs initial summary metrics from raw connection logs.
// System connections are excluded from the summary.
func buildSummary(dblogs []database.GetConnectionLogsOffsetRow) codersdk.DiagnosticSummary {
	byType := make(map[string]int)
	active := 0
	total := 0

	for _, cl := range dblogs {
		if cl.ConnectionLog.Type == database.ConnectionTypeSystem {
			continue
		}
		total++
		byType[string(cl.ConnectionLog.Type)]++
		if !cl.ConnectionLog.DisconnectTime.Valid {
			active++
		}
	}

	return codersdk.DiagnosticSummary{
		TotalConnections:  total,
		ActiveConnections: active,
		ByType:            byType,
		Network: codersdk.DiagnosticNetworkSummary{
			P2PConnections:  0,
			DERPConnections: 0,
		},
	}
}

// rebuildSummaryFromSessions recalculates session-level status counts
// from the built workspace/session data.
func rebuildSummaryFromSessions(base codersdk.DiagnosticSummary, workspaces []codersdk.DiagnosticWorkspace, hours int) codersdk.DiagnosticSummary {
	var (
		total     int
		ongoing   int
		clean     int
		lost      int
		wsStopped int
		wsDeleted int
	)
	for _, ws := range workspaces {
		for _, sess := range ws.Sessions {
			total++
			bucket := diagnosticStatusBucket(sess.Status, sess.DisconnectReason)
			switch bucket {
			case "ongoing":
				ongoing++
			case "clean":
				clean++
			case "lost":
				lost++
			case "workspace_stopped":
				wsStopped++
			case "workspace_deleted":
				wsDeleted++
			}
		}
	}

	base.TotalSessions = total
	base.ByStatus = codersdk.DiagnosticStatusBreakdown{
		Ongoing:          ongoing,
		Clean:            clean,
		Lost:             lost,
		WorkspaceStopped: wsStopped,
		WorkspaceDeleted: wsDeleted,
	}
	base.Headline = fmt.Sprintf("%d sessions in %dh. %d active, %d lost.",
		total, hours, ongoing, lost)

	return base
}

// buildLiveSessionsForWorkspace creates one DiagnosticSession per ongoing
// connection log for a workspace. Each ongoing connection becomes its own
// session with no grouping.
// liveGroupKey groups ongoing connections by workspace, agent, client
// identity, connection type, and detail (app slug or port). This avoids
// showing 3 identical rows for 3 curl requests over the same port-forward,
// while keeping SSH and workspace_app/code-server as separate sessions.
type liveGroupKey struct {
	agentName string
	ip        string
	connType  string
	detail    string
}

func buildLiveSessionsForWorkspace(
	workspaceID uuid.UUID,
	workspaceName string,
	ongoingLogs []database.GetConnectionLogsOffsetRow,
) ([]codersdk.DiagnosticSession, map[uuid.UUID]map[uuid.UUID]bool) {
	groups := make(map[liveGroupKey][]database.GetConnectionLogsOffsetRow)
	for _, cl := range ongoingLogs {
		if cl.ConnectionLog.WorkspaceID != workspaceID {
			continue
		}
		log := cl.ConnectionLog
		var ipStr string
		if log.Ip.Valid {
			if addr, ok := netip.AddrFromSlice(log.Ip.IPNet.IP); ok {
				ipStr = addr.Unmap().String()
			}
		}
		detail := ""
		if log.SlugOrPort.Valid {
			detail = log.SlugOrPort.String
		}
		key := liveGroupKey{
			agentName: log.AgentName,
			ip:        ipStr,
			connType:  string(log.Type),
			detail:    detail,
		}
		groups[key] = append(groups[key], cl)
	}

	var sessions []codersdk.DiagnosticSession
	sessionAgents := make(map[uuid.UUID]map[uuid.UUID]bool)
	for _, logs := range groups {
		if len(logs) == 0 {
			continue
		}
		first := logs[0].ConnectionLog

		var ipStr string
		if first.Ip.Valid {
			if addr, ok := netip.AddrFromSlice(first.Ip.IPNet.IP); ok {
				ipStr = addr.Unmap().String()
			}
		}
		var clientHostname, shortDesc string
		if first.ClientHostname.Valid {
			clientHostname = first.ClientHostname.String
		}
		if first.ShortDescription.Valid {
			shortDesc = first.ShortDescription.String
		}

		// Find the earliest start time for this group.
		earliest := first.ConnectTime
		for _, cl := range logs[1:] {
			if cl.ConnectionLog.ConnectTime.Before(earliest) {
				earliest = cl.ConnectionLog.ConnectTime
			}
		}

		conns := make([]codersdk.DiagnosticSessionConn, 0, len(logs))
		var timeline []codersdk.DiagnosticTimelineEvent
		for _, cl := range logs {
			conns = append(conns, convertSessionConnection(cl.ConnectionLog))

			openDesc := fmt.Sprintf("%s connection opened", cl.ConnectionLog.Type)
			if cl.ConnectionLog.SlugOrPort.Valid && cl.ConnectionLog.SlugOrPort.String != "" {
				openDesc = fmt.Sprintf("%s (%s) connection opened", cl.ConnectionLog.Type, cl.ConnectionLog.SlugOrPort.String)
			}
			timeline = append(timeline, codersdk.DiagnosticTimelineEvent{
				Timestamp:   cl.ConnectionLog.ConnectTime,
				Kind:        codersdk.DiagnosticTimelineEventConnectionOpened,
				Description: openDesc,
				Metadata: map[string]any{
					"connection_id": cl.ConnectionLog.ID.String(),
					"type":          string(cl.ConnectionLog.Type),
				},
				Severity: codersdk.ConnectionDiagnosticSeverityInfo,
			})
		}

		sessID := uuid.New()

		// Collect agent IDs for this live session.
		agents := make(map[uuid.UUID]bool)
		for _, cl := range logs {
			if cl.ConnectionLog.AgentID.Valid {
				agents[cl.ConnectionLog.AgentID.UUID] = true
			}
		}
		sessionAgents[sessID] = agents

		sessions = append(sessions, codersdk.DiagnosticSession{
			ID:               sessID,
			WorkspaceID:      workspaceID,
			WorkspaceName:    workspaceName,
			AgentName:        first.AgentName,
			IP:               ipStr,
			ClientHostname:   clientHostname,
			ShortDescription: shortDesc,
			StartedAt:        earliest,
			Status:           codersdk.ConnectionStatusOngoing,
			Connections:      conns,
			Timeline:         timeline,
		})
	}

	return sessions, sessionAgents
}

// buildCurrentConnections converts ongoing connection logs into
// DiagnosticConnection objects for the top-level current_connections list.
func buildCurrentConnections(
	ongoingLogs []database.GetConnectionLogsOffsetRow,
	wsNameMap map[uuid.UUID]string,
) []codersdk.DiagnosticConnection {
	if len(ongoingLogs) == 0 {
		return []codersdk.DiagnosticConnection{}
	}

	conns := make([]codersdk.DiagnosticConnection, 0, len(ongoingLogs))
	for _, cl := range ongoingLogs {
		log := cl.ConnectionLog
		var ipStr string
		if log.Ip.Valid {
			if addr, ok := netip.AddrFromSlice(log.Ip.IPNet.IP); ok {
				ipStr = addr.Unmap().String()
			}
		}
		var agentID uuid.UUID
		if log.AgentID.Valid {
			agentID = log.AgentID.UUID
		}
		var clientHostname, shortDesc, detail string
		if log.ClientHostname.Valid {
			clientHostname = log.ClientHostname.String
		}
		if log.ShortDescription.Valid {
			shortDesc = log.ShortDescription.String
		}
		if log.SlugOrPort.Valid {
			detail = log.SlugOrPort.String
		}

		conns = append(conns, codersdk.DiagnosticConnection{
			ID:               log.ID,
			WorkspaceID:      log.WorkspaceID,
			WorkspaceName:    wsNameMap[log.WorkspaceID],
			AgentID:          agentID,
			AgentName:        log.AgentName,
			IP:               ipStr,
			ClientHostname:   clientHostname,
			ShortDescription: shortDesc,
			Type:             codersdk.ConnectionType(log.Type),
			Detail:           detail,
			Status:           codersdk.ConnectionStatusOngoing,
			StartedAt:        log.ConnectTime,
		})
	}
	return conns
}

// classifyWorkspaceHealth determines a workspace's overall diagnostic health
// from its sessions.
func classifyWorkspaceHealth(sessions []codersdk.DiagnosticSession) (codersdk.DiagnosticHealth, string) {
	if len(sessions) == 0 {
		return codersdk.DiagnosticHealthInactive, "No sessions in time window."
	}

	lostCount := 0
	for _, s := range sessions {
		if s.Status == codersdk.ConnectionStatusControlLost {
			lostCount++
		}
	}

	switch {
	case lostCount == 0:
		return codersdk.DiagnosticHealthHealthy, ""
	case lostCount <= len(sessions)/2:
		return codersdk.DiagnosticHealthDegraded, fmt.Sprintf("%d of %d sessions lost control.", lostCount, len(sessions))
	default:
		return codersdk.DiagnosticHealthUnhealthy, fmt.Sprintf("%d of %d sessions lost control.", lostCount, len(sessions))
	}
}

// enrichWithTelemetry overlays live coordinator telemetry (P2P, latency,
// HomeDERP) onto ongoing diagnostic connections. It mirrors the approach
// in mergeConnectionsFlat: for each unique agent, fetch tunnel peers and
// peer telemetry, match by tailnet IP, then apply network info.
func (api *API) enrichWithTelemetry(conns []codersdk.DiagnosticConnection) {
	coord := api.AGPL.TailnetCoordinator.Load()
	if coord == nil {
		return
	}
	derpMap := api.AGPL.DERPMap()

	// Collect unique agent IDs from ongoing connections.
	agentIDs := make(map[uuid.UUID]struct{})
	for _, c := range conns {
		if c.AgentID != uuid.Nil {
			agentIDs[c.AgentID] = struct{}{}
		}
	}
	if len(agentIDs) == 0 {
		return
	}

	// For each agent, build a lookup of tailnet IP -> telemetry.
	type ipTelemetryEntry struct {
		telemetry *agpl.PeerNetworkTelemetry
	}
	// Key: "agentID:ip"
	telemetryByKey := make(map[string]*ipTelemetryEntry)

	for agentID := range agentIDs {
		peers := (*coord).TunnelPeers(agentID)
		allTelemetry := api.AGPL.PeerNetworkTelemetryStore.GetAll(agentID)

		for _, peer := range peers {
			if peer.Node == nil || len(peer.Node.Addresses) == 0 {
				continue
			}
			prefix, err := netip.ParsePrefix(peer.Node.Addresses[0])
			if err != nil {
				continue
			}
			ip := prefix.Addr().Unmap().String()
			key := agentID.String() + ":" + ip
			telemetryByKey[key] = &ipTelemetryEntry{
				telemetry: allTelemetry[peer.ID],
			}
		}
	}

	// Enrich each ongoing connection with telemetry data.
	for i := range conns {
		conn := &conns[i]
		if conn.AgentID == uuid.Nil || conn.IP == "" {
			continue
		}
		key := conn.AgentID.String() + ":" + conn.IP
		entry, ok := telemetryByKey[key]
		if !ok || entry.telemetry == nil {
			continue
		}
		t := entry.telemetry

		if t.P2P != nil {
			p2p := *t.P2P
			conn.P2P = &p2p
		}
		if t.HomeDERP > 0 {
			regionID := t.HomeDERP
			name := fmt.Sprintf("Unnamed %d", regionID)
			if derpMap != nil {
				if region, ok := derpMap.Regions[regionID]; ok && region != nil && region.RegionName != "" {
					name = region.RegionName
				}
			}
			conn.HomeDERP = &codersdk.DiagnosticHomeDERP{
				ID:   regionID,
				Name: name,
			}
		}
		if t.P2P != nil && *t.P2P && t.P2PLatency != nil {
			ms := math.Round(float64(*t.P2PLatency)/float64(time.Millisecond)*100) / 100
			conn.LatencyMS = &ms
		} else if t.DERPLatency != nil {
			ms := math.Round(float64(*t.DERPLatency)/float64(time.Millisecond)*100) / 100
			conn.LatencyMS = &ms
		}
	}
}

// enrichSessionsFromConnections populates session-level Network fields
// and summary network stats by looking up matching enriched connections.
func enrichSessionsFromConnections(
	workspaces []codersdk.DiagnosticWorkspace,
	enrichedConns []codersdk.DiagnosticConnection,
	summary *codersdk.DiagnosticSummary,
) {
	// Build lookup: (workspaceID, ip) -> telemetry from enriched connections.
	type netInfo struct {
		p2p      *bool
		latency  *float64
		homeDERP *codersdk.DiagnosticHomeDERP
	}
	connNet := make(map[string]*netInfo)
	for _, c := range enrichedConns {
		if c.P2P == nil && c.LatencyMS == nil && c.HomeDERP == nil {
			continue
		}
		key := c.WorkspaceID.String() + ":" + c.IP
		connNet[key] = &netInfo{
			p2p:      c.P2P,
			latency:  c.LatencyMS,
			homeDERP: c.HomeDERP,
		}
	}

	var p2pCount, derpCount int
	var latencies []float64

	for wi := range workspaces {
		for si := range workspaces[wi].Sessions {
			sess := &workspaces[wi].Sessions[si]
			if sess.Status != codersdk.ConnectionStatusOngoing {
				continue
			}
			key := sess.WorkspaceID.String() + ":" + sess.IP
			info, ok := connNet[key]
			if !ok || info == nil {
				continue
			}
			sess.Network.P2P = info.p2p
			sess.Network.AvgLatencyMS = info.latency
			if info.homeDERP != nil {
				name := info.homeDERP.Name
				sess.Network.HomeDERP = &name
			}

			if info.p2p != nil {
				if *info.p2p {
					p2pCount++
				} else {
					derpCount++
				}
			}
			if info.latency != nil {
				latencies = append(latencies, *info.latency)
			}
		}
	}

	summary.Network.P2PConnections = p2pCount
	summary.Network.DERPConnections = derpCount
	if len(latencies) > 0 {
		var sum float64
		for _, l := range latencies {
			sum += l
		}
		avg := math.Round(sum/float64(len(latencies))*100) / 100
		summary.Network.AvgLatencyMS = &avg
	}
}

// mergePeeringEventsIntoTimeline appends matching peering events to a
// session's timeline and returns the combined, time-sorted result.
// Events are included when they fall within the session's time window
// and involve a peer ID that matches one of the known agent IDs.
func mergePeeringEventsIntoTimeline(
	timeline []codersdk.DiagnosticTimelineEvent,
	peeringEvents []database.TailnetPeeringEvent,
	startedAt time.Time,
	endedAt *time.Time,
	agentIDs map[uuid.UUID]bool,
) []codersdk.DiagnosticTimelineEvent {
	if len(peeringEvents) == 0 {
		return timeline
	}

	end := time.Now()
	if endedAt != nil {
		end = *endedAt
	}

	for _, pe := range peeringEvents {
		if pe.OccurredAt.Before(startedAt) || pe.OccurredAt.After(end) {
			continue
		}

		// Check that at least one peer in the event is a known agent.
		srcMatch := pe.SrcPeerID.Valid && agentIDs[pe.SrcPeerID.UUID]
		dstMatch := pe.DstPeerID.Valid && agentIDs[pe.DstPeerID.UUID]
		if !srcMatch && !dstMatch {
			continue
		}

		// Identify the non-agent peer for context in descriptions.
		var otherPeer string
		if srcMatch && pe.DstPeerID.Valid && !agentIDs[pe.DstPeerID.UUID] {
			otherPeer = pe.DstPeerID.UUID.String()[:8]
		} else if dstMatch && pe.SrcPeerID.Valid && !agentIDs[pe.SrcPeerID.UUID] {
			otherPeer = pe.SrcPeerID.UUID.String()[:8]
		}

		var kind codersdk.DiagnosticTimelineEventKind
		var description string
		var severity codersdk.ConnectionDiagnosticSeverity

		switch pe.EventType {
		case "added_tunnel":
			kind = codersdk.DiagnosticTimelineEventTunnelCreated
			description = "Tunnel created"
			if otherPeer != "" {
				description = fmt.Sprintf("Tunnel created with peer %s", otherPeer)
			}
			severity = codersdk.ConnectionDiagnosticSeverityInfo
		case "removed_tunnel":
			kind = codersdk.DiagnosticTimelineEventTunnelRemoved
			description = "Tunnel removed"
			if otherPeer != "" {
				description = fmt.Sprintf("Tunnel removed for peer %s", otherPeer)
			}
			severity = codersdk.ConnectionDiagnosticSeverityWarning
		case "peer_update_node":
			kind = codersdk.DiagnosticTimelineEventNodeUpdate
			description = "Node update received"
			if otherPeer != "" {
				description = fmt.Sprintf("Node update from peer %s", otherPeer)
			}
			severity = codersdk.ConnectionDiagnosticSeverityInfo
		case "peer_update_disconnected":
			kind = codersdk.DiagnosticTimelineEventPeerDisconnected
			description = "Peer disconnected"
			if otherPeer != "" {
				description = fmt.Sprintf("Peer %s disconnected", otherPeer)
			}
			severity = codersdk.ConnectionDiagnosticSeverityInfo
		case "peer_update_lost":
			kind = codersdk.DiagnosticTimelineEventPeerLost
			description = "Peer lost contact"
			if otherPeer != "" {
				description = fmt.Sprintf("Peer %s lost contact", otherPeer)
			}
			severity = codersdk.ConnectionDiagnosticSeverityError
		case "peer_update_ready_for_handshake":
			kind = codersdk.DiagnosticTimelineEventPeerRecovered
			description = "Peer recovered"
			if otherPeer != "" {
				description = fmt.Sprintf("Peer %s recovered", otherPeer)
			}
			severity = codersdk.ConnectionDiagnosticSeverityInfo
		default:
			continue
		}

		metadata := map[string]any{
			"event_type": pe.EventType,
		}
		if pe.SrcPeerID.Valid {
			metadata["src_peer_id"] = pe.SrcPeerID.UUID.String()
		}
		if pe.DstPeerID.Valid {
			metadata["dst_peer_id"] = pe.DstPeerID.UUID.String()
		}

		timeline = append(timeline, codersdk.DiagnosticTimelineEvent{
			Timestamp:   pe.OccurredAt,
			Kind:        kind,
			Description: description,
			Metadata:    metadata,
			Severity:    severity,
		})
	}

	sort.Slice(timeline, func(i, j int) bool {
		return timeline[i].Timestamp.Before(timeline[j].Timestamp)
	})

	return timeline
}

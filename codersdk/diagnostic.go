package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
)

// UserDiagnosticResponse is the top-level response from the operator
// diagnostic endpoint for a single user.
type UserDiagnosticResponse struct {
	User               DiagnosticUser         `json:"user"`
	GeneratedAt        time.Time              `json:"generated_at" format:"date-time"`
	TimeWindow         DiagnosticTimeWindow   `json:"time_window"`
	Summary            DiagnosticSummary      `json:"summary"`
	CurrentConnections []DiagnosticConnection `json:"current_connections"`
	Workspaces         []DiagnosticWorkspace  `json:"workspaces"`
	Patterns           []DiagnosticPattern    `json:"patterns"`
}

// DiagnosticUser identifies the user being diagnosed.
type DiagnosticUser struct {
	ID         uuid.UUID `json:"id" format:"uuid"`
	Username   string    `json:"username"`
	Name       string    `json:"name"`
	AvatarURL  string    `json:"avatar_url"`
	Email      string    `json:"email"`
	Roles      []string  `json:"roles"`
	LastSeenAt time.Time `json:"last_seen_at" format:"date-time"`
	CreatedAt  time.Time `json:"created_at" format:"date-time"`
}

// DiagnosticTimeWindow describes the time range covered by the diagnostic.
type DiagnosticTimeWindow struct {
	Start time.Time `json:"start" format:"date-time"`
	End   time.Time `json:"end" format:"date-time"`
	Hours int       `json:"hours"`
}

// DiagnosticSummary aggregates connection statistics across the time window.
type DiagnosticSummary struct {
	TotalSessions     int                       `json:"total_sessions"`
	TotalConnections  int                       `json:"total_connections"`
	ActiveConnections int                       `json:"active_connections"`
	ByStatus          DiagnosticStatusBreakdown `json:"by_status"`
	ByType            map[string]int            `json:"by_type"`
	Network           DiagnosticNetworkSummary  `json:"network"`
	Headline          string                    `json:"headline"`
}

// DiagnosticStatusBreakdown counts sessions by their terminal status.
type DiagnosticStatusBreakdown struct {
	Ongoing          int `json:"ongoing"`
	Clean            int `json:"clean"`
	Lost             int `json:"lost"`
	WorkspaceStopped int `json:"workspace_stopped"`
	WorkspaceDeleted int `json:"workspace_deleted"`
}

// DiagnosticNetworkSummary contains aggregate network quality metrics.
type DiagnosticNetworkSummary struct {
	P2PConnections    int      `json:"p2p_connections"`
	DERPConnections   int      `json:"derp_connections"`
	AvgLatencyMS      *float64 `json:"avg_latency_ms"`
	P95LatencyMS      *float64 `json:"p95_latency_ms"`
	PrimaryDERPRegion *string  `json:"primary_derp_region"`
}

// DiagnosticConnection describes a single live or historical connection.
type DiagnosticConnection struct {
	ID               uuid.UUID                 `json:"id" format:"uuid"`
	WorkspaceID      uuid.UUID                 `json:"workspace_id" format:"uuid"`
	WorkspaceName    string                    `json:"workspace_name"`
	AgentID          uuid.UUID                 `json:"agent_id" format:"uuid"`
	AgentName        string                    `json:"agent_name"`
	IP               string                    `json:"ip"`
	ClientHostname   string                    `json:"client_hostname"`
	ShortDescription string                    `json:"short_description"`
	Type             ConnectionType            `json:"type"`
	Detail           string                    `json:"detail"`
	Status           WorkspaceConnectionStatus `json:"status"`
	StartedAt        time.Time                 `json:"started_at" format:"date-time"`
	P2P              *bool                     `json:"p2p"`
	LatencyMS        *float64                  `json:"latency_ms"`
	HomeDERP         *DiagnosticHomeDERP       `json:"home_derp"`
	Explanation      string                    `json:"explanation"`
}

// DiagnosticHomeDERP identifies a DERP relay region.
type DiagnosticHomeDERP struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// DiagnosticWorkspace groups sessions for a single workspace.
type DiagnosticWorkspace struct {
	ID                  uuid.UUID           `json:"id" format:"uuid"`
	Name                string              `json:"name"`
	OwnerUsername       string              `json:"owner_username"`
	Status              string              `json:"status"`
	TemplateName        string              `json:"template_name"`
	TemplateDisplayName string              `json:"template_display_name"`
	Health              DiagnosticHealth    `json:"health"`
	HealthReason        string              `json:"health_reason"`
	Sessions            []DiagnosticSession `json:"sessions"`
}

// DiagnosticHealth represents workspace health status.
type DiagnosticHealth string

const (
	DiagnosticHealthHealthy   DiagnosticHealth = "healthy"
	DiagnosticHealthDegraded  DiagnosticHealth = "degraded"
	DiagnosticHealthUnhealthy DiagnosticHealth = "unhealthy"
	DiagnosticHealthInactive  DiagnosticHealth = "inactive"
)

// DiagnosticSession represents a client session with one or more connections.
type DiagnosticSession struct {
	ID               uuid.UUID                 `json:"id" format:"uuid"`
	WorkspaceID      uuid.UUID                 `json:"workspace_id" format:"uuid"`
	WorkspaceName    string                    `json:"workspace_name"`
	AgentName        string                    `json:"agent_name"`
	IP               string                    `json:"ip"`
	ClientHostname   string                    `json:"client_hostname"`
	ShortDescription string                    `json:"short_description"`
	StartedAt        time.Time                 `json:"started_at" format:"date-time"`
	EndedAt          *time.Time                `json:"ended_at" format:"date-time"`
	DurationSeconds  *float64                  `json:"duration_seconds"`
	Status           WorkspaceConnectionStatus `json:"status"`
	DisconnectReason string                    `json:"disconnect_reason"`
	Explanation      string                    `json:"explanation"`
	Network          DiagnosticSessionNetwork  `json:"network"`
	Connections      []DiagnosticSessionConn   `json:"connections"`
	Timeline         []DiagnosticTimelineEvent `json:"timeline"`
}

// DiagnosticSessionNetwork holds per-session network quality info.
type DiagnosticSessionNetwork struct {
	P2P          *bool    `json:"p2p"`
	AvgLatencyMS *float64 `json:"avg_latency_ms"`
	HomeDERP     *string  `json:"home_derp"`
}

// DiagnosticSessionConn represents a single connection within a session.
type DiagnosticSessionConn struct {
	ID             uuid.UUID                 `json:"id" format:"uuid"`
	Type           ConnectionType            `json:"type"`
	Detail         string                    `json:"detail"`
	ConnectedAt    time.Time                 `json:"connected_at" format:"date-time"`
	DisconnectedAt *time.Time                `json:"disconnected_at" format:"date-time"`
	Status         WorkspaceConnectionStatus `json:"status"`
	ExitCode       *int32                    `json:"exit_code"`
	Explanation    string                    `json:"explanation"`
}

// DiagnosticTimelineEventKind enumerates timeline event types.
type DiagnosticTimelineEventKind string

const (
	DiagnosticTimelineEventTunnelCreated        DiagnosticTimelineEventKind = "tunnel_created"
	DiagnosticTimelineEventTunnelRemoved        DiagnosticTimelineEventKind = "tunnel_removed"
	DiagnosticTimelineEventNodeUpdate           DiagnosticTimelineEventKind = "node_update"
	DiagnosticTimelineEventPeerLost             DiagnosticTimelineEventKind = "peer_lost"
	DiagnosticTimelineEventPeerRecovered        DiagnosticTimelineEventKind = "peer_recovered"
	DiagnosticTimelineEventConnectionOpened     DiagnosticTimelineEventKind = "connection_opened"
	DiagnosticTimelineEventConnectionClosed     DiagnosticTimelineEventKind = "connection_closed"
	DiagnosticTimelineEventDERPFallback         DiagnosticTimelineEventKind = "derp_fallback"
	DiagnosticTimelineEventP2PEstablished       DiagnosticTimelineEventKind = "p2p_established"
	DiagnosticTimelineEventLatencySpike         DiagnosticTimelineEventKind = "latency_spike"
	DiagnosticTimelineEventWorkspaceStateChange DiagnosticTimelineEventKind = "workspace_state_change"
)

// DiagnosticTimelineEvent records a point-in-time event within a session.
type DiagnosticTimelineEvent struct {
	Timestamp   time.Time                    `json:"timestamp" format:"date-time"`
	Kind        DiagnosticTimelineEventKind  `json:"kind"`
	Description string                       `json:"description"`
	Metadata    map[string]any               `json:"metadata"`
	Severity    ConnectionDiagnosticSeverity `json:"severity"`
}

// ConnectionDiagnosticSeverity represents event or pattern severity.
type ConnectionDiagnosticSeverity string

const (
	ConnectionDiagnosticSeverityInfo     ConnectionDiagnosticSeverity = "info"
	ConnectionDiagnosticSeverityWarning  ConnectionDiagnosticSeverity = "warning"
	ConnectionDiagnosticSeverityError    ConnectionDiagnosticSeverity = "error"
	ConnectionDiagnosticSeverityCritical ConnectionDiagnosticSeverity = "critical"
)

// DiagnosticPatternType enumerates recognized connection patterns.
type DiagnosticPatternType string

const (
	DiagnosticPatternDeviceSleep        DiagnosticPatternType = "device_sleep"
	DiagnosticPatternWorkspaceAutostart DiagnosticPatternType = "workspace_autostart"
	DiagnosticPatternNetworkPolicy      DiagnosticPatternType = "network_policy"
	DiagnosticPatternAgentCrash         DiagnosticPatternType = "agent_crash"
	DiagnosticPatternLatencyDegradation DiagnosticPatternType = "latency_degradation"
	DiagnosticPatternDERPFallback       DiagnosticPatternType = "derp_fallback"
	DiagnosticPatternCleanUsage         DiagnosticPatternType = "clean_usage"
	DiagnosticPatternUnknownDrops       DiagnosticPatternType = "unknown_drops"
)

// DiagnosticPattern describes a detected pattern across sessions.
type DiagnosticPattern struct {
	ID               uuid.UUID                    `json:"id" format:"uuid"`
	Type             DiagnosticPatternType        `json:"type"`
	Severity         ConnectionDiagnosticSeverity `json:"severity"`
	AffectedSessions int                          `json:"affected_sessions"`
	TotalSessions    int                          `json:"total_sessions"`
	Title            string                       `json:"title"`
	Description      string                       `json:"description"`
	Commonalities    DiagnosticPatternCommonality `json:"commonalities"`
	Recommendation   string                       `json:"recommendation"`
}

// DiagnosticPatternCommonality captures shared attributes of affected sessions.
type DiagnosticPatternCommonality struct {
	ConnectionTypes    []string                 `json:"connection_types"`
	ClientDescriptions []string                 `json:"client_descriptions"`
	DurationRange      *DiagnosticDurationRange `json:"duration_range"`
	DisconnectReasons  []string                 `json:"disconnect_reasons"`
	TimeOfDayRange     *string                  `json:"time_of_day_range"`
}

// DiagnosticDurationRange is a min/max pair of seconds.
type DiagnosticDurationRange struct {
	MinSeconds float64 `json:"min_seconds"`
	MaxSeconds float64 `json:"max_seconds"`
}

// UserDiagnostic fetches the operator diagnostic report for a user.
func (c *Client) UserDiagnostic(ctx context.Context, username string, hours int) (UserDiagnosticResponse, error) {
	res, err := c.Request(ctx, http.MethodGet,
		fmt.Sprintf("/api/v2/connectionlog/diagnostics/%s", username),
		nil,
		func(r *http.Request) {
			q := r.URL.Query()
			q.Set("hours", strconv.Itoa(hours))
			r.URL.RawQuery = q.Encode()
		},
	)
	if err != nil {
		return UserDiagnosticResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return UserDiagnosticResponse{}, ReadBodyAsError(res)
	}
	var resp UserDiagnosticResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

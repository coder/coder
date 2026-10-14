package codersdk

import (
	"context"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/google/uuid"
)

type ConnectionLog struct {
	ID                     uuid.UUID           `json:"id" format:"uuid"`
	ConnectTime            time.Time           `json:"connect_time" format:"date-time"`
	Organization           MinimalOrganization `json:"organization"`
	WorkspaceOwnerID       uuid.UUID           `json:"workspace_owner_id" format:"uuid"`
	WorkspaceOwnerUsername string              `json:"workspace_owner_username"`
	WorkspaceID            uuid.UUID           `json:"workspace_id" format:"uuid"`
	WorkspaceName          string              `json:"workspace_name"`
	AgentName              string              `json:"agent_name"`
	IP                     *netip.Addr         `json:"ip,omitempty"`
	// Deprecated: Use ConnectionMethod and AppName.
	Type             string              `json:"type"`
	ConnectionMethod ConnectionLogMethod `json:"connection_method"`
	// AppName is the name of the connecting application, when known. The
	// client reports it, so it is a normalized label and not a guarantee.
	// Workspace app slugs and forwarded ports are reported in WebInfo.
	AppName string `json:"app_name,omitempty"`
	// AppDisplayName is the registry display name for AppName, or AppName
	// itself when the app is unregistered.
	AppDisplayName string `json:"app_display_name,omitempty"`

	// WebInfo is set for connections that coderd records.
	WebInfo *ConnectionLogWebInfo `json:"web_info,omitempty"`

	// SSHInfo is set for connections that agents report.
	SSHInfo *ConnectionLogSSHInfo `json:"ssh_info,omitempty"`
}

// ConnectionType is a connection method with SSH split by app family. It backs
// the legacy `type` filter. Remove it when that filter and ConnectionLog.Type
// are removed.
type ConnectionType string

const (
	ConnectionTypeSSH             ConnectionType = "ssh"
	ConnectionTypeVSCode          ConnectionType = "vscode"
	ConnectionTypeJetBrains       ConnectionType = "jetbrains"
	ConnectionTypeReconnectingPTY ConnectionType = "reconnecting_pty"
	ConnectionTypeWorkspaceApp    ConnectionType = "workspace_app"
	ConnectionTypePortForwarding  ConnectionType = "port_forwarding"
	// ConnectionTypeTunnel records accepted and denied tailnet tunnel
	// requests made by authenticated users.
	ConnectionTypeTunnel ConnectionType = "tunnel"
)

func (t ConnectionType) Valid() bool {
	switch t {
	case ConnectionTypeSSH, ConnectionTypeVSCode, ConnectionTypeJetBrains,
		ConnectionTypeReconnectingPTY, ConnectionTypeWorkspaceApp,
		ConnectionTypePortForwarding, ConnectionTypeTunnel:
		return true
	default:
		return false
	}
}

// ConnectionLogStatus is the status of a connection log entry.
// It's the argument to the `status` filter when fetching connection logs.
type ConnectionLogStatus string

const (
	ConnectionLogStatusOngoing   ConnectionLogStatus = "ongoing"
	ConnectionLogStatusCompleted ConnectionLogStatus = "completed"
)

func (s ConnectionLogStatus) Valid() bool {
	switch s {
	case ConnectionLogStatusOngoing, ConnectionLogStatusCompleted:
		return true
	default:
		return false
	}
}

type ConnectionLogWebInfo struct {
	UserAgent string `json:"user_agent"`
	// User is omitted if the connection event was unauthenticated.
	User       *User  `json:"user"`
	SlugOrPort string `json:"slug_or_port"`
	// StatusCode is the HTTP status code or tunnel authorization outcome.
	StatusCode int32 `json:"status_code"`
}

// ConnectionLogDurationStatus describes how a duration was determined.
// It does not indicate whether the connection is active.
type ConnectionLogDurationStatus string

const (
	ConnectionLogDurationRecorded       ConnectionLogDurationStatus = "recorded"
	ConnectionLogDurationUnverified     ConnectionLogDurationStatus = "unverified"
	ConnectionLogDurationNoDisconnect   ConnectionLogDurationStatus = "no_disconnect"
	ConnectionLogDurationMissingConnect ConnectionLogDurationStatus = "missing_connect"
	ConnectionLogDurationInvalid        ConnectionLogDurationStatus = "invalid"
)

type ConnectionLogSSHInfo struct {
	ConnectionID uuid.UUID `json:"connection_id" format:"uuid"`
	// ClientSessionID is an optional client-supplied correlation label, not a
	// unique IDE-instance identifier. Related rows may overlap or have gaps.
	ClientSessionID string `json:"client_session_id,omitempty"`
	// DurationSeconds is the stored connect-to-disconnect interval, including
	// unverified intervals labeled by DurationStatus. It is omitted when an
	// endpoint is missing or the interval is invalid.
	DurationSeconds *float64                    `json:"duration_seconds,omitempty"`
	DurationStatus  ConnectionLogDurationStatus `json:"duration_status"`
	// DisconnectTime is omitted if a disconnect event with the same connection ID
	// has not yet been seen.
	DisconnectTime *time.Time `json:"disconnect_time,omitempty" format:"date-time"`
	// DisconnectReason is omitted if a disconnect event with the same connection ID
	// has not yet been seen.
	DisconnectReason string `json:"disconnect_reason,omitempty"`
	// ExitCode is the exit code of the SSH session. It is omitted if a
	// disconnect event with the same connection ID has not yet been seen.
	ExitCode *int32 `json:"exit_code,omitempty"`
}

type ConnectionLogsRequest struct {
	SearchQuery string `json:"q,omitempty"`
	Pagination
}

type ConnectionLogResponse struct {
	ConnectionLogs []ConnectionLog `json:"connection_logs"`
	Count          int64           `json:"count"`
	CountCap       int64           `json:"count_cap"`
}

func (c *Client) ConnectionLogs(ctx context.Context, req ConnectionLogsRequest) (ConnectionLogResponse, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/connectionlog", nil, req.asRequestOption(), func(r *http.Request) {
		q := r.URL.Query()
		var params []string
		if req.SearchQuery != "" {
			params = append(params, req.SearchQuery)
		}
		q.Set("q", strings.Join(params, " "))
		r.URL.RawQuery = q.Encode()
	})
	if err != nil {
		return ConnectionLogResponse{}, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return ConnectionLogResponse{}, ReadBodyAsError(res)
	}

	var logRes ConnectionLogResponse
	err = ReadBodyAsJSON(res, &logRes)
	if err != nil {
		return ConnectionLogResponse{}, err
	}
	return logRes, nil
}

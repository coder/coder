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
	// Type is the recorded type. For an agent-reported connection it is the
	// app that connected, so unlike ConnectionType the set is open.
	Type string `json:"type"`
	// TypeDisplayName is how to present `type`, such as "VS Code".
	TypeDisplayName string `json:"type_display_name"`

	// WebInfo is only set when `type` is one of:
	// - `ConnectionTypePortForwarding`
	// - `ConnectionTypeWorkspaceApp`
	// - `ConnectionTypeTunnel`
	WebInfo *ConnectionLogWebInfo `json:"web_info,omitempty"`

	// SSHInfo is set for every other `type`, all agent-reported.
	SSHInfo *ConnectionLogSSHInfo `json:"ssh_info,omitempty"`
}

// ConnectionType is a value the connection log can be filtered by. The
// recorded type itself is open, since an agent reports the app that
// connected.
type ConnectionType string

const (
	// Families, which also match their apps: `vscode` finds Cursor.
	ConnectionTypeSSH             = ConnectionType(AppFamilySSH)
	ConnectionTypeVSCode          = ConnectionType(AppFamilyVSCode)
	ConnectionTypeJetBrains       = ConnectionType(AppFamilyJetBrains)
	ConnectionTypeReconnectingPTY = ConnectionType(AppFamilyReconnectingPTY)

	// Recorded by coderd from an HTTP request, so not apps.
	ConnectionTypeWorkspaceApp   ConnectionType = "workspace_app"
	ConnectionTypePortForwarding ConnectionType = "port_forwarding"
	// ConnectionTypeTunnel records accepted and denied tailnet tunnel
	// requests made by authenticated users.
	ConnectionTypeTunnel ConnectionType = "tunnel"
)

// Valid reports whether t is filterable, not what the column accepts.
func (t ConnectionType) Valid() bool {
	switch t {
	case ConnectionTypeSSH, ConnectionTypeVSCode,
		ConnectionTypeJetBrains, ConnectionTypeReconnectingPTY:
		return true
	}
	_, ok := webTypeNames[t]
	return ok
}

// webTypeNames names the types that are not apps.
var webTypeNames = map[ConnectionType]string{
	ConnectionTypeWorkspaceApp:   "Workspace App",
	ConnectionTypePortForwarding: "Port Forwarding",
	ConnectionTypeTunnel:         "Tunnel",
}

// DisplayName presents t, such as "VS Code", or t itself if unrecognized.
func (t ConnectionType) DisplayName() string {
	if name, ok := webTypeNames[t]; ok {
		return name
	}
	return AppDisplayName(string(t))
}

// MatchingTypes lists the types a filter on t matches: a family also matches
// its apps. An empty t filters nothing.
func (t ConnectionType) MatchingTypes() []string {
	if t == "" {
		return nil
	}
	return AppNamesInFamily(AppFamilyName(t))
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

type ConnectionLogSSHInfo struct {
	ConnectionID uuid.UUID `json:"connection_id" format:"uuid"`
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

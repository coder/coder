package codersdk

import (
	"context"
	"net/http"
	"net/netip"
	"slices"
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
	// AppName identifies the originating client, when known. Web destinations
	// are reported separately in WebInfo.
	AppName string `json:"app_name,omitempty"`
	// AppDisplayName is the registry display name for a known client identity,
	// or its normalized identifier when unregistered.
	AppDisplayName string `json:"app_display_name,omitempty"`

	// WebInfo is set for server-recorded workspace apps, port forwards and tunnels.
	WebInfo *ConnectionLogWebInfo `json:"web_info,omitempty"`

	// SSHInfo is set for SSH and reconnecting PTY connections.
	SSHInfo *ConnectionLogSSHInfo `json:"ssh_info,omitempty"`
}

// ConnectionLogMethod identifies how a connection was established.
type ConnectionLogMethod string

const (
	ConnectionLogMethodSSH             ConnectionLogMethod = "ssh"
	ConnectionLogMethodReconnectingPTY ConnectionLogMethod = "reconnecting_pty"
	ConnectionLogMethodWorkspaceApp    ConnectionLogMethod = "workspace_app"
	ConnectionLogMethodPortForwarding  ConnectionLogMethod = "port_forwarding"
	ConnectionLogMethodTunnel          ConnectionLogMethod = "tunnel"
)

// Valid reports whether m is a supported connection-log method.
func (m ConnectionLogMethod) Valid() bool {
	switch m {
	case ConnectionLogMethodSSH, ConnectionLogMethodReconnectingPTY,
		ConnectionLogMethodWorkspaceApp, ConnectionLogMethodPortForwarding,
		ConnectionLogMethodTunnel:
		return true
	default:
		return false
	}
}

// ConnectionType lists compatibility categories for the deprecated type filter.
type ConnectionType string

const (
	// Compatibility categories retained for the deprecated type filter.
	ConnectionTypeSSH             = ConnectionType(AppFamilySSH)
	ConnectionTypeVSCode          = ConnectionType(AppFamilyVSCode)
	ConnectionTypeJetBrains       = ConnectionType(AppFamilyJetBrains)
	ConnectionTypeReconnectingPTY = ConnectionType(AppFamilyReconnectingPTY)

	// Web connection types.
	ConnectionTypeWorkspaceApp   ConnectionType = "workspace_app"
	ConnectionTypePortForwarding ConnectionType = "port_forwarding"
	// ConnectionTypeTunnel records accepted and denied tailnet tunnel
	// requests made by authenticated users.
	ConnectionTypeTunnel ConnectionType = "tunnel"
)

var webConnectionTypeNames = map[ConnectionType]string{
	ConnectionTypeWorkspaceApp:   "Workspace App",
	ConnectionTypePortForwarding: "Port Forwarding",
	ConnectionTypeTunnel:         "Tunnel",
}

// ConnectionTypeOfApp returns the deprecated compatibility type for an SSH app.
func ConnectionTypeOfApp(appName string) ConnectionType {
	switch AppNameFamily(appName) {
	case AppFamilyVSCode:
		return ConnectionTypeVSCode
	case AppFamilyJetBrains:
		return ConnectionTypeJetBrains
	default:
		return ConnectionTypeSSH
	}
}

// FilterableConnectionTypes lists the values the deprecated type filter accepts.
func FilterableConnectionTypes() []ConnectionType {
	types := []ConnectionType{
		ConnectionTypeSSH, ConnectionTypeVSCode,
		ConnectionTypeJetBrains, ConnectionTypeReconnectingPTY,
	}
	for t := range webConnectionTypeNames {
		types = append(types, t)
	}

	slices.Sort(types)
	return slices.Compact(types)
}

func (t ConnectionType) Valid() bool {
	return slices.Contains(FilterableConnectionTypes(), t)
}

// Reports whether coderd, not an agent, logs connections of type t.
func (t ConnectionType) IsWeb() bool {
	_, ok := webConnectionTypeNames[t]
	return ok
}

// Returns the human-readable name of t.
func (t ConnectionType) DisplayName() string {
	switch {
	case t.IsWeb():
		return webConnectionTypeNames[t]
	// Names the family apart from the VS Code app.
	case t == ConnectionTypeVSCode:
		return "VS Code Family"
	default:
		return AppDisplayName(string(t))
	}
}

// AppNames lists the registered SSH apps with compatibility type t, sorted.
func (t ConnectionType) AppNames() []string {
	var names []string
	for appName := range sessionApps {
		if ConnectionTypeOfApp(appName) == t {
			names = append(names, appName)
		}
	}
	slices.Sort(names)
	return names
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

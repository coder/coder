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
	Type                   ConnectionType      `json:"type"`
	// AppName is the agent-reported app, such as "cursor", or a workspace app
	// slug. Empty for port forwarding and tunnels.
	AppName        string `json:"app_name"`
	AppDisplayName string `json:"app_display_name"`

	// WebInfo is only set when `type` is one of:
	// - `ConnectionTypePortForwarding`
	// - `ConnectionTypeWorkspaceApp`
	// - `ConnectionTypeTunnel`
	WebInfo *ConnectionLogWebInfo `json:"web_info,omitempty"`

	// SSHInfo is set for every other `type`.
	SSHInfo *ConnectionLogSSHInfo `json:"ssh_info,omitempty"`
}

// ConnectionType groups connection logs and is the `type` filter value: the
// app family for agent connections, such as "vscode" for Cursor, otherwise
// the web connection type.
type ConnectionType string

const (
	// App families, one per registry family.
	ConnectionTypeSSH             = ConnectionType(AppFamilySSH)
	ConnectionTypeVSCode          = ConnectionType(AppFamilyVSCode)
	ConnectionTypeJetBrains       = ConnectionType(AppFamilyJetBrains)
	ConnectionTypeReconnectingPTY = ConnectionType(AppFamilyReconnectingPTY)
	ConnectionTypeUnknown         = ConnectionType(AppFamilyUnknown)

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

// ConnectionTypeOfApp returns the type of an agent-reported app.
func ConnectionTypeOfApp(appName string) ConnectionType {
	family := AppNameFamily(appName)
	// Only usage tracking records sftp.
	if family == AppFamilySFTP {
		return ConnectionTypeUnknown
	}
	return ConnectionType(family)
}

// FilterableConnectionTypes lists the values the `type` filter accepts.
func FilterableConnectionTypes() []ConnectionType {
	types := []ConnectionType{ConnectionTypeUnknown}
	for t := range webConnectionTypeNames {
		types = append(types, t)
	}
	for appName := range sessionApps {
		types = append(types, ConnectionTypeOfApp(appName))
	}
	slices.Sort(types)
	return slices.Compact(types)
}

// IsWeb reports whether coderd, not an agent, logs connections of type t.
func (t ConnectionType) IsWeb() bool {
	_, ok := webConnectionTypeNames[t]
	return ok
}

// DisplayName returns the human-readable name of t.
func (t ConnectionType) DisplayName() string {
	switch {
	case t == ConnectionTypeUnknown:
		return "Unknown"
	case t.IsWeb():
		return webConnectionTypeNames[t]
	// The family covers every fork, so it gets the full name.
	case t == ConnectionTypeVSCode:
		return TemplateBuiltinAppDisplayNameVSCode
	default:
		return AppDisplayName(string(t))
	}
}

// AppNames lists the registered apps of type t, sorted. It is empty for
// ConnectionTypeUnknown, which matches unregistered apps.
func (t ConnectionType) AppNames() []string {
	var names []string
	for appName := range sessionApps {
		if t != ConnectionTypeUnknown && ConnectionTypeOfApp(appName) == t {
			names = append(names, appName)
		}
	}
	slices.Sort(names)
	return names
}

// KnownConnectionAppNames lists the registered apps that `type:unknown`
// excludes.
func KnownConnectionAppNames() []string {
	var names []string
	for _, t := range FilterableConnectionTypes() {
		names = append(names, t.AppNames()...)
	}
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

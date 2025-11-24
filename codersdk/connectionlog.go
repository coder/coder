package codersdk

import (
	"context"
	"encoding/json"
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
	Type                   ConnectionType      `json:"type"`

	// WebInfo is only set when `type` is one of:
	// - `ConnectionTypePortForwarding`
	// - `ConnectionTypeWorkspaceApp`
	WebInfo *ConnectionLogWebInfo `json:"web_info,omitempty"`

	// SSHInfo is only set when `type` is one of:
	// - `ConnectionTypeSSH`
	// - `ConnectionTypeReconnectingPTY`
	// - `ConnectionTypeVSCode`
	// - `ConnectionTypeJetBrains`
	SSHInfo *ConnectionLogSSHInfo `json:"ssh_info,omitempty"`
}

// ConnectionType is the type of connection that the agent is receiving.
type ConnectionType string

const (
	ConnectionTypeSSH             ConnectionType = "ssh"
	ConnectionTypeVSCode          ConnectionType = "vscode"
	ConnectionTypeJetBrains       ConnectionType = "jetbrains"
	ConnectionTypeReconnectingPTY ConnectionType = "reconnecting_pty"
	ConnectionTypeWorkspaceApp    ConnectionType = "workspace_app"
	ConnectionTypePortForwarding  ConnectionType = "port_forwarding"
)

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
	// User is omitted if the connection event was from an unauthenticated user.
	User       *User  `json:"user"`
	SlugOrPort string `json:"slug_or_port"`
	// StatusCode is the HTTP status code of the request.
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
}

func (c *Client) ConnectionLogs(ctx context.Context, req ConnectionLogsRequest) (ConnectionLogResponse, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/connectionlog", nil, req.Pagination.asRequestOption(), func(r *http.Request) {
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
	err = json.NewDecoder(res.Body).Decode(&logRes)
	if err != nil {
		return ConnectionLogResponse{}, err
	}
	return logRes, nil
}

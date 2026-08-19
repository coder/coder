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
	Type                   ConnectionType      `json:"type"`

	// WebInfo is only set when `type` is one of:
	// - `ConnectionTypePortForwarding`
	// - `ConnectionTypeWorkspaceApp`
	// - `ConnectionTypeTunnel`
	WebInfo *ConnectionLogWebInfo `json:"web_info,omitempty"`

	// SSHInfo is only set when `type` is one of:
	// - `ConnectionTypeSSH`
	// - `ConnectionTypeReconnectingPTY`
	// - `ConnectionTypeVSCode`
	// - `ConnectionTypeJetBrains`
	// - `ConnectionTypeFileTransfer`
	SSHInfo *ConnectionLogSSHInfo `json:"ssh_info,omitempty"`

	// FileTransferInfo is only set when `type` is
	// `ConnectionTypeFileOperation`.
	FileTransferInfo *ConnectionLogFileTransferInfo `json:"file_transfer_info,omitempty"`
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
	// ConnectionTypeTunnel records accepted and denied tailnet tunnel
	// requests made by authenticated users.
	ConnectionTypeTunnel ConnectionType = "tunnel"
	// ConnectionTypeFileTransfer records file-transfer sessions (SFTP,
	// SCP, rsync), which would otherwise be recorded as plain SSH.
	ConnectionTypeFileTransfer ConnectionType = "file_transfer"
	// ConnectionTypeFileOperation records individual file operations
	// observed during a file-transfer session. Events share the
	// connection ID of their parent file_transfer session.
	ConnectionTypeFileOperation ConnectionType = "file_operation"
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

// ConnectionLogFileProtocol is the protocol that carried a file
// operation.
type ConnectionLogFileProtocol string

const (
	ConnectionLogFileProtocolSFTP  ConnectionLogFileProtocol = "sftp"
	ConnectionLogFileProtocolSCP   ConnectionLogFileProtocol = "scp"
	ConnectionLogFileProtocolRsync ConnectionLogFileProtocol = "rsync"
)

// ConnectionLogFileAction is the kind of file operation observed.
// A download is a transfer from the workspace to the client, an upload
// is a transfer from the client to the workspace, and bidirectional is
// a file opened for both at once, where either may have occurred.
type ConnectionLogFileAction string

const (
	ConnectionLogFileActionDownload      ConnectionLogFileAction = "download"
	ConnectionLogFileActionUpload        ConnectionLogFileAction = "upload"
	ConnectionLogFileActionBidirectional ConnectionLogFileAction = "bidirectional"
	ConnectionLogFileActionRemove        ConnectionLogFileAction = "remove"
	ConnectionLogFileActionRmdir         ConnectionLogFileAction = "rmdir"
	ConnectionLogFileActionRename        ConnectionLogFileAction = "rename"
	ConnectionLogFileActionSymlink       ConnectionLogFileAction = "symlink"
	// ConnectionLogFileActionSetattr records file attribute changes:
	// truncation, permissions, ownership, and timestamps.
	ConnectionLogFileActionSetattr ConnectionLogFileAction = "setattr"
	// ConnectionLogFileActionHardlink records creation of a hard link.
	// Path is the existing file, Target is the new link.
	ConnectionLogFileActionHardlink ConnectionLogFileAction = "hardlink"
)

type ConnectionLogFileTransferInfo struct {
	// ConnectionID matches the connection ID of the file_transfer
	// session the operation occurred in.
	ConnectionID uuid.UUID                 `json:"connection_id" format:"uuid"`
	Protocol     ConnectionLogFileProtocol `json:"protocol"`
	Action       ConnectionLogFileAction   `json:"action"`
	// Path is the path the operation was performed on. For SCP and
	// rsync this is the requested root path from the command line, not
	// necessarily every file transferred.
	Path string `json:"path"`
	// Target is only set for operations with a second path, such as the
	// destination of a rename or the target of a symlink.
	Target string `json:"target,omitempty"`
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
	err = ReadBodyAsJSON(res, &logRes)
	if err != nil {
		return ConnectionLogResponse{}, err
	}
	return logRes, nil
}

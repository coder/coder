package codersdk

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

// BoundaryNetworkAction represents whether a network request was allowed or denied.
type BoundaryNetworkAction string

const (
	BoundaryNetworkActionAllow BoundaryNetworkAction = "allow"
	BoundaryNetworkActionDeny  BoundaryNetworkAction = "deny"
)

func (a BoundaryNetworkAction) Valid() bool {
	switch a {
	case BoundaryNetworkActionAllow, BoundaryNetworkActionDeny:
		return true
	default:
		return false
	}
}

// BoundaryNetworkAuditLog represents a single boundary network audit log entry.
type BoundaryNetworkAuditLog struct {
	ID                     uuid.UUID             `json:"id" format:"uuid"`
	Time                   time.Time             `json:"time" format:"date-time"`
	Organization           MinimalOrganization   `json:"organization"`
	WorkspaceID            uuid.UUID             `json:"workspace_id" format:"uuid"`
	WorkspaceOwnerID       uuid.UUID             `json:"workspace_owner_id" format:"uuid"`
	WorkspaceOwnerUsername string                `json:"workspace_owner_username"`
	WorkspaceName          string                `json:"workspace_name"`
	AgentID                uuid.UUID             `json:"agent_id" format:"uuid"`
	AgentName              string                `json:"agent_name"`
	Domain                 string                `json:"domain"`
	Action                 BoundaryNetworkAction `json:"action"`
}

type BoundaryNetworkAuditLogsRequest struct {
	SearchQuery string `json:"q,omitempty"`
	Pagination
}

type BoundaryNetworkAuditLogResponse struct {
	Logs  []BoundaryNetworkAuditLog `json:"logs"`
	Count int64                     `json:"count"`
}

func (c *Client) BoundaryNetworkAuditLogs(ctx context.Context, req BoundaryNetworkAuditLogsRequest) (BoundaryNetworkAuditLogResponse, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/boundary-network-audit-logs", nil, req.Pagination.asRequestOption(), func(r *http.Request) {
		q := r.URL.Query()
		var params []string
		if req.SearchQuery != "" {
			params = append(params, req.SearchQuery)
		}
		q.Set("q", strings.Join(params, " "))
		r.URL.RawQuery = q.Encode()
	})
	if err != nil {
		return BoundaryNetworkAuditLogResponse{}, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return BoundaryNetworkAuditLogResponse{}, ReadBodyAsError(res)
	}

	var logRes BoundaryNetworkAuditLogResponse
	err = json.NewDecoder(res.Body).Decode(&logRes)
	if err != nil {
		return BoundaryNetworkAuditLogResponse{}, err
	}
	return logRes, nil
}

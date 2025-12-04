package codersdk

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

// BoundaryAuditDecision represents whether a resource access was allowed or denied.
type BoundaryAuditDecision string

const (
	BoundaryAuditDecisionAllow BoundaryAuditDecision = "allow"
	BoundaryAuditDecisionDeny  BoundaryAuditDecision = "deny"
)

func (d BoundaryAuditDecision) Valid() bool {
	switch d {
	case BoundaryAuditDecisionAllow, BoundaryAuditDecisionDeny:
		return true
	default:
		return false
	}
}

// BoundaryAuditLog represents a single boundary audit log entry.
type BoundaryAuditLog struct {
	ID                     uuid.UUID             `json:"id" format:"uuid"`
	Time                   time.Time             `json:"time" format:"date-time"`
	Organization           MinimalOrganization   `json:"organization"`
	WorkspaceID            uuid.UUID             `json:"workspace_id" format:"uuid"`
	WorkspaceOwnerID       uuid.UUID             `json:"workspace_owner_id" format:"uuid"`
	WorkspaceOwnerUsername string                `json:"workspace_owner_username"`
	WorkspaceName          string                `json:"workspace_name"`
	AgentID                uuid.UUID             `json:"agent_id" format:"uuid"`
	AgentName              string                `json:"agent_name"`
	ResourceType           string                `json:"resource_type"`
	Resource               string                `json:"resource"`
	Operation              string                `json:"operation"`
	Decision               BoundaryAuditDecision `json:"decision"`
}

type BoundaryAuditLogsRequest struct {
	SearchQuery string `json:"q,omitempty"`
	Pagination
}

type BoundaryAuditLogResponse struct {
	Logs  []BoundaryAuditLog `json:"logs"`
	Count int64                     `json:"count"`
}

func (c *Client) BoundaryAuditLogs(ctx context.Context, req BoundaryAuditLogsRequest) (BoundaryAuditLogResponse, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/boundary-audit-logs", nil, req.Pagination.asRequestOption(), func(r *http.Request) {
		q := r.URL.Query()
		var params []string
		if req.SearchQuery != "" {
			params = append(params, req.SearchQuery)
		}
		q.Set("q", strings.Join(params, " "))
		r.URL.RawQuery = q.Encode()
	})
	if err != nil {
		return BoundaryAuditLogResponse{}, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return BoundaryAuditLogResponse{}, ReadBodyAsError(res)
	}

	var logRes BoundaryAuditLogResponse
	err = json.NewDecoder(res.Body).Decode(&logRes)
	if err != nil {
		return BoundaryAuditLogResponse{}, err
	}
	return logRes, nil
}

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

type ResourceType string

const (
	ResourceTypeOrganization    ResourceType = "organization"
	ResourceTypeTemplate        ResourceType = "template"
	ResourceTypeTemplateVersion ResourceType = "template_version"
	ResourceTypeUser            ResourceType = "user"
	ResourceTypeWorkspace       ResourceType = "workspace"
	ResourceTypeWorkspaceBuild  ResourceType = "workspace_build"
	ResourceTypeGitSSHKey       ResourceType = "git_ssh_key"
	ResourceTypeAPIKey          ResourceType = "api_key"
	ResourceTypeGroup           ResourceType = "group"
)

func (r ResourceType) FriendlyString() string {
	switch r {
	case ResourceTypeOrganization:
		return "organization"
	case ResourceTypeTemplate:
		return "template"
	case ResourceTypeTemplateVersion:
		return "template version"
	case ResourceTypeUser:
		return "user"
	case ResourceTypeWorkspace:
		return "workspace"
	case ResourceTypeWorkspaceBuild:
		// workspace builds have a unique friendly string
		// see coderd/audit.go:298 for explanation
		return "workspace"
	case ResourceTypeGitSSHKey:
		return "git ssh key"
	case ResourceTypeAPIKey:
		return "api key"
	case ResourceTypeGroup:
		return "group"
	default:
		return "unknown"
	}
}

type AuditAction string

const (
	AuditActionCreate AuditAction = "create"
	AuditActionWrite  AuditAction = "write"
	AuditActionDelete AuditAction = "delete"
	AuditActionStart  AuditAction = "start"
	AuditActionStop   AuditAction = "stop"
)

func (a AuditAction) FriendlyString() string {
	switch a {
	case AuditActionCreate:
		return "created"
	case AuditActionWrite:
		return "updated"
	case AuditActionDelete:
		return "deleted"
	case AuditActionStart:
		return "started"
	case AuditActionStop:
		return "stopped"
	default:
		return "unknown"
	}
}

type AuditDiff map[string]AuditDiffField

type AuditDiffField struct {
	Old    any  `json:"old,omitempty"`
	New    any  `json:"new,omitempty"`
	Secret bool `json:"secret"`
}

type AuditLog struct {
	ID             uuid.UUID    `json:"id"`
	RequestID      uuid.UUID    `json:"request_id"`
	Time           time.Time    `json:"time"`
	OrganizationID uuid.UUID    `json:"organization_id"`
	IP             netip.Addr   `json:"ip"`
	UserAgent      string       `json:"user_agent"`
	ResourceType   ResourceType `json:"resource_type"`
	ResourceID     uuid.UUID    `json:"resource_id"`
	// ResourceTarget is the name of the resource.
	ResourceTarget   string          `json:"resource_target"`
	ResourceIcon     string          `json:"resource_icon"`
	Action           AuditAction     `json:"action"`
	Diff             AuditDiff       `json:"diff"`
	StatusCode       int32           `json:"status_code"`
	AdditionalFields json.RawMessage `json:"additional_fields"`
	Description      string          `json:"description"`
	ResourceLink     string          `json:"resource_link"`
	IsDeleted        bool            `json:"is_deleted"`

	User *User `json:"user"`
}

type AuditLogsRequest struct {
	SearchQuery string `json:"q,omitempty"`
	Pagination
}

type AuditLogResponse struct {
	AuditLogs []AuditLog `json:"audit_logs"`
	Count     int64      `json:"count"`
}

type CreateTestAuditLogRequest struct {
	Action       AuditAction  `json:"action,omitempty" enums:"create,write,delete,start,stop"`
	ResourceType ResourceType `json:"resource_type,omitempty" enums:"organization,template,template_version,user,workspace,workspace_build,git_ssh_key,api_key,group"`
	ResourceID   uuid.UUID    `json:"resource_id,omitempty"`
	Time         time.Time    `json:"time,omitempty"`
}

// AuditLogs retrieves audit logs from the given page.
func (c *Client) AuditLogs(ctx context.Context, req AuditLogsRequest) (AuditLogResponse, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/audit", nil, req.Pagination.asRequestOption(), func(r *http.Request) {
		q := r.URL.Query()
		var params []string
		if req.SearchQuery != "" {
			params = append(params, req.SearchQuery)
		}
		q.Set("q", strings.Join(params, " "))
		r.URL.RawQuery = q.Encode()
	})
	if err != nil {
		return AuditLogResponse{}, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return AuditLogResponse{}, readBodyAsError(res)
	}

	var logRes AuditLogResponse
	err = json.NewDecoder(res.Body).Decode(&logRes)
	if err != nil {
		return AuditLogResponse{}, err
	}

	return logRes, nil
}

func (c *Client) CreateTestAuditLog(ctx context.Context, req CreateTestAuditLogRequest) error {
	res, err := c.Request(ctx, http.MethodPost, "/api/v2/audit/testgenerate", req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusNoContent {
		return err
	}

	return nil
}

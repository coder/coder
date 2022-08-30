package codersdk

import (
	"encoding/json"
	"net/netip"
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
)

type AuditAction string

const (
	AuditActionCreate AuditAction = "create"
	AuditActionWrite  AuditAction = "write"
	AuditActionDelete AuditAction = "delete"
)

type AuditDiff map[string]struct {
	Old    any
	New    any
	Secret bool
}

type AuditLog struct {
	ID               uuid.UUID       `json:"id"`
	RequestID        uuid.UUID       `json:"request_id"`
	Time             time.Time       `json:"time"`
	OrganizationID   uuid.UUID       `json:"organization_id"`
	IP               netip.Addr      `json:"ip"`
	UserAgent        string          `json:"user_agent"`
	ResourceType     ResourceType    `json:"resource_type"`
	ResourceID       uuid.UUID       `json:"resource_id"`
	ResourceTarget   string          `json:"resource_target"`
	Action           AuditAction     `json:"action"`
	Diff             AuditDiff       `json:"diff"`
	StatusCode       int32           `json:"status_code"`
	AdditionalFields json.RawMessage `json:"additional_fields"`
	Description      string          `json:"description"`

	User     *User           `json:"user"`
	Resource json.RawMessage `json:"resource"`
}

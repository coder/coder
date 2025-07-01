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
	ResourceTypeTemplate              ResourceType = "template"
	ResourceTypeTemplateVersion       ResourceType = "template_version"
	ResourceTypeUser                  ResourceType = "user"
	ResourceTypeWorkspace             ResourceType = "workspace"
	ResourceTypeWorkspaceBuild        ResourceType = "workspace_build"
	ResourceTypeGitSSHKey             ResourceType = "git_ssh_key"
	ResourceTypeAPIKey                ResourceType = "api_key"
	ResourceTypeGroup                 ResourceType = "group"
	ResourceTypeLicense               ResourceType = "license"
	ResourceTypeConvertLogin          ResourceType = "convert_login"
	ResourceTypeHealthSettings        ResourceType = "health_settings"
	ResourceTypeNotificationsSettings ResourceType = "notifications_settings"
	ResourceTypeWorkspaceProxy        ResourceType = "workspace_proxy"
	ResourceTypeOrganization          ResourceType = "organization"
	ResourceTypeOAuth2ProviderApp     ResourceType = "oauth2_provider_app"
	// nolint:gosec // This is not a secret.
	ResourceTypeOAuth2ProviderAppSecret     ResourceType = "oauth2_provider_app_secret"
	ResourceTypeCustomRole                  ResourceType = "custom_role"
	ResourceTypeOrganizationMember          ResourceType = "organization_member"
	ResourceTypeNotificationTemplate        ResourceType = "notification_template"
	ResourceTypeIdpSyncSettingsOrganization ResourceType = "idp_sync_settings_organization"
	ResourceTypeIdpSyncSettingsGroup        ResourceType = "idp_sync_settings_group"
	ResourceTypeIdpSyncSettingsRole         ResourceType = "idp_sync_settings_role"
	ResourceTypeWorkspaceAgent              ResourceType = "workspace_agent"
	ResourceTypeWorkspaceApp                ResourceType = "workspace_app"
)

func (r ResourceType) FriendlyString() string {
	switch r {
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
		return "token"
	case ResourceTypeGroup:
		return "group"
	case ResourceTypeLicense:
		return "license"
	case ResourceTypeConvertLogin:
		return "login type conversion"
	case ResourceTypeWorkspaceProxy:
		return "workspace proxy"
	case ResourceTypeOrganization:
		return "organization"
	case ResourceTypeHealthSettings:
		return "health_settings"
	case ResourceTypeNotificationsSettings:
		return "notifications_settings"
	case ResourceTypeOAuth2ProviderApp:
		return "oauth2 app"
	case ResourceTypeOAuth2ProviderAppSecret:
		return "oauth2 app secret"
	case ResourceTypeCustomRole:
		return "custom role"
	case ResourceTypeOrganizationMember:
		return "organization member"
	case ResourceTypeNotificationTemplate:
		return "notification template"
	case ResourceTypeIdpSyncSettingsOrganization:
		return "settings"
	case ResourceTypeIdpSyncSettingsGroup:
		return "settings"
	case ResourceTypeIdpSyncSettingsRole:
		return "settings"
	case ResourceTypeWorkspaceAgent:
		return "workspace agent"
	case ResourceTypeWorkspaceApp:
		return "workspace app"
	default:
		return "unknown"
	}
}

type AuditAction string

const (
	AuditActionCreate               AuditAction = "create"
	AuditActionWrite                AuditAction = "write"
	AuditActionDelete               AuditAction = "delete"
	AuditActionStart                AuditAction = "start"
	AuditActionStop                 AuditAction = "stop"
	AuditActionLogin                AuditAction = "login"
	AuditActionLogout               AuditAction = "logout"
	AuditActionRegister             AuditAction = "register"
	AuditActionRequestPasswordReset AuditAction = "request_password_reset"
	AuditActionConnect              AuditAction = "connect"
	AuditActionDisconnect           AuditAction = "disconnect"
	AuditActionOpen                 AuditAction = "open"
	AuditActionClose                AuditAction = "close"
)

func (a AuditAction) Friendly() string {
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
	case AuditActionLogin:
		return "logged in"
	case AuditActionLogout:
		return "logged out"
	case AuditActionRegister:
		return "registered"
	case AuditActionRequestPasswordReset:
		return "password reset requested"
	case AuditActionConnect:
		return "connected"
	case AuditActionDisconnect:
		return "disconnected"
	case AuditActionOpen:
		return "opened"
	case AuditActionClose:
		return "closed"
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
	ID           uuid.UUID    `json:"id" format:"uuid"`
	RequestID    uuid.UUID    `json:"request_id" format:"uuid"`
	Time         time.Time    `json:"time" format:"date-time"`
	IP           netip.Addr   `json:"ip"`
	UserAgent    string       `json:"user_agent"`
	ResourceType ResourceType `json:"resource_type"`
	ResourceID   uuid.UUID    `json:"resource_id" format:"uuid"`
	// ResourceTarget is the name of the resource.
	ResourceTarget   string          `json:"resource_target"`
	ResourceIcon     string          `json:"resource_icon"`
	Action           AuditAction     `json:"action"`
	Diff             AuditDiff       `json:"diff"`
	StatusCode       int32           `json:"status_code"`
	AdditionalFields json.RawMessage `json:"additional_fields" swaggertype:"object"`
	Description      string          `json:"description"`
	ResourceLink     string          `json:"resource_link"`
	IsDeleted        bool            `json:"is_deleted"`

	// Deprecated: Use 'organization.id' instead.
	OrganizationID uuid.UUID `json:"organization_id" format:"uuid"`

	Organization *MinimalOrganization `json:"organization,omitempty"`

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
	Action           AuditAction     `json:"action,omitempty" enums:"create,write,delete,start,stop"`
	ResourceType     ResourceType    `json:"resource_type,omitempty" enums:"template,template_version,user,workspace,workspace_build,git_ssh_key,auditable_group"`
	ResourceID       uuid.UUID       `json:"resource_id,omitempty" format:"uuid"`
	AdditionalFields json.RawMessage `json:"additional_fields,omitempty"`
	Time             time.Time       `json:"time,omitempty" format:"date-time"`
	BuildReason      BuildReason     `json:"build_reason,omitempty" enums:"autostart,autostop,initiator"`
	OrganizationID   uuid.UUID       `json:"organization_id,omitempty" format:"uuid"`
	RequestID        uuid.UUID       `json:"request_id,omitempty" format:"uuid"`
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
		return AuditLogResponse{}, ReadBodyAsError(res)
	}

	var logRes AuditLogResponse
	err = json.NewDecoder(res.Body).Decode(&logRes)
	if err != nil {
		return AuditLogResponse{}, err
	}

	return logRes, nil
}

// CreateTestAuditLog creates a fake audit log. Only owners of the organization
// can perform this action. It's used for testing purposes.
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

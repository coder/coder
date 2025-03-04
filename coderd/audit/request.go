package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"go.opentelemetry.io/otel/baggage"
	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/idpsync"
	"github.com/coder/coder/v2/coderd/tracing"
)

type RequestParams struct {
	Audit Auditor
	Log   slog.Logger

	// OrganizationID is only provided when possible. If an audit resource extends
	// beyond the org scope, leave this as the nil uuid.
	OrganizationID   uuid.UUID
	Request          *http.Request
	Action           database.AuditAction
	AdditionalFields interface{}
}

type Request[T Auditable] struct {
	params *RequestParams

	Old T
	New T

	// UserID is an optional field can be passed in when the userID cannot be
	// determined from the API Key such as in the case of login, when the audit
	// log is created prior the API Key's existence.
	UserID uuid.UUID

	// Action is an optional field can be passed in if the AuditAction must be
	// overridden such as in the case of new user authentication when the Audit
	// Action is 'register', not 'login'.
	Action database.AuditAction
}

// UpdateOrganizationID can be used if the organization ID is not known
// at the initiation of an audit log request.
func (r *Request[T]) UpdateOrganizationID(id uuid.UUID) {
	r.params.OrganizationID = id
}

type BackgroundAuditParams[T Auditable] struct {
	Audit Auditor
	Log   slog.Logger

	UserID         uuid.UUID
	RequestID      uuid.UUID
	Time           time.Time
	Status         int
	Action         database.AuditAction
	OrganizationID uuid.UUID
	IP             string
	UserAgent      string
	// todo: this should automatically marshal an interface{} instead of accepting a raw message.
	AdditionalFields json.RawMessage

	New T
	Old T
}

func ResourceTarget[T Auditable](tgt T) string {
	switch typed := any(tgt).(type) {
	case database.Template:
		return typed.Name
	case database.TemplateVersion:
		return typed.Name
	case database.User:
		return typed.Username
	case database.WorkspaceTable:
		return typed.Name
	case database.WorkspaceBuild:
		// this isn't used
		return ""
	case database.GitSSHKey:
		return typed.PublicKey
	case database.AuditableGroup:
		return typed.Group.Name
	case database.APIKey:
		if typed.TokenName != "nil" {
			return typed.TokenName
		}
		// API Keys without names are used for auth
		// and don't have a target
		return ""
	case database.License:
		return strconv.Itoa(int(typed.ID))
	case database.WorkspaceProxy:
		return typed.Name
	case database.AuditOAuthConvertState:
		return string(typed.ToLoginType)
	case database.HealthSettings:
		return "" // no target?
	case database.NotificationsSettings:
		return "" // no target?
	case database.OAuth2ProviderApp:
		return typed.Name
	case database.OAuth2ProviderAppSecret:
		return typed.DisplaySecret
	case database.CustomRole:
		return typed.Name
	case database.AuditableOrganizationMember:
		return typed.Username
	case database.Organization:
		return typed.Name
	case database.NotificationTemplate:
		return typed.Name
	case idpsync.OrganizationSyncSettings:
		return "Organization Sync"
	case idpsync.GroupSyncSettings:
		return "Organization Group Sync"
	case idpsync.RoleSyncSettings:
		return "Organization Role Sync"
	case database.WorkspaceAgent:
		return typed.Name
	case database.WorkspaceApp:
		return typed.Slug
	default:
		panic(fmt.Sprintf("unknown resource %T for ResourceTarget", tgt))
	}
}

// noID can be used for resources that do not have an uuid.
// An example is singleton configuration resources.
// 51A51C = "Static"
var noID = uuid.MustParse("51A51C00-0000-0000-0000-000000000000")

func ResourceID[T Auditable](tgt T) uuid.UUID {
	switch typed := any(tgt).(type) {
	case database.Template:
		return typed.ID
	case database.TemplateVersion:
		return typed.ID
	case database.User:
		return typed.ID
	case database.WorkspaceTable:
		return typed.ID
	case database.WorkspaceBuild:
		return typed.ID
	case database.GitSSHKey:
		return typed.UserID
	case database.AuditableGroup:
		return typed.Group.ID
	case database.APIKey:
		return typed.UserID
	case database.License:
		return typed.UUID
	case database.WorkspaceProxy:
		return typed.ID
	case database.AuditOAuthConvertState:
		// The merge state is for the given user
		return typed.UserID
	case database.HealthSettings:
		// Artificial ID for auditing purposes
		return typed.ID
	case database.NotificationsSettings:
		// Artificial ID for auditing purposes
		return typed.ID
	case database.OAuth2ProviderApp:
		return typed.ID
	case database.OAuth2ProviderAppSecret:
		return typed.ID
	case database.CustomRole:
		return typed.ID
	case database.AuditableOrganizationMember:
		return typed.UserID
	case database.Organization:
		return typed.ID
	case database.NotificationTemplate:
		return typed.ID
	case idpsync.OrganizationSyncSettings:
		return noID // Deployment all uses the same org sync settings
	case idpsync.GroupSyncSettings:
		return noID // Org field on audit log has org id
	case idpsync.RoleSyncSettings:
		return noID // Org field on audit log has org id
	case database.WorkspaceAgent:
		return typed.ID
	case database.WorkspaceApp:
		return typed.ID
	default:
		panic(fmt.Sprintf("unknown resource %T for ResourceID", tgt))
	}
}

func ResourceType[T Auditable](tgt T) database.ResourceType {
	switch typed := any(tgt).(type) {
	case database.Template:
		return database.ResourceTypeTemplate
	case database.TemplateVersion:
		return database.ResourceTypeTemplateVersion
	case database.User:
		return database.ResourceTypeUser
	case database.WorkspaceTable:
		return database.ResourceTypeWorkspace
	case database.WorkspaceBuild:
		return database.ResourceTypeWorkspaceBuild
	case database.GitSSHKey:
		return database.ResourceTypeGitSshKey
	case database.AuditableGroup:
		return database.ResourceTypeGroup
	case database.APIKey:
		return database.ResourceTypeApiKey
	case database.License:
		return database.ResourceTypeLicense
	case database.WorkspaceProxy:
		return database.ResourceTypeWorkspaceProxy
	case database.AuditOAuthConvertState:
		return database.ResourceTypeConvertLogin
	case database.HealthSettings:
		return database.ResourceTypeHealthSettings
	case database.NotificationsSettings:
		return database.ResourceTypeNotificationsSettings
	case database.OAuth2ProviderApp:
		return database.ResourceTypeOauth2ProviderApp
	case database.OAuth2ProviderAppSecret:
		return database.ResourceTypeOauth2ProviderAppSecret
	case database.CustomRole:
		return database.ResourceTypeCustomRole
	case database.AuditableOrganizationMember:
		return database.ResourceTypeOrganizationMember
	case database.Organization:
		return database.ResourceTypeOrganization
	case database.NotificationTemplate:
		return database.ResourceTypeNotificationTemplate
	case idpsync.OrganizationSyncSettings:
		return database.ResourceTypeIdpSyncSettingsOrganization
	case idpsync.RoleSyncSettings:
		return database.ResourceTypeIdpSyncSettingsRole
	case idpsync.GroupSyncSettings:
		return database.ResourceTypeIdpSyncSettingsGroup
	case database.WorkspaceAgent:
		return database.ResourceTypeWorkspaceAgent
	case database.WorkspaceApp:
		return database.ResourceTypeWorkspaceApp
	default:
		panic(fmt.Sprintf("unknown resource %T for ResourceType", typed))
	}
}

// ResourceRequiresOrgID will ensure given resources are always audited with an
// organization ID.
func ResourceRequiresOrgID[T Auditable]() bool {
	var tgt T
	switch any(tgt).(type) {
	case database.Template, database.TemplateVersion:
		return true
	case database.WorkspaceTable, database.WorkspaceBuild:
		return true
	case database.AuditableGroup:
		return true
	case database.User:
		return false
	case database.GitSSHKey:
		return false
	case database.APIKey:
		return false
	case database.License:
		return false
	case database.WorkspaceProxy:
		return false
	case database.AuditOAuthConvertState:
		// The merge state is for the given user
		return false
	case database.HealthSettings:
		// Artificial ID for auditing purposes
		return false
	case database.NotificationsSettings:
		// Artificial ID for auditing purposes
		return false
	case database.OAuth2ProviderApp:
		return false
	case database.OAuth2ProviderAppSecret:
		return false
	case database.CustomRole:
		return true
	case database.AuditableOrganizationMember:
		return true
	case database.Organization:
		return true
	case database.NotificationTemplate:
		return false
	case idpsync.OrganizationSyncSettings:
		return false
	case idpsync.GroupSyncSettings:
		return true
	case idpsync.RoleSyncSettings:
		return true
	case database.WorkspaceAgent:
		return true
	case database.WorkspaceApp:
		return true
	default:
		panic(fmt.Sprintf("unknown resource %T for ResourceRequiresOrgID", tgt))
	}
}

// requireOrgID will either panic (in unit tests) or log an error (in production)
// if the given resource requires an organization ID and the provided ID is nil.
func requireOrgID[T Auditable](ctx context.Context, id uuid.UUID, log slog.Logger) uuid.UUID {
	if ResourceRequiresOrgID[T]() && id == uuid.Nil {
		var tgt T
		resourceName := fmt.Sprintf("%T", tgt)
		if flag.Lookup("test.v") != nil {
			// In unit tests we panic to fail the tests
			panic(fmt.Sprintf("missing required organization ID for resource %q", resourceName))
		}
		log.Error(ctx, "missing required organization ID for resource in audit log",
			slog.F("resource", resourceName),
		)
	}
	return id
}

// InitRequestWithCancel returns a commit function with a boolean arg.
// If the arg is false, future calls to commit() will not create an audit log
// entry.
func InitRequestWithCancel[T Auditable](w http.ResponseWriter, p *RequestParams) (*Request[T], func(commit bool)) {
	req, commitF := InitRequest[T](w, p)
	canceled := false
	return req, func(commit bool) {
		// Once 'commit=false' is called, block
		// any future commit attempts.
		if !commit {
			canceled = true
			return
		}
		// If it was ever canceled, block any commits
		if !canceled {
			commitF()
		}
	}
}

// InitRequest initializes an audit log for a request. It returns a function
// that should be deferred, causing the audit log to be committed when the
// handler returns.
func InitRequest[T Auditable](w http.ResponseWriter, p *RequestParams) (*Request[T], func()) {
	sw, ok := w.(*tracing.StatusWriter)
	if !ok {
		panic("dev error: http.ResponseWriter is not *tracing.StatusWriter")
	}

	req := &Request[T]{
		params: p,
	}

	return req, func() {
		ctx := context.Background()
		logCtx := p.Request.Context()

		// If no resources were provided, there's nothing we can audit.
		if ResourceID(req.Old) == uuid.Nil && ResourceID(req.New) == uuid.Nil {
			// If the request action is a login or logout, we always want to audit it even if
			// there is no diff. This is so we can capture events where an API Key is never created
			// because a known user fails to login.
			if req.params.Action != database.AuditActionLogin && req.params.Action != database.AuditActionLogout {
				return
			}
		}

		diffRaw := []byte("{}")
		// Only generate diffs if the request succeeded
		// and only if we aren't auditing authentication actions
		if sw.Status < 400 &&
			req.params.Action != database.AuditActionLogin && req.params.Action != database.AuditActionLogout {
			diff := Diff(p.Audit, req.Old, req.New)

			var err error
			diffRaw, err = json.Marshal(diff)
			if err != nil {
				p.Log.Warn(logCtx, "marshal diff", slog.Error(err))
				diffRaw = []byte("{}")
			}
		}

		additionalFieldsRaw := json.RawMessage("{}")

		if p.AdditionalFields != nil {
			data, err := json.Marshal(p.AdditionalFields)
			if err != nil {
				p.Log.Warn(logCtx, "marshal additional fields", slog.Error(err))
			} else {
				additionalFieldsRaw = json.RawMessage(data)
			}
		}

		var userID uuid.UUID
		key, ok := httpmw.APIKeyOptional(p.Request)
		if ok {
			userID = key.UserID
		} else if req.UserID != uuid.Nil {
			userID = req.UserID
		} else {
			// if we do not have a user associated with the audit action
			// we do not want to audit
			// (this pertains to logins; we don't want to capture non-user login attempts)
			return
		}

		action := p.Action
		if req.Action != "" {
			action = req.Action
		}

		ip := ParseIP(p.Request.RemoteAddr)
		auditLog := database.AuditLog{
			ID:               uuid.New(),
			Time:             dbtime.Now(),
			UserID:           userID,
			Ip:               ip,
			UserAgent:        sql.NullString{String: p.Request.UserAgent(), Valid: true},
			ResourceType:     either(req.Old, req.New, ResourceType[T], req.params.Action),
			ResourceID:       either(req.Old, req.New, ResourceID[T], req.params.Action),
			ResourceTarget:   either(req.Old, req.New, ResourceTarget[T], req.params.Action),
			Action:           action,
			Diff:             diffRaw,
			StatusCode:       int32(sw.Status),
			RequestID:        httpmw.RequestID(p.Request),
			AdditionalFields: additionalFieldsRaw,
			OrganizationID:   requireOrgID[T](logCtx, p.OrganizationID, p.Log),
		}
		err := p.Audit.Export(ctx, auditLog)
		if err != nil {
			p.Log.Error(logCtx, "export audit log",
				slog.F("audit_log", auditLog),
				slog.Error(err),
			)
			return
		}
	}
}

// BackgroundAudit creates an audit log for a background event.
// The audit log is committed upon invocation.
func BackgroundAudit[T Auditable](ctx context.Context, p *BackgroundAuditParams[T]) {
	ip := ParseIP(p.IP)

	diff := Diff(p.Audit, p.Old, p.New)
	var err error
	diffRaw, err := json.Marshal(diff)
	if err != nil {
		p.Log.Warn(ctx, "marshal diff", slog.Error(err))
		diffRaw = []byte("{}")
	}

	if p.Time.IsZero() {
		p.Time = dbtime.Now()
	} else {
		// NOTE(mafredri): dbtime.Time does not currently enforce UTC.
		p.Time = dbtime.Time(p.Time.In(time.UTC))
	}
	if p.AdditionalFields == nil {
		p.AdditionalFields = json.RawMessage("{}")
	}

	auditLog := database.AuditLog{
		ID:               uuid.New(),
		Time:             p.Time,
		UserID:           p.UserID,
		OrganizationID:   requireOrgID[T](ctx, p.OrganizationID, p.Log),
		Ip:               ip,
		UserAgent:        sql.NullString{Valid: p.UserAgent != "", String: p.UserAgent},
		ResourceType:     either(p.Old, p.New, ResourceType[T], p.Action),
		ResourceID:       either(p.Old, p.New, ResourceID[T], p.Action),
		ResourceTarget:   either(p.Old, p.New, ResourceTarget[T], p.Action),
		Action:           p.Action,
		Diff:             diffRaw,
		StatusCode:       int32(p.Status),
		RequestID:        p.RequestID,
		AdditionalFields: p.AdditionalFields,
	}
	err = p.Audit.Export(ctx, auditLog)
	if err != nil {
		p.Log.Error(ctx, "export audit log",
			slog.F("audit_log", auditLog),
			slog.Error(err),
		)
	}
}

type WorkspaceBuildBaggage struct {
	IP string
}

func (b WorkspaceBuildBaggage) Props() ([]baggage.Property, error) {
	ipProp, err := baggage.NewKeyValueProperty("ip", b.IP)
	if err != nil {
		return nil, xerrors.Errorf("create ip kv property: %w", err)
	}

	return []baggage.Property{ipProp}, nil
}

func WorkspaceBuildBaggageFromRequest(r *http.Request) WorkspaceBuildBaggage {
	return WorkspaceBuildBaggage{IP: r.RemoteAddr}
}

type Baggage interface {
	Props() ([]baggage.Property, error)
}

func BaggageToContext(ctx context.Context, d Baggage) (context.Context, error) {
	props, err := d.Props()
	if err != nil {
		return ctx, xerrors.Errorf("create baggage properties: %w", err)
	}

	m, err := baggage.NewMember("audit", "baggage", props...)
	if err != nil {
		return ctx, xerrors.Errorf("create new baggage member: %w", err)
	}

	b, err := baggage.New(m)
	if err != nil {
		return ctx, xerrors.Errorf("create new baggage carrier: %w", err)
	}

	return baggage.ContextWithBaggage(ctx, b), nil
}

func BaggageFromContext(ctx context.Context) WorkspaceBuildBaggage {
	d := WorkspaceBuildBaggage{}
	b := baggage.FromContext(ctx)
	props := b.Member("audit").Properties()
	for _, prop := range props {
		switch prop.Key() {
		case "ip":
			d.IP, _ = prop.Value()
		default:
		}
	}

	return d
}

func either[T Auditable, R any](old, new T, fn func(T) R, auditAction database.AuditAction) R {
	if ResourceID(new) != uuid.Nil {
		return fn(new)
	} else if ResourceID(old) != uuid.Nil {
		return fn(old)
	} else if auditAction == database.AuditActionLogin || auditAction == database.AuditActionLogout {
		// If the request action is a login or logout, we always want to audit it even if
		// there is no diff. See the comment in audit.InitRequest for more detail.
		return fn(old)
	}
	panic("both old and new are nil")
}

func ParseIP(ipStr string) pqtype.Inet {
	ip := net.ParseIP(ipStr)
	ipNet := net.IPNet{}
	if ip != nil {
		ipNet = net.IPNet{
			IP:   ip,
			Mask: net.CIDRMask(len(ip)*8, len(ip)*8),
		}
	}

	return pqtype.Inet{
		IPNet: ipNet,
		Valid: ip != nil,
	}
}

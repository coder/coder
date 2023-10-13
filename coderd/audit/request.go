package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"go.opentelemetry.io/otel/baggage"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/tracing"
)

type RequestParams struct {
	Audit Auditor
	Log   slog.Logger

	Request          *http.Request
	Action           database.AuditAction
	AdditionalFields json.RawMessage
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

type BuildAuditParams[T Auditable] struct {
	Audit Auditor
	Log   slog.Logger

	UserID           uuid.UUID
	JobID            uuid.UUID
	Status           int
	Action           database.AuditAction
	OrganizationID   uuid.UUID
	IP               string
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
	case database.Workspace:
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
	default:
		panic(fmt.Sprintf("unknown resource %T", tgt))
	}
}

func ResourceID[T Auditable](tgt T) uuid.UUID {
	switch typed := any(tgt).(type) {
	case database.Template:
		return typed.ID
	case database.TemplateVersion:
		return typed.ID
	case database.User:
		return typed.ID
	case database.Workspace:
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
	default:
		panic(fmt.Sprintf("unknown resource %T", tgt))
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
	case database.Workspace:
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
	default:
		panic(fmt.Sprintf("unknown resource %T", typed))
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

		if p.AdditionalFields == nil {
			p.AdditionalFields = json.RawMessage("{}")
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

		ip := parseIP(p.Request.RemoteAddr)
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
			AdditionalFields: p.AdditionalFields,
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

// WorkspaceBuildAudit creates an audit log for a workspace build.
// The audit log is committed upon invocation.
func WorkspaceBuildAudit[T Auditable](ctx context.Context, p *BuildAuditParams[T]) {
	ip := parseIP(p.IP)

	diff := Diff(p.Audit, p.Old, p.New)
	var err error
	diffRaw, err := json.Marshal(diff)
	if err != nil {
		p.Log.Warn(ctx, "marshal diff", slog.Error(err))
		diffRaw = []byte("{}")
	}

	if p.AdditionalFields == nil {
		p.AdditionalFields = json.RawMessage("{}")
	}

	auditLog := database.AuditLog{
		ID:               uuid.New(),
		Time:             dbtime.Now(),
		UserID:           p.UserID,
		OrganizationID:   p.OrganizationID,
		Ip:               ip,
		UserAgent:        sql.NullString{},
		ResourceType:     either(p.Old, p.New, ResourceType[T], p.Action),
		ResourceID:       either(p.Old, p.New, ResourceID[T], p.Action),
		ResourceTarget:   either(p.Old, p.New, ResourceTarget[T], p.Action),
		Action:           p.Action,
		Diff:             diffRaw,
		StatusCode:       int32(p.Status),
		RequestID:        p.JobID,
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
	} else {
		panic("both old and new are nil")
	}
}

func parseIP(ipStr string) pqtype.Inet {
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

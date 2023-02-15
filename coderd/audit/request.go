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
	"github.com/tabbed/pqtype"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/tracing"
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

	// This optional field can be passed in when the userID cannot be determined from the API Key
	// such as in the case of login, when the audit log is created prior the API Key's existence.
	UserID uuid.UUID
}

type BuildAuditParams[T Auditable] struct {
	Audit Auditor
	Log   slog.Logger

	UserID           uuid.UUID
	JobID            uuid.UUID
	Status           int
	Action           database.AuditAction
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
		// this isn't used
		return ""
	case database.License:
		return strconv.Itoa(int(typed.ID))
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
	default:
		panic(fmt.Sprintf("unknown resource %T", tgt))
	}
}

func ResourceType[T Auditable](tgt T) database.ResourceType {
	switch any(tgt).(type) {
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
	default:
		panic(fmt.Sprintf("unknown resource %T", tgt))
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
			// because an unknown user fails to login.
			// TODO: introduce the concept of an anonymous user so we always have a userID even
			// when dealing with a mystery user. https://github.com/coder/coder/issues/6054
			if req.params.Action != database.AuditActionLogin && req.params.Action != database.AuditActionLogout {
				return
			}
		}

		var diffRaw = []byte("{}")
		// Only generate diffs if the request succeeded.
		if sw.Status < 400 {
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
		} else {
			userID = req.UserID
		}

		ip := parseIP(p.Request.RemoteAddr)
		auditLog := database.AuditLog{
			ID:               uuid.New(),
			Time:             database.Now(),
			UserID:           userID,
			Ip:               ip,
			UserAgent:        sql.NullString{String: p.Request.UserAgent(), Valid: true},
			ResourceType:     either(req.Old, req.New, ResourceType[T], req.params.Action),
			ResourceID:       either(req.Old, req.New, ResourceID[T], req.params.Action),
			ResourceTarget:   either(req.Old, req.New, ResourceTarget[T], req.params.Action),
			Action:           p.Action,
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

// BuildAudit creates an audit log for a workspace build.
// The audit log is committed upon invocation.
func BuildAudit[T Auditable](ctx context.Context, p *BuildAuditParams[T]) {
	// As the audit request has not been initiated directly by a user, we omit
	// certain user details.
	ip := parseIP("")

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
		Time:             database.Now(),
		UserID:           p.UserID,
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
	exportErr := p.Audit.Export(ctx, auditLog)
	if exportErr != nil {
		p.Log.Error(ctx, "export audit log",
			slog.F("audit_log", auditLog),
			slog.Error(err),
		)
		return
	}
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

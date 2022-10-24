package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"

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
}

func ResourceTarget[T Auditable](tgt T) string {
	switch typed := any(tgt).(type) {
	case database.Organization:
		return typed.Name
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
		return string(typed.BuildNumber)
	case database.GitSSHKey:
		return typed.PublicKey
	case database.Group:
		return typed.Name
	default:
		panic(fmt.Sprintf("unknown resource %T", tgt))
	}
}

func ResourceID[T Auditable](tgt T) uuid.UUID {
	switch typed := any(tgt).(type) {
	case database.Organization:
		return typed.ID
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
	case database.Group:
		return typed.ID
	default:
		panic(fmt.Sprintf("unknown resource %T", tgt))
	}
}

func ResourceType[T Auditable](tgt T) database.ResourceType {
	switch any(tgt).(type) {
	case database.Organization:
		return database.ResourceTypeOrganization
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
	case database.Group:
		return database.ResourceTypeGroup
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
			return
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

		ip := parseIP(p.Request.RemoteAddr)
		err := p.Audit.Export(ctx, database.AuditLog{
			ID:               uuid.New(),
			Time:             database.Now(),
			UserID:           httpmw.APIKey(p.Request).UserID,
			Ip:               ip,
			UserAgent:        p.Request.UserAgent(),
			ResourceType:     either(req.Old, req.New, ResourceType[T]),
			ResourceID:       either(req.Old, req.New, ResourceID[T]),
			ResourceTarget:   either(req.Old, req.New, ResourceTarget[T]),
			Action:           p.Action,
			Diff:             diffRaw,
			StatusCode:       int32(sw.Status),
			RequestID:        httpmw.RequestID(p.Request),
			AdditionalFields: p.AdditionalFields,
		})
		if err != nil {
			p.Log.Error(logCtx, "export audit log", slog.Error(err))
			return
		}
	}
}

func either[T Auditable, R any](old, new T, fn func(T) R) R {
	if ResourceID(new) != uuid.Nil {
		return fn(new)
	} else if ResourceID(old) != uuid.Nil {
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

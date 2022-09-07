package audit

import (
	"context"
	"encoding/json"
	"net"
	"net/http"

	"github.com/google/uuid"
	"github.com/tabbed/pqtype"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
)

type RequestParams struct {
	Audit Auditor
	Log   slog.Logger

	Request        *http.Request
	ResourceID     uuid.UUID
	ResourceTarget string
	Action         database.AuditAction
	ResourceType   database.ResourceType
	Actor          uuid.UUID
}

type Request[T Auditable] struct {
	params *RequestParams

	Old T
	New T
}

// InitRequest initializes an audit log for a request. It returns a function
// that should be deferred, causing the audit log to be committed when the
// handler returns.
func InitRequest[T Auditable](w http.ResponseWriter, p *RequestParams) (*Request[T], func()) {
	sw, ok := w.(*httpapi.StatusWriter)
	if !ok {
		panic("dev error: http.ResponseWriter is not *httpapi.StatusWriter")
	}

	req := &Request[T]{
		params: p,
	}

	return req, func() {
		ctx := context.Background()

		diff := Diff(p.Audit, req.Old, req.New)
		diffRaw, _ := json.Marshal(diff)

		ip, err := parseIP(p.Request.RemoteAddr)
		if err != nil {
			p.Log.Warn(ctx, "parse ip", slog.Error(err))
		}

		err = p.Audit.Export(ctx, database.AuditLog{
			ID:             uuid.New(),
			Time:           database.Now(),
			UserID:         p.Actor,
			Ip:             ip,
			UserAgent:      p.Request.UserAgent(),
			ResourceType:   p.ResourceType,
			ResourceID:     p.ResourceID,
			ResourceTarget: p.ResourceTarget,
			Action:         p.Action,
			Diff:           diffRaw,
			StatusCode:     int32(sw.Status),
			RequestID:      httpmw.RequestID(p.Request),
		})
		if err != nil {
			p.Log.Error(ctx, "export audit log", slog.Error(err))
			return
		}
	}
}

func parseIP(ipStr string) (pqtype.Inet, error) {
	var err error

	ipStr, _, err = net.SplitHostPort(ipStr)
	if err != nil {
		return pqtype.Inet{}, err
	}

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
	}, nil
}

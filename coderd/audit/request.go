package audit

import (
	"context"
	"net/http"

	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd/database"
)

type RequestParams struct {
	Audit Auditor
	Log   slog.Logger

	Action       database.AuditAction
	ResourceType database.ResourceType
	Actor        uuid.UUID
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
	sw, ok := w.(chimw.WrapResponseWriter)
	if !ok {
		panic("dev error: http.ResponseWriter is not chimw.WrapResponseWriter")
	}

	req := &Request[T]{
		params: p,
	}

	return req, func() {
		ctx := context.Background()
		code := sw.Status()

		err := p.Audit.Export(ctx, database.AuditLog{StatusCode: int32(code)})
		if err != nil {
			p.Log.Error(ctx, "export audit log", slog.Error(err))
		}
	}
}

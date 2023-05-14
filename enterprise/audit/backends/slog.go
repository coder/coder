package backends

import (
	"context"

	"github.com/fatih/structs"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/enterprise/audit"
)

type slogBackend struct {
	log slog.Logger
}

func NewSlog(logger slog.Logger) audit.Backend {
	return slogBackend{log: logger}
}

func (slogBackend) Decision() audit.FilterDecision {
	return audit.FilterDecisionExport
}

func (b slogBackend) Export(ctx context.Context, alog database.AuditLog) error {
	// We don't use structs.Map because we don't want to recursively convert
	// fields into maps. When we keep the type information, slog can more
	// pleasantly format the output. For example, the clean result of
	// (*NullString).Value() may be printed instead of {String: "foo", Valid: true}.
	sfs := structs.Fields(alog)
	var fields []slog.Field
	for _, sf := range sfs {
		fields = append(fields, slog.F(sf.Name(), sf.Value()))
	}

	b.log.Info(ctx, "audit_log", fields...)
	return nil
}

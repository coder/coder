package backends

import (
	"context"
	"database/sql"

	"github.com/fatih/structs"
	"github.com/sqlc-dev/pqtype"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/enterprise/audit"
)

type slogBackend struct {
	log slog.Logger
}

func NewSlog(logger slog.Logger) audit.Backend {
	return &slogBackend{log: logger}
}

func (*slogBackend) Decision() audit.FilterDecision {
	return audit.FilterDecisionExport
}

func (b *slogBackend) Export(ctx context.Context, alog database.AuditLog, details audit.BackendDetails) error {
	// We don't use structs.Map because we don't want to recursively convert
	// fields into maps. When we keep the type information, slog can more
	// pleasantly format the output. For example, the clean result of
	// (*NullString).Value() may be printed instead of {String: "foo", Valid: true}.
	sfs := structs.Fields(alog)
	var fields []any
	for _, sf := range sfs {
		fields = append(fields, b.fieldToSlog(sf))
	}

	if details.Actor != nil {
		fields = append(fields, slog.F("actor", details.Actor))
	}

	b.log.Info(ctx, "audit_log", fields...)
	return nil
}

func (*slogBackend) fieldToSlog(field *structs.Field) slog.Field {
	val := field.Value()

	switch ty := field.Value().(type) {
	case pqtype.Inet:
		val = ty.IPNet.IP.String()
	case sql.NullString:
		val = ty.String
	}

	return slog.F(field.Name(), val)
}

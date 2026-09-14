package backends

import (
	"context"
	"database/sql"

	"github.com/fatih/structs"
	"github.com/sqlc-dev/pqtype"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/enterprise/audit"
)

type SlogExporter struct {
	log slog.Logger
}

func NewSlogExporter(logger slog.Logger) *SlogExporter {
	return &SlogExporter{log: logger}
}

func (e *SlogExporter) ExportStruct(ctx context.Context, data any, message string, extraFields ...slog.Field) error {
	// We don't use structs.Map because we don't want to recursively convert
	// fields into maps. When we keep the type information, slog can more
	// pleasantly format the output. For example, the clean result of
	// (*NullString).Value() may be printed instead of {String: "foo", Valid: true}.
	sfs := structs.Fields(data)
	var fields []slog.Field
	for _, sf := range sfs {
		fields = append(fields, e.fieldToSlog(sf))
	}

	fields = append(fields, extraFields...)

	e.log.Info(ctx, message, fields...)
	return nil
}

func (*SlogExporter) fieldToSlog(field *structs.Field) slog.Field {
	val := field.Value()

	switch ty := field.Value().(type) {
	case pqtype.Inet:
		val = ty.IPNet.IP.String()
	case sql.NullString:
		val = ty.String
	}

	return slog.F(field.Name(), val)
}

type auditSlogBackend struct {
	exporter *SlogExporter
}

func NewSlog(logger slog.Logger) audit.Backend {
	return &auditSlogBackend{
		exporter: NewSlogExporter(logger),
	}
}

func (*auditSlogBackend) Decision() audit.FilterDecision {
	return audit.FilterDecisionExport
}

func (b *auditSlogBackend) Export(ctx context.Context, alog database.AuditLog, details audit.BackendDetails) error {
	var extraFields []slog.Field
	if details.Actor != nil {
		extraFields = append(extraFields, slog.F("actor", details.Actor))
	}

	return b.exporter.ExportStruct(ctx, alog, "audit_log", extraFields...)
}

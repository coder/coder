package audit

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/database"
)

// Backends can store or send audit logs to arbitrary locations.
type Backend interface {
	// Decision determines the FilterDecisions that the backend tolerates.
	Decision() FilterDecision
	// Export sends an audit log to the backend.
	Export(ctx context.Context, alog database.AuditLog) error
}

// Exporter exports audit logs to an arbitrary list of backends.
type Exporter struct {
	filter   Filter
	backends []Backend
}

// NewExporter creates an exporter from the given filter and backends.
func NewExporter(filter Filter, backends ...Backend) *Exporter {
	return &Exporter{
		filter:   filter,
		backends: backends,
	}
}

// Export exports and audit log. Before exporting to a backend, it uses the
// filter to determine if the backend tolerates the audit log. If not, it is
// dropped.
func (e *Exporter) Export(ctx context.Context, alog database.AuditLog) error {
	decision, err := e.filter.Check(ctx, alog)
	if err != nil {
		return xerrors.Errorf("filter check: %w", err)
	}

	for _, backend := range e.backends {
		if decision&backend.Decision() != backend.Decision() {
			continue
		}

		err = backend.Export(ctx, alog)
		if err != nil {
			// naively return the first error. should probably make this smarter
			// by returning multiple errors.
			return xerrors.Errorf("export audit log to backend: %w", err)
		}
	}
	return nil
}

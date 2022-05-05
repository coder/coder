package audit

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/database"
)

// Backends can accept and store or send audit logs elsewhere.
type Backend interface {
	// Decision determines the FilterDecisions that the backend tolerates.
	Decision() FilterDecision
	// Export sends an audit log to the backend.
	Export(ctx context.Context, alog database.AuditLog) error
}

// Exporter exports audit logs to an arbitrary amount of backends.
type Exporter struct {
	filter   Filter
	backends []Backend
}

func NewExporter(filter Filter, backends ...Backend) *Exporter {
	return &Exporter{
		filter:   filter,
		backends: backends,
	}
}

func (e *Exporter) Export(ctx context.Context, alog database.AuditLog) error {
	for _, backend := range e.backends {
		decision, err := e.filter.Check(ctx, alog)
		if err != nil {
			return xerrors.Errorf("filter check: %w", err)
		}

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

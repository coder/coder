package audit

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/audit"
	"github.com/coder/coder/coderd/database"
)

// Backends can store or send audit logs to arbitrary locations.
type Backend interface {
	// Decision determines the FilterDecisions that the backend tolerates.
	Decision() FilterDecision
	// Export sends an audit log to the backend.
	Export(ctx context.Context, alog database.AuditLog) error
}

func NewAuditor(filter Filter, backends ...Backend) audit.Auditor {
	return &auditor{
		filter:   filter,
		backends: backends,
		Differ: audit.Differ{DiffFn: func(old, new any) audit.Map {
			return diffValues(old, new, AuditableResources)
		}},
	}
}

// auditor is the enterprise implementation of the Auditor interface.
type auditor struct {
	filter   Filter
	backends []Backend

	audit.Differ
}

func (a *auditor) Export(ctx context.Context, alog database.AuditLog) error {
	decision, err := a.filter.Check(ctx, alog)
	if err != nil {
		return xerrors.Errorf("filter check: %w", err)
	}

	for _, backend := range a.backends {
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

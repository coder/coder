package audit

import (
	"context"

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

func NewAuditor() audit.Auditor {
	return &auditor{
		Differ: audit.Differ{DiffFn: func(old, new any) audit.Map {
			return diffValues(old, new, AuditableResources)
		}},
	}
}

// auditor is the enterprise implementation of the Auditor interface.
type auditor struct {
	//nolint:unused
	filter Filter
	//nolint:unused
	backends []Backend

	audit.Differ
}

//nolint:unused
func (*auditor) Export(context.Context, database.AuditLog) error {
	panic("not implemented") // TODO: Implement
}

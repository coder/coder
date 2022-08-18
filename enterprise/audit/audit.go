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

// auditor is the enterprise impelentation of the Auditor interface.
type auditor struct {
	//nolint:unused
	filter Filter
	//nolint:unused
	backends []Backend
}

//nolint:unused
func (*auditor) Export(context.Context, database.AuditLog) error {
	panic("not implemented") // TODO: Implement
}

//nolint:unused
func (*auditor) diff(any, any) audit.Map {
	panic("not implemented") // TODO: Implement
}

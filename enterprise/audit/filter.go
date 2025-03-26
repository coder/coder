package audit

import (
	"context"

	"github.com/coder/coder/v2/coderd/database"
)

// FilterDecision is a bitwise flag describing the actions a given filter allows
// for a given audit log.
type FilterDecision uint8

const (
	// FilterDecisionDrop indicates that the audit log should be dropped. It
	// should not be stored or exported anywhere.
	FilterDecisionDrop FilterDecision = 0
	// FilterDecisionStore indicates that the audit log should be allowed to be
	// stored in the Coder database.
	FilterDecisionStore FilterDecision = 1 << iota
	// FilterDecisionExport indicates that the audit log should be exported
	// externally of Coder.
	FilterDecisionExport
)

// Filters produce a FilterDecision for a given audit log.
type Filter interface {
	Check(ctx context.Context, alog database.AuditLog) (FilterDecision, error)
}

// DefaultFilter is the default filter used when exporting audit logs. It allows
// storage and exporting for all audit logs.
var DefaultFilter Filter = FilterFunc(func(_ context.Context, _ database.AuditLog) (FilterDecision, error) {
	// Store and export all audit logs for now.
	return FilterDecisionStore | FilterDecisionExport, nil
})

// FilterFunc constructs a Filter from a simple function.
type FilterFunc func(ctx context.Context, alog database.AuditLog) (FilterDecision, error)

func (f FilterFunc) Check(ctx context.Context, alog database.AuditLog) (FilterDecision, error) {
	return f(ctx, alog)
}

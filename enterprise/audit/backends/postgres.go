package backends
import (
	"fmt"
	"errors"
	"context"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/enterprise/audit"
)
type postgresBackend struct {
	// internal indicates if the exporter is exporting to the Postgres database
	// that the rest of Coderd uses. Since this is a generic Postgres exporter,
	// we make different decisions to store the audit log based on if it's
	// pointing to the Coderd database.
	internal bool
	db       database.Store
}
func NewPostgres(db database.Store, internal bool) audit.Backend {
	return &postgresBackend{db: db, internal: internal}
}
func (b *postgresBackend) Decision() audit.FilterDecision {
	if b.internal {
		return audit.FilterDecisionStore
	}
	return audit.FilterDecisionExport
}
func (b *postgresBackend) Export(ctx context.Context, alog database.AuditLog, _ audit.BackendDetails) error {
	_, err := b.db.InsertAuditLog(ctx, database.InsertAuditLogParams(alog))
	if err != nil {
		return fmt.Errorf("insert audit log: %w", err)
	}
	return nil
}

package backends

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/audit"
	"github.com/coder/coder/coderd/database"
)

type pgBackend struct {
	// internal indicates if the exporter is exporting to the Postgres database
	// that the rest of Coderd uses. Since this is a generic Postgres exporter,
	// we make different decisions to store the audit log based on if it's
	// pointing to the Coderd database.
	internal bool
	db       database.Store
}

func NewPGBackend(db database.Store, internal bool) audit.Backend {
	return &pgBackend{db: db, internal: internal}
}

func (b *pgBackend) Decision() audit.FilterDecision {
	if b.internal {
		return audit.FilterDecisionStore
	}

	return audit.FilterDecisionExport
}

func (b *pgBackend) Export(ctx context.Context, alog database.AuditLog) error {
	_, err := b.db.InsertAuditLog(ctx, database.InsertAuditLogParams{
		ID:             alog.ID,
		Time:           alog.Time,
		UserID:         alog.UserID,
		OrganizationID: alog.OrganizationID,
		Ip:             alog.Ip,
		UserAgent:      alog.UserAgent,
		ResourceType:   alog.ResourceType,
		ResourceID:     alog.ResourceID,
		ResourceTarget: alog.ResourceTarget,
		Action:         alog.Action,
		Diff:           alog.Diff,
		StatusCode:     alog.StatusCode,
	})
	if err != nil {
		return xerrors.Errorf("insert audit log: %w", err)
	}

	return nil
}

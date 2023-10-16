package audit

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
)

type BackendDetails struct {
	Actor *Actor
}

type Actor struct {
	ID       uuid.UUID `json:"id"`
	Email    string    `json:"email"`
	Username string    `json:"username"`
}

// Backends can store or send audit logs to arbitrary locations.
type Backend interface {
	// Decision determines the FilterDecisions that the backend tolerates.
	Decision() FilterDecision
	// Export sends an audit log to the backend.
	Export(ctx context.Context, alog database.AuditLog, details BackendDetails) error
}

func NewAuditor(db database.Store, filter Filter, backends ...Backend) audit.Auditor {
	return &auditor{
		db:       db,
		filter:   filter,
		backends: backends,
		Differ: audit.Differ{DiffFn: func(old, new any) audit.Map {
			return diffValues(old, new, AuditableResources)
		}},
	}
}

// auditor is the enterprise implementation of the Auditor interface.
type auditor struct {
	db       database.Store
	filter   Filter
	backends []Backend

	audit.Differ
}

func (a *auditor) Export(ctx context.Context, alog database.AuditLog) error {
	decision, err := a.filter.Check(ctx, alog)
	if err != nil {
		return xerrors.Errorf("filter check: %w", err)
	}

	actor, err := a.db.GetUserByID(dbauthz.AsSystemRestricted(ctx), alog.UserID) //nolint
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		return err
	}

	for _, backend := range a.backends {
		if decision&backend.Decision() != backend.Decision() {
			continue
		}

		err = backend.Export(ctx, alog, BackendDetails{Actor: &Actor{
			ID:       actor.ID,
			Email:    actor.Email,
			Username: actor.Username,
		}})
		if err != nil {
			// naively return the first error. should probably make this smarter
			// by returning multiple errors.
			return xerrors.Errorf("export audit log to backend: %w", err)
		}
	}

	return nil
}

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
	Actor      *Actor
	OnBehalfOf *Actor
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
		Differ: audit.Differ{DiffFn: func(old, newVal any) audit.Map {
			return diffValues(old, newVal, AuditableResources)
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

	// AsSystemRestricted is used to look up the actor name even
	// when the caller lacks read access to the user.
	actor, err := a.db.GetUserByID(dbauthz.AsSystemRestricted(ctx), alog.UserID) //nolint:gocritic // see above
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		return err
	}

	var onBehalfOf *Actor
	if alog.OnBehalfOfUserID.Valid {
		// Delegated audit details must remain available even when the caller
		// cannot read the human owner directly.
		owner, ownerErr := a.db.GetUserByID(dbauthz.AsSystemRestricted(ctx), alog.OnBehalfOfUserID.UUID) //nolint:gocritic
		if ownerErr != nil && !xerrors.Is(ownerErr, sql.ErrNoRows) {
			return ownerErr
		}
		if ownerErr == nil {
			onBehalfOf = &Actor{
				ID:       owner.ID,
				Email:    owner.Email,
				Username: owner.Username,
			}
		}
	}

	for _, backend := range a.backends {
		if decision&backend.Decision() != backend.Decision() {
			continue
		}

		err = backend.Export(ctx, alog, BackendDetails{
			Actor: &Actor{
				ID:       actor.ID,
				Email:    actor.Email,
				Username: actor.Username,
			},
			OnBehalfOf: onBehalfOf,
		})
		if err != nil {
			// naively return the first error. should probably make this smarter
			// by returning multiple errors.
			return xerrors.Errorf("export audit log to backend: %w", err)
		}
	}

	return nil
}

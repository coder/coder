package workspaceexec

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/workspaceartifacts"
)

var (
	// ErrInvalidArtifactExpiry requires an explicit extension beyond now, the lease, and the current finite expiry.
	ErrInvalidArtifactExpiry = xerrors.New("artifact recovery requires an explicit future artifact_expires_at later than the lease and current finite expiry; indefinite retention cannot be shortened")
	// ErrInvalidRenewal requires an explicit lease later than both now and the current lease.
	ErrInvalidRenewal = xerrors.New("renewal must extend the lease into the future")
	// ErrNotRetryable means there is no failed preservation or deletion to retry.
	ErrNotRetryable = xerrors.New("session has no retryable failure")
	// ErrSessionChanged requires observing the current revision before retrying.
	ErrSessionChanged = xerrors.New("workspace execution session changed")
	// ErrCleanupCommitted means a delete build already owns this workspace.
	ErrCleanupCommitted = xerrors.New("workspace deletion has already been committed")
)

func saveSession(ctx context.Context, tx database.Store, before, after database.WorkspaceExecutionSession, now time.Time) (database.WorkspaceExecutionSession, error) {
	return tx.UpdateWorkspaceExecutionSession(ctx, database.UpdateWorkspaceExecutionSessionParams{
		ID: before.ID, ExpectedRevision: before.Revision, ExpectedState: before.State,
		State: after.State, Retained: after.Retained, LeaseExpiresAt: after.LeaseExpiresAt, Revision: after.Revision,
		DeleteBuildID: after.DeleteBuildID, NextRetryAt: after.NextRetryAt, AttemptCount: after.AttemptCount,
		Error: after.Error, UpdatedAt: dbtime.Time(now), RecoveryArtifactExpiresAt: after.RecoveryArtifactExpiresAt,
	})
}

func lockSession(ctx context.Context, tx database.Store, expected database.WorkspaceExecutionSession) (database.WorkspaceExecutionSession, database.Workspace, error) {
	if !expected.WorkspaceID.Valid {
		return expected, database.Workspace{}, ErrAdmissionClosed
	}
	if err := tx.AcquireLock(ctx, database.WorkspaceLifecycleLockID(expected.WorkspaceID.UUID)); err != nil {
		return expected, database.Workspace{}, err
	}
	if err := tx.LockWorkspaceExecutionWorkspace(ctx, expected.WorkspaceID.UUID); err != nil {
		return expected, database.Workspace{}, err
	}
	current, err := tx.GetWorkspaceExecutionSessionByID(ctx, expected.ID)
	if err != nil {
		return current, database.Workspace{}, err
	}
	workspace, err := tx.GetWorkspaceByID(ctx, expected.WorkspaceID.UUID)
	if err != nil {
		return current, workspace, err
	}
	if current.WorkspaceID != expected.WorkspaceID || current.OrganizationID != expected.OrganizationID ||
		current.WorkspaceOwnerID != expected.WorkspaceOwnerID || current.OwnerID != expected.OwnerID ||
		workspace.OwnerID != current.WorkspaceOwnerID.UUID || workspace.OrganizationID != current.OrganizationID {
		return current, workspace, ErrSessionChanged
	}
	return current, workspace, nil
}

func mutateSession(ctx context.Context, db database.Store, id uuid.UUID, revision int64, now time.Time, change func(database.Store, *database.WorkspaceExecutionSession) error) (database.WorkspaceExecutionSession, error) {
	expected, err := db.GetWorkspaceExecutionSessionByID(ctx, id)
	if err != nil {
		return expected, err
	}
	var result database.WorkspaceExecutionSession
	err = db.InTx(func(tx database.Store) error {
		current, workspace, err := lockSession(ctx, tx, expected)
		if err != nil {
			return err
		}
		if current.Revision != revision {
			return ErrSessionChanged
		}
		if current.State == "deleting" || current.State == "completed" || workspace.Deleted {
			return ErrCleanupCommitted
		}
		//nolint:gocritic // The requested session mutation is already authorized.
		committed, err := HasCommittedDeletion(dbauthz.AsSystemRestricted(ctx), tx, workspace.ID)
		if err != nil {
			return err
		}
		if committed {
			return ErrCleanupCommitted
		}
		updated := current
		if err := change(tx, &updated); err != nil {
			return err
		}
		result, err = saveSession(ctx, tx, current, updated, now)
		return err
	}, nil)
	return result, err
}

// Renew extends an open session's explicit lease without modifying retention.
// It cannot reopen preservation or deletion after the admission fence closes.
func Renew(ctx context.Context, db database.Store, id uuid.UUID, revision int64, expiresAt, now time.Time) (database.WorkspaceExecutionSession, error) {
	return mutateSession(ctx, db, id, revision, now, func(tx database.Store, session *database.WorkspaceExecutionSession) error {
		if err := CheckAdmission(ctx, tx, session.WorkspaceID.UUID); err != nil {
			return err
		}
		if session.State != "active" && session.State != "retained" {
			return ErrAdmissionClosed
		}
		expiresAt = dbtime.Time(expiresAt.UTC())
		if !expiresAt.After(now) || !expiresAt.After(session.LeaseExpiresAt) {
			return ErrInvalidRenewal
		}
		session.LeaseExpiresAt = expiresAt
		session.Revision++
		session.NextRetryAt = sql.NullTime{}
		return nil
	})
}

// Retain explicitly protects a workspace and invalidates an in-flight
// preservation claim. A committed delete build cannot be undone by retention.
func Retain(ctx context.Context, db database.Store, id uuid.UUID, revision int64, now time.Time) (database.WorkspaceExecutionSession, error) {
	return mutateSession(ctx, db, id, revision, now, func(tx database.Store, session *database.WorkspaceExecutionSession) error {
		//nolint:gocritic // This guard observes sibling lifecycle state after authorizing the session mutation.
		others, err := tx.GetOtherWorkspaceExecutionSessionsByWorkspaceID(dbauthz.AsSystemRestricted(ctx), database.GetOtherWorkspaceExecutionSessionsByWorkspaceIDParams{WorkspaceID: session.WorkspaceID, ID: session.ID})
		if err != nil {
			return err
		}
		for _, other := range others {
			if other.State == "deleting" {
				return ErrCleanupCommitted
			}
		}
		session.Retained = true
		switch session.State {
		case "active", "retained":
			session.State = "retained"
			session.Revision++
		case "preserving":
			// Invalidate collection without reopening the closed workspace.
			session.State = "preservation_failed"
			session.Revision++
		}
		// A settled generation keeps its revision so retention cannot detach
		// already preserved bytes or reopen admission.
		session.Error = ""
		session.NextRetryAt = sql.NullTime{}
		return nil
	})
}

// RetryWithArtifactExpiry explicitly extends artifact retention for a new closed
// preservation generation. Original declarations and prior generations remain
// immutable. A nil expiry retries the current generation before its expiry.
func RetryWithArtifactExpiry(ctx context.Context, db database.Store, id uuid.UUID, revision int64, expiresAt *time.Time, now time.Time) (database.WorkspaceExecutionSession, error) {
	return mutateSession(ctx, db, id, revision, now, func(tx database.Store, session *database.WorkspaceExecutionSession) error {
		if session.State != "preservation_failed" && session.State != "deletion_failed" && session.State != "preserved" {
			return ErrNotRetryable
		}
		effective, err := workspaceartifacts.EffectiveExpiry(*session)
		if err != nil {
			return err
		}
		if session.State == "preserved" && (!effective.Valid || effective.Time.After(now)) {
			return ErrNotRetryable
		}
		if expiresAt == nil && effective.Valid && !effective.Time.After(now) {
			return ErrInvalidArtifactExpiry
		}
		if expiresAt != nil {
			normalized := dbtime.Time(*expiresAt)
			expiresAt = &normalized
			// Indefinite retention cannot be extended by selecting a finite time.
			if !effective.Valid || !expiresAt.After(now) || !expiresAt.After(session.LeaseExpiresAt) || !expiresAt.After(effective.Time) {
				return ErrInvalidArtifactExpiry
			}
		}
		if expiresAt != nil {
			//nolint:gocritic // Internal activity guards disclose no other session data.
			internal := dbauthz.AsSystemRestricted(ctx)
			pending, err := tx.HasPendingWorkspaceExecutionReceiptsByWorkspaceID(internal, session.WorkspaceID)
			if err != nil {
				return err
			}
			busy, err := tx.HasBusyWorkspaceExecutionChats(internal, session.WorkspaceID)
			if err != nil {
				return err
			}
			if pending || busy {
				return ErrAdmissionClosed
			}
			session.Revision++
			session.State = "preservation_failed"
			session.RecoveryArtifactExpiresAt = sql.NullTime{Time: *expiresAt, Valid: true}
		}
		session.NextRetryAt = sql.NullTime{}
		session.Error = ""
		return nil
	})
}

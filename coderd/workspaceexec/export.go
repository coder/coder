package workspaceexec

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/workspaceartifacts"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// Export closes admission during collection and preserves this session's declared results.
// Successful adopted-workspace export releases workspace admission while keeping
// the exported session and its artifact generation immutable.
// It does not change retention or schedule deletion. Repeating the same observed
// revision recovers the same immutable generation after a lost response.
func (c *Controller) Export(ctx context.Context, id uuid.UUID, revision int64) ([]database.GetWorkspaceExecutionArtifactsBySessionIDRow, error) {
	expected, err := c.Database.GetWorkspaceExecutionSessionByID(ctx, id)
	if err != nil {
		return nil, err
	}
	var session database.WorkspaceExecutionSession
	done := false
	err = c.Database.InTx(func(tx database.Store) error {
		current, workspace, err := lockSession(ctx, tx, expected)
		if err != nil {
			return err
		}
		if (current.Revision == revision || current.Revision == revision+1) && (current.State == "preserved" || current.State == "completed") {
			session, done = current, true
			return nil
		}
		if workspace.Deleted {
			return ErrCleanupCommitted
		}
		next := current
		switch {
		case current.Revision == revision && (current.State == "active" || current.State == "retained"):

			next.Revision++
		case current.Revision == revision && current.State == "preservation_failed":
			// Explicit artifact recovery already claimed this closed generation.
		case current.Revision == revision+1 && (current.State == "preserving" || current.State == "preservation_failed"):
		default:
			return ErrSessionChanged
		}
		//nolint:gocritic // Internal activity guards disclose no other session data.
		internal := dbauthz.AsSystemRestricted(ctx)
		committed, err := HasCommittedDeletion(internal, tx, workspace.ID)
		if err != nil {
			return err
		}
		if committed {
			return ErrCleanupCommitted
		}
		others, err := tx.GetOtherWorkspaceExecutionSessionsByWorkspaceID(internal, database.GetOtherWorkspaceExecutionSessionsByWorkspaceIDParams{WorkspaceID: current.WorkspaceID, ID: current.ID})
		if err != nil {
			return err
		}
		for _, other := range others {
			if other.State == "deleting" {
				return ErrCleanupCommitted
			}
		}
		pending, err := tx.HasPendingWorkspaceExecutionReceiptsByWorkspaceID(internal, current.WorkspaceID)
		if err != nil {
			return err
		}
		busy, err := tx.HasBusyWorkspaceExecutionChats(internal, current.WorkspaceID)
		if err != nil {
			return err
		}
		if pending || busy {
			return xerrors.Errorf("%w: workspace work must settle before export", ErrAdmissionClosed)
		}
		next.State, next.Error, next.NextRetryAt = "preserving", "", sql.NullTime{}
		session, err = saveSession(ctx, tx, current, next, c.Clock.Now())
		return err
	}, nil)
	if err != nil {
		return nil, err
	}
	if done {
		rows, err := c.Database.GetWorkspaceExecutionArtifactsBySessionID(ctx, session.ID)
		if err != nil {
			return nil, err
		}
		result := make([]database.GetWorkspaceExecutionArtifactsBySessionIDRow, 0, len(rows))
		for _, row := range rows {
			if row.PreservationRevision == session.Revision {
				result = append(result, row)
			}
		}
		return result, nil
	}
	artifacts, err := workspaceartifacts.Preserve(ctx, c.Database, controllerBundler(func(ctx context.Context, request workspacesdk.BundleFilesRequest) ([]byte, error) {
		return c.collect(ctx, session, request)
	}), session, c.Clock.Now())
	if err != nil {
		if persistErr := c.fail(ctx, session, "preservation_failed", err); persistErr != nil {
			return nil, xerrors.Errorf("preserve results: %v; record preservation failure: %w", err, persistErr)
		}
		return nil, err
	}
	err = c.Database.InTx(func(tx database.Store) error {
		current, _, err := lockSession(ctx, tx, session)
		if err != nil {
			return err
		}
		if current.Revision != session.Revision {
			return ErrSessionChanged
		}
		if current.State == "preserved" || current.State == "completed" {
			return nil
		}
		if current.State != "preserving" {
			return ErrSessionChanged
		}
		next := current
		next.State, next.Error = "preserved", ""
		_, err = saveSession(ctx, tx, current, next, c.Clock.Now())
		return err
	}, nil)
	return artifacts, err
}
